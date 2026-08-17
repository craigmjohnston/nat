package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/git"
)

// diffState is how far the branch's diff has got: nothing asked for yet, a read
// in flight, a diff on screen, or a read that failed.
type diffState int

const (
	diffIdle diffState = iota
	diffLoading
	diffReady
	diffFailed
)

// The diff screen's measurements: the columns the file list takes beside the
// diff, the rule between them, the gutter every body line carries its comment
// mark in, the columns a file's box spends on its own two borders, and the
// narrowest window worth splitting in two — below it the list goes and the diff
// has the band, since a diff squeezed into thirty columns says nothing at all.
const (
	diffListWidth   = 28
	diffRuleWidth   = 1
	diffGutterWidth = 2
	diffBorderWidth = 2
	diffSplitMin    = 60
)

// commentMark is what a line carrying a pending comment is marked with, in the
// gutter every body line reserves for it. One cell wide and unambiguously so,
// since it is drawn on every line of the diff and a mark that measured two
// columns in some terminals would shift the whole body in them.
const commentMark = "▌"

// diffKeyMap is what the diff screen answers to beyond the viewport's own
// scrolling: the jumps from one file's section to the next, which is what makes
// a diff of twenty files readable without hunting for the boundaries, the key
// that folds a file away once it has been read, and the three keys of a review
// — mark a range, comment on it, send what is pending.
type diffKeyMap struct {
	NextFile key.Binding
	PrevFile key.Binding
	Viewed   key.Binding
	Select   key.Binding
	Comment  key.Binding
	Send     key.Binding
}

// defaultDiffKeyMap returns the bindings the diff screen runs with. n and p are
// the file jumps rather than anything vim-shaped, because j/k are already the
// line cursor and the two pairs should not read as versions of each other; v is
// the range mark for the one reason it is in vim, and c and s are the two ends
// of leaving a comment.
//
// enter is the viewed toggle: it is the key a list answers on the thing under
// the cursor, and the one key the screen wanted that nothing else here — nor
// the viewport under it, which pages on space — had already taken.
func defaultDiffKeyMap() diffKeyMap {
	return diffKeyMap{
		NextFile: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next file")),
		PrevFile: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "previous file")),
		Viewed:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "collapse file")),
		Select:   key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "select lines")),
		Comment:  key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "comment")),
		Send:     key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "send comments")),
	}
}

// hints are the diff screen's own hints row: how to move between files, the two
// keys a comment is left and sent with, and the way back to the board.
// Scrolling is not among them — it is the keys everyone tries first — and the
// help screen lists it.
//
// The send key says how many comments are waiting, since that count is the one
// thing about a review that is nowhere else on the screen at a glance; with
// none pending it says what the key is for instead.
func (d Diff) hints(back key.Binding) []hint {
	return []hint{
		{d.keys.Comment, 4},
		{d.sendBinding(), 5},
		{d.keys.NextFile, 3},
		{d.keys.PrevFile, 2},
		{d.viewedBinding(), 2},
		{back, 1},
	}
}

// viewedBinding is the viewed toggle as the hints row names it: what the key
// will do to the file the cursor is in rather than what it does in general,
// since the one key is both halves of a fold and the row is where the user
// finds out which half is next.
func (d Diff) viewedBinding() key.Binding {
	if !d.viewedFile(d.cursor) {
		return d.keys.Viewed
	}
	return key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand file"))
}

// sendBinding is the send key as the hints row names it: the pending count when
// there is one, so the row itself is the tally of the review so far.
func (d Diff) sendBinding() key.Binding {
	n := len(d.comments)
	if n == 0 {
		return d.keys.Send
	}
	return key.NewBinding(key.WithKeys("s"),
		key.WithHelp("s", fmt.Sprintf("send %d %s", n, plural(n, "comment", "comments"))))
}

// bindings are the diff screen's keys as the help screen lists them.
func (k diffKeyMap) bindings() []key.Binding {
	return []key.Binding{k.NextFile, k.PrevFile, k.Viewed, k.Select, k.Comment, k.Send}
}

