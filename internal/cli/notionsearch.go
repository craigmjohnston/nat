package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/notion"
)

// The two kinds of place a project page can be put, as the app's picker names
// them and project-mirror's --parent-kind takes them.
const (
	parentKindPage     = "page"
	parentKindDatabase = "database"
)

// searchLimit caps how many places one search answers with. A picker draws a
// short list to choose from and narrows it by typing, so a workspace's whole
// contents is never wanted; the cap is what keeps a huge one to a single
// request's worth of work.
const searchLimit = 30

// notionSearch lists the pages and databases of the Notion workspace a project
// page could be put under, for the app's picker: --query narrows it the way the
// workspace's own search does, and none lists what was edited last.
//
// Two kinds of hit are left out, both for the same reason. A row of a database
// is a page, but a project page under one would be a page nobody browsing the
// workspace finds; and a database is answered by its data source, since that is
// what a page is created in — its ID is the one to hand project-mirror.
//
// It reads no project and writes nothing, and so takes no --project.
func notionSearch(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("notion-search", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	query := flags.String("query", "", "narrow the list to titles matching this text")
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("notion-search: takes no arguments, given %d", len(rest))
	}

	hits, _, err := env.NewClient(env.Tokens.Token).SearchPaged(ctx, strings.TrimSpace(*query), "", "")
	if err != nil {
		return fmt.Errorf("search the workspace: %w", err)
	}
	places := placesOf(hits)

	if *asJSON {
		return writeJSON(env.Out, notionSearchJSON{Places: places})
	}
	var b strings.Builder
	for _, p := range places {
		fmt.Fprintf(&b, "- %s (%s) — %s\n", p.Title, p.Kind, p.ID)
	}
	if b.Len() == 0 {
		b.WriteString("Nothing in the workspace matches.\n")
	}
	_, err = io.WriteString(env.Out, b.String())
	return err
}

// notionSearchJSON is notion-search's structured output.
type notionSearchJSON struct {
	Places []notionPlace `json:"places"`
}

// notionPlace is somewhere a project page can go. ID is what a page is created
// under: a page's own ID, or a database's data source ID.
type notionPlace struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
}

// placesOf keeps the hits a project page can be put under, in the order Notion
// gave them, up to [searchLimit].
func placesOf(hits []notion.SearchResult) []notionPlace {
	places := []notionPlace{}
	for _, h := range hits {
		if len(places) == searchLimit {
			break
		}
		var kind string
		switch h.Object {
		case notion.SearchPage:
			if h.Parent.Type == notion.ParentDatabase || h.Parent.Type == notion.ParentDataSource {
				continue
			}
			kind = parentKindPage
		case notion.SearchDataSource:
			kind = parentKindDatabase
		default:
			continue
		}
		title := h.TitleText()
		if title == "" {
			title = "Untitled"
		}
		places = append(places, notionPlace{ID: h.ID, Kind: kind, Title: title})
	}
	return places
}
