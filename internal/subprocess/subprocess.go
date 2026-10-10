// Package subprocess is how nat runs an outside program — git, gh, tmux, a
// plugin — so that its time limit holds. Each wrapper package keeps its own
// Runner seam and its own ExitError; this is only the one way the real ones
// start a process and wait on it.
//
// os/exec's CommandContext alone kills the program at the limit and then
// waits for its output pipes to close. A program's own child (the ssh under a
// git fetch) inherits those pipes and outlives the kill, and the wait lasts as
// long as it does: on 9 October a pr-status waited nineteen hours on an ssh
// left on a dead connection. Here the program runs in a process group of its
// own, the limit kills the whole group, and the wait on the pipes is itself
// bounded.
package subprocess

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"
)

// WaitDelay is how long a call waits on a program's output once the program
// has exited or been killed: whatever still holds the pipes after that is
// abandoned, and the call returns. A var so the tests can shorten it.
var WaitDelay = 2 * time.Second

// Command is exec.CommandContext with the program in a process group of its
// own, so that when ctx ends everything it started is killed with it, and
// with the wait on its output bounded by [WaitDelay].
//
// The group is also why a program run here cannot read the terminal: it is not
// the terminal's foreground group. Nothing nat runs is meant to prompt — gnat
// and agents run nat with no terminal at all.
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// The group's ID is the program's own; negative names the group.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = WaitDelay
	return cmd
}

// Run runs name with args in dir under [Command], for at most timeout, its
// stdin, stdout and stderr wired as given (nil is the null device). A program
// that ran past the limit comes back as an error saying so, whatever exit the
// kill gave it; otherwise the error is os/exec's own — an *exec.ExitError for a
// non-zero exit, for the caller to read.
func Run(timeout time.Duration, dir string, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := Command(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if err != nil && ctx.Err() != nil {
		return fmt.Errorf("%s timed out after %s", name, timeout)
	}
	return err
}
