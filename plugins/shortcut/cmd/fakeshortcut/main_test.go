package main

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	defer func(a func() []string, e func(int), s func(net.Listener, http.Handler) error, o, er io.Writer) {
		args, exit, serve, stdout, stderr = a, e, s, o, er
	}(args, exit, serve, stdout, stderr)
	_ = args() // the real ones: the test binary's own flags
	var out, errb bytes.Buffer
	stdout, stderr = &out, &errb

	// Serving: the handler answers on the listener, then serving stops.
	var status int
	serve = func(ln net.Listener, h http.Handler) error {
		go func() { _ = http.Serve(ln, h) }()
		resp, err := http.Get("http://" + ln.Addr().String() + "/api/v3/member")
		if err == nil {
			status = resp.StatusCode
			_ = resp.Body.Close()
		}
		_ = ln.Close()
		return errors.New("stopped")
	}
	code := -1
	exit = func(c int) { code = c }
	args = func() []string { return []string{"-token", "x"} }
	main()
	if code != 1 || status != http.StatusUnauthorized || !strings.HasPrefix(out.String(), "SHORTCUT_API_URL=http://127.0.0.1:") ||
		!strings.Contains(errb.String(), "stopped") {
		t.Errorf("serve: exit %d, GET status %d, stdout %q, stderr %q", code, status, out.String(), errb.String())
	}

	if code := run([]string{"-nope"}); code != 2 {
		t.Errorf("bad flag: exit %d", code)
	}
	if code := run([]string{"-addr", "not an address"}); code != 1 {
		t.Errorf("bad address: exit %d", code)
	}
}
