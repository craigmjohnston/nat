package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/source"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/fakeshortcut"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/settings"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/shortcut"
)

type refusing struct{}

func (refusing) RoundTrip(*http.Request) (*http.Response, error) { return nil, errors.New("refused") }

var httpClientRefusing = http.Client{Transport: refusing{}}

func TestDescribe(t *testing.T) {
	h := newHarness(t)
	// source-list describes with an empty project.
	code, out, errs := h.run(`{"project":{"id":"","name":"","working_dir":""}}`, "describe")
	if code != 0 {
		t.Fatalf("describe: exit %d, %q", code, errs)
	}
	var d source.Describe
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatal(err)
	}
	if d.Protocol != source.ProtocolVersion {
		t.Errorf("protocol = %d, nat speaks %d", d.Protocol, source.ProtocolVersion)
	}
	if err := source.ValidateDescribe(d); err != nil {
		t.Errorf("nat would refuse describe: %v", err)
	}
	if d.Name != "shortcut" || d.Title != "Shortcut" || d.Tag != "SC" || d.IconSymbol != "rectangle.on.rectangle.angled" ||
		d.ContainerNoun != "card" || d.TaskNoun != "task" {
		t.Errorf("describe = %+v", d)
	}
	if len(d.IconSVG) > 8<<10 || !strings.HasPrefix(d.IconSVG, "<svg") || !strings.Contains(d.IconSVG, `fill="currentColor"`) {
		t.Errorf("icon_svg = %q", d.IconSVG)
	}
	if got := actions(d.Menu); got != "refresh(none) new-segment(text)" {
		t.Errorf("menu = %s", got)
	}
	if len(d.Setup) != 1 || d.Setup[0].ID != "token" || d.Setup[0].Label != "API token" || d.Setup[0].Input != source.InputSecret ||
		d.Setup[0].Hint != "Shortcut ▸ Settings ▸ API Tokens" || d.Setup[0].Set == nil || !*d.Setup[0].Set {
		t.Errorf("setup = %+v, want the token, set", d.Setup)
	}
	if !slices.Equal(h.tokens.asked, []string{"craig"}) {
		t.Errorf("asked the Keychain after %v, want craig", h.tokens.asked)
	}
	if len(h.fake.Requests()) != 0 {
		t.Error("describe called Shortcut")
	}

	// With no token anywhere — the lookup failing — describe still answers,
	// the same but for set, so nat learns a token is wanted.
	h.tokens.err, h.tokens.token = errors.New("not found"), ""
	setOf := func() string {
		t.Helper()
		code, out, errs := h.run(`{"project":{"id":""}}`, "describe")
		var d source.Describe
		if code != 0 || json.Unmarshal([]byte(out), &d) != nil || len(d.Setup) != 1 || d.Setup[0].Set == nil {
			t.Fatalf("describe: exit %d, %q, stderr %q", code, out, errs)
		}
		return strconv.FormatBool(*d.Setup[0].Set)
	}
	if got := setOf(); got != "false" {
		t.Errorf("set with no token = %s", got)
	}
	// SHORTCUT_API_TOKEN counts as one.
	h.vars["SHORTCUT_API_TOKEN"] = secret
	if got := setOf(); got != "true" {
		t.Errorf("set with SHORTCUT_API_TOKEN = %s", got)
	}
}

func actions(as []source.Action) string {
	var parts []string
	for _, a := range as {
		s := a.ID + "(" + a.Input
		if a.Destructive {
			s += ",destructive"
		}
		parts = append(parts, s+")")
	}
	return strings.Join(parts, " ")
}

// tree is a sidebar as lines: "id label count [lazy]: container…", children
// indented, each container as id[badge|meta].
func tree(t *testing.T, out string) string {
	t.Helper()
	var sb sidebarResponse
	if err := json.Unmarshal([]byte(out), &sb); err != nil {
		t.Fatal(err)
	}
	if err := source.ValidateGroups(sb.Groups); err != nil {
		t.Errorf("nat would refuse the sidebar: %v", err)
	}
	var lines []string
	var walk func(gs []source.Group, indent string)
	walk = func(gs []source.Group, indent string) {
		for _, g := range gs {
			line := fmt.Sprintf("%s%s %s %d", indent, g.ID, g.Label, *g.Count)
			if g.Lazy {
				line += " lazy"
			}
			if len(g.Menu) > 0 {
				line += " {" + actions(g.Menu) + "}"
			}
			line += ":"
			for _, c := range g.Containers {
				var badges []string
				for _, b := range c.Badges {
					badges = append(badges, b.Text+" "+b.Color+" "+b.Title)
				}
				line += fmt.Sprintf(" %s[%s|%s]", c.ID, strings.Join(badges, ","), c.Meta)
				if actions(c.Menu) != "assign(none) follow(none)" || c.ExternalURL == "" || c.Title == "" {
					t.Errorf("container %s = %+v", c.ID, c)
				}
			}
			lines = append(lines, line)
			walk(g.Children, indent+"  ")
		}
	}
	walk(sb.Groups, "")
	return strings.Join(lines, "\n")
}

