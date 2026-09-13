package domain

import "testing"

func TestAgentPresenceString(t *testing.T) {
	tests := []struct {
		presence AgentPresence
		want     string
	}{
		{AgentNone, "none"},
		{AgentUnknown, "unknown"},
		{AgentWorking, "working"},
		{AgentWaiting, "waiting"},
		{AgentPresence(42), "none"},
	}
	for _, tt := range tests {
		if got := tt.presence.String(); got != tt.want {
			t.Errorf("AgentPresence(%d).String() = %q, want %q", tt.presence, got, tt.want)
		}
	}
}

func TestSliceStateString(t *testing.T) {
	tests := []struct {
		state SliceState
		want  string
	}{
		{SliceStateNone, "none"},
		{SliceStateWorking, "working"},
		{SliceStateWaiting, "waiting"},
		{SliceStateBlocked, "blocked"},
		{SliceStateReadyToPush, "ready to push"},
		{SliceStateAwaitingReview, "awaiting review"},
		{SliceStateReadyToMerge, "ready to merge"},
		{SliceState(42), "none"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("SliceState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestPRReadinessString(t *testing.T) {
	tests := []struct {
		readiness PRReadiness
		want      string
	}{
		{PRUnread, "unread"},
		{PRAwaitingReview, "awaiting review"},
		{PRReadyToMerge, "ready to merge"},
		{PRReadiness(42), "unread"},
	}
	for _, tt := range tests {
		if got := tt.readiness.String(); got != tt.want {
			t.Errorf("PRReadiness(%d).String() = %q, want %q", tt.readiness, got, tt.want)
		}
	}
}

// TestStateOf walks every combination of the facts a state is derived from: the
// four readings of the agent, a branch or none, a PR or none, and a dependency
// that is unfinished or none — for a slice in progress, and then for the
// statuses that are in no flight at all. The reading of the pull request is
// left at its zero value here, which is every board before gh has been asked
// anything; TestStateOfPRReadiness is what walks the rest of it.
//
// StateOf short-circuits on the first fact that applies, so most combinations
// of the remaining fields are never read at all and add nothing as separate
// cases: presence != AgentNone is one branch regardless of whether presence is
// AgentUnknown or AgentWorking, blocked is never read once branch or PR is
// set, and no field but status is read once status != SliceClaimed. Each
// block below keeps only the cases that exercise a distinct branch or a
// distinct override of one branch by the one before it in StateOf's switch.
func TestStateOf(t *testing.T) {
	plan := []Slice{
		{ID: "dep", Name: "Notion client", Status: SliceTodo, StatusName: "Todo"},
	}
	byID := SlicesByID(plan)

	tests := []struct {
		name     string
		status   SliceStatus
		presence AgentPresence
		branch   string
		pr       string
		blocked  bool
		want     SliceState
	}{
		// No agent, nothing out, nothing waited on: the work has to be got out.
		{name: "in progress, alone", status: SliceClaimed, want: SliceStateReadyToPush},
		// No agent, nothing out, a dependency unfinished.
		{name: "in progress, blocked", status: SliceClaimed, blocked: true, want: SliceStateBlocked},
		// No agent, work out: a branch or a PR, the two distinct OR operands
		// that reach the same review-pending case. A combination of the two,
		// or either one with blocked also set, is not read any differently.
		{name: "handed back", status: SliceClaimed, branch: "slice/x", want: SliceStateAwaitingReview},
		{name: "PR recorded", status: SliceClaimed, pr: "https://gh/pr/1", want: SliceStateAwaitingReview},

		// A live agent — unclassified or working, which StateOf treats alike
		// via presence != AgentNone — wins over everything else on the page.
		// One bare case per presence value, plus one case showing a live agent
		// overrides a handed-back branch and one showing it overrides
		// blocked; unclassified and working short-circuit identically, so
		// only one of the two needs the overrides exercised.
		{name: "unclassified agent", status: SliceClaimed, presence: AgentUnknown, want: SliceStateWorking},
		{name: "working agent", status: SliceClaimed, presence: AgentWorking, want: SliceStateWorking},
		{name: "working agent, handed back", status: SliceClaimed, presence: AgentWorking, branch: "slice/x", want: SliceStateWorking},
		{name: "working agent, blocked", status: SliceClaimed, presence: AgentWorking, blocked: true, want: SliceStateWorking},

		// An agent that has stopped for input is the one thing louder than a
		// working one, and it too wins over everything on the page.
		{name: "waiting agent", status: SliceClaimed, presence: AgentWaiting, want: SliceStateWaiting},
		{name: "waiting agent, handed back", status: SliceClaimed, presence: AgentWaiting, branch: "slice/x", want: SliceStateWaiting},
		{name: "waiting agent, blocked", status: SliceClaimed, presence: AgentWaiting, blocked: true, want: SliceStateWaiting},

		// Nothing that is not in progress is in flight, however loaded the
		// page — status is read first and the function returns before any
		// other field is, so a Todo row with blocked or an agent set would add
		// nothing beyond the bare "todo" case below.
		{name: "todo", status: SliceTodo, want: SliceStateNone},
		{name: "done", status: SliceDone, branch: "slice/x", pr: "https://gh/pr/1", want: SliceStateNone},
		{name: "done with an agent still on it", status: SliceDone, presence: AgentWaiting, want: SliceStateNone},
		{name: "a status nobody knows", status: SliceStatus("Parked"), presence: AgentWorking, want: SliceStateNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Slice{ID: "s1", Name: "Board", Status: tt.status, Branch: tt.branch, PRURL: tt.pr}
			if tt.blocked {
				s.DependsOn = []string{"dep"}
			}
			if got := StateOf(s, tt.presence, PRUnread, byID); got != tt.want {
				t.Errorf("StateOf() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestStateOfPRReadiness is the refinement gh's reading makes: a pull request
// approved and mergeable is a review that is over, and everything else — an
// unreviewed one, and one nothing could be read of at all — is a review still
// to come. It applies to work that is out and to nothing else: a slice with an
// agent on it, one with nothing pushed, a Done slice, and one that is not in
// flight at all are what they were whatever GitHub says about a pull request
// they do not have — or, for a Done slice, whatever it says at all, since
// Done is in no state no matter what pr reads as.
func TestStateOfPRReadiness(t *testing.T) {
	byID := SlicesByID(nil)

	tests := []struct {
		name      string
		status    SliceStatus
		presence  AgentPresence
		branch    string
		prURL     string
		readiness PRReadiness
		want      SliceState
	}{
		{name: "approved and mergeable", status: SliceClaimed, prURL: "https://gh/pr/1",
			readiness: PRReadyToMerge, want: SliceStateReadyToMerge},
		{name: "read and still waiting", status: SliceClaimed, prURL: "https://gh/pr/1",
			readiness: PRAwaitingReview, want: SliceStateAwaitingReview},
		{name: "nothing read", status: SliceClaimed, prURL: "https://gh/pr/1",
			readiness: PRUnread, want: SliceStateAwaitingReview},
		// A branch handed back with no pull request on it yet is a review still
		// to come whatever a stale reading says.
		{name: "handed back, ready to merge", status: SliceClaimed, branch: "slice/x",
			readiness: PRReadyToMerge, want: SliceStateReadyToMerge},
		// An agent back on the branch is the review going back to it, which
		// wins over anything GitHub says.
		{name: "agent on it", status: SliceClaimed, presence: AgentWorking, prURL: "https://gh/pr/1",
			readiness: PRReadyToMerge, want: SliceStateWorking},
		{name: "nothing out", status: SliceClaimed, readiness: PRReadyToMerge, want: SliceStateReadyToPush},

		// A Done slice is in no state at all whatever pr says — Notion's status
		// is the one source of lifecycle truth now, and nothing here second-
		// guesses it. A slice Done under the old rule with its pull request
		// still open is not this function's problem: a positive reading writes
		// it back to In progress on the page (actions.ReopenUnmerged), and from
		// there it is an ordinary in-progress slice again.
		{name: "done, ready to merge", status: SliceDone, prURL: "https://gh/pr/1",
			readiness: PRReadyToMerge, want: SliceStateNone},
		{name: "done, awaiting review", status: SliceDone, prURL: "https://gh/pr/1",
			readiness: PRAwaitingReview, want: SliceStateNone},
		{name: "done, nothing read", status: SliceDone, prURL: "https://gh/pr/1",
			readiness: PRUnread, want: SliceStateNone},
		{name: "done, agent on it", status: SliceDone, presence: AgentWorking, branch: "slice/x",
			prURL: "https://gh/pr/1", readiness: PRAwaitingReview, want: SliceStateNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Slice{ID: "s1", Name: "Board", Status: tt.status, Branch: tt.branch, PRURL: tt.prURL}
			if got := StateOf(s, tt.presence, tt.readiness, byID); got != tt.want {
				t.Errorf("StateOf() = %v, want %v", got, tt.want)
			}
		})
	}
}
