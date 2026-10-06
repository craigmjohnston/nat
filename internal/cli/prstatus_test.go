package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// fakePRReader stands in for gh's batched reading: it answers each pull
// request asked about by its URL in prs, each branch by name in heads, the
// detail with detail, and records every query — or fails the whole reading
// with err. A URL not in prs is one the reading did not find: unread.
type fakePRReader struct {
	noActions
	prs     map[string]gh.PR
	heads   map[string][]gh.HeadPR
	detail  *gh.PR
	rate    *gh.RateLimit
	err     error
	queries []gh.BatchQuery
	calls   int
	// polls counts the reads that came through PollPRs; outlook answers
	// Outlook.
	polls   int
	outlook *gh.Outlook
	cost    int

	// viewed records every ViewPR — which nothing pr-status does may make.
	viewed []string

	// logs answers FailedLog by job (or run, where no job is named), logErr
	// fails it, logged records what was asked for.
	logs   map[string]string
	logErr error
	logged []string
	// comments answers ReviewComments, the review a launch on a pull request gathers.
	comments string
}

// PollPRs is the fake's ReadPRs, counted as a poll.
func (f *fakePRReader) PollPRs(q gh.BatchQuery) (gh.Batch, error) {
	f.polls++
	return f.ReadPRs(q)
}

// Outlook is outlook where the test set one, else the poll alone.
func (f *fakePRReader) Outlook(poll time.Duration) gh.Outlook {
	if f.outlook != nil {
		return *f.outlook
	}
	return gh.Outlook{PollAfter: poll}
}

func (f *fakePRReader) ReadPRs(q gh.BatchQuery) (gh.Batch, error) {
	f.calls++
	f.queries = append(f.queries, q)
	batch := gh.Batch{PRs: map[gh.PRRef]gh.PR{}, Heads: map[gh.HeadRef][]gh.HeadPR{}, RateLimit: f.rate, Cost: f.cost}
	if f.err != nil {
		return gh.Batch{}, f.err
	}
	for url, pr := range f.prs {
		ref, _ := gh.ParsePRURL(url)
		if slices.Contains(q.PRs, ref) {
			batch.PRs[ref] = pr
		}
	}
	for _, h := range q.Heads {
		if prs, ok := f.heads[h.Branch]; ok {
			batch.Heads[h] = prs
		}
	}
	if q.Detail != nil && f.detail != nil {
		d := *f.detail
		batch.Detail = &d
	}
	return batch, nil
}

// openPR is an open pull request as the reading decodes one: approved or not,
// mergeable or not, with checks.
func openPR(approved, mergeable bool, checks ...gh.Check) gh.PR {
	pr := gh.PR{State: "OPEN", BaseRefName: "main", Checks: checks}
	if approved {
		pr.ReviewDecision = "APPROVED"
	}
	if mergeable {
		pr.Mergeable = "MERGEABLE"
	}
	return pr
}

func (f *fakePRReader) FailedLog(dir string, ref gh.ActionsRef) (string, error) {
	key := ref.Job
	if key == "" {
		key = ref.Run
	}
	f.logged = append(f.logged, key)
	if f.logErr != nil {
		return "", f.logErr
	}
	return f.logs[key], nil
}

func (f *fakePRReader) ReviewComments(dir, ref string) (string, error) { return f.comments, nil }
func (f *fakePRReader) Checks(dir, ref string) (string, error)         { return "", nil }

func (f *fakePRReader) ViewPR(dir, ref string) (gh.PR, error) {
	f.viewed = append(f.viewed, ref)
	return gh.PR{}, errors.New("pr-status views no pull request")
}

// The rest of [GH] pr-status never calls; stubbed so *fakePRReader can stand
// in for the whole seam.
func (f *fakePRReader) CreatePR(dir, branch, title, body string) (string, error) { return "", nil }
func (f *fakePRReader) MergePR(dir, ref string) error                            { return nil }
func (f *fakePRReader) CommentPR(dir, ref, body string) (string, error)          { return "", nil }

