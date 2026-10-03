package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/store"
)

// sliceNote appends a note to a slice's brief: something a later session on
// that slice needs to know, which the session writing it found out — a
// constraint, a seam that moved, an assumption in the brief no longer true.
// The brief an agent is handed at launch is the page body, so the note reaches
// whoever works the slice next with nothing further to do.
//
// The target is named the way an agent knows it: by name, matched as
// plan-apply matches a depends_on title, with --milestone to say which where
// the name is filed under more than one — or by ID or URL, as every other
// slice command takes one, so an agent can note its own slice with the ID its
// prompt gave it.
//
// Where the note came from is nat's to write, never the caller's: --from names
// the slice the note was written from, read here for its name and milestone,
// and without it the note is from the person at the keyboard. Either way it
// names no ID or URL — the same rule the agents writing notes are held to.
//
// Every refusal is settled before anything is written: a Done slice (a record
// of what happened), one whose body cannot be read, an empty note, and a
// --from slice that cannot be read. There is no ownership check: the point is
// leaving context on work somebody else will pick up.
func sliceNote(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-note", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	note := flags.String("note", "", "the note to append; `-` reads it from stdin")
	from := flags.String("from", "", "the slice the note is written from, by name, ID or URL")
	milestone := flags.String("milestone", "", "the milestone the slice named is filed under, where its name alone is ambiguous")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-note: want exactly one slice, by name, URL or ID, given %d", len(rest))
	}
	text, err := briefText("slice-note", "--note", *note, env.In)
	if err != nil {
		return err
	}
	if text == "" {
		return usageErrorf("slice-note: no note given: pass --note '<text>', or --note - to pipe it in")
	}

	cfg, projectID, project, err := env.projectFor(*projectRef)
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
	r := sliceResolver{ctx: ctx, st: st, p: storeProject(projectID, project), milestones: shape.Milestones}

	s, err := r.resolve(rest[0], *milestone)
	if err != nil {
		return err
	}
	if s.Status == domain.SliceDone {
		return fmt.Errorf("%q is %s: a finished slice is a record of what happened, and takes no notes", s.Name, s.StatusName)
	}
	if _, err := st.Body(ctx, s.ID); err != nil {
		return fmt.Errorf("%q has no readable brief to add a note to: %w", s.Name, err)
	}
	provenance, err := noteProvenance(r, *from, cfg)
	if err != nil {
		return err
	}

	if err := st.RecordNote(ctx, s.ID, provenance, text); err != nil {
		return fmt.Errorf("file the note: %w", err)
	}
	env.nudged()
	_, err = fmt.Fprintf(env.Out, "# %s\n\nNote filed at the end of its brief: %s.\n", s.Name, provenance)
	return err
}

// noteProvenance is the line a note opens with: the slice --from names, read
// for its name and milestone, or the person at the keyboard where there is
// none.
func noteProvenance(r sliceResolver, from string, cfg config.Config) (string, error) {
	if strings.TrimSpace(from) == "" {
		if cfg.AssigneeUserName == "" {
			return "", fmt.Errorf("no assignee in the config to say the note is from: pass --from, or finish setting nat up")
		}
		return fromPerson(cfg.AssigneeUserName), nil
	}
	s, err := r.resolve(from, "")
	if err != nil {
		return "", fmt.Errorf("--from: %w", err)
	}
	return fromSlice(s, r.milestones), nil
}

// fromSlice says a note or a queued follow-up came from a slice, by its name
// and its milestone's — never an ID or URL, which is the tracker's own and
// which the next reader of the brief may have no way to open. The milestone is
// left off where the slice has none.
func fromSlice(s domain.Slice, milestones []domain.Milestone) string {
	return "From " + sliceLabel(s, milestones)
}

// sliceLabel names a slice as a reader of the plan knows it: its name, quoted,
// and its milestone's after it where it has one.
func sliceLabel(s domain.Slice, milestones []domain.Milestone) string {
	if m := milestoneOf(s, milestones); m.Name != "" {
		return `"` + s.Name + `" (` + m.Name + `)`
	}
	return `"` + s.Name + `"`
}

// fromPerson says a note came from a person rather than a slice: one typed by
// hand, or from a session working no slice.
func fromPerson(name string) string {
	return "From " + name
}

// sliceResolver finds the one slice a reference names in a project: an ID or
// URL read directly, or a name matched against the plan.
type sliceResolver struct {
	ctx        context.Context
	st         store.Store
	p          store.Project
	milestones []domain.Milestone
}

// resolve reads the slice ref names. A page ID or URL is read as every other
// slice command reads one; anything else is a name, matched as plan-apply
// matches a depends_on title — trimmed, case aside — and narrowed to milestone
// where one is given. A name matching nothing, and one matching several with
// no milestone to choose between them, are each refused naming what was given.
func (r sliceResolver) resolve(ref, milestone string) (domain.Slice, error) {
	if id, err := pageID("slice-note", ref); err == nil {
		if strings.TrimSpace(milestone) != "" {
			return domain.Slice{}, usageErrorf("slice-note: --milestone narrows a slice named by name, and %q is an ID", ref)
		}
		s, _, err := loadSlice(r.ctx, r.st, id)
		return s, err
	}
	name, milestone := strings.TrimSpace(ref), strings.TrimSpace(milestone)
	plan, err := r.st.Plan(r.ctx, r.p)
	if err != nil {
		return domain.Slice{}, fmt.Errorf("read the plan to find %q: %w", name, err)
	}
	var matches []domain.Slice
	for _, s := range plan.Project.Slices {
		if !strings.EqualFold(strings.TrimSpace(s.Name), name) {
			continue
		}
		if milestone != "" && !strings.EqualFold(milestoneOf(s, r.milestones).Name, milestone) {
			continue
		}
		matches = append(matches, s)
	}
	switch {
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) == 0 && milestone != "":
		return domain.Slice{}, fmt.Errorf("no slice named %q is filed under %q", name, milestone)
	case len(matches) == 0:
		return domain.Slice{}, fmt.Errorf("no slice in the project is named %q", name)
	}
	where := make([]string, len(matches))
	for i, s := range matches {
		where[i] = sliceLabel(s, r.milestones)
	}
	return domain.Slice{}, fmt.Errorf("%d slices are named %q: %s — pass --milestone to say which",
		len(matches), name, strings.Join(where, ", "))
}
