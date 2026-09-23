package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/logging"
)

// PRHeadLister is what [sessionStatus] and [sessionList] need of gh: every
// pull request a branch has ever had as its head, open or not — narrower
// than the rest of [GH], the way every other seam in this package is.
type PRHeadLister interface {
	ListPRsForHead(dir, branch string) ([]gh.HeadPR, error)
}

// sessionStatus reads every branch a session has been on — its worktree's
// current branch, plus every branch its `git reflog show HEAD` records a
// checkout to or from — and the pull requests each has opened. A session
// that made three branches and three pull requests reports all three, not
// only whichever branch happens to be checked out now.
//
// Once every pull request it knows about has merged — or, with --discard,
// the session has no live tmux session left and none of them is still open
// — its worktree is removed by the same idempotent removal slices use and
// it is marked ended. A branch whose pull requests could not be read is
// reported as such rather than as having none, and stops that decision
// short: a read that fails concludes nothing, so a session is never ended
// on the strength of one.
func sessionStatus(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("session-status", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	discard := flags.Bool("discard", false,
		"end the session once it has no open pull requests, even without every one merged")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("session-status: want exactly one session ID, given %d", len(rest))
	}
	id, err := pageID("session-status", rest[0])
	if err != nil {
		return err
	}

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}

	sessions, err := st.Sessions(ctx, storeProject(projectID, project))
	if err != nil {
		return fmt.Errorf("read the sessions: %w", err)
	}
	sess, found := findSession(sessions, id)
	if !found {
		return noSessionError(id)
	}

	gitCLI := env.NewGit()
	branches := sessionBranches(gitCLI, sess)

	ghCLI := env.NewGH()
	statuses := make([]branchStatus, len(branches))
	allRefreshed := true
	for i, b := range branches {
		prs, err := ghCLI.ListPRsForHead(sess.Dir, b)
		if err != nil {
			logging.Action("could not read a branch's pull requests; reporting no change",
				"session", id, "branch", b, "err", err)
			statuses[i] = branchStatus{Branch: b, Stale: true}
			allRefreshed = false
			continue
		}
		statuses[i] = branchStatus{Branch: b, PRs: prs}
	}

	live, err := env.NewTmux().LiveSlices()
	if err != nil {
		return fmt.Errorf("could not read live sessions: %w", err)
	}
	_, isLive := live[agent.SessionTag(projectID, id)]

	ended := false
	if allRefreshed && shouldEndSession(statuses, isLive, *discard) {
		actions.RemoveWorktree(env.NewWorktrees(), sess.Dir, sess.Branch)
		if err := st.EndSession(ctx, id); err != nil {
			return fmt.Errorf("end the session: %w", err)
		}
		ended = true
	}

	if *asJSON {
		return writeSessionStatusJSON(env.Out, sess, statuses, isLive, ended)
	}
	_, err = io.WriteString(env.Out, sessionStatusMarkdown(sess, statuses, isLive, ended))
	return err
}

// sessionBranches is every branch the session has been on: the worktree's
// current one, first, and then every branch its reflog records a checkout
// to or from, deduplicated.
func sessionBranches(gitCLI GitCLI, sess domain.Session) []string {
	seen := map[string]bool{}
	var branches []string
	add := func(b string) {
		if b == "" || seen[b] {
			return
		}
		seen[b] = true
		branches = append(branches, b)
	}
	if cur, err := gitCLI.CurrentBranch(sess.Dir); err == nil {
		add(cur)
	} else {
		logging.Action("could not read a session's current branch", "err", err)
	}
	if refs, err := gitCLI.ReflogBranches(sess.Dir); err == nil {
		for _, b := range refs {
			add(b)
		}
	} else {
		logging.Action("could not read a session's reflog", "err", err)
	}
	if len(branches) == 0 && sess.Branch != "" {
		add(sess.Branch)
	}
	return branches
}

// branchStatus is one branch of a session's and the pull requests it has
// opened, or Stale where that read failed and nothing here is known fresh.
type branchStatus struct {
	Branch string
	PRs    []gh.HeadPR
	Stale  bool
}

// shouldEndSession is the one rule [sessionStatus] ends a session by: every
// pull request it knows about has merged, or — only with --discard — the
// session's tmux is gone and none of them is still open. Both readings
// require every branch to have refreshed; the caller checks that first,
// since a session must never be ended on a read that failed.
func shouldEndSession(statuses []branchStatus, live, discard bool) bool {
	var total, open, merged int
	for _, s := range statuses {
		for _, pr := range s.PRs {
			total++
			switch pr.State {
			case "OPEN":
				open++
			case "MERGED":
				merged++
			}
		}
	}
	everyMerged := total > 0 && open == 0 && merged == total
	noneOpen := open == 0
	return everyMerged || (discard && !live && noneOpen)
}

type sessionStatusJSON struct {
	ID       string             `json:"id"`
	Live     bool               `json:"live"`
	Ended    bool               `json:"ended"`
	Dir      string             `json:"dir"`
	Branch   string             `json:"branch,omitempty"`
	Branches []branchStatusJSON `json:"branches"`
}

type branchStatusJSON struct {
	Branch string       `json:"branch"`
	Stale  bool         `json:"stale,omitempty"`
	PRs    []headPRJSON `json:"prs,omitempty"`
}

// writeSessionStatusJSON encodes the status result.
func writeSessionStatusJSON(out io.Writer, sess domain.Session, statuses []branchStatus, live, ended bool) error {
	branches := make([]branchStatusJSON, len(statuses))
	for i, s := range statuses {
		branches[i] = branchStatusJSON{Branch: s.Branch, Stale: s.Stale, PRs: headPRsJSON(s.PRs)}
	}
	doc := sessionStatusJSON{
		ID: sess.ID, Live: live, Ended: ended || sess.Ended(), Dir: sess.Dir, Branch: sess.Branch, Branches: branches,
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// sessionStatusMarkdown renders the status result.
func sessionStatusMarkdown(sess domain.Session, statuses []branchStatus, live, ended bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Session %s\n\n", sess.ID)
	state := "gone"
	switch {
	case live:
		state = "live"
	case ended || sess.Ended():
		state = "ended"
	}
	fmt.Fprintf(&b, "- State: %s\n", state)
	fmt.Fprintf(&b, "- Directory: %s\n", sess.Dir)
	for _, s := range statuses {
		fmt.Fprintf(&b, "\n## %s\n\n", s.Branch)
		switch {
		case s.Stale:
			fmt.Fprintf(&b, "Could not refresh this branch's pull requests.\n")
		case len(s.PRs) == 0:
			fmt.Fprintf(&b, "No pull requests.\n")
		default:
			for _, pr := range s.PRs {
				fmt.Fprintf(&b, "- #%d %s — %s (%s)\n", pr.Number, pr.Title, pr.State, pr.URL)
			}
		}
	}
	return b.String()
}