// Diff is the review screen: the unified diff of a slice's handed-back branch
// against the base it was cut from, one bordered box per file, scrolled in a
// viewport with a list of the files it touches beside it.
//
// It holds the parsed files rather than the rendered body, because the body is
// cut to the width it is drawn at: every resize renders again from the files.
// Nothing here writes anything — reading the change is the whole of it, and the
// key that acts on what was read is the board's approve.
type Diff struct {
	styles Styles
	keys   diffKeyMap
	vp     viewport.Model

	// sliceID is the page ID of the slice whose branch is on show, which is how
	// the comments find the agent they are sent to; slice, branch and dir are
	// what was asked for, kept so the refresh key can ask for it again: an agent
	// that pushes another commit while the diff is on screen is exactly when a
	// reread is wanted.
	sliceID string
	slice   string
	branch  string
	dir     string
	// base is what git diffed the branch against, which the screen says out
	// loud: a diff means little without it.
	base  string
	files []git.File

	// tops[i] is the line of the rendered body file i's box opens on — its
	// header row, which is what a file jump scrolls to — and offsets[i] the
	// first line of that box the cursor rests on: the line under the header, or
	// the header itself where the file is collapsed and that row is all it has.
	// Both are rebuilt with the body, since a resize moves every one of them.
	tops    []int
	offsets []int
	// lines says where each line of the rendered body came from, in step with
	// it: the line cursor and the comments are in body-line space, and this is
	// how a body line becomes a file and a line within it.
	lines []bodyLine
	// cursor is the file the list marks, and listTop the first file it draws —
	// a change of forty files has a longer list than the band is tall.
	cursor  int
	listTop int

	// viewed[i] is whether file i has been marked read and folded away to its
	// header row, in step with the files themselves. It is the screen's own and
	// nobody else's: a read of the branch drops it, since the diff it was an
	// opinion about may have changed underneath.
	viewed []bool

	// line is the body line the cursor is on, and anchor the other end of a
	// selected range while anchored — the two are the lines a comment is left
	// on. A range never leaves the file it was started in: a comment spanning
	// two files is two comments.
	line     int
	anchor   int
	anchored bool

	// comments are the review comments waiting to be sent, by the file and the
	// line each was left on. They live no longer than the session: nothing here
	// is written to Notion or to GitHub, and sending them is what empties this.
	comments map[commentKey]comment
	// marks is which of a file's lines a comment covers, so the gutter is one
	// lookup per line as the body is drawn. It is keyed by the file and line a
	// comment names rather than by where the body draws them, which is what
	// keeps it a map built from the comments alone.
	marks map[commentKey]bool

	state diffState
	err   error

	width, height int
}

// bodyLine is where one line of the rendered body came from: the file's index
// in the diff and the line's index within that file's section. A box's own two
// rows belong to the file they are drawn around rather than to its section, and
// say which row they are in place of a line index.
type bodyLine struct{ file, line int }

// The line indexes a box's own rows carry, in place of a line of the file's
// section: there is nothing on either to put a comment on. The header row is
// still a place the cursor rests where its file is collapsed, since it is then
// the only row that file has.
const (
	boxHeaderRow = -1
	boxFooterRow = -2
)

// NewDiff returns an empty diff screen, waiting for a branch to be read into it.
func NewDiff(styles Styles) Diff {
	return Diff{styles: styles, keys: defaultDiffKeyMap(), vp: viewport.New()}
}

// SetSize gives the screen the space it has and renders the diff again to it:
// the body's lines are cut to the width they are drawn at, so a resize is a
// re-render rather than a reflow.
func (d *Diff) SetSize(width, height int) {
	d.width, d.height = width, height
	d.vp.SetWidth(max(d.diffWidth(), 1))
	d.vp.SetHeight(max(height, 1))
	d.render()
}

// diffWidth is the columns the diff itself has: the whole band, less the file
// list and the rule when there is room for them.
func (d Diff) diffWidth() int {
	if !d.splitVisible() {
		return d.width
	}
	return d.width - diffListWidth - diffRuleWidth
}

// splitVisible reports whether the file list is drawn beside the diff. A window
// too narrow for both gives the columns to the diff, where the content is; the
// file jumps go on working either way, which is what per-file navigation
// actually needs.
func (d Diff) splitVisible() bool { return d.width >= diffSplitMin }

// Start marks a read of slice's branch as in flight, so the screen shows
// progress and whatever was on it before does not read as this branch's diff.
//
// The pending comments survive a read of the same slice's branch — that read is
// the refresh key, and an agent pushing another commit is no reason to throw
// away what the user has typed about the last one — and go with a read of any
// other, since a comment is about the lines of one branch.
func (d *Diff) Start(sliceID, slice, branch, dir string) {
	if sliceID != d.sliceID {
		d.comments = nil
	}
	d.sliceID, d.slice, d.branch, d.dir = sliceID, slice, branch, dir
	d.state, d.err = diffLoading, nil
	d.base, d.files, d.tops, d.offsets = "", nil, nil, nil
	d.viewed = nil
	d.cursor, d.listTop = 0, 0
	d.clearSelection()
	d.render()
	d.vp.GotoTop()
}

