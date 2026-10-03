// Package keychain keeps the Shortcut token in the macOS Keychain, through
// the `security` CLI, under service nat-source-shortcut.
//
// The token never passes through this process on the way in: login runs
// `security add-generic-password … -w` with -w last, which makes security
// prompt for it on the terminal itself. On the way out it is read into
// memory for one run and never printed.
package keychain

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// Service is the Keychain item's service name.
const Service = "nat-source-shortcut"

// ErrNoToken is a Keychain with no token, or one that couldn't be read.
var ErrNoToken = errors.New("no Shortcut token in the Keychain")

// Runner runs security. Output captures stdout (stderr discarded); Interactive
// gives the command the terminal, so it can prompt.
type Runner interface {
	Output(name string, args ...string) ([]byte, error)
	Interactive(name string, args ...string) error
}

// Keychain stores and reads the token.
type Keychain struct {
	Run Runner
}

// Token is the stored token, or ErrNoToken.
func (k Keychain) Token() (string, error) {
	out, err := k.Run.Output("security", "find-generic-password", "-s", Service, "-w")
	tok := strings.TrimSpace(string(out))
	if err != nil || tok == "" {
		return "", ErrNoToken
	}
	return tok, nil
}

// Store asks for a token on the terminal and saves it under account,
// replacing any already there (-U).
func (k Keychain) Store(account string) error {
	return k.Run.Interactive("security", "add-generic-password", "-U", "-s", Service, "-a", account, "-w")
}

// ExecRunner is the real Runner.
type ExecRunner struct{}

// Output implements Runner.
func (ExecRunner) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// Interactive implements Runner.
func (ExecRunner) Interactive(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
