package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

// sliceID is a Notion page ID, which is what the command insists a slice is
// named by — enough of one to tell a mistyped argument from a real page.
const sliceID = "3b738308f654815fa843dce9c020efb4"

// heldSlice is a slice as the workspace holds it: claimed by the user with the
// given ID, so that a slice held by somebody else is a different person and not
// merely a different name.
func heldSlice(id, name, status, userID, assignee string) notion.Page {
	p := slicePage(id, name, status, "m2", "", "")
	if userID != "" {
		p.Properties[notion.PropAssignee] = notion.PropertyValue{
			People: &[]notion.User{{ID: userID, Name: assignee}},
		}
	}
	return p
}

// completableAPI answers with one slice, claimed by the configured user and
// waiting to be closed out.
func completableAPI() *fakeAPI {
	return &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {heldSlice(sliceID, "Render the board", notion.SliceInProgress, "u1", "Craig Johnston")},
		},
	}
}

// completeEnv builds an Env for the command with nothing piped in, which is how
// it is run when the summary comes from the flag.
func completeEnv(api *fakeAPI) (Env, *strings.Builder) {
	env, _ := testEnv(testClaimConfig(), api)
	var out strings.Builder
	env.Out = &out
	env.In = strings.NewReader("")
	return env, &out
}

// blockTexts flattens appended blocks into "type: text" lines, which is enough
// to say what was written on the page without asserting on Notion's JSON shape
// twice over.
func blockTexts(t *testing.T, children []map[string]any) []string {
	t.Helper()
	lines := make([]string, 0, len(children))
	for _, block := range children {
		blockType, ok := block["type"].(string)
		if !ok {
			t.Fatalf("block %+v has no type", block)
		}
		payload, ok := block[blockType].(map[string]any)
		if !ok {
			t.Fatalf("block %+v has no %s payload", block, blockType)
		}
		spans, ok := payload["rich_text"].([]map[string]any)
		if !ok || len(spans) != 1 {
			t.Fatalf("block %+v has no single rich text span", block)
		}
		text, ok := spans[0]["text"].(map[string]any)["content"].(string)
		if !ok {
			t.Fatalf("block %+v has no text content", block)
		}
		lines = append(lines, blockType+": "+text)
	}
	return lines
}

func TestCompleteSliceFinishesTheSlice(t *testing.T) {
	api := completableAPI()
	env, out := completeEnv(api)

	err := Run(context.Background(), []string{
		"complete-slice", sliceID, "--pr", "https://github.com/x/y/pull/1",
		"--summary", "Wrote the renderer.",
	}, env)
	if err != nil {
		t.Fatalf("complete-slice: %v", err)
	}

	if len(api.gets) != 1 || api.gets[0] != sliceID {
		t.Errorf("fetched %v, want [%s]", api.gets, sliceID)
	}
	if len(api.appends) != 1 {
		t.Fatalf("appends = %+v, want exactly one", api.appends)
	}
	if api.appends[0].id != sliceID {
		t.Errorf("appended to %q, want %s", api.appends[0].id, sliceID)
	}
	want := []string{"heading_3: Summary", "paragraph: Wrote the renderer."}
	if got := blockTexts(t, api.appends[0].children); !equalLines(got, want) {
		t.Errorf("blocks = %v, want %v", got, want)
	}

	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want exactly one", api.updates)
	}
	props := api.updates[0].props
	if api.updates[0].id != sliceID {
		t.Errorf("updated %q, want %s", api.updates[0].id, sliceID)
	}
	if name := props[notion.PropStatus].SelectName(); name != notion.SliceDone {
		t.Errorf("status = %q, want %q", name, notion.SliceDone)
	}
	if props[notion.PropPR].URL != "https://github.com/x/y/pull/1" {
		t.Errorf("PR = %q, want the pull request", props[notion.PropPR].URL)
	}
	if _, wrote := props[notion.PropAssignee]; wrote {
		t.Errorf("props = %+v, want the assignee left alone", props)
	}

	wantOut := fmt.Sprintf(`# Render the board

Done. The summary is on the slice page.

- Notion page: %[1]s
- Notion URL: https://notion.so/%[1]s
- PR: https://github.com/x/y/pull/1
`, sliceID)
	if out.String() != wantOut {
		t.Errorf("output =\n%s\nwant:\n%s", out.String(), wantOut)
	}
}

