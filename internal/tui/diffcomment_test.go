package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/craigmjohnston/nat/internal/git"
)

// errSend is the failure a send is refused with.
var errSend = errors.New("no server running")

// The first file of the sample diff, and two of its lines: the removal and the
// addition that replaced it, which is what the tests comment on.
const (
	firstFile   = "internal/tui/board.go"
	removedLine = 6
	addedLine   = 7
)

// commented is a diff screen with one comment already on it, on the added line
// of the first file.
func commented(t *testing.T, text string) *Diff {
	t.Helper()
	d := newTestDiff()
	d.SetComment(firstFile, addedLine, 1, text)
	if d.Pending() != 1 {
		t.Fatalf("Pending() = %d after a comment, want 1", d.Pending())
	}
	return d
}

// TestDiffCommentMarksItsLines covers what a pending comment does to the
// screen: the lines it covers carry the gutter mark, and the hints row counts
// what is waiting.
func TestDiffCommentMarksItsLines(t *testing.T) {
	d := newTestDiff()
	if got := d.sendBinding().Help().Desc; got != "send comments" {
		t.Errorf("send hint = %q, want the key's own description with nothing pending", got)
	}
	d.SetComment(firstFile, removedLine, 2, "  both of these  ")

	for _, line := range []int{removedLine, addedLine} {
		if !d.marks[commentKey{path: firstFile, start: line}] {
			t.Errorf("line %d carries no mark, want the comment shown on it", line)
		}
	}
	if d.marks[commentKey{path: firstFile, start: addedLine + 1}] {
		t.Error("the line after the comment should carry no mark")
	}
	if !strings.Contains(d.vp.GetContent(), commentMark) {
		t.Error("the body should draw the comment's mark")
	}
	if got := d.sendBinding().Help().Desc; got != "send 1 comment" {
		t.Errorf("send hint = %q, want the pending count", got)
	}

	// The removal and the addition that replaced it: the branch leaves one line
	// there, and that is the line the comment names.
	got := d.Comments()
	if len(got) != 1 || got[0].Text != "both of these" || got[0].Ref != "line 13" ||
		len(got[0].Lines) != 2 || got[0].Path != firstFile {
		t.Errorf("Comments() = %+v, want the two lines, trimmed text and their reference", got)
	}
}

// TestDiffCommentReplacedAndRemoved covers commenting on the same lines again:
// the second comment replaces the first, and emptying it takes it back.
func TestDiffCommentReplacedAndRemoved(t *testing.T) {
	d := commented(t, "first thought")
	d.SetComment(firstFile, addedLine, 1, "second thought")
	if got := d.Comments(); len(got) != 1 || got[0].Text != "second thought" {
		t.Errorf("Comments() = %+v, want the one comment replaced", got)
	}
	d.SetComment(firstFile, addedLine, 1, "   ")
	if d.Pending() != 0 || d.marks[commentKey{path: firstFile, start: addedLine}] {
		t.Errorf("Pending() = %d, want an emptied comment taken back", d.Pending())
	}
}

// TestDiffCommentOnLinesThatAreNotThere covers a comment the screen cannot
// place: a file the diff does not hold, and a run that runs off its end.
func TestDiffCommentOnLinesThatAreNotThere(t *testing.T) {
	d := newTestDiff()
	for _, tt := range []struct {
		name        string
		path        string
		start, span int
	}{
		{"an unknown file", "nowhere.go", 0, 1},
		{"a negative line", firstFile, -1, 1},
		{"no lines at all", firstFile, 0, 0},
		{"past the end", firstFile, 0, 500},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d.SetComment(tt.path, tt.start, tt.span, "words")
			if d.Pending() != 0 {
				t.Errorf("Pending() = %d, want the comment refused", d.Pending())
			}
		})
	}
	if got := d.SelectionRef("nowhere.go", 0, 1); got != "" {
		t.Errorf("SelectionRef of an unknown file = %q, want nothing", got)
	}
}

