package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/store"
)

// info prints everything an agent needs to know about a project: the
// conventions written on its project page, its milestones in plan order, and
// its slices grouped under them. Markdown by default, because the reader is
// usually a person or a model; --json for anything parsing it.
//
// By default it reads the plan as the store's own copy stands
// ([store.StoredPlan]), never paying [store.Mirrored]'s staleness pull —
// every `nat info` is a fresh process with no board sitting on the result
// long enough for that staleness to matter, and the pull it would otherwise
// pay measures at roughly forty times the cost of a file read. --refresh
// restores today's behaviour (pull first when the file's copy is stale) for
// a caller that wants to be sure it is current.
func info(ctx context.Context, args []string, env Env) error {
	asJSON, projectRef, refresh, expand, err := parseInfoFlags(args)
	if err != nil {
		return err
	}

	cfg, projectID, project, err := env.projectFor(projectRef)
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
	var plan store.Plan
	if refresh {
		plan, err = st.Plan(ctx, storeProject(projectID, project))
	} else {
		plan, err = store.StoredPlan(ctx, st, storeProject(projectID, project))
	}
	if err != nil {
		return err
	}

	p := plan.Project
	// The plan file may hold a name for a source project (one made before
	// the rule); its plugin's title, from projectFor, is the name.
	if project.IsSource() {
		p.Name = project.Name
	}

	if asJSON {
		var src *sourceInfoJSON
		if ss, ok := st.(sourceStore); ok {
			src = sourceInfo(ctx, ss, project, p, expand)
		}
		return writeInfoJSON(env.Out, p, conventions, projectID == cfg.ScratchProject, src, plan.Shape.HasBranch)
	}
	_, err = io.WriteString(env.Out, infoMarkdown(p, conventions))
	return err
}

// parseInfoFlags reads info's own command line: --json and --project like
// every other read, plus --refresh and --expand — info's alone, so it gets its
// own parser rather than widening [parseJSONFlag] for every command that
// shares it.
func parseInfoFlags(args []string) (asJSON bool, projectRef string, refresh bool, expand []string, err error) {
	flags := flag.NewFlagSet("info", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	j := flags.Bool("json", false, "print structured JSON instead of markdown")
	r := flags.Bool("refresh", false, "pull from the workspace first if the replica is stale, instead of reading it as it stands")
	var groups stringList
	flags.Var(&groups, "expand", "a source project's lazy sidebar group to fill in, by id; repeat for more")
	ref := projectFlag(flags)
	if err := flags.Parse(args); err != nil {
		return false, "", false, nil, usageErrorf("info: %s", err)
	}
	if flags.NArg() > 0 {
		return false, "", false, nil, usageErrorf("info: unexpected argument %q", flags.Arg(0))
	}
	return *j, *ref, *r, groups, nil
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
	// Source is a source project's plugin and its sidebar tree; absent for
	// any other project.
	Source *sourceInfoJSON `json:"source,omitempty"`
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
	// Unfiled marks the scratch project's reserved milestone (unfiledMilestone).
	Unfiled bool `json:"unfiled,omitempty"`
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
	// Resumed says the slice is work resumed on a published slice — see
	// [domain.Slice.Resumed]: in progress, a pull request recorded, its
	// branch cleared, on a project with a Branch column.
	Resumed bool   `json:"resumed"`
	State   string `json:"state,omitempty"`
}

// writeInfoJSON encodes the project as JSON, indented: it is read by people as
// often as by programs, and a stream nobody can skim is a poor default.
// scratch says p is the scratch project, whose unfiledMilestone is marked.
// hasBranch is whether the project has a Branch column, which is what tells
// resumed work from a pull request recorded on a project that has none.
func writeInfoJSON(out io.Writer, p domain.Project, conventions string, scratch bool, src *sourceInfoJSON, hasBranch bool) error {
	doc := infoJSON{
		Project:    projectJSON{ID: p.ID, Name: p.Name, Conventions: conventions},
		Milestones: make([]milestoneJSON, 0, len(p.Milestones)),
		Slices:     make([]sliceJSON, 0, len(p.Slices)),
		Source:     src,
	}
	for _, m := range p.Milestones {
		doc.Milestones = append(doc.Milestones, milestoneJSON{
			ID: m.ID, Name: m.Name, Order: m.Order, Status: string(m.Status),
			Unfiled: scratch && m.Name == unfiledMilestone,
		})
	}

	slicesByID := domain.SlicesByID(p.Slices)
	for _, s := range p.Slices {
		doc.Slices = append(doc.Slices, sliceJSONOf(s, slicesByID, hasBranch))
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// sliceJSONOf is one slice as info prints it, its blocked and state read
// against slicesByID — the plan it sits in — and hasBranch, whether the
// project has a Branch column. container-show prints a container's tasks
// through it too, so a task reads the same from either.
func sliceJSONOf(s domain.Slice, slicesByID map[string]domain.Slice, hasBranch bool) sliceJSON {
	sj := sliceJSON{
		ID: s.ID, Name: s.Name, Status: s.StatusName, MilestoneID: s.MilestoneID,
		Assignee: s.AssigneeName, PR: s.PRURL, URL: s.URL,
		Branch: s.Branch, Repo: s.Repo, DependsOn: s.DependsOn,
		Blocked: domain.Blocked(s, slicesByID), HandedBack: s.HandedBack(),
		Resumed: s.Resumed(hasBranch),
	}
	// The CLI takes no tmux or gh reading, so the state is the page's own:
	// a Done slice reads as none, and a live agent's working/waiting are
	// the app's to overlay from `nat status`.
	if state := domain.StateOf(s, domain.AgentNone, domain.PRUnread, slicesByID, hasBranch); state != domain.SliceStateNone {
		sj.State = state.String()
	}
	return sj
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
