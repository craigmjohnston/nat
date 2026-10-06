// Package config handles local configuration: the XDG config file, and the
// Notion bearer token read back from Notion's official CLI.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	appDirName     = "notion-agent-tracker"
	configFileName = "config.json"
)

// marshalIndent is held as a var so tests can stub a marshal failure.
var marshalIndent = json.MarshalIndent

// ProjectConfig describes one tracked project.
type ProjectConfig struct {
	// Name is what the project is called — except a source project's, which
	// is always its plugin's title (read by nat when it names one), so
	// project-create --source writes none and one an older entry carries is
	// never read. Omitted where empty.
	Name string `json:"name,omitempty"`
	// SlicesDSID is the Notion data source the plan is kept in. A project of
	// nat's own has none, so it is omitted rather than written empty.
	SlicesDSID string `json:"slices_ds_id,omitempty"`
	WorkingDir string `json:"working_dir"`
	// Backend is where the plan lives: [BackendLocal], [BackendSource] or, for
	// anything else, Notion. Omitted until it means something, so a config
	// written before there was a choice round-trips unchanged and goes on
	// meaning what it meant.
	Backend string `json:"backend,omitempty"`
	// PlanDir is the directory a local or source project's plan file is kept
	// in, where the user chose one; empty is nat's own data directory.
	// Meaningless for Notion.
	PlanDir string `json:"plan_dir,omitempty"`
	// Source is the task-source plugin a source project's containers come from:
	// the <name> of its nat-source-<name> binary. Omitted for every other
	// project.
	Source string `json:"source,omitempty"`
	// Runs are the project's run commands: a label and a shell command each,
	// offered as one-click runs — the global ones from the latest origin/main,
	// the slice-scoped ones from a handed-back slice's worktree. Omitted until
	// set, so a config written before there were any round-trips unchanged.
	Runs []RunCommand `json:"runs,omitempty"`
	// Color is the project's colour, one of [ProjectColors] by name — never a
	// hex value, so gnat resolves it through whichever palette is on. Empty
	// until the next [Save] picks one ([Config.AssignColors]) or the user does,
	// and always for a project that takes none ([Config.Colorable]).
	Color string `json:"color,omitempty"`
}

// ProjectColors is the palette a project's colour is named from, in the order
// [Config.AssignColors] hands them out.
var ProjectColors = []string{"red", "orange", "yellow", "green", "teal", "blue", "purple", "pink"}

// ValidProjectColor says whether name is one of [ProjectColors].
func ValidProjectColor(name string) bool {
	return slices.Contains(ProjectColors, name)
}

// Colorable reports whether the project id names takes a colour: every one
// but the scratch project and a source project, which are not drawn as
// projects of the user's own.
func (c Config) Colorable(id string) bool {
	p, ok := c.Projects[id]
	return ok && !p.IsSource() && id != c.ScratchProject
}

// AssignColors gives every project with no colour one: entries walked in ID
// order, each given the palette name the fewest other entries already hold,
// a tie going to the earlier name in palette order. An entry already coloured
// is never changed — except one that takes no colour ([Config.Colorable]),
// whose colour is cleared and counts against nothing: scratch-open saves its
// project before recording it as the scratch one, and the second save takes
// back what the first gave. It writes into c.Projects in place, so a caller
// holding the same map sees what was assigned.
func (c *Config) AssignColors() {
	held := map[string]int{}
	var bare []string
	for id, p := range c.Projects {
		if !c.Colorable(id) {
			if p.Color != "" {
				p.Color = ""
				c.Projects[id] = p
			}
			continue
		}
		if p.Color == "" {
			bare = append(bare, id)
			continue
		}
		held[p.Color]++
	}
	slices.Sort(bare)
	for _, id := range bare {
		pick := ProjectColors[0]
		for _, name := range ProjectColors[1:] {
			if held[name] < held[pick] {
				pick = name
			}
		}
		held[pick]++
		p := c.Projects[id]
		p.Color = pick
		c.Projects[id] = p
	}
}

// RunCommand is one of a project's run commands: Label is what its button
// says, as short as it can be and unique within the project; Command is run
// by `sh -c`, exactly as written, in the directory its scope says; Scope is
// [RunScopeGlobal], [RunScopeSlice] or empty — both.
type RunCommand struct {
	Label   string `json:"label"`
	Command string `json:"command"`
	Scope   string `json:"scope,omitempty"`
}

// The two scope words a run command may carry; the empty string is both.
const (
	RunScopeGlobal = "global"
	RunScopeSlice  = "slice"
)

// GlobalRuns are the project's runs offered from origin/main, in the order
// written — the first being the default.
func (p ProjectConfig) GlobalRuns() []RunCommand { return p.runsIn(RunScopeGlobal) }

// SliceRuns are the project's runs offered on a handed-back slice, in the
// order written — the first being the default.
func (p ProjectConfig) SliceRuns() []RunCommand { return p.runsIn(RunScopeSlice) }

