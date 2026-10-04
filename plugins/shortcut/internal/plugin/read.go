package plugin

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/source"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/settings"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/shortcut"
)

// iconSVG is the Shortcut mark from the mock (gsc-shell.jsx's `I`), one
// colour, painted with currentColor so gnat can draw it as a template image.
const iconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48"><path fill="currentColor" fill-rule="evenodd" clip-rule="evenodd" d="M18.2765 8.46875H39.8392L30.0769 19.183L39.652 28.7301L29.7873 39.5561L8.15918 39.5506L17.9624 28.7915L8.42517 19.2828L18.2765 8.46875ZM19.7228 30.5467L13.8141 37.0315L26.2301 37.0346L19.7228 30.5467ZM29.2139 36.498L21.3993 28.7067L28.4005 21.0229L36.2151 28.8147L29.2139 36.498ZM26.6401 19.2677L19.6388 26.9516L11.8619 19.1979L18.8627 11.5129L26.6401 19.2677ZM28.3166 17.4277L34.183 10.9893H21.8593L28.3166 17.4277Z"/></svg>`

// describeResponse is who the plugin is, and the one thing it needs set up —
// the token, with whether one is there (tokenSet). It needs no project, no
// token and no API call, so a plugin with no token still says what it wants.
func describeResponse(tokenSet bool) source.Describe {
	return source.Describe{
		Protocol:      source.ProtocolVersion,
		Name:          "shortcut",
		Title:         "Shortcut",
		Tag:           "SC",
		IconSymbol:    "rectangle.on.rectangle.angled",
		IconSVG:       iconSVG,
		ContainerNoun: "card",
		TaskNoun:      "task",
		Menu: []source.Action{
			{ID: "refresh", Label: "Refresh", Input: source.InputNone},
			{ID: "new-segment", Label: "New Segment…", Input: source.InputText},
		},
		Setup: []source.SetupField{
			{ID: tokenField, Label: "API token", Input: source.InputSecret, Hint: "Shortcut ▸ Settings ▸ API Tokens", Set: &tokenSet},
		},
	}
}

// describe says whether a token is there by presence alone — the
// environment's, else a Keychain item, found as Token would find it — never
// reading the Keychain's secret; a lookup that fails is simply no token.
func (a *app) describe(context.Context) ([]byte, error) {
	set := a.env.Getenv("SHORTCUT_API_TOKEN") != "" || a.env.Tokens.Has()
	return marshal(describeResponse(set)), nil
}

// segmentMenu is a segment's menu: Rename, the filter editor — opened on the
// segment's own filter over the section's, its options the workspace's — and
// Remove.
func (r *refs) segmentMenu(f, section settings.Filter) []source.Action {
	return []source.Action{
		{ID: "rename", Label: "Rename…", Input: source.InputText},
		filterAction(r.filterFields(f, &section)),
		{ID: "remove", Label: "Remove Segment", Input: source.InputNone, Destructive: true},
	}
}

// sectionMenu is the section header's menu for this project: describe's own,
// then the section's filter editor, which narrows every search.
func (r *refs) sectionMenu(f settings.Filter) []source.Action {
	return append(slices.Clone(describeResponse(false).Menu), filterAction(r.filterFields(f, nil)))
}

// The most stories a group reads, and the most Done shows.
const (
	groupMax = 100
	doneShow = 25
)

func (a *app) sidebar(ctx context.Context) ([]byte, error) {
	expand := slices.Clone(a.req.Expand)
	slices.Sort(expand)
	return a.cached("sidebar:"+strings.Join(expand, ","), func() (any, error) {
		return a.buildSidebar(ctx, slices.Contains(expand, "done"))
	})
}

