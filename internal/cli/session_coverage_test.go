package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/store"
)

// seedHydratedProject writes a plan file directly, already marked as pulled
// from a workspace, with no slice at all — the session commands touch no
// slice — so a test can corrupt exactly the sessions table without also
// tripping store.ForProject's own hydrate.
func seedHydratedProject(t *testing.T, projectID string, breakIt func(db *sql.DB)) {
	t.Helper()
	path, err := store.LocalPath(projectID)
	if err != nil {
		t.Fatalf("resolve the plan path: %v", err)
	}
	l, err := store.OpenLocal(path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close the plan after creating it: %v", err)
	}

	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open the plan to seed it: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the seeding connection: %v", err)
		}
	}()
	if _, err := db.Exec(`INSERT INTO project (id, name, synced_at, has_assignee, has_branch) VALUES (?, ?, ?, 1, 1)`,
		projectID, "nat", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed the project: %v", err)
	}
	if breakIt != nil {
		breakIt(db)
	}
}

// dropSessionsTable is the [seedHydratedProject] break used by every test
// that wants a session read or write to fail underneath the store, exactly
// as milestones/sync tables are dropped in launch_test.go for the same
// reason: by the time these commands run, nothing about an unreachable
// workspace touches a purely local table.
func dropSessionsTable(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`DROP TABLE sessions`); err != nil {
		t.Fatalf("break the plan's sessions table: %v", err)
	}
}

// blockSessionUpdates lets a read of the sessions table succeed while any
// write to it fails — a trigger rather than dropping the table outright,
// since [sessionStatus]'s own EndSession write happens only after its own
// read of the same table has already succeeded.
func blockSessionUpdates(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`CREATE TRIGGER block_session_updates BEFORE UPDATE ON sessions
		BEGIN SELECT RAISE(ABORT, 'blocked'); END`); err != nil {
		t.Fatalf("block session updates: %v", err)
	}
}

func badFlagTests(t *testing.T, command, id string) {
	t.Helper()
	env, _ := sessionTestEnv(t)
	args := []string{command}
	if id != "" {
		args = append(args, id)
	}
	args = append(args, "--bogus", "--project", "project-1")
	err := Run(context.Background(), args, env)
	if err == nil || !strings.Contains(err.Error(), "not defined") {
		t.Errorf("%s --bogus: err = %v, want 'not defined'", command, err)
	}
}

func TestSessionCommandsRefuseUnknownFlags(t *testing.T) {
	badFlagTests(t, "session-launch", "")
	badFlagTests(t, "session-list", "")
	badFlagTests(t, "session-status", testSessionUUID)
	badFlagTests(t, "session-diff", testSessionUUID)
}

const testSessionUUID = "8f654180-9b8d-53fb-1024-9ee08f654180"

func TestSessionCommandsRefuseAnUnknownProject(t *testing.T) {
	env, _ := sessionTestEnv(t)
	tests := []struct {
		name string
		args []string
	}{
		{"session-launch", []string{"session-launch", "--project", "nope"}},
		{"session-list", []string{"session-list", "--project", "nope"}},
		{"session-status", []string{"session-status", testSessionUUID, "--project", "nope"}},
		{"session-diff", []string{"session-diff", testSessionUUID, "--project", "nope"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Run(context.Background(), tt.args, env)
			if err == nil || !strings.Contains(err.Error(), "no project") {
				t.Errorf("%s: err = %v, want the unknown project named", tt.name, err)
			}
		})
	}
}

func TestSessionCommandsFailWhenThePlanCannotBeHydrated(t *testing.T) {
	api := &fakeAPI{queryErr: map[string]error{"slices-ds": errors.New("notion: 500")}}
	tests := []struct {
		name string
		args []string
	}{
		{"session-launch", []string{"session-launch", "--dir", t.TempDir(), "--project", "project-1"}},
		{"session-list", []string{"session-list", "--project", "project-1"}},
		{"session-status", []string{"session-status", testSessionUUID, "--project", "project-1"}},
		{"session-diff", []string{"session-diff", testSessionUUID, "--project", "project-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, _ := testEnv(testClaimConfig(t), api)
			err := Run(context.Background(), tt.args, env)
			if err == nil || !strings.Contains(err.Error(), "notion: 500") {
				t.Errorf("%s: err = %v, want the failed hydrate reported", tt.name, err)
			}
		})
	}
}

