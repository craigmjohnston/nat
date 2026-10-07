package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A proposal up is taken down — the file gone, the board nudged — and a
// second withdraw finds nothing and is no error.
func TestPlanWithdrawRemovesTheProposal(t *testing.T) {
	env, out, _ := acceptEnv(t)
	propose(t, env, "ws-1", "importer", validProposalDoc)
	path, err := proposalPath("ws-1")
	if err != nil {
		t.Fatal(err)
	}
	nudges := nudgeCounter(&env)
	out.Reset()

	if err := Run(context.Background(), []string{"plan-withdraw", "--workspace", "ws-1", "--json"}, env); err != nil {
		t.Fatalf("plan-withdraw: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("proposal file still there: %v", err)
	}
	if *nudges != 1 {
		t.Errorf("nudges = %d, want 1", *nudges)
	}
	var got withdrawAnswer
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil || !got.Withdrawn {
		t.Errorf("answer = %q (%v), want withdrawn", out.String(), err)
	}

	out.Reset()
	if err := Run(context.Background(), []string{"plan-withdraw", "--workspace", "ws-1"}, env); err != nil {
		t.Fatalf("plan-withdraw with none: %v", err)
	}
	if *nudges != 1 {
		t.Errorf("nudges = %d, want none for nothing withdrawn", *nudges)
	}
	if want := "No proposal to withdraw for ws-1.\n"; out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}
}

// A project's proposal is withdrawn by --project, and said so in text.
func TestPlanWithdrawByProject(t *testing.T) {
	env, out, _ := acceptEnv(t)
	id := makeLocalProject(t, env)
	proposeToProject(t, env, id, validProposalDoc)
	out.Reset()

	if err := Run(context.Background(), []string{"plan-withdraw", "--project", id}, env); err != nil {
		t.Fatalf("plan-withdraw --project: %v", err)
	}
	if want := "Withdrew the proposal for " + id + ".\n"; out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}
	out.Reset()
	if err := Run(context.Background(), []string{"plan-proposal", "--project", id}, env); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"proposal": null`) && !strings.Contains(out.String(), `"proposal":null`) {
		t.Errorf("plan-proposal = %q, want null", out.String())
	}
}

// A proposal an accept has claimed is not at the proposal path: withdraw
// leaves it alone.
func TestPlanWithdrawLeavesAClaimedProposal(t *testing.T) {
	env, out, _ := acceptEnv(t)
	propose(t, env, "ws-1", "importer", validProposalDoc)
	claimed, err := claimProposal("ws-1")
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()

	if err := Run(context.Background(), []string{"plan-withdraw", "--workspace", "ws-1", "--json"}, env); err != nil {
		t.Fatalf("plan-withdraw: %v", err)
	}
	if _, err := os.Stat(claimed.claimed); err != nil {
		t.Errorf("claimed proposal touched: %v", err)
	}
	if !strings.Contains(out.String(), `"withdrawn": false`) {
		t.Errorf("answer = %q, want not withdrawn", out.String())
	}
}

func TestPlanWithdrawRefusals(t *testing.T) {
	env, _, _ := acceptEnv(t)
	cases := map[string][]string{
		"neither":      {"plan-withdraw"},
		"both":         {"plan-withdraw", "--workspace", "ws-1", "--project", "p1"},
		"extra args":   {"plan-withdraw", "--workspace", "ws-1", "stray"},
		"unknown flag": {"plan-withdraw", "--workspace", "ws-1", "--nope"},
	}
	for name, args := range cases {
		if err := Run(context.Background(), args, env); err == nil {
			t.Errorf("%s: want a refusal", name)
		}
	}
	if err := Run(context.Background(), []string{"plan-withdraw", "--workspace", "ws-1", "--project", "p1"}, env); err == nil ||
		!strings.Contains(err.Error(), "exactly one of --workspace or --project") {
		t.Errorf("err = %v, want the refusal naming exactly one of the two", err)
	}

	// A proposal path that is a directory cannot be removed as a file.
	dir, _ := stateDir()
	if err := os.MkdirAll(filepath.Join(dir, "proposals", "stuck.json", "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if path, _ := proposalPath("stuck"); path != filepath.Join(dir, "proposals", "stuck.json") {
		t.Fatalf("proposal path = %q, the test's assumption no longer holds", path)
	}
	if err := Run(context.Background(), []string{"plan-withdraw", "--workspace", "stuck"}, env); err == nil ||
		!strings.Contains(err.Error(), "withdraw the proposal") {
		t.Errorf("err = %v, want the removal's failure", err)
	}

	prev := stateDir
	stateDir = func() (string, error) { return "", errors.New("no state dir") }
	defer func() { stateDir = prev }()
	if err := Run(context.Background(), []string{"plan-withdraw", "--workspace", "ws-1"}, env); err == nil ||
		!strings.Contains(err.Error(), "resolve the proposal file") {
		t.Errorf("err = %v, want refused at the proposal file", err)
	}
}
