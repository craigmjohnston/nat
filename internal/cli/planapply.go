package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// planApply files a whole plan at once: the milestones and slices a planning
// session drafted, written in one run instead of a page at a time. It is what
// /queue-work reaches for once the user has approved a proposal.
//
// The document is validated entirely before anything is written. A plan naming
// a milestone that does not exist is a mistake in the drafting, and half of it
// landing in Notion leaves someone to work out which half — so the whole
// document has to make sense before any of it is applied.
//
// It only ever creates. Nothing in a plan says which existing page to change,
// and a command that adds work has no business editing work already filed.
//
// Where the plan lands is the command line's rather than the document's:
// --project names a project of the config file, and a run that names none
// applies nothing. A document says what work there is and not whose it is,
// which is what lets one drafted plan be applied to whichever project the
// session is planning for.
func planApply(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("plan-apply", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) > 1 {
		return usageErrorf("plan-apply: want at most one plan file, given %d", len(rest))
	}
	// The plan is read and parsed before Notion is touched at all, so a missing
	// file or unreadable stdin fails having written nothing.
	source := stdinRef
	if len(rest) == 1 {
		source = rest[0]
	}
	p, err := readPlan(source, env.In)
	if err != nil {
		return err
	}

	// Which project the plan lands in is the one thing --project decides, and
	// everything past this line is the same whichever project it named.
	_, _, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	client := env.NewClient(env.Tokens.Token)

	ds, err := slicesDataSource(ctx, client, project)
	if err != nil {
		return err
	}
	existing := milestonesOf(notion.ShapeOf(ds))
	// The project's own slices are only read when the plan names one: they are
	// what a depends_on title may be resolved against, and a plan that declares
	// no dependency has nothing to resolve.
	var filed []domain.Slice
	if p.dependsOnAnything() {
		pages, err := client.QueryDataSource(ctx, project.SlicesDSID, nil,
			[]notion.Sort{{Timestamp: notion.TimestampCreated, Direction: notion.SortAscending}})
		if err != nil {
			return fmt.Errorf("load slices: %w", err)
		}
		filed = domain.SlicesFromPages(pages)
	}
	targets, err := validatePlan(p, existing, filed)
	if err != nil {
		return err
	}

	applied, err := applyPlan(ctx, client, project, ds, p, targets, existing)
	// A run that failed partway has still written what it wrote — the error
	// itself says so — and the board deserves to hear about that half as much
	// as about a whole plan.
	if len(applied.Milestones) > 0 || len(applied.Slices) > 0 || len(applied.Dependencies) > 0 {
		env.nudged()
	}
	if err != nil {
		return err
	}

	logging.Action("plan applied", "milestones", len(applied.Milestones), "slices", len(applied.Slices),
		"dependencies", len(applied.Dependencies))
	if *asJSON {
		return writeJSON(env.Out, applied.jsonDoc(project))
	}
	_, err = io.WriteString(env.Out, applied.markdown(project))
	return err
}

// plan is the document plan-apply reads: the milestones to create and the
// slices to file, each slice naming the milestone it belongs under. Statuses,
// orders and assignees are absent by design — they are not the plan's to choose,
// and a document that could set them would be a way to smuggle a claim past the
// workflow.
type plan struct {
	Milestones   []planMilestone  `json:"milestones"`
	Slices       []planSlice      `json:"slices"`
	Dependencies []planDependency `json:"dependencies"`
}

// planMilestone is a milestone to create. It is an object rather than a bare
// name so that a later field — an order override, a body — can be added without
// breaking every plan already written.
type planMilestone struct {
	Name string `json:"name"`
}

// planSlice is a slice to file. Milestone names one of the plan's own new
// milestones or an existing one of the project, by name — the same way
// slice-add's --milestone does, and for the same reason: a milestone is an
// option of the slices' Milestone column and so is nothing but its name.
//
// DependsOn names the slices this one waits on, by title. A title may be one
// the same document creates — including one written further down it, since
// nothing is filed until the whole plan has been read — or one the project
// already has, which is what lets a plan hang new work off work already
// queued.
type planSlice struct {
	Title       string   `json:"title"`
	Milestone   string   `json:"milestone"`
	Description string   `json:"description"`
	Repo        string   `json:"repo"`
	DependsOn   []string `json:"depends_on"`
}

