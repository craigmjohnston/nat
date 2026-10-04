package agent

import (
	"fmt"
	"os"
	"strings"

	"github.com/craigmjohnston/nat/internal/logging"
)

// RunPaneOption is the tmux pane option a run session's pane is tagged with —
// a project's run command started by `nat run`, which is no agent. It is an
// option of its own rather than a value of [SlicePaneOption], so the run's
// pane carries no slice tag at all: [Tmux.LiveSlices], the activity watcher
// and everything built on them pass it over as they pass over any pane that
// is not nat's, and the board and gnat's rail never read it as an agent.
const RunPaneOption = "@nat_run"

// RunSessionName is the tmux session a run is started in: the prefix, run,
// the last eight hex digits of what it runs for — the slice's page ID, or the
// project's for a global run — and its label as a slug, so the same run asked
// for twice names the same session and two runs of one slice do not.
func RunSessionName(id, label string) string {
	return SessionPrefix + "run-" + hexTail(id) + "-" + runSlug(label)
}

// runSlug is a label as a piece of a session name: lowercase letters and
// digits, every run of anything else one hyphen, none at either end — tmux
// reads a dot or a colon in a target as punctuation. A label of none of
// those slugs to "run".
func runSlug(label string) string {
	var b strings.Builder
	gap := false
	for _, r := range strings.ToLower(label) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if gap && b.Len() > 0 {
				b.WriteByte('-')
			}
			gap = false
			b.WriteRune(r)
			continue
		}
		gap = true
	}
	if b.Len() == 0 {
		return "run"
	}
	return b.String()
}

// LaunchRun starts a detached tmux session named session, with workdir as its
// working directory, running command by `sh -c` exactly as written. The pane
// is tagged with tag under [RunPaneOption] — never [SlicePaneOption] — and the
// session gets what every session nat makes gets: the launching process's
// PATH, the status bar off and tmux's mouse on.
//
// A run of the same name still live is killed by name first and started
// afresh: a re-run is a fresh start. Only a session tagged as a run is ever
// killed — one of that name that carries no run tag is not nat's to touch, and
// is refused over instead.
func (t *Tmux) LaunchRun(session, workdir, command, tag string) error {
	if err := t.endRun(session); err != nil {
		return err
	}
	carryEnv := os.Getenv("PATH") != "" && t.supportsSessionEnv()
	out, err := t.run(runLaunchArgs(session, workdir, command, carryEnv)...)
	if err != nil {
		return fmt.Errorf("start tmux session %s: %w", session, err)
	}
	pane := strings.TrimSpace(out)
	if _, err := t.run("set-option", "-p", "-t", pane, RunPaneOption, tag); err != nil {
		return fmt.Errorf("tag tmux pane %s for run %s: %w", pane, tag, err)
	}
	logging.Action("run session started", "session", session, "tag", tag, "workdir", workdir, "pane", pane)
	return nil
}

// endRun kills session where it is a run still live. The target is exact (=),
// since tmux otherwise falls back to a prefix match and would end some other
// run whose name merely starts with this one's. has-session is asked first:
// display-message -p given a target that is not there answers for the
// current client instead of failing. A session tmux does not have — or no
// server at all — is nothing to end.
func (t *Tmux) endRun(session string) error {
	if _, err := t.run("has-session", "-t", "="+session); err != nil {
		return nil
	}
	tag, err := t.run("display-message", "-p", "-t", "="+session+":", "#{"+RunPaneOption+"}")
	if err != nil {
		return fmt.Errorf("read the tag of tmux session %s: %w", session, err)
	}
	if strings.TrimSpace(tag) == "" {
		return fmt.Errorf("a tmux session named %s is running and is not a run of nat's; leaving it alone", session)
	}
	if _, err := t.run("kill-session", "-t", "="+session); err != nil {
		return fmt.Errorf("end the earlier run in %s: %w", session, err)
	}
	logging.Action("earlier run session ended", "session", session)
	return nil
}

// runLaunchArgs is the tmux argv for a run session: [bareLaunchArgs]'s shape
// with the run's own command in place of an agent.
func runLaunchArgs(session, workdir, command string, carryEnv bool) []string {
	args := []string{
		"new-session", "-d",
		"-s", session,
		"-c", workdir,
	}
	if carryEnv {
		args = append(args, "-e", "PATH="+os.Getenv("PATH"))
	}
	args = append(args,
		"-P", "-F", "#{pane_id}",
		"sh", "-c", command,
	)
	args = append(args, statusOffArgs(session)...)
	args = append(args, mouseOnArgs(session)...)
	args = append(args, inputFeatureArgs()...)
	args = append(args, hyperlinkClickArgs()...)
	return append(args, copyModeDragEndArgs()...)
}
