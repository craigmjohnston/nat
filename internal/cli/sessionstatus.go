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

// sessionStatus reads every branch a session has been on — its worktree's
// current branch, plus the most recent its `git reflog show HEAD` records a
// checkout to or from ([sessionBranches]) — and the pull requests each has
// opened, as the last batched reading (pr-status's, [lastReading]) found
// them: this command asks GitHub nothing itself. A session that made three
// branches and three pull requests reports all three, not only whichever
// branch happens to be checked out now.
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

	branches := sessionBranches(env.NewGit(), sess)

	kept := env.loadLastReading().Sessions[sess.ID]
	statuses := make([]branchStatus, len(branches))
	allRefreshed := true
	for i, b := range branches {
		prs, read := kept[b]
		if !read {
			logging.Action("no reading of a branch's pull requests yet; reporting no change",
				"session", id, "branch", b)
			statuses[i] = branchStatus{Branch: b, Stale: true}
			allRefreshed = false
			continue
		}
		statuses[i] = branchStatus{Branch: b, PRs: headPRsOf(prs)}
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

// maxSessionBranches is how many of a session's branches are read: the
// current one and the most recent the reflog names. A long-lived session's
// reflog runs to every branch it ever touched, and each is a selection in
// every reading; the ones it worked on last are the ones still worth asking.
const maxSessionBranches = 5

// sessionBranches is the branches the session has been on, at most
// [maxSessionBranches]: the worktree's current one, first, and then the most
// recent its reflog records a checkout to or from, deduplicated.
func sessionBranches(gitCLI GitCLI, sess domain.Session) []string {
	seen := map[string]bool{}
	var branches []string
	add := func(b string) {
		if b == "" || seen[b] || len(branches) == maxSessionBranches {
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

// sessionHeads is one session's branches as a reading asks about them: in
// the repository its origin names, or — where that cannot be read — not at
// all, every branch then reading stale.
type sessionHeads struct {
	id          string
	owner, repo string
	known       bool
	branches    []string
}

// headsOf is the session's branches ([sessionBranches]) and the GitHub
// repository its origin names.
func headsOf(gitCLI GitCLI, sess domain.Session) sessionHeads {
	h := sessionHeads{id: sess.ID, branches: sessionBranches(gitCLI, sess)}
	if len(h.branches) == 0 {
		return h
	}
	remote, err := gitCLI.RemoteURL(sess.Dir)
	if err == nil {
		h.owner, h.repo, h.known = gh.ParseRemote(remote)
	}
	if !h.known {
		logging.Action("left a session's pull requests unread: no GitHub origin", "session", sess.ID)
	}
	return h
}

// heads is what the reading asks about this session.
func (h sessionHeads) heads() []gh.HeadRef {
	if !h.known {
		return nil
	}
	out := make([]gh.HeadRef, len(h.branches))
	for i, b := range h.branches {
		out[i] = gh.HeadRef{Owner: h.owner, Repo: h.repo, Branch: b}
	}
	return out
}

// read is every branch the batch read, with its pull requests, for the
// reading kept on disk.
func (h sessionHeads) read(batch gh.Batch) map[string][]headPRJSON {
	out := map[string][]headPRJSON{}
	for _, ref := range h.heads() {
		if prs, ok := batch.Heads[ref]; ok {
			out[ref.Branch] = headPRsJSON(prs)
		}
	}
	return out
}

// prs is every pull request the batch found for any of the session's
// branches, stale where any branch went unread.
func (h sessionHeads) prs(batch gh.Batch) ([]gh.HeadPR, bool) {
	var prs []gh.HeadPR
	stale := !h.known && len(h.branches) > 0
	for _, ref := range h.heads() {
		got, ok := batch.Heads[ref]
		if !ok {
			stale = true
			continue
		}
		prs = append(prs, got...)
	}
	return prs, stale
}

// headPRsOf is pull requests kept on disk as the [gh.HeadPR]s they were read
// as.
func headPRsOf(kept []headPRJSON) []gh.HeadPR {
	out := make([]gh.HeadPR, len(kept))
	for i, pr := range kept {
		out[i] = gh.HeadPR{Number: pr.Number, Title: pr.Title, URL: pr.URL, State: pr.State, MergedAt: pr.MergedAt}
	}
	return out
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
