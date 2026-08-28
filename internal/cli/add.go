package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// milestoneAdd files one new milestone at the end of the plan. It is the
// one-off counterpart to planning a whole milestone's worth of work: a phase
// somebody thought of afterwards, added without opening the board.
//
// The new milestone is Queued, never Active. Activating one is a decision about
// what is being worked on now, and a command that adds a phase to the plan has
// no business making it.
func milestoneAdd(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("milestone-add", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("milestone-add: want exactly one milestone name, given %d", len(rest))
	}
	name := strings.TrimSpace(rest[0])
	if name == "" {
		return usageErrorf("milestone-add: the milestone name is empty")
	}

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

	added, err := addMilestones(ctx, client, project.SlicesDSID, ds, existing, []string{name})
	if err != nil {
		return err
	}
	env.nudged()
	m := added[0]

	if *asJSON {
		return writeJSON(env.Out, milestoneAddedJSON{Milestone: milestoneJSON{
			ID: m.ID, Name: m.Name, Order: m.Order, Status: string(m.Status),
		}})
	}
	_, err = io.WriteString(env.Out, milestoneAddedMarkdown(m, project.Name))
	return err
}

// sliceAdd files one new slice under a milestone, Todo and unassigned — which
// is what makes it something an agent can be handed next.
//
// The milestone is named rather than assumed: a slice belongs to a phase of the
// plan, and one filed under the wrong phase is worse than one not filed at all.
func sliceAdd(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-add", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	milestoneRef := flags.String("milestone", "", "the milestone to file the slice under, by name")
	description := flags.String("description", "", "the brief to write on the slice page; `-` reads it from stdin")
	repo := flags.String("repo", "", "working directory for this slice, overriding the project default")
	var dependsOn stringList
	flags.Var(&dependsOn, "depends-on", "a slice this one waits on, by URL or ID; repeat for more")
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-add: want exactly one slice title, given %d", len(rest))
	}
	title := strings.TrimSpace(rest[0])
	if title == "" {
		return usageErrorf("slice-add: the slice title is empty")
	}
	if strings.TrimSpace(*milestoneRef) == "" {
		return usageErrorf("slice-add: no milestone given: pass --milestone")
	}
	// The brief is settled before anything is read from Notion, so a slice-add
	// whose stdin cannot be read fails having written nothing.
	brief, err := briefText("slice-add", *description, env.In)
	if err != nil {
		return err
	}
	deps := make([]string, len(dependsOn))
	for i, ref := range dependsOn {
		if deps[i], err = pageID("slice-add", ref); err != nil {
			return err
		}
	}

	_, _, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	client := env.NewClient(env.Tokens.Token)

	shape, err := sliceShape(ctx, client, project)
	if err != nil {
		return err
	}
	milestone, err := resolveMilestone(*milestoneRef, milestonesOf(shape))
	if err != nil {
		return err
	}

	s, err := createSlice(ctx, client, project.SlicesDSID, milestone, title, brief, strings.TrimSpace(*repo), deps)
	if err != nil {
		return err
	}
	env.nudged()

	if *asJSON {
		return writeJSON(env.Out, sliceAddedJSON{Slice: addedSliceJSON{
			ID:            s.ID,
			Name:          s.Name,
			Status:        string(s.Status),
			MilestoneID:   milestone.ID,
			MilestoneName: milestone.Name,
			Repo:          resolvedRepo(s, project),
			URL:           s.URL,
		}})
	}
	_, err = io.WriteString(env.Out, sliceAddedMarkdown(s, milestone, project))
	return err
}

// addMilestones files milestones at the end of the plan and returns them in the
// order they were given. The plan is the options of the slices' own Milestone
// column, and every new milestone is appended to it in one schema write: either
// they all arrive or none do.
func addMilestones(ctx context.Context, client API, slicesDSID string, ds *notion.DataSource, existing []domain.Milestone, names []string) ([]domain.Milestone, error) {
	// A plan that adds no milestone writes nothing: the write would replace the
	// option list with a copy of itself, which is a real edit to make of a schema
	// for the sake of nothing.
	if len(names) == 0 {
		return nil, nil
	}
	return appendMilestoneOptions(ctx, client, slicesDSID, ds, existing, names)
}

