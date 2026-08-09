// Package notion is a hand-rolled, stdlib-only client for the Notion REST API,
// pinned to the data-source API version. It carries only the endpoints and
// struct fields this app actually uses.
package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the Notion API root. Paths passed to the client are
	// appended to it, so they must start with "/".
	DefaultBaseURL = "https://api.notion.com/v1"

	// Version is the Notion-Version header sent on every request. The
	// data-source model this app depends on only exists from this version on.
	Version = "2026-03-11"

	defaultMaxRetries = 3
	defaultRetryWait  = time.Second
	maxRetryWait      = 30 * time.Second
)

// Client talks to the Notion REST API. The zero value is not usable; build one
// with New.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	maxRetries int

	// after is time.After in production; tests replace it so retries do not
	// actually sleep.
	after func(time.Duration) <-chan time.Time
}

// Option customises a Client.
type Option func(*Client)

// WithBaseURL overrides the API root, chiefly so tests can point at an
// httptest.Server. A trailing slash is trimmed.
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimSuffix(u, "/") }
}

// WithHTTPClient overrides the HTTP client used for requests.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithMaxRetries caps how many times a rate-limited request is retried. Zero
// disables retrying; negative values are treated as zero.
func WithMaxRetries(n int) Option {
	return func(c *Client) {
		if n < 0 {
			n = 0
		}
		c.maxRetries = n
	}
}

// New returns a Client authenticating with the given integration token.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:     apiKey,
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		maxRetries: defaultMaxRetries,
		after:      time.After,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// APIError is a non-2xx response from the Notion API. Code and Message come
// from Notion's error envelope; when the body is not that envelope (a proxy
// error page, say) Body holds the raw payload instead.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Body       string
}

// Error implements error.
func (e *APIError) Error() string {
	switch {
	case e.Code != "" || e.Message != "":
		return fmt.Sprintf("notion: %d %s: %s", e.StatusCode, e.Code, e.Message)
	case e.Body != "":
		return fmt.Sprintf("notion: %d: %s", e.StatusCode, e.Body)
	default:
		return fmt.Sprintf("notion: %d %s", e.StatusCode, http.StatusText(e.StatusCode))
	}
}

// NotFound reports whether the request failed because the object does not
// exist or the integration cannot see it.
func (e *APIError) NotFound() bool { return e.StatusCode == http.StatusNotFound }

// Unauthorized reports whether the API key was rejected.
func (e *APIError) Unauthorized() bool { return e.StatusCode == http.StatusUnauthorized }

// RateLimited reports whether the request was throttled — only surfaced once
// the client's retries are exhausted.
func (e *APIError) RateLimited() bool { return e.StatusCode == http.StatusTooManyRequests }

// newAPIError builds an *APIError from a response status and body.
func newAPIError(status int, body []byte) *APIError {
	e := &APIError{StatusCode: status}
	var env struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &env); err == nil && (env.Code != "" || env.Message != "") {
		e.Code = env.Code
		e.Message = env.Message
		return e
	}
	e.Body = strings.TrimSpace(string(body))
	return e
}

// do performs one API call. body, when non-nil, is JSON-encoded as the request
// payload; out, when non-nil, receives the JSON-decoded response. Non-2xx
// responses become an *APIError. Requests rejected with 429 are retried up to
// the client's retry cap, honouring Retry-After.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body for %s %s: %w", method, path, err)
		}
	}

	for attempt := 0; ; attempt++ {
		var reader io.Reader
		if payload != nil {
			reader = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
		if err != nil {
			return fmt.Errorf("build request %s %s: %w", method, path, err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Notion-Version", Version)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		status, header, data, err := c.roundTrip(req)
		if err != nil {
			return fmt.Errorf("%s %s: %w", method, path, err)
		}

		if attempt < c.maxRetries && retryable(method, path, status) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-c.after(retryWait(status, header.Get("Retry-After"), attempt)):
			}
			continue
		}
		if status < 200 || status > 299 {
			return newAPIError(status, data)
		}
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response for %s %s: %w", method, path, err)
		}
		return nil
	}
}

// roundTrip sends req and fully reads the response so the body can be closed
// before any retry wait.
func (c *Client) roundTrip(req *http.Request) (int, http.Header, []byte, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("read response body: %w", err)
	}
	return resp.StatusCode, resp.Header, data, nil
}

// retryable reports whether a failed request should be tried again.
//
// A 429 never reached the handler, so any request can safely be repeated. A
// 502/503 is ambiguous — the gateway may have failed after Notion applied the
// change — so those are only retried for requests that are safe to repeat:
// every GET, plus Notion's read-shaped POSTs (data source queries and search).
// Retrying a POST that creates or updates a page could duplicate the write.
func retryable(method, path string, status int) bool {
	switch status {
	case http.StatusTooManyRequests:
		return true
	case http.StatusBadGateway, http.StatusServiceUnavailable:
		return safeToRepeat(method, path)
	default:
		return false
	}
}

// safeToRepeat reports whether re-sending the request cannot change server
// state a second time.
func safeToRepeat(method, path string) bool {
	if method == http.MethodGet || method == http.MethodHead {
		return true
	}
	if method != http.MethodPost {
		return false
	}
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	return strings.HasSuffix(path, "/query") || path == "/search"
}

// retryWait picks how long to wait before the next attempt. Retry-After wins
// when the server sends a usable one — Notion sends it on 429. Otherwise waits
// back off exponentially from one second, so a struggling gateway is not
// hammered at a fixed rate.
func retryWait(status int, header string, attempt int) time.Duration {
	if d, ok := parseRetryAfter(header); ok {
		return d
	}
	if status == http.StatusTooManyRequests {
		return defaultRetryWait
	}
	d := defaultRetryWait << attempt
	if d > maxRetryWait || d <= 0 {
		return maxRetryWait
	}
	return d
}

// parseRetryAfter interprets a Retry-After header value as a wait duration,
// reporting whether it was usable. Notion sends whole or fractional seconds;
// absurd values are capped so a hostile header cannot stall the UI.
func parseRetryAfter(v string) (time.Duration, bool) {
	secs, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || secs <= 0 {
		return 0, false
	}
	d := time.Duration(secs * float64(time.Second))
	if d > maxRetryWait {
		return maxRetryWait, true
	}
	return d, true
}

// paginate walks a cursor-paginated list endpoint and returns every result.
// For POST endpoints the cursor is threaded through the request body; for
// bodyless (GET) endpoints it is added as a query parameter. body is not
// mutated.
func paginate[T any](ctx context.Context, c *Client, method, path string, body map[string]any) ([]T, error) {
	var all []T
	cursor := ""
	for {
		reqPath := path
		var reqBody any
		switch {
		case body != nil:
			next := make(map[string]any, len(body)+1)
			for k, v := range body {
				next[k] = v
			}
			if cursor != "" {
				next["start_cursor"] = cursor
			}
			reqBody = next
		case cursor != "":
			sep := "?"
			if strings.Contains(reqPath, "?") {
				sep = "&"
			}
			reqPath += sep + "start_cursor=" + url.QueryEscape(cursor)
		}

		var page List[T]
		if err := c.do(ctx, method, reqPath, reqBody, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Results...)

		if !page.HasMore || page.NextCursor == nil || *page.NextCursor == "" {
			return all, nil
		}
		if *page.NextCursor == cursor {
			return nil, fmt.Errorf("%s %s: pagination stalled on cursor %q", method, path, cursor)
		}
		cursor = *page.NextCursor
	}
}
