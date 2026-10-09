package actions

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/store"
)

// steps is the order things happened in across a test's fakes — tmux and the
// store alike — so a test can say the kill came before the write.
type steps []string

// fakeStopper stands in for tmux: the live sessions by slice ID, and what
// reading them or killing one fails with.
type fakeStopper struct {
	live    map[string]string
	liveErr error
	killErr error
	log     *steps
}

var _ AgentStopper = (*fakeStopper)(nil)

func (f *fakeStopper) LiveSlices() (map[string]string, error) { return f.live, f.liveErr }

func (f *fakeStopper) Kill(session string) error {
	*f.log = append(*f.log, "kill "+session)
	return f.killErr
}

// fakeCancelStore answers one slice and records the cancel written.
type fakeCancelStore struct {
	slice     domain.Slice
	sliceErr  error
	shapeErr  error
	cancelErr error
	log       *steps
	shapes    []store.Shape
}

var _ CancelStore = (*fakeCancelStore)(nil)

func (f *fakeCancelStore) Slice(context.Context, string) (domain.Slice, store.Shape, error) {
	return f.slice, store.Shape{}, f.sliceErr
}

func (f *fakeCancelStore) Shape(context.Context, store.Project) (store.Shape, error) {
	return store.Shape{HasAssignee: true, HasBranch: true}, f.shapeErr
}

func (f *fakeCancelStore) CancelSlice(_ context.Context, id string, sh store.Shape, by string) (domain.Slice, error) {
	*f.log = append(*f.log, "cancel "+id+" by "+by)
	f.shapes = append(f.shapes, sh)
	if f.cancelErr != nil {
		return domain.Slice{}, f.cancelErr
	}
	s := f.slice
	s.Status, s.Branch, s.PRURL = domain.SliceTodo, "", ""
	return s, nil
}

// handedBackSlice is a slice in progress whose agent handed its branch back.
func handedBackSlice() domain.Slice {
	return domain.Slice{ID: "s1", Name: "Render", Status: domain.SliceClaimed, Branch: "slice/handed",
		PRURL: "https://github.test/pr/1", Repo: "/repo"}
}

// The whole cancel: the agent stopped, then the store's write in the
// project's shape, then the worktree and branch the slice recorded discarded.
func TestCancelStopsTheAgentThenWritesThenDiscards(t *testing.T) {
	var log steps
	st := &fakeCancelStore{slice: handedBackSlice(), log: &log}
	tmux := &fakeStopper{live: map[string]string{"s1": "nat-s1"}, log: &log}
	w := &fakeWorktrees{}

	got, err := Cancel(context.Background(), st, tmux, w, store.Project{ID: "p"}, config.ProjectConfig{}, "s1", "Craig")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got.Status != domain.SliceTodo || got.Branch != "" {
		t.Errorf("cancelled = %+v, want it back at Todo with no branch", got)
	}
	if want := (steps{"kill nat-s1", "cancel s1 by Craig"}); !reflect.DeepEqual(log, want) {
		t.Errorf("steps = %v, want %v", log, want)
	}
	if !st.shapes[0].HasBranch || !st.shapes[0].HasAssignee {
		t.Errorf("shape = %+v, want the project's own", st.shapes[0])
	}
	if want := []worktreeCall{{dir: "/repo", branch: "slice/handed"}}; !reflect.DeepEqual(w.discards, want) {
		t.Errorf("discards = %+v, want %+v — the branch it had before the write cleared it", w.discards, want)
	}
}

