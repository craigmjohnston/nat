package store

import (
	"reflect"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/domain"
)

func TestTaskEventsNone(t *testing.T) {
	if got := TaskEvents("Just a brief, no events at all."); got != nil {
		t.Errorf("TaskEvents() = %#v, want nil", got)
	}
}

func TestTaskEventsEachSimpleKind(t *testing.T) {
	body := "Brief.\n\n" +
		"### Handed back\n\nDid the thing.\n\n" +
		"### Sent back\n\nRename the helper.\n\n" +
		"### Relaunched\n\nRelaunched to pick up the work so far.\n\n" +
		"### Blocked\n\nWaiting on infra.\n\n" +
		"### Summary\n\nClosed it out.\n\n" +
		"### PR description\n\nNot an event."
	want := []TaskEvent{
		{Kind: "handed_back", Note: "Did the thing."},
		{Kind: "sent_back", Note: "Rename the helper."},
		{Kind: "relaunched"},
		{Kind: "blocked", Note: "Waiting on infra."},
		{Kind: "summary", Note: "Closed it out."},
	}
	if got := TaskEvents(body); !reflect.DeepEqual(got, want) {
		t.Errorf("TaskEvents() =\n%#v\nwant\n%#v", got, want)
	}
}

// Headings are matched case-insensitively, as every other heading match in
// this package is.
func TestTaskEventsHeadingsAreCaseInsensitive(t *testing.T) {
	body := "### handed BACK\n\nDid it."
	want := []TaskEvent{{Kind: "handed_back", Note: "Did it."}}
	if got := TaskEvents(body); !reflect.DeepEqual(got, want) {
		t.Errorf("TaskEvents() = %#v, want %#v", got, want)
	}
}

// An empty section still files its event — heading only, nothing under it.
func TestTaskEventsEmptySection(t *testing.T) {
	body := "### Sent back"
	want := []TaskEvent{{Kind: "sent_back"}}
	if got := TaskEvents(body); !reflect.DeepEqual(got, want) {
		t.Errorf("TaskEvents() = %#v, want %#v", got, want)
	}
}

// A heading not in the vocabulary — the brief's own, or a hand-back's PR
// description — is not an event, and simply ends whatever came before it.
func TestTaskEventsUnknownHeadingEndsTheSection(t *testing.T) {
	body := "### Handed back\n\nDid it.\n\n### Notes\n\nNot an event, not kept either."
	want := []TaskEvent{{Kind: "handed_back", Note: "Did it."}}
	if got := TaskEvents(body); !reflect.DeepEqual(got, want) {
		t.Errorf("TaskEvents() = %#v, want %#v", got, want)
	}
}

// A fenced block inside a note is kept whole, and a "#" inside it is never
// mistaken for a heading.
func TestTaskEventsFenceIsPassedOverWhole(t *testing.T) {
	body := "### Summary\n\nSee the session:\n\n```\n# not a heading\n### Blocked\n$ echo hi\n```\n\nDone."
	got := TaskEvents(body)
	if len(got) != 1 || got[0].Kind != "summary" {
		t.Fatalf("TaskEvents() = %#v, want one summary event", got)
	}
	if got[0].Note == "" {
		t.Error("Note is empty, want the fenced block kept")
	}
}

// A heading nested deeper than the open section's own is passed over as
// content rather than ending the section — [lastMarkdownSection]'s own rule.
func TestTaskEventsADeeperHeadingIsPassedOver(t *testing.T) {
	body := "### Summary\n\n#### Aside\n\nStill the summary."
	got := TaskEvents(body)
	if len(got) != 1 || got[0].Kind != "summary" {
		t.Fatalf("TaskEvents() = %#v, want one summary event", got)
	}
}

