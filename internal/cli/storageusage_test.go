package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/source"
)

// fakeStorage answers storage-usage's GitHub read with a fixed reading.
type fakeStorage struct {
	reading gh.StorageReading
	err     error
	asked   time.Time
}

func (f *fakeStorage) ArtifactStorage(now time.Time) (gh.StorageReading, error) {
	f.asked = now
	return f.reading, f.err
}

// remoteGit answers `git remote get-url origin` by directory; any other
// directory has no origin.
type remoteGit struct{ remotes map[string]string }

func (r remoteGit) Run(dir, _ string, args ...string) (string, error) {
	if len(args) >= 2 && args[0] == "remote" && args[1] == "get-url" {
		if url, ok := r.remotes[dir]; ok {
			return url + "\n", nil
		}
	}
	return "", errors.New("no such remote")
}

// stubStorage points storage-usage at fake and pins the clock to 10 October
// 2026, midday UTC.
func stubStorage(t *testing.T, fake *fakeStorage) time.Time {
	t.Helper()
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	oldReader, oldNow := newStorageReader, storageNow
	t.Cleanup(func() { newStorageReader, storageNow = oldReader, oldNow })
	newStorageReader = func() StorageReader { return fake }
	storageNow = func() time.Time { return now }
	return now
}

func TestStorageUsageSharesTheMonthOutByProject(t *testing.T) {
	sp := newSourceProject(t, &source.Fake{Details: map[string]source.ContainerDetail{"c1": {ID: "c1", Title: "Card"}}})
	task := sp.addTask(t, "Task", "c1")
	cardRepo := t.TempDir()
	sp.run(t, "slice-repo", task, "--repo", cardRepo, "--project", sp.id)

	home := t.TempDir()
	t.Setenv("HOME", home)
	sp.saved.Projects["p-nat"] = config.ProjectConfig{Name: "nat", WorkingDir: "/src/nat", Color: "blue"}
	// Claims nat's repository too: it went to nat, first by name.
	sp.saved.Projects["p-twin"] = config.ProjectConfig{Name: "twin", WorkingDir: "~/twin", Color: "red"}
	sp.saved.Projects["p-none"] = config.ProjectConfig{Name: "loose", WorkingDir: "/src/loose", Color: "green"}
	sp.env.NewGit = func() GitCLI {
		return git.NewWithRunner(remoteGit{remotes: map[string]string{
			"/src/nat":                   "git@github.com:Craig/Nat.git",
			filepath.Join(home, "twin"): "https://github.com/craig/nat",
			cardRepo:                     "https://github.com/craig/cards.git",
		}})
	}
	fake := &fakeStorage{reading: gh.StorageReading{
		Login: "craig", Plan: "pro", AllowanceGB: 1,
		Repos: map[string]float64{"craig/nat": 0.5, "craig/cards": 0.2, "craig/old": 0.1, "craig/older": 0.15},
	}}
	now := stubStorage(t, fake)

	var doc storageDoc
	if err := json.Unmarshal([]byte(sp.run(t, "storage-usage", "--json")), &doc); err != nil {
		t.Fatal(err)
	}
	if !fake.asked.Equal(now) {
		t.Errorf("read for %v, want %v", fake.asked, now)
	}
	if doc.Login != "craig" || doc.Plan != "pro" || doc.AllowanceGB != 1 || doc.Year != 2026 || doc.Month != 10 || doc.DaysLeft != 22 {
		t.Errorf("header = %+v", doc)
	}
	if math.Abs(doc.TotalGB-0.95) > 1e-9 {
		t.Errorf("total = %v, want 0.95", doc.TotalGB)
	}
	type row struct {
		name, color string
		repos       string
		gb          float64
	}
	want := []row{
		{"nat", "blue", "craig/nat", 0.5},
		{"Demo source", "", "craig/cards", 0.2},
		{"loose", "green", "", 0},
		{"twin", "red", "craig/nat", 0},
	}
	if len(doc.Projects) != len(want) {
		t.Fatalf("projects = %+v, want %d", doc.Projects, len(want))
	}
	for i, w := range want {
		p := doc.Projects[i]
		if p.Name != w.name || p.Color != w.color || strings.Join(p.Repos, ",") != w.repos || math.Abs(p.GB-w.gb) > 1e-9 {
			t.Errorf("project %d = %+v, want %+v", i, p, w)
		}
	}
	if math.Abs(doc.Other.GB-0.25) > 1e-9 || len(doc.Other.Repos) != 2 ||
		doc.Other.Repos[0].Repo != "craig/older" || doc.Other.Repos[1].Repo != "craig/old" {
		t.Errorf("other = %+v, want craig/older then craig/old, 0.25", doc.Other)
	}

	text := sp.run(t, "storage-usage")
	for _, line := range []string{
		"Artifact storage 2026-10: 0.95 GB of 1.00 GB · 22 days left",
		"  nat  0.50 GB",
		"  Other repositories  0.25 GB",
	} {
		if !strings.Contains(text, line+"\n") {
			t.Errorf("text lacks %q:\n%s", line, text)
		}
	}
}

