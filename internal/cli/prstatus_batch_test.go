package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/notion"
)

// twoProjectEnv is project-1 (nat) with one slice in review on craig/nat#1
// and project-2 (brewery) with one on craig/brewery#2, both read through the
// same fake workspace.
func twoProjectEnv(t *testing.T) (Env, *strings.Builder) {
	t.Helper()
	cfg := testConfig(t)
	cfg.Projects["project-2"] = config.ProjectConfig{Name: "brewery", SlicesDSID: "slices-ds-2", WorkingDir: "/tmp/brewery"}
	api := &fakeAPI{pages: map[string][]notion.Page{
		"slices-ds": {slicePageForStatus("s1", "Nat work", notion.SliceInProgress, "",
			"https://github.test/craig/nat/pull/1")},
		"slices-ds-2": {slicePageForStatus("b1", "Brewery work", notion.SliceInProgress, "",
			"https://github.test/craig/brewery/pull/2")},
	}}
	env, _ := testEnv(cfg, api)
	var out strings.Builder
	env.Out = &out
	return env, &out
}

// One run over two projects in two repositories, with the pull request on
// screen in detail, is one GraphQL document — its repositories and pull
// requests aliased, the detail through both fragments — and its JSON keys
// each project's reading by ID, the rate limit and the detail once.
func TestPRStatusReadsEveryProjectInOneDocument(t *testing.T) {
	env, out := twoProjectEnv(t)
	status := func(n, decision string) string {
		return `{"number":` + n + `,"url":"https://github.test/craig/x/pull/` + n + `","state":"OPEN",` +
			`"reviewDecision":"` + decision + `","mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","baseRefName":"main",` +
			`"lastCommit":{"nodes":[]}`
	}
	runner := &readingRunner{out: `{"data":{"rateLimit":{"limit":5000,"remaining":4321,"resetAt":"2026-10-06T13:00:00Z"},` +
		`"r0":{"d":` + status("1", "") + `,"title":"Nat work","author":{"login":"craig"},"allCommits":{"totalCount":2}},` +
		`"p1":` + status("1", "") + `}},` +
		`"r1":{"p2":` + status("2", "APPROVED") + `}}}}`}
	env.NewGH = func() GH { return realReading{fakePRReader: &fakePRReader{}, cli: gh.NewWithRunner(runner)} }

	err := Run(context.Background(), []string{"pr-status", "--project", "project-1", "--project", "project-2",
		"--project", "project-1", "--detail", "https://github.test/craig/nat/pull/1", "--json"}, env)
	if err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if len(runner.docs) != 1 {
		t.Fatalf("ran %d documents, want one", len(runner.docs))
	}
	want := `query {
  rateLimit { limit remaining resetAt }
  r0: repository(owner: "craig", name: "nat") {
    d: pullRequest(number: 1) { ...status ...detail }
    p1: pullRequest(number: 1) { ...status }
  }
  r1: repository(owner: "craig", name: "brewery") {
    p2: pullRequest(number: 2) { ...status }
  }
}
fragment status on PullRequest {`
	if !strings.HasPrefix(runner.docs[0], want) || !strings.Contains(runner.docs[0], "fragment detail on PullRequest {") {
		t.Errorf("document =\n%s\nwant it to open\n%s\nand carry the detail fragment", runner.docs[0], want)
	}

	var doc prStatusMultiDoc
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if len(doc.Projects) != 2 || doc.Projects["project-1"].Slices[0].Readiness != "awaiting review" ||
		doc.Projects["project-2"].Slices[0].Readiness != "ready to merge" {
		t.Errorf("projects = %+v, want each project's reading by ID", doc.Projects)
	}
	if doc.Projects["project-1"].RateLimit != nil || doc.Projects["project-1"].Detail != nil {
		t.Errorf("project-1 = %+v, want the rate limit and detail at the top alone", doc.Projects["project-1"])
	}
	if doc.RateLimit == nil || doc.RateLimit.Remaining != 4321 {
		t.Errorf("rate_limit = %+v, want the document's", doc.RateLimit)
	}
	if doc.Detail == nil || doc.Detail.Number != 1 || doc.Detail.Title != "Nat work" || doc.Detail.Commits != 2 {
		t.Errorf("detail = %+v, want #1 in full", doc.Detail)
	}
}

