package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/source"
)

// demoDescribe is a plugin's describe in this build's protocol.
func demoDescribe() source.Describe {
	return source.Describe{Protocol: 1, Name: "demo", Title: "Demo source", Tag: "DM", IconSymbol: "rect",
		ContainerNoun: "card", TaskNoun: "task", Menu: []source.Action{{ID: "refresh", Label: "Refresh", Input: source.InputNone}}}
}

// sourceProject is a source project made through project-create --source over
// fake, on a machine with no Notion credential and no way to build a client —
// so every test that runs a command against one is also the proof the command
// touched Notion nowhere.
type sourceProject struct {
	env     Env
	out     *bytes.Buffer
	saved   *config.Config
	id      string
	planDir string
	fake    *source.Fake
	nudges  int
}

func newSourceProject(t *testing.T, fake *source.Fake) *sourceProject {
	t.Helper()
	if fake.DescribeResult.Protocol == 0 {
		fake.DescribeResult = demoDescribe()
	}
	env, out, saved := noNotionEnv(t, config.Config{}, false)
	sp := &sourceProject{env: env, out: out, saved: saved, fake: fake, planDir: filepath.Join(t.TempDir(), "plans")}
	sp.env.NewGit = func() GitCLI { return git.NewWithRunner(&fakeGitRunner{}) }
	sp.env.NewSource = func(name string) (source.Client, error) {
		if name != "demo" {
			t.Errorf("NewSource(%q), want demo", name)
		}
		return fake, nil
	}
	sp.env.Nudge = func() { sp.nudges++ }
	sp.run(t, "project-create", "Work", "--source", "demo", "--plan-dir", sp.planDir, "--json")
	var created projectCreatedJSON
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatalf("project-create --source: not JSON: %v\n%s", err, out.String())
	}
	sp.id = created.Project.ID
	return sp
}

// run runs a command that must succeed and returns its output alone.
func (sp *sourceProject) run(t *testing.T, args ...string) string {
	t.Helper()
	sp.out.Reset()
	if err := Run(context.Background(), args, sp.env); err != nil {
		t.Fatalf("%s: %v", strings.Join(args, " "), err)
	}
	return sp.out.String()
}

// fail runs a command that must fail and returns its error.
func (sp *sourceProject) fail(t *testing.T, args ...string) error {
	t.Helper()
	sp.out.Reset()
	err := Run(context.Background(), args, sp.env)
	if err == nil {
		t.Fatalf("%s: succeeded, want an error", strings.Join(args, " "))
	}
	return err
}

// addTask files a task under container and returns its ID.
func (sp *sourceProject) addTask(t *testing.T, title, container string) string {
	t.Helper()
	var added sliceAddedJSON
	if err := json.Unmarshal([]byte(sp.run(t, "slice-add", title, "--container", container, "--project", sp.id, "--json")), &added); err != nil {
		t.Fatal(err)
	}
	return added.Slice.ID
}

// planPath is the project's plan file.
func (sp *sourceProject) planPath(t *testing.T) string {
	t.Helper()
	matches, _ := filepath.Glob(filepath.Join(sp.planDir, "*.db"))
	if len(matches) != 1 {
		t.Fatalf("plan files in %s = %v, want one", sp.planDir, matches)
	}
	return matches[0]
}

// breakPlan drops the slices table out from under the plan, so the next read
// of the plan fails while the file still opens.
func (sp *sourceProject) breakPlan(t *testing.T) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+sp.planPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`DROP TABLE slices`); err != nil {
		t.Fatal(err)
	}
}

func intp(n int) *int { return &n }

