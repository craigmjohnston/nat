package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/gh"
)

// convoNow is the moment the conversation tests read their relative times
// against, so "2h ago" is a fact rather than whatever the clock says.
var convoNow = time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

// convoAt is a time that many hours before [convoNow].
func convoAt(hours int) time.Time { return convoNow.Add(-time.Duration(hours) * time.Hour) }

// talkedPR is the sample pull request with a conversation on it: a comment, a
// review that asked for changes, a second comment answering it, and a review
// that then approved — which is the whole shape the section is read for.
func talkedPR() gh.PR {
	pr := samplePR()
	pr.Comments = []gh.Comment{
		{Author: "craigmjohnston", Body: "Ready for a look.", CreatedAt: convoAt(9)},
		{Author: "craigmjohnston", Body: "Pushed a fix for that.", CreatedAt: convoAt(3)},
	}
	pr.Reviews = []gh.Review{
		{
			Author:      "reviewer",
			State:       "CHANGES_REQUESTED",
			Body:        "The **width** is not fit to the window.",
			SubmittedAt: convoAt(6),
		},
		{Author: "reviewer", State: "APPROVED", SubmittedAt: convoAt(1)},
	}
	return pr
}

// TestConversationOrder covers the two kinds as one timeline: interleaved by
// the time each was said, oldest first.
func TestConversationOrder(t *testing.T) {
	entries := conversation(talkedPR())
	var got []string
	for _, e := range entries {
		got = append(got, e.author+" "+e.verb)
	}
	want := []string{
		"craigmjohnston commented",
		"reviewer requested changes",
		"craigmjohnston commented",
		"reviewer approved",
	}
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Errorf("timeline = %v, want %v", got, want)
	}
}

// TestConversationStableOnATie covers two things GitHub stamped with the same
// second: the order is kept rather than swapping between one reading and the
// next, with the comments the pull request carries first.
func TestConversationStableOnATie(t *testing.T) {
	at := convoAt(2)
	pr := samplePR()
	pr.Comments = []gh.Comment{
		{Author: "first", Body: "a", CreatedAt: at},
		{Author: "second", Body: "b", CreatedAt: at},
	}
	pr.Reviews = []gh.Review{{Author: "third", State: "APPROVED", SubmittedAt: at}}
	for range 5 {
		var got []string
		for _, e := range conversation(pr) {
			got = append(got, e.author)
		}
		if strings.Join(got, ",") != "first,second,third" {
			t.Fatalf("timeline = %v, want the order it was given in", got)
		}
	}
}

// TestReviewEntry covers what each of GitHub's review states is drawn as, and
// the two reviews that are no part of the conversation at all.
func TestReviewEntry(t *testing.T) {
	tests := []struct {
		name string
		r    gh.Review
		verb string
		tone convoTone
		ok   bool
	}{
		{"approved", gh.Review{State: "APPROVED", SubmittedAt: convoAt(1)},
			"approved", convoApproved, true},
		{"changes requested", gh.Review{State: "CHANGES_REQUESTED", SubmittedAt: convoAt(1)},
			"requested changes", convoRejected, true},
		{"dismissed", gh.Review{State: "DISMISSED", SubmittedAt: convoAt(1)},
			"review dismissed", convoDismissed, true},
		{"commented with words", gh.Review{State: "COMMENTED", Body: "hm", SubmittedAt: convoAt(1)},
			"reviewed", convoNeutral, true},
		{"lower case", gh.Review{State: " approved ", SubmittedAt: convoAt(1)},
			"approved", convoApproved, true},
		{"a word this build does not know",
			gh.Review{State: "SOMETHING_ELSE", SubmittedAt: convoAt(1)},
			"reviewed (something else)", convoNeutral, true},
		{"commented with nothing to say",
			gh.Review{State: "COMMENTED", SubmittedAt: convoAt(1)}, "", 0, false},
		{"never submitted", gh.Review{State: "PENDING"}, "", 0, false},
		{"submitted with no time on it", gh.Review{State: "APPROVED"}, "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, ok := reviewEntry(tt.r)
			if ok != tt.ok {
				t.Fatalf("drawn = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if entry.verb != tt.verb || entry.tone != tt.tone {
				t.Errorf("entry = %q/%v, want %q/%v", entry.verb, entry.tone, tt.verb, tt.tone)
			}
			if !entry.review {
				t.Error("a review is counted as one")
			}
		})
	}
}

// TestConversationAuthorless covers a comment left by an account since
// deleted: still something that was said, and named as somebody.
func TestConversationAuthorless(t *testing.T) {
	pr := samplePR()
	pr.Comments = []gh.Comment{{Body: "left by nobody", CreatedAt: convoAt(1)}}
	pr.Reviews = []gh.Review{{State: "APPROVED", SubmittedAt: convoAt(1)}}
	for _, e := range conversation(pr) {
		if e.author != "someone" {
			t.Errorf("author = %q, want it named in words", e.author)
		}
	}
}

// TestConvoToneMarks covers the glyph and the colour each tone is scanned by:
// one cell wide, and different from each other's.
func TestConvoToneMarks(t *testing.T) {
	styles := DefaultStyles()
	seen := map[string]bool{}
	for _, tone := range []convoTone{convoNeutral, convoApproved, convoRejected, convoDismissed} {
		mark := tone.mark()
		if lipgloss.Width(mark) != 1 {
			t.Errorf("mark %q of %v is %d cells, want 1", mark, tone, lipgloss.Width(mark))
		}
		if seen[mark] {
			t.Errorf("mark %q of %v is another tone's", mark, tone)
		}
		seen[mark] = true
		if got := tone.style(styles).Render("x"); got == "" {
			t.Errorf("tone %v draws nothing", tone)
		}
	}
}