// Released lines are bare paragraphs, not headings, and each becomes its own
// event ending whatever section it fell inside — the release writes no
// heading of its own.
func TestTaskEventsReleasedEndsTheEnclosingSection(t *testing.T) {
	body := "### Handed back\n\nDid most of it.\n\nReleased back to Todo by Craig Johnston: the session working it ended without finishing it."
	want := []TaskEvent{
		{Kind: "handed_back", Note: "Did most of it."},
		{Kind: "released", By: "Craig Johnston"},
	}
	if got := TaskEvents(body); !reflect.DeepEqual(got, want) {
		t.Errorf("TaskEvents() =\n%#v\nwant\n%#v", got, want)
	}
}

// A release right after a claim, with no heading at all yet on the page, is
// still its own event.
func TestTaskEventsReleasedWithNoPriorSection(t *testing.T) {
	body := "Brief.\n\nReleased back to Todo by Craig Johnston: the session working it ended without finishing it."
	want := []TaskEvent{{Kind: "released", By: "Craig Johnston"}}
	if got := TaskEvents(body); !reflect.DeepEqual(got, want) {
		t.Errorf("TaskEvents() = %#v, want %#v", got, want)
	}
}

// Two releases, round and round, are two events.
func TestTaskEventsTwoReleases(t *testing.T) {
	body := "Released back to Todo by A: the session working it ended without finishing it.\n\n" +
		"Released back to Todo by B: the session working it ended without finishing it."
	want := []TaskEvent{{Kind: "released", By: "A"}, {Kind: "released", By: "B"}}
	if got := TaskEvents(body); !reflect.DeepEqual(got, want) {
		t.Errorf("TaskEvents() = %#v, want %#v", got, want)
	}
}

// Every event appears in the order its section was written, across every
// kind at once.
func TestTaskEventsOrderedTopToBottom(t *testing.T) {
	body := "### Handed back\n\nFirst pass.\n\n" +
		"### Sent back\n\nFix the helper.\n\n" +
		"### Relaunched\n\nRelaunched to pick up the work so far.\n\n" +
		"### Handed back\n\nSecond pass."
	want := []TaskEvent{
		{Kind: "handed_back", Note: "First pass."},
		{Kind: "sent_back", Note: "Fix the helper."},
		{Kind: "relaunched"},
		{Kind: "handed_back", Note: "Second pass."},
	}
	if got := TaskEvents(body); !reflect.DeepEqual(got, want) {
		t.Errorf("TaskEvents() =\n%#v\nwant\n%#v", got, want)
	}
}

// A Follow-ups section's items are read exactly as PendingFollowUps parses
// them, unfiltered — every item, pending or not.
func TestTaskEventsFollowUps(t *testing.T) {
	body := "### Follow-ups\n\n" + followUpsMarkdown(proposals)
	got := TaskEvents(body)
	if len(got) != 1 || got[0].Kind != "follow_ups" {
		t.Fatalf("TaskEvents() = %#v, want one follow_ups event", got)
	}
	want := []TaskFollowUp{
		{Index: 1, Title: proposals[0].Title, Brief: proposals[0].Brief},
		{Index: 2, Title: proposals[1].Title, Brief: proposals[1].Brief},
		{Index: 3, Title: proposals[2].Title, Brief: proposals[2].Brief},
	}
	if !reflect.DeepEqual(got[0].FollowUps, want) {
		t.Errorf("FollowUps =\n%#v\nwant\n%#v", got[0].FollowUps, want)
	}
}

// A later Follow-ups triaged section decorates the most recent follow_ups
// event's matching items by title, decision and link.
func TestTaskEventsFollowUpsTriaged(t *testing.T) {
	section := "### Follow-ups\n\n" + followUpsMarkdown(proposals)
	body := section + "\n\n### Follow-ups triaged\n\n" +
		"- Queued: " + proposals[0].Title + " → https://notion.so/abc\n" +
		"- Folded in: " + proposals[1].Title + "\n" +
		"- Dropped: " + proposals[2].Title
	got := TaskEvents(body)
	if len(got) != 1 || got[0].Kind != "follow_ups" {
		t.Fatalf("TaskEvents() = %#v, want one follow_ups event", got)
	}
	want := []TaskFollowUp{
		{Index: 1, Title: proposals[0].Title, Brief: proposals[0].Brief, Decision: "queued", Link: "https://notion.so/abc"},
		{Index: 2, Title: proposals[1].Title, Brief: proposals[1].Brief, Decision: "folded"},
		{Index: 3, Title: proposals[2].Title, Brief: proposals[2].Brief, Decision: "dropped"},
	}
	if !reflect.DeepEqual(got[0].FollowUps, want) {
		t.Errorf("FollowUps =\n%#v\nwant\n%#v", got[0].FollowUps, want)
	}
}

