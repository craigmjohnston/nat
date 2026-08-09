package notion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// testClient returns a client pointed at srv whose retry waits are instant and
// recorded in waits.
func testClient(t *testing.T, srv *httptest.Server, opts ...Option) (*Client, *[]time.Duration) {
	t.Helper()
	var waits []time.Duration
	base := []Option{WithBaseURL(srv.URL)}
	c := New("secret-key", append(base, opts...)...)
	c.after = func(d time.Duration) <-chan time.Time {
		waits = append(waits, d)
		ch := make(chan time.Time, 1)
		ch <- time.Time{}
		return ch
	}
	return c, &waits
}

func TestNewDefaults(t *testing.T) {
	c := New("secret-key")
	if c.apiKey != "secret-key" {
		t.Errorf("apiKey = %q", c.apiKey)
	}
	if c.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, DefaultBaseURL)
	}
	if c.httpClient == nil || c.httpClient.Timeout == 0 {
		t.Error("want a default http client with a timeout")
	}
	if c.maxRetries != defaultMaxRetries {
		t.Errorf("maxRetries = %d, want %d", c.maxRetries, defaultMaxRetries)
	}
	if c.after == nil {
		t.Error("after must default to time.After")
	}
}

func TestOptions(t *testing.T) {
	hc := &http.Client{}
	c := New("k",
		WithBaseURL("https://example.test/v1/"),
		WithHTTPClient(hc),
		WithMaxRetries(7),
	)
	if c.baseURL != "https://example.test/v1" {
		t.Errorf("baseURL = %q, want the trailing slash trimmed", c.baseURL)
	}
	if c.httpClient != hc {
		t.Error("WithHTTPClient did not take effect")
	}
	if c.maxRetries != 7 {
		t.Errorf("maxRetries = %d, want 7", c.maxRetries)
	}

	if got := New("k", WithMaxRetries(-1)).maxRetries; got != 0 {
		t.Errorf("negative maxRetries = %d, want 0", got)
	}
}

func TestDoSendsAuthHeadersAndBody(t *testing.T) {
	type reply struct {
		Object string `json:"object"`
	}
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-key" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Notion-Version"); got != Version {
			t.Errorf("Notion-Version = %q, want %q", got, Version)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/pages" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Write([]byte(`{"object":"page"}`))
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	var out reply
	if err := c.do(context.Background(), http.MethodPost, "/pages", map[string]any{"parent": "abc"}, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody != `{"parent":"abc"}` {
		t.Errorf("request body = %s", gotBody)
	}
	if out.Object != "page" {
		t.Errorf("decoded object = %q", out.Object)
	}
}

func TestDoWithoutBodyOrOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "" {
			t.Errorf("bodyless request set Content-Type = %q", got)
		}
		if r.ContentLength > 0 {
			t.Errorf("bodyless request sent %d bytes", r.ContentLength)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	if err := c.do(context.Background(), http.MethodGet, "/users", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDoRetriesRateLimit(t *testing.T) {
	t.Run("retries then succeeds, honouring Retry-After", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.Write([]byte(`{"object":"list"}`))
		}))
		defer srv.Close()

		c, waits := testClient(t, srv)
		if err := c.do(context.Background(), http.MethodGet, "/users", nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls.Load() != 2 {
			t.Errorf("server calls = %d, want 2", calls.Load())
		}
		if len(*waits) != 1 || (*waits)[0] != 2*time.Second {
			t.Errorf("waits = %v, want one 2s wait", *waits)
		}
	})

	t.Run("resends the request body on retry", func(t *testing.T) {
		var bodies []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			bodies = append(bodies, string(b))
			if len(bodies) == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.Write([]byte(`{}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		if err := c.do(context.Background(), http.MethodPost, "/q", map[string]any{"a": 1}, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(bodies) != 2 || bodies[0] != bodies[1] || bodies[1] != `{"a":1}` {
			t.Errorf("bodies = %q, want the same payload twice", bodies)
		}
	})

	t.Run("gives up after the retry cap", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"code":"rate_limited","message":"slow down"}`))
		}))
		defer srv.Close()

		c, waits := testClient(t, srv, WithMaxRetries(2))
		err := c.do(context.Background(), http.MethodGet, "/users", nil, nil)
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("got %v, want *APIError", err)
		}
		if !apiErr.RateLimited() {
			t.Error("want RateLimited")
		}
		if calls.Load() != 3 {
			t.Errorf("server calls = %d, want 3 (initial + 2 retries)", calls.Load())
		}
		if len(*waits) != 2 {
			t.Errorf("waits = %v, want 2", *waits)
		}
	})

	t.Run("no retries when disabled", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer srv.Close()

		c, _ := testClient(t, srv, WithMaxRetries(0))
		if err := c.do(context.Background(), http.MethodGet, "/users", nil, nil); err == nil {
			t.Fatal("want an error")
		}
		if calls.Load() != 1 {
			t.Errorf("server calls = %d, want 1", calls.Load())
		}
	})

	t.Run("a cancelled context aborts the wait", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer srv.Close()

		ctx, cancel := context.WithCancel(context.Background())
		c := New("k", WithBaseURL(srv.URL))
		c.after = func(time.Duration) <-chan time.Time {
			cancel()
			return make(chan time.Time) // never fires
		}
		if err := c.do(ctx, http.MethodGet, "/users", nil, nil); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	})
}

