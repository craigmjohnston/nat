package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// briefBlocks is a slice page body, in the shape the blocks endpoint returns it.
func briefBlocks(t *testing.T, text string) []notion.Block {
	t.Helper()
	raw := `[{"id":"b1","type":"paragraph","paragraph":{"rich_text":[{"plain_text":` +
		mustJSON(t, text) + `}]}}]`
	var blocks []notion.Block
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		t.Fatal(err)
	}
	return blocks
}

func mustJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// claimableAPI answers with a plan whose next slice is unambiguous: the
// milestone work is taken from is neither the first nor the last in the plan —
// the first is finished — and the slices under it run through every reason a
// slice cannot be taken before reaching the one that can.
func claimableAPI(t *testing.T) *fakeAPI {
	t.Helper()
	return &fakeAPI{
		blocksByID: map[string][]notion.Block{
			"project-1": conventionBlocks(t),
			"s3":        briefBlocks(t, "Render the board, then stop."),
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": assigneeSlicesDS("M1: Client", "M2: Board", "M3: Later"),
		},
		pages: map[string][]notion.Page{
			"slices-ds": {
				slicePage("s1", "Notion client", notion.SliceDone, "M1: Client", "Craig Johnston", ""),
				slicePage("s2", "Board scaffolding", notion.SliceDone, "M2: Board", "Craig Johnston", ""),
				slicePage("s3", "Render the board", notion.SliceTodo, "M2: Board", "", ""),
				slicePage("s4", "Style the board", notion.SliceTodo, "M2: Board", "", ""),
				slicePage("s5", "Queued work", notion.SliceTodo, "M3: Later", "", ""),
			},
		},
	}
}

// testClaimConfig is a config with an assignee to claim as, which is what
// onboarding writes and what claiming needs.
func testClaimConfig(t testing.TB) config.Config {
	cfg := testConfig(t)
	cfg.AssigneeUserID = "u1"
	cfg.AssigneeUserName = "Craig Johnston"
	return cfg
}

func TestNextSliceClaimsAndPrintsTheBrief(t *testing.T) {
	api := claimableAPI(t)
	env, out := testEnv(testClaimConfig(t), api)

	if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	want := `# Render the board

Claimed for Craig Johnston. Work exactly this slice.

- Project: nat
- Project page ID: project-1 (pass it as --project on every nat command)
- Milestone: M2: Board
- Notion page: s3
- Notion URL: https://notion.so/s3
- Working directory: /tmp/nat

## Brief

Render the board, then stop.

## This slice's milestone

M2: Board

- Done: Board scaffolding
- Todo: Style the board

## Project conventions

Branch per slice.
`
	if out.String() != want {
		t.Errorf("output =\n%s\nwant:\n%s", out.String(), want)
	}
}

// The slice taken is the oldest unclaimed Todo one under the lowest-ordered
// milestone still open: not a finished or later milestone's, not one already
// held, and not a later one under the same milestone.
func TestNextSliceClaimsTheRightSlice(t *testing.T) {
	api := claimableAPI(t)
	env, _ := testEnv(testClaimConfig(t), api)

	if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want exactly one", api.updates)
	}
	got := api.updates[0]
	if got.id != "s3" {
		t.Errorf("claimed %q, want s3", got.id)
	}
	if ids := got.props[notion.PropAssignee].PeopleIDs(); len(ids) != 1 || ids[0] != "u1" {
		t.Errorf("assignee = %v, want [u1]", ids)
	}
	if name := got.props[notion.PropStatus].SelectName(); name != notion.SliceInProgress {
		t.Errorf("status = %q, want %q", name, notion.SliceInProgress)
	}
	if got.props[notion.PropStatus].Select == nil {
		t.Errorf("status = %+v, want a select, the shape the page was read in", got.props[notion.PropStatus])
	}
}

