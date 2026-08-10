package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/skills"
)

// The state a skill directory was left in, which is the whole of what setup has
// to report: whether the machine gained a skill, had one brought up to date, or
// already had what the binary carries.
const (
	statusCreated   = "created"
	statusUpdated   = "updated"
	statusUnchanged = "unchanged"
	// statusLinked is a skill directory that is a symlink — the arrangement a
	// checkout of this repo uses to work on the skills in place. Writing through
	// it would edit whatever it points at, so it is left alone and said so.
	statusLinked = "linked"
)

// claudeSkillsDir is where Claude Code looks for a user's skills.
var claudeSkillsDir = []string{".claude", "skills"}

// setup installs the embedded skills into ~/.claude/skills, so that a machine
// with nothing but the binary can run the workflow the tracker is driven by.
//
// It is safe to run again: each skill is brought to what the binary carries and
// reported as created, updated or unchanged, which is what makes a stale install
// after `go install ...@latest` one command away from current. Nothing outside
// the directories named by the embedded skills is read or written.
func setup(args []string, env Env) error {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("setup: takes no arguments, given %d", len(rest))
	}

	dir, err := skillsDir()
	if err != nil {
		return err
	}
	installed, err := installSkills(skills.FS(), dir)
	if err != nil {
		return err
	}
	logging.Action("skills installed", "dir", dir, "skills", summarize(installed))

	if *asJSON {
		return writeJSON(env.Out, setupJSON{Dir: dir, Skills: installed})
	}
	_, err = io.WriteString(env.Out, setupMarkdown(dir, installed))
	return err
}

// skillsDir resolves ~/.claude/skills. It is the home directory rather than an
// XDG path because that is where Claude Code reads skills from, whatever the
// platform.
func skillsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(append([]string{home}, claudeSkillsDir...)...), nil
}

// installSkills lays every embedded skill down under dir, in name order so that
// a run reads the same way twice.
func installSkills(src fs.FS, dir string) ([]skillJSON, error) {
	entries, err := fs.ReadDir(src, ".")
	if err != nil {
		return nil, fmt.Errorf("read the embedded skills: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	var installed []skillJSON
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s, err := installSkill(src, e.Name(), dir)
		if err != nil {
			return nil, err
		}
		installed = append(installed, s)
	}
	sort.Slice(installed, func(i, j int) bool { return installed[i].Name < installed[j].Name })
	return installed, nil
}

// installSkill brings one skill directory to what the binary carries.
func installSkill(src fs.FS, name, dir string) (skillJSON, error) {
	target := filepath.Join(dir, name)
	info, err := os.Lstat(target)
	fresh := errors.Is(err, fs.ErrNotExist)
	switch {
	case fresh:
	case err != nil:
		return skillJSON{}, fmt.Errorf("read %s: %w", target, err)
	case info.Mode()&fs.ModeSymlink != 0:
		return skillJSON{Name: name, Status: statusLinked, Path: target}, nil
	case !info.IsDir():
		return skillJSON{}, fmt.Errorf("%s is not a directory: move it aside and run `nat setup` again", target)
	}

	changed, err := copyTree(src, name, target)
	if err != nil {
		return skillJSON{}, err
	}
	status := statusUnchanged
	switch {
	case fresh:
		status = statusCreated
	case changed:
		status = statusUpdated
	}
	return skillJSON{Name: name, Status: status, Path: target}, nil
}

// copyTree writes the embedded directory root over target and takes away
// whatever else is in there, so that an installed skill is the one the binary
// carries rather than that one merged with an older version of itself. It
// reports whether anything on disk had to move.
//
// Only paths under target are touched, and target is only ever a directory
// named by an embedded skill: everything else in ~/.claude/skills belongs to
// somebody else and is none of setup's business.
func copyTree(src fs.FS, root, target string) (bool, error) {
	changed := false
	wanted := map[string]bool{".": true}
	err := fs.WalkDir(src, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := relative(path, root, "/")
		wanted[rel] = true
		dst := filepath.Join(target, filepath.FromSlash(rel))
		if d.IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return fmt.Errorf("create %s: %w", dst, err)
			}
			return nil
		}
		want, err := fs.ReadFile(src, path)
		if err != nil {
			return fmt.Errorf("read the embedded %s: %w", path, err)
		}
		wrote, err := writeFile(dst, want)
		changed = changed || wrote
		return err
	})
	if err != nil {
		return changed, err
	}
	pruned, err := prune(target, wanted)
	return changed || pruned, err
}

// writeFile puts content at path, leaving a file that already says exactly that
// alone — so that a re-run of setup reports what it did rather than rewriting
// every skill every time. It reports whether it wrote.
func writeFile(path string, content []byte) (bool, error) {
	switch existing, err := os.Lstat(path); {
	case err == nil && existing.Mode().IsRegular():
		current, err := os.ReadFile(path)
		if err != nil {
			return false, fmt.Errorf("read %s: %w", path, err)
		}
		if bytes.Equal(current, content) {
			return false, nil
		}
	case err == nil:
		// A symlink or something stranger standing where a skill file belongs:
		// removed rather than written through, which would edit its target.
		if err := os.RemoveAll(path); err != nil {
			return false, fmt.Errorf("remove %s: %w", path, err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// prune takes away everything under target that the skill does not have,
// reporting whether it took anything. A file left behind by an older version of
// a skill is not harmless: skills are read whole, and a stale instruction file
// still reads as an instruction.
func prune(target string, wanted map[string]bool) (bool, error) {
	pruned := false
	err := filepath.WalkDir(target, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if wanted[relative(path, target, string(filepath.Separator))] {
			return nil
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		pruned = true
		if d.IsDir() {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return pruned, fmt.Errorf("tidy %s: %w", target, err)
	}
	return pruned, nil
}

// relative is path underneath root, with sep as its separator: the empty
// remainder that root itself leaves said as ".", and slashes throughout, so
// that a tree walked in an embedded filesystem and the same tree walked on disk
// are named the same way.
func relative(path, root, sep string) string {
	rel := strings.TrimPrefix(strings.TrimPrefix(path, root), sep)
	if rel == "" {
		return "."
	}
	return strings.ReplaceAll(rel, sep, "/")
}

// summarize is the installed skills as one line, for the log.
func summarize(installed []skillJSON) string {
	parts := make([]string, len(installed))
	for i, s := range installed {
		parts[i] = s.Name + " " + s.Status
	}
	return strings.Join(parts, ", ")
}

// setupJSON is what setup did, as a document: where the skills went and what
// happened to each.
type setupJSON struct {
	Dir    string      `json:"skills_dir"`
	Skills []skillJSON `json:"skills"`
}

// skillJSON is one installed skill.
type skillJSON struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Path   string `json:"path"`
}

// setupMarkdown reports the install, saying where it went — the one thing
// somebody running setup cannot see from the command they typed.
func setupMarkdown(dir string, installed []skillJSON) string {
	var b strings.Builder
	b.WriteString("# Skills\n\n")
	fmt.Fprintf(&b, "Installed into %s\n\n", dir)
	for _, s := range installed {
		fmt.Fprintf(&b, "- %s — %s\n", s.Name, s.Status)
		if s.Status == statusLinked {
			fmt.Fprintf(&b, "  - a symlink, left as it is: delete it and run `nat setup` again for a real copy\n")
		}
	}
	return b.String()
}
