package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// fullPlanAPI answers a [Notion.planForPull]/[Notion.Plan] read with exactly
// the plan [fillPlan] wrote, so a test can pull without disturbing what is
// already in the file: every slice the reading names is one the file already
// holds, so Hydrate rewrites each in place and deletes nothing.
func fullPlanAPI() *fakeAPI {
	return &fakeAPI{
		dataSource: func(string) (*notion.DataSource, error) {
			return settledSchema(true, "M1: The format", "M2: Reads"), nil
		},
		query: func(string) ([]notion.Page, error) {
			design := slicePage("design", "Design the local plan format", notion.SliceDone)
			design.Properties[notion.PropMilestone] = notion.PropertyValue{
				Type: notion.TypeSelect, Select: &notion.SelectOption{Name: "M1: The format"}}
			design.Properties[notion.PropPR] = notion.PropertyValue{Type: "url", URL: "https://example.test/pr/1"}

			reads := slicePage("reads", "Implement the local store: reads", notion.SliceInProgress,
				notion.User{ID: "Craig Johnston", Name: "Craig Johnston"})
			reads.Properties[notion.PropMilestone] = notion.PropertyValue{
				Type: notion.TypeSelect, Select: &notion.SelectOption{Name: "M2: Reads"}}
			reads.Properties[notion.PropRepo] = notion.PropertyValue{Type: notion.TypeRichText,
				RichText: []notion.RichText{{PlainText: "/tmp/repo"}}}
			reads.Properties[notion.PropBranch] = notion.PropertyValue{Type: notion.TypeRichText,
				RichText: []notion.RichText{{PlainText: "slice/reads"}}}

			writes := slicePage("writes", "Implement the local store: writes", notion.SliceTodo)
			writes.Properties[notion.PropMilestone] = notion.PropertyValue{
				Type: notion.TypeSelect, Select: &notion.SelectOption{Name: "M2: Reads"}}

			stray := slicePage("stray", "A slice under no milestone", notion.SliceTodo)

			return []notion.Page{*design, *reads, *writes, *stray}, nil
		},
	}
}

// paragraphBlock is a single-paragraph page body as [Notion.Body] reads one
// back, for a fake GetBlockChildren to answer with.
func paragraphBlock(t *testing.T, text string) []notion.Block {
	t.Helper()
	return pageBlocks(t, `[{"id":"b1","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"`+text+`"}]}}]`)
}

// mirroredPlan opens a local plan filled by [fillPlan] and wraps it with a
// [Mirrored] over the given API, handing both back so a test can inspect the
// file directly as well as through the store.
func mirroredPlan(t *testing.T, api *fakeAPI) (*Mirrored, *Local) {
	t.Helper()
	l, _ := openPlan(t)
	fillPlan(t, l)
	stampHydrated(t, l)
	return Mirror(l, Over(api), Project{ID: "proj"}), l
}

// stampHydrated marks a fillPlan-seeded file as already hydrated, with the
// real clock rather than a test's own frozen one: fillPlan's own row carries
// no synced_at at all, which a Mirrored now reads as a file that has never
// been pulled and hydrates for itself before any other read — a pull most of
// this file's tests mean to leave untouched, against a fake API most of them
// never configured for it.
func stampHydrated(t *testing.T, l *Local) {
	t.Helper()
	write(t, l, `UPDATE project SET synced_at = ? WHERE id = ?`, timeStamp(time.Now()), "proj")
}

func TestMirroredIsAStore(t *testing.T) {
	var _ Store = Mirror(nil, nil, Project{})
}

// Every read a Mirrored answers about a slice, or the plan, or a page's
// prose the file already has fresh, comes from the file: nothing here ever
// asks the workspace.
func TestMirroredReadsAnswerFromTheFileWithNoRequest(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	write(t, l, `UPDATE slices SET body_at = ? WHERE id = ?`, timeStamp(time.Now()), "reads")
	// Stamped with the real clock, never a frozen one: a plan seeded stale
	// would have Plan pull against the fake API this test means to leave
	// untouched.
	write(t, l, `UPDATE project SET synced_at = ? WHERE id = ?`, timeStamp(time.Now()), "proj")
	api := &fakeAPI{}
	// The row fillPlan wrote is keyed "proj" — the project a read of it names
	// has to match, or Mirrored.stale reads the row's own freshness for an ID
	// it does not hold and pulls the fake API this test means to leave
	// untouched.
	localProj := Project{ID: "proj"}
	m := Mirror(l, Over(api), localProj)
	ctx := context.Background()

	if _, err := m.Shape(ctx, localProj); err != nil {
		t.Fatalf("Shape: %v", err)
	}
	if _, err := m.Plan(ctx, localProj); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	s, _, err := m.Slice(ctx, "reads")
	if err != nil {
		t.Fatalf("Slice: %v", err)
	}
	if s.Name != "Implement the local store: reads" {
		t.Errorf("slice = %+v, want the file's own", s)
	}
	body, err := m.Body(ctx, "reads")
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	if !strings.Contains(body, "Read the plan") {
		t.Errorf("body = %q, want the file's own", body)
	}
	pr, err := m.PRDescription(ctx, "reads")
	if err != nil {
		t.Fatalf("PRDescription: %v", err)
	}
	if !strings.Contains(pr, "Read a local plan") {
		t.Errorf("PR description = %q, want the file's own", pr)
	}
	if len(api.calls) != 0 {
		t.Errorf("calls = %v, want none: every read here answers from the file", api.calls)
	}
}

// A stale plan whose own re-pull fails is logged and swallowed rather than
// returned: the file already has a plan in it, and what is on screen is
// worth more than an error over how current it is.
func TestMirroredPlanSwallowsAFailedStalePull(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	stampHydrated(t, l)
	write(t, l, `UPDATE project SET synced_at = ? WHERE id = ?`,
		timeStamp(time.Now().Add(-time.Hour)), "proj")
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) { return nil, errBoom }}
	m := Mirror(l, Over(api), Project{ID: "proj"})

	plan, err := m.Plan(context.Background(), Project{ID: "proj"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Project.Slices) == 0 {
		t.Error("Plan = empty, want the stale file's own plan kept")
	}
}

// A local read that fails once the plan is hydrated and current is Plan's own
// failure to report, not something a stale re-pull could paper over.
func TestMirroredPlanCarriesTheLocalReadFailureUpOnceHydrated(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	stampHydrated(t, l)
	write(t, l, `ALTER TABLE project DROP COLUMN has_assignee`)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})

	if _, err := m.Plan(context.Background(), Project{ID: "proj"}); err == nil {
		t.Error("Plan with a broken local column: want an error")
	}
}

