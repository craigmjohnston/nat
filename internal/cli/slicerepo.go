package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/store"
)

// sliceRepoCmd records the repository a slice is worked in. A source project has
// no working directory — its cards come from anywhere — so an agent launched
// on a task with none is told to work out which repository its card is about
// and record it here; from then on relaunch, approve, merge and the merge's
// worktree removal all find it through [actions.WorkdirFor]. Once recorded,
// the slice's worktree is found or cut there as slice-worktree does it, and
// its path printed — the agent's one command from a repository to a place to
// work.
//
// A Todo slice takes it from anyone; one in progress only from whoever holds
// it — the agent working it — and a Done one from nobody, its work being on
// main already. Only a plan of nat's own can record it on its own
// ([store.RepoSetter]); a Notion project's slice names its repo through
// slice-edit's form on the board.
func sliceRepoCmd(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-repo", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	repo := flags.String("repo", "", "the repository the slice is worked in: a directory, ~ expanded (required)")
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-repo: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-repo", rest[0])
	if err != nil {
		return err
	}
	dir, err := repoFlagDir("slice-repo", *repo)
	if err != nil {
		return err
	}

	cfg, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}
	setter, ok := st.(store.RepoSetter)
	if !ok {
		return fmt.Errorf("slice-repo: %s keeps its plan in Notion: name a slice's repository with slice-edit on the board", project.Name)
	}
	shape, err := sliceShape(ctx, st, projectID, project)
	if err != nil {
		return err
	}
	s, pageShape, err := loadSlice(ctx, st, id)
	if err != nil {
		return err
	}
	if s.Status != domain.SliceTodo && !store.Holds(s, shape.On(pageShape), cfg.AssigneeUserID) {
		return notOursError(s, cfg.AssigneeUserName, "given a repository")
	}

	if err := setter.SetSliceRepo(ctx, s.ID, dir); err != nil {
		return fmt.Errorf("record the slice's repository: %w", err)
	}
	env.nudged()
	logging.Action("slice repository recorded", "slice", s.ID, "repo", dir)

	// The repository stands recorded whatever the cut makes of it: it is
	// still the slice's, and slice-worktree cuts the worktree once the
	// repository is fit for one.
	wt, err := ensureWorktree(env, project, dir, s)
	if err != nil {
		return fmt.Errorf("slice-repo: recorded %s, but could not cut the slice's worktree there: %w", dir, err)
	}
	if *asJSON {
		return writeJSON(env.Out, sliceRepoJSON{ID: s.ID, Name: s.Name, Repo: dir, Worktree: wt})
	}
	_, err = fmt.Fprintf(env.Out, "# %s\n\nRepository recorded: %s\nWorktree: %s\n", s.Name, dir, wt.Path)
	return err
}

// repoFlagDir is a --repo as command takes it: home expanded, made absolute
// against the directory the command was typed in, and refused where it is not
// a directory — a repository nobody can launch in is no answer.
func repoFlagDir(command, repo string) (string, error) {
	dir := actions.ExpandHome(strings.TrimSpace(repo))
	if dir == "" {
		return "", usageErrorf("%s: no --repo given: name the directory the slice is worked in", command)
	}
	if !filepath.IsAbs(dir) {
		wd, err := getwd()
		if err != nil {
			return "", fmt.Errorf("%s: resolve %q against the working directory: %w", command, dir, err)
		}
		dir = filepath.Join(wd, dir)
	}
	if err := actions.ExistingDir(dir); err != nil {
		return "", usageErrorf("%s: %v", command, err)
	}
	return filepath.Clean(dir), nil
}

// sliceRepoJSON is the structured form of a recorded repository and the
// worktree cut in it.
type sliceRepoJSON struct {
	ID       string                `json:"id"`
	Name     string                `json:"name"`
	Repo     string                `json:"repo"`
	Worktree actions.SliceWorktree `json:"worktree"`
}
