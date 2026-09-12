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
	"github.com/craigmjohnston/nat/internal/store"
)

// completeSlice closes out the slice an agent was working: a summary appended
// to the page body, and the properties of whichever ending was asked for.
// --blocked is one of the ways a session ends — the slice stays in progress
// and the note says what stopped it, so the work is not lost and nobody else
// picks the slice up either.
//
// --branch is another, and the one an agent ends on now: the branch the work
// was pushed to is recorded and the slice is left in progress, which on the
// board is a slice handed back and waiting to be reviewed. Approving it there
// is what opens the pull request. The --pr ending stays for whoever already
// has a pull request to record, and it too leaves the slice in progress:
// Done means the work is on main, so the merge is what writes it — nat's own,
// or the reading that finds GitHub already made one. Only a slice closed out
// with none of the three goes straight to Done, since work with no pull
// request has no merge coming.
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
	projectRef := projectFlag(flags)
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

	cfg, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	me, err := ownerOf(cfg, project)
	if err != nil {
		return err
	}
	st, err := env.storeFor(projectID, project)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	shape, err := sliceShape(ctx, st, projectID, project)
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
	s, pageShape, err := loadSlice(ctx, st, pageID)
	if err != nil {
		return err
	}
	write := shape.On(pageShape)
	if !store.Holds(s, write, me.ID) {
		return notOursError(s, me.Name, "closed out")
	}

	// The status is written in the shape the page was read in rather than the
	// shape the schema reads as, since a Status column converted in the Notion
	// UI takes a different value from the select every project this app made
	// has; the branch column is the schema's answer, because a column that is
	// not there cannot be read off a page that does not carry it.
	closed, err := st.CompleteSlice(ctx, s.ID, write, store.Outcome{
		Summary:       note,
		Branch:        *branch,
		PR:            *pr,
		PRDescription: prDescription,
		Blocked:       *blocked,
	})
	if err != nil {
		return err
	}

	env.nudged()
	_, err = io.WriteString(env.Out,
		outcomeMarkdown(closed, *blocked, *branch, me.Name))
	return err
}

// endings settles how the session is being ended before anything is read or
// written, since the three are three different endings and no two of them are
// the same slice. Handing a branch back leaves work to review; recording a pull
// request leaves the slice for the merge to close; blocked leaves it
// unfinished. Asking for two at once is a mistake in the command line, not a
// state to pick between.
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
func notOursError(s domain.Slice, assignee, action string) error {
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
	prRecorded := branch == "" && !blocked && s.PRURL != ""
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.Name)
	switch {
	case blocked:
		fmt.Fprintf(&b, "Still in progress, held by %s. The note is on the slice page.\n\n", assignee)
	case handedBack:
		fmt.Fprintf(&b, "Handed back for review, still held by %s. "+
			"The summary is on the slice page, and approving it on the board is what opens the pull request.\n\n",
			assignee)
	case prRecorded:
		fmt.Fprintf(&b, "Pull request recorded, still held by %s. "+
			"The slice goes Done when it merges — the merge is what marks the work landed.\n\n", assignee)
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
