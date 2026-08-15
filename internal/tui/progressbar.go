package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/domain"
)

// The bar is drawn out of one half-height cell rune — whether a cell is
// finished history, the bright fill of the milestone in progress, or work
// still to do is told by colour alone — and a plain gap where one milestone
// hands over to the next.
const (
	barCell     = "▄"
	barBoundary = " "
)

// ProgressSegment is one milestone's share of the bar: its name, for the label,
// and the tally that decides how wide it is and how much of it is filled.
type ProgressSegment struct {
	Name     string
	Progress domain.Progress
}

// SegmentsOf is the board's groups as bar segments, in board order.
func SegmentsOf(groups []domain.Group) []ProgressSegment {
	segments := make([]ProgressSegment, len(groups))
	for i, g := range groups {
		segments[i] = ProgressSegment{Name: g.Name(), Progress: g.Progress()}
	}
	return segments
}

// RenderProgressBar draws a horizontal progress bar segmented by milestone,
// exactly width cells wide, with a label under it — two lines in all.
//
// Each segment is as wide as its share of the project's slices, never narrower
// than one cell, and filled in proportion to how much of it is done. Finished
// milestones recede: they draw in a dimmed fill and merge into one unbroken
// run, so the eye lands on the bright milestone the work is in. Gaps are drawn
// only where they say something — around segments with work left in them.
//
// Milestones with no slices are not drawn: there is nothing to be done in them,
// and a cell each would be cells taken from the milestones that hold the work.
// When the bar is too narrow to give every remaining milestone a cell, it
// degrades to a single unsegmented bar over the project as a whole rather than
// dropping milestones silently.
//
// This is a pure function of its arguments — it is the whole component.
func RenderProgressBar(styles Styles, width int, segments []ProgressSegment) string {
	if width <= 0 {
		return ""
	}
	return renderBar(styles, width, drawable(segments)) + "\n" +
		renderBarLabel(styles, width, segments)
}

// drawable is the segments that get cells: the ones with slices in them.
func drawable(segments []ProgressSegment) []ProgressSegment {
	out := make([]ProgressSegment, 0, len(segments))
	for _, s := range segments {
		if !s.Progress.Empty() {
			out = append(out, s)
		}
	}
	return out
}

// totalProgress tallies every segment, drawn or not.
func totalProgress(segments []ProgressSegment) domain.Progress {
	var total domain.Progress
	for _, s := range segments {
		total.Todo += s.Progress.Todo
		total.Claimed += s.Progress.Claimed
		total.Done += s.Progress.Done
		total.Total += s.Progress.Total
	}
	return total
}

// renderBar draws the bar line itself.
func renderBar(styles Styles, width int, segments []ProgressSegment) string {
	// Nothing to track yet: an empty bar, rather than a blank line, so the
	// board's shape does not change once the first slice is queued.
	if len(segments) == 0 {
		return repeat(styles.BarEmpty, barCell, width)
	}

	boundaries := boundaryCount(segments)
	cells := width - boundaries
	if cells < len(segments) {
		// Not enough room for the gaps; drop them all before dropping cells.
		boundaries, cells = 0, width
	}
	if cells < len(segments) {
		return renderSegment(styles, totalProgress(segments), width)
	}

	counts := make([]int, len(segments))
	for i, s := range segments {
		counts[i] = s.Progress.Total
	}
	widths := shareOut(cells, counts)

	var b strings.Builder
	for i, s := range segments {
		b.WriteString(renderSegment(styles, s.Progress, widths[i]))
		if boundaries > 0 && i+1 < len(segments) && !merges(segments[i], segments[i+1]) {
			b.WriteString(barBoundary)
		}
	}
	return b.String()
}

// finished reports whether a segment has nothing left in it.
func finished(s ProgressSegment) bool {
	return s.Progress.Done == s.Progress.Total
}

// merges reports whether two neighbouring segments draw as one, which two
// finished ones do: the boundary between them would only chop history up, and
// the Done section under the bar already counts the milestones. It is the one
// answer both the count of the gaps and the drawing of them are read from.
func merges(a, b ProgressSegment) bool { return finished(a) && finished(b) }

// boundaryCount is how many gaps the bar draws: one for each adjacent pair
// that does not merge.
func boundaryCount(segments []ProgressSegment) int {
	n := 0
	for i := 0; i+1 < len(segments); i++ {
		if !merges(segments[i], segments[i+1]) {
			n++
		}
	}
	return n
}

// renderSegment draws one milestone's stretch of the bar. A finished
// milestone is a quiet run of the dimmed fill; an unfinished one draws its
// done fraction in the bright fill and the rest in the empty colour.
func renderSegment(styles Styles, p domain.Progress, width int) string {
	if p.Done == p.Total {
		return repeat(styles.BarFillDone, barCell, width)
	}
	filled := filledCells(p.Fraction(), width)
	return repeat(styles.BarFill, barCell, filled) + repeat(styles.BarEmpty, barCell, width-filled)
}

// filledCells is how many of a segment's cells are filled, for a fraction in
// [0,1] as domain.Progress reports it. Rounding down is what keeps a segment
// that is not finished from ever reading as full — a bar that says done when it
// is not would be the one lie the board must not tell.
func filledCells(fraction float64, width int) int {
	if fraction >= 1 {
		return width
	}
	return int(fraction * float64(width))
}

// shareOut splits cells between segments in proportion to weights, giving every
// segment at least one cell and handing the rounding leftovers to the segments
// with the largest fractional claim on them. The caller guarantees there is at
// least one cell per segment.
func shareOut(cells int, weights []int) []int {
	var sum int
	for _, w := range weights {
		sum += w
	}

	widths := make([]int, len(weights))
	remainders := make([]float64, len(weights))
	var used int
	for i, w := range weights {
		exact := float64(cells) * float64(w) / float64(sum)
		widths[i] = max(int(exact), 1)
		remainders[i] = exact - float64(int(exact))
		used += widths[i]
	}

	// The one-cell floor can overshoot when a tiny milestone sits beside a huge
	// one; pay for it out of the widest segment, which can spare it.
	for ; used > cells; used-- {
		widest := 0
		for i, w := range widths {
			if w > widths[widest] {
				widest = i
			}
		}
		widths[widest]--
	}
	for ; used < cells; used++ {
		best := 0
		for i, r := range remainders {
			if r > remainders[best] {
				best = i
			}
		}
		widths[best]++
		remainders[best] = -1
	}
	return widths
}

// renderBarLabel is the line under the bar: how much of the project is done,
// and the milestone the work is in — the first one that is not finished.
func renderBarLabel(styles Styles, width int, segments []ProgressSegment) string {
	total := totalProgress(segments)
	label := styles.Faint.Render(fmt.Sprintf("%d/%d", total.Done, total.Total))
	if name := currentSegmentName(segments); name != "" {
		label += styles.Faint.Render(" · ") + styles.Milestone.Render(name)
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(label)
}

// currentSegmentName is the name of the first segment with work left in it, or
// "" when every segment is finished — there is no milestone to point at then.
func currentSegmentName(segments []ProgressSegment) string {
	for _, s := range segments {
		if !s.Progress.Empty() && s.Progress.Done < s.Progress.Total {
			return s.Name
		}
	}
	return ""
}

// repeat renders n copies of a bar rune, or nothing at all: an empty styled
// string would still carry the style's escape codes.
func repeat(style lipgloss.Style, s string, n int) string {
	if n <= 0 {
		return ""
	}
	return style.Render(strings.Repeat(s, n))
}
