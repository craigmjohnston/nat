package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/store"
)

// PRBatchReader is what pr-status — and every command that reads a pull
// request's state rather than its whole conversation — needs of the GitHub
// CLI: one batched reading ([gh.CLI.ReadPRs]), the way
// [internal/tui.PRReader] does for the board's own background reading.
type PRBatchReader interface {
	ReadPRs(q gh.BatchQuery) (gh.Batch, error)
}

// PRPoller is the polling half of the batched reading, which keeps GitHub's
// budget ([gh.Budget]): PollPRs reads nothing while a refusal's stop holds,
// and Outlook is the budget's policy for the next polling read — what
// pr-status reports under rate_limit.
type PRPoller interface {
	PollPRs(q gh.BatchQuery) (gh.Batch, error)
	Outlook(poll time.Duration) gh.Outlook
}

// prStatus prints the board's own PR-readiness reading, headlessly, for every
// project named — `--project` repeats — from one batched GitHub reading: one
// GraphQL document (per [gh.CLI.ReadPRs]'s chunk) naming every pull request
// worth asking about ([actions.PRsWorthAsking]) by number, every live ad hoc
// session's branches, and with `--detail <PR URL>` that one pull request in
// full. It costs a point an hour of GitHub's budget whatever the number of
// projects, where a listing per repository cost two a repository; see
// internal/gh's CLAUDE.md for the measured costs.
//
// It is also where a merge made on GitHub itself reaches Notion for anything
// that polls through this command: an in-progress slice whose pull request
// reads merged is marked Done — see [actions.SettleMerged]. It is likewise
// where a slice Done under the old rule — at approve, rather than at the
// merge — is caught and written back to In progress once its pull request
// reads open — see [actions.ReopenUnmerged], the mirror of SettleMerged and of
// internal/tui/prstate.go's own un-done rule. Between the two, these are the
// only writes this read can make, and only ever the writes the facts already
// earned.
//
// And it is where a worktree nothing witnessed the end of goes: a merge
// settled here takes its slice's worktree with it, and [landed] names every
// other Done slice whose worktree there is nothing left to do in, for
// [actions.SweepLanded] to remove.
//
// What the reading found that a later command wants without asking GitHub
// again — each pull request's base, each session's pull requests — is kept on
// disk ([lastReading]).
//
// It is a polling read ([gh.CLI.PollPRs]): while GitHub's refusal stop holds
// it runs no gh and reads everything unread. `--settle` is the read that
// follows an action — gnat's settle read — which is an action's read and
// always runs. Either way rate_limit carries the budget's policy for the next
// polling read ([gh.Outlook]).
func prStatus(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("pr-status", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	var projectRefs stringList
	flags.Var(&projectRefs, "project", "a project to read, by page `ID` (required; repeatable, one reading for all)")
	detailURL := flags.String("detail", "", "also read this pull request in full, by `URL`, as pr-view prints it")
	settle := flags.Bool("settle", false, "read after an action: run gh even while polling is paused or throttled")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("pr-status: unexpected argument %q", rest[0])
	}
	var detail *gh.PRRef
	if *detailURL != "" {
		ref, ok := gh.ParsePRURL(*detailURL)
		if !ok {
			return usageErrorf("pr-status: --detail wants a pull request URL, given %q", *detailURL)
		}
		detail = &ref
	}
	if len(projectRefs) == 0 {
		projectRefs = stringList{""}
	}

	var runs []*projectReading
	seen := map[string]bool{}
	for _, ref := range projectRefs {
		run, err := openProjectReading(ctx, env, ref)
		if err != nil {
			return err
		}
		if !seen[run.id] {
			seen[run.id] = true
			runs = append(runs, run)
		}
	}

	ghClient := env.NewGH()
	batch, asked := readBatch(ghClient, runs, detail, *settle)
	var budget *budgetReport
	if asked {
		budget = &budgetReport{outlook: ghClient.Outlook(runs[0].poll), cost: batch.Cost}
	}

	kept := env.loadLastReading()
	tmux := env.NewTmux()
	nudge := false
	for _, run := range runs {
		if run.settle(ctx, env, tmux, batch) {
			nudge = true
		}
		run.keep(&kept, batch)
	}
	if detail != nil && batch.Detail != nil {
		kept.Bases[gh.NormaliseURL(*detailURL)] = batch.Detail.BaseRefName
	}
	env.saveLastReading(kept)
	if nudge {
		env.nudged()
	}

	if asJSON := *asJSON; asJSON {
		return writeJSON(env.Out, prStatusOutput(runs, batch, budget))
	}
	_, err = io.WriteString(env.Out, prStatusText(runs, batch, budget))
	return err
}