// SetFiles shows a diff that came back, from the top of the first file, and
// reports how many pending comments it could not carry over.
//
// A comment is anchored to the lines it was left on rather than to a position:
// a re-read that moved them takes it along, and one that changed or removed
// them drops it, because a comment about lines that are no longer there is one
// the agent would be sent looking for.
//
// What has been read is not carried over at all: unlike a comment, which says
// where in the file it belongs, a file marked viewed says only that the user
// has seen what was there — and what was there is exactly what a fresh read may
// have changed.
func (d *Diff) SetFiles(base string, files []git.File) int {
	d.base, d.files = base, files
	d.viewed = make([]bool, len(files))
	d.state, d.err = diffReady, nil
	d.cursor, d.listTop, d.line = 0, 0, 0
	d.clearSelection()
	dropped := d.reanchor()
	d.render()
	d.vp.GotoTop()
	return dropped
}

// reanchor moves every pending comment onto the freshly read diff, dropping the
// ones whose lines it can no longer find and reporting how many went.
func (d *Diff) reanchor() int {
	if len(d.comments) == 0 {
		return 0
	}
	kept, dropped := make(map[commentKey]comment, len(d.comments)), 0
	for _, c := range d.comments {
		f, ok := d.fileNamed(c.path)
		if !ok {
			dropped++
			continue
		}
		start, ok := findLines(d.files[f].Lines, c.lines, c.start)
		if !ok {
			dropped++
			continue
		}
		c.start = start
		c.ref = lineRef(d.files[f].Lines, start, len(c.lines))
		kept[commentKey{path: c.path, start: start}] = c
	}
	d.comments = kept
	return dropped
}

// fileNamed is the index of the file with this path in the diff on screen.
func (d Diff) fileNamed(path string) (int, bool) {
	for i, f := range d.files {
		if f.Path == path {
			return i, true
		}
	}
	return 0, false
}

// findLines is where want sits in lines: at from if it is still there, and
// otherwise at the one other place it occurs. A run that occurs twice is no
// longer an anchor — the comment cannot be re-homed without guessing which of
// them the user meant.
func findLines(lines, want []string, from int) (int, bool) {
	if matchAt(lines, want, from) {
		return from, true
	}
	found, at := 0, 0
	for i := range lines {
		if matchAt(lines, want, i) {
			found, at = found+1, i
		}
	}
	if found != 1 {
		return 0, false
	}
	return at, true
}

// matchAt reports whether want sits in lines starting at i.
func matchAt(lines, want []string, i int) bool {
	if i < 0 || i+len(want) > len(lines) {
		return false
	}
	return slices.Equal(lines[i:i+len(want)], want)
}

// Fail reports a read that did not come back. What was on screen goes with it,
// unlike the info screen's: a diff is of one branch at one moment, and leaving
// the last one up under a failure would be showing the wrong change.
func (d *Diff) Fail(err error) {
	d.state, d.err = diffFailed, err
	d.files, d.tops, d.offsets, d.viewed = nil, nil, nil, nil
	d.render()
}

// commentKey identifies a pending comment: the file it is on and the first of
// the lines it covers. The path rather than the file's index, so a re-read that
// adds or drops a file elsewhere in the diff leaves it where it was.
type commentKey struct {
	path  string
	start int
}

// comment is one pending review comment: the lines it was left on, as git wrote
// them, and what the user had to say about them.
//
// The lines are held rather than looked up, because they are what the prompt
// quotes and what re-anchors the comment onto a freshly read diff; ref is where
// in the file they sit, in the words the prompt names them by.
type comment struct {
	path  string
	start int
	lines []string
	ref   string
	text  string
}

// Comment is one pending comment as the prompt is composed from it: the file,
// where in it the lines sit, those lines, and what was said.
type Comment struct {
	Path  string
	Ref   string
	Lines []string
	Text  string
}

// Busy reports whether a read is in flight, which is what keeps the root
// model's spinner turning.
func (d Diff) Busy() bool { return d.state == diffLoading }

// Pending is how many comments are waiting to be sent.
func (d Diff) Pending() int { return len(d.comments) }