// withDoneWorktree gives env a worktree on the agent branch of each named
// slice — what makes a Done slice worth asking about.
func withDoneWorktree(env *Env, names ...string) *fakeSessionWorktrees {
	var branches []string
	for _, name := range names {
		branches = append(branches, actions.SliceBranch(domain.Slice{Name: name}))
	}
	w := &fakeSessionWorktrees{branches: map[string][]string{"/tmp/nat": branches}}
	env.NewWorktrees = func() actions.Worktrees { return w }
	return w
}

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
				slicePageForStatus("s2", "Ready to merge", notion.SliceInProgress, "M1", "https://github.test/craig/nat/pull/2"),
				slicePageForStatus("s3", "Landed", notion.SliceDone, "M1", "https://github.test/craig/nat/pull/3"),
				slicePageForStatus("s4", "Not out yet", notion.SliceTodo, "M1", ""),
			},
		},
	}
	env, out := testEnv(testConfig(t), api)
	reader := &fakePRReader{prs: map[string]gh.PR{
		"https://github.test/craig/nat/pull/1": openPR(false, true),
		"https://github.test/craig/nat/pull/2": openPR(true, true),
		// pull/3 is Done with no worktree: not asked about at all.
	}}
	env.NewGH = func() GH { return reader }

	err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if reader.calls != 1 {
		t.Errorf("gh calls = %d, want one reading", reader.calls)
	}
	want := []gh.PRRef{{Owner: "craig", Repo: "nat", Number: 1}, {Owner: "craig", Repo: "nat", Number: 2}}
	if !reflect.DeepEqual(reader.queries[0].PRs, want) {
		t.Errorf("asked about %+v, want the two in progress and not the Done one with no worktree", reader.queries[0].PRs)
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
	reader := &fakePRReader{prs: map[string]gh.PR{"https://github.test/craig/nat/pull/1": openPR(false, false)}}
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
		{SliceID: "s1", Name: "Awaiting review", PR: "https://github.test/craig/nat/pull/1", Readiness: "awaiting review",
			Base: "main", Checks: &prChecksJSON{Verdict: "none", Failing: []prCheckJSON{}}},
	}, Branches: []branchJSON{}, Sessions: []sessionPRsJSON{}}
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

func TestPRStatusLeavesAFailedReadingUnread(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageForStatus("s1", "Awaiting review", notion.SliceInProgress, "",
				"https://github.test/craig/nat/pull/1")},
		},
	}
	env, out := testEnv(testConfig(t), api)
	reader := &fakePRReader{err: errors.New("gh: not authenticated")}
	env.NewGH = func() GH { return reader }

	err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if !strings.Contains(out.String(), "Awaiting review — unread — https://github.test/craig/nat/pull/1") {
		t.Errorf("output = %q, want the slice reported unread", out.String())
	}
}

