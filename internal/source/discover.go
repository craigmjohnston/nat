package source

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// prefix is what every plugin binary's name starts with.
const prefix = "nat-source-"

// pathEnv is the PATH Discover searches, a seam so the tests can choose it.
var pathEnv = func() string { return os.Getenv("PATH") }

// Plugin is an installed task source: its name and the binary that is it.
type Plugin struct {
	Name string
	Path string
}

// Discover lists every installed plugin, sorted by name. The plugins dir —
// `<configDir>/plugins/<name>/nat-source-<name>` — is searched first, then each
// directory on PATH for an executable `nat-source-<name>`; where both have one
// of a name, the plugins dir's wins, so a plugin installed for nat shadows one
// that happens to be on PATH. A plugins dir that doesn't exist is no plugins,
// not an error; one that can't be read is. Unreadable PATH entries are skipped,
// as a shell would skip them.
func Discover(configDir string) ([]Plugin, error) {
	found := map[string]string{}
	dir := filepath.Join(configDir, "plugins")
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		name := e.Name()
		path := filepath.Join(dir, name, prefix+name)
		if e.IsDir() && executable(path) {
			found[name] = path
		}
	}
	for _, pdir := range filepath.SplitList(pathEnv()) {
		entries, err := os.ReadDir(pdir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name, ok := strings.CutPrefix(e.Name(), prefix)
			if !ok || name == "" {
				continue
			}
			if _, seen := found[name]; seen {
				continue
			}
			path := filepath.Join(pdir, e.Name())
			if executable(path) {
				found[name] = path
			}
		}
	}
	plugins := make([]Plugin, 0, len(found))
	for name, path := range found {
		plugins = append(plugins, Plugin{Name: name, Path: path})
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })
	return plugins, nil
}

// Find is the installed plugin of one name, if there is one.
func Find(configDir, name string) (Plugin, bool, error) {
	plugins, err := Discover(configDir)
	if err != nil {
		return Plugin{}, false, err
	}
	for _, p := range plugins {
		if p.Name == name {
			return p, true, nil
		}
	}
	return Plugin{}, false, nil
}

// executable reports whether path is a regular file — following a symlink —
// that someone may execute.
func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0
}
