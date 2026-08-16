package vterm

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
)

// fakePty is a Pty whose child side the test drives: feed hands bytes to the
// read pump, finish ends the stream with a chosen error, and everything the
// session writes back is recorded.
type fakePty struct {
	// in carries fed bytes to the read pump; fin and shut end its reads, with
	// the error a real PTY would report in each case.
	in   chan []byte
	fin  chan struct{}
	shut chan struct{}

	finErr  error // written before fin is closed
	pending []byte

	mu      sync.Mutex
	written []byte
	sizes   [][2]int
	closes  int
	started []*exec.Cmd

	startErr  error
	resizeErr error
	realStart bool
}

func newFakePty() *fakePty {
	return &fakePty{
		in:   make(chan []byte),
		fin:  make(chan struct{}),
		shut: make(chan struct{}),
	}
}

// Read is only ever called from the read pump, so pending needs no lock.
func (f *fakePty) Read(p []byte) (int, error) {
	if len(f.pending) == 0 {
		select {
		case b := <-f.in:
			f.pending = b
		case <-f.fin:
			return 0, f.finErr
		case <-f.shut:
			return 0, os.ErrClosed
		}
	}
	n := copy(p, f.pending)
	f.pending = f.pending[n:]
	return n, nil
}

func (f *fakePty) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.written = append(f.written, p...)
	return len(p), nil
}

func (f *fakePty) Close() error {
	f.mu.Lock()
	first := f.closes == 0
	f.closes++
	f.mu.Unlock()
	if first {
		// A read blocked when the PTY is closed comes back with os.ErrClosed
		// on a real PTY too.
		close(f.shut)
	}
	return nil
}

func (f *fakePty) Start(cmd *exec.Cmd) error {
	f.mu.Lock()
	f.started = append(f.started, cmd)
	realStart, err := f.realStart, f.startErr
	f.mu.Unlock()
	if err != nil {
		return err
	}
	if realStart {
		return cmd.Start()
	}
	return nil
}

func (f *fakePty) Resize(width, height int) error {
	f.mu.Lock()
	f.sizes = append(f.sizes, [2]int{width, height})
	err := f.resizeErr
	f.mu.Unlock()
	return err
}

// feed hands bytes to the read pump, blocking until it has taken them.
func (f *fakePty) feed(t *testing.T, s string) {
	t.Helper()
	select {
	case f.in <- []byte(s):
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out feeding %q", s)
	}
}

// finish ends the child's output with err.
func (f *fakePty) finish(err error) {
	f.finErr = err
	close(f.fin)
}

func (f *fakePty) out() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return string(f.written)
}

func (f *fakePty) resetOut() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.written = nil
}

func (f *fakePty) snapshot() (sizes [][2]int, closes int, started []*exec.Cmd) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][2]int(nil), f.sizes...), f.closes, append([]*exec.Cmd(nil), f.started...)
}

// stubPty points newPty at f for the duration of the test.
func stubPty(t *testing.T, f *fakePty) {
	t.Helper()
	old := newPty
	newPty = func(cols, rows int) (Pty, error) {
		f.mu.Lock()
		f.sizes = append(f.sizes, [2]int{cols, rows})
		f.mu.Unlock()
		return f, nil
	}
	t.Cleanup(func() { newPty = old })
}

// startFake starts a session over f with a command that is never really run.
func startFake(t *testing.T, f *fakePty, cols, rows int) *Session {
	t.Helper()
	stubPty(t, f)
	s, err := Start(exec.Command("unused"), cols, rows)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func waitOut(t *testing.T, f *fakePty, want string) {
	t.Helper()
	waitFor(t, "pty to receive "+want, func() bool {
		return strings.Contains(f.out(), want)
	})
}

func waitDone(t *testing.T, s *Session) {
	t.Helper()
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for session to finish")
	}
}

func TestSessionRendersChildOutput(t *testing.T) {
	f := newFakePty()
	s := startFake(t, f, 20, 4)

	f.feed(t, "hello")

	select {
	case <-s.Output():
	case <-time.After(5 * time.Second):
		t.Fatal("no output notification")
	}

	waitFor(t, "hello to render", func() bool {
		return strings.Contains(s.Render(), "hello")
	})
	if got := len(strings.Split(s.Render(), "\n")); got != 4 {
		t.Fatalf("Render() lines = %d, want 4", got)
	}
}

