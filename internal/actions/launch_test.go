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
	"github.com/craigmjohnston/nat/internal/source"
	"github.com/craigmjohnston/nat/internal/worktree"
)

// TestMain pins the state dir, where every launch writes its agent's brief, so
// a test launch never leaves one in the real state dir of whoever runs the
// suite.
func TestMain(m *testing.M) {
	state, err := os.MkdirTemp("", "nat-actions-test-state-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("HOME", state)
	_ = os.Setenv("XDG_STATE_HOME", state)
	code := m.Run()
	_ = os.RemoveAll(state)
	os.Exit(code)
}

// launchCall is one session a fakeLauncher was asked to start.
type launchCall struct {
	session, workdir, promptFile, opening, sliceID, projectID string
	model                                                     config.AgentModel
}

// fakeLauncher stands in for tmux: only the two methods Launch itself calls.
// A resumed launch is recorded in launches too, its resumption beside it in
// resumes.
type fakeLauncher struct {
	launchErr error
	launches  []launchCall
	resumes   []agent.Resumption
}

var _ Launcher = (*fakeLauncher)(nil)

func (f *fakeLauncher) Launch(session, workdir, promptFile, opening, sliceID, projectID string, model config.AgentModel) error {
	f.launches = append(f.launches, launchCall{session, workdir, promptFile, opening, sliceID, projectID, model})
	return f.launchErr
}

func (f *fakeLauncher) LaunchResumed(session, workdir, promptFile, opening, sliceID, projectID string, r agent.Resumption, model config.AgentModel) error {
	f.resumes = append(f.resumes, r)
	return f.Launch(session, workdir, promptFile, opening, sliceID, projectID, model)
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
		agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view"}, ProjectID: "p1", WorkingDir: dir},
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
	if got.session != res.Session || got.workdir != want || got.sliceID != "s5" || got.projectID != "p1" {
		t.Errorf("launch = %+v, want it started in the worktree", got)
	}
	if prompt, err := os.ReadFile(got.promptFile); err != nil || !strings.Contains(string(prompt), "Info view") {
		t.Errorf("prompt file = %q (err %v), want the slice's own prompt", prompt, err)
	}
	if want := agent.OpeningLine(res.Context); got.opening != want {
		t.Errorf("opening = %q, want the slice's opening line %q", got.opening, want)
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
	// A file where the state dir would be: no brief dir can be made under it.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", file)
	t.Setenv("XDG_STATE_HOME", file)
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

// TestLaunchRecordsALaunchForAnInProgressSliceWithNoHistory covers a slice
// set In progress some other way (claimed by hand, say) with nothing on its
// record: status alone is no earlier launch, so it is launched fresh — a
// Launched, never a Relaunched.
func TestLaunchRecordsALaunchForAnInProgressSliceWithNoHistory(t *testing.T) {
	l := &fakeLauncher{}
	client := &fakeClient{getPage: func(id string) (*notion.Page, error) { return todoPage(id, true), nil }}

	_, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{}, client.store(), nil, "u1",
		agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceClaimed}, WorkingDir: t.TempDir()},
		config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	if len(client.appended) != 1 || client.appended[0] != "s5" || client.headings[0] != notion.LaunchedHeading {
		t.Errorf("appended = %v %v, want the Launched line filed on the slice", client.appended, client.headings)
	}
}

// A slice still Todo, with nothing yet in its task log, is a fresh launch:
// it files the log's first line, a Launched, and no Relaunched.
func TestLaunchRecordsALaunchForAFreshLaunch(t *testing.T) {
	l := &fakeLauncher{}
	client := &fakeClient{getPage: func(id string) (*notion.Page, error) { return todoPage(id, true), nil }}

	_, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{}, client.store(), nil, "u1",
		agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceTodo}, WorkingDir: t.TempDir()},
		config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	if len(client.appended) != 1 || client.appended[0] != "s5" || client.headings[0] != notion.LaunchedHeading {
		t.Errorf("appended = %v %v, want the Launched line alone filed on the slice", client.appended, client.headings)
	}
}

