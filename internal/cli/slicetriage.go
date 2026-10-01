package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/store"
)

// sliceTriage carries the user's decision on a slice's pending follow-ups:
// each one queued as a Todo slice of its own (under the same milestone, blocked
// on this one), folded into this slice, or dropped. It is the decision the
// agent that proposed them is waiting on, so it ends by telling that agent —
// one message, whatever the mix.
//
// Every pending follow-up is decided at once or none is: a partial triage is
// refused before anything is written, which is what lets the app's sidebar
// going away mean "all dealt with". Folding one in needs an agent to tell, so
// any --fold is refused where the slice has no live session.
//
// The record goes on the page before the message goes to the agent, since the
// record is what stops complete-slice refusing: an agent that hands back the
// moment the message lands finds the way already clear.
func sliceTriage(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-triage", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var queue, fold, drop indexList
	flags.Var(&queue, "queue", "a follow-up to queue as a slice, by index; repeat for more")
	flags.Var(&fold, "fold", "a follow-up for the agent to fold into this slice, by index; repeat for more")
	flags.Var(&drop, "drop", "a follow-up to drop, by index; repeat for more")
	dropAll := flags.Bool("drop-all", false, "drop every pending follow-up")
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-triage: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	ref, err := pageID("slice-triage", rest[0])
	if err != nil {
		return err
	}
	named := len(queue) + len(fold) + len(drop)
	switch {
	case *dropAll && named > 0:
		return usageErrorf("slice-triage: --drop-all decides every follow-up: give it alone")
	case !*dropAll && named == 0:
		return usageErrorf("slice-triage: nothing decided: pass --queue, --fold and --drop by index, or --drop-all")
	}

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}
	shape, err := sliceShape(ctx, st, projectID, project)
	if err != nil {
		return err
	}
	s, _, err := loadSlice(ctx, st, ref)
	if err != nil {
		return err
	}
	if s.Status == domain.SliceTodo {
		return fmt.Errorf("%q is %s: a slice nobody is working has no follow-ups to triage", s.Name, s.StatusName)
	}
	body, err := st.Body(ctx, s.ID)
	if err != nil {
		return fmt.Errorf("read the slice for follow-ups: %w", err)
	}
	pending := store.PendingFollowUps(body)
	if len(pending) == 0 {
		return fmt.Errorf("%q has no follow-ups awaiting a decision", s.Name)
	}
	decisions, err := decide(pending, queue, fold, drop, *dropAll)
	if err != nil {
		return err
	}

	// The live session is read up front: a fold-in with nobody to fold it in
	// is refused before anything is written, and otherwise it is who is told.
	session, live, err := liveSessionFor(env, s.ID, ref)
	if err != nil && len(fold) > 0 {
		return err
	}
	if err != nil {
		logging.Error("could not read live sessions for a triage; the agent will not be told", "slice", s.ID, "err", err)
	}
	if len(fold) > 0 && !live {
		return fmt.Errorf("%q has no live agent to fold anything into: relaunch it first, or queue instead", s.Name)
	}

	var outcome triageJSON
	record := make([]store.Triaged, len(pending))
	for i, f := range pending {
		switch decisions[f.Index] {
		case store.Queued:
			q, err := st.AddSlice(ctx, storeProject(projectID, project), store.NewSlice{
				Title:     f.Title,
				Brief:     f.Brief + "\n\n" + provenance(s),
				Milestone: milestoneOf(s, shape.Milestones),
				DependsOn: []string{s.ID},
			})
			if err != nil {
				return fmt.Errorf("queue %q as a slice (%d queued before it): %w", f.Title, len(outcome.Queued), err)
			}
			outcome.Queued = append(outcome.Queued, queuedJSON{Title: f.Title, ID: q.ID, URL: q.URL})
			record[i] = store.Triaged{Title: f.Title, Decision: store.Queued, Link: linkOf(q)}
		case store.FoldedIn:
			outcome.Folded = append(outcome.Folded, f.Title)
			record[i] = store.Triaged{Title: f.Title, Decision: store.FoldedIn}
		default:
			outcome.Dropped = append(outcome.Dropped, f.Title)
			record[i] = store.Triaged{Title: f.Title, Decision: store.Dropped}
		}
	}
	if err := st.RecordTriage(ctx, s.ID, record); err != nil {
		return fmt.Errorf("record the triage on the slice: %w", err)
	}
	env.nudged()

	if live {
		if err := env.NewTmux().SendPrompt(session, triageMessage(pending, decisions)); err != nil {
			return fmt.Errorf("the triage is recorded, but telling the agent failed: %w", err)
		}
	}

	if *asJSON {
		return writeJSON(env.Out, outcome.filled())
	}
	_, err = io.WriteString(env.Out, triageMarkdown(s, outcome, live))
	return err
}

// indexList is a repeatable flag of follow-up indexes.
type indexList []int

