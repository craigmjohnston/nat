package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

// pageBlocks decodes a page body from the JSON the blocks endpoint returns: a
// block's payload is only reachable through a decode, so the fixtures here are
// JSON rather than struct literals.
func pageBlocks(t *testing.T, raw string) []notion.Block {
	t.Helper()
	var blocks []notion.Block
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		t.Fatal(err)
	}
	return blocks
}

// wishlistPage is a project page with a wishlist of two items, an aside between
// them, and a bullet either side of the section that has nothing to do with it.
func wishlistPage(t *testing.T) []notion.Block {
	t.Helper()
	return pageBlocks(t, `[
		{"id":"conventions","type":"heading_2","heading_2":{"rich_text":[{"plain_text":"Conventions"}]}},
		{"id":"outside","type":"bulleted_list_item","bulleted_list_item":{"rich_text":[{"plain_text":"Branch per slice."}]}},
		{"id":"wishlist","type":"heading_2","heading_2":{"rich_text":[{"plain_text":"Wishlist"}]}},
		{"id":"w1","type":"bulleted_list_item","bulleted_list_item":{"rich_text":[{"plain_text":"Local file storage"}]}},
		{"id":"aside","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"an aside"}]}},
		{"id":"w2","type":"bulleted_list_item","bulleted_list_item":{"rich_text":[{"plain_text":"A newline in the status bar"}]}},
		{"id":"notes","type":"heading_2","heading_2":{"rich_text":[{"plain_text":"Notes"}]}},
		{"id":"after","type":"bulleted_list_item","bulleted_list_item":{"rich_text":[{"plain_text":"past the wishlist"}]}}
	]`)
}

// emptyWishlistPage is the page as a cleared wishlist leaves it: the heading,
// and one blank bullet waiting for the next idea.
func emptyWishlistPage(t *testing.T) []notion.Block {
	t.Helper()
	return pageBlocks(t, `[
		{"id":"wishlist","type":"heading_2","heading_2":{"rich_text":[{"plain_text":"Wishlist"}]}},
		{"id":"blank","type":"bulleted_list_item","bulleted_list_item":{"rich_text":[]}}
	]`)
}

func TestWishlistPrintsTheItemsAsMarkdown(t *testing.T) {
	env, out := testEnv(testConfig(), &fakeAPI{blocks: wishlistPage(t)})

	if err := Run(context.Background(), []string{"wishlist"}, env); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	want := "# Wishlist\n\n- Local file storage\n- A newline in the status bar\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestWishlistSaysWhenThereIsNothingOnIt(t *testing.T) {
	env, out := testEnv(testConfig(), &fakeAPI{blocks: emptyWishlistPage(t)})

	if err := Run(context.Background(), []string{"wishlist"}, env); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	want := "# Wishlist\n\n_none_\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestWishlistJSONNamesTheBlockOfEveryItem(t *testing.T) {
	env, out := testEnv(testConfig(), &fakeAPI{blocks: wishlistPage(t)})

	if err := Run(context.Background(), []string{"wishlist", "--json"}, env); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	var got wishlistJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	want := wishlistJSON{Items: []wishlistItemJSON{
		{ID: "w1", Markdown: "- Local file storage"},
		{ID: "w2", Markdown: "- A newline in the status bar"},
	}}
	if len(got.Items) != len(want.Items) {
		t.Fatalf("items = %+v, want %+v", got.Items, want.Items)
	}
	for i, item := range got.Items {
		if item != want.Items[i] {
			t.Errorf("item %d = %+v, want %+v", i, item, want.Items[i])
		}
	}
}

// An empty wishlist is an empty array, not null: whatever reads this should be
// able to range over it without checking first.
func TestWishlistJSONOfAnEmptyWishlistIsAnEmptyArray(t *testing.T) {
	env, out := testEnv(testConfig(), &fakeAPI{blocks: emptyWishlistPage(t)})

	if err := Run(context.Background(), []string{"wishlist", "--json"}, env); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	if !strings.Contains(out.String(), `"items": []`) {
		t.Errorf("output = %s, want an empty items array", out.String())
	}
}

func TestWishlistReportsAFailedPageRead(t *testing.T) {
	boom := errors.New("notion is down")
	env, _ := testEnv(testConfig(), &fakeAPI{blocksErr: boom})

	err := Run(context.Background(), []string{"wishlist"}, env)

	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func TestWishlistReportsMissingConfiguration(t *testing.T) {
	env, _ := testEnv(config.Config{}, &fakeAPI{})
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil }

	err := Run(context.Background(), []string{"wishlist"}, env)

	if err == nil || !strings.Contains(err.Error(), "no configuration yet") {
		t.Errorf("err = %v, want it to name the setup that has not happened", err)
	}
}

