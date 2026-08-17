package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

// createdSeq is what successive creations answer with: pages whose IDs say
// which write made them, so an assertion on the output can tell them apart.
func createdSeq(n int) []notion.Page {
	pages := make([]notion.Page, n)
	for i := range pages {
		id := fmt.Sprintf("new-%d", i+1)
		pages[i] = notion.Page{ID: id, URL: "https://notion.so/" + id}
	}
	return pages
}

// planAPI answers with the same three-milestone plan the add commands read,
// and hands out distinct pages for however many creations a test expects.
func planAPI(creations int) *fakeAPI {
	api := plannedAPI("")
	api.createdPages = createdSeq(creations)
	return api
}

// runPlan applies a plan piped in on stdin.
func runPlan(t *testing.T, api *fakeAPI, doc string, args ...string) (string, error) {
	t.Helper()
	env, out := testEnv(testConfig(), api)
	env.In = strings.NewReader(doc)
	err := Run(context.Background(), append([]string{"plan-apply"}, args...), env)
	return out.String(), err
}

// The plan every happy-path test applies: one new milestone with two slices,
// and a third slice filed under a milestone the project already has.
const samplePlan = `{
  "milestones": [{"name": "M4: Polish"}],
  "slices": [
    {"title": "Frame the board", "milestone": "M4: Polish", "description": "Draw a border.\n\nThen a status bar."},
    {"title": "Colour the chips", "milestone": "M4: Polish"},
    {"title": "Poll in the background", "milestone": "M2: Board", "repo": "/tmp/other"}
  ]
}`

func TestPlanApplyCreatesTheMilestonesThenTheSlices(t *testing.T) {
	api := planAPI(3)

	out, err := runPlan(t, api, samplePlan)
	if err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	want := `# Plan applied

Added 1 milestone and 3 slices to nat.

## M4: Polish

New milestone 4, Queued — ` + optionNote + `

- Frame the board — https://notion.so/new-1
- Colour the chips — https://notion.so/new-2

## M2: Board

- Poll in the background — https://notion.so/new-3
`
	if out != want {
		t.Errorf("output =\n%s\nwant:\n%s", out, want)
	}

	if len(api.creates) != 3 {
		t.Fatalf("creates = %+v, want one page per slice", api.creates)
	}
	if got := writtenMilestoneOptions(t, api); !reflect.DeepEqual(got,
		[]string{"M1: Client", "M2: Board", "M3: Agents", "M4: Polish"}) {
		t.Errorf("options = %v, want the new milestone appended", got)
	}

	first := api.creates[0]
	if first.parent != notion.DataSourceParent("slices-ds") {
		t.Errorf("slice parent = %+v, want the slices data source", first.parent)
	}
	if got := writtenText(first.props[notion.PropName]); got != "Frame the board" {
		t.Errorf("slice name = %q, want %q", got, "Frame the board")
	}
	if got := first.props[notion.PropStatus].SelectName(); got != notion.SliceTodo {
		t.Errorf("slice status = %q, want %q", got, notion.SliceTodo)
	}
	// The option is the milestone this run just added, not anything the document
	// happened to name.
	if got := first.props[notion.PropMilestone]; !reflect.DeepEqual(got, notion.NewSelect("M4: Polish")) {
		t.Errorf("milestone = %+v, want the option this run added", got)
	}
	if _, ok := first.props[notion.PropAssignee]; ok {
		t.Errorf("properties = %+v, want no assignee: a planned slice is unclaimed", first.props)
	}
	if len(first.children) != 2 {
		t.Errorf("children = %+v, want one paragraph per chunk of the description", first.children)
	}

	last := api.creates[2]
	if got := last.props[notion.PropMilestone]; !reflect.DeepEqual(got, notion.NewSelect("M2: Board")) {
		t.Errorf("milestone = %+v, want the milestone the project already had", got)
	}
	if got := writtenText(last.props[notion.PropRepo]); got != "/tmp/other" {
		t.Errorf("repo = %q, want %q", got, "/tmp/other")
	}
}

