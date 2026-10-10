package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestAFetchThatCannotCompleteGivesUpAtTheLimit is the outage pr-status hit: a
// fetch whose ssh never answers. git is killed at the limit, but the ssh it
// started still holds git's stderr open — and the read the hand-back
// conflict test makes must still come back shortly after the limit, with the
// ssh gone rather than left on the dead connection.
func TestAFetchThatCannotCompleteGivesUpAtTheLimit(t *testing.T) {
	defer func(was time.Duration) { gitTimeout = was }(gitTimeout)
	// Long enough for git to reach the ssh even on a loaded machine.
	gitTimeout = 3 * time.Second

	dir := t.TempDir()
	pidFile := filepath.Join(dir, "ssh.pid")
	// The ssh git runs: a shell whose own child sleeps on, holding the
	// stderr it inherited from git.
	ssh := filepath.Join(dir, "fake-ssh")
	script := "#!/bin/sh\nsleep 600 &\necho $! > " + pidFile + "\nwait\n"
	if err := os.WriteFile(ssh, []byte(script), 0o755); err != nil { //nolint:gosec // a test script that must run
		t.Fatal(err)
	}
	repo := filepath.Join(dir, "repo")
	for _, args := range [][]string{
		{"init", "-q", "-b", "main", repo},
		{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "base"},
		{"-C", repo, "branch", "slice/x"},
		{"-C", repo, "remote", "add", "origin", "ssh://nat.invalid/x.git"},
		{"-C", repo, "config", "core.sshCommand", ssh},
		// Said outright, or git first runs the command with -G to ask what
		// kind of ssh it is — a run on /dev/null, holding no pipe of ours.
		{"-C", repo, "config", "ssh.variant", "ssh"},
	} {
		if out, err := exec.Command(Binary, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	done := make(chan MergeState, 1)
	start := time.Now()
	go func() { done <- New().ConflictsWithBase(repo, "slice/x") }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("ConflictsWithBase() still waiting on the fetch 30s after a 3s limit")
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("ConflictsWithBase() took %s, want it back within a few seconds of the limit", took)
	}

	raw, err := os.ReadFile(pidFile) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatalf("the fake ssh never ran: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	waitGone(t, pid)
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
