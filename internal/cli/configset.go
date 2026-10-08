package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// The keys config-set answers to: the settings form's own fields, plus one
// project's working directory, run commands, colour, name, model pairs, merge
// method, delete-branch switch, base branch and tag, addressed by its page ID. Nothing else in the
// file is reachable this way — see configShow's doc comment for why.
const (
	keySplitPercent     = "agent_split_percent"
	keyPollSeconds      = "poll_seconds"
	keyWorkshopModel    = "workshop_agent.model"
	keyWorkshopEffort   = "workshop_agent.effort"
	keySliceModel       = "slice_agent.model"
	keySliceEffort      = "slice_agent.effort"
	projectKeyPrefix    = "project."
	workingDirKeySuffix = ".working_dir"
	runsKeySuffix       = ".runs"
	colorKeySuffix      = ".color"
	nameKeySuffix       = ".name"
	// The per-project keys this slice's fields answer to, each written by
	// [projectFieldSetters].
	sliceModelKeySuffix     = ".slice_agent.model"
	sliceEffortKeySuffix    = ".slice_agent.effort"
	workshopModelKeySuffix  = ".workshop_agent.model"
	workshopEffortKeySuffix = ".workshop_agent.effort"
	mergeMethodKeySuffix    = ".merge_method"
	deleteBranchKeySuffix   = ".delete_branch"
	baseBranchKeySuffix     = ".base_branch"
	tagKeySuffix            = ".tag"
	// autoColor is the value that clears a project's colour, so the save that
	// follows picks one afresh ([config.Config.AssignColors]).
	autoColor = "auto"
)

// configSet writes one local config key. There is no --project flag: a
// project is instead named inside the key itself, project.<id>.working_dir,
// since this is the one project field the key space reaches into and naming
// it any other way would need a second addressing scheme beside the first.
//
// An empty value is how a field is unset — zero for the two numbers, which is
// what the config file itself writes unset as, and the empty string for
// everything else — matching the settings form's own rule that a field
// cleared back to empty is "unset" rather than a value to keep.
func configSet(args []string, env Env) error {
	flags := flag.NewFlagSet("config-set", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 2 {
		return usageErrorf("config-set: want exactly a key and a value, given %d", len(rest))
	}
	key, value := rest[0], rest[1]

	cfg, found, err := env.Load()
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no configuration yet: run `nat` once to set it up")
	}

	if err := applyConfigSet(&cfg, key, value); err != nil {
		return err
	}
	if err := env.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	_, err = io.WriteString(env.Out, configSetMarkdown(key, reportedValue(cfg, key, value)))
	return err
}

// reportedValue is what config-set says it wrote: the value given, except a
// colour set to auto, which says the name the save chose — read off cfg, whose
// map the save filled in — and a tag, which says it as stored, uppercased.
func reportedValue(cfg config.Config, key, value string) string {
	if strings.HasPrefix(key, projectKeyPrefix) && strings.HasSuffix(key, tagKeySuffix) {
		pid, _ := projectKeyFor(cfg, strings.TrimSuffix(strings.TrimPrefix(key, projectKeyPrefix), tagKeySuffix))
		return cfg.Projects[pid].Tag
	}
	if value != autoColor || !strings.HasPrefix(key, projectKeyPrefix) || !strings.HasSuffix(key, colorKeySuffix) {
		return value
	}
	pid, _ := projectKeyFor(cfg, strings.TrimSuffix(strings.TrimPrefix(key, projectKeyPrefix), colorKeySuffix))
	return cfg.Projects[pid].Color
}

// applyConfigSet writes value onto the field key names, refusing exactly what
// a later read of the config would discard: the two numeric keys are checked
// against the same bounds the settings form validates against, so a typo is
// refused here rather than silently swapped for the default the next launch
// reads.
func applyConfigSet(cfg *config.Config, key, value string) error {
	switch {
	case key == keySplitPercent:
		n, err := parseConfigInt(key, value)
		if err != nil {
			return err
		}
		if verr := config.ValidSplitPercent(n); verr != nil {
			return fmt.Errorf("config-set: %w", verr)
		}
		cfg.AgentSplitPercent = n
	case key == keyPollSeconds:
		n, err := parseConfigInt(key, value)
		if err != nil {
			return err
		}
		if verr := config.ValidPollSeconds(n); verr != nil {
			return fmt.Errorf("config-set: %w", verr)
		}
		cfg.PollSeconds = n
	case key == keyWorkshopModel:
		cfg.WorkshopAgent.Model = value
	case key == keyWorkshopEffort:
		cfg.WorkshopAgent.Effort = value
	case key == keySliceModel:
		cfg.SliceAgent.Model = value
	case key == keySliceEffort:
		cfg.SliceAgent.Effort = value
	case strings.HasPrefix(key, projectKeyPrefix) && strings.HasSuffix(key, workingDirKeySuffix):
		return applyProjectWorkingDir(cfg, key, value)
	case strings.HasPrefix(key, projectKeyPrefix) && strings.HasSuffix(key, runsKeySuffix):
		return applyProjectRuns(cfg, key, value)
	case strings.HasPrefix(key, projectKeyPrefix) && strings.HasSuffix(key, colorKeySuffix):
		return applyProjectColor(cfg, key, value)
	case strings.HasPrefix(key, projectKeyPrefix) && strings.HasSuffix(key, nameKeySuffix):
		return applyProjectName(cfg, key, value)
	case strings.HasPrefix(key, projectKeyPrefix):
		for _, f := range projectFieldSetters {
			if strings.HasSuffix(key, f.suffix) {
				return applyProjectField(cfg, key, f.suffix, value, f.set)
			}
		}
		return usageErrorf("config-set: unknown key %q", key)
	default:
		return usageErrorf("config-set: unknown key %q", key)
	}
	return nil
}

