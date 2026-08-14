package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	Queue  key.Binding

	Launch key.Binding
	Attach key.Binding
	Plan   key.Binding

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
		Queue:  key.NewBinding(key.WithKeys("Q"), key.WithHelp("Q", "advance milestone")),

		Launch: key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "launch agent")),
		Attach: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "show/hide agent")),
		Plan:   key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "planning agent")),

		NewProject:    key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "new project")),
		SwitchProject: key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "switch project")),
	}
}

// agents are the bindings that act on an agent session: a slice's, or the
// planning agent's.
func (k boardKeyMap) agents() []key.Binding {
	return []key.Binding{k.Launch, k.Attach, k.Plan}
}

// projects are the bindings that act on the plan the board is showing rather
// than anything in it.
func (k boardKeyMap) projects() []key.Binding {
	return []key.Binding{k.NewProject, k.SwitchProject}
}

// writes are the bindings the root model handles rather than the board.
func (k boardKeyMap) writes() []key.Binding {
	return []key.Binding{k.Add, k.Edit, k.Move, k.Delete, k.Queue}
}

// sliceHints are the hints row's bindings while the cursor is on a slice: the
// actions that act on it, in the order they read. The agent keys are what the
// tracker is for, so they survive a narrow row longest. The write keys drop
// the word "slice" their help carries — the hints only show on one, and the
// row has less room than the help screen.
func (b Board) sliceHints() []hint {
	k := b.keys
	return []hint{
		{shortHint(k.Edit, "edit"), 5},
		{shortHint(k.Move, "move"), 3},
		{shortHint(k.Delete, "delete"), 4},
		{k.Launch, 7},
		{k.Attach, 6},
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
// milestone: the actions that act on it or file under it.
func (b Board) milestoneHints() []hint {
	k := b.keys
	return []hint{
		{k.Add, 5},
		{k.Queue, 4},
		{k.Toggle, 3},
		b.doneHint(),
	}
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
)

// row is one selectable line of the board, addressing back into the groups it
// was flattened from. slice is meaningless for a rowMilestone, and a rowSection
// — the Done section's own line — addresses no group at all, so its group is -1
// rather than silently aliasing the first one.
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
		styles:   styles,
		keys:     defaultBoardKeyMap(),
		expanded: map[string]bool{},
		hideDone: true,
	}
}

// SetProject shows a freshly loaded plan. Groups the user has already expanded
// or collapsed keep that state across a refresh; new ones start at their
// default, and the cursor is clamped to whatever rows remain.
func (b *Board) SetProject(p *domain.Project) {
	b.project = p
	b.rebuild()
}

// SetWidth records the space the board has to draw in; rows longer than it lose
// their trailing chips and then have their name truncated rather than wrapping,
// so one slice stays one line.
func (b *Board) SetWidth(width int) { b.width = width }