// Two separate Follow-ups sections, each triaged by the section that follows
// it, are two separate follow_ups events, each correctly decorated —
// matching only against the most recent one, never an earlier superseded
// one.
func TestTaskEventsTwoFollowUpsSectionsEachTriagedSeparately(t *testing.T) {
	body := "### Follow-ups\n\n1. A\n   Brief A.\n\n" +
		"### Follow-ups triaged\n\n- Dropped: A\n\n" +
		"### Follow-ups\n\n1. B\n   Brief B.\n\n" +
		"### Follow-ups triaged\n\n- Folded in: B"
	got := TaskEvents(body)
	if len(got) != 2 || got[0].Kind != "follow_ups" || got[1].Kind != "follow_ups" {
		t.Fatalf("TaskEvents() = %#v, want two follow_ups events", got)
	}
	if got[0].FollowUps[0].Decision != "dropped" {
		t.Errorf("first follow_ups decision = %q, want %q", got[0].FollowUps[0].Decision, "dropped")
	}
	if got[1].FollowUps[0].Decision != "folded" {
		t.Errorf("second follow_ups decision = %q, want %q", got[1].FollowUps[0].Decision, "folded")
	}
}

// A nested, deeper Follow-ups heading still supersedes the open one — the
// same "a new section while an item is open" case PendingFollowUps itself is
// tested against — and both are read as events, in order.
func TestTaskEventsFollowUpsSupersededByANestedHeading(t *testing.T) {
	body := "### Follow-ups\n\n1. A\n   Brief.\n#### Follow-ups\n\n1. B\n   Other."
	got := TaskEvents(body)
	if len(got) != 2 {
		t.Fatalf("TaskEvents() = %#v, want two follow_ups events", got)
	}
	if len(got[0].FollowUps) != 1 || got[0].FollowUps[0].Title != "A" {
		t.Errorf("first = %#v, want item A", got[0])
	}
	if len(got[1].FollowUps) != 1 || got[1].FollowUps[0].Title != "B" {
		t.Errorf("second = %#v, want item B", got[1])
	}
}

// A Follow-ups triaged section with no Follow-ups section before it at all
// names nothing to decide, and is otherwise simply not an event.
func TestTaskEventsFollowUpsTriagedWithNoPriorFollowUps(t *testing.T) {
	body := "### Follow-ups triaged\n\n- Dropped: A"
	if got := TaskEvents(body); got != nil {
		t.Errorf("TaskEvents() = %#v, want nil", got)
	}
}

// A Follow-ups heading with no numbered item under it at all is still an
// event, with no follow-ups of its own.
func TestTaskEventsFollowUpsSectionWithNoItems(t *testing.T) {
	got := TaskEvents("### Follow-ups\n\nNothing numbered here.")
	if len(got) != 1 || got[0].Kind != "follow_ups" || got[0].FollowUps != nil {
		t.Errorf("TaskEvents() = %#v, want one empty follow_ups event", got)
	}
}

// A Note section's provenance line is who it came from, and the note is what
// follows it; one typed by hand with no such line is all note.
func TestTaskEventsNote(t *testing.T) {
	body := "Brief.\n\n" +
		"### Note\n\nFrom \"Render the board\" (M2: Board)\n\nThe seam moved.\n\nTwice.\n\n" +
		"### Note\n\nFrom Craig\n\nMind the cache.\n\n" +
		"### Note\n\nJust a remark."
	want := []TaskEvent{
		{
			Kind: "note", By: `"Render the board" (M2: Board)`, Note: "The seam moved.\n\nTwice.",
			FromSlice: &NoteSource{Name: "Render the board", Milestone: "M2: Board"},
		},
		{Kind: "note", By: "Craig", Note: "Mind the cache."},
		{Kind: "note", Note: "Just a remark."},
	}
	if got := TaskEvents(body); !reflect.DeepEqual(got, want) {
		t.Errorf("TaskEvents() =\n%#v\nwant\n%#v", got, want)
	}
}

