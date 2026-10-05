package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/store"
)

// DeleteSliceForm is the confirm behind d: one question, because trashing a
// page is a single write with nothing to fill in.
type DeleteSliceForm struct {
	form    *huh.Form
	heading string

	sliceID   string
	sliceName string
	// from is the ID of the milestone the slice is filed under, which the
	// delete may leave empty.
	from string

	confirmed bool
}

// deleteWarning is what the confirm says under the question. A Done slice is
// finished work — the record of it is the only thing left — so deleting one is
// warned about rather than refused.
func deleteWarning(s domain.Slice) string {
	if s.Status == domain.SliceDone {
		return "WARNING: this slice is Done. Deleting it drops the record of finished work. " +
			"The page goes to Notion's trash."
	}
	return "The page goes to Notion's trash."
}

// newDeleteSliceForm returns the confirm for trashing a slice.
func newDeleteSliceForm(theme huh.Theme, s domain.Slice) *DeleteSliceForm {
	f := &DeleteSliceForm{
		heading:   "Delete a slice",
		sliceID:   s.ID,
		sliceName: s.Name,
		from:      s.MilestoneID,
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
	return deleteSlice(st, sp, f.sliceID, f.sliceName, f.from)
}

// deleteSlice moves a slice's page to the trash. Notion has no hard delete, so
// a slice deleted by mistake is still recoverable in the Notion UI. The
// milestone it was filed under is removed where the delete left it with no
// slice at all.
func deleteSlice(st store.Store, sp store.Project, sliceID, sliceName, from string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if err := st.DeleteSlice(ctx, sliceID); err != nil {
			return sliceSavedMsg{err: fmt.Errorf("delete slice: %w", err)}
		}
		msg := sliceSavedMsg{note: fmt.Sprintf("Deleted %q.", sliceName), sliceID: sliceID, deleted: true}
		return msg.pruned(actions.PruneEmptied(ctx, st, sp, from))
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
	if note, refused := claimedNote(s, "deleted"); refused {
		return a.showConfirm(note, sevWarning)
	}
	return a.openForm(newDeleteSliceForm(a.styles.FormTheme, s))
}
