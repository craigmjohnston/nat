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
					"Status":{"id":"s","name":"Status","type":"select","select":{"options":[{"id":"1","name":"Todo","color":"gray"}]}},
					"Order":{"id":"o","name":"Order","type":"number","number":{}}
				},
				"parent":{"type":"database_id","database_id":"db-1"}
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
		if ds.Parent.DatabaseID != "db-1" {
			t.Errorf("Parent = %+v, want the database the rows belong to", ds.Parent)
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

	t.Run("sends the edited-since filter as Notion's timestamp filter", func(t *testing.T) {
		var gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Write([]byte(`{"results":[],"has_more":false}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		// 12:00:45 CEST: the filter goes out in UTC, cut to the top of the
		// minute — Notion records last_edited_time no finer, so an edit made
		// later in the stamp's own minute has to match too.
		since := time.Date(2026, 8, 15, 12, 0, 45, 0, time.FixedZone("CEST", 2*60*60))
		if _, err := c.QueryDataSource(context.Background(), "ds-1", EditedOnOrAfter(since), nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := `{"filter":{"last_edited_time":{"on_or_after":"2026-08-15T10:00:00Z"},` +
			`"timestamp":"last_edited_time"}}`
		if gotBody != want {
			t.Errorf("request body =\n%s\nwant\n%s", gotBody, want)
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
				_ = json.NewEncoder(w).Encode(map[string]any{
					"results":     []map[string]any{{"id": "page-1"}},
					"has_more":    true,
					"next_cursor": "cursor-1",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
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

func TestUpdateDataSourceProperties(t *testing.T) {
	t.Run("sends the property definitions and decodes the schema", func(t *testing.T) {
		var gotMethod, gotPath, gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Write([]byte(`{"id":"ds-1","name":"Slices","properties":{
				"Milestone":{"id":"m","name":"Milestone","type":"select","select":{"options":[
					{"id":"o1","name":"M1: Client","color":"blue"},{"id":"o2","name":"M4: Polish","color":"default"}]}}}}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		existing := PropertySchema{Type: TypeSelect, Select: &OptionsConfig{
			Options: []SelectOption{{ID: "o1", Name: "M1: Client", Color: "blue"}},
		}}
		appended, _ := existing.AppendedOptions("M4: Polish")
		ds, err := c.UpdateDataSourceProperties(context.Background(), "ds-1",
			map[string]PropertySchema{PropMilestone: appended})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotMethod != http.MethodPatch || gotPath != "/data_sources/ds-1" {
			t.Errorf("got %s %s, want PATCH /data_sources/ds-1", gotMethod, gotPath)
		}
		want := `{"properties":{"Milestone":{"select":{"options":[` +
			`{"id":"o1","name":"M1: Client","color":"blue"},{"name":"M4: Polish"}]}}}}`
		if gotBody != want {
			t.Errorf("request body =\n%s\nwant\n%s", gotBody, want)
		}
		if got := ds.Properties[PropMilestone].OptionNames(); len(got) != 2 || got[1] != "M4: Polish" {
			t.Errorf("options = %v, want the appended one back", got)
		}
	})

	t.Run("propagates an API error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"code":"validation_error","message":"nope"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		ds, err := c.UpdateDataSourceProperties(context.Background(), "ds-1",
			map[string]PropertySchema{PropMilestone: SchemaSelect("M4: Polish")})
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("got %v, want *APIError", err)
		}
		if ds != nil {
			t.Errorf("data source = %+v, want nil on error", ds)
		}
	})
}
