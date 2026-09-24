package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// mirrorEnv is a machine holding one local project with a two-milestone plan —
// the second slice waiting on the first — and a Notion client that is the given
// fake. The config is shared state, so what a command saves is what the next
// one loads, as it is on disk.
func mirrorEnv(t *testing.T, api *fakeAPI) (Env, *strings.Builder, *config.Config, string) {
	t.Helper()
	env, _, saved := noNotionEnv(t, config.Config{}, false)
	env.NewClient = func(notion.TokenFunc) API { return api }
	out := &strings.Builder{}
	env.Out = out

	ctx := context.Background()
	if err := Run(ctx, []string{"project-create", "Mine", "--local", "--repo", "/src/mine",
		"--description", "Be small.", "--json"}, env); err != nil {
		t.Fatalf("project-create --local: %v", err)
	}
	var id string
	for k := range saved.Projects {
		id = k
	}
	saved.ActiveProjectID = id
	env.In = strings.NewReader(`{
  "milestones": [{"name": "M1: First"}, {"name": "M2: Second"}],
  "slices": [
    {"title": "Alpha", "milestone": "M1: First", "description": "Do alpha.", "repo": "/src/other"},
    {"title": "Beta", "milestone": "M2: Second", "description": "Do beta.", "depends_on": ["Alpha"]}
  ]
}`)
	if err := Run(ctx, []string{"plan-apply", "--project", id}, env); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}
	out.Reset()
	env.In = nil

	api.dataSources = map[string]notion.DataSource{"new-slices-ds": selectMilestoneSlicesDS()}
	api.createdPages = createdSeq(2)
	return env, out, saved, id
}

func TestProjectMirrorFilesThePlanAndReRegistersTheProject(t *testing.T) {
	api := &fakeAPI{}
	env, out, saved, oldID := mirrorEnv(t, api)

	err := Run(context.Background(), []string{"project-mirror", "--project", oldID,
		"--parent", "parent-page", "--parent-kind", "page", "--json"}, env)
	if err != nil {
		t.Fatalf("project-mirror: %v", err)
	}

	var got projectMirroredJSON
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("output %q: %v", out.String(), err)
	}
	if got.Project.ID != "new-project" || got.Replaced != oldID || got.Milestones != 2 || got.Slices != 2 {
		t.Errorf("output = %+v", got)
	}
	if len(api.inParents) != 1 || api.inParents[0] != notion.PageParent("parent-page") {
		t.Errorf("project created under %+v, want the page", api.inParents)
	}
	if len(api.projects) != 1 || api.projects[0].name != "Mine" {
		t.Errorf("projects created = %+v", api.projects)
	}

	// The project is now one tracked in Notion under the page's ID, keeping its
	// working directory, and the local entry is gone.
	if _, ok := saved.Projects[oldID]; ok {
		t.Error("the local entry is still in config")
	}
	entry, ok := saved.Projects["new-project"]
	if !ok || entry.IsLocal() || entry.SlicesDSID != "new-slices-ds" || entry.WorkingDir != "/src/mine" {
		t.Errorf("mirrored entry = %+v (in config: %v)", entry, ok)
	}
	if saved.ActiveProjectID != "new-project" {
		t.Errorf("active project = %q, want it to follow the project", saved.ActiveProjectID)
	}

	// Conventions on the project page; each slice's brief and the milestone
	// options in Notion; the dependency recorded once both slices exist.
	if len(api.appends) < 1 || api.appends[0].id != "new-project" {
		t.Errorf("appends = %+v, want the conventions on the project page first", api.appends)
	}
	if len(api.creates) != 2 || writtenText(api.creates[0].props[notion.PropName]) != "Alpha" ||
		writtenText(api.creates[1].props[notion.PropName]) != "Beta" {
		t.Fatalf("slices created = %+v", api.creates)
	}
	if writtenText(api.creates[0].props[notion.PropRepo]) != "/src/other" {
		t.Errorf("Alpha's repo = %+v", api.creates[0].props[notion.PropRepo])
	}
	var depends bool
	for _, u := range api.updates {
		rel := u.props[notion.PropDependsOn].Relation
		if u.id == "new-2" && rel != nil && len(*rel) == 1 && (*rel)[0].ID == "new-1" {
			depends = true
		}
	}
	if !depends {
		t.Errorf("updates = %+v, want Beta made to wait on Alpha's new page", api.updates)
	}
}

