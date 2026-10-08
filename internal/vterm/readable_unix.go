//go:build unix

package vterm

import (
	"github.com/charmbracelet/x/xpty"
	"golang.org/x/sys/unix"
)

// readable reports whether the parent end of p has bytes waiting to be read:
// a zero-timeout poll, through Control so the descriptor stays non-blocking
// for the read pump. A closed or unpollable PTY has nothing to read.
func readable(p *xpty.UnixPty) bool {
	var ready bool
	_ = p.Control(func(fd uintptr) {
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}} //nolint:gosec // a descriptor fits
		n, err := unix.Poll(fds, 0)
		ready = err == nil && n > 0 && fds[0].Revents&unix.POLLIN != 0
	})
	return ready
}
