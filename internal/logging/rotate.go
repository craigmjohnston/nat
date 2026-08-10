package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// The filesystem calls the rotation makes, held as variables so tests can stand
// in for them: a rename that fails and a file that will not open are otherwise
// hard to arrange without leaving the temporary directory in a state the test
// framework cannot clean up.
var (
	openFile = os.OpenFile
	rename   = os.Rename
)

// logMode is what a log file is created with: readable and writable by its
// owner only. A log is not secret, but it is nobody else's business either.
const logMode = 0o600

// rotator is an io.Writer over a size-capped log file. When a write would take
// the file past its cap the current log is moved aside as the previous one and
// a new one started, so the pair of them is all the disk this ever uses.
type rotator struct {
	mu   sync.Mutex
	path string
	max  int64
	size int64
	f    *os.File
}

// openRotator opens path for appending, creating its directory if needed, and
// returns a rotator that keeps it under max bytes.
func openRotator(path string, max int64) (*rotator, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	f, err := openFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, logMode)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	// The size is read from the file rather than counted from zero, so a second
	// run appending to a log that is already at its cap rotates on its first
	// line instead of growing it unboundedly.
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("measure log file: %w", err)
	}
	return &rotator{path: path, max: max, size: info.Size(), f: f}, nil
}

// Write appends p to the log, rotating first when it would not fit.
//
// A record longer than the whole cap is still written whole rather than split:
// half a line explains nothing, and the rotation it triggers means it is the
// only thing in the new file.
func (r *rotator) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size > 0 && r.size+int64(len(p)) > r.max {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

// rotate moves the current log aside as the previous one and starts a new one.
// Exactly one previous file is kept — enough to cover a failure that has
// already scrolled past — and the rename drops whatever the older one held.
func (r *rotator) rotate() error {
	// The handle is being given up either way, so a close that fails is not
	// worth failing the write that prompted the rotation.
	_ = r.f.Close()
	if err := rename(r.path, r.path+previousSuffix); err != nil {
		return fmt.Errorf("rotate log file: %w", err)
	}
	f, err := openFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, logMode)
	if err != nil {
		return fmt.Errorf("reopen log file: %w", err)
	}
	r.f, r.size = f, 0
	return nil
}

// Close closes the underlying file.
func (r *rotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f.Close()
}