func TestProjectMirrorPutsAProjectInADatabaseAsARow(t *testing.T) {
	api := &fakeAPI{}
	env, out, _, oldID := mirrorEnv(t, api)

	err := Run(context.Background(), []string{"project-mirror", "--project", oldID,
		"--parent", "some-ds", "--parent-kind", "database"}, env)
	if err != nil {
		t.Fatalf("project-mirror: %v", err)
	}
	if len(api.inParents) != 1 || api.inParents[0] != notion.DataSourceParent("some-ds") {
		t.Errorf("project created under %+v, want the data source", api.inParents)
	}
	if want := "Mirrored \"Mine\" to Notion as project new-project: 2 milestones and 2 slices.\nhttps://notion.so/new-project\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestProjectMirrorRefusesBeforeTouchingNotion(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no parent", []string{"--parent-kind", "page"}, "no --parent given"},
		{"no kind", []string{"--parent", "p"}, "--parent-kind is \"\""},
		{"a bad kind", []string{"--parent", "p", "--parent-kind", "block"}, "--parent-kind is \"block\""},
		{"an argument", []string{"--parent", "p", "--parent-kind", "page", "extra"}, "takes no arguments"},
		{"a bad flag", []string{"--nope"}, "project-mirror"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			api := &fakeAPI{}
			env, _, saved, id := mirrorEnv(t, api)
			err := Run(context.Background(), append([]string{"project-mirror", "--project", id}, c.args...), env)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %v, want one saying %q", err, c.want)
			}
			if len(api.projects)+len(api.creates) != 0 || len(saved.Projects) != 1 {
				t.Errorf("something was written: %+v %+v %+v", api.projects, api.creates, saved.Projects)
			}
		})
	}
}

func TestProjectMirrorRefusesAProjectAlreadyInNotion(t *testing.T) {
	cfg := testConfig(t)
	env, _ := testEnv(cfg, &fakeAPI{})
	err := Run(context.Background(), []string{"project-mirror", "--project", "project-1",
		"--parent", "p", "--parent-kind", "page"}, env)
	if err == nil || !strings.Contains(err.Error(), "already tracked in Notion") {
		t.Errorf("error = %v", err)
	}
}

func TestProjectMirrorNeedsAProject(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})
	err := Run(context.Background(), []string{"project-mirror", "--parent", "p", "--parent-kind", "page"}, env)
	if err == nil || !strings.Contains(err.Error(), "no project given") {
		t.Errorf("error = %v", err)
	}
}

func TestProjectMirrorRefusesAStartedPlan(t *testing.T) {
	api := &fakeAPI{}
	env, _, _, id := mirrorEnv(t, api)
	// Claim the first slice, which is what starting is.
	if err := Run(context.Background(), []string{"next-slice", "--project", id}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}
	err := Run(context.Background(), []string{"project-mirror", "--project", id,
		"--parent", "p", "--parent-kind", "page"}, env)
	if err == nil || !strings.Contains(err.Error(), "has been started") {
		t.Fatalf("error = %v, want a refusal of the started plan", err)
	}
	if len(api.projects) != 0 {
		t.Errorf("a project was created: %+v", api.projects)
	}
}

func TestRefuseStarted(t *testing.T) {
	todo := domain.Slice{Name: "A", Status: domain.SliceTodo}
	if err := refuseStarted([]domain.Slice{todo}); err != nil {
		t.Errorf("a Todo slice refused: %v", err)
	}
	for name, s := range map[string]domain.Slice{
		"claimed":   {Name: "B", Status: domain.SliceClaimed, StatusName: "In progress"},
		"a branch":  {Name: "C", Status: domain.SliceTodo, Branch: "b"},
		"a pull rq": {Name: "D", Status: domain.SliceTodo, PRURL: "https://x/1"},
	} {
		if err := refuseStarted([]domain.Slice{todo, s}); err == nil || !strings.Contains(err.Error(), s.Name) {
			t.Errorf("%s: error = %v, want it to name %q", name, err, s.Name)
		}
	}
}

