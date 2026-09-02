package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/gh"
)

// prViewState is how far the pull request has got: nothing asked for yet, a
// read in flight, one on screen, or a read that failed.
type prViewState int

const (
	prViewIdle prViewState = iota
	prViewLoading
	prViewReady
	prViewFailed
)

// prHeaderHeight is the lines the screen's own header takes above the
// description: what the pull request is, the branches it would move, and the
// blank line that separates the two from the body. It is spent whether or not
// there is a pull request to draw yet, so the viewport under it is one size
// however the read goes — a screen that changed height between loading and
// ready would scroll differently either side of it.
const prHeaderHeight = 3

// PRView is the pull request screen: what GitHub says about the pull request
// recorded on a slice, read through the GitHub CLI and drawn over the board the
// way the diff, help and info screens are.
//
// It holds gh's answer rather than the rendered body, because the description
// is markdown wrapped to the width it is drawn at: every resize renders again
// from the source, exactly as the info screen does.
//
// Nothing here writes anything, and nothing here reaches GitHub: reading is the
// whole of it, and the read itself is the root model's ([App.viewPRFlow]).
type PRView struct {
	styles Styles
	vp     viewport.Model

	// style is the glamour stylesheet the description is rendered with; a name
	// glamour does not know falls back to unrendered markdown rather than an
	// empty screen.
	style string

	// sliceID is the page ID of the slice whose pull request is on show, and
	// slice its name; ref is what names the pull request to gh — the URL the
	// slice's PR property holds — and dir the repository it is read in. The last
	// two are kept so the refresh key can ask for it again: a review left while
	// the screen is up is exactly when a second read is wanted.
	sliceID string
	slice   string
	ref     string
	dir     string

	pr    gh.PR
	state prViewState
	err   error

	// prompt is the merge question waiting to be answered, drawn on the merge
	// box it is about — the screen's own inline confirmation, the way the board
	// asks about the row the cursor is on. It is nil whenever nothing is being
	// asked.
	prompt *rowPrompt

	width, height int
}

// prKeyMap is what the pull request screen answers to beyond the viewport's own
// scrolling: the one key that acts rather than reads, which merges the pull
// request on show once its merge box says it can.
type prKeyMap struct {
	Merge key.Binding
}

// defaultPRKeyMap returns the bindings the pull request screen runs with. m is
// the merge because it is the word: the board's own m moves a slice between
// milestones, and the board is not what is on screen here.
func defaultPRKeyMap() prKeyMap {
	return prKeyMap{
		Merge: key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "merge")),
	}
}

// bindings are the screen's keys as the help screen lists them.
func (k prKeyMap) bindings() []key.Binding { return []key.Binding{k.Merge} }

// hints are the screen's own hints row: the merge, and the way back to the
// board. Scrolling is not among them, for the reason the diff screen leaves it
// out — it is the keys everyone tries first — and neither is the refresh key,
// which is the app's own and named on the help screen.
//
// The merge is offered only while there is a merge to attempt: a pull request
// that has already merged, or been closed without merging, has nothing left for
// the key to do, and a screen with no pull request read into it has nothing at
// all.
func (p PRView) hints(keys prKeyMap, back key.Binding) []hint {
	if _, ok := p.Mergeable(); !ok {
		return nil
	}
	return []hint{{keys.Merge, 3}, {back, 1}}
}

// Mergeable is the pull request the merge key would act on: the one on screen,
// so long as it is still open. A read in flight or one that failed has none,
// and neither has a pull request already merged or closed — what became of it
// is the whole of what the merge box has left to say.
func (p PRView) Mergeable() (gh.PR, bool) {
	if p.state != prViewReady || p.pr.State == gh.PRStateMerged || p.pr.State == gh.PRStateClosed {
		return gh.PR{}, false
	}
	return p.pr, true
}