// TestDiffCommentsAreOrderedByFileAndLine covers the order they are sent in,
// which is the order the diff draws them.
func TestDiffCommentsAreOrderedByFileAndLine(t *testing.T) {
	d := newTestDiff()
	d.SetComment("internal/tui/diff.go", 6, 1, "the second file")
	d.SetComment(firstFile, addedLine, 1, "the later line")
	d.SetComment(firstFile, removedLine, 1, "the earlier line")

	var got []string
	for _, c := range d.Comments() {
		got = append(got, c.Text)
	}
	want := []string{"the earlier line", "the later line", "the second file"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Comments() = %v, want %v", got, want)
	}
}

// TestDiffSelectionMarksARange covers v: the cursor line and the mark are the
// two ends of what a comment goes on, and the range never leaves the file it
// was started in.
func TestDiffSelectionMarksARange(t *testing.T) {
	d := newTestDiff()
	d.Update(keyPress("j"))
	if !d.ToggleSelect() || !d.Selecting() {
		t.Fatal("v should mark the cursor line as one end of a range")
	}
	d.Update(keyPress("j"))
	d.Update(keyPress("j"))
	path, start, span, text, ok := d.Selection()
	if !ok || path != firstFile || start != 1 || span != 3 || text != "" {
		t.Errorf("Selection() = %q %d %d %q %v, want three lines of the first file",
			path, start, span, text, ok)
	}
	if !d.selected(d.offsets[0]+1) || d.selected(d.offsets[0]+4) {
		t.Error("the marked range should be drawn as selected, and nothing beyond it")
	}

	// Every line of the sample's first file, and then some: the cursor holds at
	// the last of them rather than running into the next file.
	for range len(d.files[0].Lines) + 3 {
		d.Update(keyPress("j"))
	}
	if got := d.lines[d.line]; got.file != 0 {
		t.Errorf("cursor at %+v, want it held inside the file the range started in", got)
	}

	if d.ToggleSelect() || d.Selecting() {
		t.Error("v again should take the mark off")
	}
}

// TestDiffCancelSelect covers esc's half of the range: it drops one when there
// is one, and says so, so the app knows whether the key was spent.
func TestDiffCancelSelect(t *testing.T) {
	d := newTestDiff()
	if d.CancelSelect() {
		t.Error("CancelSelect() = true with no range marked")
	}
	d.ToggleSelect()
	if !d.CancelSelect() || d.Selecting() {
		t.Error("CancelSelect() should drop the range being marked")
	}
}

// TestDiffSelectionWithNothingToSelect covers the screen before a diff has
// landed, and a cursor sitting on the blank line between two sections: neither
// is something to comment on.
func TestDiffSelectionWithNothingToSelect(t *testing.T) {
	empty := NewDiff(DefaultStyles())
	empty.SetSize(diffTestWidth, diffTestHeight)
	if _, _, _, _, ok := empty.Selection(); ok {
		t.Error("Selection() should find nothing on a screen with no diff on it")
	}
	if empty.ToggleSelect() {
		t.Error("ToggleSelect() should mark nothing on a screen with no diff on it")
	}

	d := newTestDiff()
	d.line = d.offsets[1] - 1
	if got := d.lines[d.line]; got != separator {
		t.Fatalf("line %d is %+v, want the separator between two sections", d.line, got)
	}
	if _, _, _, _, ok := d.Selection(); ok {
		t.Error("Selection() should find nothing on the line between two files")
	}
}

// TestDiffSelectionAcrossFilesIsOneFile covers a mark left in one file and a
// cursor forced into another, which is the state a jump under a mark reaches:
// the comment is on the file the cursor is in, not both.
func TestDiffSelectionAcrossFilesIsOneFile(t *testing.T) {
	d := newTestDiff()
	d.anchor, d.anchored = 0, true
	d.line = d.offsets[1]
	path, start, span, _, ok := d.Selection()
	if !ok || path != "internal/tui/diff.go" || start != 0 || span != 1 {
		t.Errorf("Selection() = %q %d %d %v, want the cursor's own line alone", path, start, span, ok)
	}
}

