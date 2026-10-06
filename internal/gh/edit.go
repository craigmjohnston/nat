package gh

import (
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/logging"
)

// EditPRBody replaces the body of the pull request ref names, in the
// repository at dir.
//
// The ref is required for the reason [CLI.CommentPR] gives, and the body goes
// on gh's own standard input through --body-file - for the same one too: a
// description has no bound on its length, and a shell's argument list does.
func (c CLI) EditPRBody(dir, ref, body string) error {
	if ref == "" {
		return fmt.Errorf("%s pr edit needs a pull request to edit", Binary)
	}
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("%s pr edit needs a description to write", Binary)
	}
	runner, ok := c.runner.(StdinRunner)
	if !ok {
		return fmt.Errorf("%s runner cannot carry a description on its standard input", Binary)
	}
	if _, err := runner.RunWithStdin(dir, strings.NewReader(body), Binary, "pr", "edit", ref, "--body-file", "-"); err != nil {
		logging.Error("could not edit a pull request's description", "dir", dir, "ref", ref, "error", err)
		return err
	}
	logging.Action("pull request description edited", "dir", dir, "ref", ref)
	return nil
}
