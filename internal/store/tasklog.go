package store

import (
	"context"

	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// relaunchedLine is the one fixed line a relaunch files under
// [notion.RelaunchedHeading] — one constant, used by both backends, so a
// relaunch always reads the same whichever store wrote it.
const relaunchedLine = "Relaunched to pick up the work so far."

// noteText is a Note section's content under its stamp: the provenance
// paragraph, then the note — one rule, so both backends write the same
// section. Every section here is [stamped] as it is written, so the stamp
// comes first and the provenance second.
func noteText(from, text string) string {
	return from + "\n\n" + text
}

// RecordSentBack files review comments on the slice page under a heading of
// their own, in one append. Comments go on before `slice-rework`'s own
// [Notion.ClearBranch] — the same order a hand-back's own note goes on before
// its status write, for the same reason: a slice already cleared back out of
// review would read, to the refusal every write here opens with, as a slice
// never handed back at all, and the comments would be lost rather than
// retried.
func (n *Notion) RecordSentBack(ctx context.Context, id, comments string) error {
	if _, err := n.api.AppendBlockChildren(ctx, id, noteBlocks(notion.SentBackHeading, stamped(clockOr(n.Clock), comments))); err != nil {
		return err
	}
	logging.Action("slice sent back", "slice", id)
	return nil
}

// RecordNote files a note on the slice page under a heading of its own, its
// stamp the first paragraph and its provenance the second, in one append.
func (n *Notion) RecordNote(ctx context.Context, id, from, text string) error {
	if _, err := n.api.AppendBlockChildren(ctx, id, noteBlocks(notion.NoteHeading, stamped(clockOr(n.Clock), noteText(from, text)))); err != nil {
		return err
	}
	logging.Action("slice noted", "slice", id)
	return nil
}

// RecordRelaunch files the one fixed line a relaunch leaves on the slice page
// under a heading of its own, in one append.
func (n *Notion) RecordRelaunch(ctx context.Context, id string) error {
	if _, err := n.api.AppendBlockChildren(ctx, id, noteBlocks(notion.RelaunchedHeading, stamped(clockOr(n.Clock), relaunchedLine))); err != nil {
		return err
	}
	logging.Action("slice relaunched", "slice", id)
	return nil
}

// RecordSentBack appends the review comments to the slice's body, in the
// markdown Notion would render the same section to.
func (l *Local) RecordSentBack(ctx context.Context, id, comments string) error {
	if err := l.appendToBody(ctx, id, "send the slice back", notion.SentBackHeading, stamped(clockOr(l.Clock), comments)); err != nil {
		return err
	}
	logging.Action("slice sent back", "slice", id)
	return nil
}

// RecordNote appends the note to the slice's body, in the markdown Notion
// would render the same section to.
func (l *Local) RecordNote(ctx context.Context, id, from, text string) error {
	if err := l.appendToBody(ctx, id, "note the slice", notion.NoteHeading, stamped(clockOr(l.Clock), noteText(from, text))); err != nil {
		return err
	}
	logging.Action("slice noted", "slice", id)
	return nil
}

// RecordRelaunch appends the relaunch's one fixed line to the slice's body.
func (l *Local) RecordRelaunch(ctx context.Context, id string) error {
	if err := l.appendToBody(ctx, id, "relaunch the slice", notion.RelaunchedHeading, stamped(clockOr(l.Clock), relaunchedLine)); err != nil {
		return err
	}
	logging.Action("slice relaunched", "slice", id)
	return nil
}

// RecordSentBack files the comments locally, then pushes them to the
// workspace.
func (m *Mirrored) RecordSentBack(ctx context.Context, id, comments string) error {
	if err := m.local.RecordSentBack(ctx, id, comments); err != nil {
		return err
	}
	m.push(ctx, id, func() error { return m.remote.RecordSentBack(ctx, id, comments) })
	return nil
}

// RecordNote files the note locally, then pushes it to the workspace.
func (m *Mirrored) RecordNote(ctx context.Context, id, from, text string) error {
	if err := m.local.RecordNote(ctx, id, from, text); err != nil {
		return err
	}
	m.push(ctx, id, func() error { return m.remote.RecordNote(ctx, id, from, text) })
	return nil
}

// RecordRelaunch files the relaunch locally, then pushes it to the workspace.
func (m *Mirrored) RecordRelaunch(ctx context.Context, id string) error {
	if err := m.local.RecordRelaunch(ctx, id); err != nil {
		return err
	}
	m.push(ctx, id, func() error { return m.remote.RecordRelaunch(ctx, id) })
	return nil
}
