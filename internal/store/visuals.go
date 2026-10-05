package store

import (
	"context"
	"strconv"
	"strings"

	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// VisualChange is one item an agent rendered of what its slice changed: what
// it shows, and where it is — an absolute path, or a URI as given — with,
// optionally, the image it is best judged against (its before). Hash and
// BeforeHash are the sha256 of each file's bytes, taken at hand-in, and empty
// for any URI that is not a file on this machine. Index is its place in the
// Visual changes section it was read from, 1-based, and Changed whether it
// differs from the section before (see [VisualChanges]); both are ignored on a
// write, where the order given is the order filed.
type VisualChange struct {
	Index      int
	Name       string
	URI        string
	Hash       string
	BeforeURI  string
	BeforeHash string
	Changed    bool
}

// The labelled lines an item may carry under its URI, each a paragraph of its
// own. The URI stays the first indented line, so a reader that knows only name
// and URI still reads both.
const (
	visualHashLabel       = "sha256: "
	visualBeforeLabel     = "Before: "
	visualBeforeHashLabel = "Before sha256: "
)

// VisualChanges is the images handed in on a slice: the items of the last
// Visual changes section of its body. An item opens at a numbered line at the
// margin — its name — and its URI is the first non-empty line indented under
// it; the indented lines after that may label its hash, its before and its
// before's hash, and any other is passed over. An item with no URI line is
// passed over, and the ones after it keep counting from where the kept ones
// left off. A heading of the same or higher level ends the section, and a
// fenced block is passed over whole, as [PendingFollowUps] does. A section
// with no items — every image removed — reads as none.
//
// The last section wins because each hand-in files the whole set as it then
// stands. An item is Changed where the section before it has no item of that
// name, or has one showing a different image or a different before (by hash,
// or by URI where there is no hash); with no section before, every item is.
func VisualChanges(body string) []VisualChange {
	sections := visualSections(body)
	if len(sections) == 0 {
		return nil
	}
	items := sections[len(sections)-1]
	var prior map[string]VisualChange
	if len(sections) > 1 {
		prior = map[string]VisualChange{}
		for _, it := range sections[len(sections)-2] {
			prior[it.Name] = it
		}
	}
	for i, it := range items {
		was, ok := prior[it.Name]
		items[i].Changed = !ok || visualIdentity(was.URI, was.Hash) != visualIdentity(it.URI, it.Hash) ||
			visualIdentity(was.BeforeURI, was.BeforeHash) != visualIdentity(it.BeforeURI, it.BeforeHash)
	}
	return items
}

// visualIdentity is what tells one image from another: its hash where it has
// one, else its URI.
func visualIdentity(uri, hash string) string {
	if hash != "" {
		return "sha256:" + hash
	}
	return uri
}

// visualSections is every Visual changes section of body, in order, each the
// items it holds (nil for an empty one).
func visualSections(body string) [][]VisualChange {
	var sections [][]VisualChange
	in, level, fence := false, 0, ""
	// open is an item whose URI line is still to come; filed is one whose URI
	// has been read, and which the labelled lines under it add to.
	open, filed, name, indent := false, false, "", ""
	for _, line := range strings.Split(body, "\n") {
		f := fenceOf(line)
		if fence != "" {
			if f != "" && strings.HasPrefix(f, fence) {
				fence = ""
			}
			continue
		}
		if f != "" {
			fence, open, filed = f, false, false
			continue
		}
		trimmed := strings.TrimSpace(line)
		if in && (open || filed) && trimmed != "" && strings.HasPrefix(line, indent) {
			items := &sections[len(sections)-1]
			if open {
				// The first line indented under an open item is its URI,
				// whatever it looks like.
				*items = append(*items, VisualChange{Index: len(*items) + 1, Name: name, URI: trimmed})
				open, filed = false, true
				continue
			}
			labelVisual(&(*items)[len(*items)-1], trimmed)
			continue
		}
		if h, text := headingOf(line); h > 0 {
			if in && h <= level {
				in, open, filed = false, false, false
			}
			if strings.EqualFold(text, notion.VisualChangesHeading) {
				in, level, open, filed = true, h, false, false
				sections = append(sections, nil)
			}
			continue
		}
		if !in {
			continue
		}
		if m := numberedItem.FindStringSubmatch(line); m != nil {
			open, filed, name, indent = true, false, strings.TrimSpace(m[2]), strings.Repeat(" ", len(m[1])+2)
			continue
		}
		if trimmed != "" {
			open, filed = false, false
		}
	}
	return sections
}

// labelVisual reads one labelled line under an item's URI into it; a line
// with no label it knows is passed over.
func labelVisual(it *VisualChange, line string) {
	// The before's hash is tried before the before, whose label it opens with.
	if v, ok := strings.CutPrefix(line, visualBeforeHashLabel); ok {
		it.BeforeHash = strings.TrimSpace(v)
	} else if v, ok := strings.CutPrefix(line, visualBeforeLabel); ok {
		it.BeforeURI = strings.TrimSpace(v)
	} else if v, ok := strings.CutPrefix(line, visualHashLabel); ok {
		it.Hash = strings.TrimSpace(v)
	}
}

// visualLines is what an item holds under its name: its URI, then each
// labelled line it has, one paragraph each.
func visualLines(it VisualChange) []string {
	lines := []string{it.URI}
	if it.Hash != "" {
		lines = append(lines, visualHashLabel+it.Hash)
	}
	if it.BeforeURI != "" {
		lines = append(lines, visualBeforeLabel+it.BeforeURI)
	}
	if it.BeforeHash != "" {
		lines = append(lines, visualBeforeHashLabel+it.BeforeHash)
	}
	return lines
}

// visualChangesMarkdown is the Visual changes section's list as notion.Markdown
// renders the blocks [visualChangeBlocks] writes — a tight numbered list, each
// item's URI indented under its name to the width of its marker, and each
// labelled line after it a paragraph of its own — so a plan kept locally and
// one kept in Notion read back alike. A test holds the two to each other.
func visualChangesMarkdown(items []VisualChange) string {
	lines := make([]string, 0, len(items))
	for i, it := range items {
		marker := strconv.Itoa(i+1) + ". "
		indent := strings.Repeat(" ", len(marker))
		item := strings.TrimRight(marker+it.Name, " ")
		for j, l := range visualLines(it) {
			sep := "\n"
			if j > 0 {
				sep = "\n\n"
			}
			item += sep + indented(indent, l)
		}
		lines = append(lines, item)
	}
	return strings.Join(lines, "\n")
}

// visualChangeBlocks is the Visual changes section as Notion holds it: a
// heading, then one numbered item per image with its name as the item's text
// and its URI, then each labelled line, as paragraphs nested under it.
func visualChangeBlocks(items []VisualChange) []map[string]any {
	blocks := []map[string]any{textBlock("heading_3", notion.VisualChangesHeading)}
	for _, it := range items {
		b := textBlock("numbered_list_item", it.Name)
		var kids []map[string]any
		for _, l := range visualLines(it) {
			kids = append(kids, textBlock("paragraph", l))
		}
		b["numbered_list_item"].(map[string]any)["children"] = kids
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