// planDependency hangs dependencies off a slice by title, which is how a plan
// reaches a slice it does not create: `depends_on` can only say what a new
// slice waits on, and work already on the board waits on new work often enough
// that a plan needs a way to say so.
//
// It is additive, exactly as `nat slice-depends --on` is: what On names is
// added to whatever the slice already waits on. A document has no business
// dropping a dependency somebody recorded by hand, and nothing here says to.
type planDependency struct {
	Slice string   `json:"slice"`
	On    []string `json:"on"`
}

// dependsOnAnything reports whether the plan names a dependency at all, which
// is the whole reason to read the project's existing slices: a plan that
// declares none needs no more of Notion than it ever did.
func (p plan) dependsOnAnything() bool {
	if len(p.Dependencies) > 0 {
		return true
	}
	for _, s := range p.Slices {
		if len(s.DependsOn) > 0 {
			return true
		}
	}
	return false
}

// readPlan reads the document from a file, or from the reader when the source
// is `-` or absent — a plan drafted by an agent is piped in far more often than
// it is saved somewhere first.
func readPlan(source string, in io.Reader) (plan, error) {
	var raw []byte
	var err error
	if source == stdinRef {
		if in == nil {
			return plan{}, usageErrorf("plan-apply: no plan file given and there is nothing to read")
		}
		raw, err = io.ReadAll(in)
	} else {
		raw, err = os.ReadFile(source)
	}
	if err != nil {
		return plan{}, fmt.Errorf("read the plan: %w", err)
	}

	var p plan
	dec := json.NewDecoder(bytes.NewReader(raw))
	// A key the plan format does not have is a mistake worth failing on: a
	// misspelled "description" would otherwise file a slice with no brief and
	// report success.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return plan{}, fmt.Errorf("the plan is not valid JSON: %w", err)
	}
	if err := dec.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return plan{}, fmt.Errorf("the plan is not valid JSON: it holds more than one document")
	}
	return p, nil
}

// sliceTarget is the milestone one planned slice lands under: an index into the
// plan's own new milestones, or an existing milestone of the project. It is
// worked out during validation so that applying the plan has nothing left to
// resolve, and therefore nothing left to fail on halfway through. dependsOn is
// resolved the same way and for the same reason.
type sliceTarget struct {
	// newIndex indexes plan.Milestones, or is -1 when Existing holds the answer.
	newIndex int
	existing domain.Milestone
	// dependsOn is the slices this one waits on, in the order the plan names
	// them.
	dependsOn []planDep
}

// planDep is one resolved dependency: a slice the same plan creates, by its
// index into plan.Slices, or one the project already has, by page ID. A slice
// the plan is about to create has no ID to record until it exists, which is why
// the two are told apart rather than both being an ID.
type planDep struct {
	// newIndex indexes plan.Slices, or is -1 when id holds the answer.
	newIndex int
	id       string
}

// same reports whether two resolved dependencies are the same slice, which is
// how a slice made to wait on itself — a slice nothing could ever unblock — is
// spotted whichever side of the document named it.
func (d planDep) same(o planDep) bool {
	if d.newIndex >= 0 || o.newIndex >= 0 {
		return d.newIndex == o.newIndex
	}
	return domain.NormaliseID(d.id) == domain.NormaliseID(o.id)
}

// filedDeps is one additive write onto a slice the project already has: the
// slice as it was read — its own dependencies included, since they are kept —
// and what the plan adds to them.
type filedDeps struct {
	slice domain.Slice
	add   []planDep
}

// planTargets is everything validation resolved: one target per slice the plan
// creates, and the writes onto slices already on the board. Both are worked
// out before anything is written, so applying has nothing left to fail on for
// a reason the document could have been refused for.
type planTargets struct {
	slices []sliceTarget
	filed  []filedDeps
}

