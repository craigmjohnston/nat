package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/store"
)

// nextSlice claims the next slice an agent should pick up and prints a brief it
// can work from without asking anything else: the slice, where to work, its page
// body, and the project's conventions. The claim happens before the printing —
// an agent that reads a brief has, by then, already been given the slice.
func nextSlice(ctx context.Context, args []string, env Env) error {
	asJSON, projectRef, err := parseJSONFlag("next-slice", args)
	if err != nil {
		return err
	}

	cfg, projectID, project, err := env.projectFor(projectRef)
	if err != nil {
		return err
	}
	me, err := ownerOf(cfg, project)
	if err != nil {
		return err
	}
	st, err := env.storeFor(projectID, project)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	plan, err := st.Plan(ctx, storeProject(projectID, project))
	if err != nil {
		return err
	}
	milestone, next, err := selectNextSlice(plan.Project)
	if err != nil {
		return err
	}
	claimed, err := claim(ctx, st, next.ID, plan.Shape, me.ID)
	if err != nil {
		return err
	}
	// The claim is the write, so the board is nudged here — even a run that
	// fails to read the brief afterwards has already moved the slice.
	env.nudged()

	brief, err := st.Body(ctx, claimed.ID)
	if err != nil {
		return fmt.Errorf("claimed %q but could not read its brief: %w", claimed.Name, err)
	}
	conventions, err := st.Body(ctx, projectID)
	if err != nil {
		return fmt.Errorf("claimed %q but could not read the project conventions: %w", claimed.Name, err)
	}

	b := briefOf(claimed, milestone, project, me.Name, brief, conventions)
	if asJSON {
		return writeBriefJSON(env.Out, b, projectID, project.Name)
	}
	_, err = io.WriteString(env.Out, briefMarkdown(b, projectID, project.Name))
	return err
}

// selectNextSlice finds the slice to work next: the first unclaimed, unblocked
// Todo slice the board would list, under the lowest-ordered milestone still
// open. The plan is read exactly the way the board reads it — in the order the
// project's own board puts its slices in — so the slice handed out is the one
// someone looking at that board would expect to go next.
//
// A slice waiting on unfinished work is passed over rather than stopped at: it
// is not workable yet, but the slices under it may well be, and a plan that
// halts at the first blocked slice would hand out nothing until somebody
// noticed. Only when every candidate is blocked is there nothing to do, and
// then the refusal says which slices are waiting on what.
//
// A milestone has no status of its own to gate on: it is read back off the
// slices under it, so it is Queued until one of them starts. Everything not yet
// finished is open, and the plan's own order decides which comes first — gating
// on Active would leave a plan on which nothing has begun with no way to begin.
//
// The choosing is done here rather than asked of the store because a plan is
// small enough to read whole, and every rule below — the milestone's own
// status, the dependency graph, the cycles — is about the plan rather than
// about where it is kept.
func selectNextSlice(plan domain.Project) (domain.Milestone, domain.Slice, error) {
	// The whole plan is the index: every slice a dependency could name is in it,
	// and one it does not hold is a page this project cannot see.
	byID := domain.SlicesByID(plan.Slices)
	// A cycle is read here rather than left to show as a slice that is somehow
	// never ready: the plan is in hand, and this is the one place that can say
	// which dependency to break.
	cycles := logCycles(plan.Slices)

	var open []domain.Milestone
	var waiting []string
	for _, g := range plan.Groups() {
		if g.Milestone == nil || g.Milestone.Status == domain.MilestoneDone {
			continue
		}
		open = append(open, *g.Milestone)
		for _, s := range g.Slices {
			if s.Status != domain.SliceTodo || s.AssigneeName != "" {
				continue
			}
			blockers, unknown := domain.Blockers(s, byID)
			for _, id := range unknown {
				logging.Action("dependency is not in the plan", "slice", s.ID, "dependency", id)
			}
			if len(blockers) == 0 {
				return *g.Milestone, s, nil
			}
			// Only a blocked slice is reported as being in a cycle: one whose
			// dependencies are all Done is workable, whatever the graph does
			// further round.
			if cycle := cycles[domain.NormaliseID(s.ID)]; len(cycle) > 0 {
				waiting = append(waiting, cycleNote(s, cycle))
				continue
			}
			waiting = append(waiting, fmt.Sprintf("%q waits on %s", s.Name, blockerList(blockers)))
		}
	}
	if len(open) == 0 {
		return domain.Milestone{}, domain.Slice{}, fmt.Errorf(
			"no unfinished milestone: every milestone in the plan is Done")
	}
	if len(waiting) > 0 {
		return domain.Milestone{}, domain.Slice{}, fmt.Errorf(
			"every unclaimed Todo slice in the unfinished %s is blocked: %s",
			plural("milestone", len(open)), strings.Join(waiting, "; "))
	}
	return domain.Milestone{}, domain.Slice{}, fmt.Errorf("no unclaimed Todo slice in the unfinished %s: %s",
		plural("milestone", len(open)), strings.Join(milestoneNames(open), ", "))
}

// storeProject names a project to the store: the page the conventions live
// on, what it is called, and where its slices are kept. It is the one place
// this machine's idea of a project — a config entry, working directory and
// all — is narrowed to what a store has any business reading.
func storeProject(projectID string, project config.ProjectConfig) store.Project {
	return store.Project{ID: projectID, Name: project.Name, SlicesID: project.SlicesDSID}
}

