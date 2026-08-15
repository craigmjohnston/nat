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
	"github.com/craigmjohnston/nat/internal/notion"
)

// nextSlice claims the next slice an agent should pick up and prints a brief it
// can work from without asking anything else: the slice, where to work, its page
// body, and the project's conventions. The claim happens before the printing —
// an agent that reads a brief has, by then, already been given the slice.
func nextSlice(ctx context.Context, args []string, env Env) error {
	asJSON, err := parseJSONFlag("next-slice", args)
	if err != nil {
		return err
	}

	cfg, project, err := env.activeProject()
	if err != nil {
		return err
	}
	if cfg.AssigneeUserID == "" {
		return fmt.Errorf("no assignee in the config: open the board with `nat` and finish setting it up")
	}
	client := env.NewClient(env.Tokens.Token)

	shape, err := sliceShape(ctx, client, project)
	if err != nil {
		return err
	}
	milestone, next, err := selectNextSlice(ctx, client, cfg.ActiveProjectID, project, shape)
	if err != nil {
		return err
	}
	claimed, err := claim(ctx, client, next.ID, shape, cfg.AssigneeUserID)
	if err != nil {
		return err
	}

	brief, err := body(ctx, client, claimed.ID)
	if err != nil {
		return fmt.Errorf("claimed %q but could not read its brief: %w", claimed.Name, err)
	}
	conventions, err := body(ctx, client, cfg.ActiveProjectID)
	if err != nil {
		return fmt.Errorf("claimed %q but could not read the project conventions: %w", claimed.Name, err)
	}

	b := briefOf(claimed, milestone, project, cfg.AssigneeUserName, brief, conventions)
	if asJSON {
		return writeBriefJSON(env.Out, b, cfg.ActiveProjectID, project.Name)
	}
	_, err = io.WriteString(env.Out, briefMarkdown(b, project.Name))
	return err
}

// selectNextSlice finds the slice to work next: the first unclaimed Todo slice
// the board would list, under the lowest-ordered milestone still open. The plan
// is read exactly the way the board reads it — whichever shape it is kept in,
// and in the order the project's own board puts its slices in — so the slice
// handed out is the one someone looking at that board would expect to go next.
//
// The filtering is done here rather than in the query because a Status column
// may be a select or a Notion status depending on how the project was set up,
// and the two need differently-shaped filters; the plans are small enough that
// reading them whole costs nothing.
func selectNextSlice(ctx context.Context, client API, projectID string, project config.ProjectConfig, shape notion.SliceShape) (domain.Milestone, domain.Slice, error) {
	milestones, err := loadMilestones(ctx, client, project, shape)
	if err != nil {
		return domain.Milestone{}, domain.Slice{}, err
	}
	pages, err := client.QueryDataSource(ctx, project.SlicesDSID, nil,
		[]notion.Sort{{Timestamp: notion.TimestampCreated, Direction: notion.SortAscending}})
	if err != nil {
		return domain.Milestone{}, domain.Slice{}, fmt.Errorf("load slices: %w", err)
	}
	plan := domain.NewProject(projectID, project.Name, milestones, domain.InViewOrder(
		domain.SlicesFromPages(pages), notion.PlanOrder(ctx, client, shape, project.SlicesDSID)))

	var open []domain.Milestone
	for _, g := range plan.Groups() {
		if g.Milestone == nil || !drawnFrom(*g.Milestone) {
			continue
		}
		open = append(open, *g.Milestone)
		for _, s := range g.Slices {
			if s.Status == domain.SliceTodo && s.AssigneeName == "" {
				return *g.Milestone, s, nil
			}
		}
	}
	if len(open) == 0 {
		return domain.Milestone{}, domain.Slice{}, noOpenMilestoneError(shape)
	}
	return domain.Milestone{}, domain.Slice{}, fmt.Errorf("no unclaimed Todo slice in the %s %s: %s",
		openAdjective(shape), plural("milestone", len(open)), strings.Join(milestoneNames(open), ", "))
}

// drawnFrom reports whether work may be taken from a milestone. One with a page
// of its own says so itself: Queued has not been started and Done is finished,
// so only an Active milestone is drawn from, and an agent cannot run ahead of a
// plan its author has not opened up.
//
// A derived milestone has no status to say it with. Its status is read back off
// the slices under it, so it is Queued until one of them starts — gating on
// Active there would mean a plan on which nothing has begun could never begin,
// with no status anyone could write to unblock it. Everything not yet finished
// is open instead, and the plan's own order decides which comes first.
func drawnFrom(m domain.Milestone) bool {
	if m.Derived {
		return m.Status != domain.MilestoneDone
	}
	return m.Status == domain.MilestoneActive
}