func TestDoRetriesTransientServerErrors(t *testing.T) {
	t.Run("a repeatable request recovers from 503", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Write([]byte(`{"object":"list"}`))
		}))
		defer srv.Close()

		c, waits := testClient(t, srv)
		if err := c.do(context.Background(), http.MethodPost, "/data_sources/d1/query", map[string]any{}, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls.Load() != 2 {
			t.Errorf("server calls = %d, want 2", calls.Load())
		}
		if len(*waits) != 1 || (*waits)[0] != time.Second {
			t.Errorf("waits = %v, want one 1s backoff", *waits)
		}
	})

	t.Run("backs off exponentially until the cap", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer srv.Close()

		c, waits := testClient(t, srv, WithMaxRetries(3))
		var apiErr *APIError
		if err := c.do(context.Background(), http.MethodGet, "/users", nil, nil); !errors.As(err, &apiErr) {
			t.Fatalf("got %v, want *APIError", err)
		}
		if apiErr.StatusCode != http.StatusBadGateway {
			t.Errorf("status = %d", apiErr.StatusCode)
		}
		want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
		if len(*waits) != len(want) {
			t.Fatalf("waits = %v, want %v", *waits, want)
		}
		for i, w := range want {
			if (*waits)[i] != w {
				t.Errorf("wait %d = %v, want %v", i, (*waits)[i], w)
			}
		}
	})

	t.Run("a write is not repeated after 502", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer srv.Close()

		c, waits := testClient(t, srv)
		if err := c.do(context.Background(), http.MethodPost, "/pages", map[string]any{"parent": "x"}, nil); err == nil {
			t.Fatal("want an error")
		}
		if calls.Load() != 1 {
			t.Errorf("server calls = %d, want 1: a 502 may mean the write landed", calls.Load())
		}
		if len(*waits) != 0 {
			t.Errorf("waits = %v, want none", *waits)
		}
	})

	t.Run("a 500 is surfaced immediately", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		if err := c.do(context.Background(), http.MethodGet, "/users", nil, nil); err == nil {
			t.Fatal("want an error")
		}
		if calls.Load() != 1 {
			t.Errorf("server calls = %d, want 1", calls.Load())
		}
	})
}

