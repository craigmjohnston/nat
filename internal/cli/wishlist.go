package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// wishlist prints the pending wishlist items written on the active project's
// page: markdown for a person, and --json — text with the block ID beside it —
// for the workshop agent, whose next move is to clear the items it has turned
// into a plan.
func wishlist(ctx context.Context, args []string, env Env) error {
	asJSON, err := parseJSONFlag("wishlist", args)
	if err != nil {
		return err
	}

	cfg, _, err := env.activeProject()
	if err != nil {
		return err
	}
	client := env.NewClient(env.Tokens.Token)

	blocks, err := client.GetBlockChildren(ctx, cfg.ActiveProjectID)
	if err != nil {
		return fmt.Errorf("load project page: %w", err)
	}
	items := notion.WishlistOf(blocks)

	if asJSON {
		return writeWishlistJSON(env.Out, items)
	}
	_, err = io.WriteString(env.Out, wishlistMarkdown(items))
	return err
}

// wishlistJSON is the structured form of the wishlist: the items in document
// order, each with the block ID that addresses it.
type wishlistJSON struct {
	Items []wishlistItemJSON `json:"items"`
}

type wishlistItemJSON struct {
	ID       string `json:"id"`
	Markdown string `json:"markdown"`
}

// writeWishlistJSON encodes the items, indented and with an items array that is
// empty rather than null on an empty wishlist: a consumer should be able to
// range over it without checking first.
func writeWishlistJSON(out io.Writer, items []notion.WishlistItem) error {
	doc := wishlistJSON{Items: make([]wishlistItemJSON, 0, len(items))}
	for _, item := range items {
		doc.Items = append(doc.Items, wishlistItemJSON{ID: item.ID, Markdown: item.Markdown})
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// wishlistMarkdown renders the items as the page holds them, each already a
// bullet with whatever is nested under it.
func wishlistMarkdown(items []notion.WishlistItem) string {
	var b strings.Builder
	b.WriteString("# Wishlist\n\n")
	if len(items) == 0 {
		b.WriteString("_none_\n")
		return b.String()
	}
	for _, item := range items {
		fmt.Fprintf(&b, "%s\n", item.Markdown)
	}
	return b.String()
}

// wishlistClear trashes the named wishlist items and leaves the section holding
// a single empty bullet, ready for the next idea. It deletes exactly the blocks
// it was given and only those, never the section wholesale: an item typed while
// a workshop session is running has to survive that session's tidy-up. An ID
// that is not a wishlist item of the page — a block elsewhere on it, or one
// already trashed, which no longer reads as part of the section — is reported
// and left alone rather than failing the run.
func wishlistClear(ctx context.Context, args []string, env Env) error {
	ids, err := parseWishlistClearArgs(args)
	if err != nil {
		return err
	}

	cfg, _, err := env.activeProject()
	if err != nil {
		return err
	}
	client := env.NewClient(env.Tokens.Token)

	blocks, err := client.GetBlockChildren(ctx, cfg.ActiveProjectID)
	if err != nil {
		return fmt.Errorf("load project page: %w", err)
	}
	section, found := notion.FindWishlist(blocks)

	var trashed, unknown []string
	for _, id := range ids {
		if !found || !section.HasItem(id) {
			unknown = append(unknown, id)
			continue
		}
		if err := client.DeleteBlock(ctx, id); err != nil {
			return fmt.Errorf("trash wishlist item %s: %w", id, err)
		}
		trashed = append(trashed, id)
	}

	seeded := false
	if len(trashed) > 0 {
		if after, need := section.EmptyItemAfter(trashed); need {
			if _, err := client.AppendBlockChildrenAfter(ctx, cfg.ActiveProjectID, after,
				[]map[string]any{notion.EmptyItemBlock()}); err != nil {
				return fmt.Errorf("leave an empty wishlist item: %w", err)
			}
			seeded = true
		}
	}

	logging.Action("wishlist cleared", "trashed", len(trashed), "unknown", len(unknown), "seeded", seeded)
	_, err = io.WriteString(env.Out, clearMarkdown(trashed, unknown, seeded))
	return err
}

// parseWishlistClearArgs reads the block IDs off the command line, dropping the
// ones repeated: the same ID given twice would be a second delete of a block
// already gone, which Notion refuses.
func parseWishlistClearArgs(args []string) ([]string, error) {
	var ids []string
	seen := map[string]bool{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return nil, usageErrorf("wishlist-clear: unknown flag %q", arg)
		}
		if id := normalizeID(arg); !seen[id] {
			seen[id] = true
			ids = append(ids, arg)
		}
	}
	if len(ids) == 0 {
		return nil, usageErrorf("wishlist-clear: no block IDs given: name the items to clear, as `nat wishlist --json` lists them")
	}
	return ids, nil
}

// normalizeID matches block IDs the way Notion's own are written: lowercase,
// dashes ignored, so an ID copied out of a URL still names the block it came
// from.
func normalizeID(id string) string {
	return strings.ToLower(strings.ReplaceAll(id, "-", ""))
}

// clearMarkdown reports what the clear did, ID by ID, so an agent that named a
// stale block can see which one did not land.
func clearMarkdown(trashed, unknown []string, seeded bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Wishlist cleared\n\n%d %s trashed", len(trashed), plural("item", len(trashed)))
	if len(unknown) > 0 {
		fmt.Fprintf(&b, ", %d left alone", len(unknown))
	}
	b.WriteString(".\n")
	if seeded {
		b.WriteString("The wishlist is left with an empty bullet.\n")
	}
	if len(trashed) > 0 || len(unknown) > 0 {
		b.WriteString("\n")
	}
	for _, id := range trashed {
		fmt.Fprintf(&b, "- trashed: %s\n", id)
	}
	for _, id := range unknown {
		fmt.Fprintf(&b, "- not a wishlist item: %s\n", id)
	}
	return b.String()
}
