package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// wishlistLoadedMsg carries the pending wishlist of the active project's page,
// or the read that failed instead. A failure is carried here rather than as a
// notionErrMsg because the wishlist is an indicator beside the plan, not the
// plan: a page nat cannot read the wishlist off is a board with no count on it,
// not a board replaced by an error.
type wishlistLoadedMsg struct {
	items []notion.WishlistItem
	err   error
}

// fetchWishlist reads a project page's pending wishlist items. It rides the
// load pipeline rather than being asked for separately, so the count refreshes
// wherever the plan does — the refresh key today, the background poll once
// there is one — and never on its own.
func (a *App) fetchWishlist(pageID string) tea.Cmd {
	client := a.client
	return func() tea.Msg {
		items, err := client.Wishlist(context.Background(), pageID)
		return wishlistLoadedMsg{items: items, err: err}
	}
}

// wishlistLoaded takes the count the indicator draws from a finished read. A
// failure leaves no indicator and is logged rather than shown: the board it
// sits beside has loaded, and the wishlist is not what the user asked for.
func (a *App) wishlistLoaded(msg wishlistLoadedMsg) {
	if msg.err != nil {
		logging.Error("could not read the project's wishlist", "error", msg.err)
		a.wishlist = 0
		return
	}
	a.wishlist = len(msg.items)
}

// wishlistIndicator is what the status line says about the wishlist: how many
// items are waiting on it, and the key that opens a workshop session on them.
// Nothing at all when the wishlist is empty or could not be read — the count is
// a nudge, and a zero sitting on the bar is one more thing to read past.
func (a *App) wishlistIndicator() string {
	if a.wishlist == 0 {
		return ""
	}
	items := "items"
	if a.wishlist == 1 {
		items = "item"
	}
	help := a.keys.Workshop.Help()
	return a.styles.StatusDesc.Render(fmt.Sprintf("%d wishlist %s", a.wishlist, items)) +
		a.styles.HintSep.Render(" · ") +
		a.styles.StatusKey.Render(help.Key) + " " + a.styles.StatusDesc.Render(help.Desc)
}

// withWishlist puts the indicator beside what the status line already says,
// within the room the two have between them. The indicator goes last, and goes
// entirely when there is no room for it: an error or a note is about what the
// user just did, and outranks a standing count.
func withWishlist(content, indicator string, width int) string {
	if indicator == "" {
		return content
	}
	if width > 0 && lipgloss.Width(content)+1+lipgloss.Width(indicator) > width {
		return content
	}
	return content + " " + indicator
}
