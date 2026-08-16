package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// TestMain keeps the package's tests away from the real tmux: every app polls
// for live sessions as it starts, and the timer behind the poll would otherwise
// hold each test up for half a minute. Tests that care about launching put
// their own launcher in.
//
// The host pane is pinned empty for the same reason. The app reads it from the
// environment, so a suite run from inside tmux — an agent working this very
// project — would otherwise ask the real tmux about a real pane where the same
// suite run from a plain terminal asks nothing. Tests that want a host pane set
// one with t.Setenv, which puts this value back after.
func TestMain(m *testing.M) {
	newLauncher = func() AgentLauncher { return &fakeLauncher{} }
	liveTick = func() tea.Cmd { return nil }
	// The nudge watcher is pinned quiet the same way: no timer, and no stat of
	// the real marker in the home directory. Tests about the watcher put their
	// own readings in.
	nudgeTick = func() tea.Cmd { return nil }
	nudgeStat = func() (time.Time, bool) { return time.Time{}, false }
	// So is the background poll: its own tests put a firing version in.
	pollTick = func(time.Duration) tea.Cmd { return nil }
	// And so is the star animation, which would otherwise keep every app with
	// a live agent in it redrawing twice a second. Its own tests put a firing
	// version in.
	pulseTick = func() tea.Cmd { return nil }
	// And so is the activity watcher, which would otherwise poll a real tmux
	// every two seconds behind every app with a live agent in it. Its own tests
	// put a firing version in.
	activityTick = func() tea.Cmd { return nil }
	// The embedded agent terminal is pinned away from a real pseudo-terminal
	// the same way: nothing is started, and nothing waits on a channel that
	// would never fire. Tests about the viewer put a fake in with
	// fakeTermFor, and the one test of the real wait arms its own channels.
	startTerm = func(*exec.Cmd, int, int) (termSession, error) { return newFakeTerm(), nil }
	awaitTerm = func(termSession) tea.Cmd { return nil }
	// The throttled re-arm goes the same way, so no test pays a frame's sleep
	// for a redraw it drove by hand.
	awaitFrame = func(termSession) tea.Cmd { return nil }
	// The dismissal timers never fire on their own either: a pending 4-second
	// tick would hold every teatest program open, and the confirmations under
	// test would vanish before they could be asserted on. The auto-dismiss
	// tests put a firing version in themselves, and the real one is kept for
	// the test of the timer itself.
	realDismissAfter = dismissAfter
	dismissAfter = func(tea.Msg) tea.Cmd { return nil }
	if err := os.Setenv(agent.PaneEnv, ""); err != nil {
		fmt.Fprintln(os.Stderr, "pin the host pane:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// launchCall is one session the launcher was asked to start.
type launchCall struct {
	session, workdir, promptFile, sliceID string
	model                                 config.AgentModel
}

// fakeLauncher records what it was asked to do, in place of a tmux server.
type fakeLauncher struct {
	live      map[string]string
	liveErr   error
	launchErr error

	// activity is what the watcher reads back, and activityErr the failure that
	// stops it; reads counts the readings taken.
	activity    map[string]agent.Activity
	activityErr error
	reads       int

	// reclaimed is what the startup reconcile reports: how many panes an
	// earlier run had left joined, and the failure that stopped it.
	reclaimed  int
	reclaimErr error
	refreshErr error

	launches []launchCall
	// attached and clients record the sessions each of the two attaches was
	// asked for: the full-screen one, and the viewer's hidden client.
	attached []string
	clients  []string
	// reclaims and refreshes record the host pane each was asked about.
	reclaims  []string
	refreshes []string
}

var _ AgentLauncher = (*fakeLauncher)(nil)

func (f *fakeLauncher) LiveSlices() (map[string]string, error) {
	if f.liveErr != nil {
		return nil, f.liveErr
	}
	return f.live, nil
}

func (f *fakeLauncher) Activity() (map[string]agent.Activity, error) {
	f.reads++
	if f.activityErr != nil {
		return nil, f.activityErr
	}
	return f.activity, nil
}

func (f *fakeLauncher) Launch(session, workdir, promptFile, sliceID string, model config.AgentModel) error {
	f.launches = append(f.launches, launchCall{session, workdir, promptFile, sliceID, model})
	return f.launchErr
}

func (f *fakeLauncher) AttachCmd(session string) *exec.Cmd {
	f.attached = append(f.attached, session)
	// Something harmless: the test runtime never runs it, but nothing here
	// should be able to take a terminal even if it did.
	return exec.Command("true")
}

func (f *fakeLauncher) AttachClientCmd(session string) *exec.Cmd {
	f.clients = append(f.clients, session)
	// The viewer never really runs it: startTerm is stood in for.
	return exec.Command("true")
}

func (f *fakeLauncher) ReclaimStrays(hostPane string) (int, error) {
	f.reclaims = append(f.reclaims, hostPane)
	return f.reclaimed, f.reclaimErr
}

func (f *fakeLauncher) RefreshStatusBar(hostPane string) error {
	f.refreshes = append(f.refreshes, hostPane)
	return f.refreshErr
}

// launchApp returns an app showing testProject with a launcher standing in for
// tmux, a real working directory for the form to validate, and a temp dir of
// its own for the prompt files. It returns the working directory too, because
// that is what the launch flow is asserted against.
func launchApp(t *testing.T) (*App, *fakeLauncher, string) {
	t.Helper()
	workdir := t.TempDir()
	t.Setenv("TMPDIR", t.TempDir())
	// The suite may well be run from inside tmux, and the board reads its own
	// pane from the environment: the tests about the tmux bar set one, and the
	// rest are about a board that has none.
	t.Setenv(agent.PaneEnv, "")

	cfg := testConfig()
	project := cfg.Projects[testProjectID]
	project.WorkingDir = workdir
	cfg.Projects[testProjectID] = project

	app := NewApp(cfg, &fakeNotion{})
	p := testProject()
	app.project = &p
	app.board.hideDone = false // every slice addressable by row, Done ones included
	app.board.SetProject(&p)
	launcher := &fakeLauncher{}
	app.launcher = launcher
	return app, launcher, workdir
}

// sliceAt is the ID of the slice on a row — what a live agent is keyed by —
// with the name of the session such an agent would be running in.
func sliceAt(t *testing.T, a *App, cursor int) (id, session string) {
	t.Helper()
	a.board.cursor = cursor
	s, ok := a.board.SelectedSlice()
	if !ok {
		t.Fatalf("row %d is not a slice", cursor)
	}
	return s.ID, agent.SessionName(s.ID)
}

// drive runs a command and threads what it produces back through the app until
// nothing is left to run, the way the runtime does. Unlike finishForm it does
// not insist the modal is gone at the end: a launch form that fails validation
// stays open, and stepping into the edit group leaves it open on purpose.
func drive(t *testing.T, a *App, cmd tea.Cmd) {
	t.Helper()
	for range 8 {
		if cmd == nil {
			return
		}
		var next []tea.Cmd
		for _, msg := range run(cmd) {
			_, c := a.Update(msg)
			next = append(next, c)
		}
		cmd = tea.Batch(next...)
	}
	t.Fatal("the launch flow did not settle")
}

// step threads a command's messages back through the app for a few rounds and
// then lets go, without insisting anything settles: focusing an input arms a
// cursor-blink command that re-arms forever, so quiescence is not something
// every keystroke can reach.
func step(t *testing.T, a *App, cmd tea.Cmd) {
	t.Helper()
	for range 3 {
		if cmd == nil {
			return
		}
		var next []tea.Cmd
		for _, msg := range run(cmd) {
			_, c := a.Update(msg)
			next = append(next, c)
		}
		cmd = tea.Batch(next...)
	}
}

// launch presses l on the row the cursor is on and answers the prompt it opens
// with the one enter that launches on the defaults and attaches.
func launch(t *testing.T, a *App) {
	t.Helper()
	feed(t, a, press(a, "l"))
	if !a.board.Prompting() {
		t.Fatalf("no launch prompt opened: %s", a.note)
	}
	drive(t, a, press(a, "enter"))
}

// configure presses l and takes the prompt's other choice, which opens the
// launch options form.
func configure(t *testing.T, a *App) {
	t.Helper()
	feed(t, a, press(a, "l"))
	if !a.board.Prompting() {
		t.Fatalf("no launch prompt opened: %s", a.note)
	}
	feed(t, a, press(a, "right"))
	step(t, a, press(a, "enter"))
	if a.form == nil {
		t.Fatal(`"configure & launch" should open the launch options`)
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
			if got := expandHome(tt.path); got != tt.want {
				t.Errorf("expandHome(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestExpandHomeWithoutAHomeDirectory(t *testing.T) {
	t.Setenv("HOME", "")
	// Nothing to expand it to, so the path is left as the user typed it and the
	// validation that follows reports it.
	if got := expandHome("~/x"); got != "~/x" {
		t.Errorf("expandHome = %q, want it untouched", got)
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
			err := existingDir(tt.path)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("existingDir(%q) = %v, want it accepted", tt.path, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("existingDir(%q) = %v, want %q", tt.path, err, tt.want)
			}
		})
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
			if got := workdirFor(tt.slice, project); got != tt.want {
				t.Errorf("workdirFor = %q, want %q", got, tt.want)
			}
		})
	}
}

// l asks on the row itself rather than switching screens: the plan stays on
// show, with the two choices anchored to the slice the cursor is on and the
// hints row saying how to answer them.
func TestAppLaunchPromptsOnTheSelectedSlicesRow(t *testing.T) {
	app, _, _ := launchApp(t)
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app.board.cursor = rowTodoSlice

	feed(t, app, press(app, "l"))

	if app.screen != screenBoard {
		t.Fatalf("screen = %v, want the board still on show", app.screen)
	}
	if app.form != nil {
		t.Fatalf("form = %T, want the prompt on the row instead of a screen", app.form)
	}
	if !app.board.Prompting() {
		t.Fatal("l should anchor the launch prompt to the row")
	}
	// The prompt is on the selected slice's own line, with launching focused:
	// the answer one enter gives.
	line := stripANSI(selectedLine(&app.board))
	for _, want := range []string{"launch", "configure & launch"} {
		if !strings.Contains(line, want) {
			t.Errorf("row = %q, want the choice %q on it", line, want)
		}
	}
	if app.board.PromptChoice() != choiceLaunch {
		t.Errorf("choice = %d, want launching focused", app.board.PromptChoice())
	}
	view := stripANSI(app.View().Content)
	for _, want := range []string{"←/→ choose", "enter select", "esc dismiss"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing the hint %q:\n%s", want, view)
		}
	}
}

// The other choice reaches the options form, on the directory the config
// resolved to — the slice's own repo when it has one.
func TestAppLaunchConfigureOpensTheOptionsForm(t *testing.T) {
	app, _, workdir := launchApp(t)
	app.board.cursor = rowTodoSlice

	configure(t, app)

	if app.screen != screenForm {
		t.Fatalf("screen = %v, want the launch form on show", app.screen)
	}
	f, ok := app.form.(*LaunchForm)
	if !ok {
		t.Fatalf("form = %T, want the launch form", app.form)
	}
	if f.workdir != workdir {
		t.Errorf("working directory = %q, want the project default %q", f.workdir, workdir)
	}
	view := stripANSI(app.View().Content)
	if !strings.Contains(view, "Launch an agent for Info view") {
		t.Errorf("view is missing the heading:\n%s", view)
	}
	// The resolved default is on display before anything is confirmed — the
	// window edge may truncate the path, so only its head is asserted on —
	// with launching, not editing, as the first, pre-selected choice.
	for _, want := range []string{"Working directory: " + workdir[:9], "Launch and show the agent",
		"Edit the options first", "Launch in the background"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
}

// The slice pair the config names is what a plain launch runs Claude Code as,
// with nothing asked: the model is a default, not a question.
func TestAppLaunchCarriesTheConfiguredSliceModel(t *testing.T) {
	app, launcher, _ := launchApp(t)
	app.cfg.SliceAgent = config.AgentModel{Model: "opus", Effort: "high"}
	// The workshop pair is the other one, and no launch of a slice reads it.
	app.cfg.WorkshopAgent = config.AgentModel{Model: "haiku", Effort: "low"}
	app.board.cursor = rowTodoSlice

	launch(t, app)

	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", launcher.launches)
	}
	want := config.AgentModel{Model: "opus", Effort: "high"}
	if got := launcher.launches[0].model; got != want {
		t.Errorf("model = %+v, want the configured %+v", got, want)
	}
}

// A config that names neither leaves both off, which is the launch saying
// nothing at all and Claude Code deciding for itself, as it did before there
// was anywhere to say otherwise.
func TestAppLaunchWithoutAConfiguredModelSaysNothing(t *testing.T) {
	app, launcher, _ := launchApp(t)
	app.board.cursor = rowTodoSlice

	launch(t, app)

	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", launcher.launches)
	}
	if got := launcher.launches[0].model; got != (config.AgentModel{}) {
		t.Errorf("model = %+v, want it unset", got)
	}
}

