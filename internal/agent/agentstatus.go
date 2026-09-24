package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/logging"
)

// agentStatusDirName is where launched agents' statusline payloads land, under
// nat's state directory beside the log file, the nudge marker and the usage
// probe's scratch directory.
const agentStatusDirName = "agent-status"

// sweepGrace is how old a status file must be before [ReadStatuses] will
// sweep it for belonging to no live session. A launch writes its files before
// tmux has created — let alone tagged — the pane, so a poll in that window
// would otherwise sweep the launch's own record out from under it.
const sweepGrace = time.Minute

// AgentStatusDir is the directory holding one teed statusline payload and one
// launch record per agent session.
func AgentStatusDir() (string, error) {
	dir, err := logging.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, agentStatusDirName), nil
}

// payloadPath is the file a session's statusLine command tees each payload
// into, and metaPath the launch record beside it, both inside dir.
func payloadPath(dir, session string) string { return filepath.Join(dir, session+".json") }
func metaPath(dir, session string) string    { return filepath.Join(dir, session+".launch.json") }

// launchRecord is what a session was launched with — the fallback for a
// payload that does not say (yet), since the model and effort a launch asked
// for are known before Claude Code has drawn its first status line.
type launchRecord struct {
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
}

// prepareStatusSink readies session's status files for a launch as m and
// returns the payload path the session's statusLine command should tee into.
// Any old payload from a previous session of the same name is removed so it
// cannot be read as this one's. It answers "" — launching without a statusline
// rather than not at all — when the directory or record cannot be made: a
// missing readout must never cost an agent its launch.
func prepareStatusSink(session string, m config.AgentModel) string {
	dir, err := AgentStatusDir()
	if err == nil {
		err = os.MkdirAll(dir, 0o700)
	}
	var data []byte
	if err == nil {
		data, _ = json.Marshal(launchRecord{Model: m.Model, Effort: m.Effort})
		err = os.WriteFile(metaPath(dir, session), data, 0o600)
	}
	if err != nil {
		logging.Action("agent status disabled for a session", "session", session, "error", err.Error())
		return ""
	}
	_ = os.Remove(payloadPath(dir, session))
	return payloadPath(dir, session)
}

// statuslineSettings is the --settings JSON of a launch: the theme, plus —
// with a sink — a statusLine command that tees each payload Claude Code pipes
// to it into sink. --settings outranks user and project settings for the one
// session, so the user's own statusline is neither read nor shadowed beyond
// it. The statusline fires on every turn without costing one; the tee goes
// through a temp file and a rename so a reader never sees half a payload.
func statuslineSettings(sink string) string {
	type statusLine struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	}
	settings := struct {
		Theme      string      `json:"theme"`
		StatusLine *statusLine `json:"statusLine,omitempty"`
	}{Theme: "auto"}
	if sink != "" {
		tmp := shellQuote(sink + ".tmp")
		settings.StatusLine = &statusLine{
			Type:    "command",
			Command: "cat > " + tmp + " && mv " + tmp + " " + shellQuote(sink),
		}
	}
	// HTML escaping off: it would spell the redirect's ">" and "&&" as \u003e
	// and \u0026 — valid JSON, but not what anyone reading a launch wants to see.
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(settings)
	return strings.TrimSuffix(b.String(), "\n")
}

// AgentStatus is what is known of one live agent's model, reasoning effort and
// context use. Each field is independently absent — empty, or nil — when
// unknown; never zero.
type AgentStatus struct {
	Model   string
	Effort  string
	Context *float64
}

// statuslineAgentPayload is the slice of a statusline payload this reads.
type statuslineAgentPayload struct {
	Model struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Effort struct {
		Level string `json:"level"`
	} `json:"effort"`
	ContextWindow struct {
		UsedPercentage *float64 `json:"used_percentage"`
	} `json:"context_window"`
}

// readStatus reads one session's status from dir: the teed payload where there
// is a readable one, filling whatever it leaves out from the launch record.
// A missing or unreadable file concludes nothing — the field stays absent.
func readStatus(dir, session string) AgentStatus {
	var st AgentStatus
	var rec launchRecord
	if data, err := os.ReadFile(metaPath(dir, session)); err == nil {
		_ = json.Unmarshal(data, &rec)
	}
	var p statuslineAgentPayload
	if data, err := os.ReadFile(payloadPath(dir, session)); err == nil {
		if json.Unmarshal(data, &p) != nil {
			p = statuslineAgentPayload{}
		}
	}
	st.Model = firstNonEmpty(p.Model.DisplayName, p.Model.ID, rec.Model)
	st.Effort = firstNonEmpty(p.Effort.Level, rec.Effort)
	st.Context = p.ContextWindow.UsedPercentage
	return st
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// ReadStatuses reads every session in live (as [Tmux.LiveSlices] returns it:
// tag to session name) and answers the status of each by session name, then
// sweeps any status file belonging to no live session. Only file reads — no
// subprocess, no network — so it is cheap to poll.
func ReadStatuses(live map[string]string) map[string]AgentStatus {
	dir, err := AgentStatusDir()
	if err != nil {
		return nil
	}
	out := make(map[string]AgentStatus, len(live))
	sessions := make(map[string]bool, len(live))
	for _, session := range live {
		sessions[session] = true
		out[session] = readStatus(dir, session)
	}
	sweepStatus(dir, sessions)
	return out
}

// sweepStatus removes the files in dir of sessions not in keep, once they are
// older than [sweepGrace]. Failures are ignored: a leftover is swept next poll.
func sweepStatus(dir string, keep map[string]bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		session := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(name, ".tmp"), ".json"), ".launch")
		if keep[session] {
			continue
		}
		if info, err := e.Info(); err != nil || time.Since(info.ModTime()) < sweepGrace {
			continue
		}
		_ = os.Remove(filepath.Join(dir, name))
	}
}
