package plugin

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/plugins/shortcut/internal/fakeshortcut"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/settings"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/shortcut"
)

// act runs an action and returns its message.
func (h *harness) act(action, target, input string) string {
	h.t.Helper()
	extra := fmt.Sprintf(`"action":%q,"target":%s`, action, target)
	if input != "" {
		extra += fmt.Sprintf(`,"input":%q`, input)
	}
	return h.call("action", extra)
}

func (h *harness) actFail(action, target, input string) string {
	h.t.Helper()
	extra := fmt.Sprintf(`"action":%q,"target":%s,"input":%q`, action, target, input)
	return h.fail("action", extra)
}

func (h *harness) segments() []settings.Segment {
	h.t.Helper()
	f, err := settings.Load(settings.Dirs{Getenv: h.env().Getenv}.ConfigFile())
	if err != nil {
		h.t.Fatal(err)
	}
	return f.Project("p1").Segments
}

func TestActionComment(t *testing.T) {
	h := newHarness(t)
	if got := h.act("comment", `{"container":"4821"}`, "  Looks good\nto me  "); got != `{"message":"Commented on sc-4821"}`+"\n" {
		t.Errorf("message = %q", got)
	}
	if got := h.writes(); got != `POST /stories/4821/comments {"text":"Looks good\nto me"}` {
		t.Errorf("writes = %s", got)
	}
	for _, tc := range []struct{ target, input, want string }{
		{`{"container":"4821"}`, "   ", "shortcut: a comment needs some text"},
		{`{}`, "hi", `shortcut: "" is not a story id`},
		{`{"container":"999"}`, "hi", "shortcut: no story sc-999"},
	} {
		if errs := h.actFail("comment", tc.target, tc.input); errs != tc.want {
			t.Errorf("comment %s %q: %q, want %q", tc.target, tc.input, errs, tc.want)
		}
	}
}

func TestActionAssignFollow(t *testing.T) {
	h := newHarness(t)
	if got := h.act("assign", `{"container":"4811"}`, ""); !strings.Contains(got, "Assigned sc-4811 to you") {
		t.Errorf("assign = %q", got)
	}
	if got := h.act("follow", `{"container":"4821"}`, ""); !strings.Contains(got, "Following sc-4821") {
		t.Errorf("follow = %q", got)
	}
	want := `PUT /stories/4811 {"owner_ids":["` + fakeshortcut.MeID + `"]}` + "\n" +
		`PUT /stories/4821 {"follower_ids":["` + fakeshortcut.MeID + `"]}`
	if got := h.writes(); got != want {
		t.Errorf("writes:\n%s\nwant:\n%s", got, want)
	}
	// Again: already so, nothing written.
	h.fake.Reset()
	if got := h.act("assign", `{"container":"4821"}`, ""); !strings.Contains(got, "You already own sc-4821") {
		t.Errorf("assign again = %q", got)
	}
	if got := h.act("follow", `{"container":"4821"}`, ""); !strings.Contains(got, "Already following sc-4821") {
		t.Errorf("follow again = %q", got)
	}
	if got := h.writes(); got != "" {
		t.Errorf("writes = %s", got)
	}
	// An owner is added to the others, not in place of them.
	h.fake.Stories[fakeshortcut.StoryBug].OwnerIDs = []string{fakeshortcut.DanaID}
	h.act("assign", `{"container":"4811"}`, "")
	if got := h.writes(); got != `PUT /stories/4811 {"owner_ids":["`+fakeshortcut.DanaID+`","`+fakeshortcut.MeID+`"]}` {
		t.Errorf("writes = %s", got)
	}

	if errs := h.actFail("assign", `{}`, ""); !strings.Contains(errs, "not a story id") {
		t.Errorf("assign no card: %q", errs)
	}
	if errs := h.actFail("follow", `{"container":"999"}`, ""); errs != "shortcut: no story sc-999" {
		t.Errorf("follow 404: %q", errs)
	}
	h.fake.Fail = map[string]int{"PUT /stories/4802": 422}
	if errs := h.actFail("follow", `{"container":"4802"}`, ""); errs != "shortcut: PUT /stories/4802: 422 Unprocessable Entity" {
		t.Errorf("follow 422: %q", errs)
	}
}

func TestActionRefreshDropsCache(t *testing.T) {
	h := newHarness(t)
	h.call("sidebar", `"expand":[]`)
	h.fake.Reset()
	if got := h.act("refresh", `{}`, ""); got != `{"message":"Refreshed from Shortcut"}`+"\n" {
		t.Errorf("refresh = %q", got)
	}
	h.call("sidebar", `"expand":[]`)
	if len(h.fake.Requests()) == 0 {
		t.Error("sidebar after refresh came from the cache")
	}
}

