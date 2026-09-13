package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/notion"
)

func TestSliceStatusPrintsTheStatus(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceInProgress, "M1: First", "")},
		},
	}
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"slice-status", sliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-status: %v", err)
	}

	if got := out.String(); got != "In progress\n" {
		t.Errorf("output = %q, want the status alone", got)
	}
}

func TestSliceStatusPrintsJSON(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceDone, "M1: First", "")},
		},
	}
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"slice-status", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-status --json: %v", err)
	}

	var got sliceStatusJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if got.Status != notion.SliceDone {
		t.Errorf("status = %q, want %q", got.Status, notion.SliceDone)
	}
	if got.Trashed {
		t.Error("trashed = true, want a page nothing trashed to read false")
	}
}

// A slice need not be one the named project's own plan holds: --project only
// pins the credentials the page is read with.
func TestSliceStatusOfASliceInAnotherProject(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Elsewhere", notion.SliceTodo, "M9: Other", "")},
		},
	}
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"slice-status", sliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-status: %v", err)
	}

	if got := out.String(); got != "Todo\n" {
		t.Errorf("output = %q, want the status of the page named, project or no", got)
	}
}

func TestSliceStatusReportsATrashedPage(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	page := slicePageWithBranch(sliceID, "Trashed slice", notion.SliceInProgress, "M1: First", "")
	page.InTrash = true
	api := &fakeAPI{pages: map[string][]notion.Page{sliceID: {page}}}
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"slice-status", sliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-status: %v", err)
	}

	if got := out.String(); got != "In progress (trashed)\n" {
		t.Errorf("output = %q, want the trashed page said so", got)
	}
}

func TestSliceStatusReportsATrashedPageAsJSON(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	page := slicePageWithBranch(sliceID, "Trashed slice", notion.SliceInProgress, "M1: First", "")
	page.Archived = true
	api := &fakeAPI{pages: map[string][]notion.Page{sliceID: {page}}}
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"slice-status", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-status --json: %v", err)
	}

	var got sliceStatusJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if !got.Trashed {
		t.Error("trashed = false, want the archived page read true")
	}
}

func TestSliceStatusOfAnEmptyStatus(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{pages: map[string][]notion.Page{sliceID: {{ID: sliceID}}}}
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"slice-status", sliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-status: %v", err)
	}

	if got := out.String(); got != "(no status)\n" {
		t.Errorf("output = %q, want the blank status named", got)
	}
}

func TestSliceStatusOfAGonePage(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{getErr: &notion.APIError{StatusCode: http.StatusNotFound}}
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"slice-status", sliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-status: %v", err)
	}

	if got := out.String(); got != "gone\n" {
		t.Errorf("output = %q, want the page reported gone", got)
	}
}

func TestSliceStatusOfAGonePageAsJSON(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{getErr: &notion.APIError{StatusCode: http.StatusNotFound}}
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"slice-status", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-status --json: %v", err)
	}

	var got sliceStatusGoneJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if !got.Gone {
		t.Error("gone = false, want a not-found page reported gone")
	}
}

// A read that fails for any other reason is a refusal, not gone: nothing says
// the page does not exist, only that this attempt at reading it did not work.
func TestSliceStatusReportsAnOtherwiseFailedRead(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{getErr: errors.New("notion is down")}
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-status", sliceID, "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "read the slice") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}

func TestSliceStatusInvalidSliceRef(t *testing.T) {
	api := &fakeAPI{}
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-status", "not-a-url-or-id", "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("err = %v, want it to mention 'not a slice'", err)
	}
}

func TestSliceStatusMissingProject(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{}
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-status", sliceID}, env)
	if err == nil || !strings.Contains(err.Error(), "no project given") {
		t.Errorf("err = %v, want it to mention 'no project given'", err)
	}
}

func TestSliceStatusNoArgument(t *testing.T) {
	api := &fakeAPI{}
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-status", "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "want exactly one slice") {
		t.Errorf("err = %v, want it to mention 'want exactly one slice'", err)
	}
}

func TestSliceStatusTooManyArguments(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{}
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-status", sliceID, "extra", "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "want exactly one slice") {
		t.Errorf("err = %v, want it to mention 'want exactly one slice'", err)
	}
}

func TestSliceStatusBadFlag(t *testing.T) {
	env := Env{
		NewClient: DefaultNewClient,
		NewTmux:   DefaultNewTmux,
		Out:       &strings.Builder{},
	}

	err := Run(context.Background(), []string{"slice-status", "--badFlag"}, env)
	if err == nil {
		t.Fatal("slice-status with bad flag: want error, got nil")
	}
}

func TestSliceStatusJSONWriteError(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceTodo, "M1: First", "")},
		},
	}
	env, _ := testEnv(testConfig(), api)
	env.Out = failingWriter{}

	err := Run(context.Background(), []string{"slice-status", sliceID, "--json", "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("slice-status JSON with write error: want error, got nil")
	}
}

func TestSliceStatusMarkdownWriteError(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			sliceID: {slicePageWithBranch(sliceID, "Test slice", notion.SliceTodo, "M1: First", "")},
		},
	}
	env, _ := testEnv(testConfig(), api)
	env.Out = failingWriter{}

	err := Run(context.Background(), []string{"slice-status", sliceID, "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("slice-status markdown with write error: want error, got nil")
	}
}

func TestSliceStatusGoneMarkdownWriteError(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{getErr: &notion.APIError{StatusCode: http.StatusNotFound}}
	env, _ := testEnv(testConfig(), api)
	env.Out = failingWriter{}

	err := Run(context.Background(), []string{"slice-status", sliceID, "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("slice-status gone with write error: want error, got nil")
	}
}

func TestSliceStatusGoneJSONWriteError(t *testing.T) {
	const sliceID = "3b738308f65481708c99eccab4463d8f"
	api := &fakeAPI{getErr: &notion.APIError{StatusCode: http.StatusNotFound}}
	env, _ := testEnv(testConfig(), api)
	env.Out = failingWriter{}

	err := Run(context.Background(), []string{"slice-status", sliceID, "--json", "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("slice-status gone --json with write error: want error, got nil")
	}
}
