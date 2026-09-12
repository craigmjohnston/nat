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
	"github.com/craigmjohnston/nat/internal/store"
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

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(projectID, project)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	sp := storeProject(projectID, project)

	shape, err := st.Shape(ctx, sp)
	if err != nil {
		return err
	}
	added, err := st.AddMilestones(ctx, sp, shape, []string{name})
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
	brief, err := briefText("slice-add", "--description", *description, env.In)
	if err != nil {
		return err
	}
	deps := make([]string, len(dependsOn))
	for i, ref := range dependsOn {
		if deps[i], err = pageID("slice-add", ref); err != nil {
			return err
		}
	}

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(projectID, project)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	sp := storeProject(projectID, project)

	shape, err := st.Shape(ctx, sp)
	if err != nil {
		return err
	}
	milestone, err := resolveMilestone(*milestoneRef, shape.Milestones)
	if err != nil {
		return err
	}

	s, err := st.AddSlice(ctx, sp, store.NewSlice{
		Title:     title,
		Brief:     brief,
		Repo:      strings.TrimSpace(*repo),
		Milestone: milestone,
		DependsOn: deps,
	})
	if err != nil {
		return fmt.Errorf("create the slice: %w", err)
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

// briefText settles a flag's text: the value as given, or stdin when it asks
// for it. Empty is allowed — a one-line slice whose title says everything is a
// real thing to file — so this fails only when stdin cannot be read. The
// command and the flag are named because more than one command takes text this
// way and not all under the same flag, and a misuse should say which it was.
func briefText(command, flagName, value string, in io.Reader) (string, error) {
	if value != stdinRef {
		return strings.TrimSpace(value), nil
	}
	if in == nil {
		return "", usageErrorf("%s: %s - was given but there is nothing to read", command, flagName)
	}
	b, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("read the %s: %w", strings.TrimPrefix(flagName, "--"), err)
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

// textBlock builds a block of the given type holding one span of plain text,
// which is the shape every block written from here takes.
func textBlock(blockType, text string) map[string]any {
	return map[string]any{
		"object": "block",
		"type":   blockType,
		blockType: map[string]any{
			"rich_text": []map[string]any{{
				"type": "text",
				"text": map[string]any{"content": text},
			}},
		},
	}
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
