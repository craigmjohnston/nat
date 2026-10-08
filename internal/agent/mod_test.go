package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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
	launch := LaunchArgs("nat-1", "/tmp", "/tmp/p.md", "Work the slice.", config.AgentModel{}, false)
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
	args := LaunchArgs("nat-1", "/tmp", "/tmp/p.md", "Work the slice.", config.AgentModel{}, false)
	if got := args[slices.Index(args, "sh")+2]; strings.Contains(got, "--plugin-dir") {
		t.Errorf("command = %q, want no --plugin-dir", got)
	}
	if log := read(); !strings.Contains(log, "embedded mod disabled") {
		t.Errorf("log = %q, want the failure logged", log)
	}
}

// briefText stands for a brief: a line that must reach the agent through the
// prompt file and nowhere else.
const briefText = "You are a Claude Code agent working exactly one slice."

// runCommand runs a launch's shell command with a stub claude on PATH that
// prints the NAT_BRIEF it was started with and each argument, one a line.
func runCommand(t *testing.T, command string) string {
	t.Helper()
	bin := t.TempDir()
	stub := "#!/bin/sh\necho \"NAT_BRIEF=${NAT_BRIEF-unset}\"\nfor a in \"$@\"; do echo \"$a\"; done\n"
	write(t, filepath.Join(bin, "claude"), stub)
	if err := os.Chmod(filepath.Join(bin, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run %q: %v\n%s", command, err, out)
	}
	return string(out)
}

// With the mod written, the brief goes by path: claude is started with
// NAT_BRIEF naming the prompt file and the opening line as its one prompt,
// and nothing of the brief is in its argv or the log.
func TestLaunchHandsTheBriefToTheModByPath(t *testing.T) {
	read := logTo(t)
	prompt := filepath.Join(t.TempDir(), "nat-1.md")
	write(t, prompt, briefText)
	opening := `Work the slice "Craig's slice": your brief is the natBrief block of this message.`

	r := &fakeRunner{outs: map[string]string{"new-session": "%7"}}
	if err := NewTmuxWithRunner(r).Launch("nat-1", t.TempDir(), prompt, opening, "slice", config.AgentModel{}); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	args := r.calls[slices.IndexFunc(r.calls, func(c call) bool { return slices.Contains(c.args, "new-session") })].args
	command := args[slices.Index(args, "sh")+2]
	if !strings.Contains(command, "NAT_BRIEF="+shellQuote(prompt)+" claude ") {
		t.Errorf("command = %q, want NAT_BRIEF set to the prompt file on claude", command)
	}
	out := runCommand(t, command)
	if want := "NAT_BRIEF=" + prompt + "\n"; !strings.HasPrefix(out, want) {
		t.Errorf("claude saw %q, want it to start %q", out, want)
	}
	if !strings.HasSuffix(out, "\n"+opening+"\n") {
		t.Errorf("claude saw %q, want the opening line as its last argument", out)
	}
	if strings.Contains(out, briefText) || strings.Contains(command, "$(cat") {
		t.Errorf("claude saw %q, want none of the brief in its argv", out)
	}
	if _, err := os.Stat(prompt); err != nil {
		t.Errorf("the prompt file is gone (%v), want it left for the mod to read", err)
	}
	if log := read(); strings.Contains(log, briefText) {
		t.Errorf("log = %q, want none of the brief in it", log)
	}
}

// With no mod there is nothing to read NAT_BRIEF, so the brief is claude's
// positional prompt, read back from the file as it always was, and the
// opening line goes nowhere.
func TestLaunchWithoutTheModPassesTheBriefInArgv(t *testing.T) {
	// A suite run inside a session nat launched inherits that session's own
	// NAT_BRIEF; this asserts what the command sets, not what it inherited.
	t.Setenv(briefEnv, "")
	_ = os.Unsetenv(briefEnv)
	prompt := filepath.Join(t.TempDir(), "nat-1.md")
	write(t, prompt, briefText)
	command := agentCommand(t.TempDir(), prompt, "Work the slice.", config.AgentModel{}, "", "")
	if want := ` "$(cat ` + shellQuote(prompt) + `)"`; !strings.HasSuffix(command, want) {
		t.Errorf("command = %q, want it to end %q", command, want)
	}
	if out, want := runCommand(t, command), "NAT_BRIEF=unset\n--settings\n"; !strings.HasPrefix(out, want) || !strings.HasSuffix(out, "\n"+briefText+"\n") {
		t.Errorf("claude saw %q, want no NAT_BRIEF and the brief as its prompt", out)
	}
}

// A launch sweeps old mods by every pane's start command: a folder a live
// session was started with stays, one nothing names goes, and a pane read
// that fails removes nothing.
func TestLaunchSweepsOldMods(t *testing.T) {
	isolatedStatusDir(t)
	state, err := logging.Dir()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(state, "mods")
	live, dead := filepath.Join(root, "aaaaaaaaaaaa"), filepath.Join(root, "bbbbbbbbbbbb")
	for _, d := range []string{live, dead} {
		write(t, filepath.Join(d, mods.Name, "hooks", "register.ts"), "")
		age(t, d, time.Hour)
	}
	panes := "sh -c \"claude --plugin-dir '" + filepath.Join(live, mods.Name) + "'\"\n"

	failing := &fakeRunner{outs: map[string]string{"new-session": "%7"}, errs: map[string]error{"list-panes": &ExitError{Code: 1}}}
	if err := NewTmuxWithRunner(failing).LaunchBare("nat-session-1", "/tmp", "session:p:s", config.AgentModel{}); err != nil {
		t.Fatalf("LaunchBare: %v", err)
	}
	if _, err := os.Stat(dead); err != nil {
		t.Errorf("an unread server swept %s: %v", dead, err)
	}

	r := &fakeRunner{outs: map[string]string{"new-session": "%7", "list-panes": panes}}
	if err := NewTmuxWithRunner(r).Launch("nat-1", "/tmp", "/tmp/p.md", "Work the slice.", "slice", config.AgentModel{}); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if _, err := os.Stat(live); err != nil {
		t.Errorf("the live session's mod was swept: %v", err)
	}
	if _, err := os.Stat(dead); !os.IsNotExist(err) {
		t.Errorf("%s is still there (%v), want it swept", dead, err)
	}
}
