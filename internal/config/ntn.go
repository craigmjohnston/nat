package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// NtnBinary is the name of Notion's official CLI, looked up on PATH.
const NtnBinary = "ntn"

// ntnTimeout caps how long we wait for the CLI to hand back a token. The call
// is a local keychain read, so anything slower than this is a hang.
const ntnTimeout = 10 * time.Second

var (
	// ErrNtnNotInstalled is returned when the ntn binary is not on PATH.
	ErrNtnNotInstalled = errors.New("notion CLI (ntn) not found on PATH")

	// ErrNtnNotLoggedIn is returned when ntn is installed but holds no
	// credentials, i.e. `ntn login` has not been run.
	ErrNtnNotLoggedIn = errors.New("notion CLI (ntn) is not logged in")
)

// TokenSource supplies the Notion bearer token used for API requests.
//
// The app does not store a Notion credential of its own: the work workspace
// cannot issue integration tokens or PATs, so the only usable credential is the
// workspace-scoped OAuth token that `ntn login` puts in the OS keychain under
// the service name "notion-cli". We read it back out through the CLI rather
// than touching that keychain entry directly, since its format is ntn's to
// change.
type TokenSource interface {
	Token() (string, error)
}

// NtnCLI is a TokenSource backed by `ntn auth token`.
//
// The exec call is held as a function field so tests can stub it; there is no
// mock mode for a real subprocess.
type NtnCLI struct {
	run func(ctx context.Context, name string, args ...string) ([]byte, []byte, error)
}

var _ TokenSource = (*NtnCLI)(nil)

// NewNtnCLI returns an NtnCLI wired to the real ntn binary on PATH.
func NewNtnCLI() *NtnCLI { return &NtnCLI{run: runCommand} }

// runCommand executes name with args, returning stdout and stderr separately.
func runCommand(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// Token returns the workspace-scoped OAuth token held by the Notion CLI.
//
// The token is deliberately not cached on the struct: ntn owns its lifetime and
// may rotate it, and a token read is a cheap local keychain lookup.
func (n *NtnCLI) Token() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ntnTimeout)
	defer cancel()

	stdout, stderr, err := n.run(ctx, NtnBinary, "auth", "token")
	if err != nil {
		var notFound *exec.Error
		if errors.As(err, &notFound) && errors.Is(notFound.Err, exec.ErrNotFound) {
			return "", ErrNtnNotInstalled
		}
		// A non-zero exit means ntn ran but could not produce a token, which
		// in practice means no stored credentials. Keep its own message: it
		// explains the reason better than we can guess at it.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if detail := strings.TrimSpace(string(stderr)); detail != "" {
				return "", fmt.Errorf("%w: %s", ErrNtnNotLoggedIn, detail)
			}
			return "", ErrNtnNotLoggedIn
		}
		return "", fmt.Errorf("run %s auth token: %w", NtnBinary, err)
	}

	token := strings.TrimSpace(string(stdout))
	if token == "" {
		return "", ErrNtnNotLoggedIn
	}
	return token, nil
}

// StaticToken is a TokenSource returning a fixed token, for tests and for
// callers that already hold a credential.
type StaticToken string

var _ TokenSource = StaticToken("")

// Token returns the fixed token, or ErrNtnNotLoggedIn when empty.
func (s StaticToken) Token() (string, error) {
	if s == "" {
		return "", ErrNtnNotLoggedIn
	}
	return string(s), nil
}
