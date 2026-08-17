package tui

import (
	"fmt"
	"strconv"
	"strings"
)

// lineRef names where in the file a run of a section's lines sits, in the words
// a comment is addressed by: "line 42", "lines 42-45", and for a run that is
// nothing but deletions the numbers the lines had before the branch removed
// them.
//
// The numbers come from the hunk headers rather than from the diff itself,
// because they are what the agent will look the lines up by: a diff line is at
// no line number at all, and quoting the text alone leaves a line that occurs
// twice in the file ambiguous. A run the headers cannot number — the lines
// above the first hunk, or a file git described rather than diffed — is named
// by nothing, and the prompt quotes it instead.
func lineRef(lines []string, start, span int) string {
	added, removed := lineNumbers(lines)
	if ref := rangeRef(added, start, span, ""); ref != "" {
		return ref
	}
	return rangeRef(removed, start, span, " of the base")
}

// rangeRef names the numbered lines of one side within a run, or nothing at all
// where the side numbers none of them.
func rangeRef(numbers []int, start, span int, side string) string {
	first, last := 0, 0
	for i := start; i < start+span && i < len(numbers); i++ {
		if i < 0 || numbers[i] == 0 {
			continue
		}
		if first == 0 {
			first = numbers[i]
		}
		last = numbers[i]
	}
	switch first {
	case 0:
		return ""
	case last:
		return "line " + strconv.Itoa(first) + side
	}
	return fmt.Sprintf("lines %d-%d%s", first, last, side)
}

// lineNumbers is the line each of a section's lines is at on either side of the
// change: added[i] is where line i sits in the file as the branch leaves it, and
// removed[i] where it sat in the base. A line that only one side has is numbered
// only there, and a line no hunk covers — a header, a hunk marker — by neither,
// which is a zero.
func lineNumbers(lines []string) (added, removed []int) {
	added, removed = make([]int, len(lines)), make([]int, len(lines))
	var next, was int
	inHunk := false
	for i, line := range lines {
		if strings.HasPrefix(line, "@@") {
			was, next, inHunk = hunkStarts(line)
			continue
		}
		if !inHunk {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+"):
			added[i], next = next, next+1
		case strings.HasPrefix(line, "-"):
			removed[i], was = was, was+1
		case strings.HasPrefix(line, `\`):
			// git's "No newline at end of file", which is about the line above
			// rather than a line of either side.
		default:
			// A context line, and the blank line git writes for an empty one:
			// the same line on both sides.
			added[i], removed[i] = next, was
			next, was = next+1, was+1
		}
	}
	return added, removed
}

// hunkStarts reads the first line of either side out of a hunk header —
// "@@ -12,7 +12,9 @@" — and reports whether it was one at all. A header this
// does not recognise leaves the lines under it unnumbered rather than numbered
// from a guess.
func hunkStarts(line string) (was, next int, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return 0, 0, false
	}
	was, oldOK := sideStart(fields[1], "-")
	next, newOK := sideStart(fields[2], "+")
	if !oldOK || !newOK {
		return 0, 0, false
	}
	return was, next, true
}

// sideStart is the first line one side of a hunk covers, read from its
// "-12,7" or "+12" field. Zero is a start like any other: it is what the side
// of a file added or removed whole reads — "+0,0" — and no line of the hunk is
// on that side to be numbered from it.
func sideStart(field, sign string) (int, bool) {
	rest, found := strings.CutPrefix(field, sign)
	if !found {
		return 0, false
	}
	start, _, _ := strings.Cut(rest, ",")
	n, err := strconv.Atoi(start)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
