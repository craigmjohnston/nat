package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// pressTree presses one key on a picker.
func pressTree(t *treePicker, code rune) treeEvent {
	return t.Handle(tea.KeyPressMsg(tea.Key{Code: code}))
}

// loadedPage builds a page node whose children are already here, the state a
// node reaches after its fetch has answered.
func loadedPage(label string, children ...*treeNode) *treeNode {
	return &treeNode{label: label, pageID: "page-" + label, loaded: true, children: children}
}

// testTree is a small forest: one page holding a nested page and a database,
// one root database, and the create-new leaf, the way onboarding builds it
// once the fetches have run.
func testTree() []*treeNode {
	return []*treeNode{
		loadedPage("Home",
			loadedPage("Deep", &treeNode{label: "Tracker", value: "db-tracker"}),
			&treeNode{label: "Recipes", value: "db-recipes"},
		),
		{label: "Loose", value: "db-loose"},
		{label: "Create a new database…", value: createNewChoice},
	}
}

func TestTreePickerExpandsAndSelects(t *testing.T) {
	p := newTreePicker(DefaultStyles(), "Pick", "A database.", testTree())

	// Everything starts collapsed: three visible rows.
	if got := len(p.rows); got != 3 {
		t.Fatalf("visible rows = %d, want 3", got)
	}
	if ev := pressTree(p, tea.KeyRight); ev.chosen || ev.load != nil {
		t.Fatal("expanding a loaded page must not choose or fetch anything")
	}
	if got := len(p.rows); got != 5 {
		t.Fatalf("visible rows after expand = %d, want 5", got)
	}
	pressTree(p, tea.KeyDown) // onto Deep
	pressTree(p, 'l')         // expand it
	pressTree(p, 'j')         // onto Tracker
	ev := p.Handle(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !ev.chosen || ev.aborted || ev.choice != "db-tracker" {
		t.Errorf("Handle(enter) = %+v, want the nested database chosen", ev)
	}
}

func TestTreePickerEnterTogglesALoadedPage(t *testing.T) {
	p := newTreePicker(DefaultStyles(), "Pick", "", testTree())

	if ev := p.Handle(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); ev.chosen {
		t.Fatal("enter on a page opens it rather than choosing")
	}
	if got := len(p.rows); got != 5 {
		t.Fatalf("rows after enter = %d, want the page open", got)
	}
	p.Handle(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got := len(p.rows); got != 3 {
		t.Fatalf("rows after a second enter = %d, want the page shut again", got)
	}
}

func TestTreePickerLoadsAPageOnFirstOpen(t *testing.T) {
	p := newTreePicker(DefaultStyles(), "Pick", "", []*treeNode{
		{label: "Home", pageID: "page-1"},
		{label: "Create a new database…", value: createNewChoice},
	})
	home := p.roots[0]

	ev := pressTree(p, tea.KeyRight)
	if ev.load != home {
		t.Fatalf("event = %+v, want a load request for the page", ev)
	}
	if !home.loading {
		t.Error("the page should be marked loading while its fetch runs")
	}
	// Pressing again while the fetch is in flight must not fetch twice, on
	// either key that opens a node.
	if ev := pressTree(p, tea.KeyRight); ev.load != nil {
		t.Error("a second expand issued a second fetch")
	}
	if ev := p.Handle(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); ev.load != nil || ev.chosen {
		t.Error("enter while loading issued a second fetch or chose the page")
	}

	p.SetChildren(home, []*treeNode{{label: "Slices", value: "db-1"}})
	if !home.loaded || home.loading || !home.expanded {
		t.Errorf("node = %+v, want it loaded and open", home)
	}
	if got := len(p.rows); got != 3 {
		t.Fatalf("rows = %d, want the child on show", got)
	}
	// The child was registered: collapse from it steps out to its parent.
	pressTree(p, tea.KeyDown)
	pressTree(p, tea.KeyLeft)
	if got := p.rows[p.cursor]; got != home {
		t.Errorf("cursor on %q after left on the child, want Home", got.label)
	}
	// A later open costs nothing: the children are kept.
	pressTree(p, tea.KeyLeft)
	if ev := pressTree(p, tea.KeyRight); ev.load != nil {
		t.Error("reopening a loaded page fetched it again")
	}
}

func TestTreePickerEnterAsksForALoad(t *testing.T) {
	p := newTreePicker(DefaultStyles(), "Pick", "", []*treeNode{{label: "Home", pageID: "page-1"}})
	if ev := p.Handle(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); ev.load != p.roots[0] {
		t.Errorf("event = %+v, want enter to request the fetch", ev)
	}
}

func TestTreePickerSetChildrenWithNothingFound(t *testing.T) {
	// A page holding no pages or databases opens onto nothing rather than
	// erroring, and stays openable without refetching.
	p := newTreePicker(DefaultStyles(), "Pick", "", []*treeNode{{label: "Empty", pageID: "page-1"}})
	pressTree(p, tea.KeyRight)
	p.SetChildren(p.roots[0], nil)

	if got := len(p.rows); got != 1 {
		t.Errorf("rows = %d, want just the empty page", got)
	}
	pressTree(p, tea.KeyLeft)
	if ev := pressTree(p, tea.KeyRight); ev.load != nil {
		t.Error("an empty page should not be fetched again")
	}
}

func TestTreePickerAddRootsKeepsTheEscapeHatchLast(t *testing.T) {
	p := newTreePicker(DefaultStyles(), "Pick", "",
		[]*treeNode{{label: "Create a new database…", value: createNewChoice}})

	p.AddRoots(&treeNode{label: "Home", pageID: "p1"})
	p.AddRoots() // a search page with nothing to add changes nothing
	p.AddRoots(loadedPage("Docs", &treeNode{label: "Tracker", value: "db-1"}))

	if got := len(p.roots); got != 3 {
		t.Fatalf("roots = %d, want 3", got)
	}
	if got := p.roots[2].value; got != createNewChoice {
		t.Errorf("last root = %q, want the escape hatch", p.roots[2].label)
	}
	if got := p.roots[0].label; got != "Home" {
		t.Errorf("first root = %q, want the streamed order kept", got)
	}
	// The added root's subtree was registered: collapse steps out of it.
	docs := p.roots[1]
	pressTree(p, tea.KeyDown)
	pressTree(p, tea.KeyRight)
	pressTree(p, tea.KeyDown)
	pressTree(p, tea.KeyLeft)
	if got := p.rows[p.cursor]; got != docs {
		t.Errorf("cursor on %q, want Docs", got.label)
	}
}

func TestTreePickerCursorFollowsItsNodeAsRootsStream(t *testing.T) {
	p := newTreePicker(DefaultStyles(), "Pick", "",
		[]*treeNode{{label: "Create a new database…", value: createNewChoice}})
	hatch := p.roots[0]

	// Before the user touches the picker, the cursor stays on the top row, so
	// the first page to stream in lands under it.
	p.AddRoots(&treeNode{label: "Home", pageID: "p1"})
	if got := p.rows[p.cursor].label; got != "Home" {
		t.Errorf("cursor on %q after the first batch, want Home", got)
	}

	// Once the user has parked the cursor somewhere — here, back on the
	// escape hatch — pages streaming in above must not shove it onto a row
	// they never chose.
	pressTree(p, tea.KeyDown)
	p.AddRoots(&treeNode{label: "Docs", pageID: "p2"})
	if got := p.rows[p.cursor]; got != hatch {
		t.Errorf("cursor on %q after more roots streamed in, want the escape hatch", got.label)
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

	// A cursor left beyond the rows — its node collapsed out from under it —
	// comes back onto one.
	p.roots[0].expanded = true
	p.flatten()
	p.cursor = len(p.rows) - 1
	p.roots[0].expanded = false
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
	if ev := p.Handle(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})); !ev.aborted {
		t.Error("ctrl+c should abort")
	}
}

func TestTreePickerWithNothingToShow(t *testing.T) {
	p := newTreePicker(DefaultStyles(), "Pick", "", nil)
	if ev := p.Handle(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); ev.chosen || ev.aborted {
		t.Error("an empty picker has nothing to choose")
	}
}

func TestTreePickerViewMarksPagesAndTheCursor(t *testing.T) {
	p := newTreePicker(DefaultStyles(), "Which one?", "Browse down.", testTree())
	view := stripANSI(p.View())

	for _, want := range []string{"Which one?", "Browse down.", "> ▸ Home", "Loose", "enter select"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
	pressTree(p, tea.KeyRight)
	if view := stripANSI(p.View()); !strings.Contains(view, "▾ Home") {
		t.Errorf("an open page should show ▾:\n%s", view)
	}
}

func TestTreePickerViewMarksALoadingPage(t *testing.T) {
	p := newTreePicker(DefaultStyles(), "Pick", "", []*treeNode{{label: "Home", pageID: "p1"}})
	pressTree(p, tea.KeyRight)

	if view := stripANSI(p.View()); !strings.Contains(view, "Home …") {
		t.Errorf("a loading page should show an ellipsis:\n%s", view)
	}
}

func TestTreePickerViewScrollsToTheCursor(t *testing.T) {
	roots := make([]*treeNode, 20)
	for i := range roots {
		roots[i] = &treeNode{label: string(rune('a' + i)), value: "db"}
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
	roots := []*treeNode{{label: strings.Repeat("wide ", 20), value: "db"}}
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