// equalLines compares two lists of strings.
func equalLines(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// The note appended is the whole page body's worth: one paragraph per chunk,
// with the blank runs between them dropped rather than written as empty blocks.
func TestCompleteSliceAppendsAParagraphPerChunk(t *testing.T) {
	api := completableAPI()
	env, _ := completeEnv(api)

	err := Run(context.Background(), []string{
		"complete-slice", sliceID, "--summary", "Wrote the renderer.\r\n\r\n\r\n\r\nFollow-up: style it.\n",
	}, env)
	if err != nil {
		t.Fatalf("complete-slice: %v", err)
	}

	want := []string{"heading_3: Summary", "paragraph: Wrote the renderer.", "paragraph: Follow-up: style it."}
	if got := blockTexts(t, api.appends[0].children); !equalLines(got, want) {
		t.Errorf("blocks = %v, want %v", got, want)
	}
}

// Without --summary the note is read from stdin, which is how one longer than a
// shell argument gets in.
func TestCompleteSliceReadsTheSummaryFromStdin(t *testing.T) {
	api := completableAPI()
	env, _ := completeEnv(api)
	env.In = strings.NewReader("  Wrote the renderer.\n")

	if err := Run(context.Background(), []string{"complete-slice", sliceID}, env); err != nil {
		t.Fatalf("complete-slice: %v", err)
	}

	want := []string{"heading_3: Summary", "paragraph: Wrote the renderer."}
	if got := blockTexts(t, api.appends[0].children); !equalLines(got, want) {
		t.Errorf("blocks = %v, want %v", got, want)
	}
}

// --blocked is the other way a session ends: the note goes on, the status does
// not move, and the slice stays with the agent that could not finish it.
func TestCompleteSliceBlockedLeavesTheSliceInProgress(t *testing.T) {
	api := completableAPI()
	env, out := completeEnv(api)

	err := Run(context.Background(), []string{
		"complete-slice", sliceID, "--blocked", "--summary", "The API has no endpoint for it.",
	}, env)
	if err != nil {
		t.Fatalf("complete-slice --blocked: %v", err)
	}

	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want none: a blocked slice keeps its status", api.updates)
	}
	want := []string{"heading_3: Blocked", "paragraph: The API has no endpoint for it."}
	if got := blockTexts(t, api.appends[0].children); !equalLines(got, want) {
		t.Errorf("blocks = %v, want %v", got, want)
	}

	wantOut := fmt.Sprintf(`# Render the board

Still in progress, held by Craig Johnston. The note is on the slice page.

- Notion page: %[1]s
- Notion URL: https://notion.so/%[1]s
`, sliceID)
	if out.String() != wantOut {
		t.Errorf("output =\n%s\nwant:\n%s", out.String(), wantOut)
	}
}

// A slice blocked after the PR was pushed still records it: the work is on a
// branch either way, and the status is the only thing --blocked holds back.
func TestCompleteSliceBlockedStillRecordsThePR(t *testing.T) {
	api := completableAPI()
	env, out := completeEnv(api)

	err := Run(context.Background(), []string{
		"complete-slice", sliceID, "--blocked", "--pr", "https://github.com/x/y/pull/1",
		"--summary", "Review found a design problem.",
	}, env)
	if err != nil {
		t.Fatalf("complete-slice --blocked: %v", err)
	}

	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want exactly one", api.updates)
	}
	props := api.updates[0].props
	if props[notion.PropPR].URL != "https://github.com/x/y/pull/1" {
		t.Errorf("PR = %q, want the pull request", props[notion.PropPR].URL)
	}
	if _, wrote := props[notion.PropStatus]; wrote {
		t.Errorf("props = %+v, want the status left alone", props)
	}
	if !strings.Contains(out.String(), "Still in progress") {
		t.Errorf("output =\n%s\nwant it to say the slice is still in progress", out.String())
	}
}

