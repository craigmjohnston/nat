package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// API is what a Notion-backed store needs of the Notion client: the reads and
// writes the plan operations are made of, and nothing else the app asks Notion
// for. It is an interface so the store can be driven by a fake, and narrow so
// that what a plan actually costs in requests is readable in one place.
type API interface {
	notion.MigrationAPI
	DataSourceOrder(ctx context.Context, dataSourceID string) ([]string, error)
	GetPage(ctx context.Context, id string) (*notion.Page, error)
	CreatePage(ctx context.Context, parent notion.Parent, properties map[string]notion.PropertyValue, children []map[string]any) (*notion.Page, error)
	GetBlockChildren(ctx context.Context, id string) ([]notion.Block, error)
	AppendBlockChildren(ctx context.Context, id string, children []map[string]any) ([]notion.Block, error)
	TrashPage(ctx context.Context, pageID string) error
}

// NewClient builds the Notion client every plan read and write goes through.
// It is the one place in the tree a client is constructed: the token is
// fetched per attempt, so a credential rotated by the CLI is picked up
// mid-session, and everything above this package asks a [Store] instead.
func NewClient(token notion.TokenFunc) *notion.Client { return notion.NewWithToken(token) }

// Notion is a plan kept in Notion: milestones as the options of the slices'
// own Milestone column, one page per slice, and the slice's brief as that
// page's body.
type Notion struct{ api API }

// Over returns a store reading and writing a plan through the given Notion
// client. It takes the interface rather than the client itself so a caller
// that already holds one — the board, a headless command — hands it over
// instead of making a second.
func Over(api API) *Notion { return &Notion{api: api} }

// Notion is a Store.
var _ Store = (*Notion)(nil)

// Shape reads how a project's Slices data source is put together, migrating a
// project still in the shape this app started with on the way — which is how
// every read of a plan, by the board or by a command, arrives at a plan of the
// one shape.
func (n *Notion) Shape(ctx context.Context, p Project) (Shape, error) {
	ds, migration, err := notion.MigrateProject(ctx, n.api, p.SlicesID)
	if err != nil {
		return Shape{}, err
	}
	if !migration.Empty() {
		logging.Action("project migrated on the way to reading its plan", "summary", migration.Summary())
	}
	return shapeOf(ds), nil
}

// shapeOf says a Notion data source's shape in the store's own words.
func shapeOf(ds *notion.DataSource) Shape {
	s := notion.ShapeOf(ds)
	return Shape{
		HasAssignee: s.HasAssignee,
		HasBranch:   s.HasBranch,
		Milestones:  domain.MilestonesFromOptions(s.MilestoneOptions, s.MilestoneType),
		statusType:  s.StatusType,
		milestone:   ds.Properties[notion.PropMilestone],
	}
}

// Plan reads the whole plan: its milestones, which are the options of the
// Slices data source's Milestone column and so come with the schema, and its
// slices, oldest first and then put into the order the project's own board
// puts them in.
//
// A plan kept on one page has no order of its own to sort by, and created time
// is no substitute — Notion records it to the minute, so a plan written in one
// go has no order at all — which is why the view's own row order is read.
func (n *Notion) Plan(ctx context.Context, p Project) (Plan, error) {
	ds, migration, err := notion.MigrateProject(ctx, n.api, p.SlicesID)
	if err != nil {
		return Plan{}, err
	}
	sh := shapeOf(ds)
	pages, err := n.api.QueryDataSource(ctx, p.SlicesID, nil,
		[]notion.Sort{{Timestamp: notion.TimestampCreated, Direction: notion.SortAscending}})
	if err != nil {
		return Plan{}, fmt.Errorf("load slices: %w", err)
	}
	plan := Plan{
		Project: domain.NewProject(p.ID, p.Name, sh.Milestones,
			domain.InViewOrder(domain.SlicesFromPages(pages), notion.PlanOrder(ctx, n.api, p.SlicesID))),
		Shape: sh,
	}
	if !migration.Empty() {
		plan.Migrated = migration.Summary()
	}
	return plan, nil
}

