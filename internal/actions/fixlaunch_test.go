package actions

import (
	"errors"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
)

// TestFixLaunch: a pull request recorded makes a launch a fix, approved and in
// progress or Done; with none recorded, or on a slice not yet under way, it is
// an ordinary launch.
func TestFixLaunch(t *testing.T) {
	const pr = "https://example/pr/1"
	tests := []struct {
		slice domain.Slice
		want  bool
	}{
		{domain.Slice{Status: domain.SliceClaimed, PRURL: pr}, true},
		{domain.Slice{Status: domain.SliceDone, PRURL: pr}, true},
		{domain.Slice{Status: domain.SliceClaimed}, false},
		{domain.Slice{Status: domain.SliceDone}, false},
		{domain.Slice{Status: domain.SliceTodo, PRURL: pr}, false},
	}
	for _, tt := range tests {
		if got := FixLaunch(tt.slice); got != tt.want {
			t.Errorf("FixLaunch(%s, pr %q) = %v, want %v", tt.slice.Status, tt.slice.PRURL, got, tt.want)
		}
	}
}

// TestPRStillOpen asks gh in the slice's checkout about the recorded pull
// request, lets an open one through, and refuses a merged, a closed and an
// unread one, each in its own words.
func TestPRStillOpen(t *testing.T) {
	s := domain.Slice{Name: "Domain model", PRURL: "https://example/pr/1"}
	tests := []struct {
		name  string
		state string
		err   error
		toast string
		sev   Severity
		ok    bool
	}{
		{name: "open", state: "OPEN", sev: SevSuccess, ok: true},
		{name: "merged", state: gh.PRStateMerged, sev: SevWarning,
			toast: `The pull request for "Domain model" has already merged — no agent was launched.`},
		{name: "closed", state: gh.PRStateClosed, sev: SevWarning,
			toast: `The pull request for "Domain model" is closed — no agent was launched.`},
		{name: "unreadable", err: errors.New("no pull requests found"), sev: SevError,
			toast: `Could not read the pull request for "Domain model": no pull requests found — no agent was launched.`},
	}
	for _, tt := range tests {
		viewer := &fakeViewer{pr: gh.PR{State: tt.state}, err: tt.err}
		toast, sev, ok := PRStillOpen(viewer, "/repo", s)
		if toast != tt.toast || sev != tt.sev || ok != tt.ok {
			t.Errorf("%s: PRStillOpen = %q, %v, %v, want %q, %v, %v", tt.name, toast, sev, ok, tt.toast, tt.sev, tt.ok)
		}
		if len(viewer.viewed) != 1 || viewer.viewed[0] != "/repo https://example/pr/1" {
			t.Errorf("%s: viewed %v, want the recorded pull request read in the checkout", tt.name, viewer.viewed)
		}
	}
}
