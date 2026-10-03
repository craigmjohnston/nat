// Package shortcut is a thin client for the parts of the Shortcut REST API v3
// the plugin uses. It knows nothing of nat: it reads and writes Shortcut
// resources and reports failures in words safe to show — a method, a path and
// a status, never a body and never the token.
package shortcut

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the live API. SHORTCUT_API_URL overrides it (a fake
// server in tests and scratch runs).
const DefaultBaseURL = "https://api.app.shortcut.com/api/v3"

// Timeout is each request's cap. nat kills a plugin call at 20 s; no single
// request may eat that budget.
const Timeout = 8 * time.Second

// pageSize is Shortcut search's largest page.
const pageSize = 25

// Client talks to one Shortcut workspace with one token.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New returns a client for base (DefaultBaseURL when empty) with an HTTP
// client capped at Timeout.
func New(base, token string) *Client {
	if base == "" {
		base = DefaultBaseURL
	}
	return &Client{BaseURL: strings.TrimRight(base, "/"), Token: token, HTTP: &http.Client{Timeout: Timeout}}
}

// StatusError is a non-2xx answer. Its message is the method, path and
// status alone: a response body is Shortcut's to word and may quote anything.
type StatusError struct {
	Method string
	Path   string
	Status int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("shortcut: %s %s: %d %s", e.Method, e.Path, e.Status, http.StatusText(e.Status))
}

// do sends one request with body (if non-nil) as JSON, and decodes a 2xx
// answer into out (if non-nil). path is relative to BaseURL and may carry a
// query.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		// Every body is one of this package's fixed structs of strings, bools
		// and numbers, which Marshal cannot fail on.
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rd)
	if err != nil {
		return fmt.Errorf("shortcut: %s %s: %w", method, cleanPath(path), err)
	}
	req.Header.Set("Shortcut-Token", c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("shortcut: %s %s: unreachable (%s)", method, cleanPath(path), reason(err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return &StatusError{Method: method, Path: cleanPath(path), Status: resp.StatusCode}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		// The decoder's own message can quote the response; say only that it
		// didn't decode.
		return fmt.Errorf("shortcut: %s %s: malformed response", method, cleanPath(path))
	}
	return nil
}

// cleanPath is path without its query — a search query is the user's own
// text, and an error line has no need of it.
func cleanPath(path string) string {
	p, _, _ := strings.Cut(path, "?")
	return p
}

// reason is a transport error's cause, short: a timeout says so, anything
// else is the innermost error's text (which never carries the token — that
// travels in a header, and url.Error quotes only the URL).
func reason(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		if ue.Timeout() {
			return "timed out"
		}
		err = ue.Err
	}
	return err.Error()
}

// Me is GET /member.
func (c *Client) Me(ctx context.Context) (MemberInfo, error) {
	var m MemberInfo
	return m, c.do(ctx, http.MethodGet, "/member", nil, &m)
}

// Members is GET /members.
func (c *Client) Members(ctx context.Context) ([]Member, error) {
	var ms []Member
	return ms, c.do(ctx, http.MethodGet, "/members", nil, &ms)
}

// Workflows is GET /workflows.
func (c *Client) Workflows(ctx context.Context) ([]Workflow, error) {
	var ws []Workflow
	return ws, c.do(ctx, http.MethodGet, "/workflows", nil, &ws)
}

// Groups is GET /groups — the workspace's teams.
func (c *Client) Groups(ctx context.Context) ([]Group, error) {
	var gs []Group
	return gs, c.do(ctx, http.MethodGet, "/groups", nil, &gs)
}

// Projects is GET /projects — a short list, one per Shortcut project.
func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	var ps []Project
	return ps, c.do(ctx, http.MethodGet, "/projects", nil, &ps)
}

// Epic is GET /epics/{id}.
func (c *Client) Epic(ctx context.Context, id int64) (Epic, error) {
	var e Epic
	return e, c.do(ctx, http.MethodGet, fmt.Sprintf("/epics/%d", id), nil, &e)
}