func TestDefaultNewSource(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	t.Setenv("PATH", t.TempDir())

	_, err := DefaultNewSource("demo")
	want := `no task source plugin named "demo" — expected nat-source-demo under ` +
		filepath.Join(cfgDir, "notion-agent-tracker", "plugins", "demo") + " or on PATH"
	if err == nil || err.Error() != want {
		t.Errorf("DefaultNewSource() missing = %v, want %q", err, want)
	}

	dir := filepath.Join(cfgDir, "notion-agent-tracker", "plugins", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nat-source-demo"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := DefaultNewSource("demo")
	if e, ok := src.(*source.Exec); err != nil || !ok || e.Path != filepath.Join(dir, "nat-source-demo") {
		t.Errorf("DefaultNewSource() = %#v, %v, want the installed plugin", src, err)
	}
}

func TestDefaultNewSourceReportsAFailedLookup(t *testing.T) {
	// A plugins "dir" that is a file cannot be read.
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	if err := os.MkdirAll(filepath.Join(cfgDir, "notion-agent-tracker"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "notion-agent-tracker", "plugins"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := DefaultNewSource("demo"); err == nil || !strings.Contains(err.Error(), "look for task source plugins") {
		t.Errorf("DefaultNewSource() = %v, want the lookup failure", err)
	}

	// No config dir to resolve at all.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	if _, err := DefaultNewSource("demo"); err == nil {
		t.Error("DefaultNewSource() with no home = nil, want an error")
	}
}

func TestProjectCreateSourceWritesThePlanThenTheConfig(t *testing.T) {
	sp := newSourceProject(t, &source.Fake{})

	entry := sp.saved.Projects[sp.id]
	want := config.ProjectConfig{Name: "Work", Backend: config.BackendSource, Source: "demo", PlanDir: sp.planDir}
	if entry != want {
		t.Errorf("config entry = %+v, want %+v", entry, want)
	}
	sp.planPath(t)
	if sp.saved.UsesNotion() {
		t.Error("a config of one source project must not want a Notion credential")
	}

	// The JSON says what was made.
	sp.out.Reset()
	sp.run(t, "project-create", "Other", "--source", "demo", "--plan-dir", sp.planDir, "--json")
	var got projectCreatedJSON
	if err := json.Unmarshal(sp.out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Project.Backend != "source" || got.Project.Source != "demo" || got.Project.PlanDir != sp.planDir {
		t.Errorf("reported %+v", got.Project)
	}

	// The text form names the plugin by its title, or its name with none.
	if out := sp.run(t, "project-create", "Third", "--source", "demo", "--plan-dir", sp.planDir); !strings.Contains(out, "Demo source's containers (nat-source-demo)") ||
		!strings.Contains(out, "- Plan directory: "+sp.planDir) {
		t.Errorf("text = %q", out)
	}
	sp.fake.DescribeResult.Title = ""
	if out := sp.run(t, "project-create", "Fourth", "--source", "demo"); !strings.Contains(out, "under demo's containers") ||
		strings.Contains(out, "Plan directory") {
		t.Errorf("text = %q", out)
	}
}

// A source project takes no --repo: its tasks each name their own.
func TestProjectCreateSourceRefusesARepo(t *testing.T) {
	sp := newSourceProject(t, &source.Fake{})
	err := sp.fail(t, "project-create", "Other", "--source", "demo", "--repo", "/src/app")
	if err == nil || !strings.Contains(err.Error(), "--repo means nothing with --source") {
		t.Errorf("err = %v, want --repo refused", err)
	}
	if len(sp.saved.Projects) != 1 {
		t.Errorf("projects = %+v, want nothing more written", sp.saved.Projects)
	}
}

func TestProjectCreateSourceRefusesBeforeWritingAnything(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(env *Env)
		args  []string
		want  string
	}{
		{"with --local", nil, []string{"--source", "demo", "--local"}, "--local and --source are mutually exclusive"},
		{"plugin missing", func(env *Env) {
			env.NewSource = func(string) (source.Client, error) { return nil, errors.New("no task source plugin named \"demo\"") }
		}, []string{"--source", "demo"}, `project-create: no task source plugin named "demo"`},
		{"describe fails", func(env *Env) {
			env.NewSource = func(string) (source.Client, error) { return &source.Fake{DescribeErr: errors.New("no token")}, nil }
		}, []string{"--source", "demo"}, "project-create: no token"},
		{"another protocol", func(env *Env) {
			env.NewSource = func(string) (source.Client, error) {
				return &source.Fake{DescribeResult: source.Describe{Protocol: 2}}, nil
			}
		}, []string{"--source", "demo"}, "source plugin demo speaks protocol 2; this nat speaks protocol 1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env, _, saved := noNotionEnv(t, config.Config{}, false)
			env.NewSource = func(string) (source.Client, error) { t.Fatal("NewSource called"); return nil, nil }
			if tt.setup != nil {
				tt.setup(&env)
			}
			planDir := filepath.Join(t.TempDir(), "plans")
			err := Run(context.Background(), append([]string{"project-create", "Work", "--plan-dir", planDir}, tt.args...), env)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
			if len(saved.Projects) != 0 {
				t.Errorf("config gained %v", saved.Projects)
			}
			if _, err := os.Stat(planDir); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("a plan dir was laid down: %v", err)
			}
		})
	}
}

