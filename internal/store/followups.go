package store

import (
	"context"
	"database/sql"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// FollowUp is one piece of work an agent noticed beside its slice and handed
// in rather than doing: what it is called and what it is. Batch is the
// Follow-ups section it was handed in under — the section's ordinal among the
// body's Follow-ups sections, 1-based — and Index its place among every
// follow-up still pending on the slice, whichever batch, 1-based: the number
// the user triages it by. Both are ignored on a write, where the order given is
// the order filed.
type FollowUp struct {
	Batch int
	Index int
	Title string
	Brief string
}

// Decision is what the user chose to do with one follow-up.
type Decision int

// The three decisions a follow-up can be given.
const (
	// Queued is filed as a slice of its own, blocked on the slice it came from.
	Queued Decision = iota
	// FoldedIn is done by the agent as part of the slice it came from.
	FoldedIn
	// Dropped is nothing more than recorded.
	Dropped
)

// Triaged is one follow-up as the triage record names it: its title, which is
// what keys it to the proposal, the decision, and for a queued one where the
// slice it became can be found.
type Triaged struct {
	Title    string
	Decision Decision
	// Link is the queued slice's URL, or its ID where the plan gives it none.
	Link string
}

// line is the record's one line for this follow-up, the bullet's text.
func (t Triaged) line() string {
	switch t.Decision {
	case Queued:
		return "Queued: " + t.Title + queuedArrow + t.Link
	case FoldedIn:
		return foldedPrefix + t.Title
	}
	return droppedPrefix + t.Title
}

// The prefixes a triage record's lines open with, and the arrow a queued one
// names its slice after.
const (
	queuedPrefix  = "Queued: "
	foldedPrefix  = "Folded in: "
	droppedPrefix = "Dropped: "
	queuedArrow   = " → "
)

// numberedItem is a numbered list item opening at the margin: its number, and
// its text.
var numberedItem = regexp.MustCompile(`^(\d+)\. (.*)$`)

// PendingFollowUps is the follow-ups still awaiting the user's decision on a
// slice: every undecided item of every Follow-ups section of its body, in body
// order. Each section is a batch, pending until its own items are decided; no
// section supersedes another. What is decided, and by which record, is
// [TaskEvents]' reading — this is that reading's undecided items and nothing
// else, so the two can never disagree about what a record decided.
func PendingFollowUps(body string) []FollowUp {
	var pending []FollowUp
	for _, e := range TaskEvents(body) {
		for _, f := range e.FollowUps {
			if f.Decision == "" {
				pending = append(pending, FollowUp{Batch: e.Batch, Index: len(pending) + 1, Title: f.Title, Brief: f.Brief})
			}
		}
	}
	return pending
}

// PendingFollowUpsOf is what is pending on s, its body read: [PendingFollowUps],
// except on a Done slice, where nothing is. A Done slice's undecided items are
// history — a batch an earlier nat let a later one supersede, never triaged —
// and nothing waits on them or refuses over them.
func PendingFollowUpsOf(s domain.Slice, body string) []FollowUp {
	if s.Status == domain.SliceDone {
		return nil
	}
	return PendingFollowUps(body)
}

// triagedEntry is everything one of a triage record's lines names: the title
// it keys to the proposal, the decision, and — for a queued item — the slice
// it became, read by [TaskEvents].
func triagedEntry(line string) (title string, decision Decision, link string, ok bool) {
	rest, ok := strings.CutPrefix(line, "- ")
	if !ok {
		return "", 0, "", false
	}
	if t, ok := strings.CutPrefix(rest, queuedPrefix); ok {
		if i := strings.LastIndex(t, queuedArrow); i >= 0 {
			link = strings.TrimSpace(t[i+len(queuedArrow):])
			t = t[:i]
		}
		return strings.TrimSpace(t), Queued, link, true
	}
	if t, ok := strings.CutPrefix(rest, foldedPrefix); ok {
		return strings.TrimSpace(t), FoldedIn, "", true
	}
	if t, ok := strings.CutPrefix(rest, droppedPrefix); ok {
		return strings.TrimSpace(t), Dropped, "", true
	}
	return "", 0, "", false
}

// decisionString is the word [TaskEvents] names a triaged follow-up's
// decision by, in the wire vocabulary the app reads ("queued"/"folded"/
// "dropped") rather than [Decision]'s own int — a decision never otherwise
// crosses out of this package as anything but the write methods that take it.
func decisionString(d Decision) string {
	switch d {
	case Queued:
		return "queued"
	case FoldedIn:
		return "folded"
	case Dropped:
		return "dropped"
	}
	return ""
}

// paragraphsOf is text split the way [paragraphBlocks] splits it: one chunk per
// blank-line-separated run, each trimmed, the empty ones dropped.
func paragraphsOf(text string) []string {
	var out []string
	for _, chunk := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		if trimmed := strings.TrimSpace(chunk); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// followUpsMarkdown is the Follow-ups section's list as notion.Markdown renders
// the blocks [followUpBlocks] writes — a tight numbered list, each item's brief
// indented under its title to the width of its marker, a blank line between its
// paragraphs — so that a plan kept locally and one kept in Notion read back
// alike. It is hand-rolled to that renderer's output, and a test holds the two
// to each other.
func followUpsMarkdown(items []FollowUp) string {
	lines := make([]string, 0, len(items))
	for i, it := range items {
		marker := strconv.Itoa(i+1) + ". "
		indent := strings.Repeat(" ", len(marker))
		item := strings.TrimRight(marker+it.Title, " ")
		paras := paragraphsOf(it.Brief)
		for j, p := range paras {
			sep := "\n"
			if j > 0 {
				sep = "\n\n"
			}
			item += sep + indented(indent, p)
		}
		lines = append(lines, item)
	}
	return strings.Join(lines, "\n")
}

// indented prefixes every line of text with indent, trimming what each line
// ends with as the renderer does.
func indented(indent, text string) string {
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(indent+ln, " ")
	}
	return strings.Join(lines, "\n")
}

// triageMarkdown is the Follow-ups triaged section's list: one bullet per
// follow-up, as notion.Markdown renders [triageBlocks].
func triageMarkdown(items []Triaged) string {
	lines := make([]string, len(items))
	for i, t := range items {
		lines[i] = "- " + t.line()
	}
	return strings.Join(lines, "\n")
}

// followUpBlocks is the Follow-ups section as Notion holds it: a heading, then
// its stamp for at, then one numbered item per follow-up with its title as the
// item's text and its brief as paragraphs nested under it.
func followUpBlocks(at time.Time, items []FollowUp) []map[string]any {
	blocks := []map[string]any{textBlock("heading_3", notion.FollowUpsHeading), textBlock("paragraph", stampLine(at))}
	for _, it := range items {
		b := textBlock("numbered_list_item", it.Title)
		if kids := paragraphBlocks(it.Brief); len(kids) > 0 {
			b["numbered_list_item"].(map[string]any)["children"] = kids
		}
		blocks = append(blocks, b)
	}
	return blocks
}

// triageBlocks is the Follow-ups triaged section as Notion holds it: a heading,
// its stamp for at, then one bullet per follow-up.
func triageBlocks(at time.Time, items []Triaged) []map[string]any {
	blocks := []map[string]any{textBlock("heading_3", notion.FollowUpsTriagedHeading), textBlock("paragraph", stampLine(at))}
	for _, t := range items {
		blocks = append(blocks, textBlock("bulleted_list_item", t.line()))
	}
	return blocks
}

// ProposeFollowUps files the follow-ups on the slice page under a heading of
// their own, in one append.
func (n *Notion) ProposeFollowUps(ctx context.Context, id string, items []FollowUp) error {
	if _, err := n.api.AppendBlockChildren(ctx, id, followUpBlocks(clockOr(n.Clock), items)); err != nil {
		return err
	}
	logging.Action("follow-ups proposed", "slice", id, "count", len(items))
	return nil
}

// RecordTriage files the user's decision on the slice page under a heading of
// its own, in one append.
func (n *Notion) RecordTriage(ctx context.Context, id string, items []Triaged) error {
	if _, err := n.api.AppendBlockChildren(ctx, id, triageBlocks(clockOr(n.Clock), items)); err != nil {
		return err
	}
	logging.Action("follow-ups triaged", "slice", id, "count", len(items))
	return nil
}

// ProposeFollowUps appends the follow-ups to the slice's body, in the markdown
// Notion would render the same section to.
func (l *Local) ProposeFollowUps(ctx context.Context, id string, items []FollowUp) error {
	if err := l.appendToBody(ctx, id, "file the follow-ups", notion.FollowUpsHeading,
		stamped(clockOr(l.Clock), followUpsMarkdown(items))); err != nil {
		return err
	}
	logging.Action("follow-ups proposed", "slice", id, "count", len(items))
	return nil
}

// RecordTriage appends the user's decision to the slice's body.
func (l *Local) RecordTriage(ctx context.Context, id string, items []Triaged) error {
	if err := l.appendToBody(ctx, id, "record the triage", notion.FollowUpsTriagedHeading,
		stamped(clockOr(l.Clock), triageMarkdown(items))); err != nil {
		return err
	}
	logging.Action("follow-ups triaged", "slice", id, "count", len(items))
	return nil
}

// appendToBody files a section at the end of the slice's body as the
// transaction reads it.
func (l *Local) appendToBody(ctx context.Context, id, what, heading, text string) error {
	_, err := l.updateSlice(ctx, id, what, func(tx *sql.Tx, _ domain.Slice) error {
		body, err := l.sliceBody(ctx, tx, id)
		if err != nil {
			return err
		}
		return l.exec(ctx, tx, what, `UPDATE slices SET body = ? WHERE id = ?`,
			appendSection(body, heading, text), id)
	})
	return err
}

// ProposeFollowUps files the follow-ups locally, then pushes them to the
// workspace.
func (m *Mirrored) ProposeFollowUps(ctx context.Context, id string, items []FollowUp) error {
	if err := m.local.ProposeFollowUps(ctx, id, items); err != nil {
		return err
	}
	m.push(ctx, id, func() error { return m.remote.ProposeFollowUps(ctx, id, items) })
	return nil
}

// RecordTriage records the decision locally, then pushes it to the workspace.
func (m *Mirrored) RecordTriage(ctx context.Context, id string, items []Triaged) error {
	if err := m.local.RecordTriage(ctx, id, items); err != nil {
		return err
	}
	m.push(ctx, id, func() error { return m.remote.RecordTriage(ctx, id, items) })
	return nil
}

