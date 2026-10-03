package fakeshortcut

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/plugins/shortcut/internal/shortcut"
)

// do sends one request to s and returns the status and body.
func do(t *testing.T, s *Server, method, path, body string) (int, string) {
	t.Helper()
	r := httptest.NewRequest(method, Prefix+path, strings.NewReader(body))
	r.Header.Set("Shortcut-Token", s.Token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	b, _ := io.ReadAll(w.Result().Body)
	return w.Code, string(b)
}

func search(t *testing.T, s *Server, q string) []int64 {
	t.Helper()
	code, body := do(t, s, "GET", "/search/stories?query="+strings.ReplaceAll(q, " ", "+"), "")
	var res shortcut.SearchResult
	if code != 200 || json.Unmarshal([]byte(body), &res) != nil {
		t.Fatalf("search %q: %d %s", q, code, body)
	}
	var ids []int64
	for _, st := range res.Data {
		if st.Description != "" || st.Comments != nil {
			t.Errorf("search result %d isn't slim", st.ID)
		}
		ids = append(ids, st.ID)
	}
	return ids
}

func TestSearchOperators(t *testing.T) {
	s := Seed("t")
	s.Stories[StoryBug].Labels = []shortcut.Label{{Name: "sidebar"}}
	s.Stories[StoryBug].OwnerIDs = []string{"ghost", DanaID}
	for q, want := range map[string][]int64{
		"owner:craig":              {StoryDone, StoryReady, StoryDoing},
		"owner:me":                 nil,
		"owner:dana":               {StoryBug},
		"owner:craig !is:done":     {StoryReady, StoryDoing},
		"owner:craig -is:started":  {StoryDone, StoryReady},
		`state:"ready for dev"`:    {StoryReady, StoryBug},
		"type:bug":                 {StoryBug},
		"label:sidebar":            {StoryBug},
		"team:board":               {StoryReady},
		`team:"Native App"`:        {StoryDone, StoryDoing},
		"epic:10":                  {StoryBug, StoryDoing},
		`epic:"Native app parity"`: {StoryBug, StoryDoing},
		"kanban":                   {StoryReady},
		"nonsense:op":              nil,
		"  is:done  ":              {StoryDone},
	} {
		if got := search(t, s, q); !slices.Equal(got, want) {
			t.Errorf("%q = %v, want %v", q, got, want)
		}
	}
	// Pages: next carries the offset onward.
	code, body := do(t, s, "GET", "/search/stories?query=owner:craig&page_size=2", "")
	var res shortcut.SearchResult
	_ = json.Unmarshal([]byte(body), &res)
	if code != 200 || len(res.Data) != 2 || res.Total != 3 || !strings.Contains(res.Next, "next=2") {
		t.Errorf("page 1 = %d %+v", code, res)
	}
	if got := search(t, s, "owner:nobody"); got != nil {
		t.Errorf("unknown owner = %v", got)
	}
	s.Stories[StoryBug].WorkflowStateID = 1
	if got := search(t, s, "is:"); !slices.Equal(got, []int64{StoryBug}) {
		t.Errorf("unknown state = %v", got)
	}
	// No page size: 25.
	code, _ = do(t, s, "GET", "/search/stories?query=x&next=99", "")
	if code != 200 {
		t.Errorf("past the end: %d", code)
	}
}

func TestRoutesAndFailures(t *testing.T) {
	s := Seed("t")
	var log strings.Builder
	s.Log = &log
	for _, p := range []string{"/member", "/members", "/workflows", "/groups", "/epics/10", "/iterations/41", "/stories/4821"} {
		if code, _ := do(t, s, "GET", p, ""); code != 200 {
			t.Errorf("GET %s = %d", p, code)
		}
	}
	for _, tc := range []struct {
		method, path, body string
		want               int
	}{
		{"GET", "/nope", "", 404},
		{"GET", "/epics/11", "", 404},
		{"GET", "/stories/abc", "", 404},
		{"GET", "/stories/1", "", 404},
		{"PATCH", "/stories/4821", "", 404},
		{"PUT", "/stories/4821", "nope", 400},
		{"POST", "/stories/4821/comments", `{"text":""}`, 400},
		{"POST", "/stories/4821/tasks", `{}`, 400},
		{"PUT", "/stories/4821/tasks/1", `{}`, 404},
		{"POST", "/stories/4821/tasks", `{"description":"x"}`, 201},
		{"PUT", "/stories/4821/tasks/900001", `nope`, 400},
		{"PUT", "/stories/4821/tasks/900001", `{}`, 200},
		{"PATCH", "/stories/4821/tasks/900001", `{}`, 404},
		{"PUT", "/stories/4821", `{"workflow_state_id":505,"owner_ids":["x"],"follower_ids":["y"]}`, 200},
		{"PUT", "/stories/4821", `{"workflow_state_id":503}`, 200},
		{"POST", "/stories/4821/comments", `{"text":"hi"}`, 201},
		{"PUT", "/stories/4821/tasks/900001", `{"complete":true}`, 200},
		{"PUT", "/stories/4821/tasks/900001", `{"complete":false}`, 200},
		{"POST", "/stories/4821/tasks", `{"description":"y"}`, 201},
		{"DELETE", "/stories/4821/tasks/900003", ``, 204},
	} {
		if code, _ := do(t, s, tc.method, tc.path, tc.body); code != tc.want {
			t.Errorf("%s %s %s = %d, want %d", tc.method, tc.path, tc.body, code, tc.want)
		}
	}
	if st := s.Story(StoryDoing); !st.CompletedAt.IsZero() || st.Tasks[0].Complete || len(st.Tasks) != 1 ||
		st.OwnerIDs[0] != "x" || st.FollowerIDs[0] != "y" || st.Comments[3].AuthorID != MeID {
		t.Errorf("story = %+v", st)
	}

	s.Fail = map[string]int{"GET /member": 503}
	if code, _ := do(t, s, "GET", "/member", ""); code != 503 {
		t.Errorf("injected failure = %d", code)
	}
	r := httptest.NewRequest("GET", Prefix+"/member", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no token = %d", w.Code)
	}
	if !strings.Contains(log.String(), "fake-shortcut: POST /stories/4821/tasks {\"description\":\"x\"}\n") {
		t.Errorf("log = %s", log.String())
	}
	if len(s.Writes()) == 0 || len(s.Requests()) <= len(s.Writes()) {
		t.Error("Writes/Requests")
	}
	s.Reset()
	if len(s.Requests()) != 0 {
		t.Error("Reset kept requests")
	}
}

func TestNowDefault(t *testing.T) {
	s := &Server{}
	if time.Since(s.now()) > time.Minute {
		t.Error("now without Now isn't the clock")
	}
}
