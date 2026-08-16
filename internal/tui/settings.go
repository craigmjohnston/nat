package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/craigmjohnston/nat/internal/config"
)

// settingsSavedMsg carries the settings a completed form describes, for the app
// to apply and persist. It is a message rather than a write done in the form so
// that everything which changes the app's own config goes through Update, the
// way a project switch does.
type settingsSavedMsg struct{ settings Settings }

// Settings is the editable part of the config: what a user would otherwise open
// config.json for. The rest of the file — the workspace's databases, the
// assignee Notion resolved, which project is active — is wiring the wizard and
// the project keys write, not something to type over, and the Notion token is
// not in the config at all: it belongs to the `ntn` CLI and is only ever read
// per request.
//
// The numbers are held as the strings they were typed as, so a field cleared
// back to empty is "leave it to the default" and not a zero the form would have
// to render as something.
type Settings struct {
	// WorkingDir is the active project's default working directory: where its
	// agents start unless a slice names a repo of its own.
	WorkingDir string
	// SplitPercent and PollSeconds are the two numbers, as typed.
	SplitPercent string
	PollSeconds  string
	// WorkshopAgent and SliceAgent are the model pairs a planning agent and a
	// slice's agent are launched on.
	WorkshopAgent config.AgentModel
	SliceAgent    config.AgentModel
}

// settingsOf reads the current config into the form's own shape. An unset
// number comes back empty rather than as "0": the field is meant to read as
// saying nothing, which is what it does.
func settingsOf(cfg config.Config) Settings {
	project := cfg.Projects[cfg.ActiveProjectID]
	return Settings{
		WorkingDir:    project.WorkingDir,
		SplitPercent:  optionalNumber(cfg.AgentSplitPercent),
		PollSeconds:   optionalNumber(cfg.PollSeconds),
		WorkshopAgent: cfg.WorkshopAgent,
		SliceAgent:    cfg.SliceAgent,
	}
}

// optionalNumber is a config number as the form shows it: what it says, or
// nothing at all when it says nothing.
func optionalNumber(v int) string {
	if v == 0 {
		return ""
	}
	return strconv.Itoa(v)
}

// apply writes the settings onto a config, returning it changed. Whatever the
// form does not ask about is carried over untouched — the working directory
// only reaching a project that is actually configured, since there is nowhere
// else to put it.
func (s Settings) apply(cfg config.Config) config.Config {
	cfg.AgentSplitPercent, _ = parseOptionalNumber(s.SplitPercent)
	cfg.PollSeconds, _ = parseOptionalNumber(s.PollSeconds)
	cfg.WorkshopAgent = trimModel(s.WorkshopAgent)
	cfg.SliceAgent = trimModel(s.SliceAgent)
	if project, ok := cfg.Projects[cfg.ActiveProjectID]; ok {
		project.WorkingDir = expandHome(strings.TrimSpace(s.WorkingDir))
		// The map is the one the app was handed, so it is copied rather than
		// written through: a config that fails to save should not have changed
		// the one already in hand by a side door.
		projects := make(map[string]config.ProjectConfig, len(cfg.Projects))
		for id, p := range cfg.Projects {
			projects[id] = p
		}
		projects[cfg.ActiveProjectID] = project
		cfg.Projects = projects
	}
	return cfg
}

