package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/store"
)

// DeleteSliceForm is the confirm behind d: one question, because trashing a
// page is a single write with nothing to fill in.
type DeleteSliceForm struct {
	form    *huh.Form
	heading string

	// slice is the slice as the board held it when the confirm opened: its
	// status says whether there is an agent to stop and work to discard, and
	// its milestone is the one the delete may leave empty.
	slice domain.Slice

	confirmed bool
}

// deleteWarning is what the confirm says under the question. A Done slice is
// finished work — the record of it is the only thing left — so deleting one is
// warned about rather than refused, and so is a slice in progress, whose agent
// and work go with it.
func deleteWarning(s domain.Slice) string {
	switch s.Status {
	case domain.SliceDone:
		return "WARNING: this slice is Done. Deleting it drops the record of finished work. " +
			"The page goes to Notion's trash."
	case domain.SliceClaimed:
		return "WARNING: this slice is in progress. Its agent is stopped, and its worktree and branch, " +
			"with any work not yet on a pull request, are discarded. The page goes to Notion's trash."
	}
	return "The page goes to Notion's trash."
}

// newDeleteSliceForm returns the confirm for trashing a slice.
func newDeleteSliceForm(theme huh.Theme, s domain.Slice) *DeleteSliceForm {
	f := &DeleteSliceForm{
		heading: "Delete a slice",
		slice:   s,
	}
	f.form = newForm(theme, huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("Delete %q?", s.Name)).
			Description(deleteWarning(s)).
			Value(&f.confirmed),
	))
	return f
}

// Init starts the form.
func (f *DeleteSliceForm) Init() tea.Cmd { return f.form.Init() }

// Update feeds a message to the form.
func (f *DeleteSliceForm) Update(msg tea.Msg) tea.Cmd {
	form, cmd := f.form.Update(msg)
	f.form = form.(*huh.Form)
	return cmd
}

// State is how far the form has got.
func (f *DeleteSliceForm) State() huh.FormState { return f.form.State }

// View renders the form.
func (f *DeleteSliceForm) View() string { return f.form.View() }

// Heading is the title drawn over the form.
func (f *DeleteSliceForm) Heading() string { return f.heading }

// SetSize gives the form the room the window leaves it.
func (f *DeleteSliceForm) SetSize(width, height int) {
	f.form = f.form.WithWidth(width).WithHeight(height)
}

// save trashes the slice, or nothing at all when the answer was no.
func (f *DeleteSliceForm) save(a *App) tea.Cmd {
	if !f.confirmed {
		return nil
	}
	st, cfg, ok := a.activeStore()
	if !ok {
		return nil
	}
	sp := store.ProjectOf(a.cfg.ActiveProjectID, cfg)
	return deleteSlice(st, sp, cfg, a.launcher, newWorktrees(), f.slice)
}

// deleteSlice moves a slice's page to the trash. Notion has no hard delete, so
// a slice deleted by mistake is still recoverable in the Notion UI. It is
// [actions.Delete], the flow `nat slice-delete` runs: a slice in progress has
// its agent stopped first — refused where it cannot be — and its worktree and
// branch discarded after; any other has its worktree removed the safe way. The
// milestone it was filed under is removed where the delete left it with no
// slice at all.
func deleteSlice(st store.Store, sp store.Project, p config.ProjectConfig, t actions.AgentStopper, w Worktrees,
	s domain.Slice) tea.Cmd {
	return func() tea.Msg {
		removed, err := actions.Delete(context.Background(), st, t, w, sp, p, s)
		if err != nil {
			return sliceSavedMsg{err: fmt.Errorf("delete %q: %w", s.Name, err)}
		}
		msg := sliceSavedMsg{note: fmt.Sprintf("Deleted %q.", s.Name), sliceID: s.ID, deleted: true}
		return msg.pruned(removed)
	}
}

// deleteSliceFlow opens the confirm for the slice the cursor is on.
func (a *App) deleteSliceFlow() tea.Cmd {
	if !a.canWrite() {
		return nil
	}
	s, ok := a.board.SelectedSlice()
	if !ok {
		return a.showConfirm("Move to a slice to delete it.", sevWarning)
	}
	return a.openForm(newDeleteSliceForm(a.styles.FormTheme, s))
}
