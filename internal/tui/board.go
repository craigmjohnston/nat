package tui

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/craigmjohnston/nat/internal/domain"
)

// boardKeyMap is the board's own bindings: navigation, the writes, and the
// agent keys.
//
// Everything but the navigation is named here but handled by the root model:
// the writes need the Notion client and the project config, and the agent keys
// the tmux launcher, none of which the board has any business holding.
type boardKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Toggle   key.Binding
	HideDone key.Binding

	Add    key.Binding
	Edit   key.Binding
	Move   key.Binding
	Delete key.Binding
	// Diff opens the review screen on the branch a slice was handed back on,
	// which is where the work is read and where approving it now lives: the
	// board has no approve key of its own, since approving a change nobody has
	// looked at was never what the key was for.
	Diff key.Binding
	// PR opens the pull request screen on the pull request a slice records,
	// which is the reading after the diff's: the branch is what there is to
	// review, and the pull request is what became of it once it was approved.
	PR key.Binding
	// Release hands a slice in progress back to the plan — Todo and unassigned
	// — for when the session working it ended without finishing it. It is the
	// way out of the one state a slice otherwise gets stuck in.
	Release key.Binding

	Launch key.Binding
	Attach key.Binding
	Plan   key.Binding
	// Focus hands the keyboard to the agent terminal beside the board, and
	// FullAttach is the hatch that gives it the whole window instead.
	Focus      key.Binding
	FullAttach key.Binding

	NewProject    key.Binding
	SwitchProject key.Binding
}

// defaultBoardKeyMap returns the bindings the board runs with.
func defaultBoardKeyMap() boardKeyMap {
	return boardKeyMap{
		Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Toggle:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand/collapse")),
		HideDone: key.NewBinding(key.WithKeys("z"), key.WithHelp("z", "hide/show done slices")),

		Add:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add slice")),
		Edit:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit slice")),
		Move:   key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "move slice")),
		Delete: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete slice")),

		Diff:    key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "review diff")),
		PR:      key.NewBinding(key.WithKeys("V"), key.WithHelp("V", "view pull request")),
		Release: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "release slice")),

		Launch: key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "launch agent")),
		Attach: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "show/hide agent")),
		Plan:   key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "planning agent")),

		Focus:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "type at the agent")),
		FullAttach: key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "agent full-screen")),

		NewProject:    key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "new project")),
		SwitchProject: key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "switch project")),
	}
}

// agents are the bindings that act on an agent session: a slice's, or the
// planning agent's.
func (k boardKeyMap) agents() []key.Binding {
	return []key.Binding{k.Launch, k.Attach, k.Plan, k.Focus, k.FullAttach}
}

// projects are the bindings that act on the plan the board is showing rather
// than anything in it.
func (k boardKeyMap) projects() []key.Binding {
	return []key.Binding{k.NewProject, k.SwitchProject}
}

// writes are the bindings the root model handles rather than the board.
func (k boardKeyMap) writes() []key.Binding {
	return []key.Binding{k.Add, k.Edit, k.Move, k.Delete, k.Diff, k.PR, k.Release}
}

// sliceHints are the hints row's bindings while the cursor is on a slice: the
// actions that act on it, in the order they read. The agent keys are what the
// tracker is for, so they survive a narrow row longest. The write keys drop
// the word "slice" their help carries — the hints only show on one, and the
// row has less room than the help screen.
//
// The diff ranks below all of them but the board-wide toggle: it is the one key
// here that does nothing at all on most rows, since only a slice handed back on
// a branch has a branch to read. It is also the way to approving that work,
// which is the review screen's own key rather than anything the board offers.
//
// The pull request key is not in the row either, for the reason release is not:
// it does something on fewer rows than any of these — only a slice whose work
// has been approved has a pull request recorded — and the help screen is where
// it is named.
//
// Release is not here at all. It is rarer than any of these — a session that
// died, rather than anything the plan does in its ordinary course — and the
// row has less room than the help screen, which is where it is named.
func (b Board) sliceHints() []hint {
	k := b.keys
	return []hint{
		{shortHint(k.Edit, "edit"), 6},
		{shortHint(k.Move, "move"), 4},
		{shortHint(k.Delete, "delete"), 5},
		{shortHint(k.Diff, "diff"), 2},
		{k.Launch, 8},
		{k.Attach, 7},
		b.doneHint(),
	}
}

// doneHint is the hide-done toggle as the hints row names it: what the key
// would do next, since the board starts with the Done slices already hidden and
// a hint for the state it is in says nothing. It acts on the whole board rather
// than on the row the rest of the hints are about, so it takes the very lowest
// rank — the first hint to go, ahead even of the way to the help screen.
func (b Board) doneHint() hint {
	desc := "hide done"
	if b.hideDone {
		desc = "show done"
	}
	return hint{shortHint(b.keys.HideDone, desc), 1}
}

// shortHint is b with its help description replaced, for a hints row whose
// context already says what the key acts on.
func shortHint(b key.Binding, desc string) key.Binding {
	return key.NewBinding(key.WithHelp(b.Help().Key, desc))
}

// milestoneHints are the hints row's bindings while the cursor is on a
// milestone: the actions that act on it or file under it. A milestone has no
// status of its own to set — it follows the slices under it — so the row is
// about what can be filed there.
func (b Board) milestoneHints() []hint {
	k := b.keys
	return []hint{{k.Add, 5}, {k.Toggle, 3}, b.doneHint()}
}

// helpBindings are the board's bindings as the help screen lists them.
func (b Board) helpBindings() []key.Binding {
	bindings := []key.Binding{b.keys.Up, b.keys.Down, b.keys.Toggle, b.keys.HideDone}
	bindings = append(bindings, b.keys.writes()...)
	bindings = append(bindings, b.keys.agents()...)
	return append(bindings, b.keys.projects()...)
}

// rowKind tells the two kinds of line the cursor moves over apart.
type rowKind int

const (
	rowMilestone rowKind = iota
	rowSlice
	rowSection
	rowActive
)

