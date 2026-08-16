// Package agent launches Claude Code agents for slices: it builds the prompt a
// fresh agent session starts from, and manages the detached tmux sessions those
// agents run in. One live tmux session per slice, labelled after the slice's
// page and tagged with its full ID.
package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// TmuxBinary is the name of the tmux binary, looked up on PATH.
const TmuxBinary = "tmux"

// SessionPrefix namespaces our sessions inside the user's tmux server, so a
// session list can be told apart from whatever else they have running.
const SessionPrefix = "nat-"

// TUISession is the session the TUI hosts itself in. It is a fixed name, not a
// per-run one, so a second launch attaches to the window already running rather
// than standing up a rival next to it.
const TUISession = SessionPrefix + "tui"

// sessionIDLen is how much of a slice's page ID goes into its session name.
// Eight hex digits is what tmux can show without truncating the status line.
const sessionIDLen = 8

// SlicePaneOption is the tmux pane option an agent's pane is tagged with, and
// holds the full page ID of the slice that agent is working. It, not the
// session name, is what a running agent is identified by: page IDs in one
// Notion workspace share a long prefix, so any short slug of one is a name
// several slices answer to. Tagging the pane also survives the pane being
// moved into another session.
const SlicePaneOption = "@nat_slice"

// LabelPaneOption is the tmux pane option carrying the human label the status
// bar shows a pane by — the slice's short session ID, or "plan" for the
// planning agent. It is set beside [SlicePaneOption] rather than derived from
// it in the status format, because tmux's formats can compare and choose but
// not slice a string down to its last eight digits.
//
// The board's own pane carries neither option, which is how the bar tells it
// from an agent's.
const LabelPaneOption = "@nat_label"

// PlanSentinel is the value [SlicePaneOption] carries on the planning agent's
// pane, in place of a slice page ID: the planning agent works the plan itself
// and has no slice to be tagged with. It cannot collide with a real slice —
// page IDs are hex, and "plan" is not — so everything that reads the tag
// (LiveSlices, ShowPane, the break-outs) handles the planning pane the same
// way it handles a slice's.
const PlanSentinel = "plan"

// PlanSession is the tmux session the planning agent launches in. There is
// only ever one planning agent, so unlike the slice sessions it needs no ID in
// its name.
const PlanSession = SessionPrefix + PlanSentinel

// PaneEnv is set by tmux in every pane it runs, to the pane's own ID. It is how
// the TUI finds the pane it is drawing in, which is the one an agent's pane is
// joined beside.
const PaneEnv = "TMUX_PANE"

// tmuxTimeout caps how long we wait for a tmux command. These are local calls
// to a socket in /tmp, so anything slower than this is a hang.
const tmuxTimeout = 10 * time.Second

// placeholderCommand is what the throwaway pane of a freshly made session runs
// while the agent's pane is moved in beside it. A pane cannot be broken out
// into a session that does not exist yet, and a session cannot be made without
// a pane — so one is made, used as a destination, and killed. It sleeps rather
// than starting a shell: there are no rc files worth running for the moment it
// is alive, and a sleep that outlives its kill dies on its own.
const placeholderCommand = "sleep 3600"

// Runner executes a command and returns its standard output. It exists so the
// tmux calls can be faked in tests; there is no mock mode for a subprocess.
type Runner interface {
	Run(name string, args ...string) (string, error)
}

// ExitError reports a command that ran to completion and exited non-zero. The
// exit code is carried separately because tmux uses it to distinguish "no
// server is running" (1) from a genuine failure.
type ExitError struct {
	Code   int
	Stderr string
}

// Error describes the exit code, with the command's own stderr when it wrote
// any — it explains the failure better than we can guess at it.
func (e *ExitError) Error() string {
	if s := strings.TrimSpace(e.Stderr); s != "" {
		return fmt.Sprintf("exit status %d: %s", e.Code, s)
	}
	return fmt.Sprintf("exit status %d", e.Code)
}

// ExecRunner is a Runner backed by real subprocesses.
type ExecRunner struct{}

var _ Runner = ExecRunner{}

// Run executes name with args, returning its standard output. A non-zero exit
// becomes an *ExitError; anything else (a missing binary, say) is returned as
// it came back from os/exec.
func (ExecRunner) Run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return stdout.String(), &ExitError{Code: exitErr.ExitCode(), Stderr: stderr.String()}
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

// Tmux drives the tmux sessions agents run in.
type Tmux struct {
	runner Runner
}

// NewTmux returns a Tmux driving the real tmux binary on PATH.
func NewTmux() *Tmux { return &Tmux{runner: ExecRunner{}} }

