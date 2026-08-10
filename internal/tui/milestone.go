package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

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

// milestonePrompt is the question behind Q: one question, because moving a
// milestone along its workflow is a single write with nothing to fill in. The
// status type is the shape the milestone's Status column was read in, so the
// write goes back in the same one.
func milestonePrompt(m domain.Milestone, next domain.MilestoneStatus) prompt {
	return prompt{
		question: fmt.Sprintf("%q → %s?", m.Name, next),
		busy:     savingNote,
		confirm: func(a *App) tea.Cmd {
			return setMilestoneStatus(a.client, m.ID, m.Name, m.StatusType, next)
		},
	}
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

// queueMilestone asks whether to move the milestone the cursor is on. The
// implicit Unassigned group is not a page, so there is nothing there to move.
func (a *App) queueMilestone() tea.Cmd {
	if !a.canWrite() {
		return nil
	}
	m, ok := a.board.SelectedMilestone()
	if !ok {
		a.note = "Move to a milestone to change its status."
		return nil
	}
	next, ok := nextMilestoneStatus(m.Status)
	if !ok {
		a.note = fmt.Sprintf("%q is %s — there is nothing to move it to.", m.Name, m.Status)
		return nil
	}
	a.ask(milestonePrompt(m, next))
	return nil
}
