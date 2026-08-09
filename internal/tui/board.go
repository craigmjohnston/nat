package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/notion-agent-tracker/internal/domain"
)

// boardKeyMap is the board's own bindings: navigation, the writes, plus the
// keys reserved for the mutations and agent launches that land in later
// milestones. The reserved ones are matched and swallowed so that pressing them
// does nothing rather than falling through to something else once more keys
// exist.
//
// Add and Edit are named here but handled by the root model: they need the
// Notion client and the project config, which the board has no business
// holding.
type boardKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Toggle key.Binding

	Add  key.Binding
	Edit key.Binding

	// Reserved, in the order they read in the help.
	Queue     key.Binding
	Milestone key.Binding
	Done      key.Binding
	Launch    key.Binding
	Attach    key.Binding
}

// defaultBoardKeyMap returns the bindings the board runs with.
func defaultBoardKeyMap() boardKeyMap {
	return boardKeyMap{
		Up:     key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:   key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Toggle: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand/collapse")),

		Add:  key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add slice")),
		Edit: key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit slice")),

		Queue:     key.NewBinding(key.WithKeys("Q"), key.WithHelp("Q", "queue work")),
		Milestone: key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "move milestone")),
		Done:      key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "mark done")),
		Launch:    key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "launch agent")),
		Attach:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "attach to tmux")),
	}
}

// reserved are the bindings that exist only to be swallowed for now.
func (k boardKeyMap) reserved() []key.Binding {
	return []key.Binding{k.Queue, k.Milestone, k.Done, k.Launch, k.Attach}
}

// writes are the bindings the root model handles rather than the board.
func (k boardKeyMap) writes() []key.Binding {
	return []key.Binding{k.Add, k.Edit}
}

// helpBindings are the board's bindings as the help screen lists them.
func (b Board) helpBindings() []key.Binding {
	bindings := []key.Binding{b.keys.Up, b.keys.Down, b.keys.Toggle}
	bindings = append(bindings, b.keys.writes()...)
	return append(bindings, b.keys.reserved()...)
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

// SetWidth records the space the board has to draw in; rows longer than it are
// truncated rather than wrapped, so one slice stays one line.
func (b *Board) SetWidth(width int) { b.width = width }

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

// Update handles the board's keys. The reserved bindings are matched only to be
// swallowed, so that a key a later milestone claims does nothing today.
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
	case key.Matches(press, b.keys.reserved()...):
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

// View renders the board.
func (b Board) View() string {
	if len(b.groups) == 0 {
		return b.styles.Faint.Render("No milestones yet.")
	}
	lines := make([]string, len(b.rows))
	for i, r := range b.rows {
		lines[i] = b.renderRow(i, r)
	}
	return strings.Join(lines, "\n")
}

// renderRow draws one line, with the cursor marker in front of it.
func (b Board) renderRow(i int, r row) string {
	marker := "  "
	if i == b.cursor {
		marker = b.styles.Cursor.Render("❯ ")
	}
	var body string
	if r.kind == rowMilestone {
		body = b.renderMilestone(b.groups[r.group], i == b.cursor)
	} else {
		body = "  " + b.renderSlice(b.groups[r.group].Slices[r.slice])
	}
	line := marker + body
	if b.width > 0 {
		line = lipgloss.NewStyle().MaxWidth(b.width).Render(line)
	}
	return line
}

// renderMilestone draws a group's own line: the fold indicator, its name, how
// many of its slices are done, and its status.
func (b Board) renderMilestone(g domain.Group, selected bool) string {
	fold := "▸"
	if b.expanded[groupKey(g)] {
		fold = "▾"
	}
	name := b.styles.Milestone.Render(g.Name())
	if selected {
		name = b.styles.Selected.Render(g.Name())
	}
	p := g.Progress()
	parts := []string{fold, name, b.styles.Faint.Render(fmt.Sprintf("%d/%d", p.Done, p.Total))}
	if g.Milestone != nil {
		parts = append(parts, b.styles.Faint.Render(string(g.Milestone.Status)))
	}
	return strings.Join(parts, " ")
}

// renderSlice draws one slice: its status, its name, who holds it, and whether
// it has a PR.
func (b Board) renderSlice(s domain.Slice) string {
	parts := []string{b.sliceIcon(s.Status), s.Name}
	if s.AssigneeName != "" {
		parts = append(parts, b.styles.Assignee.Render("@"+s.AssigneeName))
	}
	if s.PRURL != "" {
		parts = append(parts, b.styles.PR.Render("PR"))
	}
	return strings.Join(parts, " ")
}

// sliceIcon is the glyph for a slice's status. An unknown status — one Notion
// has grown that this build does not know — draws as an empty marker rather
// than nothing at all, so the column stays aligned.
func (b Board) sliceIcon(s domain.SliceStatus) string {
	switch s {
	case domain.SliceTodo:
		return b.styles.StatusTodo.Render("○")
	case domain.SliceClaimed:
		return b.styles.StatusClaimed.Render("◐")
	case domain.SliceDone:
		return b.styles.StatusDone.Render("●")
	default:
		return b.styles.Faint.Render("·")
	}
}
