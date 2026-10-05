package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/store"
)

// PRReader is what pr-status needs of the GitHub CLI: every pull request a
// repository currently has open. It names exactly the one gh call this
// command makes, the way [internal/tui.PRReader] does for the board's own
// background reading.
type PRReader interface {
	OpenPRs(dir string) (map[string]gh.PRStatus, error)
}

// prStatus prints the board's own PR-readiness reading, headlessly: every
// slice whose pull request is still worth watching, and how close it is to
// landing. It mirrors internal/tui/prstate.go's refreshPRStates — one gh
// listing per repository the plan spans, rather than one view per slice — so
// the reading costs the number of repositories rather than the number of
// pull requests the plan has ever produced.
//
// It is also where a merge made on GitHub itself reaches Notion for anything
// that polls through this command: an in-progress slice whose pull request
// the listing no longer names is asked about directly, and one that merged is
// marked Done — see [actions.SettleMerged]. It is likewise where a slice Done
// under the old rule — at approve, rather than at the merge — is caught and
// written back to In progress once its pull request reads open — see
// [actions.ReopenUnmerged], the mirror of SettleMerged and of
// internal/tui/prstate.go's own un-done rule. Between the two, these are the
// only writes this read can make, and only ever the writes the facts already
// earned.
//
// And it is where a worktree nothing witnessed the end of goes: a merge
// settled here takes its slice's worktree with it, and [landed] names every
// other Done slice whose worktree there is nothing left to do in, for
// [actions.SweepLanded] to remove.
func prStatus(ctx context.Context, args []string, env Env) error {
	asJSON, projectRef, err := parseJSONFlag("pr-status", args)
	if err != nil {
		return err
	}

	_, projectID, project, err := env.projectFor(projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}

	plan, err := st.Plan(ctx, storeProject(projectID, project))
	if err != nil {
		return fmt.Errorf("load slices: %w", err)
	}
	slices := plan.Project.Slices

	worktrees := env.NewWorktrees()
	readings, listed, marked := prReadings(ctx, st, env.NewGH(), worktrees, slices, project)
	tmux := env.NewTmux()
	if noticeFailing(ctx, st, tmux, projectID, slices, readings) {
		marked = true
	}
	actions.SweepLanded(worktrees, tmux.LiveSlices, project, landed(slices, project, readings, listed))
	if marked {
		env.nudged()
	}
	branches := branchReadings(env.NewGit(), slices, project)

	if asJSON {
		doc := prStatusJSON(readings)
		doc.Branches = branchesJSON(branches)
		return writeJSON(env.Out, doc)
	}
	_, err = io.WriteString(env.Out, prStatusMarkdown(readings)+branchesMarkdown(branches))
	return err
}

// branchReading is one handed-back branch with no pull request yet, tested
// against its repository's default branch: the one state of a review nothing
// on GitHub can say anything about, since there is no pull request for
// GitHub to read the mergeability of.
type branchReading struct {
	SliceID     string
	SliceName   string
	Branch      string
	Base        string
	Conflicting bool
}

// awaitingReview reports whether a slice is a hand-back still to be approved:
// in progress, its branch recorded, no pull request opened from it. A resumed
// or sent-back slice has its branch cleared and is work in progress again, so
// it is not one; nor is a slice with a pull request, whose conflicts GitHub
// reads.
func awaitingReview(s domain.Slice) bool {
	return s.Status == domain.SliceClaimed && s.Branch != "" && s.PRURL == ""
}

