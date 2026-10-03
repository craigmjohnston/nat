package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/craigmjohnston/nat/internal/source"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/cache"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/settings"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/shortcut"
)

// refs is the workspace's reference data a response is drawn with: who's
// who, what each workflow state is, the teams, epics and iterations. Each
// method fetches only the parts it needs, all at once.
type refs struct {
	members    []shortcut.Member
	workflows  []shortcut.Workflow
	groups     []shortcut.Group
	epics      []shortcut.Epic
	iterations []shortcut.Iteration
}

// parallel runs fns at once and returns the first error in argument order,
// so the line nat shows doesn't depend on which request lost a race.
func parallel(fns ...func() error) error {
	errs := make([]error, len(fns))
	var wg sync.WaitGroup
	for i, f := range fns {
		wg.Go(func() { errs[i] = f() })
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// refTTL is how long a looked-up epic or iteration is trusted. Their names
// barely change, and the alternative is a request per story per sidebar.
const refTTL = time.Hour

// refScope is the cache "project" workspace lookups are kept under: shared
// by every nat project on the same API, and untouched by an action's drop of
// one project's responses (Refresh drops it explicitly).
func (a *app) refScope() string { return "workspace " + a.sc.BaseURL }

func (a *app) refCache() cache.Cache {
	return cache.Cache{Dir: a.cache.Dir, TTL: refTTL, Now: a.env.Now}
}

// lookup resolves ids (zeros and repeats skipped) with one GET each, all at
// once, through the long cache. A lookup that fails falls back to a stale
// entry, else is left out: a name it couldn't read is an empty fact, never a
// failed call.
func lookup[T any](ctx context.Context, a *app, kind string, ids []int64, fetch func(context.Context, int64) (T, error)) []T {
	ids = slices.DeleteFunc(slices.Clone(ids), func(id int64) bool { return id == 0 })
	slices.Sort(ids)
	ids = slices.Compact(ids)
	c, scope := a.refCache(), a.refScope()
	got := make([]*T, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Go(func() {
			key := kind + ":" + strconv.FormatInt(id, 10)
			body, fresh, ok := c.Get(scope, key)
			var v T
			if !fresh {
				fetched, err := fetch(ctx, id)
				if err == nil {
					c.Put(scope, key, marshal(fetched))
					got[i] = &fetched
					return
				}
			}
			if ok {
				// Our own marshalled T: it decodes.
				_ = json.Unmarshal(body, &v)
				got[i] = &v
			}
		})
	}
	wg.Wait()
	var out []T
	for _, v := range got {
		if v != nil {
			out = append(out, *v)
		}
	}
	return out
}

// into is a fetch for parallel: f's answer lands in dst.
func into[T any](ctx context.Context, dst *T, f func(context.Context) (T, error)) func() error {
	return func() error {
		v, err := f(ctx)
		*dst = v
		return err
	}
}

// state is the workflow state id names, and the workflow it belongs to.
func (r *refs) state(id int64) (shortcut.WorkflowState, shortcut.Workflow, bool) {
	for _, w := range r.workflows {
		for _, s := range w.States {
			if s.ID == id {
				return s, w, true
			}
		}
	}
	return shortcut.WorkflowState{}, shortcut.Workflow{}, false
}

func (r *refs) stateType(id int64) string {
	s, _, _ := r.state(id)
	return s.Type
}

func (r *refs) group(id string) (shortcut.Group, bool) {
	i := slices.IndexFunc(r.groups, func(g shortcut.Group) bool { return g.ID == id })
	if id == "" || i < 0 {
		return shortcut.Group{}, false
	}
	return r.groups[i], true
}

func (r *refs) epic(id int64) (shortcut.Epic, bool) {
	i := slices.IndexFunc(r.epics, func(e shortcut.Epic) bool { return e.ID == id })
	if id == 0 || i < 0 {
		return shortcut.Epic{}, false
	}
	return r.epics[i], true
}

func (r *refs) iteration(id int64) string {
	i := slices.IndexFunc(r.iterations, func(it shortcut.Iteration) bool { return it.ID == id })
	if id == 0 || i < 0 {
		return ""
	}
	return r.iterations[i].Name
}

// name is a member's display name: profile name, else mention name, else
// "someone" (a deactivated member, or one this token can't see).
func (r *refs) name(id string) string {
	i := slices.IndexFunc(r.members, func(m shortcut.Member) bool { return m.ID == id })
	switch {
	case id == "" || i < 0:
		return "someone"
	case r.members[i].Profile.Name != "":
		return r.members[i].Profile.Name
	case r.members[i].Profile.MentionName != "":
		return r.members[i].Profile.MentionName
	}
	return "someone"
}

// neutral is the badge and fact colour where Shortcut gives none.
const neutral = "#8e8e93"

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// colorKeys maps Shortcut's named team colours (a team's color_key) to hex —
// mid-tones, since gnat draws them behind light text. Shortcut publishes
// names, not values; these are chosen to read as the name.
var colorKeys = map[string]string{
	"red":           "#d64545",
	"orange":        "#e8833a",
	"yellow":        "#d9b51c",
	"yellow-green":  "#8fb738",
	"green":         "#3fa45b",
	"turquoise":     "#2aa198",
	"sky-blue":      "#3fa7d6",
	"blue":          "#3d6fd9",
	"midnight-blue": "#2c3e7a",
	"purple":        "#8656c9",
	"fuchsia":       "#c2479c",
	"pink":          "#e06c9f",
	"brass":         "#b08d3e",
	"slate":         "#607d8b",
	"gray":          neutral,
	"grey":          neutral,
	"black":         "#333333",
}

// teamColor is a team's colour: its hex color when Shortcut gives one (it
// usually gives null), else its color_key through colorKeys, else neutral.
func teamColor(g shortcut.Group) string {
	if hexColor.MatchString(g.Color) {
		return strings.ToLower(g.Color)
	}
	if c, ok := colorKeys[strings.ToLower(g.ColorKey)]; ok {
		return c
	}
	return neutral
}

var wordSep = regexp.MustCompile(`[^A-Za-z0-9]+`)

// code is a team's short tag: the initials of a multi-word mention name (up
// to three — "native-app" is NA), else the first two letters of a one-word
// one ("search" is SE). The name stands in for a missing mention name.
func code(mention, name string) string {
	src := mention
	if src == "" {
		src = name
	}
	var words []string
	for _, w := range wordSep.Split(src, -1) {
		if w != "" {
			words = append(words, w)
		}
	}
	switch {
	case len(words) == 0:
		return ""
	case len(words) == 1:
		w := []rune(words[0])
		return strings.ToUpper(string(w[:min(2, len(w))]))
	}
	var b strings.Builder
	for _, w := range words[:min(3, len(words))] {
		b.WriteRune([]rune(strings.ToUpper(w))[0])
	}
	return b.String()
}

// badges is a story's one badge: its team, else its epic, else none.
func (r *refs) badges(st shortcut.Story) []source.Badge {
	if g, ok := r.group(st.GroupID); ok {
		return []source.Badge{{Text: code(g.MentionName, g.Name), Color: teamColor(g), Title: g.Name}}
	}
	if e, ok := r.epic(st.EpicID); ok {
		c := neutral
		if g, ok := r.group(e.GroupID); ok {
			c = teamColor(g)
		}
		return []source.Badge{{Text: code("", e.Name), Color: c, Title: e.Name}}
	}
	return nil
}

// estimate is "1 pt", "3 pts", or "" when unset.
func estimate(e *int64) string {
	switch {
	case e == nil:
		return ""
	case *e == 1:
		return "1 pt"
	}
	return fmt.Sprintf("%d pts", *e)
}

// cardMenu is every story row's and story detail's menu.
var cardMenu = []source.Action{
	{ID: "assign", Label: "Assign to Me", Input: source.InputNone},
	{ID: "follow", Label: "Follow", Input: source.InputNone},
}

// row is a story as a sidebar container.
func (r *refs) row(st shortcut.Story) source.Container {
	return source.Container{
		ID:          strconv.FormatInt(st.ID, 10),
		Title:       st.Name,
		ExternalURL: st.AppURL,
		Badges:      r.badges(st),
		Meta:        estimate(st.Estimate),
		Menu:        cardMenu,
	}
}

// ago is t relative to now, as a comment or fact shows it: "just now",
// "5m ago", "3h ago", "2d ago", then a date ("18 Sep", or "18 Sep 2025" in
// another year). The zero time is "—".
func ago(now, t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	t = t.In(now.Location())
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	case t.Year() == now.Year():
		return t.Format("2 Jan")
	}
	return t.Format("2 Jan 2006")
}

// storyID parses a container id, which is always a story id.
func storyID(id string) (int64, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("shortcut: %q is not a story id", id)
	}
	return n, nil
}

