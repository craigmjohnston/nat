package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/source"
	"github.com/craigmjohnston/nat/internal/store"
)

// maxBinary caps a download: a plugin is a Go binary of some megabytes, two
// architectures lipo'd together, and anything this size is not one. A var so
// a test can lower it.
var maxBinary int64 = 256 << 20

// The filesystem calls an install makes whose failure no real directory can
// be made to produce on cue, held as vars so a test can.
var (
	chmod       = os.Chmod
	writeRecord = os.WriteFile
)

// Record is a managed install's installed.json: where the binary beside it
// came from, which release, and the digest it was checked against.
type Record struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Source      string `json:"source"`
	Version     string `json:"version"`
	SHA256      string `json:"sha256"`
	InstalledAt string `json:"installed_at"`
}

// diskRecord is the part of a Record the file itself keeps: the name and
// path are where it is.
type diskRecord struct {
	Source      string `json:"source"`
	Version     string `json:"version"`
	SHA256      string `json:"sha256"`
	InstalledAt string `json:"installed_at"`
}

// pluginDir is where a plugin of the name is installed.
func (m *Manager) pluginDir(name string) string {
	return filepath.Join(m.ConfigDir, "plugins", name)
}

// readRecord reads a managed install's record. A directory with none is a
// manual install — whatever is there, nat did not put it there.
func (m *Manager) readRecord(name string) (diskRecord, bool, error) {
	data, err := os.ReadFile(filepath.Join(m.pluginDir(name), recordFile))
	if errors.Is(err, fs.ErrNotExist) {
		return diskRecord{}, false, nil
	}
	if err != nil {
		return diskRecord{}, false, err
	}
	var r diskRecord
	if err := json.Unmarshal(data, &r); err != nil {
		return diskRecord{}, true, fmt.Errorf("%s: malformed", filepath.Join(m.pluginDir(name), recordFile))
	}
	return r, true, nil
}

// Install puts the plugin of the name in place from a source: from, where
// one is named, else the first of sources that offers it — nat's own
// repository is first. version picks a release; empty is the source's latest.
//
// The binary is downloaded beside where it goes, checked against the
// manifest's digest (a mismatch is refused and the download deleted), made
// executable and renamed into place in one step, so a plugin is never seen
// half-written; then the record is written beside it. Installing over a
// managed install is how one is updated. Installing over a directory nat did
// not make — a binary copied or linked there by hand — is refused, naming it:
// nat never overwrites what someone put there themselves.
func (m *Manager) Install(ctx context.Context, sources []string, name, from, version string) (Record, error) {
	if err := ValidName(name); err != nil {
		return Record{}, err
	}
	dir := m.pluginDir(name)
	_, statErr := os.Stat(dir)
	fresh := errors.Is(statErr, fs.ErrNotExist)
	if statErr != nil && !fresh {
		return Record{}, statErr
	}
	if !fresh {
		// A record that will not parse is still nat's: the directory is one
		// an install made, so an install may replace it.
		_, managed, err := m.readRecord(name)
		if err != nil && !managed {
			return Record{}, err
		}
		if !managed {
			return Record{}, fmt.Errorf("plugin %s is installed by hand at %s: nat will not overwrite it — remove it first", name, dir)
		}
	}

	repo, man, entry, err := m.offer(ctx, sources, name, from, version)
	if err != nil {
		return Record{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Record{}, err
	}
	rec, err := m.place(ctx, dir, repo, man, entry)
	if err != nil && fresh {
		_ = os.RemoveAll(dir)
	}
	return rec, err
}

// offer finds the plugin of the name in the sources: the named one alone,
// else each in turn. A source that could not be read, or does not offer it,
// is passed over — and named in the refusal if none does.
func (m *Manager) offer(ctx context.Context, sources []string, name, from, version string) (string, Manifest, Entry, error) {
	if from != "" {
		if err := ValidRepo(from); err != nil {
			return "", Manifest{}, Entry{}, err
		}
		sources = []string{from}
	}
	var why []string
	for _, repo := range sources {
		man, err := m.ReadSource(ctx, repo, version)
		if err != nil {
			why = append(why, err.Error())
			continue
		}
		for _, e := range man.Plugins {
			if e.Name == name {
				return repo, man, e, nil
			}
		}
		why = append(why, fmt.Sprintf("%s v%s offers no plugin %s", repo, man.Version, name))
	}
	return "", Manifest{}, Entry{}, fmt.Errorf("no source offers plugin %s: %s", name, strings.Join(why, "; "))
}

// place downloads entry's binary into dir, checks it, and puts it and its
// record in place.
func (m *Manager) place(ctx context.Context, dir, repo string, man Manifest, entry Entry) (Record, error) {
	tmp, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return Record{}, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	sum, err := m.download(ctx, m.assetURL(repo, man.Version, entry.Asset), tmp)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return Record{}, fmt.Errorf("download plugin %s: %w", entry.Name, err)
	}
	if !strings.EqualFold(sum, entry.SHA256) {
		return Record{}, fmt.Errorf("download plugin %s: its SHA-256 is %s, the manifest says %s — not installed", entry.Name, sum, entry.SHA256)
	}
	if err := chmod(tmp.Name(), 0o755); err != nil {
		return Record{}, err
	}
	bin := filepath.Join(dir, binaryPrefix+entry.Name)
	if err := os.Rename(tmp.Name(), bin); err != nil {
		return Record{}, err
	}
	rec := diskRecord{Source: repo, Version: man.Version, SHA256: strings.ToLower(entry.SHA256), InstalledAt: m.Now().UTC().Format(time.RFC3339)}
	// A struct of strings always encodes.
	data, _ := json.MarshalIndent(rec, "", "  ")
	if err := writeRecord(filepath.Join(dir, recordFile), append(data, '\n'), 0o644); err != nil {
		return Record{}, err
	}
	return Record{Name: entry.Name, Path: bin, Source: rec.Source, Version: rec.Version, SHA256: rec.SHA256, InstalledAt: rec.InstalledAt}, nil
}

