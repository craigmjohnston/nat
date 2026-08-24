package domain

// AgentPresence is what is known of the agent on a slice: whether there is a
// session on it at all and, if there is, how it is getting on. It is the board's
// two readings — the live map of running sessions and the activity watcher's
// classification of them — said as one value, so a rule about a slice in flight
// can be written without either of them being a map lookup.
//
// It is domain's own rather than a borrowed type: both readings are taken by
// packages that already import this one, so the enum they map onto has to live
// here for the mapping to be possible at all.
//
// The zero value is a slice with no session on it, which is every slice on a
// board that has launched nothing.
type AgentPresence int

const (
	// AgentNone is a slice with no session running on it.
	AgentNone AgentPresence = iota
	// AgentUnknown is a live agent whose screen has not been classified. It is
	// running, so it counts as working until a reading says otherwise.
	AgentUnknown
	// AgentWorking is a live agent getting on with the slice.
	AgentWorking
	// AgentWaiting is a live agent that has stopped and wants input.
	AgentWaiting
)

// String names the presence for logs and test failures.
func (a AgentPresence) String() string {
	switch a {
	case AgentUnknown:
		return "unknown"
	case AgentWorking:
		return "working"
	case AgentWaiting:
		return "waiting"
	default:
		return "none"
	}
}

// PRReadiness is what is known of a slice's pull request: whether anyone has
// looked at it yet, and whether it could be merged as it stands. It is the
// board's reading of the GitHub CLI said as one value, the way [AgentPresence]
// is the reading of tmux, so a rule about work that is out can be written
// without GitHub's own vocabulary reaching this far.
//
// The zero value is a slice with no pull request, and equally one whose pull
// request nothing has been read of — an unauthenticated gh, a repository that
// is not there, a network that is down. Both are the same to the rule below:
// there is a review to wait on until something says otherwise.
type PRReadiness int

const (
	// PRUnread is a slice with no pull request, or one nothing has been read of.
	PRUnread PRReadiness = iota
	// PRAwaitingReview is a pull request that has been read and is still
	// waiting: unreviewed, changes asked for, or approved but unmergeable.
	PRAwaitingReview
	// PRReadyToMerge is a pull request approved and mergeable as it stands.
	PRReadyToMerge
)

// String names the readiness for logs and test failures.
func (p PRReadiness) String() string {
	switch p {
	case PRAwaitingReview:
		return "awaiting review"
	case PRReadyToMerge:
		return "ready to merge"
	default:
		return "unread"
	}
}

// SliceState is where a slice in flight has got to: the one thing worth saying
// about a slice that is neither waiting to be started nor finished. The board
// holds the several facts it is derived from — a live agent and what that agent
// is doing, a branch handed back, a PR recorded, the slices it waits on — and
// this is what they add up to.
type SliceState int

const (
	// SliceStateNone is a slice that is not in flight at all: Todo, Done, or
	// carrying a status this app does not know. There is nothing in flight to
	// say anything about, so every other input is ignored.
	SliceStateNone SliceState = iota
	// SliceStateWorking is a slice with an agent on it, getting on with it.
	SliceStateWorking
	// SliceStateWaiting is a slice whose agent has stopped for input — a
	// permission prompt, a question, or the end of its turn.
	SliceStateWaiting
	// SliceStateBlocked is a slice with nothing running on it and nothing to
	// review, waiting on a dependency that is not Done.
	SliceStateBlocked
	// SliceStateReadyToPush is a slice in progress with no agent on it, nothing
	// handed back and nothing waited on: a session that ended without pushing
	// anything, and the one state the work still has to be got out of.
	SliceStateReadyToPush
	// SliceStateAwaitingReview is a slice whose work is out — a branch handed
	// back, or a PR recorded — and not being worked on.
	SliceStateAwaitingReview
	// SliceStateReadyToMerge is a slice whose pull request has been approved and
	// can be merged as it stands: the review is over and the work is one action
	// from landing.
	SliceStateReadyToMerge
)

// String names the state for logs and test failures.
func (s SliceState) String() string {
	switch s {
	case SliceStateWorking:
		return "working"
	case SliceStateWaiting:
		return "waiting"
	case SliceStateBlocked:
		return "blocked"
	case SliceStateReadyToPush:
		return "ready to push"
	case SliceStateAwaitingReview:
		return "awaiting review"
	case SliceStateReadyToMerge:
		return "ready to merge"
	default:
		return "none"
	}
}

// StateOf is where s has got to, given what is known of the agent on it and the
// plan its dependencies are read from. It is a pure function of what the board
// already holds — no tmux and no Notion — which is why it is here rather than
// beside either reading.
//
// The order the facts are tested in is the order they are true in. A slice that
// is not in progress is in no state at all, whatever else is recorded on it: a
// Todo slice waiting on a dependency is the board's own blocked row and a Done
// one has a PR by definition, and neither is in flight. Then a live agent wins
// over everything else on the page, because it is the only fact being taken
// fresh — an agent running on a handed-back branch is the review going back to
// it, and what it is doing now is the state. With no agent, work that is out —
// a branch, a PR — is what there is to do something about, and only a slice
// with none of that is read as blocked, since nothing is waiting on the
// dependency but the next session. What is left is a slice in progress that
// nothing is happening on and nothing has come out of.
//
// Work that is out is where pr comes in, and only there: a pull request read as
// approved and mergeable is the review over, and everything else about it — an
// unreviewed one, one with changes asked for, one nobody could read at all — is
// the review still to come, which is what a slice handed back on a branch alone
// is too.
func StateOf(s Slice, presence AgentPresence, pr PRReadiness, byID map[string]Slice) SliceState {
	switch {
	case s.Status != SliceClaimed:
		return SliceStateNone
	case presence == AgentWaiting:
		return SliceStateWaiting
	case presence != AgentNone:
		return SliceStateWorking
	case s.Branch != "" || s.PRURL != "":
		if pr == PRReadyToMerge {
			return SliceStateReadyToMerge
		}
		return SliceStateAwaitingReview
	case Blocked(s, byID):
		return SliceStateBlocked
	default:
		return SliceStateReadyToPush
	}
}
