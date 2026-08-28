package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// sliceDepends records what a slice waits on: the slices that must be Done
// before it can be handed to an agent. --on names them, one flag each, by URL
// or page ID like every other command takes a slice; --clear drops what is
// there first, so --clear on its own frees the slice and --clear with --on
// replaces the list outright.
//
// Every named slice is read before anything is written. A dependency on a page
// nobody can fetch would be a wait with no end, and the point of reading them
// is to refuse that here rather than discover it the next time work is handed
// out.
func sliceDepends(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-depends", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var on stringList
	flags.Var(&on, "on", "a slice this one waits on, by URL or ID; repeat for more")
	clear := flags.Bool("clear", false, "drop the dependencies already recorded before adding any")
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-depends: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	if len(on) == 0 && !*clear {
		return usageErrorf("slice-depends: nothing to record: pass --on, or --clear to drop what is there")
	}
	id, err := pageID("slice-depends", rest[0])
	if err != nil {
		return err
	}
	onIDs := make([]string, len(on))
	for i, ref := range on {
		if onIDs[i], err = pageID("slice-depends", ref); err != nil {
			return err
		}
	}

	if _, _, _, err := env.projectFor(*projectRef); err != nil {
		return err
	}
	client := env.NewClient(env.Tokens.Token)

	page, err := client.GetPage(ctx, id)
	if err != nil {
		return fmt.Errorf("load the slice: %w", err)
	}
	s := domain.SliceFromPage(*page)

	kept := s.DependsOn
	if *clear {
		kept = nil
	}
	wanted, err := dependencyIDs(ctx, client, s, kept, onIDs)
	if err != nil {
		return err
	}

	updated, err := client.UpdatePageProperties(ctx, page.ID,
		map[string]notion.PropertyValue{notion.PropDependsOn: notion.NewRelation(wanted...)})
	if err != nil {
		return fmt.Errorf("record the dependencies: %w", err)
	}
	env.nudged()
	s = domain.SliceFromPage(*updated)
	logging.Action("slice dependencies recorded", "slice", s.ID, "depends_on", len(s.DependsOn))

	deps := dependencyIndex(ctx, client, s)
	if *asJSON {
		return writeJSON(env.Out, dependsJSON{Slice: dependsSliceJSON{
			ID: s.ID, Name: s.Name, Status: s.StatusName, URL: s.URL,
			DependsOn: dependencyList(s, deps),
		}})
	}
	_, err = io.WriteString(env.Out, dependsMarkdown(s, deps))
	return err
}

// dependencyIDs settles the list to write: what is being kept, then what --on
// added, each named page checked as it goes and named at most once however
// often it was given.
//
// A slice cannot depend on itself. Notion accepts the relation happily, and
// what it makes is a slice no run of next-slice or start-slice could ever hand
// out again.
func dependencyIDs(ctx context.Context, client API, s domain.Slice, kept, added []string) ([]string, error) {
	var ids []string
	seen := map[string]bool{}
	for _, id := range append(append([]string{}, kept...), added...) {
		key := domain.NormaliseID(id)
		if seen[key] {
			continue
		}
		seen[key] = true
		ids = append(ids, id)
	}
	self := domain.NormaliseID(s.ID)
	for _, id := range added {
		if domain.NormaliseID(id) == self {
			return nil, fmt.Errorf("%q cannot depend on itself: it would never be unblocked", s.Name)
		}
		if _, err := client.GetPage(ctx, id); err != nil {
			return nil, fmt.Errorf("load the slice %s depends on: %w", s.Name, err)
		}
	}
	return ids, nil
}

// dependencyIndex reads the slices s waits on, one page each, so they can be
// named and their statuses read. A page that cannot be read is logged and left
// out — the same rule the blocking itself follows, since a dependency nobody
// can see must not wedge the plan.
func dependencyIndex(ctx context.Context, client API, s domain.Slice) map[string]domain.Slice {
	var loaded []domain.Slice
	for _, id := range s.DependsOn {
		page, err := client.GetPage(ctx, id)
		if err != nil {
			logging.Action("dependency could not be read", "slice", s.ID, "dependency", id, "error", err.Error())
			continue
		}
		loaded = append(loaded, domain.SliceFromPage(*page))
	}
	return domain.SlicesByID(loaded)
}

// dependencyList is the slices s names, resolved where they could be read, in
// the order the relation lists them. A dependency the index does not hold is
// reported by ID alone rather than dropped: it is on the slice in Notion, and
// somebody looking at the output should see that it is there and unreadable.
func dependencyList(s domain.Slice, byID map[string]domain.Slice) []dependencyJSON {
	out := make([]dependencyJSON, 0, len(s.DependsOn))
	for _, id := range s.DependsOn {
		dep, ok := byID[domain.NormaliseID(id)]
		if !ok {
			out = append(out, dependencyJSON{ID: id})
			continue
		}
		out = append(out, dependencyJSON{ID: dep.ID, Name: dep.Name, Status: dep.StatusName, URL: dep.URL})
	}
	return out
}

// blockedError says why a slice will not be started: what it waits on, and what
// state each of those is in, which is the whole of what somebody has to look at
// to know when it will be workable.
func blockedError(s domain.Slice, blockers []domain.Slice) error {
	return fmt.Errorf("%q waits on %d unfinished %s: %s",
		s.Name, len(blockers), plural("slice", len(blockers)), blockerList(blockers))
}

// blockerList names the slices standing in the way, each with the status it is
// actually in.
func blockerList(blockers []domain.Slice) string {
	parts := make([]string, len(blockers))
	for i, b := range blockers {
		parts[i] = fmt.Sprintf("%q (%s)", b.Name, blank(b.StatusName))
	}
	return strings.Join(parts, ", ")
}

// stringList is a flag that may be given more than once, gathering every value
// in the order they arrived — which is how --on and --depends-on name more than
// one slice.
type stringList []string

// String implements flag.Value.
func (l *stringList) String() string { return strings.Join(*l, ", ") }

// Set implements flag.Value.
func (l *stringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// dependsJSON is the structured form of what a slice now waits on.
type dependsJSON struct {
	Slice dependsSliceJSON `json:"slice"`
}

type dependsSliceJSON struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Status    string           `json:"status"`
	URL       string           `json:"url"`
	DependsOn []dependencyJSON `json:"depends_on"`
}

// dependencyJSON is one slice waited on. Name and status are empty for a
// dependency whose page could not be read, which is how that shows.
type dependencyJSON struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	URL    string `json:"url"`
}

// dependsMarkdown reports what the slice now waits on, and says plainly when it
// waits on nothing — a cleared slice printing an empty list reads as output cut
// short.
func dependsMarkdown(s domain.Slice, byID map[string]domain.Slice) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.Name)
	if len(s.DependsOn) == 0 {
		b.WriteString("Waits on nothing.\n")
		return b.String()
	}
	blocking, _ := domain.Blockers(s, byID)
	if len(blocking) == 0 {
		fmt.Fprintf(&b, "Waits on %d %s, all Done — it is not blocked.\n\n",
			len(s.DependsOn), plural("slice", len(s.DependsOn)))
	} else {
		fmt.Fprintf(&b, "Blocked: %d unfinished of the %d %s it waits on.\n\n",
			len(blocking), len(s.DependsOn), plural("slice", len(s.DependsOn)))
	}
	for _, d := range dependencyList(s, byID) {
		if d.Name == "" {
			fmt.Fprintf(&b, "- %s — could not be read\n", d.ID)
			continue
		}
		fmt.Fprintf(&b, "- %s — %s\n", d.Name, blank(d.Status))
	}
	return b.String()
}
