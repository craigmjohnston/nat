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

// launchedLine is the one fixed line a fresh launch files under
// [notion.LaunchedHeading], shared by both backends as [relaunchedLine] is.
const launchedLine = "Launched."

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

// RecordResumed files why work on a handed-back slice was taken back up on
// the slice page under a heading of its own, stamped, in one append — before
// `slice-resume`'s own [Notion.ClearBranch], for [Notion.RecordSentBack]'s
// reason.
func (n *Notion) RecordResumed(ctx context.Context, id, note string) error {
	if _, err := n.api.AppendBlockChildren(ctx, id, noteBlocks(notion.ResumedHeading, stamped(clockOr(n.Clock), note))); err != nil {
		return err
	}
	logging.Action("slice resumed", "slice", id)
	return nil
}

// RecordChecksFailed files the checks a pull request failed on the slice page
// under a heading of their own, stamped, in one append.
func (n *Notion) RecordChecksFailed(ctx context.Context, id, checks string) error {
	if _, err := n.api.AppendBlockChildren(ctx, id, noteBlocks(notion.ChecksFailedHeading, stamped(clockOr(n.Clock), checks))); err != nil {
		return err
	}
	logging.Action("slice checks failed", "slice", id)
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

// RecordLaunch files the one fixed line a fresh launch leaves on the slice
// page under a heading of its own, in one append.
func (n *Notion) RecordLaunch(ctx context.Context, id string) error {
	if _, err := n.api.AppendBlockChildren(ctx, id, noteBlocks(notion.LaunchedHeading, stamped(clockOr(n.Clock), launchedLine))); err != nil {
		return err
	}
	logging.Action("slice launched", "slice", id)
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

// RecordResumed appends why the work was taken back up to the slice's body,
// in the markdown Notion would render the same section to.
func (l *Local) RecordResumed(ctx context.Context, id, note string) error {
	if err := l.appendToBody(ctx, id, "resume the slice", notion.ResumedHeading, stamped(clockOr(l.Clock), note)); err != nil {
		return err
	}
	logging.Action("slice resumed", "slice", id)
	return nil
}

// RecordChecksFailed appends the failed checks to the slice's body, in the
// markdown Notion would render the same section to.
func (l *Local) RecordChecksFailed(ctx context.Context, id, checks string) error {
	if err := l.appendToBody(ctx, id, "record the slice's failed checks", notion.ChecksFailedHeading, stamped(clockOr(l.Clock), checks)); err != nil {
		return err
	}
	logging.Action("slice checks failed", "slice", id)
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

// RecordLaunch appends a fresh launch's one fixed line to the slice's body.
func (l *Local) RecordLaunch(ctx context.Context, id string) error {
	if err := l.appendToBody(ctx, id, "record the slice's launch", notion.LaunchedHeading, stamped(clockOr(l.Clock), launchedLine)); err != nil {
		return err
	}
	logging.Action("slice launched", "slice", id)
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

// RecordResumed files the resumption locally, then pushes it to the
// workspace.
func (m *Mirrored) RecordResumed(ctx context.Context, id, note string) error {
	if err := m.local.RecordResumed(ctx, id, note); err != nil {
		return err
	}
	m.push(ctx, id, func() error { return m.remote.RecordResumed(ctx, id, note) })
	return nil
}

// RecordChecksFailed files the failed checks locally, then pushes them to the
// workspace.
func (m *Mirrored) RecordChecksFailed(ctx context.Context, id, checks string) error {
	if err := m.local.RecordChecksFailed(ctx, id, checks); err != nil {
		return err
	}
	m.push(ctx, id, func() error { return m.remote.RecordChecksFailed(ctx, id, checks) })
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

// RecordLaunch files the launch locally, then pushes it to the workspace.
//
// It is written straight after the launch's own claim, whose push may have
// failed and left the slice ahead of the workspace — and the line landing is
// not the claim landing. So only a slice that was level before this write is
// marked sent once the line is pushed; one already ahead stays ahead, for a
// later sync to send what the claim's push could not, and a pull meanwhile
// not to read the claim back off a workspace that never heard of it. A sync
// state that cannot be read is taken as ahead: at worst the slice is sent
// again.
func (m *Mirrored) RecordLaunch(ctx context.Context, id string) error {
	ahead, err := m.local.Dirty(ctx, id)
	ahead = ahead || err != nil
	if err := m.local.RecordLaunch(ctx, id); err != nil {
		return err
	}
	send := func() error { return m.remote.RecordLaunch(ctx, id) }
	if !ahead {
		m.push(ctx, id, send)
		return nil
	}
	if err := send(); err != nil {
		logging.Error("push to the workspace failed, the file is ahead and will send it on the next sync",
			"slice", id, "err", err)
	}
	return nil
}