// Slice reads one slice, and with it the shape that slice can be written in.
// The shape comes off the page rather than off the project's schema, because
// what a write has to match is the column as it stands on the page in hand: a
// Status column converted in the Notion UI takes a different value from the
// select every project this app made has.
func (n *Notion) Slice(ctx context.Context, id string) (domain.Slice, Shape, error) {
	page, err := n.api.GetPage(ctx, id)
	if err != nil {
		return domain.Slice{}, Shape{}, err
	}
	sh := Shape{statusType: page.Properties[notion.PropStatus].Type}
	if _, tracked := page.Properties[notion.PropAssignee]; tracked {
		sh.HasAssignee = true
	}
	return domain.SliceFromPage(*page), sh, nil
}

// Body reads a page's own prose as markdown: a slice's brief, or the
// conventions written on a project page. Both are the same read, and neither
// is a property — which is why this is a read of its own rather than something
// [Notion.Slice] carries.
func (n *Notion) Body(ctx context.Context, id string) (string, error) {
	blocks, err := n.api.GetBlockChildren(ctx, id)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(notion.Markdown(blocks)), nil
}

// PRDescription reads the pull request description a hand-back filed on a
// slice. It lives on the page rather than in the command that carried it, so
// an approve days later opens the pull request with it; a slice handed back
// twice has one section per hand-back, and the last is the one that counts.
func (n *Notion) PRDescription(ctx context.Context, id string) (string, error) {
	blocks, err := n.api.GetBlockChildren(ctx, id)
	if err != nil {
		return "", err
	}
	return notion.PRDescriptionOf(blocks), nil
}

// ClaimSlice takes the slice: status to the in-progress option, and the
// assignee set to the given user where the project tracks one and there is a
// user to name. The slice Notion answers with comes back rather than the one
// asked for — a people value naming somebody the workspace does not know comes
// back empty instead of failing, and a caller must be able to see that.
func (n *Notion) ClaimSlice(ctx context.Context, id string, sh Shape, userID string) (domain.Slice, error) {
	properties := map[string]notion.PropertyValue{
		notion.PropStatus: notion.NewChoice(sh.statusType, notion.SliceInProgress),
	}
	if sh.HasAssignee && userID != "" {
		properties[notion.PropAssignee] = notion.NewPeople(userID)
	}
	updated, err := n.api.UpdatePageProperties(ctx, id, properties)
	if err != nil {
		return domain.Slice{}, err
	}
	s := domain.SliceFromPage(*updated)
	logging.Action("slice claimed", "slice", s.ID, "name", s.Name, "user", userID)
	return s, nil
}

// ReleaseSlice hands the slice back to the plan: Todo, held by nobody where
// there is a column to say so on, and a line on the page naming who let it go.
//
// The line goes on before the status does. Either write can fail, and of the
// two half-finished states this is the recoverable one: a slice still in
// progress carrying the line can be released again, whereas a slice already
// back at Todo is one the caller would refuse to add a line to.
func (n *Notion) ReleaseSlice(ctx context.Context, id string, sh Shape, by string) (domain.Slice, error) {
	if _, err := n.api.AppendBlockChildren(ctx, id,
		[]map[string]any{textBlock("paragraph", releasedLine(by))}); err != nil {
		return domain.Slice{}, fmt.Errorf("note the release on the slice: %w", err)
	}
	properties := map[string]notion.PropertyValue{
		notion.PropStatus: notion.NewChoice(sh.statusType, notion.SliceTodo),
	}
	if sh.HasAssignee {
		properties[notion.PropAssignee] = notion.NewPeople()
	}
	updated, err := n.api.UpdatePageProperties(ctx, id, properties)
	if err != nil {
		return domain.Slice{}, fmt.Errorf("release the slice: %w", err)
	}
	s := domain.SliceFromPage(*updated)
	logging.Action("slice released", "slice", s.ID, "name", s.Name)
	return s, nil
}

// releasedLine is the one line a release leaves on a slice, so a slice that
// went round twice reads as having done so rather than as having been worked
// once by somebody who wrote nothing down. The board and the headless command
// are the same act by two routes, and they say so in the same words because
// there is only one place it is said.
func releasedLine(assignee string) string {
	return fmt.Sprintf("Released back to Todo by %s: the session working it ended without finishing it.", assignee)
}

