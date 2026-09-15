package actions

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/worktree"
)

// launchCall is one session a fakeLauncher was asked to start.
type launchCall struct {
	session, workdir, promptFile, sliceID string
	model                                 config.AgentModel
}

// fakeLauncher stands in for tmux: only the one method Launch itself calls.
type fakeLauncher struct {
	launchErr error
	launches  []launchCall
}

var _ Launcher = (*fakeLauncher)(nil)

func (f *fakeLauncher) Launch(session, workdir, promptFile, sliceID string, model config.AgentModel) error {
	f.launches = append(f.launches, launchCall{session, workdir, promptFile, sliceID, model})
	return f.launchErr
}

// TestLaunchStartsTheAgentInAWorktree covers the ordinary path: a worktree
// cut for the slice's own branch, the claim written before tmux is asked for
// anything, and the session started in the worktree with the prompt file
// naming it.
func TestLaunchStartsTheAgentInAWorktree(t *testing.T) {
	dir := repoDir(t)
	w := &fakeWorktrees{}
	r := &fakeRepo{base: "origin/main"}
	l := &fakeLauncher{}
	client := &fakeClient{getPage: func(id string) (*notion.Page, error) { return todoPage(id, true), nil }}

	res, err := Launch(context.Background(), l, w, r, client.store(), nil, "u1",
		agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view"}, WorkingDir: dir},
		config.AgentModel{Model: "opus", Effort: "high"})

	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	want := filepath.Join(dir+"-worktrees", "slice/info-view")
	if res.Context.WorkingDir != want {
		t.Errorf("workdir = %q, want the worktree at %q", res.Context.WorkingDir, want)
	}
	if res.Context.Branch != "slice/info-view" || res.Context.Repo != dir {
		t.Errorf("context = %+v, want the branch and repo it was placed in", res.Context)
	}
	if res.Session != agent.SessionName("s5") {
		t.Errorf("session = %q, want the slice's own", res.Session)
	}
	if res.Toast != "" {
		t.Errorf("toast = %q, want nothing said about an ordinary launch", res.Toast)
	}
	if len(l.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", l.launches)
	}
	got := l.launches[0]
	if got.session != res.Session || got.workdir != want || got.sliceID != "s5" {
		t.Errorf("launch = %+v, want it started in the worktree", got)
	}
	if prompt, err := os.ReadFile(got.promptFile); err != nil || !strings.Contains(string(prompt), "Info view") {
		t.Errorf("prompt file = %q (err %v), want the slice's own prompt", prompt, err)
	}
	if len(client.updated) != 1 || client.updated[0].pageID != "s5" {
		t.Fatalf("writes = %+v, want exactly the launched slice claimed", client.updated)
	}
}

// TestLaunchFallsBackToTheSharedCheckout covers a working directory that is
// not a repository: the launch still goes ahead, in the directory as it
// stands, with a warning toast saying why there is no worktree.
func TestLaunchFallsBackToTheSharedCheckout(t *testing.T) {
	dir := t.TempDir()
	l := &fakeLauncher{}
	client := &fakeClient{}

	res, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{}, client.store(), nil, "u1",
		agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view"}, WorkingDir: dir},
		config.AgentModel{})

	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	if res.Context.WorkingDir != dir || res.Context.Branch != "" {
		t.Errorf("context = %+v, want the directory as it stands and no branch", res.Context)
	}
	if !strings.Contains(res.Toast, "not a git repository") || res.Sev != SevWarning {
		t.Errorf("toast = %q (sev %v), want a warning naming why", res.Toast, res.Sev)
	}
	if len(l.launches) != 1 {
		t.Errorf("launches = %+v, want the launch to go ahead", l.launches)
	}
}

