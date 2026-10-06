package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/logging"
)

// RunController is what slice-checks-rerun and slice-checks-cancel need of
// gh: a run's status, and cancelling and re-running runs and jobs.
type RunController interface {
	RunStatus(dir string, ref gh.ActionsRef) (string, error)
	CancelRun(dir string, ref gh.ActionsRef) error
	RerunRun(dir string, ref gh.ActionsRef, failedOnly bool) error
	RerunJob(dir string, ref gh.ActionsRef) error
}

// How long slice-checks-rerun waits for a run it cancelled to read completed
// before re-running it: a poll every rerunPollEvery, rerunPolls times — about
// two minutes. checksSleep is the wait, time.Sleep in production and nothing
// in tests.
const (
	rerunPollEvery = 5 * time.Second
	rerunPolls     = 24
)

var checksSleep = time.Sleep

// prStateOpen is GitHub's word for a pull request still open — the only
// state the checks of one are worth spending CI on.
const prStateOpen = "OPEN"

// ciRun is one Actions run behind a pull request's checks, with every check
// of it, in the rollup's order.
type ciRun struct {
	ref    gh.ActionsRef
	checks []ciCheck
}

// ciCheck is one check of a run, with the job its URL names (Job "" where it
// names none).
type ciCheck struct {
	check gh.Check
	ref   gh.ActionsRef
}

// going is whether any job of the run is still queued or running.
func (r ciRun) going() bool {
	return slices.ContainsFunc(r.checks, func(c ciCheck) bool { return c.check.Outcome() == gh.CheckPending })
}

// names is the names of the run's checks that keep says to.
func (r ciRun) names(keep func(gh.Check) bool) []string {
	var out []string
	for _, c := range r.checks {
		if keep(c.check) {
			out = append(out, c.check.Name)
		}
	}
	return out
}

func pendingCheck(c gh.Check) bool { return c.Outcome() == gh.CheckPending }
func failedCheck(c gh.Check) bool  { return c.Outcome() == gh.CheckFailing }
func anyCheck(gh.Check) bool       { return true }

// ciTarget is what both commands act on: the slice's pull request, read fresh
// and still open, its checks grouped into the Actions runs behind them —
// several checks of one run are one run — and the names of the checks no run
// is behind.
type ciTarget struct {
	dir      string
	checks   []gh.Check
	runs     []*ciRun
	external []string
}

// runOf is the run a check belongs to.
func (t ciTarget) runOf(name string) (*ciRun, ciCheck, bool) {
	for _, r := range t.runs {
		for _, c := range r.checks {
			if c.check.Name == name {
				return r, c, true
			}
		}
	}
	return nil, ciCheck{}, false
}

// checkNames is every check's name, for a refusal naming one that is not.
func (t ciTarget) checkNames() string {
	names := make([]string, len(t.checks))
	for i, c := range t.checks {
		names[i] = c.Name
	}
	return strings.Join(names, ", ")
}