// applyProjectWorkingDir writes value as the working directory of the project
// project.<id>.working_dir names. The ID is matched the way every other
// command matches one — as written, then with dashes and case ignored, since
// an ID copied out of a page URL has none — but the config already loaded is
// what it is matched against rather than a second read of the file, since
// config-set has that config in hand already.
func applyProjectWorkingDir(cfg *config.Config, key, value string) error {
	id := strings.TrimSuffix(strings.TrimPrefix(key, projectKeyPrefix), workingDirKeySuffix)
	pid, err := projectKeyFor(*cfg, id)
	if err != nil {
		return err
	}
	p := cfg.Projects[pid]
	p.WorkingDir = value
	cfg.Projects[pid] = p
	return nil
}

// applyProjectRuns writes value — a JSON array of run commands, the whole
// list at once — as the runs of the project project.<id>.runs names, by the
// same addressing [applyProjectWorkingDir] uses. The empty string unsets them,
// the key space's one rule for every field; a list the config would not keep
// ([config.ValidRuns]) is refused here, where it is written.
func applyProjectRuns(cfg *config.Config, key, value string) error {
	id := strings.TrimSuffix(strings.TrimPrefix(key, projectKeyPrefix), runsKeySuffix)
	pid, err := projectKeyFor(*cfg, id)
	if err != nil {
		return err
	}
	var runs []config.RunCommand
	if strings.TrimSpace(value) != "" {
		if err := json.Unmarshal([]byte(value), &runs); err != nil {
			return usageErrorf("config-set: %s wants a JSON array of runs, like "+
				`[{"label":"Run","command":"make run","scope":"slice"}]: %v`, key, err)
		}
		if err := config.ValidRuns(runs); err != nil {
			return fmt.Errorf("config-set: %w", err)
		}
	}
	p := cfg.Projects[pid]
	p.Runs = nil
	if len(runs) > 0 {
		p.Runs = runs
	}
	cfg.Projects[pid] = p
	return nil
}

// applyProjectColor writes value as the colour of the project
// project.<id>.color names, by [applyProjectWorkingDir]'s addressing. A
// palette name is written as given; auto clears the field, so the save that
// follows chooses one. Anything else is refused — the empty string too, since
// the word for "choose again" is auto, and a project is never left with none —
// and so is any colour for a project that takes none ([config.Config.Colorable]).
func applyProjectColor(cfg *config.Config, key, value string) error {
	id := strings.TrimSuffix(strings.TrimPrefix(key, projectKeyPrefix), colorKeySuffix)
	pid, err := projectKeyFor(*cfg, id)
	if err != nil {
		return err
	}
	if !cfg.Colorable(pid) {
		return fmt.Errorf("config-set: %s is the scratch project or a source project, which take no colour", id)
	}
	if value != autoColor && !config.ValidProjectColor(value) {
		return usageErrorf("config-set: %s wants one of %s, or %s, given %q",
			key, strings.Join(config.ProjectColors, ", "), autoColor, value)
	}
	p := cfg.Projects[pid]
	p.Color = value
	if value == autoColor {
		p.Color = ""
	}
	cfg.Projects[pid] = p
	return nil
}

// applyProjectName writes value, trimmed, as the name of the project
// project.<id>.name names, by [applyProjectWorkingDir]'s addressing. The
// empty string is refused rather than unsetting it — a project is never left
// with no name to be called by — and so is any name for a source project,
// which is called what its plugin's describe calls itself and never what its
// entry says.
func applyProjectName(cfg *config.Config, key, value string) error {
	id := strings.TrimSuffix(strings.TrimPrefix(key, projectKeyPrefix), nameKeySuffix)
	pid, err := projectKeyFor(*cfg, id)
	if err != nil {
		return err
	}
	p := cfg.Projects[pid]
	if p.IsSource() {
		return fmt.Errorf("config-set: %s is a source project, named by its plugin, not its config entry", id)
	}
	name := strings.TrimSpace(value)
	if name == "" {
		return usageErrorf("config-set: %s wants a name, given none", key)
	}
	p.Name = name
	cfg.Projects[pid] = p
	return nil
}

