package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// milestoneSavedMsg reports a finished milestone write: note is what the status
// bar says when it worked, err what stopped it.
type milestoneSavedMsg struct {
	note string
	err  error
}

// nextMilestoneStatus is the status Q offers to move a milestone to: queued
// work is started, work in flight is finished. A milestone that is already Done
// — or in a status this build does not know — has nowhere to go.
func nextMilestoneStatus(s domain.MilestoneStatus) (domain.MilestoneStatus, bool) {
	switch s {
	case domain.MilestoneQueued:
		return domain.MilestoneActive, true
	case domain.MilestoneActive:
		return domain.MilestoneDone, true
	}
	return "", false
}

// MilestoneForm is the confirm behind Q: one question, because moving a
// milestone along its workflow is a single write with nothing to fill in.
type MilestoneForm struct {
	form    *huh.Form
	heading string

	milestoneID string
	name        string
	// statusType is the shape the milestone's Status column was read in, so the
	// write goes back in the same one.
	statusType string
	next       domain.MilestoneStatus

	confirmed bool
}

// newMilestoneForm returns the confirm for moving a milestone to next.
func newMilestoneForm(theme huh.Theme, m domain.Milestone, next domain.MilestoneStatus) *MilestoneForm {
	f := &MilestoneForm{
		heading:     "Milestone",
		milestoneID: m.ID,
		name:        m.Name,
		statusType:  m.StatusType,
		next:        next,
	}
	f.form = newForm(theme, huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("%s — set %s?", m.Name, next)).
			Value(&f.confirmed),
	))
	return f
}

// Init starts the form.
func (f *MilestoneForm) Init() tea.Cmd { return f.form.Init() }

// Update feeds a message to the form.
func (f *MilestoneForm) Update(msg tea.Msg) tea.Cmd {
	form, cmd := f.form.Update(msg)
	f.form = form.(*huh.Form)
	return cmd
}

// State is how far the form has got.
func (f *MilestoneForm) State() huh.FormState { return f.form.State }

// View renders the form.
func (f *MilestoneForm) View() string { return f.form.View() }

// Heading is the title drawn over the form.
func (f *MilestoneForm) Heading() string { return f.heading }

// SetSize gives the form the room the window leaves it.
func (f *MilestoneForm) SetSize(width, height int) {
	f.form = f.form.WithWidth(width).WithHeight(height)
}

// save writes the new status, or nothing at all when the answer was no.
func (f *MilestoneForm) save(a *App) tea.Cmd {
	if !f.confirmed {
		return nil
	}
	return setMilestoneStatus(a.client, f.milestoneID, f.name, f.statusType, f.next)
}

// setMilestoneStatus moves a milestone to a status. Nothing else on the page is
// touched: the plan's shape is the user's, and only where it has got to changes.
func setMilestoneStatus(client NotionAPI, id, name, statusType string, status domain.MilestoneStatus) tea.Cmd {
	return func() tea.Msg {
		properties := map[string]notion.PropertyValue{
			notion.PropStatus: notion.NewChoice(statusType, string(status)),
		}
		if _, err := client.UpdatePageProperties(context.Background(), id, properties); err != nil {
			return milestoneSavedMsg{err: fmt.Errorf("update milestone: %w", err)}
		}
		return milestoneSavedMsg{note: fmt.Sprintf("%q is now %s.", name, status)}
	}
}

// queueMilestone opens the confirm for the milestone the cursor is on. The
// implicit Unassigned group is not a page, so there is nothing there to move.
//
// A derived milestone is refused rather than reordered: Q advances a milestone
// along its workflow, and where the plan is a column's options there is no
// status to advance — the order of the options is the plan itself, and rewriting
// the schema is a different question from where the work has got to.
func (a *App) queueMilestone() tea.Cmd {
	if !a.canWrite() {
		return nil
	}
	m, ok := a.board.SelectedMilestone()
	if !ok {
		return a.showConfirm("Move to a milestone to change its status.", sevWarning)
	}
	if m.Derived {
		return a.showConfirm(fmt.Sprintf(
			"%q has no page of its own — its status follows its slices; reorder the plan in Notion.",
			m.Name), sevWarning)
	}
	next, ok := nextMilestoneStatus(m.Status)
	if !ok {
		return a.showConfirm(fmt.Sprintf("%q is %s — there is nothing to move it to.", m.Name, m.Status), sevWarning)
	}
	return a.openForm(newMilestoneForm(a.styles.FormTheme, m, next))
}
