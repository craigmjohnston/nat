// Package plugins installs, updates and uninstalls task-source plugins: the
// `nat-source-<name>` binaries internal/source runs. A plugin source is a
// GitHub repository whose releases carry a `nat-plugins.json` manifest beside
// the binaries it names; nat's own repository is always the first source. The
// format is specified in docs/design/task-sources/README.md, "Installing
// plugins".
//
// Nothing here runs a plugin. What is installed is still found the one way
// nat finds a plugin, [source.Discover]; this package only puts a binary where
// that looks, or takes one away.
package plugins

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/config"
)

// DefaultSource is nat's own repository, the plugin source every machine has
// and none can remove. It is the first source read, so a plugin it offers is
// the one installed by name wherever another source offers one too.
const DefaultSource = "craigmjohnston/nat"

// The names a source's release carries, and the name a managed install's
// record is kept under beside its binary.
const (
	manifestAsset = "nat-plugins.json"
	recordFile    = "installed.json"
	binaryPrefix  = "nat-source-"
)

// The two timeouts, held as vars so a test can shorten them: a manifest is a
// few hundred bytes and a source that takes longer than this to answer is one
// to report rather than wait on; a binary is megabytes over whatever line the
// machine has.
var (
	manifestTimeout = 10 * time.Second
	downloadTimeout = 60 * time.Second
)

// Manager is the one place plugins are installed and uninstalled from. Each
// field is an edge a test stands in for: the GitHub it talks to, the client it
// talks with, the config directory plugins are installed under and the clock
// an install is stamped with.
type Manager struct {
	// ConfigDir is nat's config directory; plugins go under its plugins/.
	ConfigDir string
	// BaseURL is GitHub's web root, "https://github.com" in production.
	BaseURL string
	// HTTP is the client every fetch goes through. Its redirect policy is
	// replaced (see [Manager.client]); its transport is kept.
	HTTP *http.Client
	// Now is the clock an install's record is stamped with.
	Now func() time.Time
}

// New is the Manager production uses: GitHub itself, the default client and
// the wall clock.
func New(configDir string) *Manager {
	return &Manager{ConfigDir: configDir, BaseURL: "https://github.com", HTTP: http.DefaultClient, Now: time.Now}
}

// Sources is every plugin source the config names, in the order they are
// read: nat's own repository first, then the config's extras, each once.
func Sources(cfg config.Config) []string {
	out := []string{DefaultSource}
	for _, s := range cfg.PluginSources {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// repoShape is GitHub's own rule for an owner and a repository name, as far
// as it matters here: what can sit in a URL path without escaping and
// without meaning a different path.
var repoShape = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?/[A-Za-z0-9._-]+$`)

// ValidRepo refuses anything that is not owner/repo.
func ValidRepo(repo string) error {
	_, name, _ := strings.Cut(repo, "/")
	if !repoShape.MatchString(repo) || name == "." || name == ".." {
		return fmt.Errorf("%q is not a GitHub repository: want owner/repo", repo)
	}
	return nil
}

// nameShape is the spec's rule for a plugin's name: lower-case letters,
// digits and `-`. Every name a path is built from is held to it, so no name —
// typed or read from a manifest — can reach outside the plugins directory.
var nameShape = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ValidName refuses a plugin name the spec would not allow.
func ValidName(name string) error {
	if !nameShape.MatchString(name) {
		return fmt.Errorf("%q is not a plugin name: want lower-case letters, digits and -", name)
	}
	return nil
}

// Newer reports whether version is newer than than, both read as dotted
// integers ("1.10.0" is newer than "1.9.2"). A version either side that does
// not read that way is never newer: an update offered on a guess is worse
// than none.
func Newer(version, than string) bool {
	a, okA := dotted(version)
	b, okB := dotted(than)
	if !okA || !okB {
		return false
	}
	for i := range max(len(a), len(b)) {
		x, y := at(a, i), at(b, i)
		if x != y {
			return x > y
		}
	}
	return false
}

// dotted reads a version as its integers, refusing anything else.
func dotted(v string) ([]int, bool) {
	if v == "" {
		return nil, false
	}
	parts := strings.Split(v, ".")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || strings.HasPrefix(p, "+") {
			return nil, false
		}
		out[i] = n
	}
	return out, true
}

// at is v's i-th part, zero past its end — so "1.2" and "1.2.0" are the same.
func at(v []int, i int) int {
	if i < len(v) {
		return v[i]
	}
	return 0
}