// A stamped Note reads its stamp first, then its provenance — the order
// slice-note writes them in — and a hand-typed one under a stamp is all note.
func TestTaskEventsStampedNote(t *testing.T) {
	body := "### Note\n\n" + testStamp + "\n\nFrom \"Loose end\"\n\nSelf.\n\n" +
		"### Note\n\n" + testStamp + "\n\nJust a remark."
	want := []TaskEvent{
		{Kind: "note", By: `"Loose end"`, Note: "Self.", FromSlice: &NoteSource{Name: "Loose end"}, At: readNow},
		{Kind: "note", Note: "Just a remark.", At: readNow},
	}
	if got := TaskEvents(body); !reflect.DeepEqual(got, want) {
		t.Errorf("TaskEvents() =\n%#v\nwant\n%#v", got, want)
	}
}

// Every stamped section reads its stamp off into At, and its text is what
// follows the stamp — the relaunch's fixed line included, which is still not
// surfaced. A release's line says when inside its sentence.
func TestTaskEventsStampedSections(t *testing.T) {
	stamp := "\n\n" + testStamp + "\n\n"
	body := "Brief.\n\n" +
		"### Handed back" + stamp + "Did the thing.\n\n" +
		"### Sent back" + stamp + "Rename the helper.\n\n" +
		"### Relaunched" + stamp + "Relaunched to pick up the work so far.\n\n" +
		"### Blocked" + stamp + "Waiting on infra.\n\n" +
		"### Summary\n\n" + testStamp + "\n\n" +
		"### Follow-ups" + stamp + "1. A\n   Brief A.\n\n" +
		"### Follow-ups triaged" + stamp + "- Dropped: A\n\n" +
		releasedLine("Craig Johnston", testNow)
	want := []TaskEvent{
		{Kind: "handed_back", Note: "Did the thing.", At: readNow},
		{Kind: "sent_back", Note: "Rename the helper.", At: readNow},
		{Kind: "relaunched", At: readNow},
		{Kind: "blocked", Note: "Waiting on infra.", At: readNow},
		{Kind: "summary", At: readNow},
		{Kind: "follow_ups", At: readNow, FollowUps: []TaskFollowUp{{Index: 1, Title: "A", Brief: "Brief A.", Decision: "dropped"}}},
		{Kind: "released", By: "Craig Johnston", At: readNow},
	}
	if got := TaskEvents(body); !reflect.DeepEqual(got, want) {
		t.Errorf("TaskEvents() =\n%#v\nwant\n%#v", got, want)
	}
}

// A release's line read back names who and, where it says so, when; one
// whose name itself holds " at " still reads whole, and an old line with no
// time reads at the zero time.
func TestReleasedBy(t *testing.T) {
	tests := []struct {
		line string
		by   string
		at   time.Time
	}{
		{releasedLine("Craig Johnston", testNow), "Craig Johnston", testNow},
		{releasedLine("Jo at Home", testNow), "Jo at Home", testNow},
		{"Released back to Todo by Jo at Home: the session working it ended without finishing it.", "Jo at Home", time.Time{}},
		{"Released back to Todo by Jo at 2026-13-03T23:14:05+01:00: the session working it ended without finishing it.", "Jo", time.Time{}},
	}
	for _, tt := range tests {
		by, at, ok := releasedBy(tt.line)
		if !ok || by != tt.by || !at.Equal(tt.at) {
			t.Errorf("releasedBy(%q) = %q, %v, %v; want %q at %v", tt.line, by, at, ok, tt.by, tt.at)
		}
	}
}

