package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// UsageProbeSession is the tmux session a usage probe runs in — fixed rather
// than named after anything it is probing, since there is only ever one
// probe in flight at a time and a session a killed nat left behind is worth
// finding again by this same name.
const UsageProbeSession = SessionPrefix + "usage-probe"

// usageProbeDirName is the probe's scratch directory's name, under nat's
// state directory alongside the log file and the nudge marker — a fixed,
// nat-owned location rather than a temp directory, so a leftover from a
// killed probe is always found at the same place.
const usageProbeDirName = "usage-probe"

// UsageProbeDir is the scratch directory a usage probe writes its throwaway
// --settings file and its statusline sink into.
func UsageProbeDir() (string, error) {
	dir, err := logging.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, usageProbeDirName), nil
}

// UsageProbeSinkPath is the file the probe's statusLine command writes
// Claude Code's statusline JSON to, inside dir ([UsageProbeDir]).
func UsageProbeSinkPath(dir string) string { return filepath.Join(dir, "sink.json") }

// usageProbeSettingsPath is the probe's own throwaway --settings file, inside
// dir.
func usageProbeSettingsPath(dir string) string { return filepath.Join(dir, "settings.json") }

// usageProbeSettings is the shape of the probe's throwaway --settings file: a
// statusLine command that does nothing but hand off whatever Claude Code
// pipes to it. --settings is per-session and outranks user and project
// settings, so the user's own statusline configuration — or the lack of one —
// is never read, written or shadowed by this.
type usageProbeSettings struct {
	StatusLine usageProbeStatusLine `json:"statusLine"`
}

// usageProbeStatusLine is the one field of [usageProbeSettings] that matters:
// a command Claude Code runs on every statusline refresh, fed the statusline
// JSON on stdin.
type usageProbeStatusLine struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// usageProbeMarshal is held as a var so tests can stub a marshal failure, the
// same way [config.Save] does for its own encode.
var usageProbeMarshal = json.MarshalIndent

// WriteUsageProbeSettings writes the probe's --settings file into dir,
// pointed at sinkPath, and returns its path. The statusLine command is a
// plain shell redirect — the JSON Claude Code pipes to it needs no
// processing, only somewhere to land.
func WriteUsageProbeSettings(dir, sinkPath string) (string, error) {
	settings := usageProbeSettings{
		StatusLine: usageProbeStatusLine{
			Type:    "command",
			Command: "cat > " + shellQuote(sinkPath),
		},
	}
	data, err := usageProbeMarshal(settings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode usage probe settings: %w", err)
	}
	path := usageProbeSettingsPath(dir)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write usage probe settings: %w", err)
	}
	return path, nil
}

// usageProbeCommand is the shell command the probe's tmux session runs:
// Claude Code pinned to haiku — the cheapest model that still turns a reply —
// with the throwaway settings file that is the only sanctioned, OAuth-free
// way to read rate-limit state.
func usageProbeCommand(settingsPath string) string {
	return fmt.Sprintf("claude --model haiku --settings %s", shellQuote(settingsPath))
}

// LaunchUsageProbe starts the probe's detached tmux session in workdir,
// running Claude Code with the given --settings file. Unlike [Tmux.Launch]
// the pane is left untagged with no [SlicePaneOption]: a probe is not an
// agent working a slice, and nothing goes looking for one under this name.
func (t *Tmux) LaunchUsageProbe(session, workdir, settingsPath string) error {
	args := []string{
		"new-session", "-d",
		"-s", session,
		"-c", workdir,
		"sh", "-c", usageProbeCommand(settingsPath),
	}
	args = append(args, statusOffArgs(session)...)
	if _, err := t.run(args...); err != nil {
		return fmt.Errorf("launch usage probe session %s: %w", session, err)
	}
	return nil
}

// UsageRateLimit is one rate-limit window read off a statusline payload: how
// much of it is used and when it resets, both computed server-side by
// Claude's own usage accounting.
type UsageRateLimit struct {
	UsedPercentage float64
	ResetsAt       time.Time
}

// UsageReading is what one probe read: the two windows a Pro/Max subscriber's
// statusline payload can carry, each independently absent when the account
// has no such window at all — never read as 0%, only as unknown.
type UsageReading struct {
	FiveHour *UsageRateLimit
	SevenDay *UsageRateLimit
}

// statuslinePayload is the shape of what Claude Code pipes to a statusLine
// command's stdin — only the fields the usage probe reads out of it.
type statuslinePayload struct {
	TranscriptPath string                  `json:"transcript_path"`
	RateLimits     statuslineRateLimitsDoc `json:"rate_limits"`
}

// statuslineRateLimitsDoc is the payload's own "rate_limits" object — absent
// entirely for a session with no rate-limit state to report at all (a
// non-subscription account), which parses the same as one whose two windows
// are both null.
type statuslineRateLimitsDoc struct {
	FiveHour *statuslineRateLimit `json:"five_hour"`
	SevenDay *statuslineRateLimit `json:"seven_day"`
}

// statuslineRateLimit is one window of statuslineRateLimitsDoc, wire-for-wire
// with what Claude Code sends.
type statuslineRateLimit struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
}

// ParseUsageSink parses one probe's sink file payload, answering the reading
// and the transcript path the probe's own session wrote — reported so the
// caller can delete it, since a probe is meant to leave no residue.
func ParseUsageSink(data []byte) (UsageReading, string, error) {
	var payload statuslinePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return UsageReading{}, "", fmt.Errorf("parse usage probe payload: %w", err)
	}
	reading := UsageReading{
		FiveHour: convertRateLimit(payload.RateLimits.FiveHour),
		SevenDay: convertRateLimit(payload.RateLimits.SevenDay),
	}
	return reading, payload.TranscriptPath, nil
}

// convertRateLimit turns one wire rate-limit into [UsageRateLimit], answering
// nil for nil exactly — the window was absent from the payload, which is
// unknown, never zero.
func convertRateLimit(r *statuslineRateLimit) *UsageRateLimit {
	if r == nil {
		return nil
	}
	return &UsageRateLimit{UsedPercentage: r.UsedPercentage, ResetsAt: time.Unix(r.ResetsAt, 0)}
}
