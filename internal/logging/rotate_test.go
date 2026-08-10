package logging

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// logPath is a log file path inside a directory of the test's own.
func logPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), fileName)
}

// read returns a file's contents, or "" when it is not there — which is how a
// test says a rotation has not happened yet.
func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRotatorWritesUnderTheCap(t *testing.T) {
	path := logPath(t)
	r, err := openRotator(path, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer r.Close()

	if _, err := r.Write([]byte("one\n")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := r.Write([]byte("two\n")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := read(t, path); got != "one\ntwo\n" {
		t.Errorf("log = %q, want both lines in the one file", got)
	}
	if got := read(t, path+previousSuffix); got != "" {
		t.Errorf("previous log = %q, want nothing rotated yet", got)
	}
}

func TestRotatorMovesTheLogAsideWhenItWouldOverflow(t *testing.T) {
	path := logPath(t)
	r, err := openRotator(path, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer r.Close()

	if _, err := r.Write([]byte("0123456789\n")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Eleven more bytes would make twenty-two, so this one starts a new file.
	if _, err := r.Write([]byte("abcdefghij\n")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := read(t, path); got != "abcdefghij\n" {
		t.Errorf("log = %q, want only the line written after the rotation", got)
	}
	if got := read(t, path+previousSuffix); got != "0123456789\n" {
		t.Errorf("previous log = %q, want the line written before it", got)
	}
}

// Two files is the whole budget: a second rotation drops what the first one
// kept rather than growing a numbered pile.
func TestRotatorKeepsOnlyOnePreviousLog(t *testing.T) {
	path := logPath(t)
	r, err := openRotator(path, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer r.Close()

	for _, line := range []string{"first-line\n", "second-line\n", "third-line\n"} {
		if _, err := r.Write([]byte(line)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if got := read(t, path); got != "third-line\n" {
		t.Errorf("log = %q, want the last line", got)
	}
	if got := read(t, path+previousSuffix); got != "second-line\n" {
		t.Errorf("previous log = %q, want the line before it and not the first", got)
	}
}

// A record longer than the cap is written whole: half a line explains nothing.
func TestRotatorWritesARecordLargerThanTheCap(t *testing.T) {
	path := logPath(t)
	r, err := openRotator(path, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer r.Close()

	if _, err := r.Write([]byte("short\n")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	long := "a line far longer than the cap allows\n"
	if _, err := r.Write([]byte(long)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := read(t, path); got != long {
		t.Errorf("log = %q, want the whole record", got)
	}
}

// A run that appends to a log already at its cap must rotate on its first line
// rather than counting from zero and growing the file forever.
func TestRotatorCountsWhatIsAlreadyInTheFile(t *testing.T) {
	path := logPath(t)
	if err := os.WriteFile(path, []byte("left by an earlier run\n"), logMode); err != nil {
		t.Fatal(err)
	}
	r, err := openRotator(path, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer r.Close()

	if _, err := r.Write([]byte("new run\n")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := read(t, path); got != "new run\n" {
		t.Errorf("log = %q, want only the new run", got)
	}
	if got := read(t, path+previousSuffix); got != "left by an earlier run\n" {
		t.Errorf("previous log = %q, want the earlier run", got)
	}
}

func TestOpenRotatorReportsADirectoryItCannotCreate(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocked, []byte("in the way"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := openRotator(filepath.Join(blocked, "sub", fileName), 100); err == nil {
		t.Error("want the directory creation to fail")
	}
}

func TestOpenRotatorReportsAFileItCannotOpen(t *testing.T) {
	sentinel := errors.New("permission denied")
	stubOpenFile(t, func(string, int, os.FileMode) (*os.File, error) { return nil, sentinel })

	_, err := openRotator(logPath(t), 100)
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want it to wrap %v", err, sentinel)
	}
}

func TestOpenRotatorReportsAFileItCannotMeasure(t *testing.T) {
	stubOpenFile(t, func(name string, flag int, perm os.FileMode) (*os.File, error) {
		f, err := os.OpenFile(name, flag, perm)
		if err != nil {
			return nil, err
		}
		// A closed handle is a file that opened and cannot be measured, which
		// is otherwise not a state a filesystem hands over.
		return f, f.Close()
	})

	if _, err := openRotator(logPath(t), 100); err == nil {
		t.Error("want the measurement to fail")
	}
}

func TestRotatorReportsALogItCannotMoveAside(t *testing.T) {
	sentinel := errors.New("cross-device link")
	old := rename
	t.Cleanup(func() { rename = old })
	rename = func(string, string) error { return sentinel }

	path := logPath(t)
	r, err := openRotator(path, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := r.Write([]byte("0123456\n")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := r.Write([]byte("0123456\n")); !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want it to wrap %v", err, sentinel)
	}
}

func TestRotatorReportsALogItCannotStartAgain(t *testing.T) {
	sentinel := errors.New("permission denied")
	calls := 0
	stubOpenFile(t, func(name string, flag int, perm os.FileMode) (*os.File, error) {
		calls++
		if calls > 1 {
			return nil, sentinel
		}
		return os.OpenFile(name, flag, perm)
	})

	path := logPath(t)
	r, err := openRotator(path, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := r.Write([]byte("0123456\n")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := r.Write([]byte("0123456\n")); !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want it to wrap %v", err, sentinel)
	}
}

func TestRotatorCloses(t *testing.T) {
	r, err := openRotator(logPath(t), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := r.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
	if _, err := r.Write([]byte("after close\n")); err == nil {
		t.Error("want a write to a closed log to fail")
	}
}

// stubOpenFile stands in for the file open the rotator does, restoring the real
// one when the test ends.
func stubOpenFile(t *testing.T, f func(string, int, os.FileMode) (*os.File, error)) {
	t.Helper()
	old := openFile
	t.Cleanup(func() { openFile = old })
	openFile = f
}
