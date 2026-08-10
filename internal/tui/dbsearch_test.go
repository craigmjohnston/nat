package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/notion"
)

// dsHit builds a data source search hit: a top-level title, and the database
// that holds it as its parent.
func dsHit(id, title, dbID string) notion.SearchResult {
	return notion.SearchResult{
		Object: notion.SearchDataSource,
		ID:     id,
		Title:  []notion.RichText{{PlainText: title}},
		Parent: notion.Parent{Type: notion.ParentDatabase, DatabaseID: dbID},
	}
}

// typeInto types a string into the search view one key at a time, returning the
// event the last press produced.
func typeInto(s *searchPicker, text string) searchEvent {
	var ev searchEvent
	for _, r := range text {
		ev = s.Handle(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	return ev
}

// pressSearch presses one non-printing key on the search view.
func pressSearch(s *searchPicker, code rune) searchEvent {
	return s.Handle(tea.KeyPressMsg(tea.Key{Code: code}))
}

func TestSearchPickerTypingDrivesTheQuery(t *testing.T) {
	s := newSearchPicker(DefaultStyles(), 60, 20)

	if ev := typeInto(s, "ops"); !ev.queried {
		t.Fatalf("typing = %+v, want a fresh search due", ev)
	}
	if s.query != "ops" {
		t.Errorf("query = %q, want ops", s.query)
	}
	if ev := pressSearch(s, tea.KeyBackspace); !ev.queried || s.query != "op" {
		t.Errorf("backspace = %+v, query = %q, want op", ev, s.query)
	}

	// Backspacing an empty query has nothing to search for again.
	typeInto(s, "")
	s.query = ""
	if ev := pressSearch(s, tea.KeyBackspace); ev.queried {
		t.Errorf("backspace on an empty query = %+v, want no search", ev)
	}
}

func TestSearchPickerKeepsResultsWhileTheNextSearchRuns(t *testing.T) {
	// A list that emptied on every keystroke would flicker through a word.
	s := newSearchPicker(DefaultStyles(), 60, 20)
	s.SetResults([]notion.SearchResult{dsHit("ds-1", "Ops Board", "db-1")})
	typeInto(s, "o")

	if len(s.results) != 1 || !s.loading {
		t.Errorf("results = %d, loading = %v, want the old hits kept while loading", len(s.results), s.loading)
	}
}

func TestSearchPickerMovesAndSelects(t *testing.T) {
	s := newSearchPicker(DefaultStyles(), 60, 20)
	s.SetResults([]notion.SearchResult{
		dsHit("ds-1", "Ops Board", "db-1"),
		dsHit("ds-2", "Projects", "db-2"),
	})

	pressSearch(s, tea.KeyDown)
	ev := pressSearch(s, tea.KeyEnter)
	if !ev.chosen || ev.result.ID != "ds-2" {
		t.Fatalf("enter = %+v, want the second hit chosen", ev)
	}

	// The cursor stops at both ends.
	pressSearch(s, tea.KeyDown)
	if s.cursor != 1 {
		t.Errorf("cursor = %d, want it held at the last hit", s.cursor)
	}
	pressSearch(s, tea.KeyUp)
	pressSearch(s, tea.KeyUp)
	if s.cursor != 0 {
		t.Errorf("cursor = %d, want it held at the first hit", s.cursor)
	}
}

func TestSearchPickerEnterWithNoResultsChoosesNothing(t *testing.T) {
	s := newSearchPicker(DefaultStyles(), 60, 20)

	if ev := pressSearch(s, tea.KeyEnter); ev.chosen {
		t.Errorf("enter = %+v, want nothing chosen from an empty list", ev)
	}
}

func TestSearchPickerCloseAndAbort(t *testing.T) {
	s := newSearchPicker(DefaultStyles(), 60, 20)

	if ev := pressSearch(s, tea.KeyEscape); !ev.closed {
		t.Errorf("esc = %+v, want the search closed", ev)
	}
	ev := s.Handle(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	if !ev.aborted {
		t.Errorf("ctrl+c = %+v, want an abort", ev)
	}
}

func TestSearchPickerCursorSurvivesAShorterList(t *testing.T) {
	s := newSearchPicker(DefaultStyles(), 60, 20)
	s.SetResults([]notion.SearchResult{
		dsHit("ds-1", "One", "db-1"),
		dsHit("ds-2", "Two", "db-2"),
		dsHit("ds-3", "Three", "db-3"),
	})
	pressSearch(s, tea.KeyDown)
	pressSearch(s, tea.KeyDown)

	s.SetResults([]notion.SearchResult{dsHit("ds-1", "One", "db-1")})
	if s.cursor != 0 {
		t.Errorf("cursor = %d, want it pulled back onto a hit that exists", s.cursor)
	}

	s.SetResults(nil)
	if s.cursor != 0 {
		t.Errorf("cursor = %d, want 0 for an empty list", s.cursor)
	}
}

func TestSearchPickerResolvesTrailsForVisibleRowsOnly(t *testing.T) {
	s := newSearchPicker(DefaultStyles(), 60, searchChromeHeight+2)
	s.SetResults([]notion.SearchResult{
		dsHit("ds-1", "One", "db-1"),
		dsHit("ds-2", "Two", "db-2"),
		dsHit("ds-3", "Three", "db-3"),
	})

	first := s.pending()
	if len(first) != 2 || first[0].ID != "ds-1" || first[1].ID != "ds-2" {
		t.Fatalf("pending = %+v, want the two rows the window shows", first)
	}
	if again := s.pending(); len(again) != 0 {
		t.Errorf("pending = %+v, want each hit asked about once", again)
	}

	// Scrolling brings the third row into view, and only that one is new.
	pressSearch(s, tea.KeyDown)
	pressSearch(s, tea.KeyDown)
	scrolled := s.pending()
	if len(scrolled) != 1 || scrolled[0].ID != "ds-3" {
		t.Errorf("pending after scrolling = %+v, want ds-3 alone", scrolled)
	}
}

func TestSearchPickerViewShowsTrailsAndPlaceholders(t *testing.T) {
	s := newSearchPicker(DefaultStyles(), 80, 20)
	s.SetResults([]notion.SearchResult{
		dsHit("ds-1", "Ops Board", "db-1"),
		dsHit("ds-2", "Ops Board", "db-2"),
	})
	s.SetCrumb("ds-1", []string{"Engineering", "Projects", "Q3"})

	view := stripANSI(s.View())
	if !strings.Contains(view, "> Ops Board — Engineering / Projects / Q3") {
		t.Errorf("want the resolved trail on the first row:\n%s", view)
	}
	// The second row's walk has not answered yet, so it says so rather than
	// looking like a database sitting at the workspace root.
	if !strings.Contains(view, "Ops Board — "+notion.BreadcrumbEllipsis) {
		t.Errorf("want the placeholder on the unresolved row:\n%s", view)
	}
}

func TestSearchPickerViewDropsARedundantDatabaseSegment(t *testing.T) {
	// A database with one same-named data source is the ordinary case; naming
	// it in the trail would repeat the hit's own label on every row.
	s := newSearchPicker(DefaultStyles(), 80, 20)
	s.SetResults([]notion.SearchResult{dsHit("ds-1", "Tracker", "db-1")})
	s.SetCrumb("ds-1", []string{"Home", "Tracker"})

	view := stripANSI(s.View())
	if !strings.Contains(view, "Tracker — Home") || strings.Contains(view, "Home / Tracker") {
		t.Errorf("want the repeated segment dropped:\n%s", view)
	}

	// With nothing left in the trail, the row is just the name.
	s.SetCrumb("ds-1", []string{"Tracker"})
	if view := stripANSI(s.View()); strings.Contains(view, "—") {
		t.Errorf("want no trail separator when nothing remains:\n%s", view)
	}
}

func TestSearchPickerViewStates(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*searchPicker)
		want  string
	}{
		{"searching", func(*searchPicker) {}, "Searching…"},
		{"nothing found", func(s *searchPicker) { s.SetResults(nil) }, "No databases match."},
		{"the search failed", func(s *searchPicker) {
			s.SetError(errors.New("search for databases: boom\nsecond line"))
		}, "search for databases: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSearchPicker(DefaultStyles(), 80, 20)
			typeInto(s, "q")
			tt.setup(s)

			view := stripANSI(s.View())
			if !strings.Contains(view, tt.want) {
				t.Errorf("view = %q, want it to hold %q", view, tt.want)
			}
			if strings.Contains(view, "second line") {
				t.Errorf("an error must not take a second line:\n%s", view)
			}
		})
	}
}

