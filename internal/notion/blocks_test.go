package notion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestGetBlockChildren(t *testing.T) {
	t.Run("follows pagination and nests children", func(t *testing.T) {
		var gotPaths []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPaths = append(gotPaths, r.URL.RequestURI())
			switch r.URL.RequestURI() {
			case "/blocks/root/children":
				w.Write([]byte(`{
					"results":[{"id":"b1","type":"heading_1","has_children":false}],
					"has_more":true,"next_cursor":"cur-1"
				}`))
			case "/blocks/root/children?start_cursor=cur-1":
				w.Write([]byte(`{
					"results":[{"id":"b2","type":"bulleted_list_item","has_children":true}],
					"has_more":false,"next_cursor":null
				}`))
			case "/blocks/b2/children":
				w.Write([]byte(`{
					"results":[{"id":"b3","type":"paragraph","has_children":false}],
					"has_more":false,"next_cursor":null
				}`))
			default:
				t.Errorf("unexpected request %s", r.URL.RequestURI())
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		blocks, err := c.GetBlockChildren(context.Background(), "root")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(blocks) != 2 {
			t.Fatalf("got %d blocks, want 2: %+v", len(blocks), blocks)
		}
		if blocks[0].ID != "b1" || blocks[0].Type != "heading_1" || blocks[0].Children != nil {
			t.Errorf("first block = %+v", blocks[0])
		}
		if len(blocks[1].Children) != 1 || blocks[1].Children[0].ID != "b3" {
			t.Errorf("second block children = %+v", blocks[1].Children)
		}
		wantPaths := []string{
			"/blocks/root/children",
			"/blocks/root/children?start_cursor=cur-1",
			"/blocks/b2/children",
		}
		if len(gotPaths) != len(wantPaths) {
			t.Fatalf("requests = %v, want %v", gotPaths, wantPaths)
		}
		for i, want := range wantPaths {
			if gotPaths[i] != want {
				t.Errorf("request %d = %q, want %q", i, gotPaths[i], want)
			}
		}
	})

	t.Run("stops descending at the depth cap", func(t *testing.T) {
		// Every level claims another level below it, so only the cap ends the
		// recursion.
		var requests int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			if requests > MaxBlockDepth {
				t.Errorf("descended past the cap: request %d for %s", requests, r.URL.Path)
			}
			w.Write([]byte(`{
				"results":[{"id":"nested","type":"toggle","has_children":true}],
				"has_more":false,"next_cursor":null
			}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		blocks, err := c.GetBlockChildren(context.Background(), "root")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if requests != MaxBlockDepth {
			t.Errorf("made %d requests, want %d", requests, MaxBlockDepth)
		}

		// The deepest block fetched still reports children, but they are not
		// loaded.
		depth := 1
		b := blocks[0]
		for len(b.Children) > 0 {
			depth++
			b = b.Children[0]
		}
		if depth != MaxBlockDepth {
			t.Errorf("nesting depth = %d, want %d", depth, MaxBlockDepth)
		}
		if !b.HasChildren {
			t.Errorf("deepest block = %+v, want HasChildren true", b)
		}
	})

	t.Run("propagates an error from the first level", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":"object_not_found","message":"nope"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		blocks, err := c.GetBlockChildren(context.Background(), "root")
		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.NotFound() {
			t.Fatalf("got %v, want a not-found *APIError", err)
		}
		if blocks != nil {
			t.Errorf("blocks = %+v, want nil on error", blocks)
		}
	})

	t.Run("propagates an error from a nested level", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/blocks/root/children" {
				w.Write([]byte(`{"results":[{"id":"b1","has_children":true}],"has_more":false}`))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"code":"internal_server_error","message":"boom"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		blocks, err := c.GetBlockChildren(context.Background(), "root")
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusInternalServerError {
			t.Fatalf("got %v, want a 500 *APIError", err)
		}
		if blocks != nil {
			t.Errorf("blocks = %+v, want nil on error", blocks)
		}
	})
}

func TestPageEntries(t *testing.T) {
	t.Run("collects pages and databases, descending only into containers", func(t *testing.T) {
		// The page holds, in order: a child page, a paragraph with children (a
		// list — never descended into), a column layout hiding a database, and
		// a toggle hiding a page. Only the containers cost extra requests.
		var gotPaths []string
		responses := map[string]string{
			"/blocks/root/children": `{"results":[
				{"id":"page-a","type":"child_page","has_children":true,"child_page":{"title":"Briefs"}},
				{"id":"para","type":"paragraph","has_children":true},
				{"id":"cols","type":"column_list","has_children":true},
				{"id":"tog","type":"toggle","has_children":true},
				{"id":"empty-tog","type":"toggle","has_children":false}
			],"has_more":false}`,
			"/blocks/cols/children": `{"results":[
				{"id":"col-1","type":"column","has_children":true}
			],"has_more":false}`,
			"/blocks/col-1/children": `{"results":[
				{"id":"db-1","type":"child_database","has_children":false,"child_database":{"title":"Slices"}}
			],"has_more":false}`,
			"/blocks/tog/children": `{"results":[
				{"id":"page-b","type":"child_page","has_children":false,"child_page":{}}
			],"has_more":false}`,
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPaths = append(gotPaths, r.URL.RequestURI())
			body, ok := responses[r.URL.RequestURI()]
			if !ok {
				t.Errorf("unexpected request %s", r.URL.RequestURI())
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Write([]byte(body))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		entries, err := c.PageEntries(context.Background(), "root")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []PageEntry{
			{ID: "page-a", Title: "Briefs"},
			{ID: "db-1", Title: "Slices", Database: true},
			{ID: "page-b"},
		}
		if !reflect.DeepEqual(entries, want) {
			t.Errorf("entries = %+v, want %+v", entries, want)
		}
		wantPaths := []string{
			"/blocks/root/children",
			"/blocks/cols/children",
			"/blocks/col-1/children",
			"/blocks/tog/children",
		}
		if !reflect.DeepEqual(gotPaths, wantPaths) {
			t.Errorf("requests = %v, want %v — child pages and plain blocks must not be descended into", gotPaths, wantPaths)
		}
	})

	t.Run("follows pagination at every level", func(t *testing.T) {
		responses := map[string]string{
			"/blocks/root/children": `{"results":[
				{"id":"tog","type":"toggle","has_children":true}
			],"has_more":true,"next_cursor":"cur-1"}`,
			"/blocks/root/children?start_cursor=cur-1": `{"results":[
				{"id":"page-a","type":"child_page","has_children":false,"child_page":{"title":"After"}}
			],"has_more":false}`,
			"/blocks/tog/children": `{"results":[
				{"id":"db-1","type":"child_database","has_children":false,"child_database":{"title":"Inside"}}
			],"has_more":false}`,
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(responses[r.URL.RequestURI()]))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		entries, err := c.PageEntries(context.Background(), "root")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []PageEntry{
			{ID: "db-1", Title: "Inside", Database: true},
			{ID: "page-a", Title: "After"},
		}
		if !reflect.DeepEqual(entries, want) {
			t.Errorf("entries = %+v, want %+v", entries, want)
		}
	})

	t.Run("stops descending at the depth cap", func(t *testing.T) {
		// Every level is another container claiming children, so only the cap
		// ends the recursion.
		var requests int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			if requests > MaxBlockDepth {
				t.Errorf("descended past the cap: request %d for %s", requests, r.URL.Path)
			}
			w.Write([]byte(`{
				"results":[{"id":"nested","type":"toggle","has_children":true}],
				"has_more":false,"next_cursor":null
			}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		entries, err := c.PageEntries(context.Background(), "root")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if requests != MaxBlockDepth {
			t.Errorf("made %d requests, want %d", requests, MaxBlockDepth)
		}
		if len(entries) != 0 {
			t.Errorf("entries = %+v, want none from a page of bare toggles", entries)
		}
	})

	t.Run("propagates an error from the first level", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":"object_not_found","message":"nope"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		entries, err := c.PageEntries(context.Background(), "root")
		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.NotFound() {
			t.Fatalf("got %v, want a not-found *APIError", err)
		}
		if entries != nil {
			t.Errorf("entries = %+v, want nil on error", entries)
		}
	})

	t.Run("propagates an error from inside a container", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/blocks/root/children" {
				w.Write([]byte(`{"results":[{"id":"tog","type":"toggle","has_children":true}],"has_more":false}`))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"code":"internal_server_error","message":"boom"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		entries, err := c.PageEntries(context.Background(), "root")
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusInternalServerError {
			t.Fatalf("got %v, want a 500 *APIError", err)
		}
		if entries != nil {
			t.Errorf("entries = %+v, want nil on error", entries)
		}
	})
}

func TestBlockUnmarshalJSON(t *testing.T) {
	t.Run("keeps the payload named by the block type", func(t *testing.T) {
		var b Block
		raw := `{"id":"b1","type":"paragraph","has_children":true,"paragraph":{"rich_text":[]},"quote":{}}`
		if err := json.Unmarshal([]byte(raw), &b); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.ID != "b1" || b.Type != "paragraph" || !b.HasChildren {
			t.Errorf("block = %+v", b)
		}
		if got, want := string(b.payload), `{"rich_text":[]}`; got != want {
			t.Errorf("payload = %s, want %s", got, want)
		}
	})

	t.Run("leaves the payload empty when the type has none", func(t *testing.T) {
		var b Block
		if err := json.Unmarshal([]byte(`{"id":"b1","type":"divider"}`), &b); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.payload != nil {
			t.Errorf("payload = %s, want none", b.payload)
		}
	})

	t.Run("propagates a decode error", func(t *testing.T) {
		var b Block
		if err := json.Unmarshal([]byte(`{"id":42}`), &b); err == nil {
			t.Fatal("got nil error, want a decode error")
		}
	})
}

func TestAppendBlockChildren(t *testing.T) {
	t.Run("sends the blocks and returns them as created", func(t *testing.T) {
		var gotMethod, gotPath, gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Write([]byte(`{"results":[{"id":"new-1","type":"paragraph","has_children":false}],"has_more":false}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		blocks, err := c.AppendBlockChildren(context.Background(), "page-1", []map[string]any{{
			"type":      "paragraph",
			"paragraph": map[string]any{"rich_text": richText("done")},
		}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if gotMethod != http.MethodPatch || gotPath != "/blocks/page-1/children" {
			t.Errorf("got %s %s, want PATCH /blocks/page-1/children", gotMethod, gotPath)
		}
		want := `{"children":[{"paragraph":{"rich_text":[{"type":"text","text":{"content":"done"}}]},"type":"paragraph"}]}`
		if gotBody != want {
			t.Errorf("request body =\n%s\nwant\n%s", gotBody, want)
		}
		if len(blocks) != 1 || blocks[0].ID != "new-1" {
			t.Errorf("blocks = %+v", blocks)
		}
	})

	t.Run("propagates an API error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"code":"validation_error","message":"bad block"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		blocks, err := c.AppendBlockChildren(context.Background(), "page-1", nil)
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("got %v, want *APIError", err)
		}
		if blocks != nil {
			t.Errorf("blocks = %+v, want nil on error", blocks)
		}
	})
}

func TestDeleteBlock(t *testing.T) {
	t.Run("deletes by id", func(t *testing.T) {
		var gotMethod, gotPath string
		var gotBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			gotBody, _ = io.ReadAll(r.Body)
			w.Write([]byte(`{"id":"b1","in_trash":true}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		if err := c.DeleteBlock(context.Background(), "b 1/x"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if gotMethod != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", gotMethod)
		}
		if gotPath != "/blocks/b 1/x" {
			t.Errorf("path = %q", gotPath)
		}
		if len(gotBody) != 0 {
			t.Errorf("body = %q, want empty", gotBody)
		}
	})

	t.Run("propagates an API error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":"object_not_found","message":"nope"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		var apiErr *APIError
		if err := c.DeleteBlock(context.Background(), "b1"); !errors.As(err, &apiErr) || !apiErr.NotFound() {
			t.Fatalf("got %v, want a not-found *APIError", err)
		}
	})
}

func TestGetBlock(t *testing.T) {
	t.Run("returns the block and what holds it", func(t *testing.T) {
		var gotMethod, gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			w.Write([]byte(`{"id":"block-1","type":"column","parent":{"type":"page_id","page_id":"page-3"}}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		block, err := c.GetBlock(context.Background(), "block-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotMethod != http.MethodGet || gotPath != "/blocks/block-1" {
			t.Errorf("got %s %s, want GET /blocks/block-1", gotMethod, gotPath)
		}
		if block.Parent.Type != ParentPage || block.Parent.PageID != "page-3" {
			t.Errorf("parent = %+v, want the page that holds the block", block.Parent)
		}
	})

	t.Run("propagates an API error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":"object_not_found","message":"nope"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		block, err := c.GetBlock(context.Background(), "block-1")
		var apiErr *APIError
		if !errors.As(err, &apiErr) || block != nil {
			t.Fatalf("got %+v, %v, want nil and an *APIError", block, err)
		}
	})
}
