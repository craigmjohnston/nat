package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/craigmjohnston/nat/internal/logging"
)

// sessionDiff prints the diff of one of a session's branches against its
// merge base with the default branch — the same read [sliceDiff] makes of a
// slice's handed-back branch, reused rather than re-derived: [sliceBranchDiff]
// is the shared renderer, JSON and markdown alike.
//
// The branch is --branch when given, else the worktree's own current one —
// read fresh with [GitCLI.CurrentBranch] rather than trusted from the
// session's own record, since an ad hoc session's agent is free to check out
// anything it likes and the record is only ever what it was cut on. Working
// tree changes are included exactly when that is the branch actually diffed:
// a plain `git diff` against the merge base, with no second ref, sees the
// worktree as it stands rather than only its last commit.
func sessionDiff(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("session-diff", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of the raw diff")
	branchFlag := flags.String("branch", "", "the branch to diff; the worktree's current one if unset")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("session-diff: want exactly one session ID, given %d", len(rest))
	}
	id, err := pageID("session-diff", rest[0])
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
	branch := *branchFlag
	current, currentErr := gitCLI.CurrentBranch(sess.Dir)
	if branch == "" {
		branch = current
	}
	if branch == "" {
		return fmt.Errorf("session %s: no branch to diff — pass --branch", id)
	}

	// Working tree changes are visible only in the worktree that has this
	// branch checked out right now; a fresh read rather than the flag's own
	// spelling, since --branch may name a branch this worktree is not on.
	includeWorkingTree := currentErr == nil && branch == current
	if includeWorkingTree {
		return sessionWorkingTreeDiff(gitCLI, sess.Dir, branch, *asJSON, env.Out)
	}
	return sliceBranchDiff(gitCLI, sess.Dir, "", branch, *asJSON, env.Out)
}

// sessionWorkingTreeDiff diffs the branch actually checked out in a
// session's worktree, uncommitted changes included: `git diff --merge-base
// <base>` against no second ref reads the working tree, where naming the
// branch itself would read only its last commit.
func sessionWorkingTreeDiff(gitCLI GitCLI, dir, branch string, asJSON bool, out io.Writer) error {
	base, diff, err := gitCLI.DiffWorkingTreeFrom(dir, "")
	if err != nil {
		logging.Error("could not read a session's working tree diff", "error", err)
		return fmt.Errorf("read the diff: %w", err)
	}
	if asJSON {
		return writeDiffJSON(out, base, branch, diff)
	}
	_, err = io.WriteString(out, diff)
	return err
}