// The markdown names each project over its own pull requests, then the
// budget, then the detail.
func TestPRStatusMarkdownOfSeveralProjects(t *testing.T) {
	env, out := twoProjectEnv(t)
	detail := gh.PR{Number: 1, Title: "Nat work", State: "OPEN"}
	env.NewGH = func() GH {
		return &fakePRReader{
			prs:    map[string]gh.PR{"https://github.test/craig/nat/pull/1": openPR(true, true)},
			detail: &detail,
			rate:   &gh.RateLimit{Limit: 5000, Remaining: 12, ResetAt: time.Date(2026, 10, 6, 13, 0, 0, 0, time.UTC)},
		}
	}

	err := Run(context.Background(), []string{"pr-status", "--project", "project-1", "--project", "project-2",
		"--detail", "https://github.test/craig/nat/pull/1"}, env)
	if err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	for _, line := range []string{
		"# nat\n\n# Pull requests\n\n- Nat work — ready to merge — ",
		"# brewery\n\n# Pull requests\n\n- Brewery work — unread — ",
		"GitHub budget: 12 of 5000 points left, resets at 2026-10-06T13:00:00Z\n",
		"Nat work",
	} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("markdown lacks %q:\n%s", line, out)
		}
	}
}

// A detail that names no pull request, an argument, and no project at all
// are each refused before anything is read.
func TestPRStatusRefusesItsUsage(t *testing.T) {
	for _, args := range [][]string{
		{"pr-status", "--project", "project-1", "--detail", "https://github.test/craig/nat/issues/1"},
		{"pr-status", "--project", "project-1", "extra"},
	} {
		env, _ := testEnv(testConfig(t), &fakeAPI{})
		var usage *UsageError
		if err := Run(context.Background(), args, env); !errors.As(err, &usage) {
			t.Errorf("%v: err = %v, want a usage error", args, err)
		}
	}
	env, _ := testEnv(testConfig(t), &fakeAPI{})
	if err := Run(context.Background(), []string{"pr-status"}, env); err == nil || !strings.Contains(err.Error(), "project-1") {
		t.Errorf("err = %v, want the projects this machine tracks listed", err)
	}
}

// A PR URL that names no pull request is not asked about, and reads unread.
func TestPRStatusLeavesAnUnparseableURLUnread(t *testing.T) {
	api := &fakeAPI{pages: map[string][]notion.Page{
		"slices-ds": {slicePageForStatus("s1", "Odd link", notion.SliceInProgress, "", "https://example.test/not-a-pr")},
	}}
	env, out := testEnv(testConfig(t), api)
	reader := &fakePRReader{}
	env.NewGH = func() GH { return reader }

	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if reader.calls != 0 || !strings.Contains(out.String(), "Odd link — unread — ") {
		t.Errorf("calls = %d, output = %q, want nothing asked and the slice unread", reader.calls, out)
	}
}

