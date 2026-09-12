package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// localEnv is an Env for a machine that has no Notion at all: no client, no
// token, and one project whose plan is kept in a file under the given
// directory. Building the client panics on purpose — a command that reaches for
// Notion on a local project's path is the very thing this file is about.
func localEnv(t *testing.T, dir string, cfg *config.Config) (Env, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	return Env{
		Tokens: config.StaticToken(""),
		Load:   func() (config.Config, bool, error) { return *cfg, true, nil },
		Save:   func(c config.Config) error { *cfg = c; return nil },
		NewClient: func(notion.TokenFunc) API {
			t.Error("a local project reached for Notion")
			return &fakeAPI{}
		},
		NewTmux: DefaultNewTmux,
		Out:     &out,
	}, &out
}

// A project whose plan is kept in a file is created and then worked end to end
// with nothing on the Notion side: the plan filed, the slice claimed, its brief
// printed with the project's own conventions, and the work handed back on a
// branch — every one of those a command an agent runs, and not one of them
// reaching a workspace.
func TestALocalProjectIsCreatedAndWorkedWithNoNotion(t *testing.T) {
	plans := t.TempDir()
	cfg := config.Config{AssigneeUserName: "Craig Johnston"}
	env, out := localEnv(t, plans, &cfg)
	env.In = strings.NewReader("Branch per slice.")
	ctx := context.Background()

	// Created, with no projects database and no token in sight.
	if err := Run(ctx, []string{"project-create", "tracker", "--local",
		"--plan-dir", plans, "--repo", t.TempDir(), "--description", "-", "--json"}, env); err != nil {
		t.Fatalf("project-create --local: %v", err)
	}
	var created projectCreatedJSON
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	id := created.Project.ID
	if created.Project.Backend != config.BackendLocal {
		t.Errorf("backend = %q, want local", created.Project.Backend)
	}
	if _, err := os.Stat(created.Project.PlanPath); err != nil {
		t.Fatalf("the plan file was not written: %v", err)
	}
	if !cfg.Projects[id].IsLocal() {
		t.Fatalf("config = %+v, want the project recorded as local", cfg.Projects[id])
	}
	if cfg.NeedsNotion() {
		t.Error("a config of one local project needs no Notion")
	}

	// A plan filed into it.
	out.Reset()
	env.In = strings.NewReader(`{
	  "milestones": [{"name": "M1: Client"}],
	  "slices": [{"title": "Notion client", "milestone": "M1: Client", "description": "Write it."}]
	}`)
	if err := Run(ctx, []string{"plan-apply", "--project", id}, env); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	// Read back, conventions and all.
	out.Reset()
	if err := Run(ctx, []string{"info", "--project", id}, env); err != nil {
		t.Fatalf("info: %v", err)
	}
	for _, want := range []string{"# tracker", "Branch per slice.", "M1: Client", "Notion client"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("info is missing %q:\n%s", want, out.String())
		}
	}

	// Claimed by whoever the config names, and the brief printed.
	out.Reset()
	if err := Run(ctx, []string{"next-slice", "--project", id}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}
	if !strings.Contains(out.String(), "Write it.") {
		t.Errorf("brief is missing the slice's own words:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Craig Johnston") {
		t.Errorf("brief does not say who claimed it:\n%s", out.String())
	}

	// The slice ID the claim landed on, read back off the plan.
	out.Reset()
	if err := Run(ctx, []string{"info", "--project", id, "--json"}, env); err != nil {
		t.Fatalf("info --json: %v", err)
	}
	sliceID := firstSliceID(t, out.Bytes())

	// Re-opened rather than refused, which is what a board-launched agent does.
	out.Reset()
	if err := Run(ctx, []string{"start-slice", sliceID, "--project", id}, env); err != nil {
		t.Fatalf("start-slice: %v", err)
	}

	// And handed back on a branch.
	out.Reset()
	if err := Run(ctx, []string{"complete-slice", sliceID, "--project", id,
		"--branch", "slice/notion-client", "--summary", "Wrote it."}, env); err != nil {
		t.Fatalf("complete-slice: %v", err)
	}
	out.Reset()
	if err := Run(ctx, []string{"slice-show", sliceID, "--project", id, "--json"}, env); err != nil {
		t.Fatalf("slice-show: %v", err)
	}
	var shown sliceShowJSON
	if err := json.Unmarshal(out.Bytes(), &shown); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if shown.Branch != "slice/notion-client" {
		t.Errorf("branch = %q, want the one handed back", shown.Branch)
	}
	if !shown.HandedBack {
		t.Error("the slice should read as handed back")
	}
	if shown.Assignee != "Craig Johnston" {
		t.Errorf("assignee = %q, want the configured name", shown.Assignee)
	}
}

