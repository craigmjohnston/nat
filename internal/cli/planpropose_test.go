package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// proposeEnv builds an Env with no Notion client at all — plan-propose talks
// to none — and points the proposal directory at a temp dir of the test's
// own, restoring the real one when the test ends.
func proposeEnv(t *testing.T) (Env, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	prev := stateDir
	stateDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { stateDir = prev })

	var out bytes.Buffer
	return Env{Out: &out}, &out
}

func runPropose(t *testing.T, doc string, args ...string) (string, *int, error) {
	t.Helper()
	env, out := proposeEnv(t)
	nudges := nudgeCounter(&env)
	env.In = strings.NewReader(doc)
	err := Run(context.Background(), append([]string{"plan-propose"}, args...), env)
	return out.String(), nudges, err
}

const validProposalDoc = `{
  "milestones": [{"name": "M1: Groundwork"}],
  "slices": [
    {"title": "Lay the foundation", "milestone": "M1: Groundwork", "description": "Start here."},
    {"title": "Build on it", "milestone": "M1: Groundwork", "depends_on": ["Lay the foundation"]}
  ]
}`

func TestPlanProposeWritesTheProposalFileAndNudges(t *testing.T) {
	out, nudges, err := runPropose(t, validProposalDoc, "--workspace", "ws-1", "--name", "importer")
	if err != nil {
		t.Fatalf("plan-propose: %v", err)
	}
	if *nudges != 1 {
		t.Errorf("nudges = %d, want 1", *nudges)
	}
	if out == "" {
		t.Error("no output written")
	}

	doc := readProposal(t, "ws-1")
	if doc.Workspace != "ws-1" {
		t.Errorf("Workspace = %q, want ws-1", doc.Workspace)
	}
	if doc.Name != "importer" {
		t.Errorf("Name = %q, want importer", doc.Name)
	}
	if len(doc.Plan.Milestones) != 1 || len(doc.Plan.Slices) != 2 {
		t.Errorf("Plan = %+v, want 1 milestone and 2 slices round-tripped", doc.Plan)
	}
}

// Running plan-propose again for the same workspace replaces the proposal —
// that is how a revision lands.
func TestPlanProposeReplacesAnExistingProposal(t *testing.T) {
	env, _ := proposeEnv(t)
	nudges := nudgeCounter(&env)
	env.In = strings.NewReader(validProposalDoc)
	if err := Run(context.Background(), []string{"plan-propose", "--workspace", "ws-1", "--name", "first"}, env); err != nil {
		t.Fatalf("first plan-propose: %v", err)
	}

	revised := `{"milestones": [{"name": "M1: Groundwork"}], "slices": [
		{"title": "Lay the foundation", "milestone": "M1: Groundwork"}
	]}`
	env.In = strings.NewReader(revised)
	if err := Run(context.Background(), []string{"plan-propose", "--workspace", "ws-1", "--name", "second"}, env); err != nil {
		t.Fatalf("second plan-propose: %v", err)
	}
	if *nudges != 2 {
		t.Errorf("nudges = %d, want 2", *nudges)
	}

	doc := readProposal(t, "ws-1")
	if doc.Name != "second" {
		t.Errorf("Name = %q, want the revision to have replaced the first proposal", doc.Name)
	}
	if len(doc.Plan.Slices) != 1 {
		t.Errorf("Plan.Slices = %d, want the revision's own single slice", len(doc.Plan.Slices))
	}
}

func TestPlanProposeRefusesAMissingWorkspace(t *testing.T) {
	_, nudges, err := runPropose(t, validProposalDoc, "--name", "importer")
	if err == nil {
		t.Fatal("expected a refusal for a missing --workspace")
	}
	if !strings.Contains(err.Error(), "--workspace") {
		t.Errorf("error = %v, want it to name --workspace", err)
	}
	if *nudges != 0 {
		t.Error("a refused run should nudge nothing")
	}
	if _, _, err := runPropose(t, validProposalDoc, "--workspace", "  ", "--name", "importer"); err == nil {
		t.Error("expected a refusal for a blank --workspace")
	}
}

