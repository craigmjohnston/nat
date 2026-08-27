package cli

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/craigmjohnston/nat/skills"
)

// homeSkills points HOME at a temp directory and returns the skills directory
// setup will install into. Nothing else in the test touches the real one.
func homeSkills(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return filepath.Join(home, ".claude", "skills")
}

// runSetup runs the command and fails the test if it does not.
func runSetup(t *testing.T, args ...string) string {
	t.Helper()
	env, out := testEnv(testConfig(), &fakeAPI{})
	if err := Run(context.Background(), append([]string{"setup"}, args...), env); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return out.String()
}

// tree reads a directory into a map of relative path to content, which is how
// every test here says what is on disk.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	got := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		got[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	return got
}

// embeddedTree is the skills the binary carries, in the same shape as tree.
func embeddedTree(t *testing.T) map[string]string {
	t.Helper()
	want := map[string]string{}
	err := fs.WalkDir(skills.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(skills.FS(), path)
		if err != nil {
			return err
		}
		want[path] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("read the embedded skills: %v", err)
	}
	return want
}

func TestSetupInstallsTheSkillsFromNothing(t *testing.T) {
	dir := homeSkills(t)

	out := runSetup(t)

	want := embeddedTree(t)
	if len(want) == 0 {
		t.Fatal("no skills are embedded")
	}
	if got := tree(t, dir); !equalTrees(got, want) {
		t.Errorf("installed tree = %v, want %v", keys(got), keys(want))
	}
	for _, name := range []string{"next-slice", "queue-project", "queue-work"} {
		if !strings.Contains(out, "- "+name+" — created") {
			t.Errorf("output does not report %s as created:\n%s", name, out)
		}
	}
	if !strings.Contains(out, dir) {
		t.Errorf("output does not say where the skills went:\n%s", out)
	}
}

// The skills are embedded, so nothing checks that the copy in the binary is the
// copy in the repo — except this.
func TestEmbeddedSkillsMatchTheRepository(t *testing.T) {
	root := filepath.Join("..", "..", "skills")
	want := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) == ".go" {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		want[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("read %s: %v", root, err)
	}
	if got := embeddedTree(t); !equalTrees(got, want) {
		t.Errorf("embedded skills = %v, want the repository's %v", keys(got), keys(want))
	}
}

func TestSetupRunAgainChangesNothing(t *testing.T) {
	dir := homeSkills(t)
	runSetup(t)
	before := tree(t, dir)

	out := runSetup(t)

	if strings.Contains(out, statusCreated) || strings.Contains(out, statusUpdated) {
		t.Errorf("second run did not report the skills as unchanged:\n%s", out)
	}
	if got := tree(t, dir); !equalTrees(got, before) {
		t.Errorf("second run changed the tree: %v, want %v", keys(got), keys(before))
	}
}

