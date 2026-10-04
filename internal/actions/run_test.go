package actions

import (
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// A first global run cuts the run checkout from origin's default after a
// fetch, then resets it there; a later one reuses it and resets it again.
func TestGlobalRunDirCutsThenResetsTheRunCheckout(t *testing.T) {
	dir := repoDir(t)
	w, r := &fakeWorktrees{}, &fakeRepo{base: "origin/main"}
	path, err := GlobalRunDir(w, r, dir)
	if err != nil {
		t.Fatalf("GlobalRunDir: %v", err)
	}
	if len(r.fetches) != 1 || len(w.creates) != 1 || w.creates[0] != (worktreeCall{dir, RunBranch, "origin/main"}) {
		t.Fatalf("fetches = %v, creates = %+v", r.fetches, w.creates)
	}
	if len(w.resets) != 1 || w.resets[0] != (worktreeCall{dir: path, base: "origin/main"}) {
		t.Errorf("resets = %+v, want %s reset to origin/main", w.resets, path)
	}

	w = &fakeWorktrees{existing: map[string]string{RunBranch: "/wt/run-main"}}
	path, err = GlobalRunDir(w, r, dir)
	if err != nil || path != "/wt/run-main" || len(w.creates) != 0 || len(w.resets) != 1 || len(r.fetches) != 2 {
		t.Errorf("reuse: path %q err %v creates %+v resets %+v fetches %v", path, err, w.creates, w.resets, r.fetches)
	}
}

func TestGlobalRunDirRefusals(t *testing.T) {
	r := &fakeRepo{base: "origin/main"}
	for name, tt := range map[string]struct {
		dir  string
		w    *fakeWorktrees
		want string
	}{
		"no directory": {" ", &fakeWorktrees{}, "no working directory"},
		"not a repo":   {t.TempDir(), &fakeWorktrees{}, "is not a git repository"},
		"cut refused":  {repoDir(t), &fakeWorktrees{createErr: errors.New("locked")}, "could not make the run checkout: locked"},
		"reset refused": {repoDir(t), &fakeWorktrees{existing: map[string]string{RunBranch: "/wt"}, resetErr: errors.New("bad ref")},
			"could not bring the run checkout to origin/main: bad ref"},
	} {
		if _, err := GlobalRunDir(tt.w, r, tt.dir); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tt.want)
		}
	}
}

// A slice-scoped run is run in the worktree of the slice's agent branch — the
// recorded one where there is one — in the slice's own repo, else the
// project's.
func TestSliceRunDirFindsTheSlicesWorktree(t *testing.T) {
	w := &fakeWorktrees{existing: map[string]string{"slice/pushed": "/wt/pushed"}}
	s := domain.Slice{Name: "Thing", Status: domain.SliceClaimed, Branch: "slice/pushed", Repo: "/repos/other"}
	path, err := SliceRunDir(w, s, config.ProjectConfig{WorkingDir: "/repos/nat"})
	if err != nil || path != "/wt/pushed" {
		t.Fatalf("path %q err %v", path, err)
	}
	if w.looks[0] != (worktreeCall{dir: "/repos/other", branch: "slice/pushed"}) {
		t.Errorf("looked up %+v", w.looks[0])
	}
}

func TestSliceRunDirRefusals(t *testing.T) {
	w := &fakeWorktrees{}
	project := config.ProjectConfig{WorkingDir: "/repos/nat"}
	for name, tt := range map[string]struct {
		s    domain.Slice
		p    config.ProjectConfig
		want string
	}{
		"merged with a PR":     {domain.Slice{Name: "A", Status: domain.SliceDone, PRURL: "https://pr"}, project, `"A" is merged`},
		"merged with a branch": {domain.Slice{Name: "A", Status: domain.SliceDone, Branch: "slice/a"}, project, `"A" is merged`},
		"no repository":        {domain.Slice{Name: "A"}, config.ProjectConfig{Backend: config.BackendSource}, "no repository recorded"},
		"no directory":         {domain.Slice{Name: "A"}, config.ProjectConfig{}, "no working directory"},
		"no worktree":          {domain.Slice{Name: "A", Status: domain.SliceClaimed, Branch: "slice/a"}, project, `"A" has no worktree for slice/a`},
	} {
		if _, err := SliceRunDir(w, tt.s, tt.p); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tt.want)
		}
	}
}