// TestLaunchRefusesAWorktreeThatCannotBeMade covers git running and refusing:
// nothing is launched, the toast carries git's own reason, and neither the
// claim nor tmux is asked for anything.
func TestLaunchRefusesAWorktreeThatCannotBeMade(t *testing.T) {
	dir := repoDir(t)
	w := &fakeWorktrees{createErr: &worktree.ExitError{Code: 1, Stderr: "the repository has no commits\n"}}
	l := &fakeLauncher{}
	client := &fakeClient{}

	res, err := Launch(context.Background(), l, w, &fakeRepo{base: "origin/main"}, client.store(), nil, "u1",
		agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view"}, WorkingDir: dir},
		config.AgentModel{})

	if err != nil {
		t.Fatalf("Launch() = %v, want a toast rather than a Go error", err)
	}
	if res.Session != "" {
		t.Errorf("session = %q, want nothing launched", res.Session)
	}
	if !strings.Contains(res.Toast, "the repository has no commits") || res.Sev != SevError {
		t.Errorf("toast = %q (sev %v), want git's own reason as an error", res.Toast, res.Sev)
	}
	if len(l.launches) != 0 {
		t.Errorf("launches = %+v, want nothing started", l.launches)
	}
	if len(client.updated) != 0 {
		t.Errorf("wrote %+v, want the claim never reached", client.updated)
	}
}

// TestLaunchReportsAFailedBriefRead covers the slice's own body refusing to
// read after the claim has gone through: nothing is launched, and the claim
// stands, since the claim is what makes the brief worth reading in the first
// place.
func TestLaunchReportsAFailedBriefRead(t *testing.T) {
	l := &fakeLauncher{}
	client := &fakeClient{
		getPage: func(id string) (*notion.Page, error) { return todoPage(id, true), nil },
		blocks:  func(string) ([]notion.Block, error) { return nil, errors.New("notion: 500") },
	}

	_, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{}, client.store(), nil, "u1",
		agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view"}, WorkingDir: t.TempDir()},
		config.AgentModel{})

	if err == nil || !strings.Contains(err.Error(), `claimed "Info view" but could not read its brief: notion: 500`) {
		t.Errorf("err = %v, want the brief's read failure named", err)
	}
	if len(l.launches) != 0 {
		t.Error("no session should start without a brief to seed it")
	}
}

// TestLaunchReportsAFailedConventionsRead covers the project's own body
// refusing to read: the slice's own brief came back fine, but the launch
// still stops rather than writing a prompt with half the document missing.
func TestLaunchReportsAFailedConventionsRead(t *testing.T) {
	l := &fakeLauncher{}
	client := &fakeClient{
		getPage: func(id string) (*notion.Page, error) { return todoPage(id, true), nil },
		blocks: func(id string) ([]notion.Block, error) {
			if id == "p1" {
				return nil, errors.New("notion: 500")
			}
			return nil, nil
		},
	}

	_, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{}, client.store(), nil, "u1",
		agent.PromptContext{
			Slice: domain.Slice{ID: "s5", Name: "Info view"}, ProjectID: "p1", WorkingDir: t.TempDir(),
		}, config.AgentModel{})

	if err == nil || !strings.Contains(err.Error(), `claimed "Info view" but could not read the project conventions: notion: 500`) {
		t.Errorf("err = %v, want the conventions' read failure named", err)
	}
	if len(l.launches) != 0 {
		t.Error("no session should start without the project conventions to seed it")
	}
}

// TestLaunchIncludesAMilestoneDigest covers a launch given its milestone and
// the siblings under it: the digest — each sibling's status, and the
// hand-back summary of the Done one — lands in the prompt file, alongside
// the brief and the conventions.
func TestLaunchIncludesAMilestoneDigest(t *testing.T) {
	l := &fakeLauncher{}
	client := &fakeClient{
		getPage: func(id string) (*notion.Page, error) { return todoPage(id, true), nil },
		blocks: func(id string) ([]notion.Block, error) {
			if id == "s2" {
				return []notion.Block{
					block(t, "heading_3", "Handed back"),
					block(t, "paragraph", "Laid out the columns."),
				}, nil
			}
			return nil, nil
		},
	}

	res, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{base: "origin/main"}, client.store(), nil, "u1",
		agent.PromptContext{
			Slice:      domain.Slice{ID: "s5", Name: "Info view"},
			WorkingDir: t.TempDir(),
			Milestone:  domain.Milestone{ID: "M1", Name: "M1: Board"},
			MilestoneSlices: []domain.Slice{
				{ID: "s2", Name: "Board scaffolding", Status: domain.SliceDone, StatusName: "Done"},
				{ID: "s4", Name: "Style the board", Status: domain.SliceTodo, StatusName: "Todo"},
			},
		},
		config.AgentModel{})

	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	if res.Context.MilestoneDigest == "" {
		t.Fatal("result carries no milestone digest")
	}
	prompt, err := os.ReadFile(l.launches[0].promptFile)
	if err != nil {
		t.Fatalf("read the prompt file: %v", err)
	}
	for _, want := range []string{"M1: Board", "- Done: Board scaffolding", "Laid out the columns.", "- Todo: Style the board"} {
		if !strings.Contains(string(prompt), want) {
			t.Errorf("prompt file does not carry the milestone digest — missing %q:\n%s", want, prompt)
		}
	}
}

