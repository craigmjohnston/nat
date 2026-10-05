package actions

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/worktree"
)

// TestRemoveWorktree covers the whole rule: a worktree git names is removed,
// one it names none for passes quietly as already gone, and a removal git
// refuses is reported false rather than retried on the spot.
func TestRemoveWorktree(t *testing.T) {
	tests := []struct {
		name string
		w    *fakeWorktrees
		want bool
	}{
		{"removed", &fakeWorktrees{existing: map[string]string{"slice/x": "/worktrees/x"}}, true},
		{"already gone", &fakeWorktrees{}, true},
		{"refused", &fakeWorktrees{
			existing:  map[string]string{"slice/x": "/worktrees/x"},
			removeErr: &worktree.ExitError{Code: 1, Stderr: "worktree has uncommitted changes\n"},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RemoveWorktree(tt.w, "/repo", "slice/x"); got != tt.want {
				t.Errorf("RemoveWorktree() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRemoveSliceWorktree covers the pair a removal is named by — the slice's
// repository, ~ expanded, and its AgentBranch — and a source project's task
// with no repository, which asks git nothing.
func TestRemoveSliceWorktree(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	w := &fakeWorktrees{existing: map[string]string{"slice/handed": "/wt"}}
	s := domain.Slice{ID: "s1", Name: "Ignored", Branch: "slice/handed", Repo: "~/src/nat"}
	if !RemoveSliceWorktree(w, s, config.ProjectConfig{WorkingDir: "/project"}) {
		t.Error("RemoveSliceWorktree() = false, want the worktree gone")
	}
	want := []worktreeCall{{dir: filepath.Join(home, "src/nat"), branch: "slice/handed"}}
	if !reflect.DeepEqual(w.removes, want) {
		t.Errorf("removes = %+v, want %+v", w.removes, want)
	}

	w = &fakeWorktrees{}
	task := domain.Slice{ID: "t1", Name: "No repo"}
	if !RemoveSliceWorktree(w, task, config.ProjectConfig{Backend: config.BackendSource}) {
		t.Error("RemoveSliceWorktree() = false, want nothing to remove")
	}
	if len(w.looks)+len(w.removes) != 0 {
		t.Errorf("git asked %+v / %+v, want nothing", w.looks, w.removes)
	}
}

// TestSweepLanded covers what the sweep asks git: one listing per repository
// the slices name (none for a slice with no repository), a removal only for a
// branch that listing names and no live agent holds, a repository it cannot
// list skipped, and nothing at all where tmux cannot say who is live.
func TestSweepLanded(t *testing.T) {
	p := config.ProjectConfig{Backend: config.BackendSource}
	landed := []domain.Slice{
		{ID: "a", Name: "Gone", Repo: "/repo"},
		{ID: "b", Name: "Kept", Repo: "/repo"},
		{ID: "c", Name: "Live", Repo: "/repo"},
		{ID: "d", Name: "Elsewhere", Repo: "/other"},
		{ID: "e", Name: "Nowhere"},
	}
	existing := map[string]string{"slice/gone": "/wt/gone", "slice/live": "/wt/live", "slice/stranger": "/wt/s"}
	live := func() (map[string]string, error) { return map[string]string{"c": "nat-c"}, nil }

	t.Run("removes what landed", func(t *testing.T) {
		w := &fakeWorktrees{existing: existing}
		SweepLanded(w, live, p, landed)
		if want := []string{"/repo", "/other"}; !reflect.DeepEqual(w.listed, want) {
			t.Errorf("listed = %v, want %v", w.listed, want)
		}
		if want := []worktreeCall{{dir: "/repo", branch: "slice/gone"}}; !reflect.DeepEqual(w.removes, want) {
			t.Errorf("removes = %+v, want %+v", w.removes, want)
		}
	})

	t.Run("an unlistable repository", func(t *testing.T) {
		w := &fakeWorktrees{existing: existing, branchesErr: errors.New("not a git repository")}
		SweepLanded(w, live, p, landed)
		if len(w.removes) != 0 {
			t.Errorf("removes = %+v, want nothing", w.removes)
		}
	})

	t.Run("live sessions unread", func(t *testing.T) {
		w := &fakeWorktrees{existing: existing}
		SweepLanded(w, func() (map[string]string, error) { return nil, errors.New("tmux broke") }, p, landed)
		if len(w.removes) != 0 {
			t.Errorf("removes = %+v, want nothing", w.removes)
		}
	})

	t.Run("nothing to remove asks tmux nothing", func(t *testing.T) {
		w := &fakeWorktrees{}
		SweepLanded(w, func() (map[string]string, error) { t.Fatal("tmux asked"); return nil, nil }, p, landed)
	})
}
