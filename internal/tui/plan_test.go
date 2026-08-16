package tui

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/agent"
)

// planLaunch presses w, types the request into the form it opens, and submits
// it, which launches the agent and shows its pane straight away.
func planLaunch(t *testing.T, a *App, request string) {
	t.Helper()
	feed(t, a, press(a, "w"))
	if a.form == nil {
		t.Fatalf("no planning form opened: %s", a.note)
	}
	typeText(a, request)
	drive(t, a, press(a, "enter"))
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
	for _, want := range []string{"planning agent", "tracker", "/queue-work", "nat plan-apply"} {
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

	msg := runMsg(t, launchPlanAgent(launcher, "tracker", "/tmp", "")).(agentLaunchedMsg)

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
	f := newPlanForm(DefaultStyles().FormTheme)
	if got := busyNoteOf(f); got != "Launching the planning agent…" {
		t.Errorf("busyNoteOf = %q, want the planning launch note", got)
	}
}