// The options form shows what enter would launch on before it is opened for
// editing, so the model is never a thing the user has to guess at.
func TestAppLaunchOptionsShowTheConfiguredModel(t *testing.T) {
	app, _, _ := launchApp(t)
	app.cfg.SliceAgent = config.AgentModel{Model: "opus", Effort: "high"}
	app.board.cursor = rowTodoSlice

	configure(t, app)

	view := stripANSI(app.View().Content)
	if want := "Model: opus · effort: high"; !strings.Contains(view, want) {
		t.Errorf("view is missing %q:\n%s", want, view)
	}
}

// Editing the options edits the model too: what is typed there is what this
// one launch runs as, spaces around it trimmed off, and the config is left
// exactly as it was.
func TestAppLaunchEditsTheModelOnDemand(t *testing.T) {
	app, launcher, _ := launchApp(t)
	app.cfg.SliceAgent = config.AgentModel{}
	app.board.cursor = rowTodoSlice

	configure(t, app)
	drive(t, app, press(app, "down")) // to "Edit the options first"
	step(t, app, press(app, "enter"))
	step(t, app, press(app, "enter")) // past the working directory
	typeText(app, " opus ")
	step(t, app, press(app, "enter")) // past the model
	step(t, app, press(app, "down"))  // off the default effort, onto the first level
	finishForm(t, app, press(app, "enter"))

	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", launcher.launches)
	}
	want := config.AgentModel{Model: "opus", Effort: effortLevels[0]}
	if got := launcher.launches[0].model; got != want {
		t.Errorf("model = %+v, want the edited %+v", got, want)
	}
	if app.cfg.SliceAgent != (config.AgentModel{}) {
		t.Errorf("SliceAgent = %+v, want the config untouched by one launch", app.cfg.SliceAgent)
	}
}

