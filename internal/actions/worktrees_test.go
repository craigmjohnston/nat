package actions

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/worktree"
)

// worktreeCall is one thing the fake was asked about: the repository, the
// branch it was asked about there, and — for a create — the ref that branch
// was to be cut from.
type worktreeCall struct{ dir, branch, base string }

// fakeWorktrees stands in for git's worktrees. Nothing it is asked about
// exists unless the test says so, which is the ordinary case: a slice nobody
// has worked yet has no worktree and no branch.
type fakeWorktrees struct {
	// existing are the branches already checked out somewhere, by path.
	existing map[string]string
	// pathErr is what a branch not in existing answers with, createErr what
	// the create that follows fails with, and removeErr what a removal is
	// refused with; all nil is git working.
	pathErr   error
	createErr error
	removeErr error

	looks   []worktreeCall
	creates []worktreeCall
	removes []worktreeCall
}

var _ Worktrees = (*fakeWorktrees)(nil)

func (f *fakeWorktrees) Path(dir, branch string) (string, error) {
	f.looks = append(f.looks, worktreeCall{dir: dir, branch: branch})
	if path, ok := f.existing[branch]; ok {
		return path, nil
	}
	if f.pathErr != nil {
		return "", f.pathErr
	}
	return "", fmt.Errorf("git names no worktree for %s", branch)
}

func (f *fakeWorktrees) Create(dir, branch, base string) (string, error) {
	f.creates = append(f.creates, worktreeCall{dir, branch, base})
	if f.createErr != nil {
		return "", f.createErr
	}
	return filepath.Join(dir+"-worktrees", branch), nil
}

func (f *fakeWorktrees) Remove(dir, branch string) error {
	f.removes = append(f.removes, worktreeCall{dir: dir, branch: branch})
	return f.removeErr
}

// fakeRepo stands in for git: what the fetch was asked of, and what origin's
// HEAD is read as afterwards. The real one never fails at either — a fetch
// that could not reach the remote is swallowed, and an unreadable HEAD falls
// back to main — so there is nothing here for a test to make go wrong.
type fakeRepo struct {
	base    string
	fetches []string
}

var _ Repo = (*fakeRepo)(nil)

func (f *fakeRepo) Fetch(dir string) { f.fetches = append(f.fetches, dir) }

func (f *fakeRepo) Base(string) string { return f.base }

// repoDir is a directory that looks enough like a git checkout for the
// launch flow's own test: what it reads is whether there is a .git in it.
func repoDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The branch is derived from the title alone and has to come out the same on
// every relaunch, since it is what finds the worktree the last session left.
func TestSliceBranch(t *testing.T) {
	tests := []struct {
		name  string
		slice domain.Slice
		want  string
	}{
		{"plain", domain.Slice{Name: "Launch slice agents in a fresh worktree"},
			"slice/launch-slice-agents-in-a-fresh-worktree"},
		{"case and punctuation", domain.Slice{Name: "tmux integration + agent prompt template"},
			"slice/tmux-integration-agent-prompt-template"},
		{"runs collapse", domain.Slice{Name: "Diff — strip  the/git noise!"},
			"slice/diff-strip-the-git-noise"},
		{"digits kept", domain.Slice{Name: "M27: worktrees per slice"},
			"slice/m27-worktrees-per-slice"},
		{"trimmed at both ends", domain.Slice{Name: "  …spike?  "}, "slice/spike"},
		{"nothing to slug", domain.Slice{Name: "———", ID: "3b738308-f654-8170-8c99-eccab4463d8f"},
			"slice/" + agent.SessionName("3b738308-f654-8170-8c99-eccab4463d8f")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SliceBranch(tt.slice); got != tt.want {
				t.Errorf("SliceBranch(%q) = %q, want %q", tt.slice.Name, got, tt.want)
			}
		})
	}
}

// AgentBranch is what a relaunch is placed on: the branch recorded at
// hand-back where there is one, and the derived branch otherwise.
func TestAgentBranch(t *testing.T) {
	tests := []struct {
		name  string
		slice domain.Slice
		want  string
	}{
		{"handed back", domain.Slice{Name: "Info view", Branch: "slice/from-hand-back"}, "slice/from-hand-back"},
		{"nothing recorded yet", domain.Slice{Name: "Info view"}, "slice/info-view"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AgentBranch(tt.slice); got != tt.want {
				t.Errorf("AgentBranch(%+v) = %q, want %q", tt.slice, got, tt.want)
			}
		})
	}
}

// The ordinary launch: a worktree cut for the slice's own branch, and the
// session placed in it rather than in the checkout the user is working in.
func TestPlaceAgentCutsAWorktree(t *testing.T) {
	dir := repoDir(t)
	w := &fakeWorktrees{}
	r := &fakeRepo{base: "origin/main"}

	p := PlaceAgent(w, r, dir, domain.Slice{Name: "Info view"})

	if !p.OK {
		t.Fatalf("placement refused: %s", p.Toast)
	}
	if want := filepath.Join(dir+"-worktrees", "slice/info-view"); p.Dir != want {
		t.Errorf("dir = %q, want the worktree at %q", p.Dir, want)
	}
	if p.Branch != "slice/info-view" {
		t.Errorf("branch = %q, want the slice's own", p.Branch)
	}
	if p.Repo != dir {
		t.Errorf("repo = %q, want the checkout it was cut from %q", p.Repo, dir)
	}
	if p.Toast != "" {
		t.Errorf("toast = %q, want nothing said about an ordinary launch", p.Toast)
	}
	if want := []worktreeCall{{dir, "slice/info-view", "origin/main"}}; !equalCalls(w.creates, want) {
		t.Errorf("creates = %v, want %v", w.creates, want)
	}
	if want := []string{dir}; !slices.Equal(r.fetches, want) {
		t.Errorf("fetches = %v, want origin fetched in %v before the cut", r.fetches, want)
	}
}

