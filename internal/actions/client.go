package actions

import (
	"context"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/store"
)

// Store is what a launch or an approve does to a plan: read the slice as it
// stands, take it, read the pull request description its hand-back filed,
// record the pull request that came of it, and mark it Done when that lands.
// It is narrower than [store.Store] — the whole port — because that is
// everything else the board and the commands do to a plan, none of which
// either flow touches.
type Store interface {
	Slice(ctx context.Context, id string) (domain.Slice, store.Shape, error)
	// Body reads the prose kept on a page — a slice's brief, a project's
	// conventions — as markdown, which a launch writes into the agent's
	// opening prompt.
	Body(ctx context.Context, id string) (string, error)
	PRDescription(ctx context.Context, id string) (string, error)
	ClaimSlice(ctx context.Context, id string, sh store.Shape, userID string) (domain.Slice, error)
	RecordPR(ctx context.Context, id, url string) error
	MarkDone(ctx context.Context, id string, sh store.Shape) error
	ReopenSlice(ctx context.Context, id string, sh store.Shape) error
	// RecordRelaunch files a relaunch's one fixed line in the task log, which
	// [Launch] writes for a non-fix launch picking a session back up — never
	// a fresh one. Its own failure is logged and never fails the launch.
	RecordRelaunch(ctx context.Context, id string) error
	// RecordLaunch files a fresh launch's one fixed line in the task log, which
	// [Launch] writes for every non-fix launch that is not a relaunch. Its own
	// failure is logged and never fails the launch either.
	RecordLaunch(ctx context.Context, id string) error
}
