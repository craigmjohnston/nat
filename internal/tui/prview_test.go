package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/gh"
)

// readyPRView is the screen with a pull request on it, at a size worth drawing
// at.
func readyPRView(pr gh.PR) PRView {
	p := NewPRView(DefaultStyles())
	p.SetSize(80, 20)
	p.Start("s1", "PR screen", pr.URL, "/repo")
	p.SetPR(pr)
	return p
}

// TestPRViewStateChip covers the four words the header names a pull request's
// state in, and which of them each reading takes.
func TestPRViewStateChip(t *testing.T) {
	tests := []struct {
		name string
		pr   gh.PR
		want string
	}{
		{"open", gh.PR{State: "OPEN"}, "open"},
		{"draft", gh.PR{State: "OPEN", IsDraft: true}, "draft"},
		{"merged", gh.PR{State: "MERGED"}, "merged"},
		{"closed", gh.PR{State: "CLOSED"}, "closed"},
		{"merged draft", gh.PR{State: "MERGED", IsDraft: true}, "merged"},
		{"closed draft", gh.PR{State: "CLOSED", IsDraft: true}, "closed"},
		{"a word this build does not know", gh.PR{State: "QUEUED"}, "open"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			word, _ := prStateChip(DefaultStyles(), tt.pr)
			if word != tt.want {
				t.Errorf("chip = %q, want %q", word, tt.want)
			}
			if got := readyPRView(tt.pr).View(""); !strings.Contains(got, tt.want) {
				t.Errorf("view = %q, want the state named %q", got, tt.want)
			}
		})
	}
}

// TestPRViewStartsEmpty covers the screen before it has been pointed at
// anything: it says what to press, and there is nothing to read again.
func TestPRViewStartsEmpty(t *testing.T) {
	p := NewPRView(DefaultStyles())
	if p.Busy() || p.Loadable() || p.Number() != 0 {
		t.Error("a fresh screen holds no pull request")
	}
	if got := p.View(""); !strings.Contains(got, "press V") {
		t.Errorf("view = %q, want the key named", got)
	}
}

// TestPRViewTargetAndSlice covers what the refresh key reads back off the
// screen: the slice it is showing, the ref gh is given and where it is run.
func TestPRViewTargetAndSlice(t *testing.T) {
	p := readyPRView(samplePR())
	slice, ref, dir := p.Target()
	if slice != "PR screen" || ref != samplePR().URL || dir != "/repo" {
		t.Errorf("target = %q %q %q, want what the screen was pointed at", slice, ref, dir)
	}
	if p.SliceID() != "s1" {
		t.Errorf("slice = %q, want the page the pull request is recorded on", p.SliceID())
	}
	if !p.Loadable() {
		t.Error("a screen pointed at a pull request can be read again")
	}
}

// TestPRViewDescription covers the body: markdown rendered through glamour, and
// a pull request opened with nothing said about it drawn as such rather than as
// an empty band.
func TestPRViewDescription(t *testing.T) {
	pr := samplePR()
	pr.Body = "# Heading\n\nsome prose"
	// Each word is a run of its own once glamour has styled it, so the check is
	// per word rather than over the sentence.
	if got := readyPRView(pr).View(""); !strings.Contains(got, "Heading") ||
		!strings.Contains(got, "prose") {
		t.Errorf("view = %q, want the description rendered", got)
	}

	pr.Body = "   \n  "
	if got := readyPRView(pr).View(""); !strings.Contains(got, "no description") {
		t.Errorf("view = %q, want the empty description said out loud", got)
	}
}

// TestPRViewResizeRerenders covers the wrap: glamour renders to a fixed width,
// so a narrower screen is a re-render rather than a reflow.
func TestPRViewResizeRerenders(t *testing.T) {
	pr := samplePR()
	pr.Body = strings.Repeat("a long sentence about the change ", 8)
	p := readyPRView(pr)
	wide := p.vp.GetContent()
	p.SetSize(40, 20)
	if narrow := p.vp.GetContent(); narrow == wide {
		t.Error("the description should be rendered again at the new width")
	}
}

// TestPRViewFailDropsWhatWasOnScreen covers a read that did not come back: the
// failure is the screen's own state, and the pull request it replaces goes with
// it rather than being left up as though it were still current.
func TestPRViewFailDropsWhatWasOnScreen(t *testing.T) {
	p := readyPRView(samplePR())
	p.Fail(errors.New("gh: could not find the pull request\nand a second line"))
	if p.Number() != 0 {
		t.Error("a failed read leaves no pull request on the screen")
	}
	got := p.View("")
	if !strings.Contains(got, "could not find") {
		t.Errorf("view = %q, want gh's own refusal", got)
	}
	if strings.Contains(got, "second line") {
		t.Errorf("view = %q, want one line of it", got)
	}
}

// TestPRViewStartAndReset cover the two ways the screen is emptied: pointed at
// a fresh read, and dropped altogether.
func TestPRViewStartAndReset(t *testing.T) {
	p := readyPRView(samplePR())
	p.Start("s2", "Another", "https://github.test/craig/nat/pull/13", "/elsewhere")
	if !p.Busy() || p.Number() != 0 {
		t.Error("a read in flight holds no pull request")
	}

	p.SetPR(samplePR())
	p.Reset()
	if p.Loadable() || p.Number() != 0 || p.Busy() {
		t.Error("a reset screen holds nothing at all")
	}
	if got := p.View(""); !strings.Contains(got, "press V") {
		t.Errorf("view = %q, want the empty screen back", got)
	}
}