// A freshness check that fails once the plan is hydrated is Body's own
// failure to report.
func TestBodyCarriesAFreshnessCheckFailureUpOnceHydrated(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	stampHydrated(t, l)
	write(t, l, `ALTER TABLE slices DROP COLUMN body_at`)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})

	if _, err := m.Body(context.Background(), "reads"); err == nil {
		t.Error("Body with a broken freshness column: want an error")
	}
}

// PRDescription carries up whatever failure the Body read it is built on hit.
func TestPRDescriptionCarriesABodyFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	stampHydrated(t, l)
	write(t, l, `ALTER TABLE slices DROP COLUMN body_at`)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})

	if _, err := m.PRDescription(context.Background(), "reads"); err == nil {
		t.Error("PRDescription with a broken freshness column: want an error")
	}
}

// stale reads a freshness stamp it cannot parse as stale rather than as
// current, which is the safer of the two to assume wrongly — a plan read as
// current when it is not would never pull again on its own.
func TestMirroredPlanPullsAgainOverAnUnparseableFreshnessStamp(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	write(t, l, `UPDATE project SET synced_at = ? WHERE id = ?`, "not a time", "proj")
	api := fullPlanAPI()
	m := Mirror(l, Over(api), Project{ID: "proj"})

	if _, err := m.Plan(context.Background(), project()); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(api.calls) == 0 {
		t.Error("calls = none, want the unparseable stamp read as stale and pulled again")
	}
}

// Pull's own body — [Mirrored.pull] — carries up a failure to take a
// successful read into the file.
func TestMirroredPullCarriesAHydrateFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	stampHydrated(t, l)
	write(t, l, `DROP TABLE milestones`)
	m := Mirror(l, Over(fullPlanAPI()), Project{ID: "proj"})

	if err := m.Pull(context.Background(), project()); err == nil {
		t.Error("Pull with the milestones table gone: want an error")
	}
}

// Plan carries through the last pull's migration note, since the file has
// nowhere of its own to keep the sentence.
func TestMirroredPlanCarriesTheLastPullsMigrationNote(t *testing.T) {
	api := fullPlanAPI()
	api.dataSource = func(string) (*notion.DataSource, error) {
		ds := settledSchema(true, "M1: The format", "M2: Reads")
		delete(ds.Properties, notion.PropBranch)
		return ds, nil
	}
	m, _ := mirroredPlan(t, api)
	ctx := context.Background()

	if err := m.Pull(ctx, project()); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	plan, err := m.Plan(ctx, project())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !strings.Contains(plan.Migrated, notion.PropBranch) {
		t.Errorf("migrated = %q, want the pull's own note", plan.Migrated)
	}
}

// A slice the file has never seen is read through to the workspace, and
// taken into the file so a second read is a file read like any other.
func TestSliceMissingFromTheFileReadsThroughToTheWorkspace(t *testing.T) {
	l, _ := openPlan(t)
	api := &fakeAPI{
		page: func(id string) (*notion.Page, error) {
			return slicePage(id, "Read from the workspace", notion.SliceTodo), nil
		},
		blocks: func(string) ([]notion.Block, error) {
			return paragraphBlock(t, "Brief from the workspace."), nil
		},
	}
	m := Mirror(l, Over(api), Project{ID: "proj"})
	ctx := context.Background()

	s, sh, err := m.Slice(ctx, "remote-only")
	if err != nil {
		t.Fatalf("Slice: %v", err)
	}
	if s.Name != "Read from the workspace" {
		t.Errorf("slice = %+v, want the workspace's own", s)
	}
	if !sh.HasAssignee || !sh.HasBranch {
		t.Errorf("shape = %+v, want the file's own shape back", sh)
	}

	// Taken into the file: a second read answers with no further request.
	local, _, err := l.Slice(ctx, "remote-only")
	if err != nil {
		t.Fatalf("the slice was not taken into the file: %v", err)
	}
	if local.Name != "Read from the workspace" {
		t.Errorf("local slice = %+v, want what was taken in", local)
	}
	body, err := l.Body(ctx, "remote-only")
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	if !strings.Contains(body, "Brief from the workspace") {
		t.Errorf("body = %q, want the fetched body cached", body)
	}
}

// A slice read straight off the workspace may name a dependency the file has
// never met either — the same rule AddSlice's own ensureHeld already
// enforces, applied here because this path reaches the workspace by ID
// rather than through the plan. A dependency that cannot itself be taken in
// fails the whole read, rather than landing a slice whose edge points
// nowhere the file can ever resolve.
func TestSliceMissingFromTheFileCarriesAnUnheldDependencysFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	api := &fakeAPI{
		page: func(id string) (*notion.Page, error) {
			if id == "dep" {
				return nil, errBoom
			}
			page := slicePage(id, "Read from the workspace", notion.SliceTodo)
			page.Properties[notion.PropDependsOn] = notion.NewRelation("dep")
			return page, nil
		},
		blocks: func(string) ([]notion.Block, error) {
			return paragraphBlock(t, "Brief from the workspace."), nil
		},
	}
	m := Mirror(l, Over(api), Project{ID: "proj"})

	if _, _, err := m.Slice(context.Background(), "remote-only"); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the unheld dependency's own failure", err)
	}
}

func TestSliceMissingFromTheFileCarriesThePagesReadFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	api := &fakeAPI{page: func(string) (*notion.Page, error) { return nil, errBoom }}
	m := Mirror(l, Over(api), Project{ID: "proj"})
	if _, _, err := m.Slice(context.Background(), "ghost"); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the page read's failure", err)
	}
}

func TestSliceMissingFromTheFileCarriesTheBodysReadFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	api := &fakeAPI{
		page:   func(id string) (*notion.Page, error) { return slicePage(id, "x", notion.SliceTodo), nil },
		blocks: func(string) ([]notion.Block, error) { return nil, errBoom },
	}
	m := Mirror(l, Over(api), Project{ID: "proj"})
	if _, _, err := m.Slice(context.Background(), "ghost"); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the body read's failure", err)
	}
}

func TestMirroredSliceCarriesAnyOtherLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	if err := l.Close(); err != nil {
		t.Fatalf("close the plan early: %v", err)
	}
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if _, _, err := m.Slice(context.Background(), "whatever"); err == nil {
		t.Error("Slice on a closed file: want the failure, not a fall back to the workspace")
	}
}