// projectReading is one project of a pr-status run: what it is, its plan,
// and — once the batch is read — what the reading found for it.
type projectReading struct {
	id        string
	project   config.ProjectConfig
	st        store.Store
	slices    []domain.Slice
	worktrees actions.Worktrees
	// asked is every slice whose pull request the batch asks about, and refs
	// each one's pull request, by slice ID — a URL that names none is left
	// out of both, unread.
	asked []domain.Slice
	refs  map[string]gh.PRRef
	// sessions is every live-or-gone (not ended) ad hoc session's branches
	// to ask about, in the project's own order.
	sessions []sessionHeads

	readings []prReading
	branches []branchReading

	// poll is the configured interval between polling readings, which the
	// budget stretches.
	poll time.Duration
}

// openProjectReading resolves one --project and reads its plan, its
// worktrees and its sessions' branches: everything a reading of it asks.
func openProjectReading(ctx context.Context, env Env, ref string) (*projectReading, error) {
	cfg, projectID, project, err := env.projectFor(ref)
	if err != nil {
		return nil, err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return nil, err
	}
	plan, err := st.Plan(ctx, storeProject(projectID, project))
	if err != nil {
		return nil, fmt.Errorf("load slices: %w", err)
	}
	run := &projectReading{id: projectID, project: project, st: st, slices: plan.Project.Slices,
		worktrees: actions.ListedOnce(env.NewWorktrees()), refs: map[string]gh.PRRef{}, poll: cfg.PollInterval()}
	for _, s := range actions.PRsWorthAsking(run.worktrees, project, run.slices) {
		ref, ok := gh.ParsePRURL(s.PRURL)
		if !ok {
			logging.Action("left a pull request unread: its URL names none", "slice", s.ID, "pr", s.PRURL)
			continue
		}
		run.asked = append(run.asked, s)
		run.refs[s.ID] = ref
	}
	sessions, err := st.Sessions(ctx, storeProject(projectID, project))
	if err != nil {
		logging.Action("left a project's sessions unread", "project", projectID, "error", err)
	}
	gitCLI := env.NewGit()
	for _, sess := range sessions {
		if !sess.Ended() {
			run.sessions = append(run.sessions, headsOf(gitCLI, sess))
		}
	}
	return run, nil
}

// readBatch is the one reading every project of the run shares: each pull
// request asked about once, however many projects name it, every session's
// branches, and the detail — a polling read, or with settle an action's. It
// reports whether anything was asked: nothing to ask is no reading at all —
// no gh, and no rate limit to report.
func readBatch(reader GH, runs []*projectReading, detail *gh.PRRef, settle bool) (gh.Batch, bool) {
	var q gh.BatchQuery
	q.Detail = detail
	asked := map[gh.PRRef]bool{}
	heads := map[gh.HeadRef]bool{}
	for _, run := range runs {
		for _, s := range run.asked {
			if ref := run.refs[s.ID]; !asked[ref] {
				asked[ref] = true
				q.PRs = append(q.PRs, ref)
			}
		}
		for _, sess := range run.sessions {
			for _, h := range sess.heads() {
				if !heads[h] {
					heads[h] = true
					q.Heads = append(q.Heads, h)
				}
			}
		}
	}
	if len(q.PRs) == 0 && len(q.Heads) == 0 && q.Detail == nil {
		return gh.Batch{PRs: map[gh.PRRef]gh.PR{}, Heads: map[gh.HeadRef][]gh.HeadPR{}}, false
	}
	read := reader.PollPRs
	if settle {
		read = reader.ReadPRs
	}
	batch, err := read(q)
	if err != nil {
		// gh has logged each failed document; this is the decision taken
		// about it. What a failed document asked is absent from the batch,
		// and an absent pull request concludes nothing below.
		logging.Action("read the pull requests in part", "error", err)
	}
	return batch, true
}

