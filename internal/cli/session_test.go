package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/store"
)

// seedSession files a session directly through the store, with an exact
// branch of the test's own choosing — [launchOneSession] derives its
// branch from the session's own ID, which is fine for a test that reads it
// back with the same fakes, but not for one (like session-list's own
// pull-request read) that has to name a branch a fake gh is keyed by.
func seedSession(t *testing.T, env Env, dir, branch string) string {
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
	added, err := st.AddSession(ctx, storeProject(projectID, project),
		store.NewSession{ID: store.NewSessionID(), Dir: dir, Branch: branch})
	if err != nil {
		t.Fatalf("AddSession: %v", err)
	}
	return added.ID
}

// domainSessionFor is a bare [domain.Session] naming just the branch —
// enough for [sessionBranches]' own unit test, which needs no dir or ID.
func domainSessionFor(branch string) domain.Session {
	return domain.Session{Dir: "/repo", Branch: branch}
}

// fakeSessionWorktrees stands in for git's worktrees for the session
// commands' tests, the same shape internal/actions' own fakeWorktrees takes.
type fakeSessionWorktrees struct {
	created []struct{ dir, branch, base string }
	removed []struct{ dir, branch string }
	// existingPath, when set, is what Path answers for any branch — a
	// worktree already cut, which is what [actions.RemoveWorktree] needs to
	// find before it removes it.
	existingPath string
	// createErr, when set, is what Create refuses with.
	createErr error
}

func (f *fakeSessionWorktrees) Path(dir, branch string) (string, error) {
	if f.existingPath != "" {
		return f.existingPath, nil
	}
	return "", fmt.Errorf("no worktree for %s", branch)
}

func (f *fakeSessionWorktrees) Create(dir, branch, base string) (string, error) {
	f.created = append(f.created, struct{ dir, branch, base string }{dir, branch, base})
	if f.createErr != nil {
		return "", f.createErr
	}
	return filepath.Join(dir+".worktrees", branch), nil
}

func (f *fakeSessionWorktrees) Remove(dir, branch string) error {
	f.removed = append(f.removed, struct{ dir, branch string }{dir, branch})
	return nil
}

// fakeSessionRepo stands in for git's own reads: the actions.Repo half of
// [GitCLI] session-launch calls, and the session-status reads
// [CurrentBranch]/[ReflogBranches]; the rest of GitCLI is stubbed since
// nothing here calls it.
type fakeSessionRepo struct {
	base          string
	fetched       []string
	currentBranch string
	currentErr    error
	reflog        []string
	reflogErr     error
	diffOut       string
	diffFromCalls []string
	diffWorkingTreeCalls int
	diffWorkingTreeErr   error
}

func (f *fakeSessionRepo) Fetch(dir string)  { f.fetched = append(f.fetched, dir) }
func (f *fakeSessionRepo) Base(string) string { return f.base }
func (f *fakeSessionRepo) LogOneline(dir, base, branch string) (string, error) { return "", nil }
func (f *fakeSessionRepo) DiffStat(dir, base, branch string) (string, error)   { return "", nil }
func (f *fakeSessionRepo) DiffFrom(dir, baseName, branch string) (string, string, error) {
	f.diffFromCalls = append(f.diffFromCalls, branch)
	return "base", f.diffOut, nil
}
func (f *fakeSessionRepo) DiffWorkingTreeFrom(dir, baseName string) (string, string, error) {
	f.diffWorkingTreeCalls++
	if f.diffWorkingTreeErr != nil {
		return "", "", f.diffWorkingTreeErr
	}
	return "base", f.diffOut, nil
}
func (f *fakeSessionRepo) CommitsFrom(dir, baseName, branch string) (string, []git.Commit, error) {
	return "", nil, nil
}
func (f *fakeSessionRepo) CommitDiff(dir, sha string) (string, error) { return "", nil }
func (f *fakeSessionRepo) CurrentBranch(dir string) (string, error) {
	return f.currentBranch, f.currentErr
}
func (f *fakeSessionRepo) ReflogBranches(dir string) ([]string, error) { return f.reflog, f.reflogErr }