// A slice label written by SliceLabel reads back as the slice it names;
// anything else is no slice.
func TestSliceLabelRoundTrips(t *testing.T) {
	for _, src := range []NoteSource{
		{Name: "Render the board", Milestone: "M1"},
		{Name: "Loose end"},
		{Name: `Use "foo" (bar)`, Milestone: "M2: Board"},
	} {
		got, ok := sliceLabelOf(SliceLabel(src.Name, src.Milestone))
		if !ok || got != src {
			t.Errorf("sliceLabelOf(SliceLabel(%+v)) = %+v, %v; want it back", src, got, ok)
		}
	}
	for _, label := range []string{"Craig Johnston", `""`, `"`, `"Unclosed (M1)`, ""} {
		if got, ok := sliceLabelOf(label); ok {
			t.Errorf("sliceLabelOf(%q) = %+v, want no slice", label, got)
		}
	}
}

// Notes alone are no history; anything else is.
func TestHasHistory(t *testing.T) {
	if HasHistory(nil) {
		t.Error("HasHistory(nil) = true, want false")
	}
	if HasHistory([]TaskEvent{{Kind: noteKind}, {Kind: noteKind}}) {
		t.Error("HasHistory(notes) = true, want false")
	}
	if !HasHistory([]TaskEvent{{Kind: noteKind}, {Kind: handedBackKind}}) {
		t.Error("HasHistory(note, hand-back) = false, want true")
	}
}

// The summary a milestone digest carries is what was done, never when.
func TestHandbackSummaryOfLeavesTheStampOff(t *testing.T) {
	body := "Brief.\n\n### Handed back\n\n" + testStamp + "\n\nDid it."
	if got := HandbackSummaryOf(body); got != "Did it." {
		t.Errorf("HandbackSummaryOf = %q, want the summary without its stamp", got)
	}
	if got := HandbackSummaryOf("### Summary\n\nOld one."); got != "Old one." {
		t.Errorf("HandbackSummaryOf(unstamped) = %q, want it whole", got)
	}
}

// A first paragraph running over more than one line is not a provenance line,
// even where it opens with the same word.
func TestTaskEventsNoteFirstParagraphOverLines(t *testing.T) {
	body := "### Note\n\nFrom here on\nthe schema is v6."
	want := []TaskEvent{{Kind: "note", Note: "From here on\nthe schema is v6."}}
	if got := TaskEvents(body); !reflect.DeepEqual(got, want) {
		t.Errorf("TaskEvents() = %#v, want %#v", got, want)
	}
}

// TestFixing reads a fix under way off the record: an approved slice in
// progress whose latest event is a Relaunched or a Sent back, and not once a
// hand-back follows — nor for a slice with no pull request, or not in progress.
func TestFixing(t *testing.T) {
	approved := domain.Slice{Status: domain.SliceClaimed, PRURL: "https://github.test/pr/1"}
	const handed = "### Handed back\n\nDone.\n"
	const relaunched = handed + "\n### Relaunched\n\nRelaunched to pick up the work so far.\n"
	tests := []struct {
		name  string
		slice domain.Slice
		body  string
		want  bool
	}{
		{"relaunched after approve", approved, relaunched, true},
		{"sent back after approve", approved, handed + "\n### Sent back\n\nFix the test.\n", true},
		{"handed back after the fix", approved, relaunched + "\n### Handed back\n\nFixed.\n", false},
		{"checks failed with nobody on it", approved, handed + "\n### Checks failed\n\n- test\n", false},
		{"nothing on the record", approved, "", false},
		{"no pull request", domain.Slice{Status: domain.SliceClaimed}, relaunched, false},
		{"done", domain.Slice{Status: domain.SliceDone, PRURL: "u"}, relaunched, false},
	}
	for _, tt := range tests {
		if got := Fixing(tt.slice, tt.body); got != tt.want {
			t.Errorf("%s: Fixing = %v, want %v", tt.name, got, tt.want)
		}
	}
}
