package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/store"
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
	if err := Run(context.Background(), []string{"plan-accept", "--workspace", "ws-1", "--name", "N"}, env); err == nil ||
		!strings.Contains(err.Error(), "resolve the proposal file") {
		t.Errorf("err = %v, want plan-accept refused at the proposal file", err)
	}
}

// plan-proposal reads the same proposal back by --project as by --workspace.
func TestPlanProposalWithProjectReadsWhatWasProposed(t *testing.T) {
	env, out, _ := acceptEnv(t)
	id := makeLocalProject(t, env)
	proposeToProject(t, env, id, validProposalDoc)
	out.Reset()

	if err := Run(context.Background(), []string{"plan-proposal", "--project", id}, env); err != nil {
		t.Fatalf("plan-proposal --project: %v", err)
	}
	var got proposalAnswer
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if got.Proposal == nil || got.Proposal.Project != id || len(got.Proposal.Plan.Slices) != 2 {
		t.Errorf("proposal = %+v", got.Proposal)
	}
}

// Both or neither of --workspace/--project is a usage error for
// plan-proposal too.
func TestPlanProposalRequiresExactlyOneOfWorkspaceOrProject(t *testing.T) {
	env, _, _ := acceptEnv(t)
	if err := Run(context.Background(), []string{"plan-proposal"}, env); err == nil ||
		!strings.Contains(err.Error(), "exactly one of --workspace or --project") {
		t.Errorf("err = %v, want the refusal naming exactly one of the two", err)
	}
	if err := Run(context.Background(), []string{"plan-proposal", "--workspace", "ws-1", "--project", "p1"}, env); err == nil ||
		!strings.Contains(err.Error(), "exactly one of --workspace or --project") {
		t.Errorf("err = %v, want the refusal naming exactly one of the two", err)
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

// makeLocalProject creates a local project through the same path
// project-create --local uses, inside an acceptEnv-style env, and returns
// its ID.
func makeLocalProject(t *testing.T, env Env) string {
	t.Helper()
	var out strings.Builder
	prevOut := env.Out
	env.Out = &out
	defer func() { env.Out = prevOut }()
	if err := Run(context.Background(), []string{"project-create", "Tracked project", "--local",
		"--repo", "/src/tracked", "--json"}, env); err != nil {
		t.Fatalf("project-create --local: %v", err)
	}
	var got projectCreatedJSON
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("project-create output not JSON: %v\n%s", err, out.String())
	}
	return got.Project.ID
}

// proposeToProject files a proposal for an existing project the way a
// gnat-launched planning agent does.
func proposeToProject(t *testing.T, env Env, projectID, doc string) {
	t.Helper()
	env.In = strings.NewReader(doc)
	if err := Run(context.Background(), []string{"plan-propose", "--project", projectID}, env); err != nil {
		t.Fatalf("plan-propose --project: %v", err)
	}
}

// TestPlanAcceptWithProjectFilesIntoTheExistingProject covers the whole
// --project round trip: propose against a tracked project, accept with no
// --name, and the plan lands in that same project rather than a new one.
func TestPlanAcceptWithProjectFilesIntoTheExistingProject(t *testing.T) {
	env, out, saved := acceptEnv(t)
	id := makeLocalProject(t, env)
	nudges := nudgeCounter(&env)
	proposeToProject(t, env, id, validProposalDoc)
	*nudges = 0
	out.Reset()

	if err := Run(context.Background(), []string{"plan-accept", "--project", id, "--json"}, env); err != nil {
		t.Fatalf("plan-accept --project: %v", err)
	}
	var got planAcceptedJSON
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if got.Milestones != 1 || got.Slices != 2 || got.Project.ID != id {
		t.Errorf("reported %+v", got)
	}
	if *nudges == 0 {
		t.Error("accepting should nudge the board")
	}
	// No new project was created in config — the tracked one is still the
	// only entry.
	if len(saved.Projects) != 1 {
		t.Errorf("projects = %+v, want only the one already tracked", saved.Projects)
	}
	dir, _ := stateDir()
	if _, err := os.Stat(filepath.Join(dir, "proposals", id+".json")); !os.IsNotExist(err) {
		t.Errorf("the proposal file should be gone, stat err = %v", err)
	}

	out.Reset()
	if err := Run(context.Background(), []string{"info", "--project", id, "--json"}, env); err != nil {
		t.Fatalf("info: %v", err)
	}
	for _, want := range []string{"Lay the foundation", "Build on it", "M1: Groundwork"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("info lacks %q:\n%s", want, out.String())
		}
	}
}