// A body is fetched once, from a file copy with no stamp, and re-read
// straight from the file until a pull leaves the copy stale again.
func TestBodyFetchedOnceAndReReadAfterAPull(t *testing.T) {
	fetches := 0
	api := fullPlanAPI()
	api.blocks = func(string) ([]notion.Block, error) {
		fetches++
		return paragraphBlock(t, "Fresh from the workspace."), nil
	}
	m, _ := mirroredPlan(t, api)
	ctx := context.Background()

	body, err := m.Body(ctx, "writes")
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	if !strings.Contains(body, "Fresh from the workspace") {
		t.Errorf("body = %q, want the workspace's own", body)
	}
	if fetches != 1 {
		t.Fatalf("fetches = %d, want the one hydrating read", fetches)
	}

	if _, err := m.Body(ctx, "writes"); err != nil {
		t.Fatalf("Body (re-read): %v", err)
	}
	if fetches != 1 {
		t.Errorf("fetches = %d, want the file's own copy served with no request", fetches)
	}

	if err := m.Pull(ctx, project()); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if _, err := m.Body(ctx, "writes"); err != nil {
		t.Fatalf("Body (after pull): %v", err)
	}
	if fetches != 2 {
		t.Errorf("fetches = %d, want a pull to leave the copy stale and this read fetch it again", fetches)
	}
}

// A workspace that will not answer for a page's prose leaves the stale copy
// as the answer: the file is still the best there is.
func TestBodyFallsBackToTheStaleCopyWhenTheWorkspaceWillNotSay(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	stampHydrated(t, l)
	api := &fakeAPI{blocks: func(string) ([]notion.Block, error) { return nil, errBoom }}
	m := Mirror(l, Over(api), Project{ID: "proj"})
	body, err := m.Body(context.Background(), "writes")
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	if !strings.Contains(body, "Write the plan") {
		t.Errorf("body = %q, want the stale file copy kept as the fall back", body)
	}
}

func TestBodyCarriesAFailureToReadFreshnessUp(t *testing.T) {
	l, _ := openPlan(t)
	if err := l.Close(); err != nil {
		t.Fatalf("close early: %v", err)
	}
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if _, err := m.Body(context.Background(), "whatever"); err == nil {
		t.Error("Body against a closed file: want an error")
	}
}

func TestBodyCarriesASetBodyFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	stampHydrated(t, l)
	api := &fakeAPI{blocks: func(string) ([]notion.Block, error) {
		return paragraphBlock(t, "new"), nil
	}}
	m := Mirror(l, Over(api), Project{ID: "proj"})
	// A page not in the plan at all cannot be cached back: SetBody refuses it.
	if _, err := m.Body(context.Background(), "not-in-the-plan"); err == nil {
		t.Error("Body caching an ID neither a slice nor the project answers to: want an error")
	}
}

// A dirty slice — the file ahead of the workspace on it — is left alone by a
// pull entirely: neither its fields nor its body are written over.
func TestDirtySliceSurvivesAPull(t *testing.T) {
	api := fullPlanAPI()
	m, l := mirroredPlan(t, api)
	ctx := context.Background()

	sh, err := l.Shape(ctx, project())
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}
	if _, err := l.ClaimSlice(ctx, "writes", sh, "Somebody Else"); err != nil {
		t.Fatalf("ClaimSlice: %v", err)
	}
	dirty, err := l.Dirty(ctx, "writes")
	if err != nil || !dirty {
		t.Fatalf("Dirty = %v, %v, want it set by the claim", dirty, err)
	}

	if err := m.Pull(ctx, project()); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	s, _, err := l.Slice(ctx, "writes")
	if err != nil {
		t.Fatalf("Slice: %v", err)
	}
	if s.Status != domain.SliceClaimed || s.AssigneeName != "Somebody Else" {
		t.Errorf("slice = %+v, want the dirty local write kept over the pull's own reading", s)
	}
	dirty, err = l.Dirty(ctx, "writes")
	if err != nil || !dirty {
		t.Errorf("Dirty = %v, %v, want it still set: the pull never sent it", dirty, err)
	}
}

func TestPullCarriesTheReadsFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) { return nil, errBoom }}
	m := Mirror(l, Over(api), Project{ID: "proj"})
	if err := m.Pull(context.Background(), project()); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the read's failure", err)
	}
}

// Pull carries the query's own failure up too, distinct from the schema
// read's: a schema that reads fine but a slices query that does not still
// leaves the file untouched.
// Plan's very first read against a Mirrored that has never been hydrated at
// all pulls in the board's own view order, not left as the query gave it —
// the ordered=true half of pull neither TestMirroredPullCarriesTheQueryFailureUp
// nor any other direct call to Pull itself ever asks for, since Pull is
// always the unordered half; only the lazy hydrate-on-first-use path is.
func TestMirroredPlanHydratesInBoardOrderOnFirstEverRead(t *testing.T) {
	l, _ := openPlan(t)
	proj := Project{ID: "proj", SlicesID: slicesDS}
	m := Mirror(l, Over(fullPlanAPI()), proj)
	if _, err := m.Plan(context.Background(), proj); err != nil {
		t.Fatalf("Plan: %v", err)
	}
}

// The package-level Pull helper delegates to a Puller that has one, rather
// than only recognising the store types that do not.
func TestPullDelegatesToAPuller(t *testing.T) {
	m, _ := mirroredPlan(t, fullPlanAPI())
	if err := Pull(context.Background(), m, project()); err != nil {
		t.Fatalf("Pull: %v", err)
	}
}

func TestMirroredPullCarriesTheQueryFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	api := &fakeAPI{query: func(string) ([]notion.Page, error) { return nil, errBoom }}
	proj := Project{ID: "proj", SlicesID: slicesDS}
	m := Mirror(l, Over(api), proj)
	if err := m.Pull(context.Background(), proj); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the query's own failure", err)
	}
}

// Pull against a store with nothing behind it to pull from — a [Local] plan
// of its own — does nothing and fails nothing: there is no workspace this
// could ever mean anything against.
func TestPullAgainstAStoreWithNoWorkspaceDoesNothing(t *testing.T) {
	l, _ := openPlan(t)
	if err := Pull(context.Background(), l, project()); err != nil {
		t.Errorf("Pull against a Local: %v, want nil", err)
	}
}

func TestPullNeverReadsTheBoardsViewOrder(t *testing.T) {
	api := fullPlanAPI()
	api.order = func(string) ([]string, error) {
		t.Fatal("Pull read the board's own view order, which Hydrate never uses")
		return nil, nil
	}
	m, _ := mirroredPlan(t, api)
	if err := m.Pull(context.Background(), project()); err != nil {
		t.Fatalf("Pull: %v", err)
	}
}