func TestDoErrors(t *testing.T) {
	t.Run("maps a Notion error envelope", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"object":"error","status":404,"code":"object_not_found","message":"Could not find page"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		err := c.do(context.Background(), http.MethodGet, "/pages/x", nil, nil)
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("got %v, want *APIError", err)
		}
		if apiErr.StatusCode != 404 || apiErr.Code != "object_not_found" || apiErr.Message != "Could not find page" {
			t.Errorf("got %+v", apiErr)
		}
		if !apiErr.NotFound() || apiErr.Unauthorized() || apiErr.RateLimited() {
			t.Error("classifier helpers disagree with status 404")
		}
		if want := "notion: 404 object_not_found: Could not find page"; apiErr.Error() != want {
			t.Errorf("Error() = %q, want %q", apiErr.Error(), want)
		}
	})

	t.Run("keeps a non-envelope body verbatim", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte("<html>bad gateway</html>\n"))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv, WithMaxRetries(0))
		var apiErr *APIError
		if err := c.do(context.Background(), http.MethodGet, "/x", nil, nil); !errors.As(err, &apiErr) {
			t.Fatalf("got %v, want *APIError", err)
		}
		if apiErr.Code != "" || apiErr.Body != "<html>bad gateway</html>" {
			t.Errorf("got %+v", apiErr)
		}
		if want := "notion: 502: <html>bad gateway</html>"; apiErr.Error() != want {
			t.Errorf("Error() = %q, want %q", apiErr.Error(), want)
		}
	})

	t.Run("an empty body falls back to the status text", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		var apiErr *APIError
		if err := c.do(context.Background(), http.MethodGet, "/x", nil, nil); !errors.As(err, &apiErr) {
			t.Fatalf("got %v, want *APIError", err)
		}
		if !apiErr.Unauthorized() {
			t.Error("want Unauthorized")
		}
		if want := "notion: 401 Unauthorized"; apiErr.Error() != want {
			t.Errorf("Error() = %q, want %q", apiErr.Error(), want)
		}
	})

	t.Run("an unencodable body fails before any request", func(t *testing.T) {
		c := New("k", WithBaseURL("http://127.0.0.1:1"))
		err := c.do(context.Background(), http.MethodPost, "/x", make(chan int), nil)
		if err == nil || !strings.Contains(err.Error(), "encode request body") {
			t.Fatalf("got %v, want an encode error", err)
		}
	})

	t.Run("an invalid method fails to build a request", func(t *testing.T) {
		c := New("k", WithBaseURL("http://127.0.0.1:1"))
		err := c.do(context.Background(), "bad method", "/x", nil, nil)
		if err == nil || !strings.Contains(err.Error(), "build request") {
			t.Fatalf("got %v, want a build error", err)
		}
	})

	t.Run("a transport failure is wrapped with method and path", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		srv.Close() // nothing is listening

		c, _ := testClient(t, srv)
		err := c.do(context.Background(), http.MethodGet, "/users", nil, nil)
		if err == nil || !strings.Contains(err.Error(), "GET /users") {
			t.Fatalf("got %v, want a wrapped transport error", err)
		}
	})

	t.Run("a body read failure is wrapped", func(t *testing.T) {
		c := New("k", WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(errReader{}),
			}, nil
		})}))
		err := c.do(context.Background(), http.MethodGet, "/users", nil, nil)
		if err == nil || !strings.Contains(err.Error(), "read response body") {
			t.Fatalf("got %v, want a read error", err)
		}
	})

	t.Run("an undecodable response is wrapped", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not json"))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		var out struct{ Object string }
		err := c.do(context.Background(), http.MethodGet, "/users", nil, &out)
		if err == nil || !strings.Contains(err.Error(), "decode response") {
			t.Fatalf("got %v, want a decode error", err)
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		want   time.Duration
		wantOK bool
	}{
		{"whole seconds", "3", 3 * time.Second, true},
		{"fractional seconds", "0.25", 250 * time.Millisecond, true},
		{"padded", "  1 ", time.Second, true},
		{"absurd values are capped", "99999", maxRetryWait, true},
		{"missing", "", 0, false},
		{"unparsable", "soon", 0, false},
		{"zero", "0", 0, false},
		{"negative", "-5", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseRetryAfter(tt.value)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("parseRetryAfter(%q) = %v, %v; want %v, %v", tt.value, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestRetryable(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		status int
		want   bool
	}{
		{"429 on a write is retried", http.MethodPatch, "/pages/x", 429, true},
		{"429 on a read is retried", http.MethodGet, "/users", 429, true},
		{"502 on a GET is retried", http.MethodGet, "/users", 502, true},
		{"503 on a GET is retried", http.MethodGet, "/users", 503, true},
		{"502 on a data source query is retried", http.MethodPost, "/data_sources/d1/query", 502, true},
		{"502 on a query with a query string is retried", http.MethodPost, "/data_sources/d1/query?x=1", 502, true},
		{"503 on search is retried", http.MethodPost, "/search", 503, true},
		{"502 on page creation is not retried", http.MethodPost, "/pages", 502, false},
		{"503 on a page update is not retried", http.MethodPatch, "/pages/x", 503, false},
		{"502 on a delete is not retried", http.MethodDelete, "/blocks/x", 502, false},
		{"500 is not retried", http.MethodGet, "/users", 500, false},
		{"504 is not retried", http.MethodGet, "/users", 504, false},
		{"404 is not retried", http.MethodGet, "/users", 404, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryable(tt.method, tt.path, tt.status); got != tt.want {
				t.Errorf("retryable(%s, %s, %d) = %v, want %v", tt.method, tt.path, tt.status, got, tt.want)
			}
		})
	}
}

func TestRetryWait(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		header  string
		attempt int
		want    time.Duration
	}{
		{"Retry-After wins", 429, "5", 0, 5 * time.Second},
		{"Retry-After wins on 503 too", 503, "5", 2, 5 * time.Second},
		{"429 without a header waits a flat second", 429, "", 3, defaultRetryWait},
		{"5xx backs off from one second", 502, "", 0, time.Second},
		{"5xx doubles each attempt", 502, "", 2, 4 * time.Second},
		{"5xx backoff is capped", 503, "", 10, maxRetryWait},
		{"5xx backoff cannot overflow", 503, "", 62, maxRetryWait},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryWait(tt.status, tt.header, tt.attempt); got != tt.want {
				t.Errorf("retryWait(%d, %q, %d) = %v, want %v", tt.status, tt.header, tt.attempt, got, tt.want)
			}
		})
	}
}

type item struct {
	ID string `json:"id"`
}

// pagedHandler serves successive pages of ids, recording each request's query
// string and body.
type pagedHandler struct {
	pages   [][]string
	queries []string
	bodies  []string
}