func TestProjectMirrorReportsAFailedCreateWithNothingChanged(t *testing.T) {
	for name, api := range map[string]*fakeAPI{
		"an error": {projectErr: errors.New("no access")},
		"nothing":  {projectNothing: true},
	} {
		t.Run(name, func(t *testing.T) {
			env, _, saved, id := mirrorEnv(t, api)
			api.dataSources = nil
			err := Run(context.Background(), []string{"project-mirror", "--project", id,
				"--parent", "p", "--parent-kind", "page"}, env)
			if err == nil || !strings.Contains(err.Error(), "create the project page") {
				t.Fatalf("error = %v", err)
			}
			if _, ok := saved.Projects[id]; !ok || len(saved.Projects) != 1 {
				t.Errorf("config = %+v, want the local project alone", saved.Projects)
			}
		})
	}
}

func TestProjectMirrorSaysWhatExistsWhenFilingFails(t *testing.T) {
	api := &fakeAPI{}
	env, _, saved, id := mirrorEnv(t, api)
	api.createErr = errors.New("rate limited")
	api.failCreateAfter = 0

	err := Run(context.Background(), []string{"project-mirror", "--project", id,
		"--parent", "p", "--parent-kind", "page"}, env)
	if err == nil || !strings.Contains(err.Error(), "new-project was created") ||
		!strings.Contains(err.Error(), "local project is untouched") {
		t.Fatalf("error = %v", err)
	}
	// Both entries are in config: the local project, untouched, and the page
	// that was made, which a later command can be pointed at.
	if _, ok := saved.Projects[id]; !ok {
		t.Error("the local entry was removed")
	}
	if _, ok := saved.Projects["new-project"]; !ok {
		t.Error("the mirror's entry was not recorded")
	}
}

func TestProjectMirrorSaysWhatExistsWhenTheConventionsFail(t *testing.T) {
	api := &fakeAPI{appendErr: errors.New("boom")}
	env, _, _, id := mirrorEnv(t, api)
	err := Run(context.Background(), []string{"project-mirror", "--project", id,
		"--parent", "p", "--parent-kind", "page"}, env)
	if err == nil || !strings.Contains(err.Error(), "new-project was created") {
		t.Errorf("error = %v", err)
	}
}

func TestProjectMirrorFailsWhenTheConfigWillNotSave(t *testing.T) {
	api := &fakeAPI{}
	env, _, _, id := mirrorEnv(t, api)
	env.Save = func(config.Config) error { return errors.New("read-only") }
	err := Run(context.Background(), []string{"project-mirror", "--project", id,
		"--parent", "p", "--parent-kind", "page"}, env)
	if err == nil || !strings.Contains(err.Error(), "save config") {
		t.Errorf("error = %v", err)
	}
}

func TestNotionSearchListsWhereAProjectPageCouldGo(t *testing.T) {
	titled := func(s string) []notion.RichText { return []notion.RichText{{PlainText: s}} }
	api := &fakeAPI{searchHits: []notion.SearchResult{
		{Object: notion.SearchPage, ID: "p1", Title: titled("Plans"), Parent: notion.Parent{Type: notion.ParentWorkspace}},
		{Object: notion.SearchPage, ID: "row", Title: titled("A row"), Parent: notion.Parent{Type: notion.ParentDataSource}},
		{Object: notion.SearchPage, ID: "row2", Title: titled("Old row"), Parent: notion.Parent{Type: notion.ParentDatabase}},
		{Object: notion.SearchDataSource, ID: "ds1", Title: titled("Projects")},
		{Object: notion.SearchDataSource, ID: "ds2"},
		{Object: "block", ID: "b"},
	}}
	env, out := testEnv(testConfig(t), api)

	if err := Run(context.Background(), []string{"notion-search", "--query", " plan ", "--json"}, env); err != nil {
		t.Fatalf("notion-search: %v", err)
	}
	var got notionSearchJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := []notionPlace{
		{ID: "p1", Kind: "page", Title: "Plans"},
		{ID: "ds1", Kind: "database", Title: "Projects"},
		{ID: "ds2", Kind: "database", Title: "Untitled"},
	}
	if len(got.Places) != len(want) {
		t.Fatalf("places = %+v, want %+v", got.Places, want)
	}
	for i := range want {
		if got.Places[i] != want[i] {
			t.Errorf("place %d = %+v, want %+v", i, got.Places[i], want[i])
		}
	}
	if len(api.searches) != 1 || api.searches[0] != "plan" {
		t.Errorf("searched for %q, want the trimmed query", api.searches)
	}
}

