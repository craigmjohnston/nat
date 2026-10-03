package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// The three follow-ups the tests below hand in, as an agent would quote them.
const (
	followUpA = "Persist the split width\n\nThe width lives under one key.\nStore it per project.\nDone when: two projects keep two widths."
	followUpB = "Render the picker in a story\n\nNo story shows it open.\nDone when: a story shows it open."
	followUpC = "Remove dead code\n\nNothing calls it.\nDone when: it is gone."
)

// followUpsPlan is a project kept locally, with one slice in it — status as
// given, held by the configured user, filed under M1 — and a live agent for it
// when live is true. breakIt, when given, runs on the plan once it is seeded.
type followUpsPlan struct {
	env    Env
	out    *bytes.Buffer
	runner *agentTestRunner
	path   string
	nudges int
}

func newFollowUpsPlan(t *testing.T, status string, live bool, breakIt func(*sql.DB)) *followUpsPlan {
	t.Helper()
	cfg := testClaimConfig(t)
	p := cfg.Projects["project-1"]
	p.Backend = config.BackendLocal
	cfg.Projects["project-1"] = p

	path, err := store.PlanPath(store.ProjectOf("project-1", p))
	if err != nil {
		t.Fatalf("resolve the plan path: %v", err)
	}
	l, err := store.OpenLocal(path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close the plan: %v", err)
	}
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open the plan to seed it: %v", err)
	}
	defer func() { _ = db.Close() }()
	for _, q := range []string{
		`INSERT INTO project (id, name, has_assignee, has_branch) VALUES ('project-1', 'nat', 1, 1)`,
		`INSERT INTO milestones (name, position) VALUES ('M1', 0)`,
		`INSERT INTO slices (id, title, status, milestone, position, assignee, assignee_name, body)
		 VALUES ('` + sliceID + `', 'Render the board', '` + status + `', 'M1', 0,
		         'Craig Johnston', 'Craig Johnston', 'Do the thing.')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	if breakIt != nil {
		breakIt(db)
	}

	fp := &followUpsPlan{path: path, runner: &agentTestRunner{liveSessions: map[string]string{}}}
	if live {
		fp.runner.liveSessions[sliceID] = "nat-render"
	}
	fp.env, fp.out = testEnv(cfg, &fakeAPI{})
	fp.env.In = strings.NewReader("")
	fp.env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(fp.runner) }
	fp.env.Nudge = func() { fp.nudges++ }
	return fp
}

// run runs one command against the plan, its output starting afresh.
func (fp *followUpsPlan) run(args ...string) error {
	fp.out.Reset()
	return Run(context.Background(), append(args, "--project", "project-1"), fp.env)
}

// propose hands in the three follow-ups.
func (fp *followUpsPlan) propose(t *testing.T) {
	t.Helper()
	if err := fp.run("slice-followups", sliceID,
		"--follow-up", followUpA, "--follow-up", followUpB, "--follow-up", followUpC); err != nil {
		t.Fatalf("slice-followups: %v", err)
	}
}

// local opens the plan for a test to read back what was written.
func (fp *followUpsPlan) local(t *testing.T) *store.Local {
	t.Helper()
	l, err := store.OpenLocal(fp.path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

// body is the slice's body as the plan holds it.
func (fp *followUpsPlan) body(t *testing.T) string {
	t.Helper()
	body, err := fp.local(t).Body(context.Background(), sliceID)
	if err != nil {
		t.Fatalf("read the body: %v", err)
	}
	return body
}

// shown is slice-show --json's reading of the slice.
func (fp *followUpsPlan) shown(t *testing.T) sliceShowJSON {
	t.Helper()
	if err := fp.run("slice-show", sliceID, "--json"); err != nil {
		t.Fatalf("slice-show: %v", err)
	}
	var got sliceShowJSON
	if err := json.Unmarshal(fp.out.Bytes(), &got); err != nil {
		t.Fatalf("decode slice-show: %v\n%s", err, fp.out)
	}
	return got
}

// The whole round: the agent proposes and is refused a hand-back, the app reads
// the proposals and triages them, the agent is told in one message, and then
// hands back as it always has — leaving the queued slice in the plan, Todo and
// blocked on its parent.
func TestFollowUpsAreProposedTriagedAndThenHandedBack(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	fp.propose(t)

	wantFiled := `# Render the board

3 follow-ups filed on the slice page for the user to triage:

1. Persist the split width
2. Render the picker in a story
3. Remove dead code

Waiting for the user's decision — it arrives as a message; do not hand back before it does.
`
	if fp.out.String() != wantFiled {
		t.Errorf("slice-followups printed\n%s\nwant\n%s", fp.out, wantFiled)
	}
	if fp.nudges != 1 {
		t.Errorf("nudges = %d, want one for the filing", fp.nudges)
	}

	err := fp.run("complete-slice", sliceID, "--branch", "slice/x", "--summary", "Did it.")
	if err == nil || !strings.Contains(err.Error(), "3 follow-ups await the user's decision") {
		t.Fatalf("complete-slice with follow-ups pending: err = %v, want the refusal", err)
	}

	shown := fp.shown(t)
	wantShown := []followUpJSON{
		{1, "Persist the split width", "The width lives under one key.\nStore it per project.\nDone when: two projects keep two widths."},
		{2, "Render the picker in a story", "No story shows it open.\nDone when: a story shows it open."},
		{3, "Remove dead code", "Nothing calls it.\nDone when: it is gone."},
	}
	if !equalFollowUps(shown.FollowUps, wantShown) {
		t.Errorf("slice-show followUps = %+v, want %+v", shown.FollowUps, wantShown)
	}

	if err := fp.run("slice-triage", sliceID, "--queue", "1", "--fold", "2", "--drop", "3", "--json"); err != nil {
		t.Fatalf("slice-triage: %v", err)
	}
	var outcome triageJSON
	if err := json.Unmarshal(fp.out.Bytes(), &outcome); err != nil {
		t.Fatalf("decode slice-triage: %v\n%s", err, fp.out)
	}
	if len(outcome.Queued) != 1 || outcome.Queued[0].Title != "Persist the split width" || outcome.Queued[0].ID == "" ||
		!equalLines(outcome.Folded, []string{"Render the picker in a story"}) ||
		!equalLines(outcome.Dropped, []string{"Remove dead code"}) {
		t.Errorf("slice-triage printed %+v", outcome)
	}
	wantSent := `Follow-ups decided. Queued as slices: 1 (Persist the split width). Dropped: 3 (Remove dead code).
Fold in before handing back:

2. Render the picker in a story
   No story shows it open.
   Done when: a story shows it open.

Then hand back with nat complete-slice as usual.`
	if len(fp.runner.sends) != 1 || fp.runner.sends[0].session != "nat-render" || fp.runner.sends[0].prompt != wantSent {
		t.Errorf("sends = %+v, want one message to the agent:\n%s", fp.runner.sends, wantSent)
	}

	if shown := fp.shown(t); len(shown.FollowUps) != 0 {
		t.Errorf("slice-show after the triage: followUps = %+v, want none", shown.FollowUps)
	}
	if err := fp.run("complete-slice", sliceID, "--branch", "slice/x", "--summary", "Did it."); err != nil {
		t.Fatalf("complete-slice after the triage: %v", err)
	}

	queued, _, err := fp.local(t).Slice(context.Background(), outcome.Queued[0].ID)
	if err != nil {
		t.Fatalf("read the queued slice: %v", err)
	}
	if queued.Status != domain.SliceTodo || queued.MilestoneID != "M1" || len(queued.AssigneeIDs) != 0 ||
		!equalLines(queued.DependsOn, []string{sliceID}) {
		t.Errorf("queued slice = %+v, want Todo under M1, unassigned, waiting on its parent", queued)
	}
	brief, _ := fp.local(t).Body(context.Background(), queued.ID)
	wantBrief := "The width lives under one key.\nStore it per project.\nDone when: two projects keep two widths.\n\n" +
		`From "Render the board" (M1)`
	if brief != wantBrief {
		t.Errorf("queued brief = %q, want %q", brief, wantBrief)
	}
	if !strings.Contains(stampless(fp.body(t)), "### Follow-ups triaged\n\nAt <stamp>\n\n- Queued: Persist the split width → "+queued.ID+
		"\n- Folded in: Render the picker in a story\n- Dropped: Remove dead code") {
		t.Errorf("body = %q, want the triage recorded", fp.body(t))
	}
}

func equalFollowUps(got, want []followUpJSON) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// One pending follow-up is spoken of in the singular, and a blocked agent is
// let stop whatever is pending.
func TestCompleteSliceOverPendingFollowUps(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	if err := fp.run("slice-followups", sliceID, "--follow-up", followUpA); err != nil {
		t.Fatalf("slice-followups: %v", err)
	}
	err := fp.run("complete-slice", sliceID, "--summary", "Did it.")
	if err == nil || !strings.Contains(err.Error(), "1 follow-up awaits the user's decision; hand back once it has arrived") {
		t.Errorf("err = %v, want the singular refusal", err)
	}
	if err := fp.run("complete-slice", sliceID, "--blocked", "--summary", "Stuck."); err != nil {
		t.Errorf("complete-slice --blocked: %v, want it let through", err)
	}
}

func TestCompleteSliceReportsAFailedFollowUpsRead(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, func(db *sql.DB) {
		if _, err := db.Exec(`ALTER TABLE slices DROP COLUMN body`); err != nil {
			t.Fatalf("break the body column: %v", err)
		}
	})
	err := fp.run("complete-slice", sliceID, "--summary", "Did it.")
	if err == nil || !strings.Contains(err.Error(), "read the slice for follow-ups") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}

func TestSliceFollowUpsRefusals(t *testing.T) {
	tests := []struct {
		name   string
		status string
		args   []string
		want   string
	}{
		{"none", notion.SliceInProgress, nil, "no follow-up given"},
		{"no title", notion.SliceInProgress, []string{"--follow-up", "  \n "}, "has no title"},
		{"no brief", notion.SliceInProgress, []string{"--follow-up", "Just a title"}, `"Just a title" has no brief`},
		{"no done-condition", notion.SliceInProgress, []string{"--follow-up", "Store the width\n\nStore it per project.\ndone when: lowercase is not it."},
			`"Store the width" has no "Done when:" line`},
		{"twice", notion.SliceInProgress, []string{"--follow-up", followUpA, "--follow-up", "Persist the split width\n\nAgain.\nDone when: twice."},
			`two follow-ups are titled "Persist the split width"`},
		{"not held", notion.SliceTodo, []string{"--follow-up", followUpA}, "only a slice you claimed can be given follow-ups"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := newFollowUpsPlan(t, tt.status, true, nil)
			err := fp.run(append([]string{"slice-followups", sliceID}, tt.args...)...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
			if body := fp.body(t); body != "Do the thing." {
				t.Errorf("body = %q, want nothing written", body)
			}
		})
	}
}

func TestSliceFollowUpsMisuseAndFailures(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	for name, args := range map[string][]string{
		"no slice":    {"slice-followups", "--follow-up", followUpA},
		"not a slice": {"slice-followups", "the board", "--follow-up", followUpA},
		"bad flag":    {"slice-followups", sliceID, "--nope"},
		"no project":  {"slice-followups", sliceID, "--follow-up", followUpA},
	} {
		var err error
		if name == "no project" {
			err = Run(context.Background(), args, fp.env)
		} else {
			err = fp.run(args...)
		}
		if err == nil {
			t.Errorf("%s: want an error", name)
		}
	}

	cfg := testConfig(t)
	env, _ := testEnv(cfg, &fakeAPI{})
	err := Run(context.Background(), []string{"slice-followups", sliceID, "--follow-up", followUpA, "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "no assignee in the config") {
		t.Errorf("no assignee: err = %v", err)
	}

	broken := &fakeAPI{dataSourceErr: errors.New("boom")}
	env, _ = testEnv(testClaimConfig(t), broken)
	if err := Run(context.Background(), []string{"slice-followups", sliceID, "--follow-up", followUpA, "--project", "project-1"}, env); err == nil {
		t.Error("an unreadable workspace: want an error")
	}
}

// A plan that cannot answer its shape, the slice, or the write each stops the
// command with nothing filed.
func TestSliceFollowUpsReadAndWriteFailures(t *testing.T) {
	for name, breakIt := range map[string]func(*sql.DB) error{
		"shape": func(db *sql.DB) error { _, err := db.Exec(`ALTER TABLE milestones RENAME TO gone`); return err },
		"slice": func(db *sql.DB) error { _, err := db.Exec(`DELETE FROM slices`); return err },
		"write": func(db *sql.DB) error {
			_, err := db.Exec(`CREATE TRIGGER no_writes BEFORE UPDATE ON slices BEGIN SELECT RAISE(FAIL, 'boom'); END`)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			fp := newFollowUpsPlan(t, notion.SliceInProgress, true, func(db *sql.DB) {
				if err := breakIt(db); err != nil {
					t.Fatalf("break the plan: %v", err)
				}
			})
			if err := fp.run("slice-followups", sliceID, "--follow-up", followUpA); err == nil {
				t.Error("want an error")
			}
			if fp.nudges != 0 {
				t.Errorf("nudges = %d, want none", fp.nudges)
			}
		})
	}
}

func TestSliceFollowUpsReportsAFailedWrite(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	fp.env.Out = failingWriter{}
	if err := Run(context.Background(), []string{"slice-followups", sliceID, "--follow-up", followUpA,
		"--project", "project-1"}, fp.env); err == nil {
		t.Error("an unwritable output: want an error")
	}
}
