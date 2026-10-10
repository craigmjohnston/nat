package agent

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/config"
)

// usageDoc is a usage file as the mod writes it, read at at.
func usageDoc(at time.Time, windows string) string {
	return `{"read_at":"` + at.Format(time.RFC3339) + `","rate_limits":{` + windows + `}}`
}

const bothWindows = `"five_hour":{"used_percentage":23.5,"resets_at":"2026-10-10T15:00:00.000Z"},` +
	`"seven_day":{"used_percentage":61,"resets_at":"2026-10-14T09:00:00Z"}`

// The freshest file of a live session answers, each window as written; a
// session not live is never read, however fresh.
func TestReadAgentUsageTakesTheFreshest(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	write(t, usagePath(dir, "nat-old"), usageDoc(now.Add(-5*time.Minute), `"five_hour":{"used_percentage":10,"resets_at":"2026-10-10T15:00:00Z"}`))
	write(t, usagePath(dir, "nat-new"), usageDoc(now.Add(-time.Minute), bothWindows))
	write(t, usagePath(dir, "nat-gone"), usageDoc(now, `"five_hour":{"used_percentage":99,"resets_at":"2026-10-10T15:00:00Z"}`))

	got, ok := ReadAgentUsage(dir, map[string]string{"a": "nat-old", "b": "nat-new", "c": "nat-none"}, now)
	if !ok {
		t.Fatal("ReadAgentUsage: want a reading")
	}
	if got.FiveHour == nil || got.FiveHour.UsedPercentage != 23.5 || !got.FiveHour.ResetsAt.Equal(time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)) {
		t.Errorf("five_hour = %+v, want 23.5%% resetting 15:00Z", got.FiveHour)
	}
	if got.SevenDay == nil || got.SevenDay.UsedPercentage != 61 || !got.SevenDay.ResetsAt.Equal(time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("seven_day = %+v, want 61%% resetting 14 Oct 09:00Z", got.SevenDay)
	}
}

// A window the file leaves out, or whose reset time is missing or
// unreadable, is unknown; the others still answer.
func TestReadAgentUsageOneWindow(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	write(t, usagePath(dir, "nat-1"), usageDoc(now, `"five_hour":{"used_percentage":5},"seven_day":{"used_percentage":40,"resets_at":"2026-10-14T09:00:00Z"}`))
	got, ok := ReadAgentUsage(dir, map[string]string{"a": "nat-1"}, now)
	if !ok || got.FiveHour != nil || got.SevenDay == nil || got.SevenDay.UsedPercentage != 40 {
		t.Errorf("ReadAgentUsage = %+v, %v; want the week window alone", got, ok)
	}
}

// A file ten minutes old or more falls through to the probe, as does no file.
func TestReadAgentUsageStale(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	write(t, usagePath(dir, "nat-1"), usageDoc(now.Add(-AgentUsageMaxAge), bothWindows))
	for _, live := range []map[string]string{{"a": "nat-1"}, {"b": "nat-2"}, nil} {
		if got, ok := ReadAgentUsage(dir, live, now); ok {
			t.Errorf("ReadAgentUsage(%v) = %+v, want none", live, got)
		}
	}
}

// A file that does not parse, or carries no window, is logged and skipped;
// another live session's still answers.
func TestReadAgentUsageSkipsMalformed(t *testing.T) {
	logged := logTo(t)
	dir := t.TempDir()
	now := time.Now()
	write(t, usagePath(dir, "nat-bad"), `{"read_at":`)
	write(t, usagePath(dir, "nat-empty"), usageDoc(now, ``))
	write(t, usagePath(dir, "nat-good"), usageDoc(now.Add(-time.Minute), bothWindows))

	got, ok := ReadAgentUsage(dir, map[string]string{"a": "nat-bad", "b": "nat-empty", "c": "nat-good"}, now)
	if !ok || got.FiveHour == nil || got.FiveHour.UsedPercentage != 23.5 {
		t.Errorf("ReadAgentUsage = %+v, %v; want nat-good's reading", got, ok)
	}
	log := logged()
	for _, session := range []string{"nat-bad", "nat-empty"} {
		if !strings.Contains(log, "agent usage file skipped") || !strings.Contains(log, "session="+session) {
			t.Errorf("log = %q, want %s's file logged as skipped", log, session)
		}
	}
}

// Every launch names its usage file beside its statusline payload, where -e
// is taken; with no state directory it names none.
func TestLaunchesCarryTheUsageFile(t *testing.T) {
	dir := isolatedStatusDir(t)
	want := usageEnv + "=" + usagePath(dir, "nat-1")
	r := &fakeRunner{outs: map[string]string{"-V": "tmux 3.5a\n", "new-session": "%7\n"}}
	if err := NewTmuxWithRunner(r).LaunchBare("nat-1", "/tmp", "session:p:s", config.AgentModel{}); err != nil {
		t.Fatalf("LaunchBare: %v", err)
	}
	if !slices.Contains(r.calls[1].args, want) {
		t.Errorf("args = %v, want %s", r.calls[1].args, want)
	}

	t.Setenv("HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	if got := usageEnvArgs("nat-1"); got != nil {
		t.Errorf("usageEnvArgs = %v, want none with no state directory", got)
	}
}
