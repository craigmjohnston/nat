package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// fakePRReader stands in for gh's own PR listing, answered by directory —
// one entry per repository the plan spans, exactly as [PRReader.OpenPRs] does
// — and for the per-pull-request reading pr-status settles an absent one by.
type fakePRReader struct {
	open  map[string]map[string]gh.PRStatus
	err   map[string]error
	dirs  []string
	calls int

	// view answers ViewPR by ref, viewErr fails it, viewed records what was
	// asked about — the settling of an absent pull request.
	view    map[string]gh.PR
	viewErr error
	viewed  []string
}

func (f *fakePRReader) OpenPRs(dir string) (map[string]gh.PRStatus, error) {
	f.dirs = append(f.dirs, dir)
	f.calls++
	if err := f.err[dir]; err != nil {
		return nil, err
	}
	return f.open[dir], nil
}

func (f *fakePRReader) ViewPR(dir, ref string) (gh.PR, error) {
	f.viewed = append(f.viewed, ref)
	if f.viewErr != nil {
		return gh.PR{}, f.viewErr
	}
	return f.view[ref], nil
}

// The rest of [GH] pr-status never calls; stubbed so *fakePRReader can stand
// in for the whole seam.
func (f *fakePRReader) CreatePR(dir, branch, title, body string) (string, error) { return "", nil }
func (f *fakePRReader) MergePR(dir, ref string) error                            { return nil }
func (f *fakePRReader) CommentPR(dir, ref, body string) (string, error)          { return "", nil }
func (f *fakePRReader) ListPRsForHead(dir, branch string) ([]gh.HeadPR, error)   { return nil, nil }

func slicePageForStatus(id, name, status, milestone, pr string) notion.Page {
	props := map[string]notion.PropertyValue{
		notion.PropName:   title(name),
		notion.PropStatus: notion.NewSelect(status),
	}
	if milestone != "" {
		props[notion.PropMilestone] = notion.NewSelect(milestone)
	}
	if pr != "" {
		props[notion.PropPR] = notion.NewURL(pr)
	}
	return notion.Page{ID: id, URL: "https://notion.so/" + id, Properties: props}
}

func TestPRStatusReportsReadiness(t *testing.T) {
	api := &fakeAPI{
		dataSources: map[string]notion.DataSource{"slices-ds": selectMilestoneSlicesDS("M1")},
		pages: map[string][]notion.Page{
			"slices-ds": {
				slicePageForStatus("s1", "Awaiting review", notion.SliceInProgress, "M1", "https://github.test/craig/nat/pull/1"),
				slicePageForStatus("s2", "Ready to merge", notion.SliceDone, "M1", "https://github.test/craig/nat/pull/2"),
				slicePageForStatus("s3", "Landed", notion.SliceDone, "M1", "https://github.test/craig/nat/pull/3"),
				slicePageForStatus("s4", "Not out yet", notion.SliceTodo, "M1", ""),
			},
		},
	}
	env, out := testEnv(testConfig(t), api)
	reader := &fakePRReader{open: map[string]map[string]gh.PRStatus{
		"/tmp/nat": {
			"https://github.test/craig/nat/pull/1": {Approved: false, Mergeable: true},
			"https://github.test/craig/nat/pull/2": {Approved: true, Mergeable: true},
			// pull/3 is absent: gh no longer lists it as open, so it has landed.
		},
	}}
	env.NewGH = func() GH { return reader }

	err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if reader.calls != 1 {
		t.Errorf("gh calls = %d, want one listing for the one repository", reader.calls)
	}
	for _, want := range []string{
		"Awaiting review — awaiting review — https://github.test/craig/nat/pull/1",
		"Ready to merge — ready to merge — https://github.test/craig/nat/pull/2",
		"Landed — unread — https://github.test/craig/nat/pull/3",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "Not out yet") {
		t.Errorf("output should not mention a slice with no pull request:\n%s", out.String())
	}
}

