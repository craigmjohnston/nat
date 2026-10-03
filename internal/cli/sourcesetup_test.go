package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/source"
)

// setupEnv is an Env whose only plugin is fake, named shortcut, describing
// itself with one secret setup field, token; stdin is in.
func setupEnv(t *testing.T, fake *source.Fake, in string) (Env, *bytes.Buffer) {
	t.Helper()
	if fake.DescribeResult.Protocol == 0 {
		fake.DescribeResult = demoDescribe()
		fake.DescribeResult.Setup = []source.SetupField{{ID: "token", Label: "API token", Input: source.InputSecret}}
	}
	var out bytes.Buffer
	return Env{
		Out: &out,
		In:  strings.NewReader(in),
		NewSource: func(name string) (source.Client, error) {
			if name != "shortcut" {
				return nil, errors.New("no task source plugin named " + name)
			}
			return fake, nil
		},
	}, &out
}

func TestSourceSetup(t *testing.T) {
	fake := &source.Fake{SetupMessage: "Logged in to scratch as Craig"}
	env, out := setupEnv(t, fake, "s3cret\n")
	if err := Run(context.Background(), []string{"source-setup", "shortcut", "--id", "token", "--json"}, env); err != nil {
		t.Fatal(err)
	}
	if out.String() != "{\n  \"message\": \"Logged in to scratch as Craig\"\n}\n" {
		t.Errorf("json = %q", out.String())
	}
	// All of stdin, only the one trailing newline trimmed — a value's own
	// spaces are the plugin's to judge.
	env.In = strings.NewReader(" two words \r\n")
	out.Reset()
	if err := Run(context.Background(), []string{"source-setup", "shortcut", "--id", "token"}, env); err != nil {
		t.Fatal(err)
	}
	if out.String() != "Logged in to scratch as Craig\n" {
		t.Errorf("text = %q", out.String())
	}
	want := []source.SetupCall{{ID: "token", Input: "s3cret"}, {ID: "token", Input: " two words "}}
	if len(fake.Setups) != 2 || fake.Setups[0] != want[0] || fake.Setups[1] != want[1] {
		t.Errorf("sent %+v, want %+v", fake.Setups, want)
	}

	// A plugin with nothing to say is still a success.
	fake.SetupMessage = ""
	env.In = strings.NewReader("v")
	out.Reset()
	if err := Run(context.Background(), []string{"source-setup", "shortcut", "--id", "token"}, env); err != nil || out.String() != "Set shortcut's token.\n" {
		t.Errorf("no message = %q, %v", out.String(), err)
	}
}

func TestSourceSetupRefusals(t *testing.T) {
	ctx := context.Background()
	var usage *UsageError
	for _, args := range [][]string{
		{},
		{"a", "b", "--id", "token"},
		{" ", "--id", "token"},
		{"shortcut"},
		{"shortcut", "--bogus"},
	} {
		env, _ := setupEnv(t, &source.Fake{}, "v")
		if err := Run(ctx, append([]string{"source-setup"}, args...), env); !errors.As(err, &usage) {
			t.Errorf("%v = %v, want a usage error", args, err)
		}
	}

	for _, c := range []struct {
		name string
		fake *source.Fake
		args []string
		in   string
		want string
	}{
		{"not installed", &source.Fake{}, []string{"nope", "--id", "token"}, "v", "no task source plugin named nope"},
		{"will not describe", &source.Fake{DescribeErr: errors.New("describe broke")}, []string{"shortcut", "--id", "token"}, "v", "describe broke"},
		{"another protocol", &source.Fake{DescribeResult: source.Describe{Protocol: 2}}, []string{"shortcut", "--id", "token"}, "v", "speaks protocol 2"},
		{"an unknown id", &source.Fake{}, []string{"shortcut", "--id", "workspace"}, "v", `source-setup: shortcut has no setup field "workspace"`},
		{"an empty value", &source.Fake{}, []string{"shortcut", "--id", "token"}, " \n", "source-setup: no value for shortcut's token on stdin"},
		{"the plugin refuses", &source.Fake{SetupErr: errors.New("Shortcut refused the token")}, []string{"shortcut", "--id", "token"}, "bad", "Shortcut refused the token"},
	} {
		env, _ := setupEnv(t, c.fake, c.in)
		err := Run(ctx, append([]string{"source-setup"}, c.args...), env)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s = %v, want %q", c.name, err, c.want)
		}
		if c.name != "the plugin refuses" && len(c.fake.Setups) != 0 {
			t.Errorf("%s: the value was sent anyway", c.name)
		}
	}

	// No stdin at all, and one that cannot be read.
	env, _ := setupEnv(t, &source.Fake{}, "")
	env.In = nil
	if err := Run(ctx, []string{"source-setup", "shortcut", "--id", "token"}, env); !errors.As(err, &usage) {
		t.Errorf("nil stdin = %v, want a usage error", err)
	}
	env.In = failingReader{}
	if err := Run(ctx, []string{"source-setup", "shortcut", "--id", "token"}, env); err == nil || !strings.Contains(err.Error(), "source-setup: read the value") {
		t.Errorf("unreadable stdin = %v", err)
	}
}
