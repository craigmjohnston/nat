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
	PRDescription(ctx context.Context, id string) (string, error)
	ClaimSlice(ctx context.Context, id string, sh store.Shape, userID string) (domain.Slice, error)
	RecordPR(ctx context.Context, id, url string) error
	MarkDone(ctx context.Context, id string, sh store.Shape) error
}
