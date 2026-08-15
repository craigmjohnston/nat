package tui

import (
	"fmt"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
)

// timeNow is the clock the freshness indicator reads, held as a variable so the
// tests can stand a fixed one in.
var timeNow = time.Now

// freshnessIndicator is what the status line says about how current the board
// is: the spinner while a fetch is in flight, and otherwise how long ago the
// plan on screen came back. Nothing at all until the first load lands — there
// is no plan on show yet for a timestamp to be about, and the board says so
// itself.
//
// The age is only as current as the last redraw, and a board nobody is touching
// is redrawn by the live-session tick every liveInterval — near enough for a
// reading in whole minutes.
//
// The spinner is restyled rather than drawn as it comes: every segment of the
// status line carries the bar's own background, and the spinner's style is a
// foreground alone, which on the bar would cut a hole in the fill.
func (a *App) freshnessIndicator() string {
	if a.loading {
		return a.styles.StatusKey.Render(xansi.Strip(a.spinner.View())) +
			a.styles.StatusDesc.Render(" syncing…")
	}
	if a.syncedAt.IsZero() {
		return ""
	}
	return a.styles.StatusDesc.Render("synced " + ago(timeNow().Sub(a.syncedAt)))
}

// ago is a gap in time in the words the indicator uses: coarse on purpose,
// since what it answers is "is this still what Notion says", not how many
// seconds old it is. A clock that has gone backwards reads as just now.
func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}