// A slice that reads Todo (the caller's own record may be stale) but whose
// brief already carries a task event from an earlier pass is a relaunch too.
func TestLaunchRecordsARelaunchWhenTheBriefAlreadyHasATaskEvent(t *testing.T) {
	l := &fakeLauncher{}
	client := &fakeClient{
		getPage: func(id string) (*notion.Page, error) { return todoPage(id, true), nil },
		blocks: func(id string) ([]notion.Block, error) {
			if id == "s5" {
				return []notion.Block{
					block(t, "heading_3", "Sent back"),
					block(t, "paragraph", "Rename the helper."),
				}, nil
			}
			return nil, nil
		},
	}

	_, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{}, client.store(), nil, "u1",
		agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceTodo}, WorkingDir: t.TempDir()},
		config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	if len(client.appended) != 1 || client.appended[0] != "s5" || client.headings[0] != notion.RelaunchedHeading {
		t.Errorf("appended = %v %v, want the Relaunched line alone filed on the slice", client.appended, client.headings)
	}
}

// noteLaunch launches a Todo slice whose brief is the given blocks, and
// reports the heading of every section the launch filed.
func noteLaunch(t *testing.T, body ...notion.Block) []string {
	t.Helper()
	client := &fakeClient{
		getPage: func(id string) (*notion.Page, error) { return todoPage(id, true), nil },
		blocks: func(id string) ([]notion.Block, error) {
			if id == "s5" {
				return body, nil
			}
			return nil, nil
		},
	}
	_, err := Launch(context.Background(), &fakeLauncher{}, &fakeWorktrees{}, &fakeRepo{}, client.store(), nil, "u1",
		agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceTodo}, WorkingDir: t.TempDir()},
		config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	return client.headings
}

// A note is context left for whoever first launches the slice, not history of
// an earlier pass: a Todo slice carrying only a note launches fresh — a
// Launched, with no Relaunched line.
func TestLaunchRecordsALaunchForASliceWithOnlyANote(t *testing.T) {
	appended := noteLaunch(t,
		block(t, "heading_3", "Note"),
		block(t, "paragraph", "At 2026-10-03T23:14:05+01:00"),
		block(t, "paragraph", `From "Render the board" (M1)`),
		block(t, "paragraph", "The menu moved."))
	if len(appended) != 1 || appended[0] != notion.LaunchedHeading {
		t.Errorf("appended = %v, want the Launched line alone", appended)
	}
}

// A note beside real history does not hide it: a hand-back still makes the
// launch a relaunch.
func TestLaunchRecordsARelaunchForANoteBesideAHandBack(t *testing.T) {
	appended := noteLaunch(t,
		block(t, "heading_3", "Note"),
		block(t, "paragraph", "From Craig"),
		block(t, "paragraph", "Mind the cache."),
		block(t, "heading_3", "Handed back"),
		block(t, "paragraph", "Wrote it."))
	if len(appended) != 1 || appended[0] != notion.RelaunchedHeading {
		t.Errorf("appended = %v, want the Relaunched line alone", appended)
	}
}

// A relaunch's or a fresh launch's line that fails to write is logged and
// never fails the launch: the agent is still started.
func TestLaunchToleratesAFailedLaunchOrRelaunchWrite(t *testing.T) {
	for name, body := range map[string][]notion.Block{
		"launch":   nil,
		"relaunch": {block(t, "heading_3", "Handed back"), block(t, "paragraph", "Wrote it.")},
	} {
		l := &fakeLauncher{}
		client := &fakeClient{
			getPage: func(id string) (*notion.Page, error) { return todoPage(id, true), nil },
			blocks: func(id string) ([]notion.Block, error) {
				if id == "s5" {
					return body, nil
				}
				return nil, nil
			},
			appendBlocks: func(string, []map[string]any) ([]notion.Block, error) { return nil, errors.New("notion: 500") },
		}

		res, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{}, client.store(), nil, "u1",
			agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceClaimed}, WorkingDir: t.TempDir()},
			config.AgentModel{})
		if err != nil {
			t.Fatalf("%s: Launch() = %v, want it to go through despite the failed note", name, err)
		}
		if res.Session == "" {
			t.Errorf("%s: session = \"\", want the agent launched regardless", name)
		}
	}
}

