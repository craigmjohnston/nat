package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// slicesSyncedMsg carries the slices edited since the board's last sync — a
// selective reload's answer — or the query that failed instead. syncedAt is
// when the query went out rather than when it landed, which is where the next
// selective run's filter counts from: an edit made while this one was in
// flight is after it, and so is not skipped.
type slicesSyncedMsg struct {
	slices   []domain.Slice
	syncedAt time.Time
	err      error
}

// startSelectiveLoad kicks off a reload of only what changed: the Slices data
// source filtered on last_edited_time since the board's own stamp, and nothing
// else — no schema read, so the milestones already in the model are reused;
// they only change on a full load or a milestone mutation. Before the first
// load has landed there is no stamp to count from, so the full load runs
// instead — a selective run is never the only thing that ever reads the
// board — and a full load already in flight is left to land.
func (a *App) startSelectiveLoad() tea.Cmd {
	if a.loading {
		return nil
	}
	if a.project == nil || a.syncedAt.IsZero() {
		return a.startLoad()
	}
	project, ok := a.activeProject()
	if !ok || a.client == nil {
		return nil
	}
	return a.fetchChangedSlices(project, a.syncedAt)
}

// fetchChangedSlices queries the slices edited since the given stamp. No sorts
// and no view order: a changed slice is merged into the place the plan already
// holds it, and only one the plan has never seen needs a place, which it takes
// at the end the way any patched-in row does — until the next full load reads
// the board's order.
func (a *App) fetchChangedSlices(cfg config.ProjectConfig, since time.Time) tea.Cmd {
	client := a.client
	return func() tea.Msg {
		now := timeNow()
		pages, err := client.QueryDataSource(context.Background(), cfg.SlicesDSID,
			notion.EditedOnOrAfter(since), nil)
		if err != nil {
			return slicesSyncedMsg{err: fmt.Errorf("sync slices: %w", err)}
		}
		return slicesSyncedMsg{slices: domain.SlicesFromPages(pages), syncedAt: now}
	}
}

// slicesSynced merges the changed slices into the plan already held rather
// than replacing it: a slice the plan holds is overwritten where it sits — a
// move across milestones travels with it, since the board's groups are cut
// from the slices' own Milestone — and one it has never seen is appended. A
// slice that vanished — trashed in Notion — is not detectable this way, since
// the query returns what still matches, not what is gone: its row lingers
// until the next full load takes it off.
func (a *App) slicesSynced(msg slicesSyncedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		a.err = msg.err
		return a, nil
	}
	if a.project == nil {
		return a, nil
	}
	if len(msg.slices) == 0 {
		// Nothing changed: the board is confirmed current as of the query, and
		// there is no reason to rebuild the plan or close a prompt under the
		// user.
		a.syncedAt = msg.syncedAt
		return a, nil
	}
	merged := make([]domain.Slice, len(a.project.Slices))
	copy(merged, a.project.Slices)
	for _, s := range msg.slices {
		if i := sliceIndex(merged, s.ID); i >= 0 {
			merged[i] = s
		} else {
			merged = append(merged, s)
		}
	}
	a.setPlanSlices(merged)
	// setPlanSlices stamps the moment it ran, but a selective sync counts from
	// when its query went out — see slicesSyncedMsg.
	a.syncedAt = msg.syncedAt
	return a, nil
}