// A Status column converted to Notion's own status type in the UI is written
// back in that shape, not as the select this app would have created.
func TestNextSliceWritesTheStatusShapeItRead(t *testing.T) {
	api := claimableAPI(t)
	slices := api.pages["slices-ds"]
	slices[2].Properties[notion.PropStatus] = notion.PropertyValue{
		Type: notion.TypeStatus, Status: &notion.SelectOption{Name: notion.SliceTodo},
	}
	ds := assigneeSlicesDS("M1: Client", "M2: Board", "M3: Later")
	ds.Properties[notion.PropStatus] = notion.PropertySchema{
		Type:   notion.TypeStatus,
		Status: &notion.OptionsConfig{Options: []notion.SelectOption{{Name: notion.SliceInProgress}}},
	}
	api.dataSources = map[string]notion.DataSource{"slices-ds": ds}
	env, _ := testEnv(testClaimConfig(t), api)

	if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	status := api.updates[0].props[notion.PropStatus]
	if status.Status == nil || status.Select != nil {
		t.Errorf("status = %+v, want a status value", status)
	}
}

// The plan comes with the schema, so the slices are the only query — oldest
// first, which is what makes "the next slice" mean anything.
func TestNextSliceQueriesOnlyTheSlices(t *testing.T) {
	api := claimableAPI(t)
	env, _ := testEnv(testClaimConfig(t), api)

	if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	want := query{id: "slices-ds", sorts: []notion.Sort{{Timestamp: notion.TimestampCreated, Direction: notion.SortAscending}}}
	if len(api.queries) != 1 {
		t.Fatalf("queries = %+v, want only %+v", api.queries, want)
	}
	if q := api.queries[0]; q.id != want.id || len(q.sorts) != 1 || q.sorts[0] != want.sorts[0] {
		t.Errorf("query = %+v, want %+v", q, want)
	}
}

// A slice carrying a Repo override is worked there rather than in the project's
// default directory.
func TestNextSliceHonoursARepoOverride(t *testing.T) {
	api := claimableAPI(t)
	api.pages["slices-ds"][2].Properties[notion.PropRepo] = notion.PropertyValue{
		RichText: []notion.RichText{{PlainText: "/tmp/other"}},
	}
	env, out := testEnv(testClaimConfig(t), api)

	if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if !strings.Contains(out.String(), "- Working directory: /tmp/other\n") {
		t.Errorf("output =\n%s\nwant the slice's own repo", out.String())
	}
}

// A slice with no brief written on it, in a project with no conventions, still
// prints both headings — an empty one reads as output that got cut off.
// A Done sibling whose page will not read costs the digest one line rather
// than the whole brief: the summary is simply left out.
func TestNextSliceLogsAFailedMilestoneSummaryRead(t *testing.T) {
	api := claimableAPI(t)
	api.blocksErrByID = map[string]error{"s2": errors.New("notion: 500")}
	env, out := testEnv(testClaimConfig(t), api)

	if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if !strings.Contains(out.String(), "- Done: Board scaffolding") {
		t.Errorf("output =\n%s\nwant the sibling named despite its summary failing to read", out.String())
	}
}

func TestNextSlicePrintsEmptyBodies(t *testing.T) {
	api := claimableAPI(t)
	api.blocksByID = nil
	env, out := testEnv(testClaimConfig(t), api)

	if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if strings.Count(out.String(), "_none_\n") != 2 {
		t.Errorf("output =\n%s\nwant both bodies reported as empty", out.String())
	}
}

// A page with no URL — which a Notion page always has, but a fixture need not —
// simply leaves the line out rather than printing an empty one.
func TestNextSliceOmitsAMissingURL(t *testing.T) {
	api := claimableAPI(t)
	api.pages["slices-ds"][2].URL = ""
	cfg := testClaimConfig(t)
	cfg.Projects["project-1"] = config.ProjectConfig{Name: "nat", SlicesDSID: "slices-ds"}
	env, out := testEnv(cfg, api)

	if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if strings.Contains(out.String(), "Notion URL") || strings.Contains(out.String(), "Working directory") {
		t.Errorf("output =\n%s\nwant no empty facts", out.String())
	}
}

