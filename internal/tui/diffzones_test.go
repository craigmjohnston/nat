package tui

import (
	"fmt"
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/craigmjohnston/nat/internal/git"
)

// zonesPath is the file the expand-zone tests read, and zonesLines how many
// lines it has: enough on either side of both changes that the gaps run past a
// single step and the whole-gap control is offered.
const (
	zonesPath  = "internal/tui/wide.go"
	zonesLines = 60
)

// zonesSource is the file as the branch leaves it, one line per line so a
// revealed line says which one it is.
func zonesSource() []string {
	src := make([]string, zonesLines)
	for i := range src {
		src[i] = fmt.Sprintf("line %d", i+1)
	}
	return src
}

// zonesDiff is a change to that file in two hunks, each replacing one line, so
// the file has three gaps: before the first hunk, between the two, and after the
// last.
var zonesDiff = "diff --git a/" + zonesPath + " b/" + zonesPath + "\n" +
	"index 1111111..2222222 100644\n--- a/" + zonesPath + "\n+++ b/" + zonesPath + "\n" +
	"@@ -20,3 +20,3 @@\n line 20\n-was 21\n+line 21\n line 22\n" +
	"@@ -40,3 +40,3 @@\n line 40\n-was 41\n+line 41\n line 42\n"

// newZonesDiff is the review screen showing that change with the file behind it,
// which is what there are expand zones to draw.
func newZonesDiff(t *testing.T) *Diff {
	t.Helper()
	d := NewDiff(DefaultStyles())
	d.SetSize(80, 40)
	d.Start("slice-1", "Expand zones", "slice/zones", "/repos/nat")
	d.SetFiles("origin/main", git.ParseFiles(zonesDiff),
		map[string][]string{zonesPath: zonesSource()})
	if len(d.zones) != 1 || len(d.zones[0]) != 3 {
		t.Fatalf("built %v, want a zone for each of the file's three gaps", d.zones)
	}
	return &d
}

// body is the diff as it is drawn, without its colours.
func body(d *Diff) string { return xansi.Strip(d.vp.GetContent()) }

// diffZonesApp is the whole app on that change, with git answering for the file
// behind it, which is what the mouse tests need a window to click into.
func diffZonesApp(t *testing.T) *App {
	t.Helper()
	app, differ, _ := diffApp(t)
	differ.out = zonesDiff
	differ.files = map[string][]string{zonesPath: zonesSource()}
	cursorOn(t, app, handedBack)
	app.Update(first[diffLoadedMsg](t, run(press(app, "v"))))
	if len(app.diff.zones) != 1 || len(app.diff.zones[0]) != 3 {
		t.Fatalf("the screen built %v, want a zone for each gap", app.diff.zones)
	}
	return app
}

// controlRow is the body row of one of a file's expand controls: the zone by
// its index in the file, and whether it is the whole-gap control or the step.
func controlRow(t *testing.T, d *Diff, zone int, whole bool) int {
	t.Helper()
	for i, l := range d.lines {
		if l.line == boxExpandRow && l.zone == zone && l.whole == whole {
			return i
		}
	}
	t.Fatalf("zone %d has no %v control on the body", zone, whole)
	return 0
}

// TestBuildZonesMeasuresEveryGap pins what the gaps of a two-hunk change are:
// the lines before the first hunk, the lines between the two, and the lines
// after the last — that last one measured against the file, since the diff says
// nothing about how much of it follows the change.
func TestBuildZonesMeasuresEveryGap(t *testing.T) {
	f := git.ParseFiles(zonesDiff)[0]
	zones := buildZones(f, zonesSource())
	want := []expandZone{
		{at: 4, kind: zoneUp, first: 1, last: 19},
		{at: 9, kind: zoneUp, first: 23, last: 39},
		{at: 14, kind: zoneDown, first: 43, last: zonesLines},
	}
	if len(zones) != len(want) {
		t.Fatalf("built %d zones, want %d", len(zones), len(want))
	}
	for i, z := range zones {
		if z != want[i] {
			t.Errorf("zone %d = %+v, want %+v", i, z, want[i])
		}
	}
}