// Comments are the pending comments in the order the diff draws them: by file
// in the order the change touches them, and by line within a file. They are
// built from the comments themselves rather than from the diff on screen, so a
// read that failed after they were left does not swallow them — a comment on a
// file the diff no longer holds simply sorts after the ones it does, by path.
func (d Diff) Comments() []Comment {
	order := make(map[string]int, len(d.files))
	for i, f := range d.files {
		if _, seen := order[f.Path]; !seen {
			order[f.Path] = i
		}
	}
	rank := func(path string) int {
		if i, ok := order[path]; ok {
			return i
		}
		return len(d.files)
	}
	cs := make([]comment, 0, len(d.comments))
	for _, c := range d.comments {
		cs = append(cs, c)
	}
	slices.SortFunc(cs, func(a, b comment) int {
		if ra, rb := rank(a.path), rank(b.path); ra != rb {
			return cmp.Compare(ra, rb)
		}
		if a.path != b.path {
			return strings.Compare(a.path, b.path)
		}
		return cmp.Compare(a.start, b.start)
	})
	out := make([]Comment, len(cs))
	for i, c := range cs {
		out[i] = Comment{Path: c.path, Ref: c.ref, Lines: c.lines, Text: c.text}
	}
	return out
}

// SetComment records what the user typed about a run of lines, replacing
// whatever was on them. Text that is nothing but spaces removes the comment
// instead: an emptied comment box is how one is taken back.
func (d *Diff) SetComment(path string, start, span int, text string) {
	f, ok := d.fileNamed(path)
	if !ok || start < 0 || span <= 0 || start+span > len(d.files[f].Lines) {
		return
	}
	key := commentKey{path: path, start: start}
	if strings.TrimSpace(text) == "" {
		delete(d.comments, key)
		d.render()
		return
	}
	if d.comments == nil {
		d.comments = map[commentKey]comment{}
	}
	lines := slices.Clone(d.files[f].Lines[start : start+span])
	d.comments[key] = comment{path: path, start: start, lines: lines,
		ref: lineRef(d.files[f].Lines, start, span), text: strings.TrimSpace(text)}
	d.render()
}

// ClearComments drops every pending comment, which is what sending them does:
// they are held only until the agent has been told.
func (d *Diff) ClearComments() {
	d.comments = nil
	d.render()
}

// Selection is the file and the run of lines a comment would go on: the cursor
// line, or the whole marked range. It also hands back whatever comment is
// already there, so the box opens on what was said rather than empty.
func (d Diff) Selection() (path string, start, span int, text string, ok bool) {
	if len(d.files) == 0 || d.line >= len(d.lines) {
		return "", 0, 0, "", false
	}
	at := d.lines[d.line]
	if at.line < 0 {
		// A collapsed file's header row: there is nothing on show to say
		// anything about, and the file has to be opened before there is.
		return "", 0, 0, "", false
	}
	from, to := at.line, at.line
	if d.anchored && d.anchor < len(d.lines) && d.lines[d.anchor].file == at.file {
		from = min(from, d.lines[d.anchor].line)
		to = max(to, d.lines[d.anchor].line)
	}
	path = d.files[at.file].Path
	return path, from, to - from + 1, d.comments[commentKey{path: path, start: from}].text, true
}

// SelectionRef is where the lines a comment would go on sit in the file, which
// is what the comment box names them by.
func (d Diff) SelectionRef(path string, start, span int) string {
	f, ok := d.fileNamed(path)
	if !ok {
		return ""
	}
	return lineRef(d.files[f].Lines, start, span)
}

// ToggleSelect marks the cursor line as one end of a range, or takes the mark
// off again, and reports whether a range is now being marked.
func (d *Diff) ToggleSelect() bool {
	if d.anchored {
		d.clearSelection()
		d.render()
		return false
	}
	// A collapsed file's header row is no end of a range: the lines a range
	// would cover are the ones the fold has taken off the screen.
	if len(d.lines) == 0 || d.lines[d.line].line < 0 {
		return false
	}
	d.anchor, d.anchored = d.line, true
	d.render()
	return true
}

// Selecting reports whether a range is being marked, which is what makes esc
// mean "drop the range" rather than "leave the screen".
func (d Diff) Selecting() bool { return d.anchored }

// clearSelection takes the range mark off without redrawing, for the callers
// that are about to redraw anyway.
func (d *Diff) clearSelection() { d.anchor, d.anchored = 0, false }