func TestSessionStatusAndDiffRefuseAnInvalidID(t *testing.T) {
	env, _ := sessionTestEnv(t)
	if err := Run(context.Background(), []string{"session-status", "not-a-uuid", "--project", "project-1"}, env); err == nil {
		t.Error("session-status with an invalid ID: want a refusal")
	}
	if err := Run(context.Background(), []string{"session-diff", "not-a-uuid", "--project", "project-1"}, env); err == nil {
		t.Error("session-diff with an invalid ID: want a refusal")
	}
}

func TestSessionDiffRefusesAnUnknownSession(t *testing.T) {
	env, _ := sessionTestEnv(t)
	if err := Run(context.Background(), []string{"session-diff", testSessionUUID, "--project", "project-1"}, env); err == nil {
		t.Error("session-diff of an unknown session: want a refusal")
	}
}

func TestSessionListAndStatusAndDiffReportAFailedSessionsRead(t *testing.T) {
	env, _ := sessionTestEnv(t)
	env.NewGit = func() GitCLI { return &fakeSessionRepo{} }
	seedHydratedProject(t, "project-1", func(db *sql.DB) { dropSessionsTable(t, db) })
	tests := [][]string{
		{"session-list", "--project", "project-1"},
		{"session-status", testSessionUUID, "--project", "project-1"},
		{"session-diff", testSessionUUID, "--project", "project-1"},
	}
	for _, args := range tests {
		if err := Run(context.Background(), args, env); err == nil || !strings.Contains(err.Error(), "read the sessions") {
			t.Errorf("%v: err = %v, want 'read the sessions'", args, err)
		}
	}
}

func TestSessionListAndStatusReportAFailedLiveTmuxRead(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")
	runner := &agentTestRunner{liveFatalErr: "tmux: no such socket"}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	env.NewGit = func() GitCLI { return &fakeSessionRepo{} }
	env.NewGH = func() GH { return &fakeSessionGH{} }

	if err := Run(context.Background(), []string{"session-list", "--project", "project-1"}, env); err == nil {
		t.Error("session-list with a broken tmux read: want an error")
	}
	if err := Run(context.Background(), []string{"session-status", id, "--project", "project-1"}, env); err == nil {
		t.Error("session-status with a broken tmux read: want an error")
	}
}

// The worktree cut on the fresh session/<id> branch reused for a real repo
// launch, but with no --json this time, to cover the markdown branches
// [sessionLaunchMarkdown] takes for a branch with no warning at all.
func TestSessionLaunchMarkdownInARepoNamesTheBranch(t *testing.T) {
	env, out := sessionTestEnv(t)
	dir := repoDir(t)
	worktrees := &fakeSessionWorktrees{}
	env.NewWorktrees = func() actions.Worktrees { return worktrees }
	env.NewGit = func() GitCLI { return &fakeSessionRepo{base: "origin/main"} }
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }

	if err := Run(context.Background(), []string{"session-launch", "--dir", dir, "--project", "project-1"}, env); err != nil {
		t.Fatalf("session-launch: %v", err)
	}
	if !strings.Contains(out.String(), "- Branch: session/") {
		t.Errorf("markdown output = %q, want the branch named", out.String())
	}
	if strings.Contains(out.String(), "Warning:") {
		t.Errorf("markdown output = %q, want no warning: the worktree was cut cleanly", out.String())
	}
}

// repoDir is a directory that looks enough like a git checkout for
// [actions.InRepo] — the same fixture internal/actions' own tests use.
func repoDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSessionLaunchRefusesAWorktreeFailure(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := repoDir(t)
	env.NewWorktrees = func() actions.Worktrees { return &fakeSessionWorktrees{createErr: errors.New("no commits")} }
	env.NewGit = func() GitCLI { return &fakeSessionRepo{} }
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }

	err := Run(context.Background(), []string{"session-launch", "--dir", dir, "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("session-launch with a worktree failure: want a refusal")
	}
}