func (h *pagedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	h.queries = append(h.queries, r.URL.RawQuery)
	h.bodies = append(h.bodies, string(b))

	idx := len(h.queries) - 1
	page := h.pages[idx]
	results := make([]item, len(page))
	for i, id := range page {
		results[i] = item{ID: id}
	}
	hasMore := idx < len(h.pages)-1
	var next *string
	if hasMore {
		cursor := "cursor-" + page[len(page)-1]
		next = &cursor
	}
	json.NewEncoder(w).Encode(List[item]{Results: results, HasMore: hasMore, NextCursor: next})
}

func TestPaginate(t *testing.T) {
	t.Run("threads the cursor through a POST body", func(t *testing.T) {
		h := &pagedHandler{pages: [][]string{{"a", "b"}, {"c"}}}
		srv := httptest.NewServer(h)
		defer srv.Close()

		c, _ := testClient(t, srv)
		got, err := paginate[item](context.Background(), c, http.MethodPost, "/query", map[string]any{"page_size": 2})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 || got[0].ID != "a" || got[2].ID != "c" {
			t.Errorf("results = %+v", got)
		}
		if h.bodies[0] != `{"page_size":2}` {
			t.Errorf("first body = %s, want no cursor", h.bodies[0])
		}
		if h.bodies[1] != `{"page_size":2,"start_cursor":"cursor-b"}` {
			t.Errorf("second body = %s", h.bodies[1])
		}
	})

	t.Run("does not mutate the caller's body", func(t *testing.T) {
		h := &pagedHandler{pages: [][]string{{"a"}, {"b"}}}
		srv := httptest.NewServer(h)
		defer srv.Close()

		c, _ := testClient(t, srv)
		body := map[string]any{"page_size": 1}
		if _, err := paginate[item](context.Background(), c, http.MethodPost, "/query", body); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(body) != 1 {
			t.Errorf("caller body was mutated: %v", body)
		}
	})

	t.Run("threads the cursor as a query parameter when there is no body", func(t *testing.T) {
		h := &pagedHandler{pages: [][]string{{"a"}, {"b"}}}
		srv := httptest.NewServer(h)
		defer srv.Close()

		c, _ := testClient(t, srv)
		got, err := paginate[item](context.Background(), c, http.MethodGet, "/users", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("results = %+v", got)
		}
		if q := h.queries[0]; q != "" {
			t.Errorf("first query = %q, want empty", q)
		}
		if q := h.queries[1]; q != "start_cursor=cursor-a" {
			t.Errorf("second query = %q", q)
		}
	})

	t.Run("appends to an existing query string", func(t *testing.T) {
		h := &pagedHandler{pages: [][]string{{"a"}, {"b"}}}
		srv := httptest.NewServer(h)
		defer srv.Close()

		c, _ := testClient(t, srv)
		if _, err := paginate[item](context.Background(), c, http.MethodGet, "/users?page_size=1", nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if q := h.queries[1]; q != "page_size=1&start_cursor=cursor-a" {
			t.Errorf("second query = %q", q)
		}
	})

	t.Run("stops when has_more is false even with a cursor present", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"results":[{"id":"a"}],"has_more":false,"next_cursor":"leftover"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		got, err := paginate[item](context.Background(), c, http.MethodGet, "/users", nil)
		if err != nil || len(got) != 1 {
			t.Fatalf("got %+v, %v", got, err)
		}
	})

	t.Run("stops when has_more is true but the cursor is empty", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"results":[{"id":"a"}],"has_more":true,"next_cursor":""}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		got, err := paginate[item](context.Background(), c, http.MethodGet, "/users", nil)
		if err != nil || len(got) != 1 {
			t.Fatalf("got %+v, %v", got, err)
		}
	})

	t.Run("stops when has_more is true but the cursor is null", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"results":[{"id":"a"}],"has_more":true,"next_cursor":null}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		got, err := paginate[item](context.Background(), c, http.MethodGet, "/users", nil)
		if err != nil || len(got) != 1 {
			t.Fatalf("got %+v, %v", got, err)
		}
	})

	t.Run("fails fast when the cursor stops advancing", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"results":[{"id":"a"}],"has_more":true,"next_cursor":"same"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		_, err := paginate[item](context.Background(), c, http.MethodGet, "/users", nil)
		if err == nil || !strings.Contains(err.Error(), "pagination stalled") {
			t.Fatalf("got %v, want a stalled-pagination error", err)
		}
	})

	t.Run("propagates a request error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		got, err := paginate[item](context.Background(), c, http.MethodGet, "/users", nil)
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("got %v, want *APIError", err)
		}
		if got != nil {
			t.Errorf("results = %+v, want nil on error", got)
		}
	})
}
