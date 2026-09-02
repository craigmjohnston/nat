package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/gh"
)

// The words GitHub submits a review as, beyond the two [reviewApproved] and
// [reviewChangesRequested] the merge box already reads. A review is COMMENTED
// when it carries words and no verdict, DISMISSED when a later push or a
// maintainer took its verdict back, and PENDING while it is still being
// written, which is a review nobody but its author has seen.
const (
	reviewCommented = "COMMENTED"
	reviewDismissed = "DISMISSED"
	reviewPending   = "PENDING"
)

// convoTone is what an entry of the conversation amounts to at a glance: a
// verdict for the work, one against it, one taken back, or words with no
// verdict in them at all — which is every comment and every review that is
// only a remark. It is the conversation's own reading rather than
// [checkOutcome], because a comment is neither passing nor failing and reading
// it as either would put a colour on it that says something nobody wrote.
type convoTone int

const (
	convoNeutral convoTone = iota
	convoApproved
	convoRejected
	convoDismissed
)

// mark is the glyph an entry opens with, one cell wide so every author starts
// at the same column. The two verdicts take the same marks a check does, since
// what they say is the same thing said by a person.
func (t convoTone) mark() string {
	switch t {
	case convoApproved:
		return "✓"
	case convoRejected:
		return "✗"
	case convoDismissed:
		return "⊘"
	default:
		return "▪"
	}
}

// style is what an entry's own line is drawn in. The neutral tone recedes, so
// the verdicts are what the eye finds scrolling past a long conversation.
func (t convoTone) style(s Styles) lipgloss.Style {
	switch t {
	case convoApproved:
		return s.CheckPass
	case convoRejected:
		return s.CheckFail
	case convoDismissed:
		return s.CheckSkip
	default:
		return s.Faint
	}
}

// convoEntry is one thing said on the pull request, whichever of the two ways
// GitHub records it: who said it, what they did in saying it, when, and the
// markdown they wrote, which is empty for the many reviews that are a verdict
// and no words. review tells the two kinds apart for the heading's count
// alone — everything else about drawing one is the same either way.
type convoEntry struct {
	author string
	verb   string
	at     time.Time
	body   string
	tone   convoTone
	review bool
}

// convoAuthor is whoever GitHub names. A comment left by an account since
// deleted has no login at all, and is still something that was said.
func convoAuthor(login string) string {
	if author := strings.TrimSpace(login); author != "" {
		return author
	}
	return "someone"
}

// reviewEntry is a review as the timeline draws it, and whether it belongs on
// the timeline at all.
//
// Two do not. A review never submitted is one still being written, which
// nobody but its author can see and which carries no time to place it by. And
// a COMMENTED review with no words is the wrapper GitHub puts around comments
// left on lines of the diff — which gh's pr view does not carry — so drawing
// it would be an entry saying somebody reviewed and nothing about what they
// said. Every other wordless review is a verdict, which is the whole of what
// it had to say.
func reviewEntry(r gh.Review) (convoEntry, bool) {
	state := strings.ToUpper(strings.TrimSpace(r.State))
	body := strings.TrimSpace(r.Body)
	if r.SubmittedAt.IsZero() || state == reviewPending {
		return convoEntry{}, false
	}
	entry := convoEntry{
		author: convoAuthor(r.Author),
		at:     r.SubmittedAt,
		body:   body,
		review: true,
	}
	switch state {
	case reviewApproved:
		entry.verb, entry.tone = "approved", convoApproved
	case reviewChangesRequested:
		entry.verb, entry.tone = "requested changes", convoRejected
	case reviewDismissed:
		entry.verb, entry.tone = "review dismissed", convoDismissed
	case reviewCommented:
		if body == "" {
			return convoEntry{}, false
		}
		entry.verb = "reviewed"
	default:
		// A word GitHub has added since. It is still a review somebody
		// submitted, so it is drawn in GitHub's own word rather than dropped.
		entry.verb = "reviewed (" + mergeState(state) + ")"
	}
	return entry, true
}

// conversation is everything said on the pull request in the order it was
// said: the comments and the reviews as one timeline, oldest first, which is
// how a conversation is read.
//
// The sort is stable and the comments are gathered first, so two entries
// GitHub stamped with the same second keep an order rather than swapping
// between one reading and the next.
func conversation(pr gh.PR) []convoEntry {
	entries := make([]convoEntry, 0, len(pr.Comments)+len(pr.Reviews))
	for _, c := range pr.Comments {
		entries = append(entries, convoEntry{
			author: convoAuthor(c.Author),
			verb:   "commented",
			at:     c.CreatedAt,
			body:   strings.TrimSpace(c.Body),
		})
	}
	for _, r := range pr.Reviews {
		if entry, ok := reviewEntry(r); ok {
			entries = append(entries, entry)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].at.Before(entries[j].at) })
	return entries
}

// convoSummary is the count beside the heading, in the two kinds GitHub keeps
// them in — "2 comments · 1 review" — naming only the kind there is any of, so
// the line says exactly what the entries under it are. It is only ever called
// with entries to count, the section drawing one line and no heading at all
// where there are none.
func convoSummary(entries []convoEntry) string {
	comments, reviews := 0, 0
	for _, e := range entries {
		if e.review {
			reviews++
		} else {
			comments++
		}
	}
	parts := make([]string, 0, 2)
	for _, count := range []struct {
		n    int
		noun string
	}{{comments, "comment"}, {reviews, "review"}} {
		if count.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s",
				count.n, plural(count.n, count.noun, count.noun+"s")))
		}
	}
	return strings.Join(parts, " · ")
}

// conversationSection is the pull request's conversation as the screen draws
// it: a heading with what it holds counted beside it, and under it one entry
// per thing said, oldest first — a mark and a colour saying what it was, who
// said it, and how long ago, with whatever they wrote rendered under it as
// markdown, exactly as the description above is.
//
// The bodies go through the same glamour the description does, which already
// indents its own document, so an entry's words sit under its line rather than
// beside it and a long comment wraps to the screen instead of running off it.
//
// A pull request nobody has said anything on is one quiet line saying so: an
// empty band under a heading would read as a conversation still loading.
func (p PRView) conversationSection() string {
	entries := conversation(p.pr)
	if len(entries) == 0 {
		return fit(p.styles.Faint.Render("Nothing has been said on this pull request."), p.width)
	}
	blocks := []string{fit(p.styles.Title.Render("Conversation")+"  "+
		p.styles.Faint.Render(convoSummary(entries)), p.width)}
	for _, e := range entries {
		blocks = append(blocks, p.conversationEntry(e))
	}
	return strings.Join(blocks, "\n\n")
}

// conversationEntry is one entry: its own line, and under it the markdown it
// carries. A verdict with no words is the line alone, which is the whole of
// what was said.
func (p PRView) conversationEntry(e convoEntry) string {
	style := e.tone.style(p.styles)
	line := fit(fmt.Sprintf("  %s %s %s  %s",
		style.Render(e.tone.mark()),
		p.styles.Title.Render(e.author),
		style.Render(e.verb),
		p.styles.Faint.Render(ago(timeNow().Sub(e.at)))), p.width)
	if e.body == "" {
		return line
	}
	return line + "\n" + strings.Trim(renderMarkdown(e.body, p.style, p.width), "\n")
}
