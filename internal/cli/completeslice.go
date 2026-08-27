package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// completeSlice closes out the slice an agent was working: Status to Done, the
// PR recorded, and a summary appended to the page body. --blocked is one of the
// other ways a session ends — the slice stays in progress and the note says what
// stopped it, so the work is not lost and nobody else picks the slice up
// either.
//
// --branch is the third, and the one an agent ends on now: the branch the work
// was pushed to is recorded and the slice is left in progress, which on the
// board is a slice handed back and waiting to be reviewed. Approving it there
// is what opens the pull request and marks it Done. The --pr ending stays for
// whoever already has a pull request to record.
//
// Only a slice this user already holds can be finished. An agent that never
// claimed the slice has no business saying it is done, and a slice held by
// someone else is theirs to finish.
func completeSlice(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("complete-slice", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pr := flags.String("pr", "", "URL of the pull request this slice produced")
	branch := flags.String("branch", "", "the branch this slice's work was pushed to, handed back for review")
	summary := flags.String("summary", "", "the note to append; read from stdin when absent")
	description := flags.String("pr-description", "",
		"the pull request description for the branch handed back; `-` reads it from stdin")
	blocked := flags.Bool("blocked", false, "leave the slice in progress and record what is blocking it")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("complete-slice: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	*branch = strings.TrimSpace(*branch)
	if err := endings(*branch, *pr, *description, *blocked); err != nil {
		return err
	}
	pageID, err := pageID("complete-slice", rest[0])
	if err != nil {
		return err
	}
	// Both notes are settled before anything is written: a session with nothing
	// to say about what it did should fail having changed nothing. There is one
	// stdin and either flag may want it, so `--pr-description -` takes it and the
	// summary has to have been given as an argument.
	if *description == "-" && strings.TrimSpace(*summary) == "" {
		return usageErrorf("complete-slice: --pr-description - reads stdin, " +
			"so the summary cannot: pass --summary as well")
	}
	prDescription, err := descriptionText(*description, env.In)
	if err != nil {
		return err
	}
	in := env.In
	if *description == "-" {
		in = nil
	}
	note, err := noteText(*summary, in)
	if err != nil {
		return err
	}

	cfg, project, err := env.activeProject()
	if err != nil {
		return err
	}
	if cfg.AssigneeUserID == "" {
		return fmt.Errorf("no assignee in the config: open the board with `nat` and finish setting it up")
	}
	client := env.NewClient(env.Tokens.Token)

	shape, err := sliceShape(ctx, client, project)
	if err != nil {
		return err
	}
	// A branch nothing can hold is a hand-back that would be silently lost, so
	// it is refused here — before the note is written — rather than written
	// nowhere. Every project the app has loaded since has the column; one whose
	// column of that name is something other than text is the case left.
	if *branch != "" && !shape.HasBranch {
		return fmt.Errorf("this project's %s table has no %s text column to hand a branch back on: add one in Notion",
			notion.SlicesDBTitle, notion.PropBranch)
	}
	page, err := client.GetPage(ctx, pageID)
	if err != nil {
		return fmt.Errorf("load the slice: %w", err)
	}
	if !holds(*page, shape, cfg.AssigneeUserID) {
		return notOursError(*page, shape, cfg.AssigneeUserName, "closed out")
	}

	// The note goes on before the status does. Either write can fail, and of the
	// two half-finished states this is the recoverable one: an in-progress slice
	// carrying its summary can be completed by running this again, whereas a
	// Done slice with no summary refuses every attempt to add one.
	blocks := noteBlocks(noteHeading(*blocked, *branch), note)
	// The description goes on in the same write, under a heading of its own: it
	// is not the summary of what was done but the text the pull request will be
	// opened with, and the board reads it back off the page by that heading
	// whenever the user gets to reviewing the branch.
	if prDescription != "" {
		blocks = append(blocks, noteBlocks(notion.PRDescriptionHeading, prDescription)...)
	}
	if _, err := client.AppendBlockChildren(ctx, page.ID, blocks); err != nil {
		return fmt.Errorf("append the note to the slice: %w", err)
	}
	props := map[string]notion.PropertyValue{}
	if *pr != "" {
		props[notion.PropPR] = notion.NewURL(*pr)
	}
	if *branch != "" {
		props[notion.PropBranch] = notion.NewRichText(*branch)
	}
	// A handed-back slice stays in progress: the work is done but nobody has
	// reviewed it, and the board is where that ends — its approve key opens the
	// pull request and marks the slice Done.
	if !*blocked && *branch == "" {
		props[notion.PropStatus] = notion.NewChoice(page.Properties[notion.PropStatus].Type, notion.SliceDone)
	}
	if len(props) > 0 {
		updated, err := client.UpdatePageProperties(ctx, page.ID, props)
		if err != nil {
			return fmt.Errorf("close out the slice: %w", err)
		}
		page = updated
	}

	env.nudged()
	logging.Action("slice closed out", "slice", page.ID, "blocked", *blocked,
		"pr", *pr, "branch", *branch)
	_, err = io.WriteString(env.Out,
		outcomeMarkdown(domain.SliceFromPage(*page), *blocked, *branch, cfg.AssigneeUserName))
	return err
}

// endings settles how the session is being ended before anything is read or
// written, since the three are three different endings and no two of them are
// the same slice. Handing a branch back leaves work to review; recording a pull
// request closes the slice; blocked leaves it unfinished. Asking for two at once
// is a mistake in the command line, not a state to pick between.
// A pull request description belongs to the one ending that still has a pull
// request to open: the branch handed back for the user to review and approve.
// --pr is a pull request already open, --blocked is work that stopped, and a
// slice closed out with neither is Done — none of the three has one coming, so
// a description given alongside any of them would be written where nothing ever
// reads it.
func endings(branch, pr, description string, blocked bool) error {
	if description != "" && branch == "" {
		switch {
		case pr != "":
			return usageErrorf("complete-slice: --pr-description is for the pull request --branch has yet to open: " +
				"--pr records one that is already open")
		case blocked:
			return usageErrorf("complete-slice: --pr-description is for a branch handed back, not for stopped work: " +
				"say what stopped it in --summary")
		default:
			return usageErrorf("complete-slice: --pr-description needs the --branch it describes: " +
				"a slice closed out without one opens no pull request")
		}
	}
	if branch == "" {
		return nil
	}
	if pr != "" {
		return usageErrorf("complete-slice: --branch and --pr are two different endings: " +
			"hand the branch back, or record the pull request that came of it")
	}
	if blocked {
		return usageErrorf("complete-slice: --branch and --blocked are two different endings: " +
			"a slice handed back is finished work waiting to be reviewed, not stopped work")
	}
	return nil
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
// other — so both are accepted rather than one being the right one. The command
// is named because more than one takes a slice this way, and a misuse should
// say which one it was.
func pageID(command, ref string) (string, error) {
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
		return "", usageErrorf("%s: %q is not a slice: give its Notion URL or page ID", command, ref)
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

// descriptionText settles the pull request description: the flag as it was
// given, or stdin for the lone `-`, which is how a description longer than a
// shell argument gets in — the same convention `slice-add --description -`
// follows. A `-` with nothing behind it is a misuse rather than an empty
// description: the flag was asked for and nothing arrived, and writing no
// heading at all would silently leave the pull request to be filled from the
// commits.
func descriptionText(description string, in io.Reader) (string, error) {
	if description != "-" {
		return strings.TrimSpace(description), nil
	}
	if in == nil {
		return "", usageErrorf("complete-slice: --pr-description - has no stdin to read the description from")
	}
	b, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("read the pull request description: %w", err)
	}
	text := strings.TrimSpace(string(b))
	if text == "" {
		return "", usageErrorf("complete-slice: --pr-description - was given nothing on stdin")
	}
	return text, nil
}

// notOursError says why a slice will not be acted on, naming what the slice
// actually is: not in progress at all, or held by somebody else. It is a plain
// error rather than a usage one — the command line was fine, the slice is not.
// The action names what was refused — "closed out", "released" — since the two
// commands that hold a slice to this rule do different things with one.
//
// A project with no Assignee column never reaches the second case: holds
// decides ownership on status alone there, so there is nobody a slice could be
// held by but the person running this.
func notOursError(page notion.Page, shape notion.SliceShape, assignee, action string) error {
	s := domain.SliceFromPage(page)
	if s.Status != domain.SliceClaimed {
		return fmt.Errorf("%q is %s, not %s: only a slice you claimed can be %s",
			s.Name, blank(s.StatusName), notion.SliceInProgress, action)
	}
	if s.AssigneeName == "" {
		return fmt.Errorf("%q is in progress but held by nobody, not by %s: only a slice you claimed can be %s",
			s.Name, assignee, action)
	}
	return fmt.Errorf("%q is held by %s, not by %s: leave it to them", s.Name, s.AssigneeName, assignee)
}

// The headings the appended note is filed under, so a page read later says
// which kind of ending it was.
const (
	summaryHeading    = "Summary"
	blockedHeading    = "Blocked"
	handedBackHeading = "Handed back"
)

// noteHeading names the note by how the session ended.
func noteHeading(blocked bool, branch string) string {
	switch {
	case blocked:
		return blockedHeading
	case branch != "":
		return handedBackHeading
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
// The branch is the one just handed back rather than the one read back off the
// page: what was written is known here, and a page Notion echoes is a read of
// the same thing at best.
func outcomeMarkdown(s domain.Slice, blocked bool, branch, assignee string) string {
	if branch == "" {
		branch = s.Branch
	}
	handedBack := branch != "" && !blocked && s.PRURL == ""
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.Name)
	switch {
	case blocked:
		fmt.Fprintf(&b, "Still in progress, held by %s. The note is on the slice page.\n\n", assignee)
	case handedBack:
		fmt.Fprintf(&b, "Handed back for review, still held by %s. "+
			"The summary is on the slice page, and approving it on the board is what opens the pull request.\n\n",
			assignee)
	default:
		b.WriteString("Done. The summary is on the slice page.\n\n")
	}
	fmt.Fprintf(&b, "- Notion page: %s\n", s.ID)
	if s.URL != "" {
		fmt.Fprintf(&b, "- Notion URL: %s\n", s.URL)
	}
	if branch != "" {
		fmt.Fprintf(&b, "- Branch: %s\n", branch)
	}
	if s.PRURL != "" {
		fmt.Fprintf(&b, "- PR: %s\n", s.PRURL)
	}
	return b.String()
}
