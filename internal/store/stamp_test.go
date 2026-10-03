package store

import (
	"strings"
	"testing"
	"time"
)

// testNow is the time [fixedClock] says it is: an hour east of UTC, so a stamp
// that dropped its offset or wrote UTC would show.
var testNow = time.Date(2026, 10, 3, 23, 14, 5, 0, time.FixedZone("BST", 3600))

// readNow is [testNow] as a stamp reads back: the same instant, in whichever
// location time.Parse gives an offset, which is what a DeepEqual of a parsed
// event has to be held against.
var readNow, _ = time.Parse(time.RFC3339, "2026-10-03T23:14:05+01:00")

// fixedClock is the clock every store a test writes through is given.
func fixedClock() time.Time { return testNow }

// testStamp is the stamp line a store on [fixedClock] writes.
const testStamp = "At 2026-10-03T23:14:05+01:00"

// stampBlockJSON is [testStamp] as the paragraph block a Notion store appends.
const stampBlockJSON = `{"object":"block","paragraph":{"rich_text":[{"text":{"content":"` + testStamp + `"},"type":"text"}]},"type":"paragraph"}`

// clocked is a Notion store over api on [fixedClock].
func clocked(api API) *Notion {
	n := Over(api)
	n.Clock = fixedClock
	return n
}

func TestStampLineIsRFC3339WithItsOffset(t *testing.T) {
	if got := stampLine(testNow); got != testStamp {
		t.Errorf("stampLine = %q, want %q", got, testStamp)
	}
}

// A store with no clock of its own reads the wall clock.
func TestClockOrFallsBackToTheWallClock(t *testing.T) {
	before := time.Now()
	got := clockOr(nil)
	if got.Before(before) || time.Since(got) > time.Minute {
		t.Errorf("clockOr(nil) = %v, want about now", got)
	}
	if got := clockOr(fixedClock); !got.Equal(testNow) {
		t.Errorf("clockOr(fixedClock) = %v, want %v", got, testNow)
	}
}

func TestStampedPutsTheStampFirst(t *testing.T) {
	if got, want := stamped(testNow, "Did it."), testStamp+"\n\nDid it."; got != want {
		t.Errorf("stamped = %q, want %q", got, want)
	}
	// Nothing to say is the stamp alone, never a dangling blank paragraph.
	if got := stamped(testNow, "  \n"); got != testStamp {
		t.Errorf("stamped(empty) = %q, want %q", got, testStamp)
	}
}

func TestUnstampedReadsTheStampOffTheFront(t *testing.T) {
	at, rest := unstamped(testStamp + "\n\nDid it.\n\nTwice.")
	if !at.Equal(testNow) || rest != "Did it.\n\nTwice." {
		t.Errorf("unstamped = %v, %q; want %v and the text after the stamp", at, rest, testNow)
	}
	// The stamp alone is a section with nothing more to say.
	if at, rest := unstamped(testStamp); !at.Equal(testNow) || rest != "" {
		t.Errorf("unstamped(stamp alone) = %v, %q; want %v and nothing", at, rest, testNow)
	}
	// UTC and fractional seconds both read, as RFC 3339 allows.
	if at, _ := unstamped("At 2026-10-03T22:14:05.5Z"); !at.Equal(testNow.Add(500 * time.Millisecond)) {
		t.Errorf("unstamped(UTC, fractional) = %v, want half a second past %v", at, testNow)
	}
}

// Text with no stamp — written before sections were stamped, or by hand —
// reads exactly as it is, at the zero time; so does a line that looks like a
// stamp but names no time there is, or carries more than the time.
func TestUnstampedLeavesAnythingElseAlone(t *testing.T) {
	for _, text := range []string{
		"Did it.",
		"",
		"At home.\n\nDid it.",
		"At 2026-13-03T23:14:05+01:00\n\nDid it.",
		"At 2026-10-03T23:14:05+01:00 or so\n\nDid it.",
		"At 2026-10-03T23:14:05\n\nDid it.",
	} {
		at, rest := unstamped(text)
		if !at.IsZero() || rest != text {
			t.Errorf("unstamped(%q) = %v, %q; want the zero time and the text untouched", text, at, rest)
		}
	}
}

func TestStampAtTrimsTheLine(t *testing.T) {
	if at, ok := stampAt("  " + testStamp + " "); !ok || !at.Equal(testNow) {
		t.Errorf("stampAt = %v, %v; want %v", at, ok, testNow)
	}
	if _, ok := stampAt(strings.TrimPrefix(testStamp, "At ")); ok {
		t.Error("stampAt(bare time) = ok, want only a whole stamp line read")
	}
}
