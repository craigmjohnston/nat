// Package source talks to task sources: external `nat-source-<name>` binaries
// that let a tracker other than nat's own plan — Shortcut, say — show up as a
// project whose work is done through the ordinary slice flow.
//
// The split is fixed. nat owns the tasks: a source project's slices live in a
// Local SQLite plan like any other, and are claimed, launched, handed back and
// merged exactly as one. The plugin owns the containers those tasks hang off —
// its cards, stories, tickets — and everything about how they are grouped and
// shown: the sidebar tree, a container's facts and prose, a few named actions.
// It is told what happened to a task through events, and does what it likes
// with them.
//
// The wire is one JSON request on the plugin's stdin and one JSON response on
// its stdout, per call: `nat-source-<name> <method>`. A non-zero exit is an
// error, worded by the first line the plugin wrote to stderr. The protocol is
// specified in docs/design/task-sources/README.md; the types here mirror it
// field for field.
//
// Request and response bodies are never logged. A container's body is someone's
// ticket — free text from another system, which may carry anything — and the
// log file is not a place it was ever agreed to go. Only the method, the
// plugin's name, the ids involved and an exit code are. That matters most for
// setup, whose input is a credential.
package source

import "context"

// ProtocolVersion is the protocol this build speaks. A plugin's describe
// answers with the version it speaks, and any other number is refused rather
// than half-understood.
const ProtocolVersion = 1

// Project is the nat project a request is about, sent on every call. The
// plugin keys its own settings and credentials off the ID; nat stores neither.
type Project struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	WorkingDir string `json:"working_dir"`
}

// Describe is what a plugin says about itself: who it is, how it is drawn,
// and what its containers and tasks are called.
type Describe struct {
	Protocol      int          `json:"protocol"`
	Name          string       `json:"name"`
	Title         string       `json:"title"`
	Tag           string       `json:"tag"`
	IconSymbol    string       `json:"icon_symbol"`
	IconSVG       string       `json:"icon_svg,omitempty"`
	ContainerNoun string       `json:"container_noun"`
	TaskNoun      string       `json:"task_noun"`
	Menu          []Action     `json:"menu,omitempty"`
	Setup         []SetupField `json:"setup,omitempty"`
}

// SetupField is one thing a plugin needs set before it works — a token, a
// workspace — drawn by gnat as a field in Settings and sent back through a
// setup request. nat relays it without understanding it: the value goes to
// the plugin on stdin and nowhere else. Set, where the plugin says, is whether
// it holds a value for the field now — a presence check, never the value —
// and nil where it does not say.
type SetupField struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Input string `json:"input"`
	Hint  string `json:"hint,omitempty"`
	Set   *bool  `json:"set,omitempty"`
}

// Group is one fold of the plugin's sidebar tree. A lazy group carries only
// its count until it is named in a sidebar request's expand list.
type Group struct {
	ID         string      `json:"id"`
	Label      string      `json:"label"`
	Count      *int        `json:"count,omitempty"`
	Lazy       bool        `json:"lazy,omitempty"`
	Menu       []Action    `json:"menu,omitempty"`
	Children   []Group     `json:"children,omitempty"`
	Containers []Container `json:"containers,omitempty"`
}

// Sidebar is a sidebar response: the tree, and — where the plugin sends one —
// the section header's menu for this project, which replaces describe's own
// static one. A filter's options and selection are per project, which a
// describe answered about no project cannot carry.
type Sidebar struct {
	Groups []Group  `json:"groups"`
	Menu   []Action `json:"menu,omitempty"`
}

// Container is one row of the sidebar tree: the thing a task hangs off.
type Container struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	ExternalURL string   `json:"external_url,omitempty"`
	Badges      []Badge  `json:"badges,omitempty"`
	Meta        string   `json:"meta,omitempty"`
	Menu        []Action `json:"menu,omitempty"`
}

// Badge is a short coloured tag drawn on a container's row.
type Badge struct {
	Text  string `json:"text"`
	Color string `json:"color"`
	Title string `json:"title,omitempty"`
}

// The input an [Action] asks for before it runs. InputSecret is a
// [SetupField]'s alone — drawn masked — and never valid on an action.
// InputFilter asks for a selection in each of the action's Fields, and
// sends them back as a JSON object of field id to option ids.
const (
	InputNone   = "none"
	InputText   = "text"
	InputChoice = "choice"
	InputSecret = "secret"
	InputFilter = "filter"
)

