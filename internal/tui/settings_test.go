package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/config"
)

// stepThrough presses enter through every field and group of the open form,
// chaining what each press dispatches the way the runtime does, until the form
// completes or has plainly stopped moving.
func stepThrough(t *testing.T, a *App) {
	t.Helper()
	for range 16 {
		if a.form == nil {
			return
		}
		cmd := press(a, "enter")
		for range 4 {
			if cmd == nil {
				break
			}
			var next []tea.Cmd
			for _, msg := range run(cmd) {
				_, c := a.Update(msg)
				next = append(next, c)
			}
			cmd = tea.Batch(next...)
		}
	}
}

// settingsFormOf is the settings modal on show, failing the test when the form
// open over the board is something else.
func settingsFormOf(t *testing.T, a *App) *SettingsForm {
	t.Helper()
	f, ok := a.form.(*SettingsForm)
	if !ok {
		t.Fatalf("form = %T, want the settings form", a.form)
	}
	return f
}

// fullConfig is a config with every editable field set, so a test can tell a
// field that was carried through from one that was defaulted.
func fullConfig(dir string) config.Config {
	cfg := testConfig()
	cfg.AgentSplitPercent = 70
	cfg.PollSeconds = 45
	cfg.WorkshopAgent = config.AgentModel{Model: "sonnet", Effort: "low"}
	cfg.SliceAgent = config.AgentModel{Model: "opus", Effort: "high"}
	cfg.Projects[testProjectID] = config.ProjectConfig{
		Name: "tracker", SlicesDSID: "sl-ds", WorkingDir: dir,
	}
	return cfg
}

func TestSettingsOfReadsTheConfigAsTyped(t *testing.T) {
	got := settingsOf(fullConfig("/work"))

	want := Settings{
		WorkingDir:    "/work",
		SplitPercent:  "70",
		PollSeconds:   "45",
		WorkshopAgent: config.AgentModel{Model: "sonnet", Effort: "low"},
		SliceAgent:    config.AgentModel{Model: "opus", Effort: "high"},
	}
	if got != want {
		t.Errorf("settings = %+v, want %+v", got, want)
	}
}

func TestSettingsOfShowsAnUnsetNumberAsNothing(t *testing.T) {
	got := settingsOf(testConfig())

	if got.SplitPercent != "" || got.PollSeconds != "" {
		t.Errorf("numbers = %q/%q, want an unset number to read as empty rather than zero",
			got.SplitPercent, got.PollSeconds)
	}
}

func TestSettingsApplyWritesEveryFieldBack(t *testing.T) {
	cfg := settingsOf(fullConfig("/work")).apply(testConfig())

	if cfg.AgentSplitPercent != 70 || cfg.PollSeconds != 45 {
		t.Errorf("numbers = %d/%d, want 70/45", cfg.AgentSplitPercent, cfg.PollSeconds)
	}
	if cfg.WorkshopAgent != (config.AgentModel{Model: "sonnet", Effort: "low"}) {
		t.Errorf("workshop agent = %+v, want the pair typed", cfg.WorkshopAgent)
	}
	if cfg.SliceAgent != (config.AgentModel{Model: "opus", Effort: "high"}) {
		t.Errorf("slice agent = %+v, want the pair typed", cfg.SliceAgent)
	}
	if got := cfg.Projects[testProjectID].WorkingDir; got != "/work" {
		t.Errorf("working dir = %q, want it on the active project", got)
	}
}

func TestSettingsApplyReadsAnEmptyNumberAsUnset(t *testing.T) {
	cfg := Settings{}.apply(fullConfig("/work"))

	if cfg.AgentSplitPercent != 0 || cfg.PollSeconds != 0 {
		t.Errorf("numbers = %d/%d, want a cleared field to read as unset",
			cfg.AgentSplitPercent, cfg.PollSeconds)
	}
	// Unset is what the getters answer with the default for, which is the whole
	// point of writing zero rather than the number they would have returned.
	if got := cfg.SplitPercent(); got != config.DefaultSplitPercent {
		t.Errorf("split = %d, want the default %d", got, config.DefaultSplitPercent)
	}
}

func TestSettingsApplyTrimsAndExpandsWhatWasTyped(t *testing.T) {
	s := Settings{
		WorkingDir:   "  ~/work  ",
		SplitPercent: " 40 ",
		SliceAgent:   config.AgentModel{Model: "  opus  ", Effort: " high "},
	}

	cfg := s.apply(testConfig())

	if cfg.AgentSplitPercent != 40 {
		t.Errorf("split = %d, want the padded number read as 40", cfg.AgentSplitPercent)
	}
	if cfg.SliceAgent != (config.AgentModel{Model: "opus", Effort: "high"}) {
		t.Errorf("slice agent = %+v, want the spaces gone", cfg.SliceAgent)
	}
	if got := cfg.Projects[testProjectID].WorkingDir; strings.HasPrefix(got, "~") || strings.TrimSpace(got) != got {
		t.Errorf("working dir = %q, want it trimmed and the ~ expanded", got)
	}
}

