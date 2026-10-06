package tui

import (
	"context"
	"maps"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/logging"
)

// PRReader is what the board needs of the GitHub CLI to tell a pull request
// still waiting on a reviewer from one the review is over on, and either from
// one that has already landed: one batched reading ([gh.CLI.ReadPRs]) — the
// same `nat pr-status` takes — which the pull request screen's poll reads its
// one pull request in full through too. It is an interface for the reason
// [PRCreator] is: the flow can then be driven without gh, without a network
// and without a GitHub account.
type PRReader interface {
	ReadPRs(q gh.BatchQuery) (gh.Batch, error)
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
//
// failing names, for each slice read with a failed check, the checks that
// failed — what the Active panel says beside the state.
type prStateMsg struct {
	state    map[string]domain.PRReadiness
	failing  map[string][]string
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
// The reading is one batched GraphQL document for every pull request worth
// asking about ([actions.PRsWorthAsking] — a Done slice only while its
// worktree is still there to settle), so what it costs is a point of GitHub's
// budget a reading, however many pull requests and repositories the plan has.
// One reading runs at a time — see [App.prReading] — since a gh on a slow
// network can outlast the interval it was started on.
func (a *App) refreshPRStates() tea.Cmd {
	if a.prReader == nil || a.project == nil || a.prReading {
		return nil
	}
	project, ok := a.activeProject()
	if !ok {
		return nil
	}
	var candidates []domain.Slice
	for _, s := range a.project.Slices {
		if worthReading(s) && !a.prSettled[s.ID] {
			candidates = append(candidates, s)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	st, _, ok := a.activeStore()
	if !ok {
		return nil
	}
	a.prReading = true
	reader := a.prReader
	// The live sessions are copied here, on the event loop, since the reading
	// runs off it and the board's own map is the loop's to change.
	sender, live, projectID := a.checksSender(), maps.Clone(a.live), a.cfg.ActiveProjectID
	return func() tea.Msg {
		msg := prStateMsg{state: map[string]domain.PRReadiness{}, failing: map[string][]string{}}
		// Which Done slices still have a worktree is git's to say, so it is
		// asked here, off the event loop.
		var q gh.BatchQuery
		asked := map[string]gh.PRRef{}
		for _, s := range actions.PRsWorthAsking(newWorktrees(), project, candidates) {
			ref, ok := gh.ParsePRURL(s.PRURL)
			if !ok {
				logging.Action("left a pull request unread: its URL names none", "slice", s.ID, "pr", s.PRURL)
				continue
			}
			asked[s.ID] = ref
			q.PRs = append(q.PRs, ref)
		}
		if len(q.PRs) == 0 {
			return msg
		}
		batch, err := reader.ReadPRs(q)
		if err != nil {
			// gh has logged each failed document; this is the decision taken
			// about it. What a failed document asked about is absent below, and
			// absent concludes nothing — above all, settles nothing: a reading
			// that never happened must not be taken for a pull request landed.
			logging.Action("read the pull requests in part", "error", err)
		}
		var red []actions.FailingChecks
		for _, s := range candidates {
			ref, isAsked := asked[s.ID]
			pr, read := batch.PRs[ref]
			if !isAsked || !read {
				continue
			}
			status := gh.StatusOf(pr)
			if status.State == gh.PRStateMerged || status.State == gh.PRStateClosed {
				// Merged — GitHub made the merge nat would have — or closed,
				// which is work going round again. A merged in-progress slice
				// is marked Done here, since nothing else witnessed it; either
				// way the answer cannot change, so the slice is settled.
				if s.Status == domain.SliceClaimed {
					done, err := actions.SettleMerged(context.Background(), st, s, status)
					if err != nil {
						logging.Action("left a merged pull request unsettled", "slice", s.ID, "error", err)
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
			if msg.state[s.ID] == domain.PRChecksFailing {
				red = append(red, actions.FailingChecks{Slice: s, Failing: status.Failing})
				for _, c := range status.Failing {
					msg.failing[s.ID] = append(msg.failing[s.ID], c.Name)
				}
			}
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
		// What the reading found red is told to the agent on it, or put on the
		// record where there is none — once per failure, see
		// [actions.NoticeFailingChecks], the same function `nat pr-status` runs.
		actions.NoticeFailingChecks(context.Background(), st, sender, live, projectID, red)
		return msg
	}
}

// checksSender is the launcher as what a nudge types through, or nil on a
// board with no launcher — where every red reading is recorded rather than
// told, since there is no session the board could reach.
func (a *App) checksSender() actions.PromptSender {
	if a.launcher == nil {
		return nil
	}
	return a.launcher
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
	a.board.SetFailingChecks(msg.failing)
	// The board's rows are drawn into a viewport and cached there, so a reading
	// that is not synced never reaches the screen.
	a.syncBoard()
	cmds := []tea.Cmd{a.removeLanded(msg.settled)}
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
