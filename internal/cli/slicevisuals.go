package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/store"
)

// sliceVisuals files the images an agent rendered of what its slice changed,
// for the user to review in the app. Unlike follow-ups nothing waits on them:
// the agent carries on, and the user's comments, if any, reach it as a message.
// A later hand-in replaces an earlier one, so the agent hands in the full set
// each time.
//
// Only a slice this user holds takes them, for the reason complete-slice is
// held to the same rule: the agent working it is the one with something to
// hand in.
func sliceVisuals(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-visuals", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var given stringList
	flags.Var(&given, "visual", "an image: its first line what it shows, the next its path or URI; repeat for more")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-visuals: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-visuals", rest[0])
	if err != nil {
		return err
	}
	items, err := visualsOf(given)
	if err != nil {
		return err
	}

	cfg, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	if cfg.AssigneeUserID == "" {
		return fmt.Errorf("no assignee in the config: open the board with `nat` and finish setting it up")
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}
	shape, err := sliceShape(ctx, st, projectID, project)
	if err != nil {
		return err
	}
	s, pageShape, err := loadSlice(ctx, st, id)
	if err != nil {
		return err
	}
	if !store.Holds(s, shape.On(pageShape), cfg.AssigneeUserID) {
		return notOursError(s, cfg.AssigneeUserName, "given visual changes")
	}

	if err := st.RecordVisuals(ctx, s.ID, items); err != nil {
		return fmt.Errorf("file the visual changes: %w", err)
	}
	env.nudged()
	_, err = io.WriteString(env.Out, visualsFiledMarkdown(s, items))
	return err
}

// uriScheme is the scheme a URI opens with, which is what tells one apart from
// a path on this machine.
var uriScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)

// visualsOf reads each --visual as the image it describes, settling every
// refusal before anything is read or written: none at all, one with no name or
// no URI, a URI over more than one line, two of the same name, and a path that
// names no file. A path — bare, or a file:// URI — is filed absolute, so the
// app can open it from wherever it runs; any other URI is filed as given.
func visualsOf(given []string) ([]store.VisualChange, error) {
	if len(given) == 0 {
		return nil, usageErrorf("slice-visuals: no visual given: pass --visual '<what it shows>\\n<absolute path to the image>'")
	}
	seen := map[string]bool{}
	items := make([]store.VisualChange, 0, len(given))
	for _, g := range given {
		name, uri, _ := strings.Cut(strings.TrimSpace(strings.ReplaceAll(g, "\r\n", "\n")), "\n")
		name, uri = strings.TrimSpace(name), strings.TrimSpace(uri)
		switch {
		case name == "":
			return nil, usageErrorf("slice-visuals: a visual has no name: its first line says what it shows")
		case uri == "":
			return nil, usageErrorf("slice-visuals: %q has no image: give its path on the line after the name", name)
		case strings.Contains(uri, "\n"):
			return nil, usageErrorf("slice-visuals: %q names more than one image: give each its own --visual", name)
		case seen[name]:
			return nil, usageErrorf("slice-visuals: two visuals are named %q: give each its own name", name)
		}
		seen[name] = true
		uri, err := visualPath(name, uri)
		if err != nil {
			return nil, err
		}
		items = append(items, store.VisualChange{Name: name, URI: uri})
	}
	return items, nil
}

// visualPath is where an image is filed as being: a path made absolute against
// the directory the command was typed in and refused if nothing is there, or
// a URI of any other scheme as given.
func visualPath(name, uri string) (string, error) {
	path, isFile := strings.CutPrefix(uri, "file://")
	if !isFile && uriScheme.MatchString(uri) {
		return uri, nil
	}
	if !filepath.IsAbs(path) {
		wd, err := getwd()
		if err != nil {
			return "", fmt.Errorf("slice-visuals: resolve %q against the working directory: %w", path, err)
		}
		path = filepath.Join(wd, path)
	}
	if _, err := os.Stat(path); err != nil {
		return "", usageErrorf("slice-visuals: %q: no image at %s: render it first, or hand in its absolute path", name, path)
	}
	return path, nil
}

// visualsFiledMarkdown says what was filed and that the agent carries on: the
// user reviews the images in the app, and any comments arrive as a message.
func visualsFiledMarkdown(s domain.Slice, items []store.VisualChange) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.Name)
	fmt.Fprintf(&b, "%s filed on the slice page for the user to review:\n\n", counted(len(items), "visual change"))
	for i, it := range items {
		fmt.Fprintf(&b, "%d. %s\n", i+1, it.Name)
	}
	b.WriteString("\nCarry on — the user's comments on them, if any, arrive as a message.\n")
	return b.String()
}