var _ actions.Worktrees = (*fakeSessionWorktrees)(nil)
var _ GitCLI = (*fakeSessionRepo)(nil)

func sessionTestEnv(t *testing.T) (Env, *strings.Builder) {
	t.Helper()
	env, _ := testEnv(testClaimConfig(t), &fakeAPI{})
	var out strings.Builder
	env.Out = &out
	return env, &out
}

// A directory that is not a git repository runs the session in place, with
// no worktree cut at all.
func TestSessionLaunchRunsInPlaceOutsideARepo(t *testing.T) {
	env, out := sessionTestEnv(t)
	dir := t.TempDir()
	runner := &agentTestRunner{}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	env.NewWorktrees = func() actions.Worktrees { return nil }
	env.NewGit = func() GitCLI { return nil }

	err := Run(context.Background(), []string{
		"session-launch", "--dir", dir, "--json", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("session-launch: %v", err)
	}

	var doc sessionLaunchJSON
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out.String())
	}
	if doc.Dir != dir {
		t.Errorf("Dir = %q, want %q", doc.Dir, dir)
	}
	if doc.Branch != "" {
		t.Errorf("Branch = %q, want none outside a git repository", doc.Branch)
	}
	if !strings.HasPrefix(doc.Session, agent.SessionPrefix+"session-") {
		t.Errorf("Session = %q, want the nat-session- prefix", doc.Session)
	}
	if len(runner.launchArgs) == 0 {
		t.Fatal("no tmux launch was made")
	}
	for _, a := range runner.launchArgs {
		if strings.Contains(a, "$(cat") {
			t.Errorf("launch args = %v, want no prompt file at all", runner.launchArgs)
		}
	}
	if len(runner.tagged) != 1 || runner.tagged[0] != doc.Tag {
		t.Errorf("tagged = %v, want the pane tagged with %q", runner.tagged, doc.Tag)
	}
}

// A directory that is a git repository gets a worktree cut on session/<id>,
// fetched and based off the remote's current default first.
func TestSessionLaunchCutsAWorktreeInARepo(t *testing.T) {
	env, out := sessionTestEnv(t)
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	worktrees := &fakeSessionWorktrees{}
	repo := &fakeSessionRepo{base: "origin/main"}
	runner := &agentTestRunner{}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	env.NewWorktrees = func() actions.Worktrees { return worktrees }
	env.NewGit = func() GitCLI { return repo }

	err := Run(context.Background(), []string{
		"session-launch", "--dir", dir, "--json", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("session-launch: %v", err)
	}

	var doc sessionLaunchJSON
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out.String())
	}
	if !strings.HasPrefix(doc.Branch, "session/") {
		t.Errorf("Branch = %q, want the session/ prefix", doc.Branch)
	}
	if len(worktrees.created) != 1 || worktrees.created[0].branch != doc.Branch || worktrees.created[0].base != "origin/main" {
		t.Errorf("created = %+v, want one worktree cut on %q from origin/main", worktrees.created, doc.Branch)
	}
	if len(repo.fetched) != 1 || repo.fetched[0] != dir {
		t.Errorf("fetched = %v, want one fetch of %s", repo.fetched, dir)
	}
}

func TestSessionLaunchRefusesAMissingDirectory(t *testing.T) {
	env, _ := sessionTestEnv(t)
	err := Run(context.Background(), []string{
		"session-launch", "--dir", "/no/such/directory", "--project", "project-1",
	}, env)
	if err == nil {
		t.Fatal("session-launch: want a refusal for a missing directory")
	}
}

// fakeSessionGH stands in for gh's pull request reads across the session
// commands: OpenPRs/CreatePR/MergePR/CommentPR are stubbed since none of
// them are ever called by session-list/-status.
type fakeSessionGH struct {
	byBranch map[string][]gh.HeadPR
	err      map[string]error
	calls    []string
}