func TestModelSummary(t *testing.T) {
	tests := []struct {
		name  string
		model config.AgentModel
		want  string
	}{
		{"unset", config.AgentModel{}, "Model: default · effort: default"},
		{"both", config.AgentModel{Model: "opus", Effort: "max"}, "Model: opus · effort: max"},
		{"model only", config.AgentModel{Model: "opus"}, "Model: opus · effort: default"},
		{"effort only", config.AgentModel{Effort: "max"}, "Model: default · effort: max"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modelSummary(tt.model); got != tt.want {
				t.Errorf("modelSummary(%+v) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestAppLaunchPrefersTheSlicesOwnRepo(t *testing.T) {
	app, launcher, _ := launchApp(t)
	override := t.TempDir()
	app.project.Slices[4].Repo = override // Info view
	app.board.SetProject(app.project)
	app.board.cursor = rowTodoSlice

	launch(t, app)

	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", launcher.launches)
	}
	if got := launcher.launches[0].workdir; got != override {
		t.Errorf("workdir = %q, want the slice's own repo %q", got, override)
	}
}

// The choices are stepped side to side and stop at either end, so a held key
// cannot wrap the focus round to the choice the user was moving away from.
func TestAppLaunchPromptStepsBetweenTheChoices(t *testing.T) {
	tests := []struct {
		name  string
		keys  []string
		want  int
		focus string
	}{
		{"the default", nil, choiceLaunch, "launch"},
		{"right", []string{"right"}, choiceConfigure, "configure & launch"},
		{"tab", []string{"tab"}, choiceConfigure, "configure & launch"},
		{"back again", []string{"right", "left"}, choiceLaunch, "launch"},
		{"shift+tab", []string{"tab", "shift+tab"}, choiceLaunch, "launch"},
		{"stopping at the far end", []string{"right", "right"}, choiceConfigure, "configure & launch"},
		{"stopping at the near end", []string{"left"}, choiceLaunch, "launch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, launcher, _ := launchApp(t)
			app.board.cursor = rowTodoSlice

			feed(t, app, press(app, "l"))
			for _, k := range tt.keys {
				feed(t, app, press(app, k))
			}

			if got := app.board.PromptChoice(); got != tt.want {
				t.Errorf("choice = %d, want %d", got, tt.want)
			}
			// And it is the choice the row draws filled with the accent.
			line := selectedLine(&app.board)
			if !strings.Contains(line, app.styles.PromptFocused.Render(tt.focus)) {
				t.Errorf("row = %q, want %q drawn as the focused choice", stripANSI(line), tt.focus)
			}
			if len(launcher.launches) != 0 {
				t.Errorf("launched %+v, want nothing until the prompt is answered", launcher.launches)
			}
		})
	}
}

// esc takes the prompt down, leaving the row as it was: nothing was in flight,
// so there is nothing to report either.
func TestAppLaunchPromptIsDismissedWithEsc(t *testing.T) {
	app, launcher, _ := launchApp(t)
	app.board.cursor = rowTodoSlice

	feed(t, app, press(app, "l"))
	feed(t, app, press(app, "esc"))

	if app.board.Prompting() {
		t.Error("esc should take the prompt down")
	}
	if app.prompt != nil {
		t.Error("the answer should go with the prompt")
	}
	if len(launcher.launches) != 0 {
		t.Errorf("launched %+v, want nothing from a dismissed prompt", launcher.launches)
	}
	if app.board.confirmText != "" || app.toast != "" {
		t.Errorf("confirm = %q, toast = %q, want the dismissal to say nothing",
			app.board.confirmText, app.toast)
	}
}

// While the prompt is up it owns the keys: the board must not move out from
// under a question about the row the cursor is on, and q must not quit with it
// unanswered.
func TestAppLaunchPromptOwnsTheKeys(t *testing.T) {
	for _, k := range []string{"j", "q", "d", "?"} {
		t.Run(k, func(t *testing.T) {
			app, _, _ := launchApp(t)
			app.board.cursor = rowTodoSlice

			feed(t, app, press(app, "l"))
			if cmd := press(app, k); cmd != nil {
				t.Errorf("%q did something while the prompt was up", k)
			}

			if !app.board.Prompting() {
				t.Errorf("%q took the prompt down", k)
			}
			if app.board.cursor != rowTodoSlice {
				t.Errorf("cursor = %d, want it held on the row the prompt is about", app.board.cursor)
			}
			if app.screen != screenBoard || app.form != nil {
				t.Errorf("screen = %v, form = %T, want the board left alone", app.screen, app.form)
			}
		})
	}
}

// A reload may move the row the prompt is anchored to, or take it away
// entirely, so the question goes with the plan it was asked about.
func TestAppLaunchPromptGoesWithAReload(t *testing.T) {
	app, _, _ := launchApp(t)
	app.board.cursor = rowTodoSlice

	feed(t, app, press(app, "l"))
	app.Update(projectLoadedMsg{project: testProject()})

	if app.board.Prompting() || app.prompt != nil {
		t.Error("the prompt should go with the plan it was asked about")
	}
}

func TestAppLaunchStartsTheSessionAndAttaches(t *testing.T) {
	app, launcher, workdir := launchApp(t)
	app.board.cursor = rowTodoSlice

	launch(t, app)

	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", launcher.launches)
	}
	got := launcher.launches[0]
	if want := agent.SessionName("s5"); got.session != want {
		t.Errorf("session = %q, want %q", got.session, want)
	}
	if got.workdir != workdir {
		t.Errorf("workdir = %q, want %q", got.workdir, workdir)
	}
	// The full ID, not the session name: it is what the running agent is found
	// by afterwards.
	if got.sliceID != "s5" {
		t.Errorf("slice = %q, want %q", got.sliceID, "s5")
	}

	// The agent is seeded from the file, so what is in it is the whole contract.
	prompt, err := os.ReadFile(got.promptFile)
	if err != nil {
		t.Fatalf("read the prompt file: %v", err)
	}
	for _, want := range []string{"Info view", "s5", workdir, "Craig Johnston"} {
		if !strings.Contains(string(prompt), want) {
			t.Errorf("the prompt is missing %q:\n%s", want, prompt)
		}
	}

	// Nothing was written to Notion: the agent claims its own slice.
	if client := app.client.(*fakeNotion); len(client.updated) != 0 {
		t.Errorf("wrote %+v to Notion, want the claim left to the agent", client.updated)
	}

	// One enter is the whole flow: the prompt is gone, no form was opened at
	// all, and the terminal is the agent's.
	if app.board.Prompting() {
		t.Error("the prompt should go with the answer")
	}
	if app.form != nil {
		t.Fatalf("form = %T, want the launch to finish without a form", app.form)
	}
	if want := []string{agent.SessionName("s5")}; !equal(launcher.clients, want) {
		t.Errorf("clients = %v, want the agent shown beside the board", launcher.clients)
	}
	if app.busy {
		t.Error("the launch is over; the board is live again")
	}
}

// The edit path: configuring reaches the options form, with the directory
// opened for editing on demand and confirming from there launching likewise.
func TestAppLaunchEditsTheWorkingDirectoryOnDemand(t *testing.T) {
	app, launcher, workdir := launchApp(t)
	sub := filepath.Join(workdir, "sub")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	app.board.cursor = rowTodoSlice

	configure(t, app)
	drive(t, app, press(app, "down")) // to "Edit the options first"
	step(t, app, press(app, "enter"))
	if app.form == nil {
		t.Fatal("choosing to edit should keep the form open on the directory")
	}
	typeText(app, "/sub")             // appended to the prefilled default
	step(t, app, press(app, "enter")) // past the directory
	step(t, app, press(app, "enter")) // past the model
	finishForm(t, app, press(app, "enter"))

	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", launcher.launches)
	}
	if got := launcher.launches[0].workdir; got != sub {
		t.Errorf("workdir = %q, want the edited %q", got, sub)
	}
	// Confirming from the edit launches likewise: straight to the agent.
	if want := []string{agent.SessionName("s5")}; !equal(launcher.clients, want) {
		t.Errorf("clients = %v, want %v", launcher.clients, want)
	}
}

