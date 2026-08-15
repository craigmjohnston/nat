package notion

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Sort directions accepted by data source queries.
const (
	SortAscending  = "ascending"
	SortDescending = "descending"
)

// TimestampCreated is the page timestamp a query can sort by, for data sources
// whose schema carries no ordering of its own. TimestampLastEdited is the one a
// query can filter by, for reloads that want only what changed.
const (
	TimestampCreated    = "created_time"
	TimestampLastEdited = "last_edited_time"
)

// EditedOnOrAfter is the query filter matching pages edited at or after t.
// Notion records last_edited_time only to the minute, so t is widened to the
// top of its minute: an edit made in the same minute as t but after it would
// otherwise never match a filter. What the widening re-fetches has been seen
// already, which a caller merging pages by ID absorbs.
func EditedOnOrAfter(t time.Time) map[string]any {
	return map[string]any{
		"timestamp": TimestampLastEdited,
		TimestampLastEdited: map[string]any{
			"on_or_after": t.UTC().Truncate(time.Minute).Format(time.RFC3339),
		},
	}
}

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
	// Parent is the database whose rows the data source holds, which is how the
	// migration finds the old Milestones database to trash from the data source
	// the relation names.
	Parent Parent `json:"parent"`
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

// UpdateDataSourceProperties writes property definitions to a data source's
// schema. A property the map does not name is left alone; one it does is
// replaced by what is given, which for a fixed-choice column means its whole
// option list — so the options to keep have to be sent along with any new ones.
func (c *Client) UpdateDataSourceProperties(ctx context.Context, id string, properties map[string]PropertySchema) (*DataSource, error) {
	body := map[string]any{"properties": properties}
	var ds DataSource
	if err := c.do(ctx, http.MethodPatch, "/data_sources/"+url.PathEscape(id), body, &ds); err != nil {
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