// sourcedStore is a launch's Store with a task source behind it: the
// container and describe reads a source project's store answers, over an
// ordinary fake for everything else. Describe is answered only when describe
// is set, so a store that reads containers but not itself is a case too.
type sourcedStore struct {
	Store
	detail       source.ContainerDetail
	containerErr error
	containerIDs []string
}

func (s *sourcedStore) Container(_ context.Context, id string) (source.ContainerDetail, error) {
	s.containerIDs = append(s.containerIDs, id)
	return s.detail, s.containerErr
}

// describingStore is a sourcedStore that also says what it is.
type describingStore struct {
	*sourcedStore
	describe    source.Describe
	describeErr error
}

func (s *describingStore) Describe(context.Context) (source.Describe, error) {
	return s.describe, s.describeErr
}

// launchSourced runs an ordinary first-time launch of a slice filed under
// container c1 against st, and answers the prompt file it wrote.
func launchSourced(t *testing.T, st Store) (LaunchResult, string) {
	t.Helper()
	l := &fakeLauncher{}
	slice := domain.Slice{ID: "s5", Name: "Info view", MilestoneID: "c1"}
	res, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{base: "origin/main"}, st, &fakeReviewer{}, "u1",
		agent.PromptContext{Slice: slice, WorkingDir: t.TempDir()}, config.AgentModel{})
	if err != nil || len(l.launches) != 1 {
		t.Fatalf("Launch() = %v, launches %+v, want it to go through", err, l.launches)
	}
	prompt, err := os.ReadFile(l.launches[0].promptFile)
	if err != nil {
		t.Fatalf("read prompt file: %v", err)
	}
	return res, string(prompt)
}

func sourcedFake() *sourcedStore {
	client := &fakeClient{getPage: func(id string) (*notion.Page, error) { return todoPage(id, true), nil }}
	return &sourcedStore{Store: client.store(), detail: source.ContainerDetail{
		ID: "c1", Title: "Checkout times out", ExternalURL: "https://tracker.example/c1",
		Sections: []source.Section{
			{Kind: source.KindProse, Body: "First paragraph."},
			{Kind: source.KindComments, Comments: []source.Comment{{By: "a", Text: "not prose"}}},
			{Kind: source.KindProse},
			{Kind: source.KindProse, Body: "Second paragraph."},
		},
	}}
}

// A source project's slice is launched with its container in the prompt,
// under the plugin's own noun, carrying only the prose sections.
func TestLaunchCarriesTheContainer(t *testing.T) {
	st := &describingStore{sourcedStore: sourcedFake(), describe: source.Describe{ContainerNoun: "card"}}
	res, prompt := launchSourced(t, st)
	want := &agent.PromptContainer{Noun: "card", Title: "Checkout times out", ExternalURL: "https://tracker.example/c1",
		Prose: "First paragraph.\n\nSecond paragraph."}
	if res.Context.Container == nil || *res.Context.Container != *want {
		t.Errorf("container = %+v, want %+v", res.Context.Container, want)
	}
	if len(st.containerIDs) != 1 || st.containerIDs[0] != "c1" {
		t.Errorf("container reads = %v, want exactly the slice's own", st.containerIDs)
	}
	for _, w := range []string{"## The card", "Checkout times out", "URL: https://tracker.example/c1", "First paragraph.\n\nSecond paragraph."} {
		if !strings.Contains(prompt, w) {
			t.Errorf("prompt does not carry %q:\n%s", w, prompt)
		}
	}
	if strings.Contains(prompt, "not prose") {
		t.Errorf("prompt carries a comments section:\n%s", prompt)
	}
}