// TestBuildZonesNumbersTheBaseSide covers a change whose two sides are different
// lengths: the lines after it sit at one number on the branch and another in the
// base, and the gutter has to say both.
func TestBuildZonesNumbersTheBaseSide(t *testing.T) {
	diff := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n" +
		"@@ -5,1 +5,3 @@\n-was 5\n+line 5\n+line 6\n+line 7\n"
	zones := buildZones(git.ParseFiles(diff)[0], zonesSource())
	if len(zones) != 2 {
		t.Fatalf("built %d zones, want the gap before the hunk and the one after", len(zones))
	}
	if got := zones[0]; got.delta != 0 {
		t.Errorf("the gap above the first hunk = %+v, want the two sides in step", got)
	}
	// The hunk left the base at line 5 and the branch at line 7, so every line
	// below it is two further down on the branch than it is in the base.
	after := zones[1]
	if after.kind != zoneDown || after.first != 8 || after.delta != -2 {
		t.Errorf("the gap after the hunk = %+v, want it to start at 8, two behind in the base", after)
	}
}

// TestBuildZonesWithoutAHunk covers a file git described rather than diffed:
// there is no change for a gap to be around, so there is nothing to expand.
func TestBuildZonesWithoutAHunk(t *testing.T) {
	f := git.ParseFiles("diff --git a/p.png b/p.png\nBinary files a/p.png and b/p.png differ\n")[0]
	if zones := buildZones(f, zonesSource()); zones != nil {
		t.Errorf("buildZones() = %+v, want no gaps around a file with no hunk", zones)
	}
}

// TestBuildZonesHoldsAGapInsideTheFile covers a diff and a file that disagree —
// a branch that moved between the two reads — and a hunk that reaches the end of
// the file, which has no gap after it at all.
func TestBuildZonesHoldsAGapInsideTheFile(t *testing.T) {
	diff := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -3,2 +3,2 @@\n-was 3\n+line 3\n line 4\n"
	zones := buildZones(git.ParseFiles(diff)[0], []string{"line 1", "line 2", "line 3", "line 4"})
	if len(zones) != 1 {
		t.Fatalf("built %+v, want only the gap above the hunk", zones)
	}
	if zones[0].first != 1 || zones[0].last != 2 {
		t.Errorf("the gap = %+v, want the two lines above the hunk", zones[0])
	}
}

// TestBuildZonesAroundAHunkThatCoversNothing covers a hunk that only removes:
// its side of the branch holds no line at all, and git writes the line it sits
// after, so the gap above ends there and the gap below starts on the next.
func TestBuildZonesAroundAHunkThatCoversNothing(t *testing.T) {
	diff := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -21,1 +20,0 @@\n-was 21\n"
	zones := buildZones(git.ParseFiles(diff)[0], zonesSource())
	if len(zones) != 2 {
		t.Fatalf("built %+v, want a gap on either side", zones)
	}
	if zones[0].last != 20 {
		t.Errorf("the gap above ends at %d, want the line the hunk sits after", zones[0].last)
	}
	if zones[1].first != 21 {
		t.Errorf("the gap below starts at %d, want the line after it", zones[1].first)
	}
}

// TestBuildZonesRefusesAHeaderItCannotRead covers the hunk headers no gap can be
// measured from: one with a side missing, one with a side that is not numbers,
// and one that is not a header at all. A header the numbering does not trust is
// one the gaps do not either, and the diff is drawn as it was before there were
// zones.
func TestBuildZonesRefusesAHeaderItCannotRead(t *testing.T) {
	for _, header := range []string{"@@ -20,3", "@@ what is this @@", "@@ -20,x +20,3 @@"} {
		diff := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n" +
			header + "\n line 20\n-was 21\n+line 21\n"
		if zones := buildZones(git.ParseFiles(diff)[0], zonesSource()); zones != nil {
			t.Errorf("%q built %+v, want no gaps measured from a header git did not write",
				header, zones)
		}
	}
}

