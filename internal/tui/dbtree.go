package tui

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/notion"
)

// treeNode is one entry of the database picker: a page that groups what lives
// under it, or a selectable leaf — a database, or the create-new escape hatch.
type treeNode struct {
	label    string
	value    string // "" for a page that only groups; otherwise what selecting yields
	children []*treeNode
	expanded bool
}

// group reports whether the node exists to be opened rather than selected.
func (n *treeNode) group() bool { return n.value == "" }

// buildDBTree arranges the database candidates by where they live: pages nest
// under their parent pages, and each database sits under the page its parent
// database hangs off, so the user browses down to it. parents maps a database
// ID to that database's own parent. A database whose parent is unknown or is
// not a shared page sits at the root — the workspace — and pages that lead to
// no database are left out.
func buildDBTree(dbs, pages []notion.SearchResult, parents map[string]notion.Parent) []*treeNode {
	byID := make(map[string]notion.SearchResult, len(pages))
	nodes := make(map[string]*treeNode, len(pages))
	for _, p := range pages {
		if _, dup := nodes[p.ID]; dup {
			continue
		}
		nodes[p.ID] = &treeNode{label: resultLabel(p)}
		byID[p.ID] = p
	}

	var roots []*treeNode
	placed := map[string]bool{}
	for _, p := range pages {
		if placed[p.ID] {
			continue
		}
		placed[p.ID] = true
		if parent, ok := nodes[p.Parent.PageID]; ok && !looping(p, byID) {
			parent.children = append(parent.children, nodes[p.ID])
			continue
		}
		roots = append(roots, nodes[p.ID])
	}

	for _, db := range dbs {
		leaf := &treeNode{label: resultLabel(db), value: db.ID}
		if parent, ok := nodes[parents[db.Parent.DatabaseID].PageID]; ok {
			parent.children = append(parent.children, leaf)
			continue
		}
		roots = append(roots, leaf)
	}

	return pruneEmpty(roots)
}

// looping reports whether following p's parents runs into a loop. Real Notion
// data cannot hold one, but a defensive tree must not lose the pages — and
// whatever databases they hold — to bad data, so a looping page is treated as
// having no parent and sits at the root instead.
func looping(p notion.SearchResult, byID map[string]notion.SearchResult) bool {
	id := p.Parent.PageID
	for range len(byID) {
		if id == p.ID {
			return true
		}
		hit, ok := byID[id]
		if !ok {
			return false
		}
		id = hit.Parent.PageID
	}
	// A chain longer than the pages themselves is a loop p merely hangs off.
	return true
}

// pruneEmpty drops the groups that hold no database anywhere beneath them,
// sorting what is kept — groups first, then leaves, each alphabetically — so
// the listing reads like a directory.
func pruneEmpty(nodes []*treeNode) []*treeNode {
	kept := make([]*treeNode, 0, len(nodes))
	for _, n := range nodes {
		n.children = pruneEmpty(n.children)
		if n.group() && len(n.children) == 0 {
			continue
		}
		kept = append(kept, n)
	}
	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].group() != kept[j].group() {
			return kept[i].group()
		}
		return strings.ToLower(kept[i].label) < strings.ToLower(kept[j].label)
	})
	return kept
}

// treeKeyMap is the picker's bindings.
type treeKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Expand   key.Binding
	Collapse key.Binding
	Select   key.Binding
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
		Abort:    key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	}
}

// treePicker walks a treeNode forest: groups expand and collapse in place, and
// landing enter on a leaf chooses it. It stands where a flat select would,
// because a workspace can hold too many databases to scan as one list.
type treePicker struct {
	styles Styles
	keys   treeKeyMap

	title       string
	description string

	roots  []*treeNode
	parent map[*treeNode]*treeNode
	rows   []*treeNode // the visible nodes, in drawing order
	cursor int

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
	var walk func(n *treeNode)
	walk = func(n *treeNode) {
		for _, c := range n.children {
			t.parent[c] = n
			walk(c)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	t.flatten()
	return t
}

// SetSize tells the picker how much window it has to draw in.
func (t *treePicker) SetSize(width, height int) { t.width, t.height = width, height }

// flatten rebuilds the visible rows from the expansion state, keeping the
// cursor on a row that still exists.
func (t *treePicker) flatten() {
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
	t.cursor = max(0, min(t.cursor, len(t.rows)-1))
}

// rowOf says where a node is drawn, so collapsing to a parent can follow it.
func (t *treePicker) rowOf(n *treeNode) int {
	for i, row := range t.rows {
		if row == n {
			return i
		}
	}
	return 0
}

// Handle applies one key press. It returns the chosen leaf's value once enter
// lands on one, and aborted when the user backs out of the picker entirely.
func (t *treePicker) Handle(msg tea.KeyPressMsg) (choice string, chosen, aborted bool) {
	if key.Matches(msg, t.keys.Abort) {
		return "", false, true
	}
	if len(t.rows) == 0 {
		return "", false, false
	}
	node := t.rows[t.cursor]
	switch {
	case key.Matches(msg, t.keys.Up):
		t.cursor = max(0, t.cursor-1)
	case key.Matches(msg, t.keys.Down):
		t.cursor = min(len(t.rows)-1, t.cursor+1)
	case key.Matches(msg, t.keys.Expand):
		if node.group() && !node.expanded {
			node.expanded = true
			t.flatten()
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
		if node.group() {
			node.expanded = !node.expanded
			t.flatten()
			break
		}
		return node.value, true, false
	}
	return "", false, false
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
		glyph := ""
		if row.group() {
			glyph, style = "▸ ", t.styles.Milestone
			if row.expanded {
				glyph = "▾ "
			}
			if i == t.cursor {
				style = t.styles.Selected
			}
		}
		indent := strings.Repeat("  ", depth[row]-1)
		b.WriteString(fit(cursor+indent+style.Render(glyph+row.label), t.width) + "\n")
	}

	help := make([]string, 0, 4)
	for _, k := range []key.Binding{t.keys.Up, t.keys.Down, t.keys.Expand, t.keys.Select} {
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