// A plugin that will not describe itself, or a store that cannot say, leaves
// the container under the generic noun.
func TestLaunchNamesAContainerGenericallyWithoutANoun(t *testing.T) {
	for name, st := range map[string]Store{
		"no describer":    sourcedFake(),
		"describe failed": &describingStore{sourcedStore: sourcedFake(), describeErr: errors.New("plugin down")},
		"empty noun":      &describingStore{sourcedStore: sourcedFake()},
	} {
		t.Run(name, func(t *testing.T) {
			res, prompt := launchSourced(t, st)
			if res.Context.Container == nil || res.Context.Container.Noun != "container" {
				t.Errorf("container = %+v, want the generic noun", res.Context.Container)
			}
			if !strings.Contains(prompt, "## The container") {
				t.Errorf("prompt has no generic container section:\n%s", prompt)
			}
		})
	}
}

// A container read that fails is logged and the launch goes on without it.
func TestLaunchGoesOnWithoutAContainerThatCannotBeRead(t *testing.T) {
	st := sourcedFake()
	st.containerErr = errors.New("plugin down")
	res, prompt := launchSourced(t, st)
	if res.Context.Container != nil {
		t.Errorf("container = %+v, want none after a failed read", res.Context.Container)
	}
	if strings.Contains(prompt, "## The container") || res.Session == "" {
		t.Errorf("session %q, prompt:\n%s\nwant a launch with no container section", res.Session, prompt)
	}
}

// A slice filed under no container reads none.
func TestLaunchReadsNoContainerForAnUnfiledSlice(t *testing.T) {
	st := sourcedFake()
	l := &fakeLauncher{}
	res, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{base: "origin/main"}, st, nil, "u1",
		agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view"}, WorkingDir: t.TempDir()}, config.AgentModel{})
	if err != nil || res.Context.Container != nil || len(st.containerIDs) != 0 {
		t.Errorf("unfiled launch: err %v, container %+v, reads %v, want none", err, res.Context.Container, st.containerIDs)
	}
}

// A source project's task with no repository — the project has no working
// directory and the task none of its own — is launched with no worktree cut
// and no git read, in the home directory, its prompt sending the agent to find
// the repository; the claim is written as for any launch.
func TestLaunchesATaskWithNoRepositoryInTheHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	st := &describingStore{sourcedStore: sourcedFake(), describe: source.Describe{ContainerNoun: "card"}}
	l, w, r := &fakeLauncher{}, &fakeWorktrees{}, &fakeRepo{base: "origin/main"}
	c := agent.PromptContext{
		Slice:   domain.Slice{ID: "s5", Name: "Info view", MilestoneID: "c1", Branch: "slice/info-view"},
		Project: config.ProjectConfig{Name: "Shortcut", Backend: config.BackendSource}, ProjectID: "p1",
	}
	res, err := Launch(context.Background(), l, w, r, st, nil, "u1", c, config.AgentModel{})
	if err != nil || res.Session == "" {
		t.Fatalf("Launch() = %+v, %v, want a session", res, err)
	}
	if len(w.looks) != 0 || len(w.creates) != 0 || len(r.fetches) != 0 || r.loggedFor != "" {
		t.Errorf("worktrees looked %v, cut %v, fetched %v, logged %q: want git untouched", w.looks, w.creates, r.fetches, r.loggedFor)
	}
	if len(l.launches) != 1 || l.launches[0].workdir != home {
		t.Fatalf("launches = %+v, want one in %s", l.launches, home)
	}
	if !res.Context.RepoUnknown || res.Context.Branch != "" || res.Context.WorkingDir != home || res.Toast != "" {
		t.Errorf("context = %+v, toast %q", res.Context, res.Toast)
	}
	prompt, err := os.ReadFile(l.launches[0].promptFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompt), "## First, find the repository") || !strings.Contains(string(prompt), "nat slice-repo s5 --project p1") {
		t.Errorf("prompt does not send the agent to find the repository:\n%s", prompt)
	}

	// No home directory to start in is an error, before anything is written.
	t.Setenv("HOME", "")
	l2 := &fakeLauncher{}
	if _, err := Launch(context.Background(), l2, w, r, st, nil, "u1", c, config.AgentModel{}); err == nil || len(l2.launches) != 0 {
		t.Errorf("Launch() with no home = %v, launches %+v, want an error and nothing started", err, l2.launches)
	}
}

