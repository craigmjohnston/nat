package git

import (
	"errors"
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/logging"
)

// DirtyPaths is every path dir's working tree holds that is not committed —
// modified, staged, deleted or untracked and not ignored — as `git status
// --porcelain --untracked-files=normal` names them, in its order. None is a
// clean tree. A rename comes back as git writes it, `old -> new`, which is
// what a person reading the list wants to see.
func (c CLI) DirtyPaths(dir string) ([]string, error) {
	out, err := c.runner.Run(dir, Binary, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		logging.Error("could not read a worktree's status", "dir", dir, "error", err)
		return nil, err
	}
	var paths []string
	for line := range strings.SplitSeq(out, "\n") {
		// Each entry is two status letters, a space, then the path.
		if len(line) < 4 {
			continue
		}
		paths = append(paths, line[3:])
	}
	return paths, nil
}

// Push sends branch to origin from dir, setting it as the branch's upstream:
// `git push --force-with-lease -u origin <branch>`. The lease is what lets a
// branch rebased onto its base be pushed over its old self, while still
// refusing to overwrite a remote that moved past what dir last fetched.
//
// A push git refuses comes back with everything git wrote to stderr, not only
// its first line: a rejected push explains itself over several, and the
// reason is rarely in the first.
func (c CLI) Push(dir, branch string) error {
	_, err := c.runner.Run(dir, Binary, "push", "--force-with-lease", "-u", "origin", branch)
	if err == nil {
		logging.Action("pushed a branch", "dir", dir, "branch", branch)
		return nil
	}
	logging.Error("could not push a branch", "dir", dir, "branch", branch, "error", err)
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		if out := strings.TrimSpace(exitErr.Stderr); out != "" {
			return fmt.Errorf("git push refused %s:\n%s", branch, out)
		}
	}
	return fmt.Errorf("git push %s: %w", branch, err)
}