func TestSetupBringsAStaleSkillUpToDate(t *testing.T) {
	dir := homeSkills(t)
	runSetup(t)
	skill := filepath.Join(dir, "next-slice")
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("an older version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "left-behind.md"), []byte("from a version gone by\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runSetup(t)

	if !strings.Contains(out, "- next-slice — updated") {
		t.Errorf("output does not report next-slice as updated:\n%s", out)
	}
	if !strings.Contains(out, "- queue-work — unchanged") {
		t.Errorf("output does not report queue-work as unchanged:\n%s", out)
	}
	if got := tree(t, dir); !equalTrees(got, embeddedTree(t)) {
		t.Errorf("installed tree = %v, want the embedded skills %v", keys(got), keys(embeddedTree(t)))
	}
}

func TestSetupLeavesSkillsItDoesNotOwnAlone(t *testing.T) {
	dir := homeSkills(t)
	other := filepath.Join(dir, "someone-elses")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	theirs := filepath.Join(other, "SKILL.md")
	if err := os.WriteFile(theirs, []byte("not ours\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runSetup(t)

	b, err := os.ReadFile(theirs)
	if err != nil || string(b) != "not ours\n" {
		t.Errorf("someone else's skill = %q, %v; want it untouched", b, err)
	}
	if strings.Contains(out, "someone-elses") {
		t.Errorf("output mentions a skill setup does not install:\n%s", out)
	}
}

func TestSetupLeavesASymlinkedSkillAlone(t *testing.T) {
	dir := homeSkills(t)
	checkout := t.TempDir()
	if err := os.WriteFile(filepath.Join(checkout, "SKILL.md"), []byte("worked on in place\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(checkout, filepath.Join(dir, "next-slice")); err != nil {
		t.Fatal(err)
	}

	out := runSetup(t)

	b, err := os.ReadFile(filepath.Join(checkout, "SKILL.md"))
	if err != nil || string(b) != "worked on in place\n" {
		t.Errorf("the symlink's target = %q, %v; want it untouched", b, err)
	}
	if !strings.Contains(out, "- next-slice — linked") || !strings.Contains(out, "delete it") {
		t.Errorf("output does not report the symlink and what to do about it:\n%s", out)
	}
}

func TestSetupReplacesASymlinkedFileInsideASkill(t *testing.T) {
	dir := homeSkills(t)
	runSetup(t)
	elsewhere := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(elsewhere, []byte("somewhere else\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	skillFile := filepath.Join(dir, "next-slice", "SKILL.md")
	if err := os.Remove(skillFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, skillFile); err != nil {
		t.Fatal(err)
	}

	runSetup(t)

	if info, err := os.Lstat(skillFile); err != nil || !info.Mode().IsRegular() {
		t.Errorf("skill file = %v, %v; want a real file", info, err)
	}
	b, err := os.ReadFile(elsewhere)
	if err != nil || string(b) != "somewhere else\n" {
		t.Errorf("the symlink's target = %q, %v; want it untouched", b, err)
	}
}

func TestSetupWritesJSON(t *testing.T) {
	dir := homeSkills(t)

	out := runSetup(t, "--json")

	var doc setupJSON
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("parse %s: %v", out, err)
	}
	if doc.Dir != dir {
		t.Errorf("skills_dir = %q, want %q", doc.Dir, dir)
	}
	want := []skillJSON{
		{Name: "next-slice", Status: statusCreated, Path: filepath.Join(dir, "next-slice")},
		{Name: "queue-project", Status: statusCreated, Path: filepath.Join(dir, "queue-project")},
		{Name: "queue-work", Status: statusCreated, Path: filepath.Join(dir, "queue-work")},
	}
	if len(doc.Skills) != len(want) {
		t.Fatalf("skills = %+v, want %+v", doc.Skills, want)
	}
	for i, s := range doc.Skills {
		if s != want[i] {
			t.Errorf("skills[%d] = %+v, want %+v", i, s, want[i])
		}
	}
}

func TestSetupRefusesAFileWhereASkillBelongs(t *testing.T) {
	dir := homeSkills(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "next-slice"), []byte("not a skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	env, _ := testEnv(testConfig(), &fakeAPI{})
	err := Run(context.Background(), []string{"setup"}, env)

	if err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Fatalf("err = %v, want it to say the path is not a directory", err)
	}
}

func TestSetupReportsAHomeDirectoryItCannotResolve(t *testing.T) {
	t.Setenv("HOME", "")

	env, _ := testEnv(testConfig(), &fakeAPI{})
	err := Run(context.Background(), []string{"setup"}, env)

	if err == nil || !strings.Contains(err.Error(), "resolve home dir") {
		t.Fatalf("err = %v, want it to say the home directory could not be resolved", err)
	}
}

func TestSetupReportsADirectoryItCannotCreate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".claude"), []byte("in the way\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	env, _ := testEnv(testConfig(), &fakeAPI{})
	err := Run(context.Background(), []string{"setup"}, env)

	if err == nil || !strings.Contains(err.Error(), "create ") {
		t.Fatalf("err = %v, want it to say the directory could not be created", err)
	}
}

func TestSetupRejectsMisuse(t *testing.T) {
	for _, args := range [][]string{{"setup", "please"}, {"setup", "--nope"}} {
		env, _ := testEnv(testConfig(), &fakeAPI{})
		err := Run(context.Background(), args, env)
		if _, ok := err.(*UsageError); !ok {
			t.Errorf("%v: err = %v, want a UsageError", args, err)
		}
	}
}

// errFS is a source of skills that fails to open the paths it is given, for the
// failures a read-only embedded filesystem cannot really have.
type errFS struct {
	fs.FS
	fail map[string]bool
}

func (e errFS) Open(name string) (fs.File, error) {
	if e.fail[name] {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return e.FS.Open(name)
}

// fakeSkills is a two-file skill, enough to exercise a nested directory.
func fakeSkills() fstest.MapFS {
	return fstest.MapFS{
		"a-skill/SKILL.md":                  &fstest.MapFile{Data: []byte("do the thing\n")},
		"a-skill/references/detail.md":      &fstest.MapFile{Data: []byte("at length\n")},
		"a-skill/references/nested/more.md": &fstest.MapFile{Data: []byte("further\n")},
	}
}

func TestInstallSkillsReportsASourceItCannotRead(t *testing.T) {
	src := errFS{FS: fakeSkills(), fail: map[string]bool{".": true}}

	_, err := installSkills(src, t.TempDir())

	if err == nil || !strings.Contains(err.Error(), "read the embedded skills") {
		t.Fatalf("err = %v, want it to say the embedded skills could not be read", err)
	}
}

func TestInstallSkillsReportsAFileItCannotRead(t *testing.T) {
	src := errFS{FS: fakeSkills(), fail: map[string]bool{"a-skill/SKILL.md": true}}

	_, err := installSkills(src, t.TempDir())

	if err == nil || !strings.Contains(err.Error(), "read the embedded a-skill/SKILL.md") {
		t.Fatalf("err = %v, want it to name the embedded file", err)
	}
}

func TestInstallSkillsReportsADirectoryOfTheSourceItCannotRead(t *testing.T) {
	src := errFS{FS: fakeSkills(), fail: map[string]bool{"a-skill/references": true}}

	_, err := installSkills(src, t.TempDir())

	if err == nil || !strings.Contains(err.Error(), "a-skill/references") {
		t.Fatalf("err = %v, want it to name the directory it could not read", err)
	}
}

func TestInstallSkillsReportsADirectoryItCannotCreate(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A file standing exactly where a directory of the skill has to go.
	if err := os.WriteFile(filepath.Join(dir, "a-skill", "references"), []byte("in the way\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := installSkills(fakeSkills(), dir)

	if err == nil || !strings.Contains(err.Error(), "create "+filepath.Join(dir, "a-skill", "references")) {
		t.Fatalf("err = %v, want it to name the directory it could not create", err)
	}
}

func TestInstallSkillsReportsAPathItCannotStat(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads every directory")
	}
	dir := t.TempDir()
	if _, err := installSkills(fakeSkills(), dir); err != nil {
		t.Fatalf("install: %v", err)
	}
	shut := filepath.Join(dir, "a-skill")
	if err := os.Chmod(shut, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(shut, 0o755) })

	_, err := installSkills(fakeSkills(), dir)

	if err == nil || !strings.Contains(err.Error(), "read "+filepath.Join(shut, "SKILL.md")) {
		t.Fatalf("err = %v, want it to name the path it could not read", err)
	}
}

func TestInstallSkillsReportsASymlinkItCannotReplace(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root writes into every directory")
	}
	dir := t.TempDir()
	if _, err := installSkills(fakeSkills(), dir); err != nil {
		t.Fatalf("install: %v", err)
	}
	skill := filepath.Join(dir, "a-skill")
	link := filepath.Join(skill, "SKILL.md")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "elsewhere.md"), link); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(skill, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(skill, 0o755) })

	_, err := installSkills(fakeSkills(), dir)

	if err == nil || !strings.Contains(err.Error(), "remove "+link) {
		t.Fatalf("err = %v, want it to name the file it could not remove", err)
	}
}

func TestPruneReportsATreeItCannotWalk(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads every directory")
	}
	dir := t.TempDir()
	shut := filepath.Join(dir, "references")
	if err := os.MkdirAll(shut, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(shut, 0o755) })

	_, err := prune(dir, map[string]bool{".": true, "references": true})

	if err == nil || !strings.Contains(err.Error(), "tidy "+dir) {
		t.Fatalf("err = %v, want it to say the tree could not be tidied", err)
	}
}