func TestActionSegments(t *testing.T) {
	h := newHarness(t)
	if got := h.act("new-segment", `{}`, "Board work"); !strings.Contains(got, "Added segment Board work, showing every unstarted story") {
		t.Errorf("new-segment = %q", got)
	}
	h.act("new-segment", `{}`, "Board work")
	segs := h.segments()
	if len(segs) != 3 || segs[0].ID != "ready" || segs[1].ID != "board-work" || segs[1].Name != "Board work" ||
		!reflect.DeepEqual(segs[1].Filter, settings.Filter{}) || segs[2].ID != "board-work-2" {
		t.Fatalf("segments = %+v", segs)
	}

	if got := h.act("rename", `{"group":"ready/board-work"}`, "Board"); !strings.Contains(got, "Renamed Board work to Board") {
		t.Errorf("rename = %q", got)
	}
	filter := `{"team":["board"],"project":["30"],"epic":["10"],"labels":["diff"," ","agent"]}`
	if got := h.act("filter", `{"group":"ready/board-work"}`, filter); !strings.Contains(got,
		"Board now shows team board; project 30; epic 10; labels diff, agent") {
		t.Errorf("filter = %q", got)
	}
	if got := h.act("remove", `{"group":"ready/board-work-2"}`, ""); !strings.Contains(got, "Removed segment Board work") {
		t.Errorf("remove = %q", got)
	}
	segs = h.segments()
	want := settings.Segment{ID: "board-work", Name: "Board", Filter: settings.Filter{Team: "board", Project: "30", Epic: "10", Labels: []string{"diff", "agent"}}}
	if len(segs) != 2 || !reflect.DeepEqual(segs[1], want) {
		t.Errorf("segments = %+v", segs)
	}
	// The renamed segment keeps its group id in the tree.
	if got := tree(t, h.call("sidebar", `"expand":[]`)); !strings.Contains(got, "ready/board-work Board 0") {
		t.Errorf("sidebar after edits:\n%s", got)
	}
	// An empty answer clears the filter back to every unstarted story.
	if got := h.act("filter", `{"group":"ready/board-work"}`, `{"team":[],"labels":[]}`); !strings.Contains(got, "Board now shows every unstarted story") {
		t.Errorf("filter cleared = %q", got)
	}
	if segs := h.segments(); !reflect.DeepEqual(segs[1].Filter, settings.Filter{}) {
		t.Errorf("cleared filter = %+v", segs[1].Filter)
	}
	// Removing the last one leaves none, not the default back.
	h.act("remove", `{"group":"ready/ready"}`, "")
	h.act("remove", `{"group":"ready/board-work"}`, "")
	if segs := h.segments(); len(segs) != 0 {
		t.Errorf("segments after removing all = %+v", segs)
	}

	for _, tc := range []struct{ action, target, input, want string }{
		{"new-segment", `{}`, " ", "shortcut: a new segment needs a name"},
		{"rename", `{"group":"ready/nope"}`, "x", `shortcut: rename needs a segment (got "ready/nope")`},
		{"rename", `{"group":"doing"}`, "x", `shortcut: rename needs a segment (got "doing")`},
	} {
		if errs := h.actFail(tc.action, tc.target, tc.input); errs != tc.want {
			t.Errorf("%s: %q, want %q", tc.action, errs, tc.want)
		}
	}
	h.act("new-segment", `{}`, "X")
	if errs := h.actFail("rename", `{"group":"ready/x"}`, ""); errs != "shortcut: a segment needs a name" {
		t.Errorf("rename blank: %q", errs)
	}
	for input, want := range map[string]string{
		"":                      "shortcut: a filter is a JSON object of field ids to lists of choices",
		"label:bug":             "shortcut: a filter is a JSON object of field ids to lists of choices",
		`{"team":["a","b"]}`:    "shortcut: a segment's team is one choice, not 2",
		`{"project":["1","2"]}`: "shortcut: a segment's project is one choice, not 2",
		`{"epic":["1","2"]}`:    "shortcut: a segment's epic is one choice, not 2",
		`{"owner":["me"]}`:      `shortcut: a segment has no filter field "owner"`,
	} {
		if errs := h.actFail("filter", `{"group":"ready/x"}`, input); errs != want {
			t.Errorf("filter %q: %q, want %q", input, errs, want)
		}
	}
	if errs := h.actFail("frobnicate", `{}`, ""); errs != `shortcut: unknown action "frobnicate"` {
		t.Errorf("unknown: %q", errs)
	}
	h.corruptConfig()
	if errs := h.actFail("new-segment", `{}`, "Y"); !strings.Contains(errs, "not valid JSON") {
		t.Errorf("corrupt config: %q", errs)
	}
}