// loadCITarget loads the slice and its pull request — its state and its
// checks' run URLs, off one batched reading of it alone ([readOnePR]). No
// pull request is refused, and so is one that is not open — asking GitHub,
// and refusing where it cannot answer, since the cost of being wrong is CI
// spent or killed on a review that is over.
func loadCITarget(ctx context.Context, command, sliceRef, projectRef string, env Env) (ciTarget, GH, error) {
	id, err := pageID(command, sliceRef)
	if err != nil {
		return ciTarget{}, nil, err
	}
	_, projectID, project, err := env.projectFor(projectRef)
	if err != nil {
		return ciTarget{}, nil, err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return ciTarget{}, nil, err
	}
	s, _, err := st.Slice(ctx, id)
	if err != nil {
		return ciTarget{}, nil, fmt.Errorf("load the slice: %w", err)
	}
	if s.PRURL == "" {
		return ciTarget{}, nil, fmt.Errorf("%s: %q has no pull request recorded: no checks to act on", command, s.Name)
	}
	ghClient := env.NewGH()
	dir := actions.WorkdirFor(s, project)
	pr, err := readOnePR(ghClient, s.PRURL)
	if err != nil {
		return ciTarget{}, nil, fmt.Errorf("%s: read the pull request %s: %w", command, s.PRURL, err)
	}
	if pr.State != prStateOpen {
		return ciTarget{}, nil, fmt.Errorf("%s: the pull request %s is %s, not open: its review is over", command, s.PRURL, strings.ToLower(pr.State))
	}
	t := ciTarget{dir: dir, checks: pr.Checks}
	byRun := map[string]*ciRun{}
	for _, c := range pr.Checks {
		ref, ok := gh.ActionsRun(c.URL)
		if !ok {
			t.external = append(t.external, c.Name)
			continue
		}
		key := ref.Owner + "/" + ref.Repo + "/" + ref.Run
		run, seen := byRun[key]
		if !seen {
			run = &ciRun{ref: gh.ActionsRef{Owner: ref.Owner, Repo: ref.Repo, Run: ref.Run}}
			byRun[key] = run
			t.runs = append(t.runs, run)
		}
		run.checks = append(run.checks, ciCheck{check: c, ref: ref})
	}
	return t, ghClient, nil
}

// namedRuns is the runs behind the checks names, each once, in rollup order,
// with the checks named in each. A name that is no check, or a check no
// Actions run is behind, is refused.
func (t ciTarget) namedRuns(command string, names []string) ([]*ciRun, map[*ciRun][]ciCheck, error) {
	asked := map[*ciRun][]ciCheck{}
	for _, name := range names {
		run, c, ok := t.runOf(name)
		if !ok {
			if slices.Contains(t.external, name) {
				return nil, nil, fmt.Errorf("%s: %q was reported by a service other than GitHub Actions: nat cannot act on it", command, name)
			}
			return nil, nil, fmt.Errorf("%s: no check is named %q — the checks are: %s", command, name, t.checkNames())
		}
		if !slices.ContainsFunc(asked[run], func(a ciCheck) bool { return a.check.Name == name }) {
			asked[run] = append(asked[run], c)
		}
	}
	var runs []*ciRun
	for _, r := range t.runs {
		if _, ok := asked[r]; ok {
			runs = append(runs, r)
		}
	}
	return runs, asked, nil
}

// checksActionDoc is what both commands report: the checks cancelled (those
// asked for and every sibling stopped with them), the checks re-run, and the
// checks skipped for having no Actions run behind them. Always arrays.
type checksActionDoc struct {
	Cancelled []string `json:"cancelled"`
	Rerun     []string `json:"rerun,omitempty"`
	Skipped   []string `json:"skipped"`
}

