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
	api := planAPI(4)

	out, err := runPlan(t, api, samplePlan)
	if err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	want := `# Plan applied

Added 1 milestone and 3 slices to nat.

## M4: Polish

New milestone 4, Queued — https://notion.so/new-1

- Frame the board — https://notion.so/new-2
- Colour the chips — https://notion.so/new-3

## M2: Board

- Poll in the background — https://notion.so/new-4
`
	if out != want {
		t.Errorf("output =\n%s\nwant:\n%s", out, want)
	}

	noWritesBut(t, api, 4)
	m := api.creates[0]
	if m.parent != notion.DataSourceParent("milestones-ds") {
		t.Errorf("milestone parent = %+v, want the milestones data source", m.parent)
	}
	if got := writtenText(m.props[notion.PropName]); got != "M4: Polish" {
		t.Errorf("milestone name = %q, want %q", got, "M4: Polish")
	}
	if got, _ := m.props[notion.PropOrder].NumberValue(); got != 4 {
		t.Errorf("milestone order = %v, want 4", got)
	}
	if got := m.props[notion.PropStatus].SelectName(); got != notion.MilestoneQueued {
		t.Errorf("milestone status = %q, want %q", got, notion.MilestoneQueued)
	}

	first := api.creates[1]
	if first.parent != notion.DataSourceParent("slices-ds") {
		t.Errorf("slice parent = %+v, want the slices data source", first.parent)
	}
	if got := writtenText(first.props[notion.PropName]); got != "Frame the board" {
		t.Errorf("slice name = %q, want %q", got, "Frame the board")
	}
	if got := first.props[notion.PropStatus].SelectName(); got != notion.SliceTodo {
		t.Errorf("slice status = %q, want %q", got, notion.SliceTodo)
	}
	// The relation points at the milestone this run just made, not at anything
	// named in the document.
	if got := first.props[notion.PropMilestone].RelationIDs(); !reflect.DeepEqual(got, []string{"new-1"}) {
		t.Errorf("milestone relation = %v, want [new-1]", got)
	}
	if _, ok := first.props[notion.PropAssignee]; ok {
		t.Errorf("properties = %+v, want no assignee: a planned slice is unclaimed", first.props)
	}
	if len(first.children) != 2 {
		t.Errorf("children = %+v, want one paragraph per chunk of the description", first.children)
	}

	last := api.creates[3]
	if got := last.props[notion.PropMilestone].RelationIDs(); !reflect.DeepEqual(got, []string{"m2"}) {
		t.Errorf("milestone relation = %v, want [m2]: the existing milestone", got)
	}
	if got := writtenText(last.props[notion.PropRepo]); got != "/tmp/other" {
		t.Errorf("repo = %q, want %q", got, "/tmp/other")
	}
}

// New milestones continue the plan's numbering, one after another, from past
// the highest order there already is.
func TestPlanApplyNumbersNewMilestonesFromTheEndOfThePlan(t *testing.T) {
	api := planAPI(2)

	if _, err := runPlan(t, api, `{"milestones": [{"name": "M4"}, {"name": "M5"}]}`); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	for i, want := range []float64{4, 5} {
		if got, _ := api.creates[i].props[notion.PropOrder].NumberValue(); got != want {
			t.Errorf("milestone %d order = %v, want %v", i+1, got, want)
		}
	}
}

// A slice may name an existing milestone however the drafting had it to hand.
func TestPlanApplyResolvesAnExistingMilestoneHoweverItIsNamed(t *testing.T) {
	// The milestone IDs of the shared fake are not UUID-shaped, so a reference
	// by ID needs a plan whose milestones are.
	const id = "3b838308-f654-8130-a22d-f57de208079e"
	refs := map[string]string{
		"by name":            "M2: Board",
		"by name, any case":  "m2: board",
		"by ID":              id,
		"by ID without dash": strings.ReplaceAll(id, "-", ""),
		"by URL":             "https://notion.so/M2-Board-" + strings.ReplaceAll(id, "-", ""),
	}
	for name, ref := range refs {
		t.Run(name, func(t *testing.T) {
			api := planAPI(1)
			api.pages["milestones-ds"] = []notion.Page{milestonePage(id, "M2: Board", 2, notion.MilestoneActive)}
			doc := fmt.Sprintf(`{"slices": [{"title": "Do it", "milestone": %q}]}`, ref)

			if _, err := runPlan(t, api, doc); err != nil {
				t.Fatalf("plan-apply: %v", err)
			}

			if got := api.creates[0].props[notion.PropMilestone].RelationIDs(); !reflect.DeepEqual(got, []string{id}) {
				t.Errorf("milestone relation = %v, want [%s]", got, id)
			}
		})
	}
}

// A milestone created with nothing under it is reported as such rather than as
// a heading with nothing after it, and a page Notion returned no URL for is
// named by its ID.
func TestPlanApplyReportsAMilestoneWithNoSlices(t *testing.T) {
	api := planAPI(1)
	api.createdPages = []notion.Page{{ID: "new-1"}}

	out, err := runPlan(t, api, `{"milestones": [{"name": "M4: Polish"}]}`)
	if err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	want := `# Plan applied

Added 1 milestone and 0 slices to nat.

## M4: Polish

New milestone 4, Queued — new-1

_no slices_
`
	if out != want {
		t.Errorf("output =\n%s\nwant:\n%s", out, want)
	}
}

