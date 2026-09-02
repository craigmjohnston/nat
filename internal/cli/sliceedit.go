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
// it as — with new text, clearing whatever was there first.
//
// Only a Todo slice is editable. One in progress is being worked by an agent
// that already has its own idea of the brief, written the moment it claimed
// the slice, and editing it out from under that session would leave the
// agent working from a brief nobody can see any more; one Done is finished
// work, and there is nothing left to brief.
func sliceEdit(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-edit", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	description := flags.String("description", "", "the new brief to write on the slice page (required); `-` reads it from stdin")
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
	brief, err := briefText("slice-edit", *description, env.In)
	if err != nil {
		return err
	}
	if brief == "" {
		return usageErrorf("slice-edit: no description given: pass --description or pipe one in with -")
	}

	_, _, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	client := env.NewClient(env.Tokens.Token)

	page, err := client.GetPage(ctx, id)
	if err != nil {
		return fmt.Errorf("load the slice: %w", err)
	}
	s := domain.SliceFromPage(*page)
	if err := editable(s); err != nil {
		return err
	}

	blocks, err := client.GetBlockChildren(ctx, page.ID)
	if err != nil {
		return fmt.Errorf("read the slice's current brief: %w", err)
	}
	for _, b := range blocks {
		if err := client.DeleteBlock(ctx, b.ID); err != nil {
			return fmt.Errorf("clear the slice's current brief: %w", err)
		}
	}
	if _, err := client.AppendBlockChildren(ctx, page.ID, paragraphBlocks(brief)); err != nil {
		return fmt.Errorf("write the new brief: %w", err)
	}

	env.nudged()
	logging.Action("slice edited", "slice", s.ID, "name", s.Name)
	if *asJSON {
		return writeJSON(env.Out, sliceEditedJSON{ID: s.ID, Name: s.Name, URL: s.URL, Brief: brief})
	}
	_, err = io.WriteString(env.Out, sliceEditedMarkdown(s, brief, project.WorkingDir))
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
type sliceEditedJSON struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	URL   string `json:"url,omitempty"`
	Brief string `json:"brief"`
}

// sliceEditedMarkdown reports the slice as edited, the brief included so the
// caller can see exactly what landed rather than trust the write went
// through.
func sliceEditedMarkdown(s domain.Slice, brief, workingDir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.Name)
	b.WriteString("Description replaced.\n\n")
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
	b.WriteString("\n## Brief\n\n")
	b.WriteString(brief)
	b.WriteString("\n")
	return b.String()
}
