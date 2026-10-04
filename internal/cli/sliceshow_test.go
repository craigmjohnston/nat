package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/notion"
)

func TestSliceShowPrintsSliceAsMarkdown(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceTodo, "M1: First", "branch-1")},
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, out := testEnv(testConfig(t), api)

	if err := Run(context.Background(), []string{"slice-show", sliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-show: %v", err)
	}

	result := out.String()
	if !strings.Contains(result, "Test slice") {
		t.Errorf("output missing slice name: %s", result)
	}
	if !strings.Contains(result, "Todo") {
		t.Errorf("output missing status: %s", result)
	}
	if !strings.Contains(result, "M1: First") {
		t.Errorf("output missing milestone: %s", result)
	}
	if !strings.Contains(result, "branch-1") {
		t.Errorf("output missing branch: %s", result)
	}
}

func TestSliceShowPrintsJSON(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceInProgress, "M1: First", "branch-1")},
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, out := testEnv(testConfig(t), api)

	if err := Run(context.Background(), []string{"slice-show", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-show --json: %v", err)
	}

	var got sliceShowJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}

	if got.ID != sliceID {
		t.Errorf("id = %s, want %s", got.ID, sliceID)
	}
	if got.Name != "Test slice" {
		t.Errorf("name = %s, want 'Test slice'", got.Name)
	}
	if got.Status != notion.SliceInProgress {
		t.Errorf("status = %s, want %s", got.Status, notion.SliceInProgress)
	}
	if got.Milestone != "M1: First" {
		t.Errorf("milestone = %s, want 'M1: First'", got.Milestone)
	}
	if got.Branch != "branch-1" {
		t.Errorf("branch = %s, want 'branch-1'", got.Branch)
	}
	if !got.HandedBack {
		t.Errorf("handed_back = %v, want true", got.HandedBack)
	}
	if got.Base != git.DefaultBase {
		t.Errorf("base = %q, want the fallback %q with origin naming none", got.Base, git.DefaultBase)
	}
}

// The base is what the slice's repo names as its default branch, read there;
// with no repo anywhere there is nowhere to read it, and it is left out.
func TestSliceShowBase(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceTodo, "M1: First", "")},
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, out := testEnv(testConfig(t), api)
	runner := &fakeGitRunner{base: "origin/trunk"}
	env.NewGit = func() GitCLI { return git.NewWithRunner(runner) }
	if err := Run(context.Background(), []string{"slice-show", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-show --json: %v", err)
	}
	var got sliceShowJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if got.Base != "origin/trunk" || runner.dir != "/tmp/nat" {
		t.Errorf("base = %q read in %q, want origin/trunk from the working dir", got.Base, runner.dir)
	}

	cfg := testConfig(t)
	project := cfg.Projects["project-1"]
	project.WorkingDir = ""
	cfg.Projects["project-1"] = project
	env, out = testEnv(cfg, api)
	env.NewGit = func() GitCLI { t.Fatal("git asked with no repo to ask in"); return nil }
	if err := Run(context.Background(), []string{"slice-show", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-show --json: %v", err)
	}
	if strings.Contains(out.String(), `"base"`) {
		t.Errorf("output = %s, want no base with no repo", out.String())
	}
}

func TestSliceShowComputesBlocked(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	const depID = "3b738308f65481708c99eccab4463d8e"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceTodo, "M1: First", "", depID)},
			depID:   {slicePageWithBranch(depID, "Dependency", notion.SliceTodo, "M1: First", "")},
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, out := testEnv(testConfig(t), api)

	if err := Run(context.Background(), []string{"slice-show", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-show --json: %v", err)
	}

	var got sliceShowJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}

	if !got.Blocked {
		t.Errorf("blocked = %v, want true (dependency is Todo)", got.Blocked)
	}
}

func TestSliceShowComputesNotBlocked(t *testing.T) {
	// Dependency is Done, so this slice is not blocked
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	const depID = "3b738308f65481708c99eccab4463d8e"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceTodo, "M1: First", "", depID)},
			depID:   {slicePageWithBranch(depID, "Dependency", notion.SliceDone, "M1: First", "")},
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, out := testEnv(testConfig(t), api)

	if err := Run(context.Background(), []string{"slice-show", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-show --json: %v", err)
	}

	var got sliceShowJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}

	if got.Blocked {
		t.Errorf("blocked = %v, want false (dependency is Done)", got.Blocked)
	}
}

