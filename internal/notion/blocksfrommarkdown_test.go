package notion

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// decodedBlocks round-trips the raw block maps BlocksFromMarkdown returns
// through JSON, the way they travel to and from Notion, so a test can inspect
// them as [Block] the way every reader here does. plain_text is filled in from
// each span's content, since that is what Notion itself computes for a plain
// text span and echoes back on a later read — a write built here never sets it
// itself.
func decodedBlocks(t *testing.T, raw []map[string]any) []Block {
	t.Helper()
	for _, b := range raw {
		if payload, ok := b[b["type"].(string)].(map[string]any); ok {
			if spans, ok := payload["rich_text"].([]map[string]any); ok {
				for _, s := range spans {
					if text, ok := s["text"].(map[string]any); ok {
						s["plain_text"] = text["content"]
					}
				}
			}
		}
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal blocks: %v", err)
	}
	var blocks []Block
	if err := json.Unmarshal(encoded, &blocks); err != nil {
		t.Fatalf("decode blocks: %v", err)
	}
	return blocks
}

// blockShape is a block reduced to what a test wants to assert: its type and
// its plain text, read the same way [renderBlock] reads it.
type blockShape struct {
	Type string
	Text string
}

func shapesOf(t *testing.T, raw []map[string]any) []blockShape {
	t.Helper()
	blocks := decodedBlocks(t, raw)
	shapes := make([]blockShape, len(blocks))
	for i, b := range blocks {
		shapes[i] = blockShape{Type: b.Type, Text: writtenText(b)}
	}
	return shapes
}

// writtenText joins the text of every span a written block holds, the way
// Notion itself would once it had filled plain_text in — which a block built
// for a write and decoded straight back never has, since only Notion computes
// it.
func writtenText(b Block) string {
	var payload blockText
	b.decodePayload(&payload)
	var out strings.Builder
	for _, span := range payload.RichText {
		if span.Text != nil {
			out.WriteString(span.Text.Content)
		}
	}
	return out.String()
}

func TestBlocksFromMarkdown(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want []blockShape
	}{
		{"empty", "", nil},
		{"blank", "  \n\n\t", nil},
		{"one paragraph", "Just the one.", []blockShape{{"paragraph", "Just the one."}}},
		{"two paragraphs", "First.\n\nSecond.", []blockShape{
			{"paragraph", "First."}, {"paragraph", "Second."},
		}},
		{"blank runs collapse", "First.\n\n\n\nSecond.", []blockShape{
			{"paragraph", "First."}, {"paragraph", "Second."},
		}},
		{"lines within a paragraph join with a newline", "First line.\nSecond line.", []blockShape{
			{"paragraph", "First line.\nSecond line."},
		}},
		{"crlf", "First.\r\n\r\nSecond.", []blockShape{
			{"paragraph", "First."}, {"paragraph", "Second."},
		}},
		{"bulleted list", "- a\n- b\n- c", []blockShape{
			{"bulleted_list_item", "a"}, {"bulleted_list_item", "b"}, {"bulleted_list_item", "c"},
		}},
		{"bulleted list with star markers", "* a\n* b", []blockShape{
			{"bulleted_list_item", "a"}, {"bulleted_list_item", "b"},
		}},
		{"numbered list ignores the written numbers", "1. a\n1. b\n5. c", []blockShape{
			{"numbered_list_item", "a"}, {"numbered_list_item", "b"}, {"numbered_list_item", "c"},
		}},
		{"headings", "# One\n## Two\n### Three", []blockShape{
			{"heading_1", "One"}, {"heading_2", "Two"}, {"heading_3", "Three"},
		}},
		{"a heading past level three flattens to heading_3", "#### Four", []blockShape{
			{"heading_3", "Four"},
		}},
		{"a heading with no space is not a heading", "#nope", []blockShape{
			{"paragraph", "#nope"},
		}},
		{"a structured brief", "What and where.\n\n## Acceptance\n\n- One thing.\n- Another thing.",
			[]blockShape{
				{"paragraph", "What and where."},
				{"heading_2", "Acceptance"},
				{"bulleted_list_item", "One thing."},
				{"bulleted_list_item", "Another thing."},
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shapesOf(t, BlocksFromMarkdown(tt.md))
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("blocks = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestBlocksFromMarkdownSplitsLongTextUnderTheRichTextLimit is the acceptance
// case a plan-apply used to fail on: a paragraph past Notion's 2,000-character
// rich_text limit is split across spans of the one block rather than refused.
func TestBlocksFromMarkdownSplitsLongTextUnderTheRichTextLimit(t *testing.T) {
	long := strings.Repeat("a", 3000)

	blocks := decodedBlocks(t, BlocksFromMarkdown(long))
	if len(blocks) != 1 {
		t.Fatalf("blocks = %d, want the one paragraph", len(blocks))
	}
	var payload blockText
	blocks[0].decodePayload(&payload)
	if len(payload.RichText) != 2 {
		t.Fatalf("spans = %d, want two spans under the limit", len(payload.RichText))
	}
	for _, span := range payload.RichText {
		if n := len([]rune(span.Text.Content)); n > maxRichTextLength {
			t.Errorf("span is %d runes, want at most %d", n, maxRichTextLength)
		}
	}
	if got := writtenText(blocks[0]); got != long {
		t.Errorf("rejoined spans = %q, want the original text back", got)
	}
}

// TestBlocksFromMarkdownRoundTripsThroughMarkdown is the acceptance case that
// nat info still reads a written body back as readable markdown: what
// BlocksFromMarkdown writes, Markdown reads back to the same text.
func TestBlocksFromMarkdownRoundTripsThroughMarkdown(t *testing.T) {
	tests := []string{
		"Just the one.",
		"First.\n\nSecond.",
		"# A heading\n\nA paragraph under it.\n\n- One\n- Two\n\n1. Alpha\n2. Beta",
	}
	for _, md := range tests {
		t.Run(md, func(t *testing.T) {
			blocks := decodedBlocks(t, BlocksFromMarkdown(md))
			if got := strings.TrimRight(Markdown(blocks), "\n"); got != md {
				t.Errorf("round trip = %q, want %q", got, md)
			}
		})
	}
}

func TestBlocksFromMarkdownEmptyBlockIsOneEmptySpan(t *testing.T) {
	spans := richTextSpans("")
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want exactly one", len(spans))
	}
}
