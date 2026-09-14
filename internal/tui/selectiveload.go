package tui

import tea "charm.land/bubbletea/v2"

// startSelectiveLoad is now exactly a full load. The edited-since query it
// used to run had no meaning once the board started reading a plan file
// rather than the workspace directly: the file already has the plan, at
// whatever staleness [store.Mirrored.Plan] itself judges — a caller here has
// no separate "since" to ask about — and a full read of it is cheaper than
// the round trip the selective query was written to save.
func (a *App) startSelectiveLoad() tea.Cmd {
	return a.startLoad(false)
}