// TestDiffCommentsSurviveARereadThatMovedThem covers the refresh key over a
// review in progress: a comment goes with the lines it was left on when they
// can still be found, and is dropped when they cannot.
func TestDiffCommentsSurviveARereadThatMovedThem(t *testing.T) {
	d := commented(t, "clamp this")
	d.SetComment("internal/tui/diff.go", 6, 1, "and this")

	// The same diff with a line inserted above the comment, and the second
	// file's commented line changed out from under its comment.
	moved := strings.Replace(sampleDiff,
		" \tlines := b.rows", " \tlines := b.rows\n \tanother context line", 1)
	moved = strings.Replace(moved, "+package tui", "+package tui // changed", 1)

	if dropped := d.SetFiles("origin/main", git.ParseFiles(moved)); dropped != 1 {
		t.Errorf("SetFiles dropped %d comments, want the one whose lines changed", dropped)
	}
	got := d.Comments()
	if len(got) != 1 || got[0].Text != "clamp this" || got[0].Path != firstFile {
		t.Fatalf("Comments() = %+v, want the moved one kept", got)
	}
	if !d.marks[commentKey{path: firstFile, start: addedLine + 1}] {
		t.Error("the kept comment should be marked on the line it moved to")
	}
}

// TestDiffCommentsDroppedWhenTheirFileGoes covers a re-read that no longer
// touches the file a comment was left on, and one where the lines it was left
// on now occur twice: neither can be re-homed without guessing.
func TestDiffCommentsDroppedWhenTheirFileGoes(t *testing.T) {
	d := commented(t, "gone with the file")
	if dropped := d.SetFiles("origin/main", manyFiles(2)); dropped != 1 {
		t.Errorf("SetFiles dropped %d comments, want the one whose file went", dropped)
	}
	if d.Pending() != 0 {
		t.Errorf("Pending() = %d, want the comment dropped", d.Pending())
	}

	// A comment whose line has both moved and been duplicated: it is no longer
	// where it was, and there are now two places it could be.
	twice := newTestDiff()
	twice.SetComment("internal/tui/diff.go", 8, 1, "which one?")
	doubled := strings.Replace(sampleDiff, "+// Diff is the review screen.",
		"+// moved down by this\n+// Diff is the review screen.\n+// Diff is the review screen.", 1)
	if dropped := twice.SetFiles("origin/main", git.ParseFiles(doubled)); dropped != 1 {
		t.Error("a comment whose lines now occur twice should be dropped")
	}
}

// TestDiffCommentsGoWithTheSlice covers what a fresh read keeps: the comments
// left on one slice's branch survive a re-read of it — that is the refresh key
// — and go when the screen is pointed at another slice or reset entirely.
func TestDiffCommentsGoWithTheSlice(t *testing.T) {
	d := commented(t, "still here")
	d.Start("slice-1", "Diff viewer", "slice/diff-viewer", "/repos/nat")
	if d.Pending() != 1 {
		t.Error("a re-read of the same slice's branch should keep the comments")
	}
	d.Start("slice-2", "Another", "slice/other", "/repos/nat")
	if d.Pending() != 0 {
		t.Error("another slice's branch should start with no comments on it")
	}

	d = commented(t, "cleared")
	d.Reset()
	if d.Pending() != 0 {
		t.Error("Reset should drop the pending comments with the diff")
	}
}

// TestCommentPrompt covers the one turn the comments are delivered as: what it
// is about, then each comment under the lines it was left on, quoted as git
// wrote them.
func TestCommentPrompt(t *testing.T) {
	got := commentPrompt("slice/review", []Comment{
		{Path: "main.go", Ref: "line 12", Lines: []string{"+\tfoo()"}, Text: "call bar instead"},
		{Path: "other.go", Lines: []string{"binary"}, Text: "and this"},
	})
	for _, want := range []string{
		"I have reviewed the diff of slice/review and left 2 comments on it.",
		"## main.go, line 12",
		"> +\tfoo()",
		"call bar instead",
		"## other.go, 1 line",
		"and this",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt is missing %q:\n%s", want, got)
		}
	}
}

// commentApp returns an app on the review screen with the sample diff loaded
// and an agent running for the slice it is of, which is what the send key
// needs.
func commentApp(t *testing.T) (*App, *fakeLauncher) {
	t.Helper()
	app, _, _ := diffApp(t)
	launcher := &fakeLauncher{live: map[string]string{handedBack: "nat-hb"}}
	app.launcher = launcher
	app.live = launcher.live
	cursorOn(t, app, handedBack)
	drive(t, app, press(app, "v"))
	if app.screen != screenDiff || len(app.diff.files) == 0 {
		t.Fatalf("screen = %v with %d files, want the diff up", app.screen, len(app.diff.files))
	}
	return app, launcher
}