// appendMilestoneOptions adds milestones to a plan kept as the options of the
// Slices data source's Milestone column, after the options already there:
// their order in the column is the order of the plan, so a milestone added to
// the end of one is an option added to the end of the other.
//
// A name the plan already holds is refused before anything is written. Such a
// milestone is nothing but its name — it is what a slice's column names, and so
// what groups the plan — and two options sharing one could not be told apart.
func appendMilestoneOptions(ctx context.Context, client API, slicesDSID string, ds *notion.DataSource, existing []domain.Milestone, names []string) ([]domain.Milestone, error) {
	taken := map[string]string{}
	for _, m := range existing {
		taken[strings.ToLower(strings.TrimSpace(m.Name))] = m.Name
	}
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if held, dup := taken[key]; dup {
			return nil, fmt.Errorf("the plan already has a milestone named %q: "+
				"its milestones are the options of the slices' %s column, which cannot hold two of a name",
				held, notion.PropMilestone)
		}
		taken[key] = name
	}

	milestone := ds.Properties[notion.PropMilestone]
	property, ok := milestone.AppendedOptions(names...)
	if !ok {
		return nil, fmt.Errorf("the %s column is a %s: a milestone can only be added to it in Notion",
			notion.PropMilestone, milestone.Type)
	}
	if _, err := client.UpdateDataSourceProperties(ctx, slicesDSID,
		map[string]notion.PropertySchema{notion.PropMilestone: property}); err != nil {
		return nil, fmt.Errorf("create the %s: %w", plural("milestone", len(names)), err)
	}

	// The order of a derived milestone is its place among the options, counting
	// from zero, which is what reading the plan back would make of it.
	added := make([]domain.Milestone, len(names))
	for i, name := range names {
		added[i] = domain.Milestone{
			ID:         name,
			Name:       name,
			Order:      float64(len(existing) + i),
			Status:     domain.MilestoneStatusOf(nil),
			SelectType: milestone.Type,
		}
		logging.Action("milestone added", "milestone", name, "order", added[i].Order)
	}
	return added, nil
}

// createSlice writes the slice, with its brief as the page body. Status and
// assignee are not the caller's to choose: a newly filed slice is Todo and
// unclaimed, or it is not something the workflow can pick up.
// The milestone is written in whichever shape the plan is kept in — a relation
// to its page, or the option naming it — which the milestone itself knows.
//
// dependsOn is the slices the new one waits on, and is left off the write
// entirely when there are none: a project whose table has no dependency column
// can still have slices added to it, and sending an empty relation to a column
// that is not there would be the one thing stopping that.
func createSlice(ctx context.Context, client API, slicesDSID string, milestone domain.Milestone, title, brief, repo string, dependsOn []string) (domain.Slice, error) {
	properties := map[string]notion.PropertyValue{
		notion.PropName:      notion.NewTitle(title),
		notion.PropStatus:    notion.NewSelect(notion.SliceTodo),
		notion.PropMilestone: milestone.Ref(),
		notion.PropRepo:      notion.NewRichText(repo),
	}
	if len(dependsOn) > 0 {
		properties[notion.PropDependsOn] = notion.NewRelation(dependsOn...)
	}
	page, err := client.CreatePage(ctx, notion.DataSourceParent(slicesDSID), properties, paragraphBlocks(brief))
	if err != nil {
		return domain.Slice{}, fmt.Errorf("create the slice: %w", err)
	}
	s := domain.SliceFromPage(*page)
	logging.Action("slice added", "slice", s.ID, "name", title, "milestone", milestone.ID)
	return s, nil
}

