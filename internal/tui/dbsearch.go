package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/notion"
)

// searchEvent is what one key press in the search view produced. At most one
// of the outcomes is set.
type searchEvent struct {
	// result is the hit under the cursor when it was chosen.
	result *notion.SearchResult
	chosen bool
	// closed is escape: back to the tree, which kept its state.
	closed  bool
	aborted bool
	// queried is set when the key changed the query, so a fresh search is due.
	queried bool
}

// searchKeyMap is the search view's bindings. Movement is on the arrows alone:
// every printable key belongs to the query being typed.
type searchKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Select key.Binding
	Delete key.Binding
	Close  key.Binding
	Abort  key.Binding
}

// defaultSearchKeyMap returns the bindings the search view runs with.
func defaultSearchKeyMap() searchKeyMap {
	return searchKeyMap{
		Up:     key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up")),
		Down:   key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down")),
		Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Delete: key.NewBinding(key.WithKeys("backspace")),
		Close:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Abort:  key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	}
}

// searchPicker is the flat, ranked view the tree picker opens on "/": a query
// typed against Notion's search, filtered to data sources, over the whole
// workspace at once. It exists because browsing only finds a database whose
// page the user can already name, and each hit carries the trail of pages above
// it so two databases of the same name are still told apart.
type searchPicker struct {
	styles Styles
	keys   searchKeyMap

	query   string
	results []notion.SearchResult
	// crumbs holds the ancestor trail resolved for a result, keyed by its ID,
	// and asked the ones a resolve has already been issued for — walking a
	// parent chain costs a request per step, so it happens once per result and
	// only for the rows on screen.
	crumbs  map[string][]string
	asked   map[string]bool
	cursor  int
	loading bool
	err     string

	width  int
	height int
}

// newSearchPicker returns the search view, with the first search already
// counted as in flight: it opens on the unfiltered query rather than on a blank
// list.
func newSearchPicker(styles Styles, width, height int) *searchPicker {
	return &searchPicker{
		styles:  styles,
		keys:    defaultSearchKeyMap(),
		crumbs:  map[string][]string{},
		asked:   map[string]bool{},
		loading: true,
		width:   width,
		height:  height,
	}
}

// SetSize tells the view how much window it has to draw in.
func (s *searchPicker) SetSize(width, height int) { s.width, s.height = width, height }

// Handle applies one key press.
func (s *searchPicker) Handle(msg tea.KeyPressMsg) searchEvent {
	switch {
	case key.Matches(msg, s.keys.Abort):
		return searchEvent{aborted: true}
	case key.Matches(msg, s.keys.Close):
		return searchEvent{closed: true}
	case key.Matches(msg, s.keys.Up):
		s.cursor = max(0, s.cursor-1)
	case key.Matches(msg, s.keys.Down):
		s.cursor = min(max(0, len(s.results)-1), s.cursor+1)
	case key.Matches(msg, s.keys.Select):
		if s.cursor < len(s.results) {
			hit := s.results[s.cursor]
			return searchEvent{result: &hit, chosen: true}
		}
	case key.Matches(msg, s.keys.Delete):
		if q := []rune(s.query); len(q) > 0 {
			s.setQuery(string(q[:len(q)-1]))
			return searchEvent{queried: true}
		}
	default:
		if msg.Text != "" {
			s.setQuery(s.query + msg.Text)
			return searchEvent{queried: true}
		}
	}
	return searchEvent{}
}

// setQuery records what is being searched for. The hits already on show are
// kept until the new ones arrive: a list that emptied on every keystroke would
// flicker its way through a word being typed.
func (s *searchPicker) setQuery(query string) {
	s.query, s.loading, s.err = query, true, ""
}

// SetResults shows what a search returned, keeping the cursor as near to where
// the user left it as the new list allows.
func (s *searchPicker) SetResults(results []notion.SearchResult) {
	s.results, s.loading, s.err = results, false, ""
	s.cursor = max(0, min(s.cursor, len(results)-1))
}

