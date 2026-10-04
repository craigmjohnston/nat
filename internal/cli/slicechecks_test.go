package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// fakeChecksGH answers ViewPR with a fixed set of checks and FailedLog from a
// table keyed by job (or run, where no job is named), stubbing the rest of
// [GH] through fakePRBase.
type fakeChecksGH struct {
	fakePRBase
	checks  []gh.Check
	viewErr error
	logs    map[string]string
	logErr  error
	logRuns []string
}

func (f *fakeChecksGH) ViewPR(dir, ref string) (gh.PR, error) {
	if f.viewErr != nil {
		return gh.PR{}, f.viewErr
	}
	return gh.PR{URL: ref, Checks: f.checks}, nil
}

func (f *fakeChecksGH) FailedLog(dir string, ref gh.ActionsRef) (string, error) {
	f.logRuns = append(f.logRuns, ref.Run+"/"+ref.Job)
	if f.logErr != nil {
		return "", f.logErr
	}
	return f.logs[ref.Job], nil
}

const checksPR = "https://github.test/craig/nat/pull/7"

func checksEnv(t *testing.T, pr string, fake *fakeChecksGH) (Env, *strings.Builder) {
	t.Helper()
	api := &fakeAPI{pages: map[string][]notion.Page{
		"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress, pr)},
	}}
	env, _ := testEnv(testConfig(t), api)
	var out strings.Builder
	env.Out = &out
	env.NewGH = func() GH { return fake }
	return env, &out
}

// TestSliceChecksVerdicts reads each of the three verdicts off the pull
// request's own checks, and lists every check by name, state and run URL.
func TestSliceChecksVerdicts(t *testing.T) {
	tests := []struct {
		name   string
		checks []gh.Check
		want   string
	}{
		{"passing", []gh.Check{{Name: "test", State: "SUCCESS", URL: "https://ci.test/1"}}, "passing"},
		{"pending", []gh.Check{{Name: "test", State: "IN_PROGRESS", URL: "https://ci.test/1"}}, "pending"},
		{"failing", []gh.Check{
			{Name: "lint", State: "SUCCESS", URL: "https://ci.test/2"},
			{Name: "test", State: "FAILURE", URL: "https://ci.test/1"},
		}, "failing"},
	}
	for _, tt := range tests {
		env, out := checksEnv(t, checksPR, &fakeChecksGH{checks: tt.checks})
		if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--project", "project-1"}, env); err != nil {
			t.Fatalf("%s: slice-checks: %v", tt.name, err)
		}
		if !strings.HasPrefix(out.String(), "Checks: "+tt.want+" — "+checksPR+"\n") {
			t.Errorf("%s: output = %q, want the %s verdict first", tt.name, out.String(), tt.want)
		}
		for _, c := range tt.checks {
			if line := fmt.Sprintf("- %s — %s — %s\n", c.Name, c.State, c.URL); !strings.Contains(out.String(), line) {
				t.Errorf("%s: output missing %q:\n%s", tt.name, line, out.String())
			}
		}
	}
}

// TestSliceChecksJSON is the structured form: the PR, the verdict, every
// check — and no log key where --log was not asked for.
func TestSliceChecksJSON(t *testing.T) {
	env, out := checksEnv(t, checksPR, &fakeChecksGH{checks: []gh.Check{
		{Name: "test", State: "FAILURE", URL: "https://github.com/o/r/actions/runs/1/job/2"},
	}})
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks --json: %v", err)
	}
	want := `{"checks":[{"name":"test","state":"FAILURE","url":"https://github.com/o/r/actions/runs/1/job/2"}],"pr":"` + checksPR + `","verdict":"failing"}`
	if got := compactJSON(t, out.String()); got != want {
		t.Errorf("json = %s, want %s", got, want)
	}
}

// TestSliceChecksLog appends the failed log of an Actions check — its job's,
// cut to the last 200 lines with a line saying how many went — and nothing but
// the URL for a status another service reported, or for a check that passed.
func TestSliceChecksLog(t *testing.T) {
	var long []string
	for i := 1; i <= 250; i++ {
		long = append(long, fmt.Sprintf("line %d", i))
	}
	fake := &fakeChecksGH{
		checks: []gh.Check{
			{Name: "test", State: "FAILURE", URL: "https://github.com/o/r/actions/runs/11/job/22"},
			{Name: "deploy", State: "ERROR", URL: "https://ci.example.com/build/9"},
			{Name: "lint", State: "SUCCESS", URL: "https://github.com/o/r/actions/runs/11/job/33"},
		},
		logs: map[string]string{"22": strings.Join(long, "\n")},
	}
	env, out := checksEnv(t, checksPR, fake)
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--log", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks --log: %v", err)
	}
	if want := []string{"11/22"}; strings.Join(fake.logRuns, ",") != strings.Join(want, ",") {
		t.Errorf("read logs %v, want only the failed Actions job's %v", fake.logRuns, want)
	}
	got := out.String()
	for _, want := range []string{
		"## test failed log", "… 50 earlier lines cut\nline 51\n", "line 250\n```",
		"- deploy — ERROR — https://ci.example.com/build/9",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"line 50\n", "## deploy", "## lint"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("output carries %q:\n%s", unwanted, got)
		}
	}
}

