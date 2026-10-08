package agent

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/mods"
)

// modFlag is the --plugin-dir flag a launch under the test's state directory
// carries.
func modFlag(t *testing.T) string {
	t.Helper()
	dir, err := mods.Materialise()
	if err != nil {
		t.Fatalf("Materialise: %v", err)
	}
	return " --plugin-dir " + shellQuote(dir)
}

// Every agent launch — prompted or bare — loads nat's mod for that one
// session; the usage probe, which is no agent, does not.
func TestLaunchesCarryThePluginDir(t *testing.T) {
	isolatedStatusDir(t)
	dir, err := mods.Materialise()
	if err != nil {
		t.Fatalf("Materialise: %v", err)
	}
	if filepath.Base(dir) != mods.Name {
		t.Errorf("mod dir = %q, want it named %s", dir, mods.Name)
	}
	flag := "--plugin-dir " + shellQuote(dir)
	launch := LaunchArgs("nat-1", "/tmp", "/tmp/p.md", config.AgentModel{}, false)
	bare := bareLaunchArgs("nat-1", "/tmp", config.AgentModel{}, false, "", prepareMod())
	for name, args := range map[string][]string{"launch": launch, "bare": bare} {
		if got := args[slices.Index(args, "sh")+2]; !strings.Contains(got, flag) {
			t.Errorf("%s command = %q, want it to carry %q", name, got, flag)
		}
	}
	if got := usageProbeCommand("/tmp", "/tmp/settings.json"); strings.Contains(got, "--plugin-dir") {
		t.Errorf("usage probe command = %q, want no --plugin-dir", got)
	}
}

// A mod that cannot be written is logged and the agent launches without it.
func TestLaunchWithoutTheModWhenItCannotBeWritten(t *testing.T) {
	read := logTo(t)
	state, err := logging.Dir()
	if err != nil {
		t.Fatal(err)
	}
	// The mods directory's own place is a file: nothing can be made there.
	write(t, filepath.Join(state, "mods"), "in the way")
	if got := prepareMod(); got != "" {
		t.Errorf("prepareMod = %q, want none", got)
	}
	args := LaunchArgs("nat-1", "/tmp", "/tmp/p.md", config.AgentModel{}, false)
	if got := args[slices.Index(args, "sh")+2]; strings.Contains(got, "--plugin-dir") {
		t.Errorf("command = %q, want no --plugin-dir", got)
	}
	if log := read(); !strings.Contains(log, "embedded mod disabled") {
		t.Errorf("log = %q, want the failure logged", log)
	}
}
