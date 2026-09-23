package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// noNotionEnv is a machine with no Notion credential and no way to make a
// client: any command that reaches for either fails the test, so a pass is
// the proof a path touched Notion nowhere. Load and Save share one in-memory
// config, which is what lets a later command find the project an earlier one
// made.
func noNotionEnv(t *testing.T, cfg config.Config, found bool) (Env, *bytes.Buffer, *config.Config) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_DATA_HOME", dir)
	stubGetwd(t, "/tmp/typed-here", nil)
	state := &cfg
	var out bytes.Buffer
	return Env{
		Tokens:    config.StaticToken(""),
		Load:      func() (config.Config, bool, error) { return *state, found, nil },
		Save:      func(c config.Config) error { *state = c; found = true; return nil },
		NewClient: func(notion.TokenFunc) API { t.Fatal("a Notion client was built"); return nil },
		NewTmux:   DefaultNewTmux,
		Out:       &out,
	}, &out, state
}

func TestProjectCreateLocalTouchesNotionNowhere(t *testing.T) {
	// No config at all: the first project of a machine with no workspace.
	env, out, saved := noNotionEnv(t, config.Config{}, false)
	planDir := filepath.Join(t.TempDir(), "plans")

	err := Run(context.Background(), []string{"project-create", "Mine", "--local",
		"--plan-dir", planDir, "--repo", "/src/mine", "--description", "Be small.", "--json"}, env)
	if err != nil {
		t.Fatalf("project-create --local: %v", err)
	}

	var got projectCreatedJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	id := got.Project.ID
	if len(id) != 36 || strings.Count(id, "-") != 4 {
		t.Errorf("id %q is not shaped like a page ID", id)
	}
	if got.Project.Backend != "local" || got.Project.PlanDir != planDir || got.Project.SlicesDSID != "" {
		t.Errorf("reported %+v", got.Project)
	}
	entry := saved.Projects[id]
	want := config.ProjectConfig{Name: "Mine", WorkingDir: "/src/mine", Backend: "local", PlanDir: planDir}
	if entry != want {
		t.Errorf("config entry = %+v, want %+v", entry, want)
	}
	if entries, _ := os.ReadDir(planDir); len(entries) == 0 {
		t.Errorf("no plan file was laid down in %s", planDir)
	}
	if saved.UsesNotion() {
		t.Error("a config of one local project must not want a Notion credential")
	}

	// Worked end to end: plan it, claim it, read it, hand it back, and ask its
	// status — every command against a project no workspace has ever heard of.
	name := func(s string) string { return s }
	run := func(args ...string) string {
		t.Helper()
		out.Reset()
		if err := Run(context.Background(), append(args, "--project", id), env); err != nil {
			t.Fatalf("%s: %v", name(args[0]), err)
		}
		return out.String()
	}
	run("milestone-add", "M1")
	run("slice-add", "First slice", "--milestone", "M1", "--description", "Do the thing.")
	var brief briefJSON
	if err := json.Unmarshal([]byte(run("next-slice", "--json")), &brief); err != nil {
		t.Fatal(err)
	}
	if brief.Slice.Name != "First slice" || !strings.Contains(brief.Slice.Brief, "Do the thing.") ||
		brief.Project.Conventions != "Be small." || brief.Slice.Assignee == "" {
		t.Errorf("next-slice brief: %+v", brief)
	}
	if info := run("info"); !strings.Contains(info, "In progress") && !strings.Contains(info, "in progress") {
		t.Errorf("the slice was not claimed:\n%s", info)
	}
	run("complete-slice", brief.Slice.ID, "--branch", "slice/first", "--summary", "done")
	var status sliceStatusJSON
	if err := json.Unmarshal([]byte(run("slice-status", brief.Slice.ID, "--json")), &status); err != nil {
		t.Fatal(err)
	}
	if status.Status != "In progress" || status.Trashed {
		t.Errorf("slice-status = %+v", status)
	}
	if gone := run("slice-status", "3d938308-f654-81eb-a3c6-d4bbbf954300", "--json"); !strings.Contains(gone, "gone") {
		t.Errorf("an unknown slice should read as gone:\n%s", gone)
	}
	if md := run("slice-status", brief.Slice.ID); !strings.Contains(md, "In progress") {
		t.Errorf("slice-status markdown:\n%s", md)
	}
}