func (f *fakeSessionGH) ListPRsForHead(dir, branch string) ([]gh.HeadPR, error) {
	f.calls = append(f.calls, branch)
	if err := f.err[branch]; err != nil {
		return nil, err
	}
	return f.byBranch[branch], nil
}
func (f *fakeSessionGH) CreatePR(dir, branch, title, body string) (string, error) { return "", nil }
func (f *fakeSessionGH) MergePR(dir, ref string) error                            { return nil }
func (f *fakeSessionGH) CommentPR(dir, ref, body string) (string, error)          { return "", nil }
func (f *fakeSessionGH) OpenPRs(dir string) (map[string]gh.PRStatus, error)       { return nil, nil }
func (f *fakeSessionGH) ViewPR(dir, ref string) (gh.PR, error)                    { return gh.PR{}, nil }

var _ GH = (*fakeSessionGH)(nil)

// launchOneSession is the shared setup for session-list/-status/-diff tests:
// a session already filed against the project, with no live tmux and no
// pull requests yet, so a test only has to layer on what it means to assert.
func launchOneSession(t *testing.T, env Env, dir, branch string) string {
	t.Helper()
	env.NewWorktrees = func() actions.Worktrees { return nil }
	env.NewGit = func() GitCLI { return nil }
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	var out strings.Builder
	env.Out = &out
	if err := Run(context.Background(), []string{
		"session-launch", "--dir", dir, "--json", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-launch: %v", err)
	}
	var doc sessionLaunchJSON
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out.String())
	}
	return doc.ID
}

func TestSessionListReportsLiveAndPullRequests(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")

	tmuxRunner := &agentTestRunner{liveSessions: map[string]string{agent.SessionTag("project-1", id): "nat-session-abcd1234"}}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(tmuxRunner) }
	env.NewGH = func() GH { return &fakeSessionGH{} }
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{
		"session-list", "--json", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-list: %v", err)
	}

	var docs []sessionListJSON
	if err := json.Unmarshal([]byte(out.String()), &docs); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out.String())
	}
	if len(docs) != 1 {
		t.Fatalf("session-list = %+v, want one session", docs)
	}
	if !docs[0].Live {
		t.Errorf("Live = false, want true: tmux reports it running")
	}
	if docs[0].ID != id {
		t.Errorf("ID = %q, want %q", docs[0].ID, id)
	}
}

// This is the slice's own acceptance criterion: two pull requests opened
// from two branches in one session both appear in session-status.
func TestSessionStatusReportsPullRequestsFromEveryBranch(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")

	gitCLI := &fakeSessionRepo{}
	gitCLI.currentBranch = "session/two"
	gitCLI.reflog = []string{"session/one", "session/two"}
	env.NewGit = func() GitCLI { return gitCLI }
	ghCLI := &fakeSessionGH{byBranch: map[string][]gh.HeadPR{
		"session/one": {{Number: 1, Title: "First", State: "MERGED", URL: "https://github.test/x/y/pull/1"}},
		"session/two": {{Number: 2, Title: "Second", State: "OPEN", URL: "https://github.test/x/y/pull/2"}},
	}}
	env.NewGH = func() GH { return ghCLI }
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	env.NewWorktrees = func() actions.Worktrees { return &fakeSessionWorktrees{} }
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{
		"session-status", id, "--json", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-status: %v", err)
	}

	var doc sessionStatusJSON
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out.String())
	}
	if len(doc.Branches) != 2 {
		t.Fatalf("Branches = %+v, want both branches reported", doc.Branches)
	}
	var total int
	for _, b := range doc.Branches {
		total += len(b.PRs)
	}
	if total != 2 {
		t.Errorf("total pull requests reported = %d, want 2", total)
	}
	// Neither PR is merged (one open), so the session must not be ended.
	if doc.Ended {
		t.Errorf("Ended = true, want the session left running: one PR is still open")
	}
}