// The headings a closing note is filed under, so a page read later says which
// kind of ending it was.
const (
	summaryHeading    = "Summary"
	blockedHeading    = "Blocked"
	handedBackHeading = "Handed back"
)

// noteHeading names the note by how the session ended.
func noteHeading(o Outcome) string {
	switch {
	case o.Blocked:
		return blockedHeading
	case o.Branch != "":
		return handedBackHeading
	}
	return summaryHeading
}

// CompleteSlice closes the slice out: the summary appended to the page, and
// then the properties of whichever ending was asked for.
//
// The note goes on before the status does, for the reason a release's line
// does: an in-progress slice carrying its summary can be completed by running
// again, whereas a Done slice with no summary refuses every attempt to add
// one.
func (n *Notion) CompleteSlice(ctx context.Context, id string, sh Shape, o Outcome) (domain.Slice, error) {
	blocks := noteBlocks(noteHeading(o), o.Summary)
	// The description goes on in the same write, under a heading of its own: it
	// is not the summary of what was done but the text the pull request will be
	// opened with, and the board reads it back off the page by that heading
	// whenever the user gets to reviewing the branch.
	if o.PRDescription != "" {
		blocks = append(blocks, noteBlocks(notion.PRDescriptionHeading, o.PRDescription)...)
	}
	if _, err := n.api.AppendBlockChildren(ctx, id, blocks); err != nil {
		return domain.Slice{}, fmt.Errorf("append the note to the slice: %w", err)
	}
	props := map[string]notion.PropertyValue{}
	if o.PR != "" {
		props[notion.PropPR] = notion.NewURL(o.PR)
	}
	if o.Branch != "" {
		props[notion.PropBranch] = notion.NewRichText(o.Branch)
	}
	if o.done() {
		props[notion.PropStatus] = notion.NewChoice(sh.statusType, notion.SliceDone)
	}
	if len(props) == 0 {
		s, _, err := n.Slice(ctx, id)
		return s, err
	}
	updated, err := n.api.UpdatePageProperties(ctx, id, props)
	if err != nil {
		return domain.Slice{}, fmt.Errorf("close out the slice: %w", err)
	}
	s := domain.SliceFromPage(*updated)
	logging.Action("slice closed out", "slice", s.ID, "blocked", o.Blocked, "pr", o.PR, "branch", o.Branch)
	return s, nil
}

// RecordPR writes the pull request onto the slice and nothing else.
func (n *Notion) RecordPR(ctx context.Context, id, url string) error {
	_, err := n.api.UpdatePageProperties(ctx, id,
		map[string]notion.PropertyValue{notion.PropPR: notion.NewURL(url)})
	return err
}

// MarkDone moves a slice to Done.
func (n *Notion) MarkDone(ctx context.Context, id string, sh Shape) error {
	if _, err := n.api.UpdatePageProperties(ctx, id, map[string]notion.PropertyValue{
		notion.PropStatus: notion.NewChoice(sh.statusType, notion.SliceDone),
	}); err != nil {
		return err
	}
	logging.Action("slice marked Done", "slice", id)
	return nil
}