// branchReadings tests every hand-back awaiting review for a conflict with
// its base, by [git.CLI.ConflictsWithBase], in the plan's own order. Each
// test fetches, so this costs a fetch per such slice — few at any one time,
// since a review either opens its pull request or goes back to the agent. A
// reading that comes back unknown is left out entirely: a branch nobody could
// test is not a broken one, and nothing downstream should draw it as either.
// A slice with no repository to test in (a source project's task that has not
// recorded one) is never asked about.
func branchReadings(g GitCLI, slices []domain.Slice, project config.ProjectConfig) []branchReading {
	var out []branchReading
	bases := map[string]string{}
	for _, s := range slices {
		if !awaitingReview(s) {
			continue
		}
		dir := actions.ExpandHome(actions.WorkdirFor(s, project))
		if dir == "" {
			continue
		}
		state := g.ConflictsWithBase(dir, s.Branch)
		if state == git.MergeUnknown {
			continue
		}
		base, seen := bases[dir]
		if !seen {
			base = g.Base(dir)
			bases[dir] = base
		}
		out = append(out, branchReading{SliceID: s.ID, SliceName: s.Name, Branch: s.Branch, Base: base,
			Conflicting: state == git.MergeConflicted})
	}
	return out
}

// branchJSON is one branch reading's entry: a handed-back branch with no pull
// request, and whether it conflicts with Base, the default branch it was
// tested against.
type branchJSON struct {
	SliceID     string `json:"slice_id"`
	Name        string `json:"name"`
	Branch      string `json:"branch"`
	Base        string `json:"base"`
	Conflicting bool   `json:"conflicting"`
}

// branchesJSON maps the branch readings onto their structured form — never
// nil, so the key always reads as a list.
func branchesJSON(readings []branchReading) []branchJSON {
	out := make([]branchJSON, 0, len(readings))
	for _, r := range readings {
		out = append(out, branchJSON{SliceID: r.SliceID, Name: r.SliceName, Branch: r.Branch, Base: r.Base,
			Conflicting: r.Conflicting})
	}
	return out
}

// branchesMarkdown names every handed-back branch read conflicting, under a
// heading of its own; nothing at all where none is.
func branchesMarkdown(readings []branchReading) string {
	out := ""
	for _, r := range readings {
		if r.Conflicting {
			out += fmt.Sprintf("- %s — %s — %s\n", r.SliceName, conflictLine(r.Base), r.Branch)
		}
	}
	if out == "" {
		return ""
	}
	return "\n# Branches awaiting review\n\n" + out
}

// prReading is one slice's pull request as pr-status reports it. Checks is
// how its checks stand and whether it conflicts, set only for an open pull
// request the listing read.
type prReading struct {
	SliceID   string
	SliceName string
	PR        string
	Readiness domain.PRReadiness
	Checks    *gh.PRStatus
}

// liveReader is what pr-status needs of tmux: which slices have a session.
type liveReader interface {
	actions.PromptSender
	LiveSlices() (map[string]string, error)
}

// noticeFailing hands every red pull request the readings found to
// [actions.NoticeFailingChecks] — the same function the board runs after its
// own reading — and reports whether it wrote anything. A tmux that cannot say
// which sessions are live concludes nothing: no agent is told and nothing is
// recorded as though none were there, and the next reading asks again.
func noticeFailing(ctx context.Context, st store.Store, tmux liveReader, projectID string,
	slices []domain.Slice, readings []prReading) bool {
	byID := make(map[string]domain.Slice, len(slices))
	for _, s := range slices {
		byID[s.ID] = s
	}
	var failing []actions.FailingChecks
	for _, r := range readings {
		if r.Readiness == domain.PRChecksFailing {
			failing = append(failing, actions.FailingChecks{Slice: byID[r.SliceID], Failing: r.Checks.Failing})
		}
	}
	if len(failing) == 0 {
		return false
	}
	live, err := tmux.LiveSlices()
	if err != nil {
		logging.Action("left failing pull requests unnoticed: live sessions unread", "error", err)
		return false
	}
	return actions.NoticeFailingChecks(ctx, st, tmux, live, projectID, failing)
}

// worthReadingPR reports whether a slice has a pull request that anything
// might still be waiting on: the same rule internal/tui/prstate.go's
// worthReading applies. A slice with none has nothing to ask about, and one
// neither in progress nor Done has not got as far as producing one.
func worthReadingPR(s domain.Slice) bool {
	if s.PRURL == "" {
		return false
	}
	return s.Status == domain.SliceClaimed || s.Status == domain.SliceDone
}

