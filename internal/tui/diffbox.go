package tui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/git"
)

// The border characters a file's box is drawn with: the rounded set the app's
// other boxes take, built by hand here rather than through lipgloss's own
// border, because the box's rows are the diff's own lines and a bordered block
// would re-wrap them.
const (
	boxTopLeft     = "╭"
	boxTopRight    = "╮"
	boxBottomLeft  = "╰"
	boxBottomRight = "╯"
	boxSide        = "│"
	boxRule        = "─"
	// boxBreakRule is the rule the break between two hunks is drawn with: one
	// cell wide like every other border character here, and dashed, so it reads
	// as a gap in the file rather than an edge of the box.
	boxBreakRule = "┈"
)

// viewedMark is what the header row of a file marked viewed opens with, in the
// cell the rule would otherwise have. One column wide and unambiguously so, like
// the comment mark, since the row it is on is measured to the box's own width.
//
// A collapsed box keeps the top corners it opened with rather than borrowing the
// bottom ones: the row is its header, and the file's own diff is not gone but
// folded, which is what the user presses the key again to undo.
const viewedMark = "✓"

// sideNumbers is the line each of one file's diff lines sits at on either side
// of the change, as the gutter draws them: was in the base, now in the file as
// the branch leaves it. A line only one side has is numbered only there, and a
// line no hunk covers — a header, a hunk marker, git's note about a file it
// would not diff — by neither, which is a zero and draws as nothing.
type sideNumbers struct{ was, now []int }

// lineNumbers is the numbers of every file in the diff, read off the hunk
// headers git wrote. They are the same numbers a comment names its lines by, so
// what the gutter shows and what the agent is told to open are one answer.
func (d Diff) lineNumbers() []sideNumbers {
	nums := make([]sideNumbers, len(d.files))
	for i, f := range d.files {
		now, was := lineNumbers(f.Lines)
		nums[i] = sideNumbers{was: was, now: now}
	}
	return nums
}

// numberWidth is the columns one of the two line-number columns takes: the
// digits of the largest number anywhere in the diff, so a file's code starts at
// the same column in every box rather than shifting from one to the next.
func numberWidth(nums []sideNumbers) int {
	widest := 0
	for _, n := range nums {
		for _, side := range [][]int{n.was, n.now} {
			for _, at := range side {
				widest = max(widest, at)
			}
		}
	}
	return len(strconv.Itoa(widest))
}

// boxTop is the header row of a file's box: the path it names on the left, the
// ± tally on the right, and the rule between them run out to the columns the
// box has. inner is the width of the box's interior, its own two borders off.
//
// The path is elided from the left where it will not fit, since what names a
// file is the end of its path; the tally goes altogether where there is no room
// for both, because the list beside the diff carries it too and the path is what
// the box is for.
//
// A file marked viewed takes the first cell of the rule for a tick, so the row
// says it has been read as well as showing it — the fold below it is the same
// news, but a row that is the whole of its box is a row with nothing below it to
// compare against. It costs the rule a column and no more: the path and the
// tally sit where they sat, so a fold does not shuffle the row it collapses.
func (d Diff) boxTop(f git.File, inner int, viewed bool) string {
	edge := d.styles.DiffRule
	head := edge.Render(boxRule)
	if viewed {
		head = d.styles.DiffAdd.Render(viewedMark)
	}
	// The tally spends its own two spaces and a closing rule character, and
	// leaves at least a rule of two between itself and the path; without room for
	// all of that the path has the row to itself.
	tally, tallyWidth := "", 0
	if counts := plainCounts(f); lipgloss.Width(counts)+9 <= inner {
		tally = " " + d.fileCounts(f) + edge.Render(" "+boxRule)
		tallyWidth = lipgloss.Width(counts) + 3
	}
	name := elideLeft(f.Path, max(inner-tallyWidth-5, 0))
	rule := max(inner-lipgloss.Width(name)-tallyWidth-3, 0)
	return d.boxRow(boxTopLeft, head+" "+d.styles.DiffFile.Render(name)+" "+
		edge.Render(strings.Repeat(boxRule, rule))+tally, boxTopRight, inner)
}

