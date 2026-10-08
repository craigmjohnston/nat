// Package mods puts nat's embedded Claude Code mod on disk for a launch to
// load with `claude --plugin-dir`. See docs/design/embedded-mod/README.md.
package mods

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/version"
	embeddedmods "github.com/craigmjohnston/nat/mods"
)

// Name is the mod's plugin name, and the folder --plugin-dir is pointed at.
const Name = "nat-embedded"

// dirName is where materialised mods live under nat's state directory, beside
// agent-status/ and usage-probe/.
const dirName = "mods"

// manifest is the mod's plugin.json, whose version is stamped with the
// build's own as it is written.
const manifest = ".claude-plugin/plugin.json"

// Materialise writes the embedded mod to
// `<state dir>/mods/<hash>/nat-embedded/` and answers that path, where <hash>
// is the first 12 hex digits of a sha256 over the files as written, names and
// contents. A folder already there is used as it stands: a session loading
// it watches it and hot-reloads on any change, so one a live session loaded
// is never rewritten, and a nat whose mod differs gets a hash of its own.
// Older hashes are left for [Sweep].
func Materialise() (string, error) {
	root, err := root()
	if err != nil {
		return "", err
	}
	return materialise(root, embeddedmods.Embedded(), version.Version())
}

// root is the directory every materialised mod lives under.
func root() (string, error) {
	dir, err := logging.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, dirName), nil
}

// sweepGrace is how old a folder under the mods directory must be before
// [Sweep] removes it: another nat — an older build beside this one — may have
// just written its hash and not yet started the session that loads it, and a
// temp tree may be one still being written.
const sweepGrace = time.Minute

// Sweep removes every materialised mod but this build's and those a running
// session loaded, along with any temp tree a failed write left. inUse is the
// start command of every live tmux pane: a hash folder named in one is kept,
// since deleting a folder a session loaded unloads the mod from it there and
// then. A folder younger than [sweepGrace] is kept too. Best effort: a
// removal that fails is logged and left for the next sweep.
func Sweep(inUse []string) {
	root, err := root()
	if err != nil {
		return
	}
	sweep(root, embeddedmods.Embedded(), version.Version(), inUse, time.Now())
}

// sweep is [Sweep] under root, keeping src stamped as ver, at now. A mod that
// cannot be read sweeps nothing: what is current is not known.
func sweep(root string, src fs.FS, ver string, inUse []string, now time.Time) {
	files, err := readFiles(src, ver)
	if err != nil {
		return
	}
	current := hashOf(files)
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		dir := filepath.Join(root, name)
		if name == current || named(inUse, dir+string(filepath.Separator)) {
			continue
		}
		if info, err := e.Info(); err != nil || now.Sub(info.ModTime()) < sweepGrace {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			logging.Action("old embedded mod not removed", "dir", dir, "error", err.Error())
			continue
		}
		logging.Action("old embedded mod removed", "dir", dir)
	}
}

// named reports whether any of commands mentions path.
func named(commands []string, path string) bool {
	for _, c := range commands {
		if strings.Contains(c, path) {
			return true
		}
	}
	return false
}

// materialise is [Materialise] into root, from src, stamped as ver.
func materialise(root string, src fs.FS, ver string) (string, error) {
	files, err := readFiles(src, ver)
	if err != nil {
		return "", err
	}
	final := filepath.Join(root, hashOf(files))
	modDir := filepath.Join(final, Name)
	if _, err := os.Stat(final); err == nil {
		return modDir, nil
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	// The whole tree is written under a temp name and renamed into place, so
	// a hash directory that exists is always a complete one.
	tmp, err := os.MkdirTemp(root, ".tmp-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := writeTree(filepath.Join(tmp, Name), files); err != nil {
		return "", err
	}
	if err := rename(tmp, final); err != nil {
		// Another nat materialising the same hash at once got there first.
		if _, statErr := os.Stat(final); statErr == nil {
			return modDir, nil
		}
		return "", err
	}
	return modDir, nil
}

// rename is os.Rename, a seam for the race of two nats materialising one hash.
var rename = os.Rename

// writeTree writes files under dir, files 0600 and directories 0700.
func writeTree(dir string, files []file) error {
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f.name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(p, f.data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// file is one file of the mod as it will be written.
type file struct {
	name string // slash-separated, relative to the mod's root
	data []byte
}

// readFiles reads every file of src in lexical order, the manifest's version
// stamped with ver.
func readFiles(src fs.FS, ver string) ([]file, error) {
	var files []file
	err := fs.WalkDir(src, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(src, name)
		if err != nil {
			return err
		}
		if name == manifest {
			if data, err = stampVersion(data, ver); err != nil {
				return err
			}
		}
		files = append(files, file{name: path.Clean(name), data: data})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("the embedded mod has no files")
	}
	return files, nil
}

// stampVersion answers a manifest with its version set to ver.
func stampVersion(data []byte, ver string) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("read %s: %w", manifest, err)
	}
	m["version"] = ver
	out, _ := json.MarshalIndent(m, "", "  ") // a decoded map always encodes
	return append(out, '\n'), nil
}

// hashOf is the first 12 hex digits of a sha256 over files' names and
// contents, each prefixed with its length so no two sets hash the same input.
func hashOf(files []file) string {
	h := sha256.New()
	for _, f := range files {
		_, _ = fmt.Fprintf(h, "%d:%s%d:", len(f.name), f.name, len(f.data))
		h.Write(f.data)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}