// AddMilestones files milestones at the end of the plan, which is the options
// of the slices' own Milestone column, in one schema write: either they all
// arrive or none do, since their order in the column is the order of the plan.
//
// A name the plan already holds is refused before anything is written. Such a
// milestone is nothing but its name — it is what a slice's column names, and so
// what groups the plan — and two options sharing one could not be told apart.
func (n *Notion) AddMilestones(ctx context.Context, p Project, sh Shape, names []string) ([]domain.Milestone, error) {
	// A plan that adds no milestone writes nothing: the write would replace the
	// option list with a copy of itself, which is a real edit to make of a
	// schema for the sake of nothing.
	if len(names) == 0 {
		return nil, nil
	}
	existing := sh.Milestones
	taken := map[string]string{}
	for _, m := range existing {
		taken[strings.ToLower(strings.TrimSpace(m.Name))] = m.Name
	}
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if held, dup := taken[key]; dup {
			return nil, fmt.Errorf("the plan already has a milestone named %q: "+
				"its milestones are the options of the slices' %s column, which cannot hold two of a name",
				held, notion.PropMilestone)
		}
		taken[key] = name
	}

	milestone := sh.milestone
	property, ok := milestone.AppendedOptions(names...)
	if !ok {
		return nil, fmt.Errorf("the %s column is a %s: a milestone can only be added to it in Notion",
			notion.PropMilestone, milestone.Type)
	}
	if _, err := n.api.UpdateDataSourceProperties(ctx, p.SlicesID,
		map[string]notion.PropertySchema{notion.PropMilestone: property}); err != nil {
		return nil, fmt.Errorf("create the %s: %w", pluralise("milestone", len(names)), err)
	}

	// The order of a milestone is its place among the options, counting from
	// zero, which is what reading the plan back would make of it.
	added := make([]domain.Milestone, len(names))
	for i, name := range names {
		added[i] = domain.Milestone{
			ID:         name,
			Name:       name,
			Order:      float64(len(existing) + i),
			Status:     domain.MilestoneStatusOf(nil),
			SelectType: milestone.Type,
		}
		logging.Action("milestone added", "milestone", name, "order", added[i].Order)
	}
	return added, nil
}

// RenameMilestone gives one milestone another name, leaving it where it is in
// the plan and leaving its slices filed under it.
//
// Notion quietly ignores renaming a select option in place — a 200 whose body
// still says the old name — so this goes the long way the Claimed migration
// does, and in that order: the new option is written beside the old one, every
// slice holding the old one is refiled onto the new one, and only then is the
// old one dropped. A run refused part way therefore leaves every slice on a
// milestone that exists, which is the whole reason for the order.
//
// Beside rather than at the end, because a milestone's order is its place among
// the options and the plan is read in that order: appending would rename the
// milestone and move it to the end of the plan in the one write.
func (n *Notion) RenameMilestone(ctx context.Context, p Project, sh Shape, old, name string) (domain.Milestone, error) {
	from, err := renameTargets(sh.Milestones, old, name, func(held string) error {
		return fmt.Errorf("the plan already has a milestone named %q: "+
			"its milestones are the options of the slices' %s column, which cannot hold two of a name",
			held, notion.PropMilestone)
	}, func() error {
		return fmt.Errorf("the plan has no milestone named %q: its milestones are %s",
			old, milestoneList(sh.Milestones))
	})
	if err != nil {
		return domain.Milestone{}, err
	}

	// Everything the refiling needs is read before the first write, so a plan
	// that cannot be read is a rename that has written nothing.
	pages, err := n.api.QueryDataSource(ctx, p.SlicesID, nil,
		[]notion.Sort{{Timestamp: notion.TimestampCreated, Direction: notion.SortAscending}})
	if err != nil {
		return domain.Milestone{}, fmt.Errorf("load slices: %w", err)
	}

	milestone := sh.milestone
	added, ok := milestone.OptionInsertedAfter(from.Name, name)
	if !ok {
		return domain.Milestone{}, fmt.Errorf("the %s column is a %s: a milestone can only be renamed in Notion",
			notion.PropMilestone, milestone.Type)
	}
	updated, err := n.api.UpdateDataSourceProperties(ctx, p.SlicesID,
		map[string]notion.PropertySchema{notion.PropMilestone: added})
	if err != nil {
		return domain.Milestone{}, fmt.Errorf("add the %q option: %w", name, err)
	}

	renamed := domain.Milestone{ID: name, Name: name, Order: from.Order, SelectType: milestone.Type}
	var under []domain.Slice
	for _, page := range pages {
		if page.Properties[notion.PropMilestone].SelectName() != from.Name {
			continue
		}
		under = append(under, domain.SliceFromPage(page))
		if _, err := n.api.UpdatePageProperties(ctx, page.ID,
			map[string]notion.PropertyValue{notion.PropMilestone: renamed.Ref()}); err != nil {
			return domain.Milestone{}, fmt.Errorf("refile slice %s under %q: %w", page.ID, name, err)
		}
	}
	// The status is the slices' answer, as it is everywhere else: a milestone
	// has none of its own, and the ones just refiled are the ones under it.
	renamed.Status = domain.MilestoneStatusOf(under)

	// Nothing sits on the old option any more; drop it. The options are sent
	// back exactly as the schema write echoed them — the new one now has an ID —
	// minus the one being retired.
	without, ok := updated.Properties[notion.PropMilestone].WithoutOption(from.Name)
	if !ok {
		return domain.Milestone{}, fmt.Errorf("the %s column came back as a %s: drop its %q option in Notion",
			notion.PropMilestone, updated.Properties[notion.PropMilestone].Type, from.Name)
	}
	if _, err := n.api.UpdateDataSourceProperties(ctx, p.SlicesID,
		map[string]notion.PropertySchema{notion.PropMilestone: without}); err != nil {
		return domain.Milestone{}, fmt.Errorf("retire the %q option: %w", from.Name, err)
	}
	logging.Action("milestone renamed", "from", from.Name, "to", name, "order", renamed.Order)
	return renamed, nil
}

