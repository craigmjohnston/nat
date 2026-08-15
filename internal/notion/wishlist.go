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
	return WishlistOf(blocks), nil
}

// WishlistOf picks the wishlist items out of a page's top-level blocks — the
// same reading Wishlist does, for a caller that already has the page body.
func WishlistOf(blocks []Block) []WishlistItem {
	section, ok := FindWishlist(blocks)
	if !ok {
		return nil
	}
	var items []WishlistItem
	for _, b := range section.Blocks {
		if !isWishlistItem(b) || !hasText(b) {
			continue
		}
		items = append(items, WishlistItem{
			ID:       b.ID,
			Markdown: strings.TrimRight(Markdown([]Block{b}), "\n"),
		})
	}
	return items
}

// WishlistSection is a page's wishlist as an edit has to see it: the heading
// itself, and every top-level block under it. Blank bullets are kept, unlike in
// the items Wishlist reads, because a blank bullet is exactly what a cleared
// wishlist is left holding.
type WishlistSection struct {
	Heading Block
	Blocks  []Block
}

// FindWishlist picks the wishlist section out of a page's top-level blocks: the
// blocks between its Wishlist heading and the next heading of the same or
// higher level, so the rest of the page is untouched. The second return is
// false when the page has no Wishlist heading at all.
func FindWishlist(blocks []Block) (WishlistSection, bool) {
	level := 0
	var section WishlistSection
	for _, b := range blocks {
		h := headingLevel(b)
		if level == 0 {
			if h > 0 && strings.EqualFold(strings.TrimSpace(blockPlainText(b)), WishlistHeading) {
				level, section.Heading = h, b
			}
			continue
		}
		if h > 0 && h <= level {
			break
		}
		section.Blocks = append(section.Blocks, b)
	}
	return section, level > 0
}

// HasItem reports whether the ID names one of the section's top-level bullets,
// blank ones included — the blocks, and only the blocks, a wishlist clear may
// delete. The ID is matched the way IDs are everywhere here: case-insensitively,
// dashes ignored, so one copied out of a URL still names the block it came from.
func (s WishlistSection) HasItem(id string) bool {
	want := normalizeID(id)
	return slices.ContainsFunc(s.Blocks, func(b Block) bool {
		return isWishlistItem(b) && normalizeID(b.ID) == want
	})
}

// EmptyItemAfter says where a fresh empty bullet belongs once the removed
// blocks are gone: after the last block still standing in the section, or after
// the heading itself when nothing is. It reports false when the section already
// keeps a blank bullet, which is the state a clear is trying to reach — there
// is nothing to add then. removed holds block IDs, matched the way IDs are
// everywhere here: case-insensitively, dashes ignored.
func (s WishlistSection) EmptyItemAfter(removed []string) (after string, need bool) {
	gone := make(map[string]bool, len(removed))
	for _, id := range removed {
		gone[normalizeID(id)] = true
	}
	after = s.Heading.ID
	for _, b := range s.Blocks {
		if gone[normalizeID(b.ID)] {
			continue
		}
		if isWishlistItem(b) && !hasText(b) {
			return "", false
		}
		after = b.ID
	}
	return after, true
}

// EmptyItemBlock is the blank bullet a cleared wishlist is left with, ready for
// the next idea to be typed into.
func EmptyItemBlock() map[string]any {
	return map[string]any{
		"object": "block",
		"type":   "bulleted_list_item",
		"bulleted_list_item": map[string]any{
			"rich_text": []map[string]any{},
		},
	}
}

// isWishlistItem reports whether a block of the section is one of its entries.
func isWishlistItem(b Block) bool { return b.Type == "bulleted_list_item" }

// normalizeID is a block or page ID as it compares: Notion hands them back
// dashed and lowercase, and a URL or a hand-typed one may be neither.
func normalizeID(id string) string {
	return strings.ToLower(strings.ReplaceAll(id, "-", ""))
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
