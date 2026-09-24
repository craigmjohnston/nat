package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
)

// acceptEnv is a machine with no Notion at all (accepting a plan must touch
// none), with the proposal directory pointed at a temp dir of the test's own.
func acceptEnv(t *testing.T) (Env, *strings.Builder, *config.Config) {
	t.Helper()
	env, out, saved := noNotionEnv(t, config.Config{}, false)
	state := t.TempDir()
	prev := stateDir
	stateDir = func() (string, error) { return state, nil }
	t.Cleanup(func() { stateDir = prev })
	sb := &strings.Builder{}
	env.Out = sb
	_ = out
	return env, sb, saved
}

// propose files a proposal for a workspace the way the planning agent does.
func propose(t *testing.T, env Env, workspace, name, doc string) {
	t.Helper()
	env.In = strings.NewReader(doc)
	if err := Run(context.Background(), []string{"plan-propose", "--workspace", workspace, "--name", name}, env); err != nil {
		t.Fatalf("plan-propose: %v", err)
	}
}

func TestPlanProposalIsNullUntilOneIsProposed(t *testing.T) {
	env, out, _ := acceptEnv(t)
	if err := Run(context.Background(), []string{"plan-proposal", "--workspace", "ws-1", "--json"}, env); err != nil {
		t.Fatalf("plan-proposal: %v", err)
	}
	var got proposalAnswer
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil || got.Proposal != nil {
		t.Errorf("answer = %q (%v), want a null proposal", out.String(), err)
	}
}

func TestPlanProposalReadsWhatWasProposed(t *testing.T) {
	env, out, _ := acceptEnv(t)
	propose(t, env, "ws-1", "importer", validProposalDoc)
	out.Reset()
	if err := Run(context.Background(), []string{"plan-proposal", "--workspace", "ws-1"}, env); err != nil {
		t.Fatalf("plan-proposal: %v", err)
	}
	var got proposalAnswer
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if got.Proposal == nil || got.Proposal.Name != "importer" || len(got.Proposal.Plan.Slices) != 2 {
		t.Errorf("proposal = %+v", got.Proposal)
	}
}

func TestPlanProposalRefusals(t *testing.T) {
	env, _, _ := acceptEnv(t)
	dir, _ := stateDir()
	if err := os.MkdirAll(filepath.Join(dir, "proposals"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "proposals", "bad.json"), []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]string{
		"no workspace": {"plan-proposal"},
		"extra args":   {"plan-proposal", "--workspace", "ws-1", "stray"},
		"corrupt file": {"plan-proposal", "--workspace", "bad"},
		"unknown flag": {"plan-proposal", "--workspace", "ws-1", "--nope"},
	}
	for name, args := range cases {
		if err := Run(context.Background(), args, env); err == nil {
			t.Errorf("%s: want a refusal", name)
		}
	}
	prev := stateDir
	stateDir = func() (string, error) { return "", errors.New("no state dir") }
	defer func() { stateDir = prev }()
	if err := Run(context.Background(), []string{"plan-proposal", "--workspace", "ws-1"}, env); err == nil {
		t.Error("an unresolvable state directory should refuse")
	}
}

func TestPlanAcceptMakesALocalProjectOfTheProposal(t *testing.T) {
	env, out, saved := acceptEnv(t)
	nudges := nudgeCounter(&env)
	propose(t, env, "ws-1", "importer", validProposalDoc)
	*nudges = 0
	out.Reset()

	if err := Run(context.Background(), []string{"plan-accept", "--workspace", "ws-1", "--name", "  My Importer ", "--json"}, env); err != nil {
		t.Fatalf("plan-accept: %v", err)
	}
	var got planAcceptedJSON
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if got.Milestones != 1 || got.Slices != 2 || got.Project.Name != "My Importer" || got.Project.Backend != "local" {
		t.Errorf("reported %+v", got)
	}
	entry, ok := saved.Projects[got.Project.ID]
	if !ok || entry.Name != "My Importer" || entry.Backend != config.BackendLocal || entry.WorkingDir != "" {
		t.Errorf("config entry = %+v (found %v)", entry, ok)
	}
	if *nudges == 0 {
		t.Error("accepting should nudge the board")
	}
	dir, _ := stateDir()
	if _, err := os.Stat(filepath.Join(dir, "proposals", "ws-1.json")); !os.IsNotExist(err) {
		t.Errorf("the proposal file should be gone, stat err = %v", err)
	}

	// The plan is really in the project: the ordinary read sees it.
	out.Reset()
	if err := Run(context.Background(), []string{"info", "--project", got.Project.ID, "--json"}, env); err != nil {
		t.Fatalf("info: %v", err)
	}
	for _, want := range []string{"Lay the foundation", "Build on it", "M1: Groundwork"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("info lacks %q:\n%s", want, out.String())
		}
	}
}

func TestPlanAcceptMarkdown(t *testing.T) {
	env, out, _ := acceptEnv(t)
	propose(t, env, "ws-1", "importer", validProposalDoc)
	out.Reset()
	if err := Run(context.Background(), []string{"plan-accept", "--workspace", "ws-1", "--name", "Mine"}, env); err != nil {
		t.Fatalf("plan-accept: %v", err)
	}
	if !strings.Contains(out.String(), `Accepted 1 milestone and 2 slices as "Mine"`) {
		t.Errorf("output = %q", out.String())
	}
}