func TestWishlistRejectsAnUnknownFlag(t *testing.T) {
	env, out := testEnv(testConfig(), &fakeAPI{blocks: wishlistPage(t)})

	err := Run(context.Background(), []string{"wishlist", "--all"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want nothing", out.String())
	}
}

func TestWishlistReportsAFailedWrite(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{blocks: wishlistPage(t)})
	env.Out = failingWriter{}

	err := Run(context.Background(), []string{"wishlist"}, env)

	if !errors.Is(err, errWrite) {
		t.Errorf("err = %v, want %v", err, errWrite)
	}
}

func TestWishlistClearTrashesTheNamedItemsAndSeedsAnEmptyOne(t *testing.T) {
	api := &fakeAPI{blocks: wishlistPage(t)}
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"wishlist-clear", "w1", "w2"}, env); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	if want := []string{"w1", "w2"}; !equalStrings(api.deletes, want) {
		t.Errorf("deletes = %v, want %v", api.deletes, want)
	}
	if len(api.appends) != 1 {
		t.Fatalf("appends = %+v, want one", api.appends)
	}
	got := api.appends[0]
	// The aside is the last block still standing in the section, so the fresh
	// bullet belongs under it — not at the end of the page, past the heading
	// that closes the section.
	if got.id != "project-1" || got.after != "aside" {
		t.Errorf("append = page %q after %q, want page %q after %q", got.id, got.after, "project-1", "aside")
	}
	if len(got.children) != 1 || got.children[0]["type"] != "bulleted_list_item" {
		t.Errorf("appended = %+v, want one empty bullet", got.children)
	}
	for _, want := range []string{"2 items trashed", "empty bullet", "- trashed: w1", "- trashed: w2"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output = %q, want it to mention %q", out.String(), want)
		}
	}
}

// The blank bullet is the state a clear is trying to reach, so a section that
// still has one is left exactly as it is.
func TestWishlistClearLeavesAnExistingEmptyItemAlone(t *testing.T) {
	api := &fakeAPI{blocks: pageBlocks(t, `[
		{"id":"wishlist","type":"heading_2","heading_2":{"rich_text":[{"plain_text":"Wishlist"}]}},
		{"id":"w1","type":"bulleted_list_item","bulleted_list_item":{"rich_text":[{"plain_text":"Local file storage"}]}},
		{"id":"blank","type":"bulleted_list_item","bulleted_list_item":{"rich_text":[]}}
	]`)}
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"wishlist-clear", "w1"}, env); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	if want := []string{"w1"}; !equalStrings(api.deletes, want) {
		t.Errorf("deletes = %v, want %v", api.deletes, want)
	}
	if len(api.appends) != 0 {
		t.Errorf("appends = %+v, want none: the section already has an empty bullet", api.appends)
	}
	if strings.Contains(out.String(), "empty bullet") {
		t.Errorf("output = %q, want nothing said about seeding one", out.String())
	}
}

// A wishlist cleared out entirely has nothing left to hang the fresh bullet
// off but the heading itself.
func TestWishlistClearSeedsUnderTheHeadingWhenNothingIsLeft(t *testing.T) {
	api := &fakeAPI{blocks: pageBlocks(t, `[
		{"id":"wishlist","type":"heading_2","heading_2":{"rich_text":[{"plain_text":"Wishlist"}]}},
		{"id":"w1","type":"bulleted_list_item","bulleted_list_item":{"rich_text":[{"plain_text":"Local file storage"}]}}
	]`)}
	env, _ := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"wishlist-clear", "w1"}, env); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	if len(api.appends) != 1 || api.appends[0].after != "wishlist" {
		t.Errorf("appends = %+v, want one after the heading", api.appends)
	}
}

// The point of clearing by ID: a block that is not a wishlist item is reported
// and left where it is, whatever it is and wherever on the page it lives.
func TestWishlistClearRefusesABlockThatIsNotAWishlistItem(t *testing.T) {
	for _, id := range []string{"outside", "after", "aside", "conventions", "never-seen"} {
		t.Run(id, func(t *testing.T) {
			api := &fakeAPI{blocks: wishlistPage(t)}
			env, out := testEnv(testConfig(), api)

			if err := Run(context.Background(), []string{"wishlist-clear", id}, env); err != nil {
				t.Fatalf("Run() = %v", err)
			}

			if len(api.deletes) != 0 {
				t.Errorf("deletes = %v, want none", api.deletes)
			}
			if len(api.appends) != 0 {
				t.Errorf("appends = %+v, want none: nothing was cleared", api.appends)
			}
			if want := "- not a wishlist item: " + id; !strings.Contains(out.String(), want) {
				t.Errorf("output = %q, want it to say %q", out.String(), want)
			}
			if !strings.Contains(out.String(), "0 items trashed, 1 left alone") {
				t.Errorf("output = %q, want it to count what happened", out.String())
			}
		})
	}
}

