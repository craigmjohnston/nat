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

// usageProbeTimeout is how long a probe waits for the sink file to appear
// before giving up and reporting unavailable. rate_limits only appears after
// the session's first API response, so this has to cover one haiku turn's
// reply as well as the statusline write that follows it — long enough for
// that, short enough that gnat's refresh is never left hanging on a probe
// that will not land.
var usageProbeTimeout = 30 * time.Second

// usagePollInterval is how often the probe checks for the sink file.
var usagePollInterval = 500 * time.Millisecond

// usageSleep is what the probe waits on between polls and before its first
// prompt; time.Sleep in production, faked in tests so a command exercising a
// 30-second timeout does not take 30 seconds to run.
var usageSleep = time.Sleep

// usagePromptSettleWait is how long the probe waits after launching Claude
// Code before pasting its prompt — long enough for the composer to be ready
// to receive it.
var usagePromptSettleWait = 3 * time.Second

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
// and clear any stale sink left by a killed prior run, launch, prompt, poll,
// and clean up — the tmux session, the sink file and the probe's own
// transcript — whatever the outcome.
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

	// rate_limits only appears after the session's first API response, so one
	// minimal prompt is sent once the composer has had a moment to come up.
	usageSleep(usagePromptSettleWait)
	if err := tmux.SendPrompt(session, "hi"); err != nil {
		return agent.UsageReading{}, fmt.Errorf("prompt the usage probe: %w", err)
	}

	data, err := waitForUsageSink(sinkPath, usageProbeTimeout)
	if err != nil {
		return agent.UsageReading{}, err
	}

	reading, transcriptPath, err := agent.ParseUsageSink(data)
	_ = os.Remove(sinkPath)
	if transcriptPath != "" {
		_ = os.Remove(transcriptPath)
	}
	if err != nil {
		return agent.UsageReading{}, err
	}
	return reading, nil
}

// waitForUsageSink polls for the probe's sink file to appear, up to timeout,
// answering its contents the first time a read succeeds. A read that fails —
// the file does not exist yet, or is only half written by the redirect that
// is still going — is not itself a failure; only running out of time is.
func waitForUsageSink(path string, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	for {
		if data, err := os.ReadFile(path); err == nil {
			return data, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("usage probe timed out after %s waiting for a reading", timeout)
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
