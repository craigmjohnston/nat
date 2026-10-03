package plugin

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/source"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/settings"
)

// filterOf is a segment's filter action as the sidebar sent it.
func filterOf(t *testing.T, out, group string) source.Action {
	t.Helper()
	var sb sidebarResponse
	if err := json.Unmarshal([]byte(out), &sb); err != nil {
		t.Fatal(err)
	}
	if err := source.ValidateGroups(sb.Groups); err != nil {
		t.Errorf("nat would refuse the sidebar: %v", err)
	}
	for _, g := range sb.Groups {
		if g.ID != group {
			continue
		}
		for _, a := range g.Menu {
			if a.Input == source.InputFilter {
				return a
			}
		}
	}
	t.Fatalf("no filter action on %s in %s", group, out)
	return source.Action{}
}

// fields is a filter's fields as lines: id[multi][loading] = value | options.
func fields(a source.Action) string {
	var lines []string
	for _, f := range a.Fields {
		head := f.ID
		if f.Multi {
			head += " multi"
		}
		if f.Loading {
			head += " loading"
		}
		var opts []string
		for _, o := range f.Options {
			opt := o.ID + ":" + o.Label
			if o.Color != "" {
				opt += o.Color
			}
			opts = append(opts, opt)
		}
		lines = append(lines, head+" = "+strings.Join(f.Value, ",")+" | "+strings.Join(opts, " "))
	}
	return strings.Join(lines, "\n")
}

// A segment's Filter… opens on its own selection, offering the workspace's
// unarchived teams, projects, epics and labels — and a saved choice the
// workspace no longer offers, so a save never drops it.
func TestFilterFields(t *testing.T) {
	h := newHarness(t)
	h.warmEpics()
	h.writeConfig(settings.Project{Segments: []settings.Segment{
		{ID: "r", Name: "Ready", Filter: settings.Filter{Team: "old", Project: "32", Epic: "11", Labels: []string{"agent", "gone"}}},
	}})
	a := filterOf(t, h.call("sidebar", `"expand":[]`), "ready/r")
	if a.ID != "filter" || a.Label != "Filter…" {
		t.Errorf("action = %+v", a)
	}
	want := strings.Join([]string{
		"team = old | board:Board#2aa198 native-app:Native App#2c3e7a old:old",
		"project = 32 | 32:32 30:Mobile App#e5732a 31:Web#8e8e93",
		"epic = 11 | 10:Native app parity 11:Old epic",
		"labels multi = agent,gone | agent:agent diff:diff#d64545 gone:gone",
	}, "\n")
	if got := fields(a); got != want {
		t.Errorf("fields:\n%s\nwant:\n%s", got, want)
	}
	// An empty filter selects nothing, and says so with empty lists.
	h.writeConfig(settings.Project{Segments: []settings.Segment{{ID: "r", Name: "Ready"}}})
	h.now = h.now.Add(time.Minute)
	a = filterOf(t, h.call("sidebar", `"expand":[]`), "ready/r")
	for _, f := range a.Fields {
		if f.Value == nil || len(f.Value) != 0 {
			t.Errorf("%s value = %#v, want []", f.ID, f.Value)
		}
	}
}

