package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// releasableAPI answers with one slice in progress, held by the configured
// user: the state a session that died leaves behind, and the only one this
// command has anything to do with.
func releasableAPI() *fakeAPI {
	api := completableAPI()
	api.dataSources = map[string]notion.DataSource{"slices-ds": assigneeSlicesDS("m2")}
	return api
}

// releaseEnv builds an Env for the command; it reads nothing from stdin, since
// release-slice takes no note to write.
func releaseEnv(t testing.TB, api *fakeAPI) (Env, *strings.Builder) {
	env, _ := testEnv(testClaimConfig(t), api)
	var out strings.Builder
	env.Out = &out
	return env, &out
}

// TestReleaseSliceHandsTheSliceBack is the whole command: Todo, held by
// nobody, one line on the page, and nothing else written.
func TestReleaseSliceHandsTheSliceBack(t *testing.T) {
	api := releasableAPI()
	env, out := releaseEnv(t, api)
	var nudges int
	env.Nudge = func() { nudges++ }

	if err := Run(context.Background(), []string{"release-slice", sliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("release-slice: %v", err)
	}

	if len(api.appends) != 1 || api.appends[0].id != sliceID {
		t.Fatalf("appends = %+v, want exactly one, to the slice", api.appends)
	}
	want := []string{"paragraph: Released back to Todo by Craig Johnston: " +
		"the session working it ended without finishing it."}
	if got := blockTexts(t, api.appends[0].children); !equalLines(got, want) {
		t.Errorf("blocks = %v, want %v", got, want)
	}

	if len(api.updates) != 1 || api.updates[0].id != sliceID {
		t.Fatalf("updates = %+v, want exactly one, to the slice", api.updates)
	}
	props := api.updates[0].props
	if name := props[notion.PropStatus].SelectName(); name != notion.SliceTodo {
		t.Errorf("status = %q, want %q", name, notion.SliceTodo)
	}
	assignee, cleared := props[notion.PropAssignee]
	if !cleared {
		t.Fatalf("props = %+v, want the assignee cleared", props)
	}
	if assignee.People == nil || len(*assignee.People) != 0 {
		t.Errorf("assignee = %+v, want the empty list that clears it", assignee)
	}
	// Everything else on the page is what the next session picks the slice up
	// with, so the write says nothing about any of it.
	for _, left := range []string{notion.PropPR, notion.PropDependsOn, notion.PropRepo, notion.PropName} {
		if _, wrote := props[left]; wrote {
			t.Errorf("props = %+v, want %s left alone", props, left)
		}
	}
	if nudges != 1 {
		t.Errorf("nudged %d times, want exactly once", nudges)
	}

	wantOut := fmt.Sprintf(`# Render the board

Released. It is Todo and unassigned, and the note is on its page.

- Notion page: %[1]s
- Notion URL: https://notion.so/%[1]s
`, sliceID)
	if out.String() != wantOut {
		t.Errorf("output =\n%s\nwant\n%s", out.String(), wantOut)
	}
}

// A project with no Assignee column has nobody to clear: ownership there is
// decided on status alone, so the write is the status and nothing else.
func TestReleaseSliceWithoutAnAssigneeColumn(t *testing.T) {
	api := releasableAPI()
	api.dataSources = map[string]notion.DataSource{"slices-ds": soloSlicesDS()}
	delete(api.pages["slices-ds"][0].Properties, notion.PropAssignee)
	env, _ := releaseEnv(t, api)

	if err := Run(context.Background(), []string{"release-slice", sliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("release-slice: %v", err)
	}

	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want exactly one", api.updates)
	}
	if _, wrote := api.updates[0].props[notion.PropAssignee]; wrote {
		t.Errorf("props = %+v, want no assignee written at all", api.updates[0].props)
	}
}

// A Status column converted to Notion's own status type in the UI is written
// back in that shape, not as the select this app would have created.
func TestReleaseSliceWritesTheStatusShapeItRead(t *testing.T) {
	api := releasableAPI()
	api.pages["slices-ds"][0].Properties[notion.PropStatus] = notion.PropertyValue{
		Type: notion.TypeStatus, Status: &notion.SelectOption{Name: notion.SliceInProgress},
	}
	env, _ := releaseEnv(t, api)

	if err := Run(context.Background(), []string{"release-slice", sliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("release-slice: %v", err)
	}

	status := api.updates[0].props[notion.PropStatus]
	if status.Status == nil || status.Status.Name != notion.SliceTodo || status.Select != nil {
		t.Errorf("status = %+v, want a status value saying %q", status, notion.SliceTodo)
	}
}

// The refusals: a slice this user does not hold is named and left entirely
// alone, before anything at all is written.
func TestReleaseSliceRefusals(t *testing.T) {
	tests := []struct {
		name string
		page notion.Page
		want string
	}{
		{
			name: "held by somebody else",
			page: heldSlice(sliceID, "Render the board", notion.SliceInProgress, "u2", "Someone Else"),
			want: `"Render the board" is held by Someone Else, not by Craig Johnston: leave it to them`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := releasableAPI()
			api.pages["slices-ds"] = []notion.Page{tt.page}
			env, out := releaseEnv(t, api)
			var nudges int
			env.Nudge = func() { nudges++ }

			err := Run(context.Background(), []string{"release-slice", sliceID, "--project", "project-1"}, env)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if len(api.appends) != 0 || len(api.updates) != 0 {
				t.Errorf("wrote %+v and %+v, want nothing written", api.appends, api.updates)
			}
			if nudges != 0 {
				t.Errorf("nudged %d times, want none", nudges)
			}
			if out.String() != "" {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// The command line has to name exactly one slice, and it has to be one.
func TestReleaseSliceMisuse(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no slice", []string{"release-slice", "--project", "project-1"}, "want exactly one slice"},
		{"two slices", []string{"release-slice", sliceID, sliceID, "--project", "project-1"}, "want exactly one slice"},
		{"not a slice", []string{"release-slice", "the one about the board", "--project", "project-1"}, "is not a slice"},
		{"an unknown flag", []string{"release-slice", sliceID, "--why", "--project", "project-1"}, "flag provided but not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := releasableAPI()
			env, _ := releaseEnv(t, api)

			err := Run(context.Background(), tt.args, env)
			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("error = %v, want a usage error", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to mention %q", err, tt.want)
			}
			if len(api.updates) != 0 {
				t.Errorf("wrote %+v, want nothing written", api.updates)
			}
		})
	}
}

// Every call the command makes can fail, and each one says which step it was.
func TestReleaseSliceFailures(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name string
		set  func(*fakeAPI)
		env  func(*Env)
		want string
	}{
		{name: "the config", want: "boom",
			env: func(e *Env) {
				e.Load = func() (config.Config, bool, error) { return config.Config{}, false, boom }
			}},
		{name: "no assignee configured", want: "no assignee in the config",
			env: func(e *Env) {
				cfg := testConfig(t)
				e.Load = func() (config.Config, bool, error) { return cfg, true, nil }
			}},
		// The schema read is the plan file's own initial hydrate — every other
		// read this command makes is local by the time it runs.
		{name: "the schema", want: "boom", set: func(a *fakeAPI) { a.dataSourceErr = boom }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := releasableAPI()
			if tt.set != nil {
				tt.set(api)
			}
			env, _ := releaseEnv(t, api)
			var nudges int
			env.Nudge = func() { nudges++ }
			if tt.env != nil {
				tt.env(&env)
			}

			err := Run(context.Background(), []string{"release-slice", sliceID, "--project", "project-1"}, env)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to mention %q", err, tt.want)
			}
			if nudges != 0 {
				t.Errorf("nudged %d times, want none — nothing landed", nudges)
			}
		})
	}
}

// A push to the workspace that fails does not fail release-slice: the write
// already landed in the local plan file, the command reports success and
// still nudges the board, and the slice is left dirty for the next sync to
// resend.
func TestReleaseSliceLeavesTheSliceDirtyOnAFailedPush(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name string
		set  func(*fakeAPI)
	}{
		{name: "the note", set: func(a *fakeAPI) { a.appendErr = boom }},
		{name: "the write", set: func(a *fakeAPI) { a.updateErr = boom }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := releasableAPI()
			tt.set(api)
			env, out := releaseEnv(t, api)
			var nudges int
			env.Nudge = func() { nudges++ }

			if err := Run(context.Background(), []string{"release-slice", sliceID, "--project", "project-1"}, env); err != nil {
				t.Fatalf("release-slice: %v", err)
			}
			if nudges != 1 {
				t.Errorf("nudged %d times, want exactly one — the local write landed", nudges)
			}
			if out.Len() == 0 {
				t.Errorf("output = %q, want the slice reported released despite the failed push", out.String())
			}

			path, err := store.LocalPath("project-1")
			if err != nil {
				t.Fatal(err)
			}
			local, err := store.OpenLocal(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = local.Close() }()
			dirty, err := local.Dirty(context.Background(), sliceID)
			if err != nil {
				t.Fatal(err)
			}
			if !dirty {
				t.Error("dirty = false, want the slice left dirty for the next sync")
			}
		})
	}
}