// SetError shows why a search returned nothing. It is not fatal to the wizard:
// the next key typed issues another search, which is the natural retry.
func (s *searchPicker) SetError(err error) {
	s.loading, s.err = false, err.Error()
}

// SetCrumb records the ancestor trail resolved for one result.
func (s *searchPicker) SetCrumb(id string, trail []string) { s.crumbs[id] = trail }

// pending returns the results on screen whose trail has not been asked for yet,
// marking them asked. Calling it after anything that can change what is visible
// — new results, a moved cursor, a resize — is what keeps the resolves to the
// rows that need them.
func (s *searchPicker) pending() []notion.SearchResult {
	var out []notion.SearchResult
	for _, r := range s.visible() {
		if s.asked[r.ID] {
			continue
		}
		s.asked[r.ID] = true
		out = append(out, r)
	}
	return out
}

// searchChromeHeight is what the view draws besides the rows: the title, the
// query line, the blank line under it, and the help line.
const searchChromeHeight = 4

// View draws the search: heading, the query being typed, the hits with their
// trails, and the keys.
func (s *searchPicker) View() string {
	var b strings.Builder
	b.WriteString(fit(s.styles.Title.Render("Search for a project database"), s.width) + "\n")
	b.WriteString(fit(s.styles.Cursor.Render("/ ")+s.query+s.styles.Faint.Render("▏"), s.width) + "\n\n")

	rows, start := s.window()
	switch {
	case s.err != "":
		b.WriteString(fit(s.styles.Error.Render(oneLine(s.err)), s.width) + "\n")
	case len(rows) == 0 && s.loading:
		b.WriteString(fit(s.styles.Faint.Render("Searching…"), s.width) + "\n")
	case len(rows) == 0:
		b.WriteString(fit(s.styles.Faint.Render("No databases match."), s.width) + "\n")
	default:
		for i, r := range rows {
			b.WriteString(fit(s.row(r, start+i), s.width) + "\n")
		}
	}

	help := make([]string, 0, 4)
	for _, k := range []key.Binding{s.keys.Up, s.keys.Down, s.keys.Select, s.keys.Close} {
		help = append(help, s.styles.HelpKey.Render(k.Help().Key)+" "+s.styles.HelpDesc.Render(k.Help().Desc))
	}
	b.WriteString(fit(strings.Join(help, "  "), s.width))
	return b.String()
}

// row draws one hit: its name, then the trail of pages it lives under.
func (s *searchPicker) row(r notion.SearchResult, i int) string {
	cursor, style := "  ", lipgloss.NewStyle()
	if i == s.cursor {
		cursor, style = s.styles.Cursor.Render("> "), s.styles.Selected
	}
	label := resultLabel(r)
	line := cursor + style.Render(label)
	if trail := s.crumbText(r.ID, label); trail != "" {
		line += s.styles.Faint.Render(" — " + trail)
	}
	return line
}

// crumbText is the trail drawn after a hit, and the ellipsis placeholder while
// its parent chain is still being walked — every visible row says where it
// lives, even before the answer is in. The database holding the data source is
// dropped where the two share a name, which is the ordinary case and would
// otherwise repeat the hit's own label on every row.
func (s *searchPicker) crumbText(id, label string) string {
	trail, ok := s.crumbs[id]
	if !ok {
		return notion.BreadcrumbEllipsis
	}
	if n := len(trail); n > 0 && trail[n-1] == label {
		trail = trail[:n-1]
	}
	return strings.Join(trail, " / ")
}

// visible is the hits the height allows to be drawn.
func (s *searchPicker) visible() []notion.SearchResult {
	rows, _ := s.window()
	return rows
}

// window is the slice of hits the height allows, slid so the cursor is always
// inside it, and the index the first of them is drawn at. An unmeasured window
// shows everything.
func (s *searchPicker) window() ([]notion.SearchResult, int) {
	room := s.height - searchChromeHeight
	if s.height <= 0 || room >= len(s.results) {
		return s.results, 0
	}
	room = max(room, 1)
	start := min(max(0, s.cursor-room/2), len(s.results)-room)
	return s.results[start : start+room], start
}
