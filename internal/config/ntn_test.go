package config

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// realExitError produces a genuine *exec.ExitError, which cannot be constructed
// directly: its ProcessState is only filled in by running a process.
func realExitError(t *testing.T) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit 1").Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("want *exec.ExitError, got %T (%v)", err, err)
	}
	return exitErr
}

// realNotFoundError produces a genuine *exec.Error wrapping exec.ErrNotFound.
func realNotFoundError(t *testing.T) error {
	t.Helper()
	err := exec.Command("notion-agent-tracker-no-such-binary").Run()
	var execErr *exec.Error
	if !errors.As(err, &execErr) {
		t.Fatalf("want *exec.Error, got %T (%v)", err, err)
	}
	return execErr
}

// stubRun returns a run func yielding the given results and recording the argv
// it was called with.
func stubRun(stdout, stderr string, err error, gotArgv *[]string) func(context.Context, string, ...string) ([]byte, []byte, error) {
	return func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
		if gotArgv != nil {
			*gotArgv = append([]string{name}, args...)
		}
		return []byte(stdout), []byte(stderr), err
	}
}

func TestTokenReturnsTrimmedStdout(t *testing.T) {
	var argv []string
	n := &NtnCLI{run: stubRun("ntn_o1234\n", "", nil, &argv)}

	got, err := n.Token()
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if got != "ntn_o1234" {
		t.Errorf("token = %q, want %q", got, "ntn_o1234")
	}
	if want := []string{"ntn", "auth", "token"}; !reflect.DeepEqual(argv, want) {
		t.Errorf("argv = %v, want %v", argv, want)
	}
}

func TestTokenEmptyStdoutIsNotLoggedIn(t *testing.T) {
	n := &NtnCLI{run: stubRun("  \n", "", nil, nil)}

	if _, err := n.Token(); !errors.Is(err, ErrNtnNotLoggedIn) {
		t.Errorf("error = %v, want ErrNtnNotLoggedIn", err)
	}
}

func TestTokenMissingBinaryIsNotInstalled(t *testing.T) {
	n := &NtnCLI{run: stubRun("", "", realNotFoundError(t), nil)}

	if _, err := n.Token(); !errors.Is(err, ErrNtnNotInstalled) {
		t.Errorf("error = %v, want ErrNtnNotInstalled", err)
	}
}

func TestTokenNonZeroExitIsNotLoggedInAndKeepsStderr(t *testing.T) {
	n := &NtnCLI{run: stubRun("", "  no credentials found\n", realExitError(t), nil)}

	_, err := n.Token()
	if !errors.Is(err, ErrNtnNotLoggedIn) {
		t.Fatalf("error = %v, want ErrNtnNotLoggedIn", err)
	}
	if !strings.Contains(err.Error(), "no credentials found") {
		t.Errorf("error %q does not carry ntn's stderr", err)
	}
}

func TestTokenNonZeroExitWithoutStderr(t *testing.T) {
	n := &NtnCLI{run: stubRun("", "   ", realExitError(t), nil)}

	_, err := n.Token()
	if !errors.Is(err, ErrNtnNotLoggedIn) {
		t.Fatalf("error = %v, want ErrNtnNotLoggedIn", err)
	}
	if strings.Contains(err.Error(), ":") && strings.HasSuffix(err.Error(), " ") {
		t.Errorf("error %q has a dangling detail separator", err)
	}
}

func TestTokenOtherErrorIsWrapped(t *testing.T) {
	sentinel := errors.New("context deadline exceeded")
	n := &NtnCLI{run: stubRun("", "", sentinel, nil)}

	_, err := n.Token()
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want it to wrap %v", err, sentinel)
	}
	if errors.Is(err, ErrNtnNotLoggedIn) || errors.Is(err, ErrNtnNotInstalled) {
		t.Errorf("error %v was misclassified as an auth problem", err)
	}
	if !strings.Contains(err.Error(), "auth token") {
		t.Errorf("error %q does not say which command failed", err)
	}
}

func TestNewNtnCLIIsWiredToRealExec(t *testing.T) {
	n := NewNtnCLI()
	if n.run == nil {
		t.Fatal("NewNtnCLI() left run nil")
	}
}

func TestRunCommandSeparatesStreams(t *testing.T) {
	stdout, stderr, err := runCommand(context.Background(), "sh", "-c", "echo out; echo err >&2")
	if err != nil {
		t.Fatalf("runCommand() error: %v", err)
	}
	if got := strings.TrimSpace(string(stdout)); got != "out" {
		t.Errorf("stdout = %q, want %q", got, "out")
	}
	if got := strings.TrimSpace(string(stderr)); got != "err" {
		t.Errorf("stderr = %q, want %q", got, "err")
	}
}

func TestRunCommandReportsExitFailure(t *testing.T) {
	_, _, err := runCommand(context.Background(), "sh", "-c", "exit 3")

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error = %T (%v), want *exec.ExitError", err, err)
	}
	if got := exitErr.ExitCode(); got != 3 {
		t.Errorf("exit code = %d, want 3", got)
	}
}

func TestRunCommandReportsMissingBinary(t *testing.T) {
	_, _, err := runCommand(context.Background(), "notion-agent-tracker-no-such-binary")

	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("error = %v, want it to wrap exec.ErrNotFound", err)
	}
}

func TestStaticTokenReturnsItsValue(t *testing.T) {
	got, err := StaticToken("ntn_o1234").Token()
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if got != "ntn_o1234" {
		t.Errorf("token = %q, want %q", got, "ntn_o1234")
	}
}

func TestStaticTokenEmptyIsNotLoggedIn(t *testing.T) {
	if _, err := StaticToken("").Token(); !errors.Is(err, ErrNtnNotLoggedIn) {
		t.Errorf("error = %v, want ErrNtnNotLoggedIn", err)
	}
}
