package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// triaged runs slice-triage --json over the three proposals and decodes what it
// printed.
func (fp *followUpsPlan) triaged(t *testing.T, decision ...string) triageJSON {
	t.Helper()
	fp.propose(t)
	if err := fp.run(append([]string{"slice-triage", sliceID, "--json"}, decision...)...); err != nil {
		t.Fatalf("slice-triage: %v", err)
	}
	var got triageJSON
	if err := json.Unmarshal(fp.out.Bytes(), &got); err != nil {
		t.Fatalf("decode slice-triage: %v\n%s", err, fp.out)
	}
	return got
}

// pendingAfter is how many follow-ups the slice still has waiting.
func (fp *followUpsPlan) pendingAfter(t *testing.T) int {
	t.Helper()
	return len(store.PendingFollowUps(fp.body(t)))
}

func TestSliceTriageQueueOnly(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	got := fp.triaged(t, "--queue", "3", "--queue", "1", "--queue", "2")
	if len(got.Queued) != 3 || got.Queued[0].Title != "Persist the split width" ||
		len(got.Folded) != 0 || len(got.Dropped) != 0 {
		t.Errorf("printed %+v, want all three queued in their own order", got)
	}
	want := "Follow-ups decided. Queued as slices: 1 (Persist the split width), 2 (Render the picker in a story), " +
		"3 (Remove dead code).\nNothing to fold in — hand back now with nat complete-slice as usual."
	if len(fp.runner.sends) != 1 || fp.runner.sends[0].prompt != want {
		t.Errorf("sends = %+v, want\n%s", fp.runner.sends, want)
	}
	if n := fp.pendingAfter(t); n != 0 {
		t.Errorf("pending = %d, want none", n)
	}
}

func TestSliceTriageFoldOnly(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	got := fp.triaged(t, "--fold", "1", "--fold", "2", "--fold", "3")
	if len(got.Queued) != 0 || len(got.Folded) != 3 || len(got.Dropped) != 0 {
		t.Errorf("printed %+v, want all three folded", got)
	}
	want := `Follow-ups decided.
Fold in before handing back:

1. Persist the split width
   The width lives under one key.
   Store it per project.
   Done when: two projects keep two widths.

2. Render the picker in a story
   No story shows it open.
   Done when: a story shows it open.

3. Remove dead code
   Nothing calls it.
   Done when: it is gone.

Then hand back with nat complete-slice as usual.`
	if len(fp.runner.sends) != 1 || fp.runner.sends[0].prompt != want {
		t.Errorf("sends = %+v, want\n%s", fp.runner.sends, want)
	}
}

// Dropping everything needs no agent: the decision is recorded and the command
// says nobody was told.
func TestSliceTriageDropAllWithNoAgent(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, false, nil)
	fp.propose(t)
	if err := fp.run("slice-triage", sliceID, "--drop-all"); err != nil {
		t.Fatalf("slice-triage: %v", err)
	}
	want := `# Render the board

Queued 0 slices · folding in 0 · dropped 3.

No live agent was told; the decision is recorded on the slice page.
`
	if fp.out.String() != want {
		t.Errorf("printed\n%s\nwant\n%s", fp.out, want)
	}
	if len(fp.runner.sends) != 0 {
		t.Errorf("sends = %+v, want none with no agent", fp.runner.sends)
	}
	if n := fp.pendingAfter(t); n != 0 {
		t.Errorf("pending = %d, want none", n)
	}
	if fp.nudges != 2 {
		t.Errorf("nudges = %d, want the filing's and the triage's", fp.nudges)
	}
}

// The markdown names each queued slice where it can be found.
func TestSliceTriageMarkdownNamesTheQueuedSlices(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	fp.propose(t)
	if err := fp.run("slice-triage", sliceID, "--queue", "1", "--drop", "2", "--drop", "3"); err != nil {
		t.Fatalf("slice-triage: %v", err)
	}
	if !strings.Contains(fp.out.String(), "Queued 1 slice · folding in 0 · dropped 2.\n- Persist the split width: ") ||
		strings.Contains(fp.out.String(), "No live agent") {
		t.Errorf("printed\n%s", fp.out)
	}
}

