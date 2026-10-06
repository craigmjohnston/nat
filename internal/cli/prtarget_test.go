package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
)

const sessionPRURL = "https://github.test/craig/nat/pull/7"

// sessionWithPR files one session whose last batched reading kept pull
// request #7 on its branch, and answers its ID and directory.
func sessionWithPR(t *testing.T) (Env, *strings.Builder, string, string) {
	t.Helper()
	env, out := sessionTestEnv(t)
	dir := t.TempDir()
	id := seedSession(t, env, dir, "session/one")
	keepSessionReading(t, &env, &fakeSessionGH{byBranch: map[string][]gh.HeadPR{
		"session/one": {{Number: 7, Title: "Read a pull request", URL: sessionPRURL, State: "OPEN"}},
	}})
	return env, out, id, dir
}

func TestPRCommentOnASessionsPullRequest(t *testing.T) {
	for _, ref := range []string{sessionPRURL, "7", "https://GITHUB.test/craig/nat/pull/7/?x=1"} {
		t.Run(ref, func(t *testing.T) {
			env, out, id, dir := sessionWithPR(t)
			runner := &fakeCommentRunner{out: sessionPRURL + "#issuecomment-1\n"}
			env.NewGH = func() GH { return gh.NewWithRunner(runner) }

			err := Run(context.Background(), []string{
				"pr-comment", "--session", id, ref, "--body", "Looks good.", "--json", "--project", "project-1",
			}, env)
			if err != nil {
				t.Fatalf("pr-comment --session: %v", err)
			}
			if runner.dir != dir {
				t.Errorf("ran gh in %q, want the session's directory %q", runner.dir, dir)
			}
			if want := "pr comment " + sessionPRURL + " --body-file -"; strings.Join(runner.args, " ") != want {
				t.Errorf("args = %q, want %q", strings.Join(runner.args, " "), want)
			}
			var got prCommentedJSON
			if err := json.Unmarshal([]byte(out.String()), &got); err != nil || got.PR != sessionPRURL {
				t.Errorf("json = %+v (%v), want the session's pull request", got, err)
			}
		})
	}
}

func TestPREditOnASessionsPullRequest(t *testing.T) {
	env, out, id, dir := sessionWithPR(t)
	runner := &fakeCommentRunner{}
	env.NewGH = func() GH { return gh.NewWithRunner(runner) }

	err := Run(context.Background(), []string{
		"pr-edit", "--session", id, "#7", "--body", "A new description.", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("pr-edit --session: %v", err)
	}
	if runner.dir != dir || runner.stdin != "A new description." {
		t.Errorf("ran gh in %q with %q, want the session's directory and the description", runner.dir, runner.stdin)
	}
	if want := "pr edit " + sessionPRURL + " --body-file -"; strings.Join(runner.args, " ") != want {
		t.Errorf("args = %q, want %q", strings.Join(runner.args, " "), want)
	}
	if !strings.Contains(out.String(), sessionPRURL) {
		t.Errorf("output = %s, want the pull request named", out.String())
	}
}

func TestPRWritesOnASessionRefusals(t *testing.T) {
	for _, command := range []string{"pr-comment", "pr-edit"} {
		env, _, id, _ := sessionWithPR(t)
		runner := &fakeCommentRunner{}
		env.NewGH = func() GH { return gh.NewWithRunner(runner) }
		tests := []struct {
			name string
			args []string
			want string
		}{
			{"no pull request named", []string{command, "--session", id}, "want exactly one pull request"},
			{"invalid session", []string{command, "--session", "not-a-uuid", "7"}, "not a slice"},
			{"unknown session", []string{command, "--session", testSessionUUID, "7"}, "no session"},
			{"a pull request it does not hold", []string{command, "--session", id, "8"}, "holds no pull request 8"},
			{"another repository's", []string{command, "--session", id, "https://github.test/other/repo/pull/7"}, "holds no pull request"},
			{"unknown project", []string{command, "--session", id, "7", "--project", "nope"}, "no project nope"},
		}
		for _, tt := range tests {
			args := append(tt.args, "--body", "x")
			if !strings.Contains(strings.Join(args, " "), "--project") {
				args = append(args, "--project", "project-1")
			}
			if err := Run(context.Background(), args, env); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("%s %s: err = %v, want %q", command, tt.name, err, tt.want)
			}
		}
		if runner.args != nil {
			t.Errorf("%s: ran gh %v, want it never run", command, runner.args)
		}
	}
}

// A session the last reading kept nothing for holds no pull request at all.
func TestSessionHeldPRWithNothingKept(t *testing.T) {
	kept := lastReading{Sessions: map[string]map[string][]headPRJSON{}}
	if _, ok := sessionHeldPR(kept, domain.Session{ID: "s"}, "7"); ok {
		t.Error("held a pull request no reading kept")
	}
}