// TestCommentKeyRecordsAComment covers the whole of c: the box opens over the
// diff, what is typed into it lands on the lines the cursor is on, and closing
// it leaves the user where they were reading.
func TestCommentKeyRecordsAComment(t *testing.T) {
	app, _ := commentApp(t)
	app.diff.Update(keyPress("j"))

	press(app, "c")
	if _, ok := app.form.(*CommentForm); !ok {
		t.Fatalf("form = %T, want the comment box", app.form)
	}
	if got := app.form.Heading(); !strings.Contains(got, firstFile) {
		t.Errorf("heading = %q, want the file named", got)
	}
	// The box floats over the diff rather than the board, so the change being
	// read is still there behind it.
	if !strings.Contains(xansi.Strip(app.scrimView()), "diff --git") {
		t.Error("the backdrop should be the diff the box was opened over")
	}

	typeText(app, "clamp this")
	finishForm(t, app, press(app, "enter"))
	if app.screen != screenDiff {
		t.Errorf("screen = %v after the box, want the diff still up", app.screen)
	}
	if app.busy {
		t.Error("a comment is recorded in the session, so nothing should be left in flight")
	}
	got := app.diff.Comments()
	if len(got) != 1 || got[0].Text != "clamp this" || got[0].Path != firstFile {
		t.Fatalf("Comments() = %+v, want what was typed", got)
	}
	if !strings.Contains(app.toast, "1 comment pending") {
		t.Errorf("toast = %q, want the pending count", app.toast)
	}
}

// TestCommentKeyEmptiedTakesItBack covers the box closed on nothing, which is
// how a comment is removed.
func TestCommentKeyEmptiedTakesItBack(t *testing.T) {
	app, _ := commentApp(t)
	app.diff.SetComment(firstFile, 0, 1, "x")

	press(app, "c")
	// The box opens on what was said, so taking it back is deleting it.
	app.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	finishForm(t, app, press(app, "enter"))
	if app.diff.Pending() != 0 {
		t.Errorf("Pending() = %d, want the comment taken back", app.diff.Pending())
	}
	if !strings.Contains(app.toast, "removed") {
		t.Errorf("toast = %q, want the removal said out loud", app.toast)
	}
}

// TestCommentKeyCancelledReturnsToTheDiff covers esc over the box: the comment
// is abandoned and the diff is still up.
func TestCommentKeyCancelledReturnsToTheDiff(t *testing.T) {
	app, _ := commentApp(t)
	press(app, "c")
	press(app, "esc")
	if app.form != nil || app.screen != screenDiff {
		t.Errorf("form = %v on screen %v, want the box gone and the diff up", app.form, app.screen)
	}
	if app.diff.Pending() != 0 {
		t.Errorf("Pending() = %d, want nothing recorded", app.diff.Pending())
	}
}

// TestCommentKeyWithNothingToCommentOn covers c before a diff has landed.
func TestCommentKeyWithNothingToCommentOn(t *testing.T) {
	app, _ := commentApp(t)
	app.diff.Fail(errSend)
	press(app, "c")
	if app.form != nil {
		t.Fatalf("form = %T, want no box over a screen with no lines on it", app.form)
	}
	if !strings.Contains(app.toast, "Move to a line") {
		t.Errorf("toast = %q, want it to say what the key needs", app.toast)
	}
}

// TestSelectKeyAndEscOnTheDiff covers v marking a range and esc dropping it —
// and the esc after that leaving the screen, since a range is the nearer of the
// two undos.
func TestSelectKeyAndEscOnTheDiff(t *testing.T) {
	app, _ := commentApp(t)
	press(app, "v")
	if !app.diff.Selecting() {
		t.Fatal("v on the diff should mark a range")
	}
	press(app, "esc")
	if app.diff.Selecting() || app.screen != screenDiff {
		t.Error("esc should drop the range and leave the screen up")
	}
	press(app, "esc")
	if app.screen != screenBoard {
		t.Errorf("screen = %v, want the second esc back to the board", app.screen)
	}
}

