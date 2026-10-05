package gh

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// Job is one GitHub Actions job as the REST API reads it: where it stands
// (Status is GitHub's own word — queued, in_progress, completed, waiting —
// and Conclusion is empty until it completes), when it was queued and when a
// runner picked it up, the runner's name, and its steps in order. A time
// GitHub has not set yet is zero.
type Job struct {
	Status     string
	Conclusion string
	CreatedAt  time.Time
	StartedAt  time.Time
	Runner     string
	Steps      []JobStep
}

// JobStep is one step of a [Job]: its name, GitHub's status (pending,
// in_progress, completed) and conclusion, and when it started and finished —
// zero for a step that has not.
type JobStep struct {
	Name        string
	Status      string
	Conclusion  string
	StartedAt   time.Time
	CompletedAt time.Time
}

// The words GitHub writes a job's and a run's status in, of those a reader
// acts on.
const (
	JobQueued     = "queued"
	JobInProgress = "in_progress"
	RunCompleted  = "completed"
)

// ErrLogNotReady is a job's log refused because the job has not finished:
// GitHub has no log of a running job to give.
var ErrLogNotReady = errors.New("GitHub gives no log until the job ends")

// logBlobMissing is what the jobs logs endpoint's 404 carries for a job not
// yet finished: the endpoint redirects to blob storage, which has no blob yet.
const logBlobMissing = "BlobNotFound"

// jobPath is the REST path of one job of ref's repository.
func jobPath(ref ActionsRef) (string, error) {
	if ref.Owner == "" || ref.Repo == "" || ref.Job == "" {
		return "", fmt.Errorf("%s api needs a job's repository and id to read it", Binary)
	}
	return fmt.Sprintf("repos/%s/%s/actions/jobs/%s", ref.Owner, ref.Repo, ref.Job), nil
}

