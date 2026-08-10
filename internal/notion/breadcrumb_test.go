package notion

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// crumbServer answers the three endpoints a breadcrumb walk uses from a map of
// path to JSON body, recording every path asked for. A path with no entry is a
// 404, which is how a test makes one step of a chain unreachable.
func crumbServer(t *testing.T, bodies map[string]string) (*Client, *[]string) {
	t.Helper()
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		body, ok := bodies[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":"object_not_found","message":"nope"}`))
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c, _ := testClient(t, srv)
	return c, &asked
}

// titledPage is a page object carrying a title and the given parent JSON.
func titledPage(title, parent string) string {
	return fmt.Sprintf(`{"id":"x","parent":%s,"properties":{"Name":{"type":"title","title":[{"plain_text":%q}]}}}`,
		parent, title)
}

func TestBreadcrumbWalksToTheWorkspace(t *testing.T) {
	c, asked := crumbServer(t, map[string]string{
		"/databases/db-1": `{"id":"db-1","title":[{"plain_text":"Q3"}],` +
			`"parent":{"type":"page_id","page_id":"page-2"}}`,
		"/pages/page-2": titledPage("Projects", `{"type":"page_id","page_id":"page-1"}`),
		"/pages/page-1": titledPage("Engineering", `{"type":"workspace"}`),
	})

	got := c.Breadcrumb(context.Background(), Parent{Type: ParentDatabase, DatabaseID: "db-1"})
	want := []string{"Engineering", "Projects", "Q3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Breadcrumb = %v, want %v — outermost first", got, want)
	}
	if len(*asked) != 3 {
		t.Errorf("asked for %v, want one request per step", *asked)
	}
}

func TestBreadcrumbStepsThroughABlock(t *testing.T) {
	// A database laid out inside a column is parented by that column, not by
	// the page it is drawn on: without following the block the trail stops dead.
	c, _ := crumbServer(t, map[string]string{
		"/databases/db-1": `{"id":"db-1","title":[{"plain_text":"Tracker"}],` +
			`"parent":{"type":"block_id","block_id":"block-1"}}`,
		"/blocks/block-1": `{"id":"block-1","type":"column",` +
			`"parent":{"type":"block_id","block_id":"block-0"}}`,
		"/blocks/block-0": `{"id":"block-0","type":"column_list",` +
			`"parent":{"type":"page_id","page_id":"page-1"}}`,
		"/pages/page-1": titledPage("Home", `{"type":"workspace"}`),
	})

	got := c.Breadcrumb(context.Background(), Parent{Type: ParentDatabase, DatabaseID: "db-1"})
	want := []string{"Home", "Tracker"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Breadcrumb = %v, want %v — the columns contribute no segment", got, want)
	}
}

func TestBreadcrumbEndsAtAParentItCannotClimb(t *testing.T) {
	// A page that is a database row hangs off a data source, which the walk has
	// no way past — and must not fetch anything for.
	c, asked := crumbServer(t, map[string]string{
		"/pages/page-1": titledPage("A row", `{"type":"data_source_id","data_source_id":"ds-1"}`),
	})

	got := c.Breadcrumb(context.Background(), Parent{Type: ParentPage, PageID: "page-1"})
	if want := []string{"A row"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Breadcrumb = %v, want %v", got, want)
	}
	if len(*asked) != 1 {
		t.Errorf("asked for %v, want the walk to stop at the data source", *asked)
	}
}

func TestBreadcrumbIsEmptyAtTheWorkspaceRoot(t *testing.T) {
	c, asked := crumbServer(t, nil)

	if got := c.Breadcrumb(context.Background(), Parent{Type: ParentWorkspace}); len(got) != 0 {
		t.Errorf("Breadcrumb = %v, want nothing above a workspace-level object", got)
	}
	if len(*asked) != 0 {
		t.Errorf("asked for %v, want no requests", *asked)
	}
}

func TestBreadcrumbSkipsUntitledAncestors(t *testing.T) {
	c, _ := crumbServer(t, map[string]string{
		"/databases/db-1": `{"id":"db-1","title":[{"plain_text":"Tracker"}],` +
			`"parent":{"type":"page_id","page_id":"page-1"}}`,
		"/pages/page-1": `{"id":"page-1","parent":{"type":"workspace"},"properties":{}}`,
	})

	got := c.Breadcrumb(context.Background(), Parent{Type: ParentDatabase, DatabaseID: "db-1"})
	if want := []string{"Tracker"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Breadcrumb = %v, want %v — an untitled page has nothing to show", got, want)
	}
}

func TestBreadcrumbDegradesWhenAnAncestorCannotBeFetched(t *testing.T) {
	// The page above is shared with nobody, so the integration cannot read it.
	// What is known still gets drawn, behind an ellipsis for what is not.
	c, _ := crumbServer(t, map[string]string{
		"/databases/db-1": `{"id":"db-1","title":[{"plain_text":"Tracker"}],` +
			`"parent":{"type":"page_id","page_id":"hidden"}}`,
	})

	got := c.Breadcrumb(context.Background(), Parent{Type: ParentDatabase, DatabaseID: "db-1"})
	want := []string{BreadcrumbEllipsis, "Tracker"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Breadcrumb = %v, want %v", got, want)
	}
}

func TestBreadcrumbDegradesAtEveryKindOfStep(t *testing.T) {
	// Whichever step of the chain is unreachable, the walk stops there and says
	// so rather than dropping the segments it did resolve.
	tests := []struct {
		name   string
		bodies map[string]string
		want   []string
	}{
		{"the database itself", map[string]string{}, []string{BreadcrumbEllipsis}},
		{"a block on the way up", map[string]string{
			"/databases/db-1": `{"id":"db-1","title":[{"plain_text":"Tracker"}],` +
				`"parent":{"type":"block_id","block_id":"hidden"}}`,
		}, []string{BreadcrumbEllipsis, "Tracker"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := crumbServer(t, tt.bodies)

			got := c.Breadcrumb(context.Background(), Parent{Type: ParentDatabase, DatabaseID: "db-1"})
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Breadcrumb = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBreadcrumbStopsAtTheDepthCap(t *testing.T) {
	// Every page here is its own grandparent, so only the cap ends the walk.
	bodies := map[string]string{}
	for i := range MaxBreadcrumbDepth + 2 {
		bodies[fmt.Sprintf("/pages/page-%d", i)] = titledPage(
			fmt.Sprintf("P%d", i),
			fmt.Sprintf(`{"type":"page_id","page_id":"page-%d"}`, i+1))
	}
	c, asked := crumbServer(t, bodies)

	got := c.Breadcrumb(context.Background(), Parent{Type: ParentPage, PageID: "page-0"})
	if len(got) != MaxBreadcrumbDepth+1 || got[0] != BreadcrumbEllipsis {
		t.Errorf("Breadcrumb = %v, want %d segments behind an ellipsis", got, MaxBreadcrumbDepth)
	}
	if !strings.HasSuffix(strings.Join(got, "/"), "P1/P0") {
		t.Errorf("Breadcrumb = %v, want the nearest ancestors last", got)
	}
	if len(*asked) != MaxBreadcrumbDepth {
		t.Errorf("made %d requests, want the cap to hold at %d", len(*asked), MaxBreadcrumbDepth)
	}
}