// A slice may name an existing milestone in whatever case the drafting had it.
func TestPlanApplyResolvesAnExistingMilestoneByName(t *testing.T) {
	for _, ref := range []string{"M2: Board", "m2: board"} {
		t.Run(ref, func(t *testing.T) {
			api := planAPI(1)
			doc := fmt.Sprintf(`{"slices": [{"title": "Do it", "milestone": %q}]}`, ref)

			if _, err := runPlan(t, api, doc); err != nil {
				t.Fatalf("plan-apply: %v", err)
			}

			if got := api.creates[0].props[notion.PropMilestone]; !reflect.DeepEqual(got, notion.NewSelect("M2: Board")) {
				t.Errorf("milestone = %+v, want the option naming M2", got)
			}
		})
	}
}

// A milestone created with nothing under it is reported as such rather than as
// a heading with nothing after it.
func TestPlanApplyReportsAMilestoneWithNoSlices(t *testing.T) {
	api := planAPI(1)

	out, err := runPlan(t, api, `{"milestones": [{"name": "M4: Polish"}]}`)
	if err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	want := `# Plan applied

Added 1 milestone and 0 slices to nat.

## M4: Polish

New milestone 4, Queued — ` + optionNote + `

_no slices_
`
	if out != want {
		t.Errorf("output =\n%s\nwant:\n%s", out, want)
	}
}

func TestPlanApplyPrintsJSON(t *testing.T) {
	api := planAPI(3)

	out, err := runPlan(t, api, samplePlan, "--json")
	if err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	var got planAppliedJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	want := planAppliedJSON{
		Milestones: []milestoneJSON{{
			ID: "M4: Polish", Name: "M4: Polish", Order: 3, Status: notion.MilestoneQueued,
		}},
		Slices: []addedSliceJSON{
			{
				ID: "new-1", Name: "Frame the board", Status: notion.SliceTodo,
				MilestoneID: "M4: Polish", MilestoneName: "M4: Polish",
				Repo: "/tmp/nat", URL: "https://notion.so/new-1",
			},
			{
				ID: "new-2", Name: "Colour the chips", Status: notion.SliceTodo,
				MilestoneID: "M4: Polish", MilestoneName: "M4: Polish",
				Repo: "/tmp/nat", URL: "https://notion.so/new-2",
			},
			{
				ID: "new-3", Name: "Poll in the background", Status: notion.SliceTodo,
				MilestoneID: "M2: Board", MilestoneName: "M2: Board",
				Repo: "/tmp/other", URL: "https://notion.so/new-3",
			},
		},
		Dependencies: []addedDependencyJSON{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("json =\n%+v\nwant:\n%+v", got, want)
	}
}

// An empty run still prints lists rather than nulls, so anything parsing the
// output can iterate them without checking first.
func TestPlanApplyPrintsEmptyJSONLists(t *testing.T) {
	api := planAPI(1)

	out, err := runPlan(t, api, `{"milestones": [{"name": "M4"}]}`, "--json")
	if err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	for _, want := range []string{`"slices": []`, `"dependencies": []`} {
		if !strings.Contains(out, want) {
			t.Errorf("json =\n%s\nwant %s", out, want)
		}
	}
}

func TestPlanApplyReadsAPlanFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, []byte(`{"milestones": [{"name": "M4: Polish"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	api := planAPI(1)
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"plan-apply", path}, env); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	if !strings.Contains(out.String(), "M4: Polish") {
		t.Errorf("output =\n%s\nwant the milestone from the file", out.String())
	}
	if len(api.schemaUpdates) != 1 {
		t.Errorf("schema writes = %+v, want the milestone appended", api.schemaUpdates)
	}
}

// `-` is the same thing as no file at all: the plan is piped in.
func TestPlanApplyReadsStdinForADashedFile(t *testing.T) {
	api := planAPI(1)

	if _, err := runPlan(t, api, `{"milestones": [{"name": "M4"}]}`, "-"); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	if len(api.schemaUpdates) != 1 {
		t.Errorf("schema writes = %+v, want the milestone appended", api.schemaUpdates)
	}
}

// A plan that does not make sense is refused whole. Nothing may reach Notion —
// half a plan applied is worse than none of it.
func TestPlanApplyRejectsAnInvalidPlan(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want string
	}{
		{name: "not JSON at all", doc: `not json`, want: "not valid JSON"},
		{name: "truncated", doc: `{"milestones": [`, want: "not valid JSON"},
		{name: "two documents", doc: `{"milestones": []} {"slices": []}`, want: "more than one document"},
		{
			name: "a key the format does not have",
			doc:  `{"slices": [{"title": "Do it", "milestone": "M2: Board", "descriptoin": "oops"}]}`,
			want: "unknown field",
		},
		{name: "nothing to create", doc: `{}`, want: "creates nothing and records nothing"},
		{name: "a milestone with no name", doc: `{"milestones": [{"name": "  "}]}`, want: "milestone 1 has no name"},
		{
			name: "two milestones of one name",
			doc:  `{"milestones": [{"name": "M4"}, {"name": "m4"}]}`,
			want: `milestones 1 and 2 are both named "m4"`,
		},
		{
			name: "a milestone the project already has",
			doc:  `{"milestones": [{"name": "m2: board"}]}`,
			want: "which the project already has",
		},
		{
			name: "a slice with no title",
			doc:  `{"slices": [{"title": " ", "milestone": "M2: Board"}]}`,
			want: "slice 1 has no title",
		},
		{
			name: "a slice naming no milestone",
			doc:  `{"slices": [{"title": "Do it"}]}`,
			want: `slice 1 ("Do it") names no milestone`,
		},
		{
			name: "a slice naming a milestone that is nowhere",
			doc:  `{"slices": [{"title": "Do it", "milestone": "M9: Nope"}]}`,
			want: `no milestone named "M9: Nope"`,
		},
		{
			name: "a slice naming a page ID, which no milestone has",
			doc:  `{"slices": [{"title": "Do it", "milestone": "3b838308f65481fc8784e24878ff64f0"}]}`,
			want: `no milestone named "3b838308f65481fc8784e24878ff64f0"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := planAPI(4)

			out, err := runPlan(t, api, tt.doc)

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to mention %q", err, tt.want)
			}
			noWritesBut(t, api, 0)
			if out != "" {
				t.Errorf("output = %q, want nothing", out)
			}
		})
	}
}

