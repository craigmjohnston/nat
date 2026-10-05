package git

import (
	"errors"
	"regexp"

	"github.com/craigmjohnston/nat/internal/logging"
)

// MergeState is whether a handed-back branch would still merge into its base:
// what one slice's merge can take away from every other branch waiting for
// review.
type MergeState int

const (
	// MergeUnknown is no answer — a repository that is not there, a branch or
	// base git cannot resolve, a git too old to test a merge. It is the zero
	// value on purpose: a reading that failed concludes nothing, and above all
	// never reads as conflicted.
	MergeUnknown MergeState = iota
	// MergeClean is a branch that merges into its base with no conflict.
	MergeClean
	// MergeConflicted is a branch whose merge into its base conflicts.
	MergeConflicted
)

// mergeConflictsExit is the exit code `git merge-tree --write-tree` gives a
// merge that conflicts. It is not enough on its own: git exits 1 too for a
// revision it cannot resolve, with nothing on stdout — so a conflict is this
// code *and* the tree the merge wrote, named on stdout's first line.
const mergeConflictsExit = 1

// treeID is the object name merge-tree writes first, in SHA-1 or SHA-256 form.
var treeID = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// ConflictsWithBase is whether branch would merge cleanly into the repository
// at dir's current default branch. It fetches first, so "current" is origin's
// as of now rather than as last heard of, resolves the base exactly as
// [CLI.Diff] does, then tests the merge with `git merge-tree --write-tree`,
// which merges in the object store alone: no working tree, no index, no
// checkout is touched, so it is safe on a worktree an agent is working in.
//
// The output is pinned for the reason Diff's is — it is parsed, not shown:
// --no-messages drops the informational lines a conflict otherwise adds, and
// --name-only keeps what remains to one path per conflicted file. Only the
// first line is read.
//
// Nothing is returned as an error. Every failure — a missing repository or
// branch, a git that predates --write-tree — is logged and answered
// [MergeUnknown], which the caller must never treat as a conflict.
func (c CLI) ConflictsWithBase(dir, branch string) MergeState {
	c.Fetch(dir)
	base := c.Base(dir)
	out, err := c.runner.Run(dir, Binary, "merge-tree", "--write-tree", "--no-messages",
		"--name-only", base, branch)
	wroteTree := treeID.MatchString(firstLine(out))
	var exitErr *ExitError
	switch {
	case err == nil && wroteTree:
		return MergeClean
	case errors.As(err, &exitErr) && exitErr.Code == mergeConflictsExit && wroteTree:
		logging.Action("a branch conflicts with its base", "dir", dir, "branch", branch,
			"base", base)
		return MergeConflicted
	}
	logging.Action("could not test a branch's merge into its base", "dir", dir,
		"branch", branch, "base", base, "error", err)
	return MergeUnknown
}
