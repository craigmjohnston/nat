package cli

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/source"
	"github.com/craigmjohnston/nat/internal/store"
)

// unlistedGroupID is the group nat adds to a source project's sidebar for the
// containers with tasks that the plugin's own tree leaves out. It begins with
// `_`, which the protocol reserves for nat, so no plugin's group can be it.
const unlistedGroupID = "_unlisted"

// sourceInfoJSON is a source project's share of `info --json`: who its plugin
// says it is, and the tree its sidebar section draws — the plugin's groups,
// then nat's own _unlisted group. Error is the plugin read that failed, with
// whatever it could not read left empty.
type sourceInfoJSON struct {
	Name          string          `json:"name"`
	Title         string          `json:"title"`
	Tag           string          `json:"tag"`
	IconSymbol    string          `json:"icon_symbol"`
	IconSVG       string          `json:"icon_svg"`
	ContainerNoun string          `json:"container_noun"`
	TaskNoun      string          `json:"task_noun"`
	Menu          []source.Action `json:"menu"`
	Groups        []source.Group  `json:"groups"`
	Error         string          `json:"error"`
}

// sourceInfo reads a source project's plugin for info: describe, then the
// sidebar with expand's lazy groups filled in — whose header menu, where it
// sends one, replaces describe's static one. A failed read concludes
// nothing — info still prints, the error is carried, and the containers
// with tasks are still drawn from the plan's cached titles under _unlisted.
// A describe that fails is not followed by a sidebar read: a plugin that
// cannot say who it is will not draw a tree either, and each attempt can
// cost a timeout.
func sourceInfo(ctx context.Context, ss sourceStore, project config.ProjectConfig, p domain.Project, expand []string) *sourceInfoJSON {
	d, err := ss.Describe(ctx)
	var sb source.Sidebar
	if err == nil {
		sb, err = ss.Sidebar(ctx, expand)
	}
	groups, menu := sb.Groups, d.Menu
	if sb.Menu != nil {
		menu = sb.Menu
	}
	out := &sourceInfoJSON{
		Name:          project.Source,
		Title:         d.Title,
		Tag:           d.Tag,
		IconSymbol:    d.IconSymbol,
		IconSVG:       d.IconSVG,
		ContainerNoun: cmp.Or(d.ContainerNoun, "container"),
		TaskNoun:      cmp.Or(d.TaskNoun, "task"),
		Menu:          append([]source.Action{}, menu...),
	}
	if err != nil {
		logging.Error("task source unread for info", "project", p.ID, "source", project.Source, "err", err)
		out.Error = err.Error()
	}
	out.Groups = append(append([]source.Group{}, groups...), unlistedGroups(groups, p, out.ContainerNoun)...)
	return out
}

// unlistedGroups is nat's own _unlisted group: every container of the plan
// with at least one task under it that appears nowhere in the plugin's tree —
// one in a lazy group not yet opened, one a filter dropped, one the remote
// deleted — titled from the plan's cached title, so a task never vanishes
// from the sidebar because its container did. None when there are none.
func unlistedGroups(groups []source.Group, p domain.Project, noun string) []source.Group {
	listed := map[string]bool{}
	for _, g := range groups {
		for _, c := range g.Containers {
			listed[c.ID] = true
		}
		for _, child := range g.Children {
			for _, c := range child.Containers {
				listed[c.ID] = true
			}
		}
	}
	withTasks := map[string]bool{}
	for _, s := range p.Slices {
		withTasks[s.MilestoneID] = true
	}
	var containers []source.Container
	for _, m := range p.Milestones {
		if withTasks[m.ID] && !listed[m.ID] {
			containers = append(containers, source.Container{ID: m.ID, Title: m.Name})
		}
	}
	if len(containers) == 0 {
		return nil
	}
	count := len(containers)
	return []source.Group{{ID: unlistedGroupID, Label: "Other " + noun + "s", Count: &count, Containers: containers}}
}