// buildSidebar reads who the token is, the workflows and the names of the
// segments' epics (a segment is searched by an epic's name and a state's),
// then everything else the tree needs in one parallel round — teams,
// projects, labels and every group's search — and draws it. The epic list the
// filter editor offers is never fetched here: see cachedEpics.
func (a *app) buildSidebar(ctx context.Context, doneOpen bool) (sidebarResponse, error) {
	_, proj, err := a.settings()
	if err != nil {
		return sidebarResponse{}, err
	}
	var segEpics []int64
	for _, f := range append([]settings.Filter{proj.Filter}, segmentFilters(proj)...) {
		if id, err := strconv.ParseInt(f.Epic, 10, 64); err == nil {
			segEpics = append(segEpics, id)
		}
	}
	// Shortcut's search takes `owner:me` without complaint and matches
	// nothing, so every query goes out with the token's own mention name in
	// its place.
	var me shortcut.MemberInfo
	var r refs
	if err := parallel(
		func() (err error) { me, err = a.sc.Me(ctx); return err },
		into(ctx, &r.workflows, a.sc.Workflows),
		func() error { r.epics = lookup(ctx, a, "epic", segEpics, a.sc.Epic); return nil },
	); err != nil {
		return sidebarResponse{}, err
	}
	r.epicList, r.epicsLoading = a.cachedEpics()
	query := func(f settings.Filter, base string) string { return withMe(r.search(f, base), me.MentionName) }
	var doing, done []shortcut.Story
	var doneTotal int
	segs := make([][]shortcut.Story, len(proj.Segments))
	fns := []func() error{
		into(ctx, &r.groups, a.sc.Groups),
		// Projects are a badge and a filter's options, nothing the tree
		// needs to be drawn: a workspace that won't list them (Shortcut
		// calls them deprecated) draws no badges.
		func() error { r.projects, _ = a.sc.Projects(ctx); return nil },
		func() error { r.labels = listed(ctx, a, "labels", a.sc.Labels); return nil },
		func() (err error) {
			doing, _, err = a.sc.Search(ctx, query(proj.Filter, "owner:me is:started"), groupMax)
			return err
		},
		func() (err error) {
			// Closed, only the count is wanted: one result is the smallest
			// page that still carries Shortcut's total.
			n := 1
			if doneOpen {
				n = groupMax
			}
			done, doneTotal, err = a.sc.Search(ctx, query(proj.Filter, "owner:me is:done completed:"+weekStart(a.now)+"..*"), n)
			return err
		},
	}
	for i, s := range proj.Segments {
		fns = append(fns, func() (err error) {
			segs[i], _, err = a.sc.Search(ctx, query(merged(proj.Filter, s.Filter), "!is:done"), groupMax)
			return err
		})
	}
	if err := parallel(fns...); err != nil {
		return sidebarResponse{}, err
	}
	byPosition := func(x, y shortcut.Story) int { return compare(x.Position, y.Position) }

	slices.SortStableFunc(doing, byPosition)
	doingGroup := source.Group{ID: "doing", Label: "Doing", Count: count(len(doing))}
	for _, st := range doing {
		doingGroup.Containers = append(doingGroup.Containers, r.row(st))
	}

	// Each segment is a top-level group of its own between Doing and Done;
	// its id is still `ready/<id>`, which gnat remembers folds by.
	groups := []source.Group{doingGroup}
	for i, s := range proj.Segments {
		g := source.Group{ID: s.GroupID(), Label: s.Name, Menu: r.segmentMenu(s.Filter, proj.Filter)}
		// A segment lists unstarted stories — unless it names its state, in
		// which case the user said exactly which, and the search has it.
		stories := segs[i]
		if s.Filter.State == "" {
			stories = slices.DeleteFunc(stories, func(st shortcut.Story) bool {
				return r.stateType(st.WorkflowStateID) != shortcut.StateUnstarted
			})
		}
		slices.SortStableFunc(stories, byPosition)
		for _, st := range stories {
			g.Containers = append(g.Containers, r.row(st))
		}
		g.Count = count(len(stories))
		groups = append(groups, g)
	}

	doneGroup := source.Group{ID: "done", Label: "Done", Count: count(doneTotal), Lazy: true}
	if doneOpen {
		slices.SortStableFunc(done, func(x, y shortcut.Story) int { return y.CompletedAt.Compare(x.CompletedAt.Time) })
		for _, st := range done[:min(doneShow, len(done))] {
			doneGroup.Containers = append(doneGroup.Containers, r.row(st))
		}
	}
	return sidebarResponse{Groups: append(groups, doneGroup), Menu: r.sectionMenu(proj.Filter)}, nil
}

