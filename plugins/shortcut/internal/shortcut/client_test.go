package shortcut_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/plugins/shortcut/internal/fakeshortcut"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/shortcut"
)

func setup(t *testing.T) (*fakeshortcut.Server, *shortcut.Client) {
	t.Helper()
	fake := fakeshortcut.Seed("tok")
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, shortcut.New(srv.URL+fakeshortcut.Prefix+"/", "tok")
}

func TestNewDefaults(t *testing.T) {
	c := shortcut.New("", "t")
	if c.BaseURL != shortcut.DefaultBaseURL || c.HTTP.Timeout != shortcut.Timeout {
		t.Errorf("New = %+v", c)
	}
}

func TestReads(t *testing.T) {
	fake, c := setup(t)
	ctx := context.Background()
	me, err := c.Me(ctx)
	if err != nil || me.ID != fakeshortcut.MeID || me.Workspace2.URLSlug != "scratch" {
		t.Errorf("Me = %+v, %v", me, err)
	}
	ms, err := c.Members(ctx)
	if err != nil || len(ms) != 3 || ms[1].Profile.Name != "Dana Wolfe" {
		t.Errorf("Members = %+v, %v", ms, err)
	}
	ws, err := c.Workflows(ctx)
	if err != nil || len(ws) != 1 || len(ws[0].States) != 6 {
		t.Errorf("Workflows = %+v, %v", ws, err)
	}
	gs, err := c.Groups(ctx)
	if err != nil || len(gs) != 3 || gs[0].ColorKey != "midnight-blue" || !gs[2].Archived {
		t.Errorf("Groups = %+v, %v", gs, err)
	}
	e, err := c.Epic(ctx, fakeshortcut.EpicParity)
	if err != nil || e.Name != "Native app parity" || e.GroupID != fakeshortcut.TeamBoard {
		t.Errorf("Epic = %+v, %v", e, err)
	}
	it, err := c.Iteration(ctx, fakeshortcut.IterationSprint41)
	if err != nil || it.Name != "Sprint 41" {
		t.Errorf("Iteration = %+v, %v", it, err)
	}
	st, err := c.Story(ctx, fakeshortcut.StoryDoing)
	if err != nil || st.Name != "Improve diff review ergonomics" || *st.Estimate != 3 || len(st.Comments) != 3 ||
		!st.CreatedAt.Equal(fakeshortcut.SeedNow.Add(-15*24*time.Hour)) {
		t.Errorf("Story = %+v, %v", st, err)
	}
	var paths []string
	for _, r := range fake.Requests() {
		paths = append(paths, r.Method+" "+r.Path)
	}
	want := "GET /member,GET /members,GET /workflows,GET /groups,GET /epics/10,GET /iterations/41,GET /stories/4821"
	if got := strings.Join(paths, ","); got != want {
		t.Errorf("requests = %s", got)
	}
}

func TestHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	c := shortcut.New(srv.URL, "secret")
	if err := c.CreateComment(context.Background(), 1, "x"); err != nil {
		t.Fatal(err)
	}
	if got.Get("Shortcut-Token") != "secret" || got.Get("Content-Type") != "application/json" || got.Get("Accept") != "application/json" {
		t.Errorf("headers = %v", got)
	}
	if _, err := c.Me(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got.Get("Content-Type") != "" {
		t.Errorf("GET sent a Content-Type: %v", got)
	}
}