// resolveMilestone finds the milestone a new slice is filed under, by name: a
// milestone is an option of the slices' Milestone column and so is nothing but
// its name — there is no page to name it by instead.
//
// Names are matched case-insensitively and exactly: a prefix match would make
// adding a slice depend on which milestones happen to exist, which is not
// something anyone typing a name can see.
func resolveMilestone(ref string, milestones []domain.Milestone) (domain.Milestone, error) {
	name := strings.TrimSpace(ref)
	var matches []domain.Milestone
	for _, m := range milestones {
		if strings.EqualFold(strings.TrimSpace(m.Name), name) {
			matches = append(matches, m)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return domain.Milestone{}, fmt.Errorf("no milestone named %q: %s", name, knownMilestones(milestones))
	default:
		// Two options of a name cannot be told apart: a milestone is its name,
		// and nothing else names it.
		return domain.Milestone{}, fmt.Errorf("%d milestones are named %q: rename one in Notion", len(matches), name)
	}
}

// knownMilestones says what there was to choose from, which is the whole of
// what someone who named one wrongly needs to fix it.
func knownMilestones(ms []domain.Milestone) string {
	if len(ms) == 0 {
		return "the project has no milestones yet — add one with `nat milestone-add`"
	}
	return "the project's milestones are " + strings.Join(milestoneNames(ms), ", ")
}

// stdinRef is what --description is given to mean "the brief is being piped in".
// Reading stdin whenever the flag is absent would be the shorter rule, but a
// slice-add typed at a terminal without a brief would then hang waiting for one
// — and a brief is optional, so that is an ordinary way to run the command.
const stdinRef = "-"

// briefText settles a page's body: the flag, or stdin when the flag asks for
// it. An empty brief is allowed — a one-line slice whose title says everything
// is a real thing to file — so this fails only when stdin cannot be read. The
// command is named because more than one takes a description this way, and a
// misuse should say which one it was.
func briefText(command, description string, in io.Reader) (string, error) {
	if description != stdinRef {
		return strings.TrimSpace(description), nil
	}
	if in == nil {
		return "", usageErrorf("%s: --description - was given but there is nothing to read", command)
	}
	b, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("read the description: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

// paragraphBlocks turns a brief into the page body: one paragraph per
// blank-line-separated chunk, and nothing at all for an empty brief. Plain
// paragraphs only, for the same reason the completion note is — the text
// arrives as text, and half-parsing markdown out of it would mislead.
func paragraphBlocks(text string) []map[string]any {
	var blocks []map[string]any
	for _, chunk := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		if trimmed := strings.TrimSpace(chunk); trimmed != "" {
			blocks = append(blocks, textBlock("paragraph", trimmed))
		}
	}
	return blocks
}

// resolvedRepo is the directory work on the slice happens in: its own override
// when it has one, the project default otherwise — the same resolution a brief
// prints, so a slice reads the same when it is added as when it is claimed.
func resolvedRepo(s domain.Slice, project config.ProjectConfig) string {
	if s.Repo != "" {
		return s.Repo
	}
	return project.WorkingDir
}

// milestoneAddedJSON and sliceAddedJSON are the structured forms of what was
// created, each wrapping the page in a named field so a consumer reads the same
// shape whichever command it ran.
type milestoneAddedJSON struct {
	Milestone milestoneJSON `json:"milestone"`
}

type sliceAddedJSON struct {
	Slice addedSliceJSON `json:"slice"`
}

// addedSliceJSON is a created slice: what info reports about a slice, plus the
// milestone's name and the resolved working directory, which are what the
// person who just filed it wants confirmed.
type addedSliceJSON struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	MilestoneID   string `json:"milestone_id"`
	MilestoneName string `json:"milestone_name"`
	Repo          string `json:"repo"`
	URL           string `json:"url"`
}

// writeJSON encodes a document, indented for the same reason every other JSON
// output here is: it is read by people as often as by programs.
func writeJSON(out io.Writer, doc any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// milestoneAddedMarkdown reports the milestone as filed, saying where in the
// plan it landed and that it is Queued — the two things that were decided for
// the caller rather than by them.
func milestoneAddedMarkdown(m domain.Milestone, projectName string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", m.Name)
	fmt.Fprintf(&b, "Added to %s as milestone %s, %s.\n\n", projectName, planPosition(m), blank(string(m.Status)))
	fmt.Fprintf(&b, "- %s\n", optionNote)
	return b.String()
}

// optionNote is what there is to say about where a milestone lives: it has no
// page to link to, so the line that would carry one says why.
const optionNote = "An option of the slices' Milestone column, with no page of its own — " +
	"its status follows the slices filed under it."

// planPosition is where in the plan a milestone sits, as someone reading the
// board counts: its place among the Milestone column's options, which counts
// from zero, so the first milestone is milestone 1.
func planPosition(m domain.Milestone) string {
	return formatOrder(m.Order + 1)
}

// sliceAddedMarkdown reports the slice as filed: which milestone holds it, and
// where the work would happen.
func sliceAddedMarkdown(s domain.Slice, m domain.Milestone, project config.ProjectConfig) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.Name)
	fmt.Fprintf(&b, "Added to %s, %s and unclaimed.\n\n", m.Name, blank(s.StatusName))
	fmt.Fprintf(&b, "- Notion page: %s\n", s.ID)
	if s.URL != "" {
		fmt.Fprintf(&b, "- Notion URL: %s\n", s.URL)
	}
	if repo := resolvedRepo(s, project); repo != "" {
		fmt.Fprintf(&b, "- Working directory: %s\n", repo)
	}
	return b.String()
}
