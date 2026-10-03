package shortcut

import (
	"encoding/json"
	"time"
)

// The Shortcut v3 resources the plugin reads, cut to the fields it uses.
// Decoding is lenient on purpose: unknown fields are ignored, every field may
// be absent or null (a plain string or number left at zero), and a time that
// won't parse is the zero time. A field Shortcut renames therefore degrades
// to an empty fact, never to a failed call.

// MemberInfo is GET /member: who the token belongs to.
type MemberInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MentionName string `json:"mention_name"`
	Workspace2  struct {
		URLSlug string `json:"url_slug"`
	} `json:"workspace2"`
}

// Member is one entry of GET /members.
type Member struct {
	ID      string `json:"id"`
	Profile struct {
		Name        string `json:"name"`
		MentionName string `json:"mention_name"`
	} `json:"profile"`
}

// Workflow is one entry of GET /workflows.
type Workflow struct {
	ID     int64           `json:"id"`
	Name   string          `json:"name"`
	States []WorkflowState `json:"states"`
}

// The workflow state types. Which states count as started or done is read
// from these, never from a state's name.
const (
	StateBacklog   = "backlog"
	StateUnstarted = "unstarted"
	StateStarted   = "started"
	StateDone      = "done"
)

// WorkflowState is one column of a workflow.
type WorkflowState struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Position int64  `json:"position"`
}

// Group is a Shortcut team (GET /groups).
type Group struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MentionName string `json:"mention_name"`
	// Color is a hex override, usually null; ColorKey names a colour from
	// Shortcut's palette ("midnight-blue").
	Color    string `json:"color"`
	ColorKey string `json:"color_key"`
	Archived bool   `json:"archived"`
}

// Project is one entry of GET /projects — the story's project, whose
// abbreviation and colour are a story's badge.
type Project struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
	Color        string `json:"color"`
	Archived     bool   `json:"archived"`
}

// Epic is GET /epics/{id}, and one entry of the slim list (GET /epics with
// includes_description=false), cut to what a badge, fact or filter needs.
type Epic struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	GroupID  string `json:"group_id"`
	Archived bool   `json:"archived"`
}

// Iteration is GET /iterations/{id}.
type Iteration struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Label is a label as a story carries it, and one entry of GET /labels.
type Label struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	Archived bool   `json:"archived"`
}

// Story is GET /stories/{id}, and (with the detail-only fields empty) one
// search result.
type Story struct {
	ID              int64    `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	AppURL          string   `json:"app_url"`
	StoryType       string   `json:"story_type"`
	WorkflowID      int64    `json:"workflow_id"`
	WorkflowStateID int64    `json:"workflow_state_id"`
	Estimate        *int64   `json:"estimate"`
	EpicID          int64    `json:"epic_id"`
	ProjectID       int64    `json:"project_id"`
	GroupID         string   `json:"group_id"`
	IterationID     int64    `json:"iteration_id"`
	OwnerIDs        []string `json:"owner_ids"`
	FollowerIDs     []string `json:"follower_ids"`
	RequestedByID   string   `json:"requested_by_id"`
	Labels          []Label  `json:"labels"`
	Position        int64    `json:"position"`
	CreatedAt       Time     `json:"created_at"`
	UpdatedAt       Time     `json:"updated_at"`
	CompletedAt     Time     `json:"completed_at"`

	Comments      []Comment     `json:"comments"`
	Tasks         []Task        `json:"tasks"`
	Branches      []Branch      `json:"branches"`
	PullRequests  []PullRequest `json:"pull_requests"`
	StoryLinks    []StoryLink   `json:"story_links"`
	ExternalLinks []string      `json:"external_links"`
}

// Comment is one comment on a story.
type Comment struct {
	ID        int64  `json:"id"`
	AuthorID  string `json:"author_id"`
	Text      string `json:"text"`
	CreatedAt Time   `json:"created_at"`
	Deleted   bool   `json:"deleted"`
}

// Task is a story's checklist item — what a nat task is mirrored as.
type Task struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	Complete    bool   `json:"complete"`
	Position    int64  `json:"position"`
}

// Branch is a VCS branch linked to a story.
type Branch struct {
	ID           int64         `json:"id"`
	Name         string        `json:"name"`
	URL          string        `json:"url"`
	Merged       bool          `json:"merged"`
	Deleted      bool          `json:"deleted"`
	PullRequests []PullRequest `json:"pull_requests"`
}

// PullRequest is a VCS pull request linked to a story.
type PullRequest struct {
	ID     int64  `json:"id"`
	Number int64  `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Merged bool   `json:"merged"`
	Closed bool   `json:"closed"`
	Draft  bool   `json:"draft"`
}

// StoryLink is a story-to-story relation as one side of it sees it: Type is
// "subject" when this story is the subject of Verb, "object" when the other
// story is.
type StoryLink struct {
	ID        int64  `json:"id"`
	SubjectID int64  `json:"subject_id"`
	ObjectID  int64  `json:"object_id"`
	Verb      string `json:"verb"`
	Type      string `json:"type"`
}

// SearchResult is one page of GET /search/stories. Next is the path of the
// next page, "" on the last.
type SearchResult struct {
	Data  []Story `json:"data"`
	Next  string  `json:"next"`
	Total int     `json:"total"`
}

// Time is a timestamp that decodes leniently: null, absent, a non-string or
// an unparseable string all read as the zero time rather than failing the
// response they sit in.
type Time struct{ time.Time }

// UnmarshalJSON implements json.Unmarshaler.
func (t *Time) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) != nil {
		t.Time = time.Time{}
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		parsed = time.Time{}
	}
	t.Time = parsed
	return nil
}

// MarshalJSON implements json.Marshaler, writing the zero time as null.
func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.Format(time.RFC3339Nano))
}
