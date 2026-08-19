package tui

import (
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/git"
)

// expandStep is how many lines the short expand control reveals at a time,
// GitHub's own twenty rounded down to something that fits a terminal band: a
// step that filled more than the screen would scroll the change the user is
// reading off the top of it.
const expandStep = 15

// zoneKind is which way an expand zone reveals its lines: up towards the hunk
// below it, or down away from the hunk above.
//
// Every zone but the last of a file is an up zone, because what a gap is
// measured from is the change under it — the reader is heading down the file and
// the context they want first is the lines the next hunk opens on. The gap after
// the last hunk has no hunk under it at all, so it reveals downwards, out of the
// change it hangs off; drawing it with the same upward arrow would point at the
// end of the file rather than at what it reveals.
type zoneKind int

const (
	zoneUp zoneKind = iota
	zoneDown
)

// expandZone is one gap in a file's diff: the run of the file's own lines that
// the hunks around it left out, and how much of it the user has since asked for.
//
// The lines are named by their number on the branch's side of the change, which
// is the side [git.CLI.Show] reads them from; delta is what turns one of those
// numbers into the base's, since a gap is context and every line in it is on
// both sides. first..last is what is still hidden and shrinks as it is revealed,
// shownFirst..shownLast what has been revealed and grows — contiguously, and
// always against the hunk the zone hangs off, so the two runs stay two runs.
type expandZone struct {
	// at is where in the file's section the zone's rows are drawn: the index of
	// the hunk header whose gap it is, or one past the last line for the gap
	// after the last hunk.
	at   int
	kind zoneKind

	first, last           int
	shownFirst, shownLast int
	delta                 int
}

// hidden is how many of the zone's lines are still off the screen, which is what
// the controls count and what an expansion takes from.
func (z expandZone) hidden() int { return max(z.last-z.first+1, 0) }

// shown is how many of the zone's lines have been revealed.
func (z expandZone) shown() int {
	if z.shownFirst == 0 {
		return 0
	}
	return z.shownLast - z.shownFirst + 1
}

// buildZones is the gaps of one file's diff, in the order the file draws them:
// the lines before its first hunk, between each pair of hunks, and after the
// last. src is the file's own lines at the branch, which is where the count
// after the last hunk comes from — the diff says where its hunks end and nothing
// at all about how much file follows them.
//
// A file with no hunk git could read has no gaps: there is no change for a gap
// to be around, and a section that is nothing but git's own description of a
// file it would not diff is exactly that case.
//
// A gap running past the end of the file is cut back to it rather than dropped.
// It means the diff and the file disagree — a branch that moved under the read —
// and showing the lines that are there beats showing none of them.
func buildZones(f git.File, src []string) []expandZone {
	var zones []expandZone
	prevNewTo, prevOldTo, seen := 0, 0, false
	for j, line := range f.Lines {
		oldStart, oldCount, newStart, newCount, ok := hunkSpans(line)
		if !ok {
			continue
		}
		newFrom, newTo := hunkSide(newStart, newCount)
		_, oldTo := hunkSide(oldStart, oldCount)
		if z, ok := gapZone(j, zoneUp, prevNewTo+1, newFrom-1, prevOldTo-prevNewTo, len(src)); ok {
			zones = append(zones, z)
		}
		prevNewTo, prevOldTo, seen = newTo, oldTo, true
	}
	if !seen {
		return nil
	}
	if z, ok := gapZone(len(f.Lines), zoneDown, prevNewTo+1, len(src),
		prevOldTo-prevNewTo, len(src)); ok {
		zones = append(zones, z)
	}
	return zones
}

// hunkSide is the first and last line of the file one side of a hunk covers.
// A hunk that covers no line at all on its side — a pure addition read from the
// base, or a pure deletion read from the branch — is written by git as the line
// it sits after, which is what both of these are then: the gap before it ends at
// that line and the gap after it starts on the next.
func hunkSide(start, count int) (from, to int) {
	if count == 0 {
		return start + 1, start
	}
	return start, start + count - 1
}

// gapZone is a zone over the lines first..last, or nothing at all where that run
// is empty once it has been held inside the file.
func gapZone(at int, kind zoneKind, first, last, delta, lines int) (expandZone, bool) {
	first, last = max(first, 1), min(last, lines)
	if first > last {
		return expandZone{}, false
	}
	return expandZone{at: at, kind: kind, first: first, last: last, delta: delta}, true
}