// Each session that has not ended rides the reading: its five most recent
// branches in the repository its origin names, its pull requests in the
// output — stale where a branch went unread — and kept on disk, where
// session-list reads them back without asking GitHub. A session whose origin
// is no GitHub repository is asked nothing and reads stale; one with no
// branch at all, nothing.
func TestPRStatusReadsSessionBranches(t *testing.T) {
	env, _ := sessionTestEnv(t)
	path := filepath.Join(t.TempDir(), lastReadingFileName)
	env.ReadingPath = func() (string, error) { return path, nil }
	read := seedSession(t, env, "/repos/read", "session/one")
	foreign := seedSession(t, env, "/repos/foreign", "session/f")
	bare := seedSession(t, env, "/repos/bare", "")
	ended := seedSession(t, env, "/repos/ended", "session/e")
	endSession(t, env, ended)
	fallback := &fakeSessionRepo{currentBranch: "session/one", reflog: []string{"b2", "b3", "b4", "b5", "b6"},
		remote: "git@github.com:Craig/Nat.git"}
	gitByDir := map[string]*fakeSessionRepo{
		"/repos/foreign": {currentBranch: "session/f", remote: "/somewhere/else"},
		"/repos/bare":    {},
	}
	env.NewGit = func() GitCLI { return dirGit{fallback: fallback, byDir: gitByDir} }
	reader := &fakePRReader{heads: map[string][]gh.HeadPR{
		"session/one": {{Number: 4, Title: "Four", URL: "https://github.com/craig/nat/pull/4", State: "MERGED"}},
		"b2":          nil, "b3": nil, "b4": nil,
	}}
	env.NewGH = func() GH { return reader }
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1", "--json"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	var heads []string
	for _, h := range reader.queries[0].Heads {
		if h.Owner != "craig" || h.Repo != "nat" {
			t.Errorf("asked %+v, want craig/nat", h)
		}
		heads = append(heads, h.Branch)
	}
	if want := []string{"session/one", "b2", "b3", "b4", "b5"}; !reflect.DeepEqual(heads, want) {
		t.Errorf("asked about %v, want the five most recent of the read session alone", heads)
	}
	var doc prStatusDoc
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	got := map[string]sessionPRsJSON{}
	for _, s := range doc.Sessions {
		got[s.ID] = s
	}
	if len(doc.Sessions) != 3 {
		t.Errorf("sessions = %+v, want the three not ended", doc.Sessions)
	}
	if s := got[read]; len(s.PRs) != 1 || s.PRs[0].Number != 4 || !s.PRsStale {
		t.Errorf("read session = %+v, want #4, stale for b5", s)
	}
	if s := got[foreign]; len(s.PRs) != 0 || !s.PRsStale {
		t.Errorf("foreign session = %+v, want none and stale", s)
	}
	if s := got[bare]; len(s.PRs) != 0 || s.PRsStale || s.PRs == nil {
		t.Errorf("bare session = %+v, want an empty list, not stale", s)
	}

	reader.calls = 0
	var listed strings.Builder
	env.Out = &listed
	if err := Run(context.Background(), []string{"session-list", "--project", "project-1", "--json"}, env); err != nil {
		t.Fatalf("session-list: %v", err)
	}
	var rows []sessionListJSON
	if err := json.Unmarshal([]byte(listed.String()), &rows); err != nil {
		t.Fatalf("session-list is not JSON: %v\n%s", err, listed.String())
	}
	for _, row := range rows {
		if row.ID == read && (len(row.PRs) != 1 || row.PRs[0].Number != 4 || !row.PRsStale) {
			t.Errorf("session-list row = %+v, want the reading kept", row)
		}
	}
	if reader.calls != 0 {
		t.Errorf("session-list read GitHub %d times, want none", reader.calls)
	}
}

// dirGit answers each session's git reads by its directory, falling back to
// one repository for the rest.
type dirGit struct {
	fallback GitCLI
	byDir    map[string]*fakeSessionRepo
}

func (d dirGit) repo(dir string) GitCLI {
	if r, ok := d.byDir[dir]; ok {
		return r
	}
	return d.fallback
}

func (d dirGit) CurrentBranch(dir string) (string, error)    { return d.repo(dir).CurrentBranch(dir) }
func (d dirGit) ReflogBranches(dir string) ([]string, error) { return d.repo(dir).ReflogBranches(dir) }
func (d dirGit) RemoteURL(dir string) (string, error)        { return d.repo(dir).RemoteURL(dir) }
func (d dirGit) Fetch(dir string)                            {}
func (d dirGit) Base(dir string) string                      { return "main" }
func (d dirGit) LogOneline(string, string, string) (string, error) {
	return "", nil
}
func (d dirGit) DiffStat(string, string, string) (string, error) { return "", nil }
func (d dirGit) DiffFrom(string, string, string) (string, string, error) {
	return "", "", nil
}
func (d dirGit) DiffWorkingTreeFrom(string, string) (string, string, error) { return "", "", nil }
func (d dirGit) CommitsFrom(string, string, string) (string, []git.Commit, error) {
	return "", nil, nil
}
func (d dirGit) CommitDiff(string, string) (string, error)       { return "", nil }
func (d dirGit) Show(string, string, string) ([]string, error)   { return nil, nil }
func (d dirGit) ConflictsWithBase(string, string) git.MergeState { return git.MergeUnknown }

// endSession marks a seeded session ended.
func endSession(t *testing.T, env Env, id string) {
	t.Helper()
	ctx := context.Background()
	_, projectID, project, err := env.projectFor("project-1")
	if err != nil {
		t.Fatalf("projectFor: %v", err)
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		t.Fatalf("storeFor: %v", err)
	}
	if err := st.EndSession(ctx, id); err != nil {
		t.Fatalf("EndSession: %v", err)
	}
}

// A project whose sessions cannot be read still has its pull requests read:
// the sessions are left out, logged.
func TestPRStatusReadsPullRequestsDespiteUnreadSessions(t *testing.T) {
	env, _ := sessionTestEnv(t)
	seedHydratedProject(t, "project-1", func(db *sql.DB) { dropSessionsTable(t, db) })
	reader := &fakePRReader{}
	env.NewGH = func() GH { return reader }
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }

	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
}