// describeSource asks a plugin who it is with an empty project — the
// envelope's fields all "", since no project is in question — and refuses one
// speaking any protocol but this build's. [source.Exec] refuses that too; this
// is the check for every client, said in the spec's words.
func describeSource(ctx context.Context, name string, src source.Client) (source.Describe, error) {
	d, err := src.Describe(ctx, source.Project{})
	if err != nil {
		return source.Describe{}, err
	}
	if d.Protocol != source.ProtocolVersion {
		return source.Describe{}, fmt.Errorf("source plugin %s speaks protocol %d; this nat speaks protocol %d",
			name, d.Protocol, source.ProtocolVersion)
	}
	return d, nil
}

// sourcePluginJSON is one installed plugin as source-list prints it: where it
// was found, and either what it says about itself or why it would not say.
type sourcePluginJSON struct {
	Name     string           `json:"name"`
	Path     string           `json:"path"`
	Describe *source.Describe `json:"describe,omitempty"`
	Error    string           `json:"error,omitempty"`
}

// sourceList lists every installed task-source plugin, each described
// best-effort: one that will not describe — it fails, times out, speaks
// another protocol — is listed with its error, never dropped, since an
// installed plugin that is broken is exactly what someone listing them is
// looking for. It acts on no project.
func sourceList(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("source-list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of plain text")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("source-list: takes no arguments, given %d", len(rest))
	}
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	plugins, err := source.Discover(dir)
	if err != nil {
		return fmt.Errorf("look for task source plugins: %w", err)
	}

	listed := make([]sourcePluginJSON, 0, len(plugins))
	for _, p := range plugins {
		pj := sourcePluginJSON{Name: p.Name, Path: p.Path}
		d, err := describePlugin(ctx, env, p.Name)
		if err != nil {
			pj.Error = err.Error()
		} else {
			pj.Describe = &d
		}
		listed = append(listed, pj)
	}

	if *asJSON {
		return writeJSON(env.Out, listed)
	}
	_, err = io.WriteString(env.Out, sourceListText(listed))
	return err
}

// describePlugin builds the named plugin's client and describes it.
func describePlugin(ctx context.Context, env Env, name string) (source.Describe, error) {
	src, err := env.NewSource(name)
	if err != nil {
		return source.Describe{}, err
	}
	return describeSource(ctx, name, src)
}

// sourceListText is source-list's plain form: one line per plugin.
func sourceListText(listed []sourcePluginJSON) string {
	if len(listed) == 0 {
		return "no task source plugins installed\n"
	}
	var b strings.Builder
	for _, p := range listed {
		if p.Describe == nil {
			fmt.Fprintf(&b, "%s\t%s\terror: %s\n", p.Name, p.Path, p.Error)
			continue
		}
		fmt.Fprintf(&b, "%s\t%s\t%s (%s)\n", p.Name, p.Path, p.Describe.Title, p.Describe.Tag)
	}
	return b.String()
}

// containerShowJSON is one container as its plugin describes it, with the
// plan's tasks filed under it, each in the shape info gives a slice.
type containerShowJSON struct {
	Container source.ContainerDetail `json:"container"`
	Tasks     []sliceJSON            `json:"tasks"`
}

// containerShow prints one of a source project's containers: the plugin's
// detail, as-is, and the tasks the plan files under it. A failed plugin read
// is the command's error — there is no cached detail to fall back on, only a
// title.
func containerShow(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("container-show", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 || strings.TrimSpace(rest[0]) == "" {
		return usageErrorf("container-show: want exactly one container, by id")
	}
	id := strings.TrimSpace(rest[0])

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	ss, err := env.sourceStoreFor(ctx, "container-show", projectID, project)
	if err != nil {
		return err
	}
	detail, err := ss.Container(ctx, id)
	if err != nil {
		return err
	}
	plan, err := store.StoredPlan(ctx, ss, storeProject(projectID, project))
	if err != nil {
		return err
	}
	slicesByID := domain.SlicesByID(plan.Project.Slices)
	tasks := []sliceJSON{}
	for _, s := range plan.Project.Slices {
		if s.MilestoneID == id {
			tasks = append(tasks, sliceJSONOf(s, slicesByID))
		}
	}

	if *asJSON {
		return writeJSON(env.Out, containerShowJSON{Container: detail, Tasks: tasks})
	}
	_, err = io.WriteString(env.Out, containerMarkdown(detail, tasks))
	return err
}

// containerMarkdown is container-show's plain form: the title, the facts, the
// tasks.
func containerMarkdown(d source.ContainerDetail, tasks []sliceJSON) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", d.Title)
	if d.ExternalURL != "" {
		fmt.Fprintf(&b, "%s\n\n", d.ExternalURL)
	}
	for _, f := range d.Facts {
		fmt.Fprintf(&b, "- %s: %s\n", f.Label, f.Value)
	}
	b.WriteString("\n## Tasks\n\n")
	if len(tasks) == 0 {
		b.WriteString("_none_\n")
	}
	for _, t := range tasks {
		fmt.Fprintf(&b, "- %s — %s\n", t.Name, blank(t.Status))
	}
	return b.String()
}

