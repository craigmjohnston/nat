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
	"github.com/craigmjohnston/nat/internal/notion"
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

	_, _, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	client := env.NewClient(env.Tokens.Token)

	shape, err := sliceShape(ctx, client, project)
	if err != nil {
		return err
	}
	page, err := client.GetPage(ctx, id)
	if err != nil {
		return fmt.Errorf("load the slice: %w", err)
	}

	// Read the slice and its dependencies.
	s := domain.SliceFromPage(*page)
	depByID := dependencyIndex(ctx, client, s)

	milestone := milestoneOf(s, shape)
	brief, err := body(ctx, client, s.ID)
	if err != nil {
		return fmt.Errorf("could not read the slice's brief: %w", err)
	}

	if *asJSON {
		return writeSliceShowJSON(env.Out, s, milestone, project, shape, depByID, brief)
	}
	return writeSliceShowMarkdown(env.Out, s, milestone, project, brief)
}

// sliceShowJSON is the full structured form of a single slice.
type sliceShowJSON struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	URL        string   `json:"url"`
	Status     string   `json:"status"`
	Milestone  string   `json:"milestone"`
	Assignee   string   `json:"assignee"`
	Branch     string   `json:"branch,omitempty"`
	Repo       string   `json:"repo,omitempty"`
	PR         string   `json:"pr,omitempty"`
	DependsOn  []string `json:"depends_on,omitempty"`
	Blocked    bool     `json:"blocked"`
	HandedBack bool     `json:"handed_back"`
	State      string   `json:"state,omitempty"`
	Brief      string   `json:"brief"`
}

// writeSliceShowJSON encodes the slice as JSON.
func writeSliceShowJSON(out io.Writer, s domain.Slice, m domain.Milestone, project config.ProjectConfig, shape notion.SliceShape, depByID map[string]domain.Slice, brief string) error {
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
		PR:         s.PRURL,
		DependsOn:  s.DependsOn,
		Blocked:    domain.Blocked(s, slicesByID),
		HandedBack: s.HandedBack(),
		Brief:      brief,
	}
	if state != domain.SliceStateNone {
		sj.State = state.String()
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
func sliceRepo(s domain.Slice, project config.ProjectConfig) string {
	if s.Repo != "" {
		return s.Repo
	}
	return project.WorkingDir
}
