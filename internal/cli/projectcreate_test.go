package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

// workspaceConfig is a machine that has been through onboarding but is not
// tracking anything yet: a projects database, and no project at all — which is
// exactly where the first project-create is run.
func workspaceConfig() config.Config {
	return config.Config{
		ProjectDBID:           "projects-db",
		ProjectDBDataSourceID: "projects-ds",
	}
}

// createEnv is an Env over a workspace, with the config write captured rather
// than let anywhere near the real config file.
func createEnv(t *testing.T, cfg config.Config, api *fakeAPI) (Env, *bytes.Buffer, *config.Config) {
	t.Helper()
	env, out := testEnv(cfg, api)
	saved := &config.Config{}
	env.Save = func(c config.Config) error {
		*saved = c
		return nil
	}
	stubGetwd(t, "/tmp/typed-here", nil)
	return env, out, saved
}

// stubGetwd stands in for the directory the command was typed in.
func stubGetwd(t *testing.T, dir string, err error) {
	t.Helper()
	previous := getwd
	getwd = func() (string, error) { return dir, err }
	t.Cleanup(func() { getwd = previous })
}

func TestProjectCreateBuildsTheProjectAndRegistersIt(t *testing.T) {
	api := &fakeAPI{}
	env, out, saved := createEnv(t, workspaceConfig(), api)

	err := Run(context.Background(), []string{"project-create", "  nat  ",
		"--repo", "/src/nat", "--description", "Small slices.\n\nOne PR each."}, env)
	if err != nil {
		t.Fatalf("project-create: %v", err)
	}

	if len(api.projects) != 1 {
		t.Fatalf("projects = %+v, want exactly one", api.projects)
	}
	if got := api.projects[0]; got.dsID != "projects-ds" || got.name != "nat" {
		t.Errorf("created %+v, want %q under the configured projects data source", got, "nat")
	}
	// The description is the page body, which is what `nat info` prints back as
	// the project's conventions.
	if len(api.appends) != 1 || api.appends[0].id != "new-project" {
		t.Fatalf("appends = %+v, want the body written on the new page", api.appends)
	}
	body := api.appends[0].children
	if len(body) != 2 {
		t.Fatalf("body = %+v, want one paragraph per chunk", body)
	}
	if got := paragraphText(t, body[0]); got != "Small slices." {
		t.Errorf("first paragraph = %q, want %q", got, "Small slices.")
	}
	if got := paragraphText(t, body[1]); got != "One PR each." {
		t.Errorf("second paragraph = %q, want %q", got, "One PR each.")
	}

	want := config.ProjectConfig{Name: "nat", SlicesDSID: "new-slices-ds", WorkingDir: "/src/nat"}
	if got := saved.Projects["new-project"]; got != want {
		t.Errorf("saved project = %+v, want %+v", got, want)
	}
	for _, want := range []string{"# nat", "new-project", "https://notion.so/new-project",
		"new-slices-ds", "/src/nat"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output = %q, want it to mention %q", out.String(), want)
		}
	}
}

// The board's switch picker is how a new project gets opened: the CLI has no
// project switch, so moving the active project here would point every later
// headless command at an empty plan.
func TestProjectCreateLeavesTheActiveProjectAlone(t *testing.T) {
	cfg := workspaceConfig()
	cfg.ActiveProjectID = "project-1"
	cfg.Projects = map[string]config.ProjectConfig{"project-1": {Name: "nat"}}
	env, out, saved := createEnv(t, cfg, &fakeAPI{})

	if err := Run(context.Background(), []string{"project-create", "other"}, env); err != nil {
		t.Fatalf("project-create: %v", err)
	}

	if saved.ActiveProjectID != "project-1" {
		t.Errorf("active project = %q, want the one it was on", saved.ActiveProjectID)
	}
	if len(saved.Projects) != 2 {
		t.Errorf("projects = %+v, want the new one alongside the old", saved.Projects)
	}
	if !strings.Contains(out.String(), "press P") {
		t.Errorf("output = %q, want it to say how to open the project", out.String())
	}
}

// With no --repo the project's agents work where the command was typed, which
// is the answer for a project created from inside its own checkout.
func TestProjectCreateDefaultsTheWorkingDirToTheCurrentOne(t *testing.T) {
	env, _, saved := createEnv(t, workspaceConfig(), &fakeAPI{})

	if err := Run(context.Background(), []string{"project-create", "nat"}, env); err != nil {
		t.Fatalf("project-create: %v", err)
	}

	if got := saved.Projects["new-project"].WorkingDir; got != "/tmp/typed-here" {
		t.Errorf("working dir = %q, want the current directory", got)
	}
}

