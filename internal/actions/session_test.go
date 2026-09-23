package actions

import (
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/worktree"
)

func TestSessionBranch(t *testing.T) {
	got := SessionBranch("8f654180-9b8d-53fb-1024-9ee08f654180")
	want := "session/" + agent.SessionIDPrefix("8f654180-9b8d-53fb-1024-9ee08f654180")
	if got != want {
		t.Errorf("SessionBranch = %q, want %q", got, want)
	}
	if !strings.HasPrefix(got, "session/") {
		t.Errorf("SessionBranch = %q, want the session/ prefix", got)
	}
}

// A session's directory outside any git repository runs the agent there
// directly, with a warning rather than a refusal — the same shared-checkout
// fallback [PlaceAgent] gives a slice, worded for a session instead.
func TestPlaceSessionOutsideARepo(t *testing.T) {
	dir := t.TempDir()
	p := PlaceSession(&fakeWorktrees{}, &fakeRepo{}, dir, "session/abcd1234")
	if !p.OK {
		t.Fatalf("PlaceSession outside a repo: OK = false, want true")
	}
	if p.Dir != dir {
		t.Errorf("PlaceSession outside a repo: Dir = %q, want %q", p.Dir, dir)
	}
	if p.Branch != "" {
		t.Errorf("PlaceSession outside a repo: Branch = %q, want none", p.Branch)
	}
	if p.Toast == "" {
		t.Error("PlaceSession outside a repo: want a toast explaining where it runs")
	}
}

// A fresh branch is cut off the remote's current default, fetched first —
// the same rule [PlaceAgent] cuts a slice's worktree by.
func TestPlaceSessionCutsAFreshWorktree(t *testing.T) {
	dir := repoDir(t)
	w := &fakeWorktrees{}
	r := &fakeRepo{base: "origin/main"}
	p := PlaceSession(w, r, dir, "session/abcd1234")
	if !p.OK {
		t.Fatalf("PlaceSession: OK = false, toast %q", p.Toast)
	}
	if p.Branch != "session/abcd1234" {
		t.Errorf("PlaceSession: Branch = %q, want session/abcd1234", p.Branch)
	}
	if len(r.fetches) != 1 || r.fetches[0] != dir {
		t.Errorf("fetches = %v, want one fetch of %s", r.fetches, dir)
	}
	if len(w.creates) != 1 || w.creates[0].base != "origin/main" {
		t.Errorf("creates = %+v, want one cut from origin/main", w.creates)
	}
}

// An existing worktree for the branch is reused rather than re-cut — the
// same reuse rule [PlaceAgent] follows for a relaunched slice, though a
// session is never relaunched itself; the mechanics are shared regardless.
func TestPlaceSessionReusesAnExistingWorktree(t *testing.T) {
	dir := repoDir(t)
	w := &fakeWorktrees{existing: map[string]string{"session/abcd1234": "/repo.worktrees/session-abcd1234"}}
	r := &fakeRepo{base: "origin/main"}
	p := PlaceSession(w, r, dir, "session/abcd1234")
	if !p.OK {
		t.Fatalf("PlaceSession: OK = false, toast %q", p.Toast)
	}
	if p.Dir != "/repo.worktrees/session-abcd1234" {
		t.Errorf("PlaceSession: Dir = %q, want the existing worktree's path", p.Dir)
	}
	if len(w.creates) != 0 {
		t.Errorf("creates = %+v, want none: the worktree already exists", w.creates)
	}
}

// A git that refuses to cut the worktree fails the placement outright,
// rather than falling back to the shared directory.
func TestPlaceSessionRefusesOnAWorktreeFailure(t *testing.T) {
	dir := repoDir(t)
	w := &fakeWorktrees{createErr: &worktree.ExitError{Code: 1, Stderr: "the repository has no commits\n"}}
	p := PlaceSession(w, &fakeRepo{}, dir, "session/abcd1234")
	if p.OK {
		t.Error("PlaceSession with a worktree failure: OK = true, want false")
	}
	if p.Toast == "" {
		t.Error("PlaceSession with a worktree failure: want a toast naming it")
	}
}