// openAdjective names what the milestones drawn from have in common, so a
// refusal describes the plan the reader is actually looking at.
func openAdjective(shape notion.SliceShape) string {
	if shape.MilestonesRelated() {
		return "Active"
	}
	return "unfinished"
}

// noOpenMilestoneError says there is nowhere to take work from, and what would
// change that: a milestone to activate under the shape that has statuses, and
// under the shape that has none — where every milestone is finished — a plan
// with more in it.
func noOpenMilestoneError(shape notion.SliceShape) error {
	if shape.MilestonesRelated() {
		return fmt.Errorf("no Active milestone: activate one on the board and run this again")
	}
	return fmt.Errorf("no unfinished milestone: every milestone in the plan is Done")
}

// sliceShape reads how the project's Slices table is put together: what its
// in-progress status is called, and whether it has an Assignee column at all.
// Both differ between projects created before and after the app started asking,
// and neither can be guessed from a page alone.
func sliceShape(ctx context.Context, client API, project config.ProjectConfig) (notion.SliceShape, error) {
	ds, err := slicesDataSource(ctx, client, project)
	if err != nil {
		return notion.SliceShape{}, err
	}
	return notion.ShapeOf(ds), nil
}

// slicesDataSource reads the project's Slices data source. Its schema is both
// where the shape is read from and — for a project whose plan is that schema's
// own Milestone options — what a new milestone is appended to, so a command
// that files one needs the schema itself rather than the shape read off it.
func slicesDataSource(ctx context.Context, client API, project config.ProjectConfig) (*notion.DataSource, error) {
	ds, err := client.GetDataSource(ctx, project.SlicesDSID)
	if err != nil {
		return nil, fmt.Errorf("read the slices schema: %w", err)
	}
	return ds, nil
}

// claim takes the slice: status to the project's in-progress option, and the
// assignee set to the configured user where the project tracks one. The page
// Notion answers with is checked rather than assumed — a people value naming
// someone the workspace does not know comes back empty instead of failing, and
// an agent must not be handed a brief for a slice it does not actually hold.
func claim(ctx context.Context, client API, sliceID string, shape notion.SliceShape, userID string) (domain.Slice, error) {
	properties := map[string]notion.PropertyValue{
		notion.PropStatus: notion.NewChoice(shape.StatusType, shape.InProgress),
	}
	if shape.HasAssignee {
		properties[notion.PropAssignee] = notion.NewPeople(userID)
	}
	updated, err := client.UpdatePageProperties(ctx, sliceID, properties)
	if err != nil {
		return domain.Slice{}, fmt.Errorf("claim the slice: %w", err)
	}
	if !holds(*updated, shape, userID) {
		return domain.Slice{}, fmt.Errorf("the claim on %q did not stick: someone else holds it",
			domain.SliceFromPage(*updated).Name)
	}
	s := domain.SliceFromPage(*updated)
	logging.Action("slice claimed", "slice", s.ID, "name", s.Name, "user", userID)
	return s, nil
}

// holds reports whether the page came back held by the given user, which is what
// a successful claim looks like from the outside. Without an Assignee column the
// status is the whole answer: there is nobody else the slice could belong to, so
// a project that tracks no assignee decides ownership on status alone.
func holds(page notion.Page, shape notion.SliceShape, userID string) bool {
	if page.Properties[notion.PropStatus].SelectName() != shape.InProgress {
		return false
	}
	if !shape.HasAssignee {
		return true
	}
	for _, id := range page.Properties[notion.PropAssignee].PeopleIDs() {
		if id == userID {
			return true
		}
	}
	return false
}

// body renders a page's content as markdown, which is how both the slice brief
// and the project conventions reach the agent reading them.
func body(ctx context.Context, client API, pageID string) (string, error) {
	blocks, err := client.GetBlockChildren(ctx, pageID)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(notion.Markdown(blocks)), nil
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
func briefMarkdown(b brief, projectName string) string {
	var s strings.Builder
	fmt.Fprintf(&s, "# %s\n\n", b.Slice.Name)
	fmt.Fprintf(&s, "Claimed for %s. Work exactly this slice.\n\n", b.Assignee)

	fmt.Fprintf(&s, "- Project: %s\n", projectName)
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
