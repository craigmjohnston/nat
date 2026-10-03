package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3"

	"github.com/craigmjohnston/nat/internal/logging"
)

// holdPlan opens a second, raw connection to the plan file and takes its lock
// away from everyone else: WAL with exclusive locking mode, so once it has
// written, no other connection can so much as read until it lets go. That is
// the instant every sibling `nat` process meets when another is mid-commit or
// checkpointing the WAL away on its way out. The returned func lets go.
func holdPlan(t *testing.T, path string) func() {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path+"?_pragma=journal_mode(wal)&_pragma=locking_mode(exclusive)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS held (x); INSERT INTO held VALUES (1)`); err != nil {
		t.Fatalf("take the lock: %v", err)
	}
	var once sync.Once
	release := func() { once.Do(func() { _ = db.Close() }) }
	t.Cleanup(release)
	return release
}

// A plan another process holds the lock on at the instant this one opens it
// waits that process out rather than failing: the open-time pragmas run under
// the busy timeout, because the timeout is the first of them. With the timeout
// after journal_mode(wal), the open failed at once with "invalid _pragma:
// database is locked" — the error the app's log was full of.
func TestOpenLocalWaitsOutAPlanAnotherProcessHolds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.db")
	release := holdPlan(t, path)
	go func() {
		time.Sleep(200 * time.Millisecond)
		release()
	}()

	l, err := OpenLocal(path)
	if err != nil {
		t.Fatalf("open a plan another process briefly holds: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
}

// stubRetrySleep stands in for the backoff's clock for one test, recording each
// wait it was asked for and running whatever the test wants done in it.
func stubRetrySleep(t *testing.T, during func(ctx context.Context, n int) error) *[]time.Duration {
	t.Helper()
	var waits []time.Duration
	was := localRetrySleep
	localRetrySleep = func(ctx context.Context, d time.Duration) error {
		waits = append(waits, d)
		if during == nil {
			return nil
		}
		return during(ctx, len(waits))
	}
	t.Cleanup(func() { localRetrySleep = was })
	return &waits
}

// logTo points the log at a file of the test's own and returns a func reading
// back what was written to it.
func logTo(t *testing.T) func() string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", "")
	path, err := logging.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logging.Close() })
	return func() string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
}

func TestRetry(t *testing.T) {
	busy := fmt.Errorf("read the plan at /plans/p.db: %w", sqlite3.BUSY)
	other := errors.New("not a lock at all")
	l := &Local{path: "/plans/p.db"}

	// calls runs f, failing with each of errs in turn and succeeding after.
	calls := func(errs ...error) (func() error, *int) {
		n := 0
		return func() error {
			n++
			if n <= len(errs) {
				return errs[n-1]
			}
			return nil
		}, &n
	}

	t.Run("succeeds first time", func(t *testing.T) {
		waits := stubRetrySleep(t, nil)
		f, n := calls()
		if err := l.retry(context.Background(), f); err != nil || *n != 1 || len(*waits) != 0 {
			t.Errorf("err = %v, calls = %d, waits = %v; want one call and no wait", err, *n, *waits)
		}
	})

	t.Run("busy then through", func(t *testing.T) {
		read := logTo(t)
		waits := stubRetrySleep(t, nil)
		f, n := calls(busy)
		if err := l.retry(context.Background(), f); err != nil || *n != 2 {
			t.Errorf("err = %v, calls = %d; want through on the second", err, *n)
		}
		if !reflect.DeepEqual(*waits, localRetryBackoff[:1]) {
			t.Errorf("waits = %v, want %v", *waits, localRetryBackoff[:1])
		}
		_ = logging.Close()
		got := read()
		for _, want := range []string{"level=WARN", "plan busy, retrying", "path=/plans/p.db", "attempt=2", "backoff=100ms"} {
			if !strings.Contains(got, want) {
				t.Errorf("log = %q, want %q", got, want)
			}
		}
	})

	t.Run("busy throughout", func(t *testing.T) {
		waits := stubRetrySleep(t, nil)
		f, n := calls(busy, busy, busy, busy)
		if err := l.retry(context.Background(), f); err != busy || *n != 3 { //nolint:errorlint // the very error f gave
			t.Errorf("err = %v, calls = %d; want f's own last error after three calls", err, *n)
		}
		if !reflect.DeepEqual(*waits, localRetryBackoff) {
			t.Errorf("waits = %v, want %v", *waits, localRetryBackoff)
		}
	})

	t.Run("not busy", func(t *testing.T) {
		for _, e := range []error{other, sql.ErrNoRows} {
			waits := stubRetrySleep(t, nil)
			f, n := calls(e)
			if err := l.retry(context.Background(), f); err != e || *n != 1 || len(*waits) != 0 { //nolint:errorlint // the very error f gave
				t.Errorf("err = %v, calls = %d, waits = %v; want %v at once", err, *n, *waits, e)
			}
		}
	})

	t.Run("given up on while waiting", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		stubRetrySleep(t, func(ctx context.Context, _ int) error { cancel(); return ctx.Err() })
		f, n := calls(busy)
		if err := l.retry(ctx, f); !errors.Is(err, context.Canceled) || *n != 1 {
			t.Errorf("err = %v, calls = %d; want the cancellation and no second call", err, *n)
		}
	})
}

// The real clock waits out its backoff, and stops waiting the moment the
// caller gives up.
func TestLocalRetrySleep(t *testing.T) {
	if err := localRetrySleep(context.Background(), time.Millisecond); err != nil {
		t.Errorf("a short wait: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := localRetrySleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled wait = %v, want context.Canceled", err)
	}
}

// A write that meets another process's write lock with no busy timeout to wait
// it out — the case the timeout cannot cover — is tried again once the lock is
// gone, and lands.
func TestLocalWriteRetriesAPlanAnotherProcessIsWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.db")
	noWait := "file:" + path + "?_pragma=busy_timeout(0)&_pragma=journal_mode(wal)&_pragma=foreign_keys(on)&_txlock=immediate"
	l, err := openLocalDSN(path, noWait)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })

	ctx := context.Background()
	holder, err := sql.Open("sqlite3", noWait)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = holder.Close() })
	conn, err := holder.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatalf("take the write lock: %v", err)
	}
	waits := stubRetrySleep(t, func(ctx context.Context, _ int) error {
		_, err := conn.ExecContext(ctx, `ROLLBACK`)
		return err
	})

	s, err := l.AddSlice(ctx, Project{}, NewSlice{Title: "Waited its turn"})
	if err != nil {
		t.Fatalf("AddSlice once the lock is gone: %v", err)
	}
	if len(*waits) != 1 {
		t.Errorf("waits = %v, want one retry", *waits)
	}
	if got := readBack(t, l, s.ID); got.Name != "Waited its turn" {
		t.Errorf("read back %+v, want the slice written", got)
	}
}

// Opening goes through the same retry: a plan locked at the instant it is
// opened, with no busy timeout, opens once the lock is gone.
func TestOpenLocalRetriesAPlanLockedWithNoTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.db")
	release := holdPlan(t, path)
	waits := stubRetrySleep(t, func(context.Context, int) error { release(); return nil })

	l, err := openLocalDSN(path, "file:"+path+"?_pragma=busy_timeout(0)&_pragma=journal_mode(wal)")
	if err != nil {
		t.Fatalf("open once the lock is gone: %v", err)
	}
	_ = l.Close()
	if len(*waits) != 1 {
		t.Errorf("waits = %v, want one retry", *waits)
	}
}
