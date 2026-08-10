package logging

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// tempHome points the log location at a directory of the test's own, so nothing
// here writes to the machine's real log directory. XDG_STATE_HOME is cleared
// rather than left alone so the answer does not depend on the environment the
// suite happens to run in.
func tempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	return home
}

// noHome makes the home directory unresolvable, which is the only way the path
// lookup fails.
func noHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
}

// closeAfter shuts the package logger down when the test ends, so one test's
// file is never still installed for the next.
func closeAfter(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { _ = Close() })
}

func TestDirIsTheMacOSLogDirectory(t *testing.T) {
	home := tempHome(t)

	got, err := dirFor("darwin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(home, "Library", "Logs", appDirName); got != want {
		t.Errorf("dirFor(darwin) = %q, want %q", got, want)
	}
}

func TestDirIsTheXDGStateDirectoryElsewhere(t *testing.T) {
	home := tempHome(t)

	got, err := dirFor("linux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(home, ".local", "state", appDirName); got != want {
		t.Errorf("dirFor(linux) = %q, want %q", got, want)
	}
}

// XDG_STATE_HOME is the user saying where state goes, and outranks the default.
func TestDirHonoursXDGStateHome(t *testing.T) {
	tempHome(t)
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)

	got, err := dirFor("linux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(state, appDirName); got != want {
		t.Errorf("dirFor(linux) = %q, want %q", got, want)
	}
}

// macOS has a log directory of its own, so XDG says nothing about where logs go
// there.
func TestDirIgnoresXDGStateHomeOnMacOS(t *testing.T) {
	home := tempHome(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	got, err := dirFor("darwin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(home, "Library", "Logs", appDirName); got != want {
		t.Errorf("dirFor(darwin) = %q, want %q", got, want)
	}
}

func TestDirReportsAnUnresolvableHome(t *testing.T) {
	noHome(t)

	for _, goos := range []string{"darwin", "linux"} {
		if _, err := dirFor(goos); err == nil {
			t.Errorf("dirFor(%s) = nil error, want the home lookup to fail", goos)
		}
	}
}

// Dir is dirFor for the machine it is running on, which is the only thing the
// exported one adds.
func TestDirFollowsTheRunningPlatform(t *testing.T) {
	tempHome(t)

	got, err := Dir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, err := dirFor(runtime.GOOS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestPathIsTheLogFileInTheLogDirectory(t *testing.T) {
	tempHome(t)

	got, err := Path()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dir, err := Dir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(dir, fileName); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestPathReportsAnUnresolvableHome(t *testing.T) {
	noHome(t)

	if _, err := Path(); err == nil {
		t.Error("want the home lookup to fail")
	}
}

func TestOpenWritesActionsAndErrorsToTheLogFile(t *testing.T) {
	tempHome(t)
	closeAfter(t)

	path, err := Open()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	Action("agent launched", "slice", "s1")
	Error("notion request failed", "path", "/pages/p1")
	if err := Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readFile(t, path)
	for _, want := range []string{"level=INFO", "agent launched", "slice=s1", "level=ERROR", "notion request failed", "/pages/p1"} {
		if !strings.Contains(got, want) {
			t.Errorf("log = %q, want it to contain %q", got, want)
		}
	}
}

// Nothing is written until the app asks for it: a package that logged from an
// init would leave a file in the home directory of anything that imported it.
func TestLoggingIsDiscardedUntilOpened(t *testing.T) {
	home := tempHome(t)
	closeAfter(t)

	Action("before open", "slice", "s1")
	Error("before open", "slice", "s1")

	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Errorf("home holds %v (err %v), want nothing written", entries, err)
	}
}

// A second run appends rather than starting again, so the failure that made
// someone look is still there beside the one they are looking at.
func TestOpenAppendsToAnExistingLog(t *testing.T) {
	tempHome(t)
	closeAfter(t)

	path, err := Open()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	Action("first run")
	if err := Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := Open(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	Action("second run")
	if err := Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readFile(t, path)
	if !strings.Contains(got, "first run") || !strings.Contains(got, "second run") {
		t.Errorf("log = %q, want both runs in it", got)
	}
}

// Opening twice without closing in between must not leave the first file open
// with nothing left holding it.
func TestOpenReplacesAnAlreadyOpenLog(t *testing.T) {
	tempHome(t)
	closeAfter(t)

	if _, err := Open(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	first := current
	if _, err := Open(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if current == first {
		t.Fatal("want a fresh file behind the logger")
	}
	if err := first.Close(); err == nil {
		t.Error("want the replaced file to have been closed already")
	}
}

func TestOpenReportsAnUnresolvablePath(t *testing.T) {
	noHome(t)
	closeAfter(t)

	path, err := Open()
	if err == nil {
		t.Fatal("want the home lookup to fail")
	}
	if path != "" {
		t.Errorf("path = %q, want nothing: there is no path to name", path)
	}
}

// A log that cannot be opened still names where it was going, because that is
// what the caller reporting the failure has to tell the user.
func TestOpenReturnsThePathEvenWhenItCannotBeOpened(t *testing.T) {
	tempHome(t)
	closeAfter(t)
	blockLogDir(t)

	path, err := Open()
	if err == nil {
		t.Fatal("want the open to fail")
	}
	want, err := Path()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
}

// blockLogDir puts a plain file where the log directory's parent has to be, so
// creating the directory cannot succeed.
func blockLogDir(t *testing.T) {
	t.Helper()
	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(filepath.Dir(parent), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parent, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCloseWithNothingOpenIsNotAnError(t *testing.T) {
	if err := Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

// After a close the calls go nowhere rather than at a file that has been shut.
func TestLoggingIsDiscardedAfterClose(t *testing.T) {
	tempHome(t)
	closeAfter(t)

	path, err := Open()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	Action("after close")

	if got := readFile(t, path); strings.Contains(got, "after close") {
		t.Errorf("log = %q, want nothing written after the close", got)
	}
}

// readFile reads a file the test wrote through the package.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
