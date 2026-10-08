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

	"github.com/craigmjohnston/nat/internal/source"
)

// repoOf reads a task's repository back through slice-show.
func (sp *sourceProject) repoOf(t *testing.T, id string) string {
	t.Helper()
	var d struct {
		Repo string `json:"repo"`
	}
	if err := json.Unmarshal([]byte(sp.run(t, "slice-show", id, "--json", "--project", sp.id)), &d); err != nil {
		t.Fatal(err)
	}
	return d.Repo
}

// claimTask puts a task in progress for whoever the project's tasks are worked
// by, as a launch's claim would.
func (sp *sourceProject) claimTask(t *testing.T, id string) {
	t.Helper()
	cfg, projectID, project, err := sp.env.projectFor(sp.id)
	if err != nil {
		t.Fatal(err)
	}
	st, err := sp.env.storeFor(context.Background(), projectID, project)
	if err != nil {
		t.Fatal(err)
	}
	sh, err := sliceShape(context.Background(), st, projectID, project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ClaimSlice(context.Background(), id, sh, cfg.AssigneeUserID); err != nil {
		t.Fatal(err)
	}
}

func TestSliceRepoRecordsTheRepository(t *testing.T) {
	sp := newSourceProject(t, &source.Fake{Details: map[string]source.ContainerDetail{"c1": {ID: "c1", Title: "Card"}}})
	task := sp.addTask(t, "Task", "c1")
	repo := t.TempDir()

	// A Todo task takes one, as given; the text form says what was recorded.
	if out := sp.run(t, "slice-repo", task, "--repo", repo, "--project", sp.id); out != "# Task\n\nRepository recorded: "+repo+"\n" {
		t.Errorf("text = %q", out)
	}
	if got := sp.repoOf(t, task); got != repo {
		t.Errorf("repo = %q, want %q", got, repo)
	}
	if sp.nudges == 0 {
		t.Error("the board was not nudged")
	}

	// One in progress, held by the caller: a relative path is made absolute
	// against where the command was run.
	sp.claimTask(t, task)
	stubGetwd(t, filepath.Dir(repo), nil)
	var got sliceRepoJSON
	if err := json.Unmarshal([]byte(sp.run(t, "slice-repo", task, "--repo", filepath.Base(repo), "--json", "--project", sp.id)), &got); err != nil {
		t.Fatal(err)
	}
	if got != (sliceRepoJSON{ID: task, Name: "Task", Repo: repo}) {
		t.Errorf("json = %+v", got)
	}
}

func TestSliceRepoRefusals(t *testing.T) {
	sp := newSourceProject(t, &source.Fake{Details: map[string]source.ContainerDetail{"c1": {ID: "c1", Title: "Card"}}})
	task := sp.addTask(t, "Task", "c1")
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"--repo", "/x", "--project", sp.id}, "want exactly one slice"},
		{[]string{"not-an-id", "--repo", "/x", "--project", sp.id}, "slice-repo"},
		{[]string{task, "--project", sp.id}, "no --repo given"},
		{[]string{task, "--repo", "/no/such/dir", "--project", sp.id}, "/no/such/dir is not there"},
		{[]string{task, "--repo", file, "--project", sp.id}, "is not a directory"},
		{[]string{task, "--repo", t.TempDir()}, "--project"},
		{[]string{task, "--repo", t.TempDir(), "--bogus", "--project", sp.id}, "flag provided but not defined"},
	} {
		if err := sp.fail(t, append([]string{"slice-repo"}, tt.args...)...); !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%v: err = %v, want %q", tt.args, err, tt.want)
		}
	}

	// Done is nobody's to change.
	sp.claimTask(t, task)
	sp.run(t, "complete-slice", task, "--no-branch", "--summary", "done", "--project", sp.id)
	if err := sp.fail(t, "slice-repo", task, "--repo", t.TempDir(), "--project", sp.id); !strings.Contains(err.Error(), "only a slice you claimed can be given a repository") {
		t.Errorf("done: err = %v", err)
	}

	// A plan that will not open, and a relative path with nowhere to resolve it.
	sp.env.NewSource = func(string) (source.Client, error) { return nil, errors.New("gone") }
	stubGetwd(t, "", errors.New("no cwd"))
	if err := sp.fail(t, "slice-repo", task, "--repo", "rel", "--project", sp.id); !strings.Contains(err.Error(), "no cwd") {
		t.Errorf("no cwd: err = %v", err)
	}
}

// A Notion project's slice names its repository on the board instead.
func TestSliceRepoRefusesANotionProject(t *testing.T) {
	env, _ := testEnv(testClaimConfig(t), startableAPI(t))
	err := Run(context.Background(), []string{"slice-repo", startSliceID, "--repo", t.TempDir(), "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "keeps its plan in Notion") {
		t.Errorf("err = %v, want the Notion project refused", err)
	}
}

// A plan that will not open, will not say its shape, has no such slice, or
// refuses the write is reported as itself.
func TestSliceRepoReportsTheStoresFailures(t *testing.T) {
	sp := newSourceProject(t, &source.Fake{Details: map[string]source.ContainerDetail{"c1": {ID: "c1", Title: "Card"}}})
	task := sp.addTask(t, "Task", "c1")
	repo := t.TempDir()

	exec := func(stmt string) {
		t.Helper()
		db, err := sql.Open("sqlite3", "file:"+sp.planPath(t))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE TRIGGER no_repo BEFORE UPDATE ON slices BEGIN SELECT RAISE(ABORT, 'refused'); END`)
	if err := sp.fail(t, "slice-repo", task, "--repo", repo, "--project", sp.id); !strings.Contains(err.Error(), "record the slice's repository") {
		t.Errorf("refused write: err = %v", err)
	}
	if err := sp.fail(t, "slice-repo", "00000000-0000-4000-8000-000000000009", "--repo", repo, "--project", sp.id); !strings.Contains(err.Error(), "load the slice") {
		t.Errorf("no such slice: err = %v", err)
	}
	exec(`DROP TABLE milestones`)
	if err := sp.fail(t, "slice-repo", task, "--repo", repo, "--project", sp.id); err == nil {
		t.Error("unreadable shape: want an error")
	}

	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	entry := sp.saved.Projects[sp.id]
	entry.PlanDir = filepath.Join(file, "plans")
	sp.saved.Projects[sp.id] = entry
	if err := sp.fail(t, "slice-repo", task, "--repo", repo, "--project", sp.id); !strings.Contains(err.Error(), "a-file") {
		t.Errorf("plan that will not open: err = %v", err)
	}
}