// TestBuildZonesReadsASideWithNoCount covers git's shorthand for a hunk of one
// line — "+20" rather than "+20,1" — which is the whole of the header on a
// one-line change.
func TestBuildZonesReadsASideWithNoCount(t *testing.T) {
	diff := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -20 +20 @@\n-was 20\n+line 20\n"
	zones := buildZones(git.ParseFiles(diff)[0], zonesSource())
	if len(zones) != 2 || zones[0].last != 19 || zones[1].first != 21 {
		t.Errorf("built %+v, want the hunk read as the one line it covers", zones)
	}
}

// TestDiffWrapsARevealedLine covers a revealed line too wide for the box: it
// takes as many rows as it needs, exactly as a line of the diff does, and only
// the row it starts on carries its numbers.
func TestDiffWrapsARevealedLine(t *testing.T) {
	src := zonesSource()
	src[19] = "line 20 " + strings.Repeat("wide ", 20)
	diff := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -21,1 +21,1 @@\n-was 21\n+line 21\n"
	d := NewDiff(DefaultStyles())
	d.SetSize(diffTestWidth, 40)
	d.SetFiles("origin/main", git.ParseFiles(diff), map[string][]string{"x.go": src})
	d.setLine(controlRow(t, &d, 0, true))
	d.Update(keyPress("enter"))

	rows := strings.Split(body(&d), "\n")
	at := d.rowOf(bodyLine{file: 0, line: boxContextRow, zone: 0}, -1)
	if at < 0 {
		t.Fatal("the gap drew no revealed line at all")
	}
	first := at + 19
	if !strings.Contains(rows[first], "20 20  line 20 wide") {
		t.Fatalf("row %d = %q, want the wide line numbered", first, rows[first])
	}
	if got := rows[first+1]; !strings.Contains(got, "wide") || strings.Contains(got, "20 20") {
		t.Errorf("the continuation row = %q, want the tail of the line and no numbers", got)
	}
}

// TestDiffDrawsTheExpandControls pins the whole of what a gap is drawn as: a
// step control saying what one press would bring, the whole-gap control under it
// saying what the gap holds, and the arrow pointing the way the lines will come
// — down for the gap after the last hunk, which has no hunk below it to reveal
// towards.
func TestDiffDrawsTheExpandControls(t *testing.T) {
	d := newZonesDiff(t)
	got := body(d)
	for _, want := range []string{
		"↑ (+15 lines)", "↑↑ (+19 lines)", // the 19 lines above the first hunk
		"↑↑ (+17 lines)", // the 17 between the hunks
		"↓ (+15 lines)", "↓↓ (+18 lines)", // the 18 after the last
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the body does not offer %q:\n%s", want, got)
		}
	}
}

// TestDiffOffersTheWholeGapOnlyWhenAStepWillNotDoIt covers a gap of fewer lines
// than a step: one control is the whole of it, and a second saying the same
// number would be the same offer twice.
func TestDiffOffersTheWholeGapOnlyWhenAStepWillNotDoIt(t *testing.T) {
	diff := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -4,1 +4,1 @@\n-was 4\n+line 4\n"
	d := NewDiff(DefaultStyles())
	d.SetSize(80, 40)
	d.SetFiles("origin/main", git.ParseFiles(diff), map[string][]string{"x.go": zonesSource()[:4]})
	got := body(&d)
	if !strings.Contains(got, "↑ (+3 lines)") {
		t.Errorf("the body does not offer the three lines above the hunk:\n%s", got)
	}
	if strings.Contains(got, "↑↑") {
		t.Errorf("a gap a step finishes should offer no second control:\n%s", got)
	}
}