// What a reading finds is kept for the commands that read it back: each pull
// request's base — the detail's too — merged over what an earlier reading
// kept.
func TestPRStatusKeepsTheBases(t *testing.T) {
	api := &fakeAPI{pages: map[string][]notion.Page{
		"slices-ds": {slicePageForStatus("s1", "Release work", notion.SliceInProgress, "",
			"https://github.test/craig/nat/pull/1")},
	}}
	env, _ := testEnv(testConfig(t), api)
	keepBase(t, &env, "https://github.test/craig/nat/pull/99", "old")
	release := openPR(false, false)
	release.BaseRefName = "release"
	detail := gh.PR{Number: 5, BaseRefName: "develop"}
	env.NewGH = func() GH {
		return &fakePRReader{prs: map[string]gh.PR{"https://github.test/craig/nat/pull/1": release}, detail: &detail}
	}

	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1",
		"--detail", "https://github.test/craig/nat/pull/5"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	want := map[string]string{
		"https://github.test/craig/nat/pull/99": "old",
		"https://github.test/craig/nat/pull/1":  "release",
		"https://github.test/craig/nat/pull/5":  "develop",
	}
	if got := env.loadLastReading().Bases; !reflect.DeepEqual(got, want) {
		t.Errorf("bases = %v, want %v", got, want)
	}
}

// The last reading reads as nothing where there is no path to keep it at, a
// path that cannot be resolved, a file that cannot be read or one that will
// not parse — and a write that cannot land changes nothing.
func TestLastReadingConcludesNothingFromWhatItCannotRead(t *testing.T) {
	dir := t.TempDir()
	notJSON := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(notJSON, []byte("not JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]func() (string, error){
		"unresolved":  func() (string, error) { return "", errors.New("no state dir") },
		"a directory": func() (string, error) { return dir, nil },
		"not JSON":    func() (string, error) { return notJSON, nil },
		"missing":     func() (string, error) { return filepath.Join(dir, "none.json"), nil },
	} {
		env := Env{ReadingPath: path}
		if got := env.loadLastReading(); len(got.Bases) != 0 || len(got.Sessions) != 0 || got.Bases == nil {
			t.Errorf("%s: reading = %+v, want empty maps", name, got)
		}
	}

	blocked := filepath.Join(notJSON, "under-a-file.json")
	env := Env{ReadingPath: func() (string, error) { return blocked, nil }}
	env.saveLastReading(lastReading{Bases: map[string]string{"u": "b"}})
	if _, err := os.Stat(blocked); err == nil {
		t.Error("a reading was written under a file")
	}
	Env{}.saveLastReading(lastReading{})
}

// The reading lives in nat's state directory.
func TestDefaultReadingPath(t *testing.T) {
	prev := stateDir
	t.Cleanup(func() { stateDir = prev })
	stateDir = func() (string, error) { return "/state", nil }
	if got, err := DefaultReadingPath(); err != nil || got != "/state/github-reading.json" {
		t.Errorf("DefaultReadingPath() = %q, %v", got, err)
	}
	stateDir = func() (string, error) { return "", errors.New("no home") }
	if _, err := DefaultReadingPath(); err == nil {
		t.Error("an unresolvable state directory resolved")
	}
}

// A merge whose Done cannot be written is logged and left for the next run:
// nothing is nudged and no worktree is taken.
func TestPRStatusLeavesAMergeUnsettledWhenDoneCannotBeWritten(t *testing.T) {
	cfg := testConfig(t)
	seedHydratedSlice(t, "project-1", testSliceID, "Merged elsewhere", "In progress", func(db *sql.DB) {
		if _, err := db.Exec(`UPDATE slices SET pr = ? WHERE id = ?`,
			"https://github.test/craig/nat/pull/7", testSliceID); err != nil {
			t.Fatalf("seed the PR: %v", err)
		}
		if _, err := db.Exec(`DROP TABLE sync`); err != nil {
			t.Fatalf("break the plan's sync table: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})
	var nudges int
	env.Nudge = func() { nudges++ }
	env.NewGH = func() GH {
		return &fakePRReader{prs: map[string]gh.PR{"https://github.test/craig/nat/pull/7": {State: gh.PRStateMerged}}}
	}
	w := withWorktree(&env)

	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if nudges != 0 || len(w.removed) != 0 {
		t.Errorf("nudges = %d, removed = %+v, want neither", nudges, w.removed)
	}
}