// CancelSelect drops the range being marked, reporting whether there was one.
func (d *Diff) CancelSelect() bool {
	if !d.anchored {
		return false
	}
	d.clearSelection()
	d.render()
	return true
}

// Loadable reports whether there is a branch to read again — a diff the screen
// has been pointed at, which the refresh key can ask for a second time.
func (d Diff) Loadable() bool { return d.branch != "" && d.dir != "" }

// Target is the slice, branch and directory the screen was last pointed at.
func (d Diff) Target() (slice, branch, dir string) { return d.slice, d.branch, d.dir }

// SliceID is the page ID of the slice whose branch is on show, which is how the
// comments left here find the agent working it.
func (d Diff) SliceID() string { return d.sliceID }

// Reset drops the diff and what it was of — the pending comments included, since
// they are about a branch that is no longer on show — so nothing of one slice's
// branch is left on the screen the next slice's opens.
func (d *Diff) Reset() {
	*d = Diff{styles: d.styles, keys: d.keys, vp: d.vp, width: d.width, height: d.height}
	d.render()
	d.vp.GotoTop()
}

// The keys the line cursor moves on: the same ones the viewport scrolls with,
// taken before it sees them. A cursor that moved and a body that scrolled under
// a still cursor would be two answers to one key.
var (
	diffLineDown = key.NewBinding(key.WithKeys("down", "j"))
	diffLineUp   = key.NewBinding(key.WithKeys("up", "k"))
)

// Update handles the screen's keys: the line cursor, the jumps between files,
// the fold that puts a file it has been read behind its own header row,
// and otherwise the viewport's own scrolling — after which the cursor is
// brought back onto the screen, so the line a comment would go on is always one
// the user can see. Everything else — leaving the screen, commenting, sending,
// refreshing — belongs to the root model.
func (d *Diff) Update(msg tea.Msg) tea.Cmd {
	if press, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(press, diffLineDown):
			d.moveCursor(1)
			return nil
		case key.Matches(press, diffLineUp):
			d.moveCursor(-1)
			return nil
		case key.Matches(press, d.keys.NextFile):
			d.jump(1)
			return nil
		case key.Matches(press, d.keys.PrevFile):
			d.jump(-1)
			return nil
		case key.Matches(press, d.keys.Viewed):
			d.ToggleViewed()
			return nil
		}
	}
	vp, cmd := d.vp.Update(msg)
	d.vp = vp
	d.followView()
	return cmd
}

// moveCursor moves the line cursor one line up or down — delta is 1 or -1 —
// stepping over the border rows between two boxes, since there is nothing there
// to comment on, and scrolling the least it can to keep the cursor on screen.
//
// The step is a walk rather than a single hop over one row: between two files
// there is a footer and a header, and the last box's footer is the end of the
// body, where the cursor holds where it was.
func (d *Diff) moveCursor(delta int) {
	if len(d.lines) == 0 {
		return
	}
	for want := d.line; ; {
		next := d.clamp(want + delta)
		if next == want {
			return
		}
		want = next
		if d.stop(want) {
			d.setLine(want)
			return
		}
	}
}

// clamp holds a body line inside the diff, and inside the file a range is being
// marked in: a selection that ran into the next file would be a comment on two
// files at once, which is two comments.
func (d Diff) clamp(line int) int {
	line = min(max(line, 0), len(d.lines)-1)
	if !d.anchored || d.anchor >= len(d.lines) {
		return line
	}
	first, last := d.fileSpan(d.lines[d.anchor].file)
	return min(max(line, first), last)
}

// fileSpan is the first and last body line of a file's section. It is only ever
// asked about the file a range is being marked in, which is one whose lines are
// on the body: a fold clears the mark, and there is no marking one from a
// collapsed file's header row.
func (d Diff) fileSpan(file int) (first, last int) {
	first = d.offsets[file]
	return first, first + len(d.files[file].Lines) - 1
}

// setLine puts the cursor on a body line, marks the file it lands in on the
// list beside the diff, and redraws.
func (d *Diff) setLine(line int) {
	d.line = line
	if line < len(d.lines) {
		d.cursor = d.lines[line].file
		d.syncList()
	}
	d.render()
	d.scrollToCursor()
}

