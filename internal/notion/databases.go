package notion

import (
	"context"
	"net/http"
	"net/url"
)

// DataSourceRef identifies one data source of a database, as listed on the
// database object.
type DataSourceRef struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Database is a Notion database: a container whose rows actually live in one or
// more data sources. Every query and page write in this app addresses a data
// source, so a database is only ever fetched to find its data source IDs.
type Database struct {
	ID          string          `json:"id"`
	URL         string          `json:"url"`
	Title       []RichText      `json:"title"`
	DataSources []DataSourceRef `json:"data_sources"`
	// Parent is where the database lives — a page, or the workspace itself —
	// which the onboarding picker uses to show databases in place.
	Parent Parent `json:"parent"`
}

// TitleText returns the database title as plain text.
func (d Database) TitleText() string { return PlainText(d.Title) }

// DataSourceID returns the ID of the database's first data source, reporting
// false when it has none. The databases this app creates have exactly one.
func (d Database) DataSourceID() (string, bool) {
	if len(d.DataSources) == 0 {
		return "", false
	}
	return d.DataSources[0].ID, true
}

// CreateDatabase creates a database as a child of a page, with one initial data
// source carrying the given property schema. Exactly one property must be a
// title.
func (c *Client) CreateDatabase(ctx context.Context, parentPageID, title string, properties map[string]PropertySchema) (*Database, error) {
	body := map[string]any{
		"parent":              map[string]any{"type": "page_id", "page_id": parentPageID},
		"title":               richText(title),
		"initial_data_source": map[string]any{"properties": properties},
	}
	var db Database
	if err := c.do(ctx, http.MethodPost, "/databases", body, &db); err != nil {
		return nil, err
	}
	return &db, nil
}

// GetDatabase fetches a database by ID.
func (c *Client) GetDatabase(ctx context.Context, id string) (*Database, error) {
	var db Database
	if err := c.do(ctx, http.MethodGet, "/databases/"+url.PathEscape(id), nil, &db); err != nil {
		return nil, err
	}
	return &db, nil
}

// SetDatabaseInline sets whether a database renders inline on its parent page —
// its rows shown on the page itself — rather than as a child page of its own.
func (c *Client) SetDatabaseInline(ctx context.Context, id string, inline bool) error {
	return c.do(ctx, http.MethodPatch, "/databases/"+url.PathEscape(id),
		map[string]any{"is_inline": inline}, nil)
}
