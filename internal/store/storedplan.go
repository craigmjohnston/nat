package store

import "context"

// StoredPlan reads a project's plan as the store's own copy stands, with no
// staleness pull: for a [*Mirrored], it hydrates the file the one time that
// has never happened at all ([Mirrored.ensureHydrated]) and then reads
// straight off it ([Local.Plan] by way of m.local), never consulting
// [Mirrored.stale] the way [Mirrored.Plan] itself does. Every other store has
// no staleness rule to skip, so it answers [Store.Plan] unchanged.
//
// This exists for callers that run once and exit — every `nat` invocation —
// where paying a fresh Notion pull merely because the file's last pull is
// older than [planStaleAfter] buys nothing: there is no board sitting on this
// process watching that copy age, only a read about to end the process that
// asked for it. [Mirrored.Plan] keeps its staleness rule unchanged for every
// other caller, the TUI's board included, which does live long enough for it
// to matter.
func StoredPlan(ctx context.Context, s Store, p Project) (Plan, error) {
	m, ok := s.(*Mirrored)
	if !ok {
		return s.Plan(ctx, p)
	}
	if err := m.ensureHydrated(ctx, true); err != nil {
		return Plan{}, err
	}
	return m.local.Plan(ctx, p)
}