func TestProjectCreateSourceReportsAPlanItCouldNotLayDown(t *testing.T) {
	env, _, saved := noNotionEnv(t, config.Config{}, false)
	env.NewSource = func(string) (source.Client, error) { return &source.Fake{DescribeResult: demoDescribe()}, nil }
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), []string{"project-create", "Work", "--source", "demo", "--plan-dir", filepath.Join(file, "plans")}, env)
	if err == nil || !strings.Contains(err.Error(), "create the plan") {
		t.Errorf("err = %v, want the plan refused", err)
	}
	if len(saved.Projects) != 0 {
		t.Errorf("config gained %v", saved.Projects)
	}
}

func TestSliceAddContainer(t *testing.T) {
	fake := &source.Fake{Details: map[string]source.ContainerDetail{
		"c1": {ID: "c1", Title: "Improve diff review"},
		"c2": {ID: "c2", Title: "  "},
	}}
	sp := newSourceProject(t, fake)

	// A container new to the plan is read for its title, and the plugin told.
	first := sp.addTask(t, "Diff comments", "c1")
	if !reflect.DeepEqual(fake.ContainerIDs, []string{"c1"}) {
		t.Errorf("container reads = %v, want c1 once", fake.ContainerIDs)
	}
	if len(fake.Events) != 1 || fake.Events[0].Event != source.EventCreated || fake.Events[0].Container != "c1" || fake.Events[0].Task.ID != first {
		t.Errorf("events = %+v, want created under c1", fake.Events)
	}

	// One already in the plan is filed under directly, by its cached title.
	out := sp.run(t, "slice-add", "Highlighting", "--container", "c1", "--project", sp.id)
	if !strings.Contains(out, "Added to Improve diff review") || len(fake.ContainerIDs) != 1 {
		t.Errorf("out = %q, reads = %v", out, fake.ContainerIDs)
	}

	// A plugin titling it as nothing leaves it named by its id.
	var added sliceAddedJSON
	if err := json.Unmarshal([]byte(sp.run(t, "slice-add", "Mouse", "--container", "c2", "--project", sp.id, "--json")), &added); err != nil {
		t.Fatal(err)
	}
	if added.Slice.MilestoneID != "c2" || added.Slice.MilestoneName != "c2" {
		t.Errorf("added = %+v, want filed under c2 named c2", added.Slice)
	}

	// A new container the plugin cannot read is refused, nothing filed.
	fake.ContainerErr = errors.New("plugin down")
	if err := sp.fail(t, "slice-add", "Lost", "--container", "c9", "--project", sp.id); !strings.Contains(err.Error(), "read container c9: plugin down") {
		t.Errorf("err = %v", err)
	}
	if len(fake.Events) != 3 {
		t.Errorf("events = %d, want the three adds alone", len(fake.Events))
	}
}