const segMenu = "{rename(text) edit-query(text) remove(none,destructive)}"

func TestSidebarDefault(t *testing.T) {
	h := newHarness(t)
	got := tree(t, h.call("sidebar", `"expand":[]`))
	want := strings.Join([]string{
		"doing Doing 1: 4821[NA #2c3e7a Native App|3 pts]",
		"ready Ready 1:",
		"  ready/mine Mine 1 " + segMenu + ": 4802[BO #2aa198 Board|1 pt]",
		"done Done 1 lazy:",
	}, "\n")
	if got != want {
		t.Errorf("sidebar:\n%s\nwant:\n%s", got, want)
	}
	// owner:me goes out as the token's mention name; no team-less story,
	// so no epic is looked up.
	wantGets := []string{
		"/groups", "/member",
		"/search/stories owner:craig !is:done", "/search/stories owner:craig is:done", "/search/stories owner:craig is:started",
		"/workflows",
	}
	if g := h.gets(); !slices.Equal(g, wantGets) {
		t.Errorf("requests = %q", g)
	}
	for _, r := range h.fake.Requests() {
		if r.Query.Get("query") == "owner:craig is:done" && r.Query.Get("page_size") != "25" {
			t.Errorf("done count query = %v", r.Query)
		}
	}
}

func TestSidebarDoneExpanded(t *testing.T) {
	h := newHarness(t)
	// More done stories than Done shows, completed in a scrambled order.
	for i := int64(1); i <= 30; i++ {
		h.fake.Stories[7000+i] = &shortcut.Story{
			ID: 7000 + i, Name: fmt.Sprintf("done %d", i), AppURL: fmt.Sprintf("https://app.shortcut.com/scratch/story/%d", 7000+i),
			WorkflowStateID: fakeshortcut.StateDone, OwnerIDs: []string{fakeshortcut.MeID},
			CompletedAt: shortcut.Time{Time: fakeshortcut.SeedNow.Add(-time.Duration((i*7)%31) * time.Hour)},
		}
	}
	var sb sidebarResponse
	_ = json.Unmarshal([]byte(h.call("sidebar", `"expand":["done"]`)), &sb)
	done := sb.Groups[2]
	if *done.Count != 31 || len(done.Containers) != doneShow {
		t.Fatalf("done = count %d, %d shown", *done.Count, len(done.Containers))
	}
	// Most recently completed first: i*7 mod 31 == 0 is i=31 (absent), so
	// the newest is the one with the smallest offset, 1h (i=9).
	if done.Containers[0].ID != "7009" {
		t.Errorf("newest = %s", done.Containers[0].ID)
	}
	completed := func(id string) time.Time {
		var n int64
		_, _ = fmt.Sscan(id, &n)
		return h.fake.Stories[n].CompletedAt.Time
	}
	for i := 1; i < len(done.Containers); i++ {
		if completed(done.Containers[i].ID).After(completed(done.Containers[i-1].ID)) {
			t.Errorf("done not newest-first at %d", i)
		}
	}
	// The 7-day-old seeded story is older than every bulk one: not shown.
	for _, c := range done.Containers {
		if c.ID == "4756" {
			t.Error("the oldest done story made the 25")
		}
	}
}