// row is one selectable line of the board, addressing back into the groups it
// was flattened from. slice is meaningless for a rowMilestone, and a rowSection
// — the Done section's own line — addresses no group at all, so its group is -1
// rather than silently aliasing the first one. A rowActive addresses no group
// either, for the same reason and by the same -1: its slice indexes the Active
// section's own list, which is drawn from the whole plan rather than from any
// one milestone.
type row struct {
	kind  rowKind
	group int
	slice int
}

// Board is the main screen: the project's milestones in plan order, each with
// its slices under it. Expanded groups list their slices; collapsed ones show
// only a done/total count.
//
// Groups are flattened into a list of rows on every change, so the cursor is a
// single index and navigation does not care about the tree underneath.
type Board struct {
	styles Styles
	keys   boardKeyMap

	project  *domain.Project
	groups   []domain.Group
	expanded map[string]bool
	rows     []row
	cursor   int
	// active is the plan's slices in flight, in the order the board draws their
	// milestones: the Active section's own list, rebuilt with the rows, and what
	// a rowActive addresses. showActive is whether the section is drawn at all,
	// which the layout decides and [Board.SetShowActive] records: with it off the
	// entries take no rows, so the cursor is never left on one nothing draws.
	// byID is the whole plan keyed the way domain.SlicesByID keys it, which is
	// what a slice's state is classified against — see active.go.
	active     []domain.Slice
	showActive bool
	byID       map[string]domain.Slice
	// hideDone keeps the Done slices of milestones still in flight off the
	// board, so what is left of a half-finished milestone is what shows. It
	// starts on, because what is left to do is what the board is read for; the
	// key turns it off to see the whole milestone. It is one board-wide bit,
	// kept for the session like the expanded map, and it only changes what is
	// drawn: progress and counts still weigh every slice. Milestones inside the
	// Done section are exempt — everything under there is done, and hiding it
	// would leave them empty.
	hideDone bool
	// live maps the ID of each slice with an agent running to the session it
	// runs in, so a slice with an agent on it can be marked.
	live map[string]string
	// blocked maps the ID of each slice waiting on unfinished work to the
	// slices it waits on, keyed the way domain.SlicesByID keys them. It is
	// computed from the whole plan whenever one is loaded, since a slice's
	// dependencies are other rows of the same board, and it is what both the
	// blocked chip and the launch key's refusal are read from.
	blocked map[string][]domain.Slice
	// activity is how those agents are getting on, and pulse the frame the
	// star animation is on. Both are only ever read through the star chip —
	// see presence.go.
	activity map[string]Presence
	pulse    int
	// prState maps the ID of each slice whose pull request was read as still
	// open to how ready it is, which is what tells a review still to come from
	// one that is over, and what keeps a Done slice in the Active section until
	// its pull request lands. A slice the map says nothing about — no pull
	// request, one gh could not be asked about, or one that has merged — is a
	// review still to come while the slice is in flight, and nothing at all once
	// it is Done; see [Board.state].
	prState map[string]domain.PRReadiness

	// confirmText is the inline confirmation anchored to the row the cursor is
	// on, drawn from its right edge in confirmSev's colour; empty when there is
	// none. Moving the cursor dismisses it — it is about the row it was born
	// on, and would otherwise follow the cursor to rows it says nothing about.
	confirmText string
	confirmSev  severity
	// prompt is the question anchored to that same row, waiting to be answered;
	// nil when there is none. It is drawn where a confirmation would be and in
	// the same shape, but it is answered rather than waited out, so the root
	// model gives it the keys while it is up.
	prompt *rowPrompt

	width int
}

// rowPrompt is an inline question on a board row: the choices as they read,
// left to right, and which of them is focused — the one enter would answer
// with.
type rowPrompt struct {
	options []string
	cursor  int
}

// NewBoard returns an empty board, waiting for a project to be loaded into it.
func NewBoard(styles Styles) Board {
	return Board{
		styles:     styles,
		keys:       defaultBoardKeyMap(),
		expanded:   map[string]bool{},
		hideDone:   true,
		showActive: true,
	}
}

// SetProject shows a freshly loaded plan. Groups the user has already expanded
// or collapsed keep that state across a refresh; new ones start at their
// default, and the cursor is clamped to whatever rows remain.
func (b *Board) SetProject(p *domain.Project) {
	b.project = p
	b.rebuild()
}

// SetWidth records the space the board has to draw in; a row longer than it
// wraps onto continuation lines rather than losing its tail, so nothing of it
// is hidden by a narrow board.
func (b *Board) SetWidth(width int) { b.width = width }

// Cursor is the index of the row the user is on. A row is not a line — a
// wrapped one takes several — so the layout scrolls by CursorSpan rather than
// by this.
func (b Board) Cursor() int { return b.cursor }

// SetLive records the slices with an agent running, which is what the live
// marker on a slice is drawn from.
func (b *Board) SetLive(live map[string]string) { b.live = live }

// SetConfirm anchors an inline confirmation to the row the cursor is on, and
// ClearConfirm takes it down.
func (b *Board) SetConfirm(text string, sev severity) { b.confirmText, b.confirmSev = text, sev }
func (b *Board) ClearConfirm()                        { b.confirmText = "" }

// SetPrompt anchors a question to the row the cursor is on, focused on its
// first choice, and ClearPrompt takes it down. A prompt and a confirmation are
// drawn in the same place, so opening one takes the other down.
func (b *Board) SetPrompt(options []string) {
	b.prompt, b.confirmText = &rowPrompt{options: options}, ""
}
func (b *Board) ClearPrompt() { b.prompt = nil }

// Prompting reports whether a prompt is waiting to be answered — while one is,
// the root model gives it the keys.
func (b Board) Prompting() bool { return b.prompt != nil }

// PromptChoice is the index of the focused choice, which is what answering the
// prompt answers with. With no prompt up there is nothing to answer, and the
// first choice is as good an answer as any.
func (b Board) PromptChoice() int {
	if b.prompt == nil {
		return 0
	}
	return b.prompt.cursor
}

// MovePrompt steps the focused choice, stopping at either end rather than
// wrapping — the same way the cursor moves over the rows.
func (b *Board) MovePrompt(delta int) {
	if b.prompt == nil {
		return
	}
	next := b.prompt.cursor + delta
	if next < 0 || next >= len(b.prompt.options) {
		return
	}
	b.prompt.cursor = next
}

