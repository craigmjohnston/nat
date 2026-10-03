package gh

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/craigmjohnston/nat/internal/logging"
)

// actionsRunURL matches a check's link where GitHub Actions ran it: the run,
// and — for a check run, which is one job of it — the job within the run.
var actionsRunURL = regexp.MustCompile(`/actions/runs/(\d+)(?:/job/(\d+))?`)

// ActionsRun reads a check's URL for the GitHub Actions run behind it: the
// run's id and, where the URL names one, the job's. ok is false for a status
// some other service reported — its URL is somewhere gh cannot read a log from.
func ActionsRun(url string) (run, job string, ok bool) {
	m := actionsRunURL.FindStringSubmatch(url)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// FailedLog is the log of a GitHub Actions run's failed steps, as
// `gh run view --log-failed` prints it: of the one job named where there is
// one — a check is a job, and the log wanted is that check's own — and of the
// whole run's failed steps where there is not.
func (c CLI) FailedLog(dir, run, job string) (string, error) {
	if run == "" && job == "" {
		return "", fmt.Errorf("%s run view needs a run to read", Binary)
	}
	args := []string{"run", "view", run}
	if job != "" {
		args = []string{"run", "view", "--job", job}
	}
	out, err := c.runner.Run(dir, Binary, append(args, "--log-failed")...)
	if err != nil {
		logging.Error("could not read a run's failed log", "dir", dir, "run", run, "job", job, "error", err)
		return "", err
	}
	return strings.TrimRight(out, "\n"), nil
}
