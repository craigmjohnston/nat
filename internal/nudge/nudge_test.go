package nudge

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// stubDir points the marker at a directory of the test's own, so nothing here
// ever stats or writes the real state directory.
func stubDir(t *testing.T, path string) {
	t.Helper()
	prev := dir
	dir = func() (string, error) { return path, nil }
	t.Cleanup(func() { dir = prev })
}

func TestTouchCreatesTheMarkerAndItsDirectory(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	stubDir(t, state)

	Touch()

	mtime, ok := Stat()
	if !ok {
		t.Fatal("Stat() found no marker after a Touch")
	}
	if mtime.IsZero() {
		t.Error("Stat() returned a zero mtime for a marker that is there")
	}
}

func TestTouchMovesTheMarkersMtime(t *testing.T) {
	stubDir(t, t.TempDir())
	Touch()
	path, err := Path()
	if err != nil {
		t.Fatalf("Path() = %v", err)
	}
	// The first touch is aged rather than slept past, so the second provably
	// moved the mtime instead of landing inside the clock's resolution.
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("age the marker: %v", err)
	}

	Touch()

	mtime, ok := Stat()
	if !ok {
		t.Fatal("Stat() found no marker after a Touch")
	}
	if !mtime.After(old) {
		t.Errorf("mtime = %v, want after %v: the second Touch should move it", mtime, old)
	}
}

func TestStatWithoutAMarkerReportsNothing(t *testing.T) {
	stubDir(t, t.TempDir())

	if mtime, ok := Stat(); ok {
		t.Errorf("Stat() = %v, %v, want no reading: nothing has ever touched the marker", mtime, ok)
	}
}

func TestAnUnresolvableStateDirIsSwallowed(t *testing.T) {
	prev := dir
	dir = func() (string, error) { return "", errors.New("no home") }
	t.Cleanup(func() { dir = prev })

	if _, err := Path(); err == nil {
		t.Error("Path() should surface the unresolvable dir")
	}
	Touch() // must not panic: the touch is fire-and-forget
	if _, ok := Stat(); ok {
		t.Error("Stat() should have no reading when the dir cannot be resolved")
	}
}

func TestAnUnwritableStateDirIsSwallowed(t *testing.T) {
	// The "directory" is a file, so creating it as a directory fails.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatalf("write the blocker: %v", err)
	}
	stubDir(t, filepath.Join(blocker, "state"))

	Touch() // must not panic or fail: the touch is fire-and-forget
	if _, ok := Stat(); ok {
		t.Error("Stat() should have no reading when the marker could not be written")
	}
}
