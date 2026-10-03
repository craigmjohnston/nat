// Command nat-source-shortcut is nat's task-source plugin for Shortcut: it
// puts Shortcut stories in front of nat as the cards a source project's
// tasks hang off, and keeps the stories in step with what nat does to those
// tasks. nat runs it as `nat-source-shortcut <method>` with one JSON request
// on stdin; a person runs `login` and `config`. See internal/plugin.
package main

import (
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/craigmjohnston/nat/plugins/shortcut/internal/keychain"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/plugin"
)

// The process's edges, held as variables so a test can stand in for them —
// above all the Keychain, which no test may touch, and an exit that would
// take the test binary with it.
var (
	args                   = func() []string { return os.Args }
	stdin    io.Reader     = os.Stdin
	stdout   io.Writer     = os.Stdout
	stderr   io.Writer     = os.Stderr
	exit                   = os.Exit
	getenv                 = os.Getenv
	tokens   plugin.Tokens = keychain.Keychain{Run: keychain.ExecRunner{}}
	clockNow               = time.Now
	executable             = os.Executable
)

func main() {
	exit(plugin.Run(args(), stdin, stdout, stderr, plugin.Env{Getenv: getenv, Now: clockNow, Tokens: tokens, Spawn: spawn}))
}

// spawn starts this binary again with args, detached — plugin.Env.Spawn.
func spawn(args ...string) error {
	exe, err := executable()
	if err != nil {
		return err
	}
	return detach(exe, args...)
}

// detach starts name with args in a session of its own, so nat's kill of the
// call that started it does not reach it, with stdin, stdout and stderr all
// /dev/null — nat waits on its child's pipes, and nothing the warm-up could
// say belongs there — and lets it go without waiting. It inherits the
// environment: the token (from SHORTCUT_API_TOKEN or the Keychain), the API
// and the cache directory are the ones the call that started it used.
func detach(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
