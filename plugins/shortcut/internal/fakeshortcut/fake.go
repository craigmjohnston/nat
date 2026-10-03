// Package fakeshortcut is an in-memory stand-in for the slice of the
// Shortcut REST API v3 the plugin uses: the reads (member, members,
// workflows, groups, one epic, one iteration, story search, one story) and the
// writes (comments, story updates, story tasks). It records every request it
// is sent, so a test can assert the exact method, path and body, and it can
// be told to fail any route with a status.
//
// It serves both the unit tests (through httptest) and cmd/fakeshortcut, the
// scratch server an end-to-end run with nat is pointed at when there is no
// live token to spend.
package fakeshortcut

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/craigmjohnston/nat/plugins/shortcut/internal/shortcut"
)

// Prefix is the API path every route sits under, as on the live host.
const Prefix = "/api/v3"

// Request is one request the fake was sent. Path excludes Prefix; Body is
// the raw body ("" for none).
type Request struct {
	Method string
	Path   string
	Query  url.Values
	Body   string
}

// String is the request as one line: method, path, then the body if any.
func (r Request) String() string {
	if r.Body == "" {
		return r.Method + " " + r.Path
	}
	return r.Method + " " + r.Path + " " + r.Body
}

// Server is the fake. Set its fields before serving; read them back (under
// Lock, or after the requests are done) to see what changed.
type Server struct {
	// Token is the only Shortcut-Token accepted; anything else is a 401.
	Token string

	Me         shortcut.MemberInfo
	Members    []shortcut.Member
	Workflows  []shortcut.Workflow
	Groups     []shortcut.Group
	Epics      []shortcut.Epic
	Iterations []shortcut.Iteration
	Stories    map[int64]*shortcut.Story

	// Fail answers a route with a status instead of serving it, keyed
	// "METHOD /path" (no prefix, no query) — "GET /search/stories".
	Fail map[string]int
	// Log, when set, gets one line per request.
	Log io.Writer
	// Now stamps new comments; time.Now when nil.
	Now func() time.Time

	mu       sync.Mutex
	requests []Request
	nextID   int64
}

// Requests is a copy of every request so far, in arrival order.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.requests)
}

// Writes is Requests without the GETs.
func (s *Server) Writes() []Request {
	var out []Request
	for _, r := range s.Requests() {
		if r.Method != http.MethodGet {
			out = append(out, r)
		}
	}
	return out
}

// Reset forgets the requests so far.
func (s *Server) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = nil
}

