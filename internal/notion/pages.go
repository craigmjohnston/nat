package notion

import (
	"context"
	"net/http"
	"net/url"
)

// Parent says where a page lives, and — when read back off an object rather
// than sent — where any other object lives. Slices and milestones are rows of a
// data source; a project page is a child of an ordinary page; a data source
// hangs off its database.
// A page nested in a column or a toggle is parented by that block rather than
// by the page it is drawn on, so BlockID is a link the breadcrumb walk has to
// follow to get any further.
type Parent struct {
	Type         string `json:"type"`
	DataSourceID string `json:"data_source_id,omitempty"`
	PageID       string `json:"page_id,omitempty"`
	DatabaseID   string `json:"database_id,omitempty"`
	BlockID      string `json:"block_id,omitempty"`
}

// The parent types Notion reports on an object it hands back.
const (
	ParentWorkspace  = "workspace"
	ParentPage       = "page_id"
	ParentDatabase   = "database_id"
	ParentDataSource = "data_source_id"
	ParentBlock      = "block_id"
)

// DataSourceParent returns the parent for a page created as a row of a data
// source.
func DataSourceParent(id string) Parent {
	return Parent{Type: "data_source_id", DataSourceID: id}
}

// PageParent returns the parent for a page created as a child of another page.
func PageParent(id string) Parent {
	return Parent{Type: "page_id", PageID: id}
}

// CreatePage creates a page under parent. properties are keyed by schema
// property name — build the values with the New* constructors. children is the
// page's initial block content in Notion's block shape, and may be nil; block
// payloads are too variadic to be worth modelling.
func (c *Client) CreatePage(ctx context.Context, parent Parent, properties map[string]PropertyValue, children []map[string]any) (*Page, error) {
	body := map[string]any{
		"parent":     parent,
		"properties": properties,
	}
	if len(children) > 0 {
		body["children"] = children
	}
	var p Page
	if err := c.do(ctx, http.MethodPost, "/pages", body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetPage fetches a page by ID. Breadcrumb resolution is what needs it: a page
// found on a parent chain is fetched for its own title and for whatever holds
// it in turn.
func (c *Client) GetPage(ctx context.Context, id string) (*Page, error) {
	var p Page
	if err := c.do(ctx, http.MethodGet, "/pages/"+url.PathEscape(id), nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdatePageProperties writes the given properties to a page. Properties the
// map does not mention are left alone.
func (c *Client) UpdatePageProperties(ctx context.Context, pageID string, properties map[string]PropertyValue) (*Page, error) {
	body := map[string]any{"properties": properties}
	var p Page
	if err := c.do(ctx, http.MethodPatch, "/pages/"+url.PathEscape(pageID), body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// TrashPage moves a page to the trash. Notion has no hard delete for pages, so
// this is as far as deleting a slice goes.
func (c *Client) TrashPage(ctx context.Context, pageID string) error {
	body := map[string]any{"in_trash": true}
	return c.do(ctx, http.MethodPatch, "/pages/"+url.PathEscape(pageID), body, nil)
}
