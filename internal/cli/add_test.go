package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

// The ID a created page comes back with, in the shape Notion returns.
const (
	addedMilestoneID = "3b838308f65481fc8784e24878ff64f0"
	addedSliceID     = "3b838308f65481948471f9695af198f5"
)

// plannedAPI answers with a plan of three milestones and nothing else, which is
// all either add command reads before writing.
func plannedAPI(createdID string) *fakeAPI {
	return &fakeAPI{
		createdPage: notion.Page{ID: createdID, URL: "https://notion.so/" + createdID},
		pages: map[string][]notion.Page{
			"milestones-ds": {
				milestonePage("m1", "M1: Client", 1, notion.MilestoneDone),
				milestonePage("m2", "M2: Board", 2, notion.MilestoneActive),
				milestonePage("m3", "M3: Agents", 3, notion.MilestoneQueued),
			},
		},
	}
}

// writtenText is the text of a title or rich_text property as it was written:
// a write carries its content in text.content, and only a read has plain_text.
func writtenText(v notion.PropertyValue) string {
	spans := v.Title
	if len(spans) == 0 {
		spans = v.RichText
	}
	var b strings.Builder
	for _, s := range spans {
		if s.Text != nil {
			b.WriteString(s.Text.Content)
		}
	}
	return b.String()
}

// noWritesBut fails the test if anything was written other than the creations
// allowed for: neither command may touch a page that already exists.
func noWritesBut(t *testing.T, api *fakeAPI, creations int) {
	t.Helper()
	if len(api.creates) != creations {
		t.Errorf("creates = %d (%+v), want %d", len(api.creates), api.creates, creations)
	}
	if len(api.updates) != 0 || len(api.appends) != 0 {
		t.Errorf("writes to existing pages = %+v %+v, want none", api.updates, api.appends)
	}
}

func TestMilestoneAddFilesAQueuedMilestoneAtTheEndOfThePlan(t *testing.T) {
	api := plannedAPI(addedMilestoneID)
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"milestone-add", "M4: Polish"}, env); err != nil {
		t.Fatalf("milestone-add: %v", err)
	}

	want := `# M4: Polish

Added to nat as milestone 4, Queued.

- Notion page: 3b838308f65481fc8784e24878ff64f0
- Notion URL: https://notion.so/3b838308f65481fc8784e24878ff64f0
`
	if out.String() != want {
		t.Errorf("output =\n%s\nwant:\n%s", out.String(), want)
	}
	noWritesBut(t, api, 1)
	c := api.creates[0]
	if c.parent != notion.DataSourceParent("milestones-ds") {
		t.Errorf("parent = %+v, want the milestones data source", c.parent)
	}
	if got := writtenText(c.props[notion.PropName]); got != "M4: Polish" {
		t.Errorf("name = %q, want %q", got, "M4: Polish")
	}
	if got, ok := c.props[notion.PropOrder].NumberValue(); !ok || got != 4 {
		t.Errorf("order = %v (ok=%t), want 4", got, ok)
	}
	if got := c.props[notion.PropStatus].SelectName(); got != notion.MilestoneQueued {
		t.Errorf("status = %q, want %q", got, notion.MilestoneQueued)
	}
	if len(c.props) != 3 {
		t.Errorf("properties = %+v, want exactly name, order and status", c.props)
	}
	if len(c.children) != 0 {
		t.Errorf("children = %+v, want none: a milestone has no body", c.children)
	}
}

