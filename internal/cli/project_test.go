package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

// The second project of the config and the two slices of it a command can be
// pointed at. The slice IDs are real Notion page IDs because every command that
// takes a slice insists on one.
const (
	otherProjectID = "project-2"
	otherTodoID    = "2a738308f654812fb0d1e1a3c0a30d51"
	otherWorkingID = "2a738308f654812fb0d1e1a3c0a30d52"
)

// everyCommandAPI is a workspace holding both projects whole — each with its
// own page body and its own Slices table — so a command pointed at one of them
// has the other there to reach for by mistake.
func everyCommandAPI(t *testing.T) *fakeAPI {
	t.Helper()
	other := selectMilestoneSlicesDS("M1: Other")
	other.ID = "other-ds"
	other.Properties[notion.PropAssignee] = notion.PropertySchema{Type: notion.TypePeople}
	other.Properties[notion.PropDependsOn] = dependsOnColumn("other-ds")
	return &fakeAPI{
		blocksByID: map[string][]notion.Block{
			"project-1":    conventionBlocks(t),
			otherProjectID: wishlistPage(t),
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": assigneeSlicesDS("M1: Client"),
			"other-ds":  other,
		},
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage("s1", "Draw the board", notion.SliceTodo, "M1: Client", "", "")},
			"other-ds": {
				slicePage(otherTodoID, "Wire the client", notion.SliceTodo, "M1: Other", "", ""),
				slicePage(otherWorkingID, "Ship it", notion.SliceInProgress, "M1: Other", "Craig Johnston", ""),
			},
		},
		createdPage: notion.Page{ID: "new-slice", URL: "https://notion.so/new-slice"},
	}
}

// everyCommandConfig is twoProjectConfig with an assignee, since half the
// commands swept here claim or close out a slice.
func everyCommandConfig() config.Config {
	cfg := twoProjectConfig(otherProjectID)
	cfg.AssigneeUserID = "u1"
	cfg.AssigneeUserName = "Craig Johnston"
	return cfg
}

// Every command that acts on a project acts on the one --project names, and on
// nothing of the other: the two projects here share no page and no data source,
// so a command that reached for the wrong one would be caught by the ID it
// reached for.
func TestEveryProjectScopedCommandTakesAProjectFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		in   string
		// touched is an ID of the named project the command must have reached
		// for; empty where the command uses the project only to resolve itself.
		touched string
	}{
		{name: "info", args: []string{"info"}, touched: otherProjectID},
		{name: "next-slice", args: []string{"next-slice"}, touched: "other-ds"},
		{name: "start-slice", args: []string{"start-slice", otherTodoID}, touched: "other-ds"},
		{name: "milestone-add", args: []string{"milestone-add", "M2: Other"}, touched: "other-ds"},
		{
			name:    "slice-add",
			args:    []string{"slice-add", "Wire it up", "--milestone", "M1: Other"},
			touched: "other-ds",
		},
		{name: "slice-depends", args: []string{"slice-depends", otherTodoID, "--on", otherWorkingID}},
		{name: "wishlist", args: []string{"wishlist"}, touched: otherProjectID},
		{name: "wishlist-clear", args: []string{"wishlist-clear", "w1"}, touched: otherProjectID},
		{
			name:    "complete-slice",
			args:    []string{"complete-slice", otherWorkingID, "--branch", "slice/ship-it", "--summary", "shipped"},
			touched: "other-ds",
		},
		{name: "release-slice", args: []string{"release-slice", otherWorkingID}, touched: "other-ds"},
		{
			name:    "plan-apply",
			args:    []string{"plan-apply"},
			in:      `{"milestones": [{"name": "M2: Other"}]}`,
			touched: "other-ds",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := everyCommandAPI(t)
			env, _ := testEnv(everyCommandConfig(), api)
			env.In = strings.NewReader(tt.in)

			args := append(append([]string{}, tt.args...), "--project", otherProjectID)
			if err := Run(context.Background(), args, env); err != nil {
				t.Fatalf("%v: %v", args, err)
			}

			touched := touchedIDs(api)
			for _, id := range touched {
				if id == "project-1" || id == "slices-ds" || id == "s1" {
					t.Errorf("touched %q of the other project; the run reached for %v", id, touched)
				}
			}
			if tt.touched != "" && !touches(touched, tt.touched) {
				t.Errorf("touched %v, want it to include %q", touched, tt.touched)
			}
		})
	}
}

