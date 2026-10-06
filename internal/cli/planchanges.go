package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/store"
)

// planMove refiles a slice the project already has, as `nat slice-move` does:
// Slice names it by title, Milestone the milestone it goes under — one the
// project has, or one the same document creates.
type planMove struct {
	Slice     string `json:"slice"`
	Milestone string `json:"milestone"`
}

// planEdit retitles a slice the project already has, replaces its brief, or
// both, as `nat slice-edit` does: Title is the new title, Description the
// whole new body, not an append. Each is optional, but not both. Title is
// omitted when empty, so a proposal written before it existed round-trips
// unchanged; Description is not, since the app reads it as always present.
type planEdit struct {
	Slice       string `json:"slice"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description"`
}

// changesAnything reports whether the plan touches a slice already on the
// board — the other reason, beside a dependency, to read the project's slices.
func (p plan) changesAnything() bool {
	return len(p.Remove) > 0 || len(p.Move) > 0 || len(p.Edit) > 0
}

// resolvedEdit, resolvedMove and resolvedRemoval are the three lists as
// validation leaves them: each slice read off the board, so applying has
// nothing left to look up.
type resolvedEdit struct {
	slice domain.Slice
	// title is the new title, brief the new brief; either is empty where the
	// edit leaves it alone.
	title, brief string
}

type resolvedMove struct {
	slice domain.Slice
	// newIndex indexes plan.Milestones, or is -1 when existing holds the answer.
	newIndex int
	existing domain.Milestone
}

type resolvedRemoval struct {
	slice domain.Slice
	// dependents is every slice left on the board that waited on this one,
	// for the output to name: the run drops that wait.
	dependents []domain.Slice
}

// planChanges is what the document does to slices already on the board, in
// the order it is applied: edits, moves, removals. unhooked is every slice
// left on the board that waited on a removed one, its DependsOn already
// without them — one write each, made before the removals, so no slice is
// left waiting on a page that is gone (a trashed Notion page keeps its
// relations, and a wait on it would never end).
type planChanges struct {
	edits    []resolvedEdit
	moves    []resolvedMove
	removals []resolvedRemoval
	unhooked []domain.Slice
}