// validatePlan checks the whole document and resolves every milestone
// reference and every dependency, returning one target per slice in the plan's
// order alongside the additive writes its dependencies list asks for.
// existingSlices are the project's own slices, which a dependency may name; it
// is empty when the plan declares no dependencies, because then there is
// nothing to resolve against.
func validatePlan(p plan, existing []domain.Milestone, existingSlices []domain.Slice) (planTargets, error) {
	if len(p.Milestones) == 0 && len(p.Slices) == 0 && len(p.Dependencies) == 0 {
		return planTargets{}, fmt.Errorf("the plan creates nothing and records nothing: " +
			"it has no milestones, no slices and no dependencies")
	}

	seen := map[string]int{}
	for i, m := range p.Milestones {
		name := strings.TrimSpace(m.Name)
		if name == "" {
			return planTargets{}, fmt.Errorf("milestone %d has no name", i+1)
		}
		key := strings.ToLower(name)
		if first, dup := seen[key]; dup {
			return planTargets{}, fmt.Errorf("milestones %d and %d are both named %q", first+1, i+1, name)
		}
		// A new milestone sharing a name with one already in the plan would make
		// every slice naming it ambiguous, so it is refused rather than guessed at.
		for _, e := range existing {
			if strings.EqualFold(strings.TrimSpace(e.Name), name) {
				return planTargets{}, fmt.Errorf("milestone %d is named %q, which the project already has: "+
					"drop it from the plan to file slices under the existing one", i+1, name)
			}
		}
		seen[key] = i
	}

	targets := make([]sliceTarget, len(p.Slices))
	for i, s := range p.Slices {
		if strings.TrimSpace(s.Title) == "" {
			return planTargets{}, fmt.Errorf("slice %d has no title", i+1)
		}
		ref := strings.TrimSpace(s.Milestone)
		if ref == "" {
			return planTargets{}, fmt.Errorf("slice %d (%q) names no milestone", i+1, strings.TrimSpace(s.Title))
		}
		if idx, ok := seen[strings.ToLower(ref)]; ok {
			targets[i] = sliceTarget{newIndex: idx}
			continue
		}
		m, err := resolveMilestone(ref, existing)
		if err != nil {
			return planTargets{}, fmt.Errorf("slice %d (%q): %w", i+1, strings.TrimSpace(s.Title), err)
		}
		targets[i] = sliceTarget{newIndex: -1, existing: m}
	}
	filed, err := resolveDependencies(p, existingSlices, targets)
	if err != nil {
		return planTargets{}, err
	}
	if err := checkPlanCycles(p, existingSlices, targets, filed); err != nil {
		return planTargets{}, err
	}
	return planTargets{slices: targets, filed: filed}, nil
}

// resolveDependencies turns every depends_on title into the slice it names,
// filling in the targets. A title is looked for among the plan's own slices
// first and the project's afterwards: a plan that creates a slice and then
// depends on it means the one it just wrote, whatever else happens to share the
// name.
// It resolves both halves of the document that name a slice: each new slice's
// own depends_on, which fills in the targets, and the top-level dependencies
// list, which is returned as the additive writes onto slices the project
// already has. An entry there naming a slice the plan itself creates is folded
// into that slice's target instead — the page does not exist yet, so there is
// nothing to add to.
func resolveDependencies(p plan, existingSlices []domain.Slice, targets []sliceTarget) ([]filedDeps, error) {
	planned := map[string][]int{}
	for i, s := range p.Slices {
		key := strings.ToLower(strings.TrimSpace(s.Title))
		planned[key] = append(planned[key], i)
	}
	filed := map[string][]domain.Slice{}
	for _, s := range existingSlices {
		key := strings.ToLower(strings.TrimSpace(s.Name))
		filed[key] = append(filed[key], s)
	}

	for i, s := range p.Slices {
		title := strings.TrimSpace(s.Title)
		for _, ref := range s.DependsOn {
			dep, err := resolveDependency(strings.TrimSpace(ref), planDep{newIndex: i}, planned, filed, "depends on")
			if err != nil {
				return nil, fmt.Errorf("slice %d (%q): %w", i+1, title, err)
			}
			targets[i].dependsOn = appendDep(targets[i].dependsOn, dep)
		}
	}
	return resolveAdditions(p, planned, filed, targets)
}