// readinessOf turns what gh said about an open pull request into the reading
// pr-status reports, the same mapping prstate.go's readinessOf makes: a
// failed check first, whatever the review says, then approved and mergeable
// is the review over, and anything else is a review still to come.
func readinessOf(status gh.PRStatus) domain.PRReadiness {
	if status.Checks == gh.ChecksFailing {
		return domain.PRChecksFailing
	}
	if status.Approved && status.Mergeable {
		return domain.PRReadyToMerge
	}
	return domain.PRAwaitingReview
}

// landed is every slice whose work has ended by what this reading saw, and
// whose worktree a sweep may take: Done, with no pull request or one the
// listing read and did not find open. A Done slice whose pull request reads
// open is one [actions.ReopenUnmerged] has just written back to In progress,
// and one whose repository's listing could not be read concludes nothing, so
// neither is named. A pull request closed unmerged never made its slice Done,
// so it is no case of its own here.
func landed(slices []domain.Slice, project config.ProjectConfig, readings []prReading, listed map[string]bool) []domain.Slice {
	open := map[string]bool{}
	for _, r := range readings {
		if r.Checks != nil {
			open[r.SliceID] = true
		}
	}
	var out []domain.Slice
	for _, s := range slices {
		if s.Status != domain.SliceDone || open[s.ID] {
			continue
		}
		if s.PRURL != "" && !listed[actions.WorkdirFor(s, project)] {
			continue
		}
		out = append(out, s)
	}
	return out
}

// prReadings reads what GitHub says about the pull request of every slice
// worth asking about, one listing per repository, and reports a reading per
// slice in the plan's own order — plus the repositories whose listing was
// read, and whether any slice was marked Done on the way, so the caller knows
// a nudge is owed.
//
// A slice whose pull request is no longer open, or whose repository's
// listing could not be read at all, comes back with the zero
// [domain.PRReadiness] — unread — which is exactly how the board reads either
// case: nothing distinguishes them, because a pull request the reading never
// reached is worth exactly as much attention as one that has already landed.
// A repository whose listing fails is logged and left out, never guessed at.
//
// The exception absence earns is an in-progress slice: its pull request being
// gone is either the merge nat was not running to witness or a close that
// sends the work round again, and the pull request's own reading tells them
// apart — a merged one marks the slice Done and takes its worktree away
// ([actions.RemoveSliceWorktree]), a failed reading is logged and changes
// nothing, and the next run asks again.
//
// The other exception is a Done slice whose pull request is still open: the
// un-done rule, [actions.ReopenUnmerged], written back to In progress so
// Done goes on meaning what the merge made true everywhere else the app reads
// a slice's status from.
func prReadings(ctx context.Context, st store.Store, ghClient GH, worktrees actions.Worktrees,
	slices []domain.Slice, project config.ProjectConfig) ([]prReading, map[string]bool, bool) {
	var dirs []string
	reads := map[string][]domain.Slice{}
	for _, s := range slices {
		if !worthReadingPR(s) {
			continue
		}
		dir := actions.WorkdirFor(s, project)
		if _, seen := reads[dir]; !seen {
			dirs = append(dirs, dir)
		}
		reads[dir] = append(reads[dir], s)
	}

	marked := false
	listed := map[string]bool{}
	state := map[string]gh.PRStatus{}
	for _, dir := range dirs {
		open, err := ghClient.OpenPRs(dir)
		if err != nil {
			logging.Action("left a repository's pull requests unread", "dir", dir, "error", err)
			continue
		}
		listed[dir] = true
		for _, s := range reads[dir] {
			if status, still := open[gh.NormaliseURL(s.PRURL)]; still {
				state[s.ID] = status
				if s.Status == domain.SliceDone {
					if err := actions.ReopenUnmerged(ctx, st, s); err != nil {
						logging.Action("left a Done slice with an open pull request unreopened", "slice", s.ID, "error", err)
						continue
					}
					marked = true
				}
				continue
			}
			if s.Status != domain.SliceClaimed {
				continue
			}
			done, err := actions.SettleMerged(ctx, st, ghClient, s, dir)
			if err != nil {
				logging.Action("left an absent pull request unsettled", "slice", s.ID, "error", err)
				continue
			}
			if done {
				actions.RemoveSliceWorktree(worktrees, s, project)
				marked = true
			}
		}
	}

	var out []prReading
	for _, s := range slices {
		if !worthReadingPR(s) {
			continue
		}
		r := prReading{SliceID: s.ID, SliceName: s.Name, PR: s.PRURL}
		if status, read := state[s.ID]; read {
			r.Readiness, r.Checks = readinessOf(status), &status
		}
		out = append(out, r)
	}
	return out, listed, marked
}

