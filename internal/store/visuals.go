package store

import (
	"context"
	"strconv"
	"strings"

	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// VisualChange is one image an agent rendered of what its slice changed: what
// it shows, and where it is — an absolute path, or a URI as given. Index is its
// place in the Visual changes section it was read from, 1-based, and is ignored
// on a write, where the order given is the order filed.
type VisualChange struct {
	Index int
	Name  string
	URI   string
}

// VisualChanges is the images handed in on a slice: the items of the last
// Visual changes section of its body. An item opens at a numbered line at the
// margin — its name — and its URI is the first non-empty line indented under
// it; an item with no such line is passed over, and the ones after it keep
// counting from where the kept ones left off. A heading of the same or higher
// level ends the section, and a fenced block is passed over whole, as
// [PendingFollowUps] does.
//
// The last section wins because an agent hands in the full set each time: a
// later hand-in replaces an earlier one rather than adding to it.
func VisualChanges(body string) []VisualChange {
	var items []VisualChange
	in, level, fence := false, 0, ""
	open, name, indent := false, "", ""
	for _, line := range strings.Split(body, "\n") {
		f := fenceOf(line)
		if fence != "" {
			if f != "" && strings.HasPrefix(f, fence) {
				fence = ""
			}
			continue
		}
		if f != "" {
			fence, open = f, false
			continue
		}
		trimmed := strings.TrimSpace(line)
		// The first line indented under an open item is its URI, whatever it
		// looks like.
		if in && open && trimmed != "" && strings.HasPrefix(line, indent) {
			items = append(items, VisualChange{Index: len(items) + 1, Name: name, URI: trimmed})
			open = false
			continue
		}
		if h, text := headingOf(line); h > 0 {
			if in && h <= level {
				in, open = false, false
			}
			if strings.EqualFold(text, notion.VisualChangesHeading) {
				in, level, items, open = true, h, nil, false
			}
			continue
		}
		if !in {
			continue
		}
		if m := numberedItem.FindStringSubmatch(line); m != nil {
			open, name, indent = true, strings.TrimSpace(m[2]), strings.Repeat(" ", len(m[1])+2)
			continue
		}
		if trimmed != "" {
			open = false
		}
	}
	return items
}

// visualChangesMarkdown is the Visual changes section's list as notion.Markdown
// renders the blocks [visualChangeBlocks] writes — a tight numbered list, each
// item's URI indented under its name to the width of its marker — so a plan kept
// locally and one kept in Notion read back alike. A test holds the two to each
// other.
func visualChangesMarkdown(items []VisualChange) string {
	lines := make([]string, 0, len(items))
	for i, it := range items {
		marker := strconv.Itoa(i+1) + ". "
		lines = append(lines, strings.TrimRight(marker+it.Name, " ")+"\n"+
			indented(strings.Repeat(" ", len(marker)), it.URI))
	}
	return strings.Join(lines, "\n")
}

// visualChangeBlocks is the Visual changes section as Notion holds it: a
// heading, then one numbered item per image with its name as the item's text
// and its URI as a paragraph nested under it.
func visualChangeBlocks(items []VisualChange) []map[string]any {
	blocks := []map[string]any{textBlock("heading_3", notion.VisualChangesHeading)}
	for _, it := range items {
		b := textBlock("numbered_list_item", it.Name)
		b["numbered_list_item"].(map[string]any)["children"] = paragraphBlocks(it.URI)
		blocks = append(blocks, b)
	}
	return blocks
}

// RecordVisuals files the visual changes on the slice page under a heading of
// their own, in one append.
func (n *Notion) RecordVisuals(ctx context.Context, id string, items []VisualChange) error {
	if _, err := n.api.AppendBlockChildren(ctx, id, visualChangeBlocks(items)); err != nil {
		return err
	}
	logging.Action("visuals recorded", "slice", id, "count", len(items))
	return nil
}

// RecordVisuals appends the visual changes to the slice's body, in the markdown
// Notion would render the same section to.
func (l *Local) RecordVisuals(ctx context.Context, id string, items []VisualChange) error {
	if err := l.appendToBody(ctx, id, "file the visual changes", notion.VisualChangesHeading, visualChangesMarkdown(items)); err != nil {
		return err
	}
	logging.Action("visuals recorded", "slice", id, "count", len(items))
	return nil
}

// RecordVisuals files the visual changes locally, then pushes them to the
// workspace.
func (m *Mirrored) RecordVisuals(ctx context.Context, id string, items []VisualChange) error {
	if err := m.local.RecordVisuals(ctx, id, items); err != nil {
		return err
	}
	m.push(ctx, id, func() error { return m.remote.RecordVisuals(ctx, id, items) })
	return nil
}
