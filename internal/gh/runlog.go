package gh

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/craigmjohnston/nat/internal/logging"
)

// actionsRunURL matches a check's link where GitHub Actions ran it: the
// repository it ran in, the run, and — for a check run, which is one job of it
// — the job within the run.
var actionsRunURL = regexp.MustCompile(`(?:/([^/]+)/([^/]+))?/actions/runs/(\d+)(?:/job/(\d+))?`)

// ActionsRef is a GitHub Actions run read off a check's URL: the repository
// it ran in (which need not be the worktree's remote), the run's id and, where
// the URL names one, the job's.
type ActionsRef struct {
	Owner, Repo string
	Run, Job    string
}

// ActionsRun reads a check's URL for the GitHub Actions run behind it. ok is
// false for a status some other service reported — its URL is somewhere gh
// cannot read a log from.
func ActionsRun(url string) (ref ActionsRef, ok bool) {
	m := actionsRunURL.FindStringSubmatch(url)
	if m == nil {
		return ActionsRef{}, false
	}
	return ActionsRef{Owner: m[1], Repo: m[2], Run: m[3], Job: m[4]}, true
}

// runInProgress is the words gh refuses `run view --log-failed` with while any
// job of the run is still going — even where the job asked about has already
// finished and failed.
const runInProgress = "still in progress"

// FailedLog is the log of a GitHub Actions run's failed steps, as
// `gh run view --log-failed` prints it: of the one job named where there is
// one — a check is a job, and the log wanted is that check's own — and of the
// whole run's failed steps where there is not.
//
// gh refuses that read while any job of the run is still running, which is
// exactly when a check first reads red. A job's own log is readable the moment
// it completes, though, so where gh refuses for that reason and the URL named
// a job (and the repository it ran in), the job's whole log is read through
// the REST API instead. A run-level log has no such fallback: it is only
// readable once the run is complete, so that refusal stands.
func (c CLI) FailedLog(dir string, ref ActionsRef) (string, error) {
	if ref.Run == "" && ref.Job == "" {
		return "", fmt.Errorf("%s run view needs a run to read", Binary)
	}
	args := []string{"run", "view", ref.Run}
	if ref.Job != "" {
		args = []string{"run", "view", "--job", ref.Job}
	}
	out, err := c.runner.Run(dir, Binary, append(args, "--log-failed")...)
	var exitErr *ExitError
	if err != nil && errors.As(err, &exitErr) && strings.Contains(exitErr.Stderr, runInProgress) &&
		ref.Job != "" && ref.Owner != "" && ref.Repo != "" {
		out, err = c.runner.Run(dir, Binary, "api", fmt.Sprintf("repos/%s/%s/actions/jobs/%s/logs", ref.Owner, ref.Repo, ref.Job))
	}
	if err != nil {
		logging.Error("could not read a run's failed log", "dir", dir, "run", ref.Run, "job", ref.Job, "error", err)
		return "", err
	}
	return strings.TrimRight(out, "\n"), nil
}