func TestSliceAddContainerRefusals(t *testing.T) {
	sp := newSourceProject(t, &source.Fake{})
	var usage *UsageError
	if err := sp.fail(t, "slice-add", "X", "--milestone", "M1", "--project", sp.id); !errors.As(err, &usage) || !strings.Contains(err.Error(), "pass --container, not --milestone") {
		t.Errorf("--milestone on a source project = %v", err)
	}
	if err := sp.fail(t, "slice-add", "X", "--project", sp.id); !errors.As(err, &usage) || !strings.Contains(err.Error(), "no container given") {
		t.Errorf("no --container = %v", err)
	}

	env, _ := testEnv(testConfig(t), &fakeAPI{})
	err := Run(context.Background(), []string{"slice-add", "X", "--container", "c1", "--project", "project-1"}, env)
	if !errors.As(err, &usage) || !strings.Contains(err.Error(), `--container only means something on a source project, and "nat" is not one`) {
		t.Errorf("--container on a Notion project = %v", err)
	}
}

func TestInfoCarriesTheSourceAndItsUnlistedGroup(t *testing.T) {
	fake := &source.Fake{
		Details: map[string]source.ContainerDetail{"c1": {Title: "Listed"}, "c2": {Title: "Scrolled away"}, "c3": {Title: "Empty"}},
		Groups: []source.Group{
			{ID: "doing", Label: "Doing", Count: intp(1), Containers: []source.Container{{ID: "c1", Title: "Listed (remote title)"}}},
			{ID: "ready", Label: "Ready", Children: []source.Group{{ID: "mine", Label: "Mine", Containers: []source.Container{{ID: "c3"}}}}},
		},
	}
	sp := newSourceProject(t, fake)
	sp.addTask(t, "A", "c1")
	sp.addTask(t, "B", "c2")

	var doc infoJSON
	if err := json.Unmarshal([]byte(sp.run(t, "info", "--json", "--expand", "done", "--expand", "later", "--project", sp.id)), &doc); err != nil {
		t.Fatal(err)
	}
	if got := fake.Expands[len(fake.Expands)-1]; !reflect.DeepEqual(got, []string{"done", "later"}) {
		t.Errorf("expand = %v, want both groups passed through", got)
	}
	src := doc.Source
	if src == nil || src.Name != "demo" || src.Title != "Demo source" || src.Tag != "DM" || src.ContainerNoun != "card" ||
		src.TaskNoun != "task" || len(src.Menu) != 1 || src.Error != "" {
		t.Fatalf("source = %+v", src)
	}
	want := append(append([]source.Group{}, fake.Groups...), source.Group{
		ID: "_unlisted", Label: "Other cards", Count: intp(1),
		Containers: []source.Container{{ID: "c2", Title: "Scrolled away"}},
	})
	if !reflect.DeepEqual(src.Groups, want) {
		t.Errorf("groups = %+v, want %+v", src.Groups, want)
	}
	// Containers still appear among the milestones, by their ids.
	if len(doc.Milestones) != 2 || doc.Milestones[0].ID != "c1" || doc.Milestones[1].ID != "c2" {
		t.Errorf("milestones = %+v", doc.Milestones)
	}

	// With every container listed, there is no _unlisted group, and no
	// --expand is no expand.
	fake.Groups = append(fake.Groups, source.Group{ID: "done", Label: "Done", Containers: []source.Container{{ID: "c2"}}})
	if err := json.Unmarshal([]byte(sp.run(t, "info", "--json", "--project", sp.id)), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Source.Groups) != 3 || fake.Expands[len(fake.Expands)-1] != nil {
		t.Errorf("groups = %+v, expand = %v", doc.Source.Groups, fake.Expands[len(fake.Expands)-1])
	}

	// A sidebar that sends a header menu replaces describe's static one.
	filter := source.Action{ID: "filter", Label: "Filter…", Input: source.InputFilter,
		Fields: []source.FilterField{{ID: "team", Label: "Team", Options: []source.FilterOption{}, Value: []string{}}}}
	fake.SidebarMenu = []source.Action{filter}
	if err := json.Unmarshal([]byte(sp.run(t, "info", "--json", "--project", sp.id)), &doc); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc.Source.Menu, []source.Action{filter}) {
		t.Errorf("menu = %+v, want the sidebar's", doc.Source.Menu)
	}
}

