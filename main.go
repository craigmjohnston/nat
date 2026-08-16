// Command nat is a terminal UI over a Notion workspace for tracking project
// work executed by Claude Code agents.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/cli"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/nudge"
	"github.com/craigmjohnston/nat/internal/tui"
)

// tmuxEnv is set by tmux inside every pane it runs, and is how a process tells
// that it is already in one.
const tmuxEnv = "TMUX"

// noTmuxEnv opts out of hosting the TUI in tmux, for running it somewhere the
// split view is not wanted. Set to any non-empty value.
const noTmuxEnv = "NAT_NO_TMUX"

// The process's edges, held as variables so tests can stand in for them: main
// is otherwise unexercisable without the real Notion CLI, a terminal, an exit
// that would take the test binary with it, and an exec that would replace it.
var (
	newTokens                      = ntnCLI
	args                           = processArgs
	stdin        io.Reader         = os.Stdin
	stdout       io.Writer         = os.Stdout
	stderr       io.Writer         = os.Stderr
	exit                           = os.Exit
	newClient    tui.NewClientFunc = tui.DefaultNewClient
	newCLIClient cli.NewClientFunc = cli.DefaultNewClient
	lookPath                       = exec.LookPath
	executable                     = os.Executable
	execProcess                    = syscall.Exec
)

// ntnCLI is where the Notion credential really comes from.
func ntnCLI() config.TokenSource { return config.NewNtnCLI() }

// processArgs is what the binary was invoked with, less its own name.
func processArgs() []string { return os.Args[1:] }

func main() {
	logPath := openLog()
	defer func() { _ = logging.Close() }()

	if cli.IsCommand(args()) {
		if err := command(newTokens()); err != nil {
			fail(logPath, authHint(err))
		}
		return
	}
	if err := host(); err != nil {
		fail(logPath, err)
		return
	}
	if err := run(newTokens(), stdin, stdout); err != nil {
		fail(logPath, err)
	}
}

// openLog starts the log file and returns its path, or "" when there is not
// even a path to name. A log that cannot be opened is said once on stderr and
// then left alone: it is not a reason to refuse to run, and every call through
// the package is discarded from here on.
func openLog() string {
	path, err := logging.Open()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nat: could not open the log file:", err)
	}
	logging.Action("nat starting", "args", args())
	return path
}

// fail reports a failure that stops the app, in both the places it has to go.
//
// stderr alone is not enough: started outside tmux the TUI re-execs itself into
// a session, and a process that dies on the way up takes the pane it was
// writing to with it — which is how a startup crash becomes a binary that
// appears to do nothing at all. The log is where that run can still be read
// afterwards, so the message says where to find it.
func fail(logPath string, err error) {
	logging.Error("nat exiting on a failure", "err", err)
	// A stderr that will not take the message is exactly the case the log is
	// there for, so a failed write is nothing to report — there is nowhere left
	// to report it to.
	_, _ = fmt.Fprintln(stderr, "nat:", err)
	if logPath != "" {
		_, _ = fmt.Fprintln(stderr, "log:", logPath)
	}
	exit(1)
}

// command runs a headless subcommand. It deliberately runs before host: a
// command prints to the terminal it was typed in and exits, and re-execing it
// into a tmux session would send its output somewhere nobody is looking.
func command(tokens config.TokenSource) error {
	return cli.Run(context.Background(), args(), cli.Env{
		Tokens:    tokens,
		Load:      config.Load,
		NewClient: newCLIClient,
		Out:       stdout,
		In:        stdin,
		Nudge:     nudge.Touch,
	})
}

// host puts the TUI inside tmux, by replacing this process with a tmux session
// running the same binary. The agents the board launches are tmux sessions — so
// rather than ask the user to remember to start us under tmux, we do it for
// them. Nothing else needs it: the board draws its own status line inside its
// frame and turns tmux's bar off, and the terminal beside it is nat's own.
//
// It returns only when the app should carry on in this process: already inside
// tmux (running in place, because nesting a server inside a pane is not what
// anyone means by it), or opted out through [noTmuxEnv]. On success there is
// nothing to return to — exec has replaced us.
//
// The binary is resolved through [os.Executable] rather than argv[0] so that a
// launch through a relative path or a PATH lookup still names something tmux
// can run from a session whose working directory is its own.
func host() error {
	if os.Getenv(tmuxEnv) != "" || os.Getenv(noTmuxEnv) != "" {
		return nil
	}
	tmux, err := lookPath(agent.TmuxBinary)
	if err != nil {
		return tmuxHint(err)
	}
	self, err := executable()
	if err != nil {
		return fmt.Errorf("find the running binary: %w", err)
	}
	argv := append([]string{agent.TmuxBinary}, agent.HostArgs(self)...)
	if err := execProcess(tmux, argv, os.Environ()); err != nil {
		return fmt.Errorf("start tmux: %w", err)
	}
	return nil
}

// tmuxHint appends the commands that fix a missing tmux, the same way an
// authentication failure is reported with the command that fixes it — including
// the way out for someone who would rather not install it at all.
func tmuxHint(err error) error {
	return fmt.Errorf("%s not found on PATH: %w\ninstall it with: brew install %s\nor run without it: %s=1 nat",
		agent.TmuxBinary, err, agent.TmuxBinary, noTmuxEnv)
}

// run builds the root model and hands it to Bubble Tea.
//
// There is nothing to hand back on the way out: an agent watched from the board
// lives in a tmux session of its own throughout, and the hidden client nat runs
// on a pseudo-terminal to draw it hangs up when this process goes — however it
// goes.
func run(tokens config.TokenSource, in io.Reader, out io.Writer) error {
	app, err := buildApp(tokens)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(app, tea.WithInput(in), tea.WithOutput(out)).Run()
	return err
}

// buildApp decides which screen the app starts on: the first-run wizard when
// there is no config file yet, otherwise the board.
//
// The token is checked here so that an unusable credential is reported plainly
// on stderr, with the command that fixes it, rather than as a failed call once
// the terminal has been taken over. The client is handed the source rather than
// the token itself, so a rotation later in the session is picked up too.
func buildApp(tokens config.TokenSource) (*tui.App, error) {
	cfg, found, err := config.Load()
	if err != nil {
		return nil, err
	}
	if _, err := tokens.Token(); err != nil {
		return nil, authHint(err)
	}
	client := newClient(tokens.Token)
	if !found {
		return tui.NewAppWithOnboarding(cfg, client, tui.NewOnboarding(cfg, client, config.Save)), nil
	}
	return tui.NewApp(cfg, client), nil
}

// authHint appends the command that fixes an authentication failure, so the
// user is not left to work out what "not logged in" wants from them.
func authHint(err error) error {
	switch {
	case errors.Is(err, config.ErrNtnNotInstalled):
		return fmt.Errorf("%w\ninstall it with: curl -fsSL https://ntn.dev | bash", err)
	case errors.Is(err, config.ErrNtnNotLoggedIn):
		return fmt.Errorf("%w\nlog in with: %s login", err, config.NtnBinary)
	default:
		return err
	}
}
