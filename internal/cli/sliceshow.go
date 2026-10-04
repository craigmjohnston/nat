package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/source"
	"github.com/craigmjohnston/nat/internal/store"
)

// sliceShow reads and prints one slice in full, without claiming it. It is
// read-only: this is the read the app's Brief tab does, with the full status
// and its blocked computation.
func sliceShow(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-show", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-show: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-show", rest[0])
	if err != nil {
		return err
	}

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}

	shape, err := sliceShape(ctx, st, projectID, project)
	if err != nil {
		return err
	}
	s, _, err := loadSlice(ctx, st, id)
	if err != nil {
		return err
	}

	// Read the slice's dependencies.
	depByID := dependencyIndex(ctx, st, s)

	milestone := milestoneOf(s, shape.Milestones)
	brief, err := st.Body(ctx, s.ID)
	if err != nil {
		return fmt.Errorf("could not read the slice's brief: %w", err)
	}

	if *asJSON {
		var container *sliceContainerJSON
		if cr, ok := st.(store.ContainerReader); ok {
			container = sliceContainer(ctx, cr, s, milestone)
		}
		return writeSliceShowJSON(env.Out, s, milestone, project, depByID, brief, sliceBase(env, s, project), container)
	}
	return writeSliceShowMarkdown(env.Out, s, milestone, project, brief)
}

// sliceShowJSON is the full structured form of a single slice.
type sliceShowJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Status    string `json:"status"`
	Milestone string `json:"milestone"`
	Assignee  string `json:"assignee"`
	Branch    string `json:"branch,omitempty"`
	Repo      string `json:"repo,omitempty"`
	// Base is the branch a launch cuts the slice's worktree from, as
	// [git.CLI.Base] resolves it in the slice's repo — omitted with no repo
	// to ask.
	Base       string   `json:"base,omitempty"`
	PR         string   `json:"pr,omitempty"`
	DependsOn  []string `json:"depends_on,omitempty"`
	Blocked    bool     `json:"blocked"`
	HandedBack bool     `json:"handed_back"`
	// Fixing says a fix is under way, read off the record — see
	// [store.Fixing].
	Fixing bool   `json:"fixing"`
	State  string `json:"state,omitempty"`
	Brief  string `json:"brief"`
	// FollowUps are the follow-ups the slice's agent handed in that still
	// await the user's decision, each by the index slice-triage takes.
	FollowUps []followUpJSON `json:"followUps,omitempty"`
	// Visuals are the images the slice's agent last handed in of what it
	// changed, for the app's Visual changes section.
	Visuals []visualJSON `json:"visuals,omitempty"`
	// Events is the slice's whole task log: every store.TaskEvent its body
	// carries, in the order they were written, plus — read off the slice's
	// own properties rather than its body — an "approved" event where a pull
	// request is recorded and a "merged" event where the slice is Done with
	// a pull request or branch recorded. Always an array, even an empty one:
	// the app ranges over it with no nil check, so it is never omitted or
	// left null.
	Events []taskEventJSON `json:"events"`
	// Container is the plugin's container a source project's task hangs
	// off; absent for any other project.
	Container *sliceContainerJSON `json:"container,omitempty"`
}

// sliceContainerJSON is what a task's view needs of its container: enough to
// draw the brief's facts and the PR section's note, and to link out.
type sliceContainerJSON struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	ExternalURL string        `json:"external_url,omitempty"`
	TaskNote    string        `json:"task_note,omitempty"`
	Facts       []source.Fact `json:"facts,omitempty"`
}

// sliceContainer reads a source project's task's container from its plugin.
// A failed read concludes nothing: it is logged, and the container is given
// by what the plan knows of it — its id and cached title.
func sliceContainer(ctx context.Context, cr store.ContainerReader, s domain.Slice, m domain.Milestone) *sliceContainerJSON {
	d, err := cr.Container(ctx, s.MilestoneID)
	if err != nil {
		logging.Error("task source container unread for slice-show",
			"slice", s.ID, "container", s.MilestoneID, "err", err)
		return &sliceContainerJSON{ID: s.MilestoneID, Title: m.Name}
	}
	return &sliceContainerJSON{
		ID: s.MilestoneID, Title: d.Title, ExternalURL: d.ExternalURL, TaskNote: d.TaskNote, Facts: d.Facts,
	}
}

// taskEventJSON is one entry of a slice's task log, the wire form of
// [store.TaskEvent].
type taskEventJSON struct {
	Kind string `json:"kind"`
	Note string `json:"note,omitempty"`
	By   string `json:"by,omitempty"`
	// FromSlice is the slice a note came from, by name and milestone, where
	// its provenance names one — never resolved to an ID here, which would
	// mean reading the whole plan; the app matches it against the plan it
	// already holds.
	FromSlice *noteSourceJSON `json:"fromSlice,omitempty"`
	// At is when the event was written, RFC 3339 — omitted for one written
	// before sections were stamped, and for "approved" and "merged", which
	// are read off properties that record no time.
	At        string             `json:"at,omitempty"`
	PR        string             `json:"pr,omitempty"`
	FollowUps []taskFollowUpJSON `json:"followUps,omitempty"`
}

// noteSourceJSON is the wire form of [store.NoteSource]; milestone is omitted
// for a slice filed under none.
type noteSourceJSON struct {
	Name      string `json:"name"`
	Milestone string `json:"milestone,omitempty"`
}