// The background option, still on the options form behind the prompt's second
// choice, launches without taking the terminal, and the confirmation names the
// key that attaches later.
func TestAppLaunchInTheBackground(t *testing.T) {
	app, launcher, _ := launchApp(t)
	app.board.cursor = rowTodoSlice

	configure(t, app)
	drive(t, app, press(app, "down"))
	drive(t, app, press(app, "down")) // to "Launch in the background"
	drive(t, app, press(app, "enter"))

	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", launcher.launches)
	}
	if len(launcher.attached) != 0 {
		t.Errorf("attached = %v, want the terminal left alone", launcher.attached)
	}
	if want := `Launched nat-5 for "Info view" — t attaches.`; app.board.confirmText != want {
		t.Errorf("confirm = %q, want %q", app.board.confirmText, want)
	}
	if app.busy {
		t.Error("a background launch leaves nothing in flight")
	}
}

// A default directory that is not there is caught before tmux is asked to
// start anything, where a session would fail with nobody looking. The refusal
// takes the prompt's place on the row it was about.
func TestAppLaunchRefusesAMissingDefaultDirectory(t *testing.T) {
	app, launcher, _ := launchApp(t)
	missing := filepath.Join(t.TempDir(), "not-there")
	project := app.cfg.Projects[testProjectID]
	project.WorkingDir = missing
	app.cfg.Projects[testProjectID] = project
	app.board.cursor = rowTodoSlice

	launch(t, app)

	if len(launcher.launches) != 0 {
		t.Errorf("launched %+v, want nothing until the directory is fixed", launcher.launches)
	}
	if app.busy {
		t.Error("a refused launch leaves nothing in flight")
	}
	want := fmt.Sprintf("Cannot launch an agent for %q: %s is not there.", "Info view", missing)
	if app.board.confirmText != want {
		t.Errorf("confirm = %q, want %q", app.board.confirmText, want)
	}
	// The prompt makes way for the report; the other choice is a fresh l away.
	if app.board.Prompting() {
		t.Error("the prompt should go with the answer, refused or not")
	}
}

