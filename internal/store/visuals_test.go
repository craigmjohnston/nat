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

// vc is an item as read back from the only section of a body: changed, since
// nothing came before it, and with no hash or before.
func vc(index int, name, uri string) VisualChange {
	return VisualChange{Index: index, Name: name, URI: uri, Changed: true}
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
			vc(1, visuals[0].Name, visuals[0].URI),
			vc(2, visuals[1].Name, visuals[1].URI),
		}},
		{"superseded", section + "\n\n### Visual changes\n\n1. Newer\n   /tmp/newer.png",
			[]VisualChange{vc(1, "Newer", "/tmp/newer.png")}},
		{"an empty last section is none", section + "\n\n### Visual changes", nil},
		{"a later heading ends the section",
			"### Visual changes\n\n1. A\n   /a.png\n\n### Handed back\n\n1. Not an image\n   /b.png",
			[]VisualChange{vc(1, "A", "/a.png")}},
		{"a deeper heading inside the section is passed over",
			"### Visual changes\n\n#### Aside\n\n1. A\n   /a.png",
			[]VisualChange{vc(1, "A", "/a.png")}},
		{"a fenced hash line is passed over whole",
			"### Visual changes\n\n1. A\n   /a.png\n\n```\n# not a heading\n### Visual changes\n1. Not an item\n   /no.png\n```\n\n2. B\n   /b.png",
			[]VisualChange{vc(1, "A", "/a.png"), vc(2, "B", "/b.png")}},
		{"an item with no URI line is skipped",
			"### Visual changes\n\n1. Nameless\n2. A\n   /a.png\n3. Cut off\nnot indented\n   /stray.png\n4. Fenced\n```\n/x\n```",
			[]VisualChange{vc(1, "A", "/a.png")}},
		{"a blank line between name and URI is allowed",
			"### Visual changes\n\n1. A\n\n   /a.png",
			[]VisualChange{vc(1, "A", "/a.png")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VisualChanges(tt.body); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("VisualChanges() =\n%#v\nwant\n%#v", got, tt.want)
			}
		})
	}
}

// The labelled lines under a URI read into the item, in any order, and a line
// with no label it knows is passed over.
func TestVisualChangesReadsLabelledLines(t *testing.T) {
	body := "### Visual changes\n\n1. Pair\n   /after.png\n\n   Before sha256: bbb\n\n   sha256: aaa\n" +
		"   Something else: x\n\n   Before: /before.png\n2. Plain\n   https://example.com/p.png"
	want := []VisualChange{
		{Index: 1, Name: "Pair", URI: "/after.png", Hash: "aaa", BeforeURI: "/before.png", BeforeHash: "bbb", Changed: true},
		vc(2, "Plain", "https://example.com/p.png"),
	}
	if got := VisualChanges(body); !reflect.DeepEqual(got, want) {
		t.Errorf("VisualChanges() =\n%#v\nwant\n%#v", got, want)
	}
}

// An item of the last section is changed against the section before it: by
// name, then by hash where there is one and by URI where not, the before
// counted as part of it.
func TestVisualChangesDerivesChanged(t *testing.T) {
	prior := []VisualChange{
		{Name: "Same", URI: "/same.png", Hash: "1"},
		{Name: "Rerendered", URI: "/r.png", Hash: "1"},
		{Name: "New before", URI: "/nb.png", Hash: "1"},
		{Name: "Same URI", URI: "https://example.com/u.png"},
		{Name: "Moved URI", URI: "https://example.com/old.png"},
		{Name: "Same pair", URI: "/p.png", Hash: "1", BeforeURI: "/pb.png", BeforeHash: "2"},
		{Name: "Hashed now", URI: "/h.png"},
	}
	last := []VisualChange{
		{Name: "Same", URI: "/same.png", Hash: "1"},
		{Name: "Rerendered", URI: "/r.png", Hash: "2"},
		{Name: "New before", URI: "/nb.png", Hash: "1", BeforeURI: "/b.png", BeforeHash: "9"},
		{Name: "Same URI", URI: "https://example.com/u.png"},
		{Name: "Moved URI", URI: "https://example.com/new.png"},
		{Name: "Same pair", URI: "/p.png", Hash: "1", BeforeURI: "/pb.png", BeforeHash: "2"},
		{Name: "Hashed now", URI: "/h.png", Hash: "3"},
		{Name: "Brand new", URI: "/n.png", Hash: "1"},
	}
	body := "### Visual changes\n\n" + visualChangesMarkdown(prior) + "\n\n### Visual changes\n\n" + visualChangesMarkdown(last)
	want := map[string]bool{"Same": false, "Rerendered": true, "New before": true, "Same URI": false,
		"Moved URI": true, "Same pair": false, "Hashed now": true, "Brand new": true}
	got := VisualChanges(body)
	if len(got) != len(last) {
		t.Fatalf("VisualChanges() = %#v, want the %d of the last section", got, len(last))
	}
	for _, it := range got {
		if it.Changed != want[it.Name] {
			t.Errorf("%q changed = %v, want %v", it.Name, it.Changed, want[it.Name])
		}
	}

	// After an empty section — everything removed — every item is new again.
	body = "### Visual changes\n\n" + visualChangesMarkdown(prior) + "\n\n### Visual changes\n\n### Visual changes\n\n" +
		visualChangesMarkdown(prior[:1])
	if got := VisualChanges(body); len(got) != 1 || !got[0].Changed {
		t.Errorf("VisualChanges() after an empty section = %#v, want its one item changed", got)
	}
}

func TestVisualChangesMarkdown(t *testing.T) {
	want := "1. Settings pane, dark palette\n   /Users/craig/shots/settings-dark.png\n" +
		"2. Settings pane, light palette\n   https://example.com/settings-light.png"
	if got := visualChangesMarkdown(visuals); got != want {
		t.Errorf("visualChangesMarkdown() =\n%s\nwant\n%s", got, want)
	}
	pair := []VisualChange{{Name: "P", URI: "/a.png", Hash: "aa", BeforeURI: "/b.png", BeforeHash: "bb"}}
	want = "1. P\n   /a.png\n\n   sha256: aa\n\n   Before: /b.png\n\n   Before sha256: bb"
	if got := visualChangesMarkdown(pair); got != want {
		t.Errorf("visualChangesMarkdown(pair) =\n%s\nwant\n%s", got, want)
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

	many[3].Hash, many[3].BeforeURI, many[3].BeforeHash = "abc123", "/shots/d-before.png", "def456"
	many[4].BeforeURI = "https://example.com/e-before.png"
	rounds := [][]VisualChange{visuals, nil, many}

	api := &fakeAPI{}
	remote := Over(api)
	for _, items := range rounds {
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
	for _, items := range rounds {
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
	if got := VisualChanges(fromLocal); len(got) != 11 || got[10] != vc(11, "K pane", "/shots/k.png") {
		t.Errorf("VisualChanges() = %#v, want the eleven of the newer section", got)
	}
	if got, want := VisualChanges(fromLocal)[3], (VisualChange{Index: 4, Name: "D pane", URI: "/shots/d.png",
		Hash: "abc123", BeforeURI: "/shots/d-before.png", BeforeHash: "def456", Changed: true}); got != want {
		t.Errorf("the pair read back as %#v, want %#v", got, want)
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