func TestWrites(t *testing.T) {
	fake, c := setup(t)
	ctx := context.Background()
	state := fakeshortcut.StateDone
	steps := []func() error{
		func() error {
			return c.UpdateStory(ctx, fakeshortcut.StoryBug, shortcut.StoryUpdate{WorkflowStateID: &state, OwnerIDs: []string{"a"}})
		},
		func() error {
			return c.UpdateStory(ctx, fakeshortcut.StoryBug, shortcut.StoryUpdate{FollowerIDs: []string{"b"}})
		},
		func() error { return c.CreateComment(ctx, fakeshortcut.StoryBug, "hello") },
		func() error { return c.CreateTask(ctx, fakeshortcut.StoryBug, "Try it (nat:t1)", false) },
		func() error { return c.CompleteTask(ctx, fakeshortcut.StoryBug, 900002) },
		func() error { return c.DeleteTask(ctx, fakeshortcut.StoryBug, 900002) },
	}
	for i, f := range steps {
		if err := f(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	var got []string
	for _, r := range fake.Writes() {
		got = append(got, r.String())
	}
	want := []string{
		`PUT /stories/4811 {"workflow_state_id":505,"owner_ids":["a"]}`,
		`PUT /stories/4811 {"follower_ids":["b"]}`,
		`POST /stories/4811/comments {"text":"hello"}`,
		`POST /stories/4811/tasks {"description":"Try it (nat:t1)","complete":false}`,
		`PUT /stories/4811/tasks/900002 {"complete":true}`,
		`DELETE /stories/4811/tasks/900002`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("writes:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	st := fake.Story(fakeshortcut.StoryBug)
	if st.WorkflowStateID != state || len(st.Tasks) != 0 || len(st.Comments) != 1 || st.CompletedAt.IsZero() {
		t.Errorf("story after writes = %+v", st)
	}
}

func TestSearchPages(t *testing.T) {
	fake, c := setup(t)
	for i := int64(1); i <= 60; i++ {
		fake.Stories[10000+i] = &shortcut.Story{ID: 10000 + i, Name: fmt.Sprintf("bulk %d", i), WorkflowStateID: fakeshortcut.StateReady}
	}
	ctx := context.Background()
	got, total, err := c.Search(ctx, `bulk !is:done`, 100)
	if err != nil || len(got) != 60 || total != 60 {
		t.Fatalf("Search = %d stories, total %d, %v", len(got), total, err)
	}
	reqs := fake.Requests()
	if len(reqs) != 3 {
		t.Fatalf("pages = %d, want 3", len(reqs))
	}
	q := reqs[0].Query
	if q.Get("query") != "bulk !is:done" || q.Get("page_size") != "25" || q.Get("detail") != "slim" {
		t.Errorf("first page query = %v", q)
	}
	if reqs[1].Query.Get("next") != "25" {
		t.Errorf("second page query = %v", reqs[1].Query)
	}

	fake.Reset()
	got, total, err = c.Search(ctx, "bulk", 30)
	if err != nil || len(got) != 30 || total != 60 || len(fake.Requests()) != 2 {
		t.Errorf("Search max 30 = %d stories, total %d, %d requests, %v", len(got), total, len(fake.Requests()), err)
	}
	got, _, err = c.Search(ctx, "bulk", 1)
	if err != nil || len(got) != 1 {
		t.Errorf("Search max 1 = %d, %v", len(got), err)
	}
}

func TestSearchLaterPageFails(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		if page > 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":1},{"id":2}],"next":"/api/v3/search/stories?query=x&next=abc","total":40}`))
	}))
	defer srv.Close()
	c := shortcut.New(srv.URL, "t")
	got, total, err := c.Search(context.Background(), "x", 100)
	if err != nil || len(got) != 2 || total != 40 {
		t.Errorf("Search = %v, %d, %v; want the first page kept", got, total, err)
	}
	if _, _, err := c.Search(context.Background(), "x", 100); err == nil {
		t.Error("failed first page was not an error")
	}
}

func TestSearchNextLinks(t *testing.T) {
	for _, next := range []string{"/api/v3/search/stories", "://bad"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]int{{"id": 1}}, "next": next, "total": 9})
		}))
		got, _, err := shortcut.New(srv.URL, "t").Search(context.Background(), "x", 100)
		srv.Close()
		if err != nil || len(got) != 1 {
			t.Errorf("next %q: Search = %v, %v; want one page and stop", next, got, err)
		}
	}
}

func TestErrors(t *testing.T) {
	fake, c := setup(t)
	ctx := context.Background()

	fake.Fail = map[string]int{"GET /search/stories": 503}
	_, _, err := c.Search(ctx, "owner:me secret-text", 10)
	if err == nil || err.Error() != "shortcut: GET /search/stories: 503 Service Unavailable" {
		t.Errorf("503 = %v", err)
	}
	var se *shortcut.StatusError
	if !errors.As(err, &se) || se.Status != 503 {
		t.Errorf("not a StatusError: %v", err)
	}

	_, err = shortcut.New(c.BaseURL, "wrong").Me(ctx)
	if err == nil || err.Error() != "shortcut: GET /member: 401 Unauthorized" || strings.Contains(err.Error(), "wrong") {
		t.Errorf("401 = %v", err)
	}

	malformed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id": "secret body`))
	}))
	defer malformed.Close()
	_, err = shortcut.New(malformed.URL, "t").Me(ctx)
	if err == nil || err.Error() != "shortcut: GET /member: malformed response" {
		t.Errorf("malformed = %v", err)
	}

	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	_, err = shortcut.New(gone.URL, "t").Me(ctx)
	if err == nil || !strings.HasPrefix(err.Error(), "shortcut: GET /member: unreachable (") {
		t.Errorf("unreachable = %v", err)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer slow.Close()
	sc := shortcut.New(slow.URL, "t")
	sc.HTTP.Timeout = 20 * time.Millisecond
	_, err = sc.Me(ctx)
	if err == nil || err.Error() != "shortcut: GET /member: unreachable (timed out)" {
		t.Errorf("timeout = %v", err)
	}

	_, err = shortcut.New("http://bad host", "t").Me(ctx)
	if err == nil || !strings.HasPrefix(err.Error(), "shortcut: GET /member: ") {
		t.Errorf("bad base = %v", err)
	}

	// A transport that fails with something other than a *url.Error.
	sc = shortcut.New("http://example.invalid", "t")
	sc.HTTP = &http.Client{Transport: failing{}}
	_, err = sc.Me(ctx)
	if err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("transport error = %v", err)
	}
}

type failing struct{}

func (failing) RoundTrip(*http.Request) (*http.Response, error) { return nil, errors.New("no route") }

func TestTime(t *testing.T) {
	var v struct {
		A, B, C, D shortcut.Time
	}
	if err := json.Unmarshal([]byte(`{"A":"2026-10-03T12:00:00Z","B":null,"C":"yesterday","D":42}`), &v); err != nil {
		t.Fatal(err)
	}
	if !v.A.Equal(fakeshortcut.SeedNow) || !v.B.IsZero() || !v.C.IsZero() || !v.D.IsZero() {
		t.Errorf("decoded = %+v", v)
	}
	b, _ := json.Marshal(v)
	if string(b) != `{"A":"2026-10-03T12:00:00Z","B":null,"C":null,"D":null}` {
		t.Errorf("encoded = %s", b)
	}
}