// TestSliceChecksLogShortAndUnreadable prints a log under the limit whole,
// and says under its check, in both forms, why a log could not be read — the
// verdict still answers.
func TestSliceChecksLogShortAndUnreadable(t *testing.T) {
	failed := []gh.Check{{Name: "test", State: "FAILURE", URL: "https://github.com/o/r/actions/runs/11"}}
	env, out := checksEnv(t, checksPR, &fakeChecksGH{checks: failed, logs: map[string]string{"": "boom\nat step 3"}})
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--log", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks --log: %v", err)
	}
	if !strings.Contains(out.String(), "```\nboom\nat step 3\n```") || strings.Contains(out.String(), "cut") {
		t.Errorf("output = %q, want the short log whole", out.String())
	}

	env, out = checksEnv(t, checksPR, &fakeChecksGH{checks: failed, logErr: errors.New("gone\nand more")})
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--log", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks --log over an unreadable log: %v", err)
	}
	want := "- test — FAILURE — https://github.com/o/r/actions/runs/11\n  log not available: gone\n"
	if !strings.HasPrefix(out.String(), "Checks: failing") || !strings.Contains(out.String(), want) ||
		strings.Contains(out.String(), "failed log") || strings.Contains(out.String(), "and more") {
		t.Errorf("output = %q, want the verdict and %q, and no log", out.String(), want)
	}

	env, out = checksEnv(t, checksPR, &fakeChecksGH{checks: failed, logErr: errors.New("gone\nand more")})
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--log", "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks --log --json over an unreadable log: %v", err)
	}
	wantJSON := `{"checks":[{"log_error":"gone","name":"test","state":"FAILURE","url":"https://github.com/o/r/actions/runs/11"}],"pr":"` + checksPR + `","verdict":"failing"}`
	if got := compactJSON(t, out.String()); got != wantJSON {
		t.Errorf("json = %s, want %s", got, wantJSON)
	}
}

// TestSliceChecksNoPullRequest says when checks run and exits 0, in either
// form, without asking gh anything.
func TestSliceChecksNoPullRequest(t *testing.T) {
	fake := &fakeChecksGH{viewErr: errors.New("gh must not be asked")}
	env, out := checksEnv(t, "", fake)
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks: %v", err)
	}
	if !strings.Contains(out.String(), "checks run once the slice is approved") {
		t.Errorf("output = %q, want it to say when checks run", out.String())
	}
	env, out = checksEnv(t, "", fake)
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks --json: %v", err)
	}
	if want := `{"checks":[],"pr":"","verdict":"none"}`; compactJSON(t, out.String()) != want {
		t.Errorf("json = %s, want %s", out.String(), want)
	}
}

// TestSliceChecksRefusals: a gh read that fails is the command's error, as
// are a bad invocation, an unknown project and a slice that cannot be read.
func TestSliceChecksRefusals(t *testing.T) {
	env, _ := checksEnv(t, checksPR, &fakeChecksGH{viewErr: errors.New("rate limited")})
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--project", "project-1"}, env); err == nil ||
		!strings.Contains(err.Error(), "rate limited") {
		t.Errorf("err = %v, want the gh failure", err)
	}
	for _, args := range [][]string{
		{"slice-checks", "--project", "project-1"},
		{"slice-checks", testSliceID, "--bogus", "--project", "project-1"},
		{"slice-checks", "not a page", "--project", "project-1"},
		{"slice-checks", testSliceID},
		{"slice-checks", "00000000-0000-0000-0000-000000000099", "--project", "project-1"},
	} {
		env, _ := checksEnv(t, checksPR, &fakeChecksGH{})
		if err := Run(context.Background(), args, env); err == nil {
			t.Errorf("%v: want a refusal", args)
		}
	}
}

// TestSliceChecksRefusesAnUnopenablePlan reports a plan that cannot be opened
// before gh is asked anything.
func TestSliceChecksRefusesAnUnopenablePlan(t *testing.T) {
	env, _ := checksEnv(t, checksPR, &fakeChecksGH{})
	path, err := store.LocalPath("project-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--project", "project-1"}, env); err == nil {
		t.Error("slice-checks over an unopenable plan: want an error")
	}
}

func compactJSON(t *testing.T, s string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, s)
	}
	b, _ := json.Marshal(v)
	return string(b)
}
