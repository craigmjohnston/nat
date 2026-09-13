package agent

import (
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
)

func TestMilestoneDigestWithNoMilestone(t *testing.T) {
	if got := MilestoneDigest(domain.Milestone{}, []domain.Slice{{ID: "s1", Name: "Other"}}, nil); got != "" {
		t.Errorf("MilestoneDigest with no milestone = %q, want empty", got)
	}
}

func TestMilestoneDigestWithNoSiblings(t *testing.T) {
	if got := MilestoneDigest(domain.Milestone{Name: "M1"}, nil, nil); got != "" {
		t.Errorf("MilestoneDigest with no siblings = %q, want empty", got)
	}
}

// A sibling with no StatusName — a project's own Status column read before
// it carried one, or a fixture that never set it — falls back to the raw
// status, so the digest never shows a slice with no status word at all.
func TestMilestoneDigestFallsBackToTheRawStatusWithNoStatusName(t *testing.T) {
	got := MilestoneDigest(domain.Milestone{Name: "M1"}, []domain.Slice{
		{ID: "s1", Name: "Nameless status", Status: domain.SliceTodo},
	}, nil)
	want := "M1\n\n- Todo: Nameless status"
	if got != want {
		t.Errorf("MilestoneDigest = %q, want %q", got, want)
	}
}

func TestMilestoneDigestListsEverySiblingWithItsStatus(t *testing.T) {
	got := MilestoneDigest(domain.Milestone{Name: "M2: Board"}, []domain.Slice{
		{ID: "s2", Name: "Board scaffolding", Status: domain.SliceDone, StatusName: "Done"},
		{ID: "s4", Name: "Style the board", Status: domain.SliceTodo, StatusName: "Todo"},
	}, nil)
	want := "M2: Board\n\n- Done: Board scaffolding\n- Todo: Style the board"
	if got != want {
		t.Errorf("MilestoneDigest = %q, want %q", got, want)
	}
}

// Only a Done sibling's summary is shown: a slice still in progress has no
// hand-back to summarise, and one it merely has a stale summary from an
// earlier hand-back would be misleading to show as settled.
func TestMilestoneDigestOnlyShowsSummariesForDoneSiblings(t *testing.T) {
	got := MilestoneDigest(domain.Milestone{Name: "M1"}, []domain.Slice{
		{ID: "s1", Name: "In flight", Status: domain.SliceClaimed, StatusName: "In progress"},
	}, map[string]string{"s1": "Should not appear."})
	if strings.Contains(got, "Should not appear.") {
		t.Errorf("digest shows a summary for a non-Done slice:\n%s", got)
	}
}

// A Done sibling with no summary on record — a slice completed before there
// was anything to summarise, or one whose read failed — gets no summary line
// rather than a blank one.
func TestMilestoneDigestSkipsAnEmptySummary(t *testing.T) {
	got := MilestoneDigest(domain.Milestone{Name: "M1"}, []domain.Slice{
		{ID: "s1", Name: "Done, unread", Status: domain.SliceDone, StatusName: "Done"},
	}, nil)
	want := "M1\n\n- Done: Done, unread"
	if got != want {
		t.Errorf("MilestoneDigest = %q, want %q", got, want)
	}
}

// A meandering old summary is capped defensively, so one Done sibling cannot
// flood the whole prompt on its own.
func TestMilestoneDigestCapsALongSummary(t *testing.T) {
	long := strings.Repeat("a", MaxHandbackSummary+50)
	got := MilestoneDigest(domain.Milestone{Name: "M1"}, []domain.Slice{
		{ID: "s1", Name: "Rambling", Status: domain.SliceDone, StatusName: "Done"},
	}, map[string]string{"s1": long})
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("digest = %q, want 4 lines", got)
	}
	summary := strings.TrimPrefix(lines[3], "  ")
	if !strings.HasSuffix(summary, "…") {
		t.Errorf("capped summary does not end with an ellipsis: %q", summary)
	}
	if got, want := len([]rune(summary)), MaxHandbackSummary+1; got != want {
		t.Errorf("capped summary is %d runes, want %d", got, want)
	}
}

// A summary spread over several lines is collapsed to one, so a sibling's
// entry in the digest stays exactly one line.
func TestMilestoneDigestCollapsesAMultilineSummary(t *testing.T) {
	got := MilestoneDigest(domain.Milestone{Name: "M1"}, []domain.Slice{
		{ID: "s1", Name: "Done", Status: domain.SliceDone, StatusName: "Done"},
	}, map[string]string{"s1": "Line one.\n\nLine two, after a blank line."})
	want := "M1\n\n- Done: Done\n  Line one. Line two, after a blank line."
	if got != want {
		t.Errorf("MilestoneDigest = %q, want %q", got, want)
	}
}
