package cli

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/source"
)

// assigningSave wraps a test's Save in what config.Save does first — fill in
// every missing colour, into the map it was handed — so a command's report of
// the colour it was given can be read.
func assigningSave(save func(config.Config) error) func(config.Config) error {
	return func(c config.Config) error {
		c.AssignColors()
		return save(c)
	}
}

func createdColor(t *testing.T, out []byte) string {
	t.Helper()
	var doc projectCreatedJSON
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	return doc.Project.Color
}

// Each form of project-create reports the colour its save gave the project.
func TestProjectCreateReportsTheColorItWasGiven(t *testing.T) {
	ctx := context.Background()

	cfg := workspaceConfig()
	cfg.Projects = map[string]config.ProjectConfig{"other": {Color: "red"}}
	env, out, saved := createEnv(t, cfg, &fakeAPI{})
	env.Save = assigningSave(env.Save)
	if err := Run(ctx, []string{"project-create", "nat", "--repo", "/src/nat", "--json"}, env); err != nil {
		t.Fatal(err)
	}
	if got := createdColor(t, out.Bytes()); got != "orange" || saved.Projects["new-project"].Color != "orange" {
		t.Errorf("notion: reported %q, saved %+v", got, saved.Projects)
	}

	env, lout, _ := noNotionEnv(t, config.Config{}, false)
	env.Save = assigningSave(env.Save)
	if err := Run(ctx, []string{"project-create", "Mine", "--local", "--repo", "/src/mine", "--json"}, env); err != nil {
		t.Fatal(err)
	}
	if got := createdColor(t, lout.Bytes()); got != "red" {
		t.Errorf("local: reported %q, want red", got)
	}

	sp := newSourceProject(t, &source.Fake{})
	first := sp.saved.Projects[sp.id]
	first.Color = "red"
	sp.saved.Projects[sp.id] = first
	sp.env.Save = assigningSave(sp.env.Save)
	if got := createdColor(t, []byte(sp.run(t, "project-create", "--source", "demo", "--plan-dir", sp.planDir, "--json"))); got != "orange" {
		t.Errorf("source: reported %q, want orange (red is held)", got)
	}
}

// A project mirrored into Notion keeps the colour its local entry had.
func TestProjectMirrorCarriesTheColor(t *testing.T) {
	api := &fakeAPI{}
	env, out, saved, oldID := mirrorEnv(t, api)
	p := saved.Projects[oldID]
	p.Color = "teal"
	saved.Projects[oldID] = p

	if err := Run(context.Background(), []string{"project-mirror", "--project", oldID,
		"--parent", "parent-page", "--parent-kind", "page", "--json"}, env); err != nil {
		t.Fatalf("project-mirror: %v", err)
	}
	if got := saved.Projects["new-project"].Color; got != "teal" {
		t.Errorf("mirrored entry's colour = %q, want teal", got)
	}
	var got projectMirroredJSON
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatal(err)
	}
	if got.Project.Color != "teal" {
		t.Errorf("reported colour = %q, want teal", got.Project.Color)
	}
}
