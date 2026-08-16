// Package vterm runs a child command on a pseudo-terminal and mirrors its
// screen through an in-process VT emulator, so a TUI can draw the child as a
// widget instead of handing the whole terminal over to it.
//
// A [Session] owns three things: the PTY the child is attached to, the
// emulator that interprets what the child writes, and two goroutines. The read
// pump moves bytes from the PTY into the emulator; the reply pump moves the
// emulator's answers to the child's queries (DA1, DA2, DSR/CPR, DECRPM) back
// out to the PTY. The reply pump is not optional: the emulator writes those
// answers into an internal pipe, and a child that queries the terminal on
// startup stalls forever if nothing drains it.
//
// Everything that touches the emulator does so under the Session's own mutex.
// vt.SafeEmulator has a lock of its own, but an upstream fix for a race in it
// was merged and then reverted, so it is not trusted alone.
package vterm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/charmbracelet/x/xpty"
)

// readBufSize is the size of the read pump's buffer.
const readBufSize = 32 * 1024

// reapTimeout bounds how long a child is given to exit once its side of the
// PTY has gone, before it is killed. A var so tests need not wait it out.
var reapTimeout = 2 * time.Second

// Pty is the slice of a pseudo-terminal a Session uses. xpty.Pty satisfies it;
// tests supply a fake.
type Pty interface {
	io.ReadWriteCloser

	// Start starts a command on the PTY, with the command's standard input,
	// output and error connected to it.
	Start(cmd *exec.Cmd) error

	// Resize resizes the PTY.
	Resize(width, height int) error
}

// newPty opens a pseudo-terminal of the given size. A package var so tests can
// substitute a fake.
var newPty = func(cols, rows int) (Pty, error) {
	return hangupOn(xpty.NewPty(cols, rows))
}

// hangupOn adapts the result of opening an xpty PTY into this package's own
// interface.
func hangupOn(pty xpty.Pty, err error) (Pty, error) {
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped by the caller
	}
	return &hangupPty{Pty: pty}, nil
}

// hangupPty is an xpty PTY that drops the parent's copy of the child end once
// the child holds one of its own.
//
// xpty keeps both ends open for the PTY's lifetime. With the child end still
// open here, a read of the parent end never ends — not even once the child has
// exited — because this process is itself a writer. Closing it is what makes
// the read report EOF, or EIO on darwin, when the child goes away.
type hangupPty struct {
	xpty.Pty
}

func (p *hangupPty) Start(cmd *exec.Cmd) error {
	if err := p.Pty.Start(cmd); err != nil {
		return err //nolint:wrapcheck // wrapped by the caller
	}
	if unix, ok := p.Pty.(*xpty.UnixPty); ok {
		// The child has its own descriptors for this by now; Close below
		// closes it a second time and says so, which is not worth reporting.
		_ = unix.Slave().Close()
	}
	return nil
}

// waitProcess reaps a started command. A package var so tests can substitute a
// fake. xpty's own wrapper is used because on Windows cmd.Wait does not work
// with ConPTY.
var waitProcess = xpty.WaitProcess

// Session is a child command running on a pseudo-terminal, with its screen
// mirrored by an in-process terminal emulator.
type Session struct {
	pty Pty
	cmd *exec.Cmd

	// mu guards every use of emu. See the package comment.
	mu  sync.Mutex
	emu *vt.SafeEmulator

	// cursorVisible tracks DECTCEM (mode 25). The emulator reports it through
	// a callback fired from inside a write, so it cannot live under mu.
	cursorVisible atomic.Bool

	output chan struct{}
	done   chan struct{}

	closeOnce sync.Once

	errMu sync.Mutex
	err   error
}

// Start runs cmd on a new pseudo-terminal of cols by rows cells and returns a
// Session mirroring its screen. The caller owns the returned Session and must
// [Session.Close] it.
func Start(cmd *exec.Cmd, cols, rows int) (*Session, error) {
	cols, rows = clampSize(cols, rows)

	pty, err := newPty(cols, rows)
	if err != nil {
		return nil, fmt.Errorf("vterm: open pty: %w", err)
	}

	s := &Session{
		pty:    pty,
		cmd:    cmd,
		emu:    vt.NewSafeEmulator(cols, rows),
		output: make(chan struct{}, 1),
		done:   make(chan struct{}),
	}
	s.cursorVisible.Store(true)
	s.emu.SetCallbacks(vt.Callbacks{
		CursorVisibility: s.cursorVisible.Store,
	})

	if err := pty.Start(cmd); err != nil {
		_ = pty.Close()
		return nil, fmt.Errorf("vterm: start command: %w", err)
	}

	// The reply pump goes first: it must already be draining the emulator's
	// input pipe before the read pump feeds the child's first startup query
	// into it, or the answer has nowhere to go and both sides stall.
	go s.replyPump()
	go s.readPump()

	return s, nil
}

// clampSize keeps a size at one cell or more in each direction; the emulator
// and the PTY both reject less.
func clampSize(cols, rows int) (int, int) {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	return cols, rows
}

