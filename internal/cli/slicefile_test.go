package cli

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/notion"
)

const sampleFile = "package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n"

// sliceFileEnv is a handed-back slice on feature/ui whose git answers show
// with runner's own showOut.
func sliceFileEnv(t *testing.T, runner *fakeGitRunner) (Env, *strings.Builder) {
	t.Helper()
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "feature/ui")},
		},
	}
	env, _ := testEnv(testClaimConfig(t), api)
	env.NewGit = func() GitCLI { return git.NewWithRunner(runner) }
	var out strings.Builder
	env.Out = &out
	return env, &out
}

func runSliceFile(env Env, args ...string) error {
	return Run(context.Background(), append([]string{"slice-file", testSliceID, "--project", "project-1"}, args...), env)
}

func TestSliceFileJSONIsTheLinesAskedForAtTheBranch(t *testing.T) {
	runner := &fakeGitRunner{showOut: sampleFile}
	env, out := sliceFileEnv(t, runner)

	if err := runSliceFile(env, "--json", "--path", "main.go", "--from", "3", "--to", "4"); err != nil {
		t.Fatalf("slice-file: %v", err)
	}
	if !slices.Contains(runner.showArgs, "feature/ui:main.go") {
		t.Errorf("show args = %v, want the file read at the slice's branch", runner.showArgs)
	}
	var got fileLinesJSON
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if got.Path != "main.go" || got.Ref != "feature/ui" || got.From != 3 || got.Total != 5 {
		t.Errorf("got %+v, want main.go at feature/ui from line 3 of 5", got)
	}
	if want := []string{"func main() {", "\tprintln(\"hi\")"}; !slices.Equal(got.Lines, want) {
		t.Errorf("lines = %q, want %q", got.Lines, want)
	}
	if got.Language != "Go" || len(got.Tokens) != 2 || len(got.Tokens[0]) == 0 {
		t.Errorf("language %q, tokens %v: want Go, one lexed run list per line", got.Language, got.Tokens)
	}
}

func TestSliceFileReadsOneCommitWhenAsked(t *testing.T) {
	runner := &fakeGitRunner{showOut: sampleFile}
	env, _ := sliceFileEnv(t, runner)

	if err := runSliceFile(env, "--json", "--path", "main.go", "--commit", "abc123"); err != nil {
		t.Fatalf("slice-file: %v", err)
	}
	if !slices.Contains(runner.showArgs, "abc123:main.go") {
		t.Errorf("show args = %v, want the file read at the commit", runner.showArgs)
	}
}

func TestSliceFileWithNoLanguageCarriesNoTokens(t *testing.T) {
	env, out := sliceFileEnv(t, &fakeGitRunner{showOut: "one\ntwo\n"})

	if err := runSliceFile(env, "--json", "--path", "notes.unknownext"); err != nil {
		t.Fatalf("slice-file: %v", err)
	}
	var got fileLinesJSON
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if got.Language != "" || got.Tokens != nil || !slices.Equal(got.Lines, []string{"one", "two"}) {
		t.Errorf("got %+v, want both lines and no language or tokens", got)
	}
}

func TestSliceFilePrintsRawLines(t *testing.T) {
	env, out := sliceFileEnv(t, &fakeGitRunner{showOut: sampleFile})

	if err := runSliceFile(env, "--path", "main.go", "--from", "4"); err != nil {
		t.Fatalf("slice-file: %v", err)
	}
	if got, want := out.String(), "\tprintln(\"hi\")\n}\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestSliceFilePastTheEndPrintsNothing(t *testing.T) {
	env, out := sliceFileEnv(t, &fakeGitRunner{showOut: sampleFile})

	if err := runSliceFile(env, "--path", "main.go", "--from", "9"); err != nil {
		t.Fatalf("slice-file: %v", err)
	}
	if out.String() != "" {
		t.Errorf("output = %q, want nothing past the file's end", out.String())
	}
}

func TestLinesBetweenCutsToTheFile(t *testing.T) {
	all := []string{"a", "b", "c"}
	cases := []struct {
		from, to int
		want     []string
	}{
		{1, 0, []string{"a", "b", "c"}},
		{2, 2, []string{"b"}},
		{2, 9, []string{"b", "c"}},
		{4, 0, []string{}},
	}
	for _, c := range cases {
		if got := linesBetween(all, c.from, c.to); !slices.Equal(got, c.want) {
			t.Errorf("linesBetween(%d, %d) = %q, want %q", c.from, c.to, got, c.want)
		}
	}
}

func TestSliceFileRefusals(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no path", nil, "--path"},
		{"from below one", []string{"--path", "main.go", "--from", "0"}, "--from"},
		{"to before from", []string{"--path", "main.go", "--from", "3", "--to", "2"}, "--from"},
	}
	for _, c := range cases {
		env, _ := sliceFileEnv(t, &fakeGitRunner{showOut: sampleFile})
		err := runSliceFile(env, c.args...)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want one naming %s", c.name, err, c.want)
		}
	}

	env, _ := sliceFileEnv(t, &fakeGitRunner{})
	if err := Run(context.Background(), []string{"slice-file", "--project", "project-1", "--path", "x"}, env); err == nil {
		t.Error("no slice: want a usage error")
	}
	if err := runSliceFile(env, "--path", "x", "--nonsense"); err == nil {
		t.Error("an unknown flag: want a usage error")
	}
}

func TestSliceFileRefusesASliceWithNoBranch(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceTodo, "m1", "", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(t), api)
	err := runSliceFile(env, "--path", "main.go")
	if err == nil || !strings.Contains(err.Error(), "not handed back") {
		t.Errorf("error %v, want not handed back", err)
	}
}

func TestSliceFileSaysWhenGitCannotShowTheFile(t *testing.T) {
	env, _ := sliceFileEnv(t, &fakeGitRunner{showErr: errors.New("fatal: path 'gone.go' does not exist")})
	err := runSliceFile(env, "--path", "gone.go")
	if err == nil || !strings.Contains(err.Error(), "read gone.go at feature/ui") {
		t.Errorf("error %v, want the read named", err)
	}
}
