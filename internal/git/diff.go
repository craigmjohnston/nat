package git

import "strings"

// File is one file's section of a unified diff: the lines git wrote for it,
// verbatim and header included, alongside the little that has to be understood
// about them to draw a file list and jump between files.
//
// The lines are kept as they came rather than parsed into hunks and cells. The
// viewer is read-only, so the shape it needs is the shape git already produced;
// what a hunk model would buy — side-by-side columns, intra-line highlighting —
// is not on offer here.
type File struct {
	// Path is the file as the diff leaves it, and OldPath where it came from.
	// They differ only on a rename; on a deletion Path is the file that was
	// removed, since a path of /dev/null names nothing the user can look for.
	Path    string
	OldPath string
	// Added and Removed count the section's own +/- lines.
	Added   int
	Removed int
	// Binary marks a file git described rather than diffed, which is a section
	// with no +/- lines in it at all.
	Binary bool
	// Lines is the section as git wrote it, from its "diff --git" line to the
	// line before the next one.
	Lines []string
}

// Renamed reports whether the change moved the file, which is the one case
// where naming the old path as well says something.
func (f File) Renamed() bool { return f.OldPath != "" && f.OldPath != f.Path }

// The markers of a unified diff, as prefixes of the lines that carry them.
const (
	fileMarker    = "diff --git "
	oldFileMarker = "--- "
	newFileMarker = "+++ "
	// oldPrefix and newPrefix are the source and destination prefixes, which
	// [CLI.Diff] pins so that they are these whatever the repository is
	// configured with.
	oldPrefix = "a/"
	newPrefix = "b/"
	// binaryMarker opens the line git writes in place of a diff it will not
	// show, and binaryPatch the block it writes for --binary, which nat does not
	// ask for but a repository's own configuration might.
	binaryMarker = "Binary files "
	binaryPatch  = "GIT binary patch"
)

// ParseFiles splits a unified diff into its files. Anything before the first
// file header is dropped: git writes none, but a diff that arrived from
// somewhere else may carry a preamble, and it belongs to no file.
//
// A diff of no changes is no files, which is not an error — a branch whose work
// is already in the base has nothing to show, and saying so is the viewer's job.
func ParseFiles(diff string) []File {
	var files []File
	for _, line := range splitLines(diff) {
		if strings.HasPrefix(line, fileMarker) {
			old, path := headerPaths(line)
			files = append(files, File{Path: path, OldPath: old, Lines: []string{line}})
			continue
		}
		if len(files) == 0 {
			continue
		}
		f := &files[len(files)-1]
		f.Lines = append(f.Lines, line)
		countLine(f, line)
	}
	return files
}

// countLine folds one line of a file's section into what is known about it: the
// paths git spelled out, the tallies, and whether there is a diff here at all.
//
// The +++/--- lines are preferred over the "diff --git" header they follow,
// because that header pairs two paths with a single space between them and a
// filename may hold spaces of its own; these two lines each carry exactly one
// path and settle it. A path of /dev/null is passed over: it is git saying the
// file is absent from that side, not naming it.
func countLine(f *File, line string) {
	switch {
	case strings.HasPrefix(line, oldFileMarker):
		if path, ok := strings.CutPrefix(line[len(oldFileMarker):], oldPrefix); ok {
			f.OldPath = path
		}
	case strings.HasPrefix(line, newFileMarker):
		if path, ok := strings.CutPrefix(line[len(newFileMarker):], newPrefix); ok {
			f.Path = path
		}
	case strings.HasPrefix(line, "+"):
		f.Added++
	case strings.HasPrefix(line, "-"):
		f.Removed++
	case strings.HasPrefix(line, binaryMarker), line == binaryPatch:
		f.Binary = true
	}
}

// headerPaths are the two paths of a "diff --git a/x b/x" line, with their
// prefixes off. It is a first answer only — the +++/--- lines that follow say
// the same thing unambiguously and overwrite it — so the split is the simple
// one: the first " b/" that could separate them. A file created or deleted has
// no such line to correct this, but git names it on both sides of the header
// either way, so the answer here is already right.
func headerPaths(line string) (old, path string) {
	rest := line[len(fileMarker):]
	i := strings.Index(rest, " "+newPrefix)
	if i < 0 {
		return "", strings.TrimPrefix(rest, oldPrefix)
	}
	return strings.TrimPrefix(rest[:i], oldPrefix), rest[i+1+len(newPrefix):]
}

// splitLines is diff by line, without the empty last element a trailing newline
// leaves behind — git ends its output with one, and it is not a line of the
// last file's section.
func splitLines(diff string) []string {
	if diff == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(diff, "\n"), "\n")
}
