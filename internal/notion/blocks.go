package notion

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// MaxBlockDepth caps how far GetBlockChildren descends into nested blocks. Page
// bodies here are briefs and summaries — lists inside lists inside toggles is
// already deeper than anything this app writes, and the cap keeps a pathological
// page from costing an unbounded number of requests.
const MaxBlockDepth = 4

// Block is one block of a page's content. Only the fields needed to walk the
// tree are modelled; the type-specific payload is kept raw and decoded by
// whoever renders it.
type Block struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	HasChildren bool    `json:"has_children"`
	Children    []Block `json:"children,omitempty"`

	// payload is the object Notion nests under the block's own type name —
	// {"type":"paragraph","paragraph":{...}} — kept raw because its shape
	// depends on Type.
	payload json.RawMessage
}

// UnmarshalJSON decodes a block and keeps the payload named by its type.
func (b *Block) UnmarshalJSON(data []byte) error {
	type plain Block // sheds this method, and the unexported payload with it
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	// The decode above succeeded, so data is an object or null — both of which
	// decode into a map too.
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(data, &fields)
	*b = Block(p)
	b.payload = fields[p.Type]
	return nil
}

// decodePayload unmarshals the block's type-specific payload into v. A block
// with no payload, or one whose payload does not fit v, leaves v as it was:
// rendering a malformed block as empty beats failing the whole page.
func (b Block) decodePayload(v any) {
	if len(b.payload) == 0 {
		return
	}
	_ = json.Unmarshal(b.payload, v)
}

// GetBlockChildren returns the children of a page or block, following
// pagination to the end and recursing into nested blocks up to MaxBlockDepth
// levels. Children below the cap are left unfetched — a block there keeps
// HasChildren true with no Children.
func (c *Client) GetBlockChildren(ctx context.Context, id string) ([]Block, error) {
	return c.blockChildren(ctx, id, MaxBlockDepth)
}

// blockChildren fetches one level of children and recurses while depth remains.
func (c *Client) blockChildren(ctx context.Context, id string, depth int) ([]Block, error) {
	blocks, err := paginate[Block](ctx, c, http.MethodGet, "/blocks/"+url.PathEscape(id)+"/children", nil)
	if err != nil {
		return nil, err
	}
	if depth <= 1 {
		return blocks, nil
	}
	for i := range blocks {
		if !blocks[i].HasChildren {
			continue
		}
		kids, err := c.blockChildren(ctx, blocks[i].ID, depth-1)
		if err != nil {
			return nil, err
		}
		blocks[i].Children = kids
	}
	return blocks, nil
}

// PageEntry is a child page or child (inline) database sitting in a page's
// content. Its ID is the page or database ID — for both block types Notion
// gives the block the ID of the thing it holds.
type PageEntry struct {
	ID       string
	Title    string
	Database bool
}

// containerBlockTypes are the block types that can hold child pages and
// databases without being pages themselves: columns, toggles, callouts and
// synced blocks. PageEntries descends into these — and only these — so a
// database laid out inside a column is still found without fetching the bodies
// of every subpage.
var containerBlockTypes = map[string]bool{
	"column_list":  true,
	"column":       true,
	"toggle":       true,
	"callout":      true,
	"synced_block": true,
}

// PageEntries lists the pages and databases living in a page's content, in
// document order. Container blocks are recursed into up to MaxBlockDepth
// levels; child pages are reported but never descended into, which is what
// keeps one call cheap on a large workspace.
func (c *Client) PageEntries(ctx context.Context, id string) ([]PageEntry, error) {
	return c.pageEntries(ctx, id, MaxBlockDepth)
}

// pageEntries collects one level's entries and recurses into containers while
// depth remains.
func (c *Client) pageEntries(ctx context.Context, id string, depth int) ([]PageEntry, error) {
	blocks, err := paginate[Block](ctx, c, http.MethodGet, "/blocks/"+url.PathEscape(id)+"/children", nil)
	if err != nil {
		return nil, err
	}
	var entries []PageEntry
	for _, b := range blocks {
		switch {
		case b.Type == "child_page" || b.Type == "child_database":
			var payload struct {
				Title string `json:"title"`
			}
			b.decodePayload(&payload)
			entries = append(entries, PageEntry{ID: b.ID, Title: payload.Title, Database: b.Type == "child_database"})
		case depth > 1 && b.HasChildren && containerBlockTypes[b.Type]:
			kids, err := c.pageEntries(ctx, b.ID, depth-1)
			if err != nil {
				return nil, err
			}
			entries = append(entries, kids...)
		}
	}
	return entries, nil
}

// AppendBlockChildren appends blocks to the end of a page or block's content
// and returns the blocks as created. children are raw Notion block objects.
func (c *Client) AppendBlockChildren(ctx context.Context, id string, children []map[string]any) ([]Block, error) {
	body := map[string]any{"children": children}
	var list List[Block]
	if err := c.do(ctx, http.MethodPatch, "/blocks/"+url.PathEscape(id)+"/children", body, &list); err != nil {
		return nil, err
	}
	return list.Results, nil
}

// DeleteBlock moves a block to the trash, which is how a page body is cleared
// before being rewritten.
func (c *Client) DeleteBlock(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/blocks/"+url.PathEscape(id), nil, nil)
}
