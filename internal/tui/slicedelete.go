package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/domain"
)

// deleteSlicePrompt is the question behind d: one question, because trashing a
// page is a single write with nothing to fill in.
func deleteSlicePrompt(s domain.Slice) prompt {
	return prompt{
		question: deleteQuestion(s),
		busy:     savingNote,
		confirm:  func(a *App) tea.Cmd { return deleteSlice(a.client, s.ID, s.Name) },
	}
}

// deleteQuestion is what the bar asks before trashing a slice. A Done slice is
// finished work — the record of it is the only thing left — so deleting one is
// warned about rather than refused.
func deleteQuestion(s domain.Slice) string {
	if s.Status == domain.SliceDone {
		return fmt.Sprintf("Delete %q? It is Done — the record goes with it.", s.Name)
	}
	return fmt.Sprintf("Delete %q? The page goes to Notion's trash.", s.Name)
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

// deleteSliceFlow asks whether to trash the slice the cursor is on.
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
	a.ask(deleteSlicePrompt(s))
	return nil
}
