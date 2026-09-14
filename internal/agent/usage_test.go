package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

func TestUsageProbeDir(t *testing.T) {
	logDir, err := logging.Dir()
	if err != nil {
		t.Fatalf("logging.Dir: %v", err)
	}
	dir, err := UsageProbeDir()
	if err != nil {
		t.Fatalf("UsageProbeDir: %v", err)
	}
	want := filepath.Join(logDir, "usage-probe")
	if dir != want {
		t.Errorf("dir = %s, want %s", dir, want)
	}
}

func TestUsageProbeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	if _, err := UsageProbeDir(); err == nil {
		t.Fatal("UsageProbeDir: want an error with no resolvable home directory")
	}
}

func TestUsageProbeSinkPath(t *testing.T) {
	got := UsageProbeSinkPath("/tmp/probe")
	want := filepath.Join("/tmp/probe", "sink.json")
	if got != want {
		t.Errorf("sink path = %s, want %s", got, want)
	}
}

func TestWriteUsageProbeSettings(t *testing.T) {
	dir := t.TempDir()
	sink := filepath.Join(dir, "sink.json")
	path, err := WriteUsageProbeSettings(dir, sink)
	if err != nil {
		t.Fatalf("WriteUsageProbeSettings: %v", err)
	}
	if want := filepath.Join(dir, "settings.json"); path != want {
		t.Errorf("path = %s, want %s", path, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var settings usageProbeSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	if settings.StatusLine.Type != "command" {
		t.Errorf("statusLine.type = %s, want command", settings.StatusLine.Type)
	}
	want := "cat > " + shellQuote(sink)
	if settings.StatusLine.Command != want {
		t.Errorf("statusLine.command = %s, want %s", settings.StatusLine.Command, want)
	}
}

func TestWriteUsageProbeSettingsMarshalFailure(t *testing.T) {
	orig := usageProbeMarshal
	usageProbeMarshal = func(v any, prefix, indent string) ([]byte, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { usageProbeMarshal = orig })

	if _, err := WriteUsageProbeSettings(t.TempDir(), "/tmp/sink.json"); err == nil {
		t.Fatal("WriteUsageProbeSettings: want an error when marshalling fails")
	}
}

func TestWriteUsageProbeSettingsWriteFailure(t *testing.T) {
	dir := t.TempDir()
	// A directory sitting where the settings file needs to be written turns
	// the write into EISDIR rather than a successful write.
	if err := os.Mkdir(filepath.Join(dir, "settings.json"), 0o700); err != nil {
		t.Fatalf("seed a directory in place of the settings file: %v", err)
	}

	if _, err := WriteUsageProbeSettings(dir, "/tmp/sink.json"); err == nil {
		t.Fatal("WriteUsageProbeSettings: want an error when the write fails")
	}
}

func TestUsageProbeCommand(t *testing.T) {
	got := usageProbeCommand("/tmp/probe/settings.json")
	want := "claude --model haiku --settings " + shellQuote("/tmp/probe/settings.json")
	if got != want {
		t.Errorf("command = %s, want %s", got, want)
	}
}

func TestLaunchUsageProbe(t *testing.T) {
	r := &fakeRunner{}
	if err := NewTmuxWithRunner(r).LaunchUsageProbe(UsageProbeSession, "/tmp/probe", "/tmp/probe/settings.json"); err != nil {
		t.Fatalf("LaunchUsageProbe: %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("calls = %v, want exactly one", r.calls)
	}
	want := []string{
		"-u",
		"new-session", "-d",
		"-s", UsageProbeSession,
		"-c", "/tmp/probe",
		"sh", "-c", usageProbeCommand("/tmp/probe/settings.json"),
		";", "set-option", "-t", UsageProbeSession, "status", "off",
	}
	if r.calls[0].name != TmuxBinary || !slices.Equal(r.calls[0].args, want) {
		t.Errorf("call = %v %v, want tmux %v", r.calls[0].name, r.calls[0].args, want)
	}
}

func TestLaunchUsageProbeFailure(t *testing.T) {
	r := &fakeRunner{err: errors.New("boom")}
	err := NewTmuxWithRunner(r).LaunchUsageProbe(UsageProbeSession, "/tmp/probe", "/tmp/probe/settings.json")
	if err == nil {
		t.Fatal("LaunchUsageProbe: want an error")
	}
}

func TestParseUsageSink(t *testing.T) {
	tests := []struct {
		name           string
		payload        string
		wantFiveHour   *UsageRateLimit
		wantSevenDay   *UsageRateLimit
		wantTranscript string
	}{
		{
			name: "both windows present",
			payload: `{
				"transcript_path": "/tmp/t.jsonl",
				"rate_limits": {
					"five_hour": {"used_percentage": 38, "resets_at": 1000},
					"seven_day": {"used_percentage": 81, "resets_at": 2000}
				}
			}`,
			wantFiveHour:   &UsageRateLimit{UsedPercentage: 38, ResetsAt: time.Unix(1000, 0)},
			wantSevenDay:   &UsageRateLimit{UsedPercentage: 81, ResetsAt: time.Unix(2000, 0)},
			wantTranscript: "/tmp/t.jsonl",
		},
		{
			name:           "no rate_limits at all reads as both absent",
			payload:        `{"transcript_path": "/tmp/t.jsonl"}`,
			wantFiveHour:   nil,
			wantSevenDay:   nil,
			wantTranscript: "/tmp/t.jsonl",
		},
		{
			name: "one window absent, one present",
			payload: `{
				"transcript_path": "/tmp/t.jsonl",
				"rate_limits": {"seven_day": {"used_percentage": 5, "resets_at": 42}}
			}`,
			wantFiveHour:   nil,
			wantSevenDay:   &UsageRateLimit{UsedPercentage: 5, ResetsAt: time.Unix(42, 0)},
			wantTranscript: "/tmp/t.jsonl",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reading, transcript, err := ParseUsageSink([]byte(tt.payload))
			if err != nil {
				t.Fatalf("ParseUsageSink: %v", err)
			}
			if transcript != tt.wantTranscript {
				t.Errorf("transcript = %s, want %s", transcript, tt.wantTranscript)
			}
			assertRateLimit(t, "five_hour", reading.FiveHour, tt.wantFiveHour)
			assertRateLimit(t, "seven_day", reading.SevenDay, tt.wantSevenDay)
		})
	}
}

func TestParseUsageSinkInvalid(t *testing.T) {
	if _, _, err := ParseUsageSink([]byte("not json")); err == nil {
		t.Fatal("ParseUsageSink: want an error for invalid JSON")
	}
}

func assertRateLimit(t *testing.T, label string, got, want *UsageRateLimit) {
	t.Helper()
	if (got == nil) != (want == nil) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	if got == nil {
		return
	}
	if got.UsedPercentage != want.UsedPercentage || !got.ResetsAt.Equal(want.ResetsAt) {
		t.Errorf("%s = %+v, want %+v", label, got, want)
	}
}