func TestNotionSearchMarkdownAndEmpty(t *testing.T) {
	api := &fakeAPI{searchHits: []notion.SearchResult{
		{Object: notion.SearchPage, ID: "p1", Title: []notion.RichText{{PlainText: "Plans"}}},
	}}
	env, out := testEnv(testConfig(t), api)
	if err := Run(context.Background(), []string{"notion-search"}, env); err != nil {
		t.Fatal(err)
	}
	if want := "- Plans (page) — p1\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}

	api.searchHits = nil
	out.Reset()
	if err := Run(context.Background(), []string{"notion-search"}, env); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Nothing in the workspace matches") {
		t.Errorf("output = %q", out.String())
	}
}

func TestNotionSearchRefusals(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{searchErr: errors.New("offline")})
	if err := Run(context.Background(), []string{"notion-search"}, env); err == nil ||
		!strings.Contains(err.Error(), "search the workspace: offline") {
		t.Errorf("error = %v", err)
	}
	if err := Run(context.Background(), []string{"notion-search", "extra"}, env); err == nil ||
		!strings.Contains(err.Error(), "takes no arguments") {
		t.Errorf("error = %v", err)
	}
	if err := Run(context.Background(), []string{"notion-search", "--nope"}, env); err == nil {
		t.Error("a bad flag was accepted")
	}
}

func TestPlacesOfCapsTheList(t *testing.T) {
	var hits []notion.SearchResult
	for i := 0; i < searchLimit+5; i++ {
		hits = append(hits, notion.SearchResult{Object: notion.SearchPage, ID: "p"})
	}
	if got := placesOf(hits); len(got) != searchLimit {
		t.Errorf("places = %d, want %d", len(got), searchLimit)
	}
}

// breakPlan runs a statement against a local project's plan file, to make the
// one read that hits it fail.
func breakPlan(t *testing.T, cfg *config.Config, id, stmt string) {
	t.Helper()
	path, err := store.PlanPath(store.ProjectOf(id, cfg.Projects[id]))
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(stmt); err != nil {
		t.Fatal(err)
	}
}

func TestProjectMirrorRefusesAPlanItCannotRead(t *testing.T) {
	for name, c := range map[string]struct{ stmt, want string }{
		"the plan":  {"ALTER TABLE slices RENAME TO gone", "read the plan"},
		"the prose": {"ALTER TABLE slices DROP COLUMN body", "read the plan's prose"},
	} {
		t.Run(name, func(t *testing.T) {
			api := &fakeAPI{}
			env, _, saved, id := mirrorEnv(t, api)
			breakPlan(t, saved, id, c.stmt)
			err := Run(context.Background(), []string{"project-mirror", "--project", id,
				"--parent", "p", "--parent-kind", "page"}, env)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %v, want one saying %q", err, c.want)
			}
			if len(api.projects) != 0 {
				t.Errorf("Notion was touched: %+v", api.projects)
			}
		})
	}
}

func TestProjectMirrorRefusesAPlanFileThatWillNotOpen(t *testing.T) {
	api := &fakeAPI{}
	env, _, saved, id := mirrorEnv(t, api)
	path, err := store.PlanPath(store.ProjectOf(id, saved.Projects[id]))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"project-mirror", "--project", id,
		"--parent", "p", "--parent-kind", "page"}, env); err == nil {
		t.Error("a plan that is not a database was mirrored")
	}
}