func TestNextSlicePrintsJSON(t *testing.T) {
	api := claimableAPI(t)
	env, out := testEnv(testClaimConfig(t), api)

	if err := Run(context.Background(), []string{"next-slice", "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice --json: %v", err)
	}

	var got briefJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	want := briefJSON{
		Slice: briefSliceJSON{
			ID: "s3", Name: "Render the board", Status: notion.SliceInProgress,
			Assignee: "Craig Johnston", MilestoneID: "M2: Board", MilestoneName: "M2: Board",
			MilestoneDigest: "M2: Board\n\n- Done: Board scaffolding\n- Todo: Style the board",
			Repo:            "/tmp/nat", Brief: "Render the board, then stop.", URL: "https://notion.so/s3",
		},
		Project: projectJSON{ID: "project-1", Name: "nat", Conventions: "Branch per slice."},
	}
	if got != want {
		t.Errorf("json = %+v\nwant %+v", got, want)
	}
}

// claim's own guard — that a claim which comes back not naming the caller (or
// leaving the slice where it was) is refused rather than trusted — is no
// longer reachable through a full next-slice run: every command now claims
// through store.Local (store.Mirrored's local-first half), whose ClaimSlice
// always writes exactly the assignee it was asked to, unconditionally, so a
// local claim always sticks for whoever made it. The old next-slice-level
// test relied on Notion's own fake echoing back a mangled page — a shape that
// can no longer surface through this command at all. This tests claim() on
// its own instead, against a store.Store whose ClaimSlice hands back a slice
// that does not actually reflect the claim, which is exactly the shape the
// guard exists to catch.
func TestClaimRefusesAClaimThatDidNotStick(t *testing.T) {
	tests := []struct {
		name    string
		claimed domain.Slice
	}{
		{
			name:    "assignee dropped",
			claimed: domain.Slice{ID: sliceID, Name: "Render the board", Status: domain.SliceClaimed},
		},
		{
			name: "someone else holds it",
			claimed: domain.Slice{
				ID: sliceID, Name: "Render the board", Status: domain.SliceClaimed, AssigneeIDs: []string{"u2"},
			},
		},
		{
			name:    "status unchanged",
			claimed: domain.Slice{ID: sliceID, Name: "Render the board", Status: domain.SliceTodo},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := stubClaimStore{claimed: tt.claimed}

			_, err := claim(context.Background(), st, sliceID, store.Shape{HasAssignee: true}, "u1")

			if err == nil || !strings.Contains(err.Error(), "did not stick") {
				t.Fatalf("err = %v, want a refused claim", err)
			}
			if !strings.Contains(err.Error(), "Render the board") {
				t.Errorf("err = %q, want it to name the slice", err)
			}
		})
	}
}

// stubClaimStore answers ClaimSlice with whatever it is given and panics on
// any other call — claim() only ever calls ClaimSlice, and a test that
// reached further would be testing something else.
type stubClaimStore struct {
	store.Store
	claimed domain.Slice
}

func (s stubClaimStore) ClaimSlice(context.Context, string, store.Shape, string) (domain.Slice, error) {
	return s.claimed, nil
}

// The only call left that can still fail the whole command is the plan's own
// first read: the store's initial hydrate, which every command needs and
// none can work without. Claiming, reading the brief and reading the
// conventions used to be three more ways next-slice could fail outright —
// they no longer are. store.Mirrored.ClaimSlice writes the claim to the
// local file first and only then pushes it to the workspace; that push is
// fire-and-forget (see store.Mirrored's own doc comment), so a workspace
// that refuses the write leaves the claim landed and the command successful.
// store.Mirrored.Body falls back to the file's own stale copy — empty, for a
// body never fetched — rather than failing when the workspace cannot answer.
// See TestNextSliceSucceedsThoughTheWorkspaceCannotBeReached below for that
// half of the old test's story.
func TestNextSliceReportsAFailedCall(t *testing.T) {
	boom := errors.New("notion: 500")
	api := &fakeAPI{queryErr: map[string]error{"slices-ds": boom}}
	env, out := testEnv(testClaimConfig(t), api)

	err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env)

	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if !strings.Contains(err.Error(), "load slices") {
		t.Errorf("err = %q, want it to mention %q", err, "load slices")
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want nothing", out.String())
	}
}