// boxBottom is the footer row that closes a file's box.
func (d Diff) boxBottom(inner int) string {
	return d.boxRow(boxBottomLeft, d.styles.DiffRule.Render(strings.Repeat(boxRule, inner)),
		boxBottomRight, inner)
}

// boxLine draws one row of a file's diff inside its box: the gutter that says
// whether a comment is pending on it, the line's number on either side of the
// change, and text — one row's worth of a line, already wrapped to the columns
// the box leaves it — in the style its line was measured for.
//
// The style comes from the caller rather than from the text, because a wrapped
// line is drawn a row at a time and only the first of those rows still carries
// the +/- that says what the line is: a continuation styled by its own leading
// character would read as a line of its own, added or removed on its own account.
//
// A selected line is filled across the box's interior the way the board fills
// the row under its cursor, and drawn plain underneath: a line's own colour
// would break the run of background, exactly as a chip's does there.
func (d Diff) boxLine(text string, style lipgloss.Style, was, now, numWidth, inner int,
	marked, selected bool) string {
	nums := numberCell(was, numWidth) + " " + numberCell(now, numWidth) + " "
	gutter := " "
	if marked {
		gutter = commentMark
	}
	if selected {
		// The fill is one style across the interior, so the mark and the numbers
		// inside it are drawn plain: their own colours would break the run of
		// background, exactly as a chip's does on the board's selected row.
		return d.boxRow(boxSide, d.styles.SelectedRow.Render(cell(gutter+" "+nums+text, inner)),
			boxSide, inner)
	}
	if marked {
		gutter = d.styles.DiffComment.Render(gutter)
	}
	return d.boxRow(boxSide, gutter+" "+d.styles.DiffCount.Render(nums)+
		style.Render(text), boxSide, inner)
}

// boxBreak draws the row that stands where a hunk header was: a dashed rule
// across the box's interior, so a jump in the gutter's numbers is not the only
// sign that the lines between two hunks were skipped. It is drawn in the colour
// the hunk header itself was, since it says exactly what that line said — and
// dashed rather than solid, so it is not read as a box's own edge.
func (d Diff) boxBreak(inner int) string {
	return d.boxRow(boxSide, d.styles.DiffHunk.Render(strings.Repeat(boxBreakRule, inner)),
		boxSide, inner)
}

// commentLine draws one row of a pending comment, inside the box of the file it
// was left on and under the last of the lines it covers. It keeps the mark in
// the gutter and spends the two line-number columns on nothing, so the text
// starts where the code does and the whole comment reads as hanging off the
// lines above it — a comment is about the file rather than in it, and numbering
// it would say otherwise.
func (d Diff) commentLine(text string, numWidth, inner int) string {
	indent := strings.Repeat(" ", 2*numWidth+2)
	return d.boxRow(boxSide, d.styles.DiffComment.Render(commentMark)+" "+indent+
		d.styles.DiffCommentText.Render(text), boxSide, inner)
}

// boxRow is one row of a file's box: the interior held to exactly the columns
// the box has, between the two border characters that close it.
func (d Diff) boxRow(left, interior, right string, inner int) string {
	edge := d.styles.DiffRule
	return edge.Render(left) + cell(interior, inner) + edge.Render(right)
}

// numberCell is one line-number column: the number right-aligned in it, and
// nothing at all where that side of the change does not have the line.
func numberCell(at, width int) string {
	if at == 0 {
		return strings.Repeat(" ", width)
	}
	n := strconv.Itoa(at)
	return strings.Repeat(" ", max(width-len(n), 0)) + n
}

// cell is s cut and padded to exactly width columns, so a box's right border
// lands in the same column on every one of its rows.
func cell(s string, width int) string {
	s = fit(s, width)
	return s + strings.Repeat(" ", max(width-lipgloss.Width(s), 0))
}
