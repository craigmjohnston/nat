package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/store"
)

func stubHomeDir(t *testing.T, dir string, err error) {
	t.Helper()
	previous := homeDir
	homeDir = func() (string, error) { return dir, err }
	t.Cleanup(func() { homeDir = previous })
}

func TestScratchOpenCreatesOnceThenReusesIt(t *testing.T) {
	stubHomeDir(t, "/home/craig", nil)
	env, out, saved := noNotionEnv(t, config.Config{}, false)

	if err := Run(context.Background(), []string{"scratch-open", "--json"}, env); err != nil {
		t.Fatalf("first scratch-open: %v", err)
	}
	var first scratchOpenJSON
	if err := json.Unmarshal(out.Bytes(), &first); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if !first.Created || first.ID == "" {
		t.Fatalf("first call = %+v, want created with an ID", first)
	}
	if saved.ScratchProject != first.ID {
		t.Errorf("scratch_project = %q, want %q", saved.ScratchProject, first.ID)
	}
	p, ok := saved.Projects[first.ID]
	if !ok || !p.IsLocal() || p.Name != "Scratch" || p.WorkingDir != "/home/craig" {
		t.Errorf("project entry = %+v (present %v), want local Scratch in the home directory", p, ok)
	}

	out.Reset()
	// --dir is for creation only: a later call ignores it and creates nothing.
	if err := Run(context.Background(), []string{"scratch-open", "--json", "--dir", "/elsewhere"}, env); err != nil {
		t.Fatalf("second scratch-open: %v", err)
	}
	var second scratchOpenJSON
	if err := json.Unmarshal(out.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second.Created || second.ID != first.ID {
		t.Errorf("second call = %+v, want the same ID, not created", second)
	}
	if got := saved.Projects[first.ID].WorkingDir; got != "/home/craig" {
		t.Errorf("working dir changed to %q on a later call", got)
	}
	if len(saved.Projects) != 1 {
		t.Errorf("%d projects, want 1", len(saved.Projects))
	}
}

func TestScratchOpenPrintsBareIDAndHonoursDir(t *testing.T) {
	env, out, saved := noNotionEnv(t, config.Config{}, false)
	if err := Run(context.Background(), []string{"scratch-open", "--dir", "/work"}, env); err != nil {
		t.Fatalf("scratch-open: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != saved.ScratchProject || got == "" {
		t.Errorf("printed %q, want the bare ID %q", got, saved.ScratchProject)
	}
	if got := saved.Projects[saved.ScratchProject].WorkingDir; got != "/work" {
		t.Errorf("working dir = %q, want /work", got)
	}
}

func TestScratchOpenRecreatesWhenConfigLostTheEntry(t *testing.T) {
	env, _, saved := noNotionEnv(t, config.Config{ScratchProject: "dangling"}, true)
	if err := Run(context.Background(), []string{"scratch-open"}, env); err != nil {
		t.Fatalf("scratch-open: %v", err)
	}
	if saved.ScratchProject == "dangling" || !saved.Projects[saved.ScratchProject].IsLocal() {
		t.Errorf("scratch_project = %q, want a fresh local project", saved.ScratchProject)
	}
}

func TestScratchOpenFailures(t *testing.T) {
	ctx := context.Background()

	env, _, _ := noNotionEnv(t, config.Config{}, false)
	if err := Run(ctx, []string{"scratch-open", "extra"}, env); err == nil {
		t.Error("a positional argument was accepted")
	}
	if err := Run(ctx, []string{"scratch-open", "--bogus"}, env); err == nil {
		t.Error("an unknown flag was accepted")
	}

	stubHomeDir(t, "", errors.New("no home"))
	if err := Run(ctx, []string{"scratch-open"}, env); err == nil || !strings.Contains(err.Error(), "home directory") {
		t.Errorf("no home: err = %v", err)
	}
	stubHomeDir(t, "/h", nil)

	loadErr := errors.New("unreadable")
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, loadErr }
	if err := Run(ctx, []string{"scratch-open"}, env); !errors.Is(err, loadErr) {
		t.Errorf("load failure: err = %v", err)
	}

	// A plan that cannot be laid down is reported, and nothing is recorded.
	env1, _, saved1 := noNotionEnv(t, config.Config{}, false)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", blocker)
	t.Setenv("HOME", blocker)
	if err := Run(ctx, []string{"scratch-open"}, env1); err == nil || !strings.Contains(err.Error(), "create the plan") {
		t.Errorf("unwritable plan dir: err = %v", err)
	}
	if saved1.ScratchProject != "" {
		t.Errorf("scratch_project = %q after a failed create", saved1.ScratchProject)
	}

	// A save that fails after the plan was made is reported, not swallowed.
	env2, _, _ := noNotionEnv(t, config.Config{}, false)
	saves := 0
	env2.Save = func(config.Config) error {
		saves++
		if saves == 2 {
			return errors.New("disk full")
		}
		return nil
	}
	loaded := config.Config{}
	env2.Load = func() (config.Config, bool, error) { return loaded, true, nil }
	if err := Run(ctx, []string{"scratch-open"}, env2); err == nil || !strings.Contains(err.Error(), "save config") {
		t.Errorf("second save failure: err = %v", err)
	}

	// The reload between the two writes failing is reported too.
	env3, _, _ := noNotionEnv(t, config.Config{}, false)
	loads := 0
	env3.Load = func() (config.Config, bool, error) {
		loads++
		if loads == 3 {
			return config.Config{}, false, loadErr
		}
		return config.Config{}, true, nil
	}
	if err := Run(ctx, []string{"scratch-open"}, env3); !errors.Is(err, loadErr) {
		t.Errorf("reload failure: err = %v", err)
	}
}

// scratchWithWork opens a scratch project and fills its plan: milestone
// "Finished" holding one Done slice, "Mixed" holding a Done and a Todo one,
// "Empty" holding nothing, plus one ended and one running session.
func scratchWithWork(t *testing.T) (Env, string, store.Store, store.Project, func() string) {
	t.Helper()
	ctx := context.Background()
	env, out, saved := noNotionEnv(t, config.Config{}, false)
	if err := Run(ctx, []string{"scratch-open"}, env); err != nil {
		t.Fatal(err)
	}
	id := saved.ScratchProject
	entry := saved.Projects[id]
	st, err := env.storeFor(ctx, id, entry)
	if err != nil {
		t.Fatal(err)
	}
	sp := storeProject(id, entry)
	sh, err := st.Shape(ctx, sp)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := st.AddMilestones(ctx, sp, sh, []string{"Finished", "Mixed", "Empty"})
	if err != nil {
		t.Fatal(err)
	}
	add := func(title string, m domain.Milestone, done bool) {
		s, err := st.AddSlice(ctx, sp, store.NewSlice{Title: title, Milestone: m})
		if err != nil {
			t.Fatal(err)
		}
		if done {
			if err := st.MarkDone(ctx, s.ID, sh); err != nil {
				t.Fatal(err)
			}
		}
	}
	add("done one", ms[0], true)
	add("done two", ms[1], true)
	add("still todo", ms[1], false)
	for _, sid := range []string{"ended", "running"} {
		if _, err := st.AddSession(ctx, sp, store.NewSession{ID: sid, Dir: "/d"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.EndSession(ctx, "ended"); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	return env, id, st, sp, out.String
}

func TestDoneClearRemovesDoneEndedAndEmpty(t *testing.T) {
	ctx := context.Background()
	env, id, st, sp, _ := scratchWithWork(t)
	out := env.Out.(interface {
		String() string
		Reset()
	})

	if err := Run(ctx, []string{"done-clear", "--project", id, "--json"}, env); err != nil {
		t.Fatalf("done-clear: %v", err)
	}
	var got doneClearJSON
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if strings.Join(got.Slices, ",") != "done one,done two" ||
		strings.Join(got.Sessions, ",") != "ended" ||
		strings.Join(got.Milestones, ",") != "Finished,Empty" {
		t.Errorf("reported %+v", got)
	}

	plan, err := st.Plan(ctx, sp)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Project.Slices) != 1 || plan.Project.Slices[0].Name != "still todo" {
		t.Errorf("slices left = %+v, want only the Todo one", plan.Project.Slices)
	}
	if len(plan.Project.Milestones) != 1 || plan.Project.Milestones[0].Name != "Mixed" {
		t.Errorf("milestones left = %+v, want only Mixed", plan.Project.Milestones)
	}
	sessions, err := st.Sessions(ctx, sp)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].ID != "running" {
		t.Errorf("sessions left = %+v, want only the running one", sessions)
	}

	// Nothing left to clear is not an error, and says so.
	out.Reset()
	if err := Run(ctx, []string{"done-clear", "--project", id}, env); err != nil {
		t.Fatalf("second done-clear: %v", err)
	}
	if !strings.Contains(out.String(), "Removed 0 Done slices, 0 ended sessions and 0 empty milestones") {
		t.Errorf("second report = %q", out.String())
	}
}

func TestDoneClearMarkdownListsWhatWasRemoved(t *testing.T) {
	ctx := context.Background()
	env, id, _, _, _ := scratchWithWork(t)
	out := env.Out.(interface{ String() string })
	if err := Run(ctx, []string{"done-clear", "--project", id}, env); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Removed 2 Done slices, 1 ended sessions and 2 empty milestones", "- done one", "## Sessions", "- ended", "## Milestones", "- Empty"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}
}

func TestDoneClearRefusesAProjectWithAWorkspace(t *testing.T) {
	cfg := config.Config{Projects: map[string]config.ProjectConfig{
		"3b738308-f654-811c-948d-e1fb36f71df3": {Name: "Real", SlicesDSID: "ds", WorkingDir: "/r"},
	}}
	env, _, _ := noNotionEnv(t, cfg, true)
	err := Run(context.Background(), []string{"done-clear", "--project", "3b738308-f654-811c-948d-e1fb36f71df3"}, env)
	if err == nil || !strings.Contains(err.Error(), `"Real"`) || !strings.Contains(err.Error(), "workspace") {
		t.Errorf("err = %v, want a refusal naming the project", err)
	}
}

func TestDoneClearUsageErrors(t *testing.T) {
	ctx := context.Background()
	env, id, _, _, _ := scratchWithWork(t)
	for name, args := range map[string][]string{
		"no project":  {"done-clear"},
		"positional":  {"done-clear", "--project", id, "x"},
		"bad flag":    {"done-clear", "--bogus"},
		"bad project": {"done-clear", "--project", "nope"},
	} {
		if err := Run(ctx, args, env); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestDoneClearReportsEachStoreFailure breaks the plan file underneath the
// store, one way per case, so every read and write done-clear makes is shown to
// stop the command and say which step it was.
func TestDoneClearReportsEachStoreFailure(t *testing.T) {
	for name, tc := range map[string]struct {
		sql  string
		want string
	}{
		"plan read":        {`DROP TABLE milestones`, "read the plan"},
		"slice delete":     {`CREATE TRIGGER t BEFORE DELETE ON slices BEGIN SELECT RAISE(ABORT, 'no'); END`, "delete slice"},
		"sessions read":    {`DROP TABLE sessions`, "read the sessions"},
		"session delete":   {`CREATE TRIGGER t BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT, 'no'); END`, "delete session"},
		"plan re-read":     {`CREATE TRIGGER t AFTER DELETE ON sessions BEGIN UPDATE milestones SET position = 'x'; END`, "re-read the plan"},
		"milestone remove": {`CREATE TRIGGER t BEFORE DELETE ON milestones BEGIN SELECT RAISE(ABORT, 'no'); END`, "remove milestone"},
		"shape re-read":    {`CREATE TRIGGER t AFTER UPDATE ON milestones BEGIN UPDATE milestones SET position = 'x'; END`, "re-read the shape"},
	} {
		t.Run(name, func(t *testing.T) {
			env, id, _, _, _ := scratchWithWork(t)
			path, err := store.LocalPath(id)
			if err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite3", "file:"+path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.sql); err != nil {
				t.Fatalf("break the plan: %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			err = Run(context.Background(), []string{"done-clear", "--project", id}, env)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestDoneClearReportsAnUnopenablePlan(t *testing.T) {
	env, id, _, _, _ := scratchWithWork(t)
	path, err := store.LocalPath(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"done-clear", "--project", id}, env); err == nil {
		t.Error("an unopenable plan was cleared")
	}
}

func TestConfigShowNamesTheScratchProject(t *testing.T) {
	cfg := config.Config{ScratchProject: "abc", Projects: map[string]config.ProjectConfig{"abc": {Name: "Scratch", Backend: config.BackendLocal}}}
	env, out, _ := noNotionEnv(t, cfg, true)
	if err := Run(context.Background(), []string{"config-show", "--json"}, env); err != nil {
		t.Fatal(err)
	}
	var doc configDoc
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil || doc.ScratchProject != "abc" {
		t.Errorf("json scratch_project = %q (%v)", doc.ScratchProject, err)
	}
	out.Reset()
	if err := Run(context.Background(), []string{"config-show"}, env); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Scratch project: abc") {
		t.Errorf("markdown lacks the scratch project:\n%s", out.String())
	}
}