// groupKey identifies a group across reloads. The implicit Unassigned group has
// no milestone and so no ID, which is a key no milestone can collide with.
func groupKey(g domain.Group) string {
	if g.Milestone == nil {
		return ""
	}
	return g.Milestone.ID
}

// doneSectionKey is the expanded-map key of the Done section. It is not a
// group's key: milestones key by page ID and the Unassigned group by "", so it
// collides with neither.
const doneSectionKey = "done-section"

// doneGroup reports whether a group folds into the Done section: a real
// milestone whose status is Done. The Unassigned group never folds — its
// slices are stray, and worth seeing.
func doneGroup(g domain.Group) bool {
	return g.Milestone != nil && g.Milestone.Status == domain.MilestoneDone
}

// defaultExpanded is how a group is shown before the user touches it: the work
// in flight is open, everything else is a one-line summary. Slices with no
// milestone are open too — they are stray, and worth seeing.
func defaultExpanded(g domain.Group) bool {
	return g.Milestone == nil || g.Milestone.Status == domain.MilestoneActive
}

// rebuild recomputes the groups and the rows they flatten to. The Done groups
// all fold behind a single section row, which gathers every one of them: a
// mature plan is one Done line, not a wall of them. That section goes last of
// all, whatever the plan's order says — the work still in flight is what the
// board is read for, so finished milestones sitting in the middle of the plan
// are drawn beneath the ones that are not, rather than splitting them. The
// section starts collapsed and remembers its state like any group; expanding it
// reveals the Done milestones, in plan order, behaving as usual.
//
// The slices in flight are gathered first and take the first rows of all, the
// Active section's own: a plan with none — or a window with no room to draw the
// section in — takes no rows for it and behaves exactly as it did before there
// was one. They are rows of this same board and nothing apart from it, even
// though they are drawn in a panel of their own, so the cursor runs from the
// section straight on into the plan; see active.go.
func (b *Board) rebuild() {
	b.groups, b.blocked, b.byID = nil, nil, nil
	if b.project != nil {
		b.groups = b.project.Groups()
		b.blocked = blockedSlices(b.project.Slices)
		b.byID = domain.SlicesByID(b.project.Slices)
	}
	b.rows, b.active = nil, b.activeSlices()
	for i := range b.activeRowCount() {
		b.rows = append(b.rows, row{kind: rowActive, group: -1, slice: i})
	}
	for i, g := range b.groups {
		if !doneGroup(g) {
			b.appendGroup(i)
		}
	}
	if slices.ContainsFunc(b.groups, doneGroup) {
		if _, ok := b.expanded[doneSectionKey]; !ok {
			b.expanded[doneSectionKey] = false
		}
		b.rows = append(b.rows, row{kind: rowSection, group: -1})
		if b.expanded[doneSectionKey] {
			for j, d := range b.groups {
				if doneGroup(d) {
					b.appendGroup(j)
				}
			}
		}
	}

	if b.cursor >= len(b.rows) {
		b.cursor = len(b.rows) - 1
	}
	if b.cursor < 0 {
		b.cursor = 0
	}
}

// appendGroup flattens one group onto the rows: its own line, then its slices
// if it is expanded. A group not seen before starts at its default fold state.
//
// The blocked slices of a group sink to the bottom of it, keeping the plan's
// order among themselves: what can be worked now is what the milestone is read
// for, and a slice waiting on another is not it. That is a fact about the rows
// this board draws and nothing else — the plan in Notion is untouched, so
// next-slice and the plan order it reads hand work out exactly as they did.
func (b *Board) appendGroup(i int) {
	g := b.groups[i]
	key := groupKey(g)
	if _, ok := b.expanded[key]; !ok {
		b.expanded[key] = defaultExpanded(g)
	}
	b.rows = append(b.rows, row{kind: rowMilestone, group: i})
	if !b.expanded[key] {
		return
	}
	hide := b.hideDone && !doneGroup(g)
	var sunk []row
	for j, s := range g.Slices {
		if hide && s.Status == domain.SliceDone {
			continue
		}
		r := row{kind: rowSlice, group: i, slice: j}
		if len(b.Blockers(s)) > 0 {
			sunk = append(sunk, r)
			continue
		}
		b.rows = append(b.rows, r)
	}
	b.rows = append(b.rows, sunk...)
}

// hiddenDone is how many of a group's slices the hide-done toggle is keeping
// off the board, which is what the milestone's cue reports. A collapsed group
// shows no slices to begin with, so nothing of it is hidden by this toggle.
func (b Board) hiddenDone(g domain.Group) int {
	if !b.hideDone || doneGroup(g) || !b.expanded[groupKey(g)] {
		return 0
	}
	n := 0
	for _, s := range g.Slices {
		if s.Status == domain.SliceDone {
			n++
		}
	}
	return n
}

// Update handles the board's own keys — the ones that move the cursor. The
// rest reach the root model, which never passes them on.
func (b *Board) Update(msg tea.Msg) tea.Cmd {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch {
	case key.Matches(press, b.keys.Up):
		b.move(-1)
	case key.Matches(press, b.keys.Down):
		b.move(1)
	case key.Matches(press, b.keys.Toggle):
		b.toggle()
	case key.Matches(press, b.keys.HideDone):
		b.toggleHideDone()
	}
	return nil
}

// move steps the cursor, stopping at either end rather than wrapping. Leaving
// the row dismisses the confirmation anchored to it.
func (b *Board) move(delta int) {
	next := b.cursor + delta
	if next < 0 || next >= len(b.rows) {
		return
	}
	b.cursor = next
	b.ClearConfirm()
}

// toggle expands or collapses the group the cursor is in — or, on the Done
// section's own row, the section. Collapsing from a slice row would leave the
// cursor on a line that no longer exists, so the cursor moves to the group's
// own row either way.
func (b *Board) toggle() {
	if len(b.rows) == 0 {
		return
	}
	// Folding moves the cursor to the group's own row, which is not the row the
	// confirmation was anchored to.
	b.ClearConfirm()
	r := b.rows[b.cursor]
	if r.kind == rowActive {
		// The Active section folds nothing: it is a list of slices, and a slice
		// row has never folded.
		return
	}
	if r.kind == rowSection {
		b.expanded[doneSectionKey] = !b.expanded[doneSectionKey]
		b.rebuild()
		b.cursorTo(func(r row) bool { return r.kind == rowSection })
		return
	}
	g := r.group
	key := groupKey(b.groups[g])
	b.expanded[key] = !b.expanded[key]
	b.rebuild()
	b.cursorTo(func(r row) bool { return r.kind == rowMilestone && r.group == g })
}

