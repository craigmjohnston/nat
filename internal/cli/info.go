package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// info prints everything an agent needs to know about the active project: the
// conventions written on its project page, its milestones in plan order, and
// its slices grouped under them. Markdown by default, because the reader is
// usually a person or a model; --json for anything parsing it.
func info(ctx context.Context, args []string, env Env) error {
	asJSON, err := parseJSONFlag("info", args)
	if err != nil {
		return err
	}

	cfg, project, err := env.activeProject()
	if err != nil {
		return err
	}
	client := env.NewClient(env.Tokens.Token)

	blocks, err := client.GetBlockChildren(ctx, cfg.ActiveProjectID)
	if err != nil {
		return fmt.Errorf("load project page: %w", err)
	}
	shape, err := sliceShape(ctx, client, project)
	if err != nil {
		return err
	}
	milestones, err := loadMilestones(ctx, client, project, shape)
	if err != nil {
		return err
	}
	slices, err := client.QueryDataSource(ctx, project.SlicesDSID, nil,
		[]notion.Sort{{Timestamp: notion.TimestampCreated, Direction: notion.SortAscending}})
	if err != nil {
		return fmt.Errorf("load slices: %w", err)
	}

	p := domain.NewProject(cfg.ActiveProjectID, project.Name, milestones, domain.InViewOrder(
		domain.SlicesFromPages(slices), notion.PlanOrder(ctx, client, shape, project.SlicesDSID)))
	conventions := strings.TrimSpace(notion.Markdown(blocks))

	if asJSON {
		return writeInfoJSON(env.Out, p, conventions)
	}
	_, err = io.WriteString(env.Out, infoMarkdown(p, conventions))
	return err
}

// loadMilestones reads a project's plan in whichever shape it keeps it: the
// pages of its Milestones data source, in plan order, or — for a project whose
// slices name their milestone on a select — that select's options, which the
// schema already carries and so need no query of their own. Either way one list
// of domain milestones comes out, so nothing downstream asks about the shape.
func loadMilestones(ctx context.Context, client API, project config.ProjectConfig, shape notion.SliceShape) ([]domain.Milestone, error) {
	if !shape.MilestonesRelated() {
		return domain.MilestonesFromOptions(shape.MilestoneOptions), nil
	}
	pages, err := client.QueryDataSource(ctx, project.MilestonesDSID, nil,
		[]notion.Sort{{Property: notion.PropOrder, Direction: notion.SortAscending}})
	if err != nil {
		return nil, fmt.Errorf("load milestones: %w", err)
	}
	return domain.MilestonesFromPages(pages), nil
}

// parseJSONFlag reads the command line of a command whose only flag is --json
// and which takes no arguments, which is every command here so far. The flag
// package's own error output is thrown away and the failure returned instead, so
// a bad flag is reported the same way every other misuse is.
func parseJSONFlag(command string, args []string) (bool, error) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	if err := flags.Parse(args); err != nil {
		return false, usageErrorf("%s: %s", command, err)
	}
	if flags.NArg() > 0 {
		return false, usageErrorf("%s: unexpected argument %q", command, flags.Arg(0))
	}
	return *asJSON, nil
}

// infoJSON is the structured form of the info output. Milestones and slices are
// flat lists in the order they were read, related by ID, so a consumer can index
// them however it likes rather than walking the grouping this package chose.
type infoJSON struct {
	Project    projectJSON     `json:"project"`
	Milestones []milestoneJSON `json:"milestones"`
	Slices     []sliceJSON     `json:"slices"`
}

type projectJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Conventions string `json:"conventions"`
}

type milestoneJSON struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Order  float64 `json:"order"`
	Status string  `json:"status"`
	URL    string  `json:"url"`
}

type sliceJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	MilestoneID string `json:"milestone_id"`
	Assignee    string `json:"assignee"`
	PR          string `json:"pr"`
	URL         string `json:"url"`
}

// writeInfoJSON encodes the project as JSON, indented: it is read by people as
// often as by programs, and a stream nobody can skim is a poor default.
func writeInfoJSON(out io.Writer, p domain.Project, conventions string) error {
	doc := infoJSON{
		Project:    projectJSON{ID: p.ID, Name: p.Name, Conventions: conventions},
		Milestones: make([]milestoneJSON, 0, len(p.Milestones)),
		Slices:     make([]sliceJSON, 0, len(p.Slices)),
	}
	for _, m := range p.Milestones {
		doc.Milestones = append(doc.Milestones, milestoneJSON{
			ID: m.ID, Name: m.Name, Order: m.Order, Status: string(m.Status), URL: m.URL,
		})
	}
	for _, s := range p.Slices {
		doc.Slices = append(doc.Slices, sliceJSON{
			ID: s.ID, Name: s.Name, Status: s.StatusName, MilestoneID: s.MilestoneID,
			Assignee: s.AssigneeName, PR: s.PRURL, URL: s.URL,
		})
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// infoMarkdown renders the project as markdown: the conventions as written,
// then the milestones in plan order, then the slices under the milestone each
// belongs to. Slices with no milestone land in a trailing Unassigned section,
// the same as they do on the board.
func infoMarkdown(p domain.Project, conventions string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", p.Name)
	if conventions != "" {
		fmt.Fprintf(&b, "\n%s\n", conventions)
	}

	b.WriteString("\n## Milestones\n\n")
	if len(p.Milestones) == 0 {
		b.WriteString("_none_\n")
	}
	for _, g := range p.Groups() {
		if g.Milestone == nil {
			continue
		}
		fmt.Fprintf(&b, "- %s. %s — %s\n", formatOrder(g.Milestone.Order), g.Milestone.Name, blank(string(g.Milestone.Status)))
	}

	b.WriteString("\n## Slices\n\n")
	if len(p.Slices) == 0 {
		b.WriteString("_none_\n")
	}
	first := true
	for _, g := range p.Groups() {
		if len(g.Slices) == 0 {
			continue
		}
		if !first {
			b.WriteString("\n")
		}
		first = false
		fmt.Fprintf(&b, "### %s\n\n", g.Name())
		for _, s := range g.Slices {
			fmt.Fprintf(&b, "- %s — %s\n", s.Name, strings.Join(sliceFacts(s), " · "))
		}
	}
	return b.String()
}

// sliceFacts is what is worth saying about a slice beside its name: its status
// as the project's own board names it, then whoever holds it and whatever PR came out of it, each left out
// when there is none rather than printed as an empty field.
func sliceFacts(s domain.Slice) []string {
	facts := []string{blank(s.StatusName)}
	if s.AssigneeName != "" {
		facts = append(facts, s.AssigneeName)
	}
	if s.PRURL != "" {
		facts = append(facts, "PR "+s.PRURL)
	}
	return facts
}

// formatOrder prints a milestone's order without a trailing ".0": the orders
// are whole numbers in practice, and fractions only appear when something was
// slotted between two of them.
func formatOrder(order float64) string {
	return strconv.FormatFloat(order, 'f', -1, 64)
}

// blank names an empty status, which is what a page missing the property or
// carrying an unset select reads as. Printing nothing there would leave a line
// ending in a dash.
func blank(status string) string {
	if status == "" {
		return "(no status)"
	}
	return status
}