// A Status column converted to Notion's own status type in the UI is written
// back in that shape, not as the select this app would have created.
func TestCompleteSliceWritesTheStatusShapeItRead(t *testing.T) {
	api := completableAPI()
	api.pages["slices-ds"][0].Properties[notion.PropStatus] = notion.PropertyValue{
		Type: notion.TypeStatus, Status: &notion.SelectOption{Name: notion.SliceInProgress},
	}
	env, _ := completeEnv(api)

	err := Run(context.Background(), []string{"complete-slice", sliceID, "--summary", "Done."}, env)
	if err != nil {
		t.Fatalf("complete-slice: %v", err)
	}

	status := api.updates[0].props[notion.PropStatus]
	if status.Status == nil || status.Select != nil {
		t.Errorf("status = %+v, want a status value", status)
	}
}

// The slice can be named however the brief printed it: as a page ID, dashed or
// not, or as the URL with its title slug and whatever Notion hangs off the end.
func TestCompleteSliceAcceptsEveryWayOfNamingTheSlice(t *testing.T) {
	const id = "3b738308f654815fa843dce9c020efb4"
	const dashed = "3b738308-f654-815f-a843-dce9c020efb4"
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{name: "bare ID", ref: id, want: id},
		{name: "dashed ID", ref: dashed, want: dashed},
		{name: "page URL", ref: "https://app.notion.com/p/Add-a-command-" + id, want: id},
		{name: "URL with a query", ref: "https://app.notion.com/p/Add-a-command-" + id + "?pvs=204", want: id},
		{name: "URL with a fragment", ref: "https://www.notion.so/" + dashed + "#b1", want: dashed},
		{name: "URL with a trailing slash", ref: "https://www.notion.so/" + id + "/", want: id},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := completableAPI()
			api.pages["slices-ds"][0].ID = tt.want
			env, _ := completeEnv(api)

			err := Run(context.Background(), []string{"complete-slice", tt.ref, "--summary", "Done."}, env)

			if err != nil {
				t.Fatalf("complete-slice %s: %v", tt.ref, err)
			}
			if len(api.gets) != 1 || api.gets[0] != tt.want {
				t.Errorf("fetched %v, want [%s]", api.gets, tt.want)
			}
		})
	}
}

// Flags either side of the slice are both understood: the flag package stops at
// the first argument, and the order anyone writes this in puts the slice first.
func TestCompleteSliceTakesFlagsEitherSideOfTheSlice(t *testing.T) {
	for _, args := range [][]string{
		{"complete-slice", sliceID, "--summary", "Done."},
		{"complete-slice", "--summary", "Done.", sliceID},
		{"complete-slice", "--summary", "Done.", sliceID, "--pr", "https://github.com/x/y/pull/1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			api := completableAPI()
			env, _ := completeEnv(api)

			if err := Run(context.Background(), args, env); err != nil {
				t.Fatalf("complete-slice: %v", err)
			}
			if len(api.appends) != 1 || len(api.updates) != 1 {
				t.Errorf("calls = %+v %+v, want one of each", api.appends, api.updates)
			}
		})
	}
}

