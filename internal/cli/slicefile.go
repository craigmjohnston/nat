package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/logging"
)

// sliceFile prints a run of one file's own lines as a handed-back branch
// leaves it — or, with --commit, as one commit of it left it — which is what
// a diff viewer's expand controls reveal: a unified diff is a few lines of
// context around each change and nothing else, and `git show <ref>:<path>`
// is the only place the lines between the hunks can come from. The board
// reads the same lines for its own expand zones (internal/tui/diffzones.go).
//
// --from and --to are 1-based and inclusive, on the ref's side of the
// change; --to left off reads to the end of the file. The JSON form says how
// many lines the file has in all, since a diff says where its hunks end and
// nothing about how much file follows them, and lexes each line as
// slice-diff lexes the diff's own.
func sliceFile(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-file", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of the raw lines")
	commitFlag := flags.String("commit", "", "read the file as one commit of the branch left it, by its sha")
	pathFlag := flags.String("path", "", "the file, by its path on the branch's side of the change")
	fromFlag := flags.Int("from", 1, "the first line to print, 1-based")
	toFlag := flags.Int("to", 0, "the last line to print, inclusive; 0 reads to the end of the file")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-file: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	if *pathFlag == "" {
		return usageErrorf("slice-file: --path names the file to read")
	}
	if *fromFlag < 1 || (*toFlag != 0 && *toFlag < *fromFlag) {
		return usageErrorf("slice-file: --from is 1 or more, and --to, when given, is not before it")
	}

	s, workdir, err := handedBackSlice(ctx, "slice-file", rest[0], *projectRef, env)
	if err != nil {
		return err
	}
	ref := s.Branch
	if *commitFlag != "" {
		ref = *commitFlag
	}

	all, err := env.NewGit().Show(workdir, ref, *pathFlag)
	if err != nil {
		logging.Error("could not read a file at a branch", "error", err)
		return fmt.Errorf("read %s at %s: %w", *pathFlag, ref, err)
	}
	lines := linesBetween(all, *fromFlag, *toFlag)
	if !*asJSON {
		if len(lines) == 0 {
			return nil
		}
		_, err = io.WriteString(env.Out, strings.Join(lines, "\n")+"\n")
		return err
	}
	return writeFileLinesJSON(env.Out, *pathFlag, ref, *fromFlag, len(all), lines)
}

// linesBetween is lines from..to of a file, 1-based and inclusive, with to 0
// for the end — cut to what the file has, so a range running past its end is
// the lines that are there rather than a refusal.
func linesBetween(all []string, from, to int) []string {
	if to == 0 || to > len(all) {
		to = len(all)
	}
	if from > to {
		return []string{}
	}
	return all[from-1 : to]
}

// fileLinesJSON is slice-file's structured form.
type fileLinesJSON struct {
	Path string `json:"path"`
	Ref  string `json:"ref"`
	// From is the number of Lines' first line; Total how many the file has.
	From  int      `json:"from"`
	Total int      `json:"total"`
	Lines []string `json:"lines"`
	// Language and Tokens follow diffFile's own rule: present only where
	// chroma matched the path, Tokens one entry per line of Lines.
	Language string       `json:"language,omitempty"`
	Tokens   [][]tokenRun `json:"tokens,omitempty"`
}

func writeFileLinesJSON(out io.Writer, path, ref string, from, total int, lines []string) error {
	doc := fileLinesJSON{Path: path, Ref: ref, From: from, Total: total, Lines: lines}
	if lex := lexerFor(git.File{Path: path}); lex != nil {
		doc.Language = languageOf(lex)
		doc.Tokens = make([][]tokenRun, len(lines))
		for i, line := range lines {
			// Lexed as the context line it is in the diff, prefix and all.
			doc.Tokens[i] = lineTokens(lex, " "+line)
		}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