// The record is written before the message is sent, so a send that fails still
// leaves complete-slice free to go: the command says what failed.
func TestSliceTriageFailedSendLeavesTheRecord(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	fp.runner.sendErr = "pane gone"
	fp.propose(t)
	err := fp.run("slice-triage", sliceID, "--fold", "1", "--drop", "2", "--drop", "3")
	if err == nil || !strings.Contains(err.Error(), "the triage is recorded, but telling the agent failed") {
		t.Fatalf("err = %v, want the failed send named", err)
	}
	if n := fp.pendingAfter(t); n != 0 {
		t.Errorf("pending = %d, want the record to stand", n)
	}
}

func TestSliceTriageRefusals(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		live    bool
		propose bool
		args    []string
		want    string
	}{
		{"todo", notion.SliceTodo, true, false, []string{"--drop-all"}, "has no follow-ups to triage"},
		{"nothing pending", notion.SliceInProgress, true, false, []string{"--drop-all"}, "no follow-ups awaiting a decision"},
		{"twice", notion.SliceInProgress, true, true, []string{"--queue", "1", "--drop", "1", "--drop", "2", "--drop", "3"},
			"follow-up 1 is decided twice"},
		{"left out", notion.SliceInProgress, true, true, []string{"--queue", "1"}, "follow-up 2, 3 undecided"},
		{"unknown", notion.SliceInProgress, true, true, []string{"--queue", "4"}, "no follow-up 4 awaits a decision: the pending ones are 1, 2, 3"},
		{"fold with no agent", notion.SliceInProgress, false, true, []string{"--fold", "1", "--drop", "2", "--drop", "3"},
			"no live agent to fold anything into"},
		{"drop-all and more", notion.SliceInProgress, true, true, []string{"--drop-all", "--queue", "1"}, "give it alone"},
		{"nothing decided", notion.SliceInProgress, true, true, nil, "nothing decided"},
		{"not an index", notion.SliceInProgress, true, true, []string{"--queue", "one"}, "is not a follow-up's index"},
		{"zero", notion.SliceInProgress, true, true, []string{"--queue", "0"}, "is not a follow-up's index"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := newFollowUpsPlan(t, tt.status, tt.live, nil)
			if tt.propose {
				fp.propose(t)
			}
			before := fp.body(t)
			err := fp.run(append([]string{"slice-triage", sliceID}, tt.args...)...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
			if fp.body(t) != before || len(fp.runner.sends) != 0 {
				t.Errorf("want nothing written and nothing sent")
			}
		})
	}
}

// A tmux that cannot be read refuses a fold-in, which has to be delivered; a
// triage with nothing to fold in goes ahead and tells nobody.
func TestSliceTriageOverAnUnreadableTmux(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	fp.runner.liveFatalErr = "tmux broke"
	fp.propose(t)
	if err := fp.run("slice-triage", sliceID, "--fold", "1", "--drop", "2", "--drop", "3"); err == nil ||
		!strings.Contains(err.Error(), "could not read live sessions") {
		t.Errorf("fold: err = %v, want the failed read", err)
	}
	if err := fp.run("slice-triage", sliceID, "--drop-all"); err != nil {
		t.Errorf("drop-all: %v, want it to go ahead", err)
	}
	if n := fp.pendingAfter(t); n != 0 {
		t.Errorf("pending = %d, want the drop recorded", n)
	}
}

