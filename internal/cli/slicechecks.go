package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/logging"
)

// RunLogReader is what slice-checks --log needs of gh: the failed steps' log
// of a GitHub Actions run, or of one job of it.
type RunLogReader interface {
	FailedLog(dir, run, job string) (string, error)
}

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
// logged and passed over, since the verdict and the URL are still the answer.
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
		if *withLog && c.Outcome() == gh.CheckFailing {
			entry.Log = failedLog(ghClient, workdir, c)
		}
		doc.Checks = append(doc.Checks, entry)
	}

	if *asJSON {
		return writeJSON(env.Out, doc)
	}
	_, err = io.WriteString(env.Out, checksMarkdown(doc))
	return err
}

// failedLog is a failed check's log as --log prints it, or "" for a check
// with no Actions run behind it, or one whose log could not be read.
func failedLog(ghClient GH, dir string, c gh.Check) string {
	run, job, ok := gh.ActionsRun(c.URL)
	if !ok {
		return ""
	}
	out, err := ghClient.FailedLog(dir, run, job)
	if err != nil {
		logging.Action("left a failed check's log out", "check", c.Name, "url", c.URL, "error", err)
		return ""
	}
	return lastLines(out, checksLogLines)
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
	PR      string      `json:"pr"`
	Verdict string      `json:"verdict"`
	Checks  []sliceCheckJSON `json:"checks"`
}

type sliceCheckJSON struct {
	Name  string `json:"name"`
	State string `json:"state"`
	URL   string `json:"url"`
	Log   string `json:"log,omitempty"`
}

// checksMarkdown renders the checks: the verdict, a line per check, and each
// log --log read under the check it belongs to.
func checksMarkdown(doc checksDoc) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Checks: %s — %s\n", doc.Verdict, doc.PR)
	if len(doc.Checks) > 0 {
		b.WriteString("\n")
	}
	for _, c := range doc.Checks {
		fmt.Fprintf(&b, "- %s — %s — %s\n", c.Name, c.State, c.URL)
	}
	for _, c := range doc.Checks {
		if c.Log != "" {
			fmt.Fprintf(&b, "\n## %s failed log\n\n```\n%s\n```\n", c.Name, c.Log)
		}
	}
	return b.String()
}
