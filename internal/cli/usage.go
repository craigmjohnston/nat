package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/craigmjohnston/nat/internal/agent"
)

// usageProbeTimeout is how long a probe waits for a qualifying reading (one
// carrying at least one rate-limit window) before giving up and reporting
// unavailable — long enough for the /usage panel to open, fetch, and close,
// short enough that gnat's refresh is never left hanging on a probe that
// will not land.
var usageProbeTimeout = 30 * time.Second

// usagePollInterval is how often the probe checks for a qualifying reading.
var usagePollInterval = 500 * time.Millisecond

// usageSleep is what the probe waits on between polls and before driving the
// panel; time.Sleep in production, faked in tests so a command exercising a
// 30-second timeout does not take 30 seconds to run.
var usageSleep = time.Sleep

// usageNow is how the probe reads the current time for its timeout and
// half-timeout-retry math; time.Now in production, replaced in tests
// alongside usageSleep so a poll loop's timing is exercised deterministically
// rather than by waiting on the wall clock.
var usageNow = time.Now

// usagePromptSettleWait is how long the probe waits after launching Claude
// Code before driving the /usage command — long enough for the composer to
// be ready to receive it.
var usagePromptSettleWait = 3 * time.Second

// usagePanelWait is how long the probe waits after opening the /usage panel
// before closing it again — long enough for the panel's own fetch to land,
// since closing the panel is what triggers the statusline write that
// actually carries the reading.
var usagePanelWait = 2 * time.Second

// usageProbeDirFunc resolves the probe's scratch directory; agent.UsageProbeDir
// in production, pointed at a throwaway directory in tests so a test run
// never touches the real state directory.
var usageProbeDirFunc = agent.UsageProbeDir

// usage probes Claude Code's own statusline for the account's current
// Pro/Max rate-limit usage: a throwaway detached tmux session, seeded with a
// --settings file whose statusLine command is the only sanctioned,
// OAuth-free way to read the numbers /usage shows. It requires no --project:
// the reading is a property of the logged-in account, not of any tracked
// project.
func usage(args []string, env Env) error {
	flags := flag.NewFlagSet("usage", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of plain text")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("usage: takes no arguments, given %d", len(rest))
	}

	reading, probeErr := probeUsage(env)
	if probeErr != nil {
		// Unavailable is a normal answer, not a failure: gnat renders nothing
		// for it either way, and a probe that could not run (no tmux, no
		// claude on PATH, a timeout) is not this command's to fail loudly
		// over.
		reading = agent.UsageReading{}
	}

	if *asJSON {
		return writeUsageJSON(env.Out, reading)
	}
	return writeUsageMarkdown(env.Out, reading)
}