// runsIn filters the runs to those of one scope, a scopeless run being both.
func (p ProjectConfig) runsIn(scope string) []RunCommand {
	var out []RunCommand
	for _, r := range p.Runs {
		if r.Scope == "" || r.Scope == scope {
			out = append(out, r)
		}
	}
	return out
}

// ValidRuns says whether a list of run commands is one the config would keep:
// every label and command non-empty, every scope one of the two words or none,
// and no label offered twice in one place — case ignored, since two buttons
// reading Run and run are one too many. A scopeless run is offered in both
// places, so a label may appear once scopeless, or once per scope: the scoped
// pair is how a run says it does something different in a slice's worktree.
func ValidRuns(runs []RunCommand) error {
	seen := map[string]bool{}
	for i, r := range runs {
		label := strings.TrimSpace(r.Label)
		switch {
		case label == "":
			return fmt.Errorf("run %d has no label", i+1)
		case strings.TrimSpace(r.Command) == "":
			return fmt.Errorf("run %q has no command", label)
		case r.Scope != "" && r.Scope != RunScopeGlobal && r.Scope != RunScopeSlice:
			return fmt.Errorf("run %q has scope %q: want %s, %s or none (both)", label, r.Scope, RunScopeGlobal, RunScopeSlice)
		}
		for _, scope := range []string{RunScopeGlobal, RunScopeSlice} {
			if r.Scope != "" && r.Scope != scope {
				continue
			}
			key := scope + "\x00" + strings.ToLower(label)
			if seen[key] {
				return fmt.Errorf("two %s runs are labelled %q: a label names one run in each place", scope, label)
			}
			seen[key] = true
		}
	}
	return nil
}

// The three places a plan can live.
const (
	BackendNotion = "notion"
	BackendLocal  = "local"
	BackendSource = "source"
)

// IsLocal reports whether the plan is a file of nat's own with no workspace
// and no task source behind it. Only the local word says so.
func (p ProjectConfig) IsLocal() bool { return p.Backend == BackendLocal }

// IsSource reports whether the plan is a file of nat's own whose milestones
// are a task-source plugin's containers. Only the source word says so.
//
// The local and source words are the only two that open a file: a backend a
// later nat invented is one this build cannot open a file for, and anything
// else — the empty string included — reads as Notion.
func (p ProjectConfig) IsSource() bool { return p.Backend == BackendSource }

// BackendName is the backend as it is said out loud: always one of the three
// words, even for a project whose entry leaves it unwritten.
func (p ProjectConfig) BackendName() string {
	switch {
	case p.IsLocal():
		return BackendLocal
	case p.IsSource():
		return BackendSource
	}
	return BackendNotion
}

// UsesNotion reports whether anything this machine tracks is kept in Notion,
// which is what decides whether a Notion credential is needed at all: a
// project whose plan is in Notion, or — with no project yet — the projects
// database that projects would be made in. A config of local and source
// projects alone needs none.
func (c Config) UsesNotion() bool {
	if len(c.Projects) == 0 {
		return c.ProjectDBDataSourceID != ""
	}
	for _, p := range c.Projects {
		if !p.IsLocal() && !p.IsSource() {
			return true
		}
	}
	return false
}

// AssigneeFor is who works a project's slices: the workspace user onboarding
// resolved for a project in Notion. A plan of its own — local or source — has
// no directory of users, so there the name is the identity — the configured
// name where one is set, else whoever is logged in where the config names
// nobody. Both are empty only where neither exists, which the callers already
// refuse.
func (c Config) AssigneeFor(p ProjectConfig) (id, name string) {
	if !p.IsLocal() && !p.IsSource() {
		return c.AssigneeUserID, c.AssigneeUserName
	}
	name = c.AssigneeUserName
	if name == "" {
		name = loggedIn()
	}
	return name, name
}

// currentUser is held as a variable so a test can stand in for the OS.
var currentUser = user.Current

// loggedIn is the name of whoever is logged in on this machine: their full
// name where the OS keeps one, else their login. Empty where it cannot be read.
func loggedIn() string {
	u, err := currentUser()
	if err != nil {
		return ""
	}
	if u.Name != "" {
		return u.Name
	}
	return u.Username
}

// AgentModel is which Claude Code an agent is launched as: the model and the
// effort level, exactly as the CLI's own --model and --effort take them. Both
// are optional and empty means unset — the flag is left off and Claude Code
// falls back to whatever the user's own configuration says, which is what
// every launch did before this existed.
type AgentModel struct {
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
}