// String implements flag.Value.
func (l *indexList) String() string {
	parts := make([]string, len(*l))
	for i, n := range *l {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}

// Set implements flag.Value.
func (l *indexList) Set(v string) error {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 {
		return fmt.Errorf("%q is not a follow-up's index: give the number slice-show prints", v)
	}
	*l = append(*l, n)
	return nil
}

// decide settles what becomes of each pending follow-up, by index, refusing
// any index pending does not hold, any named twice, and any left out.
func decide(pending []store.FollowUp, queue, fold, drop []int, dropAll bool) (map[int]store.Decision, error) {
	decisions := map[int]store.Decision{}
	if dropAll {
		for _, f := range pending {
			decisions[f.Index] = store.Dropped
		}
		return decisions, nil
	}
	held := map[int]bool{}
	for _, f := range pending {
		held[f.Index] = true
	}
	for _, d := range []struct {
		indexes  []int
		decision store.Decision
	}{{queue, store.Queued}, {fold, store.FoldedIn}, {drop, store.Dropped}} {
		for _, n := range d.indexes {
			if !held[n] {
				return nil, fmt.Errorf("no follow-up %d awaits a decision: the pending ones are %s", n, indexesOf(pending))
			}
			if _, twice := decisions[n]; twice {
				return nil, fmt.Errorf("follow-up %d is decided twice: name each once", n)
			}
			decisions[n] = d.decision
		}
	}
	var missing []string
	for _, f := range pending {
		if _, ok := decisions[f.Index]; !ok {
			missing = append(missing, strconv.Itoa(f.Index))
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("follow-up %s undecided: decide every one, or pass --drop-all", strings.Join(missing, ", "))
	}
	return decisions, nil
}

// indexesOf lists the pending follow-ups' indexes, for a refusal.
func indexesOf(pending []store.FollowUp) string {
	parts := make([]string, len(pending))
	for i, f := range pending {
		parts[i] = strconv.Itoa(f.Index)
	}
	return strings.Join(parts, ", ")
}

// liveSessionFor is the tmux session of the slice's agent, and whether there
// is one. A launch tags the pane with the slice's own ID; the ID as the caller
// typed it is tried as well, the way agent-send takes it.
func liveSessionFor(env Env, ids ...string) (string, bool, error) {
	live, err := env.NewTmux().LiveSlices()
	if err != nil {
		return "", false, fmt.Errorf("could not read live sessions: %w", err)
	}
	for _, id := range ids {
		if session, ok := live[id]; ok {
			return session, true, nil
		}
	}
	return "", false, nil
}

// provenance is the line a queued follow-up's brief ends with, saying where it
// came from.
func provenance(s domain.Slice) string {
	return fmt.Sprintf("Proposed by the agent working %q (%s).", s.Name, linkOf(s))
}

// linkOf is where a slice can be found: its URL, or its ID where the plan gives
// it none.
func linkOf(s domain.Slice) string {
	if s.URL != "" {
		return s.URL
	}
	return s.ID
}

// triageMessage is what the agent waiting on the decision is told: what was
// queued and dropped, then the follow-ups to fold in with their briefs, or that
// there is nothing to fold in — and in either case how to finish.
func triageMessage(pending []store.FollowUp, decisions map[int]store.Decision) string {
	var queued, dropped []string
	var folds []store.FollowUp
	for _, f := range pending {
		switch decisions[f.Index] {
		case store.Queued:
			queued = append(queued, fmt.Sprintf("%d (%s)", f.Index, f.Title))
		case store.FoldedIn:
			folds = append(folds, f)
		default:
			dropped = append(dropped, fmt.Sprintf("%d (%s)", f.Index, f.Title))
		}
	}
	var b strings.Builder
	b.WriteString("Follow-ups decided.")
	if len(queued) > 0 {
		fmt.Fprintf(&b, " Queued as slices: %s.", strings.Join(queued, ", "))
	}
	if len(dropped) > 0 {
		fmt.Fprintf(&b, " Dropped: %s.", strings.Join(dropped, ", "))
	}
	if len(folds) == 0 {
		b.WriteString("\nNothing to fold in — hand back now with nat complete-slice as usual.")
		return b.String()
	}
	b.WriteString("\nFold in before handing back:\n")
	for _, f := range folds {
		marker := strconv.Itoa(f.Index) + ". "
		fmt.Fprintf(&b, "\n%s%s\n%s\n", marker, f.Title, indented(strings.Repeat(" ", len(marker)), f.Brief))
	}
	b.WriteString("\nThen hand back with nat complete-slice as usual.")
	return b.String()
}

// indented prefixes every non-empty line of text with indent.
func indented(indent, text string) string {
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		if ln != "" {
			lines[i] = indent + ln
		}
	}
	return strings.Join(lines, "\n")
}

// triageJSON is what slice-triage --json prints: what became of each
// follow-up.
type triageJSON struct {
	Queued  []queuedJSON `json:"queued"`
	Folded  []string     `json:"folded"`
	Dropped []string     `json:"dropped"`
}

// queuedJSON is a follow-up queued as a slice, and the slice it became.
type queuedJSON struct {
	Title string `json:"title"`
	ID    string `json:"id"`
	URL   string `json:"url"`
}

// filled is the outcome with every list present, empty rather than null, so a
// reader can decode each as a list unconditionally.
func (t triageJSON) filled() triageJSON {
	if t.Queued == nil {
		t.Queued = []queuedJSON{}
	}
	if t.Folded == nil {
		t.Folded = []string{}
	}
	if t.Dropped == nil {
		t.Dropped = []string{}
	}
	return t
}

// triageMarkdown reports the triage in a line or two.
func triageMarkdown(s domain.Slice, t triageJSON, told bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.Name)
	fmt.Fprintf(&b, "Queued %s · folding in %d · dropped %d.\n",
		counted(len(t.Queued), "slice"), len(t.Folded), len(t.Dropped))
	for _, q := range t.Queued {
		fmt.Fprintf(&b, "- %s: %s\n", q.Title, linkOf(domain.Slice{ID: q.ID, URL: q.URL}))
	}
	if !told {
		b.WriteString("\nNo live agent was told; the decision is recorded on the slice page.\n")
	}
	return b.String()
}