// TestSendCommentsGoesToTheAgentInOneTurn covers s: every pending comment, for
// every file, typed at the agent's pane as one prompt and cleared once it has
// got there.
func TestSendCommentsGoesToTheAgentInOneTurn(t *testing.T) {
	app, launcher := commentApp(t)
	app.diff.SetComment(firstFile, addedLine, 1, "clamp this")
	app.diff.SetComment("internal/tui/diff.go", 6, 1, "and name it better")

	drive(t, app, press(app, "s"))
	if len(launcher.prompts) != 1 || launcher.prompts[0].session != "nat-hb" {
		t.Fatalf("sends = %+v, want one prompt to the slice's own session", launcher.prompts)
	}
	sent := launcher.prompts[0].text
	for _, want := range []string{"slice/approve", firstFile, "clamp this",
		"internal/tui/diff.go", "and name it better"} {
		if !strings.Contains(sent, want) {
			t.Errorf("prompt is missing %q:\n%s", want, sent)
		}
	}
	if app.diff.Pending() != 0 {
		t.Errorf("Pending() = %d, want the comments cleared once sent", app.diff.Pending())
	}
	if !strings.Contains(app.toast, "Sent 2 comments to nat-hb.") {
		t.Errorf("toast = %q, want the send reported", app.toast)
	}
}

// TestSendCommentsWithNothingPending covers s before anything has been said.
func TestSendCommentsWithNothingPending(t *testing.T) {
	app, launcher := commentApp(t)
	drive(t, app, press(app, "s"))
	if len(launcher.prompts) != 0 {
		t.Errorf("sends = %+v, want nothing sent", launcher.prompts)
	}
	if !strings.Contains(app.toast, "No comments to send") {
		t.Errorf("toast = %q, want it to say there is nothing to send", app.toast)
	}
}

// TestSendCommentsWithNoAgentRunning covers the agent having exited: there is
// nobody to tell, and the comments stay where they are until there is.
func TestSendCommentsWithNoAgentRunning(t *testing.T) {
	app, launcher := commentApp(t)
	app.live = nil
	app.diff.SetComment(firstFile, addedLine, 1, "clamp this")

	drive(t, app, press(app, "s"))
	if len(launcher.prompts) != 0 {
		t.Errorf("sends = %+v, want nothing sent", launcher.prompts)
	}
	if app.diff.Pending() != 1 {
		t.Error("the comments should still be pending with no agent to send them to")
	}
	if !strings.Contains(app.toast, "No agent session is running") {
		t.Errorf("toast = %q, want it to name what is missing", app.toast)
	}
}

// TestSendCommentsThatDidNotGetThere covers tmux refusing: the comments are held
// nowhere else, so a failed send leaves every one of them pending.
func TestSendCommentsThatDidNotGetThere(t *testing.T) {
	app, launcher := commentApp(t)
	launcher.sendErr = errSend
	app.diff.SetComment(firstFile, addedLine, 1, "clamp this")

	drive(t, app, press(app, "s"))
	if app.diff.Pending() != 1 {
		t.Errorf("Pending() = %d, want a failed send to keep the comments", app.diff.Pending())
	}
	if !strings.Contains(app.toast, "Could not send the comments to nat-hb") {
		t.Errorf("toast = %q, want the failure named", app.toast)
	}
}

// TestSendCommentsWithoutATmux covers the app built without a launcher at all,
// which is what the flows check before they reach for one.
func TestSendCommentsWithoutATmux(t *testing.T) {
	app, _ := commentApp(t)
	app.launcher = nil
	if cmd := app.sendCommentsFlow(); cmd != nil {
		t.Error("the send should do nothing with no tmux to send through")
	}
}

// TestDiffRereadSaysWhatItDropped covers the refresh key over a review whose
// lines have moved out from under a comment: the drop is said out loud, since
// the comments are held nowhere else.
func TestDiffRereadSaysWhatItDropped(t *testing.T) {
	app, _ := commentApp(t)
	app.diff.SetComment(firstFile, addedLine, 1, "clamp this")

	app.Update(diffLoadedMsg{base: "origin/main", files: manyFiles(2)})
	if app.diff.Pending() != 0 {
		t.Errorf("Pending() = %d, want the comment dropped with its lines", app.diff.Pending())
	}
	if !strings.Contains(app.toast, "1 comment dropped") {
		t.Errorf("toast = %q, want the drop reported", app.toast)
	}
}