// A claim lands locally first, dirty set by its own transaction, and is then
// pushed to the workspace, the flag cleared on a successful push.
func TestClaimSliceWritesLocallyThenPushes(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, err := m.Shape(ctx, project())
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}

	s, err := m.ClaimSlice(ctx, "writes", sh, "u1")
	if err != nil {
		t.Fatalf("ClaimSlice: %v", err)
	}
	if s.Status != domain.SliceClaimed {
		t.Errorf("status = %v, want claimed", s.Status)
	}
	if len(api.updates) == 0 {
		t.Fatal("want the claim pushed to the workspace")
	}
	dirty, err := l.Dirty(ctx, "writes")
	if err != nil {
		t.Fatalf("Dirty: %v", err)
	}
	if dirty {
		t.Error("dirty = true, want the successful push to have cleared it")
	}
	if _, ok, err := l.LastSynced(ctx, "writes"); err != nil || !ok {
		t.Errorf("LastSynced = %v, %v, want the push's own stamp", ok, err)
	}
}

// A push that fails does not fail the write: the local claim stands and the
// flag it set stays set for the next sync to send.
func TestClaimSlicePushFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
		return nil, errBoom
	}}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())

	s, err := m.ClaimSlice(ctx, "writes", sh, "u1")
	if err != nil {
		t.Fatalf("ClaimSlice: want the command to succeed despite the failed push, got %v", err)
	}
	if s.Status != domain.SliceClaimed {
		t.Errorf("status = %v, want the local claim to have landed", s.Status)
	}
	dirty, err := l.Dirty(ctx, "writes")
	if err != nil || !dirty {
		t.Errorf("Dirty = %v, %v, want it still set: the push never reached the workspace", dirty, err)
	}
}

// The page's own shape is read for a push that writes a status; a failure
// reading it is a push failure like any other, logged and swallowed.
func TestClaimSlicePageShapeFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{page: func(string) (*notion.Page, error) { return nil, errBoom }}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())

	if _, err := m.ClaimSlice(ctx, "writes", sh, "u1"); err != nil {
		t.Fatalf("ClaimSlice: %v", err)
	}
	dirty, err := l.Dirty(ctx, "writes")
	if err != nil || !dirty {
		t.Errorf("Dirty = %v, %v, want it still set", dirty, err)
	}
}

// A local claim's own failure — a slice the file does not hold — fails the
// command outright, before anything is pushed.
func TestClaimSliceCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if _, err := m.ClaimSlice(context.Background(), "ghost", Shape{}, "u1"); err == nil {
		t.Error("ClaimSlice on a slice not in the plan: want an error")
	}
}

func TestReleaseSliceWritesLocallyThenPushes(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())
	if _, err := l.ClaimSlice(ctx, "writes", sh, "u1"); err != nil {
		t.Fatalf("seed a claim: %v", err)
	}

	s, err := m.ReleaseSlice(ctx, "writes", sh, "Craig")
	if err != nil {
		t.Fatalf("ReleaseSlice: %v", err)
	}
	if s.Status != domain.SliceTodo {
		t.Errorf("status = %v, want Todo", s.Status)
	}
	if len(api.appended) == 0 {
		t.Error("want the release's line pushed to the workspace")
	}
	if dirty, _ := l.Dirty(ctx, "writes"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

func TestReleaseSlicePushFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{appendBlocks: func(string, []map[string]any) ([]notion.Block, error) { return nil, errBoom }}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())

	if _, err := m.ReleaseSlice(ctx, "writes", sh, "Craig"); err != nil {
		t.Fatalf("ReleaseSlice: %v", err)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); !dirty {
		t.Error("dirty = false, want it still set")
	}
}

func TestReleaseSliceCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if _, err := m.ReleaseSlice(context.Background(), "ghost", Shape{}, "Craig"); err == nil {
		t.Error("ReleaseSlice on a slice not in the plan: want an error")
	}
}

func TestCompleteSliceDoneWritesLocallyThenPushesWithThePagesShape(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())

	s, err := m.CompleteSlice(ctx, "writes", sh, Outcome{Summary: "Done."})
	if err != nil {
		t.Fatalf("CompleteSlice: %v", err)
	}
	if s.Status != domain.SliceDone {
		t.Errorf("status = %v, want Done", s.Status)
	}
	if len(api.calls) == 0 {
		t.Fatal("want the workspace pushed to")
	}
	found := false
	for _, c := range api.calls {
		if c == "GetPage" {
			found = true
		}
	}
	if !found {
		t.Error("want the page's own shape read before a status-writing push")
	}
	if dirty, _ := l.Dirty(ctx, "writes"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

// An ending with no status to write — a hand-back — never reads the page's
// shape, since nothing it pushes needs to know the Status column's type.
func TestCompleteSliceHandOffPushesWithNoPageShapeRead(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())
	if _, err := l.ClaimSlice(ctx, "writes", sh, "u1"); err != nil {
		t.Fatalf("seed a claim: %v", err)
	}

	s, err := m.CompleteSlice(ctx, "writes", sh, Outcome{Summary: "Handed off.", Branch: "slice/writes"})
	if err != nil {
		t.Fatalf("CompleteSlice: %v", err)
	}
	if s.Status != domain.SliceClaimed {
		t.Errorf("status = %v, want it left in progress", s.Status)
	}
	for _, c := range api.calls {
		if c == "GetPage" {
			t.Error("a hand-back needs no page shape and should not have read the page")
		}
	}
	if dirty, _ := l.Dirty(ctx, "writes"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

func TestCompleteSlicePushFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{appendBlocks: func(string, []map[string]any) ([]notion.Block, error) { return nil, errBoom }}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())

	if _, err := m.CompleteSlice(ctx, "writes", sh, Outcome{Summary: "Done."}); err != nil {
		t.Fatalf("CompleteSlice: %v", err)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); !dirty {
		t.Error("dirty = false, want it still set")
	}
}

func TestCompleteSliceCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if _, err := m.CompleteSlice(context.Background(), "ghost", Shape{}, Outcome{Summary: "x"}); err == nil {
		t.Error("CompleteSlice on a slice not in the plan: want an error")
	}
}

func TestRecordPRWritesLocallyThenPushes(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()

	if err := m.RecordPR(ctx, "writes", "https://example.test/pr/9"); err != nil {
		t.Fatalf("RecordPR: %v", err)
	}
	s, _, err := l.Slice(ctx, "writes")
	if err != nil {
		t.Fatalf("Slice: %v", err)
	}
	if s.PRURL != "https://example.test/pr/9" {
		t.Errorf("PR = %q, want it recorded locally", s.PRURL)
	}
	if len(api.updates) == 0 {
		t.Error("want the PR pushed to the workspace")
	}
	if dirty, _ := l.Dirty(ctx, "writes"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

func TestRecordPRPushFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
		return nil, errBoom
	}}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	if err := m.RecordPR(ctx, "writes", "https://example.test/pr/9"); err != nil {
		t.Fatalf("RecordPR: %v", err)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); !dirty {
		t.Error("dirty = false, want it still set")
	}
}

func TestRecordPRCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if err := m.RecordPR(context.Background(), "ghost", "url"); err == nil {
		t.Error("RecordPR on a slice not in the plan: want an error")
	}
}

func TestMarkDoneWritesLocallyThenPushes(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())

	if err := m.MarkDone(ctx, "writes", sh); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}
	s, _, err := l.Slice(ctx, "writes")
	if err != nil {
		t.Fatalf("Slice: %v", err)
	}
	if s.Status != domain.SliceDone {
		t.Errorf("status = %v, want Done", s.Status)
	}
	if len(api.updates) == 0 {
		t.Error("want the status pushed to the workspace")
	}
	if dirty, _ := l.Dirty(ctx, "writes"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

func TestMarkDonePushFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
		return nil, errBoom
	}}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())
	if err := m.MarkDone(ctx, "writes", sh); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); !dirty {
		t.Error("dirty = false, want it still set")
	}
}

func TestMarkDoneCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if err := m.MarkDone(context.Background(), "ghost", Shape{}); err == nil {
		t.Error("MarkDone on a slice not in the plan: want an error")
	}
}

func TestReopenSliceWritesLocallyThenPushes(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())
	if err := m.MarkDone(ctx, "writes", sh); err != nil {
		t.Fatalf("seed Done: %v", err)
	}

	if err := m.ReopenSlice(ctx, "writes", sh); err != nil {
		t.Fatalf("ReopenSlice: %v", err)
	}
	s, _, err := l.Slice(ctx, "writes")
	if err != nil {
		t.Fatalf("Slice: %v", err)
	}
	if s.Status != domain.SliceClaimed {
		t.Errorf("status = %v, want In progress", s.Status)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

func TestReopenSlicePushFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
		return nil, errBoom
	}}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())
	if err := m.ReopenSlice(ctx, "writes", sh); err != nil {
		t.Fatalf("ReopenSlice: %v", err)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); !dirty {
		t.Error("dirty = false, want it still set")
	}
}

func TestReopenSliceCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if err := m.ReopenSlice(context.Background(), "ghost", Shape{}); err == nil {
		t.Error("ReopenSlice on a slice not in the plan: want an error")
	}
}

func TestClearBranchWritesLocallyThenPushes(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()

	if err := m.ClearBranch(ctx, "writes"); err != nil {
		t.Fatalf("ClearBranch: %v", err)
	}
	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want the clear pushed", api.updates)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

func TestClearBranchPushFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
		return nil, errBoom
	}}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	if err := m.ClearBranch(ctx, "writes"); err != nil {
		t.Fatalf("ClearBranch: %v", err)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); !dirty {
		t.Error("dirty = false, want it still set")
	}
}

func TestClearBranchCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if err := m.ClearBranch(context.Background(), "ghost"); err == nil {
		t.Error("ClearBranch on a slice not in the plan: want an error")
	}
}

func TestEditSliceWritesLocallyThenPushes(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()

	if err := m.EditSlice(ctx, "writes", "New title", "/tmp/x", "New brief."); err != nil {
		t.Fatalf("EditSlice: %v", err)
	}
	s, _, err := l.Slice(ctx, "writes")
	if err != nil {
		t.Fatalf("Slice: %v", err)
	}
	if s.Name != "New title" || s.Repo != "/tmp/x" {
		t.Errorf("slice = %+v, want the edit landed locally", s)
	}
	if len(api.updates) == 0 {
		t.Error("want the edit pushed to the workspace")
	}
	if dirty, _ := l.Dirty(ctx, "writes"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

func TestEditSlicePushFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
		return nil, errBoom
	}}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	if err := m.EditSlice(ctx, "writes", "New title", "/tmp/x", "New brief."); err != nil {
		t.Fatalf("EditSlice: %v", err)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); !dirty {
		t.Error("dirty = false, want it still set")
	}
}

func TestEditSliceCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if err := m.EditSlice(context.Background(), "ghost", "t", "r", "b"); err == nil {
		t.Error("EditSlice on a slice not in the plan: want an error")
	}
}

func TestSetSliceBriefWritesLocallyThenPushes(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()

	if err := m.SetSliceBrief(ctx, "writes", "A new brief."); err != nil {
		t.Fatalf("SetSliceBrief: %v", err)
	}
	body, err := l.Body(ctx, "writes")
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	if body != "A new brief." {
		t.Errorf("body = %q", body)
	}
	if len(api.deleted) == 0 && len(api.appended) == 0 {
		t.Error("want the brief pushed to the workspace")
	}
	if dirty, _ := l.Dirty(ctx, "writes"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

func TestSetSliceBriefPushFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{blocks: func(string) ([]notion.Block, error) { return nil, errBoom }}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	if err := m.SetSliceBrief(ctx, "writes", "A new brief."); err != nil {
		t.Fatalf("SetSliceBrief: %v", err)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); !dirty {
		t.Error("dirty = false, want it still set")
	}
}

func TestSetSliceBriefCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if err := m.SetSliceBrief(context.Background(), "ghost", "b"); err == nil {
		t.Error("SetSliceBrief on a slice not in the plan: want an error")
	}
}

// Before a dependency is written locally, the file is made to hold the slice
// it names — read through to the workspace exactly as a direct read by ID
// would be.
func TestSetDependenciesEnsuresTheFileHoldsWhatItPointsAt(t *testing.T) {
	api := &fakeAPI{
		page: func(id string) (*notion.Page, error) {
			return slicePage(id, "Fetched for the dependency", notion.SliceTodo), nil
		},
		blocks: func(string) ([]notion.Block, error) { return nil, nil },
	}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()

	s, err := m.SetDependencies(ctx, "stray", []string{"remote-dep"})
	if err != nil {
		t.Fatalf("SetDependencies: %v", err)
	}
	if !reflectContains(s.DependsOn, "remote-dep") {
		t.Errorf("DependsOn = %v, want the new dependency", s.DependsOn)
	}
	if _, _, err := l.Slice(ctx, "remote-dep"); err != nil {
		t.Errorf("the dependency was not taken into the file: %v", err)
	}
	if len(api.updates) == 0 {
		t.Error("want the dependencies pushed to the workspace")
	}
	if dirty, _ := l.Dirty(ctx, "stray"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

func reflectContains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func TestSetDependenciesCarriesTheEnsureHeldFailureUp(t *testing.T) {
	api := &fakeAPI{page: func(string) (*notion.Page, error) { return nil, errBoom }}
	m, _ := mirroredPlan(t, api)
	if _, err := m.SetDependencies(context.Background(), "stray", []string{"remote-dep"}); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the read's failure", err)
	}
}

func TestSetDependenciesPushFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
		return nil, errBoom
	}}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	if _, err := m.SetDependencies(ctx, "stray", []string{"design"}); err != nil {
		t.Fatalf("SetDependencies: %v", err)
	}
	if dirty, _ := l.Dirty(ctx, "stray"); !dirty {
		t.Error("dirty = false, want it still set")
	}
}

func TestSetDependenciesCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if _, err := m.SetDependencies(context.Background(), "ghost", nil); err == nil {
		t.Error("SetDependencies on a slice not in the plan: want an error")
	}
}

func TestMoveSliceWritesLocallyThenPushes(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()

	if err := m.MoveSlice(ctx, "stray", domain.Milestone{ID: "M1: The format"}); err != nil {
		t.Fatalf("MoveSlice: %v", err)
	}
	s, _, err := l.Slice(ctx, "stray")
	if err != nil {
		t.Fatalf("Slice: %v", err)
	}
	if s.MilestoneID != "M1: The format" {
		t.Errorf("milestone = %q, want the move landed locally", s.MilestoneID)
	}
	if len(api.updates) == 0 {
		t.Error("want the move pushed to the workspace")
	}
	if dirty, _ := l.Dirty(ctx, "stray"); dirty {
		t.Error("dirty = true, want the push to have cleared it")
	}
}

func TestMoveSlicePushFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
		return nil, errBoom
	}}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	if err := m.MoveSlice(ctx, "stray", domain.Milestone{ID: "M1: The format"}); err != nil {
		t.Fatalf("MoveSlice: %v", err)
	}
	if dirty, _ := l.Dirty(ctx, "stray"); !dirty {
		t.Error("dirty = false, want it still set")
	}
}

func TestMoveSliceCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if err := m.MoveSlice(context.Background(), "ghost", domain.Milestone{}); err == nil {
		t.Error("MoveSlice on a slice not in the plan: want an error")
	}
}

func TestDeleteSliceDropsLocallyThenPushes(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()

	if err := m.DeleteSlice(ctx, "stray"); err != nil {
		t.Fatalf("DeleteSlice: %v", err)
	}
	if _, _, err := l.Slice(ctx, "stray"); !errors.Is(err, ErrSliceNotFound) {
		t.Errorf("err = %v, want the slice gone from the file", err)
	}
	if len(api.calls) == 0 || api.calls[len(api.calls)-1] != "TrashPage" {
		t.Errorf("calls = %v, want the delete pushed last", api.calls)
	}
}

// A push that fails here cannot be retried by a later sync — the row a dirty
// flag would have lived on is already gone — so the command still succeeds
// and the failure is only logged.
func TestDeleteSlicePushFailureStillSucceeds(t *testing.T) {
	api := &fakeAPI{trash: func(string) error { return errBoom }}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	if err := m.DeleteSlice(ctx, "stray"); err != nil {
		t.Fatalf("DeleteSlice: want the command to succeed despite the failed push, got %v", err)
	}
	if _, _, err := l.Slice(ctx, "stray"); !errors.Is(err, ErrSliceNotFound) {
		t.Errorf("err = %v, want the slice gone from the file regardless", err)
	}
}

func TestDeleteSliceCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if err := m.DeleteSlice(context.Background(), "ghost"); err == nil {
		t.Error("DeleteSlice on a slice not in the plan: want an error")
	}
}

// AddSlice files the slice in the workspace first, and takes the page ID it
// hands back as the slice's own — the one write that does.
func TestAddSliceTakesThePageIDAsTheSliceID(t *testing.T) {
	api := &fakeAPI{createPage: func(_ notion.Parent, properties map[string]notion.PropertyValue, _ []map[string]any) (*notion.Page, error) {
		page := slicePage("new-page-id", properties[notion.PropName].Title[0].Text.Content, notion.SliceTodo)
		page.Properties[notion.PropMilestone] = properties[notion.PropMilestone]
		page.Properties[notion.PropDependsOn] = properties[notion.PropDependsOn]
		return page, nil
	}}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()

	added, err := m.AddSlice(ctx, project(), NewSlice{
		Title: "A new slice", Brief: "Its brief.",
		Milestone: domain.Milestone{ID: "M1: The format", Name: "M1: The format"},
		DependsOn: []string{"design"},
	})
	if err != nil {
		t.Fatalf("AddSlice: %v", err)
	}
	if added.ID != "new-page-id" {
		t.Errorf("ID = %q, want the page ID the workspace handed back", added.ID)
	}
	local, _, err := l.Slice(ctx, "new-page-id")
	if err != nil {
		t.Fatalf("the slice was not taken into the file under that ID: %v", err)
	}
	if local.Name != "A new slice" || !reflectContains(local.DependsOn, "design") {
		t.Errorf("local slice = %+v, want the workspace's own answer taken in", local)
	}
	body, err := l.Body(ctx, "new-page-id")
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	if body != "Its brief." {
		t.Errorf("body = %q, want the brief just filed", body)
	}
}

func TestAddSliceCarriesTheWorkspacesFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	api := &fakeAPI{createPage: func(notion.Parent, map[string]notion.PropertyValue, []map[string]any) (*notion.Page, error) {
		return nil, errBoom
	}}
	m := Mirror(l, Over(api), Project{ID: "proj"})
	if _, err := m.AddSlice(context.Background(), project(), NewSlice{Title: "x"}); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the workspace's own failure, file untouched", err)
	}
}

func TestAddSliceCarriesTheEnsureHeldFailureUp(t *testing.T) {
	api := &fakeAPI{
		createPage: func(_ notion.Parent, properties map[string]notion.PropertyValue, _ []map[string]any) (*notion.Page, error) {
			return slicePage("new-page-id", "x", notion.SliceTodo), nil
		},
		page: func(string) (*notion.Page, error) { return nil, errBoom },
	}
	l, _ := openPlan(t)
	m := Mirror(l, Over(api), Project{ID: "proj"})
	if _, err := m.AddSlice(context.Background(), project(),
		NewSlice{Title: "x", DependsOn: []string{"missing"}}); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the dependency read's failure", err)
	}
}

func TestAddMilestonesWritesToTheWorkspaceFirstThenTakesThemIn(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())

	added, err := m.AddMilestones(ctx, project(), sh, []string{"M3: New"})
	if err != nil {
		t.Fatalf("AddMilestones: %v", err)
	}
	if len(added) != 1 || added[0].Name != "M3: New" {
		t.Errorf("added = %+v", added)
	}
	ms, err := l.milestones(ctx, l.db)
	if err != nil {
		t.Fatalf("milestones: %v", err)
	}
	found := false
	for _, m := range ms {
		found = found || m.Name == "M3: New"
	}
	if !found {
		t.Error("want the new milestone taken into the file")
	}
}

