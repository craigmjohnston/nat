package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/store"
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
	s, workdir, err := handedBackSlice(ctx, "slice-diff", rest[0], *projectRef, env)
	if err != nil {
		return err
	}
	gitCLI := env.NewGit()

	// The base is whatever base the branch actually has: the branch its pull
	// request records, where there is one — a pull request against anything
	// but the default branch is measured against what it would merge into —
	// and the repository's own default where there is not, which is every
	// hand-back still waiting to be approved. The pull request's base is the
	// last batched reading's ([lastReading]): this ran on every Changes tab
	// load and every tally refresh, and asks GitHub nothing. A pull request
	// no reading has reached yet diffs against the default, as one gh could
	// not answer for always did: a diff against main beats no diff.
	baseName := ""
	if s.PRURL != "" {
		baseName = env.loadLastReading().Bases[gh.NormaliseURL(s.PRURL)]
		if baseName == "" {
			logging.Action("no reading of the pull request's base yet; diffing against the default", "pr", s.PRURL)
		}
	}

	switch {
	case *commitsFlag:
		return sliceCommits(gitCLI, workdir, baseName, s.Branch, *asJSON, env.Out)
	case *commitFlag != "":
		return sliceCommitDiff(gitCLI, workdir, *commitFlag, *asJSON, env.Out)
	default:
		return sliceBranchDiff(gitCLI, workdir, baseName, s.Branch, *asJSON, env.Out)
	}
}

// handedBackSlice is the slice a branch read is about and the checkout its
// branch is read in — refused for a slice with no branch to read. Shared by
// slice-diff and slice-file.
//
// A slice in progress whose branch was cleared after a hand-back — resumed,
// or sent back — is read on [actions.AgentBranch], the very branch a
// relaunch places its agent on, so the review keeps a reading while the work
// is redone; the slice comes back with that branch filled in. Only a slice
// never handed back at all (no Handed back in its task log) is refused.
func handedBackSlice(ctx context.Context, command, ref, projectRef string, env Env) (domain.Slice, string, error) {
	id, err := pageID(command, ref)
	if err != nil {
		return domain.Slice{}, "", err
	}

	_, projectID, project, err := env.projectFor(projectRef)
	if err != nil {
		return domain.Slice{}, "", err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return domain.Slice{}, "", err
	}

	s, _, err := st.Slice(ctx, id)
	if err != nil {
		return domain.Slice{}, "", fmt.Errorf("load the slice: %w", err)
	}
	// Only a slice with a branch recorded has a diff to read at all. A Done
	// one is no longer refused: the board marks a slice Done as it opens the
	// pull request, and the review goes on reading the branch until that
	// lands — the same reason such a slice stays in the NEEDS REVIEW section.
	if s.Branch == "" && s.Status == domain.SliceClaimed && handedBackBefore(ctx, st, s.ID) {
		s.Branch = actions.AgentBranch(s)
	}
	if s.Branch == "" {
		return domain.Slice{}, "", fmt.Errorf("%q is not handed back: only a slice with a branch has a diff to read", s.Name)
	}

	workdir := s.Repo
	if workdir == "" {
		workdir = project.WorkingDir
	}
	return s, workdir, nil
}

// sliceBranchDiff is the plain, whole-branch read: the same call slice-diff
// always made, factored out so the two finer reads sit beside it rather than
// inside one growing function.
func sliceBranchDiff(gitCLI GitCLI, workdir, baseName, branch string, asJSON bool, out io.Writer) error {
	base, diff, err := gitCLI.DiffFrom(workdir, baseName, branch)
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
// without diffing any of it — measured against the same base the whole-branch
// diff uses, so the two reads describe one stretch of history.
func sliceCommits(gitCLI GitCLI, workdir, baseName, branch string, asJSON bool, out io.Writer) error {
	base, commits, err := gitCLI.CommitsFrom(workdir, baseName, branch)
	if err != nil {
		logging.Error("could not read the branch's commits", "error", err)
		return fmt.Errorf("read the branch's commits: %w", err)
	}
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

// handedBackBefore reports whether a slice's task log holds a hand-back: a
// slice whose branch is empty now was handed back once and taken back to
// work, rather than never handed back at all. A body that cannot be read is
// logged and concludes nothing — the slice is read as never handed back.
func handedBackBefore(ctx context.Context, st store.Store, id string) bool {
	return holdsHandBack(taskLogOf(ctx, st, id))
}

// taskLogOf is a slice's body, for its task log — "" where it cannot be
// read, which is logged and reads as a log holding nothing.
func taskLogOf(ctx context.Context, st store.Store, id string) string {
	body, err := st.Body(ctx, id)
	if err != nil {
		logging.Action("could not read a slice's task log for an earlier hand-back", "slice", id, "err", err)
		return ""
	}
	return body
}

// ciProvenance is the By a nudge's Sent back reads back with:
// [actions.ChecksProvenance] with its "From " taken off, as
// [store.TaskEvents] takes it.
var ciProvenance = strings.TrimPrefix(actions.ChecksProvenance, "From ")

// fixingChecks is the checks, by name, that the latest CI failure on a task
// log read off body names — a Checks failed, or a Sent back from CI — where
// no hand-back follows it: the failure the slice's agent was given and has
// not yet handed back a fix for. Nil where a hand-back follows the last one,
// or there is none. gnat keeps a resumed slice's failing mark on these while
// its fix's checks run.
func fixingChecks(body string) []string {
	var names []string
	for _, e := range store.TaskEvents(body) {
		switch {
		case e.Kind == store.HandedBackKind:
			names = nil
		case e.Kind == store.ChecksFailedKind, e.Kind == store.SentBackKind && e.By == ciProvenance:
			names = checkNames(e.Note)
		}
	}
	return names
}

// checkNames reads the check names back off a failure record's bullets, as
// actions' checkLines writes them: a name, then ": " and its run URL where it
// has one.
func checkNames(record string) []string {
	var names []string
	for _, line := range strings.Split(record, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "- ")
		if !ok {
			continue
		}
		if i := strings.LastIndex(rest, ": "); i >= 0 && strings.HasPrefix(rest[i+2:], "http") {
			rest = rest[:i]
		}
		names = append(names, strings.TrimSpace(rest))
	}
	return names
}

// holdsHandBack reports whether a task log, read off body, holds a Handed
// back.
func holdsHandBack(body string) bool {
	for _, e := range store.TaskEvents(body) {
		if e.Kind == store.HandedBackKind {
			return true
		}
	}
	return false
}

// takenBack reports whether s is a slice handed back and then taken back to
// work — resumed, or sent back: in progress, its Branch cleared on a project
// that has the column, and a Handed back on its task log. handedBack answers
// that last question, and is asked only once the rest hold, since it may cost
// a body read. It is what keeps gnat's Changes section, read on
// [actions.AgentBranch], on such a slice with or without a pull request.
func takenBack(s domain.Slice, hasBranch bool, handedBack func() bool) bool {
	return hasBranch && s.Status == domain.SliceClaimed && s.Branch == "" && handedBack()
}