// NewTmuxWithRunner returns a Tmux that executes through r.
func NewTmuxWithRunner(r Runner) *Tmux { return &Tmux{runner: r} }

// SessionName is the tmux session name for a slice's page ID: the prefix plus
// the last eight hex digits of the ID, with the UUID dashes dropped. IDs are
// already hex, but non-hex characters are skipped rather than trusted, so a
// surprising ID cannot produce a name tmux would reject.
//
// The tail rather than the head, because page IDs made in one workspace share
// a long leading prefix: taken from the front, every slice of a project names
// the same session, and the second launch is refused as a duplicate. The name
// is only a human label — what a session belongs to is read from
// [SlicePaneOption] — but it still has to be one tmux will accept twice.
func SessionName(slicePageID string) string {
	// The planning agent's tag is not a page ID at all: hex-filtering it would
	// name a session tmux could confuse with a slice's.
	if slicePageID == PlanSentinel {
		return PlanSession
	}
	var b strings.Builder
	for _, r := range strings.ToLower(slicePageID) {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			b.WriteRune(r)
		}
	}
	hex := b.String()
	if len(hex) > sessionIDLen {
		hex = hex[len(hex)-sessionIDLen:]
	}
	return SessionPrefix + hex
}

// PaneLabel is what the status bar calls the pane an agent for slicePageID
// runs in: its session name without the shared prefix, so the label reads as
// the eight hex digits the user attaches that session by ("plan" for the
// planning agent). The prefix is dropped because the bar is nat's own — every
// pane on it would carry it.
func PaneLabel(slicePageID string) string {
	return strings.TrimPrefix(SessionName(slicePageID), SessionPrefix)
}

// LiveSlices maps the page ID of every slice with an agent running to the tmux
// session that agent's pane is currently in — the ID to mark the slice by, and
// the session to attach to it with. Panes that are not ours carry no slice tag
// and are left out.
//
// It is a server-wide scan of the panes rather than a look at the session
// names, because the tag is the identity: a pane the user has moved or renamed
// their way — or that the board has joined in beside itself — is still the
// agent for its slice.
func (t *Tmux) LiveSlices() (map[string]string, error) {
	panes, err := t.panes()
	if err != nil {
		return nil, err
	}

	live := map[string]string{}
	for _, p := range panes {
		if p.slice == "" {
			continue
		}
		// A slice with two panes tagged for it should not happen, and if it
		// does the first one found is as good an answer as the last.
		if _, seen := live[p.slice]; !seen {
			live[p.slice] = p.session
		}
	}
	return live, nil
}

// pane is one of the tmux server's panes: which slice it is the agent for, if
// any, where it currently is, and what the status bar needs to draw a section
// for it — its index within the window, how wide it is, and its label.
type pane struct {
	slice   string
	id      string
	session string
	window  string
	index   string
	width   int
	label   string
}

// statusContent is what the bar draws in a pane's section, as the tmux format
// that yields it: the label an agent's pane was tagged with, or the pane's own
// title for the pane with no tag at all — the one nat itself is drawing in.
//
// The board's section is its title because that is how nat's messages reach the
// bar: the app writes the line the in-TUI status bar used to draw as its
// terminal title (OSC 2), which inside tmux is the pane's, and tmux redraws the
// status line itself whenever a title changes — so the bar follows the app with
// no tmux command sent per keystroke. tmux does not re-expand what it
// substitutes, so a title carrying `#{...}`, `#[...]` or a `%` reaches the bar
// as the text it is.
//
// A pane an earlier run launched carries the slice tag but no label, and would
// otherwise read as the board; it falls back to the bare word, which is at
// least true of it. The agents outlive the run that started them, so this is
// the ordinary state of things for a while after an upgrade, not a corner.
func (p pane) statusContent() string {
	switch {
	case p.label != "":
		return literalContent(p.label)
	case p.slice == "":
		return paneTitleContent
	default:
		return literalContent(agentLabel)
	}
}

// panes lists every pane on the server. Panes that are not ours carry no slice
// tag and come back with an empty one rather than being dropped — the board's
// own pane is in here too, and is found by its ID.
//
// tmux exits 1 when no server is running at all, which is the ordinary state
// before the first agent launches — that reads as no panes, not an error.
func (t *Tmux) panes() ([]pane, error) {
	out, err := t.runner.Run(TmuxBinary, "list-panes", "-a", "-F", listPanesFormat())
	if err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) && exitErr.Code == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("list tmux panes: %w", err)
	}

	var panes []pane
	for _, line := range strings.Split(out, "\n") {
		// Trimmed on the right only: an untagged pane's line starts with the
		// empty slice field, and trimming that away would shift every field
		// along by one.
		fields := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(fields) != listPanesFields {
			continue
		}
		// A width tmux could not give us is no reason to drop the pane: it is
		// still an agent to be found and moved, and a zero-width section is
		// simply one the bar has nothing to draw.
		width, _ := strconv.Atoi(fields[5])
		panes = append(panes, pane{slice: fields[0], id: fields[1], session: fields[2], window: fields[3],
			index: fields[4], width: width, label: fields[6]})
	}
	return panes, nil
}

