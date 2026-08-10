package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/notion"
)

// treeLabels flattens a forest into indented labels, so a test can state the
// whole shape it expects in one literal.
func treeLabels(nodes []*treeNode) []string {
	var out []string
	var walk func(n *treeNode, depth int)
	walk = func(n *treeNode, depth int) {
		out = append(out, strings.Repeat("  ", depth)+n.label)
		for _, c := range n.children {
			walk(c, depth+1)
		}
	}
	for _, n := range nodes {
		walk(n, 0)
	}
	return out
}

func TestBuildDBTreeNestsDatabasesUnderTheirPages(t *testing.T) {
	pages := []notion.SearchResult{
		pageHit("home", "Home"),
		pageHit("projects", "Projects"),
	}
	pages[1].Parent = notion.PageParent("home")
	dbs := []notion.SearchResult{
		dataSourceHit("ds-deep", "db-deep", "Tracker"),
		dataSourceHit("ds-top", "db-top", "Recipes"),
	}
	parents := map[string]notion.Parent{
		"db-deep": notion.PageParent("projects"),
		"db-top":  {Type: "workspace"},
	}

	got := treeLabels(buildDBTree(dbs, pages, parents))
	want := []string{
		"Home",
		"  Projects",
		"    Tracker",
		"Recipes",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tree = %#v, want %#v", got, want)
	}
}

func TestBuildDBTreePutsOrphansAtTheRootAndPrunesEmptyPages(t *testing.T) {
	pages := []notion.SearchResult{
		pageHit("empty", "Notes"), // no database anywhere beneath: pruned
		pageHit("home", "Home"),
	}
	dbs := []notion.SearchResult{
		dataSourceHit("ds-1", "db-known", "Kept"),
		dataSourceHit("ds-2", "db-unknown", "Orphan"), // parent fetch failed
		{ID: "ds-3", Object: notion.SearchDataSource}, // untitled, no parent at all
	}
	parents := map[string]notion.Parent{"db-known": notion.PageParent("home")}

	got := treeLabels(buildDBTree(dbs, pages, parents))
	want := []string{
		"Home",
		"  Kept",
		"(untitled) ds-3",
		"Orphan",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tree = %#v, want %#v", got, want)
	}
}

func TestBuildDBTreeSortsGroupsBeforeLeaves(t *testing.T) {
	pages := []notion.SearchResult{
		pageHit("zeta", "zeta"),
		pageHit("alpha", "Alpha"),
	}
	dbs := []notion.SearchResult{
		dataSourceHit("ds-a", "db-a", "aardvark"),
		dataSourceHit("ds-z", "db-z", "Zebra"),
		dataSourceHit("ds-1", "db-1", "One"),
		dataSourceHit("ds-2", "db-2", "Two"),
	}
	parents := map[string]notion.Parent{
		"db-1": notion.PageParent("zeta"),
		"db-2": notion.PageParent("alpha"),
	}

	got := treeLabels(buildDBTree(dbs, pages, parents))
	want := []string{
		"Alpha", "  Two",
		"zeta", "  One",
		"aardvark",
		"Zebra",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tree = %#v, want groups first, each level alphabetical:\n%#v", got, want)
	}
}

func TestBuildDBTreeSurvivesAParentCycle(t *testing.T) {
	// Two pages claiming each other as parent must not hang or recurse forever.
	pages := []notion.SearchResult{
		pageHit("a", "A"),
		pageHit("b", "B"),
		pageHit("self", "Self"),
	}
	pages[0].Parent = notion.PageParent("b")
	pages[1].Parent = notion.PageParent("a")
	pages[2].Parent = notion.PageParent("self")
	dbs := []notion.SearchResult{dataSourceHit("ds-1", "db-1", "DB")}
	parents := map[string]notion.Parent{"db-1": notion.PageParent("a")}

	got := treeLabels(buildDBTree(dbs, pages, parents))
	// The loopers are re-rooted so nothing beneath them is lost: A keeps its
	// database, while B and Self hold nothing and are pruned.
	want := []string{"A", "  DB"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tree = %#v, want %#v", got, want)
	}
}

func TestBuildDBTreeIgnoresDuplicatePageHits(t *testing.T) {
	// Paginated search could hand the same page back twice; it must appear — and
	// hold its children — once.
	pages := []notion.SearchResult{
		pageHit("home", "Home"),
		pageHit("home", "Home"),
	}
	dbs := []notion.SearchResult{dataSourceHit("ds-1", "db-1", "DB")}
	parents := map[string]notion.Parent{"db-1": notion.PageParent("home")}

	got := treeLabels(buildDBTree(dbs, pages, parents))
	want := []string{"Home", "  DB"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tree = %#v, want %#v", got, want)
	}
}

