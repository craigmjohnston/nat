package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// statusWord names a slice's status the way the project's own table does —
// Claimed or In progress, whichever it was created with — so a refusal on the
// status bar reads as what is on the page, falling back to the workflow status
// where the page named none. A slice carrying no status at all still needs a
// word, or the message ends mid-sentence.
func statusWord(s domain.Slice) string {
	switch {
	case s.StatusName != "":
		return s.StatusName
	case s.Status != "":
		return string(s.Status)
	default:
		return "(no status)"
	}
}

// claimedNote is the status-bar message refusing a slice an agent is working
// on, and whether there is one to show. A slice in progress is work in flight:
// the plan may still describe it, but the page underneath belongs to whoever
// took it.
func claimedNote(s domain.Slice, verb string) (string, bool) {
	if s.Status != domain.SliceClaimed {
		return "", false
	}
	return fmt.Sprintf("%q is %s — work in flight cannot be %s.", s.Name, statusWord(s), verb), true
}

// MoveSliceForm is the picker behind m: the milestones the slice could be filed
// under instead, in plan order.
type MoveSliceForm struct {
	form    *huh.Form
	heading string

	sliceID   string
	sliceName string
	// names is each target milestone's name by ID, so the note the write reports
	// can name where the slice went.
	names map[string]string

	// chosen is the ID of the milestone picked, bound to the select.
	chosen string
}

// newMoveSliceForm returns the picker for moving a slice to one of targets,
// which must not be empty.
func newMoveSliceForm(theme huh.Theme, s domain.Slice, targets []domain.Milestone) *MoveSliceForm {
	f := &MoveSliceForm{
		heading:   "Move " + s.Name,
		sliceID:   s.ID,
		sliceName: s.Name,
		names:     make(map[string]string, len(targets)),
		chosen:    targets[0].ID,
	}
	options := make([]huh.Option[string], len(targets))
	for i, m := range targets {
		options[i] = huh.NewOption(m.Name, m.ID)
		f.names[m.ID] = m.Name
	}
	f.form = newForm(theme, huh.NewGroup(
		huh.NewSelect[string]().
			Title("Milestone").
			Description("Where the slice is filed; the work itself is untouched.").
			Options(options...).
			Value(&f.chosen),
	))
	return f
}

// Init starts the form.
func (f *MoveSliceForm) Init() tea.Cmd { return f.form.Init() }

// Update feeds a message to the form.
func (f *MoveSliceForm) Update(msg tea.Msg) tea.Cmd {
	form, cmd := f.form.Update(msg)
	f.form = form.(*huh.Form)
	return cmd
}

// State is how far the form has got.
func (f *MoveSliceForm) State() huh.FormState { return f.form.State }

// View renders the form.
func (f *MoveSliceForm) View() string { return f.form.View() }

// Heading is the title drawn over the form.
func (f *MoveSliceForm) Heading() string { return f.heading }

// SetSize gives the form the room the window leaves it.
func (f *MoveSliceForm) SetSize(width, height int) {
	f.form = f.form.WithWidth(width).WithHeight(height)
}

// save writes the milestone that was picked. A select always holds one of its
// options, so there is nothing here for a form to decline to write.
func (f *MoveSliceForm) save(a *App) tea.Cmd {
	return moveSlice(a.client, f.sliceID, f.sliceName, f.chosen, f.names[f.chosen])
}

// moveSlice refiles a slice under another milestone. Only the relation is
// written: the slice's own brief, status and repo say nothing about where in
// the plan it sits.
func moveSlice(client NotionAPI, sliceID, sliceName, milestoneID, milestoneName string) tea.Cmd {
	return func() tea.Msg {
		properties := map[string]notion.PropertyValue{
			notion.PropMilestone: notion.NewRelation(milestoneID),
		}
		if _, err := client.UpdatePageProperties(context.Background(), sliceID, properties); err != nil {
			return sliceSavedMsg{err: fmt.Errorf("move slice: %w", err)}
		}
		return sliceSavedMsg{note: fmt.Sprintf("Moved %q to %s.", sliceName, milestoneName)}
	}
}

// moveTargets are the milestones a slice could be moved to: every milestone in
// the plan but the one it is already under, in the order the board draws them.
func moveTargets(p *domain.Project, s domain.Slice) []domain.Milestone {
	if p == nil {
		return nil
	}
	var targets []domain.Milestone
	// Groups sorts the plan by Order, so taking the milestones back off it keeps
	// the picker in the same order as the board.
	for _, g := range p.Groups() {
		if g.Milestone == nil || g.Milestone.ID == s.MilestoneID {
			continue
		}
		targets = append(targets, *g.Milestone)
	}
	return targets
}

// moveSliceFlow opens the picker for the slice the cursor is on.
func (a *App) moveSliceFlow() tea.Cmd {
	if !a.canWrite() {
		return nil
	}
	s, ok := a.board.SelectedSlice()
	if !ok {
		return a.showConfirm("Move to a slice to move it.", sevWarning)
	}
	if note, refused := claimedNote(s, "moved"); refused {
		return a.showConfirm(note, sevWarning)
	}
	targets := moveTargets(a.project, s)
	if len(targets) == 0 {
		return a.showConfirm(fmt.Sprintf("There is no other milestone to move %q to.", s.Name), sevWarning)
	}
	return a.openForm(newMoveSliceForm(a.styles.FormTheme, s, targets))
}
