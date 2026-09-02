package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/logging"
)

// sliceDiff prints the unified diff of a handed-back branch — the same read
// the board's v key runs, but to a writer instead of a screen.
func sliceDiff(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-diff", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of the raw diff")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-diff: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-diff", rest[0])
	if err != nil {
		return err
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
	// Only a handed-back slice — in progress with a branch recorded on it —
	// has a diff to read; the same rule the board's v key and approve apply.
	if s.Branch == "" {
		return fmt.Errorf("%q is not handed back: only a slice with a branch has a diff to read", s.Name)
	}
	if s.Status == domain.SliceDone {
		return fmt.Errorf("%q is already Done: diff is for handed-back work under review", s.Name)
	}

	workdir := s.Repo
	if workdir == "" {
		workdir = project.WorkingDir
	}

	base, diff, err := env.NewGit().Diff(workdir, s.Branch)
	if err != nil {
		logging.Error("could not read diff", "error", err)
		return fmt.Errorf("read the diff: %w", err)
	}

	if *asJSON {
		return writeDiffJSON(env.Out, base, s.Branch, diff)
	}
	_, err = io.WriteString(env.Out, diff)
	return err
}

// diffJSON is the structured form of the diff output.
type diffJSON struct {
	Base   string     `json:"base"`
	Branch string     `json:"branch"`
	Files  []diffFile `json:"files"`
}

type diffFile struct {
	Path      string   `json:"path"`
	OldPath   string   `json:"old_path,omitempty"`
	Adds      int      `json:"adds"`
	Dels      int      `json:"dels"`
	Described bool     `json:"described"`
	Lines     []string `json:"lines"`
}

// writeDiffJSON encodes the diff result with parsed files.
func writeDiffJSON(out io.Writer, base, branch, diff string) error {
	files := git.ParseFiles(diff)
	docFiles := make([]diffFile, len(files))
	for i, f := range files {
		docFiles[i] = diffFile{
			Path:      f.Path,
			OldPath:   f.OldPath,
			Adds:      f.Added,
			Dels:      f.Removed,
			Described: f.Binary,
			Lines:     f.Lines,
		}
	}

	doc := diffJSON{Base: base, Branch: branch, Files: docFiles}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
