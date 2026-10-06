package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
)

// sliceEdit replaces a slice's description — the page body slice-add writes
// it as — with new text, clearing whatever was there first, and with --title
// renames it: either or both, never neither. A new title is held to
// [domain.MaxSliceTitleLen]; whether another slice already has it is not this
// command's to refuse — a direct edit is the caller's deliberate act, as a
// direct slice-add is.
//
// Only a Todo slice is editable. One in progress is being worked by an agent
// that already has its own idea of the brief, written the moment it claimed
// the slice, and editing it out from under that session would leave the
// agent working from a brief nobody can see any more; one Done is finished
// work, and there is nothing left to brief.
func sliceEdit(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-edit", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	description := flags.String("description", "", "the new brief to write on the slice page; `-` reads it from stdin")
	newTitle := flags.String("title", "", "the slice's new title")
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-edit: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-edit", rest[0])
	if err != nil {
		return err
	}
	// The new brief is settled before anything is read from Notion, so an edit
	// whose stdin cannot be read fails having changed nothing.
	brief, err := briefText("slice-edit", "--description", *description, env.In)
	if err != nil {
		return err
	}
	title := strings.TrimSpace(*newTitle)
	if brief == "" && title == "" {
		return usageErrorf("slice-edit: nothing to edit: pass --title, --description (or pipe one in with -), or both")
	}
	if err := domain.CheckSliceTitle(title); err != nil {
		return usageErrorf("slice-edit: %v", err)
	}

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}

	s, _, err := loadSlice(ctx, st, id)
	if err != nil {
		return err
	}
	if err := editable(s); err != nil {
		return err
	}

	if title != "" {
		if err := st.SetSliceTitle(ctx, s.ID, title); err != nil {
			return err
		}
	}
	if brief != "" {
		if err := st.SetSliceBrief(ctx, s.ID, brief); err != nil {
			// A rename already written stands, and the board should hear of it.
			if title != "" {
				env.nudged()
			}
			return err
		}
	}

	env.nudged()
	logging.Action("slice edited", "slice", s.ID, "name", s.Name, "retitled", title != "")
	if *asJSON {
		return writeJSON(env.Out, sliceEditedJSON{ID: s.ID, Name: s.Name, URL: s.URL, Title: title, Brief: brief})
	}
	_, err = io.WriteString(env.Out, sliceEditedMarkdown(s, title, brief, project.WorkingDir))
	return err
}

// editable refuses a slice edit unless the slice is Todo, naming what it
// actually is and why that rules an edit out — the same shape [notOursError]
// takes for the other rule a slice's own status refuses an action by.
func editable(s domain.Slice) error {
	switch s.Status {
	case domain.SliceClaimed:
		return fmt.Errorf("%q is in progress: a slice being worked is not edited under its agent", s.Name)
	case domain.SliceDone:
		return fmt.Errorf("%q is already Done: a finished slice's brief is not edited after the fact", s.Name)
	}
	return nil
}

// sliceEditedJSON is the structured form of a successful edit.
// Name is the slice's name as it was read; Title is the one it was renamed
// to, omitted where the edit left the title alone.
type sliceEditedJSON struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	URL   string `json:"url,omitempty"`
	Title string `json:"title,omitempty"`
	Brief string `json:"brief"`
}

// sliceEditedMarkdown reports the slice as edited, under the name it now
// has, the brief included so the caller can see exactly what landed rather
// than trust the write went through.
func sliceEditedMarkdown(s domain.Slice, title, brief, workingDir string) string {
	var b strings.Builder
	name := s.Name
	if title != "" {
		name = title
	}
	fmt.Fprintf(&b, "# %s\n\n", name)
	if title != "" {
		fmt.Fprintf(&b, "Renamed from %q.\n\n", s.Name)
	}
	if brief != "" {
		b.WriteString("Description replaced.\n\n")
	}
	fmt.Fprintf(&b, "- Notion page: %s\n", s.ID)
	if s.URL != "" {
		fmt.Fprintf(&b, "- Notion URL: %s\n", s.URL)
	}
	if repo := s.Repo; repo != "" {
		workingDir = repo
	}
	if workingDir != "" {
		fmt.Fprintf(&b, "- Working directory: %s\n", workingDir)
	}
	if brief != "" {
		b.WriteString("\n## Brief\n\n")
		b.WriteString(brief)
		b.WriteString("\n")
	}
	return b.String()
}