func TestStorageUsageWithNoProjectsOrAllowance(t *testing.T) {
	env, out, _ := noNotionEnv(t, config.Config{}, false)
	stubStorage(t, &fakeStorage{reading: gh.StorageReading{Login: "craig", Repos: map[string]float64{}}})

	if err := Run(context.Background(), []string{"storage-usage", "--json"}, env); err != nil {
		t.Fatal(err)
	}
	// Always arrays, never null: gnat ranges over both.
	if s := out.String(); !strings.Contains(s, `"projects": []`) || !strings.Contains(s, `"repos": []`) {
		t.Errorf("json = %s", s)
	}
	out.Reset()
	if err := Run(context.Background(), []string{"storage-usage"}, env); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "Artifact storage 2026-10: 0.00 GB no known allowance · 22 days left\n") {
		t.Errorf("text = %q", out.String())
	}
}

func TestStorageUsageRefusals(t *testing.T) {
	env, _, _ := noNotionEnv(t, config.Config{}, false)
	stubStorage(t, &fakeStorage{err: gh.ErrBillingScope})

	// The text form refuses, saying the command; the JSON form answers the
	// scope and the command, for gnat to offer.
	err := Run(context.Background(), []string{"storage-usage"}, env)
	if !errors.Is(err, gh.ErrBillingScope) || !strings.Contains(err.Error(), gh.ScopeCommand) {
		t.Errorf("err = %v, want the scope refusal", err)
	}
	var out strings.Builder
	env.Out = &out
	if err := Run(context.Background(), []string{"storage-usage", "--json"}, env); err != nil {
		t.Fatalf("--json: %v", err)
	}
	var scope storageScopeJSON
	if err := json.Unmarshal([]byte(out.String()), &scope); err != nil || scope != (storageScopeJSON{NeedsScope: "user", ScopeCommand: gh.ScopeCommand}) {
		t.Errorf("--json = %s (%v)", out.String(), err)
	}
	stubStorage(t, &fakeStorage{err: errors.New("offline")})
	if err := Run(context.Background(), []string{"storage-usage", "--json"}, env); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Errorf("err = %v, want the read's own", err)
	}
	for _, args := range [][]string{{"storage-usage", "extra"}, {"storage-usage", "--project", "x"}} {
		var usage *UsageError
		if err := Run(context.Background(), args, env); !errors.As(err, &usage) {
			t.Errorf("%v: err = %v, want a usage error", args, err)
		}
	}
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, errors.New("bad config") }
	if err := Run(context.Background(), []string{"storage-usage"}, env); err == nil || !strings.Contains(err.Error(), "bad config") {
		t.Errorf("err = %v, want the config's", err)
	}
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil }
	env.Out = failingWriter{}
	stubStorage(t, &fakeStorage{reading: gh.StorageReading{Repos: map[string]float64{}}})
	if err := Run(context.Background(), []string{"storage-usage"}, env); err == nil {
		t.Error("a failed write was not reported")
	}
}