// TestDiffExpandsAStepAtATime covers the key on a control: the lines it offered
// come on screen numbered on both sides and with no ± marker of their own, the
// control stays where it was saying what is left, and the whole-gap control goes
// once a step would finish the gap.
func TestDiffExpandsAStepAtATime(t *testing.T) {
	d := newZonesDiff(t)
	d.setLine(controlRow(t, d, 0, false))

	d.Update(keyPress("enter"))
	if got := d.zones[0][0]; got.hidden() != 4 || got.shown() != expandStep {
		t.Fatalf("after one step the gap is %+v, want fifteen of its nineteen lines revealed", got)
	}
	got := body(d)
	if !strings.Contains(got, "  5  5  line 5") {
		t.Errorf("the revealed lines are not numbered on both sides:\n%s", got)
	}
	if !strings.Contains(got, "↑ (+4 lines)") || strings.Contains(got, "↑↑ (+4") {
		t.Errorf("the control does not offer the four lines left, alone:\n%s", got)
	}
	if !d.onExpandRow() {
		t.Error("the cursor should still be on the control it activated")
	}

	d.Update(keyPress("enter"))
	if got := d.zones[0][0]; got.hidden() != 0 || got.shown() != 19 {
		t.Fatalf("after the second step the gap is %+v, want the whole of it revealed", got)
	}
	if got := body(d); strings.Contains(got, "↑ (+4") || !strings.Contains(got, "  1  1  line 1") {
		t.Errorf("a filled gap should draw its lines and no control:\n%s", got)
	}
}

// TestDiffExpandsTheWholeGap covers the doubled arrow: every line of the gap at
// once, and the control gone with it.
func TestDiffExpandsTheWholeGap(t *testing.T) {
	d := newZonesDiff(t)
	d.setLine(controlRow(t, d, 1, true))
	d.Update(keyPress("enter"))

	if got := d.zones[0][1]; got.hidden() != 0 || got.shown() != 17 {
		t.Fatalf("the gap is %+v, want the whole of it revealed at once", got)
	}
	got := body(d)
	for _, want := range []string{" 23 23  line 23", " 39 39  line 39"} {
		if !strings.Contains(got, want) {
			t.Errorf("the body does not hold %q:\n%s", want, got)
		}
	}
}

// TestDiffExpandsTheGapAfterTheLastHunkDownwards covers the one zone that
// reveals away from its hunk: the lines come in from the top of the gap, so the
// first press shows what follows the change rather than the end of the file.
func TestDiffExpandsTheGapAfterTheLastHunkDownwards(t *testing.T) {
	d := newZonesDiff(t)
	d.setLine(controlRow(t, d, 2, false))
	d.Update(keyPress("enter"))

	z := d.zones[0][2]
	if z.shownFirst != 43 || z.shownLast != 57 || z.first != 58 {
		t.Fatalf("the gap is %+v, want the fifteen lines after the change", z)
	}
	got := body(d)
	if !strings.Contains(got, " 43 43  line 43") || strings.Contains(got, "line 58") {
		t.Errorf("the body should open on line 43, not the end of the file:\n%s", got)
	}
	// The control is drawn under what it has revealed, since that is the end the
	// next press takes from.
	rows := strings.Split(got, "\n")
	if !strings.Contains(rows[controlRow(t, d, 2, false)-1], "line 57") {
		t.Errorf("the control should sit under the lines it revealed:\n%s", got)
	}
}

// TestDiffExpandsOnAClick covers the other way a control is activated, and that
// a click acts where it landed rather than where the cursor was.
func TestDiffExpandsOnAClick(t *testing.T) {
	app := diffZonesApp(t)
	row := controlRow(t, &app.diff, 0, false)
	clickDiff(app, 4, row)

	if got := app.diff.zones[0][0].shown(); got != expandStep {
		t.Errorf("%d lines revealed, want a step of them", got)
	}
	if app.diff.line != row {
		t.Errorf("the cursor is on row %d, want the control that was clicked at %d", app.diff.line, row)
	}
}