func TestPlanApplyPrintsJSON(t *testing.T) {
	api := planAPI(4)

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
			ID: "new-1", Name: "M4: Polish", Order: 4,
			Status: notion.MilestoneQueued, URL: "https://notion.so/new-1",
		}},
		Slices: []addedSliceJSON{
			{
				ID: "new-2", Name: "Frame the board", Status: notion.SliceTodo,
				MilestoneID: "new-1", MilestoneName: "M4: Polish",
				Repo: "/tmp/nat", URL: "https://notion.so/new-2",
			},
			{
				ID: "new-3", Name: "Colour the chips", Status: notion.SliceTodo,
				MilestoneID: "new-1", MilestoneName: "M4: Polish",
				Repo: "/tmp/nat", URL: "https://notion.so/new-3",
			},
			{
				ID: "new-4", Name: "Poll in the background", Status: notion.SliceTodo,
				MilestoneID: "m2", MilestoneName: "M2: Board",
				Repo: "/tmp/other", URL: "https://notion.so/new-4",
			},
		},
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

	if !strings.Contains(out, `"slices": []`) {
		t.Errorf("json =\n%s\nwant an empty slices list", out)
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
	if len(api.creates) != 1 {
		t.Errorf("creates = %d, want 1", len(api.creates))
	}
}

// `-` is the same thing as no file at all: the plan is piped in.
func TestPlanApplyReadsStdinForADashedFile(t *testing.T) {
	api := planAPI(1)

	if _, err := runPlan(t, api, `{"milestones": [{"name": "M4"}]}`, "-"); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	if len(api.creates) != 1 {
		t.Errorf("creates = %d, want 1", len(api.creates))
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
		{name: "nothing to create", doc: `{}`, want: "creates nothing"},
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
			name: "a slice naming a milestone ID of some other workspace",
			doc:  `{"slices": [{"title": "Do it", "milestone": "3b838308f65481fc8784e24878ff64f0"}]}`,
			want: "no milestone 3b838308f65481fc8784e24878ff64f0 in this project",
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

func TestPlanApplyReportsAFailedMilestoneQuery(t *testing.T) {
	want := errors.New("notion down")
	api := planAPI(1)
	api.queryErr = map[string]error{"milestones-ds": want}

	_, err := runPlan(t, api, samplePlan)

	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
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
		{name: "the first milestone", after: 0, want: "create the milestone"},
		{name: "a slice, after a milestone landed", after: 1, want: "1 milestone and 0 slices were created"},
		{name: "the last slice", after: 3, want: "1 milestone and 2 slices were created"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			boom := errors.New("notion down")
			api := planAPI(4)
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

// The fake client says what the command asked for; this says what Notion is
// actually sent — the whole way through the real client, so a property built
// with the wrong constructor is caught here rather than in production.
func TestPlanApplyWritesTheRequestsNotionExpects(t *testing.T) {
	responses := []string{
		`{"results":[{"id":"m2","url":"https://notion.so/m2","properties":{
			"Name":{"type":"title","title":[{"plain_text":"M2: Board"}]},
			"Order":{"type":"number","number":2},
			"Status":{"type":"select","select":{"name":"Active"}}}}],"has_more":false}`,
		`{"id":"new-1","url":"https://notion.so/new-1"}`,
		`{"id":"new-2","url":"https://notion.so/new-2"}`,
	}
	type request struct {
		method, path string
		body         any
	}
	var got []request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("request %d body: %v", len(got), err)
		}
		got = append(got, request{method: r.Method, path: r.URL.Path, body: body})
		if n := len(got) - 1; n < len(responses) {
			_, _ = io.WriteString(w, responses[n])
			return
		}
		t.Errorf("unexpected request %d: %s %s", len(got), r.Method, r.URL.Path)
	}))
	defer srv.Close()

	env, _ := testEnv(testConfig(), nil)
	env.NewClient = func(token notion.TokenFunc) API {
		return notion.NewWithToken(token, notion.WithBaseURL(srv.URL))
	}
	env.In = strings.NewReader(`{
	  "milestones": [{"name": "M4: Polish"}],
	  "slices": [{"title": "Frame the board", "milestone": "M4: Polish", "description": "Draw a border.", "repo": "/tmp/other"}]
	}`)

	if err := Run(context.Background(), []string{"plan-apply"}, env); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	want := []request{
		{
			method: http.MethodPost,
			path:   "/data_sources/milestones-ds/query",
			body: map[string]any{
				"sorts": []any{map[string]any{"property": "Order", "direction": "ascending"}},
			},
		},
		{
			method: http.MethodPost,
			path:   "/pages",
			body: map[string]any{
				"parent": map[string]any{"type": "data_source_id", "data_source_id": "milestones-ds"},
				"properties": map[string]any{
					"Name":   map[string]any{"title": []any{textSpan("M4: Polish")}},
					"Order":  map[string]any{"number": 3.0},
					"Status": map[string]any{"select": map[string]any{"name": "Queued"}},
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
					"Milestone": map[string]any{"relation": []any{map[string]any{"id": "new-1"}}},
					"Repo":      map[string]any{"rich_text": []any{textSpan("/tmp/other")}},
				},
				"children": []any{map[string]any{
					"object":    "block",
					"type":      "paragraph",
					"paragraph": map[string]any{"rich_text": []any{textSpan("Draw a border.")}},
				}},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("requests =\n%+v\nwant:\n%+v", got, want)
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