// firstSliceID reads the ID of the plan's one slice out of `info --json`.
func firstSliceID(t *testing.T, data []byte) string {
	t.Helper()
	var doc struct {
		Slices []struct {
			ID string `json:"id"`
		} `json:"slices"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, data)
	}
	if len(doc.Slices) != 1 {
		t.Fatalf("slices = %+v, want exactly one", doc.Slices)
	}
	return doc.Slices[0].ID
}

// A machine that has never onboarded names no user at all, and a plan kept in a
// file has no directory of users behind it: whoever is logged in is who is
// working it, which is the only answer such a file could have.
func TestALocalProjectIsWorkedAsWhoeverIsLoggedIn(t *testing.T) {
	plans := t.TempDir()
	id := "p-local"
	if _, err := createPlan(t, plans, id); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Projects: map[string]config.ProjectConfig{
		id: {Name: "tracker", Backend: config.BackendLocal, PlanDir: plans},
	}}

	me, err := ownerOf(cfg, cfg.Projects[id])
	if err != nil {
		t.Fatalf("ownerOf: %v", err)
	}
	if me.ID == "" || me.ID != me.Name {
		t.Errorf("owner = %+v, want one name standing for both", me)
	}
}

// A machine whose own account cannot be read — or which has one with no name —
// is not one to guess a name for: an empty owner would claim every slice for
// nobody and read back as somebody else's.
func TestALocalProjectRefusesWithNoOneToWorkItAs(t *testing.T) {
	tests := []struct {
		name string
		user func() (*user.User, error)
	}{
		{"the account cannot be read", func() (*user.User, error) { return nil, errNoLogin }},
		{"the account has no name", func() (*user.User, error) { return &user.User{Username: " "}, nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := currentUser
			currentUser = tt.user
			defer func() { currentUser = old }()

			_, err := ownerOf(config.Config{}, config.ProjectConfig{Backend: config.BackendLocal})
			if err == nil || !strings.Contains(err.Error(), "who is working this project") {
				t.Fatalf("err = %v, want a refusal saying it cannot tell who this is", err)
			}
		})
	}
}

// A project kept in Notion still needs the workspace user onboarding resolved.
func TestANotionProjectStillNeedsAnAssignee(t *testing.T) {
	_, err := ownerOf(config.Config{}, config.ProjectConfig{Name: "nat"})
	if err == nil || !strings.Contains(err.Error(), "no assignee in the config") {
		t.Fatalf("err = %v, want the assignee refusal", err)
	}
}

// The commands that read something only a workspace keeps are refused by name
// on a local project rather than handed a client to go looking with.
func TestWishlistIsRefusedOnALocalProject(t *testing.T) {
	plans := t.TempDir()
	id := "p-local"
	if _, err := createPlan(t, plans, id); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Projects: map[string]config.ProjectConfig{
		id: {Name: "tracker", Backend: config.BackendLocal, PlanDir: plans},
	}}
	env, _ := localEnv(t, plans, &cfg)

	for _, args := range [][]string{
		{"wishlist", "--project", id},
		{"wishlist-clear", "block-1", "--project", id},
	} {
		err := Run(context.Background(), args, env)
		if err == nil || !strings.Contains(err.Error(), "tracker") {
			t.Errorf("%v: err = %v, want a refusal naming the project", args, err)
		}
	}
}

// --plan-dir is for a local project: a plan kept in Notion is kept in Notion,
// and a flag that would be silently ignored is one somebody is wrong about.
func TestProjectCreateRefusesAPlanDirWithoutLocal(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})
	err := Run(context.Background(), []string{"project-create", "tracker", "--plan-dir", "/plans"}, env)
	var ue *UsageError
	if err == nil || !errorsAs(err, &ue) {
		t.Fatalf("err = %v, want a usage error", err)
	}
}

// Every command that acts on a project opens its store first, so a plan that
// will not open is refused before anything else is attempted — whichever
// command it was.
func TestEveryProjectCommandRefusesAPlanThatWillNotOpen(t *testing.T) {
	dir := t.TempDir()
	id := "p-local"
	// A file where the plan should be, and not a plan.
	if err := os.WriteFile(filepath.Join(dir, id+".db"), []byte("not a database"), 0o644); err != nil {
		t.Fatalf("write the file: %v", err)
	}
	cfg := config.Config{
		AssigneeUserName: "Craig Johnston",
		Projects: map[string]config.ProjectConfig{
			id: {Name: "tracker", Backend: config.BackendLocal, PlanDir: dir},
		},
	}
	env, out := localEnv(t, dir, &cfg)

	commands := [][]string{
		{"info"},
		{"next-slice"},
		{"start-slice", testSliceID},
		{"slice-show", testSliceID},
		{"slice-launch", testSliceID},
		{"slice-approve", testSliceID},
		{"slice-diff", testSliceID},
		{"slice-edit", testSliceID, "--description", "x"},
		{"slice-move", testSliceID, "--milestone", "M1"},
		{"slice-delete", testSliceID},
		{"slice-add", "One", "--milestone", "M1"},
		{"slice-depends", testSliceID, "--clear"},
		{"milestone-add", "M1"},
		{"milestone-rename", "M1", "M2"},
		{"milestone-remove", "M1"},
		{"milestone-move", "M1", "--after", "M2"},
		{"complete-slice", testSliceID, "--summary", "done"},
		{"release-slice", testSliceID},
		{"pr-view", testSliceID},
		{"pr-comment", testSliceID, "--body", "x"},
		{"pr-merge", testSliceID},
		{"pr-status"},
	}
	for _, cmd := range commands {
		out.Reset()
		args := append(append([]string{}, cmd...), "--project", id)
		if err := Run(context.Background(), args, env); err == nil {
			t.Errorf("%v: want a refusal, the plan being unreadable", cmd)
		}
	}
}

// plan-apply reads its document before it opens anything, so its own refusal
// comes after the document is understood and before the first write.
func TestPlanApplyRefusesAPlanThatWillNotOpen(t *testing.T) {
	dir := t.TempDir()
	id := "p-local"
	if err := os.WriteFile(filepath.Join(dir, id+".db"), []byte("not a database"), 0o644); err != nil {
		t.Fatalf("write the file: %v", err)
	}
	cfg := config.Config{Projects: map[string]config.ProjectConfig{
		id: {Name: "tracker", Backend: config.BackendLocal, PlanDir: dir},
	}}
	env, _ := localEnv(t, dir, &cfg)
	env.In = strings.NewReader(`{"milestones": [{"name": "M1"}]}`)

	if err := Run(context.Background(), []string{"plan-apply", "--project", id}, env); err == nil {
		t.Fatal("want a refusal, the plan being unreadable")
	}
}

// createPlan lays down an empty local plan for a project, the way
// project-create --local does.
func createPlan(t *testing.T, dir, id string) (string, error) {
	t.Helper()
	return store.CreateLocalProject(dir, id, "tracker", "Branch per slice.")
}

// errNoLogin stands in for os/user failing to say who this is.
var errNoLogin = errors.New("no such user")

// errorsAs is errors.As, named so the assertions above read as sentences.
func errorsAs(err error, target any) bool { return errors.As(err, target) }

// config-show says where every project's plan is kept, including for the
// Notion projects the config file leaves the word unwritten for: a listing read
// by somebody asking which is which must not answer with a blank.
func TestConfigShowNamesEachProjectsBackend(t *testing.T) {
	cfg := config.Config{Projects: map[string]config.ProjectConfig{
		"p1": {Name: "in Notion", WorkingDir: "/a"},
		"p2": {Name: "on this machine", WorkingDir: "/b",
			Backend: config.BackendLocal, PlanDir: "/plans"},
	}}
	env, out := testEnv(cfg, &fakeAPI{})

	if err := Run(context.Background(), []string{"config-show"}, env); err != nil {
		t.Fatalf("config-show: %v", err)
	}
	for _, want := range []string{
		`p1 (in Notion): backend=notion working_dir="/a"`,
		`p2 (on this machine): backend=local working_dir="/b" plan_dir="/plans"`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output is missing %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	if err := Run(context.Background(), []string{"config-show", "--json"}, env); err != nil {
		t.Fatalf("config-show --json: %v", err)
	}
	var doc configDoc
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if doc.Projects["p1"].Backend != config.BackendNotion || doc.Projects["p1"].PlanDir != "" {
		t.Errorf("p1 = %+v, want it named as a Notion project with no plan dir", doc.Projects["p1"])
	}
	if doc.Projects["p2"].Backend != config.BackendLocal || doc.Projects["p2"].PlanDir != "/plans" {
		t.Errorf("p2 = %+v, want it named as a local project", doc.Projects["p2"])
	}
}

// The markdown a local creation reports itself in names the one thing there is
// to go and look at — the file the plan is in — where the Notion one names a
// page and a data source.
func TestProjectCreateLocalReportsThePlanFile(t *testing.T) {
	plans := t.TempDir()
	cfg := config.Config{}
	env, out := localEnv(t, plans, &cfg)

	if err := Run(context.Background(), []string{"project-create", "tracker",
		"--local", "--plan-dir", plans, "--repo", "/work"}, env); err != nil {
		t.Fatalf("project-create --local: %v", err)
	}
	for _, want := range []string{"# tracker", "kept in a file of nat's own", plans, "/work", switchNote} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output is missing %q:\n%s", want, out.String())
		}
	}
}

// The three ways creating a local project fails, each of them said rather than
// half done.
func TestProjectCreateLocalReportsWhatStoppedIt(t *testing.T) {
	t.Run("the config will not load", func(t *testing.T) {
		env, _ := localEnv(t, t.TempDir(), &config.Config{})
		env.Load = func() (config.Config, bool, error) { return config.Config{}, false, errNoLogin }
		if err := Run(context.Background(), []string{"project-create", "tracker", "--local"}, env); err == nil {
			t.Fatal("want the config failure")
		}
	})
	t.Run("the plan will not be written", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "wall")
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatalf("write the file: %v", err)
		}
		env, _ := localEnv(t, dir, &config.Config{})
		err := Run(context.Background(), []string{"project-create", "tracker",
			"--local", "--plan-dir", file, "--repo", "/work"}, env)
		if err == nil {
			t.Fatal("want the plan file failure")
		}
	})
	t.Run("the config will not save", func(t *testing.T) {
		plans := t.TempDir()
		env, _ := localEnv(t, plans, &config.Config{})
		env.Save = func(config.Config) error { return errNoLogin }
		err := Run(context.Background(), []string{"project-create", "tracker",
			"--local", "--plan-dir", plans, "--repo", "/work"}, env)
		if err == nil || !strings.Contains(err.Error(), "save config") {
			t.Fatalf("err = %v, want the config failure", err)
		}
	})
}

// A plan kept in a file reads its slices back in the order they were written,
// so plan-apply writes the document front to back — reversing there, as it must
// for Notion, would be the one thing that put the plan out of order.
func TestPlanApplyWritesALocalPlanInTheDocumentsOrder(t *testing.T) {
	plans := t.TempDir()
	id := "p-local"
	if _, err := createPlan(t, plans, id); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		AssigneeUserName: "Craig Johnston",
		Projects: map[string]config.ProjectConfig{
			id: {Name: "tracker", Backend: config.BackendLocal, PlanDir: plans},
		},
	}
	env, out := localEnv(t, plans, &cfg)
	env.In = strings.NewReader(`{
	  "milestones": [{"name": "M1"}],
	  "slices": [
	    {"title": "One", "milestone": "M1"},
	    {"title": "Two", "milestone": "M1"},
	    {"title": "Three", "milestone": "M1"}
	  ]
	}`)

	if err := Run(context.Background(), []string{"plan-apply", "--project", id, "--json"}, env); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}
	var doc struct {
		Ordering string `json:"ordering"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if doc.Ordering != appendingWord {
		t.Errorf("ordering = %q, want %q", doc.Ordering, appendingWord)
	}

	out.Reset()
	if err := Run(context.Background(), []string{"info", "--project", id}, env); err != nil {
		t.Fatalf("info: %v", err)
	}
	body := out.String()
	one, two, three := strings.Index(body, "- One"), strings.Index(body, "- Two"), strings.Index(body, "- Three")
	if one < 0 || one > two || two > three {
		t.Errorf("the plan is out of the document's order:\n%s", body)
	}
	if !strings.Contains(body, "M1") {
		t.Errorf("the milestone is missing:\n%s", body)
	}
}

// The markdown says the plain thing for a store that appends: there is nothing
// to explain, the order being the order things were written in.
func TestPlanApplyExplainsTheOrderItWroteIn(t *testing.T) {
	plans := t.TempDir()
	id := "p-local"
	if _, err := createPlan(t, plans, id); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Projects: map[string]config.ProjectConfig{
		id: {Name: "tracker", Backend: config.BackendLocal, PlanDir: plans},
	}}
	env, out := localEnv(t, plans, &cfg)
	env.In = strings.NewReader(`{"milestones": [{"name": "M1"}],
	  "slices": [{"title": "One", "milestone": "M1"}]}`)

	if err := Run(context.Background(), []string{"plan-apply", "--project", id}, env); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}
	if !strings.Contains(out.String(), appendNote) {
		t.Errorf("output does not say what order it wrote in:\n%s", out.String())
	}
	if strings.Contains(out.String(), orderNote) {
		t.Errorf("output explains Notion's problem to a plan that has not got it:\n%s", out.String())
	}
}