// No live agent is nothing to stop, and a discard git refuses leaves the
// cancel standing.
func TestCancelWithNoAgentAndARefusedDiscard(t *testing.T) {
	var log steps
	st := &fakeCancelStore{slice: handedBackSlice(), log: &log}
	w := &fakeWorktrees{discardErr: errors.New("locked")}
	if _, err := Cancel(context.Background(), st, &fakeStopper{log: &log}, w, store.Project{}, config.ProjectConfig{}, "s1", "Craig"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if want := (steps{"cancel s1 by Craig"}); !reflect.DeepEqual(log, want) {
		t.Errorf("steps = %v, want the write alone", log)
	}
}

// Every refusal lands before anything is stopped or written.
func TestCancelRefusals(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name  string
		slice domain.Slice
		st    func(*fakeCancelStore)
		tmux  func(*fakeStopper)
		want  string
	}{
		{"todo", domain.Slice{Name: "Render", Status: domain.SliceTodo}, nil, nil, `"Render" is Todo: nothing has been started`},
		{"done", domain.Slice{Name: "Render", Status: domain.SliceDone}, nil, nil, `"Render" is Done: its work is merged`},
		{"another status", domain.Slice{Name: "Render", Status: "Parked", StatusName: "Parked"}, nil, nil,
			`"Render" is Parked: only a slice in progress can be cancelled`},
		{"unreadable slice", handedBackSlice(), func(f *fakeCancelStore) { f.sliceErr = boom }, nil, "load the slice: boom"},
		{"unreadable shape", handedBackSlice(), func(f *fakeCancelStore) { f.shapeErr = boom }, nil, "read the project's shape: boom"},
		{"unreadable tmux", handedBackSlice(), nil, func(f *fakeStopper) { f.liveErr = boom }, "could not read live sessions"},
		{"a failed kill", handedBackSlice(), nil, func(f *fakeStopper) { f.killErr = boom }, "stop its agent: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var log steps
			st := &fakeCancelStore{slice: tt.slice, log: &log}
			tmux := &fakeStopper{live: map[string]string{"s1": "nat-s1"}, log: &log}
			if tt.st != nil {
				tt.st(st)
			}
			if tt.tmux != nil {
				tt.tmux(tmux)
			}
			w := &fakeWorktrees{}
			_, err := Cancel(context.Background(), st, tmux, w, store.Project{}, config.ProjectConfig{}, "s1", "Craig")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
			if len(st.shapes) != 0 || len(w.discards) != 0 {
				t.Errorf("wrote %d cancels and %d discards, want none", len(st.shapes), len(w.discards))
			}
		})
	}
}

// A failed write is the cancel's failure, and nothing is discarded behind it.
func TestCancelReportsAFailedWrite(t *testing.T) {
	var log steps
	st := &fakeCancelStore{slice: handedBackSlice(), log: &log, cancelErr: errors.New("boom")}
	w := &fakeWorktrees{}
	if _, err := Cancel(context.Background(), st, &fakeStopper{log: &log}, w, store.Project{}, config.ProjectConfig{}, "s1", "Craig"); err == nil {
		t.Fatal("Cancel: want the write's failure")
	}
	if len(w.discards) != 0 {
		t.Errorf("discards = %+v, want none after a failed write", w.discards)
	}
}

// fakeDeleteStore records the trash on the shared steps, and prunes as the
// pruner fake does.
type fakeDeleteStore struct {
	*fakePruner
	deleteErr error
	log       *steps
}

var _ DeleteStore = (*fakeDeleteStore)(nil)

func (f *fakeDeleteStore) DeleteSlice(_ context.Context, id string) error {
	*f.log = append(*f.log, "delete "+id)
	return f.deleteErr
}

// A slice in progress is stopped, trashed, and its work discarded; the
// milestone it emptied is pruned.
func TestDeleteInProgressStopsTheAgentAndDiscards(t *testing.T) {
	var log steps
	st := &fakeDeleteStore{fakePruner: &fakePruner{milestones: []domain.Milestone{{ID: "M1", Name: "M1"}}}, log: &log}
	tmux := &fakeStopper{live: map[string]string{"s1": "nat-s1"}, log: &log}
	w := &fakeWorktrees{}
	s := handedBackSlice()
	s.MilestoneID = "M1"

	removed, err := Delete(context.Background(), st, tmux, w, store.Project{ID: "p"}, config.ProjectConfig{}, s)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if want := (steps{"kill nat-s1", "delete s1"}); !reflect.DeepEqual(log, want) {
		t.Errorf("steps = %v, want %v", log, want)
	}
	if len(w.discards) != 1 || len(w.removes) != 0 {
		t.Errorf("discards = %+v, removes = %+v, want the one forced discard", w.discards, w.removes)
	}
	if !reflect.DeepEqual(removed, []string{"M1"}) {
		t.Errorf("removed = %v, want M1", removed)
	}
}

