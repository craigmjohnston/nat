package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/notion"
)

// fakeGitRunner is a fake git runner for testing: diff answers diffOut/diffErr,
// symbolic-ref answers base when it is given and refuses otherwise — which is
// what sends [git.CLI.Base] on to its own local fallback, "main".
type fakeGitRunner struct {
	diffOut string
	diffErr error
	base    string
	dir     string
}

func (f *fakeGitRunner) Run(dir, _ string, args ...string) (string, error) {
	f.dir = dir
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "diff":
		return f.diffOut, f.diffErr
	case "symbolic-ref":
		if f.base == "" {
			return "", errors.New("no such ref")
		}
		return f.base, nil
	default:
		return "", errors.New("no such ref")
	}
}

func TestSliceDiffRefusesNotHandedBack(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceTodo, "m1", "", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-diff", testSliceID, "--project", "project-1",
	}, env)
	if err == nil {
		t.Fatal("slice-diff: expected error for non-handed-back slice")
	}
	if !strings.Contains(err.Error(), "not handed back") {
		t.Errorf("slice-diff error: %v, want 'not handed back'", err)
	}
}

func TestSliceDiffRefusesDone(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceDone, "m1", "main")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-diff", testSliceID, "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "already Done") {
		t.Errorf("slice-diff error = %v, want 'already Done'", err)
	}
}

const sampleDiff = "diff --git a/main.go b/main.go\nindex 1234..5678 100644\n--- a/main.go\n+++ b/main.go\n@@ -1,1 +1,1 @@\n-old\n+new\n"

func TestSliceDiffReadsDiff(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "main")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	runner := &fakeGitRunner{diffOut: sampleDiff}
	env.NewGit = func() GitCLI { return git.NewWithRunner(runner) }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-diff", testSliceID, "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-diff: unexpected error: %v", err)
	}
	if out.String() != sampleDiff {
		t.Errorf("slice-diff output = %q, want git's own diff verbatim", out.String())
	}
	if runner.dir != "/tmp/nat" {
		t.Errorf("diff ran in %q, want the project's working directory", runner.dir)
	}
}

func TestSliceDiffFailure(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "main")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	env.NewGit = func() GitCLI {
		return git.NewWithRunner(&fakeGitRunner{
			diffErr: &git.ExitError{Code: 1, Stderr: "branch not found"},
		})
	}
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-diff", testSliceID, "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "branch not found") {
		t.Errorf("slice-diff error = %v, want git's own reason", err)
	}
}

// The base a branch is diffed against and the branch itself are two
// different things — the bug this pins is the JSON encoder having answered
// the base for both.
func TestSliceDiffJSON(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "feature/ui")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	env.NewGit = func() GitCLI {
		return git.NewWithRunner(&fakeGitRunner{diffOut: sampleDiff, base: "origin/main"})
	}
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-diff", testSliceID, "--json", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-diff: unexpected error: %v", err)
	}

	var got diffJSON
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if got.Base != "origin/main" {
		t.Errorf("base = %q, want the branch's own base", got.Base)
	}
	if got.Branch != "feature/ui" {
		t.Errorf("branch = %q, want the slice's own handed-back branch, not the base", got.Branch)
	}
	if len(got.Files) != 1 || got.Files[0].Path != "main.go" || got.Files[0].Adds != 1 || got.Files[0].Dels != 1 {
		t.Errorf("files = %+v, want main.go with one line added and removed", got.Files)
	}
	if got.Files[0].Language != "Go" {
		t.Errorf("language = %q, want the lexer chroma matched main.go to", got.Files[0].Language)
	}
	// One tokens entry per line, in the same order as Lines: no runs at all
	// for the header/meta/hunk lines, and the one run "old"/"new" lex to on
	// their own line.
	wantTokens := [][]tokenRun{
		{}, {}, {}, {}, {},
		{{Kind: kindText, Length: 3}},
		{{Kind: kindText, Length: 3}},
	}
	if len(got.Files[0].Tokens) != len(wantTokens) {
		t.Fatalf("tokens = %+v, want %d entries (one per line)", got.Files[0].Tokens, len(wantTokens))
	}
	for i := range wantTokens {
		assertRuns(t, got.Files[0].Tokens[i], wantTokens[i])
	}
}

// TestSliceDiffJSONOmitsTokensWithoutALanguage covers the file-level fallback
// on the wire: a path chroma knows no language for gets neither a "language"
// nor a "tokens" key at all, which is what tells the reader to fall all the
// way back to its own unhighlighted colouring — the same rule diffsyntax.go
// draws by.
func TestSliceDiffJSONOmitsTokensWithoutALanguage(t *testing.T) {
	const unmatchedDiff = "diff --git a/notes.xyzzy b/notes.xyzzy\n" +
		"index 1111111..2222222 100644\n--- a/notes.xyzzy\n+++ b/notes.xyzzy\n" +
		"@@ -1,1 +1,1 @@\n-old\n+new\n"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "feature/notes")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	env.NewGit = func() GitCLI { return git.NewWithRunner(&fakeGitRunner{diffOut: unmatchedDiff}) }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-diff", testSliceID, "--json", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-diff: unexpected error: %v", err)
	}
	if strings.Contains(out.String(), `"language"`) {
		t.Errorf("output names a language for an unmatched path:\n%s", out.String())
	}
	if strings.Contains(out.String(), `"tokens"`) {
		t.Errorf("output carries tokens for a file with no matched language:\n%s", out.String())
	}
}

// TestSliceDiffJSONOmitsTokensForADescribedFile covers the other file-level
// fallback: a file git described rather than diffed has no content to lex at
// all, whatever chroma would have matched its path to.
func TestSliceDiffJSONOmitsTokensForADescribedFile(t *testing.T) {
	const binaryDiff = "diff --git a/docs/shot.png b/docs/shot.png\n" +
		"index 3333333..4444444 100644\nBinary files a/docs/shot.png and b/docs/shot.png differ\n"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "feature/shot")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	env.NewGit = func() GitCLI { return git.NewWithRunner(&fakeGitRunner{diffOut: binaryDiff}) }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-diff", testSliceID, "--json", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-diff: unexpected error: %v", err)
	}
	var got diffJSON
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if len(got.Files) != 1 || !got.Files[0].Described {
		t.Fatalf("files = %+v, want one described file", got.Files)
	}
	if got.Files[0].Language != "" {
		t.Errorf("language = %q, want empty for a described file", got.Files[0].Language)
	}
	if got.Files[0].Tokens != nil {
		t.Errorf("tokens = %+v, want omitted for a described file", got.Files[0].Tokens)
	}
}

func TestSliceDiffRefusesWrongArgumentCount(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-diff", "--project", "project-1",
	}, env)
	if err == nil {
		t.Fatal("slice-diff: expected error for missing slice")
	}
	if !strings.Contains(err.Error(), "want exactly one") {
		t.Errorf("slice-diff error: %v, want 'want exactly one'", err)
	}
}

func TestSliceDiffRefusesAnInvalidSliceID(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-diff", "not-a-uuid", "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("slice-diff error = %v, want 'not a slice'", err)
	}
}

func TestSliceDiffRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-diff", testSliceID, "--bogus", "--project", "project-1"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestSliceDiffRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-diff", testSliceID, "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestSliceDiffReportsAFailedRead(t *testing.T) {
	api := &fakeAPI{getErr: errors.New("notion is down")}
	env, _ := testEnv(testClaimConfig(), api)

	err := Run(context.Background(), []string{"slice-diff", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "load the slice") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}
