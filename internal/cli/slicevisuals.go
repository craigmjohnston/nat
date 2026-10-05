package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/store"
)

// sliceVisuals files the images an agent rendered of what its slice changed,
// for the user to review in the app. Unlike follow-ups nothing waits on them:
// the agent carries on, and the user's comments, if any, reach it as a message.
//
// A hand-in is incremental: --visual adds an image or replaces the one filed
// under its name, --before gives an image the one it is best judged against,
// and --remove drops one. The set filed is still the whole of it, worked out
// here from the last one filed (see [handIn.apply]), since the last section is
// the one that wins.
//
// Only a slice this user holds takes them — or one of theirs that is Done with
// its pull request still out; see [canHandInVisuals].
func sliceVisuals(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-visuals", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var given, befores, removes stringList
	flags.Var(&given, "visual", "an image: its first line what it shows, the next its path or URI; repeat for more")
	flags.Var(&befores, "before", "the before of a visual: its first line the visual's name, the next the before's path or URI")
	flags.Var(&removes, "remove", "the name of a visual to drop; repeat for more")
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
	h, err := handInOf(given, befores, removes)
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
	if !canHandInVisuals(s, shape.On(pageShape), cfg.AssigneeUserID) {
		return visualsRefusal(s, cfg.AssigneeUserName)
	}
	body, err := st.Body(ctx, s.ID)
	if err != nil {
		return fmt.Errorf("read the slice for its visual changes: %w", err)
	}
	items, report, err := h.apply(store.VisualChanges(body))
	if err != nil {
		return err
	}

	if err := st.RecordVisuals(ctx, s.ID, items); err != nil {
		return fmt.Errorf("file the visual changes: %w", err)
	}
	env.nudged()
	_, err = io.WriteString(env.Out, visualsFiledMarkdown(s, items, report))
	return err
}

// canHandInVisuals says whether the caller's agent may hand images in on s: a
// slice they hold, as complete-slice asks — the agent working it is the one
// with something to hand in — or one of theirs that is Done with a pull
// request recorded: a session that outlived its slice's merge, whose renders
// are still review material. Whether that pull request is still open is not
// asked of gh here: a read that fails concludes nothing.
func canHandInVisuals(s domain.Slice, sh store.Shape, userID string) bool {
	if store.Holds(s, sh, userID) {
		return true
	}
	if s.Status != domain.SliceDone || s.PRURL == "" {
		return false
	}
	return !sh.HasAssignee || slices.Contains(s.AssigneeIDs, userID)
}

// visualsRefusal says why a slice takes no visual changes from this user, and
// what would.
func visualsRefusal(s domain.Slice, assignee string) error {
	held := ""
	switch {
	case s.AssigneeName == "":
		held = ", held by nobody"
	case s.AssigneeName != assignee:
		held = ", held by " + s.AssigneeName
	}
	return fmt.Errorf("%q is %s%s: visual changes can be given only to a slice you hold, "+
		"or one of yours that is Done with its pull request still open", s.Name, blank(s.StatusName), held)
}

// uriScheme is the scheme a URI opens with, which is what tells one apart from
// a path on this machine.
var uriScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)

// handIn is one slice-visuals command, read and checked: the images given,
// the befores given, and the names removed.
type handIn struct {
	visuals []store.VisualChange
	befores []store.VisualChange
	removes []string
}

// handInOf reads the command's flags, settling every refusal that needs no
// read of the slice before anything is read or written: nothing given at all,
// an image with no name or no URI, a URI over more than one line, a name given
// twice to one flag, a name both removed and given, and a path that names no
// file. A path — bare, or a file:// URI — is filed absolute, with the sha256 of
// its bytes as they are now, so the app can open it from wherever it runs and
// tell a re-render at the same path from the image it replaces; any other URI
// is filed as given, with no hash.
func handInOf(given, befores, removes []string) (handIn, error) {
	if len(given)+len(befores)+len(removes) == 0 {
		return handIn{}, usageErrorf("slice-visuals: no visual given: pass --visual '<what it shows>\\n<absolute path to the image>', " +
			"--before '<its name>\\n<absolute path to the before>' or --remove '<its name>'")
	}
	var h handIn
	var err error
	if h.visuals, err = imagesOf(given, "visual", "visuals"); err != nil {
		return handIn{}, err
	}
	if h.befores, err = imagesOf(befores, "before", "befores"); err != nil {
		return handIn{}, err
	}
	givenNames := map[string]bool{}
	for _, it := range append(slices.Clone(h.visuals), h.befores...) {
		givenNames[it.Name] = true
	}
	seen := map[string]bool{}
	for _, r := range removes {
		name := strings.TrimSpace(r)
		switch {
		case name == "":
			return handIn{}, usageErrorf("slice-visuals: a --remove names nothing: give the name of the visual to drop")
		case seen[name]:
			return handIn{}, usageErrorf("slice-visuals: %q is removed twice: name each visual once", name)
		case givenNames[name]:
			return handIn{}, usageErrorf("slice-visuals: %q is both removed and given: an image handed in under a name already filed replaces it, so do not remove one you are updating", name)
		}
		seen[name] = true
		h.removes = append(h.removes, name)
	}
	return h, nil
}

