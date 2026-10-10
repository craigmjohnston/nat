package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// sessionRecordEnv is the variable every session nat launches carries naming
// the file the embedded mod records its Claude Code session in, set by
// new-session's -e beside [inboxEnv]: the whole path, since the state
// directory is nat's to resolve, never the mod's.
const sessionRecordEnv = "NAT_SESSION_RECORD"

// sessionRecordSuffix ends a session record's file name, beside the session's
// statusline payload and launch record in [AgentStatusDir]. [ReadStatuses]'
// sweep passes over it: the record is for after the session has gone.
const sessionRecordSuffix = ".session.json"

// SessionRecord is what the embedded mod writes at a session's start
// (`session.start`): the Claude Code session id a later launch can resume
// (`claude --resume`), the directory it ran in, and when it started.
type SessionRecord struct {
	SessionID string    `json:"session_id"`
	Cwd       string    `json:"cwd"`
	StartedAt time.Time `json:"started_at"`
}

// SessionRecordPath is the file session's record lives in.
func SessionRecordPath(session string) (string, error) {
	dir, err := AgentStatusDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, session+sessionRecordSuffix), nil
}

// sessionRecordEnvArgs is the -e flag naming session's record, or nothing
// where it cannot be resolved: that session is then never resumed.
func sessionRecordEnvArgs(session string) []string {
	path, err := SessionRecordPath(session)
	if err != nil {
		logging.Action("agent session record disabled for a launch", "session", session, "error", err.Error())
		return nil
	}
	return []string{"-e", sessionRecordEnv + "=" + path}
}

// ReadSessionRecord reads session's record, false where there is none or it
// cannot be read: a launch then starts fresh, as before there were records.
func ReadSessionRecord(session string) (SessionRecord, bool) {
	path, err := SessionRecordPath(session)
	if err != nil {
		return SessionRecord{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return SessionRecord{}, false
	}
	var rec SessionRecord
	if json.Unmarshal(data, &rec) != nil || strings.TrimSpace(rec.SessionID) == "" {
		return SessionRecord{}, false
	}
	return rec, true
}

// RemoveSessionRecord removes session's record, once the work it could resume
// has ended: the slice merged, closed, trashed, released or cancelled, or a
// planning or ad hoc session killed through nat. None there is nothing to do;
// a removal that fails is logged and never the caller's failure.
func RemoveSessionRecord(session string) {
	path, err := SessionRecordPath(session)
	if err != nil {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		logging.Action("could not remove an agent's session record", "session", session, "error", err.Error())
	}
}

// ForgetSliceSession is [RemoveSessionRecord] for the session of the slice
// with page ID sliceID ([SessionName]).
func ForgetSliceSession(sliceID string) { RemoveSessionRecord(SessionName(sliceID)) }
