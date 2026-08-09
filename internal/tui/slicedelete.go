package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/craigmjohnston/notion-agent-tracker/internal/domain"
)

// DeleteSliceForm is the confirm behind d: one question, because trashing a
// page is a single write with nothing to fill in.
type DeleteSliceForm struct {
	form    *huh.Form
	heading string

	sliceID   string
	sliceName string

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
func newDeleteSliceForm(s domain.Slice) *DeleteSliceForm {
	f := &DeleteSliceForm{
		heading:   "Delete a slice",
		sliceID:   s.ID,
		sliceName: s.Name,
	}
	f.form = huh.NewForm(huh.NewGroup(
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

// save trashes the slice, or nothing at all when the answer was no.
func (f *DeleteSliceForm) save(a *App) tea.Cmd {
	if !f.confirmed {
		return nil
	}
	return deleteSlice(a.client, f.sliceID, f.sliceName)
}

// deleteSlice moves a slice's page to the trash. Notion has no hard delete, so
// a slice deleted by mistake is still recoverable in the Notion UI.
func deleteSlice(client NotionAPI, sliceID, sliceName string) tea.Cmd {
	return func() tea.Msg {
		if err := client.TrashPage(context.Background(), sliceID); err != nil {
			return sliceSavedMsg{err: fmt.Errorf("delete slice: %w", err)}
		}
		return sliceSavedMsg{note: fmt.Sprintf("Deleted %q.", sliceName)}
	}
}

// deleteSliceFlow opens the confirm for the slice the cursor is on.
func (a *App) deleteSliceFlow() tea.Cmd {
	if !a.canWrite() {
		return nil
	}
	s, ok := a.board.SelectedSlice()
	if !ok {
		a.note = "Move to a slice to delete it."
		return nil
	}
	if note, refused := claimedNote(s, "deleted"); refused {
		a.note = note
		return nil
	}
	return a.openForm(newDeleteSliceForm(s))
}