func TestSidebarSegmentsAndTeam(t *testing.T) {
	h := newHarness(t)
	h.writeConfig(settings.Project{Segments: []settings.Segment{
		{ID: "bugs", Name: "Bugs", Query: "type:bug"},
		{ID: "all", Name: "Everything", Query: "Kanban"},
	}})
	got := tree(t, h.call("sidebar", `"expand":[]`))
	want := strings.Join([]string{
		"doing Doing 1: 4821[NA #2c3e7a Native App|3 pts]",
		"ready Ready 2:",
		"  ready/bugs Bugs 1 " + segMenu + ": 4811[NAP #2aa198 Native app parity|]",
		"  ready/all Everything 1 " + segMenu + ": 4802[BO #2aa198 Board|1 pt]",
		"done Done 1 lazy:",
	}, "\n")
	if got != want {
		t.Errorf("sidebar:\n%s\nwant:\n%s", got, want)
	}

	h.writeConfig(settings.Project{Team: "Native App", Segments: []settings.Segment{}})
	h.fake.Reset()
	h.now = h.now.Add(time.Minute) // past the cache
	got = tree(t, h.call("sidebar", `"expand":[]`))
	if got != "doing Doing 1: 4821[NA #2c3e7a Native App|3 pts]\nready Ready 0:\ndone Done 1 lazy:" {
		t.Errorf("team-filtered sidebar:\n%s", got)
	}
	for _, g := range h.gets() {
		if strings.HasPrefix(g, "/search") && !strings.HasSuffix(g, ` team:"Native App"`) {
			t.Errorf("search without the team filter: %q", g)
		}
	}
}

func TestSidebarPositionOrder(t *testing.T) {
	h := newHarness(t)
	h.fake.Stories[4900] = &shortcut.Story{ID: 4900, Name: "first", AppURL: "u", WorkflowStateID: fakeshortcut.StateInReview,
		OwnerIDs: []string{fakeshortcut.MeID}, Position: 1}
	got := tree(t, h.call("sidebar", `"expand":[]`))
	if !strings.HasPrefix(got, "doing Doing 2: 4900[|] 4821[") {
		t.Errorf("doing not in position order:\n%s", got)
	}
}

func TestSidebarCache(t *testing.T) {
	h := newHarness(t)
	first := h.call("sidebar", `"expand":[]`)
	h.fake.Reset()

	// Within the TTL: served from the cache, Shortcut not asked.
	h.now = h.now.Add(29 * time.Second)
	if got := h.call("sidebar", `"expand":[]`); got != first || len(h.fake.Requests()) != 0 {
		t.Errorf("cached sidebar: same=%v, %d requests", got == first, len(h.fake.Requests()))
	}
	// A different expand is a different entry.
	h.call("sidebar", `"expand":["done"]`)
	if len(h.fake.Requests()) == 0 {
		t.Error("expand=[done] was served the closed tree's cache")
	}

	// Past the TTL with Shortcut down: the stale tree, silently.
	h.now = h.now.Add(time.Minute)
	h.fake.Fail = map[string]int{"GET /search/stories": 503}
	code, out, errs := h.run(req(`"expand":[]`), "sidebar")
	if code != 0 || out != first || errs != "" {
		t.Errorf("stale on error: exit %d, same=%v, stderr %q", code, out == first, errs)
	}

	// Nothing cached and Shortcut down: the error.
	h.vars["XDG_CACHE_HOME"] = h.dir + "/empty-cache"
	if errs := h.fail("sidebar", `"expand":[]`); errs != "shortcut: GET /search/stories: 503 Service Unavailable" {
		t.Errorf("no cache: %q", errs)
	}
	// A missing expand reads as empty.
	h.fake.Fail = nil
	h.call("sidebar", "")
}

func TestSidebarBadConfig(t *testing.T) {
	h := newHarness(t)
	h.corruptConfig()
	if errs := h.fail("sidebar", `"expand":[]`); !strings.Contains(errs, "not valid JSON") {
		t.Errorf("corrupt config: %q", errs)
	}
}

// facts is a container's facts as "label=value" lines, "(#color)" appended
// where set.
func facts(fs []source.Fact) string {
	var lines []string
	for _, f := range fs {
		l := f.Label + "=" + f.Value
		if f.Color != "" {
			l += " (" + f.Color + ")"
		}
		lines = append(lines, l)
	}
	return strings.Join(lines, "\n")
}

func detail(t *testing.T, out string) source.ContainerDetail {
	t.Helper()
	var d source.ContainerDetail
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatal(err)
	}
	if err := source.ValidateContainer(d); err != nil {
		t.Errorf("nat would refuse the container: %v", err)
	}
	return d
}