// settle is what the reading does to one project: the readings, the merges
// and un-dones they earn, failing checks noticed, landed worktrees swept, and
// the hand-backs awaiting review tested. Reports whether a write was made.
func (run *projectReading) settle(ctx context.Context, env Env, tmux *agent.Tmux, batch gh.Batch) bool {
	read := map[string]gh.PRStatus{}
	for _, s := range run.asked {
		if pr, ok := batch.PRs[run.refs[s.ID]]; ok {
			read[s.ID] = gh.StatusOf(pr)
		}
	}
	var marked bool
	run.readings, marked = prReadings(ctx, run.st, run.worktrees, run.slices, read, run.project)
	if noticeFailing(ctx, run.st, tmux, run.id, run.slices, run.readings) {
		marked = true
	}
	actions.SweepLanded(run.worktrees, tmux.LiveSlices, run.project, landed(run.slices, read))
	run.branches = branchReadings(env.gitFor(run.project), run.slices, run.project)
	return marked
}

// keep files what the reading found for this project into the last reading
// kept on disk: each pull request's base, and each session's branches.
func (run *projectReading) keep(kept *lastReading, batch gh.Batch) {
	for _, s := range run.asked {
		if pr, ok := batch.PRs[run.refs[s.ID]]; ok {
			kept.Bases[gh.NormaliseURL(s.PRURL)] = pr.BaseRefName
		}
	}
	for _, sess := range run.sessions {
		if read := sess.read(batch); len(read) > 0 {
			kept.Sessions[sess.id] = read
		}
	}
}

// branchReading is one handed-back branch with no pull request yet, tested
// against its repository's default branch: the one state of a review nothing
// on GitHub can say anything about, since there is no pull request for
// GitHub to read the mergeability of.
type branchReading struct {
	SliceID     string
	SliceName   string
	Branch      string
	Base        string
	Conflicting bool
}

// awaitingReview reports whether a slice is a hand-back still to be approved:
// in progress, its branch recorded, no pull request opened from it. A resumed
// or sent-back slice has its branch cleared and is work in progress again, so
// it is not one; nor is a slice with a pull request, whose conflicts GitHub
// reads.
func awaitingReview(s domain.Slice) bool {
	return s.Status == domain.SliceClaimed && s.Branch != "" && s.PRURL == ""
}

// branchReadings tests every hand-back awaiting review for a conflict with
// its base, by [git.CLI.ConflictsWithBase], in the plan's own order. Each
// test fetches, so this costs a fetch per such slice — few at any one time,
// since a review either opens its pull request or goes back to the agent. A
// reading that comes back unknown is left out entirely: a branch nobody could
// test is not a broken one, and nothing downstream should draw it as either.
// A slice with no repository to test in (a source project's task that has not
// recorded one) is never asked about.
func branchReadings(g GitCLI, slices []domain.Slice, project config.ProjectConfig) []branchReading {
	var out []branchReading
	bases := map[string]string{}
	for _, s := range slices {
		if !awaitingReview(s) {
			continue
		}
		dir := actions.ExpandHome(actions.WorkdirFor(s, project))
		if dir == "" {
			continue
		}
		state := g.ConflictsWithBase(dir, s.Branch)
		if state == git.MergeUnknown {
			continue
		}
		base, seen := bases[dir]
		if !seen {
			base = g.Base(dir)
			bases[dir] = base
		}
		out = append(out, branchReading{SliceID: s.ID, SliceName: s.Name, Branch: s.Branch, Base: base,
			Conflicting: state == git.MergeConflicted})
	}
	return out
}

// branchJSON is one branch reading's entry: a handed-back branch with no pull
// request, and whether it conflicts with Base, the default branch it was
// tested against.
type branchJSON struct {
	SliceID     string `json:"slice_id"`
	Name        string `json:"name"`
	Branch      string `json:"branch"`
	Base        string `json:"base"`
	Conflicting bool   `json:"conflicting"`
}

// branchesJSON maps the branch readings onto their structured form — never
// nil, so the key always reads as a list.
func branchesJSON(readings []branchReading) []branchJSON {
	out := make([]branchJSON, 0, len(readings))
	for _, r := range readings {
		out = append(out, branchJSON{SliceID: r.SliceID, Name: r.SliceName, Branch: r.Branch, Base: r.Base,
			Conflicting: r.Conflicting})
	}
	return out
}