// listPanesFields is how many fields [listPanesFormat] asks for; a line with
// any other number of them is not one tmux wrote for us.
const listPanesFields = 7

// listPanesFormat is what [Tmux.panes] asks tmux to print for each pane: the
// slice tag, then the pane, the session it is in and the window within it, and
// then what the status bar draws a section for it from — its index in the
// window, its width, and its label. Tabs separate them because a session name
// can hold anything but that.
func listPanesFormat() string {
	return fmt.Sprintf("#{%s}\t#{pane_id}\t#{session_name}\t#{window_id}\t#{pane_index}\t#{pane_width}\t#{%s}",
		SlicePaneOption, LabelPaneOption)
}

// HostPane is the tmux pane this process is drawing in, or "" when it is not
// inside tmux at all — in which case there is no window to show an agent beside
// and the caller falls back to attaching full-screen.
func HostPane() string { return os.Getenv(PaneEnv) }

// ShowPane shows the agent working sliceID beside the pane the board is drawing
// in, or — when it is already there — sends it back to a session of its own. It
// reports which of the two happened.
//
// The agent is found by its slice tag rather than by session, because a pane
// that has been joined here no longer has a session of its own: the session it
// was launched in is gone the moment its only pane leaves.
func (t *Tmux) ShowPane(sliceID, hostPane string, percent int) (bool, error) {
	panes, err := t.panes()
	if err != nil {
		return false, err
	}
	agentPane, ok := find(panes, func(p pane) bool { return p.slice == sliceID })
	if !ok {
		return false, fmt.Errorf("no agent pane is tagged for slice %s", sliceID)
	}
	host, ok := find(panes, func(p pane) bool { return p.id == hostPane })
	if !ok {
		return false, fmt.Errorf("the board's own pane %s is not in tmux", hostPane)
	}

	if agentPane.window == host.window {
		if err := t.breakOut(agentPane.id, SessionName(sliceID)); err != nil {
			return false, err
		}
		t.refreshAfterLayout(hostPane)
		return false, nil
	}
	if err := t.join(agentPane.id, host, percent); err != nil {
		return true, err
	}
	t.refreshAfterLayout(hostPane)
	return true, nil
}

// find returns the first pane matching want.
func find(panes []pane, want func(pane) bool) (pane, bool) {
	for _, p := range panes {
		if want(p) {
			return p, true
		}
	}
	return pane{}, false
}

// join moves an agent's pane in beside the board, giving it percent of the
// width. The board keeps the keyboard (`-d`), so the plan stays usable with the
// agent working next to it; the mouse is what moves between them.
//
// Mouse mode is turned on for the board's own session rather than globally: it
// is a session option, so whatever the user has set for their own sessions is
// left as they set it.
func (t *Tmux) join(paneID string, host pane, percent int) error {
	if _, err := t.runner.Run(TmuxBinary, "set-option", "-t", host.session, "mouse", "on"); err != nil {
		return fmt.Errorf("enable the mouse in %s: %w", host.session, err)
	}
	if _, err := t.runner.Run(TmuxBinary, "join-pane", "-h", "-d",
		"-l", fmt.Sprintf("%d%%", percent), "-s", paneID, "-t", host.id); err != nil {
		return fmt.Errorf("join pane %s beside the board: %w", paneID, err)
	}
	logging.Action("agent pane joined beside the board", "pane", paneID, "host", host.id, "percent", percent)
	return nil
}

// breakOut sends a joined pane back to a session of its own, named after its
// slice the way it was when it launched — so it is attachable from any terminal
// again, and so the board it was sharing a window with goes back to full width.
//
// tmux has no way to break a pane straight out into a new session, so the
// session is made around a placeholder pane which is then killed. A join that
// fails takes the placeholder session with it: leaving one behind would be a
// session named for a slice whose agent is somewhere else entirely.
func (t *Tmux) breakOut(paneID, session string) error {
	args := append([]string{"new-session", "-d",
		"-s", session, "-P", "-F", "#{pane_id}", placeholderCommand}, statusOffArgs(session)...)
	args = append(args, mouseOnArgs(session)...)
	out, err := t.runner.Run(TmuxBinary, args...)
	if err != nil {
		return fmt.Errorf("make session %s for pane %s: %w", session, paneID, err)
	}
	placeholder := strings.TrimSpace(out)

	if _, err := t.runner.Run(TmuxBinary, "join-pane", "-s", paneID, "-t", session+":"); err != nil {
		logging.Error("killing the session made for a pane that would not move", "session", session, "pane", paneID)
		_, _ = t.runner.Run(TmuxBinary, "kill-session", "-t", session)
		return fmt.Errorf("move pane %s into %s: %w", paneID, session, err)
	}
	if _, err := t.runner.Run(TmuxBinary, "kill-pane", "-t", placeholder); err != nil {
		return fmt.Errorf("clear the placeholder pane %s in %s: %w", placeholder, session, err)
	}
	logging.Action("agent pane moved to a session of its own", "session", session, "pane", paneID)
	return nil
}

