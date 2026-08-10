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

	milestone, page, err := selectNextSlice(ctx, client, project)
	if err != nil {
		return err
	}
	claimed, err := claim(ctx, client, *page, cfg.AssigneeUserID)
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

// selectNextSlice finds the slice to work next: the oldest unclaimed Todo slice
// under the lowest-ordered Active milestone. Milestones that are Queued have not
// been started and Done ones are finished, so neither is drawn from — which is
// also what keeps an agent from running ahead of the plan.
//
// The filtering is done here rather than in the query because a Status column
// may be a select or a Notion status depending on how the project was set up,
// and the two need differently-shaped filters; the plans are small enough that
// reading them whole costs nothing.
func selectNextSlice(ctx context.Context, client API, project config.ProjectConfig) (domain.Milestone, *notion.Page, error) {
	milestones, err := client.QueryDataSource(ctx, project.MilestonesDSID, nil,
		[]notion.Sort{{Property: notion.PropOrder, Direction: notion.SortAscending}})
	if err != nil {
		return domain.Milestone{}, nil, fmt.Errorf("load milestones: %w", err)
	}
	slices, err := client.QueryDataSource(ctx, project.SlicesDSID, nil,
		[]notion.Sort{{Timestamp: notion.TimestampCreated, Direction: notion.SortAscending}})
	if err != nil {
		return domain.Milestone{}, nil, fmt.Errorf("load slices: %w", err)
	}

	var active []domain.Milestone
	for _, m := range domain.MilestonesFromPages(milestones) {
		if m.Status == domain.MilestoneActive {
			active = append(active, m)
		}
	}
	if len(active) == 0 {
		return domain.Milestone{}, nil, fmt.Errorf("no Active milestone: activate one on the board and run this again")
	}

	for _, m := range active {
		for i, p := range slices {
			s := domain.SliceFromPage(p)
			if s.MilestoneID == m.ID && s.Status == domain.SliceTodo && s.AssigneeName == "" {
				return m, &slices[i], nil
			}
		}
	}
	return domain.Milestone{}, nil, fmt.Errorf("no unclaimed Todo slice in the Active %s: %s",
		plural("milestone", len(active)), strings.Join(milestoneNames(active), ", "))
}

// claim takes the slice: assignee set to the configured user, status to
// Claimed. The page Notion answers with is checked rather than assumed — a
// people value naming someone the workspace does not know comes back empty
// instead of failing, and an agent must not be handed a brief for a slice it
// does not actually hold.
func claim(ctx context.Context, client API, page notion.Page, userID string) (domain.Slice, error) {
	statusType := page.Properties[notion.PropStatus].Type
	updated, err := client.UpdatePageProperties(ctx, page.ID, map[string]notion.PropertyValue{
		notion.PropAssignee: notion.NewPeople(userID),
		notion.PropStatus:   notion.NewChoice(statusType, notion.SliceClaimed),
	})
	if err != nil {
		return domain.Slice{}, fmt.Errorf("claim the slice: %w", err)
	}
	if !holds(*updated, userID) {
		return domain.Slice{}, fmt.Errorf("the claim on %q did not stick: someone else holds it",
			domain.SliceFromPage(*updated).Name)
	}
	s := domain.SliceFromPage(*updated)
	logging.Action("slice claimed", "slice", s.ID, "name", s.Name, "user", userID)
	return s, nil
}

// holds reports whether the page came back claimed by the given user, which is
// what a successful claim looks like from the outside.
func holds(page notion.Page, userID string) bool {
	if page.Properties[notion.PropStatus].SelectName() != notion.SliceClaimed {
		return false
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
			Status:        string(b.Slice.Status),
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
