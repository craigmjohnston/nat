package notion

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// wishlistServer serves a page body from a map of block ID → children JSON. The
// page's own children are keyed "page".
func wishlistServer(t *testing.T, children map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/blocks/{id}/children", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		body, ok := children[id]
		if !ok {
			t.Errorf("unexpected request for children of %q", id)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"results":` + body + `,"has_more":false,"next_cursor":null}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func bullet(id, text string, hasChildren bool) string {
	return `{"id":"` + id + `","type":"bulleted_list_item","has_children":` +
		map[bool]string{true: "true", false: "false"}[hasChildren] +
		`,"bulleted_list_item":{"rich_text":[{"plain_text":"` + text + `"}]}}`
}

func heading(id string, level int, text string) string {
	typ := "heading_" + string(rune('0'+level))
	return `{"id":"` + id + `","type":"` + typ + `","has_children":false,"` + typ +
		`":{"rich_text":[{"plain_text":"` + text + `"}]}}`
}

func TestWishlist(t *testing.T) {
	t.Run("collects nested items under the heading and stops at the next one", func(t *testing.T) {
		srv := wishlistServer(t, map[string]string{
			"page": `[` +
				heading("h0", 1, "Conventions") + `,` +
				bullet("ignored", "not in the wishlist", false) + `,` +
				heading("h1", 2, "wishlist") + `,` +
				bullet("w1", "Local file storage", true) + `,` +
				`{"id":"note","type":"paragraph","has_children":false,"paragraph":{"rich_text":[{"plain_text":"aside"}]}},` +
				bullet("w2", "A newline in the status bar", false) + `,` +
				heading("h2", 2, "Notes") + `,` +
				bullet("after", "past the wishlist", false) +
				`]`,
			"w1": `[` + bullet("w1a", "JSONL?", true) + `]`,
			"w1a": `[` + bullet("w1a1", "easy to parse", false) + `]`,
		})

		c, _ := testClient(t, srv)
		got, err := c.Wishlist(context.Background(), "page")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []WishlistItem{
			{ID: "w1", Markdown: "- Local file storage\n  - JSONL?\n    - easy to parse"},
			{ID: "w2", Markdown: "- A newline in the status bar"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("wishlist =\n%#v\nwant\n%#v", got, want)
		}
	})

	t.Run("stops at a heading of a higher level", func(t *testing.T) {
		srv := wishlistServer(t, map[string]string{
			"page": `[` +
				heading("h1", 3, "Wishlist") + `,` +
				bullet("w1", "kept", false) + `,` +
				heading("h2", 1, "Elsewhere") + `,` +
				bullet("after", "dropped", false) +
				`]`,
		})

		c, _ := testClient(t, srv)
		got, err := c.Wishlist(context.Background(), "page")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].ID != "w1" {
			t.Errorf("wishlist = %#v, want just w1", got)
		}
	})

	t.Run("keeps collecting past a heading of a lower level", func(t *testing.T) {
		srv := wishlistServer(t, map[string]string{
			"page": `[` +
				heading("h1", 1, "Wishlist") + `,` +
				bullet("w1", "before", false) + `,` +
				heading("h2", 2, "Someday") + `,` +
				bullet("w2", "after", false) +
				`]`,
		})

		c, _ := testClient(t, srv)
		got, err := c.Wishlist(context.Background(), "page")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || got[0].ID != "w1" || got[1].ID != "w2" {
			t.Errorf("wishlist = %#v, want w1 and w2", got)
		}
	})

	t.Run("reads a blank-bullet-only wishlist as empty", func(t *testing.T) {
		srv := wishlistServer(t, map[string]string{
			"page": `[` +
				heading("h1", 2, "Wishlist") + `,` +
				`{"id":"blank","type":"bulleted_list_item","has_children":false,"bulleted_list_item":{"rich_text":[]}},` +
				bullet("spaces", "   ", false) +
				`]`,
		})

		c, _ := testClient(t, srv)
		got, err := c.Wishlist(context.Background(), "page")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("wishlist = %#v, want none", got)
		}
	})

	t.Run("keeps a blank bullet whose sub-bullets carry the text", func(t *testing.T) {
		srv := wishlistServer(t, map[string]string{
			"page": `[` +
				heading("h1", 2, "Wishlist") + `,` +
				`{"id":"blank","type":"bulleted_list_item","has_children":true,"bulleted_list_item":{"rich_text":[]}}` +
				`]`,
			"blank": `[` + bullet("kid", "the real text", false) + `]`,
		})

		c, _ := testClient(t, srv)
		got, err := c.Wishlist(context.Background(), "page")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []WishlistItem{{ID: "blank", Markdown: "-\n  - the real text"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("wishlist = %#v, want %#v", got, want)
		}
	})

	t.Run("reads a page with no wishlist heading as empty", func(t *testing.T) {
		srv := wishlistServer(t, map[string]string{
			"page": `[` +
				heading("h1", 2, "Conventions") + `,` +
				bullet("b1", "an item", false) + `,` +
				`{"id":"p1","type":"paragraph","has_children":false,"paragraph":{"rich_text":[{"plain_text":"Wishlist"}]}}` +
				`]`,
		})

		c, _ := testClient(t, srv)
		got, err := c.Wishlist(context.Background(), "page")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("wishlist = %#v, want none", got)
		}
	})

	t.Run("reads a heading followed by non-bullet content as empty", func(t *testing.T) {
		srv := wishlistServer(t, map[string]string{
			"page": `[` +
				heading("h1", 2, "Wishlist") + `,` +
				`{"id":"p1","type":"paragraph","has_children":false,"paragraph":{"rich_text":[{"plain_text":"nothing yet"}]}},` +
				`{"id":"t1","type":"to_do","has_children":false,"to_do":{"rich_text":[{"plain_text":"a task"}]}}` +
				`]`,
		})

		c, _ := testClient(t, srv)
		got, err := c.Wishlist(context.Background(), "page")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("wishlist = %#v, want none", got)
		}
	})

	t.Run("propagates a fetch error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":"object_not_found","message":"nope"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		if _, err := c.Wishlist(context.Background(), "page"); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestHeadingLevel(t *testing.T) {
	cases := []struct {
		typ  string
		want int
	}{
		{"heading_1", 1},
		{"heading_3", 3},
		{"paragraph", 0},
		{"heading_", 0},
		{"heading_0", 0},
		{"heading_10", 0},
	}
	for _, tc := range cases {
		if got := headingLevel(Block{Type: tc.typ}); got != tc.want {
			t.Errorf("headingLevel(%q) = %d, want %d", tc.typ, got, tc.want)
		}
	}
}

// sectionOf decodes a page body and picks its wishlist out, which is what every
// test of the section helpers starts from.
func sectionOf(t *testing.T, blocks ...string) WishlistSection {
	t.Helper()
	var page []Block
	if err := json.Unmarshal([]byte(`[`+strings.Join(blocks, ",")+`]`), &page); err != nil {
		t.Fatal(err)
	}
	section, ok := FindWishlist(page)
	if !ok {
		t.Fatal("no wishlist section found")
	}
	return section
}

func TestFindWishlistReportsAPageWithNoWishlist(t *testing.T) {
	var page []Block
	if err := json.Unmarshal([]byte(`[`+heading("h1", 2, "Conventions")+`]`), &page); err != nil {
		t.Fatal(err)
	}
	section, ok := FindWishlist(page)
	if ok {
		t.Errorf("found %#v, want no section", section)
	}
}

func TestWishlistSectionHasItem(t *testing.T) {
	section := sectionOf(t,
		heading("h1", 2, "Wishlist"),
		bullet("3bd38308-f654-8142-9534-d3d80043f35a", "an item", false),
		`{"id":"p1","type":"paragraph","has_children":false,"paragraph":{"rich_text":[{"plain_text":"an aside"}]}}`,
	)
	cases := map[string]bool{
		"3bd38308-f654-8142-9534-d3d80043f35a": true,
		"3BD38308F65481429534D3D80043F35A":     true,
		"p1":                                   false, // in the section, but not an item
		"h1":                                   false, // the heading itself
		"nowhere":                              false,
	}
	for id, want := range cases {
		if got := section.HasItem(id); got != want {
			t.Errorf("HasItem(%q) = %t, want %t", id, got, want)
		}
	}
}

func TestWishlistSectionEmptyItemAfter(t *testing.T) {
	blank := `{"id":"blank","type":"bulleted_list_item","has_children":false,"bulleted_list_item":{"rich_text":[]}}`
	aside := `{"id":"aside","type":"paragraph","has_children":false,"paragraph":{"rich_text":[{"plain_text":"an aside"}]}}`

	t.Run("lands under the last block still standing", func(t *testing.T) {
		section := sectionOf(t, heading("h1", 2, "Wishlist"), bullet("w1", "one", false), aside, bullet("w2", "two", false))

		after, need := section.EmptyItemAfter([]string{"w1", "W2"})

		if !need || after != "aside" {
			t.Errorf("EmptyItemAfter = %q, %t, want %q, true", after, need, "aside")
		}
	})

	t.Run("lands under the heading when the section is emptied", func(t *testing.T) {
		section := sectionOf(t, heading("h1", 2, "Wishlist"), bullet("w1", "one", false))

		after, need := section.EmptyItemAfter([]string{"w1"})

		if !need || after != "h1" {
			t.Errorf("EmptyItemAfter = %q, %t, want %q, true", after, need, "h1")
		}
	})

	t.Run("is not needed while a blank bullet survives", func(t *testing.T) {
		section := sectionOf(t, heading("h1", 2, "Wishlist"), bullet("w1", "one", false), blank)

		if after, need := section.EmptyItemAfter([]string{"w1"}); need {
			t.Errorf("EmptyItemAfter = %q, %t, want false: the section already has one", after, need)
		}
	})

	t.Run("is needed again once the blank bullet is one of the removed", func(t *testing.T) {
		section := sectionOf(t, heading("h1", 2, "Wishlist"), blank, bullet("w1", "one", false))

		after, need := section.EmptyItemAfter([]string{"blank"})

		if !need || after != "w1" {
			t.Errorf("EmptyItemAfter = %q, %t, want %q, true", after, need, "w1")
		}
	})
}

func TestEmptyItemBlockIsABulletWithNothingInIt(t *testing.T) {
	block := EmptyItemBlock()

	if block["type"] != "bulleted_list_item" {
		t.Errorf("type = %v, want a bulleted_list_item", block["type"])
	}
	payload, ok := block["bulleted_list_item"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v, want an object", block["bulleted_list_item"])
	}
	if spans, ok := payload["rich_text"].([]map[string]any); !ok || len(spans) != 0 {
		t.Errorf("rich_text = %#v, want no spans", payload["rich_text"])
	}
}