func TestPRStatusJSON(t *testing.T) {
	api := &fakeAPI{
		dataSources: map[string]notion.DataSource{"slices-ds": selectMilestoneSlicesDS("M1")},
		pages: map[string][]notion.Page{
			"slices-ds": {
				slicePageForStatus("s1", "Awaiting review", notion.SliceInProgress, "M1", "https://github.test/craig/nat/pull/1"),
			},
		},
	}
	env, out := testEnv(testConfig(t), api)
	reader := &fakePRReader{open: map[string]map[string]gh.PRStatus{
		"/tmp/nat": {"https://github.test/craig/nat/pull/1": {Approved: false, Mergeable: false}},
	}}
	env.NewGH = func() GH { return reader }

	err := Run(context.Background(), []string{"pr-status", "--json", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-status --json: %v", err)
	}
	var got prStatusDoc
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	want := prStatusDoc{Slices: []prStatusSliceJSON{
		{SliceID: "s1", Name: "Awaiting review", PR: "https://github.test/craig/nat/pull/1", Readiness: "awaiting review"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("json = %+v\nwant %+v", got, want)
	}
}

func TestPRStatusNoSlicesWorthReading(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageForStatus("s1", "Not out yet", notion.SliceTodo, "", "")},
		},
	}
	env, out := testEnv(testConfig(t), api)
	reader := &fakePRReader{}
	env.NewGH = func() GH { return reader }

	err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if reader.calls != 0 {
		t.Errorf("gh calls = %d, want none: nothing on the plan is worth reading", reader.calls)
	}
	if !strings.Contains(out.String(), "_none_") {
		t.Errorf("output = %q, want it to say there is nothing", out.String())
	}
}

func TestPRStatusLeavesAnUnreadableRepositoryOut(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageForStatus("s1", "Awaiting review", notion.SliceInProgress, "",
				"https://github.test/craig/nat/pull/1")},
		},
	}
	env, out := testEnv(testConfig(t), api)
	reader := &fakePRReader{err: map[string]error{"/tmp/nat": errors.New("gh: not authenticated")}}
	env.NewGH = func() GH { return reader }

	err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if !strings.Contains(out.String(), "Awaiting review — unread — https://github.test/craig/nat/pull/1") {
		t.Errorf("output = %q, want the slice reported unread", out.String())
	}
}

// An in-progress slice whose pull request the listing no longer names is
// asked about directly, and one that merged is marked Done — with a nudge, so
// a board watching the marker sees the change.
func TestPRStatusMarksAMergedAbsentPRDone(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageForStatus("s1", "Merged on GitHub", notion.SliceInProgress, "",
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, out := testEnv(testConfig(t), api)
	var nudges int
	env.Nudge = func() { nudges++ }
	reader := &fakePRReader{
		open: map[string]map[string]gh.PRStatus{"/tmp/nat": {}},
		view: map[string]gh.PR{"https://github.test/craig/nat/pull/7": {State: gh.PRStateMerged}},
	}
	env.NewGH = func() GH { return reader }

	err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if want := []string{"https://github.test/craig/nat/pull/7"}; !equalLines(reader.viewed, want) {
		t.Errorf("viewed = %v, want the absent pull request asked about", reader.viewed)
	}
	if len(api.updates) != 1 || api.updates[0].id != "s1" {
		t.Fatalf("updates = %+v, want the slice marked Done", api.updates)
	}
	if name := api.updates[0].props[notion.PropStatus].SelectName(); name != notion.SliceDone {
		t.Errorf("status = %q, want %q", name, notion.SliceDone)
	}
	if nudges != 1 {
		t.Errorf("nudges = %d, want one for the write", nudges)
	}
	if !strings.Contains(out.String(), "Merged on GitHub — unread — ") {
		t.Errorf("output = %q, want the settled slice reported unread", out.String())
	}
}

// A slice Done under the old rule — at approve, rather than at the merge —
// whose pull request the listing still names is written back to In progress:
// the un-done rule, with a nudge like any other write this read makes.
func TestPRStatusReopensADoneSliceWithAnOpenPR(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageForStatus("s2", "Awaiting merge", notion.SliceDone, "",
				"https://github.test/craig/nat/pull/2")},
		},
	}
	env, out := testEnv(testConfig(t), api)
	var nudges int
	env.Nudge = func() { nudges++ }
	reader := &fakePRReader{open: map[string]map[string]gh.PRStatus{
		"/tmp/nat": {"https://github.test/craig/nat/pull/2": {Approved: true, Mergeable: true}},
	}}
	env.NewGH = func() GH { return reader }

	err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if len(api.updates) != 1 || api.updates[0].id != "s2" {
		t.Fatalf("updates = %+v, want the slice reopened", api.updates)
	}
	if name := api.updates[0].props[notion.PropStatus].SelectName(); name != notion.SliceInProgress {
		t.Errorf("status = %q, want %q", name, notion.SliceInProgress)
	}
	if nudges != 1 {
		t.Errorf("nudges = %d, want one for the write", nudges)
	}
	// The readiness reading itself is unaffected: still ready to merge.
	if !strings.Contains(out.String(), "Awaiting merge — ready to merge — ") {
		t.Errorf("output = %q, want the readiness reported as it read", out.String())
	}
}