// A source project whose plan cannot be opened claims nothing — the rest of
// the reading stands.
func TestStorageUsageSourcePlanUnreadable(t *testing.T) {
	env, out, _ := noNotionEnv(t, config.Config{Projects: map[string]config.ProjectConfig{
		"p-src": {Backend: config.BackendSource, Source: "demo", PlanDir: filepath.Join(t.TempDir(), "missing", "\x00")},
	}}, true)
	env.NewSource = func(string) (source.Client, error) { return nil, errors.New("gone") }
	stubStorage(t, &fakeStorage{reading: gh.StorageReading{Repos: map[string]float64{"a/b": 1}}})

	if err := Run(context.Background(), []string{"storage-usage", "--json"}, env); err != nil {
		t.Fatal(err)
	}
	var doc storageDoc
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Projects) != 1 || doc.Projects[0].Name != "demo" || len(doc.Projects[0].Repos) != 0 || doc.Other.GB != 1 {
		t.Errorf("doc = %+v", doc)
	}
}

// Two projects of one name go in ID order, one directory is asked of git
// once, and repositories of equal storage go in name order.
func TestStorageUsageTiesAndSharedDirectories(t *testing.T) {
	env, out, _ := noNotionEnv(t, config.Config{Projects: map[string]config.ProjectConfig{
		"p-b": {Name: "same", WorkingDir: "/src/x"},
		"p-a": {Name: "same", WorkingDir: "/src/x"},
	}}, true)
	asked := 0
	env.NewGit = func() GitCLI {
		return git.NewWithRunner(countingGit{remoteGit{remotes: map[string]string{"/src/x": "git@github.com:o/x.git"}}, &asked})
	}
	stubStorage(t, &fakeStorage{reading: gh.StorageReading{Repos: map[string]float64{"o/x": 1, "o/z": 0.5, "o/y": 0.5}}})

	if err := Run(context.Background(), []string{"storage-usage", "--json"}, env); err != nil {
		t.Fatal(err)
	}
	var doc storageDoc
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if asked != 1 {
		t.Errorf("git asked %d times for one directory, want 1", asked)
	}
	if doc.Projects[0].ID != "p-a" || doc.Projects[0].GB != 1 || doc.Projects[1].GB != 0 {
		t.Errorf("projects = %+v, want p-a holding o/x", doc.Projects)
	}
	if doc.Other.Repos[0].Repo != "o/y" || doc.Other.Repos[1].Repo != "o/z" {
		t.Errorf("other = %+v, want o/y then o/z", doc.Other.Repos)
	}
}

// countingGit is remoteGit counting every call.
type countingGit struct {
	remoteGit
	n *int
}

func (c countingGit) Run(dir, name string, args ...string) (string, error) {
	*c.n++
	return c.remoteGit.Run(dir, name, args...)
}

// A source project whose plan opens but will not read claims nothing either.
func TestStorageUsageSourcePlanBroken(t *testing.T) {
	sp := newSourceProject(t, &source.Fake{})
	db, err := sql.Open("sqlite3", "file:"+sp.planPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE milestones`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	stubStorage(t, &fakeStorage{reading: gh.StorageReading{Repos: map[string]float64{}}})

	var doc storageDoc
	if err := json.Unmarshal([]byte(sp.run(t, "storage-usage", "--json")), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Projects) != 1 || len(doc.Projects[0].Repos) != 0 {
		t.Errorf("projects = %+v", doc.Projects)
	}
}

func TestDefaultStorageReaderKeepsNoBudget(t *testing.T) {
	if r, ok := newStorageReader().(gh.CLI); !ok || r != gh.NewWithRunner(gh.ExecRunner{}) {
		t.Errorf("newStorageReader() = %#v, want a budgetless gh.CLI", r)
	}
}

func TestDaysLeftInMonth(t *testing.T) {
	for _, tc := range []struct {
		now  time.Time
		want int
	}{
		{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), 31},
		{time.Date(2026, 10, 31, 23, 0, 0, 0, time.UTC), 1},
		{time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC), 1},
	} {
		if got := daysLeftInMonth(tc.now); got != tc.want {
			t.Errorf("daysLeftInMonth(%v) = %d, want %d", tc.now, got, tc.want)
		}
	}
}
