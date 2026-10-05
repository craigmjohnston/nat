package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// proposals is three follow-ups as an agent hands them in: one brief of two
// paragraphs, one quoting a fenced shell session with a hash line in it, and
// one with a line of its own that looks like a heading.
var proposals = []FollowUp{
	{Title: "Persist the conversation split width per project",
		Brief: "The split's width lives under one AppStorage key.\n\nStore it per project."},
	{Title: "Render the emoji picker open in a gallery story",
		Brief: "No story shows it open:\n\n```\n# run the gallery\ngnat --story pr\n```"},
	{Title: "Remove the dead reply-threading code",
		Brief: "# not a heading\nNothing calls it."},
}

func TestPendingFollowUps(t *testing.T) {
	section := "### Follow-ups\n\n" + followUpsMarkdown(proposals)
	tests := []struct {
		name string
		body string
		want []FollowUp
	}{
		{"none", "Do the thing.\n\n### Handed back\n\nDid it.", nil},
		{"one section", "Do the thing.\n\n" + section, []FollowUp{
			{1, 1, proposals[0].Title, proposals[0].Brief},
			{1, 2, proposals[1].Title, proposals[1].Brief},
			{1, 3, proposals[2].Title, proposals[2].Brief},
		}},
		{"a later batch supersedes nothing", section + "\n\n### Follow-ups\n\n1. A newer one\n   Its brief.",
			[]FollowUp{
				{1, 1, proposals[0].Title, proposals[0].Brief},
				{1, 2, proposals[1].Title, proposals[1].Brief},
				{1, 3, proposals[2].Title, proposals[2].Brief},
				{2, 4, "A newer one", "Its brief."},
			}},
		{"partially triaged", section + "\n\n### Follow-ups triaged\n\n" +
			"- Queued: " + proposals[0].Title + " → https://notion.so/abc\n" +
			"- Dropped: " + proposals[2].Title + "\n- Something else entirely",
			[]FollowUp{{1, 1, proposals[1].Title, proposals[1].Brief}}},
		{"wholly triaged", section + "\n\n### Follow-ups triaged\n\n" +
			"- Queued: " + proposals[0].Title + " → abc\n" +
			"- Folded in: " + proposals[1].Title + "\n" +
			"- Dropped: " + proposals[2].Title, nil},
		{"a record before the section counts for nothing",
			"### Follow-ups triaged\n\n- Dropped: A\n\n### Follow-ups\n\n1. A\n   Brief.",
			[]FollowUp{{1, 1, "A", "Brief."}}},
		{"a later heading ends the section",
			"### Follow-ups\n\n1. A\n   Brief.\n\n### Handed back\n\n1. Not a follow-up",
			[]FollowUp{{1, 1, "A", "Brief."}}},
		{"a record ends at the next heading",
			"### Follow-ups\n\n1. A\n   Brief.\n\n### Follow-ups triaged\n\n### Notes\n\n- Dropped: A",
			[]FollowUp{{1, 1, "A", "Brief."}}},
		{"a deeper heading inside the section is passed over",
			"### Follow-ups\n\n#### Aside\n\n1. A\n   Brief.",
			[]FollowUp{{1, 1, "A", "Brief."}}},
		{"a fence at the margin is passed over whole",
			"### Follow-ups\n\n1. A\n   Brief.\n\n```\n### Follow-ups\n2. Not an item\n```\n\n2. B\n   More.",
			[]FollowUp{{1, 1, "A", "Brief."}, {1, 2, "B", "More."}}},
		{"a new section while an item is open",
			"### Follow-ups\n\n1. A\n   Brief.\n#### Follow-ups\n\n1. B\n   Other.",
			[]FollowUp{{1, 1, "A", "Brief."}, {2, 2, "B", "Other."}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PendingFollowUps(tt.body); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PendingFollowUps() =\n%#v\nwant\n%#v", got, tt.want)
			}
		})
	}
}

