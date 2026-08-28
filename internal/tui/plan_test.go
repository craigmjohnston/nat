package tui

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
)

// planLaunch presses w, types the request into the form it opens, and submits
// it with the one enter that takes, which launches the agent and shows its
// pane straight away. The model and the effort are left as the config named
// them: the form never asks, and nothing here presses the key that would.
func planLaunch(t *testing.T, a *App, request string) {
	t.Helper()
	feed(t, a, press(a, "w"))
	if a.form == nil {
		t.Fatalf("no planning form opened: %s", a.note)
	}
	typeText(a, request)
	drive(t, a, press(a, "enter"))
}

// planConfigure presses the key that reveals the model pair on an open
// planning form.
func planConfigure(t *testing.T, a *App) {
	t.Helper()
	feed(t, a, pressKey(a, tea.Key{Code: 'o', Mod: tea.ModCtrl}))
}

func TestAppPlanKeyOpensTheForm(t *testing.T) {
	app, _, _ := launchApp(t)

	feed(t, app, press(app, "w"))

	if app.screen != screenForm {
		t.Fatalf("screen = %v, want the planning form on show", app.screen)
	}
	if _, ok := app.form.(*PlanForm); !ok {
		t.Fatalf("form = %T, want the planning form", app.form)
	}
	view := stripANSI(app.View().Content)
	for _, want := range []string{"Launch a planning agent", "What do you want to workshop?"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
}

// The planning key needs no slice under the cursor: the plan is what it acts
// on, wherever the user is.
func TestAppPlanKeyWorksFromAnyRow(t *testing.T) {
	app, _, _ := launchApp(t)
	app.board.cursor = rowActiveMilestone

	feed(t, app, press(app, "w"))

	if _, ok := app.form.(*PlanForm); !ok {
		t.Fatalf("form = %T, want the planning form", app.form)
	}
}

func TestAppPlanLaunchStartsTheSessionAndAttaches(t *testing.T) {
	app, launcher, workdir := launchApp(t)

	planLaunch(t, app, "")

	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", launcher.launches)
	}
	got := launcher.launches[0]
	if got.session != agent.PlanSession {
		t.Errorf("session = %q, want %q", got.session, agent.PlanSession)
	}
	// The project default, unasked: there is no directory question any more.
	if got.workdir != workdir {
		t.Errorf("workdir = %q, want the project default %q", got.workdir, workdir)
	}
	// The sentinel, not a page ID: it is what the running planning agent is
	// found by afterwards.
	if got.sliceID != agent.PlanSentinel {
		t.Errorf("tag = %q, want %q", got.sliceID, agent.PlanSentinel)
	}

	// The agent is seeded from the file, so what is in it is the whole contract.
	prompt, err := os.ReadFile(got.promptFile)
	if err != nil {
		t.Fatalf("read the prompt file: %v", err)
	}
	for _, want := range []string{"planning agent", "tracker", "/queue-work",
		"nat plan-apply", "--project " + testProjectID} {
		if !strings.Contains(string(prompt), want) {
			t.Errorf("the prompt is missing %q:\n%s", want, prompt)
		}
	}
	// An empty input is a plain planning session: no request section to start on.
	if strings.Contains(string(prompt), "## The request") {
		t.Errorf("the prompt carries a request nobody typed:\n%s", prompt)
	}

	// No offer to attach: the launch shows the agent straight away.
	if app.form != nil {
		t.Fatalf("form = %T, want the agent shown with nothing to confirm", app.form)
	}
	if want := []string{agent.PlanSession}; !equal(launcher.clients, want) {
		t.Errorf("clients = %v, want %v", launcher.clients, want)
	}
	if app.busy {
		t.Error("the launch is over; the board is live again")
	}
}

// What the user typed into the launch input rides in the prompt, so the agent
// starts on it rather than opening with a question.
func TestAppPlanLaunchCarriesTheRequestInThePrompt(t *testing.T) {
	app, launcher, _ := launchApp(t)

	planLaunch(t, app, "split the reporting milestone")

	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", launcher.launches)
	}
	prompt, err := os.ReadFile(launcher.launches[0].promptFile)
	if err != nil {
		t.Fatalf("read the prompt file: %v", err)
	}
	for _, want := range []string{"## The request", "split the reporting milestone"} {
		if !strings.Contains(string(prompt), want) {
			t.Errorf("the prompt is missing %q:\n%s", want, prompt)
		}
	}
}