// task is an event's task JSON.
func task(id, title, status, branch, pr string) string {
	return fmt.Sprintf(`{"id":%q,"title":%q,"status":%q,"branch":%q,"pr":%q}`, id, title, status, branch, pr)
}

// event sends an event and returns the writes it caused.
func (h *harness) event(name string, container int64, taskJSON string) string {
	h.t.Helper()
	h.fake.Reset()
	out := h.call("event", fmt.Sprintf(`"container":"%d","task":%s,"event":%q`, container, taskJSON, name))
	if out != "{}\n" {
		h.t.Errorf("event %s stdout = %q", name, out)
	}
	return h.writes()
}

func TestEventCreatedDeleted(t *testing.T) {
	h := newHarness(t)
	tk := task("t1", "Try it", "Todo", "", "")
	if got := h.event("created", fakeshortcut.StoryReady, tk); got != `POST /stories/4802/tasks {"description":"Try it (nat:t1)","complete":false}` {
		t.Errorf("created = %s", got)
	}
	if got := h.event("created", fakeshortcut.StoryReady, tk); got != "" {
		t.Errorf("duplicate created wrote %s", got)
	}
	if got := h.event("created", fakeshortcut.StoryReady, task("t2", " ", "Todo", "", "")); got != `POST /stories/4802/tasks {"description":"nat task (nat:t2)","complete":false}` {
		t.Errorf("untitled created = %s", got)
	}
	id := h.fake.Story(fakeshortcut.StoryReady).Tasks[0].ID
	if got := h.event("deleted", fakeshortcut.StoryReady, tk); got != fmt.Sprintf("DELETE /stories/4802/tasks/%d", id) {
		t.Errorf("deleted = %s", got)
	}
	if got := h.event("deleted", fakeshortcut.StoryReady, tk); got != "" {
		t.Errorf("duplicate deleted wrote %s", got)
	}
	if n := len(h.fake.Story(fakeshortcut.StoryReady).Tasks); n != 1 {
		t.Errorf("tasks left = %d", n)
	}
}

func TestEventClaimed(t *testing.T) {
	h := newHarness(t)
	tk := task("t1", "Try it", "In progress", "", "")
	// Mine already: moved to the first started state by position (In
	// Development, not In Review), owners untouched.
	if got := h.event("claimed", fakeshortcut.StoryReady, tk); got != `PUT /stories/4802 {"workflow_state_id":503}` {
		t.Errorf("claimed = %s", got)
	}
	if got := h.event("claimed", fakeshortcut.StoryReady, tk); got != "" {
		t.Errorf("duplicate claimed wrote %s", got)
	}
	// Nobody's, in Backlog: moved and owned.
	h.fake.Stories[fakeshortcut.StoryBug].WorkflowStateID = fakeshortcut.StateBacklog
	if got := h.event("claimed", fakeshortcut.StoryBug, tk); got != `PUT /stories/4811 {"workflow_state_id":503,"owner_ids":["`+fakeshortcut.MeID+`"]}` {
		t.Errorf("claimed unowned = %s", got)
	}
	// A claim arriving after the merge finds the story done: left alone.
	if got := h.event("claimed", fakeshortcut.StoryDone, tk); got != "" {
		t.Errorf("claim of a done story wrote %s", got)
	}
	// A state no workflow knows: concludes nothing.
	h.fake.Stories[fakeshortcut.StoryReady].WorkflowStateID = 12345
	if got := h.event("claimed", fakeshortcut.StoryReady, tk); got != "" {
		t.Errorf("claim from an unknown state wrote %s", got)
	}

	// The project's override wins, by name or id.
	h.fake.Stories[fakeshortcut.StoryReady].WorkflowStateID = fakeshortcut.StateReady
	h.writeConfig(settings.Project{StartedState: "in review"})
	if got := h.event("claimed", fakeshortcut.StoryReady, tk); got != `PUT /stories/4802 {"workflow_state_id":504}` {
		t.Errorf("claimed with override = %s", got)
	}
	h.fake.Stories[fakeshortcut.StoryReady].WorkflowStateID = fakeshortcut.StateReady
	h.writeConfig(settings.Project{StartedState: "Nope"})
	if errs := h.fail("event", `"container":"4802","task":`+tk+`,"event":"claimed"`); errs != `shortcut: workflow "Engineering" has no state "Nope"` {
		t.Errorf("bad override: %q", errs)
	}
	h.writeConfig(settings.Project{})
	h.fake.Fail = map[string]int{"GET /workflows": 500}
	if errs := h.fail("event", `"container":"4802","task":`+tk+`,"event":"claimed"`); errs != "shortcut: GET /workflows: 500 Internal Server Error" {
		t.Errorf("workflows down: %q", errs)
	}
	h.fake.Fail = nil
	h.corruptConfig()
	if errs := h.fail("event", `"container":"4802","task":`+tk+`,"event":"claimed"`); !strings.Contains(errs, "not valid JSON") {
		t.Errorf("corrupt config: %q", errs)
	}
}