// parseOptionalNumber reads a number typed into an optional field: empty is
// zero, which is how the config writes "unset".
func parseOptionalNumber(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

// optionalNumberField is the validator both numbers share: whole, and then
// whatever the config itself will keep. The bounds live in config rather than
// here, so the form refuses exactly the values a later read would have swapped
// the default back in for.
func optionalNumberField(what string, valid func(int) error) func(string) error {
	return func(s string) error {
		v, err := parseOptionalNumber(s)
		if err != nil {
			return fmt.Errorf("%s must be a whole number", what)
		}
		return valid(v)
	}
}

// SettingsForm is the modal behind S: the config file as a form, so nothing
// here has to be edited by hand. It writes local config only — no Notion call
// is made, and nothing on it is sent anywhere.
type SettingsForm struct {
	form    *huh.Form
	heading string

	settings Settings
	// hasProject says whether there is an active project to hang a working
	// directory on. Without one the field is left off entirely rather than
	// offered and quietly dropped.
	hasProject bool
}

// newSettingsForm returns the settings form filled in from the config as it
// stands.
//
// The two numbers say when they take effect, because they do not all take it
// at the same moment: the split re-shares the window on the spot, and the poll
// runs its next tick at the new interval. The rest — the directory and the two
// model pairs — are read when an agent is launched, so they apply to the next
// launch and there is nothing live for them to change.
func newSettingsForm(theme huh.Theme, cfg config.Config) *SettingsForm {
	f := &SettingsForm{heading: "Settings", settings: settingsOf(cfg)}
	_, f.hasProject = cfg.Projects[cfg.ActiveProjectID]

	var fields []huh.Field
	if f.hasProject {
		fields = append(fields, huh.NewInput().
			Title("Working directory").
			Description("Where "+projectName(cfg)+"'s agents start, unless a slice names its own repo; ~ is expanded.").
			Value(&f.settings.WorkingDir).
			Validate(existingDir))
	}
	fields = append(fields,
		huh.NewInput().
			Title("Agent split").
			Description(fmt.Sprintf("Percent of the window an agent's terminal takes beside the board; empty is %d. Applies at once.", config.DefaultSplitPercent)).
			Value(&f.settings.SplitPercent).
			Validate(optionalNumberField("the agent's share", config.ValidSplitPercent)),
		huh.NewInput().
			Title("Poll interval").
			Description(fmt.Sprintf("Seconds between background refetches of the plan; empty is %d. Applies from the next poll.", config.DefaultPollSeconds)).
			Value(&f.settings.PollSeconds).
			Validate(optionalNumberField("the poll", config.ValidPollSeconds)),
	)
	fields = append(fields, modelFieldsFor("Slice agent", &f.settings.SliceAgent)...)
	fields = append(fields, modelFieldsFor("Planning agent", &f.settings.WorkshopAgent)...)

	// One group, not a section apiece: huh pages a form group by group, and a
	// settings screen is somewhere the user arrives knowing which field they
	// came for. Every field is reachable from every other with the arrows, and
	// what the models belong to is said in their titles instead.
	f.form = newForm(theme, huh.NewGroup(fields...))
	return f
}

// projectName is what the working directory's description calls the project it
// belongs to, falling back to the ID for a project configured without a name.
func projectName(cfg config.Config) string {
	if name := cfg.Projects[cfg.ActiveProjectID].Name; name != "" {
		return name
	}
	return cfg.ActiveProjectID
}

// Init starts the form.
func (f *SettingsForm) Init() tea.Cmd { return f.form.Init() }

// Update feeds a message to the form.
func (f *SettingsForm) Update(msg tea.Msg) tea.Cmd {
	form, cmd := f.form.Update(msg)
	f.form = form.(*huh.Form)
	return cmd
}

// State is how far the form has got.
func (f *SettingsForm) State() huh.FormState { return f.form.State }

// View renders the form.
func (f *SettingsForm) View() string { return f.form.View() }

// Heading is the title drawn over the form.
func (f *SettingsForm) Heading() string { return f.heading }

// SetSize gives the form the room the window leaves it.
func (f *SettingsForm) SetSize(width, height int) {
	f.form = f.form.WithWidth(width).WithHeight(height)
}

// busyNote is empty: saving is one small local file, and the toast that follows
// is the whole report.
func (f *SettingsForm) busyNote() string { return "" }

// save hands the settings to the app to apply and persist.
func (f *SettingsForm) save(*App) tea.Cmd {
	s := f.settings
	return func() tea.Msg { return settingsSavedMsg{settings: s} }
}

// settingsFlow opens the settings form. It needs neither a client nor a
// project: the config is local, and every field but the working directory means
// something on a board with nothing loaded at all.
func (a *App) settingsFlow() tea.Cmd {
	if a.busy {
		return nil
	}
	return a.openForm(newSettingsForm(a.styles.FormTheme, a.cfg))
}

// settingsSaved applies the settings and writes them out. The split is the one
// field with something to do on the spot — the window is re-shared, which
// resizes an agent's terminal with it — and the poll picks its new interval up
// on its next tick; the rest are read at the next launch.
//
// A config that will not save is still applied to the session: the app is
// already showing what was asked for, and losing it on the next start is the
// smaller surprise — the same bargain [App.persist] describes.
func (a *App) settingsSaved(msg settingsSavedMsg) (tea.Model, tea.Cmd) {
	a.busy = false
	a.cfg = msg.settings.apply(a.cfg)
	a.resize()
	if err := a.persist(); err != nil {
		a.note, a.err = "", err
		return a, nil
	}
	a.note = ""
	return a, a.showToast("Settings saved.", sevSuccess)
}