// TestCommentBoxSizedWithTheWindow covers the box being told the room it has,
// like every other modal.
func TestCommentBoxSizedWithTheWindow(t *testing.T) {
	app, _ := commentApp(t)
	press(app, "c")
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := app.form.View(); got == "" {
		t.Error("the box should draw at the size it was given")
	}
	if got := busyNoteOf(app.form); got != "" {
		t.Errorf("busy note = %q, want nothing announced for a comment", got)
	}
}

// TestDiffCommentsSurviveARereadThatChangedNothing covers the refresh key over
// a review where the branch has not moved: every comment is exactly where it
// was.
func TestDiffCommentsSurviveARereadThatChangedNothing(t *testing.T) {
	d := commented(t, "still about this line")
	if dropped := d.SetFiles("origin/main", git.ParseFiles(sampleDiff)); dropped != 0 {
		t.Errorf("SetFiles dropped %d comments, want none", dropped)
	}
	if got := d.Comments(); len(got) != 1 || got[0].Ref != "line 13" {
		t.Errorf("Comments() = %+v, want the comment where it was", got)
	}
}

// TestDiffCommentsDroppedWhenTheirFileShrinks covers a re-read whose file no
// longer has room for the lines a comment covered.
func TestDiffCommentsDroppedWhenTheirFileShrinks(t *testing.T) {
	d := newTestDiff()
	d.SetComment(firstFile, removedLine, 3, "these three")
	shrunk := strings.Replace(sampleDiff, "-\treturn strings.Join(lines, \"\\n\")\n", "", 1)
	if dropped := d.SetFiles("origin/main", git.ParseFiles(shrunk)); dropped != 1 {
		t.Errorf("SetFiles dropped %d comments, want the one that no longer fits", dropped)
	}
}

// TestDiffCommentsAfterAFailedReread covers the comments outliving the diff
// they were left on: a read that failed takes the files with it, and the
// comments are still there to be sent, ordered by the only thing left to order
// them by.
func TestDiffCommentsAfterAFailedReread(t *testing.T) {
	d := commented(t, "about the first file")
	d.SetComment("internal/tui/diff.go", 6, 1, "about the second")
	d.Fail(errSend)

	var got []string
	for _, c := range d.Comments() {
		got = append(got, c.Path)
	}
	want := []string{firstFile, "internal/tui/diff.go"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Comments() = %v, want %v — by path, with no diff left to order them by", got, want)
	}
}

// TestDiffSelectionAnchoredOnASeparator covers a range marked from the blank
// line between two sections, which belongs to no file: there is nothing to hold
// the cursor inside, so it moves as it would with no mark at all.
func TestDiffSelectionAnchoredOnASeparator(t *testing.T) {
	d := newTestDiff()
	d.anchor, d.anchored = d.offsets[1]-1, true
	d.line = d.offsets[1]
	d.Update(keyPress("j"))
	if got := d.lines[d.line]; got != (bodyLine{file: 1, line: 1}) {
		t.Errorf("cursor at %+v, want it free to move under a mark that is on no file", got)
	}
}

// TestDiffCursorWithNoBandToDrawIn covers the screen before the window has been
// measured: the cursor still moves, and nothing tries to scroll a viewport with
// no lines in it.
func TestDiffCursorWithNoBandToDrawIn(t *testing.T) {
	d := NewDiff(DefaultStyles())
	d.SetFiles("origin/main", git.ParseFiles(sampleDiff))
	d.Update(keyPress("j"))
	d.Update(keyPress("f"))
	if want := d.offsets[0] + 1; d.line != want {
		t.Errorf("line = %d, want the cursor moved once, to %d, and left alone by the scroll",
			d.line, want)
	}
}

// TestDiffScrollThatMovesNothing covers a page key with nowhere to go: the
// cursor is already on the body, so it stays exactly where it was.
func TestDiffScrollThatMovesNothing(t *testing.T) {
	d := newTestDiff()
	d.Update(keyPress("j"))
	d.Update(keyPress("b"))
	if d.line != d.offsets[0]+1 || d.vp.YOffset() != 0 {
		t.Errorf("cursor at %d, view at %d, want both left alone", d.line, d.vp.YOffset())
	}
}
