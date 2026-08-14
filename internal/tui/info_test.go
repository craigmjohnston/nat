package tui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/craigmjohnston/nat/internal/notion"
)

// infoWidth and infoHeight are the size the info tests render at: wide enough
// for glamour to wrap into, short enough that a fixture overflows and can be
// scrolled.
const (
	infoWidth  = 60
	infoHeight = 6
)

// fixtureMarkdown is testdata/info-page.json — a small project page — put
// through the same blocks→markdown conversion the app fetches with.
func fixtureMarkdown(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "info-page.json"))
	if err != nil {
		t.Fatal(err)
	}
	var blocks []notion.Block
	if err := json.Unmarshal(raw, &blocks); err != nil {
		t.Fatal(err)
	}
	return notion.Markdown(blocks)
}

// newTestInfo returns an info screen showing the fixture page at a fixed size.
func newTestInfo(t *testing.T) *Info {
	t.Helper()
	i := NewInfo(DefaultStyles())
	i.SetSize(infoWidth, infoHeight)
	i.SetMarkdown(fixtureMarkdown(t))
	return &i
}

func TestInfoRendersTheProjectPage(t *testing.T) {
	golden(t, "info-page", newTestInfo(t).View(""))
}

func TestInfoRewrapsOnResize(t *testing.T) {
	i := newTestInfo(t)
	wide := i.vp.GetContent()

	i.SetSize(30, infoHeight)
	narrow := i.vp.GetContent()

	if narrow == wide {
		t.Fatal("a resize should re-render the body at the new width")
	}
	if got := longestLine(narrow); got > 30 {
		t.Errorf("longest line = %d cells, want it wrapped to 30", got)
	}
	if longestLine(wide) <= 30 {
		t.Error("the fixture should have been wider before the resize")
	}
}

// longestLine is the width of s's widest line, ignoring styling.
func longestLine(s string) int {
	var widest int
	for _, line := range strings.Split(stripANSI(s), "\n") {
		widest = max(widest, len([]rune(strings.TrimRight(line, " "))))
	}
	return widest
}

// stripANSI removes the escape sequences glamour and lipgloss decorate with —
// the colours, and the hyperlinks a PR chip carries — so a test can measure the
// text itself.
func stripANSI(s string) string { return xansi.Strip(s) }

func TestInfoScrolls(t *testing.T) {
	i := newTestInfo(t)
	if i.vp.TotalLineCount() <= i.vp.Height() {
		t.Fatal("the fixture should be taller than the viewport, or there is nothing to scroll")
	}

	for _, k := range []string{"j", "down"} {
		i.vp.GotoTop()
		i.Update(keyPress(k))
		if i.vp.YOffset() != 1 {
			t.Errorf("%q scrolled to %d, want 1", k, i.vp.YOffset())
		}
		i.Update(keyPress("k"))
		if i.vp.YOffset() != 0 {
			t.Errorf("k after %q scrolled to %d, want 0", k, i.vp.YOffset())
		}
	}

	i.vp.GotoTop()
	i.Update(keyPress("f"))
	if got := i.vp.YOffset(); got != i.vp.Height() {
		t.Errorf("f scrolled to %d, want a full page of %d", got, i.vp.Height())
	}
}

func TestInfoShowsAFreshPageFromTheTop(t *testing.T) {
	i := newTestInfo(t)
	i.vp.GotoBottom()

	i.SetMarkdown(fixtureMarkdown(t))

	if i.vp.YOffset() != 0 {
		t.Errorf("offset = %d, want a reloaded page shown from the top", i.vp.YOffset())
	}
}

func TestInfoStates(t *testing.T) {
	i := NewInfo(DefaultStyles())
	i.SetSize(infoWidth, infoHeight)

	if !i.Idle() || i.Busy() {
		t.Fatal("a new info screen has asked for nothing yet")
	}

	i.Start()
	if i.Idle() || !i.Busy() {
		t.Error("a fetch in flight is neither idle nor finished")
	}
	if got := i.View("⣾"); !strings.Contains(got, "⣾") || !strings.Contains(got, "Loading the project page") {
		t.Errorf("view = %q, want the spinner and the loading line", got)
	}

	i.SetMarkdown(fixtureMarkdown(t))
	if i.Idle() || i.Busy() {
		t.Error("a loaded page is neither idle nor loading")
	}

	// A failed refresh reports itself without dropping what was on show.
	i.Fail(errors.New("boom"))
	if i.Busy() {
		t.Error("a failed fetch is not still in flight")
	}
	if got := i.View(""); !strings.Contains(got, "boom") {
		t.Errorf("view = %q, want the error", got)
	}
	if i.markdown == "" {
		t.Error("a failed refresh should keep the page the user was reading")
	}

	// Reset puts it back to idle, so the next visit fetches again.
	i.Reset()
	if !i.Idle() {
		t.Error("a reset screen should fetch again")
	}
	if got := i.vp.GetContent(); got != "" {
		t.Errorf("content = %q, want it cleared", got)
	}
}

func TestInfoReportsAnEmptyPage(t *testing.T) {
	i := NewInfo(DefaultStyles())
	i.SetSize(infoWidth, infoHeight)
	i.SetMarkdown("")

	if got := i.View(""); !strings.Contains(got, "The project page is empty.") {
		t.Errorf("view = %q, want the empty-page line", got)
	}
}

func TestInfoSurvivesAZeroSizedWindow(t *testing.T) {
	i := NewInfo(DefaultStyles())
	i.SetSize(0, 0)
	i.SetMarkdown(fixtureMarkdown(t))

	if i.vp.Width() < 1 || i.vp.Height() < 1 {
		t.Errorf("viewport = %dx%d, want at least one cell each way", i.vp.Width(), i.vp.Height())
	}
	i.View("")
}

func TestRenderMarkdownFallsBackToTheSource(t *testing.T) {
	const md = "# Heading\n"
	if got := renderMarkdown(md, "no-such-style", infoWidth); got != md {
		t.Errorf("renderMarkdown = %q, want the unrendered markdown", got)
	}
	if got := renderMarkdown(md, DefaultGlamourStyle, infoWidth); got == md {
		t.Error("a style glamour knows should render, not fall back")
	}
}

func TestInfoKeysAreListed(t *testing.T) {
	for _, b := range infoKeys() {
		if h := b.Help(); h.Key == "" || h.Desc == "" {
			t.Errorf("binding %+v is not describable in the help", h)
		}
	}
}