func TestProjectCreateLocalDefaultsThePlanDirectory(t *testing.T) {
	env, out, saved := noNotionEnv(t, config.Config{Projects: map[string]config.ProjectConfig{"n": {Name: "n"}}}, true)
	if err := Run(context.Background(), []string{"project-create", "Mine", "--local"}, env); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "/tmp/typed-here") || strings.Contains(out.String(), "Plan directory") {
		t.Errorf("output:\n%s", out.String())
	}
	if len(saved.Projects) != 2 {
		t.Fatalf("the existing project must be kept: %+v", saved.Projects)
	}
	for id, p := range saved.Projects {
		if p.Backend == "local" && (p.PlanDir != "" || id == "n") {
			t.Errorf("entry %s = %+v", id, p)
		}
	}
}

func TestProjectCreateLocalMarkdownNamesThePlanDirectory(t *testing.T) {
	env, out, _ := noNotionEnv(t, config.Config{}, false)
	dir := t.TempDir()
	if err := Run(context.Background(), []string{"project-create", "Mine", "--local", "--plan-dir", dir}, env); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Plan directory: "+dir) {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestProjectCreateLocalWritesNoConfigWhenThePlanCannotBeLaid(t *testing.T) {
	env, _, saved := noNotionEnv(t, config.Config{}, false)
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), []string{"project-create", "Mine", "--local", "--plan-dir", filepath.Join(file, "sub")}, env)
	if err == nil || !strings.Contains(err.Error(), "create the plan") {
		t.Fatalf("err = %v", err)
	}
	if len(saved.Projects) != 0 {
		t.Errorf("a config was written naming a project with no plan: %+v", saved.Projects)
	}
}

func TestProjectCreateLocalFailures(t *testing.T) {
	env, _, _ := noNotionEnv(t, config.Config{}, false)
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, errors.New("unreadable") }
	if err := Run(context.Background(), []string{"project-create", "Mine", "--local"}, env); err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Errorf("load failure: %v", err)
	}

	env, _, _ = noNotionEnv(t, config.Config{}, false)
	env.Save = func(config.Config) error { return errors.New("read-only") }
	if err := Run(context.Background(), []string{"project-create", "Mine", "--local"}, env); err == nil || !strings.Contains(err.Error(), "save config") {
		t.Errorf("save failure: %v", err)
	}
}

func TestProjectCreatePlanDirNeedsLocal(t *testing.T) {
	env, _, _ := noNotionEnv(t, config.Config{}, false)
	err := Run(context.Background(), []string{"project-create", "Mine", "--plan-dir", "/x"}, env)
	if err == nil || !strings.Contains(err.Error(), "--plan-dir only means something with --local") {
		t.Errorf("err = %v", err)
	}
}

func TestProjectCreateLocalReportsAPlanDirThatCannotBeResolved(t *testing.T) {
	env, _, saved := noNotionEnv(t, config.Config{}, false)
	stubGetwd(t, "", errors.New("no cwd"))
	err := Run(context.Background(), []string{"project-create", "Mine", "--local", "--repo", "/r", "--plan-dir", "relative/plans"}, env)
	if err == nil || !strings.Contains(err.Error(), "resolve --plan-dir") {
		t.Errorf("err = %v", err)
	}
	if len(saved.Projects) != 0 {
		t.Errorf("a project was recorded: %+v", saved.Projects)
	}
}

func TestAbsPlanDirExpandsAndResolves(t *testing.T) {
	stubGetwd(t, "/here", nil)
	if got, _ := absPlanDir("plans/../plans"); got != "/here/plans" {
		t.Errorf("relative not resolved against the working directory: %q", got)
	}
	if got, _ := absPlanDir("/abs/x/../plans"); got != "/abs/plans" {
		t.Errorf("absolute not cleaned: %q", got)
	}
	t.Setenv("HOME", "/home/u")
	if got, _ := absPlanDir(" ~/plans "); got != "/home/u/plans" {
		t.Errorf("home not expanded: %q", got)
	}
	if got, _ := absPlanDir(""); got != "" {
		t.Errorf("empty must stay empty: %q", got)
	}
}