func TestProjectCreateReportsAnUnreadableWorkingDir(t *testing.T) {
	want := errors.New("no such directory")
	api := &fakeAPI{}
	env, _, _ := createEnv(t, workspaceConfig(), api)
	stubGetwd(t, "", want)

	err := Run(context.Background(), []string{"project-create", "nat"}, env)

	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	// It failed before Notion was touched, so there is no project to explain.
	if len(api.projects) != 0 {
		t.Errorf("projects = %+v, want nothing created", api.projects)
	}
}

// The Slices table follows the configured user: a workspace where claiming
// writes a person needs somewhere to write them, and one where it does not is
// better off without a column nobody fills.
func TestProjectCreateTracksAnAssigneeWhenTheConfigNamesOne(t *testing.T) {
	tests := []struct {
		name   string
		userID string
		want   bool
		note   string
	}{
		{name: "a configured user", userID: "user-1", want: true, note: "Slices track an assignee"},
		{name: "no user at all", want: false, note: "Slices track no assignee"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := workspaceConfig()
			cfg.AssigneeUserID = tt.userID
			api := &fakeAPI{}
			env, out, _ := createEnv(t, cfg, api)

			if err := Run(context.Background(), []string{"project-create", "nat"}, env); err != nil {
				t.Fatalf("project-create: %v", err)
			}

			if got := api.projects[0].assignee; got != tt.want {
				t.Errorf("assignee = %v, want %v", got, tt.want)
			}
			if !strings.Contains(out.String(), tt.note) {
				t.Errorf("output = %q, want it to mention %q", out.String(), tt.note)
			}
		})
	}
}

// An empty description writes no body at all, the same way an empty slice brief
// does: a project whose conventions are not written yet is a real thing.
func TestProjectCreateWithNoDescriptionWritesNoBody(t *testing.T) {
	api := &fakeAPI{}
	env, _, _ := createEnv(t, workspaceConfig(), api)

	if err := Run(context.Background(), []string{"project-create", "nat"}, env); err != nil {
		t.Fatalf("project-create: %v", err)
	}

	if len(api.appends) != 0 {
		t.Errorf("appends = %+v, want none", api.appends)
	}
}

func TestProjectCreateReadsTheDescriptionFromStdin(t *testing.T) {
	api := &fakeAPI{}
	env, _, _ := createEnv(t, workspaceConfig(), api)
	env.In = strings.NewReader("  Small slices.  ")

	if err := Run(context.Background(), []string{"project-create", "nat", "--description", "-"}, env); err != nil {
		t.Fatalf("project-create: %v", err)
	}

	if len(api.appends) != 1 || len(api.appends[0].children) != 1 {
		t.Fatalf("appends = %+v, want the piped conventions", api.appends)
	}
	if got := paragraphText(t, api.appends[0].children[0]); got != "Small slices." {
		t.Errorf("paragraph = %q, want the piped conventions", got)
	}
}

// --description - with nothing to read is a misuse, named for the command that
// made it rather than for whichever one shares the helper.
func TestProjectCreateRejectsAPipedDescriptionWithNoStdin(t *testing.T) {
	env, _, _ := createEnv(t, workspaceConfig(), &fakeAPI{})
	env.In = nil

	err := Run(context.Background(), []string{"project-create", "nat", "--description", "-"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
	if !strings.Contains(err.Error(), "project-create:") {
		t.Errorf("err = %q, want it to name the command", err)
	}
}

func TestProjectCreatePrintsJSON(t *testing.T) {
	cfg := workspaceConfig()
	cfg.AssigneeUserID = "user-1"
	env, out, _ := createEnv(t, cfg, &fakeAPI{})

	if err := Run(context.Background(), []string{"project-create", "nat", "--repo", "/src/nat", "--json"}, env); err != nil {
		t.Fatalf("project-create: %v", err)
	}

	var doc projectCreatedJSON
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("output %q is not JSON: %v", out.String(), err)
	}
	want := createdProjectJSON{
		ID:         "new-project",
		Name:       "nat",
		URL:        "https://notion.so/new-project",
		SlicesDBID: "new-slices-db",
		SlicesDSID: "new-slices-ds",
		WorkingDir: "/src/nat",
		Assignee:   true,
	}
	if doc.Project != want {
		t.Errorf("project = %+v, want %+v", doc.Project, want)
	}
}

func TestProjectCreateReportsAFailedCreation(t *testing.T) {
	want := errors.New("notion is down")
	env, out, saved := createEnv(t, workspaceConfig(), &fakeAPI{projectErr: want})

	err := Run(context.Background(), []string{"project-create", "nat"}, env)

	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want nothing", out.String())
	}
	if len(saved.Projects) != 0 {
		t.Errorf("saved = %+v, want nothing recorded", saved.Projects)
	}
}