// Every Follow-ups section is a batch of its own, pending until its own items
// are decided: none supersedes another, a record decides only what it names,
// and a batch filed after a record is pending beside whatever the record left.
func TestPendingFollowUpsAcrossBatches(t *testing.T) {
	first := "### Follow-ups\n\n1. A\n   Brief A.\n2. B\n   Brief B."
	second := "### Follow-ups\n\n1. C\n   Brief C."
	third := "### Follow-ups\n\n1. D\n   Brief D.\n2. E\n   Brief E."
	tests := []struct {
		name string
		body string
		want []FollowUp
	}{
		{"two undecided batches are both pending", first + "\n\n" + second, []FollowUp{
			{1, 1, "A", "Brief A."}, {1, 2, "B", "Brief B."}, {2, 3, "C", "Brief C."},
		}},
		{"triaging the second leaves the first pending",
			first + "\n\n" + second + "\n\n### Follow-ups triaged\n\n- Dropped: C",
			[]FollowUp{{1, 1, "A", "Brief A."}, {1, 2, "B", "Brief B."}}},
		{"triaging the first leaves the second pending",
			first + "\n\n" + second + "\n\n### Follow-ups triaged\n\n- Dropped: A\n- Folded in: B",
			[]FollowUp{{2, 1, "C", "Brief C."}}},
		{"a third batch after a triage changes neither of the others",
			first + "\n\n" + second + "\n\n### Follow-ups triaged\n\n- Dropped: A\n- Dropped: B\n\n" + third,
			[]FollowUp{{2, 1, "C", "Brief C."}, {3, 2, "D", "Brief D."}, {3, 3, "E", "Brief E."}}},
		{"two batches sharing a title are two items",
			first + "\n\n### Follow-ups\n\n1. A\n   Again.\n\n### Follow-ups triaged\n\n- Dropped: A",
			[]FollowUp{{1, 1, "B", "Brief B."}, {2, 2, "A", "Again."}}},
		{"a record names nothing written after it",
			first + "\n\n### Follow-ups triaged\n\n- Dropped: A\n- Dropped: B\n- Dropped: C\n\n" + second,
			[]FollowUp{{2, 1, "C", "Brief C."}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PendingFollowUps(tt.body); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PendingFollowUps() =\n%#v\nwant\n%#v", got, tt.want)
			}
		})
	}
}

// A Done slice's undecided items are history — a batch the old rule
// superseded and nobody triaged — so nothing is pending on it; any other
// status reads the body as it is.
func TestPendingFollowUpsOf(t *testing.T) {
	body := "### Follow-ups\n\n1. A\n   Brief A."
	if got := PendingFollowUpsOf(domain.Slice{Status: domain.SliceDone}, body); got != nil {
		t.Errorf("Done: PendingFollowUpsOf() = %#v, want nil", got)
	}
	want := []FollowUp{{1, 1, "A", "Brief A."}}
	if got := PendingFollowUpsOf(domain.Slice{Status: domain.SliceClaimed}, body); !reflect.DeepEqual(got, want) {
		t.Errorf("In progress: PendingFollowUpsOf() = %#v, want %#v", got, want)
	}
}

// A tenth item's marker is a character wider, and so is the indent under it.
func TestPendingFollowUpsPastNine(t *testing.T) {
	var items []FollowUp
	for i := 0; i < 10; i++ {
		items = append(items, FollowUp{Title: string(rune('A' + i)), Brief: "Brief\nover two lines."})
	}
	got := PendingFollowUps("### Follow-ups\n\n" + followUpsMarkdown(items))
	if len(got) != 10 || got[9] != (FollowUp{1, 10, "J", "Brief\nover two lines."}) {
		t.Errorf("PendingFollowUps() = %#v, want ten, the last de-indented", got)
	}
}

// decisionString's default case is unreachable through any of the three
// named Decision values; this is the line a value outside them — which
// nothing in this package ever constructs — would still answer safely.
func TestDecisionStringOfAnUnknownDecision(t *testing.T) {
	if got := decisionString(Decision(99)); got != "" {
		t.Errorf("decisionString(99) = %q, want empty", got)
	}
}

func TestTriagedLines(t *testing.T) {
	got := triageMarkdown([]Triaged{
		{Title: "A", Decision: Queued, Link: "https://notion.so/a"},
		{Title: "B", Decision: FoldedIn},
		{Title: "C", Decision: Dropped},
	})
	want := "- Queued: A → https://notion.so/a\n- Folded in: B\n- Dropped: C"
	if got != want {
		t.Errorf("triageMarkdown() = %q, want %q", got, want)
	}
}