func TestSliceShowIncludeBrief(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	briefText := "This is the slice brief."
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceTodo, "M1: First", "")},
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
		blocksByID: map[string][]notion.Block{
			sliceID: briefBlocks(t, briefText),
		},
	}
	env, out := testEnv(testConfig(t), api)

	if err := Run(context.Background(), []string{"slice-show", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-show --json: %v", err)
	}

	var got sliceShowJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}

	if got.Brief != briefText {
		t.Errorf("brief = %q, want %q", got.Brief, briefText)
	}
}

// taskLogBlocks is a slice page body of a heading_3 and one paragraph under
// it, the shape a task-log section takes.
func taskLogBlocks(t *testing.T, heading, text string) []notion.Block {
	t.Helper()
	raw := `[` +
		`{"id":"h1","type":"heading_3","heading_3":{"rich_text":[{"plain_text":` + mustJSON(t, heading) + `}]}},` +
		`{"id":"p1","type":"paragraph","paragraph":{"rich_text":[{"plain_text":` + mustJSON(t, text) + `}]}}` +
		`]`
	var blocks []notion.Block
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		t.Fatal(err)
	}
	return blocks
}

// The task log is read off the slice's body, in body order.
func TestSliceShowEventsFromTheBody(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceInProgress, "M1: First", "")},
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
		blocksByID: map[string][]notion.Block{
			sliceID: taskLogBlocks(t, "Sent back", "Rename the helper."),
		},
	}
	env, out := testEnv(testConfig(t), api)

	if err := Run(context.Background(), []string{"slice-show", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-show --json: %v", err)
	}

	var got sliceShowJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if len(got.Events) != 1 || got.Events[0].Kind != "sent_back" || got.Events[0].Note != "Rename the helper." {
		t.Errorf("events = %+v, want one sent_back event", got.Events)
	}
}

// A decided follow-up carries when its triage record was stamped, as
// decidedAt in RFC 3339; one still pending carries none, and so does one
// decided by a record written before stamps were.
func TestSliceShowEventsFollowUpDecidedAt(t *testing.T) {
	body := "### Follow-ups\n\n1. A\n   Brief A.\n2. B\n   Brief B.\n\n" +
		"### Follow-ups triaged\n\nAt 2026-10-05T09:30:00+01:00\n\n- Dropped: A\n\n" +
		"### Follow-ups\n\n1. C\n   Brief C.\n\n" +
		"### Follow-ups triaged\n\n- Folded in: C"
	got := taskEventsJSON(domain.Slice{}, body)
	if len(got) != 2 {
		t.Fatalf("events = %+v, want two follow_ups events", got)
	}
	tests := []struct {
		follow taskFollowUpJSON
		want   string
	}{
		{got[0].FollowUps[0], "2026-10-05T09:30:00+01:00"},
		{got[0].FollowUps[1], ""},
		{got[1].FollowUps[0], ""},
	}
	for _, tt := range tests {
		if tt.follow.DecidedAt != tt.want {
			t.Errorf("%s: decidedAt = %q, want %q", tt.follow.Title, tt.follow.DecidedAt, tt.want)
		}
	}
	raw, err := json.Marshal(got[0].FollowUps[1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "decidedAt") {
		t.Errorf("pending follow-up = %s, want no decidedAt", raw)
	}
}

// A slice with nothing in its task log yet still answers an empty array, not
// a null, so a consumer can range over it with no nil check.
func TestSliceShowEventsIsAnEmptyArrayNotNull(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceTodo, "M1: First", "")},
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, out := testEnv(testConfig(t), api)

	if err := Run(context.Background(), []string{"slice-show", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-show --json: %v", err)
	}
	if !strings.Contains(out.String(), `"events": []`) {
		t.Errorf("output =\n%s\nwant an empty events array", out.String())
	}
	var got sliceShowJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if got.Events == nil {
		t.Error("Events = nil, want an empty slice")
	}
}