// BreakOutJoined sends every agent pane sharing hostPane's window back to a
// session of its own, reporting how many it moved. It is what the board owes
// its agents on the way out: a joined pane belongs to the board's window, so a
// window that closes with one still in it kills the agent working there.
//
// A host pane that is not on the server — the board is not in tmux at all, or
// its pane has already gone — means there is no window to empty, which is
// nothing to do rather than a failure.
func (t *Tmux) BreakOutJoined(hostPane string) (int, error) {
	panes, err := t.panes()
	if err != nil {
		return 0, err
	}
	host, ok := find(panes, func(p pane) bool { return p.id == hostPane })
	if !ok {
		return 0, nil
	}
	return t.breakOutAll(panes, func(p pane) bool { return p.window == host.window })
}

// ReclaimStrays sends the agent panes a previous run left joined back to
// sessions of their own, reporting how many it moved. A run that died without
// getting to [Tmux.BreakOutJoined] — a panic, a kill — leaves its agents in a
// window that will take them with it when it closes, and a board starting up
// has joined nothing itself yet: every agent pane in [TUISession], or in the
// window the new board is starting in, is one of those strays.
func (t *Tmux) ReclaimStrays(hostPane string) (int, error) {
	panes, err := t.panes()
	if err != nil {
		return 0, err
	}
	host, hosted := find(panes, func(p pane) bool { return p.id == hostPane })
	moved, err := t.breakOutAll(panes, func(p pane) bool {
		return p.session == TUISession || (hosted && p.window == host.window)
	})
	// A stray taken out of the board's window is a section the bar no longer
	// has a pane for; one taken out of another window is not, but the bar is
	// rebuilt from what is there either way.
	if moved > 0 {
		t.refreshAfterLayout(hostPane)
	}
	return moved, err
}

// breakOutAll sends every agent pane matching want back to a session of its
// own, against the panes as they were listed.
//
// A pane that will not move does not stop the rest: each one left where it is
// is an agent that dies with its window, so the failures are gathered and
// reported together rather than abandoning the panes behind the first of them.
func (t *Tmux) breakOutAll(panes []pane, want func(pane) bool) (int, error) {
	moved := 0
	var errs []error
	for _, p := range panes {
		if p.slice == "" || !want(p) {
			continue
		}
		if err := t.breakOut(p.id, SessionName(p.slice)); err != nil {
			errs = append(errs, err)
			continue
		}
		moved++
	}
	return moved, errors.Join(errs...)
}

// Launch starts a detached tmux session named session, with workdir as its
// working directory, running an agent seeded with the prompt in promptFile for
// the slice with page ID sliceID.
//
// The pane the session starts in is tagged with sliceID, which is what
// [Tmux.LiveSlices] reads the running agents back out of, and with the label
// the board's status bar shows it by. A session whose pane
// could not be tagged is left running — its agent is already working — but the
// failure is reported, because until it is tagged nothing will find it again.
func (t *Tmux) Launch(session, workdir, promptFile, sliceID string) error {
	out, err := t.runner.Run(TmuxBinary, LaunchArgs(session, workdir, promptFile)...)
	if err != nil {
		return fmt.Errorf("launch tmux session %s: %w", session, err)
	}
	pane := strings.TrimSpace(out)
	if _, err := t.runner.Run(TmuxBinary,
		"set-option", "-p", "-t", pane, SlicePaneOption, sliceID,
		";", "set-option", "-p", "-t", pane, LabelPaneOption, PaneLabel(sliceID)); err != nil {
		return fmt.Errorf("tag tmux pane %s for slice %s: %w", pane, sliceID, err)
	}
	logging.Action("agent launched", "session", session, "slice", sliceID, "workdir", workdir, "pane", pane)
	return nil
}