// resolveAdditions works out the top-level dependencies list: which slice each
// entry is about, and what it is being made to wait on. Two entries naming one
// slice are merged rather than refused — each is a list of what to add, and
// adding twice is adding once.
func resolveAdditions(p plan, planned map[string][]int, filed map[string][]domain.Slice, targets []sliceTarget) ([]filedDeps, error) {
	var out []filedDeps
	at := map[string]int{}
	for i, d := range p.Dependencies {
		title := strings.TrimSpace(d.Slice)
		if title == "" {
			return nil, fmt.Errorf("dependencies %d names no slice", i+1)
		}
		if len(d.On) == 0 {
			return nil, fmt.Errorf("dependencies %d (%q) names nothing for it to wait on", i+1, title)
		}
		target, err := resolveDependency(title, planDep{newIndex: -1}, planned, filed, "names")
		if err != nil {
			return nil, fmt.Errorf("dependencies %d: %w", i+1, err)
		}
		// A slice the plan creates is not on the board yet: what it waits on goes
		// on with the rest of the new relations, from its own target.
		slot := -1
		if target.newIndex < 0 {
			key := domain.NormaliseID(target.id)
			idx, ok := at[key]
			if !ok {
				idx = len(out)
				at[key] = idx
				out = append(out, filedDeps{slice: filed[strings.ToLower(title)][0]})
			}
			slot = idx
		}
		for _, ref := range d.On {
			dep, err := resolveDependency(strings.TrimSpace(ref), target, planned, filed, "depends on")
			if err != nil {
				return nil, fmt.Errorf("dependencies %d (%q): %w", i+1, title, err)
			}
			if slot < 0 {
				targets[target.newIndex].dependsOn = appendDep(targets[target.newIndex].dependsOn, dep)
				continue
			}
			out[slot].add = appendDep(out[slot].add, dep)
		}
	}
	return out, nil
}

// appendDep adds one resolved dependency to a list, dropping a slice already
// named: a plan may reach the same slice twice — its own depends_on and an
// entry in the dependencies list — and Notion would take the duplicate happily.
func appendDep(list []planDep, dep planDep) []planDep {
	for _, d := range list {
		if d.same(dep) {
			return list
		}
	}
	return append(list, dep)
}

// resolveDependency finds the one slice a title names, refusing anything that
// could mean more than one thing — and the slice it is being resolved for,
// which would be a slice nothing could ever unblock. The plan's own slices are
// looked at first and the project's afterwards: a plan that creates a slice and
// then depends on it means the one it just wrote, whatever else happens to
// share the name.
//
// what is how the document put it — a slice "depends on" a title, an entry of
// the dependencies list "names" one — so a refusal reads as the thing refused.
func resolveDependency(ref string, self planDep, planned map[string][]int, filed map[string][]domain.Slice, what string) (planDep, error) {
	if ref == "" {
		return planDep{}, fmt.Errorf("names an empty dependency")
	}
	key := strings.ToLower(ref)
	switch matches := planned[key]; len(matches) {
	case 1:
		return checkSelf(planDep{newIndex: matches[0]}, self)
	case 0:
	default:
		return planDep{}, fmt.Errorf("%s %q, which the plan creates %d times", what, ref, len(matches))
	}
	switch matches := filed[key]; len(matches) {
	case 1:
		return checkSelf(planDep{newIndex: -1, id: matches[0].ID}, self)
	case 0:
		return planDep{}, fmt.Errorf("%s %q, which is neither in the plan nor in the project", what, ref)
	default:
		return planDep{}, fmt.Errorf("%s %q, which the project already has %d slices named: "+
			"rename one in Notion", what, ref, len(matches))
	}
}

// checkSelf refuses the one resolution that is never meant: a slice waiting on
// itself.
func checkSelf(dep, self planDep) (planDep, error) {
	if dep.same(self) {
		return planDep{}, fmt.Errorf("depends on itself")
	}
	return dep, nil
}

// appliedPlan is what the run did: the new milestones, the slices each paired
// with whichever milestone — new or existing — it was filed under, and the
// slices already on the board it added dependencies to, which is the one thing
// a plan changes rather than creates.
type appliedPlan struct {
	Milestones   []domain.Milestone
	Slices       []appliedSlice
	Dependencies []appliedDependency
}