// scrollToCursor scrolls the body the least it can to bring the cursor line
// back onto it, and the header row of its box with it where the cursor is on the
// first line of a file: the row that names the file is worth the one line it
// costs, and a diff scrolled to an unnamed first line reads as starting nowhere.
func (d *Diff) scrollToCursor() {
	h := d.vp.Height()
	if h <= 0 {
		return
	}
	reveal := d.line
	if d.line < len(d.lines) && d.lines[d.line].line == 0 {
		reveal = max(d.line-1, 0)
	}
	switch top := d.vp.YOffset(); {
	case reveal < top:
		d.vp.SetYOffset(reveal)
	case d.line >= top+h:
		d.vp.SetYOffset(d.line - h + 1)
	}
}

// followView brings the cursor back onto the body after the viewport has been
// scrolled out from under it, which is what the page and half-page keys do.
func (d *Diff) followView() {
	h := d.vp.Height()
	if len(d.lines) == 0 || h <= 0 {
		return
	}
	top := d.vp.YOffset()
	line := min(max(d.line, top), min(top+h-1, len(d.lines)-1))
	if line == d.line {
		return
	}
	d.setLine(d.contentLine(d.clamp(line)))
}

// jump moves the cursor by delta files and scrolls the diff to the top of the
// box the one it lands on is drawn in — the header row, so the file the jump
// landed on is named on screen. The ends hold rather than wrap: a jump that came
// back round to the first file would read as having done nothing at all.
func (d *Diff) jump(delta int) {
	if len(d.files) == 0 {
		return
	}
	d.cursor = min(max(d.cursor+delta, 0), len(d.files)-1)
	if d.cursor < len(d.offsets) {
		d.vp.SetYOffset(d.tops[d.cursor])
		// The line cursor goes with the jump: it is what a comment is left on,
		// and leaving it in the file the jump was away from would be a comment
		// on a section that is no longer on screen.
		d.setLine(d.clamp(d.offsets[d.cursor]))
	}
	d.syncList()
}

// syncList scrolls the file list the least it can to bring the cursor back onto
// it, the way the board's own viewport is kept in step with its cursor.
func (d *Diff) syncList() {
	rows := d.listRows()
	if rows <= 0 {
		d.listTop = 0
		return
	}
	switch {
	case d.cursor < d.listTop:
		d.listTop = d.cursor
	case d.cursor >= d.listTop+rows:
		d.listTop = d.cursor - rows + 1
	}
	d.listTop = min(max(d.listTop, 0), max(len(d.files)-rows, 0))
}

// listRows is the files the list has room to draw: every line of the band but
// the heading that names how many there are.
func (d Diff) listRows() int { return max(d.height-1, 0) }

// render rebuilds the viewport's content from the files at the current width:
// one bordered box per file, its header row naming the path and its footer row
// closing it, and the file's diff between them with the line numbers of either
// side down the left. Where each file's diff starts and where each body line
// came from are recorded as it goes.
func (d *Diff) render() {
	if len(d.files) == 0 {
		d.tops, d.offsets, d.lines, d.marks = nil, nil, nil, nil
		d.vp.SetContent("")
		return
	}
	inner := max(d.diffWidth()-diffBorderWidth, 1)
	nums := d.lineNumbers()
	numWidth := numberWidth(nums)

	tops, offsets := make([]int, len(d.files)), make([]int, len(d.files))
	var from []bodyLine
	for i, f := range d.files {
		tops[i] = len(from)
		from = append(from, bodyLine{file: i, line: boxHeaderRow})
		if d.viewedFile(i) {
			// A collapsed file is its header row and nothing else: no diff lines
			// to walk, and no footer row, since there is no interior to close.
			offsets[i] = tops[i]
			continue
		}
		offsets[i] = len(from)
		for j := range f.Lines {
			from = append(from, bodyLine{file: i, line: j})
		}
		from = append(from, bodyLine{file: i, line: boxFooterRow})
	}
	d.tops, d.offsets, d.lines = tops, offsets, from
	d.marks = d.commentMarks()
	d.line = d.contentLine(d.line)

	lines := make([]string, 0, len(from))
	for i, f := range d.files {
		lines = append(lines, d.boxTop(f, inner, d.viewedFile(i)))
		if d.viewedFile(i) {
			continue
		}
		for j, line := range f.Lines {
			lines = append(lines, d.boxLine(line, nums[i].was[j], nums[i].now[j], numWidth, inner,
				d.marks[commentKey{path: f.Path, start: j}], d.selected(len(lines))))
		}
		lines = append(lines, d.boxBottom(inner))
	}
	d.vp.SetContent(strings.Join(lines, "\n"))
}

