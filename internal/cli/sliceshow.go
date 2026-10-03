package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
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
		return writeSliceShowJSON(env.Out, s, milestone, project, depByID, brief, sliceBase(env, s, project))
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
	State      string   `json:"state,omitempty"`
	Brief      string   `json:"brief"`
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
}

// taskEventJSON is one entry of a slice's task log, the wire form of
// [store.TaskEvent].
type taskEventJSON struct {
	Kind      string             `json:"kind"`
	Note      string             `json:"note,omitempty"`
	By        string             `json:"by,omitempty"`
	PR        string             `json:"pr,omitempty"`
	FollowUps []taskFollowUpJSON `json:"followUps,omitempty"`
}

// taskFollowUpJSON is one follow-up of a "follow_ups" event, the wire form of
// [store.TaskFollowUp].
type taskFollowUpJSON struct {
	Index    int    `json:"index"`
	Title    string `json:"title"`
	Brief    string `json:"brief"`
	Decision string `json:"decision,omitempty"`
	Link     string `json:"link,omitempty"`
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
		for _, f := range e.FollowUps {
			tj.FollowUps = append(tj.FollowUps, taskFollowUpJSON{
				Index: f.Index, Title: f.Title, Brief: f.Brief, Decision: f.Decision, Link: f.Link,
			})
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
func writeSliceShowJSON(out io.Writer, s domain.Slice, m domain.Milestone, project config.ProjectConfig, depByID map[string]domain.Slice, brief, base string) error {
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
		Brief:      brief,
		Events:     taskEventsJSON(s, brief),
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
