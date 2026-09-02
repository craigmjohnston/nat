package cli

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// The keys config-set answers to: the settings form's own fields, plus one
// project's working directory addressed by its page ID. Nothing else in the
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

	_, err = io.WriteString(env.Out, configSetMarkdown(key, value))
	return err
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
