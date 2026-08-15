package notion

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateDatabase(t *testing.T) {
	t.Run("sends the initial data source schema", func(t *testing.T) {
		var gotMethod, gotPath, gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Write([]byte(`{
				"id":"db-1",
				"url":"https://notion.so/db-1",
				"title":[{"plain_text":"Slices"}],
				"data_sources":[{"id":"ds-1","name":"Slices"}]
			}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		db, err := c.CreateDatabase(context.Background(), "page-1", "Slices", map[string]PropertySchema{
			"Name":      SchemaTitle(),
			"Status":    SchemaSelect("Todo", "In progress", "Done"),
			"Milestone": SchemaSelect(),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if gotMethod != http.MethodPost || gotPath != "/databases" {
			t.Errorf("got %s %s, want POST /databases", gotMethod, gotPath)
		}
		want := `{"initial_data_source":{"properties":{` +
			`"Milestone":{"select":{}},` +
			`"Name":{"title":{}},` +
			`"Status":{"select":{"options":[{"name":"Todo"},{"name":"In progress"},{"name":"Done"}]}}}},` +
			`"parent":{"page_id":"page-1","type":"page_id"},` +
			`"title":[{"type":"text","text":{"content":"Slices"}}]}`
		if gotBody != want {
			t.Errorf("request body =\n%s\nwant\n%s", gotBody, want)
		}

		if db.ID != "db-1" || db.URL != "https://notion.so/db-1" {
			t.Errorf("got %+v", db)
		}
		if db.TitleText() != "Slices" {
			t.Errorf("TitleText() = %q", db.TitleText())
		}
		id, ok := db.DataSourceID()
		if !ok || id != "ds-1" {
			t.Errorf("DataSourceID() = %q, %v; want ds-1, true", id, ok)
		}
	})

	t.Run("propagates an API error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"code":"validation_error","message":"missing title"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		db, err := c.CreateDatabase(context.Background(), "page-1", "Slices", nil)
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("got %v, want *APIError", err)
		}
		if db != nil {
			t.Errorf("database = %+v, want nil on error", db)
		}
	})
}

func TestGetDatabase(t *testing.T) {
	t.Run("fetches by id", func(t *testing.T) {
		var gotMethod, gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			w.Write([]byte(`{"id":"db-1","data_sources":[{"id":"ds-1"},{"id":"ds-2"}],` +
				`"parent":{"type":"page_id","page_id":"page-1"}}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		db, err := c.GetDatabase(context.Background(), "db 1/x")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotMethod != http.MethodGet {
			t.Errorf("method = %s", gotMethod)
		}
		if gotPath != "/databases/db 1/x" {
			// PathEscape leaves the escaping to the transport; the server sees
			// the decoded path, so what matters is that it round-trips.
			t.Errorf("path = %q", gotPath)
		}
		if id, ok := db.DataSourceID(); !ok || id != "ds-1" {
			t.Errorf("DataSourceID() = %q, %v; want the first data source", id, ok)
		}
		if db.Parent.PageID != "page-1" {
			t.Errorf("Parent = %+v, want the page the database lives under", db.Parent)
		}
	})

	t.Run("reports a database with no data sources", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"id":"db-1"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		db, err := c.GetDatabase(context.Background(), "db-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id, ok := db.DataSourceID(); ok || id != "" {
			t.Errorf("DataSourceID() = %q, %v; want \"\", false", id, ok)
		}
		if db.TitleText() != "" {
			t.Errorf("TitleText() = %q, want empty", db.TitleText())
		}
	})

	t.Run("propagates a not-found error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":"object_not_found","message":"nope"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		db, err := c.GetDatabase(context.Background(), "db-1")
		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.NotFound() {
			t.Fatalf("got %v, want a not-found *APIError", err)
		}
		if db != nil {
			t.Errorf("database = %+v, want nil on error", db)
		}
	})
}