// ReopenUnmerged writes the local plan first, exactly as every other single-
// slice write does now: a push to the workspace that fails no longer leaves
// the slice unreopened, only unsent — the local write lands, the readiness
// reading still reports, and the slice is left dirty for a later sync to send
// what the push could not.
func TestPRStatusReopensLocallyEvenWhenThePushFails(t *testing.T) {
	cfg := testConfig(t)
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageForStatus("s2", "Awaiting merge", notion.SliceDone, "",
				"https://github.test/craig/nat/pull/2")},
		},
		updateErr: errors.New("notion is down"),
	}
	env, out := testEnv(cfg, api)
	var nudges int
	env.Nudge = func() { nudges++ }
	reader := &fakePRReader{open: map[string]map[string]gh.PRStatus{
		"/tmp/nat": {"https://github.test/craig/nat/pull/2": {Approved: true, Mergeable: true}},
	}}
	env.NewGH = func() GH { return reader }

	err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if nudges != 1 {
		t.Errorf("nudges = %d, want one for the local write that landed", nudges)
	}
	if !strings.Contains(out.String(), "Awaiting merge — ready to merge — ") {
		t.Errorf("output = %q, want the readiness reported", out.String())
	}

	path, err := store.LocalPath("project-1")
	if err != nil {
		t.Fatalf("resolve the plan path: %v", err)
	}
	local, err := store.OpenLocal(path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	defer func() {
		if err := local.Close(); err != nil {
			t.Errorf("close the plan: %v", err)
		}
	}()
	dirty, err := local.Dirty(context.Background(), "s2")
	if err != nil {
		t.Fatalf("read whether the slice is dirty: %v", err)
	}
	if !dirty {
		t.Error("slice not marked dirty, want the failed push left for a later sync to send")
	}
}

// The other thing absence means: a pull request closed unmerged is work going
// round again, and the slice is left exactly as it is — no write, no nudge.
func TestPRStatusLeavesAClosedAbsentPRAlone(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageForStatus("s1", "Closed unmerged", notion.SliceInProgress, "",
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	var nudges int
	env.Nudge = func() { nudges++ }
	reader := &fakePRReader{
		open: map[string]map[string]gh.PRStatus{"/tmp/nat": {}},
		view: map[string]gh.PR{"https://github.test/craig/nat/pull/7": {State: gh.PRStateClosed}},
	}
	env.NewGH = func() GH { return reader }

	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if len(api.updates) != 0 || nudges != 0 {
		t.Errorf("updates = %+v, nudges = %d, want nothing written for a closed pull request", api.updates, nudges)
	}
}

// A reading that fails settles nothing: it is logged, the slice reads unread,
// and the next run asks again.
func TestPRStatusLeavesAnUnviewableAbsentPRAlone(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageForStatus("s1", "Unreadable", notion.SliceInProgress, "",
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, out := testEnv(testConfig(t), api)
	reader := &fakePRReader{
		open:    map[string]map[string]gh.PRStatus{"/tmp/nat": {}},
		viewErr: errors.New("gh: not authenticated"),
	}
	env.NewGH = func() GH { return reader }

	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing concluded from a reading that never happened", api.updates)
	}
	if !strings.Contains(out.String(), "Unreadable — unread — ") {
		t.Errorf("output = %q, want the slice reported unread", out.String())
	}
}

func TestPRStatusGroupsRepositoriesBySliceRepo(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {
				slicePageWithAllFields("s1", "In repo one", notion.SliceInProgress, "", "", "", "https://github.test/craig/nat/pull/1", "/repo/one"),
				slicePageWithAllFields("s2", "In repo two", notion.SliceInProgress, "", "", "", "https://github.test/craig/nat/pull/2", "/repo/two"),
			},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	reader := &fakePRReader{open: map[string]map[string]gh.PRStatus{
		"/repo/one": {"https://github.test/craig/nat/pull/1": {Approved: true, Mergeable: true}},
		"/repo/two": {"https://github.test/craig/nat/pull/2": {Approved: true, Mergeable: true}},
	}}
	env.NewGH = func() GH { return reader }

	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if reader.calls != 2 {
		t.Errorf("gh calls = %d, want one listing per repository", reader.calls)
	}
}

func TestPRStatusRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-status", "--bogus", "--project", "project-1"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestPRStatusRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-status", "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestPRStatusReportsAFailedQuery(t *testing.T) {
	api := &fakeAPI{queryErr: map[string]error{"slices-ds": errors.New("notion is down")}}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "load slices") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}

