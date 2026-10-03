package store

import (
	"regexp"
	"strings"

	"github.com/craigmjohnston/nat/internal/notion"
)

// TaskEvent is one entry of a slice's task log, read back off its body by
// [TaskEvents] in the order it was written. Kind is one of: "handed_back",
// "sent_back", "relaunched", "released", "blocked", "summary", "follow_ups".
// `nat slice-show --json` adds two more of its own, read off the slice's
// properties rather than its body — see its own doc comment.
type TaskEvent struct {
	Kind string
	// Note is the section's text, trimmed, and "" where there is none to
	// show — every kind but "relaunched", whose one line is always the same
	// fixed sentence and so carries nothing worth surfacing a second time.
	Note string
	// By is who released the slice, for a "released" event alone.
	By string
	// FollowUps is the proposals of a "follow_ups" event alone.
	FollowUps []TaskFollowUp
}

// TaskFollowUp is one follow-up as a "follow_ups" event names it: the
// proposal [PendingFollowUps] itself would read, plus whatever a later
// Follow-ups triaged section decided about it.
type TaskFollowUp struct {
	Index int
	Title string
	Brief string
	// Decision is "queued", "folded", "dropped", or "" where the item is
	// still pending the user's decision.
	Decision string
	// Link is the queued slice's URL (or ID), set only where Decision is
	// "queued".
	Link string
}

// releasedLineRe matches [releasedLine]'s own text, capturing the assignee it
// named. It is a bare paragraph, never a heading, so [TaskEvents] watches for
// it on every line of whatever section it turns up inside rather than only at
// a section boundary.
var releasedLineRe = regexp.MustCompile(`^Released back to Todo by (.+): the session working it ended without finishing it\.$`)

// releasedBy reports the name a release's line named, trimmed first the way
// every other heading match here is.
func releasedBy(line string) (string, bool) {
	m := releasedLineRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return "", false
	}
	return m[1], true
}