func TestSearchPickerViewFitsTheWindow(t *testing.T) {
	s := newSearchPicker(DefaultStyles(), 0, 0)
	results := make([]notion.SearchResult, 8)
	for i := range results {
		results[i] = dsHit("ds-"+string(rune('a'+i)), strings.Repeat("wide ", 20), "db-1")
	}
	s.SetResults(results)

	// Unmeasured, everything is drawn.
	if got := len(s.visible()); got != len(results) {
		t.Errorf("visible = %d, want all %d rows before the window is measured", got, len(results))
	}

	s.SetSize(20, 10)
	for i, line := range strings.Split(s.View(), "\n") {
		if got := lipgloss.Width(line); got > 20 {
			t.Errorf("line %d is %d columns wide: %q", i, got, stripANSI(line))
		}
	}
	if got := lipgloss.Height(s.View()); got > 10 {
		t.Errorf("view is %d lines in a 10-line window", got)
	}

	// A window shorter than the chrome still shows the cursor's row.
	s.SetSize(60, searchChromeHeight)
	if got := len(s.visible()); got != 1 {
		t.Errorf("visible = %d, want the cursor's row alone", got)
	}
}

func TestSearchPickerGolden(t *testing.T) {
	s := newSearchPicker(DefaultStyles(), 60, 12)
	typeInto(s, "ops")
	s.SetResults([]notion.SearchResult{
		dsHit("ds-1", "Ops Board", "db-1"),
		dsHit("ds-2", "Ops Archive", "db-2"),
		notion.SearchResult{ID: "ds-3", Parent: notion.Parent{Type: notion.ParentDatabase, DatabaseID: "db-3"}},
	})
	s.SetCrumb("ds-1", []string{"Engineering", "Projects", "Q3"})
	s.SetCrumb("ds-2", []string{"Engineering", "Archive"})

	golden(t, "onboarding-search", s.View())
}

func TestSearchKeyMapHelpIsComplete(t *testing.T) {
	// Every key the help line draws needs both halves of its help text.
	keys := defaultSearchKeyMap()
	for name, h := range map[string][2]string{
		"up":     {keys.Up.Help().Key, keys.Up.Help().Desc},
		"down":   {keys.Down.Help().Key, keys.Down.Help().Desc},
		"select": {keys.Select.Help().Key, keys.Select.Help().Desc},
		"close":  {keys.Close.Help().Key, keys.Close.Help().Desc},
		"abort":  {keys.Abort.Help().Key, keys.Abort.Help().Desc},
	} {
		if h[0] == "" || h[1] == "" {
			t.Errorf("%s help = %q/%q, want both halves", name, h[0], h[1])
		}
	}
	if !reflect.DeepEqual(keys.Delete.Keys(), []string{"backspace"}) {
		t.Errorf("delete keys = %v, want backspace", keys.Delete.Keys())
	}
}