// This is the slice's own acceptance criterion: the worktree is removed
// once every pull request reads merged.
func TestSessionStatusEndsOnceEveryPRIsMerged(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")

	gitCLI := &fakeSessionRepo{currentBranch: "session/one"}
	env.NewGit = func() GitCLI { return gitCLI }
	ghCLI := &fakeSessionGH{byBranch: map[string][]gh.HeadPR{
		"session/one": {{Number: 1, Title: "First", State: "MERGED"}},
	}}
	env.NewGH = func() GH { return ghCLI }
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	worktrees := &fakeSessionWorktrees{}
	worktrees.existingPath = "/repo.worktrees/session-one"
	env.NewWorktrees = func() actions.Worktrees { return worktrees }
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{
		"session-status", id, "--json", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-status: %v", err)
	}

	var doc sessionStatusJSON
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out.String())
	}
	if !doc.Ended {
		t.Fatalf("Ended = false, want the session ended once every PR merged")
	}
	if len(worktrees.removed) != 1 {
		t.Errorf("removed = %v, want the worktree removed exactly once", worktrees.removed)
	}

	// A second read confirms the session is now recorded ended.
	var out2 strings.Builder
	env.Out = &out2
	if err := Run(context.Background(), []string{
		"session-status", id, "--json", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-status (second read): %v", err)
	}
	var doc2 sessionStatusJSON
	if err := json.Unmarshal([]byte(out2.String()), &doc2); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out2.String())
	}
	if !doc2.Ended {
		t.Errorf("Ended (second read) = false, want the session to still read ended")
	}
}

func TestSessionStatusRefusesAnUnknownID(t *testing.T) {
	env, _ := sessionTestEnv(t)
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	err := Run(context.Background(), []string{
		"session-status", "8f654180-9b8d-53fb-1024-9ee08f654180", "--project", "project-1",
	}, env)
	if err == nil {
		t.Fatal("session-status: want a refusal for an unknown session")
	}
}

// tagOrPageID accepts a session's own pane tag literally, so agent-kill and
// agent-send can end or nudge an ad hoc session the same way they reach a
// slice's live agent.
func TestTagOrPageIDAcceptsASessionTag(t *testing.T) {
	tag := agent.SessionTag("project-1", "sess-1")
	got, err := tagOrPageID("agent-kill", tag)
	if err != nil {
		t.Fatalf("tagOrPageID: %v", err)
	}
	if got != tag {
		t.Errorf("tagOrPageID(%q) = %q, want it unchanged", tag, got)
	}
}

func TestAgentKillEndsALiveSession(t *testing.T) {
	env, _ := sessionTestEnv(t)
	tag := agent.SessionTag("project-1", "sess-1")
	runner := &agentTestRunner{liveSessions: map[string]string{tag: "nat-session-abcd1234"}}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }

	if err := Run(context.Background(), []string{
		"agent-kill", tag, "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("agent-kill: %v", err)
	}
	if len(runner.kills) != 1 || runner.kills[0] != "nat-session-abcd1234" {
		t.Errorf("kills = %v, want the session tagged for %q killed", runner.kills, tag)
	}
}

// The worktree's current branch is diffed against the working tree — an ad
// hoc session's agent may still have uncommitted changes on it.
func TestSessionDiffIncludesTheWorkingTreeOnTheCheckedOutBranch(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")

	gitCLI := &fakeSessionRepo{currentBranch: "session/one", diffOut: sampleDiff}
	env.NewGit = func() GitCLI { return gitCLI }
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{
		"session-diff", id, "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-diff: %v", err)
	}
	if out.String() != sampleDiff {
		t.Errorf("output = %q, want git's diff verbatim", out.String())
	}
	if gitCLI.diffWorkingTreeCalls != 1 {
		t.Errorf("diffWorkingTreeCalls = %d, want 1", gitCLI.diffWorkingTreeCalls)
	}
	if len(gitCLI.diffFromCalls) != 0 {
		t.Errorf("diffFromCalls = %v, want none: the checked-out branch reads the working tree instead", gitCLI.diffFromCalls)
	}
}

