package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/source"
	"github.com/craigmjohnston/nat/internal/store"
)

// runSliceID is the slice a slice-scoped run is asked for.
const runSliceID = "3ef38308-f654-8197-b70f-df65ce31f137"

// runTmux stands in for tmux under `nat run`: nothing of the run's name is
// live, and every call is recorded by subcommand.
type runTmux struct {
	calls     [][]string
	launchErr error
}

func (r *runTmux) Run(_ string, args ...string) (string, error) {
	args = args[1:] // the -u client flag
	r.calls = append(r.calls, args)
	switch args[0] {
	case "has-session":
		return "", &agent.ExitError{Code: 1, Stderr: "can't find session"}
	case "new-session":
		return "%4\n", r.launchErr
	}
	return "", nil
}

func (r *runTmux) call(sub string) []string {
	for _, c := range r.calls {
		if c[0] == sub {
			return c
		}
	}
	return nil
}

// runEnv is a config of one project whose working directory is a repository,
// with the runs given, and tmux, git and the worktrees all fakes.
func runEnv(t *testing.T, runs ...config.RunCommand) (Env, *strings.Builder, *runTmux, *fakeSessionWorktrees, *fakeSessionRepo) {
	t.Helper()
	cfg := testConfig(t)
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	p := cfg.Projects["project-1"]
	p.WorkingDir, p.Runs = repo, runs
	cfg.Projects["project-1"] = p
	env, _ := testEnv(cfg, &fakeAPI{})
	var out strings.Builder
	env.Out = &out
	tmux, wt, git := &runTmux{}, &fakeSessionWorktrees{}, &fakeSessionRepo{base: "origin/main"}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(tmux) }
	env.NewWorktrees = func() actions.Worktrees { return wt }
	env.NewGit = func() GitCLI { return git }
	return env, &out, tmux, wt, git
}

var (
	runGlobal = config.RunCommand{Label: "Serve", Command: "make serve", Scope: config.RunScopeGlobal}
	runBoth   = config.RunCommand{Label: "Play", Command: "./play"}
	runSlice  = config.RunCommand{Label: "Test", Command: "make test", Scope: config.RunScopeSlice}
)

// A global run with no label is the first global run, run in nat's run
// checkout — fetched, cut where there was none, reset to origin's default —
// in a session named after the project and tagged as a run.
func TestRunGlobalRunsTheDefaultInTheRunCheckout(t *testing.T) {
	env, out, tmux, wt, git := runEnv(t, runSlice, runGlobal, runBoth)
	if err := Run(context.Background(), []string{"run", "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("run: %v", err)
	}
	var doc runJSON
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out.String())
	}
	wantDir := filepath.Join(env.mustProject(t).WorkingDir+".worktrees", actions.RunBranch)
	if doc != (runJSON{Session: agent.RunSessionName("project-1", "Serve"), Label: "Serve", Command: "make serve", Dir: wantDir}) {
		t.Errorf("doc = %+v (dir want %s)", doc, wantDir)
	}
	if len(git.fetched) != 1 || len(wt.created) != 1 || wt.created[0].branch != actions.RunBranch ||
		len(wt.resets) != 1 || wt.resets[0] != (struct{ path, ref string }{wantDir, "origin/main"}) {
		t.Errorf("fetched %v created %+v resets %+v", git.fetched, wt.created, wt.resets)
	}
	ns := strings.Join(tmux.call("new-session"), " ")
	if !strings.Contains(ns, "-c "+wantDir) || !strings.Contains(ns, "sh -c make serve") {
		t.Errorf("new-session = %s", ns)
	}
	if tag := tmux.call("set-option"); !slices.Contains(tag, agent.RunPaneOption) || tag[len(tag)-1] != "project-1:Serve" {
		t.Errorf("tag = %v", tag)
	}
}

// --label picks a run of the scope, case ignored; markdown says the same as
// the JSON.
func TestRunGlobalPicksALabel(t *testing.T) {
	env, out, tmux, _, _ := runEnv(t, runGlobal, runBoth)
	if err := Run(context.Background(), []string{"run", "--label", "play", "--project", "project-1"}, env); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"# Run started", "- Label: Play", "- Command: ./play", "- Session: " + agent.RunSessionName("project-1", "Play")} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
	if !strings.Contains(strings.Join(tmux.call("new-session"), " "), "sh -c ./play") {
		t.Errorf("new-session = %v", tmux.call("new-session"))
	}
}

// A slice-scoped run is run in the slice's own worktree, its session named
// after the slice.
func TestRunSliceRunsInTheSlicesWorktree(t *testing.T) {
	env, out, tmux, wt, git := runEnv(t, runGlobal, runSlice)
	seedHydratedSlice(t, "project-1", runSliceID, "Thing", "In progress", func(db *sql.DB) {
		if _, err := db.Exec(`UPDATE slices SET branch = 'slice/thing'`); err != nil {
			t.Fatal(err)
		}
	})
	wt.existingPath = "/repos/nat.worktrees/slice-thing"
	if err := Run(context.Background(), []string{"run", "--slice", runSliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("run: %v", err)
	}
	var doc runJSON
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatal(err)
	}
	if doc != (runJSON{Session: agent.RunSessionName(runSliceID, "Test"), Label: "Test", Command: "make test", Dir: wt.existingPath}) {
		t.Errorf("doc = %+v", doc)
	}
	if len(git.fetched) != 0 || len(wt.resets) != 0 {
		t.Errorf("a slice's run fetched %v / reset %+v; its worktree is the agent's", git.fetched, wt.resets)
	}
	if tag := tmux.call("set-option"); tag[len(tag)-1] != runSliceID+":Test" {
		t.Errorf("tag = %v", tag)
	}
}

