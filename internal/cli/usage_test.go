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

// usageFakeRunner drives a fake tmux for the usage probe. The probe never
// pastes a prompt — it types "/usage" then Enter, waits, and sends Escape to
// close the panel — so this fake reacts to those literal send-keys calls
// instead of SendPrompt's paste-buffer/set-buffer pair.
type usageFakeRunner struct {
	sinkPath string
	// launchErr fails the probe's own launch (new-session).
	launchErr error
	// typeErr fails the send-keys call that types "/usage".
	typeErr error
	// closeErr fails the first Escape (the deliberate panel close).
	closeErr error
	// readyContent, when non-empty, is written to sinkPath the
	// readyOnEscape'th time Escape is sent (1 = the deliberate close, 2 =
	// the half-timeout retry). 0 (the default) never writes it.
	readyContent  string
	readyOnEscape int

	escapeCount int
	calls       []usageCall
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
	case "send-keys":
		last := args[len(args)-1]
		switch last {
		case "Escape":
			f.escapeCount++
			if f.escapeCount == 1 && f.closeErr != nil {
				return "", f.closeErr
			}
			if f.readyOnEscape != 0 && f.escapeCount == f.readyOnEscape && f.readyContent != "" {
				if err := os.WriteFile(f.sinkPath, []byte(f.readyContent), 0o600); err != nil {
					return "", err
				}
			}
			return "", nil
		case "Enter":
			return "", nil
		default:
			// The literal-text call typing "/usage" in.
			return "", f.typeErr
		}
	}
	return "", nil
}

// usageClock is a fake, manually advanced clock: usageNow reads it, and the
// shared test knob's usageSleep advances it by whatever duration the probe
// asks to wait on — so a 30-second timeout's poll loop runs to completion in
// real time without ever depending on the wall clock.
type usageClock struct{ now time.Time }

