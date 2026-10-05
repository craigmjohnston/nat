package cli

import (
	"context"
	"flag"
	"fmt"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/logging"
)

// RunLogReader is what slice-checks --log needs of gh: the failed steps' log
// of a GitHub Actions run, or of one job of it.
type RunLogReader interface {
	FailedLog(dir string, ref gh.ActionsRef) (string, error)
}

// JobReader is what slice-checks --log needs of gh for a check still
// running: the job behind it, read while it runs, and its log where GitHub
// will give one ([gh.ErrLogNotReady] where it will not yet).
type JobReader interface {
	ActionsJob(dir string, ref gh.ActionsRef) (gh.Job, error)
	JobLog(dir string, ref gh.ActionsRef) (string, error)
}

// checksNow is the clock a running job's durations are measured against —
// time.Now in production, a fixed instant in tests so durations are exact.
var checksNow = time.Now

// checksLogLines is how much of one job's failed log slice-checks --log
// prints: the last lines, where a failure says what went wrong, and a line
// saying how many before them were cut.
const checksLogLines = 200

// sliceChecks prints how a slice's pull request's checks stand: the verdict
// the board reads ([gh.Verdict], over the checks gh's own view of the pull
// request decodes), then every check by name, state and run URL. It is how an
// agent reads CI — the prompts name it, never `gh` — and a read alone: no
// write, no nudge, and any status, since a fix session's slice is in progress
// and a landed one's checks are still worth reading.
//
// --log appends, for each failed check run by GitHub Actions, the failed
// steps' log, cut to its last [checksLogLines] lines; a status some other
// service reported has only its URL to give. A log that cannot be read is
// logged and said to be unavailable under its check, naming why — the verdict
// and the URL are still the answer, and an agent reading it knows the log was
// tried rather than going to gh for it.
//
// For a check still pending with an Actions job behind its URL, --log reads
// the job ([runningJob]) and says under the check where it stands — waiting
// for a runner, or the step it is on — then every step; and its log where
// GitHub gives one. A pending check with no job named, or from another
// service, gets nothing more.
func sliceChecks(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-checks", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	withLog := flags.Bool("log", false, "append the failed steps' log of each failed GitHub Actions check")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-checks: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-checks", rest[0])
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
	s, _, err := st.Slice(ctx, id)
	if err != nil {
		return fmt.Errorf("load the slice: %w", err)
	}

	doc := checksDoc{PR: s.PRURL, Verdict: gh.ChecksNone.String(), Checks: []sliceCheckJSON{}}
	if s.PRURL == "" {
		if *asJSON {
			return writeJSON(env.Out, doc)
		}
		_, err := fmt.Fprintf(env.Out, "%q has no pull request recorded: its checks run once the slice is approved.\n", s.Name)
		return err
	}

	ghClient := env.NewGH()
	workdir := actions.WorkdirFor(s, project)
	pr, err := ghClient.ViewPR(workdir, s.PRURL)
	if err != nil {
		return fmt.Errorf("read the pull request %s: %w", s.PRURL, err)
	}
	verdict, _ := gh.Verdict(pr.Checks)
	doc.Verdict = verdict.String()
	for _, c := range pr.Checks {
		entry := sliceCheckJSON{Name: c.Name, State: c.State, URL: c.URL}
		switch {
		case *withLog && c.Outcome() == gh.CheckFailing:
			entry.Log, entry.LogError = failedLog(ghClient, workdir, c)
		case *withLog && c.Outcome() == gh.CheckPending:
			runningJob(ghClient, workdir, c, &entry)
		}
		doc.Checks = append(doc.Checks, entry)
	}

	if *asJSON {
		return writeJSON(env.Out, doc)
	}
	_, err = io.WriteString(env.Out, checksMarkdown(doc, checksNow()))
	return err
}

// failedLog is a failed check's log as --log prints it, or "" for a check
// with no Actions run behind it. A log that could not be read is "" with the
// first line of why as logErr.
func failedLog(ghClient GH, dir string, c gh.Check) (log, logErr string) {
	ref, ok := gh.ActionsRun(c.URL)
	if !ok {
		return "", ""
	}
	out, err := ghClient.FailedLog(dir, ref)
	if err != nil {
		logging.Action("left a failed check's log out", "check", c.Name, "url", c.URL, "error", err)
		return "", firstLineOf(err)
	}
	return lastLines(out, checksLogLines), ""
}

// runningJob fills in a pending check's job and log, where an Actions job is
// behind it. Each read that fails is logged and left as its first line of
// why — a job_error or log_error — and concludes nothing else. A log GitHub
// will not give before the job ends is neither: it is [sliceCheckJSON.LogPending].
func runningJob(ghClient GH, dir string, c gh.Check, entry *sliceCheckJSON) {
	ref, ok := gh.ActionsRun(c.URL)
	if !ok || ref.Job == "" {
		return
	}
	job, err := ghClient.ActionsJob(dir, ref)
	if err != nil {
		logging.Action("left a running check's job out", "check", c.Name, "url", c.URL, "error", err)
		entry.JobError = firstLineOf(err)
	} else {
		entry.Job = jobJSONOf(job)
	}
	out, err := ghClient.JobLog(dir, ref)
	switch {
	case errors.Is(err, gh.ErrLogNotReady):
		entry.LogPending = true
	case err != nil:
		logging.Action("left a running check's log out", "check", c.Name, "url", c.URL, "error", err)
		entry.LogError = firstLineOf(err)
	default:
		entry.Log = lastLines(out, checksLogLines)
	}
}

