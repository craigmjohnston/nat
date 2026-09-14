package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/agent"
)

// usageFakeRunner drives a fake tmux for the usage probe: launching and
// killing the session always succeed, and pasting the prompt (the last of
// the three calls [agent.Tmux.SendPrompt] makes) is the moment a real Claude
// Code session would eventually answer — so that is where this fake writes
// the sink file, simulating the statusline command's own write.
type usageFakeRunner struct {
	sinkPath    string
	sinkContent string
	writeErr    error
	// launchErr fails the probe's own launch (new-session).
	launchErr error
	// sendPromptErr fails SendPrompt at its first call (set-buffer).
	sendPromptErr error
	calls         []usageCall
}

// usageCall is one invocation the fake tmux runner recorded.
type usageCall struct {
	name string
	args []string
}

func (f *usageFakeRunner) Run(name string, args ...string) (string, error) {
	f.calls = append(f.calls, usageCall{name: name, args: args})
	if len(args) > 0 && args[0] == "-u" {
		args = args[1:]
	}
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "new-session":
		return "", f.launchErr
	case "set-buffer":
		return "", f.sendPromptErr
	case "paste-buffer":
		if f.sinkContent != "" {
			if err := os.WriteFile(f.sinkPath, []byte(f.sinkContent), 0o600); err != nil {
				return "", err
			}
		}
		return "", f.writeErr
	}
	return "", nil
}

// setUsageTestKnobs points every var probeUsage reads at fast, isolated
// stand-ins, restoring each on the test's cleanup.
func setUsageTestKnobs(t *testing.T, dir string) {
	t.Helper()
	origDirFunc, origSleep, origSettle := usageProbeDirFunc, usageSleep, usagePromptSettleWait
	usageProbeDirFunc = func() (string, error) { return dir, nil }
	usageSleep = func(d time.Duration) {}
	usagePromptSettleWait = 0
	t.Cleanup(func() {
		usageProbeDirFunc, usageSleep, usagePromptSettleWait = origDirFunc, origSleep, origSettle
	})
}

func TestUsagePrintsReadingAsJSON(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	sink := agent.UsageProbeSinkPath(dir)
	runner := &usageFakeRunner{
		sinkPath: sink,
		sinkContent: `{
			"transcript_path": "` + filepath.Join(dir, "transcript.jsonl") + `",
			"rate_limits": {
				"five_hour": {"used_percentage": 38, "resets_at": 1000},
				"seven_day": {"used_percentage": 81, "resets_at": 2000}
			}
		}`,
	}
	if err := os.WriteFile(filepath.Join(dir, "transcript.jsonl"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("seed transcript: %v", err)
	}

	env := Env{
		NewTmux: func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) },
		Out:     &strings.Builder{},
	}
	if err := Run(context.Background(), []string{"usage", "--json"}, env); err != nil {
		t.Fatalf("usage: %v", err)
	}

	var doc usageJSON
	if err := json.Unmarshal([]byte(env.Out.(*strings.Builder).String()), &doc); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if doc.FiveHour == nil || doc.FiveHour.UsedPercentage != 38 || doc.FiveHour.ResetsAt != 1000 {
		t.Errorf("five_hour = %+v, want 38%% resetting at 1000", doc.FiveHour)
	}
	if doc.SevenDay == nil || doc.SevenDay.UsedPercentage != 81 || doc.SevenDay.ResetsAt != 2000 {
		t.Errorf("seven_day = %+v, want 81%% resetting at 2000", doc.SevenDay)
	}

	// The probe cleans up after itself: no sink and no transcript left.
	if _, err := os.Stat(sink); !os.IsNotExist(err) {
		t.Errorf("sink file left behind: err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "transcript.jsonl")); !os.IsNotExist(err) {
		t.Errorf("transcript file left behind: err = %v", err)
	}

	// Kill is called both before the launch (clearing any stale session) and
	// after (cleaning up this one).
	kills := 0
	for _, c := range runner.calls {
		if len(c.args) > 1 && c.args[1] == "kill-session" {
			kills++
		}
	}
	if kills != 2 {
		t.Errorf("kill-session calls = %d, want 2", kills)
	}
}