func TestNotionProposeFollowUpsWritesOneAppend(t *testing.T) {
	api := &fakeAPI{}
	err := clocked(api).ProposeFollowUps(context.Background(), "s5", []FollowUp{
		{Title: "A", Brief: "One.\n\nTwo."},
	})
	if err != nil {
		t.Fatalf("ProposeFollowUps() error = %v", err)
	}
	if len(api.appended) != 1 {
		t.Fatalf("appends = %d, want one", len(api.appended))
	}
	got, _ := json.Marshal(api.appended[0])
	want := `[{"heading_3":{"rich_text":[{"text":{"content":"Follow-ups"},"type":"text"}]},"object":"block","type":"heading_3"},` +
		stampBlockJSON + `,` +
		`{"numbered_list_item":{"children":[` +
		`{"object":"block","paragraph":{"rich_text":[{"text":{"content":"One."},"type":"text"}]},"type":"paragraph"},` +
		`{"object":"block","paragraph":{"rich_text":[{"text":{"content":"Two."},"type":"text"}]},"type":"paragraph"}],` +
		`"rich_text":[{"text":{"content":"A"},"type":"text"}]},"object":"block","type":"numbered_list_item"}]`
	if string(got) != want {
		t.Errorf("blocks =\n%s\nwant\n%s", got, want)
	}
}

func TestNotionRecordTriageWritesOneAppend(t *testing.T) {
	api := &fakeAPI{}
	err := clocked(api).RecordTriage(context.Background(), "s5", []Triaged{
		{Title: "A", Decision: Queued, Link: "https://notion.so/a"}, {Title: "B", Decision: Dropped},
	})
	if err != nil {
		t.Fatalf("RecordTriage() error = %v", err)
	}
	want := [][2]string{
		{"heading_3", notion.FollowUpsTriagedHeading},
		{"paragraph", testStamp},
		{"bulleted_list_item", "Queued: A → https://notion.so/a"},
		{"bulleted_list_item", "Dropped: B"},
	}
	if got := texts(t, api.appended[0]); !reflect.DeepEqual(got, want) {
		t.Errorf("blocks = %v, want %v", got, want)
	}
}

func TestNotionFollowUpWritesCarryTheFailureUp(t *testing.T) {
	api := &fakeAPI{appendBlocks: func(string, []map[string]any) ([]notion.Block, error) { return nil, errBoom }}
	if err := Over(api).ProposeFollowUps(context.Background(), "s5", proposals); !errors.Is(err, errBoom) {
		t.Errorf("ProposeFollowUps err = %v, want the append's failure", err)
	}
	if err := Over(api).RecordTriage(context.Background(), "s5", nil); !errors.Is(err, errBoom) {
		t.Errorf("RecordTriage err = %v, want the append's failure", err)
	}
}

// asRead turns blocks as this package writes them into blocks as Notion
// reads them back: a span's text in plain_text rather than text.content, and a
// block's children beside its payload rather than inside it.
func asRead(t *testing.T, written []map[string]any) []notion.Block {
	t.Helper()
	var convert func(blocks []map[string]any) []map[string]any
	convert = func(blocks []map[string]any) []map[string]any {
		out := make([]map[string]any, len(blocks))
		for i, b := range blocks {
			kind := b["type"].(string)
			payload := map[string]any{}
			for k, v := range b[kind].(map[string]any) {
				payload[k] = v
			}
			read := map[string]any{"type": kind}
			if kids, ok := payload["children"].([]map[string]any); ok {
				delete(payload, "children")
				read["has_children"] = true
				read["children"] = convert(kids)
			}
			var spans []map[string]any
			for _, s := range payload["rich_text"].([]map[string]any) {
				spans = append(spans, map[string]any{"plain_text": s["text"].(map[string]any)["content"]})
			}
			payload["rich_text"] = spans
			read[kind] = payload
			out[i] = read
		}
		return out
	}
	raw, err := json.Marshal(convert(written))
	if err != nil {
		t.Fatal(err)
	}
	var blocks []notion.Block
	if err := json.Unmarshal(raw, &blocks); err != nil {
		t.Fatal(err)
	}
	return blocks
}

