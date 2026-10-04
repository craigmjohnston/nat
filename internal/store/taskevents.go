package store

import (
	"regexp"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// TaskEvent is one entry of a slice's task log, read back off its body by
// [TaskEvents] in the order it was written. Kind is one of: "handed_back",
// "sent_back", "relaunched", "released", "blocked", "summary", "follow_ups",
// "note", "checks_failed".
// `nat slice-show --json` adds two more of its own, read off the slice's
// properties rather than its body — see its own doc comment.
type TaskEvent struct {
	Kind string
	// Note is the section's text, trimmed, and "" where there is none to
	// show — every kind but "relaunched", whose one line is always the same
	// fixed sentence and so carries nothing worth surfacing a second time.
	Note string
	// By is who released the slice, for a "released" event, and who a "note"
	// came from — its provenance line less the leading "From " — for a note,
	// and likewise for a "sent_back" a checks nudge filed (a review's own
	// comments carry no such line, and no By). Note is the text without it.
	By string
	// FromSlice is the slice a "note" came from, where By reads as the label
	// [SliceLabel] writes — its name and milestone, never an ID, since the
	// page names none. Nil for a note from a person, one with no provenance
	// at all, and every other kind; By is the same either way.
	FromSlice *NoteSource
	// At is when the event was written, off the stamp its section opens with
	// (or, for a release, the time its line names) — the zero time for one
	// written before sections were stamped.
	At time.Time
	// FollowUps is the proposals of a "follow_ups" event alone.
	FollowUps []TaskFollowUp
}

// NoteSource is a slice as a note's provenance names it: by name, and by its
// milestone's name where it has one ("" where it has none).
type NoteSource struct {
	Name      string
	Milestone string
}

// SliceLabel names a slice as a reader of the plan knows it: its name, quoted,
// and its milestone's in brackets after it where it has one — `"Name"
// (Milestone)`. It is what a note's provenance line says of the slice it came
// from, and [sliceLabelOf] is its one reader, kept beside it so the two cannot
// drift apart.
func SliceLabel(name, milestone string) string {
	if milestone != "" {
		return `"` + name + `" (` + milestone + `)`
	}
	return `"` + name + `"`
}

// sliceLabelOf reads a label [SliceLabel] wrote back into the slice it names,
// and false for anything else — a person's name, or text typed by hand. The
// milestone is whatever follows the last `" (`, so a name that itself holds
// quotes or brackets still reads whole.
func sliceLabelOf(label string) (NoteSource, bool) {
	rest, ok := strings.CutPrefix(label, `"`)
	if !ok {
		return NoteSource{}, false
	}
	if inner, ok := strings.CutSuffix(rest, ")"); ok {
		if i := strings.LastIndex(inner, `" (`); i > 0 {
			return NoteSource{Name: inner[:i], Milestone: inner[i+3:]}, true
		}
	}
	if name, ok := strings.CutSuffix(rest, `"`); ok && name != "" {
		return NoteSource{Name: name}, true
	}
	return NoteSource{}, false
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
	// DecidedAt is when it was decided, off the stamp of the Follow-ups
	// triaged section that decided it — the zero time while it is pending,
	// and for a decision recorded before sections were stamped.
	DecidedAt time.Time
}

// releasedLineRe matches [releasedLine]'s own text, capturing the assignee it
// named and, where it names one, when. It is a bare paragraph, never a
// heading, so [TaskEvents] watches for it on every line of whatever section it
// turns up inside rather than only at a section boundary. The time is
// optional: a line written before releases named one still reads.
var releasedLineRe = regexp.MustCompile(`^Released back to Todo by (.+?)(?: at (\d{4}-\d\d-\d\dT\S+))?: the session working it ended without finishing it\.$`)

// releasedBy reports the name a release's line named, and when where it says
// so (the zero time where it does not, or names one that will not parse),
// trimmed first the way every other heading match here is.
func releasedBy(line string) (string, time.Time, bool) {
	m := releasedLineRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return "", time.Time{}, false
	}
	at, _ := time.Parse(time.RFC3339, m[2])
	return m[1], at, true
}