func TestBuildDBTreeRootsAPageHangingOffALoop(t *testing.T) {
	// C is not part of the A↔B loop, but its parent chain never ends; it keeps
	// its database at the root rather than vanishing with the loop.
	pages := []notion.SearchResult{
		pageHit("a", "A"),
		pageHit("b", "B"),
		pageHit("c", "C"),
	}
	pages[0].Parent = notion.PageParent("b")
	pages[1].Parent = notion.PageParent("a")
	pages[2].Parent = notion.PageParent("a")
	dbs := []notion.SearchResult{dataSourceHit("ds-1", "db-1", "DB")}
	parents := map[string]notion.Parent{"db-1": notion.PageParent("c")}

	got := treeLabels(buildDBTree(dbs, pages, parents))
	want := []string{"C", "  DB"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tree = %#v, want %#v", got, want)
	}
}

// pickerKeys press one key on a picker.
func pressTree(t *treePicker, code rune) (string, bool, bool) {
	return t.Handle(tea.KeyPressMsg(tea.Key{Code: code}))
}

// testTree is a small forest: one page holding a database, one root database,
// and the create-new leaf, the way onboarding builds it.
func testTree() []*treeNode {
	return []*treeNode{
		{label: "Home", children: []*treeNode{
			{label: "Deep", children: []*treeNode{{label: "Tracker", value: "ds-tracker"}}},
			{label: "Recipes", value: "ds-recipes"},
		}},
		{label: "Loose", value: "ds-loose"},
		{label: "Create a new database…", value: createNewChoice},
	}
}

func TestTreePickerExpandsAndSelects(t *testing.T) {
	p := newTreePicker(DefaultStyles(), "Pick", "A database.", testTree())

	// Everything starts collapsed: three visible rows.
	if got := len(p.rows); got != 3 {
		t.Fatalf("visible rows = %d, want 3", got)
	}
	if _, chosen, _ := pressTree(p, tea.KeyRight); chosen {
		t.Fatal("expanding a group must not choose anything")
	}
	if got := len(p.rows); got != 5 {
		t.Fatalf("visible rows after expand = %d, want 5", got)
	}
	pressTree(p, tea.KeyDown) // onto Deep
	pressTree(p, 'l')         // expand it
	pressTree(p, 'j')         // onto Tracker
	choice, chosen, aborted := p.Handle(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !chosen || aborted || choice != "ds-tracker" {
		t.Errorf("Handle(enter) = (%q, %v, %v), want the nested database chosen", choice, chosen, aborted)
	}
}

func TestTreePickerEnterTogglesAGroup(t *testing.T) {
	p := newTreePicker(DefaultStyles(), "Pick", "", testTree())

	if _, chosen, _ := p.Handle(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); chosen {
		t.Fatal("enter on a group opens it rather than choosing")
	}
	if got := len(p.rows); got != 5 {
		t.Fatalf("rows after enter = %d, want the group open", got)
	}
	p.Handle(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got := len(p.rows); got != 3 {
		t.Fatalf("rows after a second enter = %d, want the group shut again", got)
	}
}

func TestTreePickerCollapseStepsOutToTheParent(t *testing.T) {
	p := newTreePicker(DefaultStyles(), "Pick", "", testTree())
	pressTree(p, tea.KeyRight) // open Home
	pressTree(p, tea.KeyDown)  // Deep
	pressTree(p, tea.KeyDown)  // Recipes, a leaf

	pressTree(p, tea.KeyLeft) // a leaf cannot collapse: go to Home
	if got := p.rows[p.cursor].label; got != "Home" {
		t.Fatalf("cursor on %q after left on a leaf, want Home", got)
	}
	pressTree(p, tea.KeyLeft) // Home is open: shut it
	if got := len(p.rows); got != 3 {
		t.Errorf("rows after collapsing Home = %d, want 3", got)
	}
	pressTree(p, tea.KeyLeft) // shut and at the root: nowhere further
	if got := p.rows[p.cursor].label; got != "Home" {
		t.Errorf("cursor on %q, want it to stay on Home", got)
	}
}

func TestTreePickerKeepsTheCursorInRange(t *testing.T) {
	p := newTreePicker(DefaultStyles(), "Pick", "", testTree())

	pressTree(p, tea.KeyUp)
	if p.cursor != 0 {
		t.Errorf("cursor = %d after up at the top, want 0", p.cursor)
	}
	for range 10 {
		pressTree(p, tea.KeyDown)
	}
	if want := len(p.rows) - 1; p.cursor != want {
		t.Errorf("cursor = %d after down past the end, want %d", p.cursor, want)
	}

	// Collapsing everything with the cursor at the bottom keeps it on a row.
	p.cursor = len(p.rows) - 1
	p.flatten()
	if p.cursor >= len(p.rows) {
		t.Errorf("cursor = %d of %d rows", p.cursor, len(p.rows))
	}
}

func TestTreePickerRowOfAnUnknownNode(t *testing.T) {
	// A node that is not drawn — its parent collapsed out from under it —
	// resolves to the top rather than out of range.
	p := newTreePicker(DefaultStyles(), "Pick", "", testTree())
	if got := p.rowOf(&treeNode{label: "elsewhere"}); got != 0 {
		t.Errorf("rowOf = %d, want 0", got)
	}
}

func TestTreePickerAborts(t *testing.T) {
	p := newTreePicker(DefaultStyles(), "Pick", "", testTree())
	if _, _, aborted := p.Handle(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})); !aborted {
		t.Error("ctrl+c should abort")
	}
}