func TestAddMilestonesCarriesTheTakeMilestonesFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	api := &fakeAPI{updateSchema: func(string, map[string]notion.PropertySchema) (*notion.DataSource, error) {
		if err := l.Close(); err != nil {
			t.Fatalf("close the file mid-write: %v", err)
		}
		return settledSchema(true, "M1: The format", "M2: Reads", "M3: New"), nil
	}}
	m := Mirror(l, Over(api), Project{ID: "proj"})
	sh, _ := m.local.Shape(context.Background(), project())
	if _, err := m.AddMilestones(context.Background(), project(), sh, []string{"M3: New"}); err == nil {
		t.Error("AddMilestones with the file closed before it could take the milestone in: want an error")
	}
}

// A milestone write the workspace refuses leaves the file untouched and
// fails the command.
func TestAddMilestonesRefusalLeavesTheFileUntouched(t *testing.T) {
	api := &fakeAPI{updateSchema: func(string, map[string]notion.PropertySchema) (*notion.DataSource, error) {
		return nil, errBoom
	}}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())
	before, err := l.milestones(ctx, l.db)
	if err != nil {
		t.Fatalf("milestones: %v", err)
	}

	if _, err := m.AddMilestones(ctx, project(), sh, []string{"M3: New"}); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the workspace's own failure", err)
	}
	after, err := l.milestones(ctx, l.db)
	if err != nil {
		t.Fatalf("milestones: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("milestones = %+v, want the file untouched at %+v", after, before)
	}
}

func TestRenameMilestoneWritesToTheWorkspaceFirstThenRenamesLocally(t *testing.T) {
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) {
		return settledSchema(true, "M1: The format", "M2: Reads"), nil
	}}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())

	renamed, err := m.RenameMilestone(ctx, project(), sh, "M1: The format", "M1: Renamed")
	if err != nil {
		t.Fatalf("RenameMilestone: %v", err)
	}
	if renamed.Name != "M1: Renamed" {
		t.Errorf("renamed = %+v", renamed)
	}
	s, _, err := l.Slice(ctx, "design")
	if err != nil {
		t.Fatalf("Slice: %v", err)
	}
	if s.MilestoneID != "M1: Renamed" {
		t.Errorf("milestone = %q, want the slice refiled locally too", s.MilestoneID)
	}
}

func TestRenameMilestoneRefusalLeavesTheFileUntouched(t *testing.T) {
	api := &fakeAPI{
		dataSource: func(string) (*notion.DataSource, error) {
			return settledSchema(true, "M1: The format", "M2: Reads"), nil
		},
		updateSchema: func(string, map[string]notion.PropertySchema) (*notion.DataSource, error) {
			return nil, errBoom
		},
	}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())
	if _, err := m.RenameMilestone(ctx, project(), sh, "M1: The format", "M1: Renamed"); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the workspace's own failure", err)
	}
	if _, found := milestoneNamed(mustMilestones(t, l), "M1: The format"); !found {
		t.Error("want the old name still in the file")
	}
}

func mustMilestones(t *testing.T, l *Local) []domain.Milestone {
	t.Helper()
	ms, err := l.milestones(context.Background(), l.db)
	if err != nil {
		t.Fatalf("milestones: %v", err)
	}
	return ms
}

func TestRemoveMilestoneWritesToTheWorkspaceFirstThenRemovesLocally(t *testing.T) {
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) {
		return settledSchema(true, "Empty milestone"), nil
	}}
	l, _ := openPlan(t)
	write(t, l, `INSERT INTO project (id, name) VALUES (?, ?)`, "proj", "nat")
	write(t, l, `INSERT INTO milestones (name, position) VALUES (?, ?)`, "Empty milestone", 0)
	m := Mirror(l, Over(api), Project{ID: "proj"})
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())

	if _, err := m.RemoveMilestone(ctx, project(), sh, "Empty milestone"); err != nil {
		t.Fatalf("RemoveMilestone: %v", err)
	}
	if _, found := milestoneNamed(mustMilestones(t, l), "Empty milestone"); found {
		t.Error("want the milestone removed from the file")
	}
}

func TestRemoveMilestoneRefusalLeavesTheFileUntouched(t *testing.T) {
	api := &fakeAPI{
		dataSource: func(string) (*notion.DataSource, error) {
			return settledSchema(true, "Empty milestone"), nil
		},
		updateSchema: func(string, map[string]notion.PropertySchema) (*notion.DataSource, error) {
			return nil, errBoom
		},
	}
	l, _ := openPlan(t)
	write(t, l, `INSERT INTO project (id, name) VALUES (?, ?)`, "proj", "nat")
	write(t, l, `INSERT INTO milestones (name, position) VALUES (?, ?)`, "Empty milestone", 0)
	m := Mirror(l, Over(api), Project{ID: "proj"})
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())

	if _, err := m.RemoveMilestone(ctx, project(), sh, "Empty milestone"); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the workspace's own failure", err)
	}
	if _, found := milestoneNamed(mustMilestones(t, l), "Empty milestone"); !found {
		t.Error("want the milestone still in the file")
	}
}

func TestMoveMilestoneWritesToTheWorkspaceFirstThenReordersLocally(t *testing.T) {
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) {
		return settledSchema(true, "M1: The format", "M2: Reads"), nil
	}}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())

	moved, to, err := m.MoveMilestone(ctx, project(), sh, "M1: The format", "M2: Reads", false)
	if err != nil {
		t.Fatalf("MoveMilestone: %v", err)
	}
	if moved.Order <= to.Order {
		t.Errorf("moved = %+v, to = %+v, want it placed after", moved, to)
	}
	ms := mustMilestones(t, l)
	if ms[0].Name != "M2: Reads" {
		t.Errorf("milestones = %+v, want the file's own order updated too", ms)
	}
}

func TestMoveMilestoneRefusalLeavesTheFileUntouched(t *testing.T) {
	api := &fakeAPI{
		dataSource: func(string) (*notion.DataSource, error) {
			return settledSchema(true, "M1: The format", "M2: Reads"), nil
		},
		updateSchema: func(string, map[string]notion.PropertySchema) (*notion.DataSource, error) {
			return nil, errBoom
		},
	}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())
	before := mustMilestones(t, l)

	if _, _, err := m.MoveMilestone(ctx, project(), sh, "M1: The format", "M2: Reads", false); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the workspace's own failure", err)
	}
	after := mustMilestones(t, l)
	if after[0].Name != before[0].Name {
		t.Errorf("milestones = %+v, want the file's order untouched at %+v", after, before)
	}
}

func TestMirroredPlanCarriesTheLocalFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	if err := l.Close(); err != nil {
		t.Fatalf("close early: %v", err)
	}
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})
	if _, err := m.Plan(context.Background(), project()); err == nil {
		t.Error("Plan against a closed file: want an error")
	}
}

