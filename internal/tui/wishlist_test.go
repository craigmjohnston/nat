package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/notion"
)

// wishlistItems is a wishlist of n items, as the client reads them off the
// project page.
func wishlistItems(n int) []notion.WishlistItem {
	items := make([]notion.WishlistItem, n)
	for i := range items {
		items[i] = notion.WishlistItem{ID: string(rune('a'+i)) + "-block", Markdown: "- something"}
	}
	return items
}

// loadedApp is the app after a full load: the plan, and whatever the wishlist
// read answered with. The window is sized so the bar has a width to lay out to.
func loadedApp(t *testing.T, client *loadingClient) *App {
	t.Helper()
	a := NewApp(testConfig(), client)
	for _, msg := range run(a.Init()) {
		a.Update(msg)
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return a
}

// wishlistClient answers the load pipeline's queries and hands back items for
// the wishlist read, or err instead when it is set.
func wishlistClient(items []notion.WishlistItem, err error) *loadingClient {
	c := newLoadingClient()
	c.wishlist = func(string) ([]notion.WishlistItem, error) { return items, err }
	return c
}

func TestAppReadsTheWishlistOfTheActiveProjectPage(t *testing.T) {
	client := wishlistClient(wishlistItems(3), nil)
	a := loadedApp(t, client)

	if got := client.wishlistPages; len(got) != 1 || got[0] != testProjectID {
		t.Errorf("wishlist read for %v, want the active project page %q", got, testProjectID)
	}
	if a.wishlist != 3 {
		t.Errorf("wishlist = %d, want the 3 items read", a.wishlist)
	}
}

func TestAppRefreshRereadsTheWishlist(t *testing.T) {
	client := wishlistClient(wishlistItems(1), nil)
	a := loadedApp(t, client)

	for _, msg := range run(press(a, "r")) {
		a.Update(msg)
	}
	if got := len(client.wishlistPages); got != 2 {
		t.Errorf("wishlist read %d times, want the refresh to have re-read it", got)
	}
	if a.wishlist != 1 {
		t.Errorf("wishlist = %d, want the one item still pending", a.wishlist)
	}
}

func TestBarShowsTheWishlistCount(t *testing.T) {
	golden(t, "bar-wishlist", loadedApp(t, wishlistClient(wishlistItems(3), nil)).View().WindowTitle)
}

func TestBarWithoutPendingWishlistItemsSaysNothingAboutIt(t *testing.T) {
	golden(t, "bar-no-wishlist", loadedApp(t, wishlistClient(nil, nil)).View().WindowTitle)
}

func TestBarCountsOneWishlistItemInTheSingular(t *testing.T) {
	a := loadedApp(t, wishlistClient(wishlistItems(1), nil))

	if got := a.View().WindowTitle; !strings.Contains(got, "1 wishlist item ") {
		t.Errorf("bar = %q, want one item named in the singular", got)
	}
}

func TestAFailedWishlistReadLeavesTheBoardRendered(t *testing.T) {
	client := wishlistClient(nil, errors.New("boom"))
	a := loadedApp(t, client)

	if a.err != nil {
		t.Errorf("err = %v, want a wishlist failure kept off the screen", a.err)
	}
	if a.wishlist != 0 {
		t.Errorf("wishlist = %d, want no count from a failed read", a.wishlist)
	}
	if got := a.View().WindowTitle; strings.Contains(got, "wishlist") || strings.Contains(got, "boom") {
		t.Errorf("bar = %q, want no indicator and no error on it", got)
	}
	// The plan the failure rode alongside is on the board all the same.
	view := stripANSI(a.View().Content)
	for _, want := range []string{"tracker", "1/2"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q, want the plan still drawn:\n%s", want, view)
		}
	}
}

func TestTheWishlistIndicatorGivesWayToTheStatusMessage(t *testing.T) {
	a := loadedApp(t, wishlistClient(wishlistItems(3), nil))
	a.Update(notionErrMsg{err: errors.New(strings.Repeat("wide ", 12))})

	if got := a.View().WindowTitle; strings.Contains(got, "wishlist") {
		t.Errorf("bar = %q, want the indicator dropped for want of room", got)
	}
	// It is back as soon as the error is dismissed and the room with it.
	press(a, "esc")
	if got := a.View().WindowTitle; !strings.Contains(got, "3 wishlist items") {
		t.Errorf("bar = %q, want the indicator back", got)
	}
}

func TestTheWishlistIndicatorNamesTheWorkshopKey(t *testing.T) {
	a := loadedApp(t, wishlistClient(wishlistItems(2), nil))

	help := a.keys.Workshop.Help()
	if got := a.View().WindowTitle; !strings.Contains(got, help.Key+" "+help.Desc) {
		t.Errorf("bar = %q, want the key that opens a workshop on the wishlist", got)
	}
}

func TestSwitchingProjectDropsTheWishlistCount(t *testing.T) {
	a := loadedApp(t, wishlistClient(wishlistItems(3), nil))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	a.Update(projectSwitchedMsg{id: "other", name: "other"})

	if a.wishlist != 0 {
		t.Errorf("wishlist = %d, want the last project's count dropped", a.wishlist)
	}
}
