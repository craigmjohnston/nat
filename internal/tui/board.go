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
	Up     key.Binding
	Down   key.Binding
	Toggle key.Binding

	Add    key.Binding
	Edit   key.Binding
	Move   key.Binding
	Delete key.Binding
	Queue  key.Binding

	Launch key.Binding
	Attach key.Binding

	NewProject    key.Binding
	SwitchProject key.Binding
}

// defaultBoardKeyMap returns the bindings the board runs with.
func defaultBoardKeyMap() boardKeyMap {
	return boardKeyMap{
		Up:     key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:   key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Toggle: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand/collapse")),

		Add:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add slice")),
		Edit:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit slice")),
		Move:   key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "move slice")),
		Delete: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete slice")),
		Queue:  key.NewBinding(key.WithKeys("Q"), key.WithHelp("Q", "advance milestone")),

		Launch: key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "launch agent")),
		Attach: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "show/hide agent")),

		NewProject:    key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "new project")),
		SwitchProject: key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "switch project")),
	}
}

// agents are the bindings that act on a slice's agent session.
func (k boardKeyMap) agents() []key.Binding {
	return []key.Binding{k.Launch, k.Attach}
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

// helpBindings are the board's bindings as the help screen lists them.
func (b Board) helpBindings() []key.Binding {
	bindings := []key.Binding{b.keys.Up, b.keys.Down, b.keys.Toggle}
	bindings = append(bindings, b.keys.writes()...)
	bindings = append(bindings, b.keys.agents()...)
	return append(bindings, b.keys.projects()...)
}

// rowKind tells the two kinds of line the cursor moves over apart.
type rowKind int

const (
	rowMilestone rowKind = iota
	rowSlice
)

// row is one selectable line of the board, addressing back into the groups it
// was flattened from. slice is meaningless for a rowMilestone.
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
	// live maps the ID of each slice with an agent running to the session it
	// runs in, so a slice with an agent on it can be marked.
	live map[string]string

	width int
}

// NewBoard returns an empty board, waiting for a project to be loaded into it.
func NewBoard(styles Styles) Board {
	return Board{styles: styles, keys: defaultBoardKeyMap(), expanded: map[string]bool{}}
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

// groupKey identifies a group across reloads. The implicit Unassigned group has
// no milestone and so no ID, which is a key no milestone can collide with.
func groupKey(g domain.Group) string {
	if g.Milestone == nil {
		return ""
	}
	return g.Milestone.ID
}

// defaultExpanded is how a group is shown before the user touches it: the work
// in flight is open, everything else is a one-line summary. Slices with no
// milestone are open too — they are stray, and worth seeing.
func defaultExpanded(g domain.Group) bool {
	return g.Milestone == nil || g.Milestone.Status == domain.MilestoneActive
}

// rebuild recomputes the groups and the rows they flatten to.
func (b *Board) rebuild() {
	b.groups = nil
	if b.project != nil {
		b.groups = b.project.Groups()
	}
	b.rows = nil
	for i, g := range b.groups {
		key := groupKey(g)
		if _, ok := b.expanded[key]; !ok {
			b.expanded[key] = defaultExpanded(g)
		}
		b.rows = append(b.rows, row{kind: rowMilestone, group: i})
		if !b.expanded[key] {
			continue
		}
		for j := range g.Slices {
			b.rows = append(b.rows, row{kind: rowSlice, group: i, slice: j})
		}
	}

	if b.cursor >= len(b.rows) {
		b.cursor = len(b.rows) - 1
	}
	if b.cursor < 0 {
		b.cursor = 0
	}
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
	}
	return nil
}

// move steps the cursor, stopping at either end rather than wrapping.
func (b *Board) move(delta int) {
	next := b.cursor + delta
	if next < 0 || next >= len(b.rows) {
		return
	}
	b.cursor = next
}

// toggle expands or collapses the group the cursor is in. Collapsing from a
// slice row would leave the cursor on a line that no longer exists, so the
// cursor moves to the group's own row either way.
func (b *Board) toggle() {
	if len(b.rows) == 0 {
		return
	}
	g := b.rows[b.cursor].group
	key := groupKey(b.groups[g])
	b.expanded[key] = !b.expanded[key]
	b.rebuild()
	for i, r := range b.rows {
		if r.kind == rowMilestone && r.group == g {
			b.cursor = i
			break
		}
	}
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
// out to the board's width so the highlight is the row rather than its text.
func (b Board) finishRow(selected bool, line string) string {
	if !selected {
		return line
	}
	st := b.styles.SelectedRow
	if b.width > 0 {
		st = st.Width(b.width)
	}
	return st.Render(line)
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
// board. The pill goes first as the board narrows, then the count.
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
	title := groupTitle(g)
	if pad := l.title - lipgloss.Width(title); pad > 0 {
		title += strings.Repeat(" ", pad)
	}
	name := paint(selected, b.styles.Milestone, title)
	return b.finishRow(selected, fitRow(b.width, head, name, chips...))
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