// A Done slice whose reopen cannot even be written locally — not merely a
// push that failed, which is left dirty and retried, but the local write
// itself refusing — is logged and left alone: the readiness reading still
// reports it, nothing is nudged, and the run itself still succeeds, since
// nothing else it read depends on this one slice's own state.
func TestPRStatusLeavesADoneSliceAloneWhenItCannotBeReopenedLocally(t *testing.T) {
	cfg := testConfig(t)
	seedHydratedSlice(t, "project-1", testSliceID, "Awaiting merge", "Done", func(db *sql.DB) {
		if _, err := db.Exec(`UPDATE slices SET pr = ? WHERE id = ?`,
			"https://github.test/craig/nat/pull/2", testSliceID); err != nil {
			t.Fatalf("seed the PR: %v", err)
		}
		if _, err := db.Exec(`DROP TABLE sync`); err != nil {
			t.Fatalf("break the plan's sync table: %v", err)
		}
	})
	env, out := testEnv(cfg, &fakeAPI{})
	var nudges int
	env.Nudge = func() { nudges++ }
	reader := &fakePRReader{open: map[string]map[string]gh.PRStatus{
		"/tmp/nat": {"https://github.test/craig/nat/pull/2": {Approved: true, Mergeable: true}},
	}}
	env.NewGH = func() GH { return reader }

	err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env)

	if err != nil {
		t.Fatalf("pr-status: %v, want it to succeed despite the one slice it could not reopen", err)
	}
	if nudges != 0 {
		t.Errorf("nudges = %d, want none: nothing was actually written", nudges)
	}
	if !strings.Contains(out.String(), "Awaiting merge — ready to merge — ") {
		t.Errorf("output = %q, want the readiness reported despite the failed reopen", out.String())
	}
}

// The plan is read from the local file once it has been hydrated, and a
// file that cannot even answer that fails the command outright — there is
// nothing to read pull requests for without a plan.
func TestPRStatusReportsAFailedLocalPlanRead(t *testing.T) {
	cfg := testConfig(t)
	seedHydratedSlice(t, "project-1", testSliceID, "Write the UI", "In progress", func(db *sql.DB) {
		if _, err := db.Exec(`DROP TABLE milestones`); err != nil {
			t.Fatalf("break the plan's milestones table: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})

	err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "load slices") {
		t.Errorf("err = %v, want the failed local read named", err)
	}
}

// TestWorthReadingPRAndReadinessOf pins the two pure rules directly, since
// every combination is cheaper to state here than to drive through a whole
// plan and a fake gh.
func TestWorthReadingPRAndReadinessOf(t *testing.T) {
	if worthReadingPR(domain.Slice{PRURL: "", Status: domain.SliceClaimed}) {
		t.Error("a slice with no pull request is never worth reading")
	}
	if worthReadingPR(domain.Slice{PRURL: "x", Status: domain.SliceTodo}) {
		t.Error("a Todo slice has not produced a pull request worth reading")
	}
	if !worthReadingPR(domain.Slice{PRURL: "x", Status: domain.SliceClaimed}) {
		t.Error("a slice in progress with a pull request is worth reading")
	}
	if !worthReadingPR(domain.Slice{PRURL: "x", Status: domain.SliceDone}) {
		t.Error("a Done slice with a pull request is worth reading")
	}
	if got := readinessOf(gh.PRStatus{Approved: true, Mergeable: true}); got != domain.PRReadyToMerge {
		t.Errorf("readinessOf(approved+mergeable) = %v, want ready to merge", got)
	}
	if got := readinessOf(gh.PRStatus{Approved: true, Mergeable: false}); got != domain.PRAwaitingReview {
		t.Errorf("readinessOf(approved, not mergeable) = %v, want awaiting review", got)
	}
	if got := readinessOf(gh.PRStatus{}); got != domain.PRAwaitingReview {
		t.Errorf("readinessOf({}) = %v, want awaiting review", got)
	}
}
