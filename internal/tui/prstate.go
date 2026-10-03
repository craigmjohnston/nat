package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/logging"
)

// PRReader is what the board needs of the GitHub CLI to tell a pull request
// still waiting on a reviewer from one the review is over on, and either from
// one that has already landed. It is an interface for the reason [PRCreator]
// is: the flow can then be driven without gh, without a network and without a
// GitHub account.
type PRReader interface {
	OpenPRs(dir string) (map[string]gh.PRStatus, error)
}

// The reader's edge, held as a variable so the tests can stand in for it: the
// real one shells out to gh.
var newPRReader = defaultPRReader

// defaultPRReader is the real gh on PATH.
func defaultPRReader() PRReader { return gh.New() }

// prStateMsg carries one reading of the pull requests of the slices whose work
// is out: how ready each open one is, keyed by slice ID, the slices whose
// pull request the reading found is no longer open at all, the ones the
// reading marked Done because the pull request had merged, and the ones it
// wrote back to In progress because a Done slice's pull request read open —
// the un-done rule, for a slice marked Done under the old rule (see the
// domain rule on StateOf).
//
// A slice in none of them is one gh could not be asked about, which the board
// reads as a review still to come on work in flight and as nothing at all on
// work that is finished — exactly what each said before there was any reading.
type prStateMsg struct {
	state    map[string]domain.PRReadiness
	settled  []string
	marked   []string
	reopened []string
}

// refreshPRStates reads what GitHub says about the pull request of every slice
// that has one recorded and might still be waiting on it, and comes back with
// the lot as one reading.
//
// It has no timer of its own: it rides the plan's own cadence, kicked off by
// each plan that lands — the background poll, the nudge marker, the refresh key
// — because a pull request being approved is news of the same kind and of much
// the same age as the plan itself. There is nothing to read on a board that has
// launched nothing, and the whole command is skipped when no slice on the plan
// has a pull request left to ask about, which is most boards most of the time.
//
// Two kinds of slice are asked about, and for the same reason: a slice in
// progress whose work is out is waiting on the review, and a Done slice is
// waiting on the merge — the board marks a slice Done as it opens the pull
// request, days before that pull request lands. What settles either is the
// pull request no longer being open, and that answer is kept for the session
// (see [App.prSettled]), since a merged pull request does not unmerge and a
// plan's finished work would otherwise be re-read for as long as the board is
// up.
//
// The reading is one listing per repository rather than one view per slice, so
// what it costs is the number of repositories the plan spans rather than the
// number of pull requests it has ever produced. One reading runs at a time —
// see [App.prReading] — since a gh on a slow network can outlast the interval
// it was started on.
func (a *App) refreshPRStates() tea.Cmd {
	if a.prReader == nil || a.project == nil || a.prReading {
		return nil
	}
	project, ok := a.activeProject()
	if !ok {
		return nil
	}
	// The directories are kept in the order the plan first names them, so a
	// reading runs the same way twice.
	var dirs []string
	reads := map[string][]domain.Slice{}
	for _, s := range a.project.Slices {
		if !worthReading(s) || a.prSettled[s.ID] {
			continue
		}
		dir := expandHome(strings.TrimSpace(workdirFor(s, project)))
		if _, seen := reads[dir]; !seen {
			dirs = append(dirs, dir)
		}
		reads[dir] = append(reads[dir], s)
	}
	if len(dirs) == 0 {
		return nil
	}
	st, _, ok := a.activeStore()
	if !ok {
		return nil
	}
	a.prReading = true
	reader, viewer := a.prReader, a.prViewer
	return func() tea.Msg {
		msg := prStateMsg{state: map[string]domain.PRReadiness{}}
		for _, dir := range dirs {
			open, err := reader.OpenPRs(dir)
			if err != nil {
				// gh has logged the failure itself; this is the decision taken
				// about it. Every slice of that repository is left out — nothing
				// is read and, above all, nothing is settled: a listing that never
				// happened must not be taken for a pull request that has landed.
				logging.Action("left a repository's pull requests unread", "dir", dir, "error", err)
				continue
			}
			for _, s := range reads[dir] {
				status, still := open[gh.NormaliseURL(s.PRURL)]
				if !still {
					// An in-progress slice whose pull request is absent is either
					// merged — GitHub made the merge nat would have — or closed
					// unmerged, which is work going round again; the pull
					// request's own reading tells them apart, and a merged one
					// marks the slice Done here, since nothing else witnessed it.
					// A reading that failed settles nothing: the next pass asks
					// again rather than watching an answer nobody has.
					if s.Status == domain.SliceClaimed && viewer != nil {
						done, err := actions.SettleMerged(context.Background(), st, viewer, s, dir)
						if err != nil {
							logging.Action("left an absent pull request unsettled", "slice", s.ID, "error", err)
							continue
						}
						if done {
							msg.marked = append(msg.marked, s.ID)
						}
					}
					msg.settled = append(msg.settled, s.ID)
					continue
				}
				msg.state[s.ID] = readinessOf(status)
				if s.Status == domain.SliceDone {
					// A Done slice whose pull request reads open is Notion's
					// word disagreeing with the work: Done was written at
					// approve, under the old rule, rather than at a merge
					// that has not happened. Writing it back to In progress
					// is the un-done rule — the mirror of the settle branch
					// above — and what lets every other reading of the page
					// trust Done to mean merged from here on.
					if err := actions.ReopenUnmerged(context.Background(), st, s); err != nil {
						logging.Action("left a Done slice with an open pull request unreopened", "slice", s.ID, "error", err)
						continue
					}
					msg.reopened = append(msg.reopened, s.ID)
				}
			}
		}
		return msg
	}
}