// TestDiffClickAwayFromAControl covers a click on a line of the diff while there
// are controls on the body: it expands nothing, exactly as it folds nothing.
func TestDiffClickAwayFromAControl(t *testing.T) {
	app := diffZonesApp(t)
	clickDiff(app, 4, app.diff.offsets[0]+2)
	if got := app.diff.zones[0][0].shown(); got != 0 {
		t.Errorf("%d lines revealed by a click on a line of the diff, want none", got)
	}
}

// TestDiffJumpsStayRightAsABoxGrows covers what expanding does to everything
// numbered against the body: the file jumps land on the box they name, and the
// line cursor lands inside it, however many lines have been let in above.
func TestDiffJumpsStayRightAsABoxGrows(t *testing.T) {
	d := NewDiff(DefaultStyles())
	// Short enough that the body is longer than the band, so a jump really has
	// somewhere to scroll to.
	d.SetSize(80, 10)
	second := strings.ReplaceAll(zonesDiff, zonesPath, "internal/tui/other.go")
	d.SetFiles("origin/main", git.ParseFiles(zonesDiff+second), map[string][]string{
		zonesPath: zonesSource(), "internal/tui/other.go": zonesSource(),
	})

	d.setLine(controlRow(t, &d, 0, true))
	d.Update(keyPress("enter"))

	d.jump(1)
	if d.cursor != 1 {
		t.Fatalf("the jump landed on file %d, want the second", d.cursor)
	}
	if d.vp.YOffset() != d.tops[1] || d.lines[d.tops[1]].file != 1 {
		t.Errorf("the jump scrolled to row %d, want the second box's header at %d",
			d.vp.YOffset(), d.tops[1])
	}
	if at := d.lines[d.line]; at.file != 1 || d.line != d.offsets[1] {
		t.Errorf("the cursor is at %+v, want the top of the second file", at)
	}
	// The rows the first box grew by are all inside it: nothing of the second
	// file is drawn above its own header row.
	for i := d.tops[1]; i < len(d.lines); i++ {
		if d.lines[i].file != 1 {
			t.Fatalf("row %d of the second box belongs to file %d", i, d.lines[i].file)
		}
	}
}

// TestDiffCursorWalksOverRevealedLines covers what the line cursor makes of a
// filled gap: a revealed line is the file's own rather than the diff's, so there
// is nothing on one to comment on and the cursor steps over it to the lines
// there are.
func TestDiffCursorWalksOverRevealedLines(t *testing.T) {
	d := newZonesDiff(t)
	d.setLine(controlRow(t, d, 0, false))
	d.Update(keyPress("enter"))

	d.moveCursor(1)
	at := d.lines[d.line]
	if at.line < 0 {
		t.Fatalf("the cursor landed on %+v, want a line of the diff", at)
	}
	if d.line != d.rowOf(bodyLine{file: 0, line: firstShown(d.files[0])}, -1) {
		t.Errorf("the cursor landed at row %d, want the first line of the diff", d.line)
	}
	if _, _, _, _, ok := d.Selection(); !ok {
		t.Error("the line the cursor landed on should be one a comment can go on")
	}
	// And back onto the control, which is the one row that is no line at all and
	// still a place to stop.
	d.moveCursor(-1)
	if !d.onExpandRow() {
		t.Errorf("the cursor moved back to %+v, want the control above the gap", d.lines[d.line])
	}
	if _, _, _, _, ok := d.Selection(); ok {
		t.Error("a control is no line, so there should be nothing to comment on")
	}
	if d.ToggleSelect() {
		t.Error("a control is no end of a range either")
	}
}

