package mods

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	embeddedmods "github.com/craigmjohnston/nat/mods"
)

func testMod() fstest.MapFS {
	return fstest.MapFS{
		".claude-plugin/plugin.json": {Data: []byte(`{"name":"nat-embedded","version":"0.0.0"}`)},
		"hooks/hooks.json":           {Data: []byte(`{ "modules": ["./register.ts"] }`)},
		"hooks/register.ts":          {Data: []byte("export const register = () => {}\n")},
	}
}

func TestMaterialiseWritesTheMod(t *testing.T) {
	root := filepath.Join(t.TempDir(), "mods")
	dir, err := materialise(root, testMod(), "v1.2.3")
	if err != nil {
		t.Fatalf("materialise: %v", err)
	}
	if filepath.Base(dir) != Name || len(filepath.Base(filepath.Dir(dir))) != 12 || filepath.Dir(filepath.Dir(dir)) != root {
		t.Errorf("dir = %q, want %s/<12 hex>/%s", dir, root, Name)
	}
	got, err := os.ReadFile(filepath.Join(dir, "hooks", "register.ts"))
	if err != nil || string(got) != "export const register = () => {}\n" {
		t.Errorf("register.ts = %q, %v", got, err)
	}
	var m map[string]any
	data, _ := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if err := json.Unmarshal(data, &m); err != nil || m["version"] != "v1.2.3" || m["name"] != "nat-embedded" {
		t.Errorf("plugin.json = %s (%v), want the name kept and the build's version", data, err)
	}
	// Private to the user: the files 0600, every directory 0700.
	_ = filepath.WalkDir(filepath.Dir(dir), func(p string, d fs.DirEntry, err error) error {
		info, _ := d.Info()
		want := fs.FileMode(0o600)
		if d.IsDir() {
			want = 0o700
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s mode = %v, want %v", p, info.Mode().Perm(), want)
		}
		return err
	})
	// Nothing of the temp tree is left behind.
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Errorf("root holds %d entries, want only the hash directory", len(entries))
	}
}

// A folder a live session loaded is never rewritten: a second materialise of
// the same content writes nothing.
func TestMaterialiseIsWriteOnce(t *testing.T) {
	root := t.TempDir()
	dir, err := materialise(root, testMod(), "v1")
	if err != nil {
		t.Fatal(err)
	}
	reg := filepath.Join(dir, "hooks", "register.ts")
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(reg, old, old); err != nil {
		t.Fatal(err)
	}
	again, err := materialise(root, testMod(), "v1")
	if err != nil || again != dir {
		t.Fatalf("second materialise = %q, %v; want %q", again, err, dir)
	}
	if info, _ := os.Stat(reg); !info.ModTime().Equal(old) {
		t.Errorf("register.ts mtime = %v, want it untouched at %v", info.ModTime(), old)
	}
}

func TestMaterialiseHashFollowsContent(t *testing.T) {
	root := t.TempDir()
	base, _ := materialise(root, testMod(), "v1")
	changed := testMod()
	changed["hooks/register.ts"] = &fstest.MapFile{Data: []byte("export const register = on => {}\n")}
	other, _ := materialise(root, changed, "v1")
	upgraded, _ := materialise(root, testMod(), "v2")
	if base == other || base == upgraded || other == upgraded {
		t.Errorf("dirs = %q, %q, %q; want each content its own hash", base, other, upgraded)
	}
	// Older hashes are left where they are.
	if _, err := os.Stat(base); err != nil {
		t.Errorf("the first hash's folder is gone: %v", err)
	}
}

func TestMaterialiseFailures(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		root string
		src  fs.FS
	}{
		"root under a file": {filepath.Join(file, "mods"), testMod()},
		"empty mod":         {t.TempDir(), fstest.MapFS{}},
		"bad manifest": {t.TempDir(), fstest.MapFS{
			".claude-plugin/plugin.json": {Data: []byte("{")},
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if dir, err := materialise(c.root, c.src, "v1"); err == nil {
				t.Errorf("materialise = %q, want an error", dir)
			}
		})
	}
}

// The real Materialise writes the embedded mod under the state directory.
func TestMaterialiseEmbedded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir, err := Materialise()
	if err != nil {
		t.Fatalf("Materialise: %v", err)
	}
	for _, f := range []string{".claude-plugin/plugin.json", "hooks/hooks.json", "hooks/register.ts"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	data, _ := fs.ReadFile(embeddedmods.Embedded(), "hooks/register.ts")
	if !strings.Contains(string(data), "PromptHint") {
		t.Errorf("embedded register.ts = %q, want the PromptHint hook", data)
	}
	t.Setenv("HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	if _, err := Materialise(); err == nil {
		t.Error("Materialise: want an error with no resolvable state directory")
	}
}

func TestWriteTreeFailures(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o700); err != nil {
		t.Fatal(err)
	}
	// A directory wanted where a file is, and a file where a directory is.
	for _, name := range []string{"a/b", "d"} {
		if err := writeTree(dir, []file{{name: name}}); err == nil {
			t.Errorf("writeTree(%s): want an error", name)
		}
	}
}

func TestMaterialiseUnwritableRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	if dir, err := materialise(root, testMod(), "v1"); err == nil {
		t.Errorf("materialise = %q, want an error when no temp tree can be made", dir)
	}
	// A root that is itself a file fails at the temp tree's first file.
	file := filepath.Join(t.TempDir(), "mods")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if dir, err := materialise(file, testMod(), "v1"); err == nil {
		t.Errorf("materialise = %q, want an error", dir)
	}
}

func TestMaterialiseRenameRace(t *testing.T) {
	t.Cleanup(func() { rename = os.Rename })
	root := t.TempDir()
	// Another nat lands the same hash between the check and the rename: its
	// folder is used, and this one's temp tree is cleared away.
	rename = func(tmp, final string) error {
		if err := os.Rename(tmp, final); err != nil {
			t.Fatal(err)
		}
		return os.ErrExist
	}
	dir, err := materialise(root, testMod(), "v1")
	if err != nil {
		t.Fatalf("materialise: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Error(err)
	}
	// A rename that fails with nothing in place is the failure.
	rename = func(string, string) error { return os.ErrPermission }
	if dir, err := materialise(t.TempDir(), testMod(), "v1"); err == nil {
		t.Errorf("materialise = %q, want the rename's error", dir)
	}
}

// failRead is a mod whose directories list but whose files cannot be read.
type failRead struct{ fstest.MapFS }

func (failRead) ReadFile(string) ([]byte, error) { return nil, fs.ErrPermission }

func TestMaterialiseUnreadableFile(t *testing.T) {
	if dir, err := materialise(t.TempDir(), failRead{testMod()}, "v1"); err == nil {
		t.Errorf("materialise = %q, want the read's error", dir)
	}
}

// clash is a mod listing "a" both as a file and as a directory holding "b",
// which no tree on disk can hold.
type clash struct{ fstest.MapFS }

func (c clash) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := c.MapFS.ReadDir(name)
	if name != "." {
		return entries, err
	}
	file, _ := fs.Stat(fstest.MapFS{"a": {}}, "a")
	return append([]fs.DirEntry{fs.FileInfoToDirEntry(file)}, entries...), err
}

func (c clash) ReadFile(name string) ([]byte, error) {
	if name == "a" {
		return nil, nil
	}
	return c.MapFS.ReadFile(name)
}

func TestMaterialiseUnwritableTree(t *testing.T) {
	src := clash{fstest.MapFS{"a/b": {Data: []byte("x")}}}
	if dir, err := materialise(t.TempDir(), src, "v1"); err == nil {
		t.Errorf("materialise = %q, want the write's error", dir)
	}
}

func TestSweep(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	mk := func(name string, age time.Duration) string {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Join(dir, Name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(dir, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	files, _ := readFiles(testMod(), "v1")
	current := mk(hashOf(files), time.Hour)
	inUse := mk("aaaaaaaaaaaa", time.Hour)
	fresh := mk("ffffffffffff", time.Second)
	old := mk("000000000000", time.Hour)
	stale := mk(".tmp-123", time.Hour)
	// A prefix of a kept hash is a different folder, not a mention of it.
	prefix := mk("aaaaaaaaaaa", time.Hour)
	commands := []string{"", "sh -c \"claude --plugin-dir '" + filepath.Join(inUse, Name) + "'\""}
	// A mod that cannot be read sweeps nothing.
	sweep(root, failRead{testMod()}, "v1", commands, now)
	if _, err := os.Stat(old); err != nil {
		t.Errorf("an unreadable mod swept %s: %v", old, err)
	}
	sweep(root, testMod(), "v1", commands, now)
	for _, kept := range []string{current, inUse, fresh} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s swept: %v", kept, err)
		}
	}
	for _, gone := range []string{old, stale, prefix} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s kept (%v), want it swept", gone, err)
		}
	}
	// A root that is not there sweeps nothing and does not fail.
	sweep(filepath.Join(root, "missing"), testMod(), "v1", nil, now)
}

func TestSweepRemovalFailure(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "000000000000")
	if err := os.MkdirAll(filepath.Join(old, Name), 0o700); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	// Its contents cannot be unlinked from a directory with no write bit.
	if err := os.Chmod(old, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(old, 0o700) })
	sweep(root, testMod(), "v1", nil, time.Now())
	if _, err := os.Stat(old); err != nil {
		t.Errorf("%s: %v, want a failed removal left in place", old, err)
	}
}

// The real Sweep keeps this build's own mod and clears an old one.
func TestSweepEmbedded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir, err := Materialise()
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	_ = os.Chtimes(filepath.Dir(dir), past, past)
	old := filepath.Join(filepath.Dir(filepath.Dir(dir)), "000000000000")
	if err := os.Mkdir(old, 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(old, past, past)
	Sweep(nil)
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("this build's mod swept: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old mod kept (%v), want it swept", err)
	}
	t.Setenv("HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	Sweep(nil) // no state directory: nothing to sweep, no panic
}
