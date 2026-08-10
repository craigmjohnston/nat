// Package logging writes nat's failures and major actions to a file in the
// platform's standard per-user log location, so that a run whose stderr nobody
// ever sees still leaves a trace.
//
// stderr is not enough on its own: the TUI hosts itself in a tmux session, and
// a process that dies on the way up takes the pane it was writing to with it —
// which is how a startup crash becomes a binary that "does nothing". Everything
// worth explaining that afterwards goes here as well.
//
// Nothing is written until [Open] is called: a package that logs from an
// init would put a file in the home directory of anyone who imported it,
// tests included. Until then, and after [Close], every call is discarded.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

const (
	appDirName = "notion-agent-tracker"

	// fileName is the current log; the previous one sits beside it under
	// fileName+previousSuffix.
	fileName       = "nat.log"
	previousSuffix = ".1"

	// maxBytes is how large the current log grows before it is rotated. A
	// megabyte is thousands of lines — far more than the run that went wrong —
	// and two of them is a bound the user never has to think about.
	maxBytes = 1 << 20
)

// The logger everything goes through, and the file behind it. The default
// discards, so an unopened logger is usable rather than nil.
var (
	mu      sync.Mutex
	current *rotator
	logger  = discarding()
)

// discarding is the logger used before [Open] and after [Close].
func discarding() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// Dir is the directory nat's logs live in: ~/Library/Logs/notion-agent-tracker
// on macOS, and the XDG state directory — $XDG_STATE_HOME, or ~/.local/state —
// everywhere else. Logs are state rather than configuration, so they do not go
// beside the config file.
func Dir() (string, error) { return dirFor(runtime.GOOS) }

// dirFor resolves the log directory for an operating system, taking the OS as
// an argument so both answers are reachable from a test on either platform.
func dirFor(goos string) (string, error) {
	if goos != "darwin" {
		if x := os.Getenv("XDG_STATE_HOME"); x != "" {
			return filepath.Join(x, appDirName), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	if goos == "darwin" {
		return filepath.Join(home, "Library", "Logs", appDirName), nil
	}
	return filepath.Join(home, ".local", "state", appDirName), nil
}

// Path is the full path of the current log file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// Open starts logging to [Path], creating the directory if it is not there and
// appending to whatever is already in the file.
//
// The path is returned even when opening fails, so a caller reporting the
// failure can still say where it was trying to write — which is the same thing
// it would have told the user had it worked.
func Open() (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	r, err := openRotator(path, maxBytes)
	if err != nil {
		return path, err
	}

	mu.Lock()
	defer mu.Unlock()
	if current != nil {
		_ = current.Close()
	}
	current = r
	logger = slog.New(slog.NewTextHandler(redactWriter{w: r}, nil))
	return path, nil
}

// Close stops logging and closes the file. It is safe to call when nothing was
// ever opened.
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if current == nil {
		return nil
	}
	err := current.Close()
	current, logger = nil, discarding()
	return err
}

// Action records something the app did that a later reader would want an
// account of: an agent launched, a slice claimed, a page written. Arguments are
// slog key/value pairs.
func Action(msg string, args ...any) { write(slog.LevelInfo, msg, args...) }

// Error records something that failed. Arguments are slog key/value pairs; an
// error conventionally goes under the key "err".
func Error(msg string, args ...any) { write(slog.LevelError, msg, args...) }

// write hands a record to whichever logger is installed. The logger is read
// under the lock and used outside it, so a call racing an [Open] writes to one
// of the two files rather than to a handler being swapped underneath it.
func write(level slog.Level, msg string, args ...any) {
	mu.Lock()
	l := logger
	mu.Unlock()
	l.Log(context.Background(), level, msg, args...)
}
