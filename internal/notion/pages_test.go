package notion

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreatePage(t *testing.T) {
	t.Run("creates a data source row with properties and children", func(t *testing.T) {
		var gotMethod, gotPath, gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Write([]byte(`{
				"id":"page-1",
				"url":"https://notion.so/page-1",
				"created_time":"2026-08-09T15:00:00.000Z",
				"properties":{"Name":{"type":"title","title":[{"plain_text":"Slice one"}]}}
			}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		page, err := c.CreatePage(context.Background(), DataSourceParent("ds-1"),
			map[string]PropertyValue{
				"Assignee": NewPeople("user-1", "user-2"),
				"Name":     NewTitle("Slice one"),
				"Status":   NewSelect("Todo"),
			},
			[]map[string]any{{
				"type":      "paragraph",
				"paragraph": map[string]any{"rich_text": richText("brief")},
			}},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if gotMethod != http.MethodPost || gotPath != "/pages" {
			t.Errorf("got %s %s, want POST /pages", gotMethod, gotPath)
		}
		want := `{"children":[{"paragraph":{"rich_text":[{"type":"text","text":{"content":"brief"}}]},"type":"paragraph"}],` +
			`"parent":{"type":"data_source_id","data_source_id":"ds-1"},` +
			`"properties":{` +
			`"Assignee":{"people":[{"id":"user-1"},{"id":"user-2"}]},` +
			`"Name":{"title":[{"type":"text","text":{"content":"Slice one"}}]},` +
			`"Status":{"select":{"name":"Todo"}}}}`
		if gotBody != want {
			t.Errorf("request body =\n%s\nwant\n%s", gotBody, want)
		}

		if page.ID != "page-1" || page.URL != "https://notion.so/page-1" {
			t.Errorf("got %+v", page)
		}
		if got := page.Properties["Name"].Text(); got != "Slice one" {
			t.Errorf("Name = %q", got)
		}
	})

	t.Run("omits children when there are none", func(t *testing.T) {
		var gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Write([]byte(`{"id":"page-1"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		if _, err := c.CreatePage(context.Background(), PageParent("parent-1"),
			map[string]PropertyValue{"title": NewTitle("Project")}, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := `{"parent":{"type":"page_id","page_id":"parent-1"},` +
			`"properties":{"title":{"title":[{"type":"text","text":{"content":"Project"}}]}}}`
		if gotBody != want {
			t.Errorf("request body =\n%s\nwant\n%s", gotBody, want)
		}
	})

	t.Run("propagates an API error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"code":"validation_error","message":"bad property"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		page, err := c.CreatePage(context.Background(), DataSourceParent("ds-1"), nil, nil)
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("got %v, want *APIError", err)
		}
		if page != nil {
			t.Errorf("page = %+v, want nil on error", page)
		}
	})
}

func TestUpdatePageProperties(t *testing.T) {
	t.Run("patches only the named properties", func(t *testing.T) {
		var gotMethod, gotPath, gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Write([]byte(`{"id":"page-1","properties":{"Status":{"type":"select","select":{"name":"Claimed"}}}}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		page, err := c.UpdatePageProperties(context.Background(), "page 1/x", map[string]PropertyValue{
			"Assignee":  NewPeople("user-1"),
			"Milestone": NewRelation("ms-1"),
			"Order":     NewNumber(3),
			"PR":        NewURL("https://github.com/o/r/pull/7"),
			"Repo":      NewRichText("/tmp/repo"),
			"Status":    NewSelect("Claimed"),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if gotMethod != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", gotMethod)
		}
		if gotPath != "/pages/page 1/x" {
			// PathEscape leaves the escaping to the transport; the server sees
			// the decoded path, so what matters is that it round-trips.
			t.Errorf("path = %q", gotPath)
		}
		want := `{"properties":{` +
			`"Assignee":{"people":[{"id":"user-1"}]},` +
			`"Milestone":{"relation":[{"id":"ms-1"}]},` +
			`"Order":{"number":3},` +
			`"PR":{"url":"https://github.com/o/r/pull/7"},` +
			`"Repo":{"rich_text":[{"type":"text","text":{"content":"/tmp/repo"}}]},` +
			`"Status":{"select":{"name":"Claimed"}}}}`
		if gotBody != want {
			t.Errorf("request body =\n%s\nwant\n%s", gotBody, want)
		}
		if got := page.Properties["Status"].SelectName(); got != "Claimed" {
			t.Errorf("Status = %q", got)
		}
	})

	t.Run("propagates a not-found error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":"object_not_found","message":"nope"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		page, err := c.UpdatePageProperties(context.Background(), "page-1", nil)
		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.NotFound() {
			t.Fatalf("got %v, want a not-found *APIError", err)
		}
		if page != nil {
			t.Errorf("page = %+v, want nil on error", page)
		}
	})
}

func TestTrashPage(t *testing.T) {
	t.Run("patches in_trash", func(t *testing.T) {
		var gotMethod, gotPath, gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Write([]byte(`{"id":"page-1","in_trash":true}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		if err := c.TrashPage(context.Background(), "page-1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if gotMethod != http.MethodPatch || gotPath != "/pages/page-1" {
			t.Errorf("got %s %s, want PATCH /pages/page-1", gotMethod, gotPath)
		}
		if want := `{"in_trash":true}`; gotBody != want {
			t.Errorf("request body = %s, want %s", gotBody, want)
		}
	})

	t.Run("propagates an API error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"code":"restricted_resource","message":"no"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		var apiErr *APIError
		if err := c.TrashPage(context.Background(), "page-1"); !errors.As(err, &apiErr) {
			t.Fatalf("got %v, want *APIError", err)
		}
	})
}

func TestGetPage(t *testing.T) {
	t.Run("returns the page with its title and parent", func(t *testing.T) {
		var gotMethod, gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			w.Write([]byte(`{
				"id":"page-1",
				"parent":{"type":"block_id","block_id":"block-9"},
				"properties":{
					"Tags":{"type":"select","select":{"name":"x"}},
					"Name":{"type":"title","title":[{"plain_text":"Engineering"}]}
				}
			}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		page, err := c.GetPage(context.Background(), "page-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotMethod != http.MethodGet || gotPath != "/pages/page-1" {
			t.Errorf("got %s %s, want GET /pages/page-1", gotMethod, gotPath)
		}
		if got := page.TitleText(); got != "Engineering" {
			t.Errorf("TitleText() = %q, want Engineering", got)
		}
		if page.Parent.Type != ParentBlock || page.Parent.BlockID != "block-9" {
			t.Errorf("parent = %+v, want the block parent", page.Parent)
		}
	})

	t.Run("an untitled page has no title text", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"id":"page-1","properties":{"Tags":{"type":"select","select":{"name":"x"}}}}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		page, err := c.GetPage(context.Background(), "page-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := page.TitleText(); got != "" {
			t.Errorf("TitleText() = %q, want empty", got)
		}
	})

	t.Run("propagates an API error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":"object_not_found","message":"nope"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		page, err := c.GetPage(context.Background(), "page-1")
		var apiErr *APIError
		if !errors.As(err, &apiErr) || page != nil {
			t.Fatalf("got %+v, %v, want nil and an *APIError", page, err)
		}
	})
}