// The workshop field is a multiline text box: ctrl+j breaks the line, and the
// whole request — line break included — rides into the prompt. An input would
// have run the lines together.
func TestAppPlanLaunchAcceptsAMultilineRequest(t *testing.T) {
	app, launcher, _ := launchApp(t)

	feed(t, app, press(app, "w"))
	typeText(app, "split the reporting milestone")
	_, cmd := app.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Mod: tea.ModCtrl}))
	feed(t, app, cmd)
	typeText(app, "and slim the first slice down")
	drive(t, app, press(app, "enter"))

	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", launcher.launches)
	}
	prompt, err := os.ReadFile(launcher.launches[0].promptFile)
	if err != nil {
		t.Fatalf("read the prompt file: %v", err)
	}
	want := "split the reporting milestone\nand slim the first slice down"
	if !strings.Contains(string(prompt), want) {
		t.Errorf("the prompt is missing the two lines:\n%s", prompt)
	}
}

// Workshopping and slice work are two settings, not one: the planning agent
// runs as the workshop pair, whatever the slice pair says.
func TestAppPlanLaunchCarriesTheConfiguredWorkshopModel(t *testing.T) {
	app, launcher, _ := launchApp(t)
	app.cfg.WorkshopAgent = config.AgentModel{Model: "haiku", Effort: "low"}
	app.cfg.SliceAgent = config.AgentModel{Model: "opus", Effort: "high"}

	planLaunch(t, app, "")

	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", launcher.launches)
	}
	want := config.AgentModel{Model: "haiku", Effort: "low"}
	if got := launcher.launches[0].model; got != want {
		t.Errorf("model = %+v, want the configured %+v", got, want)
	}
}

// Revealed, the pair is prefilled and editable, like the request above it:
// what the form is left showing is what the session runs as. An effort the
// config names that is not one of the levels this binary knows is offered
// rather than dropped — the user asked for it, and the CLI is the one that
// gets to disagree.
func TestAppPlanFormEditsTheModelAndKeepsAnUnknownEffort(t *testing.T) {
	app, launcher, _ := launchApp(t)
	app.cfg.WorkshopAgent = config.AgentModel{Effort: "glacial"}

	feed(t, app, press(app, "w"))
	planConfigure(t, app)
	view := stripANSI(app.View().Content)
	for _, want := range []string{"Model", "Effort", "glacial"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
	feed(t, app, press(app, "enter")) // past the request
	typeText(app, "haiku")
	feed(t, app, press(app, "enter")) // past the model
	drive(t, app, press(app, "enter"))

	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", launcher.launches)
	}
	want := config.AgentModel{Model: "haiku", Effort: "glacial"}
	if got := launcher.launches[0].model; got != want {
		t.Errorf("model = %+v, want %+v", got, want)
	}
}

// The form opens on the request and nothing else: the model pair is behind the
// key the status line names, and the enter that would have walked onto it
// launches instead.
func TestAppPlanFormOpensWithThePromptAlone(t *testing.T) {
	app, launcher, _ := launchApp(t)

	feed(t, app, press(app, "w"))

	view := stripANSI(app.View().Content)
	for _, unwanted := range []string{"Model", "Effort"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("view carries %q before it was asked for:\n%s", unwanted, view)
		}
	}
	if line := stripANSI(app.statusMessage(80)); !strings.Contains(line, "ctrl+o config") {
		t.Errorf("status = %q, want the config key named", line)
	}

	typeText(app, "split the reporting milestone")
	drive(t, app, press(app, "enter"))

	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want the one enter to have launched", launcher.launches)
	}
	if app.form != nil {
		t.Errorf("form = %T, want the form committed", app.form)
	}
}

