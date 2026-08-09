package notion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetDataSource(t *testing.T) {
	t.Run("returns the schema", func(t *testing.T) {
		var gotMethod, gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			w.Write([]byte(`{
				"id":"ds-1",
				"name":"Slices",
				"properties":{
					"Name":{"id":"title","name":"Name","type":"title","title":{}},
					"Status":{"id":"s","name":"Status","type":"status","status":{"options":[{"id":"1","name":"Todo","color":"gray"}]}},
					"Order":{"id":"o","name":"Order","type":"number","number":{}}
				}
			}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		ds, err := c.GetDataSource(context.Background(), "ds-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotMethod != http.MethodGet || gotPath != "/data_sources/ds-1" {
			t.Errorf("got %s %s, want GET /data_sources/ds-1", gotMethod, gotPath)
		}
		if ds.ID != "ds-1" || ds.Name != "Slices" || len(ds.Properties) != 3 {
			t.Fatalf("got %+v", ds)
		}
		if ds.Properties["Name"].Type != "title" || ds.Properties["Name"].Title == nil {
			t.Errorf("title property = %+v", ds.Properties["Name"])
		}
		if got := ds.Properties["Status"].OptionNames(); len(got) != 1 || got[0] != "Todo" {
			t.Errorf("status options = %v", got)
		}
		if ds.Properties["Order"].Number == nil {
			t.Error("number property config was not decoded")
		}
	})

	t.Run("propagates an API error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":"object_not_found","message":"nope"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		ds, err := c.GetDataSource(context.Background(), "ds-1")
		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.NotFound() {
			t.Fatalf("got %v, want a not-found *APIError", err)
		}
		if ds != nil {
			t.Errorf("data source = %+v, want nil on error", ds)
		}
	})
}

func TestQueryDataSource(t *testing.T) {
	t.Run("sends filter and sorts and decodes pages", func(t *testing.T) {
		var gotMethod, gotPath, gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Write([]byte(`{"results":[{
				"id":"page-1",
				"url":"https://notion.so/page-1",
				"created_time":"2026-08-09T15:01:41.000Z",
				"properties":{
					"Name":{"type":"title","title":[{"plain_text":"A slice"}]},
					"Status":{"type":"select","select":{"name":"Todo"}}
				}
			}],"has_more":false,"next_cursor":null}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		pages, err := c.QueryDataSource(context.Background(), "ds-1",
			map[string]any{"property": "Status", "select": map[string]any{"equals": "Todo"}},
			[]Sort{{Timestamp: "created_time", Direction: SortAscending}},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotMethod != http.MethodPost || gotPath != "/data_sources/ds-1/query" {
			t.Errorf("got %s %s", gotMethod, gotPath)
		}
		want := `{"filter":{"property":"Status","select":{"equals":"Todo"}},` +
			`"sorts":[{"timestamp":"created_time","direction":"ascending"}]}`
		if gotBody != want {
			t.Errorf("request body =\n%s\nwant\n%s", gotBody, want)
		}

		if len(pages) != 1 {
			t.Fatalf("pages = %+v", pages)
		}
		p := pages[0]
		if p.ID != "page-1" || p.URL != "https://notion.so/page-1" {
			t.Errorf("got %+v", p)
		}
		if want := time.Date(2026, 8, 9, 15, 1, 41, 0, time.UTC); !p.CreatedTime.Equal(want) {
			t.Errorf("CreatedTime = %v, want %v", p.CreatedTime, want)
		}
		if got := p.Properties["Name"].Text(); got != "A slice" {
			t.Errorf("Name = %q", got)
		}
		if got := p.Properties["Status"].SelectName(); got != "Todo" {
			t.Errorf("Status = %q", got)
		}
		if got := p.Properties["Missing"].SelectName(); got != "" {
			t.Errorf("a missing property should read as the zero value, got %q", got)
		}
	})

	t.Run("omits an empty filter and sorts", func(t *testing.T) {
		var gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Write([]byte(`{"results":[],"has_more":false}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		pages, err := c.QueryDataSource(context.Background(), "ds-1", nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotBody != `{}` {
			t.Errorf("request body = %s, want {}", gotBody)
		}
		if len(pages) != 0 {
			t.Errorf("pages = %+v, want none", pages)
		}
	})

	t.Run("follows pagination", func(t *testing.T) {
		var bodies []string
		var calls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			bodies = append(bodies, string(b))
			calls++
			if calls == 1 {
				json.NewEncoder(w).Encode(map[string]any{
					"results":     []map[string]any{{"id": "page-1"}},
					"has_more":    true,
					"next_cursor": "cursor-1",
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"results":  []map[string]any{{"id": "page-2"}},
				"has_more": false,
			})
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		pages, err := c.QueryDataSource(context.Background(), "ds-1", nil, []Sort{{Property: "Order", Direction: SortDescending}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pages) != 2 || pages[0].ID != "page-1" || pages[1].ID != "page-2" {
			t.Errorf("pages = %+v", pages)
		}
		if bodies[0] != `{"sorts":[{"property":"Order","direction":"descending"}]}` {
			t.Errorf("first body = %s", bodies[0])
		}
		if bodies[1] != `{"sorts":[{"property":"Order","direction":"descending"}],"start_cursor":"cursor-1"}` {
			t.Errorf("second body = %s", bodies[1])
		}
	})

	t.Run("propagates an API error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		pages, err := c.QueryDataSource(context.Background(), "ds-1", nil, nil)
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("got %v, want *APIError", err)
		}
		if pages != nil {
			t.Errorf("pages = %+v, want nil on error", pages)
		}
	})
}
