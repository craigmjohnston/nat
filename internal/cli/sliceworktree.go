package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// sliceWorktreeCmd prints the path of a slice's worktree, cutting it first
// where there is none — exactly as a board launch places an agent
// ([actions.EnsureWorktree]). It is how an agent that was not placed by a
// launch — a session started from /next-slice by hand — reaches the worktree
// a launch would have given it, by nat's own naming rather than a paragraph
// of rules re-spelling it.
//
// The repository is --repo, else the slice's own, else the project's working
// directory. A directory that is no git repository, or a git that refuses, is
// the command's error in git's words. It writes nothing to the plan, so it
// asks nothing of who holds the slice.
func sliceWorktreeCmd(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-worktree", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	repo := flags.String("repo", "", "the repository to find or cut the worktree in: a directory, ~ expanded (default: the slice's, else the project's)")
	asJSON := flags.Bool("json", false, "print structured JSON instead of the path")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-worktree: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-worktree", rest[0])
	if err != nil {
		return err
	}
	dir := ""
	if strings.TrimSpace(*repo) != "" {
		if dir, err = repoFlagDir("slice-worktree", *repo); err != nil {
			return err
		}
	}

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}
	s, _, err := loadSlice(ctx, st, id)
	if err != nil {
		return err
	}
	if dir == "" {
		dir = actions.ExpandHome(strings.TrimSpace(actions.WorkdirFor(s, project)))
	}
	if dir == "" {
		return fmt.Errorf("slice-worktree: %q has no repository yet: record it with slice-repo, or name one with --repo", s.Name)
	}
	wt, err := ensureWorktree(env, project, dir, s)
	if err != nil {
		return fmt.Errorf("slice-worktree: %w", err)
	}
	if *asJSON {
		return writeJSON(env.Out, wt)
	}
	_, err = fmt.Fprintln(env.Out, wt.Path)
	return err
}

// ensureWorktree is [actions.EnsureWorktree] through the command's own git
// drivers — the project's base branch included, as a launch reads it.
func ensureWorktree(env Env, project config.ProjectConfig, dir string, s domain.Slice) (actions.SliceWorktree, error) {
	return actions.EnsureWorktree(env.NewWorktrees(), env.gitFor(project), dir, s)
}