// One good ID among bad ones still lands: the bad ones are reported, not fatal.
func TestWishlistClearTrashesWhatItCanAndReportsTheRest(t *testing.T) {
	api := &fakeAPI{blocks: wishlistPage(t)}
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"wishlist-clear", "gone-already", "w1"}, env); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	if want := []string{"w1"}; !equalStrings(api.deletes, want) {
		t.Errorf("deletes = %v, want %v", api.deletes, want)
	}
	if !strings.Contains(out.String(), "1 item trashed, 1 left alone") {
		t.Errorf("output = %q, want it to count what happened", out.String())
	}
}

func TestWishlistClearOnAPageWithNoWishlistDeletesNothing(t *testing.T) {
	api := &fakeAPI{blocks: pageBlocks(t, `[
		{"id":"conventions","type":"heading_2","heading_2":{"rich_text":[{"plain_text":"Conventions"}]}},
		{"id":"outside","type":"bulleted_list_item","bulleted_list_item":{"rich_text":[{"plain_text":"Branch per slice."}]}}
	]`)}
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"wishlist-clear", "outside"}, env); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	if len(api.deletes) != 0 || len(api.appends) != 0 {
		t.Errorf("deletes = %v, appends = %+v, want neither", api.deletes, api.appends)
	}
	if !strings.Contains(out.String(), "- not a wishlist item: outside") {
		t.Errorf("output = %q, want the block reported", out.String())
	}
}

// An ID copied out of a URL is undashed, and Notion's own are lowercase; both
// name the same block.
func TestWishlistClearMatchesIDsHoweverTheyAreWritten(t *testing.T) {
	api := &fakeAPI{blocks: pageBlocks(t, `[
		{"id":"wishlist","type":"heading_2","heading_2":{"rich_text":[{"plain_text":"Wishlist"}]}},
		{"id":"3bd38308-f654-8142-9534-d3d80043f35a","type":"bulleted_list_item","bulleted_list_item":{"rich_text":[{"plain_text":"Local file storage"}]}},
		{"id":"blank","type":"bulleted_list_item","bulleted_list_item":{"rich_text":[]}}
	]`)}
	env, _ := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"wishlist-clear", "3BD38308F65481429534D3D80043F35A"}, env); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	if want := []string{"3BD38308F65481429534D3D80043F35A"}; !equalStrings(api.deletes, want) {
		t.Errorf("deletes = %v, want %v", api.deletes, want)
	}
}

// The same item named twice is one delete: the second would be a delete of a
// block already gone, which Notion refuses.
func TestWishlistClearIgnoresARepeatedID(t *testing.T) {
	api := &fakeAPI{blocks: wishlistPage(t)}
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"wishlist-clear", "w1", "w1"}, env); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	if want := []string{"w1"}; !equalStrings(api.deletes, want) {
		t.Errorf("deletes = %v, want %v", api.deletes, want)
	}
	if !strings.Contains(out.String(), "1 item trashed.") {
		t.Errorf("output = %q, want one item counted", out.String())
	}
}

func TestWishlistClearWantsSomethingToClear(t *testing.T) {
	for _, args := range [][]string{{"wishlist-clear"}, {"wishlist-clear", "--all"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			api := &fakeAPI{blocks: wishlistPage(t)}
			env, out := testEnv(testConfig(), api)

			err := Run(context.Background(), args, env)

			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("err = %v (%T), want a *UsageError", err, err)
			}
			if len(api.deletes) != 0 {
				t.Errorf("deletes = %v, want none", api.deletes)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

func TestWishlistClearReportsMissingConfiguration(t *testing.T) {
	env, _ := testEnv(config.Config{}, &fakeAPI{})
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil }

	err := Run(context.Background(), []string{"wishlist-clear", "w1"}, env)

	if err == nil || !strings.Contains(err.Error(), "no configuration yet") {
		t.Errorf("err = %v, want it to name the setup that has not happened", err)
	}
}

func TestWishlistClearReportsAFailedPageRead(t *testing.T) {
	boom := errors.New("notion is down")
	env, _ := testEnv(testConfig(), &fakeAPI{blocksErr: boom})

	err := Run(context.Background(), []string{"wishlist-clear", "w1"}, env)

	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func TestWishlistClearReportsAFailedTrashing(t *testing.T) {
	boom := errors.New("notion is down")
	api := &fakeAPI{blocks: wishlistPage(t), deleteErr: boom}
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"wishlist-clear", "w1"}, env)

	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if !strings.Contains(err.Error(), "w1") {
		t.Errorf("err = %q, want it to name the item", err)
	}
}

func TestWishlistClearReportsAFailedSeeding(t *testing.T) {
	boom := errors.New("notion is down")
	api := &fakeAPI{blocks: wishlistPage(t), appendErr: boom}
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"wishlist-clear", "w1", "w2"}, env)

	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func TestWishlistClearReportsAFailedWrite(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{blocks: wishlistPage(t)})
	env.Out = failingWriter{}

	err := Run(context.Background(), []string{"wishlist-clear", "w1"}, env)

	if !errors.Is(err, errWrite) {
		t.Errorf("err = %v, want %v", err, errWrite)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
