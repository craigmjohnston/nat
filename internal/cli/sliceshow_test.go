package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

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
	env, out := testEnv(testConfig(), api)

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
	env, out := testEnv(testConfig(), api)

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
	env, out := testEnv(testConfig(), api)

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
	env, out := testEnv(testConfig(), api)

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
	env, out := testEnv(testConfig(), api)

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
	env, out := testEnv(testConfig(), api)

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
	env, out := testEnv(testConfig(), api)

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
	env, _ := testEnv(testConfig(), api)

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
	env, _ := testEnv(testConfig(), api)

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
	env, _ := testEnv(testConfig(), api)

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
	env, _ := testEnv(testConfig(), api)

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
	env, _ := testEnv(testConfig(), api)

	const sliceID = "3b738308f65481708c99eccab4463d8f"
	err := Run(context.Background(), []string{"slice-show", sliceID, "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("slice-show with schema error: want error, got nil")
	}
	if !strings.Contains(err.Error(), "load the slices schema") {
		t.Errorf("err = %v, want it to mention 'load the slices schema'", err)
	}
}

func TestSliceShowPageLoadError(t *testing.T) {
	api := &fakeAPI{
		getErr: errors.New("page load failed"),
		dataSources: map[string]notion.DataSource{
			"slices-ds": selectMilestoneSlicesDS("M1: First"),
		},
	}
	env, _ := testEnv(testConfig(), api)

	const sliceID = "3b738308f65481708c99eccab4463d8f"
	err := Run(context.Background(), []string{"slice-show", sliceID, "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("slice-show with page load error: want error, got nil")
	}
	if !strings.Contains(err.Error(), "load the slice") {
		t.Errorf("err = %v, want it to mention 'load the slice'", err)
	}
}

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
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-show", sliceID, "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("slice-show with brief read error: want error, got nil")
	}
	if !strings.Contains(err.Error(), "could not read the slice's brief") {
		t.Errorf("err = %v, want it to mention 'could not read the slice's brief'", err)
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
	env, _ := testEnv(testConfig(), api)
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
	env, _ := testEnv(testConfig(), api)
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
	env, out := testEnv(testConfig(), api)

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
	env, out := testEnv(testConfig(), api)

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