// A named branch that is not the one checked out is diffed at its own tip,
// exactly as slice-diff reads a slice's branch.
func TestSessionDiffOfANonCheckedOutBranchReadsItsTip(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")

	gitCLI := &fakeSessionRepo{currentBranch: "session/one", diffOut: sampleDiff}
	env.NewGit = func() GitCLI { return gitCLI }
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{
		"session-diff", id, "--branch", "session/two", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-diff: %v", err)
	}
	if gitCLI.diffWorkingTreeCalls != 0 {
		t.Errorf("diffWorkingTreeCalls = %d, want 0: session/two is not checked out here", gitCLI.diffWorkingTreeCalls)
	}
	if len(gitCLI.diffFromCalls) != 1 || gitCLI.diffFromCalls[0] != "session/two" {
		t.Errorf("diffFromCalls = %v, want one diff of session/two", gitCLI.diffFromCalls)
	}
}

// The markdown forms are what a person reading a terminal gets when --json
// is left off; each command's own JSON path is already exercised above.
func TestSessionLaunchMarkdown(t *testing.T) {
	env, out := sessionTestEnv(t)
	dir := t.TempDir()
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	env.NewWorktrees = func() actions.Worktrees { return nil }
	env.NewGit = func() GitCLI { return nil }

	if err := Run(context.Background(), []string{
		"session-launch", "--dir", dir, "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-launch: %v", err)
	}
	if !strings.Contains(out.String(), "# Session launched") {
		t.Errorf("markdown output = %q, want a heading", out.String())
	}
	if !strings.Contains(out.String(), dir) {
		t.Errorf("markdown output = %q, want the directory", out.String())
	}
}

func TestSessionListMarkdown(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	launchOneSession(t, env, dir, "")
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	env.NewGH = func() GH { return &fakeSessionGH{} }
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{
		"session-list", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-list: %v", err)
	}
	if !strings.Contains(out.String(), "# Sessions") {
		t.Errorf("markdown output = %q, want a heading", out.String())
	}
}

func TestSessionListMarkdownWithNoSessions(t *testing.T) {
	env, _ := sessionTestEnv(t)
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{
		"session-list", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-list: %v", err)
	}
	if !strings.Contains(out.String(), "No ad hoc sessions") {
		t.Errorf("markdown output = %q, want the empty notice", out.String())
	}
}

// A session's pull requests that could not be refreshed show as stale in
// the listing rather than as having none.
func TestSessionListReportsAStaleReadRatherThanNone(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	seedSession(t, env, dir, "session/one")

	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	env.NewGH = func() GH { return &fakeSessionGH{err: map[string]error{"session/one": errListFailed}} }
	var out strings.Builder
	env.Out = &out
	if err := Run(context.Background(), []string{
		"session-list", "--json", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-list: %v", err)
	}
	var docs []sessionListJSON
	if err := json.Unmarshal([]byte(out.String()), &docs); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out.String())
	}
	if len(docs) != 1 {
		t.Fatalf("session-list = %+v, want one session", docs)
	}
	if !docs[0].PRsStale {
		t.Errorf("PRsStale = false, want true: the read failed")
	}

	var out2 strings.Builder
	env.Out = &out2
	if err := Run(context.Background(), []string{
		"session-list", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-list markdown: %v", err)
	}
	if !strings.Contains(out2.String(), "could not refresh") {
		t.Errorf("markdown output = %q, want a note that the read failed", out2.String())
	}
}

var errListFailed = fmt.Errorf("gh: could not list pull requests")

func TestSessionStatusMarkdown(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")

	env.NewGit = func() GitCLI { return &fakeSessionRepo{currentBranch: "session/one"} }
	env.NewGH = func() GH { return &fakeSessionGH{byBranch: map[string][]gh.HeadPR{
		"session/one": {{Number: 1, Title: "First", State: "OPEN", URL: "https://github.test/x/y/pull/1"}},
	}} }
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	env.NewWorktrees = func() actions.Worktrees { return &fakeSessionWorktrees{} }
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{
		"session-status", id, "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-status: %v", err)
	}
	if !strings.Contains(out.String(), "# Session "+id) {
		t.Errorf("markdown output = %q, want a heading naming the session", out.String())
	}
	if !strings.Contains(out.String(), "#1 First") {
		t.Errorf("markdown output = %q, want the pull request listed", out.String())
	}
}