// worthReading reports whether a slice has a pull request that anything might
// still be waiting on. A slice with none has nothing to ask about, and one
// neither in progress nor Done has not got as far as producing one — a Todo
// slice carrying a PR URL is work that went round again, and the URL is a
// record of the last time rather than something in flight.
func worthReading(s domain.Slice) bool {
	if s.PRURL == "" {
		return false
	}
	return s.Status == domain.SliceClaimed || s.Status == domain.SliceDone
}

// readinessOf turns what gh said about an open pull request into what the rule
// takes. A failed check comes first, whatever the review says, so a pull
// request is never ready to merge while CI is red. Approved and mergeable is
// the review over; anything else is a review
// still to come — a pull request nobody has approved, or an approved one GitHub
// cannot merge as it stands, which is work for the author again rather than for
// a reviewer.
func readinessOf(status gh.PRStatus) domain.PRReadiness {
	if status.Checks == gh.ChecksFailing {
		return domain.PRChecksFailing
	}
	if status.Approved && status.Mergeable {
		return domain.PRReadyToMerge
	}
	return domain.PRAwaitingReview
}

// prStateRead takes a reading to the board, and remembers the pull requests it
// found had landed. Nothing is toasted and nothing fails: a reading that could
// not be taken is already logged, and what it would have refined is a state the
// board is drawing perfectly well without it.
//
// A pull request that has just landed is also the end of the work it was cut
// for, so the worktrees of the slices this reading settled go with it — the
// same edge that drops those slices from the Active panel, witnessed once. See
// [App.removeLanded].
func (a *App) prStateRead(msg prStateMsg) tea.Cmd {
	a.prReading = false
	a.prState = msg.state
	if len(msg.settled) > 0 && a.prSettled == nil {
		a.prSettled = map[string]bool{}
	}
	for _, id := range msg.settled {
		a.prSettled[id] = true
	}
	a.board.SetPRState(a.prState)
	// The board's rows are drawn into a viewport and cached there, so a reading
	// that is not synced never reaches the screen.
	a.syncBoard()
	cmds := []tea.Cmd{a.removeLanded(msg.settled), a.nudgeFailingChecks(msg.state)}
	// A slice the reading marked Done, or reopened to In progress, changed
	// under the plan's copy of it, and the row should say so without waiting
	// for a poll.
	for _, id := range msg.marked {
		cmds = append(cmds, a.refreshSlice(id))
	}
	for _, id := range msg.reopened {
		cmds = append(cmds, a.refreshSlice(id))
	}
	return tea.Batch(cmds...)
}

// checksNudgeMsg reports one nudge typed at a live agent's pane: the slice it
// was about, the session it went to, and the failure that stopped it.
type checksNudgeMsg struct {
	sliceID, session string
	err              error
}

// nudgeFailingChecks tells each live agent whose slice's pull request the
// reading found failing its checks, in one prompt (see
// [agent.ChecksFailingPrompt]), and re-arms every slice the reading found
// failing no longer.
//
// It is edge-triggered, the way [App.prSettled] remembers a landing: a slice
// is nudged once as its checks go red and not again until a reading has seen
// them out of the red, so a board polling every thirty seconds never nags, and
// an agent mid-turn simply finds the prompt queued. A slice the reading left
// out — gh could not be asked — is neither nudged nor re-armed, since a
// reading that failed concludes nothing. A slice with no live agent is not
// nudged either, and not marked: the board's own checks-failing state is what
// tells the user, and an agent launched onto it later is told on the next
// reading.
//
// The mark goes on as the send is started rather than as it lands, so a
// second reading arriving first sends nothing twice; a send that fails takes
// it off again — see [App.checksNudgeSent].
func (a *App) nudgeFailingChecks(state map[string]domain.PRReadiness) tea.Cmd {
	if a.launcher == nil || a.project == nil {
		return nil
	}
	launcher := a.launcher
	var cmds []tea.Cmd
	for _, s := range a.project.Slices {
		readiness, read := state[s.ID]
		if !read {
			continue
		}
		if readiness != domain.PRChecksFailing {
			delete(a.checksNudged, s.ID)
			continue
		}
		session := a.live[s.ID]
		if session == "" || a.checksNudged[s.ID] {
			continue
		}
		if a.checksNudged == nil {
			a.checksNudged = map[string]bool{}
		}
		a.checksNudged[s.ID] = true
		id, prompt := s.ID, agent.ChecksFailingPrompt(s.PRURL, agentBranch(s))
		cmds = append(cmds, func() tea.Msg {
			return checksNudgeMsg{sliceID: id, session: session, err: launcher.SendPrompt(session, prompt)}
		})
	}
	return tea.Batch(cmds...)
}

// checksNudgeSent logs how a nudge went. One that failed is unmarked, so the
// next reading that still finds the checks failing tries again; nothing is
// toasted either way, since the board is already drawing the failing checks.
func (a *App) checksNudgeSent(msg checksNudgeMsg) {
	if msg.err != nil {
		delete(a.checksNudged, msg.sliceID)
		logging.Action("could not tell an agent its pull request's checks are failing",
			"slice", msg.sliceID, "session", msg.session, "error", msg.err)
		return
	}
	logging.Action("told an agent its pull request's checks are failing",
		"slice", msg.sliceID, "session", msg.session)
}
