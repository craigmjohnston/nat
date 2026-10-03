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
