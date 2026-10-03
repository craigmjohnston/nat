package store

import (
	"regexp"
	"strings"
	"time"
)

// stampPrefix opens the line every task-log section is stamped with, first
// thing under its heading: `At 2026-10-03T23:14:05+01:00`. It is a paragraph
// of its own, in words, rather than anything a Notion page would have to be
// taught — so both stores write it the same way, and a reader of the page sees
// when each thing happened without the app.
const stampPrefix = "At "

// stampRe matches a stamp line's whole text, capturing the time. The time is
// RFC 3339 to the second, with its offset — the local time it was written in,
// which is what a person reading the page wants, with the offset that keeps
// it one instant wherever it is read.
var stampRe = regexp.MustCompile(`^At (\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d))$`)

// clockOr is the time a store's clock says it is: its own where a test gave it
// one, else the wall clock.
func clockOr(clock func() time.Time) time.Time {
	if clock == nil {
		return time.Now()
	}
	return clock()
}

// stampLine is the stamp a section written at t opens with.
func stampLine(t time.Time) string {
	return stampPrefix + t.Format(time.RFC3339)
}

// stamped is text with the stamp for t as its first paragraph — the one rule
// every stamped section's text is built by, in both stores.
func stamped(t time.Time, text string) string {
	if strings.TrimSpace(text) == "" {
		return stampLine(t)
	}
	return stampLine(t) + "\n\n" + text
}

// stampAt is the time a line names, where the line is a stamp and nothing
// else, and false for any other line — a section written before stamps were
// is simply one without.
func stampAt(line string) (time.Time, bool) {
	m := stampRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, m[1])
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// unstamped splits a section's trimmed text into the time its first paragraph
// stamps it with and the rest — the zero time and the whole text, untouched,
// where it carries no stamp.
func unstamped(text string) (time.Time, string) {
	first, rest, _ := strings.Cut(text, "\n\n")
	t, ok := stampAt(first)
	if !ok {
		return time.Time{}, text
	}
	return t, strings.TrimSpace(rest)
}