// --name is refused with --project: the project already has one.
func TestPlanAcceptWithProjectRefusesAName(t *testing.T) {
	env, _, _ := acceptEnv(t)
	id := makeLocalProject(t, env)
	proposeToProject(t, env, id, validProposalDoc)

	err := Run(context.Background(), []string{"plan-accept", "--project", id, "--name", "Nope"}, env)
	if err == nil || !strings.Contains(err.Error(), "--name") {
		t.Errorf("err = %v, want it to name --name as refused with --project", err)
	}
}

// Both or neither of --workspace/--project is a usage error.
func TestPlanAcceptRequiresExactlyOneOfWorkspaceOrProject(t *testing.T) {
	env, _, _ := acceptEnv(t)
	if err := Run(context.Background(), []string{"plan-accept"}, env); err == nil ||
		!strings.Contains(err.Error(), "exactly one of --workspace or --project") {
		t.Errorf("err = %v, want the refusal naming exactly one of the two", err)
	}
	if err := Run(context.Background(), []string{"plan-accept", "--workspace", "ws-1", "--project", "p1", "--name", "N"}, env); err == nil ||
		!strings.Contains(err.Error(), "exactly one of --workspace or --project") {
		t.Errorf("err = %v, want the refusal naming exactly one of the two", err)
	}
}