func TestSessionDrainsDeviceAttributesReply(t *testing.T) {
	f := newFakePty()
	startFake(t, f, 20, 4)

	f.feed(t, "\x1b[c")

	waitOut(t, f, "\x1b[?62")
}

func TestSendKeySendsText(t *testing.T) {
	f := newFakePty()
	s := startFake(t, f, 20, 4)

	s.SendKey(uv.KeyPressEvent{Code: 'a', Text: "a"})

	waitOut(t, f, "a")
}

func TestSendKeyFollowsCursorKeysMode(t *testing.T) {
	f := newFakePty()
	s := startFake(t, f, 20, 4)

	s.SendKey(uv.KeyPressEvent{Code: uv.KeyUp})
	waitOut(t, f, "\x1b[A")
	f.resetOut()

	// DECCKM switches the arrow keys to application mode.
	f.feed(t, "\x1b[?1h")
	waitFor(t, "DECCKM to take effect", func() bool {
		f.resetOut()
		s.SendKey(uv.KeyPressEvent{Code: uv.KeyUp})
		return strings.Contains(f.out(), "\x1bOA")
	})
}

func TestPasteIsBracketedOnlyWhenTheChildAsks(t *testing.T) {
	f := newFakePty()
	s := startFake(t, f, 20, 4)

	s.Paste("plain")
	waitOut(t, f, "plain")
	if got := f.out(); strings.Contains(got, "\x1b[200~") {
		t.Fatalf("unbracketed paste wrote %q, want no bracket", got)
	}

	f.feed(t, "\x1b[?2004h")
	waitFor(t, "bracketed paste mode", func() bool {
		f.resetOut()
		s.Paste("wrapped")
		return strings.Contains(f.out(), "\x1b[200~wrapped\x1b[201~")
	})
}

// setMode turns a mode on and waits until the emulator has taken it in: the
// marker fed after it renders only once the sequence before it was processed.
func setMode(t *testing.T, f *fakePty, s *Session, mode, marker string) {
	t.Helper()
	f.feed(t, mode+marker)
	waitFor(t, "mode "+mode+" to be read", func() bool {
		return strings.Contains(s.Render(), marker)
	})
	f.resetOut()
}

// mouseMarker follows a mouse event down the same pipe, so waiting for it tells
// an event the emulator dropped apart from one not yet written.
const mouseMarker = "\x00marker\x00"

// sentMouse returns everything the child was sent for one mouse event.
func sentMouse(t *testing.T, f *fakePty, s *Session, send func()) string {
	t.Helper()
	send()
	s.SendBytes([]byte(mouseMarker))
	waitOut(t, f, mouseMarker)
	return strings.TrimSuffix(f.out(), mouseMarker)
}

