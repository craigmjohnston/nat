// Package nudge is the one-way signal from the headless commands to a running
// board: a marker file whose modification time says "a nat command has just
// written to Notion". A command touches it after a successful write and moves
// on; the board stats it on a short timer and refetches the plan when the time
// moves. The mtime is the whole message — nothing is stored in the file, and
// nothing is ever read out of it.
//
// The signal is strictly fire-and-forget on the writing side. A headless
// command never blocks, fails, or cares whether a board is listening: a touch
// that cannot land is logged and swallowed, and a missing state directory is
// created silently.
package nudge

import (
	"os"
	"path/filepath"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// markerName is the marker's file name in the state directory.
const markerName = "nudge"

// dir resolves the state directory the marker lives in — the same one the log
// file uses — held as a variable so tests can point it at a directory of their
// own rather than the real home.
var dir = logging.Dir

// Path is the marker file's full path.
func Path() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, markerName), nil
}

// Touch moves the marker's modification time to now, creating the state
// directory and the file as needed. A failure is logged and swallowed: the
// touch is a courtesy to a board that may not even be running, and no command
// should fail over it.
func Touch() {
	path, err := Path()
	if err == nil {
		if err = os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
			// An empty write is the touch: it creates the file the first time and
			// moves the mtime every time after.
			err = os.WriteFile(path, nil, 0o644)
		}
	}
	if err != nil {
		logging.Error("could not touch the nudge marker", "err", err)
	}
}

// Stat reads the marker's modification time, reporting false when there is
// nothing to read — no command has ever touched it, or the stat itself failed.
// The two are not worth telling apart: either way there is no reading, and the
// board simply waits for one.
func Stat() (time.Time, bool) {
	path, err := Path()
	if err != nil {
		return time.Time{}, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	return info.ModTime(), true
}