// segmentFilters are the project's segments' filters, in order.
func segmentFilters(p *settings.Project) []settings.Filter {
	fs := make([]settings.Filter, len(p.Segments))
	for i, s := range p.Segments {
		fs[i] = s.Filter
	}
	return fs
}

// weekStart is the Monday of now's week, as the date search takes it: Done
// is this week's work, from Monday on, in the local time now is in.
func weekStart(now time.Time) string {
	back := (int(now.Weekday()) + 6) % 7
	return now.AddDate(0, 0, -back).Format("2006-01-02")
}

func compare(x, y int64) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

func count(n int) *int { return &n }

func (a *app) container(ctx context.Context) ([]byte, error) {
	id, err := storyID(a.req.ID)
	if err != nil {
		return nil, err
	}
	return a.cached("container:"+a.req.ID, func() (any, error) { return a.buildContainer(ctx, id) })
}

// buildContainer reads the story with the workspace's workflows, teams,
// projects and members in one parallel round, then its epic and iteration
// through the long cache, and draws its detail.
func (a *app) buildContainer(ctx context.Context, id int64) (source.ContainerDetail, error) {
	var r refs
	var st shortcut.Story
	err := parallel(
		func() (err error) { st, err = a.story(ctx, id); return err },
		into(ctx, &r.workflows, a.sc.Workflows),
		into(ctx, &r.groups, a.sc.Groups),
		// A project is one fact; a workspace that won't list them shows "—".
		func() error { r.projects, _ = a.sc.Projects(ctx); return nil },
		into(ctx, &r.members, a.sc.Members),
	)
	if err != nil {
		return source.ContainerDetail{}, err
	}
	_ = parallel(
		func() error { r.epics = lookup(ctx, a, "epic", []int64{st.EpicID}, a.sc.Epic); return nil },
		func() error {
			r.iterations = lookup(ctx, a, "iteration", []int64{st.IterationID}, a.sc.Iteration)
			return nil
		},
	)
	sid := "sc-" + strconv.FormatInt(st.ID, 10)
	return source.ContainerDetail{
		ID:          strconv.FormatInt(st.ID, 10),
		Title:       st.Name,
		ExternalURL: st.AppURL,
		Facts:       r.facts(st, a.now),
		Sections: []source.Section{
			{ID: "story", Title: "Story", Kind: source.KindProse, Body: st.Description},
			{ID: "comments", Title: "Comments", Kind: source.KindComments, Comments: r.comments(st, a.now),
				Composer: &source.Action{ID: "comment", Label: "Comment", Input: source.InputText}},
			{ID: "links", Title: "Links", Kind: source.KindLinks, Links: links(st)},
		},
		Menu:     cardMenu,
		TaskNote: "Linked to " + sid + ". Merging moves the card to Done when it's the last open task.",
	}, nil
}