// A claim that lands locally but cannot be pushed, and a brief or the
// project's conventions that cannot be freshly read, none of them fail the
// command any more — next-slice succeeds, with whatever it could read.
func TestNextSliceSucceedsThoughTheWorkspaceCannotBeReached(t *testing.T) {
	boom := errors.New("notion: 500")
	tests := []struct {
		name string
		set  func(*fakeAPI)
	}{
		{name: "the claim's push", set: func(a *fakeAPI) { a.updateErr = boom }},
		{name: "the brief", set: func(a *fakeAPI) { a.blocksErrByID = map[string]error{"s3": boom} }},
		{name: "the conventions", set: func(a *fakeAPI) { a.blocksErrByID = map[string]error{"project-1": boom} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := claimableAPI(t)
			tt.set(api)
			env, out := testEnv(testClaimConfig(t), api)

			if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
				t.Fatalf("next-slice: %v", err)
			}
			if !strings.Contains(out.String(), "Render the board") {
				t.Errorf("output =\n%s\nwant the claimed slice reported", out.String())
			}
		})
	}
}

// Once the plan is hydrated, Plan reads the file alone — so a failure there
// is a failure of the file, not anything a fakeAPI can still stage. See
// TestNextSliceReportsAFailedCall for the hydrate's own read failing instead.
func TestNextSliceReportsAFailedPlanReadOnAnAlreadyHydratedPlan(t *testing.T) {
	cfg := testClaimConfig(t)
	api := claimableAPI(t)
	path, err := store.LocalPath("project-1")
	if err != nil {
		t.Fatalf("LocalPath: %v", err)
	}
	db := hydratedPlanDBAt(t, cfg, path)
	if _, err := db.Exec(`DROP TABLE milestones`); err != nil {
		t.Fatalf("drop milestones: %v", err)
	}

	env, _ := testEnv(cfg, api)
	err = Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "milestones") {
		t.Errorf("err = %v, want the broken read reported", err)
	}
}

// Once the plan is hydrated, a claim is a write to the file first — so a
// file that cannot even record it fails the command outright, unlike a
// workspace refusing the push afterward, whose own failure is only logged.
func TestNextSliceReportsAFailedClaimOnAnAlreadyHydratedPlan(t *testing.T) {
	cfg := testClaimConfig(t)
	api := claimableAPI(t)
	env, _ := testEnv(cfg, api)
	if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice (claim s3): %v", err)
	}

	path, err := store.LocalPath("project-1")
	if err != nil {
		t.Fatalf("LocalPath: %v", err)
	}
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`DROP TABLE sync`); err != nil {
		t.Fatalf("drop sync: %v", err)
	}

	env2, _ := testEnv(cfg, api)
	err = Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env2)

	if err == nil {
		t.Error("err = nil, want the failed local write reported")
	}
}

// BodyFresh's own read fails outright rather than falling back to the
// workspace — the fallback is for a workspace that will not answer, not for
// a file that cannot even say whether its copy is fresh.
func TestNextSliceReportsAFailedBriefFreshnessCheck(t *testing.T) {
	cfg := testClaimConfig(t)
	api := claimableAPI(t)
	path, err := store.LocalPath("project-1")
	if err != nil {
		t.Fatalf("LocalPath: %v", err)
	}
	db := hydratedPlanDBAt(t, cfg, path)
	if _, err := db.Exec(`ALTER TABLE slices DROP COLUMN body_at`); err != nil {
		t.Fatalf("drop body_at: %v", err)
	}

	env, _ := testEnv(cfg, api)
	err = Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "could not read its brief") {
		t.Errorf("err = %v, want the broken freshness check reported", err)
	}
}

