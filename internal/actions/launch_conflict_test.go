package actions

import (
	"context"
	"reflect"
	"testing"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/notion"
)

// conflictLaunch relaunches slice s5 onto its existing worktree on
// slice/info-view, its brief the given blocks, against a repository whose
// merge test answers merge — and reports the context the prompt was written
// with, what the repository was asked to test, and what it was asked to rebase.
func conflictLaunch(t *testing.T, s domain.Slice, merge git.MergeState, body ...notion.Block) (c agent.PromptContext, tested, rebased []string) {
	t.Helper()
	dir := repoDir(t)
	w := &fakeWorktrees{existing: map[string]string{"slice/info-view": dir + "-worktrees/slice/info-view"}}
	r := &fakeRepo{base: "origin/main", merge: merge}
	client := &fakeClient{blocks: func(id string) ([]notion.Block, error) {
		if id == "s5" {
			return body, nil
		}
		return nil, nil
	}}
	res, err := Launch(context.Background(), &fakeLauncher{}, w, r, client.store(), nil, "u1",
		agent.PromptContext{Slice: s, WorkingDir: dir}, config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	return res.Context, r.tested, r.rebased
}

func handedBackBody(t *testing.T) []notion.Block {
	return []notion.Block{block(t, "heading_3", "Handed back"), block(t, "paragraph", "Wrote it.")}
}

// A hand-back with no pull request, its branch tested conflicting at launch,
// is told the base it conflicts with — whether its branch is still recorded
// (relaunched from review) or was cleared when it was sent back.
func TestLaunchTellsAConflictedHandBackItsBase(t *testing.T) {
	for name, branch := range map[string]string{"in review": "slice/info-view", "sent back": ""} {
		t.Run(name, func(t *testing.T) {
			s := domain.Slice{ID: "s5", Name: "Info view", Branch: branch, Status: domain.SliceClaimed}
			c, tested, _ := conflictLaunch(t, s, git.MergeConflicted, handedBackBody(t)...)
			if c.ConflictBase != "origin/main" {
				t.Errorf("ConflictBase = %q, want origin/main", c.ConflictBase)
			}
			if len(tested) != 1 || tested[0] != c.WorkingDir+"|slice/info-view" {
				t.Errorf("tested %v, want the agent's branch in its worktree", tested)
			}
		})
	}
}

// A branch that merges cleanly, or one the test could not be made of, is
// told nothing and rebased onto nothing: an unknown reading is never a
// conflict.
func TestLaunchTellsACleanOrUntestedHandBackNothing(t *testing.T) {
	for name, merge := range map[string]git.MergeState{"clean": git.MergeClean, "unknown": git.MergeUnknown} {
		t.Run(name, func(t *testing.T) {
			s := domain.Slice{ID: "s5", Name: "Info view", Branch: "slice/info-view", Status: domain.SliceClaimed}
			c, tested, rebased := conflictLaunch(t, s, merge, handedBackBody(t)...)
			if c.ConflictBase != "" || c.ConflictRebase != agent.RebaseLeftToAgent || rebased != nil {
				t.Errorf("ConflictBase %q, rebase %v, rebased %v — want none", c.ConflictBase, c.ConflictRebase, rebased)
			}
			if len(tested) != 1 {
				t.Errorf("tested %v, want the one test", tested)
			}
		})
	}
}

// A slice with a pull request has GitHub's own reading of its conflicts, and
// one never handed back has no branch anyone reviewed: neither is tested.
func TestLaunchTestsNoBranchWithAPullRequestOrNoHandBack(t *testing.T) {
	approved := domain.Slice{ID: "s5", Name: "Info view", Branch: "slice/info-view", Status: domain.SliceClaimed,
		PRURL: "https://github.test/o/r/pull/1"}
	if c, tested, _ := conflictLaunch(t, approved, git.MergeConflicted, handedBackBody(t)...); c.ConflictBase != "" || tested != nil {
		t.Errorf("approved: ConflictBase %q, tested %v — want neither", c.ConflictBase, tested)
	}
	working := domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceClaimed}
	c, tested, _ := conflictLaunch(t, working, git.MergeConflicted, block(t, "heading_3", "Blocked"), block(t, "paragraph", "Stuck."))
	if c.ConflictBase != "" || !reflect.DeepEqual(tested, []string(nil)) {
		t.Errorf("never handed back: ConflictBase %q, tested %v — want neither", c.ConflictBase, tested)
	}
}

// A review sent back before any pull request — Branch cleared, no PR, a
// hand-back on its log — is a resume: the launch gathers its git snapshot
// and says it was handed back.
func TestLaunchGathersTheGitSnapshotForATakenBackReview(t *testing.T) {
	dir := repoDir(t)
	w := &fakeWorktrees{existing: map[string]string{"slice/info-view": dir + "-worktrees/slice/info-view"}}
	r := &fakeRepo{base: "origin/main", log: "abc1234 did the thing", stat: "a.go | 2 ++", merge: git.MergeClean}
	client := &fakeClient{blocks: func(id string) ([]notion.Block, error) {
		if id == "s5" {
			return handedBackBody(t), nil
		}
		return nil, nil
	}}
	res, err := Launch(context.Background(), &fakeLauncher{}, w, r, client.store(), nil, "u1",
		agent.PromptContext{Slice: domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceClaimed}, WorkingDir: dir},
		config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	c := res.Context
	if !c.HandedBack || c.GitLog != "abc1234 did the thing" || c.GitDiffStat != "a.go | 2 ++" {
		t.Errorf("context = %+v, want it handed back with the git snapshot gathered", c)
	}
}