// TestProjectMirrorSurvivesTheConfigChangingUnderIt: the nth load or save of the config fails, or comes back
// as the config given, which is how a config changing under a command reads.
func TestProjectMirrorSurvivesTheConfigChangingUnderIt(t *testing.T) {
	t.Run("the second load fails", func(t *testing.T) {
		api := &fakeAPI{}
		env, _, _, id := mirrorEnv(t, api)
		load, calls := env.Load, 0
		env.Load = func() (config.Config, bool, error) {
			if calls++; calls == 2 {
				return config.Config{}, false, errors.New("gone")
			}
			return load()
		}
		err := Run(context.Background(), []string{"project-mirror", "--project", id,
			"--parent", "p", "--parent-kind", "page"}, env)
		if err == nil || !strings.Contains(err.Error(), "gone") {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("the config came back with no projects", func(t *testing.T) {
		api := &fakeAPI{}
		env, _, _, id := mirrorEnv(t, api)
		load, saves, calls := env.Load, []config.Config{}, 0
		env.Load = func() (config.Config, bool, error) {
			if calls++; calls == 2 {
				return config.Config{}, true, nil
			}
			return load()
		}
		env.Save = func(c config.Config) error { saves = append(saves, c); return nil }
		if err := Run(context.Background(), []string{"project-mirror", "--project", id,
			"--parent", "p", "--parent-kind", "page"}, env); err != nil {
			t.Fatalf("project-mirror: %v", err)
		}
		if _, ok := saves[0].Projects["new-project"]; !ok {
			t.Errorf("first save = %+v, want the mirror recorded", saves[0])
		}
	})
	t.Run("the final save fails", func(t *testing.T) {
		api := &fakeAPI{}
		env, _, _, id := mirrorEnv(t, api)
		saves := 0
		env.Save = func(config.Config) error {
			if saves++; saves == 2 {
				return errors.New("read-only")
			}
			return nil
		}
		err := Run(context.Background(), []string{"project-mirror", "--project", id,
			"--parent", "p", "--parent-kind", "page"}, env)
		if err == nil || !strings.Contains(err.Error(), "read-only") || !strings.Contains(err.Error(), "new-project was created") {
			t.Errorf("error = %v", err)
		}
	})
}

func TestProjectMirrorSaysHowFarItGotWhenFilingFails(t *testing.T) {
	run := func(t *testing.T, api *fakeAPI, ready func(*config.Config)) error {
		env, _, saved, id := mirrorEnv(t, api)
		if ready != nil {
			ready(saved)
		}
		return Run(context.Background(), []string{"project-mirror", "--project", id,
			"--parent", "p", "--parent-kind", "page"}, env)
	}
	t.Run("the new plan file will not open", func(t *testing.T) {
		err := run(t, &fakeAPI{}, func(*config.Config) {
			path, perr := store.PlanPath(store.Project{ID: "new-project"})
			if perr != nil {
				t.Fatal(perr)
			}
			if merr := os.MkdirAll(path, 0o755); merr != nil {
				t.Fatal(merr)
			}
		})
		if err == nil || !strings.Contains(err.Error(), "new-project was created") {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("the milestones cannot be made", func(t *testing.T) {
		err := run(t, &fakeAPI{schemaUpdateErr: errors.New("no schema")}, nil)
		if err == nil || !strings.Contains(err.Error(), "create the milestones") {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("a dependency cannot be recorded", func(t *testing.T) {
		err := run(t, &fakeAPI{updateErr: errors.New("no relation")}, nil)
		if err == nil || !strings.Contains(err.Error(), `record what "Beta" waits on`) {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("the file cannot record a dependency the workspace took", func(t *testing.T) {
		api := &fakeAPI{}
		api.onUpdate = func() {
			breakPlan(t, &config.Config{}, "new-project", "DROP TABLE slice_deps")
		}
		err := run(t, api, nil)
		if err == nil || !strings.Contains(err.Error(), `record what "Beta" waits on`) {
			t.Errorf("error = %v", err)
		}
	})
}