// Cursor is the index of the row the user is on, which is also the line the
// row is drawn on: one row is one line. The layout scrolls to it, so that a
// plan taller than the window still shows where the user is.
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
// all fold behind a single section row, which sits where the first of them
// would have and gathers the rest up to it: a mature plan is one Done line, not
// a wall of them. The section starts collapsed and remembers its state like any
// group; expanding it reveals the Done milestones, which behave as usual.
func (b *Board) rebuild() {
	b.groups = nil
	if b.project != nil {
		b.groups = b.project.Groups()
	}
	b.rows = nil
	sectionEmitted := false
	for i, g := range b.groups {
		if !doneGroup(g) {
			b.appendGroup(i)
			continue
		}
		if sectionEmitted {
			continue
		}
		sectionEmitted = true
		if _, ok := b.expanded[doneSectionKey]; !ok {
			b.expanded[doneSectionKey] = false
		}
		b.rows = append(b.rows, row{kind: rowSection, group: -1})
		if !b.expanded[doneSectionKey] {
			continue
		}
		for j, d := range b.groups {
			if doneGroup(d) {
				b.appendGroup(j)
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
	for j, s := range g.Slices {
		if hide && s.Status == domain.SliceDone {
			continue
		}
		b.rows = append(b.rows, row{kind: rowSlice, group: i, slice: j})
	}
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
func (b Board) SelectedSlice() (domain.Slice, bool) {
	if b.cursor >= len(b.rows) {
		return domain.Slice{}, false
	}
	r := b.rows[b.cursor]
	if r.kind != rowSlice {
		return domain.Slice{}, false
	}
	return b.groups[r.group].Slices[r.slice], true
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

// planNumber is the milestone's plan number as the number column shows it —
// blank for the Unassigned group and for a milestone with no order.
func planNumber(g domain.Group) string {
	if g.Milestone == nil || g.Milestone.Order == 0 {
		return ""
	}
	return strconv.FormatFloat(g.Milestone.Order, 'f', -1, 64)
}

// groupTitle is the group's name as the title column shows it: the inline
// numbering stripped, since the number column already carries it.
func groupTitle(g domain.Group) string {
	return planPrefix.ReplaceAllString(g.Name(), "")
}

// View renders the board.
func (b Board) View() string {
	if len(b.groups) == 0 {
		return b.styles.Faint.Render("No milestones yet.")
	}
	l := b.layout()
	lines := make([]string, len(b.rows))
	for i, r := range b.rows {
		lines[i] = b.renderRow(i, r, l)
	}
	return strings.Join(lines, "\n")
}

// renderRow draws one line, with the cursor marker in front of it. The row
// under the cursor is drawn plain and handed to finishRow for its background
// fill, so the marker takes the fill's colour like everything else on it.
// Slice rows skip the number column and indent one step further, so they sit
// consistently beneath their milestone's title.
func (b Board) renderRow(i int, r row, l boardLayout) string {
	selected := i == b.cursor
	marker := "  "
	if selected {
		marker = "❯ "
	}
	if r.kind == rowSection {
		return b.renderDoneSection(marker, selected, l)
	}
	if r.kind == rowMilestone {
		return b.renderMilestone(marker, b.groups[r.group], selected, l)
	}
	indent := strings.Repeat(" ", l.num+1)
	return b.renderSlice(marker+indent, b.groups[r.group].Slices[r.slice], selected)
}

// paint styles s, unless the row it is part of is selected: a selected row is
// drawn plain, because its parts' own colours would each reset the selected
// fill's background and cut holes in it.
func paint(selected bool, st lipgloss.Style, s string) string {
	if selected {
		return s
	}
	return st.Render(s)
}

// finishRow is the last step of a row: the selected row's background fill, run
// out to the board's width so the highlight is the row rather than its text —
// and over that, the prompt waiting on the row, or the inline confirmation when
// one is anchored to it.
func (b Board) finishRow(selected bool, line string) string {
	if !selected {
		return line
	}
	raw := lipgloss.Width(line)
	st := b.styles.SelectedRow
	if b.width > 0 {
		st = st.Width(b.width)
	}
	filled := st.Render(line)
	if b.prompt != nil {
		return b.overlayChip(filled, raw, b.promptChip(), b.styles.PromptFade)
	}
	if b.confirmText == "" {
		return filled
	}
	chip, fade := b.styles.confirmStyles(b.confirmSev)
	return b.overlayChip(filled, raw, chip.Render(b.confirmText), fade)
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

// fitRow assembles one row from a head that always draws, a name, and chips in
// the order they are drawn. A row too wide for the board loses its chips from
// the tail — the last one drawn is the first to go — and only once none are left
// is the name itself truncated: the name and the head are what the row is for.
func fitRow(width int, head, name string, chips ...string) string {
	line := joinRow(head, name, chips)
	if width <= 0 {
		return line
	}
	for len(chips) > 0 && lipgloss.Width(line) > width {
		chips = chips[:len(chips)-1]
		line = joinRow(head, name, chips)
	}
	if lipgloss.Width(line) > width {
		line = lipgloss.NewStyle().MaxWidth(width).Render(line)
	}
	return line
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
// faint cue says how many. That cue goes first as the board narrows, then the
// pill, then the count.
func (b Board) renderMilestone(marker string, g domain.Group, selected bool, l boardLayout) string {
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
	title := groupTitle(g)
	if pad := l.title - lipgloss.Width(title); pad > 0 {
		title += strings.Repeat(" ", pad)
	}
	name := paint(selected, b.styles.Milestone, title)
	return b.finishRow(selected, fitRow(b.width, head, name, chips...))
}

// renderDoneSection draws the row the Done milestones fold behind: the fold
// indicator, a Done title in the title column, and a faint aggregate of what it
// hides — how many milestones, and their slices' combined count. Its number
// cell is blank: the section is not part of the plan's numbering.
func (b Board) renderDoneSection(marker string, selected bool, l boardLayout) string {
	fold := "▸"
	if b.expanded[doneSectionKey] {
		fold = "▾"
	}
	head := marker
	if l.num > 0 {
		head += strings.Repeat(" ", l.num) + " "
	}
	head += fold
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
	title := "Done"
	if pad := l.title - lipgloss.Width(title); pad > 0 {
		title += strings.Repeat(" ", pad)
	}
	name := paint(selected, b.styles.Milestone, title)
	return b.finishRow(selected, fitRow(b.width, head, name, paint(selected, b.styles.Faint, agg)))
}

// renderSlice draws one slice: its status chip, its name, whether an agent is
// live on it, who holds it, and whether it has a PR. The live marker is its own
// glyph rather than a status: a session is running or not, which is a different
// question from where the slice has got to.
//
// As the board narrows the PR marker goes first, then the assignee, and the live
// marker last of the three — a running agent is the most urgent thing about a
// row, and the status chip and name never go at all.
func (b Board) renderSlice(head string, s domain.Slice, selected bool) string {
	var chips []string
	if b.live[s.ID] != "" {
		chips = append(chips, paint(selected, b.styles.Live, "●"))
	}
	if s.AssigneeName != "" {
		chips = append(chips, paint(selected, b.styles.Assignee, "@"+s.AssigneeName))
	}
	if s.PRURL != "" {
		chips = append(chips, paint(selected, b.styles.PR, "PR"))
	}
	return b.finishRow(selected, fitRow(b.width, head+b.sliceChip(s.Status, selected), s.Name, chips...))
}

// sliceChip is the badge for a slice's status: its glyph on the status's own
// background. An unknown status — one Notion has grown that this build does not
// know — draws as an empty badge rather than nothing at all, so the column
// stays aligned. On a selected row the chip is its bare padded glyph, for the
// same reason as paint.
func (b Board) sliceChip(s domain.SliceStatus, selected bool) string {
	glyph, st := "·", b.styles.StatusUnknown
	switch s {
	case domain.SliceTodo:
		glyph, st = "○", b.styles.StatusTodo
	case domain.SliceClaimed:
		glyph, st = "◐", b.styles.StatusClaimed
	case domain.SliceDone:
		glyph, st = "●", b.styles.StatusDone
	}
	if selected {
		return " " + glyph + " "
	}
	return st.Render(glyph)
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
