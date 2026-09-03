package git

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// Commit is one commit of a branch's own history: the granularity a whole
// branch's [CLI.Diff] folds away, since that reads everything since the merge
// base as one change. SHA is the commit's full hash, Subject its first message
// line, Author who wrote it and Date when — the four fields git log is asked
// for and nothing more.
type Commit struct {
	SHA     string
	Subject string
	Author  string
	Date    time.Time
}

// commitLogFormat is git log's own placeholder syntax for the four fields
// [Commit] carries, each separated by a NUL rather than a delimiter a commit
// subject could itself contain — a comma or a pipe is nothing git promises to
// keep out of one, and a NUL is nothing a text commit message can hold.
const commitLogFormat = "%H%x00%s%x00%an%x00%aI"

// Commits is a branch's own history since the merge base [CLI.Diff] measures
// it against, oldest and newest exactly as git log orders them — newest
// first, which is git's own default and so the order every other reader of a
// branch's history already expects.
//
// The base is resolved the same way [CLI.Diff]'s is: [CLI.Base], so a
// caller asking for a branch's commits and its diff is asking about the same
// stretch of history either way.
func (c CLI) Commits(dir, branch string) ([]Commit, error) {
	base := c.Base(dir)
	out, err := c.runner.Run(dir, Binary, "log", "--format="+commitLogFormat, base+".."+branch)
	if err != nil {
		logging.Error("could not read a branch's commits", "dir", dir, "branch", branch,
			"base", base, "error", err)
		return nil, err
	}
	commits, err := parseCommits(out)
	if err != nil {
		logging.Error("could not read what git log wrote", "dir", dir, "branch", branch, "error", err)
		return nil, err
	}
	return commits, nil
}

// parseCommits splits git log's NUL-and-newline-delimited output into
// [Commit]s, one per line. A commit's date is read in git's own %aI format —
// strict ISO 8601, which [time.RFC3339] parses exactly.
func parseCommits(out string) ([]Commit, error) {
	lines := splitLines(out)
	commits := make([]Commit, 0, len(lines))
	for _, line := range lines {
		fields := strings.Split(line, "\x00")
		if len(fields) != 4 {
			return nil, fmt.Errorf("git log printed a line with %d fields, want 4: %q", len(fields), line)
		}
		when, err := time.Parse(time.RFC3339, fields[3])
		if err != nil {
			return nil, fmt.Errorf("read a commit's date: %w", err)
		}
		commits = append(commits, Commit{SHA: fields[0], Subject: fields[1], Author: fields[2], Date: when})
	}
	return commits, nil
}

// ErrNoParent is what [CLI.CommitDiff] refuses a root commit with: one has no
// parent to diff against, and diffing it against the empty tree instead would
// be a diff of everything the commit ever added rather than of the commit
// itself — a different question than the one asked, so this is a refusal
// rather than an answer dressed up as one.
var ErrNoParent = errors.New("the first commit has no parent to diff against")

// CommitDiff is the change one commit of a branch's history made, on its own:
// a unified diff against its first parent, with the same pinned prefixes and
// external diff driver refused that [CLI.Diff] pins, since this is parsed by
// the same [ParseFiles] a branch's whole diff is.
//
// The parent is checked for before the diff is ever run, rather than left to
// whatever git would have said about a diff it could not take: a merge
// commit's first parent is what "^" alone already means, so only a root
// commit — one with none — is refused, in [ErrNoParent].
func (c CLI) CommitDiff(dir, sha string) (string, error) {
	if _, err := c.runner.Run(dir, Binary, "rev-parse", "--verify", "--quiet", sha+"^"); err != nil {
		return "", ErrNoParent
	}
	out, err := c.runner.Run(dir, Binary, "diff", "--no-color", "--no-ext-diff",
		"--src-prefix=a/", "--dst-prefix=b/", sha+"^", sha)
	if err != nil {
		logging.Error("could not read the diff of a commit", "dir", dir, "sha", sha, "error", err)
		return "", err
	}
	return out, nil
}
