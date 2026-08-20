package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/logging"
)

// PRReader is what the board needs of the GitHub CLI to tell a pull request
// still waiting on a reviewer from one the review is over on. It is an
// interface for the reason [PRCreator] is: the flow can then be driven without
// gh, without a network and without a GitHub account.
type PRReader interface {
	PRStatus(dir, url string) (gh.PRStatus, error)
}

// The reader's edge, held as a variable so the tests can stand in for it: the
// real one shells out to gh.
var newPRReader = defaultPRReader

// defaultPRReader is the real gh on PATH.
func defaultPRReader() PRReader { return gh.New() }

// prStateMsg carries one reading of the pull requests of the slices whose work
// is out: how ready each is, keyed by slice ID. A slice gh could not be asked
// about is left out rather than carried as a state of its own — the board reads
// an absent slice as a review still to come, which is exactly what it said
// before there was any reading.
type prStateMsg struct {
	state map[string]domain.PRReadiness
}

// refreshPRStates reads what GitHub says about the pull request of every slice
// whose work is out, and comes back with the lot as one reading.
//
// It has no timer of its own: it rides the plan's own cadence, kicked off by
// each plan that lands — the background poll, the nudge marker, the refresh key
// — because a pull request being approved is news of the same kind and of much
// the same age as the plan itself. There is nothing to read on a board that has
// launched nothing, and the whole command is skipped when no slice on the plan
// has a pull request recorded, which is most boards most of the time.
//
// Only slices in progress are asked about: a Done slice's pull request is the
// record of work that has landed, and the classifier says nothing about it
// however GitHub answers. One reading runs at a time — see [App.prReading] —
// since a gh per slice on a slow network can outlast the interval it was
// started on.
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
		dir string
		url string
	}
	var reads []read
	for _, s := range a.project.Slices {
		if s.Status != domain.SliceClaimed || s.PRURL == "" {
			continue
		}
		reads = append(reads, read{
			id:  s.ID,
			dir: expandHome(strings.TrimSpace(workdirFor(s, project))),
			url: s.PRURL,
		})
	}
	if len(reads) == 0 {
		return nil
	}
	a.prReading = true
	reader := a.prReader
	return func() tea.Msg {
		state := make(map[string]domain.PRReadiness, len(reads))
		for _, r := range reads {
			status, err := reader.PRStatus(r.dir, r.url)
			if err != nil {
				// gh has logged the failure itself; this is the decision taken
				// about it. The slice is left out, so the board goes on saying
				// what it last said about it.
				logging.Action("left a slice's pull request unread", "url", r.url, "error", err)
				continue
			}
			state[r.id] = readinessOf(status)
		}
		return prStateMsg{state: state}
	}
}

// readinessOf turns what gh said about a pull request into what the rule takes.
// Approved and mergeable is the review over; anything else is a review still to
// come — a pull request nobody has approved, or an approved one GitHub cannot
// merge as it stands, which is work for the author again rather than for a
// reviewer.
func readinessOf(status gh.PRStatus) domain.PRReadiness {
	if status.Approved && status.Mergeable {
		return domain.PRReadyToMerge
	}
	return domain.PRAwaitingReview
}

// prStateRead takes a reading to the board. Nothing is toasted and nothing
// fails: a reading that could not be taken is already logged, and what it would
// have refined is a state the board is drawing perfectly well without it.
func (a *App) prStateRead(msg prStateMsg) tea.Cmd {
	a.prReading = false
	a.prState = msg.state
	a.board.SetPRState(a.prState)
	// The board's rows are drawn into a viewport and cached there, so a reading
	// that is not synced never reaches the screen.
	a.syncBoard()
	return nil
}
