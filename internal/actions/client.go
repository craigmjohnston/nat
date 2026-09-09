package actions

import (
	"context"

	"github.com/craigmjohnston/nat/internal/notion"
)

// Client is what a launch or an approve needs of Notion: a slice page's own
// properties, to claim it and to record a pull request's URL and mark it
// Done, and the blocks under it, to read the description an agent filed at
// hand-back. It is narrower than the board's own NotionAPI — the whole
// client — because that is everything else the board reads and writes,
// none of which either flow touches.
type Client interface {
	GetPage(ctx context.Context, id string) (*notion.Page, error)
	UpdatePageProperties(ctx context.Context, pageID string, properties map[string]notion.PropertyValue) (*notion.Page, error)
	GetBlockChildren(ctx context.Context, id string) ([]notion.Block, error)
}