// A recorded pull request is an "approved" event, named by its URL, appended
// after whatever the body itself carries.
func TestSliceShowEventsIncludesApproved(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	page := slicePageWithBranch(sliceID, "Test slice", notion.SliceInProgress, "M1: First", "")
	page.Properties[notion.PropPR] = notion.NewURL("https://github.com/o/r/pull/12")
	api := &fakeAPI{
		pages: map[string][]notion.Page{sliceID: {page}},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, out := testEnv(testConfig(t), api)

	if err := Run(context.Background(), []string{"slice-show", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-show --json: %v", err)
	}
	var got sliceShowJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if len(got.Events) != 1 || got.Events[0].Kind != "approved" || got.Events[0].PR != "https://github.com/o/r/pull/12" {
		t.Errorf("events = %+v, want one approved event naming the PR", got.Events)
	}
}

// A Done slice with a pull request recorded carries both an "approved" and a
// "merged" event, in that order.
func TestSliceShowEventsIncludesApprovedAndMerged(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	page := slicePageWithBranch(sliceID, "Test slice", notion.SliceDone, "M1: First", "")
	page.Properties[notion.PropPR] = notion.NewURL("https://github.com/o/r/pull/12")
	api := &fakeAPI{
		pages: map[string][]notion.Page{sliceID: {page}},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, out := testEnv(testConfig(t), api)

	if err := Run(context.Background(), []string{"slice-show", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-show --json: %v", err)
	}
	var got sliceShowJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if len(got.Events) != 2 || got.Events[0].Kind != "approved" || got.Events[1].Kind != "merged" {
		t.Errorf("events = %+v, want approved then merged", got.Events)
	}
}

// A Done slice with only a branch recorded (merged with no pull request
// ever opened) is a "merged" event alone, with no "approved" before it.
func TestSliceShowEventsMergedWithNoPR(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceDone, "M1: First", "branch-1")},
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, out := testEnv(testConfig(t), api)

	if err := Run(context.Background(), []string{"slice-show", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-show --json: %v", err)
	}
	var got sliceShowJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if len(got.Events) != 1 || got.Events[0].Kind != "merged" {
		t.Errorf("events = %+v, want merged alone", got.Events)
	}
}

func TestSliceShowNoDependencies(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceTodo, "M1: First", "")},
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, out := testEnv(testConfig(t), api)

	if err := Run(context.Background(), []string{"slice-show", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-show --json: %v", err)
	}

	var got sliceShowJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}

	if len(got.DependsOn) != 0 {
		t.Errorf("depends_on = %v, want empty", got.DependsOn)
	}
	if got.Blocked {
		t.Errorf("blocked = %v, want false (no dependencies)", got.Blocked)
	}
}

func TestSliceShowByURL(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"3b738308f65481708c99eccab4463d8f": {slicePageWithBranch("3b738308f65481708c99eccab4463d8f", "Test slice", notion.SliceTodo, "M1", "")},
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, out := testEnv(testConfig(t), api)

	// Use a URL that will be parsed to extract the ID
	url := "https://www.notion.so/3b738308f65481708c99eccab4463d8f"
	if err := Run(context.Background(), []string{"slice-show", url, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-show with URL: %v", err)
	}

	if !strings.Contains(out.String(), "Test slice") {
		t.Errorf("output missing slice name from URL resolution: %s", out.String())
	}
}

func TestSliceShowInvalidSliceRef(t *testing.T) {
	api := &fakeAPI{}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{"slice-show", "not-a-url-or-id", "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("slice-show with invalid ref: want error, got nil")
	}
	if !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("err = %v, want it to mention 'not a slice'", err)
	}
}

func TestSliceShowMissingProject(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{"slice-show", sliceID}, env)
	if err == nil {
		t.Fatal("slice-show without --project: want error, got nil")
	}
	if !strings.Contains(err.Error(), "no project given") {
		t.Errorf("err = %v, want it to mention 'no project given'", err)
	}
}

func TestSliceShowNoArgument(t *testing.T) {
	api := &fakeAPI{}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{"slice-show", "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("slice-show without slice arg: want error, got nil")
	}
	if !strings.Contains(err.Error(), "want exactly one slice") {
		t.Errorf("err = %v, want it to mention 'want exactly one slice'", err)
	}
}

