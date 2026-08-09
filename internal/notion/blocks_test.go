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