// The same directory, on the options form behind the prompt's other choice, is
// still caught there — with the form kept open to say so.
func TestAppLaunchConfigureRefusesAMissingDirectory(t *testing.T) {
	app, launcher, _ := launchApp(t)
	project := app.cfg.Projects[testProjectID]
	project.WorkingDir = filepath.Join(t.TempDir(), "not-there")
	app.cfg.Projects[testProjectID] = project
	app.board.cursor = rowTodoSlice

	configure(t, app)
	drive(t, app, press(app, "enter"))

	if len(launcher.launches) != 0 {
		t.Errorf("launched %+v, want nothing until the directory is fixed", launcher.launches)
	}
	if app.form == nil {
		t.Fatal("the form should stay open on the failed validation")
	}
	// The error may be wrapped mid-phrase at the modal's edge, putting a border
	// glyph in the middle of it, so the box is dropped and the view flattened
	// onto one line before looking for it.
	view := stripANSI(app.View().Content)
	flat := strings.Join(strings.Fields(strings.ReplaceAll(view, "│", " ")), " ")
	if !strings.Contains(flat, "is not there") {
		t.Errorf("view is missing the validation error:\n%s", view)
	}
}

func TestAppLaunchReportsAFailedLaunch(t *testing.T) {
	app, launcher, _ := launchApp(t)
	launcher.launchErr = errors.New("duplicate session")
	app.board.cursor = rowTodoSlice

	launch(t, app)

	if app.form != nil {
		t.Errorf("form = %T, want no offer to attach to a session that did not start", app.form)
	}
	if app.err == nil || !strings.Contains(app.err.Error(), "duplicate session") {
		t.Errorf("err = %v, want the failed launch", app.err)
	}
	if app.busy {
		t.Error("a failed launch should leave nothing in flight")
	}
}

func TestLaunchAgentReportsAFailedPromptFile(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "not-there"))
	launcher := &fakeLauncher{}

	msg := runMsg(t, launchAgent(launcher, agent.PromptContext{
		Slice: domain.Slice{ID: "s5", Name: "Info view"},
	}, config.AgentModel{}, true)).(agentLaunchedMsg)

	if msg.err == nil || !strings.Contains(msg.err.Error(), "launch agent: create prompt dir") {
		t.Errorf("err = %v, want the failed prompt file", msg.err)
	}
	if len(launcher.launches) != 0 {
		t.Error("no session should start without a prompt to seed it")
	}
}

func TestAppLaunchRefusesASliceThatIsNotTodo(t *testing.T) {
	tests := []struct {
		name   string
		cursor int
		want   string
	}{
		{"claimed", rowClaimedSlice, `"Board screen" is In progress — only Todo slices can be launched.`},
		{"done", rowDoneSlice, `"Domain model" is Done — only Todo slices can be launched.`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, launcher, _ := launchApp(t)
			app.board.cursor = tt.cursor

			press(app, "l")

			if app.form != nil {
				t.Errorf("form = %T, want no launch form", app.form)
			}
			if len(launcher.launches) != 0 {
				t.Errorf("launched %+v, want nothing", launcher.launches)
			}
			if app.board.confirmText != tt.want {
				t.Errorf("confirm = %q, want %q", app.board.confirmText, tt.want)
			}
		})
	}
}