// TestDiffHintNamesWhatTheKeyWillDo covers the one key that does three things:
// the hints row says which of them the row under the cursor is for.
func TestDiffHintNamesWhatTheKeyWillDo(t *testing.T) {
	d := newZonesDiff(t)
	d.setLine(controlRow(t, d, 0, false))
	if got := d.viewedBinding().Help().Desc; got != "expand lines" {
		t.Errorf("on a control the key reads %q, want it to offer the lines", got)
	}
	d.setLine(rowAt(d, 0, firstShown(d.files[0])))
	if got := d.viewedBinding().Help().Desc; got != "collapse file" {
		t.Errorf("on a line the key reads %q, want the fold", got)
	}
}

// TestDiffFoldStillWorksOverAZone covers the key away from a control: it is the
// fold it always was, and folding a file leaves what has been expanded inside it
// alone — the lines are still the file's, and opening the box shows them again.
func TestDiffFoldStillWorksOverAZone(t *testing.T) {
	d := newZonesDiff(t)
	d.setLine(controlRow(t, d, 0, true))
	d.Update(keyPress("enter"))
	d.setLine(d.offsets[0])

	d.Update(keyPress("enter"))
	if !d.viewedFile(0) {
		t.Fatal("the key on a line of the diff should fold the file")
	}
	if got := body(d); strings.Contains(got, "line 1 ") {
		t.Errorf("a folded file should draw none of its lines:\n%s", got)
	}
	d.Update(keyPress("enter"))
	if got := body(d); !strings.Contains(got, "  1  1  line 1") {
		t.Errorf("opening the file again should draw what was expanded:\n%s", got)
	}
}

// TestDiffDropsExpansionsOnAFreshRead covers what a second read of the branch
// does to the gaps: it measures its own, since the gap a zone stood over is a
// fact about the diff that came back.
func TestDiffDropsExpansionsOnAFreshRead(t *testing.T) {
	d := newZonesDiff(t)
	d.setLine(controlRow(t, d, 0, true))
	d.Update(keyPress("enter"))

	d.SetFiles("origin/main", git.ParseFiles(zonesDiff),
		map[string][]string{zonesPath: zonesSource()})
	if got := d.zones[0][0].shown(); got != 0 {
		t.Errorf("%d lines still revealed after a fresh read, want the gaps measured again", got)
	}
}

// TestDiffWithoutTheFilesBehindIt covers a read whose files git would not show:
// the diff is drawn exactly as it was before there were zones, hunk breaks and
// all.
func TestDiffWithoutTheFilesBehindIt(t *testing.T) {
	d := NewDiff(DefaultStyles())
	d.SetSize(80, 40)
	d.SetFiles("origin/main", git.ParseFiles(zonesDiff), nil)
	got := body(&d)
	if strings.Contains(got, "↑") || strings.Contains(got, "↓") {
		t.Errorf("a diff with no files behind it should offer no expanding:\n%s", got)
	}
	if !strings.Contains(got, boxBreakRule) {
		t.Errorf("the break between the two hunks should still be drawn:\n%s", got)
	}
}

// TestExpandAtIgnoresEverythingElse covers the rows that are not controls and
// the zones that are not there: neither is an expansion, and neither panics.
func TestExpandAtIgnoresEverythingElse(t *testing.T) {
	d := newZonesDiff(t)
	for _, row := range []int{-1, len(d.lines), d.tops[0], rowAt(d, 0, firstShown(d.files[0]))} {
		if d.ExpandAt(row) {
			t.Errorf("ExpandAt(%d) = true, want only a control activated", row)
		}
	}
	d.expandZoneBy(9, 0, false)
	d.expandZoneBy(0, 9, false)
	if got := d.zones[0][0].shown(); got != 0 {
		t.Errorf("%d lines revealed by a zone that is not there, want none", got)
	}
	// A control the render is about to drop is activated once and no more: there
	// is nothing left in the gap to take.
	d.expandZoneBy(0, 0, true)
	d.expandZoneBy(0, 0, true)
	if got := d.zones[0][0]; got.shown() != 19 || got.hidden() != 0 {
		t.Errorf("the gap = %+v, want it revealed once and left alone", got)
	}
}