// A plan the live project has outgrown since the proposal was written — a
// milestone it named has since been renamed away — is refused rather than
// half-applied, the whole reason --project validates against the project's
// current plan rather than trusting what was true when it was proposed.
func TestPlanAcceptWithProjectRefusesAPlanTheProjectHasOutgrown(t *testing.T) {
	env, _, _ := acceptEnv(t)
	id := makeLocalProject(t, env)
	proposeToProject(t, env, id, validProposalDoc)

	// Rewrite the proposal file underneath, as if a plan that validated
	// against the project when it was proposed no longer does — here, a
	// milestone it names was never created. --project's validation at
	// accept-time has to catch this, exactly as plan-apply would.
	dir, err := stateDir()
	if err != nil {
		t.Fatal(err)
	}
	broken := `{"project":"` + id + `","plan":{"slices":[{"title":"A slice","milestone":"Ghost milestone"}]}}`
	if err := os.WriteFile(filepath.Join(dir, "proposals", id+".json"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run(context.Background(), []string{"plan-accept", "--project", id}, env); err == nil {
		t.Error("expected a refusal for a milestone the project does not have")
	}
}

// An unknown --project is refused the same way every other project-scoped
// command refuses it, and the proposal — there may be none anyway — is
// never reached.
func TestPlanAcceptRefusesAnUnknownProject(t *testing.T) {
	env, _, _ := acceptEnv(t)
	err := Run(context.Background(), []string{"plan-accept", "--project", "nope"}, env)
	if err == nil {
		t.Fatal("expected a refusal for an unknown project")
	}
}

// A proposal that was never written for this project is refused before the
// store is even opened.
func TestPlanAcceptWithProjectRefusesWithNoProposal(t *testing.T) {
	env, _, _ := acceptEnv(t)
	id := makeLocalProject(t, env)
	if err := Run(context.Background(), []string{"plan-accept", "--project", id}, env); err == nil {
		t.Error("expected a refusal for a project with no proposal")
	}
}

// A project the store cannot be opened for — its plan file damaged since it
// was created — fails at the store, not at the proposal, which already read
// fine.
func TestPlanAcceptWithProjectFailsWhereTheStoreCannotBeOpened(t *testing.T) {
	env, _, _ := acceptEnv(t)
	id := makeLocalProject(t, env)
	proposeToProject(t, env, id, validProposalDoc)

	path, err := store.LocalPath(id)
	if err != nil {
		t.Fatalf("LocalPath: %v", err)
	}
	stampNewerSchema(t, path)

	if err := Run(context.Background(), []string{"plan-accept", "--project", id}, env); err == nil {
		t.Error("expected a refusal for a plan written by a newer nat")
	}
}

// stampNewerSchema bumps a plan file's own user_version past what this build
// reads, through a live write via sqlite itself — never a raw byte overwrite,
// which a WAL-backed file open since can simply reconstruct around.
func stampNewerSchema(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`PRAGMA user_version = 99;`); err != nil {
		t.Fatal(err)
	}
}

// applyPlan itself failing (the slices table dropped under a project
// already tracked) surfaces for --project exactly as it does for
// --workspace, and leaves the proposal in place. The proposal names no
// dependency, so validation itself never reads the slices table — the
// failure this exercises is applyPlan's own AddSlice, not the earlier read.
func TestPlanAcceptWithProjectReportsAFailedApply(t *testing.T) {
	env, _, _ := acceptEnv(t)
	id := makeLocalProject(t, env)
	noDepsDoc := `{"milestones": [{"name": "M1: Groundwork"}], "slices": [
		{"title": "Lay the foundation", "milestone": "M1: Groundwork"}
	]}`
	proposeToProject(t, env, id, noDepsDoc)

	path, err := store.LocalPath(id)
	if err != nil {
		t.Fatalf("LocalPath: %v", err)
	}
	dropTable(t, "slices")(path)

	if err := Run(context.Background(), []string{"plan-accept", "--project", id}, env); err == nil {
		t.Fatal("want the failed apply surfaced")
	}
	dir, _ := stateDir()
	if _, err := os.Stat(filepath.Join(dir, "proposals", id+".json")); err != nil {
		t.Errorf("a failed accept must leave the proposal: %v", err)
	}
}

// Markdown output, like --workspace's, names what was accepted and into
// which project.
func TestPlanAcceptWithProjectMarkdown(t *testing.T) {
	env, out, _ := acceptEnv(t)
	id := makeLocalProject(t, env)
	proposeToProject(t, env, id, validProposalDoc)
	out.Reset()

	if err := Run(context.Background(), []string{"plan-accept", "--project", id}, env); err != nil {
		t.Fatalf("plan-accept --project: %v", err)
	}
	if !strings.Contains(out.String(), `Accepted 1 milestone and 2 slices into "Tracked project"`) {
		t.Errorf("output = %q", out.String())
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

// plan-accept's last nudge fires only once the proposal file is gone, on both
// halves: whatever it wakes reads the plan in and the proposal dropped
// together, never the filed plan with the accepted proposal still beside it.
// (The workspace half's project-create nudges earlier, before the plan is in,
// which is a true state of its own that the last nudge then supersedes.)
func TestPlanAcceptNudgesOnlyOnceTheProposalIsGone(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(t *testing.T, env Env) (key string, args []string)
	}{
		{"workspace", func(t *testing.T, env Env) (string, []string) {
			propose(t, env, "ws-1", "importer", validProposalDoc)
			return "ws-1", []string{"plan-accept", "--workspace", "ws-1", "--name", "Importer"}
		}},
		{"project", func(t *testing.T, env Env) (string, []string) {
			id := makeLocalProject(t, env)
			proposeToProject(t, env, id, validProposalDoc)
			return id, []string{"plan-accept", "--project", id}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, _, _ := acceptEnv(t)
			key, args := tc.run(t, env)
			dir, _ := stateDir()
			path := filepath.Join(dir, "proposals", key+".json")
			var nudges int
			var lastSawProposal bool
			env.Nudge = func() {
				nudges++
				_, err := os.Stat(path)
				lastSawProposal = err == nil
			}

			if err := Run(context.Background(), args, env); err != nil {
				t.Fatalf("plan-accept: %v", err)
			}
			if nudges == 0 {
				t.Fatal("accepting should nudge the board")
			}
			if lastSawProposal {
				t.Error("the last nudge fired with the accepted proposal still on disk")
			}
		})
	}
}

// A proposal that cannot be claimed — a read-only proposals directory, so it
// cannot be moved aside — is refused before anything is written, on both
// halves: it would otherwise stay acceptable after its plan was filed, and a
// second Accept would file it twice.
func TestPlanAcceptRefusesWhereTheProposalCannotBeClaimed(t *testing.T) {
	for _, half := range []string{"workspace", "project"} {
		t.Run(half, func(t *testing.T) {
			env, _, saved := acceptEnv(t)
			key, args := "ws-1", []string{"plan-accept", "--workspace", "ws-1", "--name", "N"}
			if half == "project" {
				key = makeLocalProject(t, env)
				args = []string{"plan-accept", "--project", key}
				proposeToProject(t, env, key, validProposalDoc)
			} else {
				propose(t, env, key, "importer", validProposalDoc)
			}
			before := len(saved.Projects)
			dir, _ := stateDir()
			proposals := filepath.Join(dir, "proposals")
			if err := os.Chmod(proposals, 0o555); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.Chmod(proposals, 0o755) }()

			err := Run(context.Background(), args, env)
			if err == nil || !strings.Contains(err.Error(), "claim the proposal") {
				t.Fatalf("err = %v, want the claim refused", err)
			}
			if len(saved.Projects) != before {
				t.Errorf("a refused accept made a project: %+v", saved.Projects)
			}
			if _, err := os.Stat(filepath.Join(proposals, key+".json")); err != nil {
				t.Errorf("a refused accept must leave the proposal: %v", err)
			}
		})
	}
}

// A proposal is accepted once: the second accept finds none, and the plan
// holds one copy of what was filed.
func TestPlanAcceptAcceptsAProposalOnce(t *testing.T) {
	env, out, _ := acceptEnv(t)
	id := makeLocalProject(t, env)
	proposeToProject(t, env, id, validProposalDoc)
	if err := Run(context.Background(), []string{"plan-accept", "--project", id}, env); err != nil {
		t.Fatalf("first accept: %v", err)
	}
	if err := Run(context.Background(), []string{"plan-accept", "--project", id}, env); err == nil ||
		!strings.Contains(err.Error(), "no proposal to accept for "+id) {
		t.Fatalf("err = %v, want the second accept to find no proposal", err)
	}
	out.Reset()
	if err := Run(context.Background(), []string{"info", "--project", id, "--json"}, env); err != nil {
		t.Fatalf("info: %v", err)
	}
	if n := strings.Count(out.String(), "Lay the foundation"); n != 1 {
		t.Errorf("the slice is filed %d times, want once:\n%s", n, out.String())
	}
}

// An accepted proposal whose claimed file cannot be removed is still an
// accept that succeeded, and still gone from where anything reads one.
func TestPlanAcceptSucceedsThoughTheClaimedProposalWillNotBeRemoved(t *testing.T) {
	env, out, _ := acceptEnv(t)
	propose(t, env, "ws-1", "importer", validProposalDoc)
	dir, _ := stateDir()
	proposals := filepath.Join(dir, "proposals")
	// Read-only from project creation on: after the claim, before the drop.
	save := env.Save
	env.Save = func(c config.Config) error {
		_ = os.Chmod(proposals, 0o555)
		return save(c)
	}
	defer func() { _ = os.Chmod(proposals, 0o755) }()

	if err := Run(context.Background(), []string{"plan-accept", "--workspace", "ws-1", "--name", "N"}, env); err != nil {
		t.Fatalf("plan-accept: %v", err)
	}
	out.Reset()
	if err := Run(context.Background(), []string{"plan-proposal", "--workspace", "ws-1"}, env); err != nil {
		t.Fatalf("plan-proposal: %v", err)
	}
	if !strings.Contains(out.String(), `"proposal": null`) {
		t.Errorf("the accepted proposal is still readable: %s", out.String())
	}
}

// A failed accept puts its proposal back — unless a revision was proposed
// while it ran, which is newer and stays — and nudges once it has.
func TestPlanAcceptFailingPutsTheProposalBackUnlessARevisionLanded(t *testing.T) {
	for _, revised := range []bool{false, true} {
		t.Run(fmt.Sprintf("revised=%v", revised), func(t *testing.T) {
			env, _, _ := acceptEnv(t)
			propose(t, env, "ws-1", "importer", validProposalDoc)
			dir, _ := stateDir()
			path := filepath.Join(dir, "proposals", "ws-1.json")
			revision := []byte(`{"workspace":"ws-1","name":"revised","plan":{}}`)
			damagePlanOnSave(t, &env, func(plan string) {
				dropTable(t, "slices")(plan)
				if revised {
					if err := os.WriteFile(path, revision, 0o644); err != nil {
						t.Fatal(err)
					}
				}
			})
			var lastSawProposal bool
			env.Nudge = func() {
				_, err := os.Stat(path)
				lastSawProposal = err == nil
			}

			if err := Run(context.Background(), []string{"plan-accept", "--workspace", "ws-1", "--name", "N"}, env); err == nil {
				t.Fatal("want the failed apply surfaced")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("the proposal should be back: %v", err)
			}
			if revised != (string(got) == string(revision)) {
				t.Errorf("proposal on disk = %s", got)
			}
			if !lastSawProposal {
				t.Error("the last nudge fired before the proposal was back")
			}
			if leftovers, _ := filepath.Glob(path + ".accepting-*"); len(leftovers) != 0 {
				t.Errorf("claimed files left behind: %v", leftovers)
			}
		})
	}
}

// A failed accept whose proposal cannot be put back keeps it at its claimed
// path rather than losing it.
func TestPlanAcceptKeepsAProposalItCannotPutBack(t *testing.T) {
	env, _, _ := acceptEnv(t)
	propose(t, env, "ws-1", "importer", validProposalDoc)
	dir, _ := stateDir()
	proposals := filepath.Join(dir, "proposals")
	damagePlanOnSave(t, &env, func(plan string) {
		dropTable(t, "slices")(plan)
		_ = os.Chmod(proposals, 0o555)
	})
	defer func() { _ = os.Chmod(proposals, 0o755) }()

	if err := Run(context.Background(), []string{"plan-accept", "--workspace", "ws-1", "--name", "N"}, env); err == nil {
		t.Fatal("want the failed apply surfaced")
	}
	if kept, _ := filepath.Glob(filepath.Join(proposals, "ws-1.json.accepting-*")); len(kept) != 1 {
		t.Errorf("claimed files = %v, want the one proposal kept", kept)
	}
}

// A proposal that will not parse is refused, and left where it was.
func TestPlanAcceptRefusesAProposalThatWillNotParseAndLeavesIt(t *testing.T) {
	env, _, _ := acceptEnv(t)
	propose(t, env, "ws-1", "importer", validProposalDoc)
	dir, _ := stateDir()
	path := filepath.Join(dir, "proposals", "ws-1.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), []string{"plan-accept", "--workspace", "ws-1", "--name", "N"}, env)
	if err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("err = %v, want the parse refusal", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the proposal should be left where it was: %v", err)
	}
}
