package notion

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/craigmjohnston/nat/internal/logging"
)

// viewQueryPageSize is how many rows one page of a view query returns. 100 is
// the API's maximum, so a plan of any size we expect comes back in one or two
// round trips.
const viewQueryPageSize = 100

// View is one saved view of a data source: what someone looking at the database
// in Notion actually sees, filters, sorts and all. Only the fields this app
// reads are modelled.
type View struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// The view types this app tells apart: the default table a database is born
// with, and the board a plan is arranged on.
const (
	ViewTypeTable = "table"
	ViewTypeBoard = "board"
)

// ListViews returns a data source's views, in the order Notion lists them —
// which is the order of the view tabs above the database. The list carries bare
// stubs, an ID and nothing else; a view's type is only readable from GetView.
func (c *Client) ListViews(ctx context.Context, dataSourceID string) ([]View, error) {
	return paginate[View](ctx, c, http.MethodGet,
		"/views?data_source_id="+url.QueryEscape(dataSourceID), nil)
}

// GetView fetches one view in full.
func (c *Client) GetView(ctx context.Context, id string) (*View, error) {
	var v View
	if err := c.do(ctx, http.MethodGet, "/views/"+url.PathEscape(id), nil, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// CreateBoardView creates a board view of a data source, grouped by the select
// property named. The groups sit in manual order, which for a select is its
// option order — so a board grouped by a Milestone column shows the plan in the
// plan's own order. The API addresses a new view by database and data source
// both, the database saying where it lives and the data source what it shows.
func (c *Client) CreateBoardView(ctx context.Context, databaseID, dataSourceID, name, groupPropertyID string) (*View, error) {
	body := map[string]any{
		"database_id":    databaseID,
		"data_source_id": dataSourceID,
		"name":           name,
		"type":           ViewTypeBoard,
		"configuration": map[string]any{
			"type": ViewTypeBoard,
			"group_by": map[string]any{
				"type":        TypeSelect,
				"property_id": groupPropertyID,
				"sort":        map[string]any{"type": "manual"},
			},
		},
	}
	var v View
	if err := c.do(ctx, http.MethodPost, "/views", body, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// DeleteView deletes a view. Unlike a block there is no trash to recover one
// from, so this app only ever deletes the empty default table a migration
// replaces with the board.
func (c *Client) DeleteView(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/views/"+url.PathEscape(id), nil, nil)
}

// viewQueryResult is a page of a view query: the rows as bare page stubs, plus
// — on the response to the POST that starts the query — the ID later pages are
// fetched under. Notion caches a query for fifteen minutes; each call here
// starts a fresh one, so the order returned is the order the view has now.
type viewQueryResult struct {
	List[Page]
	QueryID string `json:"id"`
}

// ViewOrder returns the IDs of a view's rows in the order the view puts them
// in: the vertical order someone reading that view in Notion sees, which for a
// view with no sorts of its own is the manual order they arranged by dragging.
// It is the only way to read that order — a row's position is not a property,
// and nothing in the API writes it.
//
// The rows come back as page stubs carrying nothing but their IDs, so this is
// an ordering to apply to pages already read, not a way to read them.
func (c *Client) ViewOrder(ctx context.Context, viewID string) ([]string, error) {
	path := "/views/" + url.PathEscape(viewID) + "/queries"

	var first viewQueryResult
	if err := c.do(ctx, http.MethodPost, path, map[string]any{"page_size": viewQueryPageSize}, &first); err != nil {
		return nil, err
	}
	ids, cursor := idsOf(first.Results), nextCursor(first.List)

	for cursor != "" {
		var page viewQueryResult
		next := path + "/" + url.PathEscape(first.QueryID) +
			"?page_size=" + fmt.Sprint(viewQueryPageSize) +
			"&start_cursor=" + url.QueryEscape(cursor)
		if err := c.do(ctx, http.MethodGet, next, nil, &page); err != nil {
			return nil, err
		}
		ids = append(ids, idsOf(page.Results)...)
		if got := nextCursor(page.List); got != cursor {
			cursor = got
			continue
		}
		return nil, fmt.Errorf("view %s: pagination stalled on cursor %q", viewID, cursor)
	}
	return ids, nil
}

// DataSourceOrder returns the IDs of a data source's pages in the order its
// first view puts them in, and nothing at all for a data source with no views —
// a database always has one, but a caller ordering by this must cope with a
// workspace that answers otherwise.
//
// The first view is taken because that is the one Notion opens: a project whose
// plan is a board is read the way its owner reads it. A view of a shape other
// than a board is used just the same — whatever it shows, top to bottom, is
// what the plan says.
func (c *Client) DataSourceOrder(ctx context.Context, dataSourceID string) ([]string, error) {
	views, err := c.ListViews(ctx, dataSourceID)
	if err != nil {
		return nil, err
	}
	if len(views) == 0 {
		return nil, nil
	}
	return c.ViewOrder(ctx, views[0].ID)
}

// OrderReader is the part of the client PlanOrder needs, so a caller holding
// its own narrower interface can pass it straight through.
type OrderReader interface {
	DataSourceOrder(ctx context.Context, dataSourceID string) ([]string, error)
}

// PlanOrder is the order a project's slices are meant to be worked in: where
// they sit in the Slices data source's first view, which is the plan as its
// author arranged it in Notion.
//
// A failure to read it does not stop anything: an unordered plan is still a
// plan, so the error is logged and the slices are left in the order they were
// queried in.
func PlanOrder(ctx context.Context, r OrderReader, slicesDSID string) []string {
	order, err := r.DataSourceOrder(ctx, slicesDSID)
	if err != nil {
		logging.Error("could not read the slice order from the board", "data_source", slicesDSID, "err", err)
		return nil
	}
	return order
}

// idsOf reduces page stubs to their IDs.
func idsOf(pages []Page) []string {
	ids := make([]string, len(pages))
	for i, p := range pages {
		ids[i] = p.ID
	}
	return ids
}

// nextCursor is the cursor to ask for the next page with, and "" when the list
// is finished.
func nextCursor[T any](list List[T]) string {
	if !list.HasMore || list.NextCursor == nil {
		return ""
	}
	return *list.NextCursor
}
