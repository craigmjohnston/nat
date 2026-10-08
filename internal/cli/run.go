package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/logging"
)

// runCmd starts one of a project's run commands in a detached tmux session of
// nat's own and says where. With --slice it is one of the project's
// slice-scoped runs, run in that slice's worktree ([actions.SliceRunDir]);
// without, one of its global runs, run in nat's run checkout at the latest
// origin/main ([actions.GlobalRunDir]). --label picks the run, the first of
// that scope otherwise. It writes nothing to any plan: a global run reads no
// plan at all, and a slice-scoped one only the slice.
func runCmd(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sliceRef := flags.String("slice", "", "the slice whose worktree a slice-scoped run is run in, by URL or ID; a global run without")
	label := flags.String("label", "", "the run to start, by label; the first of its scope if unset")
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("run: takes no positional arguments, given %d", len(rest))
	}

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}

	scope, runs, scopeID := "global", project.GlobalRuns(), projectID
	if *sliceRef != "" {
		scope, runs = "slice-scoped", project.SliceRuns()
	}
	run, err := pickRun(runs, scope, strings.TrimSpace(*label))
	if err != nil {
		return err
	}

	var dir string
	if *sliceRef == "" {
		dir, err = actions.GlobalRunDir(env.NewWorktrees(), env.gitFor(project), project.WorkingDir)
	} else {
		dir, scopeID, err = sliceRunDir(ctx, env, projectID, project, *sliceRef)
	}
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}

	session := agent.RunSessionName(scopeID, run.Label)
	if err := env.NewTmux().LaunchRun(session, dir, run.Command, scopeID+":"+run.Label); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	logging.Action("run started", "label", run.Label, "dir", dir, "session", session)

	doc := runJSON{Session: session, Label: run.Label, Command: run.Command, Dir: dir}
	if *asJSON {
		return writeJSON(env.Out, doc)
	}
	_, err = fmt.Fprintf(env.Out, "# Run started\n\n- Label: %s\n- Command: %s\n- Directory: %s\n- Session: %s\n",
		doc.Label, doc.Command, doc.Dir, doc.Session)
	return err
}

// sliceRunDir reads the slice --slice names and finds the worktree its run is
// run in, answering with the slice's own ID, which is what its run session is
// named after.
func sliceRunDir(ctx context.Context, env Env, projectID string, project config.ProjectConfig, ref string) (dir, sliceID string, err error) {
	id, err := pageID("run", ref)
	if err != nil {
		return "", "", err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return "", "", err
	}
	s, _, err := loadSlice(ctx, st, id)
	if err != nil {
		return "", "", err
	}
	dir, err = actions.SliceRunDir(env.NewWorktrees(), s, project)
	return dir, s.ID, err
}

// pickRun is the run label names among runs, or the first where label is
// empty — refused where there are none of the scope, or none of that label.
func pickRun(runs []config.RunCommand, scope, label string) (config.RunCommand, error) {
	if len(runs) == 0 {
		return config.RunCommand{}, fmt.Errorf("run: the project has no %s runs: add one to its config entry with config-set project.<id>.runs", scope)
	}
	if label == "" {
		return runs[0], nil
	}
	labels := make([]string, 0, len(runs))
	for _, r := range runs {
		if strings.EqualFold(r.Label, label) {
			return r, nil
		}
		labels = append(labels, r.Label)
	}
	return config.RunCommand{}, fmt.Errorf("run: no %s run is labelled %q: the project's are %s", scope, label, strings.Join(labels, ", "))
}

// runJSON is the structured form of a started run: the tmux session to attach
// to, and what was run where.
type runJSON struct {
	Session string `json:"session"`
	Label   string `json:"label"`
	Command string `json:"command"`
	Dir     string `json:"dir"`
}