func TestRepoUnknownAndLaunchDir(t *testing.T) {
	src := config.ProjectConfig{Backend: config.BackendSource}
	if !RepoUnknown(" ", src) || RepoUnknown("/x", src) || RepoUnknown("", config.ProjectConfig{}) {
		t.Error("RepoUnknown: want only a source project with no directory")
	}
	if err := LaunchDir("", src); err != nil {
		t.Errorf("LaunchDir(source, none) = %v, want it let through", err)
	}
	if err := LaunchDir("", config.ProjectConfig{}); err == nil {
		t.Error("LaunchDir(notion, none) = nil, want ExistingDir's refusal")
	}
	if err := LaunchDir(t.TempDir(), src); err != nil {
		t.Errorf("LaunchDir(source, dir) = %v", err)
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
	// A source project has no directory: a task's recorded repository is the
	// whole answer — what relaunch, approve and merge all read — and one with
	// none is nothing ([RepoUnknown]).
	src := config.ProjectConfig{Backend: config.BackendSource}
	if got := WorkdirFor(domain.Slice{Repo: "/src/app"}, src); got != "/src/app" {
		t.Errorf("WorkdirFor(source, repo) = %q", got)
	}
	if got := WorkdirFor(domain.Slice{}, src); got != "" || !RepoUnknown(got, src) {
		t.Errorf("WorkdirFor(source, none) = %q, want nothing", got)
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

// A launch of a slice with a pull request recorded — work already out,
// resumed — gathers the review as it stood: the gh reads come back on the
// context, and the slice is claimed as for any launch.
func TestLaunchGathersTheReviewOfARecordedPullRequest(t *testing.T) {
	dir := repoDir(t)
	l := &fakeLauncher{}
	client := &fakeClient{}
	reviewer := &fakeReviewer{comments: "craig: nit on naming", checks: "X build 1m"}

	res, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{base: "origin/main"}, client.store(),
		reviewer, "u1",
		agent.PromptContext{
			Slice:      domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceClaimed, PRURL: "https://example/pr/1"},
			WorkingDir: dir,
		}, config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	if res.Context.ReviewComments != "craig: nit on naming" || res.Context.ReviewChecks != "X build 1m" {
		t.Errorf("context = %+v, want the gathered review", res.Context)
	}
	if len(client.updated) != 1 {
		t.Errorf("wrote %+v, want the claim", client.updated)
	}
}

// A nil viewer gathers nothing rather than panicking.
func TestLaunchReviewGatherToleratesANilViewer(t *testing.T) {
	dir := repoDir(t)
	client := &fakeClient{}

	res, err := Launch(context.Background(), &fakeLauncher{}, &fakeWorktrees{}, &fakeRepo{base: "origin/main"},
		client.store(), nil, "u1",
		agent.PromptContext{
			Slice:      domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceClaimed, PRURL: "https://example/pr/1"},
			WorkingDir: dir,
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
			Slice:      domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceClaimed, PRURL: "https://example/pr/1"},
			WorkingDir: dir,
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
			Slice:      domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceClaimed, PRURL: "https://example/pr/1"},
			WorkingDir: dir,
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