func TestUsagePrintsMarkdown(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	sink := agent.UsageProbeSinkPath(dir)
	runner := &usageFakeRunner{
		sinkPath: sink,
		sinkContent: `{
			"rate_limits": {"five_hour": {"used_percentage": 38, "resets_at": 1700000000}}
		}`,
	}

	env := Env{
		NewTmux: func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) },
		Out:     &strings.Builder{},
	}
	if err := Run(context.Background(), []string{"usage"}, env); err != nil {
		t.Fatalf("usage: %v", err)
	}
	out := env.Out.(*strings.Builder).String()
	if !strings.Contains(out, "Session 38%") {
		t.Errorf("output = %q, want it to mention Session 38%%", out)
	}
}

func TestUsageUnavailableOnTimeout(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	// No sinkContent: the fake never writes the file, so the poll runs out
	// the clock — sped up by shrinking the timeout itself for the test.
	origTimeout := usageProbeTimeout
	usageProbeTimeout = 0
	t.Cleanup(func() { usageProbeTimeout = origTimeout })

	runner := &usageFakeRunner{sinkPath: agent.UsageProbeSinkPath(dir)}
	env := Env{
		NewTmux: func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) },
		Out:     &strings.Builder{},
	}
	if err := Run(context.Background(), []string{"usage"}, env); err != nil {
		t.Fatalf("usage: %v", err)
	}
	if got := env.Out.(*strings.Builder).String(); got != "usage unavailable\n" {
		t.Errorf("output = %q, want %q", got, "usage unavailable\n")
	}
}

func TestUsageUnavailableOnLaunchFailure(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	runner := &usageFakeRunner{sinkPath: agent.UsageProbeSinkPath(dir), launchErr: errors.New("no tmux")}
	env := Env{
		NewTmux: func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) },
		Out:     &strings.Builder{},
	}
	if err := Run(context.Background(), []string{"usage"}, env); err != nil {
		t.Fatalf("usage: %v", err)
	}
	if got := env.Out.(*strings.Builder).String(); got != "usage unavailable\n" {
		t.Errorf("output = %q, want %q", got, "usage unavailable\n")
	}
}

func TestUsageRejectsBadFlags(t *testing.T) {
	env := Env{Out: &strings.Builder{}}
	if err := Run(context.Background(), []string{"usage", "--bogus"}, env); err == nil {
		t.Fatal("usage: want an error for an unknown flag")
	}
}

func TestUsageUnavailableWhenDirCannotBeResolved(t *testing.T) {
	origDirFunc := usageProbeDirFunc
	usageProbeDirFunc = func() (string, error) { return "", errors.New("no home directory") }
	t.Cleanup(func() { usageProbeDirFunc = origDirFunc })

	env := Env{Out: &strings.Builder{}}
	if err := Run(context.Background(), []string{"usage"}, env); err != nil {
		t.Fatalf("usage: %v", err)
	}
	if got := env.Out.(*strings.Builder).String(); got != "usage unavailable\n" {
		t.Errorf("output = %q, want %q", got, "usage unavailable\n")
	}
}

func TestUsageUnavailableWhenDirCannotBeCreated(t *testing.T) {
	// A file sitting where the scratch directory needs to be turns
	// MkdirAll into ENOTDIR rather than a successful create.
	parent := t.TempDir()
	blocked := filepath.Join(parent, "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed a file in place of the scratch dir: %v", err)
	}
	setUsageTestKnobs(t, filepath.Join(blocked, "usage-probe"))

	env := Env{
		NewTmux: func() *agent.Tmux { return agent.NewTmuxWithRunner(&usageFakeRunner{}) },
		Out:     &strings.Builder{},
	}
	if err := Run(context.Background(), []string{"usage"}, env); err != nil {
		t.Fatalf("usage: %v", err)
	}
	if got := env.Out.(*strings.Builder).String(); got != "usage unavailable\n" {
		t.Errorf("output = %q, want %q", got, "usage unavailable\n")
	}
}

func TestUsageUnavailableWhenSettingsCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	// A directory sitting where the settings file needs to be written turns
	// the write into EISDIR.
	if err := os.Mkdir(filepath.Join(dir, "settings.json"), 0o700); err != nil {
		t.Fatalf("seed a directory in place of the settings file: %v", err)
	}

	env := Env{
		NewTmux: func() *agent.Tmux { return agent.NewTmuxWithRunner(&usageFakeRunner{}) },
		Out:     &strings.Builder{},
	}
	if err := Run(context.Background(), []string{"usage"}, env); err != nil {
		t.Fatalf("usage: %v", err)
	}
	if got := env.Out.(*strings.Builder).String(); got != "usage unavailable\n" {
		t.Errorf("output = %q, want %q", got, "usage unavailable\n")
	}
}