// A workspace slice taken into the file is surfaced whole: a failure writing
// it in is not silently dropped.
func TestSliceMissingFromTheFileCarriesTheTakeSliceFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	api := &fakeAPI{
		page: func(id string) (*notion.Page, error) { return slicePage(id, "x", notion.SliceTodo), nil },
		blocks: func(string) ([]notion.Block, error) {
			if err := l.Close(); err != nil {
				t.Fatalf("close the file mid-read: %v", err)
			}
			return nil, nil
		},
	}
	m := Mirror(l, Over(api), Project{ID: "proj"})
	if _, _, err := m.Slice(context.Background(), "ghost"); err == nil {
		t.Error("Slice with the file closed before it could be taken in: want an error")
	}
}

// A push that succeeds but cannot be marked sent — the row it would be
// marked on is gone by the time the mark is attempted — only logs: the
// command has already succeeded and there is nothing left to flag.
func TestClaimSlicePushSucceedsButMarkSentFails(t *testing.T) {
	api := &fakeAPI{}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())
	api.updatePage = func(id string, _ map[string]notion.PropertyValue) (*notion.Page, error) {
		if err := l.DeleteSlice(context.Background(), id); err != nil {
			t.Fatalf("delete the slice out from under the push: %v", err)
		}
		return &notion.Page{ID: id}, nil
	}

	if _, err := m.ClaimSlice(ctx, "writes", sh, "u1"); err != nil {
		t.Fatalf("ClaimSlice: %v", err)
	}
	if _, _, err := l.Slice(ctx, "writes"); !errors.Is(err, ErrSliceNotFound) {
		t.Errorf("err = %v, want the slice gone, as the push's own side effect left it", err)
	}
}

func TestReleaseSlicePageShapeFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{page: func(string) (*notion.Page, error) { return nil, errBoom }}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())
	if _, err := l.ClaimSlice(ctx, "writes", sh, "u1"); err != nil {
		t.Fatalf("seed a claim: %v", err)
	}
	if _, err := m.ReleaseSlice(ctx, "writes", sh, "Craig"); err != nil {
		t.Fatalf("ReleaseSlice: %v", err)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); !dirty {
		t.Error("dirty = false, want it still set")
	}
}

func TestCompleteSlicePageShapeFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{page: func(string) (*notion.Page, error) { return nil, errBoom }}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())
	if _, err := m.CompleteSlice(ctx, "writes", sh, Outcome{Summary: "Done."}); err != nil {
		t.Fatalf("CompleteSlice: %v", err)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); !dirty {
		t.Error("dirty = false, want it still set")
	}
}

func TestMarkDonePageShapeFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{page: func(string) (*notion.Page, error) { return nil, errBoom }}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())
	if err := m.MarkDone(ctx, "writes", sh); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); !dirty {
		t.Error("dirty = false, want it still set")
	}
}

func TestReopenSlicePageShapeFailureLeavesTheFlagSet(t *testing.T) {
	api := &fakeAPI{page: func(string) (*notion.Page, error) { return nil, errBoom }}
	m, l := mirroredPlan(t, api)
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())
	if err := m.ReopenSlice(ctx, "writes", sh); err != nil {
		t.Fatalf("ReopenSlice: %v", err)
	}
	if dirty, _ := l.Dirty(ctx, "writes"); !dirty {
		t.Error("dirty = false, want it still set")
	}
}

func TestAddMilestonesCarriesTheShapeReadFailureUp(t *testing.T) {
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) { return nil, errBoom }}
	m, _ := mirroredPlan(t, api)
	if _, err := m.AddMilestones(context.Background(), project(), Shape{}, []string{"M3"}); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the shape read's failure", err)
	}
}

func TestMirroredAddMilestonesWritesNothingWhenThereIsNothingToAdd(t *testing.T) {
	m, l := mirroredPlan(t, &fakeAPI{})
	ctx := context.Background()
	sh, _ := m.Shape(ctx, project())
	before := mustMilestones(t, l)

	added, err := m.AddMilestones(ctx, project(), sh, nil)
	if err != nil {
		t.Fatalf("AddMilestones: %v", err)
	}
	if added != nil {
		t.Errorf("added = %+v, want nothing", added)
	}
	after := mustMilestones(t, l)
	if len(after) != len(before) {
		t.Errorf("milestones = %+v, want the file untouched", after)
	}
}

func TestRenameMilestoneCarriesTheShapeReadFailureUp(t *testing.T) {
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) { return nil, errBoom }}
	m, _ := mirroredPlan(t, api)
	sh, _ := m.local.Shape(context.Background(), project())
	if _, err := m.RenameMilestone(context.Background(), project(), sh, "M1: The format", "New"); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the shape read's failure", err)
	}
}

func TestRemoveMilestoneCarriesTheShapeReadFailureUp(t *testing.T) {
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) { return nil, errBoom }}
	m, _ := mirroredPlan(t, api)
	sh, _ := m.local.Shape(context.Background(), project())
	if _, err := m.RemoveMilestone(context.Background(), project(), sh, "M1: The format"); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the shape read's failure", err)
	}
}

func TestMoveMilestoneCarriesTheShapeReadFailureUp(t *testing.T) {
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) { return nil, errBoom }}
	m, _ := mirroredPlan(t, api)
	sh, _ := m.local.Shape(context.Background(), project())
	if _, _, err := m.MoveMilestone(context.Background(), project(), sh,
		"M1: The format", "M2: Reads", false); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the shape read's failure", err)
	}
}

func TestAddSliceCarriesTheTakeSliceFailureUp(t *testing.T) {
	l, _ := openPlan(t)
	api := &fakeAPI{createPage: func(_ notion.Parent, properties map[string]notion.PropertyValue, _ []map[string]any) (*notion.Page, error) {
		if err := l.Close(); err != nil {
			t.Fatalf("close the file mid-write: %v", err)
		}
		return slicePage("new-page-id", properties[notion.PropName].Title[0].Text.Content, notion.SliceTodo), nil
	}}
	m := Mirror(l, Over(api), Project{ID: "proj"})
	if _, err := m.AddSlice(context.Background(), project(), NewSlice{Title: "x"}); err == nil {
		t.Error("AddSlice with the file closed before it could take the slice in: want an error")
	}
}

func TestPullCarriesTheHydrateFailureUp(t *testing.T) {
	api := fullPlanAPI()
	l, _ := openPlan(t)
	if err := l.Close(); err != nil {
		t.Fatalf("close early: %v", err)
	}
	m := Mirror(l, Over(api), Project{ID: "proj"})
	if err := m.Pull(context.Background(), project()); err == nil {
		t.Error("Pull against a closed file: want an error")
	}
}
