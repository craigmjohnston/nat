package notion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearch(t *testing.T) {
	t.Run("sends the query and object filter and decodes hits", func(t *testing.T) {
		var gotMethod, gotPath, gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Write([]byte(`{"results":[
				{"object":"data_source","id":"ds-1","url":"https://notion.so/ds-1","title":[{"plain_text":"Slices"}],
				 "parent":{"type":"database_id","database_id":"db-1"}},
				{"object":"page","id":"page-1","url":"https://notion.so/page-1",
				 "properties":{"Name":{"type":"title","title":[{"plain_text":"Projects"}]}}},
				{"object":"page","id":"page-2"}
			],"has_more":false,"next_cursor":null}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		hits, err := c.Search(context.Background(), "Slices", SearchDataSource)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotMethod != http.MethodPost || gotPath != "/search" {
			t.Errorf("got %s %s, want POST /search", gotMethod, gotPath)
		}
		want := `{"filter":{"property":"object","value":"data_source"},"query":"Slices"}`
		if gotBody != want {
			t.Errorf("request body =\n%s\nwant\n%s", gotBody, want)
		}

		if len(hits) != 3 {
			t.Fatalf("hits = %+v", hits)
		}
		if hits[0].Object != "data_source" || hits[0].ID != "ds-1" || hits[0].URL != "https://notion.so/ds-1" {
			t.Errorf("got %+v", hits[0])
		}
		if got := hits[0].TitleText(); got != "Slices" {
			t.Errorf("data source title = %q, want Slices", got)
		}
		if got := hits[0].Parent; got.Type != "database_id" || got.DatabaseID != "db-1" {
			t.Errorf("data source parent = %+v, want database db-1", got)
		}
		if got := hits[1].TitleText(); got != "Projects" {
			t.Errorf("page title = %q, want Projects", got)
		}
		if got := hits[2].TitleText(); got != "" {
			t.Errorf("untitled hit = %q, want empty", got)
		}
	})

	t.Run("omits an empty query and filter", func(t *testing.T) {
		var gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Write([]byte(`{"results":[],"has_more":false}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		hits, err := c.Search(context.Background(), "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotBody != `{}` {
			t.Errorf("request body = %s, want {}", gotBody)
		}
		if len(hits) != 0 {
			t.Errorf("hits = %+v, want none", hits)
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
					"results":     []map[string]any{{"object": "page", "id": "page-1"}},
					"has_more":    true,
					"next_cursor": "cursor-1",
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"results":  []map[string]any{{"object": "page", "id": "page-2"}},
				"has_more": false,
			})
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		hits, err := c.Search(context.Background(), "", SearchPage)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(hits) != 2 || hits[0].ID != "page-1" || hits[1].ID != "page-2" {
			t.Errorf("hits = %+v", hits)
		}
		if bodies[0] != `{"filter":{"property":"object","value":"page"}}` {
			t.Errorf("first body = %s", bodies[0])
		}
		if bodies[1] != `{"filter":{"property":"object","value":"page"},"start_cursor":"cursor-1"}` {
			t.Errorf("second body = %s", bodies[1])
		}
	})

	t.Run("propagates an API error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"code":"validation_error","message":"bad filter"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		hits, err := c.Search(context.Background(), "", "nonsense")
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
			t.Fatalf("got %v, want a 400 *APIError", err)
		}
		if hits != nil {
			t.Errorf("hits = %+v, want nil on error", hits)
		}
	})
}