// sliceShape reads how the project's Slices table is put together: whether it
// records ownership or a branch at all, and the milestones there are to file a
// slice under. None of it can be guessed from a page alone.
func sliceShape(ctx context.Context, st store.Store, projectID string, project config.ProjectConfig) (store.Shape, error) {
	return st.Shape(ctx, storeProject(projectID, project))
}

// loadSlice reads one slice, saying what it was doing when the read failed —
// the store reports what went wrong and the command says what it was after.
func loadSlice(ctx context.Context, st store.Store, id string) (domain.Slice, store.Shape, error) {
	s, sh, err := st.Slice(ctx, id)
	if err != nil {
		return s, sh, fmt.Errorf("load the slice: %w", err)
	}
	return s, sh, nil
}

// claim takes the slice for the configured user. The slice the store answers
// with is checked rather than assumed — a people value naming someone the
// workspace does not know comes back empty instead of failing, and an agent
// must not be handed a brief for a slice it does not actually hold.
func claim(ctx context.Context, st store.Store, sliceID string, shape store.Shape, userID string) (domain.Slice, error) {
	claimed, err := st.ClaimSlice(ctx, sliceID, shape, userID)
	if err != nil {
		return domain.Slice{}, fmt.Errorf("claim the slice: %w", err)
	}
	if !store.Holds(claimed, shape, userID) {
		return domain.Slice{}, fmt.Errorf("the claim on %q did not stick: someone else holds it", claimed.Name)
	}
	return claimed, nil
}

// milestoneNames lists milestones by name, for saying which ones were looked in.
func milestoneNames(ms []domain.Milestone) []string {
	names := make([]string, len(ms))
	for i, m := range ms {
		names[i] = m.Name
	}
	return names
}

// plural pluralises an English noun by the count it is being used with.
func plural(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// brief is everything printed about a claimed slice, gathered once so the
// markdown and the JSON say the same things.
type brief struct {
	Slice       domain.Slice
	Milestone   domain.Milestone
	Repo        string
	Assignee    string
	Body        string
	Conventions string
}

// briefOf assembles the brief. The repo is the slice's own override when it has
// one and the project default otherwise — resolved here so the agent is told one
// directory rather than a rule to apply.
func briefOf(s domain.Slice, m domain.Milestone, project config.ProjectConfig, assignee, body, conventions string) brief {
	repo := s.Repo
	if repo == "" {
		repo = project.WorkingDir
	}
	return brief{Slice: s, Milestone: m, Repo: repo, Assignee: assignee, Body: body, Conventions: conventions}
}

// briefJSON is the structured form of the brief, for anything parsing it. It is
// what both commands that hand out a brief print, so an agent reads the same
// document however it was started.
type briefJSON struct {
	Slice   briefSliceJSON `json:"slice"`
	Project projectJSON    `json:"project"`
}

type briefSliceJSON struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	Assignee      string `json:"assignee"`
	MilestoneID   string `json:"milestone_id"`
	MilestoneName string `json:"milestone_name"`
	Repo          string `json:"repo"`
	Brief         string `json:"brief"`
	URL           string `json:"url"`
}

// writeBriefJSON encodes the brief, indented for the same reason info's is: it
// is read by people as often as by programs.
func writeBriefJSON(out io.Writer, b brief, projectID, projectName string) error {
	doc := briefJSON{
		Slice: briefSliceJSON{
			ID:            b.Slice.ID,
			Name:          b.Slice.Name,
			Status:        b.Slice.StatusName,
			Assignee:      b.Assignee,
			MilestoneID:   b.Milestone.ID,
			MilestoneName: b.Milestone.Name,
			Repo:          b.Repo,
			Brief:         b.Body,
			URL:           b.Slice.URL,
		},
		Project: projectJSON{ID: projectID, Name: projectName, Conventions: b.Conventions},
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// briefMarkdown renders the brief: what was claimed, then the facts an agent
// needs before it starts, then the slice's own body and the project conventions
// as they were written.
//
// The project's page ID is among those facts, and says what it is for: it is
// what --project names, and a session that reads only the brief would otherwise
// have to go looking for it — leaving every command it ran acting on whatever
// project the user's board happened to be on.
func briefMarkdown(b brief, projectID, projectName string) string {
	var s strings.Builder
	fmt.Fprintf(&s, "# %s\n\n", b.Slice.Name)
	fmt.Fprintf(&s, "Claimed for %s. Work exactly this slice.\n\n", b.Assignee)

	fmt.Fprintf(&s, "- Project: %s\n", projectName)
	fmt.Fprintf(&s, "- Project page ID: %s (pass it as --project on every nat command)\n", projectID)
	if b.Milestone.Name != "" {
		fmt.Fprintf(&s, "- Milestone: %s\n", b.Milestone.Name)
	}
	fmt.Fprintf(&s, "- Notion page: %s\n", b.Slice.ID)
	if b.Slice.URL != "" {
		fmt.Fprintf(&s, "- Notion URL: %s\n", b.Slice.URL)
	}
	if b.Repo != "" {
		fmt.Fprintf(&s, "- Working directory: %s\n", b.Repo)
	}

	s.WriteString("\n## Brief\n\n")
	s.WriteString(section(b.Body))
	s.WriteString("\n## Project conventions\n\n")
	s.WriteString(section(b.Conventions))
	return s.String()
}

// section prints a page body, or says it is empty — an empty heading reads as
// output that got cut off.
func section(text string) string {
	if text == "" {
		return "_none_\n"
	}
	return text + "\n"
}