type appliedSlice struct {
	Slice     domain.Slice
	Milestone domain.Milestone
}

// appliedDependency is one slice of the project made to wait on more than it
// did: the slice as it was read, and the page IDs the run added — what it
// already waited on is kept and is not the run's doing.
type appliedDependency struct {
	Slice domain.Slice
	Added []string
}

// orderNote says how the run put the slices in the order the document wrote
// them, since it is not the way anyone would guess and the plan's own order is
// what the board and next-slice hand work out in.
//
// The board reads that order from the Slices data source's first view
// (notion.PlanOrder), and a row the view's manual order does not name — which
// is every row the API creates, since nothing in the API adds one to that
// order — falls back to newest created first. There is no writing that order:
// PATCH /views takes a name, a filter, sorts, quick filters and a
// configuration and nothing else, and a row order sent at the top level,
// inside the configuration, or beside a sorts write is answered with a 200
// whose echo does not carry it and a view whose rows have not moved;
// /views/{id}/rows, /row_order and /order are not endpoints at all.
//
// So the plan is written back to front, and newest-first reads back as the
// order the document wrote. That is the whole of what a run can do about
// position: a row created now sorts above every row already on the board, and
// it is the view's own sorts — a project's plan is sorted by Milestone — that
// put new work under the milestone it belongs to.
const orderNote = "Notion's API cannot write a view's row order, so the slices were created " +
	"back to front: an unordered row reads back newest first, which is what puts them " +
	"on the board in the order the plan wrote them."

// orderingWord is orderNote for something parsing the output rather than
// reading it: one stable word for how the run arrived at the plan's order.
const orderingWord = "reverse-creation"

// applyPlan writes the plan: milestones first, because the slices are filed
// under them, then the slices — last one first, for the reason orderNote
// gives — and last the dependencies between them, which have to come last since
// a slice may wait on one the plan creates further down and there is no page to
// point at until every slice exists.
//
// A write that fails stops the run, and whatever was created stays created —
// there is no transaction to roll back, and deleting pages to tidy up would be
// a worse thing to get wrong. The error says how far it got, so the plan can be
// trimmed and run again. Because the slices are written backwards, what such a
// run got as far as is the tail of the document rather than its head, and that
// is what it reports: the slices that exist, in the order the document put
// them.
func applyPlan(ctx context.Context, client API, project config.ProjectConfig, ds *notion.DataSource, p plan, resolved planTargets, existing []domain.Milestone) (appliedPlan, error) {
	targets := resolved.slices
	var applied appliedPlan
	names := make([]string, len(p.Milestones))
	for i, pm := range p.Milestones {
		names[i] = strings.TrimSpace(pm.Name)
	}
	added, err := addMilestones(ctx, client, project.SlicesDSID, ds, existing, names)
	applied.Milestones = added
	if err != nil {
		return applied, appliedErr(applied, err)
	}
	made := make([]appliedSlice, len(p.Slices))
	for i := len(p.Slices) - 1; i >= 0; i-- {
		ps := p.Slices[i]
		m := targets[i].existing
		if targets[i].newIndex >= 0 {
			m = applied.Milestones[targets[i].newIndex]
		}
		s, err := createSlice(ctx, client, project.SlicesDSID, m,
			strings.TrimSpace(ps.Title), strings.TrimSpace(ps.Description), strings.TrimSpace(ps.Repo), nil)
		if err != nil {
			// Everything below this line of the document is written; nothing
			// above it is. The tail is what exists, and it is already in the
			// order the document put it.
			applied.Slices = made[i+1:]
			return applied, appliedErr(applied, err)
		}
		made[i] = appliedSlice{Slice: s, Milestone: m}
	}
	applied.Slices = made
	if err := applyDependencies(ctx, client, targets, applied.Slices); err != nil {
		return applied, appliedErr(applied, err)
	}
	deps, err := applyAdditions(ctx, client, resolved.filed, applied.Slices)
	applied.Dependencies = deps
	if err != nil {
		return applied, appliedErr(applied, err)
	}
	return applied, nil
}

