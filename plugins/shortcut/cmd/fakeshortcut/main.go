// Command fakeshortcut serves the seeded scratch workspace of
// internal/fakeshortcut over HTTP, for driving nat-source-shortcut end to end
// (through nat itself) with no live Shortcut token. It prints the
// SHORTCUT_API_URL to point the plugin at, then logs every request it is
// sent, writes with their bodies, to stderr.
//
//	go run ./plugins/shortcut/cmd/fakeshortcut -token fake -addr 127.0.0.1:47811
package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/craigmjohnston/nat/plugins/shortcut/internal/fakeshortcut"
)

// The process's edges, held as variables so a test can run main without
// serving forever or exiting the test binary.
var (
	args             = func() []string { return os.Args[1:] }
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
	exit             = os.Exit
	serve            = http.Serve
)

func main() { exit(run(args())) }

// run serves until serving fails, returning the exit code: 2 for bad flags,
// 1 for anything else.
func run(argv []string) int {
	fs := flag.NewFlagSet("fakeshortcut", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", "127.0.0.1:0", "address to listen on")
	token := fs.String("token", "fake", "the Shortcut-Token to accept")
	if fs.Parse(argv) != nil {
		return 2
	}
	s := fakeshortcut.Seed(*token)
	s.Log, s.Now = stderr, time.Now
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "SHORTCUT_API_URL=http://%s%s\n", ln.Addr(), fakeshortcut.Prefix)
	_, _ = fmt.Fprintln(stderr, serve(ln, s))
	return 1
}
