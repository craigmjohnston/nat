package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/craigmjohnston/nat/internal/logging"
)

// planPropose validates a drafted plan the same way plan-apply does, but
// writes it to a proposal file instead of Notion — there is no project for
// it to land in yet. The new-project workshop drafts a plan with the user
// and hands it here; the app's own new-project tab reads the file back and
// shows it in the rail, and it is the user's Accept that turns it into a
// real project and files it with project-create and plan-apply. Nothing
// here talks to Notion, and nothing here creates anything the user has not
// yet seen.
//
// A plan with nothing to resolve against is a narrower document than
// plan-apply's: every milestone a slice names has to be one the same
// document creates, since there is no project's own plan to resolve it
// against, and depends_on can likewise only reach a slice the document
// itself creates. The top-level dependencies list exists only to reach a
// slice already on a board — there is none yet — so it is refused outright
// rather than silently accepted and later ignored.
//
// Running plan-propose again for the same --workspace overwrites its
// proposal file: that is how a revision lands, whether the user asked for
// changes or the planning session simply drafted again.
func planPropose(_ context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("plan-propose", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	workspace := flags.String("workspace", "", "the app's own id for this new-project session (required)")
	name := flags.String("name", "", "the plan's suggested project name (required)")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) > 1 {
		return usageErrorf("plan-propose: want at most one plan file, given %d", len(rest))
	}

	// The workspace id is the app's own, with no fallback: it is what tells the
	// app which new-project tab a proposal belongs to, and a command that
	// guessed at it would file the proposal somewhere the app is not looking.
	ws := strings.TrimSpace(*workspace)
	if ws == "" {
		return usageErrorf("plan-propose: no --workspace given: the app's own id for this session, with no fallback")
	}
	suggested := strings.TrimSpace(*name)
	if suggested == "" {
		return usageErrorf("plan-propose: no --name given: the plan's suggested project name")
	}

	// The plan is read and validated before anything is written, exactly as
	// plan-apply's is — a proposal half-written is worse than no proposal at
	// all, since the app would show it as though the workshop had settled.
	source := stdinRef
	if len(rest) == 1 {
		source = rest[0]
	}
	p, err := readPlan(source, env.In)
	if err != nil {
		return err
	}
	if len(p.Dependencies) > 0 {
		return fmt.Errorf("plan-propose: the plan holds a top-level `dependencies` list, which reaches a " +
			"slice already on a project's board — there is no project yet for it to reach. Put what a new " +
			"slice waits on in its own `depends_on` instead")
	}
	// No existing project means no existing milestones or slices to resolve
	// against: every milestone a slice names, and everything it depends on,
	// has to be something this same document creates.
	if _, err := validatePlan(p, nil, nil); err != nil {
		return err
	}

	path, err := proposalPath(ws)
	if err != nil {
		return fmt.Errorf("resolve the proposal file: %w", err)
	}
	doc := proposalDoc{Workspace: ws, Name: suggested, Plan: p}
	data, err := marshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the proposal: %w", err)
	}
	if err := writeProposalFile(path, data); err != nil {
		return fmt.Errorf("write the proposal file: %w", err)
	}
	// The app polls the same marker every other write does, so it notices a
	// fresh or revised proposal within a second rather than on its own timer.
	env.nudged()
	logging.Action("plan proposed", "workspace", ws, "milestones", len(p.Milestones), "slices", len(p.Slices))

	if *asJSON {
		return writeJSON(env.Out, doc)
	}
	_, err = fmt.Fprintf(env.Out, "Proposed %s to workspace %s as %q.\n", counts(len(p.Milestones), len(p.Slices)), ws, suggested)
	return err
}

// proposalDoc is what plan-propose writes to the proposal file: the plan
// exactly as validated, and the name the app shows in the rail until the
// user accepts it or edits it first. Workspace rides along in the file
// itself as well as in its name, so a reader handed the file on its own
// still knows which session it came from.
type proposalDoc struct {
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	Plan      plan   `json:"plan"`
}

// proposalsDirName is the proposal files' own directory under nat's state
// directory — the same one [logging.Dir] resolves and [internal/nudge]
// already shares the marker file in.
const proposalsDirName = "proposals"

// stateDir resolves nat's state directory, held as a variable so a test can
// point it at a directory of its own rather than the real home — the same
// seam [internal/nudge] uses for the marker file that lives beside these.
var stateDir = logging.Dir

// marshalIndent is held as a var so a test can stub a marshal failure, the
// same seam internal/config's own Save uses for the same reason.
var marshalIndent = json.MarshalIndent

// proposalPath is the file one workspace's proposal lives in: nat's state
// directory, named by the workspace id the app gave plan-propose.
func proposalPath(workspaceID string) (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, proposalsDirName, workspaceID+".json"), nil
}

// writeProposalFile writes data to path atomically: a temp file beside it,
// written whole in one call, then renamed into place. The app polls this
// file's existence and content on its own, with no lock of its own to take,
// so it must never be able to read one that is half-written — a rename on
// the same filesystem is what makes the swap instantaneous from its side.
func writeProposalFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create the proposals dir: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write the temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		// Best-effort tidy-up: the rename already failed, and a stray temp file
		// is nothing worse than what a plain os.WriteFile leaves after a crash.
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename the temp file into place: %w", err)
	}
	return nil
}