// toggleHideDone flips the board-wide hide-done bit. The row the cursor was on
// may be one of the ones that just went, so the cursor is put back on it if it
// survived and otherwise falls back to its milestone's own row, which always
// does: it must never be left on a row that is no longer drawn.
func (b *Board) toggleHideDone() {
	b.ClearConfirm()
	if len(b.rows) == 0 {
		b.hideDone = !b.hideDone
		b.rebuild()
		return
	}
	was := b.rows[b.cursor]
	b.hideDone = !b.hideDone
	b.rebuild()
	if b.cursorTo(func(r row) bool { return r == was }) {
		return
	}
	b.cursorTo(func(r row) bool { return r.kind == rowMilestone && r.group == was.group })
}

// cursorTo moves the cursor to the first row match picks out, reporting whether
// there was one.
func (b *Board) cursorTo(match func(row) bool) bool {
	for i, r := range b.rows {
		if match(r) {
			b.cursor = i
			return true
		}
	}
	return false
}

// SelectedSlice is the slice under the cursor, if the cursor is on one. The
// keys reserved above act on it once they do something.
//
// An entry of the Active section is one of them: it is the same page as the row
// further down the plan, drawn a second time where the work in flight is
// gathered, so everything a slice row answers to it answers to as well.
func (b Board) SelectedSlice() (domain.Slice, bool) {
	if s, ok := b.SelectedActive(); ok {
		return s, true
	}
	if b.cursor >= len(b.rows) {
		return domain.Slice{}, false
	}
	r := b.rows[b.cursor]
	if r.kind != rowSlice {
		return domain.Slice{}, false
	}
	return b.groups[r.group].Slices[r.slice], true
}

// Blockers is the slices s is waiting on that are not Done, in the order s
// names them, and nothing at all for a slice that waits on none — which is
// every slice of a project whose table has no dependency column. It answers
// off the index built with the plan, so the launch key and the chip on the row
// say the same thing about the same slice.
func (b Board) Blockers(s domain.Slice) []domain.Slice {
	return b.blocked[domain.NormaliseID(s.ID)]
}

// BlockedBy names each slice s is waiting on the way the board files it — the
// milestone it is under, then the slice itself — which is where the user would
// go to find it, and nothing at all for a slice waiting on none. The
// milestone's inline numbering goes the way the title column drops it, so the
// two names read as one reference rather than three parts.
//
// It is the status line's reading of what the mark on the row can only say
// there is; see [App.blockedIndicator].
func (b Board) BlockedBy(s domain.Slice) []string {
	blockers := b.Blockers(s)
	if len(blockers) == 0 {
		return nil
	}
	refs := make([]string, len(blockers))
	for i, blocker := range blockers {
		refs[i] = b.groupTitleOf(blocker) + ": " + blocker.Name
	}
	return refs
}

// groupTitleOf is the title of the group the board draws s in: its milestone's,
// or the Unassigned group's for a slice filed under a milestone the plan does
// not hold — which is exactly where such a slice is drawn.
func (b Board) groupTitleOf(s domain.Slice) string {
	for _, g := range b.groups {
		if g.Milestone != nil && g.Milestone.ID == s.MilestoneID {
			return groupTitle(g)
		}
	}
	return domain.UnassignedName
}

// blockedSlices indexes a plan's blocked slices by ID: those waiting on slices
// that are not Done, mapped to what they wait on. The whole plan is the index a
// dependency is looked up in, since every slice a dependency can name is a row
// of this board; one it does not hold is a page the project cannot see, which
// domain.Blockers passes over rather than counting as unfinished.
func blockedSlices(slices []domain.Slice) map[string][]domain.Slice {
	byID := domain.SlicesByID(slices)
	blocked := map[string][]domain.Slice{}
	for _, s := range slices {
		if blocking, _ := domain.Blockers(s, byID); len(blocking) > 0 {
			blocked[domain.NormaliseID(s.ID)] = blocking
		}
	}
	return blocked
}

// SelectedMilestone is the milestone under the cursor, if the cursor is on a
// group's own row and that group is a real milestone: the implicit Unassigned
// group is not a page, so nothing can be filed under it.
func (b Board) SelectedMilestone() (domain.Milestone, bool) {
	if b.cursor >= len(b.rows) {
		return domain.Milestone{}, false
	}
	r := b.rows[b.cursor]
	if r.kind != rowMilestone {
		return domain.Milestone{}, false
	}
	m := b.groups[r.group].Milestone
	if m == nil {
		return domain.Milestone{}, false
	}
	return *m, true
}

// boardLayout is the column geometry one render shares across all its rows:
// the widths of the plan-number column and of the title, count and pill cells,
// each sized to the widest of its kind so the cells line up vertically.
type boardLayout struct {
	num, title, count, pill int
}

// layout measures the groups into the columns the rows align to.
func (b Board) layout() boardLayout {
	var l boardLayout
	for _, g := range b.groups {
		l.num = max(l.num, len(planNumber(g)))
		l.title = max(l.title, lipgloss.Width(groupTitle(g)))
		p := g.Progress()
		l.count = max(l.count, len(fmt.Sprintf("%d/%d", p.Done, p.Total)))
		if g.Milestone != nil {
			l.pill = max(l.pill, lipgloss.Width(string(g.Milestone.Status))+2)
		}
	}
	return l
}

// planPrefix is the numbering a milestone name carries in Notion ("M10: …"),
// which the board strips: the number is drawn as its own column instead.
var planPrefix = regexp.MustCompile(`^M\d+:\s*`)