// resolveChanges checks the remove, move and edit lists against the board and
// returns them resolved, beside the board as it will stand once the removals
// are made — removed slices gone, and every wait on one dropped — which is the
// board depends_on is resolved against and the cycle check walks. gone holds
// the removed slices' titles, lower-cased, so a dependency naming one is
// refused as that rather than as a title nobody has. newMilestones is the
// document's own milestones by lower-cased name.
//
// Each list names a Todo slice by title, matched exactly as depends_on is; a
// slice in progress or Done is refused by the lifecycle rule, and a removed
// slice may not be moved or edited as well, nor any slice named twice in one
// list.
func resolveChanges(p plan, newMilestones map[string]int, existing []domain.Milestone, board []domain.Slice) (planChanges, []domain.Slice, map[string]bool, error) {
	var changes planChanges
	byTitle := map[string][]domain.Slice{}
	for _, s := range board {
		key := strings.ToLower(strings.TrimSpace(s.Name))
		byTitle[key] = append(byTitle[key], s)
	}

	removed := map[string]int{}
	gone := map[string]bool{}
	for i, ref := range p.Remove {
		s, err := changedSlice(byTitle, ref, fmt.Sprintf("remove %d", i+1))
		if err != nil {
			return planChanges{}, nil, nil, err
		}
		key := domain.NormaliseID(s.ID)
		if _, dup := removed[key]; dup {
			return planChanges{}, nil, nil, fmt.Errorf("remove %d names %q, which the list already removes", i+1, s.Name)
		}
		removed[key] = len(changes.removals)
		gone[strings.ToLower(strings.TrimSpace(s.Name))] = true
		changes.removals = append(changes.removals, resolvedRemoval{slice: s})
	}

	moved := map[string]bool{}
	for i, m := range p.Move {
		what := fmt.Sprintf("move %d", i+1)
		s, err := changedSlice(byTitle, m.Slice, what)
		if err != nil {
			return planChanges{}, nil, nil, err
		}
		key := domain.NormaliseID(s.ID)
		if err := notRemoved(removed, key, what, s); err != nil {
			return planChanges{}, nil, nil, err
		}
		if moved[key] {
			return planChanges{}, nil, nil, fmt.Errorf("%s names %q, which the list already moves", what, s.Name)
		}
		moved[key] = true
		ref := strings.TrimSpace(m.Milestone)
		if ref == "" {
			return planChanges{}, nil, nil, fmt.Errorf("%s (%q) names no milestone", what, s.Name)
		}
		if idx, ok := newMilestones[strings.ToLower(ref)]; ok {
			changes.moves = append(changes.moves, resolvedMove{slice: s, newIndex: idx})
			continue
		}
		to, err := resolveMilestone(ref, existing)
		if err != nil {
			return planChanges{}, nil, nil, fmt.Errorf("%s (%q): %w", what, s.Name, err)
		}
		if s.MilestoneID == to.ID {
			return planChanges{}, nil, nil, fmt.Errorf("%s (%q): it is already filed under %s", what, s.Name, to.Name)
		}
		changes.moves = append(changes.moves, resolvedMove{slice: s, newIndex: -1, existing: to})
	}

	edited := map[string]bool{}
	for i, e := range p.Edit {
		what := fmt.Sprintf("edit %d", i+1)
		s, err := changedSlice(byTitle, e.Slice, what)
		if err != nil {
			return planChanges{}, nil, nil, err
		}
		key := domain.NormaliseID(s.ID)
		if err := notRemoved(removed, key, what, s); err != nil {
			return planChanges{}, nil, nil, err
		}
		if edited[key] {
			return planChanges{}, nil, nil, fmt.Errorf("%s names %q, which the list already edits", what, s.Name)
		}
		edited[key] = true
		title, brief := strings.TrimSpace(e.Title), strings.TrimSpace(e.Description)
		if title == "" && brief == "" {
			return planChanges{}, nil, nil, fmt.Errorf("%s (%q) has neither a title nor a description: "+
				"give it a new title, a new brief, or both", what, s.Name)
		}
		if err := domain.CheckSliceTitle(title); err != nil {
			return planChanges{}, nil, nil, fmt.Errorf("%s (%q): %w", what, s.Name, err)
		}
		changes.edits = append(changes.edits, resolvedEdit{slice: s, title: title, brief: brief})
	}

	// The board as the removals leave it.
	remaining := make([]domain.Slice, 0, len(board))
	for _, s := range board {
		if _, ok := removed[domain.NormaliseID(s.ID)]; ok {
			continue
		}
		kept := make([]string, 0, len(s.DependsOn))
		for _, id := range s.DependsOn {
			idx, ok := removed[domain.NormaliseID(id)]
			if !ok {
				kept = append(kept, id)
				continue
			}
			changes.removals[idx].dependents = append(changes.removals[idx].dependents, s)
		}
		if len(kept) != len(s.DependsOn) {
			s.DependsOn = kept
			changes.unhooked = append(changes.unhooked, s)
		}
		remaining = append(remaining, s)
	}
	return changes, remaining, gone, nil
}

// changedSlice finds the one Todo slice an entry of the remove, move or edit
// list names. what is the entry ("move 2"), so a refusal points at it.
func changedSlice(byTitle map[string][]domain.Slice, ref, what string) (domain.Slice, error) {
	title := strings.TrimSpace(ref)
	if title == "" {
		return domain.Slice{}, fmt.Errorf("%s names no slice", what)
	}
	matches := byTitle[strings.ToLower(title)]
	switch len(matches) {
	case 1:
	case 0:
		return domain.Slice{}, fmt.Errorf("%s names %q, which the project has no slice named", what, title)
	default:
		return domain.Slice{}, fmt.Errorf("%s names %q, which the project has %d slices named: rename one first",
			what, title, len(matches))
	}
	s := matches[0]
	switch s.Status {
	case domain.SliceClaimed:
		return domain.Slice{}, fmt.Errorf("%s names %q, which is in progress: work in flight is not changed under its agent",
			what, s.Name)
	case domain.SliceDone:
		return domain.Slice{}, fmt.Errorf("%s names %q, which is already Done: a finished slice is not changed after the fact",
			what, s.Name)
	}
	return s, nil
}

// notRemoved refuses a move or edit of a slice the same document removes:
// one of the two is a mistake, and which one is not this command's to guess.
func notRemoved(removed map[string]int, key, what string, s domain.Slice) error {
	if _, ok := removed[key]; ok {
		return fmt.Errorf("%s names %q, which the plan also removes", what, s.Name)
	}
	return nil
}

// appliedEdit, appliedMove and appliedRemoval are what the run did to slices
// already on the board, for its output.
type appliedEdit struct {
	Slice domain.Slice
	// Title is the new title and Brief the new brief, each empty where the
	// edit left it alone.
	Title, Brief string
}

type appliedMove struct {
	Slice     domain.Slice
	Milestone domain.Milestone
}

type appliedRemoval struct {
	Slice      domain.Slice
	Dependents []domain.Slice
}