// Epics is GET /epics with includes_description=false — the slim list. With
// the descriptions, a real workspace's list runs to megabytes and seconds;
// without them it is ids and names, which is all a filter offers.
func (c *Client) Epics(ctx context.Context) ([]Epic, error) {
	var es []Epic
	return es, c.do(ctx, http.MethodGet, "/epics?includes_description=false", nil, &es)
}

// Labels is GET /labels in its slim form.
func (c *Client) Labels(ctx context.Context) ([]Label, error) {
	var ls []Label
	return ls, c.do(ctx, http.MethodGet, "/labels?slim=true", nil, &ls)
}

// Iteration is GET /iterations/{id}.
func (c *Client) Iteration(ctx context.Context, id int64) (Iteration, error) {
	var it Iteration
	return it, c.do(ctx, http.MethodGet, fmt.Sprintf("/iterations/%d", id), nil, &it)
}

// Search runs a story search, following pages until it has max stories or
// Shortcut has no more, and returns them with Shortcut's own total. A page
// after the first that fails ends the walk with what was already read — the
// caller has a time budget and a partial list is still the right list's head
// — so only a failed first page is an error.
func (c *Client) Search(ctx context.Context, query string, max int) ([]Story, int, error) {
	q := url.Values{}
	q.Set("query", query)
	q.Set("page_size", strconv.Itoa(pageSize))
	q.Set("detail", "slim")
	path := "/search/stories?" + q.Encode()
	var stories []Story
	total := 0
	for first := true; path != ""; first = false {
		var page SearchResult
		if err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
			if first {
				return nil, 0, err
			}
			break
		}
		if first {
			total = page.Total
		}
		stories = append(stories, page.Data...)
		if len(stories) >= max {
			return stories[:max], total, nil
		}
		path = nextPath(page.Next)
	}
	return stories, total, nil
}

// nextPath turns a search page's next link into a path on BaseURL. Shortcut
// gives it as "/api/v3/search/stories?…"; only its query is kept, so the token
// is never sent anywhere but the configured host.
func nextPath(next string) string {
	if next == "" {
		return ""
	}
	u, err := url.Parse(next)
	if err != nil || u.RawQuery == "" {
		return ""
	}
	return "/search/stories?" + u.RawQuery
}

// Story is GET /stories/{id}.
func (c *Client) Story(ctx context.Context, id int64) (Story, error) {
	var s Story
	return s, c.do(ctx, http.MethodGet, fmt.Sprintf("/stories/%d", id), nil, &s)
}

// StoryUpdate is the body of PUT /stories/{id}; only the fields set are sent.
type StoryUpdate struct {
	WorkflowStateID *int64   `json:"workflow_state_id,omitempty"`
	OwnerIDs        []string `json:"owner_ids,omitempty"`
	FollowerIDs     []string `json:"follower_ids,omitempty"`
}

// UpdateStory is PUT /stories/{id}.
func (c *Client) UpdateStory(ctx context.Context, id int64, u StoryUpdate) error {
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/stories/%d", id), u, nil)
}

// CreateComment is POST /stories/{id}/comments.
func (c *Client) CreateComment(ctx context.Context, id int64, text string) error {
	body := struct {
		Text string `json:"text"`
	}{text}
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/stories/%d/comments", id), body, nil)
}

// CreateTask is POST /stories/{id}/tasks.
func (c *Client) CreateTask(ctx context.Context, id int64, description string, complete bool) error {
	body := struct {
		Description string `json:"description"`
		Complete    bool   `json:"complete"`
	}{description, complete}
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/stories/%d/tasks", id), body, nil)
}

// CompleteTask is PUT /stories/{id}/tasks/{task-id} marking it complete.
func (c *Client) CompleteTask(ctx context.Context, id, taskID int64) error {
	body := struct {
		Complete bool `json:"complete"`
	}{true}
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/stories/%d/tasks/%d", id, taskID), body, nil)
}

// DeleteTask is DELETE /stories/{id}/tasks/{task-id}.
func (c *Client) DeleteTask(ctx context.Context, id, taskID int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/stories/%d/tasks/%d", id, taskID), nil, nil)
}
