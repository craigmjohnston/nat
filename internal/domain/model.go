// Package domain holds the plan the TUI renders: a project's milestones and
// slices, mapped out of Notion pages, plus the progress math the board draws.
// Everything here is pure — no I/O, no Notion client.
package domain

import (
	"sort"
	"strings"

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

// The slice statuses, in workflow order. SliceClaimed is the in-progress one —
// a slice an agent holds — named for what it means to the plan rather than for
// the option, which every project spells "In progress" once it has been loaded
// once. The name as the project writes it is kept on the slice as StatusName,
// for output that names it back to the user.
const (
	SliceTodo    SliceStatus = notion.SliceTodo
	SliceClaimed SliceStatus = notion.SliceInProgress
	SliceDone    SliceStatus = notion.SliceDone
)

// UnassignedName is the name of the implicit group holding slices that belong
// to no milestone. It is not a milestone in Notion — nothing can be written to
// it — it exists so those slices are still visible on the board.
const UnassignedName = "Unassigned"

// Milestone is one phase of a project's plan: an option of the Slices data
// source's Milestone column, with no page of its own, so its Order is its place
// among the options and its Status is computed from the slices under it.
// SelectType is the property type of that column, so a slice can be filed under
// the milestone in the shape the column was read in.
type Milestone struct {
	ID         string
	Name       string
	Order      float64
	Status     MilestoneStatus
	SelectType string
}

// Ref is the Milestone property value filing a slice under m: the option naming
// it, written back as the type the column was read as.
func (m Milestone) Ref() notion.PropertyValue {
	return notion.NewChoice(m.SelectType, m.Name)
}

// Slice is one unit of work, small enough for a single agent session. Status is
// the workflow status, normalised across the two spellings of in-progress;
// StatusName is what the project's own Status column calls it, so a message
// naming a slice's status says what someone would see in Notion.
//
// Branch is the branch an agent pushed its work to and handed back on, empty
// until it does and on every project whose Slices table has no such column. A
// slice in progress that names one is work waiting to be reviewed, which is
// what the board's approve action opens a pull request from.
//
// DependsOn is the page IDs of the slices this one waits on, in the order the
// relation lists them, and empty both for a slice that waits on nothing and on
// a project whose table has no such column — so a project without one behaves
// exactly as it did before there was one.
type Slice struct {
	ID           string
	Name         string
	Status       SliceStatus
	StatusName   string
	MilestoneID  string
	AssigneeName string
	Repo         string
	Branch       string
	PRURL        string
	URL          string
	DependsOn    []string
}

// Project is a tracked project's whole plan, as loaded from Notion.
type Project struct {
	ID         string
	Name       string
	Milestones []Milestone
	Slices     []Slice
}

// MilestonesFromOptions maps the options of a Slices data source's Milestone
// column onto milestones, which is a project's whole plan. Such a milestone is
// nothing but its name, so the name is its ID here too — it is what a slice's
// column names, and so what groups the plan — and its order is its place among
// the options, which is the order the plan is written in. It has no page, and no
// status of its own: NewProject computes the status from the slices under it.
// propertyType is the type of the column the options came off, carried so that
// filing a slice under one of them writes the column in the shape it was read
// in.
func MilestonesFromOptions(names []string, propertyType string) []Milestone {
	ms := make([]Milestone, len(names))
	for i, n := range names {
		ms[i] = Milestone{ID: n, Name: n, Order: float64(i), SelectType: propertyType}
	}
	return ms
}

// SliceFromPage maps a page of a project's Slices data source. The Milestone
// column names the milestone the slice is under, as one of that column's
// options. Only the first assignee is read, since claiming sets exactly one.
func SliceFromPage(p notion.Page) Slice {
	name := p.Properties[notion.PropStatus].SelectName()
	s := Slice{
		ID:         p.ID,
		Name:       p.Properties[notion.PropName].Text(),
		Status:     SliceStatus(name),
		StatusName: name,
		Repo:       p.Properties[notion.PropRepo].Text(),
		Branch:     p.Properties[notion.PropBranch].Text(),
		PRURL:      p.Properties[notion.PropPR].URL,
		URL:        p.URL,
		DependsOn:  p.Properties[notion.PropDependsOn].RelationIDs(),
	}
	s.MilestoneID = p.Properties[notion.PropMilestone].SelectName()
	if people := p.Properties[notion.PropAssignee].Users(); len(people) > 0 {
		s.AssigneeName = people[0].Name
	}
	return s
}

// SlicesFromPages maps a Slices query result, preserving its order.
func SlicesFromPages(pages []notion.Page) []Slice {
	ss := make([]Slice, len(pages))
	for i, p := range pages {
		ss[i] = SliceFromPage(p)
	}
	return ss
}

// InViewOrder returns the slices in the order the given page IDs put them in —
// the order of a Notion view, which is what someone reading the plan in Notion
// sees. A slice the order does not name keeps its place relative to the other
// unnamed ones and follows every named slice, so a slice a view's filter hides,
// or one created since the order was read, is still there to work.
//
// The input is not modified; the IDs are compared ignoring dashes and case,
// since a Notion ID is written both ways.
func InViewOrder(slices []Slice, order []string) []Slice {
	if len(order) == 0 {
		return slices
	}
	rank := make(map[string]int, len(order))
	for i, id := range order {
		rank[NormaliseID(id)] = i
	}
	ordered := make([]Slice, len(slices))
	copy(ordered, slices)
	sort.SliceStable(ordered, func(i, j int) bool {
		return rankOf(rank, ordered[i]) < rankOf(rank, ordered[j])
	})
	return ordered
}

// rankOf is a slice's place in a view's order, and one past the last place for
// a slice the order does not name.
func rankOf(rank map[string]int, s Slice) int {
	if i, ok := rank[NormaliseID(s.ID)]; ok {
		return i
	}
	return len(rank)
}

// NormaliseID puts a Notion ID in one form, so IDs read from different places
// compare equal however they were written. It is what SlicesByID keys on, so
// anything looking a slice up in that index normalises its key the same way.
func NormaliseID(id string) string {
	return strings.ToLower(strings.ReplaceAll(id, "-", ""))
}

// SlicesByID indexes slices by page ID, for looking up the ones a slice
// depends on. IDs are normalised, so an index built from a query answers a
// lookup by an ID written the other way round.
func SlicesByID(slices []Slice) map[string]Slice {
	byID := make(map[string]Slice, len(slices))
	for _, s := range slices {
		byID[NormaliseID(s.ID)] = s
	}
	return byID
}

// Blockers is the slices s waits on that are not Done yet, in the order s names
// them. That is the whole rule: a slice is blocked while any slice it depends
// on is unfinished, and a slice naming none — which is every slice of a project
// whose table has no dependency column — is never blocked.
//
// unknown is the dependencies the index does not hold, which are passed over
// rather than counted as unfinished: such a page cannot be read at all —
// trashed, or filed outside this project — and a slice waiting forever on one
// nobody can see is worse than one that goes ahead. They are returned so the
// caller doing the reading can say so in the log.
func Blockers(s Slice, byID map[string]Slice) (blocking []Slice, unknown []string) {
	for _, id := range s.DependsOn {
		dep, ok := byID[NormaliseID(id)]
		switch {
		case !ok:
			unknown = append(unknown, id)
		case dep.Status != SliceDone:
			blocking = append(blocking, dep)
		}
	}
	return blocking, unknown
}

// Blocked reports whether s is waiting on any unfinished slice.
func Blocked(s Slice, byID map[string]Slice) bool {
	blocking, _ := Blockers(s, byID)
	return len(blocking) > 0
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

// MilestoneStatusOf is the status a milestone is in, read off the slices under
// it: nothing started is Queued, everything finished
// is Done, anything in between — a slice in progress, or some but not all of
// them Done — is Active. A milestone with no slices at all has nothing started
// either, so it is Queued.
func MilestoneStatusOf(slices []Slice) MilestoneStatus {
	p := ProgressOf(slices)
	switch {
	case p.Total > 0 && p.Done == p.Total:
		return MilestoneDone
	case p.Claimed > 0 || p.Done > 0:
		return MilestoneActive
	default:
		return MilestoneQueued
	}
}

// NewProject assembles a plan read from Notion, filling in what a milestone
// with no page of its own cannot carry: its status, computed from the slices
// that name it.
func NewProject(id, name string, milestones []Milestone, slices []Slice) Project {
	ms := make([]Milestone, len(milestones))
	copy(ms, milestones)
	for i := range ms {
		var under []Slice
		for _, s := range slices {
			if s.MilestoneID == ms[i].ID {
				under = append(under, s)
			}
		}
		ms[i].Status = MilestoneStatusOf(under)
	}
	return Project{ID: id, Name: name, Milestones: ms, Slices: slices}
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
