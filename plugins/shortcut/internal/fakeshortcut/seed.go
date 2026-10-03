package fakeshortcut

import (
	"time"

	"github.com/craigmjohnston/nat/plugins/shortcut/internal/shortcut"
)

// IDs in the seeded workspace, for tests to name.
const (
	MeID    = "11111111-0000-4000-8000-000000000001"
	DanaID  = "11111111-0000-4000-8000-000000000002"
	PriyaID = "11111111-0000-4000-8000-000000000003"

	TeamNative = "22222222-0000-4000-8000-000000000001"
	TeamBoard  = "22222222-0000-4000-8000-000000000002"
	TeamOld    = "22222222-0000-4000-8000-000000000003"

	WorkflowEng       int64 = 500
	StateBacklog      int64 = 501
	StateReady        int64 = 502
	StateInDev        int64 = 503
	StateInReview     int64 = 504
	StateDone         int64 = 505
	StateWontDo       int64 = 506
	EpicParity        int64 = 10
	EpicOld           int64 = 11
	IterationSprint41 int64 = 41

	// Shortcut projects: Mobile App with an abbreviation and a colour, Web
	// with neither, Legacy archived.
	ProjectMobile int64 = 30
	ProjectWeb    int64 = 31
	ProjectLegacy int64 = 32

	// StoryDoing is started and mine; StoryReady unstarted and mine;
	// StoryBug unstarted and nobody's; StoryDone done and mine, last week;
	// StoryDoneThisWeek done and mine, this week.
	StoryDoing        int64 = 4821
	StoryReady        int64 = 4802
	StoryBug          int64 = 4811
	StoryDone         int64 = 4756
	StoryDoneThisWeek int64 = 4760
)

// SeedNow is the moment the seeded timestamps are relative to: a Saturday,
// so this week began on Monday 28 September.
var SeedNow = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

func at(d time.Duration) shortcut.Time { return shortcut.Time{Time: SeedNow.Add(-d)} }

func member(id, name, mention string) shortcut.Member {
	var m shortcut.Member
	m.ID, m.Profile.Name, m.Profile.MentionName = id, name, mention
	return m
}

