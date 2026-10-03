package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/notion"
)

func TestNotionRecordSentBackWritesOneAppend(t *testing.T) {
	api := &fakeAPI{}
	err := clocked(api).RecordSentBack(context.Background(), "s5", "Please rename the helper.")
	if err != nil {
		t.Fatalf("RecordSentBack() error = %v", err)
	}
	if len(api.appended) != 1 {
		t.Fatalf("appends = %d, want one", len(api.appended))
	}
	got, _ := json.Marshal(api.appended[0])
	want := `[{"heading_3":{"rich_text":[{"text":{"content":"Sent back"},"type":"text"}]},"object":"block","type":"heading_3"},` +
		stampBlockJSON + `,` +
		`{"object":"block","paragraph":{"rich_text":[{"text":{"content":"Please rename the helper."},"type":"text"}]},"type":"paragraph"}]`
	if string(got) != want {
		t.Errorf("blocks =\n%s\nwant\n%s", got, want)
	}
}

// Empty comments still file the heading and its stamp — a slice sent back with
// nothing written is still an event in the log.
func TestNotionRecordSentBackWithEmptyComments(t *testing.T) {
	api := &fakeAPI{}
	if err := clocked(api).RecordSentBack(context.Background(), "s5", ""); err != nil {
		t.Fatalf("RecordSentBack() error = %v", err)
	}
	got, _ := json.Marshal(api.appended[0])
	want := `[{"heading_3":{"rich_text":[{"text":{"content":"Sent back"},"type":"text"}]},"object":"block","type":"heading_3"},` +
		stampBlockJSON + `]`
	if string(got) != want {
		t.Errorf("blocks =\n%s\nwant\n%s", got, want)
	}
}

func TestNotionRecordSentBackCarriesTheFailureUp(t *testing.T) {
	api := &fakeAPI{appendBlocks: func(string, []map[string]any) ([]notion.Block, error) { return nil, errBoom }}
	if err := Over(api).RecordSentBack(context.Background(), "s5", "comments"); !errors.Is(err, errBoom) {
		t.Errorf("RecordSentBack err = %v, want the append's failure", err)
	}
}

func TestNotionRecordRelaunchWritesOneAppend(t *testing.T) {
	api := &fakeAPI{}
	if err := clocked(api).RecordRelaunch(context.Background(), "s5"); err != nil {
		t.Fatalf("RecordRelaunch() error = %v", err)
	}
	if len(api.appended) != 1 {
		t.Fatalf("appends = %d, want one", len(api.appended))
	}
	got, _ := json.Marshal(api.appended[0])
	want := `[{"heading_3":{"rich_text":[{"text":{"content":"Relaunched"},"type":"text"}]},"object":"block","type":"heading_3"},` +
		stampBlockJSON + `,` +
		`{"object":"block","paragraph":{"rich_text":[{"text":{"content":"Relaunched to pick up the work so far."},"type":"text"}]},"type":"paragraph"}]`
	if string(got) != want {
		t.Errorf("blocks =\n%s\nwant\n%s", got, want)
	}
}

func TestNotionRecordRelaunchCarriesTheFailureUp(t *testing.T) {
	api := &fakeAPI{appendBlocks: func(string, []map[string]any) ([]notion.Block, error) { return nil, errBoom }}
	if err := Over(api).RecordRelaunch(context.Background(), "s5"); !errors.Is(err, errBoom) {
		t.Errorf("RecordRelaunch err = %v, want the append's failure", err)
	}
}

func TestLocalRecordSentBackAppendsTheSection(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	write(t, l, `UPDATE slices SET body = ? WHERE id = ?`, "Do the thing.", "writes")

	if err := l.RecordSentBack(context.Background(), "writes", "Rename the helper."); err != nil {
		t.Fatalf("RecordSentBack: %v", err)
	}
	body, err := l.Body(context.Background(), "writes")
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	if !strings.Contains(body, "### Sent back") || !strings.Contains(body, "Rename the helper.") {
		t.Errorf("body = %q, want the Sent back section", body)
	}
}

func TestLocalRecordSentBackWithEmptyComments(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	write(t, l, `UPDATE slices SET body = ? WHERE id = ?`, "Do the thing.", "writes")

	if err := l.RecordSentBack(context.Background(), "writes", ""); err != nil {
		t.Fatalf("RecordSentBack: %v", err)
	}
	body, err := l.Body(context.Background(), "writes")
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	if !strings.HasSuffix(strings.TrimRight(body, "\n"), "### Sent back\n\n"+testStamp) {
		t.Errorf("body = %q, want it to end in a Sent back heading with only its stamp", body)
	}
}

func TestLocalRecordSentBackCarriesTheFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	if err := l.RecordSentBack(context.Background(), "ghost", "comments"); err == nil {
		t.Error("RecordSentBack on a slice not in the plan: want an error")
	}
}

func TestLocalRecordRelaunchAppendsTheSection(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	write(t, l, `UPDATE slices SET body = ? WHERE id = ?`, "Do the thing.", "writes")

	if err := l.RecordRelaunch(context.Background(), "writes"); err != nil {
		t.Fatalf("RecordRelaunch: %v", err)
	}
	body, err := l.Body(context.Background(), "writes")
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	if !strings.Contains(body, "### Relaunched") || !strings.Contains(body, relaunchedLine) {
		t.Errorf("body = %q, want the Relaunched section", body)
	}
}

func TestLocalRecordRelaunchCarriesTheFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	if err := l.RecordRelaunch(context.Background(), "ghost"); err == nil {
		t.Error("RecordRelaunch on a slice not in the plan: want an error")
	}
}

func TestMirroredRecordSentBackGoesLocallyThenPushes(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	if err := m.RecordSentBack(ctx, "writes", "Rename the helper."); err != nil {
		t.Fatalf("RecordSentBack: %v", err)
	}
	if len(api.appended) != 1 {
		t.Errorf("appends = %d, want it pushed", len(api.appended))
	}
	body, _ := l.Body(ctx, "writes")
	if !strings.Contains(body, "### Sent back") {
		t.Errorf("local body = %q, want the Sent back section filed", body)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

func TestMirroredRecordSentBackCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if err := m.RecordSentBack(context.Background(), "ghost", "comments"); err == nil {
		t.Error("RecordSentBack on a slice not in the plan: want an error")
	}
}

func TestMirroredRecordRelaunchGoesLocallyThenPushes(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	if err := m.RecordRelaunch(ctx, "writes"); err != nil {
		t.Fatalf("RecordRelaunch: %v", err)
	}
	if len(api.appended) != 1 {
		t.Errorf("appends = %d, want it pushed", len(api.appended))
	}
	body, _ := l.Body(ctx, "writes")
	if !strings.Contains(body, "### Relaunched") {
		t.Errorf("local body = %q, want the Relaunched section filed", body)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

func TestMirroredRecordRelaunchCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if err := m.RecordRelaunch(context.Background(), "ghost"); err == nil {
		t.Error("RecordRelaunch on a slice not in the plan: want an error")
	}
}

func TestNotionRecordNoteWritesOneAppend(t *testing.T) {
	api := &fakeAPI{}
	err := clocked(api).RecordNote(context.Background(), "s5", `From "Draw it" (M2)`, "The seam moved.\n\nTwice.")
	if err != nil {
		t.Fatalf("RecordNote() error = %v", err)
	}
	if len(api.appended) != 1 {
		t.Fatalf("appends = %d, want one", len(api.appended))
	}
	got, _ := json.Marshal(api.appended[0])
	want := `[{"heading_3":{"rich_text":[{"text":{"content":"Note"},"type":"text"}]},"object":"block","type":"heading_3"},` +
		stampBlockJSON + `,` +
		`{"object":"block","paragraph":{"rich_text":[{"text":{"content":"From \"Draw it\" (M2)"},"type":"text"}]},"type":"paragraph"},` +
		`{"object":"block","paragraph":{"rich_text":[{"text":{"content":"The seam moved."},"type":"text"}]},"type":"paragraph"},` +
		`{"object":"block","paragraph":{"rich_text":[{"text":{"content":"Twice."},"type":"text"}]},"type":"paragraph"}]`
	if string(got) != want {
		t.Errorf("blocks =\n%s\nwant\n%s", got, want)
	}
}

func TestNotionRecordNoteCarriesTheFailureUp(t *testing.T) {
	api := &fakeAPI{appendBlocks: func(string, []map[string]any) ([]notion.Block, error) { return nil, errBoom }}
	if err := clocked(api).RecordNote(context.Background(), "s5", "From Craig", "note"); !errors.Is(err, errBoom) {
		t.Errorf("RecordNote err = %v, want the append's failure", err)
	}
}

// The brief an agent is handed is the body, so a note reaches whoever works
// the slice later by ending it.
func TestLocalRecordNoteEndsTheBody(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	write(t, l, `UPDATE slices SET body = ? WHERE id = ?`, "Do the thing.", "writes")

	if err := l.RecordNote(context.Background(), "writes", `From "Draw it" (M2)`, "The seam moved."); err != nil {
		t.Fatalf("RecordNote: %v", err)
	}
	body, err := l.Body(context.Background(), "writes")
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	want := "Do the thing.\n\n### Note\n\n" + testStamp + "\n\nFrom \"Draw it\" (M2)\n\nThe seam moved."
	if strings.TrimRight(body, "\n") != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestLocalRecordNoteCarriesTheFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	if err := l.RecordNote(context.Background(), "ghost", "From Craig", "note"); err == nil {
		t.Error("RecordNote on a slice not in the plan: want an error")
	}
}

func TestMirroredRecordNoteGoesLocallyThenPushes(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	if err := m.RecordNote(ctx, "writes", "From Craig", "Mind the cache."); err != nil {
		t.Fatalf("RecordNote: %v", err)
	}
	if len(api.appended) != 1 {
		t.Errorf("appends = %d, want it pushed", len(api.appended))
	}
	body, _ := l.Body(ctx, "writes")
	if !strings.HasSuffix(strings.TrimRight(body, "\n"), "### Note\n\n"+testStamp+"\n\nFrom Craig\n\nMind the cache.") {
		t.Errorf("body = %q, want it to end in the Note section", body)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

func TestMirroredRecordNoteCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if err := m.RecordNote(context.Background(), "ghost", "From Craig", "note"); err == nil {
		t.Error("RecordNote on a slice not in the plan: want an error")
	}
}
