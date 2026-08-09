package notion

import (
	"context"
	"encoding/json"
	"net/http"
)

// Object types accepted by Search as a filter. Notion's search only filters on
// these two; databases are reached through their data sources.
const (
	SearchPage       = "page"
	SearchDataSource = "data_source"
)

// SearchResult is one hit from search. The endpoint returns mixed object types,
// so both title shapes are modelled: data sources carry a top-level title,
// pages carry theirs in the title property.
type SearchResult struct {
	Object string     `json:"object"`
	ID     string     `json:"id"`
	URL    string     `json:"url"`
	Title  []RichText `json:"title"`
	// Properties is left undecoded because its shape depends on the hit: a
	// page carries property *values*, a data source carries its *schema*.
	// The two collide — a relation is an array of IDs in a value and an object
	// in a schema — so decoding into either type fails on the other.
	Properties map[string]json.RawMessage `json:"properties"`
	// Parent is where the hit lives. Data source hits carry the database they
	// belong to, which onboarding records alongside the data source ID.
	Parent Parent `json:"parent"`
}

// TitleText returns the result's title as plain text, wherever Notion put it,
// and "" for an untitled object.
func (r SearchResult) TitleText() string {
	if len(r.Title) > 0 {
		return PlainText(r.Title)
	}
	// Only a page's property values carry a title; a data source schema puts an
	// empty config under the same key. Anything that does not decode as a title
	// value is therefore skipped rather than treated as an error.
	for _, raw := range r.Properties {
		var p struct {
			Title []RichText `json:"title"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			continue
		}
		if len(p.Title) > 0 {
			return PlainText(p.Title)
		}
	}
	return ""
}

// Search finds pages and data sources the integration has been given access to,
// following pagination to the end. An empty query matches everything shared
// with the integration; an empty filterType returns both object types. Pass
// SearchPage or SearchDataSource to narrow it — onboarding uses both, to pick
// the Project database and the page to create new databases under.
func (c *Client) Search(ctx context.Context, query, filterType string) ([]SearchResult, error) {
	body := map[string]any{}
	if query != "" {
		body["query"] = query
	}
	if filterType != "" {
		body["filter"] = map[string]any{"property": "object", "value": filterType}
	}
	return paginate[SearchResult](ctx, c, http.MethodPost, "/search", body)
}