// A create that comes back with neither a project nor a reason is still a
// failure, and one worth saying out loud rather than dereferencing.
func TestProjectCreateReportsAnEmptyCreation(t *testing.T) {
	env, _, _ := createEnv(t, workspaceConfig(), &fakeAPI{projectNothing: true})

	err := Run(context.Background(), []string{"project-create", "nat"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project was returned") {
		t.Errorf("err = %v, want the empty answer reported", err)
	}
}

// A project that exists but whose schema did not read back is still a project:
// it is recorded and printed, and the mismatch reported alongside rather than
// instead.
func TestProjectCreateRecordsAProjectWhoseSchemaDidNotVerify(t *testing.T) {
	want := errors.New("Slices schema: missing property \"PR\"")
	api := &fakeAPI{
		projectErr: want,
		projectStructure: &notion.ProjectStructure{
			PageID: "new-project", SlicesDSID: "new-slices-ds",
		},
	}
	env, out, saved := createEnv(t, workspaceConfig(), api)

	err := Run(context.Background(), []string{"project-create", "nat"}, env)

	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if _, ok := saved.Projects["new-project"]; !ok {
		t.Errorf("saved = %+v, want the project recorded anyway", saved.Projects)
	}
	if !strings.Contains(out.String(), "new-project") {
		t.Errorf("output = %q, want the project printed anyway", out.String())
	}
}

// A body that did not land is worth saying, but the project is made and
// recorded: the conventions can be typed into the page afterwards.
func TestProjectCreateReportsAFailedPageBody(t *testing.T) {
	want := errors.New("no room")
	env, _, saved := createEnv(t, workspaceConfig(), &fakeAPI{appendErr: want})

	err := Run(context.Background(), []string{"project-create", "nat", "--description", "Small slices."}, env)

	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if _, ok := saved.Projects["new-project"]; !ok {
		t.Errorf("saved = %+v, want the project recorded anyway", saved.Projects)
	}
}

// A config that could not be written is the one failure that loses the project
// on this machine, so it is reported — but the IDs are printed first, since
// they are then the only record of what was made.
func TestProjectCreateReportsAFailedConfigWrite(t *testing.T) {
	want := errors.New("read-only")
	env, out, _ := createEnv(t, workspaceConfig(), &fakeAPI{})
	env.Save = func(config.Config) error { return want }

	err := Run(context.Background(), []string{"project-create", "nat"}, env)

	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if !strings.Contains(out.String(), "new-slices-ds") {
		t.Errorf("output = %q, want the IDs printed anyway", out.String())
	}
}

// A failed print is reported alongside whatever else went wrong rather than in
// place of it: an error with nowhere to be written is still an error.
func TestProjectCreateReportsAFailedWrite(t *testing.T) {
	env, _, _ := createEnv(t, workspaceConfig(), &fakeAPI{})
	env.Out = failingWriter{}

	err := Run(context.Background(), []string{"project-create", "nat"}, env)

	if !errors.Is(err, errWrite) {
		t.Errorf("err = %v, want %v", err, errWrite)
	}
}

func TestProjectCreateNudgesTheBoard(t *testing.T) {
	env, _, _ := createEnv(t, workspaceConfig(), &fakeAPI{})
	nudged := 0
	env.Nudge = func() { nudged++ }

	if err := Run(context.Background(), []string{"project-create", "nat"}, env); err != nil {
		t.Fatalf("project-create: %v", err)
	}

	if nudged != 1 {
		t.Errorf("nudged %d times, want once", nudged)
	}
}

func TestProjectCreateRejectsAMisuse(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no name", args: []string{"project-create"}, want: "given 0"},
		{name: "two names", args: []string{"project-create", "a", "b"}, want: "given 2"},
		{name: "an empty name", args: []string{"project-create", "   "}, want: "name is empty"},
		{name: "an unknown flag", args: []string{"project-create", "nat", "--nope"}, want: "project-create:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{}
			env, _, _ := createEnv(t, workspaceConfig(), api)

			err := Run(context.Background(), tt.args, env)

			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("err = %v (%T), want a *UsageError", err, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if len(api.projects) != 0 {
				t.Errorf("projects = %+v, want nothing created", api.projects)
			}
		})
	}
}

// Creating a project needs the projects database onboarding picked, and nothing
// else: no active project, because there may not be one yet.
func TestProjectCreateReportsUnfinishedSetup(t *testing.T) {
	tests := []struct {
		name  string
		cfg   config.Config
		found bool
		err   error
		want  string
	}{
		{name: "no config file", found: false, want: "run `nat` once to set it up"},
		{name: "no projects database", cfg: config.Config{}, found: true, want: "no projects database"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, _, _ := createEnv(t, config.Config{}, &fakeAPI{})
			env.Load = func() (config.Config, bool, error) { return tt.cfg, tt.found, nil }

			err := Run(context.Background(), []string{"project-create", "nat"}, env)

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestProjectCreateReportsAFailedConfigRead(t *testing.T) {
	want := errors.New("disk gone")
	env, _, _ := createEnv(t, config.Config{}, &fakeAPI{})
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, want }

	err := Run(context.Background(), []string{"project-create", "nat"}, env)

	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}