// projectFieldSetter writes one per-project key's value onto its entry, or
// refuses it — the value as given, the key for a refusal to name.
type projectFieldSetter struct {
	suffix string
	set    func(p *config.ProjectConfig, key, value string) error
}

// projectFieldSetters are the per-project keys that need no more than the
// entry itself. A model or effort is written as given — an unknown one is
// Claude Code's to refuse at launch, as the global pair's is — and the empty
// string unsets every one of them.
var projectFieldSetters = []projectFieldSetter{
	{sliceModelKeySuffix, func(p *config.ProjectConfig, _, v string) error { p.SliceAgent.Model = v; return nil }},
	{sliceEffortKeySuffix, func(p *config.ProjectConfig, _, v string) error { p.SliceAgent.Effort = v; return nil }},
	{workshopModelKeySuffix, func(p *config.ProjectConfig, _, v string) error { p.WorkshopAgent.Model = v; return nil }},
	{workshopEffortKeySuffix, func(p *config.ProjectConfig, _, v string) error { p.WorkshopAgent.Effort = v; return nil }},
	{mergeMethodKeySuffix, setMergeMethod},
	{deleteBranchKeySuffix, setDeleteBranch},
	{baseBranchKeySuffix, func(p *config.ProjectConfig, _, v string) error { p.BaseBranch = strings.TrimSpace(v); return nil }},
	{tagKeySuffix, setTag},
}

// setMergeMethod writes a merge word, refusing one gh pr merge has no flag for.
func setMergeMethod(p *config.ProjectConfig, key, value string) error {
	if !config.ValidMergeMethod(value) {
		return usageErrorf("config-set: %s wants one of %s, or nothing for the default, given %q",
			key, strings.Join(config.MergeMethods, ", "), value)
	}
	p.MergeMethod = value
	return nil
}

// setDeleteBranch writes the delete-branch switch: true or false, the empty
// string being false — the field's unset.
func setDeleteBranch(p *config.ProjectConfig, key, value string) error {
	if strings.TrimSpace(value) == "" {
		p.DeleteBranch = false
		return nil
	}
	b, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return usageErrorf("config-set: %s wants true or false, given %q", key, value)
	}
	p.DeleteBranch = b
	return nil
}

// setTag writes a tag uppercased, refusing one that is not 1–3 letters or
// digits ([config.NormaliseTag]); the empty string unsets it.
func setTag(p *config.ProjectConfig, key, value string) error {
	tag, err := config.NormaliseTag(value)
	if err != nil {
		return usageErrorf("config-set: %s: %v", key, err)
	}
	p.Tag = tag
	return nil
}

// applyProjectField writes value onto the project key names through set, by
// [applyProjectWorkingDir]'s addressing.
func applyProjectField(cfg *config.Config, key, suffix, value string,
	set func(p *config.ProjectConfig, key, value string) error) error {
	id := strings.TrimSuffix(strings.TrimPrefix(key, projectKeyPrefix), suffix)
	pid, err := projectKeyFor(*cfg, id)
	if err != nil {
		return err
	}
	p := cfg.Projects[pid]
	if err := set(&p, key, value); err != nil {
		return err
	}
	cfg.Projects[pid] = p
	return nil
}

// projectKeyFor is the config file's own key for the project id names —
// matched as written first and normalised afterwards, the same two-step
// [Env.namedProject] resolves --project by. It is its own small copy rather
// than a call to namedProject because that helper re-reads the config file
// from disk, and config-set already has the one copy of it that is about to
// be written back.
func projectKeyFor(cfg config.Config, id string) (string, error) {
	if _, ok := cfg.Projects[id]; ok {
		return id, nil
	}
	want := domain.NormaliseID(id)
	for key := range cfg.Projects {
		if domain.NormaliseID(key) == want {
			return key, nil
		}
	}
	return "", fmt.Errorf("no project %s in the config file%s", id, knownProjects(cfg))
}

// parseConfigInt reads a numeric config value, treating an empty value as
// zero — the config file's own spelling of "unset" — rather than the error
// strconv.Atoi would give it.
func parseConfigInt(key, value string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, usageErrorf("config-set: %s wants a number, given %q", key, value)
	}
	return n, nil
}

// configSetMarkdown reports what was written.
func configSetMarkdown(key, value string) string {
	if value == "" {
		return fmt.Sprintf("# Config updated\n\n- %s: unset\n", key)
	}
	return fmt.Sprintf("# Config updated\n\n- %s: %s\n", key, value)
}