// LaunchArgs is the tmux argv for launching an agent session. Assembling it in
// one place keeps the shape of an agent invocation — the binary, its flags, how
// the prompt reaches it — in a single spot, ready for per-project agent
// definitions to vary it later.
//
// The prompt is passed as a positional argument read back from the file rather
// than inlined, so a long prompt cannot run into the argv size limit. The new
// session prints its pane's ID, which is the handle the slice tag goes on:
// pane IDs are unique for the life of the server, where a name is whatever it
// has last been set to.
func LaunchArgs(session, workdir, promptFile string) []string {
	args := append([]string{
		"new-session", "-d",
		"-s", session,
		"-c", workdir,
		"-P", "-F", "#{pane_id}",
		"sh", "-c", agentCommand(promptFile),
	}, statusOffArgs(session)...)
	args = append(args, mouseOnArgs(session)...)
	args = append(args, inputFeatureArgs()...)
	return append(args, hyperlinkClickArgs()...)
}

// HostArgs is the tmux argv that runs binary as the TUI, inside [TUISession].
//
// `-A` is what makes a second launch attach to the session already there
// instead of failing on the duplicate name; the command is only used when the
// session is being created, so the attaching launch cannot start a second copy
// of the binary. The command is run directly rather than through a shell, so a
// path with spaces in it needs no quoting.
//
// [TUISession] only ever exists because a launch outside tmux made it — a
// launch inside tmux never gets here — so styling its status bar and borders
// touches no session the user was already in.
func HostArgs(binary string) []string {
	args := append([]string{"new-session", "-A", "-s", TUISession, binary}, statusBarArgs(TUISession)...)
	args = append(args, inputFeatureArgs()...)
	return append(args, hyperlinkClickArgs()...)
}

// statusOffArgs is the command that hides the tmux status bar in an agent's own
// session, chained onto the new-session that makes it so the bar is never up
// even for a moment. tmux's stock bar under an agent is noise beside the
// agent's output, and says nothing the agent's own session does not. It is a
// per-session option on a named session, so the sessions the user was already
// running keep their bars.
//
// The board's session gets a bar of nat's own instead — see [statusBarArgs] —
// because that is the one window an agent's pane is ever joined into, and so
// the one place the bar has something to say.
//
// The lone ";" is tmux's own command separator, read from argv the way `\;` is
// from a shell: everything before it belongs to new-session, and the set-option
// runs once the session is there.
func statusOffArgs(session string) []string {
	return []string{";", "set-option", "-t", session, "status", "off"}
}

// nat's palette as tmux takes it: the colours the TUI's dark theme draws with,
// spelled out because tmux has no notion of a theme that follows the terminal.
const (
	statusBarBG     = "#313244" // surface0
	statusBarFG     = "#7f849c" // overlay1
	statusBarAccent = "#cba6f7" // mauve
	statusBarOnFill = "#11111b" // crust, for text on the accent
	paneBorderFG    = "#45475a" // surface1
)

// agentLabel is the word the bar falls back to for an agent's pane that carries
// no label of its own: one an earlier run launched has the slice tag but no
// label yet.
const agentLabel = "agent"

// paneTitleContent draws a pane's own title, and literalContent a fixed word.
// The word goes through `#{l:...}` because the body of a padding is read as a
// format: a bare word in there is a variable name, and expands to nothing.
const paneTitleContent = "#{pane_title}"

func literalContent(label string) string { return "#{l:" + label + "}" }

// The two styles a section is drawn in, picked between by the focus. Attributes
// are separated by spaces rather than commas: a comma inside `#{?...}` is where
// tmux splits its branches, and would cut a style in half.
const (
	activeSectionStyle   = "#[fg=" + statusBarOnFill + " bg=" + statusBarAccent + " bold]"
	inactiveSectionStyle = "#[fg=" + statusBarFG + " bg=" + statusBarBG + " nobold]"
)

// statusSeparator is the one column between two sections — the pane border
// standing between the panes above them, drawn in the bar's own style so it
// reads as the same seam. It is what makes the sections total the window width:
// tmux counts the border column in the window's width but in no pane's.
const statusSeparator = "#[default] "

// statusFormatOption is the status line's one row, which is the whole bar: nat
// sets no status-left or status-right, so row zero is all there is.
const statusFormatOption = "status-format[0]"

// statusSection is one pane as the bar draws it: the pane's index within the
// window, the tmux format for what it has to say, and how wide the pane is.
type statusSection struct {
	index   string
	content string
	width   int
}

// sectionsFor turns the panes of one window into the sections of its bar, in
// the order tmux listed them — which for the board's window, split left to
// right, is the order they sit in.
func sectionsFor(panes []pane) []statusSection {
	sections := make([]statusSection, 0, len(panes))
	for _, p := range panes {
		sections = append(sections, statusSection{index: p.index, content: p.statusContent(), width: p.width})
	}
	return sections
}