// Story is a copy of one story's current state.
func (s *Server) Story(id int64) shortcut.Story {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.Stories[id]
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Server) id() int64 {
	s.nextID++
	return 900000 + s.nextID
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	path := strings.TrimPrefix(r.URL.Path, Prefix)
	req := Request{Method: r.Method, Path: path, Query: r.URL.Query(), Body: strings.TrimSpace(string(body))}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, req)
	if s.Log != nil {
		_, _ = fmt.Fprintf(s.Log, "fake-shortcut: %s\n", req)
	}
	if r.Header.Get("Shortcut-Token") != s.Token {
		http.Error(w, `{"message":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if code := s.Fail[r.Method+" "+path]; code != 0 {
		http.Error(w, `{"message":"injected failure"}`, code)
		return
	}
	status, out := s.route(r.Method, path, req)
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if out != nil {
		_ = json.NewEncoder(w).Encode(out)
	}
}

// route serves one request, returning a status (0 for 200) and a value to
// encode (nil for no body).
func (s *Server) route(method, path string, req Request) (int, any) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch {
	case method == http.MethodGet && path == "/member":
		return 0, s.Me
	case method == http.MethodGet && path == "/members":
		return 0, s.Members
	case method == http.MethodGet && path == "/workflows":
		return 0, s.Workflows
	case method == http.MethodGet && path == "/groups":
		return 0, s.Groups
	case method == http.MethodGet && len(parts) == 2 && parts[0] == "epics":
		return found(s.Epics, parts[1], func(e shortcut.Epic) int64 { return e.ID })
	case method == http.MethodGet && len(parts) == 2 && parts[0] == "iterations":
		return found(s.Iterations, parts[1], func(it shortcut.Iteration) int64 { return it.ID })
	case method == http.MethodGet && path == "/search/stories":
		return s.search(req.Query)
	case len(parts) >= 2 && parts[0] == "stories":
		id, err := strconv.ParseInt(parts[1], 10, 64)
		st := s.Stories[id]
		if err != nil || st == nil {
			return http.StatusNotFound, map[string]string{"message": "Resource not found."}
		}
		return s.story(method, parts[2:], st, req.Body)
	}
	return http.StatusNotFound, map[string]string{"message": "no such route"}
}

// found serves the one of items whose id is idText, or a 404.
func found[T any](items []T, idText string, id func(T) int64) (int, any) {
	for _, it := range items {
		if strconv.FormatInt(id(it), 10) == idText {
			return 0, it
		}
	}
	return http.StatusNotFound, map[string]string{"message": "Resource not found."}
}

// story serves /stories/{id}[/rest…].
func (s *Server) story(method string, rest []string, st *shortcut.Story, body string) (int, any) {
	switch {
	case len(rest) == 0 && method == http.MethodGet:
		return 0, st
	case len(rest) == 0 && method == http.MethodPut:
		var u map[string]json.RawMessage
		if json.Unmarshal([]byte(body), &u) != nil {
			return http.StatusBadRequest, nil
		}
		if v, ok := u["workflow_state_id"]; ok {
			_ = json.Unmarshal(v, &st.WorkflowStateID)
			st.CompletedAt = shortcut.Time{}
			if s.stateType(st.WorkflowStateID) == shortcut.StateDone {
				st.CompletedAt = shortcut.Time{Time: s.now()}
			}
		}
		if v, ok := u["owner_ids"]; ok {
			_ = json.Unmarshal(v, &st.OwnerIDs)
		}
		if v, ok := u["follower_ids"]; ok {
			_ = json.Unmarshal(v, &st.FollowerIDs)
		}
		st.UpdatedAt = shortcut.Time{Time: s.now()}
		return 0, st
	case len(rest) == 1 && rest[0] == "comments" && method == http.MethodPost:
		var c shortcut.Comment
		if json.Unmarshal([]byte(body), &c) != nil || c.Text == "" {
			return http.StatusBadRequest, nil
		}
		c.ID, c.AuthorID, c.CreatedAt = s.id(), s.Me.ID, shortcut.Time{Time: s.now()}
		st.Comments = append(st.Comments, c)
		return http.StatusCreated, c
	case len(rest) == 1 && rest[0] == "tasks" && method == http.MethodPost:
		var t shortcut.Task
		if json.Unmarshal([]byte(body), &t) != nil || t.Description == "" {
			return http.StatusBadRequest, nil
		}
		t.ID, t.Position = s.id(), int64(len(st.Tasks)+1)
		st.Tasks = append(st.Tasks, t)
		return http.StatusCreated, t
	case len(rest) == 2 && rest[0] == "tasks":
		tid, _ := strconv.ParseInt(rest[1], 10, 64)
		i := slices.IndexFunc(st.Tasks, func(t shortcut.Task) bool { return t.ID == tid })
		if i < 0 {
			return http.StatusNotFound, map[string]string{"message": "Resource not found."}
		}
		switch method {
		case http.MethodPut:
			var u struct {
				Complete *bool `json:"complete"`
			}
			if json.Unmarshal([]byte(body), &u) != nil {
				return http.StatusBadRequest, nil
			}
			if u.Complete != nil {
				st.Tasks[i].Complete = *u.Complete
			}
			return 0, st.Tasks[i]
		case http.MethodDelete:
			st.Tasks = slices.Delete(st.Tasks, i, i+1)
			return http.StatusNoContent, nil
		}
	}
	return http.StatusNotFound, map[string]string{"message": "no such route"}
}

// search answers GET /search/stories over the stories, in id order, a page
// at a time; next is an offset carried in the query.
func (s *Server) search(q url.Values) (int, any) {
	terms := splitQuery(q.Get("query"))
	ids := make([]int64, 0, len(s.Stories))
	for id := range s.Stories {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var hits []shortcut.Story
	for _, id := range ids {
		st := s.Stories[id]
		if s.matches(st, terms) {
			hits = append(hits, slim(*st))
		}
	}
	size, _ := strconv.Atoi(q.Get("page_size"))
	if size <= 0 {
		size = 25
	}
	from, _ := strconv.Atoi(q.Get("next"))
	to := min(from+size, len(hits))
	from = min(from, to)
	res := shortcut.SearchResult{Data: hits[from:to], Total: len(hits)}
	if res.Data == nil {
		res.Data = []shortcut.Story{}
	}
	if to < len(hits) {
		n := url.Values{}
		n.Set("query", q.Get("query"))
		n.Set("page_size", strconv.Itoa(size))
		n.Set("next", strconv.Itoa(to))
		res.Next = Prefix + "/search/stories?" + n.Encode()
	}
	return 0, res
}

// slim is a story as a slim search result carries it: no description,
// comments, tasks or VCS links.
func slim(st shortcut.Story) shortcut.Story {
	st.Description, st.Comments, st.Tasks = "", nil, nil
	st.Branches, st.PullRequests, st.StoryLinks, st.ExternalLinks = nil, nil, nil, nil
	return st
}

// splitQuery splits a search query on spaces, keeping a quoted value whole
// and dropping its quotes: `state:"Ready for Dev" !is:done` is two terms.
func splitQuery(q string) []string {
	var terms []string
	var cur strings.Builder
	quoted := false
	for _, r := range q {
		switch {
		case r == '"':
			quoted = !quoted
		case r == ' ' && !quoted:
			if cur.Len() > 0 {
				terms = append(terms, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		terms = append(terms, cur.String())
	}
	return terms
}

// matches is whether st satisfies every term — the handful of Shortcut
// search operators the plugin and its default segment use. A bare word
// matches the story's name.
func (s *Server) matches(st *shortcut.Story, terms []string) bool {
	for _, t := range terms {
		neg := strings.HasPrefix(t, "!") || strings.HasPrefix(t, "-")
		if neg {
			t = t[1:]
		}
		if s.term(st, t) == neg {
			return false
		}
	}
	return true
}

func (s *Server) term(st *shortcut.Story, t string) bool {
	op, val, ok := strings.Cut(t, ":")
	if !ok {
		return strings.Contains(strings.ToLower(st.Name), strings.ToLower(t))
	}
	switch op {
	case "owner":
		// Like the live API, `owner:me` is accepted and matches nothing:
		// only a mention name names an owner.
		return slices.ContainsFunc(st.OwnerIDs, func(id string) bool { return s.mention(id) == val })
	case "is":
		return s.stateType(st.WorkflowStateID) == val
	case "state":
		return strings.EqualFold(s.stateName(st.WorkflowStateID), val)
	case "type":
		return st.StoryType == val
	case "label":
		return slices.ContainsFunc(st.Labels, func(l shortcut.Label) bool { return l.Name == val })
	case "team":
		return slices.ContainsFunc(s.Groups, func(g shortcut.Group) bool {
			return g.ID == st.GroupID && (g.MentionName == val || g.Name == val)
		})
	case "epic":
		return slices.ContainsFunc(s.Epics, func(e shortcut.Epic) bool {
			return e.ID == st.EpicID && (e.Name == val || strconv.FormatInt(e.ID, 10) == val)
		})
	}
	return false
}

func (s *Server) mention(memberID string) string {
	for _, m := range s.Members {
		if m.ID == memberID {
			return m.Profile.MentionName
		}
	}
	return ""
}

func (s *Server) state(id int64) shortcut.WorkflowState {
	for _, w := range s.Workflows {
		for _, st := range w.States {
			if st.ID == id {
				return st
			}
		}
	}
	return shortcut.WorkflowState{}
}

func (s *Server) stateType(id int64) string { return s.state(id).Type }
func (s *Server) stateName(id int64) string { return s.state(id).Name }