// applyDependencies writes the relations between slices, once every page in the
// plan exists to be pointed at. A slice naming none is not written to at all:
// there is nothing to say, and a project whose table has no dependency column
// applies such a plan exactly as it always did.
func applyDependencies(ctx context.Context, client API, targets []sliceTarget, created []appliedSlice) error {
	for i, t := range targets {
		if len(t.dependsOn) == 0 {
			continue
		}
		ids := make([]string, len(t.dependsOn))
		for j, d := range t.dependsOn {
			ids[j] = d.id
			if d.newIndex >= 0 {
				ids[j] = created[d.newIndex].Slice.ID
			}
		}
		if _, err := client.UpdatePageProperties(ctx, created[i].Slice.ID,
			map[string]notion.PropertyValue{notion.PropDependsOn: notion.NewRelation(ids...)}); err != nil {
			return fmt.Errorf("record what %q waits on: %w", created[i].Slice.Name, err)
		}
		logging.Action("plan dependencies recorded", "slice", created[i].Slice.ID, "depends_on", len(ids))
	}
	return nil
}

// applyAdditions is the last phase's other half: the dependencies the plan adds
// to slices the project already has, one write each and additive, since what
// such a slice already waits on is nobody's to drop. A write is skipped where
// it would change nothing — a plan re-run over dependencies already recorded
// touches no page at all.
func applyAdditions(ctx context.Context, client API, filed []filedDeps, created []appliedSlice) ([]appliedDependency, error) {
	var out []appliedDependency
	for _, f := range filed {
		seen := map[string]bool{}
		ids := make([]string, 0, len(f.slice.DependsOn)+len(f.add))
		for _, id := range f.slice.DependsOn {
			if key := domain.NormaliseID(id); !seen[key] {
				seen[key] = true
				ids = append(ids, id)
			}
		}
		var added []string
		for _, d := range f.add {
			id := d.id
			if d.newIndex >= 0 {
				id = created[d.newIndex].Slice.ID
			}
			if key := domain.NormaliseID(id); !seen[key] {
				seen[key] = true
				ids = append(ids, id)
				added = append(added, id)
			}
		}
		if len(added) == 0 {
			continue
		}
		if _, err := client.UpdatePageProperties(ctx, f.slice.ID,
			map[string]notion.PropertyValue{notion.PropDependsOn: notion.NewRelation(ids...)}); err != nil {
			return out, fmt.Errorf("record what %q waits on: %w", f.slice.Name, err)
		}
		logging.Action("plan dependencies added", "slice", f.slice.ID, "depends_on", len(ids), "added", len(added))
		out = append(out, appliedDependency{Slice: f.slice, Added: added})
	}
	return out, nil
}

// appliedErr says what a failed run had already written, so nobody re-runs a
// plan whose first half is already in Notion.
func appliedErr(applied appliedPlan, err error) error {
	made := counts(len(applied.Milestones), len(applied.Slices))
	if n := len(applied.Dependencies); n > 0 {
		return fmt.Errorf("%w — %s were created, and %d %s already on the board were made to wait on more, "+
			"before this failed; all of it is still in Notion", err, made, n, plural("slice", n))
	}
	if len(applied.Milestones) == 0 && len(applied.Slices) == 0 {
		return err
	}
	return fmt.Errorf("%w — %s were created before this failed and are still in Notion", err, made)
}

// counts phrases how much a run created, pluralised, for the one sentence that
// says what happened.
func counts(milestones, slices int) string {
	return fmt.Sprintf("%d %s and %d %s",
		milestones, plural("milestone", milestones), slices, plural("slice", slices))
}

// planAppliedJSON is the structured form of what the run created: the new
// milestones, and every slice with the milestone it was filed under. Existing
// milestones are not listed on their own — the plan did not create them, and
// the slices that name them carry their ID.
type planAppliedJSON struct {
	Milestones   []milestoneJSON       `json:"milestones"`
	Slices       []addedSliceJSON      `json:"slices"`
	Dependencies []addedDependencyJSON `json:"dependencies"`
	// Ordering is how the run got the slices into the plan's order — see
	// orderNote, which is the same thing said out loud.
	Ordering string `json:"ordering"`
}

