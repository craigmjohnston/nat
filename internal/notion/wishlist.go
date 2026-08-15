package notion

import (
	"context"
	"slices"
	"strings"
)

// WishlistHeading is the heading a project page's wishlist lives under. It is
// matched case-insensitively, at any heading level.
const WishlistHeading = "Wishlist"

// WishlistItem is one pending entry of a project page's wishlist: the ID of the
// top-level bullet, and the whole item — sub-bullets included — rendered as
// markdown. The ID is what a later edit addresses; the markdown is what a
// workshop agent reads.
type WishlistItem struct {
	ID       string
	Markdown string
}

// Wishlist returns the pending wishlist items on a page: the bulleted list
// items beneath its Wishlist heading, in document order. Collection stops at
// the next heading of the same or higher level, so the rest of the page is
// untouched, and blocks between the bullets that are not bullets themselves are
// skipped. Items with no text — the empty bullet a seeded wishlist carries —
// are dropped, so an empty wishlist and a page with no Wishlist heading at all
// both come back empty.
func (c *Client) Wishlist(ctx context.Context, pageID string) ([]WishlistItem, error) {
	blocks, err := c.GetBlockChildren(ctx, pageID)
	if err != nil {
		return nil, err
	}
	return wishlistOf(blocks), nil
}

// wishlistOf picks the wishlist items out of a page's top-level blocks.
func wishlistOf(blocks []Block) []WishlistItem {
	level := 0
	var items []WishlistItem
	for _, b := range blocks {
		h := headingLevel(b)
		if level == 0 {
			if h > 0 && strings.EqualFold(strings.TrimSpace(blockPlainText(b)), WishlistHeading) {
				level = h
			}
			continue
		}
		if h > 0 && h <= level {
			break
		}
		if b.Type != "bulleted_list_item" || !hasText(b) {
			continue
		}
		items = append(items, WishlistItem{
			ID:       b.ID,
			Markdown: strings.TrimRight(Markdown([]Block{b}), "\n"),
		})
	}
	return items
}

// headingLevel is the level of a heading block — 1 for heading_1, and so on —
// or 0 for anything that is not a heading.
func headingLevel(b Block) int {
	rest, ok := strings.CutPrefix(b.Type, "heading_")
	if !ok || len(rest) != 1 || rest[0] < '1' || rest[0] > '9' {
		return 0
	}
	return int(rest[0] - '0')
}

// blockPlainText is a block's own rich text, unstyled and without its children.
func blockPlainText(b Block) string {
	var t blockText
	b.decodePayload(&t)
	return PlainText(t.RichText)
}

// hasText reports whether a block or anything nested under it carries text.
func hasText(b Block) bool {
	if strings.TrimSpace(blockPlainText(b)) != "" {
		return true
	}
	return slices.ContainsFunc(b.Children, hasText)
}