func (c *usageClock) Now() time.Time         { return c.now }
func (c *usageClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

// setUsageTestKnobs points every var probeUsage reads at fast, isolated
// stand-ins, restoring each on the test's cleanup, and returns the fake
// clock usageNow now reads so a test can advance it further itself.
func setUsageTestKnobs(t *testing.T, dir string) *usageClock {
	t.Helper()
	origDirFunc, origSleep, origNow := usageProbeDirFunc, usageSleep, usageNow
	origSettle, origPanel := usagePromptSettleWait, usagePanelWait
	usageProbeDirFunc = func() (string, error) { return dir, nil }
	usagePromptSettleWait = 0
	usagePanelWait = 0
	clock := &usageClock{now: time.Now()}
	usageNow = clock.Now
	usageSleep = func(d time.Duration) { clock.Advance(d) }
	t.Cleanup(func() {
		usageProbeDirFunc, usageSleep, usageNow = origDirFunc, origSleep, origNow
		usagePromptSettleWait, usagePanelWait = origSettle, origPanel
	})
	return clock
}

func TestUsagePrintsReadingAsJSON(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	sink := agent.UsageProbeSinkPath(dir)
	transcript := filepath.Join(dir, "transcript.jsonl")
	runner := &usageFakeRunner{
		sinkPath:      sink,
		readyOnEscape: 1,
		readyContent: `{
			"transcript_path": "` + transcript + `",
			"rate_limits": {
				"five_hour": {"used_percentage": 38, "resets_at": 1000},
				"seven_day": {"used_percentage": 81, "resets_at": 2000}
			}
		}`,
	}
	if err := os.WriteFile(transcript, []byte("{}"), 0o600); err != nil {
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
	if _, err := os.Stat(transcript); !os.IsNotExist(err) {
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
		sinkPath: sink, readyOnEscape: 1,
		readyContent: `{"rate_limits": {"five_hour": {"used_percentage": 38, "resets_at": 1700000000}}}`,
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

// TestUsageDrivesUsageCommandNeverAPrompt is the acceptance test that the
// probe's tmux calls are exactly typing "/usage", Enter, and one Escape —
// and that SendPrompt (paste-buffer/set-buffer) is never called at all.
func TestUsageDrivesUsageCommandNeverAPrompt(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	sink := agent.UsageProbeSinkPath(dir)
	runner := &usageFakeRunner{
		sinkPath: sink, readyOnEscape: 1,
		readyContent: `{"rate_limits": {"five_hour": {"used_percentage": 1, "resets_at": 1}}}`,
	}
	env := Env{
		NewTmux: func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) },
		Out:     &strings.Builder{},
	}
	if err := Run(context.Background(), []string{"usage"}, env); err != nil {
		t.Fatalf("usage: %v", err)
	}

	var typed, entered, escaped int
	for _, c := range runner.calls {
		if len(c.args) == 0 {
			continue
		}
		switch c.args[0] {
		case "set-buffer", "paste-buffer":
			t.Fatalf("SendPrompt's own calls were made: %v", c.args)
		}
		switch c.args[len(c.args)-1] {
		case "/usage":
			typed++
		case "Enter":
			entered++
		case "Escape":
			escaped++
		}
	}
	if typed != 1 || entered != 1 || escaped != 1 {
		t.Errorf("typed=%d entered=%d escaped=%d, want 1 each", typed, entered, escaped)
	}
}

func TestUsageRetriesEscapeAtHalfTimeout(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	sink := agent.UsageProbeSinkPath(dir)

	origTimeout := usageProbeTimeout
	usageProbeTimeout = 4 * usagePollInterval
	t.Cleanup(func() { usageProbeTimeout = origTimeout })

	runner := &usageFakeRunner{
		sinkPath: sink,
		// The reading appears only once Escape has been sent a second
		// time — the half-timeout retry this test exercises, never the
		// first, deliberate close.
		readyOnEscape: 2,
		readyContent:  `{"rate_limits": {"five_hour": {"used_percentage": 7, "resets_at": 900}}}`,
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
	if doc.FiveHour == nil || doc.FiveHour.UsedPercentage != 7 {
		t.Errorf("five_hour = %+v, want 7%%", doc.FiveHour)
	}
	if runner.escapeCount != 2 {
		t.Errorf("Escape sent %d times, want 2 (the close, then the half-timeout retry)", runner.escapeCount)
	}
}

// TestUsageSkipsStartupPayloadUntilRateLimitsAppear covers the sink-polling
// race this slice fixes: Claude Code's own statusline write at startup is
// valid JSON with no rate-limit window at all, and must not be read back as
// the answer — only a later payload that actually carries one may be.
func TestUsageSkipsStartupPayloadUntilRateLimitsAppear(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	sink := agent.UsageProbeSinkPath(dir)
	if err := os.WriteFile(sink, []byte(`{"rate_limits": {}}`), 0o600); err != nil {
		t.Fatalf("seed the startup payload: %v", err)
	}

	polls := 0
	origSleep := usageSleep
	usageSleep = func(d time.Duration) {
		origSleep(d)
		if d != usagePollInterval {
			return
		}
		polls++
		if polls == 1 {
			content := `{"rate_limits": {"five_hour": {"used_percentage": 12, "resets_at": 500}}}`
			if err := os.WriteFile(sink, []byte(content), 0o600); err != nil {
				t.Fatalf("write the real reading: %v", err)
			}
		}
	}
	t.Cleanup(func() { usageSleep = origSleep })

	runner := &usageFakeRunner{sinkPath: sink}
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
	if doc.FiveHour == nil || doc.FiveHour.UsedPercentage != 12 {
		t.Errorf("five_hour = %+v, want 12%%", doc.FiveHour)
	}
	if polls < 1 {
		t.Fatalf("polls = %d, want at least 1 (the startup payload must be read and skipped)", polls)
	}
}

func TestUsageUnavailableOnTimeout(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	// No readyContent: the fake never writes a qualifying payload, so the
	// poll runs out the clock — sped up by shrinking the timeout itself.
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

func TestUsageUnavailableWhenOpeningThePanelFails(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	runner := &usageFakeRunner{sinkPath: agent.UsageProbeSinkPath(dir), typeErr: errors.New("no such session")}
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

func TestUsageUnavailableWhenClosingThePanelFails(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	runner := &usageFakeRunner{sinkPath: agent.UsageProbeSinkPath(dir), closeErr: errors.New("no such session")}
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
	origTimeout := usageProbeTimeout
	usageProbeTimeout = 2 * usagePollInterval
	t.Cleanup(func() { usageProbeTimeout = origTimeout })

	sink := agent.UsageProbeSinkPath(dir)
	if err := os.WriteFile(sink, []byte("not json"), 0o600); err != nil {
		t.Fatalf("seed an invalid payload: %v", err)
	}
	runner := &usageFakeRunner{sinkPath: sink}
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

func TestUsageMarkdownIncludesTheWeekWindow(t *testing.T) {
	dir := t.TempDir()
	setUsageTestKnobs(t, dir)
	sink := agent.UsageProbeSinkPath(dir)
	runner := &usageFakeRunner{
		sinkPath: sink, readyOnEscape: 1,
		readyContent: `{"rate_limits": {"seven_day": {"used_percentage": 81, "resets_at": 1700000000}}}`,
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
	origTimeout := usageProbeTimeout
	usageProbeTimeout = 2 * usagePollInterval
	t.Cleanup(func() { usageProbeTimeout = origTimeout })

	sink := agent.UsageProbeSinkPath(dir)
	if err := os.WriteFile(sink, []byte(`{"transcript_path": ""}`), 0o600); err != nil {
		t.Fatalf("seed a rate-limit-less payload: %v", err)
	}
	runner := &usageFakeRunner{sinkPath: sink}
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