func TestAppLaunchRefusesASliceAlreadyRunning(t *testing.T) {
	app, _, _ := launchApp(t)
	id, session := sliceAt(t, app, rowTodoSlice)
	app.live = map[string]string{id: session}

	press(app, "l")

	if app.form != nil {
		t.Errorf("form = %T, want no second agent on the same slice", app.form)
	}
	if want := `An agent is already running for "Info view" — press t to attach.`; app.board.confirmText != want {
		t.Errorf("confirm = %q, want %q", app.board.confirmText, want)
	}
}

// Two slices of one project share nearly all of their page ID, which used to
// make them one agent as far as the board was concerned: launching the second
// was refused, both rows lit up, and t went to whichever agent started first.
func TestAppTellsApartAgentsOnSlicesSharingAnIDPrefix(t *testing.T) {
	app, launcher, _ := launchApp(t)
	claimed, todo := "3b738308f654812dac8dd4c80dfecb09", "3b738308f65481708c99eccab4463d8f"
	app.project.Slices[3].ID = claimed // Board screen, Claimed
	app.project.Slices[4].ID = todo    // Info view, Todo
	app.board.SetProject(app.project)
	launcher.live = map[string]string{claimed: agent.SessionName(claimed)}

	feed(t, app, app.refreshLive())

	// Only the slice whose agent is running is marked.
	view := stripANSI(app.View().Content)
	if !strings.Contains(view, "Board screen "+pulseFrames[0]) {
		t.Errorf("the live slice is unmarked:\n%s", view)
	}
	if strings.Contains(view, "Info view "+pulseFrames[0]) {
		t.Errorf("a slice with no agent of its own is marked:\n%s", view)
	}

	// t on the claimed slice shows its agent, not the other one.
	app.board.cursor = rowClaimedSlice
	feed(t, app, press(app, "t"))
	if want := []string{agent.SessionName(claimed)}; !equal(launcher.clients, want) {
		t.Errorf("clients = %v, want %v", launcher.clients, want)
	}

	// And the slice with no agent can still be launched, under a session name
	// of its own rather than one tmux would refuse as a duplicate.
	app.board.cursor = rowTodoSlice
	launch(t, app)
	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", launcher.launches)
	}
	if got := launcher.launches[0]; got.sliceID != todo {
		t.Errorf("slice = %q, want %q", got.sliceID, todo)
	} else if got.session == agent.SessionName(claimed) {
		t.Errorf("session = %q, want one the running agent is not already using", got.session)
	}
}

func TestAppLaunchNeedsASliceUnderTheCursor(t *testing.T) {
	app, _, _ := launchApp(t)
	app.board.cursor = rowActiveMilestone

	press(app, "l")

	if app.form != nil {
		t.Error("a launch form was opened with no slice to launch")
	}
	if !strings.Contains(app.board.confirmText, "Move to a slice") {
		t.Errorf("confirm = %q, want the slice hint", app.board.confirmText)
	}
}

func TestAppLaunchIsRefusedWithNothingToLaunchWith(t *testing.T) {
	tests := []struct {
		name    string
		disable func(*App)
	}{
		{"no project", func(a *App) { a.cfg.ActiveProjectID = "" }},
		{"no launcher", func(a *App) { a.launcher = nil }},
		{"a write already in flight", func(a *App) { a.busy = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _, _ := launchApp(t)
			app.board.cursor = rowTodoSlice
			tt.disable(app)

			if cmd := press(app, "l"); cmd != nil {
				t.Error("there is nothing to launch with")
			}
			if app.board.Prompting() || app.board.confirmText != "" {
				t.Errorf("prompting = %v, confirm = %q, want the key ignored",
					app.board.Prompting(), app.board.confirmText)
			}
		})
	}
}

func TestAppShowsALiveSessionWhateverTheSlicesStatus(t *testing.T) {
	// An agent claims its slice as it starts and marks it Done as it finishes,
	// so it is watchable from a row of any status.
	tests := []struct {
		name   string
		cursor int
	}{
		{"todo", rowTodoSlice},
		{"claimed", rowClaimedSlice},
		{"done", rowDoneSlice},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, launcher, _ := launchApp(t)
			fakeTermFor(t)
			id, session := sliceAt(t, app, tt.cursor)
			app.live = map[string]string{id: session}

			feed(t, app, press(app, "t"))

			if want := []string{session}; !equal(launcher.clients, want) {
				t.Errorf("clients = %v, want %v", launcher.clients, want)
			}
			if app.viewer == nil || app.viewer.sliceID != id {
				t.Errorf("viewer = %+v, want the slice's agent on show", app.viewer)
			}
		})
	}
}

// How the whole window reads with the launch prompt up: the plan still on
// show, the choices anchored to the slice's own row, and the keys that answer
// them where the row's hints were.
func TestAppLaunchPromptGolden(t *testing.T) {
	a := sizedApp(80, 16)
	a.launcher = &fakeLauncher{}
	a.board.cursor = 1 // the plan's one slice

	feed(t, a, press(a, "l"))

	golden(t, "app-launch-prompt", a.View().Content)
}

func TestAppAttachNeedsALiveSession(t *testing.T) {
	app, launcher, _ := launchApp(t)
	app.board.cursor = rowClaimedSlice

	press(app, "t")

	if len(launcher.attached) != 0 {
		t.Errorf("attached = %v, want nothing to attach to", launcher.attached)
	}
	if want := `No agent session is running for "Board screen".`; app.board.confirmText != want {
		t.Errorf("confirm = %q, want %q", app.board.confirmText, want)
	}
}

