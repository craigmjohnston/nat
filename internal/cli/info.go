package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/craigmjohnston/nat/internal/domain"
)

// info prints everything an agent needs to know about a project: the
// conventions written on its project page, its milestones in plan order, and
// its slices grouped under them. Markdown by default, because the reader is
// usually a person or a model; --json for anything parsing it.
func info(ctx context.Context, args []string, env Env) error {
	asJSON, projectRef, err := parseJSONFlag("info", args)
	if err != nil {
		return err
	}

	_, projectID, project, err := env.projectFor(projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}

	conventions, err := st.Body(ctx, projectID)
	if err != nil {
		return fmt.Errorf("load project page: %w", err)
	}
	plan, err := st.Plan(ctx, storeProject(projectID, project))
	if err != nil {
		return err
	}

	p := plan.Project

	if asJSON {
		return writeInfoJSON(env.Out, p, conventions)
	}
	_, err = io.WriteString(env.Out, infoMarkdown(p, conventions))
	return err
}

// parseJSONFlag reads the command line of a command whose only flags are --json
// and --project and which takes no arguments, which is every command here so
// far. The flag package's own error output is thrown away and the failure
// returned instead, so a bad flag is reported the same way every other misuse
// is.
func parseJSONFlag(command string, args []string) (bool, string, error) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	if err := flags.Parse(args); err != nil {
		return false, "", usageErrorf("%s: %s", command, err)
	}
	if flags.NArg() > 0 {
		return false, "", usageErrorf("%s: unexpected argument %q", command, flags.Arg(0))
	}
	return *asJSON, *projectRef, nil
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
}

type sliceJSON struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	MilestoneID string   `json:"milestone_id"`
	Assignee    string   `json:"assignee"`
	PR          string   `json:"pr"`
	URL         string   `json:"url"`
	Branch      string   `json:"branch,omitempty"`
	Repo        string   `json:"repo,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
	Blocked     bool     `json:"blocked"`
	HandedBack  bool     `json:"handed_back"`
	State       string   `json:"state,omitempty"`
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
			ID: m.ID, Name: m.Name, Order: m.Order, Status: string(m.Status),
		})
	}

	slicesByID := domain.SlicesByID(p.Slices)
	for _, s := range p.Slices {
		sj := sliceJSON{
			ID: s.ID, Name: s.Name, Status: s.StatusName, MilestoneID: s.MilestoneID,
			Assignee: s.AssigneeName, PR: s.PRURL, URL: s.URL,
			Branch: s.Branch, Repo: s.Repo, DependsOn: s.DependsOn,
			Blocked: domain.Blocked(s, slicesByID), HandedBack: s.HandedBack(),
		}
		// The CLI takes no tmux or gh reading, so the state is the page's own:
		// a Done slice reads as none, and a live agent's working/waiting are
		// the app's to overlay from `nat status`.
		if state := domain.StateOf(s, domain.AgentNone, domain.PRUnread, slicesByID); state != domain.SliceStateNone {
			sj.State = state.String()
		}
		doc.Slices = append(doc.Slices, sj)
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// infoMarkdown renders the project as markdown — see [domain.PlanMarkdown],
// which a planning launch's inline prompt renders the same document through,
// so an agent reads the same document however it was handed one.
func infoMarkdown(p domain.Project, conventions string) string {
	return domain.PlanMarkdown(p, conventions)
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