// TestConvoSummary covers the count beside the heading: the two kinds named in
// their own words, only the kind there is any of, and singular where there is
// one.
func TestConvoSummary(t *testing.T) {
	tests := []struct {
		name string
		pr   gh.PR
		want string
	}{
		{"both kinds", talkedPR(), "2 comments · 2 reviews"},
		{"one of each", func() gh.PR {
			pr := samplePR()
			pr.Comments = []gh.Comment{{Author: "a", Body: "hi", CreatedAt: convoAt(2)}}
			pr.Reviews = []gh.Review{{Author: "b", State: "APPROVED", SubmittedAt: convoAt(1)}}
			return pr
		}(), "1 comment · 1 review"},
		{"comments alone", func() gh.PR {
			pr := samplePR()
			pr.Comments = []gh.Comment{{Author: "a", Body: "hi", CreatedAt: convoAt(2)}}
			return pr
		}(), "1 comment"},
		{"reviews alone", func() gh.PR {
			pr := samplePR()
			pr.Reviews = []gh.Review{{Author: "b", State: "APPROVED", SubmittedAt: convoAt(1)}}
			return pr
		}(), "1 review"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := convoSummary(conversation(tt.pr)); got != tt.want {
				t.Errorf("summary = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPRViewConversationRender pins the section: the heading and its count,
// and under it every entry in order with its verdict, its author, how long ago
// it was said and the markdown it carried.
func TestPRViewConversationRender(t *testing.T) {
	fixClock(t, convoNow)
	for _, tt := range []struct {
		name string
		pr   gh.PR
	}{
		{"pr-convo-timeline", talkedPR()},
		{"pr-convo-comments", func() gh.PR {
			pr := samplePR()
			pr.Comments = []gh.Comment{
				{Author: "craigmjohnston", Body: "One quick note.", CreatedAt: convoAt(30)},
			}
			return pr
		}()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			golden(t, tt.name, readyPRView(tt.pr).conversationSection())
		})
	}
}

// TestPRViewConversationInTheBody covers where the section sits: under the
// description and over the checks, in the one viewport, so the whole screen
// scrolls as one body.
func TestPRViewConversationInTheBody(t *testing.T) {
	fixClock(t, convoNow)
	pr := talkedPR()
	pr.Checks = passingChecks
	body := stripANSI(readyPRView(pr).vp.GetContent())
	described := strings.Index(body, "A screen over the board")
	convo := strings.Index(body, "Conversation")
	checks := strings.Index(body, "Checks")
	merge := strings.Index(body, "Merge")
	if described < 0 || convo < 0 || checks < 0 || merge < 0 {
		t.Fatalf("body = %q, want all four sections in it", body)
	}
	if described > convo || convo > checks || checks > merge {
		t.Errorf("body = %q, want the conversation under the description and over the checks",
			body)
	}
	for _, want := range []string{
		"requested changes", "approved", "2 comments · 2 reviews", "Pushed a fix", "6h ago",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body = %q, want %q in it", body, want)
		}
	}
}

// TestPRViewWithoutConversation covers a pull request nobody has said anything
// on: one quiet line, rather than a heading over an empty band.
func TestPRViewWithoutConversation(t *testing.T) {
	p := readyPRView(samplePR())
	section := p.conversationSection()
	if !strings.Contains(section, "Nothing has been said") {
		t.Errorf("section = %q, want the absence said out loud", section)
	}
	if strings.Contains(section, "\n") {
		t.Errorf("section = %q, want one line", section)
	}
	if !strings.Contains(stripANSI(p.View("")), "Nothing has been said") {
		t.Error("the line belongs on the screen")
	}
}

// TestPRViewConversationVerdictWithNoWords covers the review that is a verdict
// and nothing else: its own line, and no body under it.
func TestPRViewConversationVerdictWithNoWords(t *testing.T) {
	fixClock(t, convoNow)
	pr := samplePR()
	pr.Reviews = []gh.Review{{Author: "reviewer", State: "APPROVED", SubmittedAt: convoAt(1)}}
	section := stripANSI(readyPRView(pr).conversationSection())
	lines := strings.Split(strings.TrimRight(section, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("section = %q, want a heading, a blank and the one line", section)
	}
	if !strings.Contains(lines[2], "reviewer approved") || !strings.Contains(lines[2], "1h ago") {
		t.Errorf("line = %q, want the verdict and when it was left", lines[2])
	}
}

// TestPRViewConversationFitsTheWidth covers a narrow screen: an entry's own
// line is cut to the window rather than wrapping onto a line the layout has
// not left room for.
func TestPRViewConversationFitsTheWidth(t *testing.T) {
	fixClock(t, convoNow)
	pr := samplePR()
	pr.Comments = []gh.Comment{{
		Author:    strings.Repeat("a-very-long-github-login", 4),
		Body:      "a word " + strings.Repeat("and another ", 20),
		CreatedAt: convoAt(2),
	}}
	p := readyPRView(pr)
	p.SetSize(40, 20)
	for _, line := range strings.Split(p.conversationSection(), "\n") {
		if got := lipgloss.Width(line); got > 40 {
			t.Errorf("line %q is %d columns, want no more than 40", line, got)
		}
	}
}