// Config is the local configuration persisted as JSON in the XDG config dir.
type Config struct {
	ProjectDBID           string `json:"project_db_id"`
	ProjectDBDataSourceID string `json:"project_db_data_source_id"`
	AssigneeUserID        string `json:"assignee_user_id"`
	AssigneeUserName      string `json:"assignee_user_name"`
	ActiveProjectID       string `json:"active_project_id"`
	// AgentSplitPercent is how much of the window an agent's pane takes when it
	// is shown beside the board. Hand-written, and omitted until it is: unset
	// means [DefaultSplitPercent].
	AgentSplitPercent int `json:"agent_split_percent,omitempty"`
	// PollSeconds is how often the board refetches the plan on its own, for the
	// changes no nudge marker reports: the ones made in Notion itself. Written
	// by hand like the split, and omitted until it is — unset means
	// [DefaultPollSeconds].
	PollSeconds int `json:"poll_seconds,omitempty"`
	// WorkshopAgent and SliceAgent are what a planning agent and a slice's
	// agent are launched as. They are two settings rather than one because the
	// two jobs are not the same size: workshopping a plan is conversation, and
	// often wants a lighter model than the agent that goes and writes the code.
	// Hand-written like the split and the poll, and omitted until they are.
	WorkshopAgent AgentModel               `json:"workshop_agent,omitzero"`
	SliceAgent    AgentModel               `json:"slice_agent,omitzero"`
	Projects      map[string]ProjectConfig `json:"projects"`
	// ScratchProject is the ID of the reserved local project ad hoc work lives
	// in, recorded by `nat scratch-open` the first time it runs. It is one of
	// Projects like any other; this only says which. Omitted until then.
	ScratchProject string `json:"scratch_project,omitempty"`
	// PluginSources are the GitHub repositories, as owner/repo, task-source
	// plugins may be installed from besides nat's own — which is always
	// read first and is never written here (see internal/plugins). Omitted
	// until one is added.
	PluginSources []string `json:"plugin_sources,omitempty"`
}

// The share of the window an agent's pane takes beside the board. The default
// leaves the board enough for a slice name and its markers while giving the
// agent the room, which is where the reading happens; the bounds are what keeps
// a hand-edited config from producing a pane too narrow to use — either way
// round.
const (
	DefaultSplitPercent = 65
	minSplitPercent     = 10
	maxSplitPercent     = 90
)

// SplitPercent is the width to give an agent's pane, as the config asks for it
// or the default when it does not — a value outside the bounds being a typo
// rather than an instruction.
func (c Config) SplitPercent() int {
	if c.AgentSplitPercent == 0 || ValidSplitPercent(c.AgentSplitPercent) != nil {
		return DefaultSplitPercent
	}
	return c.AgentSplitPercent
}

// ValidSplitPercent says whether a split is one the config would keep, so a
// form can refuse a typo while the user is still looking at it rather than
// writing a number the next read silently swaps the default back in for. Zero
// is valid and is what "unset" is written as: the bounds only describe the
// numbers that mean something.
func ValidSplitPercent(v int) error {
	if v == 0 {
		return nil
	}
	if v < minSplitPercent || v > maxSplitPercent {
		return fmt.Errorf("the agent's share must be between %d%% and %d%%", minSplitPercent, maxSplitPercent)
	}
	return nil
}

// How often the board refetches the plan by itself. Half a minute is often
// enough that a status changed in Notion shows up while the user is still
// looking for it, and rare enough to cost nothing: the writes made through nat
// itself are reported by the nudge marker within the second, and this is only
// for the rest. The bounds are what keeps a hand-edited config from polling
// Notion every second, or from a poll so far off it reads as none at all.
const (
	DefaultPollSeconds = 30
	minPollSeconds     = 5
	maxPollSeconds     = 3600
)

// PollInterval is how long the board waits between refetches, as the config
// asks for it or the default when it does not — a value outside the bounds
// being a typo rather than an instruction.
func (c Config) PollInterval() time.Duration {
	if c.PollSeconds == 0 || ValidPollSeconds(c.PollSeconds) != nil {
		return DefaultPollSeconds * time.Second
	}
	return time.Duration(c.PollSeconds) * time.Second
}

// ValidPollSeconds is the poll's half of [ValidSplitPercent], and reads the
// same way: zero is unset, and everything else has to be a poll the config
// would actually keep.
func ValidPollSeconds(v int) error {
	if v == 0 {
		return nil
	}
	if v < minPollSeconds || v > maxPollSeconds {
		return fmt.Errorf("the poll must be between %d and %d seconds", minPollSeconds, maxPollSeconds)
	}
	return nil
}

// Dir returns the app's config directory: $XDG_CONFIG_HOME/notion-agent-tracker,
// falling back to ~/.config/notion-agent-tracker when XDG_CONFIG_HOME is unset
// or empty. XDG resolution is hand-rolled deliberately: os.UserConfigDir uses
// ~/Library/Application Support on macOS, but our config lives under ~/.config
// on every platform.
func Dir() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, appDirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", appDirName), nil
}

// Path returns the full path of the config file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// Load reads the config file. A missing file is not an error: it returns a
// zero Config with found=false so the caller can start onboarding. The file
// may be hand-written; unknown fields are ignored.
func Load() (Config, bool, error) {
	path, err := Path()
	if err != nil {
		return Config{}, false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, false, fmt.Errorf("parse config %s: %w", path, err)
	}
	return c, true, nil
}

// Save writes the config file with mode 0644, creating the config directory
// if needed. Every project with no colour is given one first
// ([Config.AssignColors]) — here rather than on [Load], since a read never
// writes and a colour held only in memory is one gnat, which reads the file,
// never sees. The caller's map is filled in too.
func Save(c Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	c.AssignColors()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := marshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