func TestEventHandedBackApproved(t *testing.T) {
	h := newHarness(t)
	tk := task("t1", "Try it", "In progress", "slice/try-it", "")
	if got := h.event("handed_back", fakeshortcut.StoryDoing, tk); got != "POST /stories/4821/comments {\"text\":\"Branch `slice/try-it` is ready for review.\"}" {
		t.Errorf("handed_back = %s", got)
	}
	if got := h.event("handed_back", fakeshortcut.StoryDoing, tk); got != "" {
		t.Errorf("duplicate handed_back wrote %s", got)
	}
	// A later comment in between: the next hand-back of the branch says so again.
	h.act("comment", `{"container":"4821"}`, "reworking")
	h.fake.Stories[fakeshortcut.StoryDoing].Comments[len(h.fake.Stories[fakeshortcut.StoryDoing].Comments)-1].CreatedAt =
		shortcut.Time{Time: fakeshortcut.SeedNow.Add(1)}
	if got := h.event("handed_back", fakeshortcut.StoryDoing, tk); got == "" {
		t.Error("hand-back after another comment was skipped")
	}
	if got := h.event("handed_back", fakeshortcut.StoryDoing, task("t1", "x", "In progress", "", "")); got != "" {
		t.Errorf("hand-back with no branch wrote %s", got)
	}

	pr := "https://github.com/scratch/app/pull/9"
	ap := task("t1", "Try it", "In progress", "slice/try-it", pr)
	if got := h.event("approved", fakeshortcut.StoryDoing, ap); got != `POST /stories/4821/comments {"text":"Pull request opened: `+pr+`"}` {
		t.Errorf("approved = %s", got)
	}
	// Once per URL, however many comments came since.
	h.act("comment", `{"container":"4821"}`, "nice")
	if got := h.event("approved", fakeshortcut.StoryDoing, ap); got != "" {
		t.Errorf("duplicate approved wrote %s", got)
	}
	if got := h.event("approved", fakeshortcut.StoryDoing, tk); got != "" {
		t.Errorf("approved with no PR wrote %s", got)
	}
}

func TestEventMerged(t *testing.T) {
	h := newHarness(t)
	t1 := task("t1", "One", "Done", "b1", "pr1")
	t2 := task("t2", "Two", "Todo", "", "")
	h.event("created", fakeshortcut.StoryDoing, t1)
	h.event("created", fakeshortcut.StoryDoing, t2)
	tasks := h.fake.Story(fakeshortcut.StoryDoing).Tasks
	id1, id2 := tasks[0].ID, tasks[1].ID

	// t2 still open: t1's task completes, the story stays where it is.
	if got := h.event("merged", fakeshortcut.StoryDoing, t1); got != fmt.Sprintf(`PUT /stories/4821/tasks/%d {"complete":true}`, id1) {
		t.Errorf("merged with another open = %s", got)
	}
	if got := h.event("merged", fakeshortcut.StoryDoing, t1); got != "" {
		t.Errorf("duplicate merged wrote %s", got)
	}
	// The last one: task completes, story to the first done state by
	// position (Done, not Won't Do).
	want := fmt.Sprintf(`PUT /stories/4821/tasks/%d {"complete":true}`, id2) + "\n" + `PUT /stories/4821 {"workflow_state_id":505}`
	if got := h.event("merged", fakeshortcut.StoryDoing, task("t2", "Two", "Done", "", "")); got != want {
		t.Errorf("last merged:\n%s\nwant:\n%s", got, want)
	}
	// Again, the story already done: nothing.
	if got := h.event("merged", fakeshortcut.StoryDoing, t1); got != "" {
		t.Errorf("merged on a done story wrote %s", got)
	}
}

