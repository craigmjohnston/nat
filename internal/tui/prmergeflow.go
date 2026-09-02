package tui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/gh"
)

// PRMerger is what the merge key needs of the GitHub CLI: the pull request on
// screen merged, in the repository the slice belongs to. It is an interface for
// the reason [PRViewer] and [PRCreator] are — the flow can then be driven
// without gh, without a network and without a GitHub account, which is what
// keeps a test of it from merging anything.
type PRMerger interface {
	MergePR(dir, ref string) error
}

// The merge flow's edge, held as a variable so the tests can stand in for it:
// the real one shells out to gh.
var newPRMerger = defaultPRMerger

// defaultPRMerger is the real gh on PATH.
func defaultPRMerger() PRMerger { return gh.New() }

// The choices the merge prompt offers, in the order they read. Merging comes
// first, so a single enter takes it; the way out is beside it as well as on
// esc, exactly as the release prompt has it.
var mergeChoices = []string{"merge", "cancel"}

const (
	choiceMerge = iota
	choiceCancelMerge
)

// mergeNote is what the status bar says while gh is merging.
const mergeNote = "Merging the pull request…"

// prMergedMsg reports the merge that happened, or the refusal that came
// instead. It carries the number the merge was of, since the screen is read
// again on the way out and the pull request it holds is replaced by that read.
type prMergedMsg struct {
	number int
	err    error
}

// mergePRFlow is the merge key on the pull request screen, which is the one
// place merging is offered: the merge box on show is what says whether the pull
// request can go in, and merging it is what reading that box is answered with.
//
// It is refused where the box has already answered no — changes requested,
// failing checks, a branch that conflicts with its base — with the box's own
// wording as the reason, so the toast and the line above it say the same thing.
// A pull request already merged or closed has nothing to merge, and neither has
// a screen with no reading on it at all.
//
// Nothing is written to Notion by any of this: the slice was marked Done as its
// pull request was opened, and the merge is the work landing rather than the
// slice changing. What the merge does move is the slice's worktree, which the
// board takes away when it next reads that the pull request is no longer open —
// see [App.removeLanded].
func (a *App) mergePRFlow() tea.Cmd {
	if a.prMerger == nil || a.busy {
		return nil
	}
	pr, ok := a.prview.Mergeable()
	if !ok {
		return a.showToast("There is no open pull request on screen to merge.", sevWarning)
	}
	if reason, refused := mergeRefusal(pr); refused {
		return a.showToast(fmt.Sprintf("Cannot merge #%d — %s.", pr.Number, reason), sevWarning)
	}
	return a.openPRPrompt(mergeChoices, func(choice int) tea.Cmd {
		return a.mergeChosen(pr, choice)
	})
}

// mergeChosen is what answering the prompt does. Backing out says nothing —
// nothing was in flight, and the pull request is as it was.
func (a *App) mergeChosen(pr gh.PR, choice int) tea.Cmd {
	if choice != choiceMerge {
		return nil
	}
	_, ref, dir := a.prview.Target()
	a.busy, a.note = true, mergeNote
	return mergePR(a.prMerger, pr.Number, ref, dir)
}

// mergePR runs gh in the slice's repository and reports what came of it. The
// ref is the one the screen was opened with — the URL recorded on the slice —
// so the pull request merged is the one that was read.
func mergePR(merger PRMerger, number int, ref, dir string) tea.Cmd {
	return func() tea.Msg {
		if err := merger.MergePR(dir, ref); err != nil {
			return prMergedMsg{number: number, err: err}
		}
		return prMergedMsg{number: number}
	}
}

// prMerged reports the merge and reads the pull request again, so the screen
// says merged rather than going on showing the question it just answered.
//
// A gh that refused is a toast rather than an error banner, for the reason the
// approve key's is: the pull request is still there and still open, and what
// stopped it — a branch protection rule, a review dismissed by a push, a check
// that went red between the reading and the key — is something to read and act
// on rather than a state the board has to be dismissed out of. Nothing is read
// again after one: the screen is showing the pull request as GitHub last had
// it, which is still how GitHub has it.
func (a *App) prMerged(msg prMergedMsg) (tea.Model, tea.Cmd) {
	a.busy, a.note = false, ""
	if msg.err != nil {
		return a, a.showToast(fmt.Sprintf("Could not merge #%d: %v", msg.number, msg.err), sevError)
	}
	return a, tea.Batch(a.showToast(fmt.Sprintf("Merged #%d.", msg.number), sevSuccess), a.startPRLoad())
}

// prKey handles the pull request screen's own keys, reporting whether the key
// was one of them. They live here rather than on the screen itself because the
// one of them reaches past it — to gh — the way the board's own write keys and
// the diff screen's do.
func (a *App) prKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if key.Matches(msg, a.prKeys.Merge) {
		return a.mergePRFlow(), true
	}
	return nil, false
}