func TestSliceShowTooManyArguments(t *testing.T) {
	api := &fakeAPI{}
	env, _ := testEnv(testConfig(t), api)

	const sliceID = "3b738308f65481708c99eccab4463d8f"
	err := Run(context.Background(), []string{"slice-show", sliceID, "extra", "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("slice-show with extra args: want error, got nil")
	}
	if !strings.Contains(err.Error(), "want exactly one slice") {
		t.Errorf("err = %v, want it to mention 'want exactly one slice'", err)
	}
}

func TestSliceShowSchemaReadError(t *testing.T) {
	api := &fakeAPI{
		dataSourceErr: errors.New("schema read failed"),
	}
	env, _ := testEnv(testConfig(t), api)

	const sliceID = "3b738308f65481708c99eccab4463d8f"
	err := Run(context.Background(), []string{"slice-show", sliceID, "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("slice-show with schema error: want error, got nil")
	}
	if !strings.Contains(err.Error(), "load the slices schema") {
		t.Errorf("err = %v, want it to mention 'load the slices schema'", err)
	}
}

// The project's shape is read from the local file once the plan has been
// pulled — no request of its own — and a file that cannot even answer that
// fails the command before the slice is ever loaded.
func TestSliceShowReportsAFailedLocalShapeRead(t *testing.T) {
	cfg := testConfig(t)
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	seedHydratedSlice(t, "project-1", sliceID, "Render the board", "Todo", func(db *sql.DB) {
		if _, err := db.Exec(`ALTER TABLE project DROP COLUMN has_assignee`); err != nil {
			t.Fatalf("break the plan's has_assignee column: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})

	err := Run(context.Background(), []string{"slice-show", sliceID, "--project", "project-1"}, env)

	if err == nil {
		t.Error("slice-show over a plan that cannot read its own shape: want an error")
	}
}

// The brief is read from the local file once the slice itself has been, and
// a file that cannot answer that read fails the command with its own words
// rather than loadSlice's.
func TestSliceShowReportsAFailedBriefRead(t *testing.T) {
	cfg := testConfig(t)
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	seedHydratedSlice(t, "project-1", sliceID, "Render the board", "Todo", func(db *sql.DB) {
		if _, err := db.Exec(`ALTER TABLE slices DROP COLUMN body_at`); err != nil {
			t.Fatalf("break the plan's body_at column: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})

	err := Run(context.Background(), []string{"slice-show", sliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "could not read the slice's brief") {
		t.Errorf("err = %v, want the failed brief read named", err)
	}
}

func TestSliceShowPageLoadError(t *testing.T) {
	api := &fakeAPI{
		getErr: errors.New("page load failed"),
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, _ := testEnv(testConfig(t), api)

	const sliceID = "3b738308f65481708c99eccab4463d8f"
	err := Run(context.Background(), []string{"slice-show", sliceID, "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("slice-show with page load error: want error, got nil")
	}
	if !strings.Contains(err.Error(), "load the slice") {
		t.Errorf("err = %v, want it to mention 'load the slice'", err)
	}
}

// A slice the local plan has never seen — this fixture's pages are keyed by
// the slice's own ID rather than by "slices-ds", so the store's initial
// hydrate pulls in none of them — is taken in by store.Mirrored.Slice on this
// read, and taking one in needs its body to seed the file with: there is no
// stale local copy for a slice new to the file the way store.Mirrored.Body's
// own lazy refresh has, so a body read that fails here fails the load itself,
// reported as loadSlice wraps it ("load the slice: ..."), rather than reaching
// sliceShow's own later "could not read the slice's brief" wrapping at all.
func TestSliceShowBriefReadError(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceTodo, "M1: First", "")},
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
		blocksErr: errors.New("brief read failed"),
	}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{"slice-show", sliceID, "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("slice-show with brief read error: want error, got nil")
	}
	if !strings.Contains(err.Error(), "load the slice") || !strings.Contains(err.Error(), "brief read failed") {
		t.Errorf("err = %v, want it to say the slice load failed and name the underlying error", err)
	}
}

func TestSliceShowJSONWriteError(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceTodo, "M1: First", "")},
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, _ := testEnv(testConfig(t), api)
	env.Out = failingWriter{}

	err := Run(context.Background(), []string{"slice-show", sliceID, "--json", "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("slice-show JSON with write error: want error, got nil")
	}
}

func TestSliceShowMarkdownWriteError(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceTodo, "M1: First", "")},
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, _ := testEnv(testConfig(t), api)
	env.Out = failingWriter{}

	err := Run(context.Background(), []string{"slice-show", sliceID, "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("slice-show markdown with write error: want error, got nil")
	}
}

// TestSliceShowBadFlag tests error handling for unknown flags.
func TestSliceShowBadFlag(t *testing.T) {
	env := Env{
		NewClient: DefaultNewClient,
		NewTmux:   DefaultNewTmux,
		Out:       &strings.Builder{},
	}

	err := Run(context.Background(), []string{"slice-show", "--badFlag"}, env)
	if err == nil {
		t.Fatal("slice-show with bad flag: want error, got nil")
	}
}

// TestSliceShowAllOptionalFields tests that all optional fields are included when present.
func TestSliceShowAllOptionalFields(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"

	// Parse blocks from JSON like conventionBlocks does
	var blocks []notion.Block
	blockJSON := `[{"id":"b1","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"This is the slice brief"}]}}]`
	if err := json.Unmarshal([]byte(blockJSON), &blocks); err != nil {
		t.Fatal(err)
	}

	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithAllFields(sliceID, "Complete slice", notion.SliceInProgress, "M1: First", "main-branch", "user@example.com", "https://github.com/repo/pull/123", "path/to/repo")},
		},
		blocksByID: map[string][]notion.Block{
			sliceID: blocks,
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, out := testEnv(testConfig(t), api)

	if err := Run(context.Background(), []string{"slice-show", sliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-show: %v", err)
	}

	result := out.String()
	// Check markdown output includes all optional fields
	if !strings.Contains(result, "Complete slice") {
		t.Errorf("output missing slice name: %s", result)
	}
	if !strings.Contains(result, "In progress") {
		t.Errorf("output missing status: %s", result)
	}
	if !strings.Contains(result, "M1: First") {
		t.Errorf("output missing milestone: %s", result)
	}
	if !strings.Contains(result, "user@example.com") {
		t.Errorf("output missing assignee: %s", result)
	}
	if !strings.Contains(result, "branch: main-branch") {
		t.Errorf("output missing branch: %s", result)
	}
	if !strings.Contains(result, "This is the slice brief") {
		t.Errorf("output missing brief: %s", result)
	}
}

// TestSliceShowJSONWithRepo tests that repo override is included in JSON output.
func TestSliceShowJSONWithRepo(t *testing.T) {
	const sliceID = "4c849409f65481708c99eccab4463d8f"

	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithAllFields(sliceID, "Slice with repo", notion.SliceInProgress, "M1: First", "branch-1", "", "", "custom/repo/path")},
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, out := testEnv(testConfig(t), api)

	if err := Run(context.Background(), []string{"slice-show", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-show --json: %v", err)
	}

	var got sliceShowJSON
	result := out.String()
	if err := json.Unmarshal([]byte(result), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, result)
	}

	if got.Repo != "custom/repo/path" {
		t.Errorf("repo = %q, want %q", got.Repo, "custom/repo/path")
	}
}

// slicePageWithAllFields creates a slice page with all optional properties set.
func slicePageWithAllFields(id, name, status, milestone, branch, assignee, pr, repo string) notion.Page {
	props := map[string]notion.PropertyValue{
		notion.PropName:   title(name),
		notion.PropStatus: notion.NewSelect(status),
	}
	if milestone != "" {
		props[notion.PropMilestone] = notion.NewSelect(milestone)
	}
	if branch != "" {
		props[notion.PropBranch] = notion.PropertyValue{RichText: []notion.RichText{{PlainText: branch, Text: &notion.TextContent{Content: branch}}}}
	}
	if assignee != "" {
		props[notion.PropAssignee] = notion.PropertyValue{People: &[]notion.User{{ID: "u1", Name: assignee}}}
	}
	if pr != "" {
		props[notion.PropPR] = notion.PropertyValue{URL: pr}
	}
	if repo != "" {
		props[notion.PropRepo] = notion.PropertyValue{RichText: []notion.RichText{{PlainText: repo, Text: &notion.TextContent{Content: repo}}}}
	}
	return notion.Page{ID: id, URL: "https://notion.so/" + id, Properties: props}
}
