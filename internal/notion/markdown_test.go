package notion

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the markdown golden files")

// TestMarkdownGolden renders every testdata/markdown/*.json fixture and
// compares it with the .md file beside it.
func TestMarkdownGolden(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join("testdata", "markdown", "*.json"))
	if err != nil {
		t.Fatalf("globbing fixtures: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no fixtures found")
	}

	for _, fixture := range fixtures {
		name := strings.TrimSuffix(filepath.Base(fixture), ".json")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatalf("reading %s: %v", fixture, err)
			}
			var blocks []Block
			if err := json.Unmarshal(raw, &blocks); err != nil {
				t.Fatalf("decoding %s: %v", fixture, err)
			}

			got := Markdown(blocks)
			golden := strings.TrimSuffix(fixture, ".json") + ".md"
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatalf("writing %s: %v", golden, err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("reading %s: %v", golden, err)
			}
			if got != string(want) {
				t.Errorf("Markdown(%s) =\n%s\nwant\n%s", name, got, want)
			}
		})
	}
}

func TestMarkdown(t *testing.T) {
	t.Run("renders nothing for no blocks", func(t *testing.T) {
		if got := Markdown(nil); got != "" {
			t.Errorf("Markdown(nil) = %q, want empty", got)
		}
	})

	t.Run("renders nothing for blocks with no content", func(t *testing.T) {
		blocks := decodeBlocks(t, `[
			{"id":"p1","type":"paragraph","paragraph":{"rich_text":[]}},
			{"id":"p2","type":"paragraph","paragraph":{"rich_text":[]}}
		]`)
		if got := Markdown(blocks); got != "" {
			t.Errorf("Markdown = %q, want empty", got)
		}
	})

	t.Run("keeps the children of an empty paragraph", func(t *testing.T) {
		blocks := decodeBlocks(t, `[{
			"id":"p1","type":"paragraph","has_children":true,"paragraph":{"rich_text":[]},
			"children":[{"id":"p2","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"Kept."}]}}]
		}]`)
		if got, want := Markdown(blocks), "  Kept.\n"; got != want {
			t.Errorf("Markdown = %q, want %q", got, want)
		}
	})

	t.Run("drops a child that renders to nothing", func(t *testing.T) {
		blocks := decodeBlocks(t, `[{
			"id":"b1","type":"bulleted_list_item","has_children":true,
			"bulleted_list_item":{"rich_text":[{"plain_text":"Only me"}]},
			"children":[{"id":"p1","type":"paragraph","paragraph":{"rich_text":[]}}]
		}]`)
		if got, want := Markdown(blocks), "- Only me\n"; got != want {
			t.Errorf("Markdown = %q, want %q", got, want)
		}
	})

	t.Run("stops descending at the depth cap", func(t *testing.T) {
		// One block per level, one level deeper than the cap allows.
		body := `{"id":"deepest","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"Too deep."}]}}`
		for i := 0; i < MaxMarkdownDepth; i++ {
			body = `{"id":"b","type":"bulleted_list_item","has_children":true,` +
				`"bulleted_list_item":{"rich_text":[{"plain_text":"Level"}]},"children":[` + body + `]}`
		}
		got := Markdown(decodeBlocks(t, "["+body+"]"))

		if strings.Contains(got, "Too deep.") {
			t.Errorf("Markdown descended past the cap:\n%s", got)
		}
		if want := strings.Count(got, "Level"); want != MaxMarkdownDepth {
			t.Errorf("rendered %d levels, want %d:\n%s", want, MaxMarkdownDepth, got)
		}
	})

	t.Run("renders a block with a malformed payload as empty", func(t *testing.T) {
		blocks := decodeBlocks(t, `[
			{"id":"p1","type":"paragraph","paragraph":"not an object"},
			{"id":"p2","type":"paragraph"}
		]`)
		if got := Markdown(blocks); got != "" {
			t.Errorf("Markdown = %q, want empty", got)
		}
	})

	t.Run("names the type of an unsupported block", func(t *testing.T) {
		blocks := decodeBlocks(t, `[{"id":"x","type":"table_of_contents","table_of_contents":{}}]`)
		if got, want := Markdown(blocks), "*[unsupported: table_of_contents]*\n"; got != want {
			t.Errorf("Markdown = %q, want %q", got, want)
		}
	})

	t.Run("indents nested code and quotes under a list item", func(t *testing.T) {
		blocks := decodeBlocks(t, `[{
			"id":"b1","type":"bulleted_list_item","has_children":true,
			"bulleted_list_item":{"rich_text":[{"plain_text":"Run it"}]},
			"children":[
				{"id":"c1","type":"code","code":{"language":"sh","rich_text":[{"plain_text":"go test ./...\n"}]}},
				{"id":"q1","type":"quote","quote":{"rich_text":[{"plain_text":"Mind the race flag."}]}}
			]
		}]`)
		want := "- Run it\n  ```sh\n  go test ./...\n\n  ```\n\n  > Mind the race flag.\n"
		if got := Markdown(blocks); got != want {
			t.Errorf("Markdown =\n%q\nwant\n%q", got, want)
		}
	})

	t.Run("renders the children of a quote after it", func(t *testing.T) {
		blocks := decodeBlocks(t, `[{
			"id":"q1","type":"quote","has_children":true,
			"quote":{"rich_text":[{"plain_text":"Quoted."}]},
			"children":[{"id":"p1","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"Not quoted."}]}}]
		}]`)
		if got, want := Markdown(blocks), "> Quoted.\n\nNot quoted.\n"; got != want {
			t.Errorf("Markdown = %q, want %q", got, want)
		}
	})

	t.Run("separates lists of different types", func(t *testing.T) {
		blocks := decodeBlocks(t, `[
			{"id":"b1","type":"bulleted_list_item","bulleted_list_item":{"rich_text":[{"plain_text":"Bullet"}]}},
			{"id":"n1","type":"numbered_list_item","numbered_list_item":{"rich_text":[{"plain_text":"Number"}]}}
		]`)
		if got, want := Markdown(blocks), "- Bullet\n\n1. Number\n"; got != want {
			t.Errorf("Markdown = %q, want %q", got, want)
		}
	})
}