// A project of nat's own has no wishlist: the two commands that read one off a
// Notion page refuse it by name rather than fail a page read.
func TestWishlistCommandsRefuseALocalProject(t *testing.T) {
	cfg := config.Config{Projects: map[string]config.ProjectConfig{
		"p-1": {Name: "Mine", WorkingDir: "/w", Backend: "local"},
	}}
	env, _, _ := noNotionEnv(t, cfg, true)
	for _, args := range [][]string{
		{"wishlist", "--project", "p-1"},
		{"wishlist-clear", "block-1", "--project", "p-1"},
	} {
		err := Run(context.Background(), args, env)
		if err == nil || !strings.Contains(err.Error(), `"Mine"`) || !strings.Contains(err.Error(), "no wishlist") {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}

// workshop-launch reads no wishlist off a project of nat's own: it launches
// the plain session, and never builds a Notion client to try.
func TestWorkshopLaunchOnALocalProjectIsAPlainSession(t *testing.T) {
	cfg := config.Config{Projects: map[string]config.ProjectConfig{
		"p-1": {Name: "Mine", WorkingDir: "/tmp/mine", Backend: "local"},
	}}
	env, out, _ := noNotionEnv(t, cfg, true)
	runner := &agentTestRunner{}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	if err := Run(context.Background(), []string{"workshop-launch", "--project", "p-1"}, env); err != nil {
		t.Fatalf("workshop-launch: %v", err)
	}
	if !strings.Contains(out.String(), agent.PlanSessionName("p-1")) || strings.Contains(out.String(), "wishlist") {
		t.Errorf("output = %q", out.String())
	}
}

// slice-status on a plan file whose store cannot be opened says so.
func TestSliceStatusOnALocalProjectReportsAnUnreadablePlan(t *testing.T) {
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Projects: map[string]config.ProjectConfig{
		"p-1": {Name: "Mine", Backend: "local", PlanDir: filepath.Join(file, "sub")},
	}}
	env, _, _ := noNotionEnv(t, cfg, true)
	err := Run(context.Background(), []string{"slice-status", "3d938308-f654-81eb-a3c6-d4bbbf954300", "--project", "p-1"}, env)
	if err == nil {
		t.Fatal("want an error")
	}
}

// Who works a local project's slices is the name, not a workspace user: with
// the config naming nobody, the command falls back to whoever is logged in
// rather than refusing for want of an assignee.
func TestALocalProjectNeedsNoConfiguredAssignee(t *testing.T) {
	env, out, saved := noNotionEnv(t, config.Config{}, false)
	if err := Run(context.Background(), []string{"project-create", "Mine", "--local"}, env); err != nil {
		t.Fatal(err)
	}
	var id string
	for k := range saved.Projects {
		id = k
	}
	out.Reset()
	if err := Run(context.Background(), []string{"milestone-add", "M", "--project", id}, env); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"slice-add", "S", "--milestone", "M", "--project", id}, env); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	err := Run(context.Background(), []string{"next-slice", "--json", "--project", id}, env)
	if err != nil && strings.Contains(err.Error(), "no assignee") {
		t.Skipf("no logged-in user is readable here: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
}

// A plan file that will not answer is an error, not a slice that is gone.
func TestSliceStatusOnALocalProjectReportsAFailedRead(t *testing.T) {
	env, out, saved := noNotionEnv(t, config.Config{}, false)
	if err := Run(context.Background(), []string{"project-create", "Mine", "--local", "--json"}, env); err != nil {
		t.Fatal(err)
	}
	var id string
	for k := range saved.Projects {
		id = k
	}
	path, err := store.PlanPath(store.ProjectOf(id, saved.Projects[id]))
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE slice_deps; DROP TABLE slices`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	out.Reset()

	err = Run(context.Background(), []string{"slice-status", "3d938308-f654-81eb-a3c6-d4bbbf954300", "--project", id}, env)
	if err == nil || !strings.Contains(err.Error(), "read the slice") {
		t.Errorf("err = %v", err)
	}
}
