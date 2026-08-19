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
		{SliceState(42), "none"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("SliceState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

// TestStateOf walks every combination of the facts a state is derived from: the
// four readings of the agent, a branch or none, a PR or none, and a dependency
// that is unfinished or none — for a slice in progress, and then for the
// statuses that are in no flight at all.
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
		// No agent, work out: a branch, a PR, or both — blocked or not, the
		// review is what there is to do.
		{name: "handed back", status: SliceClaimed, branch: "slice/x", want: SliceStateAwaitingReview},
		{name: "handed back, blocked", status: SliceClaimed, branch: "slice/x", blocked: true, want: SliceStateAwaitingReview},
		{name: "PR recorded", status: SliceClaimed, pr: "https://gh/pr/1", want: SliceStateAwaitingReview},
		{name: "PR recorded, blocked", status: SliceClaimed, pr: "https://gh/pr/1", blocked: true, want: SliceStateAwaitingReview},
		{name: "branch and PR", status: SliceClaimed, branch: "slice/x", pr: "https://gh/pr/1", want: SliceStateAwaitingReview},
		{name: "branch and PR, blocked", status: SliceClaimed, branch: "slice/x", pr: "https://gh/pr/1", blocked: true, want: SliceStateAwaitingReview},

		// A live agent nobody has classified is running, so it is working —
		// whatever else the page records.
		{name: "unclassified agent", status: SliceClaimed, presence: AgentUnknown, want: SliceStateWorking},
		{name: "unclassified agent, blocked", status: SliceClaimed, presence: AgentUnknown, blocked: true, want: SliceStateWorking},
		{name: "unclassified agent, handed back", status: SliceClaimed, presence: AgentUnknown, branch: "slice/x", want: SliceStateWorking},
		{name: "unclassified agent, handed back, blocked", status: SliceClaimed, presence: AgentUnknown, branch: "slice/x", blocked: true, want: SliceStateWorking},
		{name: "unclassified agent, PR", status: SliceClaimed, presence: AgentUnknown, pr: "https://gh/pr/1", want: SliceStateWorking},
		{name: "unclassified agent, PR, blocked", status: SliceClaimed, presence: AgentUnknown, pr: "https://gh/pr/1", blocked: true, want: SliceStateWorking},
		{name: "unclassified agent, branch and PR", status: SliceClaimed, presence: AgentUnknown, branch: "slice/x", pr: "https://gh/pr/1", want: SliceStateWorking},
		{name: "unclassified agent, branch and PR, blocked", status: SliceClaimed, presence: AgentUnknown, branch: "slice/x", pr: "https://gh/pr/1", blocked: true, want: SliceStateWorking},

		{name: "working agent", status: SliceClaimed, presence: AgentWorking, want: SliceStateWorking},
		{name: "working agent, blocked", status: SliceClaimed, presence: AgentWorking, blocked: true, want: SliceStateWorking},
		{name: "working agent, handed back", status: SliceClaimed, presence: AgentWorking, branch: "slice/x", want: SliceStateWorking},
		{name: "working agent, handed back, blocked", status: SliceClaimed, presence: AgentWorking, branch: "slice/x", blocked: true, want: SliceStateWorking},
		{name: "working agent, PR", status: SliceClaimed, presence: AgentWorking, pr: "https://gh/pr/1", want: SliceStateWorking},
		{name: "working agent, PR, blocked", status: SliceClaimed, presence: AgentWorking, pr: "https://gh/pr/1", blocked: true, want: SliceStateWorking},
		{name: "working agent, branch and PR", status: SliceClaimed, presence: AgentWorking, branch: "slice/x", pr: "https://gh/pr/1", want: SliceStateWorking},
		{name: "working agent, branch and PR, blocked", status: SliceClaimed, presence: AgentWorking, branch: "slice/x", pr: "https://gh/pr/1", blocked: true, want: SliceStateWorking},

		// An agent that has stopped for input is the one thing louder than a
		// working one, and it too wins over everything on the page.
		{name: "waiting agent", status: SliceClaimed, presence: AgentWaiting, want: SliceStateWaiting},
		{name: "waiting agent, blocked", status: SliceClaimed, presence: AgentWaiting, blocked: true, want: SliceStateWaiting},
		{name: "waiting agent, handed back", status: SliceClaimed, presence: AgentWaiting, branch: "slice/x", want: SliceStateWaiting},
		{name: "waiting agent, handed back, blocked", status: SliceClaimed, presence: AgentWaiting, branch: "slice/x", blocked: true, want: SliceStateWaiting},
		{name: "waiting agent, PR", status: SliceClaimed, presence: AgentWaiting, pr: "https://gh/pr/1", want: SliceStateWaiting},
		{name: "waiting agent, PR, blocked", status: SliceClaimed, presence: AgentWaiting, pr: "https://gh/pr/1", blocked: true, want: SliceStateWaiting},
		{name: "waiting agent, branch and PR", status: SliceClaimed, presence: AgentWaiting, branch: "slice/x", pr: "https://gh/pr/1", want: SliceStateWaiting},
		{name: "waiting agent, branch and PR, blocked", status: SliceClaimed, presence: AgentWaiting, branch: "slice/x", pr: "https://gh/pr/1", blocked: true, want: SliceStateWaiting},

		// Nothing that is not in progress is in flight, however loaded the page.
		{name: "todo", status: SliceTodo, want: SliceStateNone},
		{name: "todo, blocked", status: SliceTodo, blocked: true, want: SliceStateNone},
		{name: "todo with an agent on it", status: SliceTodo, presence: AgentWorking, want: SliceStateNone},
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
			if got := StateOf(s, tt.presence, byID); got != tt.want {
				t.Errorf("StateOf() = %v, want %v", got, tt.want)
			}
		})
	}
}
