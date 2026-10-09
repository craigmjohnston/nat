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

// CancelSliceForm is the confirm behind X: one question, because a cancel
// takes nothing but the user's yes — release's destructive sibling, which
// stops the agent and throws the work away rather than keeping it.
type CancelSliceForm struct {
	form    *huh.Form
	heading string

	slice domain.Slice

	confirmed bool
}

// cancelWarning is what the confirm says under the question: everything a
// cancel throws away, and the one thing it leaves.
const cancelWarning = "WARNING: its agent is stopped, its worktree and branch are deleted with all the work " +
	"so far, and it goes back to Todo. An open pull request is left as it is."

// newCancelSliceForm returns the confirm for cancelling a slice.
func newCancelSliceForm(theme huh.Theme, s domain.Slice) *CancelSliceForm {
	f := &CancelSliceForm{heading: "Cancel a slice", slice: s}
	f.form = newForm(theme, huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("Cancel %q and discard its work?", s.Name)).
			Description(cancelWarning).
			Value(&f.confirmed),
	))
	return f
}

// Init starts the form.
func (f *CancelSliceForm) Init() tea.Cmd { return f.form.Init() }

// Update feeds a message to the form.
func (f *CancelSliceForm) Update(msg tea.Msg) tea.Cmd {
	form, cmd := f.form.Update(msg)
	f.form = form.(*huh.Form)
	return cmd
}

// State is how far the form has got.
func (f *CancelSliceForm) State() huh.FormState { return f.form.State }

// View renders the form.
func (f *CancelSliceForm) View() string { return f.form.View() }

// Heading is the title drawn over the form.
func (f *CancelSliceForm) Heading() string { return f.heading }

// SetSize gives the form the room the window leaves it.
func (f *CancelSliceForm) SetSize(width, height int) {
	f.form = f.form.WithWidth(width).WithHeight(height)
}

// busyNote is what the status line says while the cancel is in flight.
func (f *CancelSliceForm) busyNote() string { return "Cancelling the slice…" }

// save cancels the slice, or nothing at all when the answer was no.
func (f *CancelSliceForm) save(a *App) tea.Cmd {
	if !f.confirmed {
		return nil
	}
	st, cfg, ok := a.activeStore()
	if !ok {
		return nil
	}
	_, name := a.cfg.AssigneeFor(cfg)
	sp := store.ProjectOf(a.cfg.ActiveProjectID, cfg)
	return cancelSlice(st, sp, cfg, a.launcher, newWorktrees(), f.slice, name)
}

// cancelSlice is [actions.Cancel], the flow `nat slice-cancel` runs: the
// slice re-read, its agent stopped — refused where it cannot be — then Todo
// with its Branch and pull request cleared and a line saying who cancelled it,
// then its worktree and branch discarded.
func cancelSlice(st store.Store, sp store.Project, p config.ProjectConfig, t actions.AgentStopper, w Worktrees,
	s domain.Slice, by string) tea.Cmd {
	return func() tea.Msg {
		if _, err := actions.Cancel(context.Background(), st, t, w, sp, p, s.ID, by); err != nil {
			return sliceSavedMsg{err: fmt.Errorf("cancel %q: %w", s.Name, err)}
		}
		return sliceSavedMsg{note: fmt.Sprintf("Cancelled %q: back to Todo, its work discarded.", s.Name), sliceID: s.ID}
	}
}

// cancelSliceFlow opens the confirm for the slice the cursor is on. Only a
// slice in progress has anything to cancel: a Todo one has nothing started,
// and a Done one is merged.
func (a *App) cancelSliceFlow() tea.Cmd {
	if !a.canWrite() {
		return nil
	}
	s, ok := a.board.SelectedSlice()
	if !ok {
		return a.showConfirm("Move to a slice to cancel it.", sevWarning)
	}
	if s.Status != domain.SliceClaimed {
		return a.showConfirm(fmt.Sprintf("%q is %s — only a slice in progress can be cancelled.",
			s.Name, statusWord(s)), sevWarning)
	}
	return a.openForm(newCancelSliceForm(a.styles.FormTheme, s))
}
