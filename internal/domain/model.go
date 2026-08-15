// Package domain holds the plan the TUI renders: a project's milestones and
// slices, mapped out of Notion pages, plus the progress math the board draws.
// Everything here is pure — no I/O, no Notion client.
package domain

import (
	"sort"

	"github.com/craigmjohnston/nat/internal/notion"
)

// MilestoneStatus is where a milestone sits in its workflow.
type MilestoneStatus string

// The milestone statuses, in workflow order.
const (
	MilestoneQueued MilestoneStatus = notion.MilestoneQueued
	MilestoneActive MilestoneStatus = notion.MilestoneActive
	MilestoneDone   MilestoneStatus = notion.MilestoneDone
)

// SliceStatus is where a slice sits in its workflow.
type SliceStatus string

// The slice statuses, in workflow order. There is one in-progress status here
// however the project spells it: a project's Status column offers either
// Claimed or In progress, and both map onto SliceClaimed, so everything reading
// a plan — the board chip, the progress math, the gating on work in flight —
// asks one question rather than two. The name as the project writes it is kept
// on the slice as StatusName, for output that names it back to the user.
const (
	SliceTodo    SliceStatus = notion.SliceTodo
	SliceClaimed SliceStatus = notion.SliceClaimed
	SliceDone    SliceStatus = notion.SliceDone
)

// UnassignedName is the name of the implicit group holding slices that belong
// to no milestone. It is not a milestone in Notion — nothing can be written to
// it — it exists so those slices are still visible on the board.
const UnassignedName = "Unassigned"

// Milestone is one phase of a project's plan. StatusType is the Notion property
// type its Status column turned out to have, carried so that a write can be
// sent in the same shape it was read in.
type Milestone struct {
	ID         string
	Name       string
	Order      float64
	Status     MilestoneStatus
	StatusType string
	URL        string
}

// Slice is one unit of work, small enough for a single agent session. Status is
// the workflow status, normalised across the two spellings of in-progress;
// StatusName is what the project's own Status column calls it, so a message
// naming a slice's status says what someone would see in Notion.
type Slice struct {
	ID           string
	Name         string
	Status       SliceStatus
	StatusName   string
	MilestoneID  string
	AssigneeName string
	Repo         string
	PRURL        string
	URL          string
}

// Project is a tracked project's whole plan, as loaded from Notion.
type Project struct {
	ID         string
	Name       string
	Milestones []Milestone
	Slices     []Slice
}

// MilestoneFromPage maps a page of a project's Milestones data source. Missing
// or differently-typed properties map to zero values rather than an error: the
// board would rather show a half-filled row than nothing at all.
func MilestoneFromPage(p notion.Page) Milestone {
	order, _ := p.Properties[notion.PropOrder].NumberValue()
	status := p.Properties[notion.PropStatus]
	return Milestone{
		ID:         p.ID,
		Name:       p.Properties[notion.PropName].Text(),
		Order:      order,
		Status:     MilestoneStatus(status.SelectName()),
		StatusType: status.Type,
		URL:        p.URL,
	}
}

// MilestonesFromPages maps a Milestones query result, preserving its order.
func MilestonesFromPages(pages []notion.Page) []Milestone {
	ms := make([]Milestone, len(pages))
	for i, p := range pages {
		ms[i] = MilestoneFromPage(p)
	}
	return ms
}

// MilestonesFromOptions maps the options of a Slices data source's Milestone
// select onto milestones, for a project that keeps no Milestones database. Such
// a milestone is nothing but its name, so the name is its ID here too — it is
// what a slice's select names, and so what groups the plan — and its order is
// its place among the options, which is the order the plan is written in. It
// has no page, so no status and no URL.
func MilestonesFromOptions(names []string) []Milestone {
	ms := make([]Milestone, len(names))
	for i, n := range names {
		ms[i] = Milestone{ID: n, Name: n, Order: float64(i)}
	}
	return ms
}