// A slice this user does not hold is left exactly as it was found, whatever the
// reason: nobody else's slice is closed out, and neither is one never claimed.
func TestCompleteSliceRefusesASliceItDoesNotHold(t *testing.T) {
	tests := []struct {
		name    string
		page    notion.Page
		wantErr []string
	}{
		{
			name:    "still Todo",
			page:    heldSlice(sliceID, "Render the board", notion.SliceTodo, "", ""),
			wantErr: []string{"is Todo, not In progress", "Render the board"},
		},
		{
			name:    "already Done",
			page:    heldSlice(sliceID, "Render the board", notion.SliceDone, "u1", "Craig Johnston"),
			wantErr: []string{"is Done, not In progress"},
		},
		{
			name:    "claimed by someone else",
			page:    heldSlice(sliceID, "Render the board", notion.SliceInProgress, "u2", "Someone Else"),
			wantErr: []string{"held by Someone Else, not by Craig Johnston", "leave it to them"},
		},
		{
			name:    "claimed by nobody",
			page:    heldSlice(sliceID, "Render the board", notion.SliceInProgress, "", ""),
			wantErr: []string{"in progress but held by nobody, not by Craig Johnston"},
		},
		{
			name:    "no status at all",
			page:    notion.Page{ID: sliceID, Properties: map[string]notion.PropertyValue{notion.PropName: title("Render the board")}},
			wantErr: []string{"is (no status), not In progress"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{pages: map[string][]notion.Page{"slices-ds": {tt.page}}}
			env, out := completeEnv(api)

			err := Run(context.Background(), []string{"complete-slice", sliceID, "--summary", "Done."}, env)

			if err == nil {
				t.Fatal("err = nil, want a refusal")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %q, want it to mention %q", err, want)
				}
			}
			if len(api.appends) != 0 || len(api.updates) != 0 {
				t.Errorf("writes = %+v %+v, want none", api.appends, api.updates)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// A slice closed out with nothing written on it loses the only record of what
// was done, so an empty summary is a misuse — and one caught before the slice
// is read, let alone written to.
func TestCompleteSliceNeedsASummary(t *testing.T) {
	tests := []struct {
		name string
		args []string
		in   string
	}{
		{name: "no summary anywhere", args: []string{"complete-slice", sliceID}},
		{name: "blank flag", args: []string{"complete-slice", sliceID, "--summary", "   "}},
		{name: "blank stdin", args: []string{"complete-slice", sliceID}, in: "\n\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := completableAPI()
			env, out := completeEnv(api)
			env.In = strings.NewReader(tt.in)

			err := Run(context.Background(), tt.args, env)

			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("err = %v (%T), want a *UsageError", err, err)
			}
			if !strings.Contains(err.Error(), "no summary given") {
				t.Errorf("err = %q, want it to ask for a summary", err)
			}
			if len(api.gets) != 0 || len(api.appends) != 0 || len(api.updates) != 0 {
				t.Errorf("calls = %+v %+v %+v, want none", api.gets, api.appends, api.updates)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// Run with nothing to read from — no stdin at all — the missing summary is
// still reported rather than read off a nil reader.
func TestCompleteSliceWithoutAnInput(t *testing.T) {
	api := completableAPI()
	env, _ := completeEnv(api)
	env.In = nil

	err := Run(context.Background(), []string{"complete-slice", sliceID}, env)

	if err == nil || !strings.Contains(err.Error(), "no summary given") {
		t.Fatalf("err = %v, want it to ask for a summary", err)
	}
}

func TestCompleteSliceReportsAFailedCall(t *testing.T) {
	boom := errors.New("notion: 500")
	tests := []struct {
		name string
		api  func() *fakeAPI
		want string
	}{
		{
			name: "the slice",
			api: func() *fakeAPI {
				api := completableAPI()
				api.getErr = boom
				return api
			},
			want: "load the slice",
		},
		{
			name: "the note",
			api: func() *fakeAPI {
				api := completableAPI()
				api.appendErr = boom
				return api
			},
			want: "append the note to the slice",
		},
		{
			name: "the status",
			api: func() *fakeAPI {
				api := completableAPI()
				api.updateErr = boom
				return api
			},
			want: "close out the slice",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := tt.api()
			env, out := completeEnv(api)

			err := Run(context.Background(), []string{"complete-slice", sliceID, "--summary", "Done."}, env)

			if !errors.Is(err, boom) {
				t.Fatalf("err = %v, want %v", err, boom)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// A note that cannot be appended stops the command there: a slice marked Done
// with no summary on it refuses every attempt to add one afterwards.
func TestCompleteSliceLeavesTheStatusAloneWhenTheNoteFails(t *testing.T) {
	api := completableAPI()
	api.appendErr = errors.New("notion: 500")
	env, _ := completeEnv(api)

	_ = Run(context.Background(), []string{"complete-slice", sliceID, "--summary", "Done."}, env)

	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want none: the summary never landed", api.updates)
	}
}

func TestCompleteSliceReportsAnUnreadableSummary(t *testing.T) {
	api := completableAPI()
	env, _ := completeEnv(api)
	env.In = failingReader{}

	err := Run(context.Background(), []string{"complete-slice", sliceID}, env)

	if !errors.Is(err, errRead) {
		t.Fatalf("err = %v, want %v", err, errRead)
	}
	if !strings.Contains(err.Error(), "read the summary") {
		t.Errorf("err = %q, want it to say what failed", err)
	}
}

// failingReader is an input that cannot be read from.
type failingReader struct{}

var errRead = errors.New("no input")

func (failingReader) Read([]byte) (int, error) { return 0, errRead }

// Closing out a slice needs to know who this agent is, and that comes from the
// config the board wrote. Without it nothing is read and nothing is written.
func TestCompleteSliceNeedsAnAssignee(t *testing.T) {
	api := completableAPI()
	env, _ := testEnv(testConfig(), api)
	env.In = strings.NewReader("")

	err := Run(context.Background(), []string{"complete-slice", sliceID, "--summary", "Done."}, env)

	if err == nil || !strings.Contains(err.Error(), "no assignee in the config") {
		t.Fatalf("err = %v, want it to ask for an assignee", err)
	}
	if len(api.gets) != 0 || len(api.appends) != 0 || len(api.updates) != 0 {
		t.Errorf("calls = %+v %+v %+v, want none", api.gets, api.appends, api.updates)
	}
}

// Setup that has not happened yet is reported before the slice is touched.
func TestCompleteSliceReportsUnfinishedSetup(t *testing.T) {
	api := completableAPI()
	env, _ := completeEnv(api)
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil }

	err := Run(context.Background(), []string{"complete-slice", sliceID, "--summary", "Done."}, env)

	if err == nil || !strings.Contains(err.Error(), "run `nat` once to set it up") {
		t.Fatalf("err = %v, want it to point at setup", err)
	}
	if len(api.appends) != 0 || len(api.updates) != 0 {
		t.Errorf("writes = %+v %+v, want none", api.appends, api.updates)
	}
}

func TestCompleteSliceReportsAFailedWrite(t *testing.T) {
	env, _ := completeEnv(completableAPI())
	env.Out = failingWriter{}

	err := Run(context.Background(), []string{"complete-slice", sliceID, "--summary", "Done."}, env)

	if !errors.Is(err, errWrite) {
		t.Errorf("err = %v, want %v", err, errWrite)
	}
}

func TestCompleteSliceRejectsAMisusedCommandLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown flag", args: []string{"complete-slice", "--nope", sliceID}, want: "not defined"},
		{name: "no slice", args: []string{"complete-slice", "--summary", "Done."}, want: "want exactly one slice"},
		{name: "two slices", args: []string{"complete-slice", sliceID, "s4", "--summary", "Done."}, want: "want exactly one slice"},
		{
			name: "not a slice",
			args: []string{"complete-slice", "the board", "--summary", "Done."},
			want: `"the board" is not a slice`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := completableAPI()
			env, out := completeEnv(api)

			err := Run(context.Background(), tt.args, env)

			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("err = %v (%T), want a *UsageError", err, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "complete-slice:") {
				t.Errorf("err = %q, want it to name the command", err)
			}
			if len(api.gets) != 0 || len(api.appends) != 0 || len(api.updates) != 0 {
				t.Errorf("calls = %+v %+v %+v, want none: the command line was rejected", api.gets, api.appends, api.updates)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// Without an Assignee column there is nobody a slice could be held by but the
// person running the command, so an In progress slice closes out on its status
// alone rather than being refused for having no assignee.
func TestCompleteSliceClosesOutAProjectWithNoAssigneeColumn(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {heldSlice(sliceID, "Render the board", notion.SliceInProgress, "", "")},
		},
		dataSources: map[string]notion.DataSource{"slices-ds": soloSlicesDS()},
	}
	env, out := completeEnv(api)

	if err := Run(context.Background(), []string{"complete-slice", sliceID, "--summary", "Done."}, env); err != nil {
		t.Fatalf("complete-slice: %v", err)
	}

	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want exactly one", api.updates)
	}
	if name := api.updates[0].props[notion.PropStatus].SelectName(); name != notion.SliceDone {
		t.Errorf("status = %q, want %q", name, notion.SliceDone)
	}
	if !strings.Contains(out.String(), "Done.") {
		t.Errorf("output =\n%s\nwant the slice reported Done", out.String())
	}
}

// A slice of such a project that is not in progress is still refused, named by
// the status its own project calls it.
func TestCompleteSliceRefusesATodoSliceOfAProjectWithNoAssigneeColumn(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {heldSlice(sliceID, "Render the board", notion.SliceTodo, "", "")},
		},
		dataSources: map[string]notion.DataSource{"slices-ds": soloSlicesDS()},
	}
	env, _ := completeEnv(api)

	err := Run(context.Background(), []string{"complete-slice", sliceID, "--summary", "Done."}, env)
	if err == nil || !strings.Contains(err.Error(), "is Todo, not In progress") {
		t.Fatalf("err = %v, want it to name the project's own in-progress status", err)
	}
}

// A project keeping its whole plan on one page closes a slice out the same way:
// what the Milestone column holds is nothing to do with finishing the work, and
// ownership is decided on the status the project's own schema names.
func TestCompleteSliceClosesOutASliceOfAOnePagePlan(t *testing.T) {
	slice := slicePage(sliceID, "Render the board", notion.SliceInProgress, "M2: Board", "", "")
	api := &fakeAPI{
		pages:       map[string][]notion.Page{"slices-ds": {slice}},
		dataSources: map[string]notion.DataSource{"slices-ds": selectMilestoneSlicesDS("M1: Client", "M2: Board")},
	}
	env, out := completeEnv(api)

	if err := Run(context.Background(), []string{"complete-slice", sliceID, "--summary", "Done."}, env); err != nil {
		t.Fatalf("complete-slice: %v", err)
	}

	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want exactly one", api.updates)
	}
	if name := api.updates[0].props[notion.PropStatus].SelectName(); name != notion.SliceDone {
		t.Errorf("status = %q, want %q", name, notion.SliceDone)
	}
	if _, wrote := api.updates[0].props[notion.PropMilestone]; wrote {
		t.Errorf("props = %+v, want the milestone left alone", api.updates[0].props)
	}
	if !strings.Contains(out.String(), "Done.") {
		t.Errorf("output =\n%s\nwant the slice reported Done", out.String())
	}
}

func TestCompleteSliceReportsAFailedSchemaRead(t *testing.T) {
	api := completableAPI()
	api.dataSourceErr = errors.New("boom")
	env, _ := completeEnv(api)

	err := Run(context.Background(), []string{"complete-slice", sliceID, "--summary", "Done."}, env)
	if err == nil || !strings.Contains(err.Error(), "load the slices schema") {
		t.Fatalf("err = %v, want the schema read named", err)
	}
	if len(api.appends) != 0 {
		t.Errorf("appends = %+v, want nothing written", api.appends)
	}
}

// The ending an agent runs now: the branch it pushed is recorded, the slice is
// left in progress for someone to review, and the note is filed as a hand-back
// rather than as the last word on the slice.
func TestCompleteSliceHandsBackABranch(t *testing.T) {
	api := completableAPI()
	env, out := completeEnv(api)

	err := Run(context.Background(), []string{
		"complete-slice", sliceID, "--branch", " slice/render-the-board ",
		"--summary", "Wrote the renderer.",
	}, env)
	if err != nil {
		t.Fatalf("complete-slice: %v", err)
	}

	if len(api.appends) != 1 {
		t.Fatalf("appends = %+v, want exactly one", api.appends)
	}
	want := []string{"heading_3: Handed back", "paragraph: Wrote the renderer."}
	if got := blockTexts(t, api.appends[0].children); !equalLines(got, want) {
		t.Errorf("blocks = %v, want %v", got, want)
	}

	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want exactly one", api.updates)
	}
	props := api.updates[0].props
	// Asserted on the spans rather than through Text(), which reads the plain
	// text Notion answers with and a write does not carry.
	if spans := props[notion.PropBranch].RichText; len(spans) != 1 ||
		spans[0].Text == nil || spans[0].Text.Content != "slice/render-the-board" {
		t.Errorf("branch = %+v, want the branch, trimmed", spans)
	}
	if _, wrote := props[notion.PropStatus]; wrote {
		t.Errorf("props = %+v, want the status left in progress", props)
	}
	if _, wrote := props[notion.PropPR]; wrote {
		t.Errorf("props = %+v, want no pull request recorded", props)
	}

	wantOut := fmt.Sprintf(`# Render the board

Handed back for review, still held by Craig Johnston. The summary is on the slice page, and approving it on the board is what opens the pull request.

- Notion page: %[1]s
- Notion URL: https://notion.so/%[1]s
- Branch: slice/render-the-board
`, sliceID)
	if out.String() != wantOut {
		t.Errorf("output =\n%s\nwant:\n%s", out.String(), wantOut)
	}
}

// The three endings are three different ones, and asking for two at once is a
// misuse: nothing is read and nothing is written.
func TestCompleteSliceRefusesTwoEndingsAtOnce(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			"a branch and a pull request",
			[]string{"--branch", "slice/x", "--pr", "https://github.com/x/y/pull/1"},
			"--branch and --pr",
		},
		{
			"a branch and blocked",
			[]string{"--branch", "slice/x", "--blocked"},
			"--branch and --blocked",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := completableAPI()
			env, out := completeEnv(api)

			args := append([]string{"complete-slice", sliceID, "--summary", "Done."}, tt.args...)
			err := Run(context.Background(), args, env)

			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("err = %v, want a usage error", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if len(api.gets) != 0 || len(api.appends) != 0 || len(api.updates) != 0 {
				t.Errorf("reads %v, writes %+v %+v; want none", api.gets, api.appends, api.updates)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// A project whose Branch column is not text cannot hold a hand-back — the
// back-fill leaves such a column alone rather than converting somebody's — so
// the branch is refused before the note is written and the slice is left exactly
// as it was.
func TestCompleteSliceRefusesABranchWithNoColumnToHoldIt(t *testing.T) {
	api := completableAPI()
	ds := assigneeSlicesDS()
	ds.Properties[notion.PropBranch] = notion.PropertySchema{Type: "url"}
	api.dataSources = map[string]notion.DataSource{"slices-ds": ds}
	env, out := completeEnv(api)

	err := Run(context.Background(), []string{
		"complete-slice", sliceID, "--branch", "slice/x", "--summary", "Done.",
	}, env)

	if err == nil || !strings.Contains(err.Error(), `no Branch text column`) {
		t.Fatalf("err = %v, want the missing column named", err)
	}
	if len(api.appends) != 0 || len(api.updates) != 0 {
		t.Errorf("writes = %+v %+v, want none", api.appends, api.updates)
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want nothing", out.String())
	}
}