// TaskEvents reads a slice's whole task log off its body, top to bottom: one
// event per Handed back, Sent back, Relaunched, Checks failed, Blocked,
// Summary, Note and Follow-ups section, plus one for every Released-back-to-Todo paragraph,
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
//
// Each section's stamp — its first paragraph, `At <RFC 3339>` — is read off
// into the event's At and is no part of its text; a section with none (one
// written before stamps were) reads exactly as it always did, at the zero
// time. A Follow-ups triaged section is a record against an earlier event,
// not an event of its own: its stamp is each item it decides' DecidedAt.
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
	// proposedAt is the stamp the Follow-ups section being read opened with,
	// and decidedAt the Follow-ups triaged section's.
	var proposedAt, decidedAt time.Time
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
		at, text := unstamped(strings.TrimSpace(strings.Join(curLines, "\n")))
		switch curKind {
		case relaunchedKind:
			events = append(events, TaskEvent{Kind: relaunchedKind, At: at})
		case noteKind:
			by, note := noteParts(text)
			e := TaskEvent{Kind: noteKind, Note: note, By: by, At: at}
			if src, ok := sliceLabelOf(by); ok {
				e.FromSlice = &src
			}
			events = append(events, e)
		case sentBackKind:
			// A Sent back opened by a provenance line was filed by something
			// other than the user — a checks nudge — and says so in By.
			by, note := noteParts(text)
			events = append(events, TaskEvent{Kind: sentBackKind, Note: note, By: by, At: at})
		default:
			events = append(events, TaskEvent{Kind: curKind, Note: text, At: at})
		}
		curKind, curLines = "", nil
	}
	closeProposals := func() {
		closeItem()
		events = append(events, TaskEvent{Kind: followUpsKind, FollowUps: taskFollowUpsOf(items), At: proposedAt})
		lastFollowUpsIdx = len(events) - 1
		items, proposedAt = nil, time.Time{}
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
		if by, at, ok := releasedBy(line); ok {
			closeCurrent()
			in, level, curKind = outside, 0, ""
			events = append(events, TaskEvent{Kind: releasedKind, By: by, At: at})
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
		case h > 0 && strings.EqualFold(text, notion.ChecksFailedHeading):
			closeCurrent()
			in, level, curKind, curLines = otherSection, h, ChecksFailedKind, nil
			continue
		case h > 0 && strings.EqualFold(text, notion.NoteHeading):
			closeCurrent()
			in, level, curKind, curLines = otherSection, h, noteKind, nil
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
			in, level, decidedAt = record, h, time.Time{}
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
			} else if t, ok := stampAt(line); ok && len(items) == 0 {
				// The stamp is the section's first paragraph, before any item.
				proposedAt = t
			}
		case record:
			if title, dec, link, ok := triagedEntry(line); ok {
				applyDecision(events, lastFollowUpsIdx, title, dec, link, decidedAt)
			} else if t, ok := stampAt(line); ok && decidedAt.IsZero() {
				// The stamp is the section's first paragraph, before any entry.
				decidedAt = t
			}
		}
	}
	closeCurrent()
	return events
}

// HasHistory reports whether a task log says the slice has been worked: any
// event at all but a note. A note alone is not history — `slice-note` leaves
// one on a slice nobody has launched yet, as context for whoever first does —
// so a slice carrying only notes launches as a fresh launch, not a relaunch.
// It is the one statement of the rule; gnat's Thread applies the same one
// to decide whether a log opens at all.
func HasHistory(events []TaskEvent) bool {
	for _, e := range events {
		if e.Kind != noteKind {
			return true
		}
	}
	return false
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
	noteKind       = "note"
	// ChecksFailedKind and SentBackKind are exported, unlike the rest,
	// because actions.NoticeFailingChecks reads them back to tell a failure
	// already on the record from news.
	ChecksFailedKind = "checks_failed"
	SentBackKind     = sentBackKind
)

// Fixing reports whether a slice is under a fix: in progress, a pull request
// recorded — approved, so the work is out — and the latest event of its task
// log, read off body, a return to work (a Relaunched, which a fix launch
// files, or a Sent back, which a review's comments or a failing reading's
// nudge does). A Handed back after it is the fix in, and the slice back at the
// pull request; any other event, or none, is the same.
//
// It is read off the record alone, so every reader — `nat info`, `slice-show`
// and the app after a restart — agrees on it without a live session or any
// memory of who launched what.
func Fixing(s domain.Slice, body string) bool {
	if s.Status != domain.SliceClaimed || s.PRURL == "" {
		return false
	}
	events := TaskEvents(body)
	if len(events) == 0 {
		return false
	}
	switch events[len(events)-1].Kind {
	case relaunchedKind, sentBackKind:
		return true
	}
	return false
}

// notePrefix opens the provenance paragraph `slice-note` writes first in a
// Note section.
const notePrefix = "From "

// noteParts splits a Note section's text — its stamp already read off it by
// [unstamped], where it had one — into who it came from and the note
// itself. A section whose first paragraph is not a provenance line — one
// typed onto the page by hand — is all note, from nobody named.
func noteParts(text string) (by, note string) {
	first, rest, _ := strings.Cut(text, "\n\n")
	if !strings.HasPrefix(first, notePrefix) || strings.Contains(first, "\n") {
		return "", text
	}
	return strings.TrimSpace(strings.TrimPrefix(first, notePrefix)), strings.TrimSpace(rest)
}

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
// all, which names nothing to decide. at is the record's stamp, the zero
// time where it has none.
func applyDecision(events []TaskEvent, idx int, title string, dec Decision, link string, at time.Time) {
	if idx < 0 || idx >= len(events) {
		return
	}
	for i := range events[idx].FollowUps {
		if events[idx].FollowUps[i].Title == title {
			events[idx].FollowUps[i].Decision = decisionString(dec)
			events[idx].FollowUps[i].Link = link
			events[idx].FollowUps[i].DecidedAt = at
			return
		}
	}
}