// sourceActionJSON is what an action's plugin had to say.
type sourceActionJSON struct {
	Message string `json:"message"`
}

// sourceAction runs one of a source project's plugin actions: against a
// group with --group, a container with --container, the source itself with
// neither. It is a write — the plugin's — so a board watching is nudged.
func sourceAction(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("source-action", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	action := flags.String("action", "", "the plugin action to run, by `id` (required)")
	group := flags.String("group", "", "run it against this sidebar group, by `id`")
	container := flags.String("container", "", "run it against this container, by `id`")
	input := flags.String("input", "", "the action's input, for a text or choice action; `-` reads it from stdin")
	asJSON := flags.Bool("json", false, "print structured JSON instead of plain text")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("source-action: takes no arguments, given %d", len(rest))
	}
	id := strings.TrimSpace(*action)
	if id == "" {
		return usageErrorf("source-action: no action given: pass --action")
	}
	target := source.Target{Group: strings.TrimSpace(*group), Container: strings.TrimSpace(*container)}
	if target.Group != "" && target.Container != "" {
		return usageErrorf("source-action: --group and --container are mutually exclusive")
	}
	text, err := briefText("source-action", "--input", *input, env.In)
	if err != nil {
		return err
	}

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	ss, err := env.sourceStoreFor(ctx, "source-action", projectID, project)
	if err != nil {
		return err
	}
	result, err := ss.Action(ctx, id, target, text)
	if err != nil {
		return err
	}
	env.nudged()

	if *asJSON {
		return writeJSON(env.Out, sourceActionJSON(result))
	}
	_, err = io.WriteString(env.Out, cmp.Or(result.Message, "Ran "+id+".")+"\n")
	return err
}

// sourceSetupJSON is what a plugin said of a value it was set up with.
type sourceSetupJSON struct {
	Message string `json:"message"`
}

// sourceSetup hands an installed plugin the value of one of its setup fields
// — an API token, say — read from stdin, never a flag, so it is in no argv
// and no `ps`. The plugin is described first, so an id it does not list is
// refused before the value is read, let alone sent; so is an empty value. It
// acts on no project.
func sourceSetup(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("source-setup", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	fieldID := flags.String("id", "", "the setup field to set, by `id` (required)")
	asJSON := flags.Bool("json", false, "print structured JSON instead of plain text")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 || strings.TrimSpace(rest[0]) == "" {
		return usageErrorf("source-setup: want exactly one plugin, by name")
	}
	name, id := strings.TrimSpace(rest[0]), strings.TrimSpace(*fieldID)
	if id == "" {
		return usageErrorf("source-setup: no setup field given: pass --id")
	}

	src, err := env.NewSource(name)
	if err != nil {
		return err
	}
	d, err := describeSource(ctx, name, src)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(d.Setup, func(f source.SetupField) bool { return f.ID == id }) {
		return fmt.Errorf("source-setup: %s has no setup field %q", name, id)
	}
	if env.In == nil {
		return usageErrorf("source-setup: the value is read from stdin, and there is nothing to read")
	}
	b, err := io.ReadAll(env.In)
	if err != nil {
		return fmt.Errorf("source-setup: read the value: %w", err)
	}
	value := strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r")
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("source-setup: no value for %s's %s on stdin", name, id)
	}
	msg, err := src.Setup(ctx, id, value)
	if err != nil {
		return err
	}

	if *asJSON {
		return writeJSON(env.Out, sourceSetupJSON{Message: msg})
	}
	_, err = io.WriteString(env.Out, cmp.Or(msg, "Set "+name+"'s "+id+".")+"\n")
	return err
}
