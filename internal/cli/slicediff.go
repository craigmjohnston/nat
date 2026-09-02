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
// the board's v key runs, but to a writer instead of a screen. --commits and
// --commit are two finer reads of the same branch: a list of what it added
// since the merge base, and the diff of exactly one of them, for a review
// that wants the granularity a whole-branch diff folds away. All three read
// the same branch under the same refusals; only what comes back differs, so
// they are one command's mutually exclusive flags rather than three commands.
func sliceDiff(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-diff", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of the raw diff")
	commitsFlag := flags.Bool("commits", false, "list the branch's commits against its merge base, instead of diffing it")
	commitFlag := flags.String("commit", "", "diff one commit of the branch's history, by its sha, instead of the whole branch")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-diff: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	if *commitsFlag && *commitFlag != "" {
		return usageErrorf("slice-diff: --commits and --commit are two different reads: " +
			"list the branch's commits, or diff one of them, not both")
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
	// has a diff to read; the same rule the board's v key and approve apply,
	// whichever of the three reads was asked for.
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
	gitCLI := env.NewGit()

	switch {
	case *commitsFlag:
		return sliceCommits(gitCLI, workdir, s.Branch, *asJSON, env.Out)
	case *commitFlag != "":
		return sliceCommitDiff(gitCLI, workdir, *commitFlag, *asJSON, env.Out)
	default:
		return sliceBranchDiff(gitCLI, workdir, s.Branch, *asJSON, env.Out)
	}
}

// sliceBranchDiff is the plain, whole-branch read: the same call slice-diff
// always made, factored out so the two finer reads sit beside it rather than
// inside one growing function.
func sliceBranchDiff(gitCLI GitCLI, workdir, branch string, asJSON bool, out io.Writer) error {
	base, diff, err := gitCLI.Diff(workdir, branch)
	if err != nil {
		logging.Error("could not read diff", "error", err)
		return fmt.Errorf("read the diff: %w", err)
	}
	if asJSON {
		return writeDiffJSON(out, base, branch, diff)
	}
	_, err = io.WriteString(out, diff)
	return err
}

// sliceCommits is --commits: the branch's own history since the merge base,
// without diffing any of it.
func sliceCommits(gitCLI GitCLI, workdir, branch string, asJSON bool, out io.Writer) error {
	commits, err := gitCLI.Commits(workdir, branch)
	if err != nil {
		logging.Error("could not read the branch's commits", "error", err)
		return fmt.Errorf("read the branch's commits: %w", err)
	}
	base := gitCLI.Base(workdir)
	if asJSON {
		return writeCommitsJSON(out, base, branch, commits)
	}
	_, err = io.WriteString(out, commitsMarkdown(base, branch, commits))
	return err
}

// sliceCommitDiff is --commit: one commit of the branch's history, diffed
// against its own parent rather than against the merge base the whole-branch
// read uses. The JSON form reuses [writeDiffJSON] with the commit's parent as
// the base and the commit itself as the branch, since that is exactly what
// was diffed.
func sliceCommitDiff(gitCLI GitCLI, workdir, sha string, asJSON bool, out io.Writer) error {
	diff, err := gitCLI.CommitDiff(workdir, sha)
	if err != nil {
		logging.Error("could not read a commit's diff", "error", err)
		return fmt.Errorf("read the commit's diff: %w", err)
	}
	if asJSON {
		return writeDiffJSON(out, sha+"^", sha, diff)
	}
	_, err = io.WriteString(out, diff)
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
	// Language is chroma's own name for the lexer matched to the file's
	// path, omitted where none matched — the same fallback rule
	// diffsyntax.go draws by: a file with no language is drawn exactly as
	// the viewer always drew one, and so gets no Tokens either.
	Language string `json:"language,omitempty"`
	// Tokens is one entry per line of Lines, each the runs — [kind,
	// length] pairs — that line's content takes after its own +/-/space
	// prefix. Present only alongside a Language: a file with none is left
	// wholly to the reader's own fallback colouring.
	Tokens [][]tokenRun `json:"tokens,omitempty"`
}

// writeDiffJSON encodes the diff result with parsed files.
func writeDiffJSON(out io.Writer, base, branch, diff string) error {
	files := git.ParseFiles(diff)
	docFiles := make([]diffFile, len(files))
	for i, f := range files {
		lex := lexerFor(f)
		docFiles[i] = diffFile{
			Path:      f.Path,
			OldPath:   f.OldPath,
			Adds:      f.Added,
			Dels:      f.Removed,
			Described: f.Binary,
			Lines:     f.Lines,
			Language:  languageOf(lex),
		}
		if lex != nil {
			tokens := make([][]tokenRun, len(f.Lines))
			for j, line := range f.Lines {
				tokens[j] = lineTokens(lex, line)
			}
			docFiles[i].Tokens = tokens
		}
	}

	doc := diffJSON{Base: base, Branch: branch, Files: docFiles}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