// orDash is s, or "—" when there's nothing to show.
func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// facts are the brief's list, in its order: id, team, project, state, type,
// epic, labels, owner(s), requester, created, updated, iteration.
func (r *refs) facts(st shortcut.Story, now time.Time) []source.Fact {
	team := source.Fact{Label: "team", Value: "—"}
	if g, ok := r.group(st.GroupID); ok {
		team = source.Fact{Label: "team", Value: code(g.MentionName, g.Name) + " · " + g.Name, Color: teamColor(g)}
	}
	project := source.Fact{Label: "project", Value: "—"}
	if p, ok := r.project(st.ProjectID); ok {
		project = source.Fact{Label: "project", Value: projectCode(p) + " · " + p.Name, Color: hexOr(p.Color, neutral)}
	}
	state, _, _ := r.state(st.WorkflowStateID)
	epic, _ := r.epic(st.EpicID)
	var labels []string
	for _, l := range st.Labels {
		labels = append(labels, l.Name)
	}
	var owners []string
	for _, id := range st.OwnerIDs {
		owners = append(owners, r.name(id))
	}
	ownerLabel, ownerValue := "owner", "unassigned"
	if len(owners) > 0 {
		ownerValue = strings.Join(owners, ", ")
	}
	if len(owners) > 1 {
		ownerLabel = "owners"
	}
	requester := "—"
	if st.RequestedByID != "" {
		requester = r.name(st.RequestedByID)
	}
	return []source.Fact{
		{Label: "id", Value: "sc-" + strconv.FormatInt(st.ID, 10)},
		team,
		project,
		{Label: "state", Value: orDash(state.Name)},
		{Label: "type", Value: orDash(st.StoryType)},
		{Label: "epic", Value: orDash(epic.Name)},
		{Label: "labels", Value: orDash(strings.Join(labels, ", "))},
		{Label: ownerLabel, Value: ownerValue},
		{Label: "requester", Value: requester},
		{Label: "created", Value: ago(now, st.CreatedAt.Time)},
		{Label: "updated", Value: ago(now, st.UpdatedAt.Time)},
		{Label: "iteration", Value: orDash(r.iteration(st.IterationID))},
	}
}

// comments are the story's live comments, oldest first.
func (r *refs) comments(st shortcut.Story, now time.Time) []source.Comment {
	cs := slices.Clone(st.Comments)
	slices.SortStableFunc(cs, func(x, y shortcut.Comment) int { return x.CreatedAt.Compare(y.CreatedAt.Time) })
	out := []source.Comment{}
	for _, c := range cs {
		if c.Deleted {
			continue
		}
		out = append(out, source.Comment{By: r.name(c.AuthorID), When: ago(now, c.CreatedAt.Time), Text: c.Text})
	}
	return out
}

// links are the story's pull requests (each once, however many branches
// carry it), then its branches, then its story links, then its external
// links.
func links(st shortcut.Story) []source.Link {
	out := []source.Link{}
	seen := map[int64]bool{}
	addPR := func(pr shortcut.PullRequest) {
		if seen[pr.ID] {
			return
		}
		seen[pr.ID] = true
		state := "open"
		switch {
		case pr.Merged:
			state = "merged"
		case pr.Closed:
			state = "closed"
		}
		out = append(out, source.Link{Label: "PR #" + strconv.FormatInt(pr.Number, 10), Text: pr.Title, State: state, URL: pr.URL})
	}
	for _, pr := range st.PullRequests {
		addPR(pr)
	}
	for _, b := range st.Branches {
		for _, pr := range b.PullRequests {
			addPR(pr)
		}
	}
	for _, b := range st.Branches {
		if b.Deleted {
			continue
		}
		state := "open"
		if b.Merged {
			state = "merged"
		}
		out = append(out, source.Link{Label: "Branch", Text: b.Name, State: state, URL: b.URL})
	}
	storyBase, _, _ := strings.Cut(st.AppURL, "/story/")
	for _, l := range st.StoryLinks {
		verb, other := l.Verb, l.ObjectID
		if l.Type == "object" {
			other = l.SubjectID
			switch l.Verb {
			case "blocks":
				verb = "blocked by"
			case "duplicates":
				verb = "duplicated by"
			}
		}
		u := ""
		if storyBase != st.AppURL {
			u = storyBase + "/story/" + strconv.FormatInt(other, 10)
		}
		out = append(out, source.Link{Label: verb, Text: "sc-" + strconv.FormatInt(other, 10), URL: u})
	}
	for _, u := range st.ExternalLinks {
		text := strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
		out = append(out, source.Link{Label: "Link", Text: text, URL: u})
	}
	return out
}