// taskFollowUpJSON is one follow-up of a "follow_ups" event, the wire form of
// [store.TaskFollowUp]. decidedAt is RFC 3339, as an event's at, and
// omitted where no decision time was recorded.
type taskFollowUpJSON struct {
	Index     int    `json:"index"`
	Title     string `json:"title"`
	Brief     string `json:"brief"`
	Decision  string `json:"decision,omitempty"`
	Link      string `json:"link,omitempty"`
	DecidedAt string `json:"decidedAt,omitempty"`
}

// taskEventsJSON is a slice's whole task log: [store.TaskEvents]' own read of
// its body, plus the two events only its properties can answer — "approved"
// and "merged" are not sections of the body at all, since slice-approve
// records only a URL and the merge only a status, so they are folded in
// here rather than taught to store.TaskEvents, which knows only the body.
func taskEventsJSON(s domain.Slice, brief string) []taskEventJSON {
	events := store.TaskEvents(brief)
	out := make([]taskEventJSON, 0, len(events)+2)
	for _, e := range events {
		tj := taskEventJSON{Kind: e.Kind, Note: e.Note, By: e.By}
		if e.FromSlice != nil {
			tj.FromSlice = &noteSourceJSON{Name: e.FromSlice.Name, Milestone: e.FromSlice.Milestone}
		}
		if !e.At.IsZero() {
			tj.At = e.At.Format(time.RFC3339)
		}
		for _, f := range e.FollowUps {
			fj := taskFollowUpJSON{
				Index: f.Index, Title: f.Title, Brief: f.Brief, Decision: f.Decision, Link: f.Link,
			}
			if !f.DecidedAt.IsZero() {
				fj.DecidedAt = f.DecidedAt.Format(time.RFC3339)
			}
			tj.FollowUps = append(tj.FollowUps, fj)
		}
		out = append(out, tj)
	}
	if s.PRURL != "" {
		out = append(out, taskEventJSON{Kind: "approved", PR: s.PRURL})
	}
	if s.Status == domain.SliceDone && (s.PRURL != "" || s.Branch != "") {
		out = append(out, taskEventJSON{Kind: "merged"})
	}
	return out
}

// visualJSON is one handed-in image.
type visualJSON struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	URI   string `json:"uri"`
}

// followUpJSON is one pending follow-up.
type followUpJSON struct {
	Index int    `json:"index"`
	Title string `json:"title"`
	Brief string `json:"brief"`
}

// writeSliceShowJSON encodes the slice as JSON.
func writeSliceShowJSON(out io.Writer, s domain.Slice, m domain.Milestone, project config.ProjectConfig, depByID map[string]domain.Slice, brief, base string, container *sliceContainerJSON) error {
	// Compute state the same way info.go does.
	slicesByID := domain.SlicesByID([]domain.Slice{s})
	// Add dependencies to the index so blocking can be computed.
	for _, dep := range depByID {
		slicesByID[dep.ID] = dep
	}

	state := domain.StateOf(s, domain.AgentNone, domain.PRUnread, slicesByID)

	sj := sliceShowJSON{
		ID:         s.ID,
		Name:       s.Name,
		URL:        s.URL,
		Status:     s.StatusName,
		Milestone:  m.Name,
		Assignee:   s.AssigneeName,
		Branch:     s.Branch,
		Repo:       sliceRepo(s, project),
		Base:       base,
		PR:         s.PRURL,
		DependsOn:  s.DependsOn,
		Blocked:    domain.Blocked(s, slicesByID),
		HandedBack: s.HandedBack(),
		Fixing:     store.Fixing(s, brief),
		Brief:      brief,
		Events:     taskEventsJSON(s, brief),
		Container:  container,
	}
	if state != domain.SliceStateNone {
		sj.State = state.String()
	}
	for _, f := range store.PendingFollowUps(brief) {
		sj.FollowUps = append(sj.FollowUps, followUpJSON{Index: f.Index, Title: f.Title, Brief: f.Brief})
	}
	for _, v := range store.VisualChanges(brief) {
		sj.Visuals = append(sj.Visuals, visualJSON{Index: v.Index, Name: v.Name, URI: v.URI})
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(sj)
}

// writeSliceShowMarkdown renders the slice as markdown: name, facts line,
// then the brief.
func writeSliceShowMarkdown(out io.Writer, s domain.Slice, m domain.Milestone, project config.ProjectConfig, brief string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.Name)

	// Facts line.
	facts := []string{blank(s.StatusName)}
	if m.Name != "" {
		facts = append(facts, m.Name)
	}
	if s.AssigneeName != "" {
		facts = append(facts, s.AssigneeName)
	}
	if s.PRURL != "" {
		facts = append(facts, "PR "+s.PRURL)
	}
	if s.Branch != "" {
		facts = append(facts, "branch: "+s.Branch)
	}
	fmt.Fprintf(&b, "%s\n\n", strings.Join(facts, " · "))

	// Brief.
	if brief != "" {
		fmt.Fprintf(&b, "%s\n", brief)
	}

	_, err := io.WriteString(out, b.String())
	return err
}

// sliceRepo is the repo a slice is working in: the slice's own override when it
// has one, and the project default otherwise. It is the same logic
// briefOf uses.
// sliceBase is the base a launch of s would cut from: the same resolution
// [actions.PlaceAgent] makes, read from the slice's repo. Empty with no repo,
// since there is nowhere to read one from.
func sliceBase(env Env, s domain.Slice, project config.ProjectConfig) string {
	dir := sliceRepo(s, project)
	if dir == "" {
		return ""
	}
	return env.NewGit().Base(dir)
}

func sliceRepo(s domain.Slice, project config.ProjectConfig) string {
	if s.Repo != "" {
		return s.Repo
	}
	return project.WorkingDir
}