// viewedFile reports whether a file has been marked read, which is what folds
// its box away to the header row.
func (d Diff) viewedFile(i int) bool { return i >= 0 && i < len(d.viewed) && d.viewed[i] }

// ToggleViewed marks the file the cursor is in as read, folding its box away to
// the header row — or unfolds it again — and reports which of the two it did.
// The cursor goes to the top of the box either way: the row that names the file
// is all a collapsed one has, and the first of its lines is where an unfolded
// one is read from.
func (d *Diff) ToggleViewed() bool {
	if d.cursor < 0 || d.cursor >= len(d.viewed) {
		return false
	}
	d.viewed[d.cursor] = !d.viewed[d.cursor]
	// A range being marked in the file cannot survive its lines going away, and
	// a range marked anywhere else would be no easier to see once the body has
	// moved under it.
	d.clearSelection()
	d.render()
	d.setLine(d.offsets[d.cursor])
	return d.viewed[d.cursor]
}

// ToggleViewedAt folds the box a body line belongs to, for the click that
// landed on one of that box's own two rows, and reports whether it was one of
// them: a click on a line of the diff itself is not a fold.
func (d *Diff) ToggleViewedAt(line int) bool {
	if line < 0 || line >= len(d.lines) || d.lines[line].line >= 0 {
		return false
	}
	d.cursor = d.lines[line].file
	d.ToggleViewed()
	return true
}

// LineAt is the body line drawn at a cell of the band the screen has, and
// whether that cell is on the diff at all — the file list beside it is not, and
// nor is a row past the end of the body.
func (d Diff) LineAt(col, row int) (int, bool) {
	if row < 0 || row >= d.vp.Height() {
		return 0, false
	}
	if d.splitVisible() {
		col -= diffListWidth + diffRuleWidth
	}
	if col < 0 || col >= d.diffWidth() {
		return 0, false
	}
	if line := d.vp.YOffset() + row; line < len(d.lines) {
		return line, true
	}
	return 0, false
}

// contentLine is the nearest body line the cursor rests on, searched forward
// from line and then back: a box's footer row is furniture, and so is the header
// row of a box that is open, with nothing on either to put the cursor on.
func (d Diff) contentLine(line int) int {
	line = min(max(line, 0), max(len(d.lines)-1, 0))
	for i := line; i < len(d.lines); i++ {
		if d.stop(i) {
			return i
		}
	}
	for i := line - 1; i >= 0; i-- {
		if d.stop(i) {
			return i
		}
	}
	return 0
}

// stop reports whether the cursor rests on the body line at i, which every
// caller has already held inside the body: a line of a file's diff, or the
// header row of a collapsed file, which is the only row that file has and so
// the only place a cursor moving through it can be.
func (d Diff) stop(i int) bool {
	at := d.lines[i]
	return at.line >= 0 || (at.line == boxHeaderRow && d.viewedFile(at.file))
}

// commentMarks is which of a file's lines a pending comment covers, so the
// gutter is one lookup per line rather than a walk of every comment.
func (d Diff) commentMarks() map[commentKey]bool {
	if len(d.comments) == 0 {
		return nil
	}
	marks := map[commentKey]bool{}
	for _, c := range d.comments {
		for i := range c.lines {
			marks[commentKey{path: c.path, start: c.start + i}] = true
		}
	}
	return marks
}

// selected reports whether a body line is under the cursor, or inside the range
// being marked from it.
func (d Diff) selected(i int) bool {
	if !d.anchored {
		return i == d.line
	}
	return i >= min(d.line, d.anchor) && i <= max(d.line, d.anchor)
}

// lineStyle is the style a diff line is drawn in, chosen by its prefix. The
// header lines are tested before the +/- ones they look like: "+++ b/main.go"
// is a header, not three added characters.
func (d Diff) lineStyle(line string) lipgloss.Style {
	switch {
	case strings.HasPrefix(line, "diff --git "):
		return d.styles.DiffFile
	case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		return d.styles.DiffMeta
	case strings.HasPrefix(line, "@@"):
		return d.styles.DiffHunk
	case strings.HasPrefix(line, "+"):
		return d.styles.DiffAdd
	case strings.HasPrefix(line, "-"):
		return d.styles.DiffDel
	case strings.HasPrefix(line, " "), line == "":
		return lipgloss.NewStyle()
	default:
		// index, mode, rename and similarity lines, and git's note about a file
		// it would not diff: all of them are about the change rather than in it.
		return d.styles.DiffMeta
	}
}

