package actions

import (
	"testing"

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
