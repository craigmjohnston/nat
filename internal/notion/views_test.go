package notion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListViews(t *testing.T) {
	t.Run("returns the views a data source has", func(t *testing.T) {
		var gotMethod, gotPath, gotQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
			w.Write([]byte(`{"object":"list","results":[
				{"object":"view","id":"v-1","name":"Board","type":"board"},
				{"object":"view","id":"v-2","name":"All","type":"table"}
			],"has_more":false,"next_cursor":null}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		views, err := c.ListViews(context.Background(), "ds 1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotMethod != http.MethodGet || gotPath != "/views" || gotQuery != "data_source_id=ds+1" {
			t.Errorf("got %s %s?%s, want GET /views?data_source_id=ds+1", gotMethod, gotPath, gotQuery)
		}
		want := []View{{ID: "v-1", Name: "Board", Type: "board"}, {ID: "v-2", Name: "All", Type: "table"}}
		if len(views) != len(want) || views[0] != want[0] || views[1] != want[1] {
			t.Errorf("views = %+v, want %+v", views, want)
		}
	})

	t.Run("propagates an API error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":"object_not_found","message":"nope"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		views, err := c.ListViews(context.Background(), "ds-1")
		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.NotFound() {
			t.Fatalf("got %v, want a not-found *APIError", err)
		}
		if views != nil {
			t.Errorf("views = %+v, want nil on error", views)
		}
	})
}

func TestViewOrder(t *testing.T) {
	t.Run("starts a query and returns the rows in the view's order", func(t *testing.T) {
		var gotMethod, gotPath string
		var gotBody map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.EscapedPath()
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			w.Write([]byte(`{"object":"view_query","id":"q-1","results":[
				{"object":"page","id":"p-3"},
				{"object":"page","id":"p-1"},
				{"object":"page","id":"p-2"}
			],"has_more":false,"next_cursor":null}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		ids, err := c.ViewOrder(context.Background(), "v 1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotMethod != http.MethodPost || gotPath != "/views/v%201/queries" {
			t.Errorf("got %s %s, want POST /views/v%%201/queries", gotMethod, gotPath)
		}
		if want := map[string]any{"page_size": float64(100)}; len(gotBody) != 1 || gotBody["page_size"] != want["page_size"] {
			t.Errorf("body = %v, want %v", gotBody, want)
		}
		if strings.Join(ids, ",") != "p-3,p-1,p-2" {
			t.Errorf("ids = %v, want the view's order", ids)
		}
	})

	t.Run("follows pagination under the query it started", func(t *testing.T) {
		var paths []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.Method+" "+r.URL.RequestURI())
			if r.Method == http.MethodPost {
				w.Write([]byte(`{"id":"q-1","results":[{"id":"p-1"}],"has_more":true,"next_cursor":"c-1"}`))
				return
			}
			if r.URL.Query().Get("start_cursor") == "c-1" {
				w.Write([]byte(`{"results":[{"id":"p-2"}],"has_more":true,"next_cursor":"c-2"}`))
				return
			}
			w.Write([]byte(`{"results":[{"id":"p-3"}],"has_more":false,"next_cursor":null}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		ids, err := c.ViewOrder(context.Background(), "v-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Join(ids, ",") != "p-1,p-2,p-3" {
			t.Errorf("ids = %v, want every page in order", ids)
		}
		want := []string{
			"POST /views/v-1/queries",
			"GET /views/v-1/queries/q-1?page_size=100&start_cursor=c-1",
			"GET /views/v-1/queries/q-1?page_size=100&start_cursor=c-2",
		}
		if len(paths) != len(want) {
			t.Fatalf("requests = %v, want %v", paths, want)
		}
		for i, w := range want {
			if paths[i] != w {
				t.Errorf("request %d = %q, want %q", i, paths[i], w)
			}
		}
	})

	t.Run("gives up when pagination stalls", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				w.Write([]byte(`{"id":"q-1","results":[{"id":"p-1"}],"has_more":true,"next_cursor":"c-1"}`))
				return
			}
			w.Write([]byte(`{"results":[{"id":"p-2"}],"has_more":true,"next_cursor":"c-1"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		ids, err := c.ViewOrder(context.Background(), "v-1")
		if err == nil || !strings.Contains(err.Error(), "pagination stalled") {
			t.Fatalf("err = %v, want a stalled pagination error", err)
		}
		if ids != nil {
			t.Errorf("ids = %v, want nil on error", ids)
		}
	})

	t.Run("propagates the error starting the query", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":"object_not_found","message":"nope"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		if _, err := c.ViewOrder(context.Background(), "v-1"); err == nil {
			t.Fatal("err = nil, want the API error")
		}
	})

	t.Run("propagates the error reading a later page", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				w.Write([]byte(`{"id":"q-1","results":[{"id":"p-1"}],"has_more":true,"next_cursor":"c-1"}`))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"code":"internal_server_error","message":"nope"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv, WithMaxRetries(0))
		if _, err := c.ViewOrder(context.Background(), "v-1"); err == nil {
			t.Fatal("err = nil, want the API error")
		}
	})
}

func TestDataSourceOrder(t *testing.T) {
	t.Run("orders by the data source's first view", func(t *testing.T) {
		var queried string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/views" {
				w.Write([]byte(`{"results":[{"id":"v-1","type":"board"},{"id":"v-2","type":"table"}]}`))
				return
			}
			queried = r.URL.Path
			w.Write([]byte(`{"id":"q-1","results":[{"id":"p-2"},{"id":"p-1"}]}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		ids, err := c.DataSourceOrder(context.Background(), "ds-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if queried != "/views/v-1/queries" {
			t.Errorf("queried %q, want the first view", queried)
		}
		if strings.Join(ids, ",") != "p-2,p-1" {
			t.Errorf("ids = %v", ids)
		}
	})

	t.Run("orders nothing where the data source has no views", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/views" {
				t.Errorf("queried %s, want no query at all", r.URL.Path)
			}
			w.Write([]byte(`{"results":[]}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		ids, err := c.DataSourceOrder(context.Background(), "ds-1")
		if err != nil || ids != nil {
			t.Errorf("got %v, %v, want no order and no error", ids, err)
		}
	})

	t.Run("propagates the error listing the views", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"code":"restricted_resource","message":"nope"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		if _, err := c.DataSourceOrder(context.Background(), "ds-1"); err == nil {
			t.Fatal("err = nil, want the API error")
		}
	})
}

// orderStub is an OrderReader whose one call is a field.
type orderStub struct {
	ids   []string
	err   error
	asked []string
}

func (s *orderStub) DataSourceOrder(_ context.Context, id string) ([]string, error) {
	s.asked = append(s.asked, id)
	return s.ids, s.err
}

func TestPlanOrder(t *testing.T) {
	t.Run("reads the order the project's own board puts its slices in", func(t *testing.T) {
		stub := &orderStub{ids: []string{"p-2", "p-1"}}
		got := PlanOrder(context.Background(), stub, "sl-ds")
		if strings.Join(got, ",") != "p-2,p-1" {
			t.Errorf("order = %v", got)
		}
		if len(stub.asked) != 1 || stub.asked[0] != "sl-ds" {
			t.Errorf("asked %v, want the slices data source", stub.asked)
		}
	})

	t.Run("falls back to no order when the read fails", func(t *testing.T) {
		stub := &orderStub{err: errors.New("boom")}
		if got := PlanOrder(context.Background(), stub, "sl-ds"); got != nil {
			t.Errorf("order = %v, want none", got)
		}
	})
}