// probeUsage runs one throwaway probe session end to end: lay the settings
// and clear any stale sink left by a killed prior run, launch, drive the
// local /usage panel, poll, and clean up — the tmux session, the sink file
// and the probe's own transcript — whatever the outcome.
//
// The probe never sends this session a prompt and spends no model turn to
// read usage: /usage is a local slash command, free to open and close, and
// is driven by literal keystrokes ([agent.Tmux.SendKeys]/[agent.Tmux.Interrupt])
// rather than [agent.Tmux.SendPrompt], which this file never calls.
func probeUsage(env Env) (agent.UsageReading, error) {
	dir, err := usageProbeDirFunc()
	if err != nil {
		return agent.UsageReading{}, fmt.Errorf("resolve usage probe dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return agent.UsageReading{}, fmt.Errorf("create usage probe dir: %w", err)
	}

	sinkPath := agent.UsageProbeSinkPath(dir)
	// A sink left by a run that was killed before it cleaned up would
	// otherwise be read back as this probe's own answer.
	_ = os.Remove(sinkPath)

	settingsPath, err := agent.WriteUsageProbeSettings(dir, sinkPath)
	if err != nil {
		return agent.UsageReading{}, err
	}

	tmux := env.NewTmux()
	session := agent.UsageProbeSession
	// A session left over from a run that did not clean up after itself is
	// cleared before this one starts, so the launch below cannot collide
	// with it.
	_ = tmux.Kill(session)
	defer func() { _ = tmux.Kill(session) }()

	if err := tmux.LaunchUsageProbe(session, dir, settingsPath); err != nil {
		return agent.UsageReading{}, err
	}

	// Once the composer has had a moment to come up, /usage opens the panel —
	// literal keystrokes, never a prompt.
	usageSleep(usagePromptSettleWait)
	if err := tmux.SendKeys(session, "/usage", "Enter"); err != nil {
		return agent.UsageReading{}, fmt.Errorf("open the usage panel in the usage probe: %w", err)
	}

	// Verified live 2026-09-14 on Claude Code 2.1.236: the /usage panel's own
	// fetch populates the session's rate-limit state itself, ahead of the
	// statusline docs' claim that rate_limits appears "only after the first
	// API response" — that is inexact, /usage populates it too. Closing the
	// panel is what triggers the statusline refresh that carries the
	// reading.
	usageSleep(usagePanelWait)
	if err := tmux.Interrupt(session); err != nil {
		return agent.UsageReading{}, fmt.Errorf("close the usage panel in the usage probe: %w", err)
	}

	reading, transcriptPath, err := waitForUsageReading(tmux, session, sinkPath, usageProbeTimeout)
	_ = os.Remove(sinkPath)
	if transcriptPath != "" {
		_ = os.Remove(transcriptPath)
	}
	if err != nil {
		return agent.UsageReading{}, err
	}
	return reading, nil
}

// waitForUsageReading polls the probe's sink file up to timeout, answering
// the first reading that carries at least one rate-limit window. Claude Code
// writes a statusline payload at startup, before any rate-limit state
// exists — a payload that parses but carries neither window is "not yet",
// never an answer, and so is one that fails to parse at all (the redirect
// writing it can be read mid-write). If half the timeout passes with nothing
// to show, Escape is sent again — harmless at the composer, and covers a
// panel that opened late.
func waitForUsageReading(tmux *agent.Tmux, session, path string, timeout time.Duration) (agent.UsageReading, string, error) {
	deadline := usageNow().Add(timeout)
	retryAt := usageNow().Add(timeout / 2)
	retried := false
	for {
		if data, err := os.ReadFile(path); err == nil {
			if reading, transcriptPath, err := agent.ParseUsageSink(data); err == nil &&
				(reading.FiveHour != nil || reading.SevenDay != nil) {
				return reading, transcriptPath, nil
			}
		}
		if usageNow().After(deadline) {
			return agent.UsageReading{}, "", fmt.Errorf("usage probe timed out after %s waiting for a reading", timeout)
		}
		if !retried && !usageNow().Before(retryAt) {
			retried = true
			_ = tmux.Interrupt(session)
		}
		usageSleep(usagePollInterval)
	}
}

// usageRateLimitJSON is the wire form of one rate-limit window.
type usageRateLimitJSON struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
}

// usageJSON is the structured form of the usage output. A window not read at
// all — a probe that never ran, or an account with no such window — is
// omitted rather than written as zero, so the reader can tell "unknown" from
// "empty".
type usageJSON struct {
	FiveHour *usageRateLimitJSON `json:"five_hour,omitempty"`
	SevenDay *usageRateLimitJSON `json:"seven_day,omitempty"`
}

// writeUsageJSON encodes the reading as JSON, indented.
func writeUsageJSON(out io.Writer, reading agent.UsageReading) error {
	doc := usageJSON{
		FiveHour: usageRateLimitJSONOf(reading.FiveHour),
		SevenDay: usageRateLimitJSONOf(reading.SevenDay),
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func usageRateLimitJSONOf(r *agent.UsageRateLimit) *usageRateLimitJSON {
	if r == nil {
		return nil
	}
	return &usageRateLimitJSON{UsedPercentage: r.UsedPercentage, ResetsAt: r.ResetsAt.Unix()}
}

// writeUsageMarkdown renders the reading as plain text: one line per window
// read, or "usage unavailable" when neither was.
func writeUsageMarkdown(out io.Writer, reading agent.UsageReading) error {
	if reading.FiveHour == nil && reading.SevenDay == nil {
		_, err := io.WriteString(out, "usage unavailable\n")
		return err
	}
	var b []byte
	if reading.FiveHour != nil {
		b = fmt.Appendf(b, "Session %.0f%% · resets %s\n",
			reading.FiveHour.UsedPercentage, reading.FiveHour.ResetsAt.Local().Format("3:04 PM"))
	}
	if reading.SevenDay != nil {
		b = fmt.Appendf(b, "Week %.0f%% · resets %s\n",
			reading.SevenDay.UsedPercentage, reading.SevenDay.ResetsAt.Local().Format("Mon"))
	}
	_, err := out.Write(b)
	return err
}
