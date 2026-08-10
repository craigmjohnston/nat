package notion

import (
	"context"
	"encoding/json"
	"net/http"
)

// The object types accepted by SearchPaged as a filter. The onboarding picker
// browses with SearchPage — databases are reached by walking a page's content —
// and searches with SearchDataSource, which finds the thing being chosen
// directly rather than the page it happens to sit in.
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
	// Parent is where the hit lives. Onboarding keeps only the pages parented
	// by the workspace itself, which is also what rules out database rows.
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

// SearchPaged fetches one page of search results, returning the cursor for the
// page after it — "" once the search is exhausted. It exists instead of a
// follow-every-page search because the caller streams results into the UI as
// they arrive: a large workspace shows its first hits immediately rather than
// after the whole search has run. An empty query matches everything shared
// with the integration; pass SearchPage as filterType to get only pages.
func (c *Client) SearchPaged(ctx context.Context, query, filterType, startCursor string) ([]SearchResult, string, error) {
	body := map[string]any{}
	if query != "" {
		body["query"] = query
	}
	if filterType != "" {
		body["filter"] = map[string]any{"property": "object", "value": filterType}
	}
	if startCursor != "" {
		body["start_cursor"] = startCursor
	}
	var page List[SearchResult]
	if err := c.do(ctx, http.MethodPost, "/search", body, &page); err != nil {
		return nil, "", err
	}
	if !page.HasMore || page.NextCursor == nil {
		return page.Results, "", nil
	}
	return page.Results, *page.NextCursor, nil
}