// The same freshness check, for the project's own conventions instead of a
// slice's brief.
func TestNextSliceReportsAFailedConventionsFreshnessCheck(t *testing.T) {
	cfg := testClaimConfig(t)
	api := claimableAPI(t)
	path, err := store.LocalPath("project-1")
	if err != nil {
		t.Fatalf("LocalPath: %v", err)
	}
	db := hydratedPlanDBAt(t, cfg, path)
	if _, err := db.Exec(`ALTER TABLE project DROP COLUMN conventions_at`); err != nil {
		t.Fatalf("drop conventions_at: %v", err)
	}

	env, _ := testEnv(cfg, api)
	err = Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "could not read the project conventions") {
		t.Errorf("err = %v, want the broken freshness check reported", err)
	}
}

// hydratedPlanDBAt hydrates the given config's project-1 plan by claiming its
// one workable slice, then hands back a raw connection to the same file so a
// test can break some column a later, already-hydrated read still has to
// use.
func hydratedPlanDBAt(t *testing.T, cfg config.Config, path string) *sql.DB {
	t.Helper()
	env, _ := testEnv(cfg, claimableAPI(t))
	if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice (hydrate): %v", err)
	}
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// A milestone sibling's own hand-back summary comes from st.Body too, and is
// read the same lazy way a slice's brief is — [Mirrored.Body] falling back to
// the workspace only once BodyFresh says the file's copy is stale, and
// falling back to the file's own stale copy, silently, when the workspace
// will not answer (see TestNextSliceLogsAFailedMilestoneSummaryRead). The one
// way this read still fails outright is BodyFresh's own check failing — a
// garbled stamp on the file, not a Notion outage.
func TestNextSliceLogsAFailedFreshnessCheckOnAMilestoneSibling(t *testing.T) {
	cfg := testClaimConfig(t)
	api := claimableAPI(t)
	path, err := store.LocalPath("project-1")
	if err != nil {
		t.Fatalf("LocalPath: %v", err)
	}
	db := hydratedPlanDBAt(t, cfg, path)
	if _, err := db.Exec(`UPDATE slices SET body_at = 'not a timestamp' WHERE id = 's2'`); err != nil {
		t.Fatalf("corrupt s2's stamp: %v", err)
	}

	env, out := testEnv(cfg, api)
	if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if !strings.Contains(out.String(), "- Done: Board scaffolding") {
		t.Errorf("output =\n%s\nwant the sibling named despite its summary failing to read", out.String())
	}
}

// Claiming needs someone to claim as, and that comes from the config the board
// wrote. Without it nothing is read and nothing is written.
func TestNextSliceNeedsAnAssignee(t *testing.T) {
	api := claimableAPI(t)
	env, out := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "no assignee in the config") {
		t.Fatalf("err = %v, want it to ask for an assignee", err)
	}
	if len(api.queries) != 0 || len(api.updates) != 0 {
		t.Errorf("calls = %+v %+v, want none", api.queries, api.updates)
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want nothing", out.String())
	}
}

// Setup that has not happened yet is reported before anything is claimed.
func TestNextSliceReportsUnfinishedSetup(t *testing.T) {
	api := claimableAPI(t)
	env, _ := testEnv(testClaimConfig(t), api)
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil }

	err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "run `nat` once to set it up") {
		t.Fatalf("err = %v, want it to point at setup", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want none", api.updates)
	}
}

func TestNextSliceReportsAFailedWrite(t *testing.T) {
	for _, args := range [][]string{{"next-slice", "--project", "project-1"}, {"next-slice", "--json", "--project", "project-1"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env, _ := testEnv(testClaimConfig(t), claimableAPI(t))
			env.Out = failingWriter{}

			err := Run(context.Background(), args, env)

			if !errors.Is(err, errWrite) {
				t.Errorf("err = %v, want %v", err, errWrite)
			}
		})
	}
}

func TestNextSliceRejectsAMisusedCommandLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown flag", args: []string{"next-slice", "--nope", "--project", "project-1"}, want: "not defined"},
		{name: "stray argument", args: []string{"next-slice", "extra", "--project", "project-1"}, want: `unexpected argument "extra"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := claimableAPI(t)
			env, out := testEnv(testClaimConfig(t), api)

			err := Run(context.Background(), tt.args, env)

			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("err = %v (%T), want a *UsageError", err, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "next-slice:") {
				t.Errorf("err = %q, want it to name the command", err)
			}
			if len(api.queries) != 0 || len(api.updates) != 0 {
				t.Errorf("calls = %+v %+v, want none: the command line was rejected", api.queries, api.updates)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// A project created without an Assignee column is claimed on status alone: the
// in-progress option is the one its own schema offers, and no people property
// is written to a table that has none.
func TestNextSliceClaimsAProjectWithNoAssigneeColumn(t *testing.T) {
	api := claimableAPI(t)
	api.dataSources = map[string]notion.DataSource{
		"slices-ds": selectMilestoneSlicesDS("M1: Client", "M2: Board", "M3: Later"),
	}
	env, out := testEnv(testClaimConfig(t), api)

	if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want exactly one", api.updates)
	}
	got := api.updates[0]
	if _, wrote := got.props[notion.PropAssignee]; wrote {
		t.Errorf("props = %+v, want no assignee written to a table without the column", got.props)
	}
	if name := got.props[notion.PropStatus].SelectName(); name != notion.SliceInProgress {
		t.Errorf("status = %q, want %q", name, notion.SliceInProgress)
	}
	if !strings.Contains(out.String(), "Claimed for Craig Johnston") {
		t.Errorf("output =\n%s\nwant the brief for the slice it took", out.String())
	}
}

// The slice handed out is the one at the top of the milestone on the project's
// own board, not whichever the query happened to return first: a plan written
// in one go shares a created time to the minute, so the board's order is the
// only order it has.
// next-slice used to read the board's own view order (notion.PlanOrder) to
// decide which Todo slice comes first. Every command now reads its plan from
// the local file store.ForProject hydrates once, and that hydrate — see
// Notion.planForPull's own doc comment — deliberately never asks for the
// board's view order at all: the local file's own position, seeded from
// whatever order the workspace's slices query answered in, is the plan's
// order from here on, on this path and every other.
func TestNextSliceTakesTheFirstSliceTheQueryAnswered(t *testing.T) {
	api := claimableAPI(t)
	// A view order that would pick a different slice first, to prove it is
	// never read.
	api.order = map[string][]string{"slices-ds": {"s1", "s2", "s5", "s4", "s3"}}
	env, _ := testEnv(testClaimConfig(t), api)

	if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if len(api.updates) != 1 || api.updates[0].id != "s3" {
		t.Fatalf("updates = %+v, want s3, the first Todo slice the query answered", api.updates)
	}
	if len(api.ordered) != 0 {
		t.Errorf("read the board's view order %v, want it never read", api.ordered)
	}
}

// An order that cannot be read is not worth refusing to work over: the slices
// stay in the order they were queried and the next one is still handed out.
func TestNextSliceWorksWithoutAReadableBoardOrder(t *testing.T) {
	api := claimableAPI(t)
	api.orderErr = errors.New("notion: 500")
	env, _ := testEnv(testClaimConfig(t), api)

	if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if len(api.updates) != 1 || api.updates[0].id != "s3" {
		t.Fatalf("updates = %+v, want s3, the plan in the order it was queried", api.updates)
	}
}

// With nothing left to take the refusal says so in the terms the plan is kept
// in: there are no statuses to activate, so it is the milestones themselves
// that are finished or empty. Nothing is written either — an agent reading the
// refusal must not mistake "none left" for a brief.
func TestNextSliceReportsNothingToClaim(t *testing.T) {
	tests := []struct {
		name    string
		options []string
		slices  []notion.Page
		wantErr []string
	}{
		{
			name:    "every milestone Done",
			options: []string{"M1: Client", "M2: Board"},
			slices: []notion.Page{
				slicePage("s1", "Notion client", notion.SliceDone, "M1: Client", "", ""),
				slicePage("s2", "Render the board", notion.SliceDone, "M2: Board", "", ""),
			},
			wantErr: []string{"no unfinished milestone", "every milestone in the plan is Done"},
		},
		{
			name:    "no milestone in the plan at all",
			options: []string{},
			slices:  []notion.Page{slicePage("s1", "Stray idea", notion.SliceTodo, "", "", "")},
			wantErr: []string{"no unfinished milestone"},
		},
		{
			name:    "nothing unclaimed under the unfinished milestones",
			options: []string{"M1: Client", "M2: Board"},
			slices: []notion.Page{
				slicePage("s1", "Notion client", notion.SliceInProgress, "M1: Client", "", ""),
				slicePage("s2", "Render the board", notion.SliceDone, "M2: Board", "", ""),
			},
			wantErr: []string{"no unclaimed Todo slice in the unfinished milestone", "M1: Client"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{
				dataSources: map[string]notion.DataSource{"slices-ds": selectMilestoneSlicesDS(tt.options...)},
				pages:       map[string][]notion.Page{"slices-ds": tt.slices},
			}
			env, out := testEnv(testClaimConfig(t), api)

			err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env)

			if err == nil {
				t.Fatal("err = nil, want a refusal")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %q, want it to mention %q", err, want)
				}
			}
			if len(api.updates) != 0 {
				t.Errorf("updates = %+v, want none", api.updates)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// A slice whose milestone is not in the plan — an option since deleted out from
// under it — is not work anyone is owed, so it is passed over rather than
// claimed under a milestone that isn't there.
func TestNextSlicePassesOverASliceOutsideThePlan(t *testing.T) {
	api := &fakeAPI{
		dataSources: map[string]notion.DataSource{"slices-ds": selectMilestoneSlicesDS("M1: Client")},
		pages:       map[string][]notion.Page{"slices-ds": {slicePage("s1", "Orphan", notion.SliceTodo, "gone", "", "")}},
	}
	env, _ := testEnv(testClaimConfig(t), api)

	err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "no unclaimed Todo slice") {
		t.Fatalf("err = %v, want the orphan passed over", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want none", api.updates)
	}
}

// The schema is read before anything is claimed, so a failure to read it leaves
// the plan untouched.
func TestNextSliceReportsAFailedSchemaRead(t *testing.T) {
	api := claimableAPI(t)
	api.dataSourceErr = errors.New("boom")
	env, _ := testEnv(testClaimConfig(t), api)

	err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "load the slices schema") {
		t.Fatalf("err = %v, want the schema read named", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing written", api.updates)
	}
}

// A project still in the shape this app started with is migrated on the way to
// the command that reads it, so an agent works a plan of the one shape whatever
// the project was stored as.
func TestNextSliceMigratesAnOldProject(t *testing.T) {
	old := notion.DataSource{ID: "slices-ds", Properties: map[string]notion.PropertySchema{
		notion.PropStatus:    notion.SchemaSelect(notion.SliceTodo, notion.SliceClaimed, notion.SliceDone),
		notion.PropDependsOn: dependsOnColumn("slices-ds"),
		notion.PropBranch:    branchColumn(),
		notion.PropMilestone: {
			Type:     "relation",
			Relation: &notion.RelationConfig{DataSourceID: "milestones-ds"},
		},
		notion.PropAssignee: {Type: notion.TypePeople},
	}}
	api := &fakeAPI{
		blocksByID: map[string][]notion.Block{
			"project-1": conventionBlocks(t),
			"s2":        briefBlocks(t, "Render the board, then stop."),
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": old,
			"milestones-ds": {ID: "milestones-ds",
				Parent: notion.Parent{Type: notion.ParentDatabase, DatabaseID: "milestones-db"}},
		},
		pages: map[string][]notion.Page{
			"milestones-ds": {
				{ID: "m1", Properties: map[string]notion.PropertyValue{notion.PropName: title("M1: Client")}},
				{ID: "m2", Properties: map[string]notion.PropertyValue{notion.PropName: title("M2: Board")}},
			},
			"slices-ds": {
				relatedSlicePage("s1", "Notion client", notion.SliceDone, "m1"),
				relatedSlicePage("s2", "Render the board", notion.SliceTodo, "m2"),
			},
		},
	}
	env, out := testEnv(testClaimConfig(t), api)

	if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	// The milestones moved onto the slices' own column, the slices were refiled
	// under them, and the claim went to the slice under M2 — all of which needs
	// the plan to have been read in its new shape.
	if len(api.schemaUpdates) != 2 {
		t.Fatalf("schema writes = %+v, want the migration's two", api.schemaUpdates)
	}
	written := api.schemaUpdates[0].props
	if got := written[notion.PropMilestone].OptionNames(); !reflect.DeepEqual(got, []string{"M1: Client", "M2: Board"}) {
		t.Errorf("options = %v, want the milestones moved onto the column", got)
	}
	// In progress arrives alongside Claimed first — the API will not rename an
	// option in place — and Claimed is retired by the second write.
	if got := written[notion.PropStatus].OptionNames(); !reflect.DeepEqual(got,
		[]string{notion.SliceTodo, notion.SliceClaimed, notion.SliceDone, notion.SliceInProgress}) {
		t.Errorf("status options = %v, want the new name appended", got)
	}
	if got := api.schemaUpdates[1].props[notion.PropStatus].OptionNames(); !reflect.DeepEqual(got,
		[]string{notion.SliceTodo, notion.SliceDone, notion.SliceInProgress}) {
		t.Errorf("status options = %v, want the old name retired", got)
	}
	if want := []string{"milestones-db"}; !reflect.DeepEqual(api.deletes, want) {
		t.Errorf("deletes = %v, want the Milestones database trashed", api.deletes)
	}
	var claimed string
	for _, u := range api.updates {
		if _, ok := u.props[notion.PropStatus]; ok {
			claimed = u.id
		}
	}
	if claimed != "s2" {
		t.Errorf("claimed %q, want s2", claimed)
	}
	if !strings.Contains(out.String(), "- Milestone: M2: Board\n") {
		t.Errorf("output =\n%s\nwant the migrated milestone named", out.String())
	}
}

// relatedSlicePage is a slice of a project in the old shape: its milestone a
// relation to a page of a Milestones data source.
func relatedSlicePage(id, name, status, milestoneID string) notion.Page {
	p := slicePage(id, name, status, "", "", "")
	p.Properties[notion.PropMilestone] = notion.PropertyValue{Relation: &[]notion.Relation{{ID: milestoneID}}}
	return p
}

// selectNextSlice is handed a domain.Project directly, so a dependency
// naming an ID the plan itself never carries — logged and never counted, the
// same rule Blockers itself follows — is exactly checkable here, whatever
// backend actually produced the plan; the local replica's own foreign keys
// make such an edge impossible to have hydrated with in the first place, so
// this is the one place left able to construct it at all.
func TestSelectNextSlicePassesOverAnUnknownDependency(t *testing.T) {
	milestone := domain.Milestone{ID: "M1", Name: "M1", Status: domain.MilestoneActive}
	plan := domain.NewProject("proj", "nat", []domain.Milestone{milestone}, []domain.Slice{
		{ID: "s1", Name: "Waits on nothing readable", Status: domain.SliceTodo, MilestoneID: "M1",
			DependsOn: []string{"nowhere"}},
	})

	_, s, err := selectNextSlice(plan)
	if err != nil {
		t.Fatalf("selectNextSlice: %v", err)
	}
	if s.ID != "s1" {
		t.Errorf("slice = %+v, want the one unknown-dependency slice handed out", s)
	}
}
