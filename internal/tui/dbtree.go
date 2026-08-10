package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// treeNode is one entry of the database picker: a page whose contents load on
// demand, or a selectable leaf — a database, or the create-new escape hatch.
type treeNode struct {
	label    string
	value    string // "" for a page; otherwise what selecting yields
	pageID   string // the page a group node fetches its children from
	children []*treeNode
	expanded bool
	loaded   bool // the children have been fetched
	loading  bool // the fetch is in flight
}

// group reports whether the node exists to be opened rather than selected.
func (n *treeNode) group() bool { return n.value == "" }

// treeEvent is what one key press produced. At most one field is set: a chosen
// leaf's value, an abort, a request to search instead of browse, or a page
// whose children must be fetched before it can open.
type treeEvent struct {
	choice  string
	chosen  bool
	aborted bool
	search  bool
	load    *treeNode
}

// treeKeyMap is the picker's bindings.
type treeKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Expand   key.Binding
	Collapse key.Binding
	Select   key.Binding
	Search   key.Binding
	Abort    key.Binding
}

// defaultTreeKeyMap returns the bindings the picker runs with.
func defaultTreeKeyMap() treeKeyMap {
	return treeKeyMap{
		Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("↓/j", "down")),
		Expand:   key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("→/l", "expand")),
		Collapse: key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("←/h", "collapse")),
		Select:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Search:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Abort:    key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	}
}

// treePicker walks a treeNode forest: pages expand and collapse in place —
// fetching their contents the first time — and landing enter on a leaf chooses
// it. It stands where a flat select would, because a workspace can hold too
// many pages to load, let alone scan, as one list.
type treePicker struct {
	styles Styles
	keys   treeKeyMap

	title       string
	description string

	roots  []*treeNode
	parent map[*treeNode]*treeNode
	rows   []*treeNode // the visible nodes, in drawing order
	cursor int
	// moved is set once the user has put the cursor somewhere. Until then the
	// cursor sits on the top row, so the first page the search streams in
	// lands under it; afterwards it follows the node the user chose as rows
	// shift around it.
	moved bool

	width  int
	height int
}

// newTreePicker returns the picker over roots, everything collapsed.
func newTreePicker(styles Styles, title, description string, roots []*treeNode) *treePicker {
	t := &treePicker{
		styles:      styles,
		keys:        defaultTreeKeyMap(),
		title:       title,
		description: description,
		roots:       roots,
		parent:      map[*treeNode]*treeNode{},
	}
	for _, r := range roots {
		t.register(r)
	}
	t.flatten()
	return t
}

// register records who holds whom below n, so collapse can step out of any row.
func (t *treePicker) register(n *treeNode) {
	for _, c := range n.children {
		t.parent[c] = n
		t.register(c)
	}
}

// AddRoots inserts nodes just above the picker's last root — the create-new
// escape hatch, which stays at the bottom while the workspace search streams
// pages in above it.
func (t *treePicker) AddRoots(nodes ...*treeNode) {
	if len(nodes) == 0 {
		return
	}
	for _, n := range nodes {
		t.register(n)
	}
	i := max(0, len(t.roots)-1)
	t.roots = append(t.roots[:i:i], append(nodes, t.roots[i:]...)...)
	t.flatten()
}

// SetChildren hands a page node the children its fetch returned and opens it.
func (t *treePicker) SetChildren(n *treeNode, children []*treeNode) {
	n.children = children
	n.loaded, n.loading, n.expanded = true, false, true
	t.register(n)
	t.flatten()
}

// SetSize tells the picker how much window it has to draw in.
func (t *treePicker) SetSize(width, height int) { t.width, t.height = width, height }

