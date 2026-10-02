package gh

import (
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/logging"
)

// EditReviewers asks for, and withdraws, reviews on the pull request ref
// names, in the repository at dir: `gh pr edit <ref> --add-reviewer a,b
// --remove-reviewer c`. A login is a user's; a team goes as `org/slug`, as gh
// itself takes one.
//
// The ref is required for the reason [CLI.ViewPR] requires one, and an edit
// naming nobody at all is refused before gh runs, since gh would only answer
// it with an interactive prompt nothing here can answer.
func (c CLI) EditReviewers(dir, ref string, add, remove []string) error {
	if ref == "" {
		return fmt.Errorf("%s pr edit needs a pull request to edit", Binary)
	}
	add, remove = logins(add), logins(remove)
	if len(add) == 0 && len(remove) == 0 {
		return fmt.Errorf("%s pr edit needs a reviewer to add or remove", Binary)
	}
	args := []string{"pr", "edit", ref}
	if len(add) > 0 {
		args = append(args, "--add-reviewer", strings.Join(add, ","))
	}
	if len(remove) > 0 {
		args = append(args, "--remove-reviewer", strings.Join(remove, ","))
	}
	if _, err := c.runner.Run(dir, Binary, args...); err != nil {
		logging.Error("could not edit a pull request's reviewers", "dir", dir, "ref", ref, "error", err)
		return err
	}
	logging.Action("reviewers edited", "dir", dir, "ref", ref, "added", add, "removed", remove)
	return nil
}

// Collaborators is everyone who can be asked to review in the repository at
// dir — its collaborators, by login, read through the REST API with gh
// filling in the owner and name from the checkout's own remote. Reading the
// list needs push access; a gh that cannot is an error, never an empty list.
func (c CLI) Collaborators(dir string) ([]string, error) {
	out, err := c.runner.Run(dir, Binary,
		"api", "repos/{owner}/{repo}/collaborators", "--paginate", "--jq", ".[].login")
	if err != nil {
		logging.Error("could not list a repository's collaborators", "dir", dir, "error", err)
		return nil, err
	}
	return logins(strings.Split(out, "\n")), nil
}

// logins trims each name and drops the empty ones.
func logins(names []string) []string {
	var out []string
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" {
			out = append(out, name)
		}
	}
	return out
}
