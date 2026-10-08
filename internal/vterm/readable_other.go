//go:build !unix

package vterm

import "github.com/charmbracelet/x/xpty"

// readable has no poll to ask off unix, where no UnixPty is made anyway.
func readable(*xpty.UnixPty) bool { return false }