func TestPlanApplyRejectsAMisusedCommandLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "two plan files", args: []string{"plan-apply", "a.json", "b.json"}, want: "given 2"},
		{name: "unknown flag", args: []string{"plan-apply", "--nope"}, want: "not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := planAPI(1)
			env, _ := testEnv(testConfig(), api)
			env.In = strings.NewReader(`{}`)

			err := Run(context.Background(), tt.args, env)

			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("err = %v (%T), want a *UsageError", err, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			noWritesBut(t, api, 0)
		})
	}
}

// Nothing is piped in and no file was named, so there is no plan to apply.
func TestPlanApplyRejectsHavingNothingToRead(t *testing.T) {
	env, _ := testEnv(testConfig(), planAPI(1))

	err := Run(context.Background(), []string{"plan-apply"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestPlanApplyReportsAnUnreadableFile(t *testing.T) {
	env, _ := testEnv(testConfig(), planAPI(1))

	err := Run(context.Background(), []string{"plan-apply", filepath.Join(t.TempDir(), "gone.json")}, env)

	if err == nil || !strings.Contains(err.Error(), "read the plan") {
		t.Errorf("err = %v, want it to report the unreadable plan", err)
	}
}

// The plan parses before the config is read, so an unfinished setup is still
// what a plan-apply run with no active project is told about.
func TestPlanApplyReportsUnfinishedSetup(t *testing.T) {
	api := planAPI(1)
	env, _ := testEnv(testConfig(), api)
	env.In = strings.NewReader(samplePlan)
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil }

	err := Run(context.Background(), []string{"plan-apply"}, env)

	if err == nil || !strings.Contains(err.Error(), "run `nat` once to set it up") {
		t.Errorf("err = %v, want it to report the unfinished setup", err)
	}
	noWritesBut(t, api, 0)
}

// A write that fails stops the run. What was already created stays in Notion,
// so the error says how much of the plan landed rather than leaving it to be
// worked out by re-running and duplicating half of it.
func TestPlanApplyReportsAFailedCreate(t *testing.T) {
	tests := []struct {
		name  string
		after int
		want  string
	}{
		{name: "the first slice, after the milestone landed", after: 0, want: "1 milestone and 0 slices were created"},
		{name: "the last slice", after: 2, want: "1 milestone and 2 slices were created"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			boom := errors.New("notion down")
			api := planAPI(3)
			api.failCreateAfter = tt.after
			api.createErr = boom

			out, err := runPlan(t, api, samplePlan)

			if !errors.Is(err, boom) {
				t.Fatalf("err = %v, want %v", err, boom)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if out != "" {
				t.Errorf("output = %q, want nothing", out)
			}
		})
	}
}

// textSpan is one written rich text span, as it arrives at Notion.
func textSpan(content string) map[string]any {
	return map[string]any{"type": "text", "text": map[string]any{"content": content}}
}

func TestPlanApplyReportsAFailedWrite(t *testing.T) {
	for _, args := range [][]string{{"plan-apply"}, {"plan-apply", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env, _ := testEnv(testConfig(), planAPI(1))
			env.In = strings.NewReader(`{"milestones": [{"name": "M4"}]}`)
			env.Out = failingWriter{}

			err := Run(context.Background(), args, env)

			if !errors.Is(err, errWrite) {
				t.Errorf("err = %v, want %v", err, errWrite)
			}
		})
	}
}

// A whole plan's worth of milestones is one schema write: the options already
// there, then the new ones in the order the document wrote them. Either they
// all arrive or none do, which is the most a plan can be applied atomically.
func TestPlanApplyAppendsEveryNewMilestoneInOneSchemaWrite(t *testing.T) {
	api := planAPI(3)

	out, err := runPlan(t, api, `{
	  "milestones": [{"name": "M4: Polish"}, {"name": "M5: Ship"}],
	  "slices": [
	    {"title": "Frame the board", "milestone": "M4: Polish"},
	    {"title": "Cut a release", "milestone": "M5: Ship"},
	    {"title": "Poll in the background", "milestone": "M2: Board"}
	  ]
	}`)
	if err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	want := `# Plan applied

Added 2 milestones and 3 slices to nat.

## M4: Polish

New milestone 4, ` + notion.MilestoneQueued + ` — ` + optionNote + `

- Frame the board — https://notion.so/new-1

## M5: Ship

New milestone 5, ` + notion.MilestoneQueued + ` — ` + optionNote + `

- Cut a release — https://notion.so/new-2

## M2: Board

- Poll in the background — https://notion.so/new-3
`
	if out != want {
		t.Errorf("output =\n%s\nwant:\n%s", out, want)
	}
	if len(api.queries) != 0 {
		t.Errorf("queries = %+v, want none: there is no Milestones data source to read", api.queries)
	}
	wantOptions := []string{"M1: Client", "M2: Board", "M3: Agents", "M4: Polish", "M5: Ship"}
	if got := writtenMilestoneOptions(t, api); !reflect.DeepEqual(got, wantOptions) {
		t.Errorf("options = %v, want %v", got, wantOptions)
	}

	if len(api.creates) != 3 {
		t.Fatalf("creates = %+v, want one page per slice", api.creates)
	}
	for i, want := range []string{"M4: Polish", "M5: Ship", "M2: Board"} {
		c := api.creates[i]
		if c.parent != notion.DataSourceParent("slices-ds") {
			t.Errorf("slice %d parent = %+v, want the slices data source", i, c.parent)
		}
		if got := c.props[notion.PropMilestone]; !reflect.DeepEqual(got, notion.NewSelect(want)) {
			t.Errorf("slice %d milestone = %+v, want the option %q", i, got, want)
		}
	}
}

// A plan that files slices under milestones the project already has writes no
// schema at all: the options it needs are there.
func TestPlanApplyLeavesTheOptionsAloneWhereItAddsNoMilestone(t *testing.T) {
	api := planAPI(1)

	if _, err := runPlan(t, api, `{"slices": [{"title": "Do it", "milestone": "M3: Agents"}]}`); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	if len(api.schemaUpdates) != 0 {
		t.Errorf("schema writes = %+v, want none", api.schemaUpdates)
	}
	if got := api.creates[0].props[notion.PropMilestone]; !reflect.DeepEqual(got, notion.NewSelect("M3: Agents")) {
		t.Errorf("milestone = %+v, want the option naming it", got)
	}
}

// The option list is written once, so a failure there leaves the plan whole:
// nothing was created, and the document can be run again as it is.
func TestPlanApplyReportsAFailedOptionWrite(t *testing.T) {
	boom := errors.New("notion down")
	api := planAPI(1)
	api.schemaUpdateErr = boom

	out, err := runPlan(t, api, `{"milestones": [{"name": "M4: Polish"}],
	  "slices": [{"title": "Frame the board", "milestone": "M4: Polish"}]}`)

	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if strings.Contains(err.Error(), "were created before this failed") {
		t.Errorf("err = %q, want nothing reported as created", err)
	}
	if len(api.creates) != 0 {
		t.Errorf("creates = %+v, want none", api.creates)
	}
	if out != "" {
		t.Errorf("output = %q, want nothing", out)
	}
}

// A plan naming a milestone the option list does not offer is refused whole,
// the same way one naming a milestone page that does not exist is.
func TestPlanApplyRefusesAnUnknownOption(t *testing.T) {
	api := planAPI(1)

	_, err := runPlan(t, api, `{"slices": [{"title": "Do it", "milestone": "M9: Later"}]}`)

	if err == nil || !strings.Contains(err.Error(), `no milestone named "M9: Later"`) {
		t.Fatalf("err = %v, want the milestone refused", err)
	}
	if len(api.creates) != 0 || len(api.schemaUpdates) != 0 {
		t.Errorf("writes = %+v %+v, want none", api.creates, api.schemaUpdates)
	}
}

// A plan whose new milestone is already an option is refused in validation,
// before the schema write that would otherwise carry two of a name.
func TestPlanApplyRefusesAnOptionThePlanAlreadyHas(t *testing.T) {
	api := planAPI(1)

	_, err := runPlan(t, api, `{"milestones": [{"name": "M2: Board"}]}`)

	if err == nil || !strings.Contains(err.Error(), "which the project already has") {
		t.Fatalf("err = %v, want the duplicate refused", err)
	}
	if len(api.schemaUpdates) != 0 {
		t.Errorf("schema writes = %+v, want none", api.schemaUpdates)
	}
}

// The plan is read against the shape the project keeps it in, so a schema that
// cannot be read stops the run before anything is written.
func TestPlanApplyReportsAFailedSchemaRead(t *testing.T) {
	boom := errors.New("notion: 500")
	api := planAPI(1)
	api.dataSourceErr = boom

	out, err := runPlan(t, api, samplePlan)

	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "load the slices schema") {
		t.Fatalf("err = %v, want the schema read reported", err)
	}
	noWritesBut(t, api, 0)
	if out != "" {
		t.Errorf("output = %q, want nothing", out)
	}
}

