// Package mods carries the Claude Code mods nat loads into the sessions it
// launches, built into the binary the way package skills carries the skills:
// a `go install`ed nat has no checkout to read them from. Embedding lives
// here, beside the mod, because go:embed reaches no file above its own
// directory; internal/mods is what puts them on disk.
package mods

import (
	"embed"
	"io/fs"
)

// Only what Claude Code loads is embedded — the manifest and the hooks — not
// the mod's tests or README, nor the types and tsconfig.json a build lays
// beside a loaded mod (gitignored, but present in a checkout that loaded it).
//
//go:embed embedded/.claude-plugin/plugin.json embedded/hooks
var embedded embed.FS

// Embedded is nat's own mod, its files at the root (`.claude-plugin/`,
// `hooks/`).
func Embedded() fs.FS {
	sub, _ := fs.Sub(embedded, "embedded") // a static, valid name: never fails
	return sub
}
