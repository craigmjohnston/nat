package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/notion-agent-tracker/internal/config"
)

func TestAppShowsTheBoardPlaceholder(t *testing.T) {
	app := NewApp(config.Config{
		ProjectDBDataSourceID: "ds-1",
		AssigneeUserName:      "Craig Johnston",
		Projects:              map[string]config.ProjectConfig{"p1": {Name: "tracker"}},
	})

	if cmd := app.Init(); cmd != nil {
		t.Error("the board has nothing to load yet")
	}
	view := app.View().Content
	for _, want := range []string{"ds-1", "Craig Johnston", "Projects configured: 1"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
}

func TestAppShowsDashesForAnEmptyConfig(t *testing.T) {
	if view := NewApp(config.Config{}).View().Content; !strings.Contains(view, "—") {
		t.Errorf("view = %q, want dashes for the unset values", view)
	}
}

func TestAppQuitKeys(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}),
		tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}),
		tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}),
	} {
		app := NewApp(config.Config{})
		_, cmd := app.Update(key)
		if cmd == nil {
			t.Errorf("%v should quit", key)
		}
	}
}

func TestAppQuitsAFailedWizard(t *testing.T) {
	// A wizard that failed has no form left to handle ctrl+c itself.
	o := NewOnboarding(config.Config{}, func(string) NotionAPI { return &fakeNotion{} }, &config.MemorySecrets{}, func(config.Config) error { return nil })
	o.fail(errNoPeople)
	app := NewAppWithOnboarding(o)

	if _, cmd := app.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})); cmd == nil {
		t.Error("ctrl+c should quit even when the wizard has failed")
	}
}

func TestAppIgnoresOtherKeysOnTheBoard(t *testing.T) {
	app := NewApp(config.Config{})
	if _, cmd := app.Update(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"})); cmd != nil {
		t.Error("x should do nothing yet")
	}
}

func TestAppRoutesToOnboarding(t *testing.T) {
	o := NewOnboarding(config.Config{}, func(string) NotionAPI { return &fakeNotion{} }, &config.MemorySecrets{}, func(config.Config) error { return nil })
	app := NewAppWithOnboarding(o)

	if cmd := app.Init(); cmd == nil {
		t.Error("onboarding should start its first form")
	}
	if got := app.View().Content; got != o.View() {
		t.Errorf("view = %q, want the wizard's own view", got)
	}

	// While the wizard is on show, "q" is typed into the form rather than
	// quitting the program.
	app.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
	if app.onboarding == nil {
		t.Fatal("the wizard should still be on show")
	}
	if app.onboarding.apiKey != "q" {
		t.Errorf("api key = %q, want the key press to have reached the form", app.onboarding.apiKey)
	}
}

func TestAppTakesOverWhenOnboardingFinishes(t *testing.T) {
	cfg := config.Config{ProjectDBDataSourceID: "ds-1"}
	tests := []struct {
		name string
		msg  OnboardingDoneMsg
		want string
	}{
		{"with projects", OnboardingDoneMsg{Config: cfg}, "Setup complete."},
		{"without projects", OnboardingDoneMsg{Config: cfg, NeedsProject: true}, "No projects yet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := NewOnboarding(config.Config{}, func(string) NotionAPI { return &fakeNotion{} }, &config.MemorySecrets{}, func(config.Config) error { return nil })
			app := NewAppWithOnboarding(o)

			_, cmd := app.Update(tt.msg)
			if cmd != nil {
				t.Error("finishing onboarding should not spawn a command")
			}
			if app.onboarding != nil {
				t.Error("the wizard should be gone")
			}
			view := app.View().Content
			if !strings.Contains(view, tt.want) || !strings.Contains(view, "ds-1") {
				t.Errorf("view = %q, want %q and the new config", view, tt.want)
			}
		})
	}
}