// buildStatusFormat is the whole status line for a window's panes: one section
// per pane, each exactly as wide as the pane above it, so the bar reads as a
// footer to the frames rather than a row of chips of its own.
//
// The widths are baked in because tmux will not expand a format inside a
// padding width — `#{p#{pane_width}:...}` pads to nothing — so the bar has to
// be rebuilt whenever the layout changes. The focus, though, is left to tmux:
// each section carries a condition comparing the window's focused pane against
// the index it was built for, so moving the focus redraws the bar with no
// command sent and nothing to poll.
//
// The comparison is on the pane's index rather than its ID because tmux passes
// a status format through strftime, which eats the `%` an ID begins with — and
// eats it once more for every `#{?...}` the comparison sits inside, so no
// amount of escaping survives reliably. An index is current for exactly as long
// as the layout it was read from, which is exactly how long this format lives.
//
// A window with only the board in it renders as one full-width section, always
// in the accent: the sole pane of a window is the focused one, so the condition
// would have nothing to decide.
func buildStatusFormat(sections []statusSection) string {
	var b strings.Builder
	for i, s := range sections {
		if i > 0 {
			b.WriteString(statusSeparator)
		}
		if len(sections) == 1 {
			b.WriteString(activeSectionStyle)
		} else {
			b.WriteString("#{?#{==:#{pane_index}," + s.index + "}," +
				activeSectionStyle + "," + inactiveSectionStyle + "}")
		}
		// Cut before padding: a section holding more than its pane is wide —
		// the board's title carrying a long error — would otherwise push every
		// section after it out of line with the pane above it.
		fmt.Fprintf(&b, "#{=%d;p%d:#{l: }%s}", s.width, s.width, s.content)
	}
	b.WriteString("#[default]")
	return b.String()
}

// initialStatusWidth is how wide the board's one section is drawn before the
// first rebuild, when the window it will sit in does not exist yet and its
// width cannot be asked for. It is wider than any terminal, and tmux clips the
// status line at the window's edge, so the section fills the row whatever that
// turns out to be.
const initialStatusWidth = 1024

// initialStatusFormat is the bar a freshly made board session starts with: the
// one section for the pane it starts with. The first layout change — the first
// resize, which the TUI takes as it starts — replaces it with one built from
// the panes actually there.
func initialStatusFormat() string {
	return buildStatusFormat([]statusSection{{content: paneTitleContent, width: initialStatusWidth}})
}

// RefreshStatusBar redraws the status bar of the session hostPane's window
// belongs to, with one section per pane as the panes now are. It is what every
// layout change owes the bar: a section's width is baked into the format, so a
// pane joined, broken out or resized leaves the old sections lined up against
// nothing.
//
// A host pane that is not on the server — the board is not in tmux at all, or
// its pane has already gone — means there is no bar of ours to redraw, which is
// nothing to do rather than a failure.
func (t *Tmux) RefreshStatusBar(hostPane string) error {
	panes, err := t.panes()
	if err != nil {
		return err
	}
	host, ok := find(panes, func(p pane) bool { return p.id == hostPane })
	if !ok {
		return nil
	}
	var window []pane
	for _, p := range panes {
		if p.window == host.window {
			window = append(window, p)
		}
	}
	format := buildStatusFormat(sectionsFor(window))
	if _, err := t.runner.Run(TmuxBinary, "set-option", "-t", host.session, statusFormatOption, format); err != nil {
		return fmt.Errorf("redraw the status bar of %s: %w", host.session, err)
	}
	return nil
}

// refreshAfterLayout redraws the bar for a layout that has just changed, and
// keeps a failure to itself. The panes have already moved by then, and a bar
// still showing the sections of a moment ago is not worth failing the move the
// user asked for over — it is logged and corrected by the next change.
func (t *Tmux) refreshAfterLayout(hostPane string) {
	if err := t.RefreshStatusBar(hostPane); err != nil {
		logging.Error("could not redraw the board's status bar", "error", err)
	}
}