// planNumber is the milestone's plan number as the number column shows it: its
// place among the Milestone column's options, which counts from zero, so the
// first milestone of the plan is milestone 1. The Unassigned group is no
// milestone of the plan and carries no number.
func planNumber(g domain.Group) string {
	if g.Milestone == nil {
		return ""
	}
	return strconv.FormatFloat(g.Milestone.Order+1, 'f', -1, 64)
}

// groupTitle is the group's name as the title column shows it: the inline
// numbering stripped, since the number column already carries it.
func groupTitle(g domain.Group) string {
	return planPrefix.ReplaceAllString(g.Name(), "")
}

// View renders the plan. The Active section's entries are rows of this board
// too, but they are drawn in a panel of their own above it — see
// [Board.ActiveLines] — so everything measured from here is measured over the
// plan's rows alone.
func (b Board) View() string {
	if len(b.groups) == 0 {
		return b.styles.Faint.Render("No milestones yet.")
	}
	var lines []string
	for _, rl := range b.rowLines() {
		lines = append(lines, rl...)
	}
	return strings.Join(lines, "\n")
}

// rowLines is every row of the plan as the lines it is drawn on, in board
// order. A row that fits is one line and a wrapped one several, which is what
// the cursor's position in the plan is measured from as well as what View
// joins.
//
// The Active section's rows are not among them: they lead the board's rows and
// are drawn in their own panel, so the plan's lines start at the row after the
// last of them.
func (b Board) rowLines() [][]string {
	l := b.layout()
	rows := b.rows[b.activeRowCount():]
	lines := make([][]string, len(rows))
	for i, r := range rows {
		lines[i] = b.renderRow(i+b.activeRowCount(), r, l)
	}
	return lines
}

// CursorSpan is where the row under the cursor sits in the drawn plan: the line
// it starts on, and how many lines it takes. Selection is per row, so a wrapped
// row is brought on screen whole.
//
// A cursor in the Active section is in the other panel entirely and has nothing
// here to bring on screen, which is what a height of zero says; the section
// scrolls itself — see [App.syncActive].
func (b Board) CursorSpan() (top, height int) {
	n := b.activeRowCount()
	if b.cursor < n {
		return 0, 0
	}
	for i, lines := range b.rowLines() {
		if i+n == b.cursor {
			return top, len(lines)
		}
		top += len(lines)
	}
	return top, 1
}

// RowAtLine is the row drawn on a line of the plan, counted from the first line
// of the whole render, and whether that line has a row on it at all: the
// mouse's way back from a line of the window to the row it points at, since a
// wrapped row takes more than one line and nothing below the last row is any
// row's. The row it names is the board's own index, the section's entries
// counted, since that is what the cursor is moved by.
func (b Board) RowAtLine(line int) (int, bool) {
	if line < 0 {
		return 0, false
	}
	at := 0
	for i, lines := range b.rowLines() {
		at += len(lines)
		if line < at {
			return i + b.activeRowCount(), true
		}
	}
	return 0, false
}

// SelectRow puts the cursor on a row, the way the movement keys do — a row that
// is not there is no move at all.
func (b *Board) SelectRow(i int) { b.move(i - b.cursor) }

// CursorToVisible keeps the cursor on a row the band of lines [top, top+height)
// draws whole, moving it to the nearest such row when scrolling has left it
// off screen. It is what makes the wheel and the layout's own scrolling agree:
// the layout brings the cursor's row back on screen whenever it re-syncs, so a
// cursor left behind by a scroll would drag the board back where it was.
//
// A band too short for the row it lands on has no whole row to offer, so the
// cursor goes to whichever row the top line belongs to and the re-sync scrolls
// to suit it.
//
// A cursor in the Active section is left where it is: that panel does not
// scroll with the plan, so nothing has moved out from under it and there is no
// re-sync for it to fight.
func (b *Board) CursorToVisible(top, height int) {
	n := b.activeRowCount()
	if height <= 0 || len(b.rows) == 0 || b.cursor < n {
		return
	}
	first, last, at := -1, -1, 0
	for i, lines := range b.rowLines() {
		if at >= top && at+len(lines) <= top+height {
			if first < 0 {
				first = i + n
			}
			last = i + n
		}
		at += len(lines)
	}
	if first < 0 {
		if i, ok := b.RowAtLine(top); ok {
			b.SelectRow(i)
		}
		return
	}
	switch {
	case b.cursor < first:
		b.SelectRow(first)
	case b.cursor > last:
		b.SelectRow(last)
	}
}

// LinkAt is the URL of the hyperlink drawn at a cell of the board — the PR
// chip, the only one there is — and whether that cell carries one. With mouse
// reporting on, the click that would have opened it belongs to the app, so the
// app has to know what sits under it; see [App.boardClick].
func (b Board) LinkAt(line, col int) (string, bool) {
	at := 0
	for _, lines := range b.rowLines() {
		if line < at+len(lines) {
			if line < at {
				return "", false
			}
			return hyperlinkAt(lines[line-at], col)
		}
		at += len(lines)
	}
	return "", false
}

// hyperlinkAt is the URL of the OSC 8 hyperlink covering a cell of one rendered
// line, walking the line sequence by sequence so the escapes cost the count no
// columns. A link left open at the end of the line runs to that end.
func hyperlinkAt(line string, col int) (string, bool) {
	var (
		url   string
		start int
		width int
		state byte
	)
	for len(line) > 0 {
		seq, w, n, next := xansi.DecodeSequence(line, state, nil)
		if u, ok := hyperlinkTarget(seq); ok {
			if u == "" {
				if url != "" && col >= start && col < width {
					return url, true
				}
				url = ""
			} else {
				url, start = u, width
			}
		}
		width += w
		line, state = line[n:], next
	}
	if url != "" && col >= start {
		return url, true
	}
	return "", false
}

// hyperlinkTarget is the URL an OSC 8 sequence sets, and whether the sequence
// is one at all. The closing sequence sets none, which is how a link ends.
func hyperlinkTarget(seq string) (string, bool) {
	const open = "\x1b]8;"
	if !strings.HasPrefix(seq, open) {
		return "", false
	}
	body := strings.TrimSuffix(strings.TrimSuffix(seq[len(open):], "\a"), "\x1b\\")
	// The parameters come first and this app sets none, so the URL is whatever
	// follows them.
	_, url, _ := strings.Cut(body, ";")
	return url, true
}