// branchesMarkdown names every handed-back branch read conflicting, under a
// heading of its own; nothing at all where none is.
func branchesMarkdown(readings []branchReading) string {
	out := ""
	for _, r := range readings {
		if r.Conflicting {
			out += fmt.Sprintf("- %s — %s — %s\n", r.SliceName, conflictLine(r.Base), r.Branch)
		}
	}
	if out == "" {
		return ""
	}
	return "\n# Branches awaiting review\n\n" + out
}

// prReading is one slice's pull request as pr-status reports it. Checks is
// how its checks stand and whether it conflicts, set only for an open pull
// request the listing read.
type prReading struct {
	SliceID   string
	SliceName string
	PR        string
	Readiness domain.PRReadiness
	Checks    *gh.PRStatus
}

// liveReader is what pr-status needs of tmux: which slices have a session.
type liveReader interface {
	actions.PromptSender
	LiveSlices() (map[string]string, error)
}

// noticeFailing hands every red pull request the readings found to
// [actions.NoticeFailingChecks] — the same function the board runs after its
// own reading — and reports whether it wrote anything. A tmux that cannot say
// which sessions are live concludes nothing: no agent is told and nothing is
// recorded as though none were there, and the next reading asks again.
func noticeFailing(ctx context.Context, st store.Store, tmux liveReader, projectID string,
	slices []domain.Slice, readings []prReading) bool {
	byID := make(map[string]domain.Slice, len(slices))
	for _, s := range slices {
		byID[s.ID] = s
	}
	var failing []actions.FailingChecks
	for _, r := range readings {
		if r.Readiness == domain.PRChecksFailing {
			failing = append(failing, actions.FailingChecks{Slice: byID[r.SliceID], Failing: r.Checks.Failing})
		}
	}
	if len(failing) == 0 {
		return false
	}
	live, err := tmux.LiveSlices()
	if err != nil {
		logging.Action("left failing pull requests unnoticed: live sessions unread", "error", err)
		return false
	}
	return actions.NoticeFailingChecks(ctx, st, tmux, live, projectID, failing)
}

// worthReadingPR reports whether a slice has a pull request that anything
// might still be waiting on: the same rule internal/tui/prstate.go's
// worthReading applies. A slice with none has nothing to ask about, and one
// neither in progress nor Done has not got as far as producing one.
func worthReadingPR(s domain.Slice) bool {
	if s.PRURL == "" {
		return false
	}
	return s.Status == domain.SliceClaimed || s.Status == domain.SliceDone
}

// readinessOf turns what gh said about an open pull request into the reading
// pr-status reports, the same mapping prstate.go's readinessOf makes: a
// failed check first, whatever the review says, then approved and mergeable
// is the review over, and anything else is a review still to come.
func readinessOf(status gh.PRStatus) domain.PRReadiness {
	if status.Checks == gh.ChecksFailing {
		return domain.PRChecksFailing
	}
	if status.Approved && status.Mergeable {
		return domain.PRReadyToMerge
	}
	return domain.PRAwaitingReview
}

