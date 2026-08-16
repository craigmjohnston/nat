// Package config handles local configuration: the XDG config file, and the
// Notion bearer token read back from Notion's official CLI.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	Name       string `json:"name"`
	SlicesDSID string `json:"slices_ds_id"`
	WorkingDir string `json:"working_dir"`
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
	if c.AgentSplitPercent < minSplitPercent || c.AgentSplitPercent > maxSplitPercent {
		return DefaultSplitPercent
	}
	return c.AgentSplitPercent
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
	if c.PollSeconds < minPollSeconds || c.PollSeconds > maxPollSeconds {
		return DefaultPollSeconds * time.Second
	}
	return time.Duration(c.PollSeconds) * time.Second
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
// if needed.
func Save(c Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
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