// ActionsJob is the job ref names, read through the REST API — the one read
// of a job that answers while it is still running.
//
// Observed against a running public job (gh 2.83.1, October 2026):
// `gh api repos/<o>/<r>/actions/jobs/<job>` answers for a job in progress with
// its status ("in_progress"), started_at, runner_name and every step, each
// step's status "completed", "in_progress" or "pending" — a pending step's
// started_at and completed_at null, the step under way's completed_at null.
// The owner, repository and job are all required, and refused before gh runs
// without them: the path needs every one.
func (c CLI) ActionsJob(dir string, ref ActionsRef) (Job, error) {
	path, err := jobPath(ref)
	if err != nil {
		return Job{}, err
	}
	out, err := c.runner.Run(dir, Binary, "api", path)
	if err != nil {
		logging.Error("could not read an Actions job", "run", ref.Run, "job", ref.Job, "code", exitCode(err))
		return Job{}, err
	}
	var raw struct {
		Status     string    `json:"status"`
		Conclusion string    `json:"conclusion"`
		CreatedAt  time.Time `json:"created_at"`
		StartedAt  time.Time `json:"started_at"`
		Runner     string    `json:"runner_name"`
		Steps      []struct {
			Name        string    `json:"name"`
			Status      string    `json:"status"`
			Conclusion  string    `json:"conclusion"`
			StartedAt   time.Time `json:"started_at"`
			CompletedAt time.Time `json:"completed_at"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		logging.Error("could not read what gh said about an Actions job", "run", ref.Run, "job", ref.Job)
		return Job{}, fmt.Errorf("%s api printed no readable job: %w", Binary, err)
	}
	job := Job{Status: raw.Status, Conclusion: raw.Conclusion, CreatedAt: raw.CreatedAt, StartedAt: raw.StartedAt, Runner: raw.Runner}
	for _, s := range raw.Steps {
		job.Steps = append(job.Steps, JobStep(s))
	}
	return job, nil
}

// JobLog is the log of the job ref names, as far as GitHub will give it,
// through the jobs logs endpoint [CLI.FailedLog] already falls back on.
//
// Observed against a running public job (gh 2.83.1, October 2026): the
// endpoint refuses a job still running — gh follows its redirect to blob
// storage, exits 1 saying "gh: HTTP 404", and prints the storage's
// `<Code>BlobNotFound</Code>` body on stdout — and answers with the whole log
// the moment the job completes. That refusal is [ErrLogNotReady]; any other
// failure is returned as it came. There is no public way to read the log of a
// step still running: the web UI's live log is a private endpoint.
func (c CLI) JobLog(dir string, ref ActionsRef) (string, error) {
	path, err := jobPath(ref)
	if err != nil {
		return "", err
	}
	out, err := c.runner.Run(dir, Binary, "api", path+"/logs")
	var exitErr *ExitError
	if err != nil && errors.As(err, &exitErr) && strings.Contains(out, logBlobMissing) {
		return "", ErrLogNotReady
	}
	if err != nil {
		logging.Error("could not read an Actions job's log", "run", ref.Run, "job", ref.Job, "code", exitCode(err))
		return "", err
	}
	return strings.TrimRight(out, "\n"), nil
}

// RunStatus is GitHub's status word for the run ref names — "completed" once
// every job of it has finished, cancelled ones included.
func (c CLI) RunStatus(dir string, ref ActionsRef) (string, error) {
	if ref.Run == "" {
		return "", fmt.Errorf("%s run view needs a run to read", Binary)
	}
	out, err := c.runner.Run(dir, Binary, runArgs(ref, "run", "view", ref.Run, "--json", "status")...)
	if err != nil {
		logging.Error("could not read an Actions run's status", "run", ref.Run, "code", exitCode(err))
		return "", err
	}
	var raw struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return "", fmt.Errorf("%s run view printed no readable status: %w", Binary, err)
	}
	return raw.Status, nil
}

// CancelRun cancels the whole run ref names: `gh run cancel` — GitHub cancels
// runs, never one job of one, so every job of it still going stops. gh
// refuses a run already completed ("Cannot cancel a workflow run that is
// completed", a 409 from GitHub).
func (c CLI) CancelRun(dir string, ref ActionsRef) error {
	if ref.Run == "" {
		return fmt.Errorf("%s run cancel needs a run to cancel", Binary)
	}
	_, err := c.runner.Run(dir, Binary, runArgs(ref, "run", "cancel", ref.Run)...)
	logRunCall("cancel", ref, err)
	return err
}

// RerunRun re-runs the run ref names: whole, or — failedOnly — its failed
// jobs and the jobs that depend on them (`gh run rerun --failed`, GitHub's
// "Re-run failed jobs").
//
// GitHub refuses a re-run of a run still going: per gh's source (v2.83.1,
// pkg/cmd/run/rerun), GitHub answers 403 and gh exits saying "run <id> cannot
// be rerun; <GitHub's message>" — so a caller cancels a run still going and
// waits for it to read completed first. GitHub's documentation does not say
// whether "failed jobs" covers cancelled ones, so a run cancelled to be
// re-run is re-run whole, never with failedOnly.
func (c CLI) RerunRun(dir string, ref ActionsRef, failedOnly bool) error {
	if ref.Run == "" {
		return fmt.Errorf("%s run rerun needs a run to re-run", Binary)
	}
	args := []string{"run", "rerun", ref.Run}
	if failedOnly {
		args = append(args, "--failed")
	}
	_, err := c.runner.Run(dir, Binary, runArgs(ref, args...)...)
	logRunCall("rerun", ref, err)
	return err
}

// RerunJob re-runs the one job ref names and the jobs that depend on it
// (`gh run rerun --job`). The job is the id a check's URL carries
// (`/actions/runs/<run>/job/<id>`), which is the databaseId gh wants — not
// the step number of the web UI's `/jobs/<n>` URLs gh's help warns about.
// Refused by GitHub, as [CLI.RerunRun] is, while the run is still going.
func (c CLI) RerunJob(dir string, ref ActionsRef) error {
	if ref.Job == "" {
		return fmt.Errorf("%s run rerun --job needs a job to re-run", Binary)
	}
	_, err := c.runner.Run(dir, Binary, runArgs(ref, "run", "rerun", "--job", ref.Job)...)
	logRunCall("rerun job", ref, err)
	return err
}

// runArgs is a `gh run` call pinned, where ref knows it, to the repository
// the run is in — which need not be the working directory's remote.
func runArgs(ref ActionsRef, args ...string) []string {
	if ref.Owner != "" && ref.Repo != "" {
		return append(args, "--repo", ref.Owner+"/"+ref.Repo)
	}
	return args
}

// logRunCall logs a call that acts on a run: the method, the ids, and the
// exit code where gh refused — never gh's own words.
func logRunCall(method string, ref ActionsRef, err error) {
	if err != nil {
		logging.Error("gh refused an Actions call", "method", method, "run", ref.Run, "job", ref.Job, "code", exitCode(err))
		return
	}
	logging.Action("Actions call sent", "method", method, "run", ref.Run, "job", ref.Job)
}

// exitCode is gh's exit code where it ran and refused, -1 where it never ran.
func exitCode(err error) int {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return -1
}