// TestLaunchLogsAFailedMilestoneSummaryRead covers a Done sibling whose body
// fails to read: the launch still goes ahead, with that sibling's summary
// simply missing from the digest rather than the whole launch failing over
// one page.
func TestLaunchLogsAFailedMilestoneSummaryRead(t *testing.T) {
	l := &fakeLauncher{}
	client := &fakeClient{
		getPage: func(id string) (*notion.Page, error) { return todoPage(id, true), nil },
		blocks: func(id string) ([]notion.Block, error) {
			if id == "s2" {
				return nil, errors.New("notion: 500")
			}
			return nil, nil
		},
	}

	_, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{base: "origin/main"}, client.store(), nil, "u1",
		agent.PromptContext{
			Slice:      domain.Slice{ID: "s5", Name: "Info view"},
			WorkingDir: t.TempDir(),
			Milestone:  domain.Milestone{ID: "M1", Name: "M1: Board"},
			MilestoneSlices: []domain.Slice{
				{ID: "s2", Name: "Board scaffolding", Status: domain.SliceDone, StatusName: "Done"},
			},
		},
		config.AgentModel{})

	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through despite the failed read", err)
	}
	prompt, err := os.ReadFile(l.launches[0].promptFile)
	if err != nil {
		t.Fatalf("read the prompt file: %v", err)
	}
	if !strings.Contains(string(prompt), "- Done: Board scaffolding") {
		t.Errorf("prompt file does not name the sibling despite its summary failing to read:\n%s", prompt)
	}
}

// TestLaunchReportsAFailedPromptFile covers the prompt file itself failing to
// write: the claim and the brief it is written with have already happened by
// then, since fetching the brief needs the claim to have gone through first.
func TestLaunchReportsAFailedPromptFile(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "not-there"))
	l := &fakeLauncher{}
	client := &fakeClient{}

	_, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{base: "origin/main"}, client.store(), nil, "u1",
		agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view"}},
		config.AgentModel{})

	if err == nil || !strings.Contains(err.Error(), "launch agent: create prompt dir") {
		t.Errorf("err = %v, want the failed prompt file", err)
	}
	if len(l.launches) != 0 {
		t.Error("no session should start without a prompt to seed it")
	}
	if len(client.updated) != 1 {
		t.Errorf("wrote %+v, want the slice claimed", client.updated)
	}
}

// TestLaunchRefusesWithoutTheClaim covers Notion refusing the claim, either
// on the read or the write: no session starts, and the toast is what
// launchAgent reports rather than a Go error, since nothing has gone wrong
// with the board and the slice is still there to launch.
func TestLaunchRefusesWithoutTheClaim(t *testing.T) {
	tests := []struct {
		name string
		fail func(*fakeClient)
	}{
		{"the read", func(c *fakeClient) {
			c.getPage = func(string) (*notion.Page, error) { return nil, errors.New("notion: 500") }
		}},
		{"the write", func(c *fakeClient) {
			c.updatePage = func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
				return nil, errors.New("notion: 500")
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeClient{}
			tt.fail(client)
			l := &fakeLauncher{}

			res, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{base: "origin/main"}, client.store(), nil, "u1",
				agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view"}},
				config.AgentModel{})

			if err != nil {
				t.Fatalf("Launch() = %v, want the refusal said as a toast", err)
			}
			if res.Session != "" {
				t.Errorf("session = %q, want nothing launched", res.Session)
			}
			want := `Could not claim "Info view": notion: 500 — no agent was launched.`
			if res.Toast != want {
				t.Errorf("toast = %q, want %q", res.Toast, want)
			}
			if res.Sev != SevError {
				t.Errorf("severity = %v, want an error", res.Sev)
			}
			if len(l.launches) != 0 {
				t.Errorf("launched %+v, want nothing without the claim", l.launches)
			}
		})
	}
}