// The order counts from the highest there is rather than from how many
// milestones there are, so a plan that has had one removed does not reuse a
// number — and an empty plan starts at 1.
func TestMilestoneAddNumbersFromTheHighestOrder(t *testing.T) {
	tests := []struct {
		name       string
		milestones []notion.Page
		want       float64
	}{
		{name: "empty plan", want: 1},
		{
			name:       "a gap in the middle",
			milestones: []notion.Page{milestonePage("m1", "M1", 1, notion.MilestoneDone), milestonePage("m9", "M9", 9, notion.MilestoneActive)},
			want:       10,
		},
		{
			name:       "out of order and fractional",
			milestones: []notion.Page{milestonePage("m2", "M2", 2.5, notion.MilestoneDone), milestonePage("m1", "M1", 1, notion.MilestoneDone)},
			want:       3.5,
		},
		{
			name:       "no order set at all",
			milestones: []notion.Page{{ID: "m1", Properties: map[string]notion.PropertyValue{notion.PropName: title("M1")}}},
			want:       1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := plannedAPI(addedMilestoneID)
			api.pages["milestones-ds"] = tt.milestones
			env, _ := testEnv(testConfig(), api)

			if err := Run(context.Background(), []string{"milestone-add", "Next"}, env); err != nil {
				t.Fatalf("milestone-add: %v", err)
			}

			if got, _ := api.creates[0].props[notion.PropOrder].NumberValue(); got != tt.want {
				t.Errorf("order = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMilestoneAddPrintsJSON(t *testing.T) {
	for _, args := range [][]string{{"milestone-add", "M4: Polish", "--json"}, {"milestone-add", "--json", "M4: Polish"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env, out := testEnv(testConfig(), plannedAPI(addedMilestoneID))

			if err := Run(context.Background(), args, env); err != nil {
				t.Fatalf("%v: %v", args, err)
			}

			var got milestoneAddedJSON
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("output is not JSON: %v\n%s", err, out.String())
			}
			want := milestoneAddedJSON{Milestone: milestoneJSON{
				ID: addedMilestoneID, Name: "M4: Polish", Order: 4,
				Status: notion.MilestoneQueued, URL: "https://notion.so/" + addedMilestoneID,
			}}
			if got != want {
				t.Errorf("json = %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestMilestoneAddRejectsAMisusedCommandLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no name", args: []string{"milestone-add"}, want: "given 0"},
		{name: "two names", args: []string{"milestone-add", "M4", "M5"}, want: "given 2"},
		{name: "blank name", args: []string{"milestone-add", "   "}, want: "name is empty"},
		{name: "unknown flag", args: []string{"milestone-add", "M4", "--nope"}, want: "not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := plannedAPI(addedMilestoneID)
			env, out := testEnv(testConfig(), api)

			err := Run(context.Background(), tt.args, env)

			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("err = %v (%T), want a *UsageError", err, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "milestone-add:") {
				t.Errorf("err = %q, want it to name the command", err)
			}
			noWritesBut(t, api, 0)
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

func TestMilestoneAddReportsAFailedCall(t *testing.T) {
	boom := errors.New("notion: 500")
	tests := []struct {
		name     string
		fail     func(*fakeAPI)
		want     string
		creation bool
	}{
		{
			name: "the milestones",
			fail: func(api *fakeAPI) { api.queryErr = map[string]error{"milestones-ds": boom} },
			want: "load milestones",
		},
		{
			name:     "the creation",
			fail:     func(api *fakeAPI) { api.createErr = boom },
			want:     "create the milestone",
			creation: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := plannedAPI(addedMilestoneID)
			tt.fail(api)
			env, out := testEnv(testConfig(), api)

			err := Run(context.Background(), []string{"milestone-add", "M4"}, env)

			if !errors.Is(err, boom) {
				t.Fatalf("err = %v, want %v", err, boom)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if tt.creation != (len(api.creates) == 1) {
				t.Errorf("creates = %+v, want attempted = %t", api.creates, tt.creation)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

func TestMilestoneAddNeedsAConfiguredProject(t *testing.T) {
	api := plannedAPI(addedMilestoneID)
	env, _ := testEnv(testConfig(), api)
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil }

	err := Run(context.Background(), []string{"milestone-add", "M4"}, env)

	if err == nil || !strings.Contains(err.Error(), "run `nat` once to set it up") {
		t.Fatalf("err = %v, want the setup reported", err)
	}
	noWritesBut(t, api, 0)
}

func TestMilestoneAddReportsAFailedWrite(t *testing.T) {
	for _, args := range [][]string{{"milestone-add", "M4"}, {"milestone-add", "M4", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env, _ := testEnv(testConfig(), plannedAPI(addedMilestoneID))
			env.Out = failingWriter{}

			err := Run(context.Background(), args, env)

			if !errors.Is(err, errWrite) {
				t.Errorf("err = %v, want %v", err, errWrite)
			}
		})
	}
}

func TestSliceAddFilesATodoSliceUnderTheNamedMilestone(t *testing.T) {
	api := plannedAPI(addedSliceID)
	env, out := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-add", "Render the board",
		"--milestone", "M2: Board", "--description", "Draw the groups.\n\nThen stop."}, env)

	if err != nil {
		t.Fatalf("slice-add: %v", err)
	}
	want := `# Render the board

Added to M2: Board, Todo and unclaimed.

- Notion page: 3b838308f65481948471f9695af198f5
- Notion URL: https://notion.so/3b838308f65481948471f9695af198f5
- Working directory: /tmp/nat
`
	if out.String() != want {
		t.Errorf("output =\n%s\nwant:\n%s", out.String(), want)
	}
	noWritesBut(t, api, 1)
	c := api.creates[0]
	if c.parent != notion.DataSourceParent("slices-ds") {
		t.Errorf("parent = %+v, want the slices data source", c.parent)
	}
	if got := writtenText(c.props[notion.PropName]); got != "Render the board" {
		t.Errorf("name = %q, want %q", got, "Render the board")
	}
	if got := c.props[notion.PropStatus].SelectName(); got != notion.SliceTodo {
		t.Errorf("status = %q, want %q", got, notion.SliceTodo)
	}
	if got := c.props[notion.PropMilestone].RelationIDs(); len(got) != 1 || got[0] != "m2" {
		t.Errorf("milestone = %v, want [m2]", got)
	}
	if got := writtenText(c.props[notion.PropRepo]); got != "" {
		t.Errorf("repo = %q, want empty", got)
	}
	if _, assigned := c.props[notion.PropAssignee]; assigned {
		t.Errorf("properties = %+v, want no assignee: a new slice is unclaimed", c.props)
	}
	if len(c.children) != 2 {
		t.Fatalf("children = %+v, want one paragraph per chunk", c.children)
	}
	if got := paragraphText(t, c.children[0]); got != "Draw the groups." {
		t.Errorf("first paragraph = %q, want %q", got, "Draw the groups.")
	}
	if got := paragraphText(t, c.children[1]); got != "Then stop." {
		t.Errorf("second paragraph = %q, want %q", got, "Then stop.")
	}
}

// paragraphText reads the text back out of a block this package built.
func paragraphText(t *testing.T, block map[string]any) string {
	t.Helper()
	if got := block["type"]; got != "paragraph" {
		t.Fatalf("block type = %v, want paragraph", got)
	}
	spans := block["paragraph"].(map[string]any)["rich_text"].([]map[string]any)
	return spans[0]["text"].(map[string]any)["content"].(string)
}

// The milestone may be named however the caller has it: its name in any case,
// its page ID, or the URL a Notion page was copied from.
func TestSliceAddResolvesTheMilestoneHoweverItIsNamed(t *testing.T) {
	tests := []struct {
		name string
		ref  string
	}{
		{name: "by name", ref: "M2: Board"},
		{name: "by name in another case", ref: "m2: bOARD"},
		{name: "by name with stray spaces", ref: "  M2: Board  "},
		{name: "by ID", ref: "3b738308-f654-8138-b972-f81cb0d5fd6f"},
		{name: "by ID without dashes", ref: "3b738308f6548138b972f81cb0d5fd6f"},
		{name: "by URL", ref: "https://www.notion.so/M2-Board-3b738308f6548138b972f81cb0d5fd6f?pvs=4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := plannedAPI(addedSliceID)
			api.pages["milestones-ds"][1].ID = "3b738308-f654-8138-b972-f81cb0d5fd6f"
			env, _ := testEnv(testConfig(), api)

			err := Run(context.Background(), []string{"slice-add", "Render the board", "--milestone", tt.ref}, env)

			if err != nil {
				t.Fatalf("slice-add: %v", err)
			}
			got := api.creates[0].props[notion.PropMilestone].RelationIDs()
			if len(got) != 1 || got[0] != "3b738308-f654-8138-b972-f81cb0d5fd6f" {
				t.Errorf("milestone = %v, want the M2 page", got)
			}
		})
	}
}

// A milestone that cannot be pinned down is a refusal, and nothing is written:
// a slice filed under the wrong phase of the plan is worse than one not filed.
func TestSliceAddRefusesAMilestoneItCannotResolve(t *testing.T) {
	tests := []struct {
		name       string
		ref        string
		milestones []notion.Page
		want       []string
	}{
		{
			name: "unknown name",
			ref:  "M4: Polish",
			want: []string{`no milestone named "M4: Polish"`, "M1: Client, M2: Board, M3: Agents"},
		},
		{
			name: "a name that is only a prefix",
			ref:  "M2",
			want: []string{`no milestone named "M2"`},
		},
		{
			name:       "two milestones sharing a name",
			ref:        "M2: Board",
			milestones: []notion.Page{milestonePage("m2", "M2: Board", 2, notion.MilestoneActive), milestonePage("m2b", "M2: Board", 3, notion.MilestoneQueued)},
			want:       []string{`2 milestones are named "M2: Board"`, "m2, m2b"},
		},
		{
			name: "an ID from somewhere else",
			ref:  "3b838308f65481fc8784e24878ff64f0",
			want: []string{"no milestone 3b838308f65481fc8784e24878ff64f0 in this project"},
		},
		{
			name:       "a project with no milestones yet",
			ref:        "M1",
			milestones: []notion.Page{},
			want:       []string{"no milestones yet", "nat milestone-add"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := plannedAPI(addedSliceID)
			if tt.milestones != nil {
				api.pages["milestones-ds"] = tt.milestones
			}
			env, out := testEnv(testConfig(), api)

			err := Run(context.Background(), []string{"slice-add", "Render the board", "--milestone", tt.ref}, env)

			if err == nil {
				t.Fatal("err = nil, want a refusal")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %q, want it to mention %q", err, want)
				}
			}
			var usage *UsageError
			if errors.As(err, &usage) {
				t.Errorf("err = %v, want a plain error: the command line was fine", err)
			}
			noWritesBut(t, api, 0)
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// A brief long enough to be awkward as an argument is piped in instead, which
// --description asks for by name so that a slice-add typed without one at a
// terminal returns rather than waiting on stdin.
func TestSliceAddReadsTheDescriptionFromStdin(t *testing.T) {
	api := plannedAPI(addedSliceID)
	env, _ := testEnv(testConfig(), api)
	env.In = strings.NewReader("  Draw the groups.  \n")

	err := Run(context.Background(), []string{"slice-add", "Render the board",
		"--milestone", "M2: Board", "--description", "-"}, env)

	if err != nil {
		t.Fatalf("slice-add: %v", err)
	}
	if len(api.creates[0].children) != 1 {
		t.Fatalf("children = %+v, want the piped brief", api.creates[0].children)
	}
	if got := paragraphText(t, api.creates[0].children[0]); got != "Draw the groups." {
		t.Errorf("paragraph = %q, want the piped brief", got)
	}
}

// A slice whose title says everything needs no brief, and a page body is not
// invented for it.
func TestSliceAddFilesASliceWithNoDescription(t *testing.T) {
	for _, description := range []string{"", "   \n\n  "} {
		t.Run(strings.TrimSpace(description), func(t *testing.T) {
			api := plannedAPI(addedSliceID)
			env, _ := testEnv(testConfig(), api)

			err := Run(context.Background(), []string{"slice-add", "Render the board",
				"--milestone", "M2: Board", "--description", description}, env)

			if err != nil {
				t.Fatalf("slice-add: %v", err)
			}
			if len(api.creates[0].children) != 0 {
				t.Errorf("children = %+v, want none", api.creates[0].children)
			}
		})
	}
}

// A stdin that cannot be read fails before anything reaches Notion.
func TestSliceAddReportsAnUnreadableDescription(t *testing.T) {
	api := plannedAPI(addedSliceID)
	env, _ := testEnv(testConfig(), api)
	env.In = failingReader{}

	err := Run(context.Background(), []string{"slice-add", "Render the board",
		"--milestone", "M2: Board", "--description", "-"}, env)

	if !errors.Is(err, errRead) {
		t.Fatalf("err = %v, want %v", err, errRead)
	}
	noWritesBut(t, api, 0)
}

// Asking for a piped brief where nothing can be piped in is a misuse, not a
// silently empty one.
func TestSliceAddRejectsAPipedDescriptionWithNoInput(t *testing.T) {
	api := plannedAPI(addedSliceID)
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-add", "Render the board",
		"--milestone", "M2: Board", "--description", "-"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
	noWritesBut(t, api, 0)
}

// A slice may override the project's working directory, and the override is
// what the slice reads as afterwards.
func TestSliceAddHonoursARepoOverride(t *testing.T) {
	api := plannedAPI(addedSliceID)
	env, out := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-add", "Render the board",
		"--milestone", "M2: Board", "--repo", "  /tmp/other  "}, env)

	if err != nil {
		t.Fatalf("slice-add: %v", err)
	}
	if got := writtenText(api.creates[0].props[notion.PropRepo]); got != "/tmp/other" {
		t.Errorf("repo = %q, want %q", got, "/tmp/other")
	}
	if !strings.Contains(out.String(), "- Working directory: /tmp/other\n") {
		t.Errorf("output =\n%s\nwant the slice's own repo", out.String())
	}
}

// A project with no working directory configured has nothing to say about where
// the work happens, and says nothing rather than printing an empty line.
func TestSliceAddOmitsAnUnknownWorkingDirectory(t *testing.T) {
	cfg := testConfig()
	project := cfg.Projects["project-1"]
	project.WorkingDir = ""
	cfg.Projects["project-1"] = project
	env, out := testEnv(cfg, plannedAPI(addedSliceID))

	err := Run(context.Background(), []string{"slice-add", "Render the board", "--milestone", "M2: Board"}, env)

	if err != nil {
		t.Fatalf("slice-add: %v", err)
	}
	if strings.Contains(out.String(), "Working directory") {
		t.Errorf("output =\n%s\nwant no working directory line", out.String())
	}
}

func TestSliceAddPrintsJSON(t *testing.T) {
	for _, args := range [][]string{
		{"slice-add", "Render the board", "--milestone", "M2: Board", "--json"},
		{"slice-add", "--milestone", "M2: Board", "--json", "Render the board"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env, out := testEnv(testConfig(), plannedAPI(addedSliceID))

			if err := Run(context.Background(), args, env); err != nil {
				t.Fatalf("%v: %v", args, err)
			}

			var got sliceAddedJSON
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("output is not JSON: %v\n%s", err, out.String())
			}
			want := sliceAddedJSON{Slice: addedSliceJSON{
				ID: addedSliceID, Name: "Render the board", Status: notion.SliceTodo,
				MilestoneID: "m2", MilestoneName: "M2: Board", Repo: "/tmp/nat",
				URL: "https://notion.so/" + addedSliceID,
			}}
			if got != want {
				t.Errorf("json = %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestSliceAddRejectsAMisusedCommandLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no title", args: []string{"slice-add", "--milestone", "M2: Board"}, want: "given 0"},
		{name: "two titles", args: []string{"slice-add", "one", "two", "--milestone", "M2: Board"}, want: "given 2"},
		{name: "blank title", args: []string{"slice-add", "  ", "--milestone", "M2: Board"}, want: "title is empty"},
		{name: "no milestone", args: []string{"slice-add", "Render the board"}, want: "pass --milestone"},
		{name: "blank milestone", args: []string{"slice-add", "Render the board", "--milestone", " "}, want: "pass --milestone"},
		{name: "unknown flag", args: []string{"slice-add", "Render the board", "--nope"}, want: "not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := plannedAPI(addedSliceID)
			env, out := testEnv(testConfig(), api)

			err := Run(context.Background(), tt.args, env)

			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("err = %v (%T), want a *UsageError", err, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "slice-add:") {
				t.Errorf("err = %q, want it to name the command", err)
			}
			noWritesBut(t, api, 0)
			if len(api.queries) != 0 {
				t.Errorf("queries = %+v, want none: the command line was rejected", api.queries)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

func TestSliceAddReportsAFailedCall(t *testing.T) {
	boom := errors.New("notion: 500")
	tests := []struct {
		name     string
		fail     func(*fakeAPI)
		want     string
		creation bool
	}{
		{
			name: "the milestones",
			fail: func(api *fakeAPI) { api.queryErr = map[string]error{"milestones-ds": boom} },
			want: "load milestones",
		},
		{
			name:     "the creation",
			fail:     func(api *fakeAPI) { api.createErr = boom },
			want:     "create the slice",
			creation: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := plannedAPI(addedSliceID)
			tt.fail(api)
			env, out := testEnv(testConfig(), api)

			err := Run(context.Background(), []string{"slice-add", "Render the board", "--milestone", "M2: Board"}, env)

			if !errors.Is(err, boom) {
				t.Fatalf("err = %v, want %v", err, boom)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if tt.creation != (len(api.creates) == 1) {
				t.Errorf("creates = %+v, want attempted = %t", api.creates, tt.creation)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

func TestSliceAddNeedsAConfiguredProject(t *testing.T) {
	api := plannedAPI(addedSliceID)
	env, _ := testEnv(testConfig(), api)
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil }

	err := Run(context.Background(), []string{"slice-add", "Render the board", "--milestone", "M2: Board"}, env)

	if err == nil || !strings.Contains(err.Error(), "run `nat` once to set it up") {
		t.Fatalf("err = %v, want the setup reported", err)
	}
	noWritesBut(t, api, 0)
}

func TestSliceAddReportsAFailedWrite(t *testing.T) {
	for _, args := range [][]string{
		{"slice-add", "Render the board", "--milestone", "M2: Board"},
		{"slice-add", "Render the board", "--milestone", "M2: Board", "--json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env, _ := testEnv(testConfig(), plannedAPI(addedSliceID))
			env.Out = failingWriter{}

			err := Run(context.Background(), args, env)

			if !errors.Is(err, errWrite) {
				t.Errorf("err = %v, want %v", err, errWrite)
			}
		})
	}
}