func TestContainer(t *testing.T) {
	h := newHarness(t)
	h.fake.Stories[fakeshortcut.StoryDoing].OwnerIDs = []string{fakeshortcut.MeID, fakeshortcut.PriyaID}
	d := detail(t, h.call("container", `"id":"4821"`))
	if d.ID != "4821" || d.Title != "Improve diff review ergonomics" || d.ExternalURL != "https://app.shortcut.com/scratch/story/4821" {
		t.Errorf("head = %+v", d)
	}
	want := strings.Join([]string{
		"id=sc-4821",
		"team=NA · Native App (#2c3e7a)",
		"state=In Development",
		"type=feature",
		"epic=Native app parity",
		"labels=diff, agent",
		"owners=Craig Scratch, Priya Deol",
		"requester=Dana Wolfe",
		"created=18 Sep",
		"updated=2h ago",
		"iteration=Sprint 41",
	}, "\n")
	if got := facts(d.Facts); got != want {
		t.Errorf("facts:\n%s\nwant:\n%s", got, want)
	}
	if len(d.Sections) != 3 {
		t.Fatalf("sections = %+v", d.Sections)
	}
	story, comments, links := d.Sections[0], d.Sections[1], d.Sections[2]
	if story.ID != "story" || story.Kind != "prose" || !strings.HasPrefix(story.Body, "Comments left on a diff") {
		t.Errorf("story = %+v", story)
	}
	if comments.Kind != "comments" || comments.Composer == nil || actions([]source.Action{*comments.Composer}) != "comment(text)" || comments.Composer.Label != "Comment" {
		t.Errorf("comments = %+v", comments)
	}
	wantComments := []source.Comment{
		{By: "Dana Wolfe", When: "3d ago", Text: "Pairs with the syntax highlighting work."},
		{By: "Craig Scratch", When: "2d ago", Text: "Agreed. Splitting into three tasks."},
	}
	if !slices.Equal(comments.Comments, wantComments) {
		t.Errorf("comments = %+v", comments.Comments)
	}
	wantLinks := []source.Link{
		{Label: "PR #418", Text: "Diff comments reach the agent", State: "open", URL: "https://github.com/scratch/app/pull/418"},
		{Label: "PR #416", Text: "Syntax highlighting", State: "merged", URL: "https://github.com/scratch/app/pull/416"},
		{Label: "Branch", Text: "slice/diff-comments", State: "open", URL: "https://github.com/scratch/app/tree/slice/diff-comments"},
		{Label: "Branch", Text: "old-spike", State: "merged", URL: "https://github.com/scratch/app/tree/old-spike"},
		{Label: "blocks", Text: "sc-4802", URL: "https://app.shortcut.com/scratch/story/4802"},
		{Label: "blocked by", Text: "sc-4811", URL: "https://app.shortcut.com/scratch/story/4811"},
		{Label: "relates to", Text: "sc-4756", URL: "https://app.shortcut.com/scratch/story/4756"},
		{Label: "Link", Text: "www.figma.com/file/scratch/review-pane", URL: "https://www.figma.com/file/scratch/review-pane"},
	}
	if !slices.Equal(links.Links, wantLinks) {
		t.Errorf("links:\n%+v\nwant:\n%+v", links.Links, wantLinks)
	}
	if actions(d.Menu) != "assign(none) follow(none)" {
		t.Errorf("menu = %s", actions(d.Menu))
	}
	if d.TaskNote != "Linked to sc-4821. Merging moves the card to Done when it's the last open task." {
		t.Errorf("task_note = %q", d.TaskNote)
	}
}

func TestContainerSparse(t *testing.T) {
	h := newHarness(t)
	st := h.fake.Stories[fakeshortcut.StoryBug]
	st.Comments = []shortcut.Comment{{ID: 1, AuthorID: "gone", Text: "hi", CreatedAt: shortcut.Time{Time: fakeshortcut.SeedNow.Add(-400 * 24 * time.Hour)}}}
	st.StoryLinks = []shortcut.StoryLink{{SubjectID: 1, ObjectID: StoryOther, Verb: "duplicates", Type: "object"}}
	st.AppURL = "not-a-story-url"
	st.Branches = []shortcut.Branch{{Name: "gone", Deleted: true}}
	st.PullRequests = []shortcut.PullRequest{{ID: 5, Number: 5, Closed: true}}
	d := detail(t, h.call("container", `"id":"4811"`))
	want := strings.Join([]string{
		"id=sc-4811", "team=—", "state=Ready for Dev", "type=bug", "epic=Native app parity", "labels=—",
		"owner=unassigned", "requester=—", "created=16 Sep", "updated=6d ago", "iteration=—",
	}, "\n")
	if got := facts(d.Facts); got != want {
		t.Errorf("facts:\n%s\nwant:\n%s", got, want)
	}
	if c := d.Sections[1].Comments; len(c) != 1 || c[0].By != "someone" || c[0].When != "29 Aug 2025" {
		t.Errorf("comments = %+v", c)
	}
	wantLinks := []source.Link{
		{Label: "PR #5", State: "closed"},
		{Label: "duplicated by", Text: "sc-1"},
	}
	if !slices.Equal(d.Sections[2].Links, wantLinks) {
		t.Errorf("links = %+v", d.Sections[2].Links)
	}
}