// SetPrompt anchors a question to the merge box, focused on its first choice,
// and ClearPrompt takes it down. Both render again, since the prompt is drawn
// in the viewport's own content.
func (p *PRView) SetPrompt(options []string) {
	p.prompt = &rowPrompt{options: options}
	p.render()
	// The merge box is the last thing in the content, so the question is asked
	// where it can be read.
	p.vp.GotoBottom()
}

// ClearPrompt takes the prompt down, answered or abandoned.
func (p *PRView) ClearPrompt() {
	p.prompt = nil
	p.render()
}

// Prompting reports whether a prompt is waiting to be answered — while one is,
// the root model gives it the keys.
func (p PRView) Prompting() bool { return p.prompt != nil }

// PromptChoice is the index of the focused choice, which is what answering the
// prompt answers with. With no prompt up there is nothing to answer, and the
// first choice is as good an answer as any.
func (p PRView) PromptChoice() int {
	if p.prompt == nil {
		return 0
	}
	return p.prompt.cursor
}

// MovePrompt steps the focused choice, stopping at either end rather than
// wrapping, and draws the chip again where it has got to.
func (p *PRView) MovePrompt(delta int) {
	if p.prompt == nil {
		return
	}
	p.prompt.move(delta)
	p.render()
}

// NewPRView returns an empty pull request screen, waiting for one to be read
// into it.
func NewPRView(styles Styles) PRView {
	return PRView{styles: styles, vp: viewport.New(), style: DefaultGlamourStyle}
}

// SetSize gives the screen the space it has and renders the description again
// to it: glamour wraps to a fixed width, so a resize is a re-render rather than
// a reflow.
func (p *PRView) SetSize(width, height int) {
	p.width, p.height = width, height
	p.vp.SetWidth(max(width, 1))
	p.vp.SetHeight(max(height-prHeaderHeight, 1))
	p.render()
}

// Start marks a read of a slice's pull request as in flight, so the screen
// shows progress and whatever was on it before does not read as this one's.
func (p *PRView) Start(sliceID, slice, ref, dir string) {
	p.sliceID, p.slice, p.ref, p.dir = sliceID, slice, ref, dir
	p.state, p.err = prViewLoading, nil
	p.pr = gh.PR{}
	// A question about the last reading is not one to leave open over the next.
	p.prompt = nil
	p.render()
	p.vp.GotoTop()
}

// SetPR shows a pull request that came back, from the top of its description.
func (p *PRView) SetPR(pr gh.PR) {
	p.pr, p.state, p.err = pr, prViewReady, nil
	p.render()
	p.vp.GotoTop()
}

// Fail reports a read that did not come back. What was on screen goes with it,
// the way the diff's does rather than the info screen's: a pull request is one
// reading at one moment, and leaving the last one up under a failure would be
// saying that is how GitHub still has it. The slice and its pull request are
// still there, which is why this is a state of the screen rather than an error
// the board has to be got out of.
func (p *PRView) Fail(err error) {
	p.state, p.err = prViewFailed, err
	p.pr = gh.PR{}
	p.render()
}

// Busy reports whether a read is in flight, which is what keeps the root
// model's spinner turning.
func (p PRView) Busy() bool { return p.state == prViewLoading }

// Loadable reports whether there is a pull request to read again — one the
// screen has been pointed at, which the refresh key can ask for a second time.
func (p PRView) Loadable() bool { return p.ref != "" && p.dir != "" }

// Target is the slice, pull request ref and directory the screen was last
// pointed at.
func (p PRView) Target() (slice, ref, dir string) { return p.slice, p.ref, p.dir }

// SliceID is the page ID of the slice whose pull request is on show.
func (p PRView) SliceID() string { return p.sliceID }

// Number is the pull request's own number, or zero before one has been read.
// It is what the layout's header names the screen by.
func (p PRView) Number() int { return p.pr.Number }