// The local writer is hand-rolled to match what Notion renders, and this is the
// one thing that keeps the two agreeing: the same proposals and the same
// record, filed through each, read back as the same body.
func TestBothStoresFileTheSameBody(t *testing.T) {
	ctx := context.Background()
	record := []Triaged{
		{Title: proposals[0].Title, Decision: Queued, Link: "https://notion.so/abc"},
		{Title: proposals[1].Title, Decision: FoldedIn},
		{Title: proposals[2].Title, Decision: Dropped},
	}
	many := make([]FollowUp, 0, 11)
	for i := 0; i < 11; i++ {
		many = append(many, FollowUp{Title: string(rune('A' + i)), Brief: "Line one\nline two  \n\n\n  Para two."})
	}

	api := &fakeAPI{}
	remote := clocked(api)
	for _, items := range [][]FollowUp{proposals, many} {
		if err := remote.ProposeFollowUps(ctx, "writes", items); err != nil {
			t.Fatalf("Notion ProposeFollowUps: %v", err)
		}
	}
	if err := remote.RecordTriage(ctx, "writes", record); err != nil {
		t.Fatalf("Notion RecordTriage: %v", err)
	}
	blocks := pageBlocks(t, `[{"type":"paragraph","paragraph":{"rich_text":[{"plain_text":"Do the thing."}]}}]`)
	for _, a := range api.appended {
		blocks = append(blocks, asRead(t, a)...)
	}
	fromNotion := notion.Markdown(blocks)

	l, _ := openPlan(t)
	fillPlan(t, l)
	write(t, l, `UPDATE slices SET body = ? WHERE id = ?`, "Do the thing.", "writes")
	for _, items := range [][]FollowUp{proposals, many} {
		if err := l.ProposeFollowUps(ctx, "writes", items); err != nil {
			t.Fatalf("Local ProposeFollowUps: %v", err)
		}
	}
	if err := l.RecordTriage(ctx, "writes", record); err != nil {
		t.Fatalf("Local RecordTriage: %v", err)
	}
	fromLocal, err := l.Body(ctx, "writes")
	if err != nil {
		t.Fatalf("Local Body: %v", err)
	}

	if want := fromLocal + "\n"; fromNotion != want {
		t.Errorf("Notion renders\n%s\nlocal holds\n%s", fromNotion, want)
	}
	if got := PendingFollowUps(fromLocal); len(got) != 11 || got[10].Brief != "Line one\nline two\n\nPara two." {
		t.Errorf("PendingFollowUps() = %#v, want the eleven of the newer section", got)
	}
	// Each section's stamp is read off as its time, and is no item of it.
	events := TaskEvents(fromLocal)
	if len(events) != 2 || !events[0].At.Equal(testNow) || !events[1].At.Equal(testNow) ||
		len(events[0].FollowUps) != 3 || len(events[1].FollowUps) != 11 {
		t.Errorf("TaskEvents() = %+v, want two stamped proposals of three and eleven", events)
	}
}

func TestLocalFollowUpWritesCarryTheFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	if err := l.ProposeFollowUps(context.Background(), "ghost", proposals); err == nil {
		t.Error("ProposeFollowUps on a slice not in the plan: want an error")
	}
	if err := l.RecordTriage(context.Background(), "ghost", nil); err == nil {
		t.Error("RecordTriage on a slice not in the plan: want an error")
	}
}

func TestMirroredFollowUpWritesGoLocallyThenPush(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	if err := m.ProposeFollowUps(ctx, "writes", proposals); err != nil {
		t.Fatalf("ProposeFollowUps: %v", err)
	}
	if err := m.RecordTriage(ctx, "writes", []Triaged{{Title: "A", Decision: Dropped}}); err != nil {
		t.Fatalf("RecordTriage: %v", err)
	}
	if len(api.appended) != 2 {
		t.Errorf("appends = %d, want both pushed", len(api.appended))
	}
	body, _ := l.Body(ctx, "writes")
	if got := PendingFollowUps(body); len(got) != 3 {
		t.Errorf("local pending = %#v, want the three filed", got)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

func TestMirroredFollowUpWritesCarryTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if err := m.ProposeFollowUps(context.Background(), "ghost", proposals); err == nil {
		t.Error("ProposeFollowUps on a slice not in the plan: want an error")
	}
	if err := m.RecordTriage(context.Background(), "ghost", nil); err == nil {
		t.Error("RecordTriage on a slice not in the plan: want an error")
	}
}