func TestSendMouseFollowsTheChildsMouseModes(t *testing.T) {
	// Every reporting mode without an extended encoding uses X10's: a press of
	// the left button (button byte 0) at cell (3, 1), each coordinate offset by
	// one and then by the encoding's 32.
	const x10Press = "\x1b[M \x24\x22"

	tests := []struct {
		name string
		mode string
		want string
	}{
		{"no mouse reporting", "", ""},
		{"x10", "\x1b[?9h", x10Press},
		{"normal", "\x1b[?1000h", x10Press},
		{"button event", "\x1b[?1002h", x10Press},
		{"any event", "\x1b[?1003h", x10Press},
		{"sgr", "\x1b[?1000h\x1b[?1006h", "\x1b[<0;4;2M"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakePty()
			s := startFake(t, f, 20, 4)

			if tt.mode == "" {
				f.resetOut()
			} else {
				setMode(t, f, s, tt.mode, "m")
			}

			got := sentMouse(t, f, s, func() {
				s.SendMouse(3, 1, uv.MouseLeft, 0, MousePress)
			})
			if got != tt.want {
				t.Fatalf("child was sent %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSendMouseEncodesEachKindOfEvent(t *testing.T) {
	tests := []struct {
		name   string
		button uv.MouseButton
		kind   MouseKind
		want   string
	}{
		{"press", uv.MouseLeft, MousePress, "\x1b[<0;4;2M"},
		{"release", uv.MouseLeft, MouseRelease, "\x1b[<0;4;2m"},
		{"motion", uv.MouseNone, MouseMotion, "\x1b[<35;4;2M"},
		{"wheel", uv.MouseWheelUp, MouseWheel, "\x1b[<64;4;2M"},
		{"unknown kind", uv.MouseLeft, MouseKind(-1), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakePty()
			s := startFake(t, f, 20, 4)
			setMode(t, f, s, "\x1b[?1000h\x1b[?1006h", "m")

			got := sentMouse(t, f, s, func() {
				s.SendMouse(3, 1, tt.button, 0, tt.kind)
			})
			if got != tt.want {
				t.Fatalf("child was sent %q, want %q", got, tt.want)
			}
		})
	}
}

// The modifiers held during an event are encoded into the button byte — 4 for
// shift, 8 for alt, 16 for ctrl, added to the button's own code — so a child
// that binds a modified gesture sees the one that happened.
func TestSendMouseEncodesTheModifiers(t *testing.T) {
	tests := []struct {
		name string
		mod  uv.KeyMod
		want string
	}{
		{"none", 0, "\x1b[<0;4;2M"},
		{"shift", uv.ModShift, "\x1b[<4;4;2M"},
		{"alt", uv.ModAlt, "\x1b[<8;4;2M"},
		{"ctrl", uv.ModCtrl, "\x1b[<16;4;2M"},
		{"all three", uv.ModShift | uv.ModAlt | uv.ModCtrl, "\x1b[<28;4;2M"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakePty()
			s := startFake(t, f, 20, 4)
			setMode(t, f, s, "\x1b[?1000h\x1b[?1006h", "m")

			got := sentMouse(t, f, s, func() {
				s.SendMouse(3, 1, uv.MouseLeft, tt.mod, MousePress)
			})
			if got != tt.want {
				t.Fatalf("child was sent %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSendMouseStaysOrderedWithKeys(t *testing.T) {
	f := newFakePty()
	s := startFake(t, f, 20, 4)
	setMode(t, f, s, "\x1b[?1000h\x1b[?1006h", "m")

	s.SendBytes([]byte("a"))
	s.SendMouse(3, 1, uv.MouseLeft, 0, MousePress)
	s.SendKey(uv.KeyPressEvent{Code: 'b', Text: "b"})

	waitOut(t, f, "a\x1b[<0;4;2Mb")
}

func TestSendBytesStaysOrderedWithKeys(t *testing.T) {
	f := newFakePty()
	s := startFake(t, f, 20, 4)

	s.SendBytes([]byte("ab"))
	s.SendKey(uv.KeyPressEvent{Code: 'c', Text: "c"})
	s.SendBytes([]byte("de"))

	waitOut(t, f, "abcde")
}

func TestResizeReachesEmulatorAndPty(t *testing.T) {
	f := newFakePty()
	s := startFake(t, f, 20, 4)

	if err := s.Resize(10, 2); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	sizes, _, _ := f.snapshot()
	if len(sizes) != 2 || sizes[1] != [2]int{10, 2} {
		t.Fatalf("pty sizes = %v, want the open size then {10 2}", sizes)
	}
	if got := len(strings.Split(s.Render(), "\n")); got != 2 {
		t.Fatalf("Render() lines after resize = %d, want 2", got)
	}
}

func TestResizeClampsAndSurfacesPtyFailure(t *testing.T) {
	f := newFakePty()
	f.resizeErr = errors.New("boom")
	s := startFake(t, f, 0, 0)

	sizes, _, _ := f.snapshot()
	if sizes[0] != [2]int{1, 1} {
		t.Fatalf("open size = %v, want {1 1}", sizes[0])
	}

	err := s.Resize(-5, 0)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Resize error = %v, want it to wrap boom", err)
	}
	sizes, _, _ = f.snapshot()
	if sizes[1] != [2]int{1, 1} {
		t.Fatalf("resize size = %v, want {1 1}", sizes[1])
	}
}

func TestCursorTracksPositionAndVisibility(t *testing.T) {
	f := newFakePty()
	s := startFake(t, f, 20, 4)

	if x, y, visible := s.Cursor(); x != 0 || y != 0 || !visible {
		t.Fatalf("Cursor() = %d, %d, %v, want 0, 0, true", x, y, visible)
	}

	f.feed(t, "ab\x1b[?25l")
	waitFor(t, "cursor to move and hide", func() bool {
		x, y, visible := s.Cursor()
		return x == 2 && y == 0 && !visible
	})

	f.feed(t, "\x1b[?25h")
	waitFor(t, "cursor to show again", func() bool {
		_, _, visible := s.Cursor()
		return visible
	})
}

func TestExitClassification(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name string
		err  error
		want error
	}{
		{"eof", io.EOF, nil},
		{"eio", syscall.EIO, nil},
		{"closed", os.ErrClosed, nil},
		{"wrapped eio", &os.PathError{Op: "read", Err: syscall.EIO}, nil},
		{"other", boom, boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakePty()
			s := startFake(t, f, 20, 4)

			f.finish(tt.err)
			waitDone(t, s)

			if !errors.Is(s.Err(), tt.want) {
				t.Fatalf("Err() = %v, want %v", s.Err(), tt.want)
			}
		})
	}
}

func TestReapWaitsForTheChild(t *testing.T) {
	var (
		mu     sync.Mutex
		called bool
	)
	stubWait(t, func(ctx context.Context, cmd *exec.Cmd) error {
		mu.Lock()
		called = true
		mu.Unlock()
		return cmd.Wait() //nolint:wrapcheck // test seam
	})

	f := newFakePty()
	f.realStart = true
	stubPty(t, f)

	s, err := Start(shortCmd(), 20, 4)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(s.Close)

	f.finish(io.EOF)
	waitDone(t, s)

	mu.Lock()
	defer mu.Unlock()
	if !called {
		t.Fatal("waitProcess was not called")
	}
}

func TestReapKillsAChildThatWillNotBeWaitedFor(t *testing.T) {
	shortenReapTimeout(t)
	stubWait(t, func(ctx context.Context, cmd *exec.Cmd) error {
		// Stand in for a child that outlives its PTY: return only once the
		// kill has landed.
		<-ctx.Done()
		return cmd.Wait() //nolint:wrapcheck // test seam
	})

	f := newFakePty()
	f.realStart = true
	stubPty(t, f)

	cmd := exec.Command("sleep", "60")
	s, err := Start(cmd, 20, 4)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(s.Close)

	f.finish(io.EOF)
	waitDone(t, s)

	if cmd.ProcessState == nil || cmd.ProcessState.Exited() {
		t.Fatalf("child state = %v, want killed by a signal", cmd.ProcessState)
	}
}

func TestReapSkipsAnUnstartedChild(t *testing.T) {
	stubWait(t, func(context.Context, *exec.Cmd) error {
		t.Error("waitProcess called for an unstarted child")
		return nil
	})

	f := newFakePty()
	s := startFake(t, f, 20, 4)

	f.finish(io.EOF)
	waitDone(t, s)

	if s.Err() != nil {
		t.Fatalf("Err() = %v, want nil", s.Err())
	}
}

func TestCloseIsIdempotentAndUnblocksTheReadPump(t *testing.T) {
	f := newFakePty()
	s := startFake(t, f, 20, 4)

	s.Close()
	waitDone(t, s)
	if s.Err() != nil {
		t.Fatalf("Err() after Close = %v, want nil", s.Err())
	}

	s.Close()
	s.Close()
	if _, closes, _ := f.snapshot(); closes != 1 {
		t.Fatalf("pty closed %d times, want 1", closes)
	}
}

func TestCloseAfterTheChildHasExited(t *testing.T) {
	f := newFakePty()
	s := startFake(t, f, 20, 4)

	f.finish(io.EOF)
	waitDone(t, s)

	s.Close()
	if _, closes, _ := f.snapshot(); closes != 1 {
		t.Fatalf("pty closed %d times, want 1", closes)
	}
}

func TestRapidOutputNeverBlocksTheReadPump(t *testing.T) {
	f := newFakePty()
	s := startFake(t, f, 250, 4)

	// Nothing ever receives from Output; the pump must not care.
	for range 200 {
		f.feed(t, "x")
	}
	waitFor(t, "the last write to render", func() bool {
		return strings.Count(s.Render(), "x") == 200
	})
}

func TestStartReportsAPtyThatWillNotOpen(t *testing.T) {
	want := errors.New("no ptys left")
	old := newPty
	newPty = func(int, int) (Pty, error) { return nil, want }
	t.Cleanup(func() { newPty = old })

	s, err := Start(exec.Command("unused"), 20, 4)
	if s != nil {
		t.Fatal("Start returned a session")
	}
	if !errors.Is(err, want) {
		t.Fatalf("Start error = %v, want it to wrap %v", err, want)
	}
}

func TestStartClosesThePtyWhenTheCommandWillNotStart(t *testing.T) {
	want := errors.New("exec format error")
	f := newFakePty()
	f.startErr = want
	stubPty(t, f)

	s, err := Start(exec.Command("unused"), 20, 4)
	if s != nil {
		t.Fatal("Start returned a session")
	}
	if !errors.Is(err, want) {
		t.Fatalf("Start error = %v, want it to wrap %v", err, want)
	}
	if _, closes, _ := f.snapshot(); closes != 1 {
		t.Fatalf("pty closed %d times, want 1", closes)
	}
}

// TestStartOnARealPty exercises the real xpty-backed seam end to end.
func TestStartOnARealPty(t *testing.T) {
	s, err := Start(exec.Command("echo", "real-pty"), 40, 6)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()

	waitFor(t, "the child's output", func() bool {
		return strings.Contains(s.Render(), "real-pty")
	})
	waitDone(t, s)
	if s.Err() != nil {
		t.Fatalf("Err() = %v, want nil", s.Err())
	}
}

func TestHangupOnPassesAFailureToOpenStraightBack(t *testing.T) {
	want := errors.New("no ptys left")
	pty, err := hangupOn(nil, want)
	if pty != nil {
		t.Fatalf("hangupOn returned %v, want nil", pty)
	}
	if !errors.Is(err, want) {
		t.Fatalf("hangupOn error = %v, want %v", err, want)
	}
}

func TestStartOnARealPtyReportsACommandThatWillNotRun(t *testing.T) {
	s, err := Start(exec.Command("./no-such-command-for-vterm-tests"), 40, 6)
	if s != nil {
		t.Fatal("Start returned a session")
	}
	if err == nil || !strings.Contains(err.Error(), "start command") {
		t.Fatalf("Start error = %v, want a start-command failure", err)
	}
}

func TestOutputAndDoneAreTheSessionsOwnChannels(t *testing.T) {
	f := newFakePty()
	s := startFake(t, f, 20, 4)

	if s.Output() != s.output {
		t.Fatal("Output() is not the session's channel")
	}
	if s.Done() != s.done {
		t.Fatal("Done() is not the session's channel")
	}
}

// shortenReapTimeout makes the wait for a lingering child a test-length one.
func shortenReapTimeout(t *testing.T) {
	t.Helper()
	old := reapTimeout
	reapTimeout = 10 * time.Millisecond
	t.Cleanup(func() { reapTimeout = old })
}

// stubWait points waitProcess at fn for the duration of the test.
func stubWait(t *testing.T, fn func(context.Context, *exec.Cmd) error) {
	t.Helper()
	old := waitProcess
	waitProcess = fn
	t.Cleanup(func() { waitProcess = old })
}

// shortCmd is a child process that exits at once and is available everywhere
// the tests run: the test binary itself, told to run no tests.
func shortCmd() *exec.Cmd {
	return exec.Command(os.Args[0], "-test.run=XXXNoSuchTestXXX")
}
