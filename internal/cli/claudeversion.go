package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// claudeLatestURL is the release feed `claude-version` reads the newest
// Claude Code from: one unauthenticated GitHub REST read, no gh and no token.
// A var so tests point it at an httptest server.
var claudeLatestURL = "https://api.github.com/repos/anthropics/claude-code/releases/latest"

// claudeVersionHTTP is the client that feed is read through; its timeout
// keeps a slow network from holding gnat's read open.
var claudeVersionHTTP = &http.Client{Timeout: 10 * time.Second}

// claudeVersionTTL is how long a latest version read off the feed is kept
// before it is read again, so gnat's polling never fans out into GitHub.
const claudeVersionTTL = time.Hour

// claudeVersionNow is the clock the cache is aged against; faked in tests.
var claudeVersionNow = time.Now

// claudeRun runs name with args, answering its combined output: `claude
// --version` for the installed version, and the update itself — `claude
// update`, or `brew upgrade` for a Homebrew install. A var so tests never run
// the real ones.
var claudeRun = func(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

// claudePath is where the claude on PATH really lives, symlinks resolved —
// how a Homebrew install is told apart. A var so tests say where.
var claudePath = func() (string, error) {
	path, err := exec.LookPath("claude")
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}

// homebrewCask is the cask a Homebrew install of Claude Code came from — the
// path segment after `/Caskroom/`, as Claude Code itself detects one
// (`claude-code` or `claude-code@latest`) — or "" for any other install.
func homebrewCask(path string) string {
	_, after, ok := strings.Cut(path, "/Caskroom/")
	if !ok {
		return ""
	}
	cask, _, _ := strings.Cut(after, "/")
	return cask
}

// claudeVersionCachePath is where the latest version read off the feed is
// kept: `<state dir>/claude-version.json`, beside the pr-status reading. A
// var so tests point it at a throwaway directory.
var claudeVersionCachePath = func() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "claude-version.json"), nil
}

// claudeVersionCache is the cache file's shape: the feed's latest version
// and when it was read.
type claudeVersionCache struct {
	Latest string    `json:"latest"`
	ReadAt time.Time `json:"read_at"`
}

// claudeVersionJSON is `claude-version --json`'s answer. A side not read is
// absent, never empty, and then update_available is false.
type claudeVersionJSON struct {
	Installed       string `json:"installed,omitempty"`
	Latest          string `json:"latest,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
}

// claudeVersion answers which Claude Code is installed and the newest one
// released, for gnat's update notice. Not project-scoped: like `usage`, it
// is a property of this machine, not of any tracked project. It never fails
// over a read: either side unreadable is that field left out.
func claudeVersion(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("claude-version", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of plain text")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("claude-version: takes no arguments, given %d", len(rest))
	}

	doc := claudeVersionJSON{Installed: installedClaude(ctx), Latest: latestClaude(ctx)}
	doc.UpdateAvailable = doc.Installed != "" && doc.Latest != "" && newerVersion(doc.Latest, doc.Installed)

	if *asJSON {
		enc := json.NewEncoder(env.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(doc)
	}
	_, err = fmt.Fprintf(env.Out, "installed: %s\nlatest: %s\nupdate available: %t\n",
		orUnknown(doc.Installed), orUnknown(doc.Latest), doc.UpdateAvailable)
	return err
}

// orUnknown is a version for the plain-text answer, "unknown" where unread.
func orUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}

// installedClaude is the first token of `claude --version` ("2.1.294 (Claude
// Code)" reads 2.1.294), or "" where it cannot be run.
func installedClaude(ctx context.Context) string {
	out, err := claudeRun(ctx, "claude", "--version")
	if err != nil {
		logging.Error("claude --version failed", "err", err)
		return ""
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// latestClaude is the newest Claude Code release: the cache's where it was
// read within [claudeVersionTTL], else the feed's, written back to the cache.
// A failed feed read answers "" and leaves the cache as it was.
func latestClaude(ctx context.Context) string {
	path, pathErr := claudeVersionCachePath()
	if pathErr == nil {
		if data, err := os.ReadFile(path); err == nil {
			var cached claudeVersionCache
			if json.Unmarshal(data, &cached) == nil && cached.Latest != "" &&
				claudeVersionNow().Sub(cached.ReadAt) < claudeVersionTTL {
				return cached.Latest
			}
		}
	}
	latest, err := readLatestClaude(ctx)
	if err != nil {
		logging.Error("read the latest Claude Code release", "err", err)
		return ""
	}
	if pathErr == nil {
		data, _ := json.Marshal(claudeVersionCache{Latest: latest, ReadAt: claudeVersionNow()})
		err := os.MkdirAll(filepath.Dir(path), 0o700)
		if err == nil {
			err = os.WriteFile(path, data, 0o600)
		}
		if err != nil {
			logging.Error("cache the latest Claude Code release", "err", err)
		}
	}
	return latest
}

// readLatestClaude reads the release feed's tag_name, without its leading v.
func readLatestClaude(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, claudeLatestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := claudeVersionHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("release feed answered %s", resp.Status)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("parse release feed: %w", err)
	}
	latest := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	if latest == "" {
		return "", fmt.Errorf("release feed carried no tag_name")
	}
	return latest, nil
}

// newerVersion reports whether dotted version a is later than b, comparing
// each numeric part in turn ("2.1.295" > "2.1.294", "2.10.0" > "2.9.9"). A
// part that is not a number reads as 0, so a suffix never makes one newer.
func newerVersion(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(as), len(bs)); i++ {
		x, y := versionPart(as, i), versionPart(bs, i)
		if x != y {
			return x > y
		}
	}
	return false
}

// versionPart is the i'th part of a split version as a number: its leading
// digits, 0 where it has none or there is no such part.
func versionPart(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n := 0
	for _, r := range parts[i] {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// claudeUpdate updates Claude Code and relays what the update printed —
// gnat's update notice's one action. A Homebrew install is upgraded with
// `brew upgrade <cask>`: there `claude update` installs nothing, only
// printing the brew command and exiting 0. Any other install runs `claude
// update`. Live agents keep the binary they started with; a session launched
// after it gets the new one. The installed version is read afresh on every
// `claude-version`, so the notice goes with the next read. A failed update is
// the command's error, carrying the updater's output.
func claudeUpdate(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("claude-update", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of plain text")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("claude-update: takes no arguments, given %d", len(rest))
	}
	name, updateArgs := "claude", []string{"update"}
	if path, err := claudePath(); err == nil {
		if cask := homebrewCask(path); cask != "" {
			name, updateArgs = "brew", []string{"upgrade", cask}
		}
	}
	out, err := claudeRun(ctx, name, updateArgs...)
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(updateArgs, " "), err, strings.TrimSpace(out))
	}
	if *asJSON {
		enc := json.NewEncoder(env.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Output string `json:"output"`
		}{out})
	}
	_, err = io.WriteString(env.Out, out)
	return err
}