// The config key reveals the pair for that one launch: the request typed so far
// rides through the rebuild, the key itself types nothing into it, and the
// status line stops offering what is already on show. Pressed again it is the
// field's — a key with nothing left to reveal must not rebuild the form out
// from under what has been typed since.
func TestAppPlanFormConfigKeyRevealsTheModelFields(t *testing.T) {
	app, _, _ := launchApp(t)
	app.cfg.WorkshopAgent = config.AgentModel{Model: "haiku", Effort: "low"}

	feed(t, app, press(app, "w"))
	typeText(app, "split it")
	planConfigure(t, app)

	form := app.form.(*PlanForm)
	if form.request != "split it" {
		t.Errorf("request = %q, want what was typed before the key", form.request)
	}
	view := stripANSI(app.View().Content)
	for _, want := range []string{"split it", "Model", "haiku", "Effort", "low"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
	if line := stripANSI(app.statusMessage(80)); strings.Contains(line, "config") {
		t.Errorf("status = %q, want nothing left to reveal", line)
	}

	built := form.form
	planConfigure(t, app)
	if form.form != built {
		t.Error("the second press rebuilt the form")
	}
	if got := form.request; got != "split it" {
		t.Errorf("request = %q, want the second press to have changed nothing", got)
	}
}

// A form with no key of its own says only what the app handles for it.
func TestAppFormKeysWithoutAFormHint(t *testing.T) {
	app, _, _ := launchApp(t)

	feed(t, app, press(app, "S"))

	if line := stripANSI(app.statusMessage(80)); line != "esc cancel" {
		t.Errorf("status = %q, want esc alone", line)
	}
}

// A launch shows the planning agent in the terminal beside the board straight
// away, with no confirm between the input and the frame.
func TestAppPlanLaunchOpensTheViewer(t *testing.T) {
	app, launcher, _ := launchApp(t)
	fakeTermFor(t)
	// The refresh that follows the launch sees the session running, as the
	// real tmux would; without it the viewer would be read as exited.
	launcher.live = map[string]string{agent.PlanSentinel: agent.PlanSession}

	planLaunch(t, app, "")

	if want := []string{agent.PlanSession}; !reflect.DeepEqual(launcher.clients, want) {
		t.Errorf("clients = %v, want %v", launcher.clients, want)
	}
	if app.viewer == nil || app.viewer.sliceID != agent.PlanSentinel {
		t.Errorf("viewer = %+v, want the planning agent on show", app.viewer)
	}
	if app.form != nil {
		t.Fatalf("form = %T, want the agent shown with nothing to confirm", app.form)
	}
}

func TestAppPlanLaunchReportsAFailedLaunch(t *testing.T) {
	app, launcher, _ := launchApp(t)
	launcher.launchErr = errors.New("duplicate session")

	planLaunch(t, app, "")

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

func TestLaunchPlanAgentReportsAFailedPromptFile(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "not-there"))
	launcher := &fakeLauncher{}

	msg := runMsg(t, launchPlanAgent(launcher, "project-1", "tracker", "/tmp", "", config.AgentModel{})).(agentLaunchedMsg)

	if msg.err == nil || !strings.Contains(msg.err.Error(), "launch planning agent: create prompt dir") {
		t.Errorf("err = %v, want the failed prompt file", msg.err)
	}
	if len(launcher.launches) != 0 {
		t.Error("no session should start without a prompt to seed it")
	}
}

// With a planning agent already running, w is the same toggle t is for a
// slice's agent: it shows the running one and never launches a second, and w
// again takes it off the board.
func TestAppPlanKeyTogglesTheRunningAgent(t *testing.T) {
	app, launcher, _ := launchApp(t)
	term := fakeTermFor(t)
	app.live = map[string]string{agent.PlanSentinel: agent.PlanSession}

	feed(t, app, press(app, "w"))

	if app.form != nil {
		t.Fatalf("form = %T, want no second planning agent", app.form)
	}
	if want := []string{agent.PlanSession}; !equal(launcher.clients, want) {
		t.Errorf("clients = %v, want %v", launcher.clients, want)
	}
	if app.viewer == nil || app.viewer.sliceID != agent.PlanSentinel {
		t.Errorf("viewer = %+v, want the planning agent on show", app.viewer)
	}

	feed(t, app, press(app, "w"))

	if app.viewer != nil {
		t.Errorf("viewer = %+v, want the agent taken off the board", app.viewer)
	}
	if term.closes != 1 {
		t.Errorf("closes = %d, want the session closed exactly once", term.closes)
	}
}

// The viewer's guidance names the key that opened it: w for the planning
// agent, t for a slice's.
func TestAppViewerHintsNameThePlanKey(t *testing.T) {
	app, _, _ := launchApp(t)
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	app.viewer = &agentViewer{session: newFakeTerm(), sliceID: agent.PlanSentinel, name: "the plan"}
	if line := stripANSI(strings.Join(app.wrapHints(app.viewerHints(), 60, 1), "\n")); !strings.Contains(line, "w hide the agent") {
		t.Errorf("line = %q, want the planning key named", line)
	}

	app.viewer.sliceID = "s5"
	if line := stripANSI(strings.Join(app.wrapHints(app.viewerHints(), 60, 1), "\n")); !strings.Contains(line, "t hide the agent") {
		t.Errorf("line = %q, want the slice key named", line)
	}
}