const StoryOther int64 = 99

func TestContainerErrors(t *testing.T) {
	h := newHarness(t)
	for _, id := range []string{"", "abc", "-4", "0"} {
		if errs := h.fail("container", fmt.Sprintf(`"id":%q`, id)); !strings.Contains(errs, "is not a story id") {
			t.Errorf("id %q: %q", id, errs)
		}
	}
	if errs := h.fail("container", `"id":"999"`); errs != "shortcut: no story sc-999" {
		t.Errorf("404: %q", errs)
	}
	h.fake.Fail = map[string]int{"GET /members": 500}
	if errs := h.fail("container", `"id":"4821"`); errs != "shortcut: GET /members: 500 Internal Server Error" {
		t.Errorf("500: %q", errs)
	}
	// Cached, then Shortcut down past the TTL: the stale detail.
	h.fake.Fail = nil
	first := h.call("container", `"id":"4821"`)
	h.now = h.now.Add(time.Hour)
	h.fake.Fail = map[string]int{"GET /stories/4821": 502}
	if got := h.call("container", `"id":"4821"`); got != first {
		t.Error("stale container not served")
	}
}

// The Env's HTTP client is the one every request goes through.
func TestHTTPOverride(t *testing.T) {
	h := newHarness(t)
	env := h.env()
	env.HTTP = &httpClientRefusing
	var out, errb strings.Builder
	code := Run([]string{"x", "container"}, strings.NewReader(req(`"id":"4821"`)), &out, &errb, env)
	if code != 1 || !strings.Contains(errb.String(), "unreachable (refused)") {
		t.Errorf("exit %d, stderr %q", code, errb.String())
	}
}

// count of GETs of path since the last reset.
func (h *harness) gotten(path string) int {
	n := 0
	for _, r := range h.fake.Requests() {
		if r.Method == "GET" && r.Path == path {
			n++
		}
	}
	return n
}

func factValue(d source.ContainerDetail, label string) string {
	for _, f := range d.Facts {
		if f.Label == label {
			return f.Value
		}
	}
	return ""
}

func TestReferenceLookups(t *testing.T) {
	h := newHarness(t)
	h.call("container", `"id":"4821"`)
	if h.gotten("/epics/10") != 1 || h.gotten("/iterations/41") != 1 {
		t.Fatalf("first read: %v", h.gets())
	}
	// Past the 30 s response cache but inside the hour: the story is read
	// again, its epic and iteration are not.
	h.fake.Reset()
	h.now = h.now.Add(time.Minute)
	h.call("container", `"id":"4821"`)
	if h.gotten("/stories/4821") != 1 || h.gotten("/epics/10") != 0 || h.gotten("/iterations/41") != 0 {
		t.Errorf("within the hour: %v", h.gets())
	}
	// Past the hour with the epic failing: the stale name stands.
	h.now = h.now.Add(2 * time.Hour)
	h.fake.Fail = map[string]int{"GET /epics/10": 500}
	if d := detail(t, h.call("container", `"id":"4821"`)); factValue(d, "epic") != "Native app parity" {
		t.Errorf("stale epic = %q", factValue(d, "epic"))
	}
	// Nothing cached and the epic failing: an empty fact, not a failure.
	h.vars["XDG_CACHE_HOME"] = h.dir + "/cache2"
	if d := detail(t, h.call("container", `"id":"4821"`)); factValue(d, "epic") != "—" || factValue(d, "iteration") != "Sprint 41" {
		t.Errorf("failed epic: epic %q, iteration %q", factValue(d, "epic"), factValue(d, "iteration"))
	}
	// Refresh drops the lookups too.
	h.fake.Fail = nil
	h.call("container", `"id":"4821"`)
	h.act("refresh", `{}`, "")
	h.fake.Reset()
	h.call("container", `"id":"4821"`)
	if h.gotten("/iterations/41") != 1 {
		t.Errorf("after refresh: %v", h.gets())
	}
}

func TestSidebarMemberFails(t *testing.T) {
	h := newHarness(t)
	h.fake.Fail = map[string]int{"GET /member": 401}
	if errs := h.fail("sidebar", `"expand":[]`); errs != "shortcut: GET /member: 401 Unauthorized" {
		t.Errorf("member fails: %q", errs)
	}
}