func TestInfoConcludesNothingFromAFailedPluginRead(t *testing.T) {
	fake := &source.Fake{Details: map[string]source.ContainerDetail{"c1": {Title: "Cached"}}}
	sp := newSourceProject(t, fake)
	sp.addTask(t, "A", "c1")
	unlisted := []source.Group{{ID: "_unlisted", Label: "Other cards", Count: intp(1),
		Containers: []source.Container{{ID: "c1", Title: "Cached"}}}}

	// The sidebar fails: describe's fields stand, the tree is _unlisted alone.
	fake.SidebarErr = errors.New("rate limited")
	var doc infoJSON
	if err := json.Unmarshal([]byte(sp.run(t, "info", "--json", "--project", sp.id)), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Source.Error != "rate limited" || doc.Source.Tag != "DM" || !reflect.DeepEqual(doc.Source.Groups, unlisted) {
		t.Errorf("source = %+v", doc.Source)
	}

	// Describe fails: nothing read, nouns default, no sidebar read at all.
	fake.DescribeErr = errors.New("no token")
	reads := len(fake.Expands)
	if err := json.Unmarshal([]byte(sp.run(t, "info", "--json", "--project", sp.id)), &doc); err != nil {
		t.Fatal(err)
	}
	unlisted[0].Label = "Other containers"
	if doc.Source.Error != "no token" || doc.Source.Name != "demo" || doc.Source.Tag != "" || doc.Source.ContainerNoun != "container" ||
		doc.Source.TaskNoun != "task" || doc.Source.Menu == nil || !reflect.DeepEqual(doc.Source.Groups, unlisted) {
		t.Errorf("source = %+v", doc.Source)
	}
	if len(fake.Expands) != reads {
		t.Error("the sidebar was read after describe failed")
	}

	// A plugin that cannot even be found: the project still opens.
	sp.env.NewSource = func(string) (source.Client, error) { return nil, errors.New("not installed") }
	if err := json.Unmarshal([]byte(sp.run(t, "info", "--json", "--project", sp.id)), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Source.Error != "not installed" || len(doc.Slices) != 1 {
		t.Errorf("source = %+v, slices = %d", doc.Source, len(doc.Slices))
	}
}

func TestInfoHasNoSourceForAnyOtherProject(t *testing.T) {
	env, out := testEnv(testConfig(t), &fakeAPI{})
	if err := Run(context.Background(), []string{"info", "--json", "--project", "project-1"}, env); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), `"source"`) {
		t.Errorf("info = %s, want no source", out.String())
	}
}

func TestContainerShow(t *testing.T) {
	detail := source.ContainerDetail{ID: "c1", Title: "Improve diff review", ExternalURL: "https://x/c1",
		Facts: []source.Fact{{Label: "state", Value: "Doing"}}}
	fake := &source.Fake{Details: map[string]source.ContainerDetail{"c1": detail, "c2": {Title: "Other"}}}
	sp := newSourceProject(t, fake)
	a := sp.addTask(t, "A", "c1")
	sp.addTask(t, "B", "c2")

	var doc containerShowJSON
	if err := json.Unmarshal([]byte(sp.run(t, "container-show", "c1", "--json", "--project", sp.id)), &doc); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc.Container, detail) || len(doc.Tasks) != 1 || doc.Tasks[0].ID != a ||
		doc.Tasks[0].Status != "Todo" || doc.Tasks[0].MilestoneID != "c1" {
		t.Errorf("container-show = %+v", doc)
	}

	text := sp.run(t, "container-show", "c1", "--project", sp.id)
	want := "# Improve diff review\n\nhttps://x/c1\n\n- state: Doing\n\n## Tasks\n\n- A — Todo\n"
	if text != want {
		t.Errorf("text =\n%s\nwant\n%s", text, want)
	}
	fake.Details["c3"] = source.ContainerDetail{Title: "Nothing yet"}
	if text := sp.run(t, "container-show", "c3", "--project", sp.id); text != "# Nothing yet\n\n\n## Tasks\n\n_none_\n" {
		t.Errorf("text = %q", text)
	}
}