// The epic list is never fetched on the sidebar's path. With nothing cached
// the epic field says it is loading, a warm-up is started detached (once a
// minute at most), and the sidebar is not cached — so the next read, once the
// warm-up has landed the list, offers it.
func TestEpicsWarmInTheBackground(t *testing.T) {
	h := newHarness(t)
	first := h.call("sidebar", `"expand":[]`)
	if f := filterOf(t, first, "ready/ready").Fields[2]; f.ID != "epic" || !f.Loading || len(f.Options) != 0 {
		t.Errorf("epic field before the warm-up = %+v", f)
	}
	if !slices.Equal(h.spawned, []string{"warm"}) {
		t.Errorf("spawned %v, want one warm-up", h.spawned)
	}
	if h.gotten("/epics") != 0 {
		t.Error("the sidebar fetched the epic list itself")
	}
	// Read again inside the minute: not served from the cache, and no second
	// warm-up while the first is under way.
	h.fake.Reset()
	h.call("sidebar", `"expand":[]`)
	if len(h.fake.Requests()) == 0 || len(h.spawned) != 1 {
		t.Errorf("second read: %d requests, spawned %v", len(h.fake.Requests()), h.spawned)
	}

	// The warm-up lands the list, through the slim read.
	if code, _, _ := h.run("", "warm"); code != 0 {
		t.Fatalf("warm: exit %d", code)
	}
	for _, r := range h.fake.Requests() {
		if r.Path == "/epics" && r.Query.Get("includes_description") != "false" {
			t.Errorf("epic list read with %v", r.Query)
		}
	}
	h.fake.Reset()
	f := filterOf(t, h.call("sidebar", `"expand":[]`), "ready/ready").Fields[2]
	if f.Loading || len(f.Options) != 1 || f.Options[0].Label != "Native app parity" {
		t.Errorf("epic field after the warm-up = %+v", f)
	}
	// Now cached, the tree is too.
	h.fake.Reset()
	h.call("sidebar", `"expand":[]`)
	if len(h.fake.Requests()) != 0 {
		t.Errorf("a sidebar with the list in hand was not cached: %d requests", len(h.fake.Requests()))
	}

	// Past the hour the stale list is offered meanwhile and a refresh started.
	h.now = h.now.Add(2 * time.Hour)
	h.spawned = nil
	f = filterOf(t, h.call("sidebar", `"expand":[]`), "ready/ready").Fields[2]
	if f.Loading || len(f.Options) != 1 || !slices.Equal(h.spawned, []string{"warm"}) {
		t.Errorf("stale: field %+v, spawned %v", f, h.spawned)
	}
}

// A warm-up that cannot start marks nothing, so the next sidebar tries again;
// with no way to start one at all, nothing is tried.
func TestEpicsWarmStartFailures(t *testing.T) {
	h := newHarness(t)
	h.spawnErr = errors.New("no fork")
	h.call("sidebar", `"expand":[]`)
	h.call("sidebar", `"expand":[]`)
	if len(h.spawned) != 2 {
		t.Errorf("spawned %v, want a retry after a failed start", h.spawned)
	}
	env := h.env()
	env.Spawn = nil
	var out, errb strings.Builder
	if code := Run([]string{"x", "sidebar"}, strings.NewReader(req(`"expand":[]`)), &out, &errb, env); code != 0 {
		t.Errorf("sidebar with no Spawn: exit %d, %q", code, errb.String())
	}
}

// The warm-up says nothing whatever happens, and caches nothing it did not
// read: no token, Shortcut down.
func TestWarmFailures(t *testing.T) {
	h := newHarness(t)
	h.fake.Fail = map[string]int{"GET /epics": 500}
	if code, out, errs := h.run("", "warm"); code != 1 || out != "" || errs != "" {
		t.Errorf("Shortcut down: exit %d, %q, %q", code, out, errs)
	}
	h.fake.Fail = nil
	h.tokens.token, h.tokens.err = "", errors.New("none")
	if code, out, errs := h.run("", "warm"); code != 1 || out != "" || errs != "" {
		t.Errorf("no token: exit %d, %q, %q", code, out, errs)
	}
	if h.gotten("/epics") != 1 {
		t.Errorf("epic list reads = %d, want only the failed one", h.gotten("/epics"))
	}
	// SHORTCUT_API_TOKEN serves as the token, as for every method; the
	// override client is used where one is given.
	h.vars["SHORTCUT_API_TOKEN"] = secret
	env := h.env()
	env.HTTP = &httpClientRefusing
	var out, errb strings.Builder
	if code := Run([]string{"x", "warm"}, strings.NewReader(""), &out, &errb, env); code != 1 {
		t.Errorf("refusing client: exit %d", code)
	}
	if code, _, _ := h.run("", "warm"); code != 0 {
		t.Errorf("env token: exit %d", code)
	}
}

