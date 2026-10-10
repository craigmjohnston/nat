package actions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// recordFor leaves a session record for the slice with page ID id and
// answers its path.
func recordFor(t *testing.T, id string) string {
	t.Helper()
	path, err := agent.SessionRecordPath(agent.SessionName(id))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"session_id":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func recordGone(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

// Where a slice's work ends — merged, closed Done, trashed, cancelled — its
// session record goes with its worktree, a task with no repository's too.
func TestEndingTheWorkForgetsTheSession(t *testing.T) {
	isolatedState(t)
	merged, task, discarded := recordFor(t, "s-merged"), recordFor(t, "s-task"), recordFor(t, "s-discarded")

	RemoveSliceWorktree(&fakeWorktrees{}, domain.Slice{ID: "s-merged", Name: "Merged", Repo: "/repo"}, config.ProjectConfig{})
	RemoveSliceWorktree(&fakeWorktrees{}, domain.Slice{ID: "s-task", Name: "Task"}, config.ProjectConfig{Backend: config.BackendSource})
	DiscardSliceWorktree(&fakeWorktrees{}, domain.Slice{ID: "s-discarded", Name: "Discarded", Repo: "/repo"}, config.ProjectConfig{})

	for name, path := range map[string]string{"merged": merged, "task": task, "discarded": discarded} {
		if !recordGone(t, path) {
			t.Errorf("%s slice's session record left behind", name)
		}
	}
}

// The landed sweep forgets the session of a slice whose worktree it removes,
// and of no other: a live agent's record, and a slice with no worktree left,
// are not its to touch.
func TestSweepLandedForgetsOnlyWhatItRemoves(t *testing.T) {
	isolatedState(t)
	gone, live, elsewhere := recordFor(t, "a"), recordFor(t, "c"), recordFor(t, "d")
	w := &fakeWorktrees{existing: map[string]string{"slice/gone": "/wt/gone", "slice/live": "/wt/live"}}
	SweepLanded(w, func() (map[string]string, error) { return map[string]string{"c": "nat-c"}, nil },
		config.ProjectConfig{Backend: config.BackendSource}, []domain.Slice{
			{ID: "a", Name: "Gone", Repo: "/repo"},
			{ID: "c", Name: "Live", Repo: "/repo"},
			{ID: "d", Name: "Elsewhere", Repo: "/repo"},
		})
	if !recordGone(t, gone) {
		t.Error("the swept slice's record is still there")
	}
	if recordGone(t, live) || recordGone(t, elsewhere) {
		t.Error("the sweep removed a record of a slice it did not sweep")
	}
}
