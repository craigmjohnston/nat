package notion

import (
	"context"
	"net/http"
	"net/url"
)

// Sort directions accepted by data source queries.
const (
	SortAscending  = "ascending"
	SortDescending = "descending"
)

// TimestampCreated is the page timestamp a query can sort by, for data sources
// whose schema carries no ordering of its own.
const TimestampCreated = "created_time"

// Sort is one entry of a query's sort order. Set either Property (a schema
// property name) or Timestamp ("created_time" or "last_edited_time").
type Sort struct {
	Property  string `json:"property,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Direction string `json:"direction"`
}

// DataSource is one data source of a database: the schema its pages conform to.
type DataSource struct {
	ID         string                    `json:"id"`
	Name       string                    `json:"name"`
	Properties map[string]PropertySchema `json:"properties"`
}

// GetDataSource fetches a data source by ID, chiefly to verify its schema
// against what this app expects.
func (c *Client) GetDataSource(ctx context.Context, id string) (*DataSource, error) {
	var ds DataSource
	if err := c.do(ctx, http.MethodGet, "/data_sources/"+url.PathEscape(id), nil, &ds); err != nil {
		return nil, err
	}
	return &ds, nil
}

// QueryDataSource returns every page of a data source matching filter, in the
// given sort order, following pagination to the end. A nil filter or sorts is
// omitted from the request. Filters are passed through as Notion's filter
// object — the shapes are too variadic to be worth modelling.
func (c *Client) QueryDataSource(ctx context.Context, id string, filter map[string]any, sorts []Sort) ([]Page, error) {
	body := map[string]any{}
	if len(filter) > 0 {
		body["filter"] = filter
	}
	if len(sorts) > 0 {
		body["sorts"] = sorts
	}
	return paginate[Page](ctx, c, http.MethodPost, "/data_sources/"+url.PathEscape(id)+"/query", body)
}