// Action is a named thing the plugin can do, offered on a menu. Its ID is
// what comes back in an action request; Options are the choices an
// [InputChoice] offers, Fields what an [InputFilter] edits.
type Action struct {
	ID          string        `json:"id"`
	Label       string        `json:"label"`
	Input       string        `json:"input"`
	Options     []string      `json:"options,omitempty"`
	Fields      []FilterField `json:"fields,omitempty"`
	Destructive bool          `json:"destructive,omitempty"`
}

// FilterField is one field of an [InputFilter] action: a choice among
// Options — one, or several where Multi — with Value the current selection,
// which the plugin sends afresh on every response, so the editor always opens
// on what is saved. An empty Value is "any" — or, where Inherited names
// something, falls through to that: a wider filter's own choice for the
// field, which the editor shows beside "Any" so an override reads as one.
// Loading says the plugin is still fetching the options in the background:
// the editor draws the field as loading and reads the tree once more, and
// never holds the other fields for it.
type FilterField struct {
	ID        string         `json:"id"`
	Label     string         `json:"label"`
	Multi     bool           `json:"multi,omitempty"`
	Options   []FilterOption `json:"options"`
	Value     []string       `json:"value"`
	Inherited string         `json:"inherited,omitempty"`
	Loading   bool           `json:"loading,omitempty"`
}

// FilterOption is one choice of a [FilterField]; Color, `#rrggbb`, tints it.
type FilterOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Color string `json:"color,omitempty"`
}

// ContainerDetail is everything the plugin shows about one container: its
// facts, its sections, its menu, and a note to put beside the PR of any task
// under it.
type ContainerDetail struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	ExternalURL string    `json:"external_url,omitempty"`
	Facts       []Fact    `json:"facts"`
	Sections    []Section `json:"sections"`
	Menu        []Action  `json:"menu,omitempty"`
	TaskNote    string    `json:"task_note,omitempty"`
}

// Fact is one label/value line of a container's facts list. Badge, where
// set, is drawn before the value in place of Color's dot — a fact naming
// what one of the container's badges stands for (a Shortcut card's
// project), drawn as that badge.
type Fact struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Color string `json:"color,omitempty"`
	Badge *Badge `json:"badge,omitempty"`
}

// The kinds of [Section]: which of its fields it is drawn from.
const (
	KindProse    = "prose"
	KindComments = "comments"
	KindLinks    = "links"
)

// Section is one block of a container's detail. Its Kind says which of Body,
// Comments or Links it carries; Composer, when set, is the action a reply
// written beneath it is sent through.
type Section struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Kind     string    `json:"kind"`
	Body     string    `json:"body,omitempty"`
	Comments []Comment `json:"comments,omitempty"`
	Links    []Link    `json:"links,omitempty"`
	Composer *Action   `json:"composer,omitempty"`
}

// Comment is one entry of a comments section.
type Comment struct {
	By   string `json:"by"`
	When string `json:"when"`
	Text string `json:"text"`
}

// Link is one entry of a links section — a PR with its state, or a document.
type Link struct {
	Label string `json:"label"`
	Text  string `json:"text"`
	State string `json:"state,omitempty"`
	URL   string `json:"url"`
}

// Target is what an action is run against: a group, a container, or neither
// (an action off the source's own menu).
type Target struct {
	Group     string `json:"group,omitempty"`
	Container string `json:"container,omitempty"`
}

// ActionResult is a plugin's answer to an action: a message worth showing,
// or nothing.
type ActionResult struct {
	Message string `json:"message,omitempty"`
}

// Task is the nat slice an event is about, as the plugin is told of it.
type Task struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Branch string `json:"branch"`
	PR     string `json:"pr"`
}

// The events a plugin is told of, each sent after nat's own write has landed.
const (
	EventCreated    = "created"
	EventClaimed    = "claimed"
	EventReleased   = "released"
	EventHandedBack = "handed_back"
	EventApproved   = "approved"
	EventMerged     = "merged"
	EventDeleted    = "deleted"
)

// Client is a task source, one method per protocol call. [Exec] is the real
// one; [Fake] stands in for it in tests.
type Client interface {
	Describe(ctx context.Context, p Project) (Describe, error)
	Sidebar(ctx context.Context, p Project, expand []string) (Sidebar, error)
	Container(ctx context.Context, p Project, id string) (ContainerDetail, error)
	Action(ctx context.Context, p Project, action string, target Target, input string) (ActionResult, error)
	Event(ctx context.Context, p Project, container string, task Task, event string) error
	Setup(ctx context.Context, id, input string) (string, error)
}