func TestUsageUnavailableWhenPromptFails(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	runner := &usageFakeRunner{sinkPath: agent.UsageProbeSinkPath(dir), sendPromptErr: errors.New("no such session")}
	env := Env{
		NewTmux: func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) },
		Out:     &strings.Builder{},
	}
	if err := Run(context.Background(), []string{"usage"}, env); err != nil {
		t.Fatalf("usage: %v", err)
	}
	if got := env.Out.(*strings.Builder).String(); got != "usage unavailable\n" {
		t.Errorf("output = %q, want %q", got, "usage unavailable\n")
	}
}

func TestUsageUnavailableOnInvalidSinkPayload(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	runner := &usageFakeRunner{sinkPath: agent.UsageProbeSinkPath(dir), sinkContent: "not json"}
	env := Env{
		NewTmux: func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) },
		Out:     &strings.Builder{},
	}
	if err := Run(context.Background(), []string{"usage"}, env); err != nil {
		t.Fatalf("usage: %v", err)
	}
	if got := env.Out.(*strings.Builder).String(); got != "usage unavailable\n" {
		t.Errorf("output = %q, want %q", got, "usage unavailable\n")
	}
}

func TestUsageWaitsAcrossMoreThanOnePoll(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	sink := agent.UsageProbeSinkPath(dir)
	content := `{"rate_limits": {"five_hour": {"used_percentage": 5, "resets_at": 1000}}}`

	sleepCalls := 0
	usageSleep = func(time.Duration) {
		sleepCalls++
		// The first call is the pre-prompt settle wait; the sink is written
		// only once a poll (the second call onward) has actually happened,
		// so waitForUsageSink is exercised at least once around its loop.
		if sleepCalls == 2 {
			if err := os.WriteFile(sink, []byte(content), 0o600); err != nil {
				t.Fatalf("write sink: %v", err)
			}
		}
	}

	runner := &usageFakeRunner{sinkPath: sink}
	env := Env{
		NewTmux: func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) },
		Out:     &strings.Builder{},
	}
	if err := Run(context.Background(), []string{"usage", "--json"}, env); err != nil {
		t.Fatalf("usage: %v", err)
	}
	if sleepCalls < 2 {
		t.Fatalf("sleepCalls = %d, want at least 2 (settle wait + a poll)", sleepCalls)
	}
	var doc usageJSON
	if err := json.Unmarshal([]byte(env.Out.(*strings.Builder).String()), &doc); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if doc.FiveHour == nil || doc.FiveHour.UsedPercentage != 5 {
		t.Errorf("five_hour = %+v, want 5%%", doc.FiveHour)
	}
}

func TestUsageMarkdownIncludesTheWeekWindow(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	sink := agent.UsageProbeSinkPath(dir)
	runner := &usageFakeRunner{
		sinkPath:    sink,
		sinkContent: `{"rate_limits": {"seven_day": {"used_percentage": 81, "resets_at": 1700000000}}}`,
	}
	env := Env{
		NewTmux: func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) },
		Out:     &strings.Builder{},
	}
	if err := Run(context.Background(), []string{"usage"}, env); err != nil {
		t.Fatalf("usage: %v", err)
	}
	if got := env.Out.(*strings.Builder).String(); !strings.Contains(got, "Week 81%") {
		t.Errorf("output = %q, want it to mention Week 81%%", got)
	}
}

func TestUsageRejectsArguments(t *testing.T) {
	env := Env{Out: &strings.Builder{}}
	if err := Run(context.Background(), []string{"usage", "extra"}, env); err == nil {
		t.Fatal("usage: want an error for an unexpected argument")
	}
}

func TestUsageNoRateLimitsIsUnavailable(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	sink := agent.UsageProbeSinkPath(dir)
	runner := &usageFakeRunner{sinkPath: sink, sinkContent: `{"transcript_path": ""}`}
	env := Env{
		NewTmux: func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) },
		Out:     &strings.Builder{},
	}
	if err := Run(context.Background(), []string{"usage", "--json"}, env); err != nil {
		t.Fatalf("usage: %v", err)
	}
	if got := env.Out.(*strings.Builder).String(); strings.TrimSpace(got) != "{}" {
		t.Errorf("output = %q, want an empty object", got)
	}
}
