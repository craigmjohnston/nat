package actions

import (
	"fmt"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
)

// FixLaunch reports whether launching on a slice is a fix session rather than
// work on the slice itself: a pull request is recorded on it, so its work is
// out — approved, in progress until it merges, or Done under the old rule that
// wrote Done at approve — and what is left of it is the review of that pull
// request: comments to answer and checks to get green.
//
// It is the one discriminator: the board's l key and headless slice-launch
// both ask it, so the two can never disagree about which launches are fixes.
// Whether that pull request is still open is deliberately not part of it —
// only gh can say, and that is asked once a launch is under way, see
// [PRStillOpen].
func FixLaunch(s domain.Slice) bool {
	return s.PRURL != "" && (s.Status == domain.SliceClaimed || s.Status == domain.SliceDone)
}

// PRStillOpen is a fix launch's one question: does GitHub still call the
// slice's pull request open? It is read in dir, the slice's checkout, at the
// moment of the launch — first of everything, before any worktree is cut —
// rather than taken from a background listing, which is a poll and may not
// have run since the pull request merged.
//
// Only an open one is worth an agent. A merged pull request is the work landed
// and a closed one is the work given up on, and a session started at either
// would be a worktree cut and a Claude Code launched at a review nobody is
// waiting on. The two are refused separately because what to do next differs:
// one slice is finished and the other holds a decision to revisit.
//
// A read that failed refuses too, which is the opposite of what nat does
// everywhere else it reads gh — there an unread pull request is no news,
// because what it costs is a chip left undrawn. What it costs here is an agent
// sent at a pull request that may have merged an hour ago, so the reading that
// never happened stops the launch and says so.
//
// It answers with a message and how loudly to show it rather than a Go error,
// for the reason a worktree failure does: nothing has gone wrong with nat, and
// the slice is exactly as it was.
func PRStillOpen(viewer PRViewer, dir string, s domain.Slice) (string, Severity, bool) {
	pr, err := viewer.ViewPR(dir, s.PRURL)
	switch {
	case err != nil:
		return fmt.Sprintf("Could not read the pull request for %q: %v — no agent was launched.", s.Name, err), SevError, false
	case pr.State == gh.PRStateMerged:
		return fmt.Sprintf("The pull request for %q has already merged — no agent was launched.", s.Name), SevWarning, false
	case pr.State == gh.PRStateClosed:
		return fmt.Sprintf("The pull request for %q is closed — no agent was launched.", s.Name), SevWarning, false
	}
	return "", SevSuccess, true
}