// renameTargets settles a rename before any store writes anything: the
// milestone the old name refers to, or a refusal in the store's own words for a
// new name the plan already holds and for an old name it does not.
//
// Names are matched case-insensitively and trimmed, the way every other lookup
// of a milestone by name is — a milestone is nothing but its name, so two that
// differ only in case could not be told apart on the board.
func renameTargets(milestones []domain.Milestone, old, name string,
	duplicate func(held string) error, missing func() error) (domain.Milestone, error) {
	oldKey, newKey := milestoneKey(old), milestoneKey(name)
	var from domain.Milestone
	found := false
	for _, m := range milestones {
		key := milestoneKey(m.Name)
		// A rename to the name it already has is a duplicate of itself, which is
		// a write for nothing rather than a plan with two of a name — but it is
		// still refused, since there is nothing there for it to do.
		if key == newKey {
			return domain.Milestone{}, duplicate(m.Name)
		}
		if key == oldKey {
			from, found = m, true
		}
	}
	if !found {
		return domain.Milestone{}, missing()
	}
	return from, nil
}

// milestoneKey is a milestone name as names are compared: trimmed and folded.
func milestoneKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// milestoneList is the plan's milestones as an error reads them out, so a
// refusal over a name that is not there says which names are.
func milestoneList(milestones []domain.Milestone) string {
	if len(milestones) == 0 {
		return "none"
	}
	names := make([]string, len(milestones))
	for i, m := range milestones {
		names[i] = fmt.Sprintf("%q", m.Name)
	}
	return strings.Join(names, ", ")
}

