package cli

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
)

// fakePRReader stands in for gh's own PR listing, answered by directory —
// one entry per repository the plan spans, exactly as [PRReader.OpenPRs] does.
type fakePRReader struct {
	open  map[string]map[string]gh.PRStatus
	err   map[string]error
	dirs  []string
	calls int
}

func (f *fakePRReader) OpenPRs(dir string) (map[string]gh.PRStatus, error) {
	f.dirs = append(f.dirs, dir)
	f.calls++
	if err := f.err[dir]; err != nil {
		return nil, err
	}
	return f.open[dir], nil
}

// The rest of [GH] pr-status never calls; stubbed so *fakePRReader can stand
// in for the whole seam.
func (f *fakePRReader) CreatePR(dir, branch, title, body string) (string, error) { return "", nil }
func (f *fakePRReader) ViewPR(dir, ref string) (gh.PR, error)                    { return gh.PR{}, nil }
func (f *fakePRReader) MergePR(dir, ref string) error                            { return nil }
func (f *fakePRReader) CommentPR(dir, ref, body string) (string, error)          { return "", nil }

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
	env, out := testEnv(testConfig(), api)
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
	env, out := testEnv(testConfig(), api)
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
	env, out := testEnv(testConfig(), api)
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
	env, out := testEnv(testConfig(), api)
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

func TestPRStatusGroupsRepositoriesBySliceRepo(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {
				slicePageWithAllFields("s1", "In repo one", notion.SliceInProgress, "", "", "", "https://github.test/craig/nat/pull/1", "/repo/one"),
				slicePageWithAllFields("s2", "In repo two", notion.SliceInProgress, "", "", "", "https://github.test/craig/nat/pull/2", "/repo/two"),
			},
		},
	}
	env, _ := testEnv(testConfig(), api)
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
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-status", "--bogus", "--project", "project-1"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestPRStatusRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-status", "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestPRStatusReportsAFailedQuery(t *testing.T) {
	api := &fakeAPI{queryErr: map[string]error{"slices-ds": errors.New("notion is down")}}
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "load slices") {
		t.Errorf("err = %v, want the failed read named", err)
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