// statusBarArgs is the command chain that gives the board's session a status
// bar of nat's own and takes the highlight off the pane borders.
//
// The bar is there to say which pane the keyboard is talking to, which is the
// question the border highlight used to answer badly: tmux's stock active
// border is a green edge that reads as an alert, and against nat's own frames
// it is one line of chrome too many. Both border styles are set to the same
// neutral grey, so the split shows as a seam and the bar alone says where the
// focus is.
//
// It sits at the bottom, under the frames its sections are the width of, so it
// reads as their footer rather than a row of its own.
//
// Per session, like the bar we hide in an agent's own — except the border
// styles, which tmux keeps per window; targeting the session sets them on the
// window it currently has, which for the board's session is the window it was
// made with and never leaves.
func statusBarArgs(session string) []string {
	var args []string
	for _, opt := range [][2]string{
		{"status", "on"},
		{"status-position", "bottom"},
		{"status-style", "fg=" + statusBarFG + " bg=" + statusBarBG},
		{statusFormatOption, initialStatusFormat()},
		{"pane-border-style", "fg=" + paneBorderFG},
		{"pane-active-border-style", "fg=" + paneBorderFG},
	} {
		args = append(args, ";", "set-option", "-t", session, opt[0], opt[1])
	}
	return args
}

// mouseOnArgs is the command chained onto an agent session's creation that
// turns tmux's own mouse handling on for it. The hyperlink click binding only
// fires in a session where tmux holds the mouse: with the option off, tmux
// hands mouse reporting straight through to the agent, and a click on a link
// reaches Claude Code instead of the binding. Session-scoped like the status
// bar, so the user's own sessions keep whatever they had; the board's session
// gets its mouse at join time, where it always has.
func mouseOnArgs(session string) []string {
	return []string{";", "set-option", "-t", session, "mouse", "on"}
}

// hyperlinkUnderMouse is the tmux format condition that is true when the cell
// under the mouse carries an OSC 8 hyperlink. The comparison against empty
// rather than bare truthiness, so a hyperlink whose target is the one string
// tmux reads as false ("0") still counts.
const hyperlinkUnderMouse = "#{!=:#{mouse_hyperlink},}"

// stockClick and stockCtrlClick are tmux's own default root-table bindings for
// the two mouse events, kept as the fall-through of the hyperlink bindings: a
// key binding is server-wide — tmux has no narrower scope for one — so away
// from a link every click behaves exactly as it did before, in the user's own
// sessions too.
const (
	stockClick     = "select-pane -t = ; send-keys -M"
	stockCtrlClick = "swap-pane -s @"
)

// urlOpener is the command that hands a URL to the desktop on this platform.
func urlOpener() string { return urlOpenerFor(runtime.GOOS) }

// urlOpenerFor picks the opener for a GOOS, split out so both answers are
// reachable from a test run on either platform.
func urlOpenerFor(goos string) string {
	if goos == "darwin" {
		return "open"
	}
	return "xdg-open"
}

// hyperlinkClickArgs is the command chain that makes a link in a pane open on
// click. tmux ends up holding the mouse everywhere an agent's links show — its
// own mouse option is on in the sessions nat makes and in the board's at join,
// and Claude Code asks for mouse reporting besides — so no click ever reaches
// the outer terminal's own link handling; the OSC 8 hyperlink reaches it fine
// (that is what the terminal-features entry above arranges), but the gesture
// to open it never does. So tmux is taught the gesture instead: it knows the
// hyperlink under the mouse, and a click on one hands the URL to the desktop.
//
// A plain click also keeps its stock behaviour — focusing the pane, reaching
// the program inside — so on a link it is that plus the open; ctrl-click's
// stock meaning (swapping panes) is suppressed on a link, because a swap is
// never what a click on a URL wants. Bindings are idempotent to repeat, so
// chaining them onto every launch costs nothing.
func hyperlinkClickArgs() []string {
	open := fmt.Sprintf("run-shell -b '%s #{q:mouse_hyperlink}'", urlOpener())
	return []string{
		";", "bind-key", "-T", "root", "MouseDown1Pane",
		"if-shell", "-F", "-t", "=", hyperlinkUnderMouse,
		open + " ; " + stockClick, stockClick,
		";", "bind-key", "-T", "root", "C-MouseDown1Pane",
		"if-shell", "-F", "-t", "=", hyperlinkUnderMouse,
		open, stockCtrlClick,
	}
}

// terminalFeatures is the terminal-features entry chained onto our session
// creation: every outer terminal ("*") is told to support extended keys, so
// tmux asks it for them and shift+enter arrives distinguishable from enter,
// and OSC 8 hyperlinks, so a URL an agent prints stays a link the terminal can
// open even with the mouse held by tmux. A terminal that truly supports
// neither degrades to what it did before; the claim costs it nothing.
const terminalFeatures = "*:extkeys:hyperlinks"

// inputFeatureArgs is the command chain that lets an agent's pane receive
// modified keys and emit clickable links: extended-keys forwards keys like
// shift+enter to a program that asks for them (Claude Code's composer does),
// and the terminal-features entry covers both directions of the outer
// terminal. Both are server options — tmux has no narrower scope for them —
// so they are chained onto the new-session commands rather than set per
// session; appending to terminal-features (-a) leaves whatever entries the
// user has set, at the cost of a repeated identical entry per launch, which
// tmux reads happily.
func inputFeatureArgs() []string {
	return []string{
		";", "set-option", "-s", "extended-keys", "on",
		";", "set-option", "-s", "-a", "terminal-features", terminalFeatures,
	}
}