func TestPlanProposeRefusesAMissingName(t *testing.T) {
	_, _, err := runPropose(t, validProposalDoc, "--workspace", "ws-1")
	if err == nil {
		t.Fatal("expected a refusal for a missing --name")
	}
	if !strings.Contains(err.Error(), "--name") {
		t.Errorf("error = %v, want it to name --name", err)
	}
}

func TestPlanProposeRefusesATopLevelDependenciesList(t *testing.T) {
	doc := `{
		"milestones": [{"name": "M1: Groundwork"}],
		"slices": [{"title": "A slice", "milestone": "M1: Groundwork"}],
		"dependencies": [{"slice": "A slice", "on": ["Something"]}]
	}`
	_, nudges, err := runPropose(t, doc, "--workspace", "ws-1", "--name", "importer")
	if err == nil {
		t.Fatal("expected a refusal for a top-level dependencies list")
	}
	if !strings.Contains(err.Error(), "dependencies") {
		t.Errorf("error = %v, want it to name the dependencies list", err)
	}
	if *nudges != 0 {
		t.Error("a refused run should nudge nothing")
	}
	assertNothingWritten(t, "ws-1")
}

func TestPlanProposeRefusesAnUnknownMilestone(t *testing.T) {
	doc := `{"slices": [{"title": "A slice", "milestone": "No such milestone"}]}`
	_, nudges, err := runPropose(t, doc, "--workspace", "ws-1", "--name", "importer")
	if err == nil {
		t.Fatal("expected a refusal for a milestone the document does not create")
	}
	if *nudges != 0 {
		t.Error("a refused run should nudge nothing")
	}
	assertNothingWritten(t, "ws-1")
}

func TestPlanProposeRefusesACyclicDependsOn(t *testing.T) {
	doc := `{
		"milestones": [{"name": "M1: Groundwork"}],
		"slices": [
			{"title": "A", "milestone": "M1: Groundwork", "depends_on": ["B"]},
			{"title": "B", "milestone": "M1: Groundwork", "depends_on": ["A"]}
		]
	}`
	_, nudges, err := runPropose(t, doc, "--workspace", "ws-1", "--name", "importer")
	if err == nil {
		t.Fatal("expected a refusal for a cyclic depends_on")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %v, want it to name the cycle", err)
	}
	if *nudges != 0 {
		t.Error("a refused run should nudge nothing")
	}
	assertNothingWritten(t, "ws-1")
}