// addedDependencyJSON is one slice already on the board the run made to wait on
// more: the slice, and the page IDs added to what it already waited on.
type addedDependencyJSON struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	URL   string   `json:"url"`
	Added []string `json:"added"`
}

// jsonDoc renders the run as JSON, with empty lists rather than nulls so a
// consumer can iterate them all without checking.
func (a appliedPlan) jsonDoc(project config.ProjectConfig) planAppliedJSON {
	doc := planAppliedJSON{
		Milestones:   make([]milestoneJSON, 0, len(a.Milestones)),
		Slices:       make([]addedSliceJSON, 0, len(a.Slices)),
		Dependencies: make([]addedDependencyJSON, 0, len(a.Dependencies)),
		Ordering:     orderingWord,
	}
	for _, d := range a.Dependencies {
		doc.Dependencies = append(doc.Dependencies, addedDependencyJSON{
			ID: d.Slice.ID, Name: d.Slice.Name, URL: d.Slice.URL, Added: d.Added,
		})
	}
	for _, m := range a.Milestones {
		doc.Milestones = append(doc.Milestones, milestoneJSON{
			ID: m.ID, Name: m.Name, Order: m.Order, Status: string(m.Status),
		})
	}
	for _, s := range a.Slices {
		doc.Slices = append(doc.Slices, addedSliceJSON{
			ID:            s.Slice.ID,
			Name:          s.Slice.Name,
			Status:        string(s.Slice.Status),
			MilestoneID:   s.Milestone.ID,
			MilestoneName: s.Milestone.Name,
			Repo:          resolvedRepo(s.Slice, project),
			URL:           s.Slice.URL,
		})
	}
	return doc
}

// markdown reports the run grouped by milestone, in the order the plan wrote
// them: the new milestones first, then any existing ones slices were added to.
// Grouping is how the plan was read and how it is checked over afterwards.
func (a appliedPlan) markdown(project config.ProjectConfig) string {
	var b strings.Builder
	b.WriteString("# Plan applied\n\n")
	fmt.Fprintf(&b, "Added %s to %s.\n", counts(len(a.Milestones), len(a.Slices)), project.Name)
	fmt.Fprintf(&b, "\n%s\n", orderNote)

	for _, m := range a.Milestones {
		fmt.Fprintf(&b, "\n## %s\n\n", m.Name)
		fmt.Fprintf(&b, "New milestone %s, %s — %s\n\n", planPosition(m), blank(string(m.Status)), optionNote)
		b.WriteString(sliceList(a.slicesUnder(m.ID)))
	}
	for _, m := range a.existingMilestones() {
		fmt.Fprintf(&b, "\n## %s\n\n", m.Name)
		b.WriteString(sliceList(a.slicesUnder(m.ID)))
	}
	// The dependencies added to slices already on the board belong under no
	// milestone of the run: nothing was filed anywhere, and what changed is what
	// those slices now wait on.
	if len(a.Dependencies) > 0 {
		b.WriteString("\n## Dependencies added\n\n")
		for _, d := range a.Dependencies {
			fmt.Fprintf(&b, "- %s — now waits on %d more %s\n",
				d.Slice.Name, len(d.Added), plural("slice", len(d.Added)))
		}
	}
	return b.String()
}

// slicesUnder is the slices of the run filed under one milestone, in plan order.
func (a appliedPlan) slicesUnder(milestoneID string) []appliedSlice {
	var out []appliedSlice
	for _, s := range a.Slices {
		if s.Milestone.ID == milestoneID {
			out = append(out, s)
		}
	}
	return out
}

// existingMilestones is the project's own milestones the run added slices to,
// first-mentioned first and each named once.
func (a appliedPlan) existingMilestones() []domain.Milestone {
	created := map[string]bool{}
	for _, m := range a.Milestones {
		created[m.ID] = true
	}
	seen := map[string]bool{}
	var out []domain.Milestone
	for _, s := range a.Slices {
		if created[s.Milestone.ID] || seen[s.Milestone.ID] {
			continue
		}
		seen[s.Milestone.ID] = true
		out = append(out, s.Milestone)
	}
	return out
}