func TestPlanAcceptRefusals(t *testing.T) {
	env, _, saved := acceptEnv(t)
	propose(t, env, "ws-1", "importer", validProposalDoc)
	dir, _ := stateDir()
	// A proposal that no longer validates (written by hand, or by a newer nat).
	if err := os.WriteFile(filepath.Join(dir, "proposals", "empty.json"),
		[]byte(`{"workspace":"empty","name":"x","plan":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]string{
		"no workspace": {"plan-accept", "--name", "N"},
		"no name":      {"plan-accept", "--workspace", "ws-1", "--name", "  "},
		"extra args":   {"plan-accept", "--workspace", "ws-1", "--name", "N", "stray"},
		"no proposal":  {"plan-accept", "--workspace", "ghost", "--name", "N"},
		"invalid plan": {"plan-accept", "--workspace", "empty", "--name", "N"},
		"unknown flag": {"plan-accept", "--workspace", "ws-1", "--name", "N", "--nope"},
	}
	for name, args := range cases {
		if err := Run(context.Background(), args, env); err == nil {
			t.Errorf("%s: want a refusal", name)
		}
	}
	if len(saved.Projects) != 0 {
		t.Errorf("a refused accept made projects: %+v", saved.Projects)
	}
	if _, err := os.Stat(filepath.Join(dir, "proposals", "ws-1.json")); err != nil {
		t.Errorf("a refused accept must leave the proposal: %v", err)
	}
}

func TestPlanAcceptFailsWhereTheProjectCannotBeMade(t *testing.T) {
	env, _, _ := acceptEnv(t)
	propose(t, env, "ws-1", "importer", validProposalDoc)
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, errors.New("config unreadable") }
	if err := Run(context.Background(), []string{"plan-accept", "--workspace", "ws-1", "--name", "N"}, env); err == nil {
		t.Error("want the config failure surfaced")
	}
}

// A proposal file that will not go is no reason to fail an accept that has
// already filed the plan.
func TestPlanAcceptSucceedsThoughTheProposalWillNotBeRemoved(t *testing.T) {
	env, out, _ := acceptEnv(t)
	propose(t, env, "ws-1", "importer", validProposalDoc)
	out.Reset()
	dir, _ := stateDir()
	proposals := filepath.Join(dir, "proposals")
	// A read-only directory: the file reads, but cannot be unlinked.
	if err := os.Chmod(proposals, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(proposals, 0o755) }()
	if err := Run(context.Background(), []string{"plan-accept", "--workspace", "ws-1", "--name", "N"}, env); err != nil {
		t.Fatalf("plan-accept: %v", err)
	}
	if !strings.Contains(out.String(), "Accepted") {
		t.Errorf("output = %q", out.String())
	}
}

// damagePlanOnSave runs damage against the plan file right after project
// creation saves its config entry — the moment the plan exists and nothing has
// yet been filed into it — so the steps after it meet a plan that is broken.
func damagePlanOnSave(t *testing.T, env *Env, damage func(path string)) {
	t.Helper()
	save := env.Save
	env.Save = func(c config.Config) error {
		if err := save(c); err != nil {
			return err
		}
		_ = filepath.WalkDir(os.Getenv("XDG_DATA_HOME"), func(path string, d fs.DirEntry, _ error) error {
			if d != nil && !d.IsDir() && strings.HasSuffix(path, ".db") {
				damage(path)
			}
			return nil
		})
		return nil
	}
}

func dropTable(t *testing.T, table string) func(string) {
	return func(path string) {
		db, err := sql.Open("sqlite3", "file:"+path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		if _, err := db.Exec("DROP TABLE " + table); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPlanAcceptReportsAPlanThatBreaksAfterItWasMade(t *testing.T) {
	cases := map[string]func(t *testing.T) func(string){
		"the plan cannot be opened": func(t *testing.T) func(string) {
			return func(path string) {
				if err := os.WriteFile(path, []byte("not a plan"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		},
		"the milestones cannot be read": func(t *testing.T) func(string) { return dropTable(t, "milestones") },
		"the slices cannot be filed":    func(t *testing.T) func(string) { return dropTable(t, "slices") },
	}
	for name, damage := range cases {
		t.Run(name, func(t *testing.T) {
			env, _, _ := acceptEnv(t)
			propose(t, env, "ws-1", "importer", validProposalDoc)
			damagePlanOnSave(t, &env, damage(t))
			if err := Run(context.Background(), []string{"plan-accept", "--workspace", "ws-1", "--name", "N"}, env); err == nil {
				t.Fatal("want the failure surfaced")
			}
			dir, _ := stateDir()
			if _, err := os.Stat(filepath.Join(dir, "proposals", "ws-1.json")); err != nil {
				t.Errorf("a failed accept must leave the proposal: %v", err)
			}
		})
	}
}

func TestPlanAcceptFailsWhereTheProjectCannotBeReadBack(t *testing.T) {
	env, _, _ := acceptEnv(t)
	propose(t, env, "ws-1", "importer", validProposalDoc)
	load := env.Load
	calls := 0
	env.Load = func() (config.Config, bool, error) {
		calls++
		if calls > 1 {
			return config.Config{}, false, errors.New("config unreadable")
		}
		return load()
	}
	if err := Run(context.Background(), []string{"plan-accept", "--workspace", "ws-1", "--name", "N"}, env); err == nil {
		t.Error("want the config failure surfaced")
	}
}
