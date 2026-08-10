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
func planApply(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("plan-apply", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
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

	_, project, err := env.activeProject()
	if err != nil {
		return err
	}
	client := env.NewClient(env.Tokens.Token)

	existing, err := client.QueryDataSource(ctx, project.MilestonesDSID, nil,
		[]notion.Sort{{Property: notion.PropOrder, Direction: notion.SortAscending}})
	if err != nil {
		return fmt.Errorf("load milestones: %w", err)
	}
	targets, err := validatePlan(p, domain.MilestonesFromPages(existing))
	if err != nil {
		return err
	}

	applied, err := applyPlan(ctx, client, project, p, targets, nextOrder(existing))
	if err != nil {
		return err
	}

	logging.Action("plan applied", "milestones", len(applied.Milestones), "slices", len(applied.Slices))
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
	Milestones []planMilestone `json:"milestones"`
	Slices     []planSlice     `json:"slices"`
}

// planMilestone is a milestone to create. It is an object rather than a bare
// name so that a later field — an order override, a body — can be added without
// breaking every plan already written.
type planMilestone struct {
	Name string `json:"name"`
}

// planSlice is a slice to file. Milestone names one of the plan's own new
// milestones or an existing one of the project, by name, URL or ID, the same
// way slice-add's --milestone does.
type planSlice struct {
	Title       string `json:"title"`
	Milestone   string `json:"milestone"`
	Description string `json:"description"`
	Repo        string `json:"repo"`
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
// resolve, and therefore nothing left to fail on halfway through.
type sliceTarget struct {
	// newIndex indexes plan.Milestones, or is -1 when Existing holds the answer.
	newIndex int
	existing domain.Milestone
}

// validatePlan checks the whole document and resolves every milestone
// reference, returning one target per slice in the plan's order.
func validatePlan(p plan, existing []domain.Milestone) ([]sliceTarget, error) {
	if len(p.Milestones) == 0 && len(p.Slices) == 0 {
		return nil, fmt.Errorf("the plan creates nothing: it has no milestones and no slices")
	}

	seen := map[string]int{}
	for i, m := range p.Milestones {
		name := strings.TrimSpace(m.Name)
		if name == "" {
			return nil, fmt.Errorf("milestone %d has no name", i+1)
		}
		key := strings.ToLower(name)
		if first, dup := seen[key]; dup {
			return nil, fmt.Errorf("milestones %d and %d are both named %q", first+1, i+1, name)
		}
		// A new milestone sharing a name with one already in the plan would make
		// every slice naming it ambiguous, so it is refused rather than guessed at.
		for _, e := range existing {
			if strings.EqualFold(strings.TrimSpace(e.Name), name) {
				return nil, fmt.Errorf("milestone %d is named %q, which the project already has: "+
					"drop it from the plan to file slices under the existing one", i+1, name)
			}
		}
		seen[key] = i
	}

	targets := make([]sliceTarget, len(p.Slices))
	for i, s := range p.Slices {
		if strings.TrimSpace(s.Title) == "" {
			return nil, fmt.Errorf("slice %d has no title", i+1)
		}
		ref := strings.TrimSpace(s.Milestone)
		if ref == "" {
			return nil, fmt.Errorf("slice %d (%q) names no milestone", i+1, strings.TrimSpace(s.Title))
		}
		if idx, ok := seen[strings.ToLower(ref)]; ok {
			targets[i] = sliceTarget{newIndex: idx}
			continue
		}
		m, err := resolveMilestone(ref, existing)
		if err != nil {
			return nil, fmt.Errorf("slice %d (%q): %w", i+1, strings.TrimSpace(s.Title), err)
		}
		targets[i] = sliceTarget{newIndex: -1, existing: m}
	}
	return targets, nil
}

// appliedPlan is what the run created: the new milestones, and the slices each
// paired with whichever milestone — new or existing — it was filed under.
type appliedPlan struct {
	Milestones []domain.Milestone
	Slices     []appliedSlice
}

type appliedSlice struct {
	Slice     domain.Slice
	Milestone domain.Milestone
}

// applyPlan writes the plan: milestones first, because the slices relate to
// them, then the slices in the order they were written.
//
// A write that fails stops the run, and whatever was created stays created —
// there is no transaction to roll back, and deleting pages to tidy up would be
// a worse thing to get wrong. The error says how far it got, so the plan can be
// trimmed and run again.
func applyPlan(ctx context.Context, client API, project config.ProjectConfig, p plan, targets []sliceTarget, order float64) (appliedPlan, error) {
	var applied appliedPlan
	for _, pm := range p.Milestones {
		m, err := createMilestone(ctx, client, project.MilestonesDSID, strings.TrimSpace(pm.Name), order)
		if err != nil {
			return applied, appliedErr(applied, err)
		}
		applied.Milestones = append(applied.Milestones, m)
		order++
	}
	for i, ps := range p.Slices {
		m := targets[i].existing
		if targets[i].newIndex >= 0 {
			m = applied.Milestones[targets[i].newIndex]
		}
		s, err := createSlice(ctx, client, project.SlicesDSID, m.ID,
			strings.TrimSpace(ps.Title), strings.TrimSpace(ps.Description), strings.TrimSpace(ps.Repo))
		if err != nil {
			return applied, appliedErr(applied, err)
		}
		applied.Slices = append(applied.Slices, appliedSlice{Slice: s, Milestone: m})
	}
	return applied, nil
}

// appliedErr says what a failed run had already written, so nobody re-runs a
// plan whose first half is already in Notion.
func appliedErr(applied appliedPlan, err error) error {
	if len(applied.Milestones) == 0 && len(applied.Slices) == 0 {
		return err
	}
	return fmt.Errorf("%w — %s were created before this failed and are still in Notion",
		err, counts(len(applied.Milestones), len(applied.Slices)))
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
	Milestones []milestoneJSON  `json:"milestones"`
	Slices     []addedSliceJSON `json:"slices"`
}

// jsonDoc renders the run as JSON, with empty lists rather than nulls so a
// consumer can iterate both without checking.
func (a appliedPlan) jsonDoc(project config.ProjectConfig) planAppliedJSON {
	doc := planAppliedJSON{
		Milestones: make([]milestoneJSON, 0, len(a.Milestones)),
		Slices:     make([]addedSliceJSON, 0, len(a.Slices)),
	}
	for _, m := range a.Milestones {
		doc.Milestones = append(doc.Milestones, milestoneJSON{
			ID: m.ID, Name: m.Name, Order: m.Order, Status: string(m.Status), URL: m.URL,
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

	for _, m := range a.Milestones {
		fmt.Fprintf(&b, "\n## %s\n\n", m.Name)
		fmt.Fprintf(&b, "New milestone %s, %s — %s\n\n", formatOrder(m.Order), blank(string(m.Status)), pageRef(m.ID, m.URL))
		b.WriteString(sliceList(a.slicesUnder(m.ID)))
	}
	for _, m := range a.existingMilestones() {
		fmt.Fprintf(&b, "\n## %s\n\n", m.Name)
		b.WriteString(sliceList(a.slicesUnder(m.ID)))
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
