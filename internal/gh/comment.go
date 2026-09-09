package gh

import (
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/logging"
)

// CommentPR posts a comment on the pull request ref names, in the repository
// at dir, and returns the comment's own URL where gh printed one.
//
// The ref is whatever identifies the pull request, exactly as [CLI.ViewPR]
// takes one, and is required for the same reason: gh with nothing named reads
// or writes against whatever branch the directory happens to be on, which for
// a shared checkout is nobody's slice in particular.
//
// The body goes on gh's own standard input through --body-file - rather than
// as a --body argument. A review comment is free text with no bound on its
// length, and a shell's argument list does have one — long enough that it is
// rarely hit by hand, but a comment built out of a diff's own quoted lines,
// the way the board's own review prompts are, can run into it exactly where
// --body-file - cannot: gh already reads "-" off a pipe for every other flag
// that takes a file, and asking it to do the same here costs nothing a short
// comment would have paid for by argument instead.
func (c CLI) CommentPR(dir, ref, body string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("%s pr comment needs a pull request to comment on", Binary)
	}
	if strings.TrimSpace(body) == "" {
		return "", fmt.Errorf("%s pr comment needs a comment to post", Binary)
	}
	runner, ok := c.runner.(StdinRunner)
	if !ok {
		return "", fmt.Errorf("%s runner cannot carry a comment on its standard input", Binary)
	}
	out, err := runner.RunWithStdin(dir, strings.NewReader(body), Binary, "pr", "comment", ref, "--body-file", "-")
	if err != nil {
		logging.Error("could not comment on a pull request", "dir", dir, "ref", ref, "error", err)
		return "", err
	}
	url := prURL(out)
	logging.Action("comment posted", "dir", dir, "ref", ref, "url", url)
	return url, nil
}
