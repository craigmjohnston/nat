package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/domain"
)

// sliceRefreshedMsg carries one slice refetched after a mutation the board
// itself made, or the fetch that failed instead.
type sliceRefreshedMsg struct {
	slice domain.Slice
	err   error
}

// refreshSlice refetches the one page a finished write touched, so the board
// can patch its row rather than reload the whole plan.
func (a *App) refreshSlice(pageID string) tea.Cmd {
	client := a.client
	return func() tea.Msg {
		page, err := client.GetPage(context.Background(), pageID)
		if err != nil {
			return sliceRefreshedMsg{err: fmt.Errorf("refresh slice: %w", err)}
		}
		return sliceRefreshedMsg{slice: domain.SliceFromPage(*page)}
	}
}

// sliceRefreshed patches the refetched slice into the plan. The board is as
// current about this page as a full reload would have made it, so the freshness
// stamp moves with it; the rest of the plan is trusted to be what it was.
func (a *App) sliceRefreshed(msg sliceRefreshedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		a.err = msg.err
		return a, nil
	}
	if a.project == nil {
		return a, nil
	}
	slices := make([]domain.Slice, len(a.project.Slices))
	copy(slices, a.project.Slices)
	i := sliceIndex(slices, msg.slice.ID)
	if i < 0 {
		// A slice the plan does not hold yet — one just created — trails its
		// milestone's group, which is where InViewOrder puts a slice the view's
		// order was read before.
		slices = append(slices, msg.slice)
	} else {
		slices[i] = msg.slice
	}
	a.setPlanSlices(slices)
	return a, nil
}

// removeSlice takes a deleted slice's row off the plan. The page is in the
// trash — there is nothing to refetch — so the plan is patched on the spot.
func (a *App) removeSlice(id string) {
	if a.project == nil {
		return
	}
	i := sliceIndex(a.project.Slices, id)
	if i < 0 {
		return
	}
	slices := make([]domain.Slice, 0, len(a.project.Slices)-1)
	slices = append(slices, a.project.Slices[:i]...)
	slices = append(slices, a.project.Slices[i+1:]...)
	a.setPlanSlices(slices)
}

// setPlanSlices rebuilds the plan around a patched slice list — the milestone
// statuses are computed from the slices under them, so they are re-derived the
// way a full load would — and puts it on the board. The patch stands in for the
// reload the mutation used to kick off, so the freshness stamp moves and an
// open prompt closes, exactly as a landed load would have them.
func (a *App) setPlanSlices(slices []domain.Slice) {
	p := domain.NewProject(a.project.ID, a.project.Name, a.project.Milestones, slices)
	a.project = &p
	a.syncedAt = timeNow()
	// A prompt is a question about a row that may just have moved or gone.
	a.closeBoardPrompt()
	a.board.SetProject(a.project)
	a.syncBoard()
}

// sliceIndex is where the plan holds the named slice, or -1 when it does not.
func sliceIndex(slices []domain.Slice, id string) int {
	for i, s := range slices {
		if s.ID == id {
			return i
		}
	}
	return -1
}