func TestEventMergedOutOfOrder(t *testing.T) {
	h := newHarness(t)
	h.writeConfig(settings.Project{DoneState: fmt.Sprint(fakeshortcut.StateWontDo)})
	// merged before its created ever landed: the story task is made
	// complete, so the late created finds it and adds nothing.
	tk := task("t9", "Late", "Done", "b", "p")
	want := `POST /stories/4802/tasks {"description":"Late (nat:t9)","complete":true}` + "\n" + `PUT /stories/4802 {"workflow_state_id":506}`
	if got := h.event("merged", fakeshortcut.StoryReady, tk); got != want {
		t.Errorf("merged first:\n%s\nwant:\n%s", got, want)
	}
	if got := h.event("created", fakeshortcut.StoryReady, tk); got != "" {
		t.Errorf("late created wrote %s", got)
	}

	// Override naming no state, and a workflow with no done state at all.
	h.fake.Stories[fakeshortcut.StoryBug].Tasks = nil
	h.writeConfig(settings.Project{DoneState: "Shipped"})
	if errs := h.fail("event", `"container":"4811","task":`+tk+`,"event":"merged"`); errs != `shortcut: workflow "Engineering" has no state "Shipped"` {
		t.Errorf("bad override: %q", errs)
	}
	h.writeConfig(settings.Project{})
	// The story task from the failed run above is there now (the create
	// landed before the move failed); this run completes nothing new.
	h.fake.Workflows[0].States = h.fake.Workflows[0].States[:4]
	if errs := h.fail("event", `"container":"4811","task":`+tk+`,"event":"merged"`); errs != `shortcut: workflow "Engineering" has no done state` {
		t.Errorf("no done state: %q", errs)
	}
	// A state no workflow knows: nothing moved.
	h.fake.Stories[fakeshortcut.StoryBug].WorkflowStateID = 12345
	if got := h.event("merged", fakeshortcut.StoryBug, tk); got != "" {
		t.Errorf("unknown state wrote %s", got)
	}
}

func TestEventMergedFailures(t *testing.T) {
	h := newHarness(t)
	tk := task("t1", "One", "Done", "", "")
	h.fake.Fail = map[string]int{"POST /stories/4802/tasks": 500}
	if errs := h.fail("event", `"container":"4802","task":`+tk+`,"event":"merged"`); errs != "shortcut: POST /stories/4802/tasks: 500 Internal Server Error" {
		t.Errorf("create fails: %q", errs)
	}
	h.fake.Fail = nil
	h.event("created", fakeshortcut.StoryReady, tk)
	id := h.fake.Story(fakeshortcut.StoryReady).Tasks[0].ID
	h.fake.Fail = map[string]int{fmt.Sprintf("PUT /stories/4802/tasks/%d", id): 500}
	if errs := h.fail("event", `"container":"4802","task":`+tk+`,"event":"merged"`); !strings.Contains(errs, "500") {
		t.Errorf("complete fails: %q", errs)
	}
	h.fake.Fail = map[string]int{"GET /workflows": 503}
	if errs := h.fail("event", `"container":"4802","task":`+tk+`,"event":"merged"`); !strings.Contains(errs, "GET /workflows: 503") {
		t.Errorf("workflows fail: %q", errs)
	}
	h.fake.Fail = nil
	h.corruptConfig()
	if errs := h.fail("event", `"container":"4802","task":`+tk+`,"event":"merged"`); !strings.Contains(errs, "not valid JSON") {
		t.Errorf("corrupt config: %q", errs)
	}
}

func TestEventIgnoredAndRefused(t *testing.T) {
	h := newHarness(t)
	tk := task("t1", "One", "Todo", "", "")
	for _, name := range []string{"released", "moved", ""} {
		h.fake.Reset()
		h.call("event", `"container":"4802","task":`+tk+`,"event":"`+name+`"`)
		if n := len(h.fake.Requests()); n != 0 {
			t.Errorf("%q event made %d requests", name, n)
		}
	}
	for _, tc := range []struct{ extra, want string }{
		{`"container":"4802","task":{},"event":"created"`, "shortcut: event has no task"},
		{`"container":"x","task":` + tk + `,"event":"created"`, `shortcut: "x" is not a story id`},
		{`"container":"999","task":` + tk + `,"event":"created"`, "shortcut: no story sc-999"},
	} {
		if errs := h.fail("event", tc.extra); errs != tc.want {
			t.Errorf("%s: %q, want %q", tc.extra, errs, tc.want)
		}
	}
}