// notionRequest is one call as it reached the server: what the real client
// actually sent, rather than what the fake was asked for.
type notionRequest struct {
	method, path string
	body         any
}

// recordingNotion serves the given responses in order and records every request
// it was sent, so a test can assert the exact JSON Notion receives.
func recordingNotion(t *testing.T, responses []string) (*httptest.Server, *[]notionRequest) {
	t.Helper()
	var got []notionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body any
		// A GET carries none, and decoding one would read as a malformed request.
		if r.Method != http.MethodGet {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("request %d body: %v", len(got), err)
			}
		}
		got = append(got, notionRequest{method: r.Method, path: r.URL.Path, body: body})
		if n := len(got) - 1; n < len(responses) {
			_, _ = io.WriteString(w, responses[n])
			return
		}
		t.Errorf("unexpected request %d: %s %s", len(got), r.Method, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

// The fake client says what the command asked for; this says what Notion is
// actually sent — the whole way through the real client, so a property built
// with the wrong constructor is caught here rather than in production. The
// options already there are sent back with their IDs and colours, since the
// write replaces the list rather than adding to it.
func TestPlanApplyWritesTheRequestsNotionExpects(t *testing.T) {
	responses := []string{
		`{"id":"slices-ds","properties":{
			"Status":{"id":"s","name":"Status","type":"select","select":{"options":[{"id":"t","name":"Todo","color":"gray"}]}},
			"Milestone":{"id":"m","name":"Milestone","type":"select","select":{"options":[
				{"id":"o1","name":"M1: Client","color":"blue"},{"id":"o2","name":"M2: Board","color":"green"}]}},
			"Depends on":{"id":"d","name":"Depends on","type":"relation","relation":{
				"data_source_id":"slices-ds","type":"single_property","single_property":{}}},
			"Branch":{"id":"b","name":"Branch","type":"rich_text","rich_text":{}}}}`,
		`{"id":"slices-ds","properties":{"Milestone":{"type":"select","select":{"options":[
			{"id":"o1","name":"M1: Client"},{"id":"o2","name":"M2: Board"},{"id":"o3","name":"M3: Agents"}]}}}}`,
		`{"id":"new-1","url":"https://notion.so/new-1"}`,
	}
	srv, got := recordingNotion(t, responses)

	env, _ := testEnv(testConfig(), nil)
	env.NewClient = func(token notion.TokenFunc) API {
		return notion.NewWithToken(token, notion.WithBaseURL(srv.URL))
	}
	env.In = strings.NewReader(`{
	  "milestones": [{"name": "M3: Agents"}],
	  "slices": [{"title": "Frame the board", "milestone": "M3: Agents", "description": "Draw a border."}]
	}`)

	if err := Run(context.Background(), []string{"plan-apply"}, env); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	want := []notionRequest{
		{method: http.MethodGet, path: "/data_sources/slices-ds"},
		{
			method: http.MethodPatch,
			path:   "/data_sources/slices-ds",
			body: map[string]any{
				"properties": map[string]any{
					"Milestone": map[string]any{"select": map[string]any{"options": []any{
						map[string]any{"id": "o1", "name": "M1: Client", "color": "blue"},
						map[string]any{"id": "o2", "name": "M2: Board", "color": "green"},
						map[string]any{"name": "M3: Agents"},
					}}},
				},
			},
		},
		{
			method: http.MethodPost,
			path:   "/pages",
			body: map[string]any{
				"parent": map[string]any{"type": "data_source_id", "data_source_id": "slices-ds"},
				"properties": map[string]any{
					"Name":      map[string]any{"title": []any{textSpan("Frame the board")}},
					"Status":    map[string]any{"select": map[string]any{"name": "Todo"}},
					"Milestone": map[string]any{"select": map[string]any{"name": "M3: Agents"}},
					"Repo":      map[string]any{"rich_text": []any{textSpan("")}},
				},
				"children": []any{map[string]any{
					"object":    "block",
					"type":      "paragraph",
					"paragraph": map[string]any{"rich_text": []any{textSpan("Draw a border.")}},
				}},
			},
		},
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("requests =\n%+v\nwant:\n%+v", *got, want)
	}
}

// A page Notion returned no URL for is named by its ID, so the report still
// points at what it made.
func TestPlanApplyNamesAPageWithNoURLByItsID(t *testing.T) {
	api := planAPI(0)
	api.createdPage = notion.Page{ID: "new-1"}

	out, err := runPlan(t, api, `{"slices": [{"title": "Do it", "milestone": "M2: Board"}]}`)
	if err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	if !strings.Contains(out, "- Do it — new-1\n") {
		t.Errorf("output =\n%s\nwant the slice named by its ID", out)
	}
}
