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
// Claude Code on the latest channel from: one unauthenticated GitHub REST
// read, no gh and no token. A var so tests point it at an httptest server.
var claudeLatestURL = "https://api.github.com/repos/anthropics/claude-code/releases/latest"

// claudeStableURL is the stable channel's pointer — the plain-text version
// the native installer itself reads for `stable`, about a week behind latest.
// One unauthenticated read. A var so tests point it at an httptest server.
var claudeStableURL = "https://downloads.claude.ai/claude-code-releases/stable"

// The two release channels Claude Code updates along.
const (
	channelLatest = "latest"
	channelStable = "stable"
)

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

// claudeSettingsPath is the user's Claude Code settings file, where a
// non-Homebrew install's `autoUpdatesChannel` lives. A var so tests say where.
var claudeSettingsPath = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// claudeChannel is the release channel this install updates along, as Claude
// Code itself decides it: a Homebrew install by its cask's name (`claude-code`
// is stable, `claude-code@latest` latest), any other by `autoUpdatesChannel`
// in the user's settings. Anything unread or unrecognised is latest, Claude
// Code's own default.
func claudeChannel() string {
	if path, err := claudePath(); err == nil {
		if cask := homebrewCask(path); cask != "" {
			if cask == "claude-code" {
				return channelStable
			}
			return channelLatest
		}
	}
	path, err := claudeSettingsPath()
	if err != nil {
		return channelLatest
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return channelLatest
	}
	var settings struct {
		AutoUpdatesChannel string `json:"autoUpdatesChannel"`
	}
	if json.Unmarshal(data, &settings) != nil || settings.AutoUpdatesChannel != channelStable {
		return channelLatest
	}
	return channelStable
}

// claudeVersionCachePath is where the newest version read for channel is
// kept: `<state dir>/claude-version-<channel>.json`, beside the pr-status
// reading — one file a channel, so the two answers never mix. A var so tests
// point it at a throwaway directory.
var claudeVersionCachePath = func(channel string) (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "claude-version-"+channel+".json"), nil
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

// latestClaude is the newest Claude Code release on this install's channel
// ([claudeChannel]): the cache's where it was read within
// [claudeVersionTTL], else the channel's source's — the release feed for
// latest, the stable pointer for stable — written back to the cache. A failed
// read answers "" and leaves the cache as it was.
func latestClaude(ctx context.Context) string {
	channel := claudeChannel()
	read := readLatestClaude
	if channel == channelStable {
		read = readStableClaude
	}
	path, pathErr := claudeVersionCachePath(channel)
	if pathErr == nil {
		if data, err := os.ReadFile(path); err == nil {
			var cached claudeVersionCache
			if json.Unmarshal(data, &cached) == nil && cached.Latest != "" &&
				claudeVersionNow().Sub(cached.ReadAt) < claudeVersionTTL {
				return cached.Latest
			}
		}
	}
	latest, err := read(ctx)
	if err != nil {
		logging.Error("read the newest Claude Code release", "channel", channel, "err", err)
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

// fetchClaudeRelease is one unauthenticated GET of url, its body where it
// answered 200.
func fetchClaudeRelease(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := claudeVersionHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// readStableClaude reads the stable pointer: a bare version, one line.
func readStableClaude(ctx context.Context) (string, error) {
	body, err := fetchClaudeRelease(ctx, claudeStableURL)
	if err != nil {
		return "", err
	}
	stable := strings.TrimSpace(string(body))
	if stable == "" || stable[0] < '0' || stable[0] > '9' {
		return "", fmt.Errorf("stable pointer carried no version: %.40q", stable)
	}
	return stable, nil
}

// readLatestClaude reads the release feed's tag_name, without its leading v.
func readLatestClaude(ctx context.Context) (string, error) {
	body, err := fetchClaudeRelease(ctx, claudeLatestURL)
	if err != nil {
		return "", err
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
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
