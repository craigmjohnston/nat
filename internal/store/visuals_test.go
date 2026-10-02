package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/craigmjohnston/nat/internal/notion"
)

// visuals is two images as an agent hands them in.
var visuals = []VisualChange{
	{Name: "Settings pane, dark palette", URI: "/Users/craig/shots/settings-dark.png"},
	{Name: "Settings pane, light palette", URI: "https://example.com/settings-light.png"},
}

func TestVisualChanges(t *testing.T) {
	section := "### Visual changes\n\n" + visualChangesMarkdown(visuals)
	tests := []struct {
		name string
		body string
		want []VisualChange
	}{
		{"none", "Do the thing.\n\n### Handed back\n\nDid it.", nil},
		{"one section", "Do the thing.\n\n" + section, []VisualChange{
			{1, visuals[0].Name, visuals[0].URI},
			{2, visuals[1].Name, visuals[1].URI},
		}},
		{"superseded", section + "\n\n### Visual changes\n\n1. Newer\n   /tmp/newer.png",
			[]VisualChange{{1, "Newer", "/tmp/newer.png"}}},
		{"a later heading ends the section",
			"### Visual changes\n\n1. A\n   /a.png\n\n### Handed back\n\n1. Not an image\n   /b.png",
			[]VisualChange{{1, "A", "/a.png"}}},
		{"a deeper heading inside the section is passed over",
			"### Visual changes\n\n#### Aside\n\n1. A\n   /a.png",
			[]VisualChange{{1, "A", "/a.png"}}},
		{"a fenced hash line is passed over whole",
			"### Visual changes\n\n1. A\n   /a.png\n\n```\n# not a heading\n### Visual changes\n1. Not an item\n   /no.png\n```\n\n2. B\n   /b.png",
			[]VisualChange{{1, "A", "/a.png"}, {2, "B", "/b.png"}}},
		{"an item with no URI line is skipped",
			"### Visual changes\n\n1. Nameless\n2. A\n   /a.png\n3. Cut off\nnot indented\n   /stray.png\n4. Fenced\n```\n/x\n```",
			[]VisualChange{{1, "A", "/a.png"}}},
		{"a blank line between name and URI is allowed",
			"### Visual changes\n\n1. A\n\n   /a.png",
			[]VisualChange{{1, "A", "/a.png"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VisualChanges(tt.body); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("VisualChanges() =\n%#v\nwant\n%#v", got, tt.want)
			}
		})
	}
}

func TestVisualChangesMarkdown(t *testing.T) {
	want := "1. Settings pane, dark palette\n   /Users/craig/shots/settings-dark.png\n" +
		"2. Settings pane, light palette\n   https://example.com/settings-light.png"
	if got := visualChangesMarkdown(visuals); got != want {
		t.Errorf("visualChangesMarkdown() =\n%s\nwant\n%s", got, want)
	}
}

func TestNotionRecordVisualsWritesOneAppend(t *testing.T) {
	api := &fakeAPI{}
	err := Over(api).RecordVisuals(context.Background(), "s5", []VisualChange{{Name: "A", URI: "/a.png"}})
	if err != nil {
		t.Fatalf("RecordVisuals() error = %v", err)
	}
	if len(api.appended) != 1 {
		t.Fatalf("appends = %d, want one", len(api.appended))
	}
	got, _ := json.Marshal(api.appended[0])
	want := `[{"heading_3":{"rich_text":[{"text":{"content":"Visual changes"},"type":"text"}]},"object":"block","type":"heading_3"},` +
		`{"numbered_list_item":{"children":[` +
		`{"object":"block","paragraph":{"rich_text":[{"text":{"content":"/a.png"},"type":"text"}]},"type":"paragraph"}],` +
		`"rich_text":[{"text":{"content":"A"},"type":"text"}]},"object":"block","type":"numbered_list_item"}]`
	if string(got) != want {
		t.Errorf("blocks =\n%s\nwant\n%s", got, want)
	}
}

func TestNotionRecordVisualsCarriesTheFailureUp(t *testing.T) {
	api := &fakeAPI{appendBlocks: func(string, []map[string]any) ([]notion.Block, error) { return nil, errBoom }}
	if err := Over(api).RecordVisuals(context.Background(), "s5", visuals); !errors.Is(err, errBoom) {
		t.Errorf("RecordVisuals err = %v, want the append's failure", err)
	}
}

// The same hand-ins, filed through each store, read back as the same body —
// the one thing keeping the hand-rolled local markdown and Notion's renderer
// agreeing.
func TestBothStoresFileTheSameVisuals(t *testing.T) {
	ctx := context.Background()
	many := make([]VisualChange, 0, 11)
	for i := 0; i < 11; i++ {
		many = append(many, VisualChange{Name: string(rune('A'+i)) + " pane", URI: "/shots/" + string(rune('a'+i)) + ".png"})
	}

	api := &fakeAPI{}
	remote := Over(api)
	for _, items := range [][]VisualChange{visuals, many} {
		if err := remote.RecordVisuals(ctx, "writes", items); err != nil {
			t.Fatalf("Notion RecordVisuals: %v", err)
		}
	}
	blocks := pageBlocks(t, `[{"type":"paragraph","paragraph":{"rich_text":[{"plain_text":"Do the thing."}]}}]`)
	for _, a := range api.appended {
		blocks = append(blocks, asRead(t, a)...)
	}
	fromNotion := notion.Markdown(blocks)

	l, _ := openPlan(t)
	fillPlan(t, l)
	write(t, l, `UPDATE slices SET body = ? WHERE id = ?`, "Do the thing.", "writes")
	for _, items := range [][]VisualChange{visuals, many} {
		if err := l.RecordVisuals(ctx, "writes", items); err != nil {
			t.Fatalf("Local RecordVisuals: %v", err)
		}
	}
	fromLocal, err := l.Body(ctx, "writes")
	if err != nil {
		t.Fatalf("Local Body: %v", err)
	}

	if want := fromLocal + "\n"; fromNotion != want {
		t.Errorf("Notion renders\n%s\nlocal holds\n%s", fromNotion, want)
	}
	if got := VisualChanges(fromLocal); len(got) != 11 || got[10] != (VisualChange{11, "K pane", "/shots/k.png"}) {
		t.Errorf("VisualChanges() = %#v, want the eleven of the newer section", got)
	}
}

func TestLocalRecordVisualsCarriesTheFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	if err := l.RecordVisuals(context.Background(), "ghost", visuals); err == nil {
		t.Error("RecordVisuals on a slice not in the plan: want an error")
	}
}

func TestMirroredRecordVisualsGoesLocallyThenPushes(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	if err := m.RecordVisuals(ctx, "writes", visuals); err != nil {
		t.Fatalf("RecordVisuals: %v", err)
	}
	if len(api.appended) != 1 {
		t.Errorf("appends = %d, want it pushed", len(api.appended))
	}
	body, _ := l.Body(ctx, "writes")
	if got := VisualChanges(body); len(got) != 2 {
		t.Errorf("local visuals = %#v, want the two filed", got)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

func TestMirroredRecordVisualsCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if err := m.RecordVisuals(context.Background(), "ghost", visuals); err == nil {
		t.Error("RecordVisuals on a slice not in the plan: want an error")
	}
}