// The base is whatever git answers with, passed through as it stands: a
// repository with no origin to read falls back to its local default branch,
// which is all such a checkout has to cut from.
func TestPlaceAgentCutsFromTheFallbackBase(t *testing.T) {
	dir := repoDir(t)
	w := &fakeWorktrees{}
	r := &fakeRepo{base: git.DefaultBase}

	if p := PlaceAgent(w, r, dir, domain.Slice{Name: "Info view"}); !p.OK {
		t.Fatalf("placement refused: %s", p.Toast)
	}
	if want := []worktreeCall{{dir, "slice/info-view", git.DefaultBase}}; !equalCalls(w.creates, want) {
		t.Errorf("creates = %v, want %v", w.creates, want)
	}
}

// A relaunched slice wants the worktree it was working in, not an empty
// second copy of the repository: the branch already having one is the whole
// answer.
func TestPlaceAgentReusesTheBranchesWorktree(t *testing.T) {
	dir := repoDir(t)
	w := &fakeWorktrees{existing: map[string]string{"slice/info-view": "/repos/nat-info-view"}}
	r := &fakeRepo{base: "origin/main"}

	p := PlaceAgent(w, r, dir, domain.Slice{Name: "Info view"})

	if !p.OK || p.Dir != "/repos/nat-info-view" {
		t.Fatalf("placement = %+v, want the worktree the branch already has", p)
	}
	if p.Branch != "slice/info-view" || p.Repo != dir {
		t.Errorf("placement = %+v, want the branch and its checkout named", p)
	}
	if len(w.creates) != 0 {
		t.Errorf("creates = %v, want the existing worktree left alone", w.creates)
	}
	if len(r.fetches) != 0 {
		t.Errorf("fetches = %v, want nothing fetched for a worktree that already exists", r.fetches)
	}
}

// A look-up that fails is not read for a reason of its own: whatever git
// could not say about the branch's worktree, the answer is to cut one, and it
// is that cut which says whether the agent can be placed at all.
func TestPlaceAgentWhenTheLookUpFails(t *testing.T) {
	dir := repoDir(t)
	w := &fakeWorktrees{pathErr: &worktree.ExitError{Code: 128, Stderr: "fatal: not a git repository\n"}}
	r := &fakeRepo{base: "origin/main"}

	p := PlaceAgent(w, r, dir, domain.Slice{Name: "Info view"})

	if !p.OK || p.Branch != "slice/info-view" || p.Repo != dir {
		t.Fatalf("placement = %+v, want a worktree cut for the slice", p)
	}
	if p.Toast != "" {
		t.Errorf("toast = %q, want nothing said about a launch that worked", p.Toast)
	}
	if want := []worktreeCall{{dir, "slice/info-view", "origin/main"}}; !equalCalls(w.creates, want) {
		t.Errorf("creates = %v, want %v", w.creates, want)
	}
}

// A working directory that is not in a git repository at all has no
// worktree to give: that launch is the one that always worked, so it goes
// ahead.
func TestPlaceAgentOutsideARepository(t *testing.T) {
	dir := t.TempDir()
	w := &fakeWorktrees{}
	r := &fakeRepo{base: "origin/main"}

	p := PlaceAgent(w, r, dir, domain.Slice{Name: "Info view"})

	if !p.OK || p.Dir != dir || p.Branch != "" {
		t.Fatalf("placement = %+v, want the directory as it stands", p)
	}
	if !strings.Contains(p.Toast, dir) || !strings.Contains(p.Toast, "not a git repository") {
		t.Errorf("toast = %q, want it to name the directory and why", p.Toast)
	}
	if len(w.looks)+len(w.creates)+len(r.fetches) != 0 {
		t.Error("git should not be asked anything about a directory that is not a repository")
	}
}

// A git that ran and refused is not a fallback: something is wrong with the
// repository, and an agent half-placed on the strength of it would be
// working somewhere nobody chose.
func TestPlaceAgentWhenTheWorktreeCannotBeMade(t *testing.T) {
	dir := repoDir(t)
	w := &fakeWorktrees{createErr: &worktree.ExitError{Code: 1, Stderr: "branch 'slice/info-view' already exists\n"}}
	r := &fakeRepo{base: "origin/main"}

	p := PlaceAgent(w, r, dir, domain.Slice{Name: "Info view"})

	if p.OK {
		t.Fatalf("placement = %+v, want nothing launched", p)
	}
	if !strings.Contains(p.Toast, `"Info view"`) || !strings.Contains(p.Toast, "already exists") {
		t.Errorf("toast = %q, want the slice and git's own reason", p.Toast)
	}
	if p.Sev != SevError {
		t.Errorf("severity = %v, want an error", p.Sev)
	}
}

// git's own rule: a directory inside a checkout is in it, and a worktree's
// .git is a file rather than a directory.
func TestInRepo(t *testing.T) {
	root := repoDir(t)
	sub := filepath.Join(root, "internal", "tui")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	if !InRepo(sub) {
		t.Error("a directory under a checkout is in it")
	}

	linked := t.TempDir()
	if err := os.WriteFile(filepath.Join(linked, ".git"), []byte("gitdir: /repos/nat/.git/worktrees/x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !InRepo(linked) {
		t.Error("a worktree keeps its .git as a file, and is still a checkout")
	}

	if InRepo(t.TempDir()) {
		t.Error("a plain directory is not a checkout")
	}
}

// equalCalls compares two runs of calls, which reflect.DeepEqual would do
// too — but nil and empty are the same answer here.
func equalCalls(got, want []worktreeCall) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