// A run that failed part way reports what exists, whichever way it was
// writing: for a store that appends, the head of the document, since that is
// the end it started from.
func TestPlanApplyReportsTheHeadOfADocumentItGotPartWayThrough(t *testing.T) {
	st := &appendingStore{failAt: 2}
	p := plan{
		Milestones: []planMilestone{{Name: "M1"}},
		Slices: []planSlice{
			{Title: "One", Milestone: "M1"},
			{Title: "Two", Milestone: "M1"},
			{Title: "Three", Milestone: "M1"},
		},
	}
	targets, err := validatePlan(p, nil, nil)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	applied, err := applyPlan(context.Background(), st, store.Project{ID: "p1"}, store.Shape{}, p, targets, nil)
	if err == nil {
		t.Fatal("want the run to fail part way")
	}
	if len(applied.Slices) != 1 || applied.Slices[0].Slice.Name != "One" {
		t.Errorf("applied = %+v, want the head of the document", applied.Slices)
	}
	if st.written[0] != "One" || len(st.written) != 1 {
		t.Errorf("written = %v, want the document front to back", st.written)
	}
}

// appendingStore is a store that reads its slices back in the order they were
// written, and refuses the failAt'th one. Everything else a plan-apply asks of
// a store is here; the rest of the interface is never reached, which is what
// the embedded nil says.
type appendingStore struct {
	store.Store
	failAt  int
	written []string
}

func (s *appendingStore) Appends() bool { return true }

func (s *appendingStore) AddMilestones(_ context.Context, _ store.Project, _ store.Shape, names []string) ([]domain.Milestone, error) {
	ms := make([]domain.Milestone, len(names))
	for i, n := range names {
		ms[i] = domain.Milestone{ID: n, Name: n, Order: float64(i)}
	}
	return ms, nil
}

func (s *appendingStore) AddSlice(_ context.Context, _ store.Project, n store.NewSlice) (domain.Slice, error) {
	if len(s.written)+1 == s.failAt {
		return domain.Slice{}, errNoLogin
	}
	s.written = append(s.written, n.Title)
	return domain.Slice{ID: n.Title, Name: n.Title, MilestoneID: n.Milestone.ID}, nil
}
