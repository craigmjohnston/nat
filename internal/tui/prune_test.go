package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// pruneLocal is testProject kept in a file of its own, opened as the store a
// move or delete writes through. M1: Config holds two Done slices; M3:
// Mutations holds none.
func pruneLocal(t *testing.T) (*store.Local, store.Project) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", home)
	seedLocalPlan(t, "local-1", testProject())
	path, err := store.LocalPath("local-1")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.OpenLocal(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st, store.Project{ID: "local-1", Name: "tracker", Local: true}
}

func planMilestones(t *testing.T, st *store.Local, sp store.Project) []string {
	t.Helper()
	plan, err := st.Plan(t.Context(), sp)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, m := range plan.Project.Milestones {
		names = append(names, m.Name)
	}
	return names
}

// The last slice moved out of a milestone takes it with it, and the note says
// so; the first one leaves it standing.
func TestMoveSliceRemovesTheMilestoneItEmptied(t *testing.T) {
	st, sp := pruneLocal(t)
	to := domain.Milestone{ID: "M3: Mutations", Name: "M3: Mutations", SelectType: notion.TypeSelect}

	got := runMsg(t, moveSlice(st, sp, "s1", "XDG config", "M1: Config", to)).(sliceSavedMsg)
	if got.err != nil || got.removed != nil {
		t.Fatalf("msg = %+v, want the move with nothing removed", got)
	}
	got = runMsg(t, moveSlice(st, sp, "s2", "Keyring", "M1: Config", to)).(sliceSavedMsg)
	if want := `Moved "Keyring" to M3: Mutations. Removed M1: Config, which no slice is filed under any more.`; got.note != want {
		t.Errorf("note = %q, want %q", got.note, want)
	}
	if !equal(got.removed, []string{"M1: Config"}) {
		t.Errorf("removed = %v, want M1: Config", got.removed)
	}
	if names := planMilestones(t, st, sp); !equal(names, []string{"M2: Board", "M3: Mutations"}) {
		t.Errorf("milestones = %v, want M1: Config gone", names)
	}
}

func TestDeleteSliceRemovesTheMilestoneItEmptied(t *testing.T) {
	st, sp := pruneLocal(t)
	del := func(id, name string) tea.Cmd {
		return deleteSlice(st, sp, config.ProjectConfig{}, &fakeLauncher{}, &fakeWorktrees{},
			domain.Slice{ID: id, Name: name, MilestoneID: "M1: Config"})
	}
	runMsg(t, del("s1", "XDG config"))
	got := runMsg(t, del("s2", "Keyring")).(sliceSavedMsg)
	if got.err != nil || !got.deleted || !equal(got.removed, []string{"M1: Config"}) {
		t.Errorf("msg = %+v, want the delete with M1: Config removed", got)
	}
	// M3: Mutations was empty before either delete, and stays.
	if names := planMilestones(t, st, sp); !equal(names, []string{"M2: Board", "M3: Mutations"}) {
		t.Errorf("milestones = %v, want only M1: Config gone", names)
	}
}

// The board's own milestone list drops a removed milestone as the write lands,
// for a delete and a move alike.
func TestAppDropsTheMilestoneAWriteRemoved(t *testing.T) {
	for name, msg := range map[string]sliceSavedMsg{
		"delete": {sliceID: "s5", deleted: true, removed: []string{"M3: Mutations"}},
		"move":   {sliceID: "s5", removed: []string{"M3: Mutations"}},
	} {
		t.Run(name, func(t *testing.T) {
			app := newWriteApp(t, &fakeNotion{})
			app.saved(msg)
			for _, m := range app.project.Milestones {
				if m.Name == "M3: Mutations" {
					t.Errorf("milestones = %+v, want M3: Mutations dropped", app.project.Milestones)
				}
			}
			if len(app.project.Milestones) != 2 {
				t.Errorf("milestones = %+v, want the other two kept", app.project.Milestones)
			}
		})
	}
}

// Nothing removed, or no plan on the board yet, leaves the list alone.
func TestDropMilestonesWithNothingToDrop(t *testing.T) {
	app := newWriteApp(t, &fakeNotion{})
	before := app.project
	app.dropMilestones(nil)
	if app.project != before {
		t.Error("an empty drop rebuilt the plan")
	}
	app.project = nil
	app.dropMilestones([]string{"M1: Config"})
	if app.project != nil {
		t.Error("a drop with no plan made one")
	}
}
