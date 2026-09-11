package actions

import (
	"context"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// Client is what a launch or an approve needs of Notion beyond the plan
// itself: the blocks under a slice page, to read the pull request description
// an agent filed at hand-back. A page body is not plan work — it is prose
// somebody wrote — which is why it is still read through the client while
// everything these flows do to the plan goes through [Store].
type Client interface {
	GetBlockChildren(ctx context.Context, id string) ([]notion.Block, error)
}

// Store is what a launch or an approve does to the plan: read the slice as it
// stands, take it, record the pull request that came of it, and mark it Done
// when that lands. It is narrower than [store.Store] — the whole port —
// because that is everything else the board and the commands do to a plan,
// none of which either flow touches.
type Store interface {
	Slice(ctx context.Context, id string) (domain.Slice, store.Shape, error)
	ClaimSlice(ctx context.Context, id string, sh store.Shape, userID string) (domain.Slice, error)
	RecordPR(ctx context.Context, id, url string) error
	MarkDone(ctx context.Context, id string, sh store.Shape) error
}