// The section header's own Filter… narrows every search the sidebar makes;
// a segment's filter overrides it field by field (labels as a whole), and its
// editor names what each "Any" falls through to.
func TestSectionFilter(t *testing.T) {
	h := newHarness(t)
	h.warmEpics()
	var sb sidebarResponse
	_ = json.Unmarshal([]byte(h.call("sidebar", `"expand":[]`)), &sb)
	if got := actions(sb.Menu); got != "refresh(none) new-segment(text) filter(filter)" {
		t.Errorf("section menu = %s", got)
	}

	if got := h.act("filter", `{}`, `{"project":["30"],"labels":["diff"]}`); !strings.Contains(got, "Every Shortcut list now shows project 30; labels diff") {
		t.Errorf("section filter = %q", got)
	}
	if got := h.act("filter", `{"group":"ready/ready"}`, `{"labels":["agent"],"team":["board"]}`); !strings.Contains(got, "Ready now shows team board; labels agent") {
		t.Errorf("segment filter = %q", got)
	}
	h.fake.Reset()
	out := h.call("sidebar", `"expand":[]`)
	for _, q := range []string{
		"/search/stories project:30 label:\"diff\" owner:craig is:started",
		"/search/stories project:30 label:\"diff\" " + doneThisWeek,
		// The segment's team and labels replace the section's; its project
		// falls through.
		"/search/stories team:board project:30 label:\"agent\" !is:done",
	} {
		if !slices.Contains(h.gets(), q) {
			t.Errorf("no search %q in %q", q, h.gets())
		}
	}
	_ = json.Unmarshal([]byte(out), &sb)
	section := sb.Menu[len(sb.Menu)-1]
	if got := fields(section); !strings.HasPrefix(got, "team =  |") || !strings.Contains(got, "project = 30 |") {
		t.Errorf("section fields:\n%s", got)
	}
	for _, f := range section.Fields {
		if f.Inherited != "" {
			t.Errorf("the section's own %s inherits %q", f.ID, f.Inherited)
		}
	}
	inherited := map[string]string{}
	for _, f := range filterOf(t, out, "ready/ready").Fields {
		inherited[f.ID] = f.Inherited
	}
	if want := map[string]string{"team": "", "project": "Mobile App", "epic": "", "labels": "diff"}; !reflect.DeepEqual(inherited, want) {
		t.Errorf("inherited = %v, want %v", inherited, want)
	}

	// The section's epic is searched by name and named in what falls through.
	h.act("filter", `{}`, `{"epic":["10"],"labels":["gone"]}`)
	out = h.call("sidebar", `"expand":[]`)
	for _, f := range filterOf(t, out, "ready/ready").Fields {
		if (f.ID == "epic" && f.Inherited != "Native app parity") || (f.ID == "labels" && f.Inherited != "gone") {
			t.Errorf("%s inherits %q", f.ID, f.Inherited)
		}
	}
	if !slices.Contains(h.gets(), `/search/stories epic:"Native app parity" label:"gone" owner:craig is:started`) {
		t.Errorf("searches = %q", h.gets())
	}
	if got := h.act("filter", `{}`, `{}`); !strings.Contains(got, "Every Shortcut list now shows everything") {
		t.Errorf("cleared = %q", got)
	}
	if errs := h.actFail("filter", `{}`, `nope`); !strings.Contains(errs, "a filter is a JSON object") {
		t.Errorf("bad section filter: %q", errs)
	}
}

// An epic that cannot be looked up is searched, and offered, by its id.
func TestUnknownEpicFallsBackToItsID(t *testing.T) {
	h := newHarness(t)
	h.writeConfig(settings.Project{Segments: []settings.Segment{{ID: "e", Name: "Gone", Filter: settings.Filter{Epic: "999"}}}})
	out := h.call("sidebar", `"expand":[]`)
	if !slices.Contains(h.gets(), `/search/stories epic:"999" !is:done`) {
		t.Errorf("searches = %q", h.gets())
	}
	if f := filterOf(t, out, "ready/e").Fields[2]; len(f.Options) != 1 || f.Options[0] != (source.FilterOption{ID: "999", Label: "999"}) {
		t.Errorf("epic field = %+v", f)
	}
}