// renderRow draws one row, with the cursor marker in front of it. The row
// under the cursor is drawn plain and handed to finishRow for its background
// fill, so the marker takes the fill's colour like everything else on it.
// Slice rows skip the number column and indent one step further, so they sit
// consistently beneath their milestone's title.
func (b Board) renderRow(i int, r row, l boardLayout) []string {
	selected := i == b.cursor
	marker := "  "
	if selected {
		marker = "❯ "
	}
	if r.kind == rowSection {
		lines := b.renderDoneSection(marker, selected, l)
		// The section closes the board off from the plan above it, so it is set
		// apart by a blank line — which belongs to the section's own row, since
		// a line of the board that is no row's is a line the cursor and the
		// mouse cannot account for. There is nothing to be set apart from when
		// the section is the whole plan — the Active panel above is a box of its
		// own, and no row of this one.
		if i > b.activeRowCount() {
			lines = append([]string{""}, lines...)
		}
		return lines
	}
	if r.kind == rowMilestone {
		return b.renderMilestone(marker, b.groups[r.group], selected, l)
	}
	indent := strings.Repeat(" ", l.num+1)
	return b.renderSlice(marker+indent, b.groups[r.group].Slices[r.slice], selected)
}

// blanks is s's width in spaces: the left edge of a row's continuation lines,
// where whatever the head drew there has nothing to say a second time.
func blanks(s string) string {
	return strings.Repeat(" ", lipgloss.Width(s))
}

// paint styles s, unless the row it is part of is selected: a selected row is
// drawn plain, because its parts' own colours would each reset the selected
// fill's background and cut holes in it.
func paint(selected bool, st lipgloss.Style, s string) string {
	return painter(selected, st).Render(s)
}

// painter is the style paint would use, for text that is styled a piece at a
// time rather than in one go — a wrapped name, whose lines are each rendered
// on their own.
func painter(selected bool, st lipgloss.Style) lipgloss.Style {
	if selected {
		return lipgloss.NewStyle()
	}
	return st
}

// finishRow is the last step of a row: the selected row's background fill, run
// out to the board's width on every one of the row's lines, so the highlight is
// the whole row rather than its text — and over that, the prompt waiting on the
// row, or the inline confirmation when one is anchored to it. Both of those go
// on the row's last line, which is the one with room for them: the lines above
// it only exist because the row filled them.
func (b Board) finishRow(selected bool, lines []string) []string {
	if !selected {
		return lines
	}
	st := b.styles.SelectedRow
	if b.width > 0 {
		st = st.Width(b.width)
	}
	filled := make([]string, len(lines))
	for i, line := range lines {
		filled[i] = st.Render(line)
	}
	last := len(filled) - 1
	filled[last] = b.overlayAnchored(filled[last], lipgloss.Width(lines[last]))
	return filled
}

// overlayAnchored lays whatever is anchored to the row the cursor is on over
// that row's last line — the prompt waiting to be answered, or the inline
// confirmation when one is up — and hands the line back untouched when there is
// neither. It is the one place that choice is made, because the Active
// section's entries are rows the same keys act on and so answer the same
// anchoring: see [Board.renderActive]. line is the row already filled and run
// out to the board's width, raw the width of its content before that fill.
func (b Board) overlayAnchored(line string, raw int) string {
	switch {
	case b.prompt != nil:
		return b.overlayChip(line, raw, b.promptChip(), b.styles.PromptFade)
	case b.confirmText != "":
		chip, fade := b.styles.confirmStyles(b.confirmSev)
		return b.overlayChip(line, raw, chip.Render(b.confirmText), fade)
	}
	return line
}

// promptChip is the open prompt as one chip: its choices side by side, the
// focused one filled with the accent and the rest quiet, so the answer enter
// would give is the one that stands out.
func (b Board) promptChip() string {
	var chip strings.Builder
	for i, option := range b.prompt.options {
		st := b.styles.PromptOption
		if i == b.prompt.cursor {
			st = b.styles.PromptFocused
		}
		chip.WriteString(st.Render(option))
	}
	return chip.String()
}

// confirmFadeWidth is the dithered edge a chip carries where it overlaps the
// row's content, in cells.
const confirmFadeWidth = 2

// confirmFadeRunes are the edge's cells, reading toward the chip: lighter
// shade first, so the chip appears to condense out of the row under it.
const confirmFadeRunes = "░▒"

// overlayChip lays an already rendered chip — an inline confirmation, or the
// prompt waiting on the row — over the selected row's filled line, from its
// right edge. line is the row already run out to the board's width and raw the
// width of its content before the fill, which is what says whether the chip
// lands on content or on empty fill: on content it carries the dithered fade
// on its left edge, so it reads as sliding over the row.
func (b Board) overlayChip(line string, raw int, chip string, fadeStyle lipgloss.Style) string {
	if b.width <= 0 {
		// Unmeasured: nothing to anchor to, so the chip simply follows the row.
		return line + " " + chip
	}
	chipWidth := lipgloss.Width(chip)
	if chipWidth >= b.width {
		return fit(chip, b.width)
	}
	start := b.width - chipWidth
	if raw+confirmFadeWidth <= start {
		// The chip lands on the fill with room to spare, so there is nothing to
		// fade over.
		return fit(line, start) + chip
	}
	cells := min(confirmFadeWidth, start)
	fade := fadeStyle.Render(string([]rune(confirmFadeRunes)[confirmFadeWidth-cells:]))
	// fit reads a width of zero as "unmeasured, leave it whole", so a chip and
	// fade that take the whole board keep nothing of the row at all.
	left := ""
	if keep := start - cells; keep > 0 {
		left = fit(line, keep)
	}
	return left + fade + chip
}

// rowName is a row's name as fitRow lays it out: the raw text, the style each
// of its lines is drawn in, and the spaces that pad it out to its column.
//
// The style is held rather than applied because a name that wraps is rendered a
// line at a time — one render over the whole name would be cut in half at the
// break, leaving the second half unstyled. The padding only survives while the
// chips it aligns stay on the name's own line; once they wrap there is no
// column left to align to.
type rowName struct {
	text  string
	style lipgloss.Style
	pad   int
}