// story reads one story, a 404 worded as one.
func (a *app) story(ctx context.Context, id int64) (shortcut.Story, error) {
	st, err := a.sc.Story(ctx, id)
	return st, notFound(err, id)
}

// pickState is the state of typ a story in workflow w moves to: the
// project's override when set (a state name or id in w), else the first
// state of that type by position.
func pickState(w shortcut.Workflow, typ, override string) (shortcut.WorkflowState, error) {
	if override != "" {
		for _, s := range w.States {
			if strings.EqualFold(s.Name, override) || strconv.FormatInt(s.ID, 10) == override {
				return s, nil
			}
		}
		return shortcut.WorkflowState{}, fmt.Errorf("shortcut: workflow %q has no state %q", w.Name, override)
	}
	states := slices.Clone(w.States)
	slices.SortStableFunc(states, func(a, b shortcut.WorkflowState) int { return int(a.Position - b.Position) })
	for _, s := range states {
		if s.Type == typ {
			return s, nil
		}
	}
	return shortcut.WorkflowState{}, fmt.Errorf("shortcut: workflow %q has no %s state", w.Name, typ)
}

// withMe is query with every whole `owner:me` term (any case, negated or
// not) naming mention instead; unchanged when mention is unknown.
func withMe(query, mention string) string {
	if mention == "" {
		return query
	}
	terms := strings.Split(query, " ")
	for i, t := range terms {
		neg := ""
		if strings.HasPrefix(t, "!") || strings.HasPrefix(t, "-") {
			neg, t = t[:1], t[1:]
		}
		if strings.EqualFold(t, "owner:me") {
			terms[i] = neg + "owner:" + mention
		}
	}
	return strings.Join(terms, " ")
}

// withTeam is query restricted to the project's team, if one is set.
func withTeam(query string, p *settings.Project) string {
	if p.Team == "" {
		return query
	}
	t := p.Team
	if strings.ContainsAny(t, " \t") {
		t = `"` + t + `"`
	}
	return query + " team:" + t
}
