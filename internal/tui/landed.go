package tui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
)

// landedWorktree is one worktree there is no longer any work to do in: the
// repository it was cut from, the branch it was cut for, and the slice it
// belonged to, which is what the removal is remembered by.
type landedWorktree struct{ id, dir, branch string }

// worktreesRemovedMsg names the slices whose worktree is now gone — removed, or
// found not to be there at all. It is the session's memory of what is done
// with, so the sweep on the next plan load asks git about nothing it has
// already settled; a removal git refused is deliberately absent from it, since
// that is exactly what the sweep is a retry for.
type worktreesRemovedMsg struct{ ids []string }

// removeLanded takes away the worktree of every named slice whose pull request
// has landed.
//
// A slice's worktree outlives its approve: approving opens the pull request and
// marks the slice Done, and a review that asks for one more commit needs the
// checkout that commit is written in. What ends the work is the merge, so that
// is what the removal rides — the reading that finds a Done slice's pull
// request no longer open, which is the same edge that drops the slice from the
// Active panel. Nothing here reads git for that: a slice is only ever named
// once, at the transition, or again by [App.settledSlices] on a plan load,
// which is the retry for a removal that failed at the transition and the
// clean-up for a merge nat was not running to witness.
//
// Every removal is off the main loop, since each is a git or two, and none of
// them reports anything to the board: what git refuses is a line in the log and
// nothing else — see [removeWorktree].
func (a *App) removeLanded(ids []string) tea.Cmd {
	jobs := a.landedWorktrees(ids)
	if len(jobs) == 0 {
		return nil
	}
	w := newWorktrees()
	return func() tea.Msg {
		msg := worktreesRemovedMsg{}
		for _, j := range jobs {
			if removeWorktree(w, j.dir, j.branch) {
				msg.ids = append(msg.ids, j.id)
			}
		}
		return msg
	}
}

// landedWorktrees is the worktrees of the named slices that there is anything
// to remove for: a slice is one whose work has landed only if it is Done, and
// one this session has already settled is not asked about again.
//
// The repository is the slice's own where it names one and the project's
// default otherwise, exactly as the launch that cut the worktree read it.
func (a *App) landedWorktrees(ids []string) []landedWorktree {
	if len(ids) == 0 || a.project == nil {
		return nil
	}
	project, ok := a.activeProject()
	if !ok {
		return nil
	}
	landed := map[string]bool{}
	for _, id := range ids {
		landed[id] = true
	}
	var jobs []landedWorktree
	for _, s := range a.project.Slices {
		if !landed[s.ID] || a.worktreeGone[s.ID] || s.Status != domain.SliceDone {
			continue
		}
		jobs = append(jobs, landedWorktree{
			id:     s.ID,
			dir:    expandHome(strings.TrimSpace(workdirFor(s, project))),
			branch: landedBranch(s),
		})
	}
	return jobs
}

// landedBranch is the branch a slice's worktree is on: the one recorded at
// hand-back where there is one, since what the agent actually pushed is what
// its worktree is checked out on, and otherwise the branch a launch derives, so
// a slice finished before there was a Branch column to record one still has its
// worktree found.
func landedBranch(s domain.Slice) string {
	if b := strings.TrimSpace(s.Branch); b != "" {
		return b
	}
	return sliceBranch(s)
}

// settledSlices is every slice this session has watched a pull request settle
// for, sorted so a sweep runs the same way twice. It is what a plan load sweeps
// over: the ones whose worktree is already gone are dropped by
// [App.landedWorktrees], so what is left is a removal that has yet to succeed.
func (a *App) settledSlices() []string {
	ids := make([]string, 0, len(a.prSettled))
	for id := range a.prSettled {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// worktreesRemoved records what is done with, so the next sweep asks git
// nothing about it.
func (a *App) worktreesRemoved(msg worktreesRemovedMsg) {
	if len(msg.ids) > 0 && a.worktreeGone == nil {
		a.worktreeGone = map[string]bool{}
	}
	for _, id := range msg.ids {
		a.worktreeGone[id] = true
	}
}

// removeWorktree takes a slice's worktree off its repository, and reports
// whether there is anything left to remove there — which a removal git refused
// is the only case of.
//
// A branch git names no worktree for is not a failure at all: it is a slice
// whose worktree has already gone, which is every slice on every sweep after
// the first, so it passes silently rather than filling the log with a line a
// load. What git does refuse — a worktree holding uncommitted changes, a
// repository it will not answer about — is dropped after one line: the pull
// request is merged and the slice is Done whatever became of the checkout, and
// git's own rules mean a refusal never costs any work.
func removeWorktree(w Worktrees, dir, branch string) bool {
	if _, err := w.Path(dir, branch); err != nil {
		return true
	}
	if err := w.Remove(dir, branch); err != nil {
		// git's own failure is already in the log; this is the decision
		// taken about it.
		logging.Action("left the slice's worktree in place", "dir", dir, "branch", branch, "error", err)
		return false
	}
	return true
}