func TestInstallSkillsIgnoresLooseFilesAtTheRoot(t *testing.T) {
	src := fakeSkills()
	src["loose.md"] = &fstest.MapFile{Data: []byte("not a skill\n")}
	dir := t.TempDir()

	installed, err := installSkills(src, dir)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	if len(installed) != 1 || installed[0].Name != "a-skill" {
		t.Errorf("installed = %+v, want only a-skill", installed)
	}
	if _, err := os.Stat(filepath.Join(dir, "loose.md")); !os.IsNotExist(err) {
		t.Errorf("loose.md was installed")
	}
}

func TestInstallSkillsCopiesNestedDirectoriesAndPrunesThem(t *testing.T) {
	dir := t.TempDir()
	if _, err := installSkills(fakeSkills(), dir); err != nil {
		t.Fatalf("install: %v", err)
	}
	stale := filepath.Join(dir, "a-skill", "references", "gone")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "old.md"), []byte("outdated\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	installed, err := installSkills(fakeSkills(), dir)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	if installed[0].Status != statusUpdated {
		t.Errorf("status = %q, want %q", installed[0].Status, statusUpdated)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("the stale directory survived: %v", err)
	}
	want := map[string]string{
		"a-skill/SKILL.md":                  "do the thing\n",
		"a-skill/references/detail.md":      "at length\n",
		"a-skill/references/nested/more.md": "further\n",
	}
	if got := tree(t, dir); !equalTrees(got, want) {
		t.Errorf("tree = %v, want %v", got, want)
	}
}