// landed is every slice whose work has ended by what this reading saw, and
// whose worktree a sweep may take: Done, with no pull request or one the
// reading found merged or closed. A Done slice whose pull request reads open
// is one [actions.ReopenUnmerged] has just written back to In progress, and
// one not read — no worktree to settle, or a document that failed —
// concludes nothing, so neither is named. A pull request closed unmerged
// never made its slice Done, so it is no case of its own here.
func landed(slices []domain.Slice, read map[string]gh.PRStatus) []domain.Slice {
	var out []domain.Slice
	for _, s := range slices {
		if s.Status != domain.SliceDone {
			continue
		}
		if s.PRURL != "" {
			status, ok := read[s.ID]
			if !ok || !prEnded(status) {
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

// prReadings turns what the batch read — read, by slice ID, for every slice
// it asked about and found — into a reading per slice worth reporting, in the
// plan's own order, and reports whether any slice was written on the way, so
// the caller knows a nudge is owed.
//
// Only an open pull request carries a readiness. A slice whose pull request
// merged, closed, or was never read comes back with the zero
// [domain.PRReadiness] — unread — which is exactly how the board reads each:
// a pull request the reading never reached is worth exactly as much attention
// as one that has already landed.
//
// The exception a merge earns is an in-progress slice: GitHub made the merge
// nat would have, so the slice is marked Done ([actions.SettleMerged]) and
// its worktree goes ([actions.RemoveSliceWorktree]); a write that fails is
// logged and the next run tries again.
//
// The other exception is a Done slice whose pull request is still open: the
// un-done rule, [actions.ReopenUnmerged], written back to In progress so
// Done goes on meaning what the merge made true everywhere else the app reads
// a slice's status from.
func prReadings(ctx context.Context, st store.Store, worktrees actions.Worktrees, slices []domain.Slice,
	read map[string]gh.PRStatus, project config.ProjectConfig) ([]prReading, bool) {
	marked := false
	var out []prReading
	for _, s := range slices {
		if !worthReadingPR(s) {
			continue
		}
		r := prReading{SliceID: s.ID, SliceName: s.Name, PR: s.PRURL}
		status, ok := read[s.ID]
		switch {
		case !ok:
		case !prEnded(status):
			r.Readiness, r.Checks = readinessOf(status), &status
			if s.Status == domain.SliceDone {
				if err := actions.ReopenUnmerged(ctx, st, s); err != nil {
					logging.Action("left a Done slice with an open pull request unreopened", "slice", s.ID, "error", err)
				} else {
					marked = true
				}
			}
		case s.Status == domain.SliceClaimed:
			done, err := actions.SettleMerged(ctx, st, s, status)
			if err != nil {
				logging.Action("left a merged pull request unsettled", "slice", s.ID, "error", err)
			} else if done {
				actions.RemoveSliceWorktree(worktrees, s, project)
				marked = true
			}
		}
		out = append(out, r)
	}
	return out, marked
}

// prStatusDoc is the structured form of one project's reading: one entry per
// slice worth watching, in the plan's own order; under Branches, every
// hand-back awaiting review whose merge into its base could be tested
// ([branchReadings]); and under Sessions, each ad hoc session's pull
// requests. A run of one project carries the reading's own RateLimit and
// Detail beside them; a run of several keys each project's doc by its ID
// under Projects, and carries those two once, at the top ([prStatusMultiDoc]).
type prStatusDoc struct {
	Slices    []prStatusSliceJSON `json:"slices"`
	Branches  []branchJSON        `json:"branches"`
	Sessions  []sessionPRsJSON    `json:"sessions"`
	RateLimit *rateLimitJSON      `json:"rate_limit,omitempty"`
	Detail    *prDoc              `json:"detail,omitempty"`
}

// prStatusMultiDoc is a run of more than one project: each project's reading
// by its ID, and the one rate limit and detail the shared reading took.
type prStatusMultiDoc struct {
	Projects  map[string]prStatusDoc `json:"projects"`
	RateLimit *rateLimitJSON         `json:"rate_limit,omitempty"`
	Detail    *prDoc                 `json:"detail,omitempty"`
}

// rateLimitJSON is GitHub's GraphQL budget as the reading's document left it
// — else, a reading the budget's stop skipped, as the last one did — and the
// budget's policy for the next polling read: what it projects is left at the
// reset, whether polling is stretched (Throttled) or stopped (PausedUntil),
// the interval nat wants next, and what this reading spent. Absent where
// nothing was asked, since then no gh runs at all.
type rateLimitJSON struct {
	Limit            int        `json:"limit"`
	Remaining        int        `json:"remaining"`
	ResetAt          time.Time  `json:"reset_at"`
	Projected        int        `json:"projected_remaining_at_reset"`
	Throttled        bool       `json:"throttled"`
	PausedUntil      *time.Time `json:"paused_until,omitempty"`
	PollAfterSeconds int        `json:"poll_after_seconds"`
	Cost             int        `json:"cost"`
}

// budgetReport is what a reading that asked something says of the budget:
// the policy after it, and the points it spent.
type budgetReport struct {
	outlook gh.Outlook
	cost    int
}

// rateLimitOf is the run's rate_limit block: the reading's own rate limit
// where it read one, else the last the budget kept; nil where nothing was
// asked, or nothing was ever read and no stop holds.
func rateLimitOf(batch gh.Batch, budget *budgetReport) *rateLimitJSON {
	if budget == nil {
		return nil
	}
	o := budget.outlook
	rl := &rateLimitJSON{Projected: o.Projected, Throttled: o.Throttled,
		PollAfterSeconds: int(o.PollAfter / time.Second), Cost: budget.cost}
	switch {
	case batch.RateLimit != nil:
		rl.Limit, rl.Remaining, rl.ResetAt = batch.RateLimit.Limit, batch.RateLimit.Remaining, batch.RateLimit.ResetAt
		if o.Reading == nil {
			// No budget kept: nothing to project from but this reading.
			rl.Projected = rl.Remaining
		}
	case o.Reading != nil:
		rl.Limit, rl.Remaining, rl.ResetAt = o.Reading.Limit, o.Reading.Remaining, o.Reading.ResetAt
	case o.PausedUntil.IsZero():
		return nil
	}
	if !o.PausedUntil.IsZero() {
		at := o.PausedUntil
		rl.PausedUntil = &at
	}
	return rl
}

// budgetLine is the rate_limit block in words, for the markdown.
func budgetLine(rl *rateLimitJSON) string {
	if rl == nil {
		return ""
	}
	line := fmt.Sprintf("\nGitHub budget: %d of %d points left, resets at %s; %d projected at the reset; "+
		"this reading cost %d", rl.Remaining, rl.Limit, rl.ResetAt.Format(time.RFC3339), rl.Projected, rl.Cost)
	switch {
	case rl.PausedUntil != nil:
		line += fmt.Sprintf("; GitHub refused on its limit: polling paused until %s",
			rl.PausedUntil.Format(time.RFC3339))
	case rl.Throttled:
		line += "; throttled to keep the reserve"
	}
	return line + fmt.Sprintf("; next reading in %ds\n", rl.PollAfterSeconds)
}

// sessionPRsJSON is one ad hoc session's pull requests as the reading found
// them, every branch read; Stale where a branch could not be read.
type sessionPRsJSON struct {
	ID       string       `json:"id"`
	PRs      []headPRJSON `json:"prs"`
	PRsStale bool         `json:"prs_stale,omitempty"`
}

// prStatusOutput is the run's JSON: one project's doc as it stands, or every
// project's keyed by ID.
func prStatusOutput(runs []*projectReading, batch gh.Batch, budget *budgetReport) any {
	rl := rateLimitOf(batch, budget)
	var detail *prDoc
	if batch.Detail != nil {
		d := prJSON(*batch.Detail)
		detail = &d
	}
	docs := make(map[string]prStatusDoc, len(runs))
	for _, run := range runs {
		doc := prStatusJSON(run.readings)
		doc.Branches = branchesJSON(run.branches)
		doc.Sessions = make([]sessionPRsJSON, 0, len(run.sessions))
		for _, sess := range run.sessions {
			prs, stale := sess.prs(batch)
			doc.Sessions = append(doc.Sessions, sessionPRsJSON{ID: sess.id, PRs: headPRsJSON(prs), PRsStale: stale})
		}
		docs[run.id] = doc
	}
	if len(runs) == 1 {
		doc := docs[runs[0].id]
		doc.RateLimit, doc.Detail = rl, detail
		return doc
	}
	return prStatusMultiDoc{Projects: docs, RateLimit: rl, Detail: detail}
}

// prStatusText is the run's markdown: each project's pull requests and
// conflicted hand-backs — under the project's name where there are several —
// then the budget line and the detail.
func prStatusText(runs []*projectReading, batch gh.Batch, budget *budgetReport) string {
	out := ""
	for _, run := range runs {
		if len(runs) > 1 {
			out += fmt.Sprintf("# %s\n\n", run.project.Name)
		}
		out += prStatusMarkdown(run.readings) + branchesMarkdown(run.branches)
		if len(runs) > 1 {
			out += "\n"
		}
	}
	out += budgetLine(rateLimitOf(batch, budget))
	if batch.Detail != nil {
		out += "\n" + prMarkdown(*batch.Detail)
	}
	return out
}

// prStatusSliceJSON is one slice's entry. Conflicting is GitHub positively
// saying the branch conflicts with Base — false for a mergeable branch, one
// whose mergeability is still unknown, and every slice the listing did not
// read, since a read that never happened concludes nothing. Base is the
// branch the pull request merges into, where the listing read it.
type prStatusSliceJSON struct {
	SliceID     string        `json:"slice_id"`
	Name        string        `json:"name"`
	PR          string        `json:"pr"`
	Readiness   string        `json:"readiness"`
	Conflicting bool          `json:"conflicting"`
	Base        string        `json:"base,omitempty"`
	Checks      *prChecksJSON `json:"checks,omitempty"`
}

// prChecksJSON is how an open pull request's checks stand: the verdict in
// [gh.ChecksVerdict]'s words, every failed check by name and run URL, and
// every check at all, in gh's order, with the raw state it gave — so a reader
// lists them from the same reading the verdict came from.
type prChecksJSON struct {
	Verdict string             `json:"verdict"`
	Failing []prCheckJSON      `json:"failing"`
	Checks  []prCheckStateJSON `json:"checks"`
}

type prCheckStateJSON struct {
	Name  string `json:"name"`
	State string `json:"state"`
	URL   string `json:"url"`
}

type prCheckJSON struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// prStatusJSON maps the readings onto the structured form, in
// [domain.PRReadiness]'s own words, so a consumer reads the same vocabulary
// the board's own state does.
func prStatusJSON(readings []prReading) prStatusDoc {
	doc := prStatusDoc{Slices: make([]prStatusSliceJSON, 0, len(readings))}
	for _, r := range readings {
		entry := prStatusSliceJSON{SliceID: r.SliceID, Name: r.SliceName, PR: r.PR, Readiness: r.Readiness.String()}
		if r.Checks != nil {
			checks := &prChecksJSON{
				Verdict: r.Checks.Checks.String(), Failing: []prCheckJSON{}, Checks: []prCheckStateJSON{},
			}
			for _, c := range r.Checks.Failing {
				checks.Failing = append(checks.Failing, prCheckJSON{Name: c.Name, URL: c.URL})
			}
			for _, c := range r.Checks.All {
				checks.Checks = append(checks.Checks, prCheckStateJSON{Name: c.Name, State: c.State, URL: c.URL})
			}
			entry.Checks = checks
			entry.Conflicting, entry.Base = r.Checks.Conflicting, r.Checks.Base
		}
		doc.Slices = append(doc.Slices, entry)
	}
	return doc
}

// prStatusMarkdown renders the readings as a list, one line per slice.
func prStatusMarkdown(readings []prReading) string {
	out := "# Pull requests\n\n"
	if len(readings) == 0 {
		return out + "_none_\n"
	}
	for _, r := range readings {
		out += fmt.Sprintf("- %s — %s — %s\n", r.SliceName, r.Readiness, r.PR)
		if r.Checks != nil {
			for _, c := range r.Checks.Failing {
				out += fmt.Sprintf("  - failing: %s %s\n", c.Name, c.URL)
			}
			if r.Checks.Conflicting {
				out += "  - " + conflictLine(r.Checks.Base) + "\n"
			}
		}
	}
	return out
}

// prEnded is whether a reading found the pull request merged or closed —
// GitHub's two words for one that is no longer open. Any other word is read
// as open, which is what OPEN is.
func prEnded(status gh.PRStatus) bool {
	return status.State == gh.PRStateMerged || status.State == gh.PRStateClosed
}

// readOnePR is one pull request's state off a batched reading of it alone —
// a point of GitHub's budget, where `gh pr view` was one too but read the
// whole conversation to get it. The one-off actions that only need to know
// where a pull request stands (pr-merge, slice-checks-rerun and -cancel)
// read it so. A URL that names no pull request, a reading that failed, and
// one GitHub could not resolve are each refused, since each of those
// actions spends or ends something on the strength of the answer.
func readOnePR(reader PRBatchReader, url string) (gh.PR, error) {
	ref, ok := gh.ParsePRURL(url)
	if !ok {
		return gh.PR{}, fmt.Errorf("%s names no pull request", url)
	}
	batch, err := reader.ReadPRs(gh.BatchQuery{PRs: []gh.PRRef{ref}})
	if err != nil {
		return gh.PR{}, err
	}
	pr, read := batch.PRs[ref]
	if !read {
		return gh.PR{}, fmt.Errorf("GitHub has no pull request at %s", url)
	}
	return pr, nil
}

// conflictLine says a pull request conflicts, naming its base where the
// listing read one.
func conflictLine(base string) string {
	if base == "" {
		return "conflicting"
	}
	return "conflicting with " + base
}
