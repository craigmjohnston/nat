package agent

import "testing"

// An empty body reads as "_none_" rather than a blank heading, which would
// otherwise look like output that got cut off.
func TestSectionOfAnEmptyBody(t *testing.T) {
	if got := Section(""); got != "_none_\n" {
		t.Errorf("Section(\"\") = %q, want the empty placeholder", got)
	}
}

func TestBriefSectionsWithNothingToShow(t *testing.T) {
	got := BriefSections("", "")
	want := "## Brief\n\n_none_\n\n## Project conventions\n\n_none_\n"
	if got != want {
		t.Errorf("BriefSections(\"\", \"\") = %q, want %q", got, want)
	}
}