func TestSettingsApplyLeavesTheConfigItWasHandedAlone(t *testing.T) {
	before := testConfig()
	before.Projects[testProjectID] = config.ProjectConfig{Name: "tracker", WorkingDir: "/old"}

	Settings{WorkingDir: "/new"}.apply(before)

	if got := before.Projects[testProjectID].WorkingDir; got != "/old" {
		t.Errorf("the original config's working dir is now %q, want it untouched at /old", got)
	}
}

func TestSettingsApplyWithNoActiveProjectDropsTheDirectory(t *testing.T) {
	cfg := config.Config{}

	got := Settings{WorkingDir: "/work", PollSeconds: "9"}.apply(cfg)

	if len(got.Projects) != 0 {
		t.Errorf("projects = %+v, want no project invented to hold a directory", got.Projects)
	}
	if got.PollSeconds != 9 {
		t.Errorf("poll = %d, want the rest of the settings applied anyway", got.PollSeconds)
	}
}

func TestOptionalNumberFieldValidates(t *testing.T) {
	validate := optionalNumberField("the poll", config.ValidPollSeconds)
	for _, tt := range []struct {
		name, in, want string
	}{
		{"empty is unset", "", ""},
		{"a number in bounds", "60", ""},
		{"spaces around it", "  60  ", ""},
		{"not a number", "soon", "the poll must be a whole number"},
		{"below the bound", "1", "the poll must be between"},
		{"above the bound", "9999", "the poll must be between"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validate(tt.in)
			switch {
			case tt.want == "" && err != nil:
				t.Errorf("err = %v, want %q accepted", err, tt.in)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Errorf("err = %v, want one about %q", err, tt.want)
			}
		})
	}
}

func TestSettingsFormAsksForTheWorkingDirectoryOfTheActiveProject(t *testing.T) {
	f := newSettingsForm(DefaultStyles().FormTheme, fullConfig("/work"))
	f.Init()

	if !f.hasProject {
		t.Fatal("the form should ask for a working directory when a project is configured")
	}
	view := stripANSI(f.View())
	if !strings.Contains(view, "Working directory") || !strings.Contains(view, "tracker") {
		t.Errorf("the form should name the project whose directory it asks for:\n%s", view)
	}
}

func TestSettingsFormWithoutAProjectAsksForNoDirectory(t *testing.T) {
	f := newSettingsForm(DefaultStyles().FormTheme, config.Config{})
	f.Init()

	if f.hasProject {
		t.Fatal("there is no project to hang a working directory on")
	}
	if view := stripANSI(f.View()); strings.Contains(view, "Working directory") {
		t.Errorf("the form should leave the directory off entirely:\n%s", view)
	}
}

func TestSettingsFormNamesAProjectWithoutAName(t *testing.T) {
	cfg := config.Config{ActiveProjectID: "p9", Projects: map[string]config.ProjectConfig{"p9": {}}}

	if got := projectName(cfg); got != "p9" {
		t.Errorf("project name = %q, want the ID as the fallback", got)
	}
}

func TestSettingsFormShowsTheNumbersItWasOpenedOn(t *testing.T) {
	f := newSettingsForm(DefaultStyles().FormTheme, fullConfig("/work"))

	if got := f.settings.SplitPercent; got != "70" {
		t.Errorf("split field = %q, want the config's own 70", got)
	}
	if got := f.Heading(); got != "Settings" {
		t.Errorf("heading = %q, want Settings", got)
	}
	if got := f.busyNote(); got != "" {
		t.Errorf("busy note = %q, want nothing announced for a local write", got)
	}
}

func TestSettingsFormSaveCarriesWhatWasTyped(t *testing.T) {
	f := newSettingsForm(DefaultStyles().FormTheme, fullConfig("/work"))
	f.settings.PollSeconds = "12"

	msg := runMsg(t, f.save(nil)).(settingsSavedMsg)

	if msg.settings.PollSeconds != "12" {
		t.Errorf("saved = %+v, want the edited poll carried through", msg.settings)
	}
}

func TestSettingsKeyOpensTheForm(t *testing.T) {
	a := newWriteApp(&fakeNotion{})

	feed(t, a, press(a, "S"))

	settingsFormOf(t, a)
	if a.screen != screenForm {
		t.Errorf("screen = %v, want the form on show", a.screen)
	}
}

func TestSettingsKeyIsIgnoredWhileAWriteIsInFlight(t *testing.T) {
	a := newWriteApp(&fakeNotion{})
	a.busy = true

	if cmd := a.settingsFlow(); cmd != nil || a.form != nil {
		t.Error("the settings form should not open over a write in flight")
	}
}