func TestSessionLaunchReportsATmuxFailure(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	env.NewWorktrees = func() actions.Worktrees { return nil }
	env.NewGit = func() GitCLI { return nil }
	env.NewTmux = func() *agent.Tmux {
		return agent.NewTmuxWithRunner(&agentTestRunner{launchErr: "duplicate session"})
	}

	err := Run(context.Background(), []string{"session-launch", "--dir", dir, "--project", "project-1"}, env)
	if err == nil {
		t.Fatal("session-launch with a tmux failure: want a refusal")
	}
}

// A session's write fails underneath an already-launched agent — the file's
// sessions table is gone by the second launch, its schema already stamped
// from the first.
func TestSessionLaunchReportsAFailedRecord(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	env.NewWorktrees = func() actions.Worktrees { return nil }
	env.NewGit = func() GitCLI { return nil }
	launchOneSession(t, env, dir, "")

	path, err := store.LocalPath("project-1")
	if err != nil {
		t.Fatalf("resolve the plan path: %v", err)
	}
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open the plan to break it: %v", err)
	}
	dropSessionsTable(t, db)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if err := Run(context.Background(), []string{
		"session-launch", "--dir", dir, "--project", "project-1",
	}, env); err == nil || !strings.Contains(err.Error(), "could not record the session") {
		t.Errorf("err = %v, want the failed record reported", err)
	}
}

// The default working directory is the project's own, when --dir is left
// off entirely.
func TestSessionLaunchDefaultsToTheProjectsWorkingDir(t *testing.T) {
	dir := t.TempDir()
	cfg := testClaimConfig(t)
	cfg.Projects["project-1"] = config.ProjectConfig{Name: "nat", SlicesDSID: "slices-ds", WorkingDir: dir}
	env, out := testEnv(cfg, &fakeAPI{})
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	env.NewWorktrees = func() actions.Worktrees { return nil }
	env.NewGit = func() GitCLI { return nil }

	if err := Run(context.Background(), []string{"session-launch", "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("session-launch: %v", err)
	}
	var doc sessionLaunchJSON
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out.String())
	}
	if doc.Dir != dir {
		t.Errorf("Dir = %q, want the project's own working directory %q", doc.Dir, dir)
	}
}

func TestSessionListReadsSuccessfulPullRequests(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	seedSession(t, env, dir, "session/one")
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	env.NewGH = func() GH {
		return &fakeSessionGH{byBranch: map[string][]gh.HeadPR{
			"session/one": {{Number: 1, Title: "First", State: "OPEN"}},
		}}
	}
	var out strings.Builder
	env.Out = &out
	if err := Run(context.Background(), []string{"session-list", "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("session-list: %v", err)
	}
	var docs []sessionListJSON
	if err := json.Unmarshal([]byte(out.String()), &docs); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out.String())
	}
	if len(docs) != 1 || len(docs[0].PRs) != 1 || docs[0].PRs[0].Number != 1 {
		t.Fatalf("session-list = %+v, want the one pull request", docs)
	}
	if docs[0].PRsStale {
		t.Error("PRsStale = true, want a successful read")
	}
}