// replyPump copies the emulator's answers to the child's queries out to the
// PTY. It ends when the read pump stops the emulator's input pipe.
func (s *Session) replyPump() {
	_, _ = io.Copy(s.pty, s.emu)
}

// readPump moves bytes from the PTY into the emulator until the child's side
// of the PTY goes away, then reaps the child and marks the Session done.
func (s *Session) readPump() {
	defer close(s.done)

	buf := make([]byte, readBufSize)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			s.mu.Lock()
			_, _ = s.emu.Write(buf[:n])
			s.mu.Unlock()
			s.notify()
		}
		if err != nil {
			if !normalExit(err) {
				s.setErr(err)
			}
			break
		}
	}

	stopInput(s.emu.InputPipe())
	s.reap()
}

// stopInput ends the emulator's input pipe, so the reply pump blocked on the
// far end of it comes back with io.EOF.
//
// The emulator's own Close would do this too, but it also sets an unguarded
// flag that the reply pump reads on every pass — a data race, and one this
// package cannot fix from outside.
func stopInput(w io.Writer) {
	if pw, ok := w.(*io.PipeWriter); ok {
		_ = pw.CloseWithError(io.EOF)
	}
}

// normalExit reports whether err is how a PTY ordinarily reports that the
// child has gone: EOF, EIO once the child's side is closed (darwin), or a use
// of the PTY after Close raced a blocked read.
func normalExit(err error) bool {
	return errors.Is(err, io.EOF) ||
		errors.Is(err, syscall.EIO) ||
		errors.Is(err, os.ErrClosed)
}

// reap waits for the child to exit, and kills it if it will not within
// reapTimeout of its PTY going quiet. A Session over a fake PTY may have no
// started process at all.
func (s *Session) reap() {
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), reapTimeout)
	defer cancel()

	waited := make(chan struct{})
	go func() {
		defer close(waited)
		_ = waitProcess(ctx, s.cmd)
	}()

	select {
	case <-waited:
	case <-ctx.Done():
		_ = s.cmd.Process.Kill()
		<-waited
	}
}

// notify announces that the screen may have changed, without ever blocking the
// read pump: the channel holds one pending notification and further ones
// coalesce into it.
func (s *Session) notify() {
	select {
	case s.output <- struct{}{}:
	default:
	}
}

func (s *Session) setErr(err error) {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	s.err = err
}

// Output fires whenever the child has written something and the screen may
// need redrawing. Notifications coalesce, so a receive means "at least one
// write happened", never how many.
func (s *Session) Output() <-chan struct{} { return s.output }

// Done is closed once the child's output has ended and the child is reaped.
func (s *Session) Done() <-chan struct{} { return s.done }

// Err reports why the session ended abnormally: a failure to read the PTY that
// is not one of the ways a child ordinarily goes away. It is nil for an
// ordinary exit — including a non-zero one — and meaningful only once
// [Session.Done] is closed.
func (s *Session) Err() error {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.err
}

// Render returns a snapshot of the child's screen, one line per row, with
// styles and links encoded as ANSI escape sequences.
//
// This is the only way the screen is read. The emulator's Draw and Touched
// report damage since the last draw, and that list goes nil across a resize.
func (s *Session) Render() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.emu.Render()
}

// Cursor returns the cursor's cell and whether the child has it shown.
func (s *Session) Cursor() (x, y int, visible bool) {
	s.mu.Lock()
	pos := s.emu.CursorPosition()
	s.mu.Unlock()
	return pos.X, pos.Y, s.cursorVisible.Load()
}

// Resize resizes both the emulator and the PTY, so the child is told the new
// size and its output is interpreted at it.
func (s *Session) Resize(cols, rows int) error {
	cols, rows = clampSize(cols, rows)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.emu.Resize(cols, rows)
	if err := s.pty.Resize(cols, rows); err != nil {
		return fmt.Errorf("vterm: resize pty: %w", err)
	}
	return nil
}

// SendKey sends a key press to the child, encoded as the child's current modes
// ask for it (DECCKM, the Kitty keyboard protocol, and so on).
func (s *Session) SendKey(key uv.KeyPressEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emu.SendKey(key)
}

// SendBytes sends raw bytes to the child.
//
// They go through the emulator's input pipe rather than to the PTY directly,
// so they stay in order with what [Session.SendKey], [Session.Paste] and the
// emulator's own query replies write.
func (s *Session) SendBytes(p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.emu.InputPipe().Write(p)
}

// Paste sends text to the child, bracketed iff the child has turned bracketed
// paste mode on.
func (s *Session) Paste(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emu.Paste(text)
}

// Close ends the session. It closes the PTY, which unblocks the read pump and
// in turn ends the reply pump and reaps the child. It does not wait for any of
// that — [Session.Done] reports when it has happened — and is safe to call
// more than once, and after the child has already exited.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		_ = s.pty.Close()
	})
}
