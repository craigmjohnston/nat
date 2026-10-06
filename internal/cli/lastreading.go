package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/craigmjohnston/nat/internal/logging"
)

// lastReadingFileName is the file under nat's state directory that holds what
// the last batched reading found.
const lastReadingFileName = "github-reading.json"

// DefaultReadingPath is where the last batched reading is kept: nat's state
// directory, beside the proposals and the workspaces.
func DefaultReadingPath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, lastReadingFileName), nil
}

// lastReading is what pr-status's batched reading found that a command run
// later wants and has no reason to spend a GitHub call on again: the branch
// each pull request merges into (slice-diff's base), and each ad hoc
// session's branches' pull requests (session-list's and session-status's).
// Written by every pr-status, merged over the last — a pull request or a
// session this reading did not ask about keeps what an earlier one found.
//
// Something not in it is unread: slice-diff diffs against the default
// branch, as it does where gh cannot answer, and a session's branch reads as
// stale, never as having no pull requests.
type lastReading struct {
	// Bases is each pull request's base branch, by its URL as
	// [gh.NormaliseURL] writes it.
	Bases map[string]string `json:"bases"`
	// Sessions is each session's branches' pull requests, by session ID and
	// branch.
	Sessions map[string]map[string][]headPRJSON `json:"sessions"`
}

// loadLastReading is the last reading on disk, empty where there is none, no
// path to keep one at (a test's Env), or a file that will not parse — each
// read as nothing read, which is what it is.
func (e Env) loadLastReading() lastReading {
	reading := lastReading{Bases: map[string]string{}, Sessions: map[string]map[string][]headPRJSON{}}
	if e.ReadingPath == nil {
		return reading
	}
	path, err := e.ReadingPath()
	if err != nil {
		logging.Action("no last GitHub reading to read: its path is unresolved", "error", err)
		return reading
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logging.Action("could not read the last GitHub reading", "error", err)
		}
		return reading
	}
	var stored lastReading
	if err := json.Unmarshal(data, &stored); err != nil {
		logging.Action("could not parse the last GitHub reading", "error", err)
		return reading
	}
	for url, base := range stored.Bases {
		reading.Bases[url] = base
	}
	for id, branches := range stored.Sessions {
		reading.Sessions[id] = branches
	}
	return reading
}

// saveLastReading writes reading over the file, whole and atomically, so a
// command reading it meanwhile reads the last one or this one and never half
// of either. A failure is logged and changes nothing the caller reports: the
// reading it would have kept is the one it already printed.
func (e Env) saveLastReading(reading lastReading) {
	if e.ReadingPath == nil {
		return
	}
	path, err := e.ReadingPath()
	if err == nil {
		var data []byte
		if data, err = json.Marshal(reading); err == nil {
			err = writeProposalFile(path, data)
		}
	}
	if err != nil {
		logging.Action("could not keep the GitHub reading", "error", err)
	}
}
