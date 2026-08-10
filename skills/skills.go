// Package skills carries the Claude Code skills that drive the tracker, built
// into the binary itself.
//
// They are embedded rather than read from disk because the binary is meant to
// be installed with `go install github.com/craigmjohnston/nat@latest`, which
// leaves no checkout behind: a copy of the skills has to travel with the
// binary, or `nat setup` would have nothing to install.
package skills

import (
	"embed"
	"io/fs"
)

// Each skill is named rather than matched by a wildcard, so that a directory
// added here is a deliberate addition to what `nat setup` installs.
//
//go:embed next-slice queue-work
var embedded embed.FS

// FS is the embedded skills: one directory per skill, at the root.
func FS() fs.FS { return embedded }