// touchedIDs is every page and data source a run reached for, in no particular
// order: whichever of them belongs to a project is the project the command ran
// against.
func touchedIDs(api *fakeAPI) []string {
	ids := append([]string{}, api.blockReads...)
	ids = append(ids, api.dataSourceReads...)
	ids = append(ids, api.ordered...)
	ids = append(ids, api.gets...)
	ids = append(ids, api.deletes...)
	for _, q := range api.queries {
		ids = append(ids, q.id)
	}
	for _, u := range api.updates {
		ids = append(ids, u.id)
	}
	for _, u := range api.schemaUpdates {
		ids = append(ids, u.id)
	}
	for _, a := range api.appends {
		ids = append(ids, a.id)
	}
	for _, c := range api.creates {
		ids = append(ids, c.parent.DataSourceID)
	}
	return ids
}

func touches(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// A project the config does not hold is refused before anything is read, and
// the refusal lists the projects it does hold — the same refusal --project
// already gave on plan-apply, now given by every command that takes the flag.
func TestProjectFlagRefusesAProjectTheConfigDoesNotHold(t *testing.T) {
	api := everyCommandAPI(t)
	env, out := testEnv(everyCommandConfig(), api)

	err := Run(context.Background(), []string{"info", "--project", "project-9"}, env)

	want := "no project project-9 in the config file: it tracks project-1 (nat), project-2 (other)"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
	if ids := touchedIDs(api); len(ids) != 0 {
		t.Errorf("touched %v, want nothing read", ids)
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want nothing", out.String())
	}
}

// Left off, the flag has nothing to fall back to: the project the board is on
// is the board's own, and the user switches it while a session runs, so a
// command that took it could write into a plan the session never read. Every
// project-scoped command is refused instead, before anything is read, and the
// refusal lists what this machine tracks — so the ID to pass is one failed call
// away.
func TestWithoutTheFlagEveryProjectScopedCommandIsRefused(t *testing.T) {
	for _, args := range [][]string{
		{"info"},
		{"next-slice"},
		{"start-slice", otherTodoID},
		{"milestone-add", "M2: Other"},
		{"slice-add", "Wire it up", "--milestone", "M1: Other"},
		{"slice-depends", otherTodoID, "--on", otherWorkingID},
		{"wishlist"},
		{"wishlist-clear", "w1"},
		{"complete-slice", otherWorkingID, "--branch", "slice/ship-it"},
		{"release-slice", otherWorkingID},
		{"plan-apply"},
	} {
		t.Run(args[0], func(t *testing.T) {
			api := everyCommandAPI(t)
			env, out := testEnv(everyCommandConfig(), api)
			env.In = strings.NewReader(`{"milestones": [{"name": "M2: Other"}]}`)

			err := Run(context.Background(), args, env)

			want := "no project given: pass --project with a project's page ID: " +
				"it tracks project-1 (nat), project-2 (other)"
			if err == nil || err.Error() != want {
				t.Errorf("err = %v, want %q", err, want)
			}
			if ids := touchedIDs(api); len(ids) != 0 {
				t.Errorf("touched %v, want nothing read", ids)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// A config that will not load lists no projects rather than replacing the
// missing flag with a complaint about the file: the flag is missing either way,
// and a second error would only bury the first.
func TestWithoutTheFlagAnUnreadableConfigStillNamesTheFlag(t *testing.T) {
	env, _ := testEnv(everyCommandConfig(), everyCommandAPI(t))
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, errors.New("disk gone") }

	err := Run(context.Background(), []string{"info"}, env)

	want := "no project given: pass --project with a project's page ID: it tracks no projects yet"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

// The project a command was pointed at is the project its output names, since
// an agent reading a brief has no other way of telling which plan it came from.
func TestAProjectScopedCommandNamesTheProjectItRanAgainst(t *testing.T) {
	env, out := testEnv(everyCommandConfig(), everyCommandAPI(t))

	args := []string{"start-slice", otherTodoID, "--project", otherProjectID}
	if err := Run(context.Background(), args, env); err != nil {
		t.Fatalf("start-slice: %v", err)
	}

	if !strings.Contains(out.String(), "- Project: other") {
		t.Errorf("output =\n%s\nwant it to name the other project", out.String())
	}
}

// The usage text says the flag once, for all of the commands that require it,
// rather than leaving each line to explain it.
func TestUsageDescribesTheProjectFlag(t *testing.T) {
	if !strings.Contains(Usage, "Every command below that acts on a project requires --project") {
		t.Errorf("usage does not describe --project:\n%s", Usage)
	}
}