// View renders the screen's body; the layout's header is what names it. spinner
// is the root model's current frame, drawn while the read is in flight so the
// app turns one spinner rather than two.
func (d Diff) View(spinner string) string {
	switch d.state {
	case diffIdle:
		return d.styles.Faint.Render("Move to a handed-back slice and press v to read its diff.")
	case diffLoading:
		return spinner + fmt.Sprintf(" Reading the diff of %s…", d.branch)
	case diffFailed:
		return d.styles.Error.Render(oneLine(d.err.Error()))
	}
	if len(d.files) == 0 {
		return d.styles.Faint.Render(fmt.Sprintf("%s has no changes against %s.", d.branch, d.base))
	}
	if !d.splitVisible() {
		return d.vp.View()
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, d.listView(), d.ruleView(), d.vp.View())
}

// listView is the file list beside the diff: how many files the branch touches
// and what it diffed against, then a row per file with its ± tally, the one the
// cursor is on filled the way the board fills its selected row.
func (d Diff) listView() string {
	rows := d.listRows()
	lines := make([]string, 0, rows+1)
	lines = append(lines, d.styles.DiffMeta.Render(fit(d.listHeading(), diffListWidth)))
	for i := d.listTop; i < len(d.files) && len(lines) <= rows; i++ {
		lines = append(lines, d.fileRow(i))
	}
	for len(lines) < rows+1 {
		lines = append(lines, strings.Repeat(" ", diffListWidth))
	}
	return strings.Join(lines, "\n")
}

// listHeading says what the list is of: the files the branch touches, and the
// base they are diffed against.
func (d Diff) listHeading() string {
	return fmt.Sprintf("%d %s vs %s", len(d.files), plural(len(d.files), "file", "files"), d.base)
}

// plural picks the word for a count, so a change of one file does not report
// "1 files".
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// fileRow is one file of the list: its path, elided from the left so the name
// survives where the directories do not, and its ± tally on the right. The row
// under the cursor is filled across the list's whole width, so it reads as the
// section the diff beside it is scrolled to.
func (d Diff) fileRow(i int) string {
	f := d.files[i]
	counts := d.fileCounts(f)
	name := elideLeft(f.Path, max(diffListWidth-lipgloss.Width(counts)-1, 1))
	gap := max(diffListWidth-lipgloss.Width(name)-lipgloss.Width(counts), 1)
	if i == d.cursor {
		// The fill is one style across the row, so the tally is drawn plain: its
		// own colour would break the run of background the way a chip's does.
		return d.styles.SelectedRow.Width(diffListWidth).Render(
			fit(name+strings.Repeat(" ", gap)+plainCounts(f), diffListWidth))
	}
	return fit(name+strings.Repeat(" ", gap)+counts, diffListWidth)
}

// fileCounts is a file's ± tally as the list draws it: added in the diff's
// green, removed in its red, and a binary file said in words, since it has
// neither.
func (d Diff) fileCounts(f git.File) string {
	if f.Binary {
		return d.styles.DiffCount.Render("bin")
	}
	return d.styles.DiffAdd.Render(fmt.Sprintf("+%d", f.Added)) + " " +
		d.styles.DiffDel.Render(fmt.Sprintf("-%d", f.Removed))
}

// plainCounts is the same tally without its colours, for the selected row's
// fill.
func plainCounts(f git.File) string {
	if f.Binary {
		return "bin"
	}
	return fmt.Sprintf("+%d -%d", f.Added, f.Removed)
}

// ruleView is the vertical rule between the file list and the diff.
func (d Diff) ruleView() string {
	rows := d.listRows() + 1
	lines := make([]string, max(rows, 1))
	for i := range lines {
		lines[i] = d.styles.DiffRule.Render("│")
	}
	return strings.Join(lines, "\n")
}

// elideLeft cuts a path to width columns from the left, keeping the tail: two
// files in the same deep directory differ at the end of their paths, not the
// start, and it is the end that names them.
func elideLeft(path string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(path) <= width {
		return path
	}
	if width == 1 {
		return "…"
	}
	runes := []rune(path)
	// Trim from the front until what is left fits beside the ellipsis.
	for i := range runes {
		if tail := string(runes[i:]); lipgloss.Width(tail)+1 <= width {
			return "…" + tail
		}
	}
	return "…"
}