// A branch that could not be refreshed is reported as such in the markdown
// too, and a branch with genuinely no pull requests says so.
func TestSessionStatusMarkdownStaleAndEmptyBranches(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")

	gitCLI := &fakeSessionRepo{currentBranch: "session/one", reflog: []string{"session/one", "session/two"}}
	env.NewGit = func() GitCLI { return gitCLI }
	env.NewGH = func() GH {
		return &fakeSessionGH{err: map[string]error{"session/one": errListFailed}}
	}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{
		"session-status", id, "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-status: %v", err)
	}
	if !strings.Contains(out.String(), "Could not refresh") {
		t.Errorf("markdown output = %q, want the stale branch reported", out.String())
	}
	if !strings.Contains(out.String(), "No pull requests") {
		t.Errorf("markdown output = %q, want the empty branch reported", out.String())
	}
}

// A session with a live tmux session reads as live regardless of its pull
// requests, and a gone session that was never ended reads as gone.
func TestSessionStatusLiveAndGoneStates(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")
	env.NewGit = func() GitCLI { return &fakeSessionRepo{} }
	env.NewGH = func() GH { return &fakeSessionGH{} }

	tag := agent.SessionTag("project-1", id)
	env.NewTmux = func() *agent.Tmux {
		return agent.NewTmuxWithRunner(&agentTestRunner{liveSessions: map[string]string{tag: "nat-session-abcd1234"}})
	}
	var out strings.Builder
	env.Out = &out
	if err := Run(context.Background(), []string{"session-status", id, "--project", "project-1"}, env); err != nil {
		t.Fatalf("session-status: %v", err)
	}
	if !strings.Contains(out.String(), "State: live") {
		t.Errorf("markdown output = %q, want it reported live", out.String())
	}

	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	var out2 strings.Builder
	env.Out = &out2
	if err := Run(context.Background(), []string{"session-status", id, "--project", "project-1"}, env); err != nil {
		t.Fatalf("session-status: %v", err)
	}
	if !strings.Contains(out2.String(), "State: gone") {
		t.Errorf("markdown output = %q, want it reported gone", out2.String())
	}
}

// --discard ends a session with no open pull requests once its tmux is
// gone, even without any pull request having merged.
func TestSessionStatusDiscardEndsAnIdleSession(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")

	env.NewGit = func() GitCLI { return &fakeSessionRepo{currentBranch: "session/one"} }
	env.NewGH = func() GH {
		return &fakeSessionGH{byBranch: map[string][]gh.HeadPR{
			"session/one": {{Number: 1, Title: "First", State: "CLOSED"}},
		}}
	}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	worktrees := &fakeSessionWorktrees{existingPath: "/repo.worktrees/session-one"}
	env.NewWorktrees = func() actions.Worktrees { return worktrees }
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{
		"session-status", id, "--discard", "--json", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-status --discard: %v", err)
	}
	var doc sessionStatusJSON
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out.String())
	}
	if !doc.Ended {
		t.Fatalf("Ended = false, want --discard to end an idle session with a closed, non-open PR")
	}
	if len(worktrees.removed) != 1 {
		t.Errorf("removed = %v, want the worktree removed", worktrees.removed)
	}
}

// --discard never ends a session while its tmux is still live.
func TestSessionStatusDiscardNeverEndsALiveSession(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")

	env.NewGit = func() GitCLI { return &fakeSessionRepo{} }
	env.NewGH = func() GH { return &fakeSessionGH{} }
	tag := agent.SessionTag("project-1", id)
	env.NewTmux = func() *agent.Tmux {
		return agent.NewTmuxWithRunner(&agentTestRunner{liveSessions: map[string]string{tag: "nat-session-abcd1234"}})
	}
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{
		"session-status", id, "--discard", "--json", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-status --discard: %v", err)
	}
	var doc sessionStatusJSON
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out.String())
	}
	if doc.Ended {
		t.Errorf("Ended = true, want a live session left running")
	}
}