// String is the name as a row that fits draws it: styled, and padded out.
func (n rowName) String() string {
	return n.style.Render(n.text) + strings.Repeat(" ", n.pad)
}

// words is the name broken where fitRow may put a line end: at its spaces, and
// inside any single word too long for a line of its own.
func (n rowName) words(limit int) []string {
	var out []string
	for _, w := range strings.Fields(n.text) {
		out = append(out, strings.Split(xansi.Hardwrap(w, limit, false), "\n")...)
	}
	return out
}

// minRowName is the narrowest a name's column may be and still be worth
// flowing text into. Below it the board is narrower than its own left edge and
// there is nothing sensible to wrap, so the row is cut to the width instead.
const minRowName = 4

// fitRow assembles one row from a head that always draws, a name, and chips in
// the order they are drawn. A row too wide for the board wraps: the name flows
// word by word onto continuation lines, each carrying cont — the head's width
// again — so the row's left edge runs down all of them, and the chips follow
// the name as whole pieces. Nothing is dropped and nothing is truncated.
func fitRow(width int, head, cont string, name rowName, chips ...string) []string {
	whole := joinRow(head, name.String(), chips)
	if width <= 0 || lipgloss.Width(whole) <= width {
		return []string{whole}
	}
	avail := width - lipgloss.Width(head) - 1
	if avail < minRowName {
		return []string{fit(whole, width)}
	}

	var (
		lines []string
		body  strings.Builder
		used  int
	)
	// A line's trailing spaces are the name's padding with nothing left on the
	// line to align, so they go with the break.
	flush := func() {
		prefix := cont
		if len(lines) == 0 {
			prefix = head
		}
		lines = append(lines, prefix+" "+strings.TrimRight(body.String(), " "))
		body.Reset()
		used = 0
	}
	add := func(s string, w int) {
		if used > 0 && used+1+w > avail {
			flush()
		}
		if used > 0 {
			body.WriteString(" ")
			used++
		}
		body.WriteString(s)
		used += w
	}
	for _, word := range name.words(avail) {
		add(name.style.Render(word), lipgloss.Width(word))
	}
	if used+name.pad <= avail {
		body.WriteString(strings.Repeat(" ", name.pad))
		used += name.pad
	}
	for _, chip := range chips {
		add(fit(chip, avail), min(lipgloss.Width(chip), avail))
	}
	if used > 0 {
		flush()
	}
	return lines
}

// joinRow is one row's parts as a line, space separated.
func joinRow(head, name string, chips []string) string {
	return strings.Join(append([]string{head, name}, chips...), " ")
}

// renderMilestone draws a group's own line: the plan number, the fold
// indicator, its title, how many of its slices are done, and its status pill.
// The title cell is padded to the widest title and the count and pill each
// right-align in a cell of their own, so the columns run straight down the
// board. Where the hide-done toggle is keeping slices of it off the board, a
// faint cue says how many. A board too narrow for all of that wraps the row
// rather than dropping any of it, and the columns give way with the break —
// there is nothing left on the line to align to.
func (b Board) renderMilestone(marker string, g domain.Group, selected bool, l boardLayout) []string {
	fold := "▸"
	if b.expanded[groupKey(g)] {
		fold = "▾"
	}
	head := marker
	if l.num > 0 {
		head += paint(selected, b.styles.Faint, fmt.Sprintf("%*s", l.num, planNumber(g))) + " "
	}
	head += fold
	p := g.Progress()
	count := fmt.Sprintf("%*s", l.count, fmt.Sprintf("%d/%d", p.Done, p.Total))
	chips := []string{paint(selected, b.styles.Faint, count)}
	if g.Milestone != nil {
		pill := b.milestoneChip(g.Milestone.Status, selected)
		if pad := l.pill - lipgloss.Width(pill); pad > 0 {
			pill = strings.Repeat(" ", pad) + pill
		}
		chips = append(chips, pill)
	}
	if n := b.hiddenDone(g); n > 0 {
		chips = append(chips, paint(selected, b.styles.Faint, fmt.Sprintf("· %d done hidden", n)))
	}
	name := rowName{
		text:  groupTitle(g),
		style: painter(selected, b.styles.Milestone),
		pad:   max(0, l.title-lipgloss.Width(groupTitle(g))),
	}
	return b.finishRow(selected, fitRow(b.width, head, blanks(head), name, chips...))
}

// renderDoneSection draws the row the Done milestones fold behind: the fold
// indicator, a Done title in the title column, and a faint aggregate of what it
// hides — how many milestones, and their slices' combined count. It takes no
// number cell at all rather than a blank one: the section is not part of the
// plan's numbering, so it sits out at the left edge the numbers start from,
// which is what says it is not another milestone of the plan.
func (b Board) renderDoneSection(marker string, selected bool, l boardLayout) []string {
	fold := "▸"
	if b.expanded[doneSectionKey] {
		fold = "▾"
	}
	head := marker + fold
	milestones := 0
	var p domain.Progress
	for _, g := range b.groups {
		if !doneGroup(g) {
			continue
		}
		milestones++
		gp := g.Progress()
		p.Done += gp.Done
		p.Total += gp.Total
	}
	noun := "milestones"
	if milestones == 1 {
		noun = "milestone"
	}
	agg := fmt.Sprintf("%d %s · %d/%d", milestones, noun, p.Done, p.Total)
	const title = "Done"
	name := rowName{
		text:  title,
		style: painter(selected, b.styles.Milestone),
		pad:   max(0, l.title-lipgloss.Width(title)),
	}
	return b.finishRow(selected,
		fitRow(b.width, head, blanks(head), name, paint(selected, b.styles.Faint, agg)))
}

