package tui

import (
	"fmt"
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
// diff, the rule between them, and the narrowest window worth splitting in two
// — below it the list goes and the diff has the band, since a diff squeezed
// into thirty columns says nothing at all.
const (
	diffListWidth = 28
	diffRuleWidth = 1
	diffSplitMin  = 60
)

// diffKeyMap is what the diff screen answers to beyond the viewport's own
// scrolling: the jumps from one file's section to the next, which is what makes
// a diff of twenty files readable without hunting for the boundaries.
type diffKeyMap struct {
	NextFile key.Binding
	PrevFile key.Binding
}

// defaultDiffKeyMap returns the bindings the diff screen runs with. n and p are
// the file jumps rather than anything vim-shaped, because j/k are already the
// scroll and the two pairs should not read as versions of each other.
func defaultDiffKeyMap() diffKeyMap {
	return diffKeyMap{
		NextFile: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next file")),
		PrevFile: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "previous file")),
	}
}

// hints are the diff screen's own hints row: how to move between files, and the
// way back to the board. Scrolling is not among them — it is the keys everyone
// tries first — and the help screen lists it.
func (k diffKeyMap) hints(back key.Binding) []hint {
	return []hint{{k.NextFile, 3}, {k.PrevFile, 2}, {back, 1}}
}

// bindings are the diff screen's keys as the help screen lists them.
func (k diffKeyMap) bindings() []key.Binding {
	return []key.Binding{k.NextFile, k.PrevFile}
}

// Diff is the review screen: the unified diff of a slice's handed-back branch
// against the base it was cut from, scrolled in a viewport with a list of the
// files it touches beside it.
//
// It holds the parsed files rather than the rendered body, because the body is
// cut to the width it is drawn at: every resize renders again from the files.
// Nothing here writes anything — reading the change is the whole of it, and the
// key that acts on what was read is the board's approve.
type Diff struct {
	styles Styles
	keys   diffKeyMap
	vp     viewport.Model

	// slice, branch and dir are what was asked for, kept so the refresh key can
	// ask for it again: an agent that pushes another commit while the diff is on
	// screen is exactly when a reread is wanted.
	slice  string
	branch string
	dir    string
	// base is what git diffed the branch against, which the screen says out
	// loud: a diff means little without it.
	base  string
	files []git.File

	// offsets[i] is the line of the rendered body file i starts on, which is
	// what the file jumps scroll to and what the list's cursor is kept in step
	// with. It is rebuilt with the body, since a resize moves every one of them.
	offsets []int
	// cursor is the file the list marks, and listTop the first file it draws —
	// a change of forty files has a longer list than the band is tall.
	cursor  int
	listTop int

	state diffState
	err   error

	width, height int
}

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
func (d *Diff) Start(slice, branch, dir string) {
	d.slice, d.branch, d.dir = slice, branch, dir
	d.state, d.err = diffLoading, nil
	d.base, d.files, d.offsets = "", nil, nil
	d.cursor, d.listTop = 0, 0
	d.render()
	d.vp.GotoTop()
}

// SetFiles shows a diff that came back, from the top of the first file.
func (d *Diff) SetFiles(base string, files []git.File) {
	d.base, d.files = base, files
	d.state, d.err = diffReady, nil
	d.cursor, d.listTop = 0, 0
	d.render()
	d.vp.GotoTop()
}

// Fail reports a read that did not come back. What was on screen goes with it,
// unlike the info screen's: a diff is of one branch at one moment, and leaving
// the last one up under a failure would be showing the wrong change.
func (d *Diff) Fail(err error) {
	d.state, d.err = diffFailed, err
	d.files, d.offsets = nil, nil
	d.render()
}

// Busy reports whether a read is in flight, which is what keeps the root
// model's spinner turning.
func (d Diff) Busy() bool { return d.state == diffLoading }

// Loadable reports whether there is a branch to read again — a diff the screen
// has been pointed at, which the refresh key can ask for a second time.
func (d Diff) Loadable() bool { return d.branch != "" && d.dir != "" }

// Target is the slice, branch and directory the screen was last pointed at.
func (d Diff) Target() (slice, branch, dir string) { return d.slice, d.branch, d.dir }

// Reset drops the diff and what it was of, so nothing of one slice's branch is
// left on the screen the next slice's opens.
func (d *Diff) Reset() {
	*d = Diff{styles: d.styles, keys: d.keys, vp: d.vp, width: d.width, height: d.height}
	d.render()
	d.vp.GotoTop()
}

// Update handles the screen's keys: the jumps between files, and otherwise the
// viewport's own scrolling. Everything else — leaving the screen, refreshing —
// belongs to the root model.
func (d *Diff) Update(msg tea.Msg) tea.Cmd {
	if press, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(press, d.keys.NextFile):
			d.jump(1)
			return nil
		case key.Matches(press, d.keys.PrevFile):
			d.jump(-1)
			return nil
		}
	}
	vp, cmd := d.vp.Update(msg)
	d.vp = vp
	return cmd
}

// jump moves the cursor by delta files and scrolls the diff to the top of the
// one it lands on. The ends hold rather than wrap: a jump that came back round
// to the first file would read as having done nothing at all.
func (d *Diff) jump(delta int) {
	if len(d.files) == 0 {
		return
	}
	d.cursor = min(max(d.cursor+delta, 0), len(d.files)-1)
	if d.cursor < len(d.offsets) {
		d.vp.SetYOffset(d.offsets[d.cursor])
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

// render rebuilds the viewport's content from the files at the current width,
// recording where each file's section starts as it goes.
func (d *Diff) render() {
	if len(d.files) == 0 {
		d.offsets = nil
		d.vp.SetContent("")
		return
	}
	width := max(d.diffWidth(), 1)
	offsets := make([]int, len(d.files))
	var lines []string
	for i, f := range d.files {
		if i > 0 {
			// A blank line between sections, so two files do not run together.
			lines = append(lines, "")
		}
		offsets[i] = len(lines)
		for _, line := range f.Lines {
			lines = append(lines, d.styleLine(line, width))
		}
	}
	d.offsets = offsets
	d.vp.SetContent(strings.Join(lines, "\n"))
}

// styleLine colours one line of the diff by its shape and cuts it to the
// columns the body has.
//
// A long line is truncated rather than wrapped, so that one line of the diff is
// one line of the body: the file jumps scroll to a line number, and a body
// whose lines did not correspond to git's would send them to the wrong place.
// This is a first-pass read-only viewer, and a truncated line is a line you can
// see is long.
func (d Diff) styleLine(line string, width int) string {
	return d.lineStyle(line).Render(fit(line, width))
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
