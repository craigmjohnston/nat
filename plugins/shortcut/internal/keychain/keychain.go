// Package keychain keeps the Shortcut token in the macOS Keychain, through
// the `security` CLI, under service nat-source-shortcut.
//
// The token never passes through this process on the way in from login:
// login runs `security add-generic-password … -w` with -w last, which makes
// security prompt for it on the terminal itself. From setup — gnat's Settings
// — it does pass through, and goes on to security's stdin, never its argv:
// `security -i` reads its command from stdin, so the token is in no process
// listing. On the way out it is read into memory for one run and never
// printed.
package keychain

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"unicode"
)

// Service is the Keychain item's service name.
const Service = "nat-source-shortcut"

// ErrNoToken is a Keychain with no token, or one that couldn't be read.
var ErrNoToken = errors.New("no Shortcut token in the Keychain")

// Runner runs security. Output captures stdout (stderr discarded); Interactive
// gives the command the terminal, so it can prompt; Feed writes stdin to the
// command and discards everything it prints, since what `security -i` echoes
// back is never worth showing and might quote its input.
type Runner interface {
	Output(name string, args ...string) ([]byte, error)
	Interactive(name string, args ...string) error
	Feed(stdin string, name string, args ...string) error
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

// Has says whether a token is stored, without reading it: find-generic-password
// with no -w prints only the item's attributes, which are discarded. It looks
// up by service alone, exactly as Token does, so the two can never disagree
// about an item stored by hand under some other account. Any failure — no
// item, no security — reads as none.
func (k Keychain) Has() bool {
	_, err := k.Run.Output("security", "find-generic-password", "-s", Service)
	return err == nil
}

// Store asks for a token on the terminal and saves it under account,
// replacing any already there (-U).
func (k Keychain) Store(account string) error {
	return k.Run.Interactive("security", "add-generic-password", "-U", "-s", Service, "-a", account, "-w")
}

// ErrUnstorable is a token or account with a control character in it — a
// line break above all, which would end the command `security -i` reads and
// start another.
var ErrUnstorable = errors.New("the token has a line break or control character in it")

// Save stores token under account, replacing any already there (-U), without
// putting it in argv: `security -i` is run with the add-generic-password
// command as its stdin, each argument double-quoted with `\` and `"` escaped
// — the quoting security's interactive mode reads.
func (k Keychain) Save(account, token string) error {
	if strings.IndexFunc(account+token, unicode.IsControl) >= 0 {
		return ErrUnstorable
	}
	line := "add-generic-password -U -s " + quote(Service) + " -a " + quote(account) + " -w " + quote(token) + "\n"
	return k.Run.Feed(line, "security", "-i")
}

// quote is s as one argument of a `security -i` command line.
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
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

// Feed implements Runner.
func (ExecRunner) Feed(stdin string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = strings.NewReader(stdin), io.Discard, io.Discard
	return cmd.Run()
}