// A slice not in progress asks tmux nothing and has its worktree removed the
// safe way, as before.
func TestDeleteNotInProgressRemovesSafely(t *testing.T) {
	var log steps
	st := &fakeDeleteStore{fakePruner: &fakePruner{}, log: &log}
	tmux := &fakeStopper{liveErr: errors.New("no tmux"), log: &log}
	w := &fakeWorktrees{existing: map[string]string{"slice/done": "/wt"}}
	s := domain.Slice{ID: "s2", Status: domain.SliceDone, Branch: "slice/done", Repo: "/repo"}

	if _, err := Delete(context.Background(), st, tmux, w, store.Project{}, config.ProjectConfig{}, s); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(w.removes) != 1 || len(w.discards) != 0 {
		t.Errorf("removes = %+v, discards = %+v, want the one safe removal", w.removes, w.discards)
	}
}

// A kill that cannot be made refuses before the trash, and a failed trash is
// the delete's failure with no worktree touched.
func TestDeleteRefusals(t *testing.T) {
	var log steps
	st := &fakeDeleteStore{fakePruner: &fakePruner{}, log: &log}
	w := &fakeWorktrees{}
	tmux := &fakeStopper{live: map[string]string{"s1": "nat-s1"}, killErr: errors.New("boom"), log: &log}
	if _, err := Delete(context.Background(), st, tmux, w, store.Project{}, config.ProjectConfig{}, handedBackSlice()); err == nil ||
		!strings.Contains(err.Error(), "stop its agent") {
		t.Errorf("err = %v, want the kill's failure", err)
	}
	if want := (steps{"kill nat-s1"}); !reflect.DeepEqual(log, want) {
		t.Errorf("steps = %v, want nothing trashed", log)
	}

	log = nil
	st.deleteErr = errors.New("notion is down")
	if _, err := Delete(context.Background(), st, &fakeStopper{log: &log}, w, store.Project{}, config.ProjectConfig{}, handedBackSlice()); err == nil ||
		!strings.Contains(err.Error(), "delete the slice: notion is down") {
		t.Errorf("err = %v, want the trash's failure", err)
	}
	if len(w.discards)+len(w.removes) != 0 {
		t.Errorf("worktree touched after a failed trash: %+v %+v", w.discards, w.removes)
	}
}

// DiscardSliceWorktree finds the repository and branch as the launch placed
// them, asks git nothing for a slice with no repository, and reports a refusal.
func TestDiscardSliceWorktree(t *testing.T) {
	w := &fakeWorktrees{}
	if !DiscardSliceWorktree(w, handedBackSlice(), config.ProjectConfig{WorkingDir: "/project"}) {
		t.Error("DiscardSliceWorktree() = false, want it discarded")
	}
	if want := []worktreeCall{{dir: "/repo", branch: "slice/handed"}}; !reflect.DeepEqual(w.discards, want) {
		t.Errorf("discards = %+v, want %+v", w.discards, want)
	}

	w = &fakeWorktrees{}
	if !DiscardSliceWorktree(w, domain.Slice{ID: "t1"}, config.ProjectConfig{Backend: config.BackendSource}) || len(w.discards) != 0 {
		t.Errorf("discards = %+v, want git asked nothing for a task with no repository", w.discards)
	}

	w = &fakeWorktrees{discardErr: errors.New("locked")}
	if DiscardSliceWorktree(w, handedBackSlice(), config.ProjectConfig{}) {
		t.Error("DiscardSliceWorktree() = true, want the refusal reported")
	}
}

// A hand-back before a cancel is work thrown away: a launch after it is not
// told it is picking a hand-back up, and one handed back since is.
func TestHandedBackReadsOnlySinceTheLastCancel(t *testing.T) {
	handBack := "### Handed back\n\nDid it.\n\n"
	cancel := "Cancelled by Craig at 2026-10-03T23:14:05+01:00: the work so far was discarded and it is back at Todo.\n\n"
	if handedBack(handBack + cancel) {
		t.Error("a hand-back before a cancel read as one")
	}
	if !handedBack(handBack + cancel + handBack) {
		t.Error("a hand-back since the cancel did not read as one")
	}
}