func TestRunRefusals(t *testing.T) {
	for _, tt := range []struct {
		name string
		runs []config.RunCommand
		args []string
		seed string // the slice's status, seeded when set
		want string
	}{
		{"no global runs", []config.RunCommand{runSlice}, nil, "", "the project has no global runs"},
		{"no slice runs", []config.RunCommand{runGlobal}, []string{"--slice", runSliceID}, "", "the project has no slice-scoped runs"},
		{"unknown label", []config.RunCommand{runGlobal, runBoth}, []string{"--label", "Debug"}, "",
			`no global run is labelled "Debug": the project's are Serve, Play`},
		{"merged slice", []config.RunCommand{runSlice}, []string{"--slice", runSliceID}, "Done", `"Thing" is merged`},
		{"no worktree", []config.RunCommand{runSlice}, []string{"--slice", runSliceID}, "In progress", `"Thing" has no worktree for slice/thing`},
		{"not a slice", []config.RunCommand{runSlice}, []string{"--slice", "nope"}, "", `"nope" is not a slice`},
		{"unreadable slice", []config.RunCommand{runSlice}, []string{"--slice", runSliceID}, "", "load the slice"},
		{"positional", []config.RunCommand{runGlobal}, []string{"extra"}, "", "takes no positional arguments"},
		{"bad flag", []config.RunCommand{runGlobal}, []string{"--nope"}, "", "nope"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env, _, tmux, _, _ := runEnv(t, tt.runs...)
			if tt.seed != "" {
				seedHydratedSlice(t, "project-1", runSliceID, "Thing", tt.seed, func(db *sql.DB) {
					if _, err := db.Exec(`UPDATE slices SET branch = 'slice/thing'`); err != nil {
						t.Fatal(err)
					}
				})
			}
			err := Run(context.Background(), append([]string{"run", "--project", "project-1"}, tt.args...), env)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if tmux.call("new-session") != nil {
				t.Error("a refused run started a session")
			}
		})
	}
}

// A source task with no repository recorded is refused in its own words.
func TestRunRefusesASourceTaskWithNoRepository(t *testing.T) {
	sp := newSourceProject(t, &source.Fake{Details: map[string]source.ContainerDetail{"c1": {ID: "c1", Title: "Card"}}})
	task := sp.addTask(t, "Task", "c1")
	p := sp.saved.Projects[sp.id]
	p.Runs = []config.RunCommand{runSlice}
	sp.saved.Projects[sp.id] = p
	sp.env.NewWorktrees = func() actions.Worktrees { return &fakeSessionWorktrees{} }
	err := sp.fail(t, "run", "--slice", task, "--project", sp.id)
	if err == nil || !strings.Contains(err.Error(), "no repository recorded yet") {
		t.Fatalf("err = %v", err)
	}
}

// A project with no working directory, and a tmux that will not start the
// session, each refuse; so does a project nobody tracks.
func TestRunFailures(t *testing.T) {
	env, _, _, _, _ := runEnv(t, runGlobal)
	cfg, _, _ := env.Load()
	p := cfg.Projects["project-1"]
	p.WorkingDir = ""
	cfg.Projects["project-1"] = p
	env.Load = func() (config.Config, bool, error) { return cfg, true, nil }
	if err := Run(context.Background(), []string{"run", "--project", "project-1"}, env); err == nil || !strings.Contains(err.Error(), "no working directory") {
		t.Errorf("no working dir: err = %v", err)
	}

	env, _, tmux, _, _ := runEnv(t, runGlobal)
	tmux.launchErr = errors.New("tmux broke")
	if err := Run(context.Background(), []string{"run", "--project", "project-1"}, env); err == nil || !strings.Contains(err.Error(), "tmux broke") {
		t.Errorf("tmux: err = %v", err)
	}

	if err := Run(context.Background(), []string{"run", "--project", "nope"}, env); err == nil {
		t.Error("unknown project: want an error")
	}
}

// mustProject is the project-1 entry of the env's config.
func (e Env) mustProject(t *testing.T) config.ProjectConfig {
	t.Helper()
	cfg, _, err := e.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Projects["project-1"]
}

// A plan that cannot be opened refuses a slice-scoped run before tmux is
// asked for anything.
func TestRunRefusesAnUnopenablePlan(t *testing.T) {
	env, _, tmux, _, _ := runEnv(t, runSlice)
	path, err := store.LocalPath("project-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"run", "--slice", runSliceID, "--project", "project-1"}, env); err == nil {
		t.Error("want an error")
	}
	if tmux.call("new-session") != nil {
		t.Error("a refused run started a session")
	}
}