func TestInlineSpans(t *testing.T) {
	tests := []struct {
		name string
		span RichText
		want string
	}{
		{"plain", RichText{PlainText: "plain"}, "plain"},
		{"no annotations", RichText{PlainText: "plain", Annotations: &Annotations{}}, "plain"},
		{"bold", RichText{PlainText: "b", Annotations: &Annotations{Bold: true}}, "**b**"},
		{"italic", RichText{PlainText: "i", Annotations: &Annotations{Italic: true}}, "*i*"},
		{"code", RichText{PlainText: "c", Annotations: &Annotations{Code: true}}, "`c`"},
		{"strikethrough", RichText{PlainText: "s", Annotations: &Annotations{Strikethrough: true}}, "~~s~~"},
		{
			"nested innermost first",
			RichText{PlainText: "x", Annotations: &Annotations{Bold: true, Italic: true, Code: true, Strikethrough: true}},
			"~~***`x`***~~",
		},
		{"link", RichText{PlainText: "docs", Href: "https://example.com"}, "[docs](https://example.com)"},
		{
			"annotated link",
			RichText{PlainText: "docs", Href: "https://example.com", Annotations: &Annotations{Bold: true}},
			"[**docs**](https://example.com)",
		},
		{"blank text keeps its annotations off", RichText{PlainText: " ", Annotations: &Annotations{Bold: true}}, " "},
		{"empty", RichText{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inline([]RichText{tt.span}); got != tt.want {
				t.Errorf("inline(%+v) = %q, want %q", tt.span, got, tt.want)
			}
		})
	}
}

// decodeBlocks decodes a JSON block array, failing the test if it is invalid.
func decodeBlocks(t *testing.T, raw string) []Block {
	t.Helper()
	var blocks []Block
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		t.Fatalf("decoding blocks: %v", err)
	}
	return blocks
}
