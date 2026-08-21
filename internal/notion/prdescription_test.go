package notion

import (
	"encoding/json"
	"strings"
	"testing"
)

// blocksOf decodes the block JSON the tests describe a page body with, so a
// fixture reads as the page Notion would have returned.
func blocksOf(t *testing.T, parts ...string) []Block {
	t.Helper()
	var blocks []Block
	if err := json.Unmarshal([]byte("["+strings.Join(parts, ",")+"]"), &blocks); err != nil {
		t.Fatalf("decode the page body: %v", err)
	}
	return blocks
}

func paragraph(id, text string) string {
	return `{"id":"` + id + `","type":"paragraph","has_children":false,` +
		`"paragraph":{"rich_text":[{"plain_text":"` + text + `"}]}}`
}

// TestPRDescriptionOf reads the section the way the approve action does: the
// blocks under the heading, and nothing of the page around it.
func TestPRDescriptionOf(t *testing.T) {
	body := blocksOf(t,
		heading("h1", 3, "Handed back"),
		paragraph("p1", "Did the work."),
		heading("h2", 3, "PR description"),
		paragraph("p2", "Open the PR with the recorded description"),
		paragraph("p3", "What it does, and why."),
		heading("h3", 3, "Summary"),
		paragraph("p4", "Not part of the description."),
	)
	want := "Open the PR with the recorded description\n\nWhat it does, and why."
	if got := PRDescriptionOf(body); got != want {
		t.Errorf("PRDescriptionOf() = %q, want %q", got, want)
	}
}

// A slice handed back twice carries a section per hand-back, and the one that
// describes the branch as it now stands is the last written.
func TestPRDescriptionOfTakesTheLastSection(t *testing.T) {
	body := blocksOf(t,
		heading("h1", 3, "PR description"),
		paragraph("p1", "The first attempt"),
		heading("h2", 3, "Handed back"),
		paragraph("p2", "Addressed the review."),
		heading("h3", 3, "pr description"),
		paragraph("p3", "The second attempt"),
	)
	if got, want := PRDescriptionOf(body), "The second attempt"; got != want {
		t.Errorf("PRDescriptionOf() = %q, want %q", got, want)
	}
}

// A heading deeper than the section's own is part of it: the section ends at
// the next heading of the same or higher level, exactly as the wishlist's does.
func TestPRDescriptionOfKeepsDeeperHeadings(t *testing.T) {
	body := blocksOf(t,
		heading("h1", 2, "PR description"),
		paragraph("p1", "Title line"),
		heading("h2", 3, "Notes"),
		paragraph("p2", "Body."),
	)
	want := "Title line\n\n### Notes\n\nBody."
	if got := PRDescriptionOf(body); got != want {
		t.Errorf("PRDescriptionOf() = %q, want %q", got, want)
	}
}

// A page with no such heading — every hand-back written before there was a flag
// for one — has no description, which is what tells the board to let gh fill
// the pull request from the commits instead.
func TestPRDescriptionOfWithoutASection(t *testing.T) {
	body := blocksOf(t,
		heading("h1", 3, "Handed back"),
		paragraph("p1", "Did the work."),
	)
	if got := PRDescriptionOf(body); got != "" {
		t.Errorf("PRDescriptionOf() = %q, want no description", got)
	}
}

// An empty section is no description either: nothing was written under the
// heading, so there is nothing to open a pull request with.
func TestPRDescriptionOfWithAnEmptySection(t *testing.T) {
	body := blocksOf(t, heading("h1", 3, "PR description"))
	if got := PRDescriptionOf(body); got != "" {
		t.Errorf("PRDescriptionOf() = %q, want no description", got)
	}
}
