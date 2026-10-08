package actions

import (
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// fakeHandBackGit answers the hand-back's git reads and records each push.
type fakeHandBackGit struct {
	current    string
	currentErr error
	dirty      []string
	dirtyErr   error
	pushErr    error
	statusDirs []string
	pushes     []worktreeCall
}

func (f *fakeHandBackGit) CurrentBranch(string) (string, error) { return f.current, f.currentErr }

func (f *fakeHandBackGit) DirtyPaths(dir string) ([]string, error) {
	f.statusDirs = append(f.statusDirs, dir)
	return f.dirty, f.dirtyErr
}

func (f *fakeHandBackGit) Push(dir, branch string) error {
	f.pushes = append(f.pushes, worktreeCall{dir: dir, branch: branch})
	return f.pushErr
}

var handBackProject = config.ProjectConfig{WorkingDir: "/repo"}

func handBackSlice() domain.Slice { return domain.Slice{ID: "s1", Name: "Render the board"} }

func TestPushHandBackReadsTheWorktreesBranch(t *testing.T) {
	w := &fakeWorktrees{existing: map[string]string{"slice/render-the-board": "/wt"}}
	g := &fakeHandBackGit{current: "slice/render-the-board-v2"}
	branch, err := PushHandBack(w, g, handBackSlice(), handBackProject, "")
	if err != nil {
		t.Fatalf("PushHandBack: %v", err)
	}
	if branch != "slice/render-the-board-v2" {
		t.Errorf("branch = %q, want the one the worktree has checked out", branch)
	}
	if len(w.looks) != 1 || w.looks[0] != (worktreeCall{dir: "/repo", branch: "slice/render-the-board"}) {
		t.Errorf("looked up %+v, want the agent branch in the repository", w.looks)
	}
	if len(g.statusDirs) != 1 || g.statusDirs[0] != "/wt" {
		t.Errorf("status read in %v, want /wt", g.statusDirs)
	}
	want := worktreeCall{dir: "/wt", branch: "slice/render-the-board-v2"}
	if len(g.pushes) != 1 || g.pushes[0] != want {
		t.Errorf("pushes = %+v, want exactly %+v", g.pushes, want)
	}
}

func TestPushHandBackPushesANamedBranchFromItsWorktree(t *testing.T) {
	w := &fakeWorktrees{existing: map[string]string{"slice/other": "/wt-other"}}
	g := &fakeHandBackGit{}
	branch, err := PushHandBack(w, g, handBackSlice(), handBackProject, "slice/other")
	if err != nil || branch != "slice/other" {
		t.Fatalf("PushHandBack = %q, %v", branch, err)
	}
	if len(g.pushes) != 1 || g.pushes[0] != (worktreeCall{dir: "/wt-other", branch: "slice/other"}) {
		t.Errorf("pushes = %+v", g.pushes)
	}
}

func TestPushHandBackPushesANamedBranchWithNoWorktreeFromTheRepository(t *testing.T) {
	g := &fakeHandBackGit{dirty: []string{"unrelated.go"}}
	branch, err := PushHandBack(&fakeWorktrees{}, g, handBackSlice(), handBackProject, "slice/x")
	if err != nil || branch != "slice/x" {
		t.Fatalf("PushHandBack = %q, %v", branch, err)
	}
	if len(g.statusDirs) != 0 {
		t.Errorf("status read in %v: the repository's own checkout is not the branch's", g.statusDirs)
	}
	if len(g.pushes) != 1 || g.pushes[0] != (worktreeCall{dir: "/repo", branch: "slice/x"}) {
		t.Errorf("pushes = %+v", g.pushes)
	}
}

func TestPushHandBackWithNoWorktreeAndNoBranch(t *testing.T) {
	g := &fakeHandBackGit{}
	if _, err := PushHandBack(&fakeWorktrees{}, g, handBackSlice(), handBackProject, ""); !errors.Is(err, ErrNoWorktree) {
		t.Errorf("err = %v, want ErrNoWorktree", err)
	}
	if _, err := PushHandBack(&fakeWorktrees{}, g, handBackSlice(), config.ProjectConfig{}, ""); !errors.Is(err, ErrNoWorktree) {
		t.Errorf("no repository: err = %v, want ErrNoWorktree", err)
	}
	if len(g.pushes) != 0 {
		t.Errorf("pushed %+v", g.pushes)
	}
}

func TestPushHandBackWithNoRepositoryForANamedBranch(t *testing.T) {
	g := &fakeHandBackGit{}
	_, err := PushHandBack(&fakeWorktrees{}, g, handBackSlice(), config.ProjectConfig{}, "slice/x")
	if err == nil || !strings.Contains(err.Error(), "slice-repo") {
		t.Errorf("err = %v, want it to say to record the repository", err)
	}
	if len(g.pushes) != 0 {
		t.Errorf("pushed %+v", g.pushes)
	}
}

func TestPushHandBackRefusesADirtyWorktree(t *testing.T) {
	w := &fakeWorktrees{existing: map[string]string{"slice/render-the-board": "/wt"}}
	g := &fakeHandBackGit{current: "slice/render-the-board", dirty: []string{"a.go", "notes.txt"}}
	_, err := PushHandBack(w, g, handBackSlice(), handBackProject, "")
	var dirty *DirtyError
	if !errors.As(err, &dirty) {
		t.Fatalf("err = %v, want a DirtyError", err)
	}
	for _, want := range []string{"/wt", "a.go", "notes.txt"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it naming %s", err, want)
		}
	}
	if len(g.pushes) != 0 {
		t.Errorf("pushed %+v over a dirty tree", g.pushes)
	}
}

func TestPushHandBackReportsGitFailures(t *testing.T) {
	w := &fakeWorktrees{existing: map[string]string{"slice/render-the-board": "/wt"}}
	boom := errors.New("boom")
	cases := map[string]*fakeHandBackGit{
		"current branch": {currentErr: boom},
		"status":         {current: "b", dirtyErr: boom},
		"push":           {current: "b", pushErr: boom},
	}
	for name, g := range cases {
		if _, err := PushHandBack(w, g, handBackSlice(), handBackProject, ""); !errors.Is(err, boom) {
			t.Errorf("%s: err = %v, want %v", name, err, boom)
		}
	}
}

func TestPushHandBackRefusesADetachedWorktree(t *testing.T) {
	w := &fakeWorktrees{existing: map[string]string{"slice/render-the-board": "/wt"}}
	g := &fakeHandBackGit{}
	_, err := PushHandBack(w, g, handBackSlice(), handBackProject, "")
	if err == nil || !strings.Contains(err.Error(), "detached") {
		t.Errorf("err = %v, want a detached-HEAD refusal", err)
	}
	if len(g.pushes) != 0 {
		t.Errorf("pushed %+v", g.pushes)
	}
}