func TestAppAttachNeedsASliceUnderTheCursor(t *testing.T) {
	app, _, _ := launchApp(t)
	app.board.cursor = rowActiveMilestone

	press(app, "t")

	if !strings.Contains(app.board.confirmText, "Move to a slice") {
		t.Errorf("confirm = %q, want the slice hint", app.board.confirmText)
	}
}

func TestAppAttachIsRefusedWithNothingToAttachWith(t *testing.T) {
	for _, tt := range []struct {
		name    string
		disable func(*App)
	}{
		{"no launcher", func(a *App) { a.launcher = nil }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _, _ := launchApp(t)
			app.board.cursor = rowTodoSlice
			id, session := sliceAt(t, app, rowTodoSlice)
			app.live = map[string]string{id: session}
			tt.disable(app)

			if cmd := press(app, "t"); cmd != nil {
				t.Error("there is nothing to attach with")
			}
		})
	}
}

func TestAttachedReportsTheTerminalComingBack(t *testing.T) {
	msg := attached("s5", "nat-5")(nil).(agentAttachedMsg)
	if msg.note != "Detached from nat-5." {
		t.Errorf("msg = %+v, want the detached note", msg)
	}
	if msg.slice != "s5" {
		t.Errorf("slice = %q, want the slice the session was about", msg.slice)
	}
	boom := errors.New("no such session")
	got := attached("s5", "nat-5")(boom).(agentAttachedMsg)
	if got.err == nil || !errors.Is(got.err, boom) {
		t.Errorf("err = %v, want it to wrap %v", got.err, boom)
	}
	if !strings.Contains(got.err.Error(), "nat-5") {
		t.Errorf("err = %v, want it to name the session", got.err)
	}
}

func TestAppRefreshesTheSliceAfterAttaching(t *testing.T) {
	client := &fakeNotion{}
	app := newWriteApp(client)
	app.launcher = &fakeLauncher{}
	app.busy = true

	_, cmd := app.Update(agentAttachedMsg{note: "Detached from nat-5.", slice: "s5"})
	run(cmd)

	if app.busy {
		t.Error("the terminal is back; nothing is in flight")
	}
	if app.board.confirmText != "Detached from nat-5." {
		t.Errorf("confirm = %q, want the detached confirmation", app.board.confirmText)
	}
	// The agent has had the terminal to itself, so the slice it was working on
	// is re-read rather than trusted — that one page, not the whole plan.
	if !equal(client.fetchedPages, []string{"s5"}) {
		t.Errorf("fetched %v, want the agent's slice refetched", client.fetchedPages)
	}
	if client.queriedDSIDs != nil {
		t.Errorf("queried %v, want no full reload", client.queriedDSIDs)
	}
}

func TestAppMarksSlicesWithALiveSession(t *testing.T) {
	app, launcher, _ := launchApp(t)
	id, session := sliceAt(t, app, rowTodoSlice)
	launcher.live = map[string]string{id: session, "someone-elses-slice": "nat-elsewhere"}

	feed(t, app, app.refreshLive())

	if app.live[id] != session {
		t.Errorf("live = %v, want %q under %q", app.live, session, id)
	}
	view := stripANSI(app.View().Content)
	if !strings.Contains(view, "Info view "+pulseFrames[0]) {
		t.Errorf("the live slice is unmarked:\n%s", view)
	}
	if strings.Contains(view, "Board screen "+pulseFrames[0]) {
		t.Errorf("a slice with no session of its own is marked:\n%s", view)
	}
}

func TestAppReportsAFailedSessionRead(t *testing.T) {
	app, launcher, _ := launchApp(t)
	app.live = map[string]string{"s5": "nat-5"}
	launcher.liveErr = errors.New("no server")

	feed(t, app, app.refreshLive())

	if app.live != nil {
		t.Errorf("live = %v, want it cleared when it cannot be read", app.live)
	}
	if !strings.Contains(app.toast, "Could not read tmux panes: no server") {
		t.Errorf("toast = %q, want the failed read", app.toast)
	}
	// A background poll is not worth an error banner over the board.
	if app.err != nil {
		t.Errorf("err = %v, want the failure kept to the status bar", app.err)
	}
}

func TestAppRefreshesLiveSessions(t *testing.T) {
	tests := []struct {
		name string
		act  func(t *testing.T, a *App) tea.Cmd
	}{
		{"on startup", func(_ *testing.T, a *App) tea.Cmd { return a.Init() }},
		{"on r", func(_ *testing.T, a *App) tea.Cmd { return press(a, "r") }},
		{"on the timer", func(_ *testing.T, a *App) tea.Cmd {
			_, cmd := a.Update(liveTickMsg{})
			return cmd
		}},
		{"on leaving a form", func(t *testing.T, a *App) tea.Cmd {
			a.board.cursor = rowTodoSlice
			configure(t, a)
			return press(a, "esc")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, launcher, _ := launchApp(t)
			id, session := sliceAt(t, app, rowTodoSlice)
			launcher.live = map[string]string{id: session}

			feed(t, app, tt.act(t, app))

			if app.live[id] != session {
				t.Errorf("live = %v, want the running agents re-read", app.live)
			}
		})
	}
}

func TestAppKeepsTheLiveTimerRunning(t *testing.T) {
	app, _, _ := launchApp(t)
	// The real timer, rather than the one TestMain stands in with: a tick that
	// did not arm the next one would leave the board to go stale.
	liveTick = func() tea.Cmd { return func() tea.Msg { return liveTickMsg{} } }
	t.Cleanup(func() { liveTick = func() tea.Cmd { return nil } })

	_, cmd := app.Update(liveTickMsg{})

	var ticked bool
	for _, msg := range run(cmd) {
		if _, ok := msg.(liveTickMsg); ok {
			ticked = true
		}
	}
	if !ticked {
		t.Error("a tick should arm the next one")
	}
}