func TestContainerShowRefusals(t *testing.T) {
	fake := &source.Fake{}
	sp := newSourceProject(t, fake)
	var usage *UsageError
	if err := sp.fail(t, "container-show", "--project", sp.id); !errors.As(err, &usage) {
		t.Errorf("no id = %v", err)
	}
	if err := sp.fail(t, "container-show", "c1", "--bogus"); !errors.As(err, &usage) {
		t.Errorf("bad flag = %v", err)
	}
	if err := sp.fail(t, "container-show", "c1"); !strings.Contains(err.Error(), "no project given") {
		t.Errorf("no project = %v", err)
	}
	fake.ContainerErr = errors.New("no card c1")
	if err := sp.fail(t, "container-show", "c1", "--project", sp.id); err.Error() != "no card c1" {
		t.Errorf("plugin failure = %v", err)
	}
	fake.ContainerErr = nil
	sp.breakPlan(t)
	if err := sp.fail(t, "container-show", "c1", "--project", sp.id); !strings.Contains(err.Error(), "slices") {
		t.Errorf("plan failure = %v", err)
	}

	env, _ := testEnv(testConfig(t), &fakeAPI{})
	err := Run(context.Background(), []string{"container-show", "c1", "--project", "project-1"}, env)
	if err == nil || err.Error() != `container-show: "nat" is not a source project` {
		t.Errorf("Notion project = %v", err)
	}
}

func TestSourceStoreForReportsAPlanThatWillNotOpen(t *testing.T) {
	sp := newSourceProject(t, &source.Fake{})
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	entry := sp.saved.Projects[sp.id]
	entry.PlanDir = filepath.Join(file, "plans")
	sp.saved.Projects[sp.id] = entry
	if err := sp.fail(t, "container-show", "c1", "--project", sp.id); !strings.Contains(err.Error(), "a-file") {
		t.Errorf("err = %v, want the plan's open failure", err)
	}
}

func TestSourceAction(t *testing.T) {
	fake := &source.Fake{ActionResult: source.ActionResult{Message: "Commented."}}
	sp := newSourceProject(t, fake)

	if out := sp.run(t, "source-action", "--action", "comment", "--container", "c1", "--input", "hi", "--json", "--project", sp.id); out != "{\n  \"message\": \"Commented.\"\n}\n" {
		t.Errorf("json = %q", out)
	}
	sp.env.In = strings.NewReader("line one\nline two\n")
	if out := sp.run(t, "source-action", "--action", "comment", "--group", "g", "--input", "-", "--project", sp.id); out != "Commented.\n" {
		t.Errorf("text = %q", out)
	}
	fake.ActionResult = source.ActionResult{}
	if out := sp.run(t, "source-action", "--action", "refresh", "--project", sp.id); out != "Ran refresh.\n" {
		t.Errorf("text = %q", out)
	}
	want := []source.ActionCall{
		{Action: "comment", Target: source.Target{Container: "c1"}, Input: "hi"},
		{Action: "comment", Target: source.Target{Group: "g"}, Input: "line one\nline two"},
		{Action: "refresh"},
	}
	for i := range fake.Actions {
		fake.Actions[i].Project = source.Project{}
	}
	if !reflect.DeepEqual(fake.Actions, want) {
		t.Errorf("actions = %+v, want %+v", fake.Actions, want)
	}
	if sp.nudges < 3 {
		t.Errorf("nudges = %d, want one per action", sp.nudges)
	}
}