// hunkSpans is both sides of a hunk header — "@@ -12,7 +12,9 @@" — whole: where
// each side starts and how many of its lines the hunk covers. It is the same
// read [hunkStarts] makes and refuses the same headers, since a header the
// numbering cannot trust is one no gap can be measured from either.
func hunkSpans(line string) (oldStart, oldCount, newStart, newCount int, ok bool) {
	if !strings.HasPrefix(line, "@@") {
		return 0, 0, 0, 0, false
	}
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return 0, 0, 0, 0, false
	}
	oldStart, oldCount, oldOK := sideRange(fields[1], "-")
	newStart, newCount, newOK := sideRange(fields[2], "+")
	if !oldOK || !newOK {
		return 0, 0, 0, 0, false
	}
	return oldStart, oldCount, newStart, newCount, true
}

// zoneAt is the zone drawn at a place in a file's section, and whether there is
// one: the hunk headers have one each where a gap precedes them, and the index
// one past the last line carries the gap after the last hunk.
func zoneAt(zones []expandZone, at int) (int, bool) {
	for i, z := range zones {
		if z.at == at {
			return i, true
		}
	}
	return 0, false
}

// expandLabel is what one of a zone's controls reads: the way it reveals, and
// how many lines it would bring on screen. The doubled arrow is the whole gap at
// once, which is only offered when a step would not finish it.
func expandLabel(kind zoneKind, n int, whole bool) string {
	arrow := "↑"
	if kind == zoneDown {
		arrow = "↓"
	}
	if whole {
		arrow += arrow
	}
	return fmt.Sprintf("%s (+%d %s)", arrow, n, plural(n, "line", "lines"))
}

// zoneParts is what one expand zone is drawn as: its controls and whatever of it
// has already been revealed, in the order the two go — the controls above the
// revealed lines for a zone that reveals upwards, since the lines it will bring
// next are the ones nearest the hunk below, and below them for one that reveals
// down.
//
// A zone with nothing left hidden draws no control at all: the gap is filled,
// and a control offering no lines would be a row that does nothing.
func zoneParts(z expandZone, zone int, src []string, width int) []boxPart {
	var controls []boxPart
	if hidden := z.hidden(); hidden > 0 {
		controls = append(controls, boxPart{kind: partExpand, zone: zone,
			text: expandLabel(z.kind, min(hidden, expandStep), false)})
		if hidden > expandStep {
			controls = append(controls, boxPart{kind: partExpand, zone: zone, whole: true,
				text: expandLabel(z.kind, hidden, true)})
		}
	}
	// A zone's lines are held inside the file it was built over, so what it says
	// it has revealed is always there to draw.
	var revealed []boxPart
	for n := z.shownFirst; z.shownFirst > 0 && n <= z.shownLast; n++ {
		// The line goes in with the leading space a context line of the diff
		// carries, so a revealed line starts in the same column as the lines
		// around it; the ± column is what it has nothing to put in.
		revealed = append(revealed, boxPart{kind: partContext, zone: zone, num: n,
			old: max(n+z.delta, 0), segs: wrapLine(" "+src[n-1], width)})
	}
	if z.kind == zoneUp {
		return append(controls, revealed...)
	}
	return append(revealed, controls...)
}

// expandZoneBy reveals a zone's lines: a step of [expandStep] from the end it
// grows from, or the whole of what is left. The revealed run stays one run
// against the hunk the zone hangs off, so pressing the key again picks up where
// the last press stopped.
func (d *Diff) expandZoneBy(file, zone int, whole bool) {
	if file < 0 || file >= len(d.zones) || zone < 0 || zone >= len(d.zones[file]) {
		return
	}
	z := &d.zones[file][zone]
	n := z.hidden()
	if n == 0 {
		return
	}
	if !whole {
		n = min(n, expandStep)
	}
	if z.kind == zoneUp {
		from := z.last - n + 1
		if z.shownFirst == 0 {
			z.shownLast = z.last
		}
		z.shownFirst, z.last = from, from-1
		return
	}
	to := z.first + n - 1
	if z.shownFirst == 0 {
		z.shownFirst = z.first
	}
	z.shownLast, z.first = to, to+1
}

// ExpandAt activates the expand control drawn at a body row — a key pressed on
// the row, or a click that landed on it — and reports whether that row was one
// at all.
//
// The cursor goes to the row first, so a click acts where it landed rather than
// where the cursor happened to be, and a range being marked is dropped for the
// reason a fold drops one: the body moves under it.
func (d *Diff) ExpandAt(row int) bool {
	if row < 0 || row >= len(d.lines) {
		return false
	}
	at := d.lines[row]
	if at.line != boxExpandRow {
		return false
	}
	d.line, d.cursor = row, at.file
	d.clearSelection()
	d.expandZoneBy(at.file, at.zone, at.whole)
	d.render()
	d.setLine(d.line)
	return true
}

// onExpandRow reports whether the line cursor is on an expand control, which is
// what makes the key that acts on the row under it reveal lines rather than fold
// the file away.
func (d Diff) onExpandRow() bool {
	return d.line >= 0 && d.line < len(d.lines) && d.lines[d.line].line == boxExpandRow
}