func TestInstallSkillsReportsATreeItCannotTidy(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads every directory")
	}
	dir := t.TempDir()
	if _, err := installSkills(fakeSkills(), dir); err != nil {
		t.Fatalf("install: %v", err)
	}
	stale := filepath.Join(dir, "a-skill", "gone")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "old.md"), []byte("outdated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(stale, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(stale, 0o755) })

	_, err := installSkills(fakeSkills(), dir)

	if err == nil || !strings.Contains(err.Error(), "tidy ") {
		t.Fatalf("err = %v, want it to say the tree could not be tidied", err)
	}
}

func TestInstallSkillsReportsAFileItCannotWrite(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root writes into every directory")
	}
	dir := t.TempDir()
	skill := filepath.Join(dir, "a-skill")
	if err := os.MkdirAll(skill, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(skill, 0o755) })

	_, err := installSkills(fakeSkills(), dir)

	if err == nil || !strings.Contains(err.Error(), "SKILL.md") {
		t.Fatalf("err = %v, want it to name the file it could not write", err)
	}
}

func TestInstallSkillsReportsAFileItCannotRewrite(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads every file")
	}
	dir := t.TempDir()
	if _, err := installSkills(fakeSkills(), dir); err != nil {
		t.Fatalf("install: %v", err)
	}
	shut := filepath.Join(dir, "a-skill", "SKILL.md")
	if err := os.Chmod(shut, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(shut, 0o644) })

	_, err := installSkills(fakeSkills(), dir)

	if err == nil || !strings.Contains(err.Error(), "read "+shut) {
		t.Fatalf("err = %v, want it to name the file it could not read", err)
	}
}

func TestInstallSkillReportsAPathItCannotStat(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads every directory")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	_, err := installSkill(fakeSkills(), "a-skill", dir)

	if err == nil || !strings.Contains(err.Error(), "read "+filepath.Join(dir, "a-skill")) {
		t.Fatalf("err = %v, want it to name the path it could not read", err)
	}
}

func TestSummarizeListsWhatHappenedToEachSkill(t *testing.T) {
	got := summarize([]skillJSON{
		{Name: "next-slice", Status: statusCreated},
		{Name: "queue-work", Status: statusUnchanged},
	})
	if want := "next-slice created, queue-work unchanged"; got != want {
		t.Errorf("summarize = %q, want %q", got, want)
	}
}

// equalTrees compares two directory readings.
func equalTrees(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for path, content := range want {
		if got[path] != content {
			return false
		}
	}
	return true
}

// keys names the paths of a tree, for a failure that has to say what differed.
func keys(t map[string]string) []string {
	var names []string
	for k := range t {
		names = append(names, k)
	}
	return names
}