func TestSettingsSavedAppliesPersistsAndReports(t *testing.T) {
	a := newWriteApp(&fakeNotion{})
	saved := capturedConfig(t)

	_, cmd := a.settingsSaved(settingsSavedMsg{settings: Settings{
		SplitPercent: "80", PollSeconds: "10",
		SliceAgent: config.AgentModel{Model: "opus"},
	}})
	feed(t, a, cmd)

	if a.cfg.AgentSplitPercent != 80 || a.cfg.SliceAgent.Model != "opus" {
		t.Errorf("config = %+v, want the settings applied to the session", a.cfg)
	}
	if saved.AgentSplitPercent != 80 || saved.PollSeconds != 10 {
		t.Errorf("saved = %+v, want the same numbers written out", saved)
	}
	if a.busy {
		t.Error("the app is still busy after the settings landed")
	}
	if a.err != nil {
		t.Errorf("err = %v, want a clean save", a.err)
	}
}

func TestSettingsSavedResharesTheWindowAtOnce(t *testing.T) {
	a, _, _ := viewerApp(t)
	// The write is caught before it leaves: a settings test that reaches
	// config.Save writes over the config of whoever is running the tests.
	capturedConfig(t)
	before := a.boardWidth()

	_, cmd := a.settingsSaved(settingsSavedMsg{settings: Settings{SplitPercent: "20"}})
	feed(t, a, cmd)

	// A smaller share for the agent is a wider board, without waiting for a
	// resize or a restart.
	if got := a.boardWidth(); got <= before {
		t.Errorf("board is %d columns, want it wider than %d as soon as the split changed", got, before)
	}
}

func TestSettingsSavedReportsAConfigItCannotWrite(t *testing.T) {
	a := newWriteApp(&fakeNotion{})
	failingConfig(t, errors.New("read-only"))

	_, cmd := a.settingsSaved(settingsSavedMsg{settings: Settings{PollSeconds: "10"}})
	feed(t, a, cmd)

	if a.err == nil || !strings.Contains(a.err.Error(), "read-only") {
		t.Errorf("err = %v, want the failed write reported", a.err)
	}
	// The session keeps the change either way: the app is already running on it.
	if a.cfg.PollSeconds != 10 {
		t.Errorf("poll = %d, want the setting still applied to the session", a.cfg.PollSeconds)
	}
}

func TestHelpListsTheSettingsKey(t *testing.T) {
	a := newWriteApp(&fakeNotion{})

	if body := stripANSI(a.helpBody()); !strings.Contains(body, "settings") {
		t.Errorf("the help should name the settings key:\n%s", body)
	}
}

// The whole way through: S opens the form, a number is typed into it, and what
// the last field submits reaches the config file.
func TestSettingsFormEditedAndSubmittedReachesTheConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig()
	cfg.SliceAgent = config.AgentModel{Model: "opus"}
	cfg.Projects[testProjectID] = config.ProjectConfig{Name: "tracker", WorkingDir: dir}
	a := NewApp(cfg, &fakeNotion{})
	saved := capturedConfig(t)

	feed(t, a, press(a, "S"))
	settingsFormOf(t, a)
	// Past the working directory it opened on, then the split typed into the
	// field the config left empty.
	feed(t, a, press(a, "enter"))
	typeText(a, "25")
	stepThrough(t, a)

	if a.form != nil {
		t.Fatalf("the form did not finish:\n%s", stripANSI(a.form.View()))
	}
	if saved.AgentSplitPercent != 25 {
		t.Errorf("saved split = %d, want the typed 25", saved.AgentSplitPercent)
	}
	if got := saved.Projects[testProjectID].WorkingDir; got != dir {
		t.Errorf("saved working dir = %q, want the one the form opened on, %q", got, dir)
	}
	if saved.SliceAgent.Model != "opus" {
		t.Errorf("saved slice agent = %+v, want the pair carried through untouched", saved.SliceAgent)
	}
	if a.cfg.AgentSplitPercent != 25 {
		t.Errorf("session split = %d, want the change applied without a restart", a.cfg.AgentSplitPercent)
	}
}

// A number the config would not keep is refused while the form is still up,
// rather than written and silently defaulted on the next read.
func TestSettingsFormRefusesANumberOutOfBounds(t *testing.T) {
	cfg := testConfig()
	cfg.Projects[testProjectID] = config.ProjectConfig{Name: "tracker", WorkingDir: t.TempDir()}
	a := NewApp(cfg, &fakeNotion{})
	failingConfig(t, errors.New("nothing should be written"))

	feed(t, a, press(a, "S"))
	settingsFormOf(t, a)
	feed(t, a, press(a, "enter"))
	typeText(a, "500")
	stepThrough(t, a)

	if a.form == nil {
		t.Fatal("the form completed on a split of 500%, want it held open on the refusal")
	}
	if view := stripANSI(a.form.View()); !strings.Contains(view, "between") {
		t.Errorf("the form should say what it will accept:\n%s", view)
	}
}