func TestTreePickerWithNothingToShow(t *testing.T) {
	p := newTreePicker(DefaultStyles(), "Pick", "", nil)
	if _, chosen, aborted := p.Handle(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); chosen || aborted {
		t.Error("an empty picker has nothing to choose")
	}
}

func TestTreePickerViewMarksGroupsAndTheCursor(t *testing.T) {
	p := newTreePicker(DefaultStyles(), "Which one?", "Browse down.", testTree())
	view := stripANSI(p.View())

	for _, want := range []string{"Which one?", "Browse down.", "> ▸ Home", "Loose", "enter select"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
	pressTree(p, tea.KeyRight)
	if view := stripANSI(p.View()); !strings.Contains(view, "▾ Home") {
		t.Errorf("an open group should show ▾:\n%s", view)
	}
}

func TestTreePickerViewScrollsToTheCursor(t *testing.T) {
	roots := make([]*treeNode, 20)
	for i := range roots {
		roots[i] = &treeNode{label: string(rune('a' + i)), value: "ds"}
	}
	p := newTreePicker(DefaultStyles(), "Pick", "", roots)
	p.SetSize(40, 10)

	if view := stripANSI(p.View()); strings.Contains(view, "\n  t\n") || !strings.Contains(view, "> a") {
		t.Errorf("the top of the list should show first:\n%s", view)
	}
	for range 19 {
		pressTree(p, tea.KeyDown)
	}
	view := stripANSI(p.View())
	if !strings.Contains(view, "> t") {
		t.Errorf("the cursor's row must be visible:\n%s", view)
	}
	if got := lipgloss.Height(p.View()); got > 10 {
		t.Errorf("view is %d lines in a 10-line window", got)
	}
}

func TestTreePickerViewFitsAOneRowWindow(t *testing.T) {
	// A window shorter than the chrome still shows the cursor's row.
	p := newTreePicker(DefaultStyles(), "Pick", "", testTree())
	p.SetSize(40, treeChromeHeight+1)
	pressTree(p, tea.KeyDown)

	if view := stripANSI(p.View()); !strings.Contains(view, "> Loose") {
		t.Errorf("want only the cursor's row:\n%s", view)
	}
}

func TestTreePickerViewTruncatesToTheWidth(t *testing.T) {
	roots := []*treeNode{{label: strings.Repeat("wide ", 20), value: "ds"}}
	p := newTreePicker(DefaultStyles(), "Pick", strings.Repeat("long ", 20), roots)
	p.SetSize(20, 0)

	for i, line := range strings.Split(p.View(), "\n") {
		if got := lipgloss.Width(line); got > 20 {
			t.Errorf("line %d is %d columns wide: %q", i, got, stripANSI(line))
		}
	}
}

func TestTreePickerGolden(t *testing.T) {
	p := newTreePicker(DefaultStyles(),
		"Which database holds your projects?",
		"Browse to the existing project database, or create one.", testTree())
	p.SetSize(60, 16)
	pressTree(p, tea.KeyRight)

	golden(t, "onboarding-tree", p.View())
}