// A read that fails for one branch never ends the session on the strength
// of the others alone.
func TestSessionStatusNeverEndsOnAPartialRead(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")

	gitCLI := &fakeSessionRepo{currentBranch: "session/one", reflog: []string{"session/one", "session/two"}}
	env.NewGit = func() GitCLI { return gitCLI }
	env.NewGH = func() GH {
		return &fakeSessionGH{
			byBranch: map[string][]gh.HeadPR{"session/one": {{Number: 1, State: "MERGED"}}},
			err:      map[string]error{"session/two": errListFailed},
		}
	}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{
		"session-status", id, "--json", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-status: %v", err)
	}
	var doc sessionStatusJSON
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out.String())
	}
	if doc.Ended {
		t.Errorf("Ended = true, want the session left running: one branch's read failed")
	}
}

func TestSessionBranchesFallsBackToTheRecordedBranch(t *testing.T) {
	gitCLI := &fakeSessionRepo{currentErr: errListFailed}
	got := sessionBranches(gitCLI, domainSessionFor("session/recorded"))
	if len(got) != 1 || got[0] != "session/recorded" {
		t.Errorf("sessionBranches = %v, want the recorded branch as a fallback", got)
	}
}

func TestSessionDiffMarkdown(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")
	env.NewGit = func() GitCLI { return &fakeSessionRepo{currentBranch: "session/one", diffOut: sampleDiff} }
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{
		"session-diff", id, "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-diff: %v", err)
	}
	if out.String() != sampleDiff {
		t.Errorf("output = %q, want the raw diff", out.String())
	}
}

func TestSessionDiffJSONOfACheckedOutBranch(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")
	env.NewGit = func() GitCLI { return &fakeSessionRepo{currentBranch: "session/one", diffOut: sampleDiff} }
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{
		"session-diff", id, "--json", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("session-diff: %v", err)
	}
	var doc diffJSON
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out.String())
	}
	if doc.Branch != "session/one" {
		t.Errorf("Branch = %q, want session/one", doc.Branch)
	}
}

func TestSessionDiffRefusesWrongArgumentCount(t *testing.T) {
	env, _ := sessionTestEnv(t)
	if err := Run(context.Background(), []string{"session-diff", "--project", "project-1"}, env); err == nil {
		t.Fatal("session-diff with no session: want a refusal")
	}
}

func TestSessionLaunchRefusesPositionalArguments(t *testing.T) {
	env, _ := sessionTestEnv(t)
	if err := Run(context.Background(), []string{"session-launch", "extra", "--project", "project-1"}, env); err == nil {
		t.Fatal("session-launch with a positional argument: want a refusal")
	}
}

func TestSessionListRefusesPositionalArguments(t *testing.T) {
	env, _ := sessionTestEnv(t)
	if err := Run(context.Background(), []string{"session-list", "extra", "--project", "project-1"}, env); err == nil {
		t.Fatal("session-list with a positional argument: want a refusal")
	}
}

func TestSessionStatusRefusesWrongArgumentCount(t *testing.T) {
	env, _ := sessionTestEnv(t)
	if err := Run(context.Background(), []string{"session-status", "--project", "project-1"}, env); err == nil {
		t.Fatal("session-status with no session: want a refusal")
	}
}

func TestSessionDiffRefusesWithNoRepository(t *testing.T) {
	env, _ := sessionTestEnv(t)
	id := launchOneSession(t, env, t.TempDir(), "")
	// A session launched outside a git repository has no branch at all; its
	// worktree's own "current branch" read answers the same, so there is
	// nothing to name a diff against without --branch.
	env.NewGit = func() GitCLI { return &fakeSessionRepo{} }
	if err := Run(context.Background(), []string{
		"session-diff", id, "--project", "project-1",
	}, env); err == nil {
		t.Fatal("session-diff outside a repository: want a refusal — there is nothing to diff")
	}
}