// applyEdits retitles each edited slice and replaces its brief, whichever of
// the two the edit gives.
func applyEdits(ctx context.Context, st store.Store, edits []resolvedEdit, applied *appliedPlan) error {
	for _, e := range edits {
		if e.title != "" {
			if err := st.SetSliceTitle(ctx, e.slice.ID, e.title); err != nil {
				return fmt.Errorf("edit %q: %w", e.slice.Name, err)
			}
		}
		if e.brief != "" {
			if err := st.SetSliceBrief(ctx, e.slice.ID, e.brief); err != nil {
				return fmt.Errorf("edit %q: %w", e.slice.Name, err)
			}
		}
		logging.Action("plan slice edited", "slice", e.slice.ID, "retitled", e.title != "")
		applied.Edited = append(applied.Edited, appliedEdit{Slice: e.slice, Title: e.title, Brief: e.brief})
	}
	return nil
}

// applyMoves refiles each moved slice, once the document's own milestones —
// which a move may name — exist.
func applyMoves(ctx context.Context, st store.Store, moves []resolvedMove, applied *appliedPlan) error {
	for _, m := range moves {
		to := m.existing
		if m.newIndex >= 0 {
			to = applied.Milestones[m.newIndex]
		}
		if err := st.MoveSlice(ctx, m.slice.ID, to); err != nil {
			return fmt.Errorf("move %q: %w", m.slice.Name, err)
		}
		logging.Action("plan slice moved", "slice", m.slice.ID, "milestone", to.Name)
		applied.Moved = append(applied.Moved, appliedMove{Slice: m.slice, Milestone: to})
	}
	return nil
}

// applyRemovals drops every wait on a removed slice, then removes each one as
// slice-delete does.
func applyRemovals(ctx context.Context, st store.Store, changes planChanges, applied *appliedPlan) error {
	for _, s := range changes.unhooked {
		if _, err := st.SetDependencies(ctx, s.ID, s.DependsOn); err != nil {
			return fmt.Errorf("drop what %q waited on of the slices the plan removes: %w", s.Name, err)
		}
		logging.Action("plan dependencies dropped", "slice", s.ID, "depends_on", len(s.DependsOn))
	}
	for _, r := range changes.removals {
		if err := st.DeleteSlice(ctx, r.slice.ID); err != nil {
			return fmt.Errorf("remove %q: %w", r.slice.Name, err)
		}
		logging.Action("plan slice removed", "slice", r.slice.ID)
		applied.Removed = append(applied.Removed, appliedRemoval{Slice: r.slice, Dependents: r.dependents})
	}
	return nil
}

// changed reports whether the run touched any slice already on the board.
func (a appliedPlan) changed() bool {
	return len(a.Edited) > 0 || len(a.Moved) > 0 || len(a.Removed) > 0
}

// changeCounts phrases how many slices already on the board a run (or a
// proposal) edits, moves and removes.
func changeCounts(edited, moved, removed int) string {
	return fmt.Sprintf("%d edited, %d moved and %d removed", edited, moved, removed)
}

// removedSliceJSON is one slice the run removed, and the slices on the board
// whose wait on it the run dropped.
type removedSliceJSON struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	URL        string          `json:"url,omitempty"`
	Dependents []namedSliceRef `json:"dependents"`
}

// namedSliceRef is a slice by ID and name, for a list that only points at one.
type namedSliceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// changesJSON fills in the three lists of what the run did to slices already
// on the board, each an empty list rather than null.
func (a appliedPlan) changesJSON(doc *planAppliedJSON) {
	doc.Edited = make([]sliceEditedJSON, 0, len(a.Edited))
	for _, e := range a.Edited {
		doc.Edited = append(doc.Edited, sliceEditedJSON{ID: e.Slice.ID, Name: e.Slice.Name, URL: e.Slice.URL, Title: e.Title, Brief: e.Brief})
	}
	doc.Moved = make([]sliceMovedJSON, 0, len(a.Moved))
	for _, m := range a.Moved {
		doc.Moved = append(doc.Moved, sliceMovedJSON{
			ID: m.Slice.ID, Name: m.Slice.Name, URL: m.Slice.URL,
			MilestoneID: m.Milestone.ID, MilestoneName: m.Milestone.Name,
		})
	}
	doc.Removed = make([]removedSliceJSON, 0, len(a.Removed))
	for _, r := range a.Removed {
		deps := make([]namedSliceRef, 0, len(r.Dependents))
		for _, d := range r.Dependents {
			deps = append(deps, namedSliceRef{ID: d.ID, Name: d.Name})
		}
		doc.Removed = append(doc.Removed, removedSliceJSON{ID: r.Slice.ID, Name: r.Slice.Name, URL: r.Slice.URL, Dependents: deps})
	}
}

