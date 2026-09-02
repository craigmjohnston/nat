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

	res, err := Launch(context.Background(), l, w, r, client, "u1",
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

	res, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{}, client, "u1",
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

	res, err := Launch(context.Background(), l, w, &fakeRepo{base: "origin/main"}, client, "u1",
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

// TestLaunchReportsAFailedPromptFile covers the prompt file itself failing to
// write: the claim is never reached, since a launch that got no further
// leaves the slice exactly where it was.
func TestLaunchReportsAFailedPromptFile(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "not-there"))
	l := &fakeLauncher{}
	client := &fakeClient{}

	_, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{base: "origin/main"}, client, "u1",
		agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view"}},
		config.AgentModel{})

	if err == nil || !strings.Contains(err.Error(), "launch agent: create prompt dir") {
		t.Errorf("err = %v, want the failed prompt file", err)
	}
	if len(l.launches) != 0 {
		t.Error("no session should start without a prompt to seed it")
	}
	if len(client.updated) != 0 {
		t.Errorf("wrote %+v, want the slice untouched", client.updated)
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

			res, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{base: "origin/main"}, client, "u1",
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

	_, err := Launch(context.Background(), l, &fakeWorktrees{}, &fakeRepo{base: "origin/main"}, client, "u1",
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