func TestSliceTriageMisuseAndFailures(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	for name, args := range map[string][]string{
		"no slice":    {"slice-triage", "--drop-all"},
		"not a slice": {"slice-triage", "the board", "--drop-all"},
		"bad flag":    {"slice-triage", sliceID, "--nope"},
	} {
		if err := fp.run(args...); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if err := Run(context.Background(), []string{"slice-triage", sliceID, "--drop-all"}, fp.env); err == nil {
		t.Error("no project: want an error")
	}

	broken := &fakeAPI{dataSourceErr: errors.New("boom")}
	env, _ := testEnv(testClaimConfig(t), broken)
	if err := Run(context.Background(), []string{"slice-triage", sliceID, "--drop-all", "--project", "project-1"}, env); err == nil {
		t.Error("an unreadable workspace: want an error")
	}

	fp = newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	fp.propose(t)
	fp.env.Out = failingWriter{}
	if err := Run(context.Background(), []string{"slice-triage", sliceID, "--drop-all", "--json", "--project", "project-1"}, fp.env); err == nil {
		t.Error("an unwritable output: want an error")
	}
}

// A plan that cannot answer its shape, the slice or its body stops the command
// before anything is written; one that refuses the queued slice or the record
// stops it before the agent is told.
func TestSliceTriageReadAndWriteFailures(t *testing.T) {
	tests := []struct {
		name     string
		breakIt  string
		decision []string
		want     string
	}{
		{"shape", `ALTER TABLE milestones RENAME TO gone`, []string{"--drop-all"}, ""},
		{"slice", `DELETE FROM sync; DELETE FROM slices`, []string{"--drop-all"}, "load the slice"},
		{"body", `ALTER TABLE slices DROP COLUMN body`, []string{"--drop-all"}, "read the slice for follow-ups"},
		{"queue", `CREATE TRIGGER no_adds BEFORE INSERT ON slices BEGIN SELECT RAISE(FAIL, 'boom'); END`, []string{"--queue", "1", "--queue", "2", "--drop", "3"},
			`queue "Persist the split width" as a slice (0 queued before it)`},
		{"record", `CREATE TRIGGER no_writes BEFORE UPDATE ON slices BEGIN SELECT RAISE(FAIL, 'boom'); END`,
			[]string{"--drop-all"}, "record the triage on the slice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
			fp.propose(t)
			db, err := sql.Open("sqlite3", "file:"+fp.path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tt.breakIt); err != nil {
				t.Fatalf("break the plan: %v", err)
			}
			_ = db.Close()

			err = fp.run(append([]string{"slice-triage", sliceID}, tt.decision...)...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
			if len(fp.runner.sends) != 0 {
				t.Errorf("sends = %+v, want the agent left untold", fp.runner.sends)
			}
		})
	}
}

// proposeAgain hands in a second batch, as an agent would after talking to the
// user: one follow-up, titled as given.
func (fp *followUpsPlan) proposeAgain(t *testing.T, title string) {
	t.Helper()
	if err := fp.run("slice-followups", sliceID, "--follow-up", title+"\n\nMore.\nDone when: done."); err != nil {
		t.Fatalf("slice-followups: %v", err)
	}
}

// Two batches handed in are both pending, each by its batch in slice-show —
// its pending items and its log event alike — and each is triaged on its own:
// deciding the second whole leaves the first pending and tells the agent only
// of what was decided; a third batch after changes neither; and the hand-back
// waits until every batch is decided.
func TestSliceTriageOneBatchOfTwo(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	fp.propose(t)
	fp.proposeAgain(t, "Name the second thing")

	shown := fp.shown(t)
	var batches []int
	for _, f := range shown.FollowUps {
		batches = append(batches, f.Batch)
	}
	if len(shown.FollowUps) != 4 || !equalInts(batches, []int{1, 1, 1, 2}) || shown.FollowUps[3].Index != 4 {
		t.Fatalf("slice-show followUps = %+v, want four pending, the last of batch 2 and index 4", shown.FollowUps)
	}
	var eventBatches []int
	for _, e := range shown.Events {
		if e.Kind == "follow_ups" {
			eventBatches = append(eventBatches, e.Batch)
		}
	}
	if !equalInts(eventBatches, []int{1, 2}) {
		t.Errorf("follow_ups events' batches = %v, want [1 2]", eventBatches)
	}

	if err := fp.run("slice-triage", sliceID, "--drop", "4"); err != nil {
		t.Fatalf("slice-triage of batch 2: %v", err)
	}
	want := "Follow-ups decided. Dropped: 4 (Name the second thing).\nNothing to fold in — hand back now with nat complete-slice as usual."
	if len(fp.runner.sends) != 1 || fp.runner.sends[0].prompt != want {
		t.Errorf("sends = %+v, want\n%s", fp.runner.sends, want)
	}
	pending := store.PendingFollowUps(fp.body(t))
	if len(pending) != 3 || pending[0].Batch != 1 || pending[2].Index != 3 {
		t.Errorf("pending = %+v, want batch 1 whole and alone", pending)
	}
	if err := fp.run("complete-slice", sliceID, "--branch", "slice/x", "--summary", "Did it."); err == nil ||
		!strings.Contains(err.Error(), "3 follow-ups await") {
		t.Errorf("complete-slice with batch 1 pending: err = %v, want the refusal", err)
	}

	fp.proposeAgain(t, "Name the third thing")
	pending = store.PendingFollowUps(fp.body(t))
	if len(pending) != 4 || pending[3].Batch != 3 || pending[3].Title != "Name the third thing" || pending[0].Title != "Persist the split width" {
		t.Errorf("pending after a third batch = %+v, want batch 1 then batch 3", pending)
	}

	if err := fp.run("slice-triage", sliceID, "--drop-all"); err != nil {
		t.Fatalf("slice-triage --drop-all: %v", err)
	}
	if err := fp.run("complete-slice", sliceID, "--branch", "slice/x", "--summary", "Did it."); err != nil {
		t.Errorf("complete-slice with every batch decided: %v", err)
	}
}

// A call deciding part of a batch is refused with nothing written, naming the
// batch; and a later batch whose title an earlier pending batch also proposes
// is decided with that batch or after it, never before it — the record's line
// would otherwise decide the earlier one's item.
func TestSliceTriageRefusesPartOfABatch(t *testing.T) {
	tests := []struct {
		name  string
		title string
		args  []string
		want  string
	}{
		{"part of the first", "Name the second thing", []string{"--queue", "1", "--drop", "4"},
			"follow-up 2, 3 undecided: a batch is decided whole — decide every follow-up of batch 1, or pass --drop-all"},
		{"a title an earlier pending batch shares", "Remove dead code", []string{"--drop", "4"},
			`follow-up 4 ("Remove dead code") shares its title with follow-up 3, of an earlier batch still pending: decide that batch first, or both together`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
			fp.propose(t)
			fp.proposeAgain(t, tt.title)
			before := fp.body(t)
			err := fp.run(append([]string{"slice-triage", sliceID}, tt.args...)...)
			if err == nil || err.Error() != tt.want {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
			if fp.body(t) != before || len(fp.runner.sends) != 0 {
				t.Errorf("want nothing written and nothing sent")
			}
		})
	}

	// Both together decide both: the record's lines go in pending order, so
	// the earlier batch's item takes the first of the two.
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	fp.propose(t)
	fp.proposeAgain(t, "Remove dead code")
	if err := fp.run("slice-triage", sliceID, "--drop", "1", "--drop", "2", "--queue", "3", "--drop", "4"); err != nil {
		t.Fatalf("slice-triage of both: %v", err)
	}
	if n := fp.pendingAfter(t); n != 0 {
		t.Errorf("pending = %d, want none", n)
	}
	var decisions []string
	for _, e := range store.TaskEvents(fp.body(t)) {
		for _, f := range e.FollowUps {
			decisions = append(decisions, f.Decision)
		}
	}
	if !equalLines(decisions, []string{"dropped", "dropped", "queued", "dropped"}) {
		t.Errorf("decisions = %v, want batch 1's Remove dead code queued and batch 2's dropped", decisions)
	}
}

// A Done slice's undecided items — a batch an earlier nat let a later one
// supersede — are history: slice-show lists nothing pending, and slice-triage
// finds nothing to decide.
func TestADoneSlicesUndecidedBatchIsHistory(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceDone, false, func(db *sql.DB) {
		body := "Do the thing.\n\n### Follow-ups\n\n1. Old\n   Superseded.\n\n### Follow-ups\n\n1. New\n   Decided.\n\n" +
			"### Follow-ups triaged\n\n- Dropped: New"
		if _, err := db.Exec(`UPDATE slices SET body = ?`, body); err != nil {
			t.Fatalf("seed the body: %v", err)
		}
	})
	if shown := fp.shown(t); len(shown.FollowUps) != 0 {
		t.Errorf("slice-show followUps = %+v, want none on a Done slice", shown.FollowUps)
	}
	if err := fp.run("slice-triage", sliceID, "--drop-all"); err == nil ||
		!strings.Contains(err.Error(), "no follow-ups awaiting a decision") {
		t.Errorf("slice-triage: err = %v, want nothing pending", err)
	}
}

func equalInts(got, want []int) bool {
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

// The flag prints back what it was given, as flag.Value is asked to.
func TestIndexListString(t *testing.T) {
	l := indexList{1, 3}
	if got := l.String(); got != "1, 3" {
		t.Errorf("String() = %q, want %q", got, "1, 3")
	}
}

// A slice with a URL is named by it, in the provenance line and the record.
func TestLinkOfPrefersTheURL(t *testing.T) {
	if got := linkOf(domain.Slice{ID: "x", URL: "https://notion.so/x"}); got != "https://notion.so/x" {
		t.Errorf("linkOf = %q, want the URL", got)
	}
}
