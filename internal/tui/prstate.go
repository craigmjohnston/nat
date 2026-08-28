package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

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
// is out: how ready each open one is, keyed by slice ID, and the slices whose
// pull request the reading found is no longer open at all.
//
// A slice in neither is one gh could not be asked about, which the board reads
// as a review still to come on work in flight and as nothing at all on work
// that is finished — exactly what each said before there was any reading.
type prStateMsg struct {
	state   map[string]domain.PRReadiness
	settled []string
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
	type read struct {
		id  string
		url string
	}
	// The directories are kept in the order the plan first names them, so a
	// reading runs the same way twice.
	var dirs []string
	reads := map[string][]read{}
	for _, s := range a.project.Slices {
		if !worthReading(s) || a.prSettled[s.ID] {
			continue
		}
		dir := expandHome(strings.TrimSpace(workdirFor(s, project)))
		if _, seen := reads[dir]; !seen {
			dirs = append(dirs, dir)
		}
		reads[dir] = append(reads[dir], read{id: s.ID, url: s.PRURL})
	}
	if len(dirs) == 0 {
		return nil
	}
	a.prReading = true
	reader := a.prReader
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
			for _, r := range reads[dir] {
				status, still := open[gh.NormaliseURL(r.url)]
				if !still {
					msg.settled = append(msg.settled, r.id)
					continue
				}
				msg.state[r.id] = readinessOf(status)
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
// takes. Approved and mergeable is the review over; anything else is a review
// still to come — a pull request nobody has approved, or an approved one GitHub
// cannot merge as it stands, which is work for the author again rather than for
// a reviewer.
func readinessOf(status gh.PRStatus) domain.PRReadiness {
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
	return a.removeLanded(msg.settled)
}