// renderSlice draws one slice: its status chip, its name, its marker — the star
// of an agent live on it, or the mark of one waiting on unfinished work, see
// [Board.marker] — whether it has been handed back for review, who holds it,
// and the pull request it produced.
//
// The PR chip comes last of the chips, so it is the first of them to give way
// as the board narrows: the slice's own state is worth more of a cramped row
// than a link out of the app. The review chip comes first of the rest, since it
// says the row is done being worked.
//
// A blocked slice is drawn in the muted text a blocked row takes, so the whole
// row recedes rather than only carrying a mark: what can be worked is what the
// board is read for, and a row that cannot reads as such at a glance.
//
// A row too wide for the board wraps rather than shedding any of its chips,
// and the status chip carries on down the wrapped lines as a bare strip of its
// colour, so the status reads as a band beside the whole row.
func (b Board) renderSlice(head string, s domain.Slice, selected bool) []string {
	var chips []string
	if mark, ok := b.marker(s, selected); ok {
		chips = append(chips, mark)
	}
	if s.HandedBack() {
		chips = append(chips, b.reviewChip(selected))
	}
	if s.AssigneeName != "" {
		chips = append(chips, paint(selected, b.styles.Assignee, "@"+s.AssigneeName))
	}
	if s.PRURL != "" {
		chips = append(chips, b.prChip(s.PRURL, selected))
	}
	style := lipgloss.NewStyle()
	if len(b.Blockers(s)) > 0 {
		style = b.styles.Blocked
	}
	name := rowName{text: s.Name, style: painter(selected, style)}
	return b.finishRow(selected, fitRow(b.width,
		head+b.sliceChip(s.Status, selected),
		blanks(head)+b.sliceStrip(s.Status, selected),
		name, chips...))
}

// sliceStatus is a slice status as the board draws it: a glyph whose shape says
// how far along the slice is — ○ not started, ◐ under way, ✓ finished — and the
// chip style it is badged with. An unknown status, one Notion has grown that
// this build does not know, keeps a glyph and a chip of its own rather than
// nothing at all, so the column stays aligned.
func (b Board) sliceStatus(s domain.SliceStatus) (string, lipgloss.Style) {
	switch s {
	case domain.SliceTodo:
		return "○", b.styles.StatusTodo
	case domain.SliceClaimed:
		return "◐", b.styles.StatusClaimed
	case domain.SliceDone:
		return "✓", b.styles.StatusDone
	}
	return "·", b.styles.StatusUnknown
}

// sliceChip is the badge a slice row leads with: its status glyph on the
// status's own background. On a selected row the chip is its bare padded glyph,
// for the same reason as paint.
func (b Board) sliceChip(s domain.SliceStatus, selected bool) string {
	glyph, st := b.sliceStatus(s)
	if selected {
		return " " + glyph + " "
	}
	return st.Render(glyph)
}

// sliceStrip is that same cell on the row's continuation lines, with no glyph
// in it: the colour carries on down the row, so a wrapped slice is one band of
// status rather than a mark on its first line. A selected row is drawn plain
// and filled instead, so the strip gives way to the selection.
func (b Board) sliceStrip(s domain.SliceStatus, selected bool) string {
	if selected {
		return "   "
	}
	_, st := b.sliceStatus(s)
	return st.Render(" ")
}

// blockedGlyph is the mark a slice waiting on unfinished slices carries in the
// row's marker cell. It is a glyph and not a word: it names no dependency —
// the cell is one column, and the slices it waits on are rows of this same
// board — and it has to read on a selected row, which is drawn without any
// chip's colour, so the shape carries it alone.
const blockedGlyph = "⊘"

// marker is the one cell a slice row marks its own state in, and whether it has
// anything to mark at all: the star of an agent live on the slice, or the mark
// of one waiting on unfinished work. The two share the cell because they cannot
// both be true of a slice — a blocked slice is not one the launch key will
// start an agent on — and where a plan has somehow made them both true the star
// wins, since an agent running is the more urgent of the two and the row's
// muted text goes on saying the slice is blocked.
func (b Board) marker(s domain.Slice, selected bool) (string, bool) {
	if star, live := b.star(s.ID, selected); live {
		return star, true
	}
	if len(b.Blockers(s)) > 0 {
		return paint(selected, b.styles.Blocked, blockedGlyph), true
	}
	return "", false
}

// reviewChip is the badge a slice handed back on a branch carries: work an
// agent has finished and nobody has reviewed, which would otherwise read as
// just another slice in progress. It names no branch — the row has no space for
// one, and the approve key is what acts on it — and the glyph leads for the
// reason the blocked mark is a glyph at all: a selected row is drawn without
// the chip's colour, and the shape has to carry it alone.
func (b Board) reviewChip(selected bool) string {
	return paint(selected, b.styles.Review, "↑ review")
}

// prNumberPath is the number a pull request URL ends its path with — the 71 of
// ".../pull/71" — which is what the PR chip is named after.
var prNumberPath = regexp.MustCompile(`/(\d+)/?$`)

// prLabel is the chip's text: the pull request's number, "#71". A URL that ends
// in no number — a shortened link, a review page, something that is not a forge
// at all — falls back to the bare word, which is what the chip said before it
// carried a number and still says everything it has to.
func prLabel(url string) string {
	path, _, _ := strings.Cut(url, "?")
	path, _, _ = strings.Cut(path, "#")
	if m := prNumberPath.FindStringSubmatch(path); m != nil {
		return "#" + m[1]
	}
	return "PR"
}

// prChip is the badge a slice with a pull request carries, wrapped in an OSC 8
// hyperlink to it: terminals that speak them open the PR on a click, and those
// that do not simply draw the text, since the escape takes no cells. The click
// is the terminal's — and inside tmux the hyperlink bindings the agent layer
// installs — except while nat holds the mouse for the agent terminal beside the
// board, when the app reads the link out of the drawn row and opens it itself;
// see [Board.LinkAt].
func (b Board) prChip(url string, selected bool) string {
	return xansi.SetHyperlink(url) +
		paint(selected, b.styles.PR, prLabel(url)) +
		xansi.ResetHyperlink()
}

// milestoneChip is the badge for a milestone's status word, shaped like
// sliceChip: unknown statuses take the Queued grey rather than nothing.
func (b Board) milestoneChip(s domain.MilestoneStatus, selected bool) string {
	st := b.styles.MilestoneQueued
	switch s {
	case domain.MilestoneActive:
		st = b.styles.MilestoneActive
	case domain.MilestoneDone:
		st = b.styles.MilestoneDone
	}
	if selected {
		return " " + string(s) + " "
	}
	return st.Render(string(s))
}