// prStatusDoc is the structured form of the reading: one entry per slice worth
// watching, in the plan's own order — and, under Branches, every hand-back
// awaiting review whose merge into its base could be tested
// ([branchReadings]).
type prStatusDoc struct {
	Slices   []prStatusSliceJSON `json:"slices"`
	Branches []branchJSON        `json:"branches"`
}

// prStatusSliceJSON is one slice's entry. Conflicting is GitHub positively
// saying the branch conflicts with Base — false for a mergeable branch, one
// whose mergeability is still unknown, and every slice the listing did not
// read, since a read that never happened concludes nothing. Base is the
// branch the pull request merges into, where the listing read it.
type prStatusSliceJSON struct {
	SliceID     string        `json:"slice_id"`
	Name        string        `json:"name"`
	PR          string        `json:"pr"`
	Readiness   string        `json:"readiness"`
	Conflicting bool          `json:"conflicting"`
	Base        string        `json:"base,omitempty"`
	Checks      *prChecksJSON `json:"checks,omitempty"`
}

// prChecksJSON is how an open pull request's checks stand: the verdict in
// [gh.ChecksVerdict]'s words, and every failed check by name and run URL.
type prChecksJSON struct {
	Verdict string        `json:"verdict"`
	Failing []prCheckJSON `json:"failing"`
}

type prCheckJSON struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// prStatusJSON maps the readings onto the structured form, in
// [domain.PRReadiness]'s own words, so a consumer reads the same vocabulary
// the board's own state does.
func prStatusJSON(readings []prReading) prStatusDoc {
	doc := prStatusDoc{Slices: make([]prStatusSliceJSON, 0, len(readings))}
	for _, r := range readings {
		entry := prStatusSliceJSON{SliceID: r.SliceID, Name: r.SliceName, PR: r.PR, Readiness: r.Readiness.String()}
		if r.Checks != nil {
			checks := &prChecksJSON{Verdict: r.Checks.Checks.String(), Failing: []prCheckJSON{}}
			for _, c := range r.Checks.Failing {
				checks.Failing = append(checks.Failing, prCheckJSON{Name: c.Name, URL: c.URL})
			}
			entry.Checks = checks
			entry.Conflicting, entry.Base = r.Checks.Conflicting, r.Checks.Base
		}
		doc.Slices = append(doc.Slices, entry)
	}
	return doc
}

// prStatusMarkdown renders the readings as a list, one line per slice.
func prStatusMarkdown(readings []prReading) string {
	out := "# Pull requests\n\n"
	if len(readings) == 0 {
		return out + "_none_\n"
	}
	for _, r := range readings {
		out += fmt.Sprintf("- %s — %s — %s\n", r.SliceName, r.Readiness, r.PR)
		if r.Checks != nil {
			for _, c := range r.Checks.Failing {
				out += fmt.Sprintf("  - failing: %s %s\n", c.Name, c.URL)
			}
			if r.Checks.Conflicting {
				out += "  - " + conflictLine(r.Checks.Base) + "\n"
			}
		}
	}
	return out
}

// conflictLine says a pull request conflicts, naming its base where the
// listing read one.
func conflictLine(base string) string {
	if base == "" {
		return "conflicting"
	}
	return "conflicting with " + base
}
