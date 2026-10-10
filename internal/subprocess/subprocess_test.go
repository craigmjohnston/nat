package subprocess

import (
	"bytes"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestTheLimitEndsTheProgramAndEverythingItStarted is the outage's shape: a
// program that is killed at its limit while a child it started keeps the
// output pipe open. The call comes back shortly after the limit, says it timed
// out, and the child is gone too.
func TestTheLimitEndsTheProgramAndEverythingItStarted(t *testing.T) {
	var stdout bytes.Buffer
	start := time.Now()
	// The child's pid is printed first, so the test can look for it after.
	err := Run(200*time.Millisecond, t.TempDir(), nil, &stdout, nil,
		"sh", "-c", `sh -c "sleep 60" & echo $!; wait`)
	if took := time.Since(start); took > WaitDelay {
		t.Errorf("Run() took %s, want it back well within %s of the limit", took, WaitDelay)
	}
	if err == nil || err.Error() != "sh timed out after 200ms" {
		t.Errorf("Run() = %v, want the time-out", err)
	}
	pid, perr := strconv.Atoi(strings.TrimSpace(stdout.String()))
	if perr != nil {
		t.Fatalf("no child pid printed: %q", stdout.String())
	}
	waitGone(t, pid)
}

// TestAChildLeftHoldingThePipeIsNotWaitedOn covers the program that exits by
// itself, leaving a child with its output: the call returns once WaitDelay
// has passed, rather than once the child does.
func TestAChildLeftHoldingThePipeIsNotWaitedOn(t *testing.T) {
	defer func(was time.Duration) { WaitDelay = was }(WaitDelay)
	WaitDelay = 100 * time.Millisecond

	var stdout bytes.Buffer
	start := time.Now()
	err := Run(time.Minute, t.TempDir(), nil, &stdout, nil, "sh", "-c", `sleep 60 & echo $!`)
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("Run() took %s, want it back once WaitDelay passed", took)
	}
	if !errors.Is(err, exec.ErrWaitDelay) {
		t.Errorf("Run() = %v, want the abandoned wait reported", err)
	}
	if pid, err := strconv.Atoi(strings.TrimSpace(stdout.String())); err == nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

// TestRunWiresTheProgramAndPassesItsExitThrough covers an ordinary run: stdin
// in, stdout and stderr out, the directory honoured, and a non-zero exit
// returned as os/exec's own error for the caller to read.
func TestRunWiresTheProgramAndPassesItsExitThrough(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(time.Minute, "/", strings.NewReader("in"), &stdout, &stderr,
		"sh", "-c", `cat; pwd; echo boom >&2; exit 3`)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("Run() = %v, want exit 3", err)
	}
	if stdout.String() != "in/\n" || stderr.String() != "boom\n" {
		t.Errorf("Run() wrote %q and %q, want stdin echoed, the directory and stderr", stdout.String(), stderr.String())
	}
	if err := Run(time.Minute, "", nil, nil, nil, "true"); err != nil {
		t.Errorf("Run(true) = %v, want nil", err)
	}
}

// waitGone waits for pid to be gone: a killed process is a zombie until
// whatever adopted it reaps it, which takes a moment.
func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("process %d is still running after the call returned", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