// The project's shape is read from the local file once the plan has been
// pulled — no request of its own — and a file that cannot even answer that
// fails the command before the slice is ever looked at.
func TestReleaseSliceReportsAFailedLocalShapeRead(t *testing.T) {
	cfg := testClaimConfig(t)
	seedHydratedSlice(t, "project-1", sliceID, "Render the board", "In progress", func(db *sql.DB) {
		if _, err := db.Exec(`ALTER TABLE project DROP COLUMN has_assignee`); err != nil {
			t.Fatalf("break the plan's has_assignee column: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})

	err := Run(context.Background(), []string{"release-slice", sliceID, "--project", "project-1"}, env)

	if err == nil {
		t.Error("release-slice over a plan that cannot read its own shape: want an error")
	}
}

// The slice itself is read from the same local file, and a file that cannot
// answer that fails the command the same way.
func TestReleaseSliceReportsAFailedLocalSliceRead(t *testing.T) {
	cfg := testClaimConfig(t)
	seedHydratedSlice(t, "project-1", sliceID, "Render the board", "In progress", func(db *sql.DB) {
		if _, err := db.Exec(`DROP TABLE slices`); err != nil {
			t.Fatalf("break the plan's slices table: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})

	err := Run(context.Background(), []string{"release-slice", sliceID, "--project", "project-1"}, env)

	if err == nil {
		t.Error("release-slice over a plan that cannot read the slice: want an error")
	}
}

// The release itself is a local write before anything is asked of the
// workspace, and a plan that cannot make that write fails the command
// outright — there is nothing to push if nothing was actually released.
func TestReleaseSliceReportsAFailedLocalWrite(t *testing.T) {
	cfg := testClaimConfig(t)
	seedHydratedSlice(t, "project-1", sliceID, "Render the board", "In progress", func(db *sql.DB) {
		if _, err := db.Exec(`UPDATE slices SET assignee = ?, assignee_name = ? WHERE id = ?`,
			"u1", "Craig Johnston", sliceID); err != nil {
			t.Fatalf("seed the assignee: %v", err)
		}
		if _, err := db.Exec(`DROP TABLE sync`); err != nil {
			t.Fatalf("break the plan's sync table: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})

	err := Run(context.Background(), []string{"release-slice", sliceID, "--project", "project-1"}, env)

	if err == nil {
		t.Error("release-slice over a plan that cannot make the write: want an error")
	}
}