// sliceList prints the slices of one milestone, or says there are none — a
// milestone created with nothing under it is worth seeing rather than reading
// as output that got cut off.
func sliceList(slices []appliedSlice) string {
	if len(slices) == 0 {
		return "_no slices_\n"
	}
	var b strings.Builder
	for _, s := range slices {
		fmt.Fprintf(&b, "- %s — %s\n", s.Slice.Name, pageRef(s.Slice.ID, s.Slice.URL))
	}
	return b.String()
}

// pageRef names a created page: its URL, which is what someone checking the
// result clicks, falling back to the ID when Notion returned no URL.
func pageRef(id, url string) string {
	if url != "" {
		return url
	}
	return id
}

// planNodeKey names one slice of the graph a plan is checked over. A slice the
// plan has yet to create has no page ID to key it by, so it is keyed by its
// place in the document; a normalised ID is hex, so the two can never collide.
func planNodeKey(d planDep) string {
	if d.newIndex >= 0 {
		return fmt.Sprintf("plan:%d", d.newIndex)
	}
	return domain.NormaliseID(d.id)
}

// checkPlanCycles refuses a document that would leave a slice waiting on
// itself. The graph is the whole of what the plan would leave behind — the
// slices it creates and what each waits on, every slice the project already
// has and what it already waits on, and the dependencies the plan adds to
// those — because a plan closes a cycle just as easily through work already on
// the board as through its own.
//
// It runs as part of validation, so a cyclic document is refused before the
// first page is written: half a plan is bad enough without half of it being
// unworkable.
func checkPlanCycles(p plan, existingSlices []domain.Slice, targets []sliceTarget, filed []filedDeps) error {
	edges := map[string][]string{}
	names := map[string]string{}
	for i, s := range p.Slices {
		key := planNodeKey(planDep{newIndex: i})
		edges[key] = nil
		names[key] = strings.TrimSpace(s.Title)
	}
	for _, s := range existingSlices {
		key := domain.NormaliseID(s.ID)
		edges[key] = nil
		names[key] = s.Name
	}
	// A dependency naming a page the project cannot see leads nowhere: it has no
	// dependencies here, so it can be in no cycle, and Blockers passes over it
	// for the same reason.
	edge := func(from, to string) {
		if _, ok := edges[to]; ok {
			edges[from] = append(edges[from], to)
		}
	}
	for i, t := range targets {
		from := planNodeKey(planDep{newIndex: i})
		for _, d := range t.dependsOn {
			edge(from, planNodeKey(d))
		}
	}
	for _, s := range existingSlices {
		for _, id := range s.DependsOn {
			edge(domain.NormaliseID(s.ID), domain.NormaliseID(id))
		}
	}
	for _, f := range filed {
		from := domain.NormaliseID(f.slice.ID)
		for _, d := range f.add {
			edge(from, planNodeKey(d))
		}
	}

	cycles := domain.GraphCycles(edges)
	if len(cycles) == 0 {
		return nil
	}
	read := make([]string, len(cycles))
	for i, cycle := range cycles {
		read[i] = domain.CyclePath(quoteAll(cycleNames(newestFirst(cycle), names)))
	}
	return fmt.Errorf("the plan would leave %d %s of dependencies, and no slice in a cycle can ever be unblocked: %s",
		len(cycles), plural("cycle", len(cycles)), strings.Join(read, "; "))
}

// newestFirst reads a cycle out from a slice the plan itself creates where one
// is in it: what the document asked for is the first step round, so the
// refusal starts where the mistake was made rather than wherever the walk
// happened to enter.
func newestFirst(cycle []string) []string {
	for _, key := range cycle {
		if strings.HasPrefix(key, "plan:") {
			return domain.RotateCycle(cycle, key)
		}
	}
	return cycle
}

// cycleNames turns the graph's keys back into the titles they stand for.
func cycleNames(cycle []string, names map[string]string) []string {
	out := make([]string, len(cycle))
	for i, key := range cycle {
		out[i] = names[key]
	}
	return out
}

// quoteAll quotes each name, since a cycle read out as a path of bare titles is
// unreadable the moment a title holds a space.
func quoteAll(names []string) []string {
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = fmt.Sprintf("%q", name)
	}
	return out
}