// imagesOf reads each value of one flag as a name over a path or URI. what is
// the flag's name and whats its plural, for the refusals.
func imagesOf(given []string, what, whats string) ([]store.VisualChange, error) {
	seen := map[string]bool{}
	items := make([]store.VisualChange, 0, len(given))
	for _, g := range given {
		name, uri, _ := strings.Cut(strings.TrimSpace(strings.ReplaceAll(g, "\r\n", "\n")), "\n")
		name, uri = strings.TrimSpace(name), strings.TrimSpace(uri)
		switch {
		case name == "":
			return nil, usageErrorf("slice-visuals: a --%s has no name: its first line names the visual", what)
		case uri == "":
			return nil, usageErrorf("slice-visuals: %q has no image: give its path on the line after the name", name)
		case strings.Contains(uri, "\n"):
			return nil, usageErrorf("slice-visuals: %q names more than one image: give each its own --%s", name, what)
		case seen[name]:
			return nil, usageErrorf("slice-visuals: two %s are named %q: give each its own name", whats, name)
		}
		seen[name] = true
		uri, hash, err := visualPath(name, uri)
		if err != nil {
			return nil, err
		}
		items = append(items, store.VisualChange{Name: name, URI: uri, Hash: hash})
	}
	return items, nil
}

// visualsReport is what a hand-in did to the set filed, by name.
type visualsReport struct {
	added, updated, removed []string
}

// apply works the hand-in into the set last filed, current, and is the set to
// file: removals dropped (each before with it), each --visual replacing the
// image of its name in place — keeping the before it has — or going at the
// end, then each --before set on the visual of its name. A --remove naming
// nothing filed, or a --before naming nothing in the set that results, is
// refused, listing what is filed.
func (h handIn) apply(current []store.VisualChange) ([]store.VisualChange, visualsReport, error) {
	var report visualsReport
	items := make([]store.VisualChange, 0, len(current)+len(h.visuals))
	for _, it := range current {
		items = append(items, store.VisualChange{Name: it.Name, URI: it.URI, Hash: it.Hash, BeforeURI: it.BeforeURI, BeforeHash: it.BeforeHash})
	}
	for _, name := range h.removes {
		i := visualIndex(items, name)
		if i < 0 {
			return nil, report, usageErrorf("slice-visuals: --remove %q names no visual filed: %s", name, filedNames(items))
		}
		items = slices.Delete(items, i, i+1)
		report.removed = append(report.removed, name)
	}
	for _, v := range h.visuals {
		if i := visualIndex(items, v.Name); i >= 0 {
			items[i].URI, items[i].Hash = v.URI, v.Hash
			report.updated = append(report.updated, v.Name)
			continue
		}
		items = append(items, v)
		report.added = append(report.added, v.Name)
	}
	for _, b := range h.befores {
		i := visualIndex(items, b.Name)
		if i < 0 {
			return nil, report, usageErrorf("slice-visuals: --before %q names no visual: %s, or give it with --visual in the same command", b.Name, filedNames(items))
		}
		items[i].BeforeURI, items[i].BeforeHash = b.URI, b.Hash
		if !slices.Contains(report.added, b.Name) && !slices.Contains(report.updated, b.Name) {
			report.updated = append(report.updated, b.Name)
		}
	}
	return items, report, nil
}

// visualIndex is where the visual named name is in items, or -1.
func visualIndex(items []store.VisualChange, name string) int {
	return slices.IndexFunc(items, func(it store.VisualChange) bool { return it.Name == name })
}

// filedNames says which visuals a refusal could have named.
func filedNames(items []store.VisualChange) string {
	if len(items) == 0 {
		return "none is filed"
	}
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.Name
	}
	return "filed are " + strings.Join(quoteAll(names), ", ")
}

// visualPath is where an image is filed as being, and the sha256 of its bytes:
// a path made absolute against the directory the command was typed in and
// refused if nothing is there, or a URI of any other scheme as given, with no
// hash.
func visualPath(name, uri string) (string, string, error) {
	path, isFile := strings.CutPrefix(uri, "file://")
	if !isFile && uriScheme.MatchString(uri) {
		return uri, "", nil
	}
	if !filepath.IsAbs(path) {
		wd, err := getwd()
		if err != nil {
			return "", "", fmt.Errorf("slice-visuals: resolve %q against the working directory: %w", path, err)
		}
		path = filepath.Join(wd, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", usageErrorf("slice-visuals: %q: no image at %s: render it first, or hand in its absolute path", name, path)
	}
	sum := sha256.Sum256(data)
	return path, hex.EncodeToString(sum[:]), nil
}

// visualsFiledMarkdown says what the hand-in did and the set as it now
// stands, and that the agent carries on: the user reviews the images in the
// app, and any comments arrive as a message.
func visualsFiledMarkdown(s domain.Slice, items []store.VisualChange, report visualsReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.Name)
	b.WriteString("Visual changes filed on the slice page for the user to review.\n\n")
	for _, line := range []struct {
		what  string
		names []string
	}{{"Added", report.added}, {"Updated", report.updated}, {"Removed", report.removed}} {
		if len(line.names) > 0 {
			fmt.Fprintf(&b, "%s: %s\n", line.what, strings.Join(quoteAll(line.names), ", "))
		}
	}
	if len(items) == 0 {
		b.WriteString("\nNo visual changes are filed now.\n")
	} else {
		fmt.Fprintf(&b, "\nAs it now stands, %s:\n\n", counted(len(items), "visual change"))
		for i, it := range items {
			pair := ""
			if it.BeforeURI != "" {
				pair = ", with a before"
			}
			fmt.Fprintf(&b, "%d. %s%s\n", i+1, it.Name, pair)
		}
	}
	b.WriteString("\nCarry on — the user's comments on them, if any, arrive as a message.\n")
	return b.String()
}
