package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// completeSlice closes out the slice an agent was working: Status to Done, the
// PR recorded, and a summary appended to the page body. --blocked is the other
// way a session ends — the slice stays Claimed and the note says what stopped
// it, so the work is not lost and nobody else picks the slice up either.
//
// Only a slice this user already holds can be finished. An agent that never
// claimed the slice has no business saying it is done, and a slice held by
// someone else is theirs to finish.
func completeSlice(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("complete-slice", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pr := flags.String("pr", "", "URL of the pull request this slice produced")
	summary := flags.String("summary", "", "the note to append; read from stdin when absent")
	blocked := flags.Bool("blocked", false, "leave the slice Claimed and record what is blocking it")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("complete-slice: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	pageID, err := pageID(rest[0])
	if err != nil {
		return err
	}
	// The note is settled before anything is written: a session with nothing to
	// say about what it did should fail having changed nothing.
	note, err := noteText(*summary, env.In)
	if err != nil {
		return err
	}

	cfg, _, err := env.activeProject()
	if err != nil {
		return err
	}
	if cfg.AssigneeUserID == "" {
		return fmt.Errorf("no assignee in the config: open the board with `nat` and finish setting it up")
	}
	client := env.NewClient(env.Tokens.Token)

	page, err := client.GetPage(ctx, pageID)
	if err != nil {
		return fmt.Errorf("load the slice: %w", err)
	}
	if !holds(*page, cfg.AssigneeUserID) {
		return notOursError(*page, cfg.AssigneeUserName)
	}

	// The note goes on before the status does. Either write can fail, and of the
	// two half-finished states this is the recoverable one: a Claimed slice
	// carrying its summary can be completed by running this again, whereas a
	// Done slice with no summary refuses every attempt to add one.
	if _, err := client.AppendBlockChildren(ctx, page.ID, noteBlocks(noteHeading(*blocked), note)); err != nil {
		return fmt.Errorf("append the note to the slice: %w", err)
	}
	props := map[string]notion.PropertyValue{}
	if *pr != "" {
		props[notion.PropPR] = notion.NewURL(*pr)
	}
	if !*blocked {
		props[notion.PropStatus] = notion.NewChoice(page.Properties[notion.PropStatus].Type, notion.SliceDone)
	}
	if len(props) > 0 {
		updated, err := client.UpdatePageProperties(ctx, page.ID, props)
		if err != nil {
			return fmt.Errorf("close out the slice: %w", err)
		}
		page = updated
	}

	_, err = io.WriteString(env.Out, outcomeMarkdown(domain.SliceFromPage(*page), *blocked, cfg.AssigneeUserName))
	return err
}

// parseFlags parses a command line whose flags may come either side of its
// arguments, and returns the arguments. The flag package stops at the first
// non-flag, which would make `complete-slice <slice> --pr ...` — the order
// anyone writes it in — silently drop the flags, so parsing resumes after each
// argument until the line is used up.
func parseFlags(flags *flag.FlagSet, args []string) ([]string, error) {
	var rest []string
	for {
		if err := flags.Parse(args); err != nil {
			return nil, usageErrorf("%s: %s", flags.Name(), err)
		}
		if flags.NArg() == 0 {
			return rest, nil
		}
		rest = append(rest, flags.Arg(0))
		args = flags.Args()[1:]
	}
}

// uuidTail matches a Notion page ID at the end of a string, dashed or not,
// which is where both a bare ID and a page URL keep it.
var uuidTail = regexp.MustCompile(`(?i)[0-9a-f]{8}-?[0-9a-f]{4}-?[0-9a-f]{4}-?[0-9a-f]{4}-?[0-9a-f]{12}$`)

// pageID reads the page ID out of however the slice was named: the ID itself,
// or a Notion URL, whose last path segment ends in the ID after a title slug.
// Both are what an agent has to hand — the brief prints them one under the
// other — so both are accepted rather than one being the right one.
func pageID(ref string) (string, error) {
	s := ref
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSuffix(s, "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	id := uuidTail.FindString(s)
	if id == "" {
		return "", usageErrorf("complete-slice: %q is not a slice: give its Notion URL or page ID", ref)
	}
	return id, nil
}

// noteText settles what to append: the --summary flag, or stdin when the flag is
// absent, which is how a summary longer than a shell argument gets in. An empty
// one is a misuse rather than an empty note — a slice closed out with nothing
// written on it loses the only record of what was done.
func noteText(summary string, in io.Reader) (string, error) {
	text := summary
	if strings.TrimSpace(text) == "" && in != nil {
		b, err := io.ReadAll(in)
		if err != nil {
			return "", fmt.Errorf("read the summary: %w", err)
		}
		text = string(b)
	}
	if text = strings.TrimSpace(text); text == "" {
		return "", usageErrorf("complete-slice: no summary given: pass --summary or pipe one in")
	}
	return text, nil
}

// notOursError says why a slice will not be closed out, naming what the slice
// actually is: not Claimed at all, or claimed by somebody else. It is a plain
// error rather than a usage one — the command line was fine, the slice is not.
func notOursError(page notion.Page, assignee string) error {
	s := domain.SliceFromPage(page)
	if s.Status != notion.SliceClaimed {
		return fmt.Errorf("%q is %s, not Claimed: only a slice you claimed can be closed out",
			s.Name, blank(string(s.Status)))
	}
	if s.AssigneeName == "" {
		return fmt.Errorf("%q is Claimed by nobody, not by %s: only a slice you claimed can be closed out",
			s.Name, assignee)
	}
	return fmt.Errorf("%q is claimed by %s, not by %s: leave it to them", s.Name, s.AssigneeName, assignee)
}

// The headings the appended note is filed under, so a page read later says
// which kind of ending it was.
const (
	summaryHeading = "Summary"
	blockedHeading = "Blocked"
)

// noteHeading names the note by how the session ended.
func noteHeading(blocked bool) string {
	if blocked {
		return blockedHeading
	}
	return summaryHeading
}

// noteBlocks turns the note into the blocks appended to the slice page: a
// heading, then one paragraph per blank-line-separated chunk. Paragraphs and
// nothing else — the note arrives as plain text, and pretending to parse
// markdown out of it would only sometimes be right.
func noteBlocks(heading, note string) []map[string]any {
	blocks := []map[string]any{textBlock("heading_3", heading)}
	for _, chunk := range strings.Split(strings.ReplaceAll(note, "\r\n", "\n"), "\n\n") {
		if text := strings.TrimSpace(chunk); text != "" {
			blocks = append(blocks, textBlock("paragraph", text))
		}
	}
	return blocks
}

// textBlock builds a block of the given type holding one span of plain text,
// which is the shape every block this command writes takes.
func textBlock(blockType, text string) map[string]any {
	return map[string]any{
		"object": "block",
		"type":   blockType,
		blockType: map[string]any{
			"rich_text": []map[string]any{{
				"type": "text",
				"text": map[string]any{"content": text},
			}},
		},
	}
}

// outcomeMarkdown reports what was written, so the agent that ran the command —
// and the person reading over its shoulder — can see the slice really did move.
func outcomeMarkdown(s domain.Slice, blocked bool, assignee string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.Name)
	if blocked {
		fmt.Fprintf(&b, "Still Claimed by %s. The note is on the slice page.\n\n", assignee)
	} else {
		b.WriteString("Done. The summary is on the slice page.\n\n")
	}
	fmt.Fprintf(&b, "- Notion page: %s\n", s.ID)
	if s.URL != "" {
		fmt.Fprintf(&b, "- Notion URL: %s\n", s.URL)
	}
	if s.PRURL != "" {
		fmt.Fprintf(&b, "- PR: %s\n", s.PRURL)
	}
	return b.String()
}
