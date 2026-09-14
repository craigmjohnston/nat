package tui

import (
	"errors"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// TestAppInfoErrMsgFailsTheInfoScreen pins the one branch a failed fetch of
// the project page's body takes: the info screen reports the failure itself,
// never notionErrMsg's board-wide one — see infoErrMsg's own doc comment.
func TestAppInfoErrMsgFailsTheInfoScreen(t *testing.T) {
	a := NewApp(testConfig(t), &fakeNotion{})
	boom := errors.New("boom")
	a.Update(infoErrMsg{err: boom})
	if a.info.state != infoFailed || !errors.Is(a.info.err, boom) {
		t.Errorf("info state = %v, err = %v, want failed with the fetch's own error", a.info.state, a.info.err)
	}
}

// TestAppSliceBodyLoadedReportsAFailure pins the edit form's own load
// failure: the app-wide error banner, since there is no form yet for the
// failure to be reported on.
func TestAppSliceBodyLoadedReportsAFailure(t *testing.T) {
	a := NewApp(testConfig(t), &fakeNotion{})
	boom := errors.New("boom")
	a.busy = true
	a.sliceBodyLoaded(sliceBodyMsg{err: boom})
	if a.busy {
		t.Error("busy should clear once the load answers, failed or not")
	}
	if !errors.Is(a.err, boom) {
		t.Errorf("err = %v, want the load's own failure", a.err)
	}
}

// TestFetchInfoReportsABodyReadFailure pins fetchInfo's own error path: a
// store that cannot answer for a page's body comes back as infoErrMsg,
// wrapped so it names what was being loaded.
func TestFetchInfoReportsABodyReadFailure(t *testing.T) {
	boom := errors.New("boom")
	client := &fakeNotion{blocks: func(string) ([]notion.Block, error) { return nil, boom }}
	msg := runMsg(t, NewApp(testConfig(t), client).fetchInfo(store.Over(client), "page-1"))
	got, ok := msg.(infoErrMsg)
	if !ok || !errors.Is(got.err, boom) {
		t.Errorf("msg = %+v (ok=%v), want infoErrMsg wrapping the read's failure", msg, ok)
	}
}

// TestAppStoreForFailsWhenThePlanFileCannotOpen pins App.storeFor's own
// failure — a plan path that cannot be opened at all, occupied by a
// directory rather than a database file — reported the way every other
// synchronous failure in this file is: a.err, with nothing started.
func TestAppStoreForFailsWhenThePlanFileCannotOpen(t *testing.T) {
	cfg := testConfig(t)
	breakLocalPlanFile(t, testProjectID)
	a := NewApp(cfg, &fakeNotion{})

	if cmd := a.startLoad(false); cmd != nil {
		t.Error("a plan file that cannot open should start nothing")
	}
	if a.err == nil {
		t.Error("want the open failure reported as the app's own error")
	}
}

// TestAppStoreForFailsWhenTheLocalPathCannotResolve pins storeFor's other
// failure branch, distinct from a plan file that will not open: no home
// directory for store.LocalPath to resolve one under at all.
func TestAppStoreForFailsWhenTheLocalPathCannotResolve(t *testing.T) {
	cfg := testConfig(t)
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	a := NewApp(cfg, &fakeNotion{})

	if cmd := a.startLoad(false); cmd != nil {
		t.Error("a plan path that cannot resolve should start nothing")
	}
	if a.err == nil {
		t.Error("want the resolve failure reported as the app's own error")
	}
}

// storeForBrokenApp returns an app whose active project's store can never be
// opened — the plan path is occupied by a directory — with a plan already on
// the board (set directly, not through a load, since there is no store to
// load one through). It is the fixture every activeStore "!ok" branch below
// shares: canWrite and its own siblings all pass — a client, an active
// project, a row to act on — and only the store itself fails to open.
func storeForBrokenApp(t *testing.T, client NotionAPI) *App {
	t.Helper()
	cfg := testConfig(t)
	breakLocalPlanFile(t, testProjectID)
	p := testProject()
	a := NewApp(cfg, client)
	a.project = &p
	a.board.hideDone = false
	a.board.SetProject(&p)
	return a
}

// Every write flow below refuses in the same way once its store will not
// open: nothing is dispatched, and (where the flow is the one to ask
// activeStore itself, rather than a form built ahead of it) nothing is
// marked busy.

func TestAppEditSliceRefusedWhenTheStoreCannotOpen(t *testing.T) {
	a := storeForBrokenApp(t, &fakeNotion{})
	a.board.cursor = rowTodoSlice
	if cmd := a.editSlice(); cmd != nil {
		t.Error("want nothing started against a store that cannot open")
	}
	if a.busy {
		t.Error("want busy left false: nothing was actually started")
	}
}

func TestAppStartApproveRefusedWhenTheStoreCannotOpen(t *testing.T) {
	a := storeForBrokenApp(t, &fakeNotion{})
	s := domain.Slice{ID: "s4", Name: "Board screen", Branch: "slice/board-screen"}
	if cmd := a.startApprove(s, t.TempDir()); cmd != nil {
		t.Error("want nothing started against a store that cannot open")
	}
}

func TestAppPrOpenedRefusedWhenTheStoreCannotOpen(t *testing.T) {
	a := storeForBrokenApp(t, &fakeNotion{})
	s := domain.Slice{ID: "s4", Name: "Board screen"}
	_, cmd := a.prOpened(prOpenedMsg{slice: s, url: "https://example.test/pr/9"})
	if cmd != nil {
		t.Error("want nothing dispatched against a store that cannot open")
	}
}

func TestAppStartAgentRefusedWhenTheStoreCannotOpen(t *testing.T) {
	a := storeForBrokenApp(t, &fakeNotion{})
	s := domain.Slice{ID: "s5", Name: "Info view"}
	if cmd := a.startAgent(s, "/tmp/repo", config.AgentModel{}, true); cmd != nil {
		t.Error("want nothing started against a store that cannot open")
	}
}

func TestAppMergeChosenRefusedWhenTheStoreCannotOpen(t *testing.T) {
	a := storeForBrokenApp(t, &fakeNotion{})
	if cmd := a.mergeChosen(gh.PR{Number: 9}, choiceMerge); cmd != nil {
		t.Error("want nothing started against a store that cannot open")
	}
}

func TestAppRefreshPRStatesRefusedWhenTheStoreCannotOpen(t *testing.T) {
	a := storeForBrokenApp(t, &fakeNotion{})
	a.prReader = &fakePRReader{}
	if cmd := a.refreshPRStates(); cmd != nil {
		t.Error("want no reading started against a store that cannot open")
	}
}

func TestAppReleaseChosenRefusedWhenTheStoreCannotOpen(t *testing.T) {
	a := storeForBrokenApp(t, &fakeNotion{})
	s := domain.Slice{ID: "s4", Name: "Board screen", Status: domain.SliceClaimed}
	if cmd := a.releaseChosen(s, choiceRelease); cmd != nil {
		t.Error("want nothing started against a store that cannot open")
	}
	if a.busy {
		t.Error("want busy left false: nothing was actually started")
	}
}

func TestDeleteSliceFormSaveRefusedWhenTheStoreCannotOpen(t *testing.T) {
	a := storeForBrokenApp(t, &fakeNotion{})
	f := &DeleteSliceForm{sliceID: "s5", sliceName: "Info view", confirmed: true}
	if cmd := f.save(a); cmd != nil {
		t.Error("want nothing dispatched against a store that cannot open")
	}
}

func TestMoveSliceFormSaveRefusedWhenTheStoreCannotOpen(t *testing.T) {
	a := storeForBrokenApp(t, &fakeNotion{})
	target := domain.Milestone{ID: "M1: Config", Name: "M1: Config"}
	f := &MoveSliceForm{sliceID: "s5", sliceName: "Info view",
		targets: map[string]domain.Milestone{target.ID: target}, chosen: target.ID}
	if cmd := f.save(a); cmd != nil {
		t.Error("want nothing dispatched against a store that cannot open")
	}
}

func TestSliceFormSaveRefusedWhenTheStoreCannotOpen(t *testing.T) {
	a := storeForBrokenApp(t, &fakeNotion{})
	f := &SliceForm{mode: sliceFormEdit, sliceID: "s5", title: "Renamed"}
	if cmd := f.save(a); cmd != nil {
		t.Error("want nothing dispatched against a store that cannot open")
	}
}

func TestRefreshSliceRefusedWhenTheStoreCannotOpen(t *testing.T) {
	a := storeForBrokenApp(t, &fakeNotion{})
	if cmd := a.refreshSlice("s5"); cmd != nil {
		t.Error("want nothing dispatched against a store that cannot open")
	}
}

// TestADoneSliceWithAnOpenPRLeftUnreopenedWhenTheWriteFails pins
// refreshPRStates' own log-and-continue branch: a Done slice whose pull
// request reads open is meant to be written back to In progress
// (actions.ReopenUnmerged), and a write that fails — the file's own sync
// bookkeeping broken, so the local half of the write cannot mark the slice
// dirty — is logged and left rather than losing the rest of the reading.
// actions.ReopenUnmerged reads the slice fresh before it writes it
// ([store.Mirrored.Slice]), which would silently re-create a dropped row
// from the workspace and heal right past a broken write that way — so what
// has to break here is the write itself, not the row the read finds.
func TestADoneSliceWithAnOpenPRLeftUnreopenedWhenTheWriteFails(t *testing.T) {
	app, _ := prStateApp(t)
	p := prStatePlan()
	cmd := landPlan(t, app, p)
	breakLocalColumn(t, testProjectID, "sync", "dirty")

	runPRRead(t, app, cmd)

	// The reading still settles what it could: the plan's copy of the slice
	// is untouched (still Done), since the write that would have reopened it
	// never landed.
	patched := app.project.Slices[sliceIndex(app.project.Slices, donePR)]
	if patched.Status != domain.SliceDone {
		t.Errorf("status = %v, want Done left alone: the reopening write failed", patched.Status)
	}
}
