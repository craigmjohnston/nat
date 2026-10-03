package store

import (
	"context"
	"errors"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"

	"github.com/ncruces/go-sqlite3"
)

// localRetryBackoff is how long [Local.retry] waits before each further
// attempt: two of them, so three attempts in all. The busy timeout already
// waits out an ordinary writer; what is left over is the lock no timeout
// covers — a pragma run before the timeout has applied, a deferred upgrade
// SQLite refuses rather than waits on — and those clear in a moment, so a
// couple of short waits is all it takes, and anything longer is a plan really
// wedged and better said so than hidden.
var localRetryBackoff = []time.Duration{100 * time.Millisecond, 300 * time.Millisecond}

// localRetrySleep waits out one backoff, or until the caller gives up. It is a
// variable so a test can stand in for the clock rather than wait on it.
var localRetrySleep = func(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// retry runs f again, after a backoff, for as long as it fails because the plan
// file was busy and the backoffs last. Several `nat` processes open one plan at
// once — the app spawns one per command — and a lock met at the wrong instant
// is a failure that one more try a moment later simply does not have. Every
// other failure, the store's own refusals and "no such row" among them, is
// returned at once: trying again would only say it again.
//
// The final error is f's own, unwrapped, since f already says what it was doing
// and in which file. Each retry is logged, naming the file and never anything
// that was being written.
//
// Retries never nest: it wraps the outermost read or transaction that touches
// the database, never a helper one of those calls.
func (l *Local) retry(ctx context.Context, f func() error) error {
	err := f()
	for attempt, wait := range localRetryBackoff {
		if !errors.Is(err, sqlite3.BUSY) {
			return err
		}
		logging.Warn("plan busy, retrying", "path", l.path, "attempt", attempt+2, "backoff", wait)
		if err := localRetrySleep(ctx, wait); err != nil {
			return err
		}
		err = f()
	}
	return err
}
