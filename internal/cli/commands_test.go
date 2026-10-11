package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// leaves is every runnable command line of the tree, as its words.
func leaves(cmds []*command, path []string) [][]string {
	var out [][]string
	for _, c := range cmds {
		p := append(append([]string{}, path...), c.name)
		if c.run != nil {
			out = append(out, p)
		}
		out = append(out, leaves(c.subs, p)...)
	}
	return out
}

// usageError runs args and hands back the *UsageError it must fail with.
func usageError(t *testing.T, args ...string) *UsageError {
	t.Helper()
	env, out := testEnv(testConfig(t), &fakeAPI{})
	err := Run(context.Background(), args, env)
	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("%v: err = %v (%T), want a *UsageError", args, err, err)
	}
	if out.Len() != 0 {
		t.Errorf("%v: output = %q, want nothing", args, out.String())
	}
	return usage
}

func TestAnOldHyphenatedNameIsRefusedNamingItsNewForm(t *testing.T) {
	got := usageError(t, "slice-show", "s1", "--project", "project-1").Message
	if want := "use: nat slice show"; !strings.Contains(got, want) {
		t.Errorf("message = %q, want it to say %q", got, want)
	}
	for old, now := range renamed() {
		if msg := usageError(t, old).Message; !strings.HasSuffix(msg, "use: nat "+now) {
			t.Errorf("%s: message = %q, want it to end naming nat %s", old, msg, now)
		}
	}
}

func TestTheVerbFirstNamesSwapRound(t *testing.T) {
	r := renamed()
	for old, want := range map[string]string{
		"next-slice":          "slice next",
		"start-slice":         "slice start",
		"complete-slice":      "slice complete",
		"release-slice":       "slice release",
		"done-clear":          "project done clear",
		"slice-checks-rerun":  "slice checks rerun",
		"project-open-folder": "project open folder",
		"plugin-source-add":   "plugin source add",
	} {
		if r[old] != want {
			t.Errorf("%s is now %q, want %q", old, r[old], want)
		}
	}
}

func TestAGroupAloneIsAUsageErrorCarryingItsOwnUsage(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		want    []string
		notWant []string
	}{
		{[]string{"slice"}, []string{"usage of nat slice:", "  nat slice show <slice>", "  nat slice checks rerun"},
			[]string{"nat pr view", "nat session list", "nat info"}},
		{[]string{"slice", "checks"}, []string{"usage of nat slice checks:", "  nat slice checks <slice>",
			"  nat slice checks rerun", "  nat slice checks cancel", "re-run the pull request"},
			[]string{"nat slice show", "nat slice cancel"}},
		{[]string{"plugin", "source"}, []string{"  nat plugin source add", "  nat plugin source remove"},
			[]string{"nat plugin list"}},
		{[]string{"project", "open"}, []string{"  nat project open folder <DIR>"}, []string{"nat project create"}},
	} {
		msg := usageError(t, tc.args...).Message
		for _, w := range tc.want {
			if !strings.Contains(msg, w) {
				t.Errorf("%v: message lacks %q:\n%s", tc.args, w, msg)
			}
		}
		for _, w := range tc.notWant {
			if strings.Contains(msg, w) {
				t.Errorf("%v: message has %q, another group's:\n%s", tc.args, w, msg)
			}
		}
	}
}

func TestAnUnknownWordUnderAGroupIsNamedWithTheGroupsUsage(t *testing.T) {
	msg := usageError(t, "slice", "bogus", "--project", "project-1").Message
	if !strings.Contains(msg, `unknown command "slice bogus"`) || !strings.Contains(msg, "nat slice show") {
		t.Errorf("message = %q, want the unknown words named over the slice group's usage", msg)
	}
}

func TestTheHelpListsEveryCommandAndNoOldName(t *testing.T) {
	for _, words := range leaves(commands, nil) {
		if line := "  nat " + strings.Join(words, " "); !strings.Contains(Usage, line+" ") && !strings.Contains(Usage, line+"\n") {
			t.Errorf("the help has no entry for nat %s", strings.Join(words, " "))
		}
	}
	for old := range renamed() {
		if strings.Contains(Usage, old) {
			t.Errorf("the help still names %s", old)
		}
	}
}

func TestEveryHelpEntryReachesACommand(t *testing.T) {
	for _, line := range strings.Split(Usage, "\n") {
		// "  nat  " with no word after it is the board itself.
		if !strings.HasPrefix(line, "  nat ") || strings.HasPrefix(line, "  nat  ") {
			continue
		}
		words := strings.Fields(line)[1:]
		if len(words) == 0 || words[0] == "help" {
			continue
		}
		cmd := find(commands, words[0])
		if cmd == nil {
			t.Errorf("help entry %q names no command", line)
			continue
		}
		for _, w := range words[1:] {
			next := find(cmd.subs, w)
			if next == nil {
				break
			}
			cmd = next
		}
		if cmd.run == nil {
			t.Errorf("help entry %q names no command", line)
		}
	}
}