func TestTheRealEdgesAreThere(t *testing.T) {
	// What the app runs with when nothing is standing in for them. Neither is
	// exercised further here: one drives the tmux the user is already running,
	// and the other only comes back half a minute later.
	if defaultLauncher() == nil {
		t.Error("the app should launch through the real tmux")
	}
	if defaultLiveTick() == nil {
		t.Error("the sessions should be re-read on a timer")
	}
	if _, ok := liveTicked(time.Time{}).(liveTickMsg); !ok {
		t.Error("the timer going off should prod the app to re-read them")
	}
}

func TestAppDoesNotPollWithoutAProject(t *testing.T) {
	app := NewApp(config.Config{}, &fakeNotion{})
	if cmd := app.refreshLive(); cmd != nil {
		t.Error("there are no slices to mark")
	}
}

// The board's pane has changed size, so the sections of the tmux bar under it
// — each drawn to the width of the pane above it — are rebuilt.
func TestResizeRedrawsTheTmuxStatusBar(t *testing.T) {
	app, launcher, _ := launchApp(t)
	t.Setenv(agent.PaneEnv, "%0")

	_, cmd := app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	feed(t, app, cmd)

	if !reflect.DeepEqual(launcher.refreshes, []string{"%0"}) {
		t.Errorf("refreshes = %v, want the bar rebuilt for the board's pane", launcher.refreshes)
	}
}

// A bar tmux would not redraw is chrome around a board that is still entirely
// usable: it is logged, not raised over the plan.
func TestResizeSurvivesAStatusBarThatWillNotRedraw(t *testing.T) {
	app, launcher, _ := launchApp(t)
	t.Setenv(agent.PaneEnv, "%0")
	launcher.refreshErr = errors.New("no server")

	_, cmd := app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	feed(t, app, cmd)

	if app.err != nil {
		t.Errorf("err = %v, want the failure kept off the board", app.err)
	}
}

// Outside tmux, and without a launcher, there is no bar of ours under the
// board to redraw.
func TestResizeLeavesTmuxAloneWithNoPaneToDrawFor(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pane    string
		noAgent bool
	}{
		{name: "outside tmux", pane: ""},
		{name: "without a launcher", pane: "%0", noAgent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, launcher, _ := launchApp(t)
			t.Setenv(agent.PaneEnv, tc.pane)
			if tc.noAgent {
				app.launcher = nil
			}

			_, cmd := app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
			feed(t, app, cmd)

			if len(launcher.refreshes) != 0 {
				t.Errorf("refreshes = %v, want tmux left alone", launcher.refreshes)
			}
		})
	}
}

// Starting up after a run that died with an agent joined: the stray is
// re-homed rather than left in a window whose close would kill it.
func TestStartupReclaimsStrays(t *testing.T) {
	app, launcher, _ := launchApp(t)
	t.Setenv(agent.PaneEnv, "%0")
	launcher.reclaimed = 1

	feed(t, app, app.Init())

	if !reflect.DeepEqual(launcher.reclaims, []string{"%0"}) {
		t.Errorf("reclaims = %v, want the reconcile run once for the board's pane", launcher.reclaims)
	}
	if app.toast != "Re-homed 1 agent left joined by an earlier run." {
		t.Errorf("toast = %q, want the re-homed agent reported", app.toast)
	}
}

func TestStraysReclaimedNotes(t *testing.T) {
	tests := []struct {
		name string
		msg  straysReclaimedMsg
		want string
	}{
		{"nothing to do", straysReclaimedMsg{}, ""},
		{"one pane", straysReclaimedMsg{count: 1}, "Re-homed 1 agent left joined by an earlier run."},
		{"several panes", straysReclaimedMsg{count: 3}, "Re-homed 3 agents left joined by an earlier run."},
		{"a failure", straysReclaimedMsg{err: errors.New("no server")},
			"Could not re-home the agents left by an earlier run: no server"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _, _ := launchApp(t)

			app.Update(tt.msg)

			if app.toast != tt.want {
				t.Errorf("toast = %q, want %q", app.toast, tt.want)
			}
			// The agents it could not move are all still running, and the plan
			// is still worth looking at.
			if app.err != nil {
				t.Errorf("err = %v, want the reconcile kept to the status bar", app.err)
			}
		})
	}
}

// A board with no window of its own joins nothing, and the panes it would find
// are another board's to look after.
func TestNoReclaimWithoutABoardPane(t *testing.T) {
	app, _, _ := launchApp(t)
	t.Setenv(agent.PaneEnv, "")
	if cmd := app.reclaimStrays(); cmd != nil {
		t.Error("there is no window to reconcile")
	}

	t.Setenv(agent.PaneEnv, "%0")
	app.launcher = nil
	if cmd := app.reclaimStrays(); cmd != nil {
		t.Error("there is no tmux to reconcile with")
	}
}

func TestBusyNoteOf(t *testing.T) {
	tests := []struct {
		name  string
		modal modal
		want  string
	}{
		{"a write", newDeleteSliceForm(DefaultStyles().FormTheme, domain.Slice{Name: "x"}), "Saving…"},
		{"a launch", newLaunchForm(DefaultStyles().FormTheme, domain.Slice{Name: "x"}, "/tmp", config.AgentModel{}), "Launching the agent…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := busyNoteOf(tt.modal); got != tt.want {
				t.Errorf("busyNoteOf = %q, want %q", got, tt.want)
			}
		})
	}
}