// Reset drops the pull request and what it was of, so nothing of one slice's is
// left on the screen the next slice's opens.
func (p *PRView) Reset() {
	*p = PRView{styles: p.styles, vp: p.vp, style: p.style, width: p.width, height: p.height}
	p.render()
	p.vp.GotoTop()
}

// render rebuilds the viewport's content at the current width: the description,
// under it the checks, and under those the merge box. A pull request opened
// with no description at all draws a line saying so rather than an empty band.
//
// The checks go under the description rather than over it, which is where the
// conversation will go under them: the description is what the pull request is
// and is read first, and the checks are one reading of it at one moment, which
// the refresh key is for. They are in the viewport with it rather than pinned
// above it, since a repository with a workflow per platform has more checks
// than a screen has lines. The merge box comes last because it is the
// conclusion the rows above are read into — see [PRView.mergeSection].
func (p *PRView) render() {
	body := fit(p.styles.Faint.Render("This pull request has no description."), p.width)
	if described := strings.TrimSpace(p.pr.Body); described != "" {
		body = renderMarkdown(described, p.style, p.width)
	}
	p.vp.SetContent(strings.TrimRight(body, "\n") + "\n\n" + p.checksSection() +
		"\n\n" + p.mergeSection())
}

// Update handles the screen's keys: the viewport's own scrolling. Everything
// else — leaving the screen, refreshing it — belongs to the root model.
func (p *PRView) Update(msg tea.Msg) tea.Cmd {
	vp, cmd := p.vp.Update(msg)
	p.vp = vp
	return cmd
}

// View renders the screen's body; the layout's header is what names it. spinner
// is the root model's current frame, drawn while the read is in flight so the
// app turns one spinner rather than two.
func (p PRView) View(spinner string) string {
	switch p.state {
	case prViewIdle:
		return p.styles.Faint.Render("Move to a slice with a pull request and press V to read it.")
	case prViewLoading:
		return spinner + fmt.Sprintf(" Reading the pull request of %s…", p.slice)
	case prViewFailed:
		return p.styles.Error.Render(oneLine(p.err.Error()))
	}
	return strings.Join(append(p.headerLines(), "", p.vp.View()), "\n")
}

// headerLines are the two lines above the description: what the pull request is
// — its state, its number and its title — and, under them, the branches it
// would move. They are the screen's own rather than the layout's header, which
// has one line for every screen and names this one by its number alone.
func (p PRView) headerLines() []string {
	title := fmt.Sprintf("#%d %s", p.pr.Number, p.pr.Title)
	first := p.stateChip() + " " + p.styles.Title.Render(title)
	return []string{fit(first, p.width), fit(p.styles.Faint.Render(p.branchLine()), p.width)}
}

// branchLine is what the pull request would move and where to: the branch the
// work is on, and the one it is asking to be merged into.
func (p PRView) branchLine() string {
	return fmt.Sprintf("%s → %s", p.pr.HeadRefName, p.pr.BaseRefName)
}

// stateChip is where the pull request stands, in GitHub's own four words. Draft
// is tested first and against the state as well: a draft is an open pull
// request GitHub is not offering to merge, and one that has since been merged
// or closed is no longer a draft whatever the flag says.
func (p PRView) stateChip() string {
	word, style := prStateChip(p.styles, p.pr)
	return style.Render(word)
}

// prStateChip is that chip as a word and the style it is drawn in, kept apart
// so the mapping can be read — and tested — without a rendered line.
func prStateChip(s Styles, pr gh.PR) (string, lipgloss.Style) {
	switch {
	case pr.State == gh.PRStateMerged:
		return "merged", s.PRMerged
	case pr.State == gh.PRStateClosed:
		return "closed", s.PRClosed
	case pr.IsDraft:
		return "draft", s.PRDraft
	default:
		// An open pull request, and anything GitHub says that this build does not
		// know: a state nobody here recognises is still a pull request that has
		// neither merged nor closed.
		return "open", s.PROpen
	}
}