// Every combination of open/merged/closed and live/ended/gone, so
// sessionListMarkdown and prSummary both run every branch they have.
func TestSessionListMarkdownEveryState(t *testing.T) {
	env, _ := sessionTestEnv(t)
	live := seedSession(t, env, t.TempDir(), "session/live")
	ended := seedSession(t, env, t.TempDir(), "session/ended")
	gone := seedSession(t, env, t.TempDir(), "session/gone")

	ctx := context.Background()
	_, projectID, project, err := env.projectFor("project-1")
	if err != nil {
		t.Fatalf("projectFor: %v", err)
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		t.Fatalf("storeFor: %v", err)
	}
	if err := st.EndSession(ctx, ended); err != nil {
		t.Fatalf("EndSession: %v", err)
	}

	tag := agent.SessionTag("project-1", live)
	env.NewTmux = func() *agent.Tmux {
		return agent.NewTmuxWithRunner(&agentTestRunner{liveSessions: map[string]string{tag: "nat-session-x"}})
	}
	env.NewGH = func() GH {
		return &fakeSessionGH{byBranch: map[string][]gh.HeadPR{
			"session/live":  {{Number: 1, State: "OPEN"}, {Number: 2, State: "MERGED"}, {Number: 3, State: "CLOSED"}},
			"session/ended": {{Number: 4, State: "MERGED"}},
			"session/gone":  nil,
		}}
	}
	var out strings.Builder
	env.Out = &out
	if err := Run(context.Background(), []string{"session-list", "--project", "project-1"}, env); err != nil {
		t.Fatalf("session-list: %v", err)
	}
	md := out.String()
	for _, want := range []string{"live: nat-session-x", "ended", "gone", "1 open", "1 merged", "1 closed"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown output = %q, want it to contain %q", md, want)
		}
	}
	if !strings.Contains(gone, "-") { // silence unused warnings if reordered
		_ = gone
	}
}

func TestSessionStatusReflogFailureIsLoggedAndSkipped(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")

	gitCLI := &fakeSessionRepo{currentBranch: "session/one", reflogErr: errListFailed}
	env.NewGit = func() GitCLI { return gitCLI }
	env.NewGH = func() GH { return &fakeSessionGH{byBranch: map[string][]gh.HeadPR{"session/one": nil}} }
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{"session-status", id, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("session-status: %v", err)
	}
	var doc sessionStatusJSON
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out.String())
	}
	if len(doc.Branches) != 1 || doc.Branches[0].Branch != "session/one" {
		t.Errorf("Branches = %+v, want just the current branch despite the reflog failure", doc.Branches)
	}
}

// A session that is already ended, from an earlier call, still reads as
// ended on a later call that itself has nothing to end.
func TestSessionStatusReportsAnAlreadyEndedSessionEvenWithoutEndingItAgain(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")

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

	env.NewGit = func() GitCLI { return &fakeSessionRepo{} }
	env.NewGH = func() GH { return &fakeSessionGH{} }
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	var out strings.Builder
	env.Out = &out
	if err := Run(context.Background(), []string{"session-status", id, "--project", "project-1"}, env); err != nil {
		t.Fatalf("session-status: %v", err)
	}
	if !strings.Contains(out.String(), "State: ended") {
		t.Errorf("markdown output = %q, want it reported ended", out.String())
	}
}

func TestSessionStatusReportsAFailedEnd(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")

	path, err := store.LocalPath("project-1")
	if err != nil {
		t.Fatalf("resolve the plan path: %v", err)
	}
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open the plan to break it: %v", err)
	}
	blockSessionUpdates(t, db)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	env.NewGit = func() GitCLI { return &fakeSessionRepo{currentBranch: "session/one"} }
	env.NewGH = func() GH {
		return &fakeSessionGH{byBranch: map[string][]gh.HeadPR{"session/one": {{Number: 1, State: "MERGED"}}}}
	}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	env.NewWorktrees = func() actions.Worktrees { return &fakeSessionWorktrees{} }

	err = Run(context.Background(), []string{"session-status", id, "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "end the session") {
		t.Errorf("err = %v, want the failed end reported", err)
	}
}

func TestSessionDiffWorkingTreeFailure(t *testing.T) {
	env, _ := sessionTestEnv(t)
	dir := t.TempDir()
	id := launchOneSession(t, env, dir, "")
	env.NewGit = func() GitCLI {
		return &fakeSessionRepo{currentBranch: "session/one", diffWorkingTreeErr: errListFailed}
	}
	if err := Run(context.Background(), []string{"session-diff", id, "--project", "project-1"}, env); err == nil {
		t.Fatal("session-diff with a failed working tree read: want an error")
	}
}