func TestSourceActionRefusals(t *testing.T) {
	fake := &source.Fake{}
	sp := newSourceProject(t, fake)
	var usage *UsageError
	for _, args := range [][]string{
		{"stray"},
		{"--bogus"},
		{"--project", sp.id},
		{"--action", "x", "--group", "g", "--container", "c", "--project", sp.id},
		{"--action", "x", "--input", "-", "--project", sp.id},
	} {
		if err := sp.fail(t, append([]string{"source-action"}, args...)...); !errors.As(err, &usage) {
			t.Errorf("%v: err = %v, want a usage error", args, err)
		}
	}
	if err := sp.fail(t, "source-action", "--action", "x"); !strings.Contains(err.Error(), "no project given") {
		t.Errorf("no project = %v", err)
	}
	fake.ActionErr = errors.New("not allowed")
	before := sp.nudges
	if err := sp.fail(t, "source-action", "--action", "x", "--project", sp.id); err.Error() != "not allowed" {
		t.Errorf("plugin failure = %v", err)
	}
	if sp.nudges != before {
		t.Error("a failed action nudged")
	}

	env, _ := testEnv(testConfig(t), &fakeAPI{})
	err := Run(context.Background(), []string{"source-action", "--action", "x", "--project", "project-1"}, env)
	if err == nil || err.Error() != `source-action: "nat" is not a source project` {
		t.Errorf("Notion project = %v", err)
	}
}

func TestSliceShowCarriesTheContainer(t *testing.T) {
	fake := &source.Fake{Details: map[string]source.ContainerDetail{"c1": {
		ID: "c1", Title: "Remote title", ExternalURL: "https://x/c1", TaskNote: "Linked.",
		Facts: []source.Fact{{Label: "estimate", Value: "3"}},
	}}}
	sp := newSourceProject(t, fake)
	fake.Details["c1"] = source.ContainerDetail{Title: "Cached title"}
	task := sp.addTask(t, "A", "c1")
	fake.Details["c1"] = source.ContainerDetail{ID: "c1", Title: "Remote title", ExternalURL: "https://x/c1", TaskNote: "Linked.",
		Facts: []source.Fact{{Label: "estimate", Value: "3"}}}

	var got sliceShowJSON
	if err := json.Unmarshal([]byte(sp.run(t, "slice-show", task, "--json", "--project", sp.id)), &got); err != nil {
		t.Fatal(err)
	}
	want := &sliceContainerJSON{ID: "c1", Title: "Remote title", ExternalURL: "https://x/c1", TaskNote: "Linked.",
		Facts: []source.Fact{{Label: "estimate", Value: "3"}}}
	if !reflect.DeepEqual(got.Container, want) {
		t.Errorf("container = %+v, want %+v", got.Container, want)
	}

	// A failed read falls back to the id and the cached title.
	fake.ContainerErr = errors.New("down")
	got = sliceShowJSON{}
	if err := json.Unmarshal([]byte(sp.run(t, "slice-show", task, "--json", "--project", sp.id)), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Container, &sliceContainerJSON{ID: "c1", Title: "Cached title"}) {
		t.Errorf("container = %+v, want the cached fallback", got.Container)
	}
}

func TestSourceProjectRefusalsAndLocalPaths(t *testing.T) {
	sp := newSourceProject(t, &source.Fake{Details: map[string]source.ContainerDetail{"c1": {Title: "C"}}})
	task := sp.addTask(t, "A", "c1")

	if err := sp.fail(t, "project-mirror", "--parent", "p", "--parent-kind", "page", "--project", sp.id); !strings.Contains(err.Error(), `"Work" is a source project`) {
		t.Errorf("project-mirror = %v", err)
	}
	if err := sp.fail(t, "done-clear", "--project", sp.id); !strings.Contains(err.Error(), `"Work" is a source project`) {
		t.Errorf("done-clear = %v", err)
	}
	if out := sp.run(t, "slice-status", task, "--project", sp.id); out != "Todo\n" {
		t.Errorf("slice-status = %q", out)
	}
}