// pluralise is the plural of a word for a count, for an error that names how
// many milestones it was asked for.
func pluralise(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// AddSlice writes the slice, with its brief as the page body. Status and
// ownership are not the caller's to choose: a newly filed slice is Todo and
// unclaimed, or it is not something the workflow can pick up.
//
// DependsOn is left off the write entirely when there is nothing to wait on: a
// project whose table has no dependency column can still have slices added to
// it, and sending an empty relation to a column that is not there would be the
// one thing stopping that.
func (n *Notion) AddSlice(ctx context.Context, p Project, s NewSlice) (domain.Slice, error) {
	properties := map[string]notion.PropertyValue{
		notion.PropName:      notion.NewTitle(s.Title),
		notion.PropStatus:    notion.NewSelect(notion.SliceTodo),
		notion.PropMilestone: s.Milestone.Ref(),
		notion.PropRepo:      notion.NewRichText(s.Repo),
	}
	if len(s.DependsOn) > 0 {
		properties[notion.PropDependsOn] = notion.NewRelation(s.DependsOn...)
	}
	page, err := n.api.CreatePage(ctx, notion.DataSourceParent(p.SlicesID), properties, paragraphBlocks(s.Brief))
	if err != nil {
		return domain.Slice{}, err
	}
	added := domain.SliceFromPage(*page)
	logging.Action("slice added", "slice", added.ID, "name", s.Title, "milestone", s.Milestone.ID)
	return added, nil
}

// EditSlice rewrites a slice's title, working directory and brief: its
// properties first, then its body. The milestone is left alone — moving a
// slice is its own operation — and so is the status, which only the workflow
// changes.
func (n *Notion) EditSlice(ctx context.Context, id, title, repo, brief string) error {
	if _, err := n.api.UpdatePageProperties(ctx, id, map[string]notion.PropertyValue{
		notion.PropName: notion.NewTitle(title),
		notion.PropRepo: notion.NewRichText(repo),
	}); err != nil {
		return fmt.Errorf("update the slice: %w", err)
	}
	return n.SetSliceBrief(ctx, id, brief)
}

// SetSliceBrief rewrites a slice's brief, which is its page body.
//
// Notion has no replace-content call, so the old blocks are trashed one by one
// and the new ones appended; only the top level is walked, because a trashed
// block takes its children with it.
func (n *Notion) SetSliceBrief(ctx context.Context, id, brief string) error {
	blocks, err := n.api.GetBlockChildren(ctx, id)
	if err != nil {
		return fmt.Errorf("read slice body: %w", err)
	}
	for _, b := range blocks {
		if err := n.api.DeleteBlock(ctx, b.ID); err != nil {
			return fmt.Errorf("clear slice body: %w", err)
		}
	}
	if children := paragraphBlocks(brief); len(children) > 0 {
		if _, err := n.api.AppendBlockChildren(ctx, id, children); err != nil {
			return fmt.Errorf("write slice body: %w", err)
		}
	}
	return nil
}

// SetDependencies records exactly the slices a slice waits on, replacing what
// it waited on before — an empty list being how a slice is freed.
func (n *Notion) SetDependencies(ctx context.Context, id string, on []string) (domain.Slice, error) {
	updated, err := n.api.UpdatePageProperties(ctx, id,
		map[string]notion.PropertyValue{notion.PropDependsOn: notion.NewRelation(on...)})
	if err != nil {
		return domain.Slice{}, err
	}
	s := domain.SliceFromPage(*updated)
	logging.Action("slice dependencies recorded", "slice", s.ID, "depends_on", len(s.DependsOn))
	return s, nil
}

// MoveSlice refiles a slice under another milestone. Only the Milestone column
// is written — the slice's own brief, status and repo say nothing about where
// in the plan it sits — and it is written in the shape that column was read in,
// which the milestone itself carries.
func (n *Notion) MoveSlice(ctx context.Context, id string, m domain.Milestone) error {
	if _, err := n.api.UpdatePageProperties(ctx, id,
		map[string]notion.PropertyValue{notion.PropMilestone: m.Ref()}); err != nil {
		return err
	}
	logging.Action("slice moved", "slice", id, "milestone", m.ID)
	return nil
}

// DeleteSlice moves a slice's page to Notion's trash. Notion has no hard
// delete, so a slice deleted by mistake is still recoverable in the Notion UI.
func (n *Notion) DeleteSlice(ctx context.Context, id string) error {
	if err := n.api.TrashPage(ctx, id); err != nil {
		return err
	}
	logging.Action("slice deleted", "slice", id)
	return nil
}

// noteBlocks turns a note into the blocks appended to a slice page: a heading,
// then one paragraph per blank-line-separated chunk. Paragraphs and nothing
// else — the note arrives as plain text, and pretending to parse markdown out
// of it would only sometimes be right.
func noteBlocks(heading, note string) []map[string]any {
	blocks := []map[string]any{textBlock("heading_3", heading)}
	return append(blocks, paragraphBlocks(note)...)
}

// paragraphBlocks turns text into a page body: one paragraph per
// blank-line-separated chunk, and nothing at all for empty text.
func paragraphBlocks(text string) []map[string]any {
	var blocks []map[string]any
	for _, chunk := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		if trimmed := strings.TrimSpace(chunk); trimmed != "" {
			blocks = append(blocks, textBlock("paragraph", trimmed))
		}
	}
	return blocks
}

// textBlock builds a block of the given type holding one span of plain text,
// which is the shape every block this package writes takes.
func textBlock(blockType, text string) map[string]any {
	return map[string]any{
		"object": "block",
		"type":   blockType,
		blockType: map[string]any{
			"rich_text": []map[string]any{{
				"type": "text",
				"text": map[string]any{"content": text},
			}},
		},
	}
}