// flatten rebuilds the visible rows from the expansion state. Once the user
// has moved, the cursor follows the node it was on — rows shift under it as
// pages stream in and children arrive — and falls back to a row that exists.
func (t *treePicker) flatten() {
	var focused *treeNode
	if t.moved && t.cursor < len(t.rows) {
		focused = t.rows[t.cursor]
	}
	t.rows = t.rows[:0]
	var walk func(n *treeNode)
	walk = func(n *treeNode) {
		t.rows = append(t.rows, n)
		if !n.expanded {
			return
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	for _, r := range t.roots {
		walk(r)
	}
	if focused != nil {
		t.cursor = t.rowOf(focused)
	}
	t.cursor = max(0, min(t.cursor, len(t.rows)-1))
}

// rowOf says where a node is drawn, so the cursor can follow it.
func (t *treePicker) rowOf(n *treeNode) int {
	for i, row := range t.rows {
		if row == n {
			return i
		}
	}
	return 0
}

// Handle applies one key press. Opening a page whose contents have not been
// fetched yet reports it as the event's load — the caller fetches and answers
// with SetChildren.
func (t *treePicker) Handle(msg tea.KeyPressMsg) treeEvent {
	if key.Matches(msg, t.keys.Abort) {
		return treeEvent{aborted: true}
	}
	// Searching is offered whatever the tree holds — a workspace that shared no
	// page to browse can still be searched.
	if key.Matches(msg, t.keys.Search) {
		return treeEvent{search: true}
	}
	if len(t.rows) == 0 {
		return treeEvent{}
	}
	t.moved = true
	node := t.rows[t.cursor]
	switch {
	case key.Matches(msg, t.keys.Up):
		t.cursor = max(0, t.cursor-1)
	case key.Matches(msg, t.keys.Down):
		t.cursor = min(len(t.rows)-1, t.cursor+1)
	case key.Matches(msg, t.keys.Expand):
		if node.group() && !node.expanded {
			return t.open(node)
		}
	case key.Matches(msg, t.keys.Collapse):
		if node.group() && node.expanded {
			node.expanded = false
			t.flatten()
			break
		}
		// On anything already closed, step out to the row's parent.
		if p, ok := t.parent[node]; ok {
			t.cursor = t.rowOf(p)
		}
	case key.Matches(msg, t.keys.Select):
		if !node.group() {
			return treeEvent{choice: node.value, chosen: true}
		}
		if node.expanded {
			node.expanded = false
			t.flatten()
			break
		}
		return t.open(node)
	}
	return treeEvent{}
}

// open expands a page in place when its contents are already here, and asks
// the caller to fetch them the first time. A fetch already in flight is left
// to finish rather than issued again.
func (t *treePicker) open(n *treeNode) treeEvent {
	if n.loaded {
		n.expanded = true
		t.flatten()
		return treeEvent{}
	}
	if n.loading {
		return treeEvent{}
	}
	n.loading = true
	return treeEvent{load: n}
}

// treeChromeHeight is what the picker draws besides the rows: the title, the
// description, the blank line under them, and the help line.
const treeChromeHeight = 4

// View draws the picker: heading, the visible slice of rows, and the keys.
func (t *treePicker) View() string {
	var b strings.Builder
	b.WriteString(fit(t.styles.Title.Render(t.title), t.width) + "\n")
	b.WriteString(fit(t.styles.Faint.Render(t.description), t.width) + "\n\n")

	depth := map[*treeNode]int{}
	for _, n := range t.rows {
		depth[n] = depth[t.parent[n]] + 1
	}
	for _, row := range t.visible() {
		i := t.rowOf(row)
		cursor, style := "  ", lipgloss.NewStyle()
		if i == t.cursor {
			cursor, style = t.styles.Cursor.Render("> "), t.styles.Selected
		}
		glyph, label := "", row.label
		if row.group() {
			glyph, style = "▸ ", t.styles.Milestone
			if row.expanded {
				glyph = "▾ "
			}
			if row.loading {
				label += " …"
			}
			if i == t.cursor {
				style = t.styles.Selected
			}
		}
		indent := strings.Repeat("  ", depth[row]-1)
		b.WriteString(fit(cursor+indent+style.Render(glyph+label), t.width) + "\n")
	}

	help := make([]string, 0, 5)
	for _, k := range []key.Binding{t.keys.Up, t.keys.Down, t.keys.Expand, t.keys.Select, t.keys.Search} {
		help = append(help, t.styles.HelpKey.Render(k.Help().Key)+" "+t.styles.HelpDesc.Render(k.Help().Desc))
	}
	b.WriteString(fit(strings.Join(help, "  "), t.width))
	return b.String()
}

// visible is the window of rows the height allows, slid so the cursor is
// always inside it. An unmeasured window shows everything.
func (t *treePicker) visible() []*treeNode {
	room := t.height - treeChromeHeight
	if t.height <= 0 || room >= len(t.rows) {
		return t.rows
	}
	room = max(room, 1)
	start := min(max(0, t.cursor-room/2), len(t.rows)-room)
	return t.rows[start : start+room]
}