// SliceFromPage maps a page of a project's Slices data source. A slice relates
// to at most one milestone, so only the first relation is read — and where the
// project has no Milestones database, the Milestone column is a select instead
// and the option it names is the milestone. Likewise only the first assignee is
// read, since claiming sets exactly one.
func SliceFromPage(p notion.Page) Slice {
	name := p.Properties[notion.PropStatus].SelectName()
	s := Slice{
		ID:         p.ID,
		Name:       p.Properties[notion.PropName].Text(),
		Status:     sliceStatus(name),
		StatusName: name,
		Repo:       p.Properties[notion.PropRepo].Text(),
		PRURL:      p.Properties[notion.PropPR].URL,
		URL:        p.URL,
	}
	milestone := p.Properties[notion.PropMilestone]
	if ids := milestone.RelationIDs(); len(ids) > 0 {
		s.MilestoneID = ids[0]
	} else {
		s.MilestoneID = milestone.SelectName()
	}
	if people := p.Properties[notion.PropAssignee].People; len(people) > 0 {
		s.AssigneeName = people[0].Name
	}
	return s
}

// sliceStatus maps a Status option name onto the workflow status it means. In
// progress is the newer name for Claimed, so the two land on one status;
// anything else is carried through as it was written, which is what an unknown
// status draws as on the board.
func sliceStatus(name string) SliceStatus {
	if name == notion.SliceInProgress {
		return SliceClaimed
	}
	return SliceStatus(name)
}

// SlicesFromPages maps a Slices query result, preserving its order.
func SlicesFromPages(pages []notion.Page) []Slice {
	ss := make([]Slice, len(pages))
	for i, p := range pages {
		ss[i] = SliceFromPage(p)
	}
	return ss
}

// Progress is the tally of one set of slices by status, as the segmented
// progress bar draws it. Total counts every slice, including any whose status
// is none of the three known ones.
type Progress struct {
	Todo    int
	Claimed int
	Done    int
	Total   int
}

// Fraction is the share of slices that are Done, in [0,1]. An empty set is 0
// rather than undefined, so a milestone with no slices draws as an empty bar.
func (p Progress) Fraction() float64 {
	if p.Total == 0 {
		return 0
	}
	return float64(p.Done) / float64(p.Total)
}

// Empty reports whether there are no slices to draw.
func (p Progress) Empty() bool { return p.Total == 0 }

// ProgressOf tallies a set of slices.
func ProgressOf(slices []Slice) Progress {
	var p Progress
	for _, s := range slices {
		p.Total++
		switch s.Status {
		case SliceTodo:
			p.Todo++
		case SliceClaimed:
			p.Claimed++
		case SliceDone:
			p.Done++
		}
	}
	return p
}

// Group is one row of the board: a milestone and the slices under it. Milestone
// is nil for the implicit Unassigned group.
type Group struct {
	Milestone *Milestone
	Slices    []Slice
}

// Name is the milestone's name, or UnassignedName for the implicit group.
func (g Group) Name() string {
	if g.Milestone == nil {
		return UnassignedName
	}
	return g.Milestone.Name
}

// Progress tallies the group's slices.
func (g Group) Progress() Progress { return ProgressOf(g.Slices) }

// Groups buckets the project's slices under their milestones, ordered by the
// milestone Order ascending (ties broken by name, so the order is stable).
// Every milestone gets a group even when it has no slices. Slices with no
// milestone — or one that is not in the project, e.g. a relation to a page the
// query did not return — land in a trailing Unassigned group, which is omitted
// when empty.
func (p Project) Groups() []Group {
	ordered := make([]Milestone, len(p.Milestones))
	copy(ordered, p.Milestones)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Order != ordered[j].Order {
			return ordered[i].Order < ordered[j].Order
		}
		return ordered[i].Name < ordered[j].Name
	})

	groups := make([]Group, len(ordered))
	index := make(map[string]int, len(ordered))
	for i := range ordered {
		groups[i] = Group{Milestone: &ordered[i]}
		index[ordered[i].ID] = i
	}

	var unassigned []Slice
	for _, s := range p.Slices {
		if i, ok := index[s.MilestoneID]; ok && s.MilestoneID != "" {
			groups[i].Slices = append(groups[i].Slices, s)
			continue
		}
		unassigned = append(unassigned, s)
	}
	if len(unassigned) > 0 {
		groups = append(groups, Group{Slices: unassigned})
	}
	return groups
}

// Progress tallies every slice in the project, whatever milestone it is under.
func (p Project) Progress() Progress { return ProgressOf(p.Slices) }
