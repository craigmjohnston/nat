package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/git"
)

// commitsDoc is the structured form of slice-diff --commits: the base and
// branch the commits are measured against, the same two fields the whole
// diff's own JSON carries, alongside the commits themselves.
type commitsDoc struct {
	Base    string       `json:"base"`
	Branch  string       `json:"branch"`
	Commits []commitJSON `json:"commits"`
}

// commitJSON is one commit, gh's own field names kept plain rather than
// nested: a hash, a subject line, who wrote it and when.
type commitJSON struct {
	SHA     string    `json:"sha"`
	Subject string    `json:"subject"`
	Author  string    `json:"author"`
	Date    time.Time `json:"date"`
}

// writeCommitsJSON encodes the branch's commits.
func writeCommitsJSON(out io.Writer, base, branch string, commits []git.Commit) error {
	doc := commitsDoc{Base: base, Branch: branch, Commits: make([]commitJSON, 0, len(commits))}
	for _, c := range commits {
		doc.Commits = append(doc.Commits, commitJSON{SHA: c.SHA, Subject: c.Subject, Author: c.Author, Date: c.Date})
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// commitsMarkdown renders the commits as a list, newest first exactly as git
// log — and so [git.CLI.Commits] — already orders them.
func commitsMarkdown(base, branch string, commits []git.Commit) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Commits\n\n%s..%s\n\n", base, branch)
	if len(commits) == 0 {
		b.WriteString("_none_\n")
		return b.String()
	}
	for _, c := range commits {
		fmt.Fprintf(&b, "- %s %s — %s, %s\n", shortSHA(c.SHA), c.Subject, c.Author, c.Date.Format("2006-01-02"))
	}
	return b.String()
}

// shortSHA is the first eight characters of a commit's hash — long enough to
// name one unambiguously in a list, and short enough that the subject beside
// it is what a reader's eye lands on.
func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