// changesMarkdown reports what the run did to slices already on the board, in
// the order it did it, under the work it created.
func (a appliedPlan) changesMarkdown(b *strings.Builder) {
	if len(a.Edited) > 0 {
		b.WriteString("\n## Edited\n\n")
		for _, e := range a.Edited {
			fmt.Fprintf(b, "- %s — %s\n", e.Slice.Name, editedWhat(e.Title, e.Brief))
		}
	}
	if len(a.Moved) > 0 {
		b.WriteString("\n## Moved\n\n")
		for _, m := range a.Moved {
			fmt.Fprintf(b, "- %s — now under %s\n", m.Slice.Name, m.Milestone.Name)
		}
	}
	if len(a.Removed) > 0 {
		b.WriteString("\n## Removed\n\n")
		for _, r := range a.Removed {
			fmt.Fprintf(b, "- %s", r.Slice.Name)
			if len(r.Dependents) > 0 {
				names := make([]string, len(r.Dependents))
				for i, d := range r.Dependents {
					names[i] = d.Name
				}
				fmt.Fprintf(b, " — no longer waited on by %s", strings.Join(quoteAll(names), ", "))
			}
			b.WriteString("\n")
		}
	}
	if len(a.MilestonesRemoved) > 0 {
		b.WriteString("\n## Milestones removed\n\n")
		for _, name := range a.MilestonesRemoved {
			fmt.Fprintf(b, "- %s — no slice is filed under it any more\n", name)
		}
	}
}

// editedWhat says what an edit changed: the title, the brief, or both.
func editedWhat(title, brief string) string {
	switch {
	case title == "":
		return "brief replaced"
	case brief == "":
		return fmt.Sprintf("renamed %q", title)
	}
	return fmt.Sprintf("renamed %q, brief replaced", title)
}

// titleHolder is what already answers to a title, for the refusal of a
// second slice under it: what it is, and what to do instead.
type titleHolder struct {
	what, hint string
}

// distinctHint is the refusal's advice where the title is held by something
// the plan cannot change into the new slice.
const distinctHint = "give this one a title of its own"

// checkDuplicateTitles refuses a document that would leave two slices of the
// project answering to one title — matched trimmed and case-insensitive, as
// every title match is. Every later edit, move, remove or depends_on naming
// that title would be refused as ambiguous, so the duplicate is refused at
// the start instead: a created slice whose title the board already holds,
// one a retitling edit gives, or another created slice's; and an edit's new
// title already held. board is the project's slices as the removals leave
// them, so a document removing a slice may create its replacement under the
// same title. Slices the plan does not touch that already share a title are
// not the document's doing and are left alone.
func checkDuplicateTitles(p plan, board []domain.Slice, edits []resolvedEdit) error {
	renamed := map[string]bool{}
	for _, e := range edits {
		if e.title != "" {
			renamed[domain.NormaliseID(e.slice.ID)] = true
		}
	}
	held := map[string]titleHolder{}
	for _, s := range board {
		key := strings.ToLower(strings.TrimSpace(s.Name))
		if _, ok := held[key]; ok || renamed[domain.NormaliseID(s.ID)] {
			continue
		}
		hint := distinctHint
		if s.Status == domain.SliceTodo {
			hint = "edit it to change its brief, or remove it to replace it"
		}
		held[key] = titleHolder{what: "is already on the board as " + withArticle(s.StatusName, s.Status) + " slice", hint: hint}
	}
	for i, e := range edits {
		if e.title == "" {
			continue
		}
		key := strings.ToLower(e.title)
		if h, ok := held[key]; ok {
			return fmt.Errorf("edit %d renames %q to %q, which %s: %s", i+1, e.slice.Name, e.title, h.what, distinctHint)
		}
		held[key] = titleHolder{what: fmt.Sprintf("is the title edit %d gives %q", i+1, e.slice.Name), hint: distinctHint}
	}
	for i, s := range p.Slices {
		title := strings.TrimSpace(s.Title)
		key := strings.ToLower(title)
		if h, ok := held[key]; ok {
			return fmt.Errorf("slice %d (%q) %s: %s", i+1, title, h.what, h.hint)
		}
		held[key] = titleHolder{what: fmt.Sprintf("is already slice %d of the plan", i+1), hint: "give each its own title"}
	}
	return nil
}

// withArticle names a slice's status with its article — "a Todo", "an In
// progress" — as the project spells it, falling back to the status itself.
func withArticle(name string, status domain.SliceStatus) string {
	if name == "" {
		name = string(status)
	}
	if name != "" && strings.ContainsRune("AEIOUaeiou", rune(name[0])) {
		return "an " + name
	}
	return "a " + name
}