// agentCommand is the shell command the session runs: start Claude Code with
// the contents of promptFile as its prompt.
func agentCommand(promptFile string) string {
	return fmt.Sprintf(`claude "$(cat %s)"`, shellQuote(promptFile))
}

// SessionEnv is set by tmux in every pane it runs, to the socket and session
// the pane belongs to. tmux refuses to attach a client from inside one of its
// own panes when it is non-empty, so an attach that runs under the board — the
// full-screen one, and the embedded viewer's — has to be launched without it.
const SessionEnv = "TMUX"

// ViewerFeatures is the client-features list every attach declares with tmux's
// top-level -T: 256 colours and direct RGB, extended keys so shift+enter still
// arrives distinguishable, and focus reporting. It deliberately omits sync and
// sixel: the embedded viewer's emulator speaks neither, and a client that
// claims them gets sequences it would only have to discard. The kitty and
// extended-keys negotiation between claude and tmux is untouched by this —
// extended-keys is already on server-wide from [LaunchArgs].
const ViewerFeatures = "256,RGB,extkeys,focus"

// attachArgs is the tmux argv shared by both attaches. -T is a top-level client
// flag, so it goes before the command, not after it.
func attachArgs(session string) []string {
	return []string{"-T", ViewerFeatures, "attach-session", "-t", session}
}

// scrubEnv is env with every entry naming one of names removed. Both attaches
// need the pane variables gone — tmux reads them to decide it is being nested —
// and the viewer's needs TERM gone so it can set its own.
func scrubEnv(env []string, names ...string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, found := strings.Cut(entry, "=")
		if found && slices.Contains(names, name) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// AttachCmd is the command that attaches the terminal to a running session,
// for the caller to hand to tea.ExecProcess.
//
// It usually runs from inside [TUISession], so the pane variables are scrubbed
// or tmux refuses the nested attach. The caller's TERM is kept: the terminal on
// the other end of this one is the user's real one, which knows itself better
// than we do.
func AttachCmd(session string) *exec.Cmd {
	cmd := exec.Command(TmuxBinary, attachArgs(session)...)
	cmd.Env = scrubEnv(os.Environ(), SessionEnv, PaneEnv)
	return cmd
}

// AttachCmd is the command attaching to a session, as a method so a caller
// that already holds a Tmux can launch and attach through the one thing.
func (t *Tmux) AttachCmd(session string) *exec.Cmd { return AttachCmd(session) }

// AttachClientCmd is the command the embedded viewer runs on a PTY of its own:
// the same attach, but with TERM replaced by what the viewer's emulator
// actually is, since the client on the far end of that PTY is the emulator and
// not the user's terminal.
func AttachClientCmd(session string) *exec.Cmd {
	cmd := exec.Command(TmuxBinary, attachArgs(session)...)
	cmd.Env = append(scrubEnv(os.Environ(), SessionEnv, PaneEnv, "TERM"), "TERM=xterm-256color")
	return cmd
}

// AttachClientCmd is the viewer's attach command, as a method for the same
// reason [Tmux.AttachCmd] is one.
func (t *Tmux) AttachClientCmd(session string) *exec.Cmd { return AttachClientCmd(session) }

// WritePromptFile writes an agent's opening prompt somewhere the session it is
// launched for can read it back, returning the file's path.
//
// The file goes in a directory of its own rather than at a predictable path in
// the shared temp dir: the agent obeys whatever it reads, so a file another
// user could have put there first would be an instruction we did not write.
func WritePromptFile(session, prompt string) (string, error) {
	dir, err := os.MkdirTemp("", "nat-prompt-")
	if err != nil {
		return "", fmt.Errorf("create prompt dir: %w", err)
	}
	return writePromptInto(dir, session, prompt)
}

// writePromptInto writes the prompt file inside dir. It is split out so that a
// write which fails — a directory that cannot be written to — is exercisable
// without arranging for MkdirTemp itself to succeed and then break.
func writePromptInto(dir, session, prompt string) (string, error) {
	path := filepath.Join(dir, session+".md")
	if err := os.WriteFile(path, []byte(prompt), 0o600); err != nil {
		return "", fmt.Errorf("write prompt file: %w", err)
	}
	return path, nil
}

// shellQuote wraps s in single quotes so /bin/sh reads it as one literal word,
// escaping any single quotes it contains.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