// TestLaunchReportsAFailedStart covers tmux itself refusing: a Go error, since
// the claim has already landed and something needs to be said louder than a
// toast.
func TestLaunchReportsAFailedStart(t *testing.T) {
	l := &fakeLauncher{launchErr: errors.New("duplicate session")}
	client := &fakeClient{}

	_, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{base: "origin/main"}, client.store(), nil, "u1",
		agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view"}, WorkingDir: t.TempDir()},
		config.AgentModel{})

	if err == nil || !strings.Contains(err.Error(), "duplicate session") {
		t.Errorf("err = %v, want the failed launch", err)
	}
}

func TestWorkdirFor(t *testing.T) {
	project := config.ProjectConfig{WorkingDir: "/Users/craig/Projects/tracker"}
	tests := []struct {
		name  string
		slice domain.Slice
		want  string
	}{
		{"project default", domain.Slice{}, "/Users/craig/Projects/tracker"},
		{"slice override", domain.Slice{Repo: "~/Projects/other"}, "~/Projects/other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WorkdirFor(tt.slice, project); got != tt.want {
				t.Errorf("WorkdirFor = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTrimModel(t *testing.T) {
	got := TrimModel(config.AgentModel{Model: " opus ", Effort: " high "})
	if want := (config.AgentModel{Model: "opus", Effort: "high"}); got != want {
		t.Errorf("TrimModel() = %+v, want %+v", got, want)
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
		want string
	}{
		{"bare tilde", "~", home},
		{"under home", "~/Projects/x", filepath.Join(home, "Projects", "x")},
		{"absolute path", "/tmp/x", "/tmp/x"},
		{"relative path", "Projects/x", "Projects/x"},
		{"another user's home is left alone", "~craig/x", "~craig/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExpandHome(tt.path); got != tt.want {
				t.Errorf("ExpandHome(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// fakeReviewer stands in for gh's two fix-launch reads.
type fakeReviewer struct {
	comments, checks       string
	commentsErr, checksErr error
}

var _ PRReviewReader = (*fakeReviewer)(nil)

func (f *fakeReviewer) ReviewComments(dir, ref string) (string, error) {
	return f.comments, f.commentsErr
}
func (f *fakeReviewer) Checks(dir, ref string) (string, error) { return f.checks, f.checksErr }

// TestLaunchGathersTheGitSnapshotForAResumingLaunch covers a relaunch onto a
// branch the slice already records: the worktree is placed on it, so the
// commit log and diff stat are worth reading, and both come back on the
// context the prompt renders from.
func TestLaunchGathersTheGitSnapshotForAResumingLaunch(t *testing.T) {
	dir := repoDir(t)
	worktreeDir := dir + "-worktrees/slice/info-view"
	w := &fakeWorktrees{existing: map[string]string{"slice/info-view": worktreeDir}}
	r := &fakeRepo{base: "origin/main", log: "abc1234 did the thing", stat: "a.go | 2 ++"}
	l := &fakeLauncher{}
	client := &fakeClient{}

	res, err := Launch(context.Background(), l, w, r, client.store(), nil, "u1",
		agent.PromptContext{
			Slice:      domain.Slice{ID: "s5", Name: "Info view", Branch: "slice/info-view", Status: domain.SliceClaimed},
			WorkingDir: dir,
		}, config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	if res.Context.GitBase != "origin/main" || res.Context.GitLog != "abc1234 did the thing" || res.Context.GitDiffStat != "a.go | 2 ++" {
		t.Errorf("context = %+v, want the gathered git snapshot", res.Context)
	}
}

// A first-time launch — nothing yet on the branch — never gathers git at
// all: there is nothing there worth reading, and the prompt is told so
// separately (Claude Code's own injected snapshot).
func TestLaunchNeverGathersGitForAFirstTimeLaunch(t *testing.T) {
	dir := repoDir(t)
	w := &fakeWorktrees{}
	r := &fakeRepo{base: "origin/main", log: "should not appear", stat: "should not appear"}
	l := &fakeLauncher{}
	client := &fakeClient{getPage: func(id string) (*notion.Page, error) { return todoPage(id, true), nil }}

	res, err := Launch(context.Background(), l, w, r, client.store(), nil, "u1",
		agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view"}, WorkingDir: dir},
		config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	if res.Context.GitLog != "" || res.Context.GitDiffStat != "" {
		t.Errorf("context = %+v, want no git gathered for a first-time launch", res.Context)
	}
}

// A gather that fails on one read still tries the other, and leaves only the
// failed one empty — the project's usual reads-conclude-nothing posture.
func TestLaunchLeavesTheGitSnapshotEmptyOnAFailedRead(t *testing.T) {
	dir := repoDir(t)
	worktreeDir := dir + "-worktrees/slice/info-view"
	w := &fakeWorktrees{existing: map[string]string{"slice/info-view": worktreeDir}}
	r := &fakeRepo{base: "origin/main", stat: "a.go | 2 ++"}
	r.log = "" // exercised via the LogOneline error path below
	client := &fakeClient{}

	res, err := Launch(context.Background(), &fakeLauncher{}, w, &loggingErrRepo{fakeRepo: r}, client.store(), nil, "u1",
		agent.PromptContext{
			Slice:      domain.Slice{ID: "s5", Name: "Info view", Branch: "slice/info-view", Status: domain.SliceClaimed},
			WorkingDir: dir,
		}, config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	if res.Context.GitLog != "" {
		t.Errorf("log = %q, want it empty after a failed read", res.Context.GitLog)
	}
	if res.Context.GitDiffStat != "a.go | 2 ++" {
		t.Errorf("diff stat = %q, want the other read to still succeed", res.Context.GitDiffStat)
	}
}

// loggingErrRepo fails LogOneline alone, so a test can drive one half of
// gitSnapshot's failure without the other.
type loggingErrRepo struct{ *fakeRepo }

func (r *loggingErrRepo) LogOneline(dir, base, branch string) (string, error) {
	return "", errors.New("git: unknown revision")
}

// diffStatErrRepo fails DiffStat alone, the mirror of loggingErrRepo.
type diffStatErrRepo struct{ *fakeRepo }

func (r *diffStatErrRepo) DiffStat(dir, base, branch string) (string, error) {
	return "", errors.New("git: unknown revision")
}

// The diff stat read failing leaves it empty while the commit log still
// comes back, the other half of TestLaunchLeavesTheGitSnapshotEmptyOnAFailedRead.
func TestLaunchLeavesTheDiffStatEmptyOnAFailedRead(t *testing.T) {
	dir := repoDir(t)
	worktreeDir := dir + "-worktrees/slice/info-view"
	w := &fakeWorktrees{existing: map[string]string{"slice/info-view": worktreeDir}}
	r := &fakeRepo{base: "origin/main", log: "abc1234 did the thing"}
	client := &fakeClient{}

	res, err := Launch(context.Background(), &fakeLauncher{}, w, &diffStatErrRepo{fakeRepo: r}, client.store(), nil, "u1",
		agent.PromptContext{
			Slice:      domain.Slice{ID: "s5", Name: "Info view", Branch: "slice/info-view", Status: domain.SliceClaimed},
			WorkingDir: dir,
		}, config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	if res.Context.GitLog != "abc1234 did the thing" {
		t.Errorf("log = %q, want the other read to still succeed", res.Context.GitLog)
	}
	if res.Context.GitDiffStat != "" {
		t.Errorf("diff stat = %q, want it empty after a failed read", res.Context.GitDiffStat)
	}
}

// A fix launch claims nothing and gathers the review instead: the gh reads
// come back on the context, and no claim is written.
func TestLaunchGathersTheReviewForAFixLaunch(t *testing.T) {
	dir := repoDir(t)
	l := &fakeLauncher{}
	client := &fakeClient{}
	reviewer := &fakeReviewer{comments: "craig: nit on naming", checks: "X build 1m"}

	res, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{base: "origin/main"}, client.store(),
		reviewer, "u1",
		agent.PromptContext{
			Slice:      domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceDone, PRURL: "https://example/pr/1"},
			WorkingDir: dir, Fix: true,
		}, config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	if res.Context.ReviewComments != "craig: nit on naming" || res.Context.ReviewChecks != "X build 1m" {
		t.Errorf("context = %+v, want the gathered review", res.Context)
	}
	if len(client.updated) != 0 {
		t.Errorf("wrote %+v, want a fix launch to claim nothing", client.updated)
	}
}

// A nil viewer — nothing headless ever drives a fix launch with one — gathers
// nothing rather than panicking.
func TestLaunchReviewGatherToleratesANilViewer(t *testing.T) {
	dir := repoDir(t)
	client := &fakeClient{}

	res, err := Launch(context.Background(), &fakeLauncher{}, &fakeWorktrees{}, &fakeRepo{base: "origin/main"},
		client.store(), nil, "u1",
		agent.PromptContext{
			Slice:      domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceDone, PRURL: "https://example/pr/1"},
			WorkingDir: dir, Fix: true,
		}, config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	if res.Context.ReviewComments != "" || res.Context.ReviewChecks != "" {
		t.Errorf("context = %+v, want nothing gathered with no viewer", res.Context)
	}
}

// A gh read that fails leaves just that half of the review empty.
func TestLaunchLeavesTheReviewEmptyOnAFailedRead(t *testing.T) {
	dir := repoDir(t)
	client := &fakeClient{}
	reviewer := &fakeReviewer{checks: "X build 1m", commentsErr: errors.New("gh: not authenticated")}

	res, err := Launch(context.Background(), &fakeLauncher{}, &fakeWorktrees{}, &fakeRepo{base: "origin/main"},
		client.store(), reviewer, "u1",
		agent.PromptContext{
			Slice:      domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceDone, PRURL: "https://example/pr/1"},
			WorkingDir: dir, Fix: true,
		}, config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	if res.Context.ReviewComments != "" {
		t.Errorf("comments = %q, want empty after a failed read", res.Context.ReviewComments)
	}
	if res.Context.ReviewChecks != "X build 1m" {
		t.Errorf("checks = %q, want the other read to still succeed", res.Context.ReviewChecks)
	}
}

// The other half of TestLaunchLeavesTheReviewEmptyOnAFailedRead: a failed
// checks read leaves it empty while the comments still come back.
func TestLaunchLeavesTheChecksEmptyOnAFailedRead(t *testing.T) {
	dir := repoDir(t)
	client := &fakeClient{}
	reviewer := &fakeReviewer{comments: "craig: nit on naming", checksErr: errors.New("gh: not authenticated")}

	res, err := Launch(context.Background(), &fakeLauncher{}, &fakeWorktrees{}, &fakeRepo{base: "origin/main"},
		client.store(), reviewer, "u1",
		agent.PromptContext{
			Slice:      domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceDone, PRURL: "https://example/pr/1"},
			WorkingDir: dir, Fix: true,
		}, config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	if res.Context.ReviewComments != "craig: nit on naming" {
		t.Errorf("comments = %q, want the other read to still succeed", res.Context.ReviewComments)
	}
	if res.Context.ReviewChecks != "" {
		t.Errorf("checks = %q, want it empty after a failed read", res.Context.ReviewChecks)
	}
}

func TestExpandHomeWithoutAHomeDirectory(t *testing.T) {
	t.Setenv("HOME", "")
	if got := ExpandHome("~/x"); got != "~/x" {
		t.Errorf("ExpandHome = %q, want it untouched", got)
	}
}

func TestExistingDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
		want string
	}{
		{"a directory", " " + dir + " ", ""},
		{"blank", "  ", "the agent needs a working directory"},
		{"missing", filepath.Join(dir, "nope"), "is not there"},
		{"a file", file, "is not a directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ExistingDir(tt.path)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("ExistingDir(%q) = %v, want it accepted", tt.path, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ExistingDir(%q) = %v, want %q", tt.path, err, tt.want)
			}
		})
	}
}