// TaskEvents reads a slice's whole task log off its body, top to bottom: one
// event per Handed back, Sent back, Relaunched, Blocked, Summary and
// Follow-ups section, plus one for every Released-back-to-Todo paragraph,
// wherever in a section it falls. Every other heading — PR description,
// Visual changes, a brief's own — is not an event and simply ends whatever
// section came before it.
//
// It walks the body exactly as [lastMarkdownSection] and [PendingFollowUps]
// do: fence-aware, so a section quoting a diff or a shell session is not cut
// short by a line of its own that happens to look like a heading, and a
// heading nested deeper than the section's own is passed over rather than
// ending it — the one difference being that every *matching* heading always
// opens a fresh section of its kind even where it is nested, which is what
// lets a superseded Follow-ups section (one agent's proposals overtaken by a
// later pass before the first was ever triaged) still read as two events
// rather than one.
//
// A Follow-ups section's items are [PendingFollowUps]'s own item parsing,
// unfiltered — every item, not merely the ones still pending — decorated
// with whatever the *next* Follow-ups triaged section after it decided, by
// title, the same match [PendingFollowUps] itself makes.
func TaskEvents(body string) []TaskEvent {
	const (
		outside = iota
		otherSection
		proposals
		record
	)
	var events []TaskEvent
	var curKind string
	var curLines []string
	var items []FollowUp
	var brief []string
	lastFollowUpsIdx := -1

	in, level, fence, indent := outside, 0, "", ""
	briefFence := false

	closeItem := func() {
		if len(items) > 0 && brief != nil {
			items[len(items)-1].Brief = strings.TrimSpace(strings.Join(brief, "\n"))
		}
		brief = nil
	}
	// closeOther is only ever reached through closeCurrent, which calls it
	// exactly when in == otherSection — and every place that sets in to
	// otherSection sets curKind alongside it, so curKind is never empty here.
	closeOther := func() {
		if curKind == relaunchedKind {
			events = append(events, TaskEvent{Kind: relaunchedKind})
		} else {
			events = append(events, TaskEvent{Kind: curKind, Note: strings.TrimSpace(strings.Join(curLines, "\n"))})
		}
		curKind, curLines = "", nil
	}
	closeProposals := func() {
		closeItem()
		events = append(events, TaskEvent{Kind: followUpsKind, FollowUps: taskFollowUpsOf(items)})
		lastFollowUpsIdx = len(events) - 1
		items = nil
	}
	closeCurrent := func() {
		switch in {
		case otherSection:
			closeOther()
		case proposals:
			closeProposals()
		}
	}

	for _, line := range strings.Split(body, "\n") {
		f := fenceOf(line)
		opens := f != "" && fence == ""
		closes := f != "" && fence != "" && strings.HasPrefix(f, fence)
		if opens {
			fence = f
			briefFence = in == proposals && brief != nil && strings.HasPrefix(line, indent)
			if !briefFence {
				closeItem()
			}
		}
		if fence != "" {
			switch {
			case briefFence:
				brief = append(brief, strings.TrimPrefix(line, indent))
			case in == otherSection:
				curLines = append(curLines, line)
			}
			if closes {
				fence = ""
			}
			continue
		}
		if in == proposals && brief != nil && (line == "" || strings.HasPrefix(line, indent)) {
			brief = append(brief, strings.TrimPrefix(line, indent))
			continue
		}
		if by, ok := releasedBy(line); ok {
			closeCurrent()
			in, level, curKind = outside, 0, ""
			events = append(events, TaskEvent{Kind: releasedKind, By: by})
			continue
		}
		h, text := headingOf(line)
		if h > 0 && h <= level {
			closeCurrent()
			in, level, curKind = outside, 0, ""
		}
		switch {
		case h > 0 && strings.EqualFold(text, handedBackHeading):
			closeCurrent()
			in, level, curKind, curLines = otherSection, h, handedBackKind, nil
			continue
		case h > 0 && strings.EqualFold(text, notion.SentBackHeading):
			closeCurrent()
			in, level, curKind, curLines = otherSection, h, sentBackKind, nil
			continue
		case h > 0 && strings.EqualFold(text, notion.RelaunchedHeading):
			closeCurrent()
			in, level, curKind, curLines = otherSection, h, relaunchedKind, nil
			continue
		case h > 0 && strings.EqualFold(text, blockedHeading):
			closeCurrent()
			in, level, curKind, curLines = otherSection, h, blockedKind, nil
			continue
		case h > 0 && strings.EqualFold(text, summaryHeading):
			closeCurrent()
			in, level, curKind, curLines = otherSection, h, summaryKind, nil
			continue
		case h > 0 && strings.EqualFold(text, notion.FollowUpsHeading):
			closeCurrent()
			in, level, items = proposals, h, nil
			continue
		case h > 0 && strings.EqualFold(text, notion.FollowUpsTriagedHeading):
			closeCurrent()
			in, level = record, h
			continue
		}
		switch in {
		case otherSection:
			curLines = append(curLines, line)
		case proposals:
			if m := numberedItem.FindStringSubmatch(line); m != nil {
				closeItem()
				items = append(items, FollowUp{Index: len(items) + 1, Title: strings.TrimSpace(m[2])})
				indent, brief = strings.Repeat(" ", len(m[1])+2), []string{}
			}
		case record:
			if title, dec, link, ok := triagedEntry(line); ok {
				applyDecision(events, lastFollowUpsIdx, title, dec, link)
			}
		}
	}
	closeCurrent()
	return events
}

// The kinds [TaskEvent.Kind] takes — the snake_case wire vocabulary
// `nat slice-show --json` speaks, independent of whichever words the
// headings they are read from happen to use.
const (
	handedBackKind = "handed_back"
	sentBackKind   = "sent_back"
	relaunchedKind = "relaunched"
	blockedKind    = "blocked"
	summaryKind    = "summary"
	releasedKind   = "released"
	followUpsKind  = "follow_ups"
)

// taskFollowUpsOf turns a Follow-ups section's own parsed items into the
// event's own [TaskFollowUp]s, each still pending until [applyDecision]
// hears otherwise.
func taskFollowUpsOf(items []FollowUp) []TaskFollowUp {
	if len(items) == 0 {
		return nil
	}
	out := make([]TaskFollowUp, len(items))
	for i, it := range items {
		out[i] = TaskFollowUp{Index: it.Index, Title: it.Title, Brief: it.Brief}
	}
	return out
}

// applyDecision records a triage record's line on the most recent follow_ups
// event's matching item, by title — the same match [PendingFollowUps] makes
// against its own record. idx is that event's place in events, or -1 where a
// Follow-ups triaged section turns up with no Follow-ups section before it at
// all, which names nothing to decide.
func applyDecision(events []TaskEvent, idx int, title string, dec Decision, link string) {
	if idx < 0 || idx >= len(events) {
		return
	}
	for i := range events[idx].FollowUps {
		if events[idx].FollowUps[i].Title == title {
			events[idx].FollowUps[i].Decision = decisionString(dec)
			events[idx].FollowUps[i].Link = link
			return
		}
	}
}