// sliceChecksRerun re-runs a slice's pull request's checks: every run whole
// (--all), the failed jobs of each run with one (--failed), or named checks'
// jobs (--check, repeatable). Exactly one.
//
// Where a run it would touch is still going, it cancels that run first,
// waits for it to read completed ([waitCompleted]) and only then re-runs —
// GitHub refuses a re-run of a run still going, and no caller should have to
// cancel and re-run as two steps. A cancel stops every job of the run, so a
// run it cancelled is re-run whole, siblings and all, rather than left with
// jobs stranded as cancelled; what it cancelled is reported apart from what
// it re-ran.
func sliceChecksRerun(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-checks-rerun", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	all := flags.Bool("all", false, "re-run every Actions run behind the checks whole")
	failed := flags.Bool("failed", false, "re-run each run's failed jobs")
	var named stringList
	flags.Var(&named, "check", "re-run the job of the check of this name (repeatable)")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-checks-rerun: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	modes := 0
	for _, on := range []bool{*all, *failed, len(named) > 0} {
		if on {
			modes++
		}
	}
	if modes != 1 {
		return usageErrorf("slice-checks-rerun: want exactly one of --all, --failed or --check")
	}
	t, ghClient, err := loadCITarget(ctx, "slice-checks-rerun", rest[0], *projectRef, env)
	if err != nil {
		return err
	}

	var runs []*ciRun
	asked := map[*ciRun][]ciCheck{}
	doc := checksActionDoc{Cancelled: []string{}, Rerun: []string{}, Skipped: []string{}}
	switch {
	case *all:
		runs = t.runs
		doc.Skipped = append(doc.Skipped, t.external...)
	case *failed:
		for _, r := range t.runs {
			if slices.ContainsFunc(r.checks, func(c ciCheck) bool { return failedCheck(c.check) }) {
				runs = append(runs, r)
			}
		}
		for _, c := range t.checks {
			if failedCheck(c) && slices.Contains(t.external, c.Name) {
				doc.Skipped = append(doc.Skipped, c.Name)
			}
		}
	default:
		if runs, asked, err = t.namedRuns("slice-checks-rerun", named); err != nil {
			return err
		}
	}
	if len(runs) == 0 {
		if *failed {
			return fmt.Errorf("slice-checks-rerun: no GitHub Actions check has failed: nothing to re-run")
		}
		return fmt.Errorf("slice-checks-rerun: no check has a GitHub Actions run behind it: nothing to re-run")
	}

	var cancelled []*ciRun
	for _, r := range runs {
		if !r.going() {
			continue
		}
		stopping := r.names(pendingCheck)
		if err := ghClient.CancelRun(t.dir, r.ref); err != nil {
			return fmt.Errorf("slice-checks-rerun: cancel the run of %s first: %w%s", strings.Join(stopping, ", "), err, sentSoFar(doc))
		}
		doc.Cancelled = append(doc.Cancelled, stopping...)
		cancelled = append(cancelled, r)
	}
	for _, r := range cancelled {
		if err := waitCompleted(ghClient, t.dir, r.ref); err != nil {
			return fmt.Errorf("slice-checks-rerun: cancelled %s, but %v — nothing was re-run", strings.Join(doc.Cancelled, ", "), err)
		}
	}
	for _, r := range runs {
		var err error
		switch {
		case slices.Contains(cancelled, r) || *all:
			err = ghClient.RerunRun(t.dir, r.ref, false)
			if err == nil {
				doc.Rerun = append(doc.Rerun, r.names(anyCheck)...)
			}
		case *failed:
			err = ghClient.RerunRun(t.dir, r.ref, true)
			if err == nil {
				doc.Rerun = append(doc.Rerun, r.names(failedCheck)...)
			}
		default:
			err = rerunNamed(ghClient, t.dir, r, asked[r], &doc)
		}
		if err != nil {
			return fmt.Errorf("slice-checks-rerun: re-run the run of %s: %w%s", strings.Join(r.names(anyCheck), ", "), err, sentSoFar(doc))
		}
	}
	env.nudged()
	if *asJSON {
		return writeJSON(env.Out, doc)
	}
	_, err = io.WriteString(env.Out, checksActionMarkdown(doc))
	return err
}

// rerunNamed re-runs the named checks of one run that was not going: each
// one's own job, or the run whole for a check whose URL names no job.
func rerunNamed(ghClient GH, dir string, r *ciRun, named []ciCheck, doc *checksActionDoc) error {
	for _, c := range named {
		if c.ref.Job == "" {
			if err := ghClient.RerunRun(dir, r.ref, false); err != nil {
				return err
			}
			doc.Rerun = append(doc.Rerun, r.names(anyCheck)...)
			return nil
		}
	}
	for _, c := range named {
		if err := ghClient.RerunJob(dir, c.ref); err != nil {
			return err
		}
		doc.Rerun = append(doc.Rerun, c.check.Name)
	}
	return nil
}