// firstLineOf is the first line of an error, which is what a check's line has
// room for.
func firstLineOf(err error) string {
	first, _, _ := strings.Cut(err.Error(), "\n")
	return first
}

// lastLines is the last n lines of text, after a line saying how many before
// them were cut, or the whole text where there are no more than n.
func lastLines(text string, n int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= n {
		return text
	}
	cut := len(lines) - n
	return fmt.Sprintf("… %d earlier lines cut\n", cut) + strings.Join(lines[cut:], "\n")
}

// checksDoc is slice-checks' structured form.
type checksDoc struct {
	PR      string           `json:"pr"`
	Verdict string           `json:"verdict"`
	Checks  []sliceCheckJSON `json:"checks"`
}

type sliceCheckJSON struct {
	Name  string `json:"name"`
	State string `json:"state"`
	URL   string `json:"url"`
	// Job is a pending check's Actions job as --log read it, and JobError
	// why it could not be read.
	Job      *jobJSON `json:"job,omitempty"`
	JobError string   `json:"job_error,omitempty"`
	Log      string   `json:"log,omitempty"`
	// LogError is why --log could not read a check's log.
	LogError string `json:"log_error,omitempty"`
	// LogPending is a running job's log GitHub will not give until it ends:
	// no error, and nothing to read yet.
	LogPending bool `json:"log_pending,omitempty"`
}

// jobJSON is one Actions job as slice-checks --json carries it. A time
// GitHub has not set is left out.
type jobJSON struct {
	Status    string     `json:"status"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	Runner    string     `json:"runner,omitempty"`
	Steps     []stepJSON `json:"steps"`
}

type stepJSON struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func jobJSONOf(job gh.Job) *jobJSON {
	out := &jobJSON{Status: job.Status, CreatedAt: timeOrNil(job.CreatedAt), StartedAt: timeOrNil(job.StartedAt),
		Runner: job.Runner, Steps: []stepJSON{}}
	for _, s := range job.Steps {
		out.Steps = append(out.Steps, stepJSON{Name: s.Name, Status: s.Status, Conclusion: s.Conclusion,
			StartedAt: timeOrNil(s.StartedAt), CompletedAt: timeOrNil(s.CompletedAt)})
	}
	return out
}

// timeOrNil is t, or nil where GitHub has not set it.
func timeOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// checksMarkdown renders the checks: the verdict, a line per check, and each
// log --log read under the check it belongs to.
func checksMarkdown(doc checksDoc, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Checks: %s — %s\n", doc.Verdict, doc.PR)
	if len(doc.Checks) > 0 {
		b.WriteString("\n")
	}
	for _, c := range doc.Checks {
		fmt.Fprintf(&b, "- %s — %s — %s\n", c.Name, c.State, c.URL)
		if c.JobError != "" {
			fmt.Fprintf(&b, "  job not available: %s\n", c.JobError)
		}
		if c.Job != nil {
			b.WriteString(jobMarkdown(*c.Job, now))
		}
		if c.LogError != "" {
			fmt.Fprintf(&b, "  log not available: %s\n", c.LogError)
		}
		if c.LogPending {
			fmt.Fprintf(&b, "  log: %s\n", gh.ErrLogNotReady)
		}
	}
	for _, c := range doc.Checks {
		if c.Log == "" {
			continue
		}
		heading := "failed log"
		if c.Job != nil {
			heading = "log"
		}
		fmt.Fprintf(&b, "\n## %s %s\n\n```\n%s\n```\n", c.Name, heading, c.Log)
	}
	return b.String()
}

// jobMarkdown is a running job under its check: where it stands, then each
// step with its status and how long it took or has taken so far, measured
// against now.
func jobMarkdown(job jobJSON, now time.Time) string {
	var b strings.Builder
	switch {
	case job.Status == gh.JobQueued || job.StartedAt == nil:
		since := job.CreatedAt
		if since == nil {
			since = job.StartedAt
		}
		b.WriteString("  queued, no runner yet")
		if since != nil {
			fmt.Fprintf(&b, " — waiting %s", elapsed(*since, now))
		}
		b.WriteString("\n")
	default:
		fmt.Fprintf(&b, "  %s for %s", strings.ReplaceAll(job.Status, "_", " "), elapsed(*job.StartedAt, now))
		if job.Runner != "" {
			fmt.Fprintf(&b, " on %s", job.Runner)
		}
		for _, s := range job.Steps {
			if s.Status == gh.JobInProgress && s.StartedAt != nil {
				fmt.Fprintf(&b, ", at step %q for %s", s.Name, elapsed(*s.StartedAt, now))
				break
			}
		}
		b.WriteString("\n")
	}
	if len(job.Steps) > 0 {
		b.WriteString("  steps:\n")
	}
	for _, s := range job.Steps {
		word := strings.ReplaceAll(s.Status, "_", " ")
		if s.Conclusion != "" {
			word = s.Conclusion
		}
		fmt.Fprintf(&b, "  - %s — %s", s.Name, word)
		switch {
		case s.StartedAt != nil && s.CompletedAt != nil:
			fmt.Fprintf(&b, " — %s", elapsed(*s.StartedAt, *s.CompletedAt))
		case s.StartedAt != nil:
			fmt.Fprintf(&b, " — %s so far", elapsed(*s.StartedAt, now))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// elapsed is the time from one instant to another to the second, never
// negative — a runner's clock a little ahead of this one reads as no time.
func elapsed(from, to time.Time) time.Duration {
	return max(to.Sub(from), 0).Round(time.Second)
}