// Seed returns a scratch workspace: one workflow (Backlog, Ready for Dev, In
// Development, In Review, Done, Won't Do), two teams (and an archived one),
// three Shortcut projects (one archived), two epics (one archived), three
// labels (one archived), an iteration and five stories — one per sidebar
// group, an unowned bug, and a done story from last week beside this week's —
// every one of them invented.
func Seed(token string) *Server {
	est := func(n int64) *int64 { return &n }
	s := &Server{
		Token: token,
		Members: []shortcut.Member{
			member(MeID, "Craig Scratch", "craig"),
			member(DanaID, "Dana Wolfe", "dana"),
			member(PriyaID, "Priya Deol", "priya"),
		},
		Workflows: []shortcut.Workflow{{ID: WorkflowEng, Name: "Engineering", States: []shortcut.WorkflowState{
			// Out of position order on purpose: the plugin must sort.
			{ID: StateInReview, Name: "In Review", Type: shortcut.StateStarted, Position: 4},
			{ID: StateInDev, Name: "In Development", Type: shortcut.StateStarted, Position: 3},
			{ID: StateBacklog, Name: "Backlog", Type: shortcut.StateBacklog, Position: 1},
			{ID: StateReady, Name: "Ready for Dev", Type: shortcut.StateUnstarted, Position: 2},
			{ID: StateWontDo, Name: "Won't Do", Type: shortcut.StateDone, Position: 6},
			{ID: StateDone, Name: "Done", Type: shortcut.StateDone, Position: 5},
		}}},
		Groups: []shortcut.Group{
			// As the live API has them: color null, color_key a name.
			{ID: TeamNative, Name: "Native App", MentionName: "native-app", ColorKey: "midnight-blue"},
			{ID: TeamBoard, Name: "Board", MentionName: "board", ColorKey: "turquoise"},
			{ID: TeamOld, Name: "Old Team", MentionName: "old", ColorKey: "red", Archived: true},
		},
		Projects: []shortcut.Project{
			// As the live API has them: the colour a hex, possibly upper case.
			{ID: ProjectMobile, Name: "Mobile App", Abbreviation: "MOB", Color: "#E5732A"},
			{ID: ProjectWeb, Name: "Web"},
			{ID: ProjectLegacy, Name: "Legacy", Abbreviation: "LEG", Color: "#123456", Archived: true},
		},
		Labels: []shortcut.Label{
			{ID: 1, Name: "diff", Color: "#d64545"},
			{ID: 2, Name: "agent"},
			{ID: 3, Name: "old", Archived: true},
		},
		Epics: []shortcut.Epic{
			{ID: EpicParity, Name: "Native app parity", GroupID: TeamBoard},
			{ID: EpicOld, Name: "Old epic", Archived: true},
		},
		Iterations: []shortcut.Iteration{{ID: IterationSprint41, Name: "Sprint 41"}},
		Stories: map[int64]*shortcut.Story{
			StoryDoing: {
				ID: StoryDoing, Name: "Improve diff review ergonomics", AppURL: "https://app.shortcut.com/scratch/story/4821",
				Description: "Comments left on a diff never reach the agent.\n\n**Acceptance:** they do, within a second.",
				StoryType:   "feature", WorkflowID: WorkflowEng, WorkflowStateID: StateInDev, Estimate: est(3),
				EpicID: EpicParity, ProjectID: ProjectMobile, GroupID: TeamNative, IterationID: IterationSprint41,
				OwnerIDs: []string{MeID}, RequestedByID: DanaID,
				Labels:   []shortcut.Label{{ID: 1, Name: "diff"}, {ID: 2, Name: "agent"}},
				Position: 20, CreatedAt: at(15 * 24 * time.Hour), UpdatedAt: at(2 * time.Hour),
				Comments: []shortcut.Comment{
					{ID: 2, AuthorID: MeID, Text: "Agreed. Splitting into three tasks.", CreatedAt: at(2 * 24 * time.Hour)},
					{ID: 1, AuthorID: DanaID, Text: "Pairs with the syntax highlighting work.", CreatedAt: at(3 * 24 * time.Hour)},
					{ID: 3, AuthorID: DanaID, Text: "(removed)", CreatedAt: at(time.Hour), Deleted: true},
				},
				PullRequests: []shortcut.PullRequest{{ID: 418, Number: 418, Title: "Diff comments reach the agent", URL: "https://github.com/scratch/app/pull/418"}},
				Branches: []shortcut.Branch{
					{ID: 1, Name: "slice/diff-comments", URL: "https://github.com/scratch/app/tree/slice/diff-comments",
						PullRequests: []shortcut.PullRequest{{ID: 418, Number: 418, Title: "Diff comments reach the agent", URL: "https://github.com/scratch/app/pull/418"},
							{ID: 416, Number: 416, Title: "Syntax highlighting", URL: "https://github.com/scratch/app/pull/416", Merged: true}}},
					{ID: 2, Name: "old-spike", URL: "https://github.com/scratch/app/tree/old-spike", Merged: true},
				},
				StoryLinks: []shortcut.StoryLink{
					{ID: 1, SubjectID: StoryDoing, ObjectID: StoryReady, Verb: "blocks", Type: "subject"},
					{ID: 2, SubjectID: StoryBug, ObjectID: StoryDoing, Verb: "blocks", Type: "object"},
					{ID: 3, SubjectID: StoryDoing, ObjectID: StoryDone, Verb: "relates to", Type: "subject"},
				},
				ExternalLinks: []string{"https://www.figma.com/file/scratch/review-pane"},
			},
			StoryReady: {
				ID: StoryReady, Name: "Kanban column view", AppURL: "https://app.shortcut.com/scratch/story/4802",
				Description: "Show milestones as columns.", StoryType: "feature", WorkflowID: WorkflowEng, WorkflowStateID: StateReady,
				Estimate: est(1), GroupID: TeamBoard, OwnerIDs: []string{MeID}, RequestedByID: PriyaID, Position: 10,
				CreatedAt: at(19 * 24 * time.Hour), UpdatedAt: at(4 * 24 * time.Hour),
			},
			StoryBug: {
				ID: StoryBug, Name: "Wheel scrolling in the Active panel", AppURL: "https://app.shortcut.com/scratch/story/4811",
				StoryType: "bug", WorkflowID: WorkflowEng, WorkflowStateID: StateReady, EpicID: EpicParity, ProjectID: ProjectWeb, Position: 5,
				CreatedAt: at(17 * 24 * time.Hour), UpdatedAt: at(6 * 24 * time.Hour),
			},
			StoryDone: {
				ID: StoryDone, Name: "Pluggable plan storage", AppURL: "https://app.shortcut.com/scratch/story/4756",
				StoryType: "chore", WorkflowID: WorkflowEng, WorkflowStateID: StateDone, Estimate: est(5), GroupID: TeamNative,
				OwnerIDs: []string{MeID}, Position: 1, CreatedAt: at(400 * 24 * time.Hour), UpdatedAt: at(7 * 24 * time.Hour),
				CompletedAt: at(7 * 24 * time.Hour),
			},
			StoryDoneThisWeek: {
				ID: StoryDoneThisWeek, Name: "Ship the release notes", AppURL: "https://app.shortcut.com/scratch/story/4760",
				StoryType: "chore", WorkflowID: WorkflowEng, WorkflowStateID: StateDone, Estimate: est(1), ProjectID: ProjectMobile,
				OwnerIDs: []string{MeID}, Position: 2, CreatedAt: at(9 * 24 * time.Hour), UpdatedAt: at(2 * time.Hour),
				CompletedAt: at(2 * time.Hour),
			},
		},
		Now: func() time.Time { return SeedNow },
	}
	s.Me.ID, s.Me.Name, s.Me.MentionName = MeID, "Craig Scratch", "craig"
	s.Me.Workspace2.URLSlug = "scratch"
	return s
}
