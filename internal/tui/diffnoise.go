package tui

import "strings"

// The lines of a file's section that say what the screen already says
// elsewhere: git's own header for the file, the blob hashes under it, and the
// two paths — the box's header row names the path and its ± tally — and the
// hunk headers, whose numbers the gutter carries down the left of every line.
//
// They are dropped from the render alone. Parsing keeps every one of them:
// [lineNumbers] reads the hunk headers for the gutter and for the lines a
// comment names, and a body row is still a line of the section git wrote.
const (
	gitFileMarker = "diff --git "
	gitBlobMarker = "index "
	gitOldMarker  = "--- "
	gitNewMarker  = "+++ "
)

// lineRole is what the render does with one line of a file's section: draw it,
// drop it as noise, or draw the break that says a hunk ended and the lines
// between it and the next one were skipped.
type lineRole int

const (
	roleDraw lineRole = iota
	roleDrop
	roleBreak
)

// lineRoles classifies one file's section, in step with its lines.
//
// The file headers are only ever recognised above the first hunk, which is the
// only place git writes them: inside a hunk every line carries a +/- or a space
// of its own, and a removed line reading "--- x" is three characters the branch
// took out rather than a path. A hunk header is one [hunkStarts] can read, so
// what the gutter numbers from and what the render hides are the same lines —
// one it cannot parse leaves its lines unnumbered and is worth showing.
//
// The first hunk header of a file is dropped outright: there is no hunk above
// it for a break to be the gap after. Every later one leaves the break behind,
// so the gutter's numbers jumping is not the only sign that lines went by.
//
// A file git described rather than diffed — a binary one — has no hunk at all,
// and keeps the one line git wrote for it: that line is not said anywhere else.
func lineRoles(lines []string) []lineRole {
	roles := make([]lineRole, len(lines))
	inHunk := false
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "@@") && hunkHeader(line):
			roles[i] = roleDrop
			if inHunk {
				roles[i] = roleBreak
			}
			inHunk = true
		case !inHunk && (strings.HasPrefix(line, gitFileMarker) ||
			strings.HasPrefix(line, gitBlobMarker) ||
			strings.HasPrefix(line, gitOldMarker) ||
			strings.HasPrefix(line, gitNewMarker)):
			roles[i] = roleDrop
		default:
			roles[i] = roleDraw
		}
	}
	return roles
}

// hunkHeader reports whether a line is a hunk header the numbering could read,
// which is the one the render hides.
func hunkHeader(line string) bool {
	_, _, ok := hunkStarts(line)
	return ok
}