// An in-progress slice whose pull request reads merged is marked Done — with
// a nudge, so a board watching the marker sees the change — off the reading
// itself, with no view of the pull request.
func TestPRStatusMarksAMergedPRDone(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageForStatus("s1", "Merged on GitHub", notion.SliceInProgress, "",
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, out := testEnv(testConfig(t), api)
	var nudges int
	env.Nudge = func() { nudges++ }
	reader := &fakePRReader{prs: map[string]gh.PR{"https://github.test/craig/nat/pull/7": {State: gh.PRStateMerged}}}
	env.NewGH = func() GH { return reader }

	err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if len(reader.viewed) != 0 {
		t.Errorf("viewed = %v, want no pull request viewed", reader.viewed)
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
// whose worktree is still there and whose pull request reads open is written
// back to In progress: the un-done rule, with a nudge like any other write
// this read makes.
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
	reader := &fakePRReader{prs: map[string]gh.PR{"https://github.test/craig/nat/pull/2": openPR(true, true)}}
	env.NewGH = func() GH { return reader }
	withDoneWorktree(&env, "Awaiting merge")

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
	reader := &fakePRReader{prs: map[string]gh.PR{"https://github.test/craig/nat/pull/2": openPR(true, true)}}
	env.NewGH = func() GH { return reader }
	withDoneWorktree(&env, "Awaiting merge")

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

// A pull request closed unmerged is work going round again, and the slice is
// left exactly as it is — no write, no nudge.
func TestPRStatusLeavesAClosedPRAlone(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageForStatus("s1", "Closed unmerged", notion.SliceInProgress, "",
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	var nudges int
	env.Nudge = func() { nudges++ }
	reader := &fakePRReader{prs: map[string]gh.PR{"https://github.test/craig/nat/pull/7": {State: gh.PRStateClosed}}}
	env.NewGH = func() GH { return reader }

	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if len(api.updates) != 0 || nudges != 0 {
		t.Errorf("updates = %+v, nudges = %d, want nothing written for a closed pull request", api.updates, nudges)
	}
}

// A pull request the reading did not find — GitHub could not resolve it —
// settles nothing: the slice reads unread, and the next run asks again.
func TestPRStatusLeavesAnUnresolvedPRAlone(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageForStatus("s1", "Unreadable", notion.SliceInProgress, "",
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, out := testEnv(testConfig(t), api)
	reader := &fakePRReader{}
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

// Two repositories are one reading.
func TestPRStatusReadsEveryRepositoryAtOnce(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {
				slicePageWithAllFields("s1", "In repo one", notion.SliceInProgress, "", "", "", "https://github.test/craig/nat/pull/1", "/repo/one"),
				slicePageWithAllFields("s2", "In repo two", notion.SliceInProgress, "", "", "", "https://github.test/craig/nat/pull/2", "/repo/two"),
			},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	reader := &fakePRReader{prs: map[string]gh.PR{
		"https://github.test/craig/nat/pull/1": openPR(true, true),
		"https://github.test/craig/nat/pull/2": openPR(true, true),
	}}
	env.NewGH = func() GH { return reader }

	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if reader.calls != 1 || len(reader.queries[0].PRs) != 2 {
		t.Errorf("gh calls = %d (%+v), want one reading of both", reader.calls, reader.queries)
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
	reader := &fakePRReader{prs: map[string]gh.PR{"https://github.test/craig/nat/pull/2": openPR(true, true)}}
	env.NewGH = func() GH { return reader }
	withDoneWorktree(&env, "Awaiting merge")

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
	if got := readinessOf(gh.PRStatus{Approved: true, Mergeable: true, Checks: gh.ChecksFailing}); got != domain.PRChecksFailing {
		t.Errorf("readinessOf(approved+mergeable, checks failing) = %v, want checks failing", got)
	}
	if got := readinessOf(gh.PRStatus{}); got != domain.PRAwaitingReview {
		t.Errorf("readinessOf({}) = %v, want awaiting review", got)
	}
}

func (f *fakePRReader) EditReviewers(dir, ref string, add, remove []string) error { return nil }
func (f *fakePRReader) Collaborators(dir string) ([]string, error)                { return nil, nil }

// redStatusEnv is a plan with one approved slice whose pull request reads red
// — one failing run — another reading green and one pending, with the tmux the
// test hands it.
func redStatusEnv(t *testing.T, runner *agentTestRunner) (Env, *fakeAPI, interface{ String() string }) {
	t.Helper()
	api := &fakeAPI{
		dataSources: map[string]notion.DataSource{"slices-ds": selectMilestoneSlicesDS("M1")},
		pages: map[string][]notion.Page{
			"slices-ds": {
				slicePageForStatus("s1", "Red", notion.SliceInProgress, "M1", "https://github.test/craig/nat/pull/1"),
				slicePageForStatus("s2", "Green", notion.SliceInProgress, "M1", "https://github.test/craig/nat/pull/2"),
				slicePageForStatus("s3", "Pending", notion.SliceInProgress, "M1", "https://github.test/craig/nat/pull/3"),
			},
		},
	}
	env, out := testEnv(testConfig(t), api)
	reader := &fakePRReader{prs: map[string]gh.PR{
		"https://github.test/craig/nat/pull/1": openPR(false, false,
			gh.Check{Name: "test", State: "FAILURE", URL: "https://github.test/craig/nat/actions/runs/9/job/1"}),
		"https://github.test/craig/nat/pull/2": openPR(true, true, gh.Check{Name: "test", State: "SUCCESS"}),
		"https://github.test/craig/nat/pull/3": openPR(false, false, gh.Check{Name: "test", State: "IN_PROGRESS"}),
	}}
	env.NewGH = func() GH { return reader }
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	return env, api, out
}

// A red pull request reads as checks failing, naming the check and its run URL;
// green and pending read as they always did, with their verdicts.
func TestPRStatusJSONReportsFailingChecks(t *testing.T) {
	env, _, out := redStatusEnv(t, &agentTestRunner{liveSessions: map[string]string{}})
	if err := Run(context.Background(), []string{"pr-status", "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status --json: %v", err)
	}
	var got prStatusDoc
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	want := []prStatusSliceJSON{
		{SliceID: "s1", Name: "Red", PR: "https://github.test/craig/nat/pull/1", Readiness: "checks failing",
			Base: "main", Checks: &prChecksJSON{Verdict: "failing", Failing: []prCheckJSON{
				{Name: "test", URL: "https://github.test/craig/nat/actions/runs/9/job/1"}}}},
		{SliceID: "s2", Name: "Green", PR: "https://github.test/craig/nat/pull/2", Readiness: "ready to merge",
			Base: "main", Checks: &prChecksJSON{Verdict: "passing", Failing: []prCheckJSON{}}},
		{SliceID: "s3", Name: "Pending", PR: "https://github.test/craig/nat/pull/3", Readiness: "awaiting review",
			Base: "main", Checks: &prChecksJSON{Verdict: "pending", Failing: []prCheckJSON{}}},
	}
	if !reflect.DeepEqual(got.Slices, want) {
		t.Errorf("json = %+v\nwant %+v", got.Slices, want)
	}
}

// With no agent live the red slice gets a Checks failed on its record and
// nothing is typed anywhere; the markdown names the failing check.
func TestPRStatusRecordsFailingChecksWithNoAgent(t *testing.T) {
	runner := &agentTestRunner{liveSessions: map[string]string{}}
	env, api, out := redStatusEnv(t, runner)
	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if len(api.appends) != 1 || api.appends[0].id != "s1" {
		t.Fatalf("appends = %+v, want one on s1", api.appends)
	}
	if b, _ := json.Marshal(api.appends[0].children); !strings.Contains(string(b), "Checks failed") {
		t.Errorf("appended %s, want a Checks failed section", b)
	}
	if len(runner.sends) != 0 {
		t.Errorf("sent %d prompts, want none with no agent live", len(runner.sends))
	}
	if !strings.Contains(out.String(), "failing: test https://github.test/craig/nat/actions/runs/9/job/1") {
		t.Errorf("output does not name the failing check:\n%s", out.String())
	}
}

// With an agent live it is told once, and a Sent back goes on the record.
func TestPRStatusNudgesALiveAgent(t *testing.T) {
	runner := &agentTestRunner{liveSessions: map[string]string{"s1": "nat-s1"}}
	env, api, _ := redStatusEnv(t, runner)
	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if len(runner.sends) != 1 || runner.sends[0].session != "nat-s1" ||
		!strings.Contains(runner.sends[0].prompt, "nat slice-checks s1 --log --project project-1") {
		t.Fatalf("sends = %+v, want one checks prompt to nat-s1", runner.sends)
	}
	if len(api.appends) != 1 {
		t.Fatalf("appends = %d, want one Sent back", len(api.appends))
	}
	if b, _ := json.Marshal(api.appends[0].children); !strings.Contains(string(b), "Sent back") {
		t.Errorf("appended %s, want one Sent back", b)
	}
}

// A tmux that cannot say what is live concludes nothing: no prompt, no record.
func TestPRStatusLeavesFailingChecksWhenTmuxIsUnread(t *testing.T) {
	runner := &agentTestRunner{liveFatalErr: "tmux broke"}
	env, api, _ := redStatusEnv(t, runner)
	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if len(api.appends) != 0 || len(runner.sends) != 0 {
		t.Errorf("appends %d, sends %d, want nothing", len(api.appends), len(runner.sends))
	}
}

// readingRunner answers gh api graphql with a fixed printout — GitHub's own
// answer, read by the real [gh.CLI] — and records each document it was asked.
type readingRunner struct {
	out  string
	docs []string
}

func (r *readingRunner) Run(dir, name string, args ...string) (string, error) {
	r.docs = append(r.docs, strings.TrimPrefix(args[len(args)-1], "query="))
	return r.out, nil
}

// realReading is a fakePRReader whose reading is the real gh.CLI's of a real
// GraphQL answer, so pr-status is tested against the shapes GitHub prints.
type realReading struct {
	*fakePRReader
	cli gh.CLI
}

func (r realReading) ReadPRs(q gh.BatchQuery) (gh.Batch, error) { return r.cli.ReadPRs(q) }

func (r realReading) PollPRs(q gh.BatchQuery) (gh.Batch, error) { return r.cli.PollPRs(q) }

func (r realReading) Outlook(poll time.Duration) gh.Outlook { return r.cli.Outlook(poll) }

// TestPRStatusJSONConflicting pins the per-slice conflicting fact, exactly as
// printed, from GitHub's own answer: CONFLICTING or a DIRTY merge state is a
// conflict, MERGEABLE and UNKNOWN are not, and a slice the reading did not ask
// about says false with no base. The reading's rate limit rides along.
func TestPRStatusJSONConflicting(t *testing.T) {
	const pr = "https://github.test/craig/nat/pull/"
	api := &fakeAPI{
		dataSources: map[string]notion.DataSource{"slices-ds": selectMilestoneSlicesDS("M1")},
		pages: map[string][]notion.Page{
			"slices-ds": {
				slicePageForStatus("s1", "Conflicting", notion.SliceInProgress, "M1", pr+"1"),
				slicePageForStatus("s2", "Dirty", notion.SliceInProgress, "M1", pr+"2"),
				slicePageForStatus("s3", "Clean", notion.SliceInProgress, "M1", pr+"3"),
				slicePageForStatus("s4", "Unknown", notion.SliceInProgress, "M1", pr+"4"),
				slicePageForStatus("s5", "Landed", notion.SliceDone, "M1", pr+"5"),
			},
		},
	}
	env, out := testEnv(testConfig(t), api)
	node := func(alias, n, decision, mergeable, state string) string {
		return `"` + alias + `":{"number":` + n + `,"url":"` + pr + n + `","state":"OPEN","reviewDecision":"` + decision +
			`","mergeable":"` + mergeable + `","mergeStateStatus":"` + state + `","baseRefName":"main",` +
			`"lastCommit":{"nodes":[{"commit":{"statusCheckRollup":null}}]}}`
	}
	runner := &readingRunner{out: `{"data":{"rateLimit":{"limit":5000,"remaining":4990,"resetAt":"2026-10-06T13:00:00Z"},"r0":{` +
		node("p0", "1", "", "CONFLICTING", "DIRTY") + `,` + node("p1", "2", "APPROVED", "UNKNOWN", "DIRTY") + `,` +
		node("p2", "3", "APPROVED", "MERGEABLE", "CLEAN") + `,` + node("p3", "4", "", "UNKNOWN", "UNKNOWN") + `}}}`}
	env.NewGH = func() GH { return realReading{fakePRReader: &fakePRReader{}, cli: gh.NewWithRunner(runner)} }

	if err := Run(context.Background(), []string{"pr-status", "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status --json: %v", err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, out.Bytes()); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	entry := func(id, name, n, readiness, conflicting string) string {
		return `{"slice_id":"` + id + `","name":"` + name + `","pr":"` + pr + n + `","readiness":"` + readiness +
			`","conflicting":` + conflicting + `,"base":"main","checks":{"verdict":"none","failing":[]}}`
	}
	want := `{"slices":[` +
		entry("s1", "Conflicting", "1", "awaiting review", "true") + `,` +
		entry("s2", "Dirty", "2", "awaiting review", "true") + `,` +
		entry("s3", "Clean", "3", "ready to merge", "false") + `,` +
		entry("s4", "Unknown", "4", "awaiting review", "false") + `,` +
		`{"slice_id":"s5","name":"Landed","pr":"` + pr + `5","readiness":"unread","conflicting":false}` +
		`],"branches":[],"sessions":[],"rate_limit":{"limit":5000,"remaining":4990,"reset_at":"2026-10-06T13:00:00Z",` +
		`"projected_remaining_at_reset":4990,"throttled":false,"poll_after_seconds":30,"cost":0}}`
	if compact.String() != want {
		t.Errorf("json =\n%s\nwant\n%s", compact.String(), want)
	}
}

// TestPRStatusMarkdownConflicting says a conflict under the slice's line,
// naming the base where the listing read one.
func TestPRStatusMarkdownConflicting(t *testing.T) {
	got := prStatusMarkdown([]prReading{
		{SliceName: "A", PR: "u1", Checks: &gh.PRStatus{Conflicting: true, Base: "main"}},
		{SliceName: "B", PR: "u2", Checks: &gh.PRStatus{Conflicting: true}},
		{SliceName: "C", PR: "u3", Checks: &gh.PRStatus{}},
	})
	for _, want := range []string{"- A — unread — u1\n  - conflicting with main\n", "- B — unread — u2\n  - conflicting\n- C"} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown missing %q:\n%s", want, got)
		}
	}
}
