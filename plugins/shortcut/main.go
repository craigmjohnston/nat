// Command nat-source-shortcut is nat's task-source plugin for Shortcut: it
// puts Shortcut stories in front of nat as the cards a source project's
// tasks hang off, and keeps the stories in step with what nat does to those
// tasks. nat runs it as `nat-source-shortcut <method>` with one JSON request
// on stdin; a person runs `login` and `config`. See internal/plugin.
package main

import (
	"io"
	"os"
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
)

func main() {
	exit(plugin.Run(args(), stdin, stdout, stderr, plugin.Env{Getenv: getenv, Now: clockNow, Tokens: tokens}))
}