// A planning agent that has exited has been editing the plan, so the poll that
// notices it gone reloads the board. A poll that failed proves nothing, and a
// slice agent's exit changes no plan.
func TestAppReloadsThePlanWhenThePlanningAgentExits(t *testing.T) {
	tests := []struct {
		name string
		live map[string]string
		msg  liveSessionsMsg
		want int
	}{
		{"planning agent gone", map[string]string{agent.PlanSentinel: agent.PlanSession},
			liveSessionsMsg{live: map[string]string{}}, 1},
		{"planning agent still live", map[string]string{agent.PlanSentinel: agent.PlanSession},
			liveSessionsMsg{live: map[string]string{agent.PlanSentinel: agent.PlanSession}}, 0},
		{"a failed poll", map[string]string{agent.PlanSentinel: agent.PlanSession},
			liveSessionsMsg{err: errors.New("no server")}, 0},
		{"a slice agent gone", map[string]string{"s5": "nat-5"},
			liveSessionsMsg{live: map[string]string{}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _, _ := launchApp(t)
			app.live = tt.live

			_, cmd := app.Update(tt.msg)
			run(cmd)

			client := app.client.(*fakeNotion)
			if len(client.queriedDSIDs) != tt.want {
				t.Errorf("queried %v, want %d loads", client.queriedDSIDs, tt.want)
			}
		})
	}
}

func TestAppPlanKeyIsRefusedWithNothingToLaunchWith(t *testing.T) {
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
			tt.disable(app)

			if cmd := press(app, "w"); cmd != nil {
				t.Error("there is nothing to launch with")
			}
			if app.form != nil || app.board.confirmText != "" {
				t.Errorf("form = %T, confirm = %q, want the key ignored", app.form, app.board.confirmText)
			}
		})
	}
}

func TestPlanFormBusyNote(t *testing.T) {
	f := newPlanForm(DefaultStyles().FormTheme, config.AgentModel{})
	if got := busyNoteOf(f); got != "Launching the planning agent…" {
		t.Errorf("busyNoteOf = %q, want the planning launch note", got)
	}
}

// sizedPlanApp returns an app of a given window size with the planning form
// open, started the way the runtime starts one.
func sizedPlanApp(t *testing.T, width, height int) (*App, *PlanForm) {
	t.Helper()
	app, _, _ := launchApp(t)
	app.Update(tea.WindowSizeMsg{Width: width, Height: height})
	feed(t, app, press(app, "w"))
	f, ok := app.form.(*PlanForm)
	if !ok {
		t.Fatalf("form = %T, want the planning form", app.form)
	}
	return app, f
}

// requestFills says whether the request field takes every line the modal has
// left it: the fields' share of the form's height is everything but huh's own
// key hints, and on a form of one field that is the request field alone.
func requestFills(t *testing.T, a *App, f *PlanForm) {
	t.Helper()
	_, height := a.formSize()
	want := height - planFormFooterHeight
	if got := lipgloss.Height(f.text.View()); got != want {
		t.Errorf("request field is %d lines of a %d-line modal, want %d:\n%s",
			got, height, want, f.text.View())
	}
}

func TestPlanFormRequestFillsTheModal(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 24}, {120, 44}} {
		app, f := sizedPlanApp(t, size.width, size.height)

		requestFills(t, app, f)
		if f.lines <= planFormLines {
			t.Errorf("%dx%d: request field is %d lines, want it grown past the default of %d",
				size.width, size.height, f.lines, planFormLines)
		}
	}
}

func TestPlanFormRefitsWhenTheWindowChanges(t *testing.T) {
	app, f := sizedPlanApp(t, 120, 44)
	tall := f.lines

	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	if f.lines >= tall {
		t.Errorf("request field is %d lines in the shorter window, want fewer than %d", f.lines, tall)
	}
	requestFills(t, app, f)
}

// The model pair takes its room out of the request field's, so a form that has
// been asked to configure the launch still ends inside the modal.
func TestPlanFormLeavesTheModelPairItsRoom(t *testing.T) {
	app, f := sizedPlanApp(t, 120, 44)
	alone := f.lines

	planConfigure(t, app)

	if f.lines >= alone {
		t.Errorf("request field is %d lines beside the model pair, want fewer than %d", f.lines, alone)
	}
	_, height := app.formSize()
	got := lipgloss.Height(f.text.View()) + planFormFooterHeight
	for _, field := range f.rest {
		got += lipgloss.Height(field.View()) + planFormGapHeight
	}
	if got != height {
		t.Errorf("form is %d lines of a %d-line modal, want the modal filled exactly", got, height)
	}
}

// A modal with less room than the fields want is not a reason to draw a
// textarea of no lines at all.
func TestPlanFormKeepsALineOfRequestInATinyModal(t *testing.T) {
	f := newPlanForm(DefaultStyles().FormTheme, config.AgentModel{})
	f.SetSize(30, 3)

	if f.lines != 1 {
		t.Errorf("request field is %d lines, want the one line the floor leaves it", f.lines)
	}
}

func TestPlanFormGoldenAtEachSize(t *testing.T) {
	for _, tt := range []struct {
		name          string
		width, height int
	}{
		{"plan-form-short", 80, 24},
		{"plan-form-tall", 120, 44},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := sizedPlanApp(t, tt.width, tt.height)
			golden(t, tt.name, app.View().Content)
		})
	}
}
