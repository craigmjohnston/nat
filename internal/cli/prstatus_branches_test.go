package cli

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/notion"
)

// conflictGit answers [GitCLI.ConflictsWithBase] by branch, and counts what
// it was asked: which branches were tested, and how often the base was read.
// The rest of GitCLI is fakeSessionRepo's, which nothing here calls.
type conflictGit struct {
	*fakeSessionRepo
	states    map[string]git.MergeState
	asked     []string
	baseReads int
}

func (g *conflictGit) ConflictsWithBase(dir, branch string) git.MergeState {
	g.asked = append(g.asked, dir+" "+branch)
	return g.states[branch]
}

func (g *conflictGit) Base(string) string {
	g.baseReads++
	return "origin/main"
}

func branchesPlan() *fakeAPI {
	return &fakeAPI{
		dataSources: map[string]notion.DataSource{"slices-ds": selectMilestoneSlicesDS("M1")},
		pages: map[string][]notion.Page{
			"slices-ds": {
				slicePageWithBranch("s1", "Conflicted", notion.SliceInProgress, "M1", "slice/conflicted"),
				slicePageWithBranch("s2", "Clean", notion.SliceInProgress, "M1", "slice/clean"),
				slicePageWithBranch("s3", "Untestable", notion.SliceInProgress, "M1", "slice/untestable"),
				// Resumed: its branch cleared, work in progress again.
				slicePageForStatus("s4", "Resumed", notion.SliceInProgress, "M1", ""),
				// Approved: GitHub reads its conflicts, not this.
				slicePageWithBranchAndPR("s5", "Approved", notion.SliceInProgress, "M1", "slice/approved",
					"https://github.test/craig/nat/pull/5"),
				// Released: back at Todo with its branch kept.
				slicePageWithBranch("s6", "Released", notion.SliceTodo, "M1", "slice/released"),
			},
		},
	}
}

func branchesEnv(t *testing.T) (Env, *conflictGit, interface{ String() string }) {
	t.Helper()
	env, out := testEnv(testConfig(t), branchesPlan())
	env.NewGH = func() GH { return &fakePRReader{} }
	g := &conflictGit{fakeSessionRepo: &fakeSessionRepo{}, states: map[string]git.MergeState{
		"slice/conflicted": git.MergeConflicted,
		"slice/clean":      git.MergeClean,
		"slice/untestable": git.MergeUnknown,
	}}
	env.NewGit = func() GitCLI { return g }
	return env, g, out
}

// TestPRStatusJSONReadsHandedBackBranches tests every hand-back awaiting
// review — and nothing else — and reports each reading it got, leaving out
// the one git could not test.
func TestPRStatusJSONReadsHandedBackBranches(t *testing.T) {
	env, g, out := branchesEnv(t)

	if err := Run(context.Background(), []string{"pr-status", "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status --json: %v", err)
	}
	wantAsked := []string{"/tmp/nat slice/conflicted", "/tmp/nat slice/clean", "/tmp/nat slice/untestable"}
	if !reflect.DeepEqual(g.asked, wantAsked) {
		t.Errorf("tested %v, want only the hand-backs awaiting review %v", g.asked, wantAsked)
	}
	if g.baseReads != 1 {
		t.Errorf("base read %d times, want once for the one repository", g.baseReads)
	}
	var got prStatusDoc
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	want := []branchJSON{
		{SliceID: "s1", Name: "Conflicted", Branch: "slice/conflicted", Base: "origin/main", Conflicting: true},
		{SliceID: "s2", Name: "Clean", Branch: "slice/clean", Base: "origin/main"},
	}
	if !reflect.DeepEqual(got.Branches, want) {
		t.Errorf("branches = %+v\nwant %+v", got.Branches, want)
	}
}

// TestPRStatusMarkdownNamesAConflictedBranch lists the conflicted hand-back
// under its own heading, and neither the clean nor the untested one.
func TestPRStatusMarkdownNamesAConflictedBranch(t *testing.T) {
	env, _, out := branchesEnv(t)

	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	want := "\n# Branches awaiting review\n\n- Conflicted — conflicting with origin/main — slice/conflicted\n"
	if !strings.HasSuffix(out.String(), want) {
		t.Errorf("output = %q, want it to end %q", out.String(), want)
	}
	for _, absent := range []string{"slice/clean", "slice/untestable"} {
		if strings.Contains(out.String(), absent) {
			t.Errorf("output names %s, which is not conflicted:\n%s", absent, out.String())
		}
	}
}

// TestBranchesMarkdownSaysNothingWithNoConflict adds no heading where no
// branch conflicts.
func TestBranchesMarkdownSaysNothingWithNoConflict(t *testing.T) {
	if got := branchesMarkdown([]branchReading{{SliceName: "Clean", Branch: "b"}}); got != "" {
		t.Errorf("markdown = %q, want nothing", got)
	}
}

// TestBranchReadingsSkipASliceWithNoRepository never asks git about a slice
// with nowhere to test it: a source project's task that has recorded no
// repository.
func TestBranchReadingsSkipASliceWithNoRepository(t *testing.T) {
	g := &conflictGit{fakeSessionRepo: &fakeSessionRepo{}}
	slices := []domain.Slice{{ID: "s1", Status: domain.SliceClaimed, Branch: "slice/a"}}

	if got := branchReadings(g, slices, config.ProjectConfig{}); got != nil {
		t.Errorf("readings = %+v, want none", got)
	}
	if len(g.asked) != 0 {
		t.Errorf("asked git %v, want nothing", g.asked)
	}
}
