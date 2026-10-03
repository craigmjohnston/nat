package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/plugins/shortcut/internal/keychain"
)

// noTokens is a Keychain stand-in with nothing in it.
type noTokens struct{}

func (noTokens) Token() (string, error)    { return "", errors.New("none") }
func (noTokens) Store(string) error        { return errors.New("none") }
func (noTokens) Save(string, string) error { return errors.New("none") }
func (noTokens) Has() bool                 { return false }

func TestMainWiring(t *testing.T) {
	if a := args(); len(a) == 0 {
		t.Error("the real args are empty")
	}
	if _, ok := tokens.(keychain.Keychain); !ok {
		t.Errorf("the real token source is %T, want the Keychain", tokens)
	}
	defer func(a func() []string, e func(int), g func(string) string, tk any) {
		args, exit, getenv = a, e, g
		tokens = tk.(keychain.Keychain)
	}(args, exit, getenv, tokens)

	var out, errb bytes.Buffer
	stdin, stdout, stderr = strings.NewReader(`{"project":{"id":"p1"}}`), &out, &errb
	args = func() []string { return []string{"nat-source-shortcut", "sidebar"} }
	getenv = func(string) string { return "" }
	tokens = noTokens{}
	code := -1
	exit = func(c int) { code = c }
	main()
	if code != 1 || !strings.HasPrefix(errb.String(), "Shortcut token missing") {
		t.Errorf("main: exit %d, stderr %q", code, errb.String())
	}
}

// The warm-up is started detached from the binary nat ran; a binary that
// cannot be found or started is an error, never a hang.
func TestSpawnDetaches(t *testing.T) {
	defer func(e func() (string, error)) { executable = e }(executable)
	executable = func() (string, error) { return "/usr/bin/true", nil }
	if err := spawn("warm"); err != nil {
		t.Errorf("spawn = %v", err)
	}
	executable = func() (string, error) { return "/no/such/binary", nil }
	if err := spawn("warm"); err == nil {
		t.Error("spawn of a missing binary = nil")
	}
	executable = func() (string, error) { return "", errors.New("unknown") }
	if err := spawn("warm"); err == nil {
		t.Error("spawn with no executable = nil")
	}
}