// readProposal reads back the proposal file plan-propose wrote for a
// workspace, using the test's own stateDir override.
func readProposal(t *testing.T, workspace string) proposalDoc {
	t.Helper()
	dir, err := stateDir()
	if err != nil {
		t.Fatalf("stateDir: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "proposals", workspace+".json"))
	if err != nil {
		t.Fatalf("read proposal file: %v", err)
	}
	var doc proposalDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal proposal: %v", err)
	}
	return doc
}

// assertNothingWritten checks that a refused run left no proposal file at
// all — half a proposal is worse than none, since the app would show it as
// though the workshop had settled.
func assertNothingWritten(t *testing.T, workspace string) {
	t.Helper()
	dir, err := stateDir()
	if err != nil {
		t.Fatalf("stateDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "proposals", workspace+".json")); !os.IsNotExist(err) {
		t.Errorf("expected no proposal file, stat err = %v", err)
	}
}

func TestPlanProposeJSONOutput(t *testing.T) {
	out, _, err := runPropose(t, validProposalDoc, "--workspace", "ws-1", "--name", "importer", "--json")
	if err != nil {
		t.Fatalf("plan-propose: %v", err)
	}
	var doc proposalDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if doc.Workspace != "ws-1" || doc.Name != "importer" {
		t.Errorf("doc = %+v", doc)
	}
}

func TestPlanProposeWantsAtMostOnePlanFile(t *testing.T) {
	env, _ := proposeEnv(t)
	env.In = strings.NewReader(validProposalDoc)
	err := Run(context.Background(), []string{"plan-propose", "a.json", "b.json", "--workspace", "ws-1", "--name", "x"}, env)
	if err == nil {
		t.Fatal("expected a refusal for two plan files")
	}
}

func TestPlanProposeRefusesABadFlag(t *testing.T) {
	env, _ := proposeEnv(t)
	env.In = strings.NewReader(validProposalDoc)
	err := Run(context.Background(), []string{"plan-propose", "--nope"}, env)
	if err == nil {
		t.Fatal("expected a refusal for an unknown flag")
	}
}

func TestPlanProposeReadsAPlanFromAFile(t *testing.T) {
	env, out := proposeEnv(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(file, []byte(validProposalDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), []string{"plan-propose", file, "--workspace", "ws-1", "--name", "importer"}, env)
	if err != nil {
		t.Fatalf("plan-propose: %v", err)
	}
	if out.Len() == 0 {
		t.Error("no output written")
	}
}

func TestPlanProposeRefusesAMissingPlanFile(t *testing.T) {
	env, _ := proposeEnv(t)
	err := Run(context.Background(), []string{"plan-propose", "/no/such/plan.json", "--workspace", "ws-1", "--name", "importer"}, env)
	if err == nil {
		t.Fatal("expected a refusal for a plan file that does not exist")
	}
}

func TestPlanProposeRefusesUnresolvableState(t *testing.T) {
	env, _ := proposeEnv(t)
	env.In = strings.NewReader(validProposalDoc)
	prev := stateDir
	stateDir = func() (string, error) { return "", errors.New("no home directory") }
	t.Cleanup(func() { stateDir = prev })
	err := Run(context.Background(), []string{"plan-propose", "--workspace", "ws-1", "--name", "importer"}, env)
	if err == nil {
		t.Fatal("expected a refusal when the state directory cannot be resolved")
	}
}

func TestPlanProposeRefusesAMarshalFailure(t *testing.T) {
	env, _ := proposeEnv(t)
	env.In = strings.NewReader(validProposalDoc)
	prev := marshalIndent
	marshalIndent = func(v any, prefix, indent string) ([]byte, error) { return nil, errors.New("boom") }
	t.Cleanup(func() { marshalIndent = prev })
	err := Run(context.Background(), []string{"plan-propose", "--workspace", "ws-1", "--name", "importer"}, env)
	if err == nil || !strings.Contains(err.Error(), "encode the proposal") {
		t.Fatalf("err = %v, want it to name encoding the proposal", err)
	}
	assertNothingWritten(t, "ws-1")
}

func TestWriteProposalFileMkdirError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "proposals")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err := writeProposalFile(filepath.Join(blocker, "ws-1.json"), []byte("{}"))
	if err == nil || !strings.Contains(err.Error(), "create the proposals dir") {
		t.Fatalf("err = %v, want it to name creating the proposals dir", err)
	}
}

func TestWriteProposalFileWriteError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	err := writeProposalFile(filepath.Join(dir, "ws-1.json"), []byte("{}"))
	if err == nil || !strings.Contains(err.Error(), "write the temp file") {
		t.Fatalf("err = %v, want it to name writing the temp file", err)
	}
}

func TestWriteProposalFileRenameError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ws-1.json")
	// A directory sits where the proposal file itself should land, so the
	// rename that swaps the finished temp file into place has nowhere valid
	// to go.
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	err := writeProposalFile(path, []byte("{}"))
	if err == nil || !strings.Contains(err.Error(), "rename the temp file into place") {
		t.Fatalf("err = %v, want it to name the rename, got %v", err, err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file left behind after a failed rename, stat err = %v", err)
	}
}

func TestPlanProposeRefusesWhenTheProposalCannotBeWritten(t *testing.T) {
	env, _ := proposeEnv(t)
	env.In = strings.NewReader(validProposalDoc)
	dir, err := stateDir()
	if err != nil {
		t.Fatal(err)
	}
	// A file sits where the proposals directory needs to be, so writing the
	// proposal fails at the same MkdirAll writeProposalFile's own unit test
	// covers directly — this exercises planPropose's wrapping of that error.
	if err := os.WriteFile(filepath.Join(dir, "proposals"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err = Run(context.Background(), []string{"plan-propose", "--workspace", "ws-1", "--name", "importer"}, env)
	if err == nil || !strings.Contains(err.Error(), "write the proposal file") {
		t.Fatalf("err = %v, want it to name writing the proposal file", err)
	}
}
