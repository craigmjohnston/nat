package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// usageEnv is the variable every session nat launches carries naming the file
// the embedded mod writes the account's rate-limit windows to, each time the
// session measures them (`session.measure`). The whole path, as for
// [inboxEnv]: the state directory is nat's to resolve, never the mod's.
const usageEnv = "NAT_USAGE"

// AgentUsageMaxAge is how old an agent's usage file may be and still answer
// `nat usage` in the probe's place. The mod writes after each turn, so an
// agent idle longer than this has nothing fresher to say than a probe would.
const AgentUsageMaxAge = 10 * time.Minute

// usagePath is the file session's mod writes its usage reading to, inside dir
// ([AgentStatusDir]) beside the session's statusline payload, so the same
// sweep removes it once the session is gone.
func usagePath(dir, session string) string { return filepath.Join(dir, session+".usage.json") }

// usageEnvArgs is the -e flag naming session's usage file, or nothing where
// the state directory cannot be resolved: that session then writes none, and
// `nat usage` probes as it did before agents wrote one.
func usageEnvArgs(session string) []string {
	dir, err := AgentStatusDir()
	if err != nil {
		logging.Action("agent usage file disabled for a launch", "session", session, "error", err.Error())
		return nil
	}
	return []string{"-e", usageEnv + "=" + usagePath(dir, session)}
}

// agentUsageDoc is what the mod writes: when it wrote, and each window the
// session's last measurement carried, absent where it carried none.
type agentUsageDoc struct {
	ReadAt     time.Time `json:"read_at"`
	RateLimits struct {
		FiveHour *agentUsageWindow `json:"five_hour"`
		SevenDay *agentUsageWindow `json:"seven_day"`
	} `json:"rate_limits"`
}

// agentUsageWindow is one window of [agentUsageDoc]; resets_at is the
// engine's own ISO 8601 timestamp, passed through.
type agentUsageWindow struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       string  `json:"resets_at"`
}

// ReadAgentUsage answers the freshest usage reading a session in live (tag to
// session name, as [Tmux.LiveSlices] returns it) wrote into dir
// ([AgentStatusDir]) less than [AgentUsageMaxAge] before now, and whether
// there was one. A missing file is no reading; one that does not parse, or
// carries no window, is logged and skipped. Only file reads, as
// [ReadStatuses]: the sweep of a gone session's file is that poll's.
func ReadAgentUsage(dir string, live map[string]string, now time.Time) (UsageReading, bool) {
	var best UsageReading
	var bestAt time.Time
	for _, session := range live {
		data, err := os.ReadFile(usagePath(dir, session))
		if err != nil {
			continue
		}
		var doc agentUsageDoc
		if err := json.Unmarshal(data, &doc); err != nil {
			logging.Action("agent usage file skipped", "session", session, "error", err.Error())
			continue
		}
		reading := UsageReading{
			FiveHour: agentWindow(doc.RateLimits.FiveHour),
			SevenDay: agentWindow(doc.RateLimits.SevenDay),
		}
		if reading.FiveHour == nil && reading.SevenDay == nil {
			logging.Action("agent usage file skipped", "session", session, "error", "no rate-limit window")
			continue
		}
		if now.Sub(doc.ReadAt) >= AgentUsageMaxAge || !doc.ReadAt.After(bestAt) {
			continue
		}
		best, bestAt = reading, doc.ReadAt
	}
	return best, !bestAt.IsZero()
}

// agentWindow turns one written window into [UsageRateLimit]: nil where it is
// absent, and where its reset time is missing or unreadable — a window is
// unknown rather than shown resetting at the epoch.
func agentWindow(w *agentUsageWindow) *UsageRateLimit {
	if w == nil {
		return nil
	}
	resets, err := time.Parse(time.RFC3339, w.ResetsAt)
	if err != nil {
		return nil
	}
	return &UsageRateLimit{UsedPercentage: w.UsedPercentage, ResetsAt: resets}
}
