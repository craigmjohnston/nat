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
// who, what each workflow state is, the teams, projects, epics, labels and
// iterations. Each method fetches only the parts it needs, all at once.
//
// epics are the epics looked up one by one (a badge's, a fact's, a segment's);
// epicList is the whole slim list a filter offers, read only from the cache
// on the sidebar's path — epicsLoading says it had nothing to read there yet,
// and a background fetch is filling it.
type refs struct {
	members      []shortcut.Member
	workflows    []shortcut.Workflow
	groups       []shortcut.Group
	projects     []shortcut.Project
	epics        []shortcut.Epic
	epicList     []shortcut.Epic
	epicsLoading bool
	labels       []shortcut.Label
	iterations   []shortcut.Iteration
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

// listed reads a whole workspace list through the long cache: fresh, the
// cached copy; else fetched and cached; a failed fetch falls back to a stale
// copy, else to nothing — a filter offering fewer choices, never a failed call.
func listed[T any](ctx context.Context, a *app, kind string, fetch func(context.Context) ([]T, error)) []T {
	c, scope := a.refCache(), a.refScope()
	body, fresh, ok := c.Get(scope, kind)
	if !fresh {
		if v, err := fetch(ctx); err == nil {
			c.Put(scope, kind, marshal(v))
			return v
		}
	}
	var v []T
	if ok {
		// Our own marshalled list: it decodes.
		_ = json.Unmarshal(body, &v)
	}
	return v
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

func (r *refs) project(id int64) (shortcut.Project, bool) {
	i := slices.IndexFunc(r.projects, func(p shortcut.Project) bool { return p.ID == id })
	if id == 0 || i < 0 {
		return shortcut.Project{}, false
	}
	return r.projects[i], true
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

// hexOr is c, lower-cased, where it is a #rrggbb colour, else fallback.
func hexOr(c, fallback string) string {
	if hexColor.MatchString(c) {
		return strings.ToLower(c)
	}
	return fallback
}

// projectCode is a Shortcut project's short tag: its abbreviation, else one
// made from its name as a team's is.
func projectCode(p shortcut.Project) string {
	if p.Abbreviation != "" {
		return p.Abbreviation
	}
	return code("", p.Name)
}

// badges is a story's one badge, its Shortcut project — else none: only a
// project draws as a badge.
func (r *refs) badges(st shortcut.Story) []source.Badge {
	if p, ok := r.project(st.ProjectID); ok {
		return []source.Badge{{Text: projectCode(p), Color: hexOr(p.Color, neutral), Title: p.Name}}
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

// merged is a segment's filter over the section's: each field the segment
// sets replaces the section's, and each it leaves empty ("Any") falls through
// to it — labels as a whole, so a segment with any label replaces the
// section's set, and one with none inherits it. The state is the segment's
// alone: the section has none.
func merged(section, segment settings.Filter) settings.Filter {
	f := section
	f.State = segment.State
	if segment.Team != "" {
		f.Team = segment.Team
	}
	if segment.Project != "" {
		f.Project = segment.Project
	}
	if segment.Epic != "" {
		f.Epic = segment.Epic
	}
	if len(segment.Labels) > 0 {
		f.Labels = segment.Labels
	}
	return f
}

// search is the one way a sidebar query is made: f's terms, then base (the
// group's own — `owner:me is:started`, `!is:done`, …). A team by mention name,
// a Shortcut project by id, an epic by name (the search takes an epic's
// title, quoted for an exact match; r.epics has it looked up by id, and the
// id itself stands in where it could not be), each label quoted, and a
// segment's workflow state by name, quoted (the search takes a state's name;
// r.workflows has it by id, the id itself standing in where it is not there).
func (r *refs) search(f settings.Filter, base string) string {
	var terms []string
	if f.Team != "" {
		terms = append(terms, "team:"+quoteSpaced(f.Team))
	}
	if f.Project != "" {
		terms = append(terms, "project:"+f.Project)
	}
	if f.Epic != "" {
		terms = append(terms, "epic:"+quoted(r.epicName(f.Epic)))
	}
	for _, l := range f.Labels {
		terms = append(terms, "label:"+quoted(l))
	}
	if f.State != "" {
		terms = append(terms, "state:"+quoted(r.stateName(f.State)))
	}
	return strings.Join(append(terms, base), " ")
}

// teamName is the display name of the team mention names, else mention.
func (r *refs) teamName(mention string) string {
	i := slices.IndexFunc(r.groups, func(g shortcut.Group) bool { return g.MentionName == mention })
	if i < 0 || r.groups[i].Name == "" {
		return mention
	}
	return r.groups[i].Name
}

// projectName is the name of the Shortcut project id names, else id itself.
func (r *refs) projectName(id string) string {
	if n, err := strconv.ParseInt(id, 10, 64); err == nil {
		if p, ok := r.project(n); ok && p.Name != "" {
			return p.Name
		}
	}
	return id
}

// stateLabel is a state as the filter editor labels it — its name, after its
// workflow's where the workspace has more than one — else id itself.
func (r *refs) stateLabel(id string) string {
	if n, err := strconv.ParseInt(id, 10, 64); err == nil {
		if s, w, ok := r.state(n); ok && s.Name != "" {
			return r.workflowLabel(w, s)
		}
	}
	return id
}

// workflowLabel is a state named as the editor offers it.
func (r *refs) workflowLabel(w shortcut.Workflow, s shortcut.WorkflowState) string {
	if len(r.workflows) > 1 {
		return w.Name + " › " + s.Name
	}
	return s.Name
}

// stateName is the name of the workflow state id names, else id itself.
func (r *refs) stateName(id string) string {
	if n, err := strconv.ParseInt(id, 10, 64); err == nil {
		if s, _, ok := r.state(n); ok && s.Name != "" {
			return s.Name
		}
	}
	return id
}

// epicName is the name of the epic id names, as looked up, else id itself.
func (r *refs) epicName(id string) string {
	if n, err := strconv.ParseInt(id, 10, 64); err == nil {
		if e, ok := r.epic(n); ok && e.Name != "" {
			return e.Name
		}
	}
	return id
}

// quoted is s as one quoted search value, any quote inside it dropped — the
// search has no escape for one.
func quoted(s string) string { return `"` + strings.ReplaceAll(s, `"`, "") + `"` }

// quoteSpaced is s quoted only where it holds a space, as a mention name
// never does.
func quoteSpaced(s string) string {
	if strings.ContainsAny(s, " \t") {
		return quoted(s)
	}
	return s
}

// filterFields are a filter editor — the section's (wider nil) or a
// segment's (wider the section's) — Team, Project, a segment's State, Epic
// and Labels, each offering the workspace's unarchived choices with f's own
// selection. State is a segment's alone (which stories it lists): every state
// of every workflow, labelled `<workflow> › <state>` where the workspace has
// more than one workflow, and inheriting nothing. A saved
// choice the workspace no longer offers is offered still, named as well as it
// can be, so opening and saving the editor never drops it. Where wider sets a
// field, the field says so (Inherited), since "Any" there means wider's
// choice. The epic list is only what the cache held: with nothing there yet
// the field says it is loading.
func (r *refs) filterFields(f settings.Filter, wider *settings.Filter) []source.FilterField {
	var teams, projects, epics, labels []source.FilterOption
	for _, g := range r.groups {
		if !g.Archived && g.MentionName != "" {
			teams = append(teams, source.FilterOption{ID: g.MentionName, Label: g.Name, Color: teamColor(g)})
		}
	}
	for _, p := range r.projects {
		if !p.Archived {
			projects = append(projects, source.FilterOption{ID: strconv.FormatInt(p.ID, 10), Label: p.Name, Color: hexOr(p.Color, neutral)})
		}
	}
	for _, e := range r.epicList {
		if !e.Archived {
			epics = append(epics, source.FilterOption{ID: strconv.FormatInt(e.ID, 10), Label: e.Name})
		}
	}
	for _, l := range r.labels {
		if !l.Archived {
			labels = append(labels, source.FilterOption{ID: l.Name, Label: l.Name, Color: hexOr(l.Color, "")})
		}
	}
	one := func(v string) []string {
		if v == "" {
			return []string{}
		}
		return []string{v}
	}
	labelValue := append([]string{}, f.Labels...)
	fields := []source.FilterField{
		{ID: "team", Label: "Team", Options: offering(teams, one(f.Team), nil), Value: one(f.Team)},
		{ID: "project", Label: "Project", Options: offering(projects, one(f.Project), nil), Value: one(f.Project)},
		{ID: "epic", Label: "Epic", Options: offering(epics, one(f.Epic), r.epicName), Value: one(f.Epic), Loading: r.epicsLoading},
		{ID: "labels", Label: "Labels", Multi: true, Options: offering(labels, labelValue, nil), Value: labelValue},
	}
	if wider == nil {
		return fields
	}
	named := func(options []source.FilterOption, ids []string) string {
		var names []string
		for _, id := range ids {
			i := slices.IndexFunc(options, func(o source.FilterOption) bool { return o.ID == id })
			if i < 0 {
				names = append(names, id)
			} else {
				names = append(names, options[i].Label)
			}
		}
		return strings.Join(names, ", ")
	}
	fields[0].Inherited = named(teams, one(wider.Team))
	fields[1].Inherited = named(projects, one(wider.Project))
	if wider.Epic != "" {
		fields[2].Inherited = r.epicName(wider.Epic)
	}
	fields[3].Inherited = named(labels, wider.Labels)
	var states []source.FilterOption
	for _, w := range r.workflows {
		for _, s := range w.States {
			states = append(states, source.FilterOption{ID: strconv.FormatInt(s.ID, 10), Label: r.workflowLabel(w, s)})
		}
	}
	state := source.FilterField{ID: "state", Label: "State", Options: offering(states, one(f.State), nil), Value: one(f.State)}
	return slices.Insert(fields, 2, state)
}

// filterAction is the Filter… action over fields.
func filterAction(fields []source.FilterField) source.Action {
	return source.Action{ID: "filter", Label: "Filter…", Input: source.InputFilter, Fields: fields}
}

// offering is options sorted by label, with every id in selected that it
// lacks added — labelled by name(id), else by the id itself.
func offering(options []source.FilterOption, selected []string, name func(string) string) []source.FilterOption {
	out := slices.Clone(options)
	for _, id := range selected {
		if !slices.ContainsFunc(out, func(o source.FilterOption) bool { return o.ID == id }) {
			label := id
			if name != nil {
				label = name(id)
			}
			out = append(out, source.FilterOption{ID: id, Label: label})
		}
	}
	slices.SortStableFunc(out, func(x, y source.FilterOption) int {
		return strings.Compare(strings.ToLower(x.Label), strings.ToLower(y.Label))
	})
	if out == nil {
		out = []source.FilterOption{}
	}
	return out
}
