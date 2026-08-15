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

// ListViews returns a data source's views, in the order Notion lists them —
// which is the order of the view tabs above the database.
func (c *Client) ListViews(ctx context.Context, dataSourceID string) ([]View, error) {
	return paginate[View](ctx, c, http.MethodGet,
		"/views?data_source_id="+url.QueryEscape(dataSourceID), nil)
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

// PlanOrder is the order a project's slices are meant to be worked in, for a
// project that says so by where the slices sit in its board rather than in a
// property: the order of the Slices data source's first view.
//
// A project keeping its milestones in a database of their own orders its plan
// by those milestones' Order, and its slices need no order of their own, so
// nothing is read for one. Neither does a failure to read the order stop
// anything: an unordered plan is still a plan, so the error is logged and the
// slices are left in the order they were queried in.
func PlanOrder(ctx context.Context, r OrderReader, shape SliceShape, slicesDSID string) []string {
	if shape.MilestonesRelated() {
		return nil
	}
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