// sentSoFar is what a failure part-way through has already done, for its
// error: a caller has to know a cancel or re-run went out before it.
func sentSoFar(doc checksActionDoc) string {
	var parts []string
	if len(doc.Cancelled) > 0 {
		parts = append(parts, "cancelled "+strings.Join(doc.Cancelled, ", "))
	}
	if len(doc.Rerun) > 0 {
		parts = append(parts, "re-ran "+strings.Join(doc.Rerun, ", "))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (already " + strings.Join(parts, "; ") + ")"
}

// waitCompleted polls a cancelled run until it reads completed, about two
// minutes at most. A read that fails concludes nothing and is polled again.
// Each poll is `gh run view --json status`, GitHub's REST API — a budget of
// its own, apart from the GraphQL one every pull request reading spends, and
// one nat barely touches — so it stays a poll rather than riding a reading.
func waitCompleted(ghClient GH, dir string, ref gh.ActionsRef) error {
	for range rerunPolls {
		status, err := ghClient.RunStatus(dir, ref)
		if err == nil && status == gh.RunCompleted {
			return nil
		}
		if err != nil {
			logging.Action("could not read a cancelled run's status, polling again", "run", ref.Run)
		}
		checksSleep(rerunPollEvery)
	}
	return fmt.Errorf("run %s had not finished cancelling after %s", ref.Run, rerunPollEvery*rerunPolls)
}

// sliceChecksCancel cancels every Actions run behind a slice's pull
// request's checks that is still going, or — --check, repeatable — only the
// runs of the checks named. GitHub cancels runs, never one job, so every
// check still going in a cancelled run stops and is named. It returns once
// the cancels are sent, without waiting for them to land.
func sliceChecksCancel(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-checks-cancel", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	var named stringList
	flags.Var(&named, "check", "cancel only the run of the check of this name (repeatable)")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-checks-cancel: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	t, ghClient, err := loadCITarget(ctx, "slice-checks-cancel", rest[0], *projectRef, env)
	if err != nil {
		return err
	}
	doc := checksActionDoc{Cancelled: []string{}, Skipped: []string{}}
	runs := t.runs
	if len(named) > 0 {
		if runs, _, err = t.namedRuns("slice-checks-cancel", named); err != nil {
			return err
		}
	} else {
		for _, c := range t.checks {
			if pendingCheck(c) && slices.Contains(t.external, c.Name) {
				doc.Skipped = append(doc.Skipped, c.Name)
			}
		}
	}
	runs = slices.DeleteFunc(slices.Clone(runs), func(r *ciRun) bool { return !r.going() })
	if len(runs) == 0 {
		return fmt.Errorf("slice-checks-cancel: no GitHub Actions run behind those checks is still going: nothing to cancel")
	}
	for _, r := range runs {
		if err := ghClient.CancelRun(t.dir, r.ref); err != nil {
			return fmt.Errorf("slice-checks-cancel: cancel the run of %s: %w%s",
				strings.Join(r.names(pendingCheck), ", "), err, sentSoFar(doc))
		}
		doc.Cancelled = append(doc.Cancelled, r.names(pendingCheck)...)
	}
	env.nudged()
	if *asJSON {
		return writeJSON(env.Out, doc)
	}
	_, err = io.WriteString(env.Out, checksActionMarkdown(doc))
	return err
}

// checksActionMarkdown says what was cancelled, what was re-run and what was
// skipped, a line each where there is any.
func checksActionMarkdown(doc checksActionDoc) string {
	var b strings.Builder
	if len(doc.Cancelled) > 0 {
		verb := "Cancelled"
		if len(doc.Rerun) > 0 {
			verb = "Cancelled first, since a run still going cannot be re-run"
		}
		fmt.Fprintf(&b, "%s: %s\n", verb, strings.Join(doc.Cancelled, ", "))
	}
	if len(doc.Rerun) > 0 {
		fmt.Fprintf(&b, "Re-ran: %s\n", strings.Join(doc.Rerun, ", "))
	}
	if len(doc.Skipped) > 0 {
		fmt.Fprintf(&b, "Skipped, no GitHub Actions run behind them: %s\n", strings.Join(doc.Skipped, ", "))
	}
	return b.String()
}