// download writes url's body to w and answers its SHA-256. Nothing of the
// body reaches an error.
func (m *Manager) download(ctx context.Context, url string, w io.Writer) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	resp, err := m.get(ctx, url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(resp.Body, maxBinary+1))
	if err != nil {
		return "", err
	}
	if n > maxBinary {
		return "", fmt.Errorf("GET %s: over %d MiB, not a plugin", url, maxBinary>>20)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// DeletedProject is a source project an uninstall deleted with its plugin.
type DeletedProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Uninstalled is what an uninstall took away: the plugin's directory, and
// the source projects of it deleted beside it (empty unless asked for).
type Uninstalled struct {
	Path            string
	ProjectsDeleted []DeletedProject
}

// Uninstall takes the plugin of the name away: its whole directory under
// plugins/, managed or put there by hand — the user asked. While any project
// is a source project of it, it is refused, naming them, since that
// project's sidebar would lose its plugin — unless deleteProjects says to
// delete them first. It is refused for a plugin found only on PATH, which is
// not nat's to remove, before any project is touched. Its projects are named
// title — a source project is called what its plugin calls itself, never
// what its entry says.
//
// Each project goes as project-create made it, in reverse: its plan file
// (and SQLite's -wal/-shm beside it; a file already gone is fine) first, then
// its config entry, saved through save — so a removal the OS refuses stops
// with that project still whole, and no entry ever names a plan that is
// gone. The active project is cleared where it was one of them.
func (m *Manager) Uninstall(name, title string, cfg config.Config, deleteProjects bool, save func(config.Config) error) (Uninstalled, error) {
	if err := ValidName(name); err != nil {
		return Uninstalled{}, err
	}
	var ids, using []string
	for id, p := range cfg.Projects {
		if p.IsSource() && p.Source == name {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	for _, id := range ids {
		using = append(using, fmt.Sprintf("%s (%s)", title, id))
	}
	slices.Sort(using)
	if len(using) > 0 && !deleteProjects {
		return Uninstalled{}, fmt.Errorf("plugin %s is the source of %s: delete or move those projects first, or pass --delete-projects", name, strings.Join(using, ", "))
	}
	dir := m.pluginDir(name)
	if _, err := os.Lstat(dir); err != nil {
		p, found, err := source.Find(m.ConfigDir, name)
		if err != nil {
			return Uninstalled{}, err
		}
		if found {
			return Uninstalled{}, fmt.Errorf("plugin %s is on PATH at %s, not installed by nat: remove it there", name, p.Path)
		}
		return Uninstalled{}, fmt.Errorf("no plugin %s is installed", name)
	}
	out := Uninstalled{Path: dir, ProjectsDeleted: []DeletedProject{}}
	for _, id := range ids {
		p := cfg.Projects[id]
		if err := removePlan(store.ProjectOf(id, p)); err != nil {
			return out, fmt.Errorf("delete project %s (%s): %w", title, id, err)
		}
		delete(cfg.Projects, id)
		if cfg.ActiveProjectID == id {
			cfg.ActiveProjectID = ""
		}
		if err := save(cfg); err != nil {
			return out, fmt.Errorf("delete project %s (%s): save config: %w", title, id, err)
		}
		out.ProjectsDeleted = append(out.ProjectsDeleted, DeletedProject{ID: id, Name: title})
	}
	if err := os.RemoveAll(dir); err != nil {
		return out, err
	}
	return out, nil
}

// removePlan deletes a project's plan file and the SQLite journal files
// beside it. One that is not there is already deleted.
func removePlan(p store.Project) error {
	path, err := store.PlanPath(p)
	if err != nil {
		return err
	}
	for _, f := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}
