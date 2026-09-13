package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/craigmjohnston/nat/internal/notion"
)

// sliceStatus reads one slice's status fresh, straight off its page rather
// than through a project's plan — the read the macOS app's session reaper
// verifies a kill against. A session's own claim is always written before the
// session exists, so a fresh page read taken after observing a live session
// can never show a phantom state, where a cached plan reading taken a poll
// ago might; that is the whole reason this command exists rather than the
// reaper reading a plan it already has.
//
// --project pins the config and the token exactly as every other command's
// does, but the slice named need not be one that project's plan holds: the
// page is read directly by ID, and naming a project is only ever how a
// command is told which credentials to read with. A page Notion says does
// not exist is reported as gone rather than as a refusal — a slice trashed
// for good, or one the ID never named at all, is exactly the answer a caller
// asking after it is owed, not an error to stop a sweep over.
//
// It writes nothing, so there is no nudge to fire.
func sliceStatus(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-status", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of plain text")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-status: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-status", rest[0])
	if err != nil {
		return err
	}

	if _, _, _, err := env.projectFor(*projectRef); err != nil {
		return err
	}

	client := env.NewClient(env.Tokens.Token)
	page, err := client.GetPage(ctx, id)
	if err != nil {
		var apiErr *notion.APIError
		if errors.As(err, &apiErr) && apiErr.NotFound() {
			return writeSliceStatusGone(env.Out, *asJSON)
		}
		return fmt.Errorf("read the slice: %w", err)
	}

	trashed := page.Archived || page.InTrash
	status := page.Properties[notion.PropStatus].SelectName()

	if *asJSON {
		return writeJSON(env.Out, sliceStatusJSON{Status: status, Trashed: trashed})
	}
	_, err = io.WriteString(env.Out, sliceStatusMarkdown(status, trashed))
	return err
}

// sliceStatusJSON is one slice's status and whether its page has been
// trashed — the shape a page Notion could still read answers in.
type sliceStatusJSON struct {
	Status  string `json:"status"`
	Trashed bool   `json:"trashed"`
}

// sliceStatusGoneJSON is the other shape: a page Notion no longer has any
// record of at all, distinct from one merely trashed — a trashed page is
// still there to read and still answers with its status.
type sliceStatusGoneJSON struct {
	Gone bool `json:"gone"`
}

// writeSliceStatusGone reports a page Notion says does not exist, in whichever
// shape was asked for.
func writeSliceStatusGone(out io.Writer, asJSON bool) error {
	if asJSON {
		return writeJSON(out, sliceStatusGoneJSON{Gone: true})
	}
	_, err := io.WriteString(out, "gone\n")
	return err
}

// sliceStatusMarkdown is the plain-text form: the status alone, with a
// trashed page saying so beside it — trashed is not a status Notion has a
// name for, so it is never one of the words status itself takes.
func sliceStatusMarkdown(status string, trashed bool) string {
	if trashed {
		return fmt.Sprintf("%s (trashed)\n", blank(status))
	}
	return blank(status) + "\n"
}