func TestConfigShowSaysASourceProjectsPlugin(t *testing.T) {
	cfg := config.Config{Projects: map[string]config.ProjectConfig{
		"p1": {Name: "Work", Backend: config.BackendSource, Source: "shortcut"},
	}}
	env, out := testEnv(cfg, &fakeAPI{})
	if err := Run(context.Background(), []string{"config-show", "--json"}, env); err != nil {
		t.Fatal(err)
	}
	var doc configDoc
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if p := doc.Projects["p1"]; p.Backend != "source" || p.Source != "shortcut" {
		t.Errorf("project = %+v", p)
	}
	out.Reset()
	if err := Run(context.Background(), []string{"config-show"}, env); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "backend=source working_dir=\"\" source=shortcut") {
		t.Errorf("text = %q", out.String())
	}
}

// installPlugins lays down an executable nat-source-<name> for each name in a
// fresh config dir's plugins dir, with nothing on PATH, and returns the dir.
func installPlugins(t *testing.T, names ...string) string {
	t.Helper()
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	t.Setenv("PATH", t.TempDir())
	if err := os.MkdirAll(filepath.Join(cfgDir, "notion-agent-tracker"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		dir := filepath.Join(cfgDir, "notion-agent-tracker", "plugins", n)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "nat-source-"+n), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(cfgDir, "notion-agent-tracker", "plugins")
}

func TestSourceListDescribesEveryPluginBestEffort(t *testing.T) {
	plugins := installPlugins(t, "broken", "demo", "gone", "old")
	env, out := testEnv(config.Config{}, &fakeAPI{})
	env.NewSource = func(name string) (source.Client, error) {
		switch name {
		case "demo":
			return &source.Fake{DescribeResult: demoDescribe()}, nil
		case "broken":
			return &source.Fake{DescribeErr: errors.New("no token")}, nil
		case "old":
			return &source.Fake{DescribeResult: source.Describe{Protocol: 2}}, nil
		}
		return nil, errors.New("vanished")
	}

	if err := Run(context.Background(), []string{"source-list", "--json"}, env); err != nil {
		t.Fatal(err)
	}
	var got []sourcePluginJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	d := demoDescribe()
	path := func(n string) string { return filepath.Join(plugins, n, "nat-source-"+n) }
	want := []sourcePluginJSON{
		{Name: "broken", Path: path("broken"), Error: "no token"},
		{Name: "demo", Path: path("demo"), Describe: &d},
		{Name: "gone", Path: path("gone"), Error: "vanished"},
		{Name: "old", Path: path("old"), Error: "source plugin old speaks protocol 2; this nat speaks protocol 1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("source-list = %+v, want %+v", got, want)
	}

	out.Reset()
	if err := Run(context.Background(), []string{"source-list"}, env); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "demo\t"+path("demo")+"\tDemo source (DM)\n") ||
		!strings.Contains(out.String(), "broken\t"+path("broken")+"\terror: no token\n") {
		t.Errorf("text = %q", out.String())
	}
}

func TestSourceListWithNothingInstalled(t *testing.T) {
	installPlugins(t)
	env, out := testEnv(config.Config{}, &fakeAPI{})
	if err := Run(context.Background(), []string{"source-list"}, env); err != nil {
		t.Fatal(err)
	}
	if out.String() != "no task source plugins installed\n" {
		t.Errorf("text = %q", out.String())
	}
	out.Reset()
	if err := Run(context.Background(), []string{"source-list", "--json"}, env); err != nil || out.String() != "[]\n" {
		t.Errorf("json = %q, %v", out.String(), err)
	}
}

func TestSourceListRefusals(t *testing.T) {
	env, _ := testEnv(config.Config{}, &fakeAPI{})
	var usage *UsageError
	for _, args := range [][]string{{"stray"}, {"--bogus"}} {
		if err := Run(context.Background(), append([]string{"source-list"}, args...), env); !errors.As(err, &usage) {
			t.Errorf("%v: err = %v, want a usage error", args, err)
		}
	}

	plugins := installPlugins(t)
	if err := os.WriteFile(plugins, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"source-list"}, env); err == nil || !strings.Contains(err.Error(), "look for task source plugins") {
		t.Errorf("unreadable plugins dir = %v", err)
	}

	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	if err := Run(context.Background(), []string{"source-list"}, env); err == nil {
		t.Error("no config dir = nil, want an error")
	}
}
