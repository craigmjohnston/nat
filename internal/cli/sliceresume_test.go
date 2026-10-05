package cli

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// resumeFixture is a scratch plan holding one slice handed back on a branch
// and approved — in progress, its pull request recorded — with git and gh
// answered by fakes, and a way to hand it back again.
type resumeFixture struct {
	env      Env
	id       string
	st       store.Store
	slice    domain.Slice
	handBack func()
	out      interface {
		String() string
		Reset()
	}
}

func newResumeFixture(t *testing.T) resumeFixture {
	t.Helper()
	ctx := context.Background()
	env, id, st, sp, _ := scratchWithWork(t)
	env.NewGit = func() GitCLI { return git.NewWithRunner(&fakeGitRunner{base: "origin/main", diffOut: "diff --git a/x b/x\n"}) }
	env.NewGH = func() GH { return gh.NewWithRunner(&fakeGHRunner{}) }
	sh, err := st.Shape(ctx, sp)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := st.Plan(ctx, sp)
	if err != nil {
		t.Fatal(err)
	}
	s, err := st.AddSlice(ctx, sp, store.NewSlice{Title: "approved", Milestone: plan.Shape.Milestones[0]})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReopenSlice(ctx, s.ID, sh); err != nil {
		t.Fatal(err)
	}
	handBack := func() {
		t.Helper()
		if _, err := st.CompleteSlice(ctx, s.ID, sh, store.Outcome{Summary: "Done.", Branch: "slice/approved"}); err != nil {
			t.Fatal(err)
		}
	}
	handBack()
	if err := st.RecordPR(ctx, s.ID, "https://github.test/pr/1"); err != nil {
		t.Fatal(err)
	}
	return resumeFixture{env: env, id: id, st: st, slice: s, handBack: handBack, out: env.Out.(interface {
		String() string
		Reset()
	})}
}

// run runs one command against the fixture's project, answering its output.
func (f resumeFixture) run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	f.out.Reset()
	err := Run(context.Background(), append(args, "--project", f.id), f.env)
	return f.out.String(), err
}

// resumed reads the slice's resumed flag off info --json and slice-show
// --json both, and slice-show's events.
func (f resumeFixture) resumed(t *testing.T) (info, show bool, events []taskEventJSON) {
	t.Helper()
	out, err := f.run(t, "info", "--json")
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	var doc struct {
		Slices []struct {
			ID      string `json:"id"`
			Resumed bool   `json:"resumed"`
			State   string `json:"state"`
		} `json:"slices"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("info json: %v", err)
	}
	for _, sj := range doc.Slices {
		if sj.ID == f.slice.ID {
			info = sj.Resumed
			if want := map[bool]string{true: "ready to push", false: "awaiting review"}[info]; sj.State != want {
				t.Errorf("info state = %q with resumed %v, want %q", sj.State, info, want)
			}
		} else if sj.Resumed {
			t.Errorf("slice %s reads as resumed with no pull request", sj.ID)
		}
	}
	out, err = f.run(t, "slice-show", f.slice.ID, "--json")
	if err != nil {
		t.Fatalf("slice-show: %v", err)
	}
	var sj struct {
		Resumed bool            `json:"resumed"`
		Events  []taskEventJSON `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &sj); err != nil {
		t.Fatalf("slice-show json: %v", err)
	}
	return info, sj.Resumed, sj.Events
}

// kinds is the body events' kinds, in order, without slice-show's own.
func kinds(events []taskEventJSON) []string {
	var out []string
	for _, e := range events {
		if e.Kind != "approved" && e.Kind != "merged" {
			out = append(out, e.Kind)
		}
	}
	return out
}

// slice-resume on a handed-back, approved slice files a stamped Resumed and
// clears the branch: info and slice-show read it as resumed and in progress,
// the diff still reads its branch, and a second run writes nothing. The next
// hand-back is a second Handed back after the Resumed, and the slice is back
// at its pull request.
func TestSliceResumeTakesAHandedBackSliceBackToWork(t *testing.T) {
	f := newResumeFixture(t)
	var nudges int
	f.env.Nudge = func() { nudges++ }

	if info, show, _ := f.resumed(t); info || show {
		t.Errorf("handed back and approved: resumed = %v/%v, want false", info, show)
	}
	out, err := f.run(t, "slice-resume", f.slice.ID, "--note", "Add a footer.")
	if err != nil {
		t.Fatalf("slice-resume: %v", err)
	}
	if !strings.Contains(out, "Resumed.") || nudges != 1 {
		t.Errorf("output %q, nudges %d; want the confirmation and one nudge", out, nudges)
	}
	info, show, events := f.resumed(t)
	if !info || !show {
		t.Errorf("after slice-resume: resumed = %v/%v, want true", info, show)
	}
	if got := kinds(events); !reflect.DeepEqual(got, []string{"handed_back", "resumed"}) {
		t.Errorf("events = %q, want the hand-back then the resumed", got)
	}
	if last := events[len(events)-2]; last.Note != "Add a footer." || last.At == "" {
		t.Errorf("resumed event = %+v, want the note and its stamp", last)
	}
	if out, err := f.run(t, "slice-diff", f.slice.ID); err != nil || !strings.Contains(out, "diff --git") {
		t.Errorf("slice-diff on the resumed slice = %q, %v; want its branch read", out, err)
	}

	out, err = f.run(t, "slice-resume", f.slice.ID, "--note", "Add a footer.")
	if err != nil || !strings.Contains(out, "nothing was written") || nudges != 1 {
		t.Errorf("second slice-resume = %q, %v, nudges %d; want nothing written", out, err, nudges)
	}
	if _, _, again := f.resumed(t); len(again) != len(events) {
		t.Errorf("events after a second run = %d, want still %d", len(again), len(events))
	}

	f.handBack()
	info, show, events = f.resumed(t)
	if info || show {
		t.Errorf("handed back again: resumed = %v/%v, want false", info, show)
	}
	if got := kinds(events); !reflect.DeepEqual(got, []string{"handed_back", "resumed", "handed_back"}) {
		t.Errorf("events = %q, want the second hand-back after the resumed", got)
	}
	if e := events[2]; e.Kind != "handed_back" || e.At == "" {
		t.Errorf("second hand-back = %+v, want it stamped", e)
	}
}

// The note is required, and may be piped in; a Done slice is refused by name,
// and a slice that cannot be read is the command's error.
func TestSliceResumeRefusals(t *testing.T) {
	f := newResumeFixture(t)
	if _, err := f.run(t, "slice-resume", f.slice.ID); err == nil || !strings.Contains(err.Error(), "no note given") {
		t.Errorf("no note: err = %v, want a refusal", err)
	}
	if _, err := f.run(t, "slice-resume", "--note", "x"); err == nil {
		t.Error("no slice: err = nil, want a usage error")
	}
	if _, err := f.run(t, "slice-resume", "not-a-page", "--note", "x"); err == nil {
		t.Error("a bad slice ref: err = nil, want a refusal")
	}
	if _, err := f.run(t, "slice-resume", "3b738308-f654-8170-8c99-eccab4463d8f", "--note", "x"); err == nil {
		t.Error("an unknown slice: err = nil, want the read's failure")
	}
	if _, err := f.run(t, "slice-resume", f.slice.ID, "--note", "-"); err == nil {
		t.Error("--note - with no stdin: err = nil, want a refusal")
	}
	f.env.In = strings.NewReader("Piped.\n")
	if _, err := f.run(t, "slice-resume", f.slice.ID, "--note", "-"); err != nil {
		t.Fatalf("piped note: %v", err)
	}
	if _, _, events := f.resumed(t); events[len(events)-2].Note != "Piped." {
		t.Errorf("events = %+v, want the piped note", events)
	}

	f.env.In = nil
	ctx := context.Background()
	sh, err := f.st.Shape(ctx, storeProject(f.id, config.ProjectConfig{}))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.MarkDone(ctx, f.slice.ID, sh); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(t, "slice-resume", f.slice.ID, "--note", "x"); err == nil || !strings.Contains(err.Error(), `"approved" is Done`) {
		t.Errorf("Done: err = %v, want a refusal by name", err)
	}
	if err := Run(ctx, []string{"slice-resume", f.slice.ID, "--note", "x"}, f.env); err == nil {
		t.Error("no --project: err = nil, want a refusal")
	}
}

// failingBodyStore is a plan whose slice bodies cannot be read.
type failingBodyStore struct{ store.Store }

func (failingBodyStore) Body(context.Context, string) (string, error) {
	return "", errBodyUnread
}

var errBodyUnread = errors.New("body unread")

// A slice with no branch is diffed only where its task log holds a hand-back:
// never handed back, or a log that cannot be read, is no earlier hand-back.
func TestHandedBackBefore(t *testing.T) {
	f := newResumeFixture(t)
	ctx := context.Background()
	if !handedBackBefore(ctx, f.st, f.slice.ID) {
		t.Error("a slice handed back reads as never handed back")
	}
	if handedBackBefore(ctx, failingBodyStore{f.st}, f.slice.ID) {
		t.Error("an unreadable task log concluded a hand-back")
	}
	plan, err := f.st.Plan(ctx, storeProject(f.id, config.ProjectConfig{}))
	if err != nil {
		t.Fatal(err)
	}
	var fresh domain.Slice
	for _, s := range plan.Project.Slices {
		if s.ID != f.slice.ID {
			fresh = s
		}
	}
	if handedBackBefore(ctx, f.st, fresh.ID) {
		t.Error("a slice never handed back reads as handed back")
	}
	sh, err := f.st.Shape(ctx, storeProject(f.id, config.ProjectConfig{}))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.ReopenSlice(ctx, fresh.ID, sh); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(t, "slice-diff", fresh.ID); err == nil || !strings.Contains(err.Error(), "is not handed back") {
		t.Errorf("slice-diff on a slice never handed back: err = %v, want the refusal", err)
	}
}

// A Sent back a checks nudge filed comes back from slice-show --json with by
// set to CI; a review's own carries none.
func TestSliceShowAttributesANudgeSentBack(t *testing.T) {
	ctx := context.Background()
	env, id, st, sp, _ := scratchWithWork(t)
	env.NewGit = func() GitCLI { return git.NewWithRunner(&fakeGitRunner{base: "origin/main"}) }
	plan, err := st.Plan(ctx, sp)
	if err != nil {
		t.Fatal(err)
	}
	s := plan.Project.Slices[0]
	if err := st.RecordSentBack(ctx, s.ID, "Rename the helper."); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordSentBack(ctx, s.ID, actions.ChecksProvenance+"\n\n- test: https://ci/1"); err != nil {
		t.Fatal(err)
	}
	out := env.Out.(interface {
		String() string
		Reset()
	})
	out.Reset()
	if err := Run(ctx, []string{"slice-show", s.ID, "--json", "--project", id}, env); err != nil {
		t.Fatalf("slice-show: %v", err)
	}
	var doc struct {
		Events []taskEventJSON `json:"events"`
	}
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatal(err)
	}
	var sentBack []taskEventJSON
	for _, e := range doc.Events {
		if e.Kind == "sent_back" {
			sentBack = append(sentBack, e)
		}
	}
	want := []taskEventJSON{{Kind: "sent_back", Note: "Rename the helper."}, {Kind: "sent_back", Note: "- test: https://ci/1", By: "CI"}}
	if len(sentBack) != 2 || sentBack[0].By != want[0].By || sentBack[1].By != want[1].By || sentBack[1].Note != want[1].Note {
		t.Errorf("sent backs = %+v, want %+v", sentBack, want)
	}
}

// A bad flag is a usage error, and a workspace whose schema cannot be read
// stops the command before anything is written.
func TestSliceResumeMisuseAndFailures(t *testing.T) {
	api := &fakeAPI{pages: map[string][]notion.Page{"slices-ds": {}}}
	env, _ := testEnv(testClaimConfig(t), api)
	env.Out = &strings.Builder{}
	if err := Run(context.Background(), []string{"slice-resume", testSliceID, "--nope", "--note", "x", "--project", "project-1"}, env); err == nil {
		t.Error("a bad flag: want an error")
	}

	broken := &fakeAPI{dataSourceErr: errors.New("boom")}
	env, _ = testEnv(testClaimConfig(t), broken)
	env.Out = &strings.Builder{}
	if err := Run(context.Background(), []string{"slice-resume", testSliceID, "--note", "x", "--project", "project-1"}, env); err == nil {
		t.Error("an unreadable schema: want an error")
	}
	if len(broken.appends) != 0 || len(broken.updates) != 0 {
		t.Errorf("appends %+v, updates %+v: want nothing written", broken.appends, broken.updates)
	}
}

// takenBackFlags reads one slice's taken_back and resumed off info --json and
// slice-show --json both.
func (f resumeFixture) takenBackFlags(t *testing.T, id string) (info, show, resumed bool) {
	t.Helper()
	out, err := f.run(t, "info", "--json")
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	var doc struct {
		Slices []struct {
			ID        string `json:"id"`
			TakenBack bool   `json:"taken_back"`
		} `json:"slices"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("info json: %v", err)
	}
	for _, sj := range doc.Slices {
		if sj.ID == id {
			info = sj.TakenBack
		}
	}
	out, err = f.run(t, "slice-show", id, "--json")
	if err != nil {
		t.Fatalf("slice-show: %v", err)
	}
	var sj struct {
		TakenBack bool `json:"taken_back"`
		Resumed   bool `json:"resumed"`
	}
	if err := json.Unmarshal([]byte(out), &sj); err != nil {
		t.Fatalf("slice-show json: %v", err)
	}
	return info, sj.TakenBack, sj.Resumed
}

// A slice in review — handed back, no pull request yet — taken back to work
// reads as taken back, and not as resumed, which needs a pull request; it
// stops once it is handed back again. A slice resumed after its approval is
// taken back too, and one never handed back never is.
func TestInfoAndSliceShowReadTakenBack(t *testing.T) {
	f := newResumeFixture(t)
	ctx := context.Background()
	sp := storeProject(f.id, config.ProjectConfig{})
	sh, err := f.st.Shape(ctx, sp)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := f.st.Plan(ctx, sp)
	if err != nil {
		t.Fatal(err)
	}
	review, err := f.st.AddSlice(ctx, sp, store.NewSlice{Title: "in review", Milestone: plan.Shape.Milestones[0]})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.ReopenSlice(ctx, review.ID, sh); err != nil {
		t.Fatal(err)
	}
	if info, show, _ := f.takenBackFlags(t, review.ID); info || show {
		t.Errorf("never handed back: taken_back = %v/%v, want false", info, show)
	}
	handBack := func() {
		t.Helper()
		if _, err := f.st.CompleteSlice(ctx, review.ID, sh, store.Outcome{Summary: "Done.", Branch: "slice/in-review"}); err != nil {
			t.Fatal(err)
		}
	}
	handBack()
	if info, show, _ := f.takenBackFlags(t, review.ID); info || show {
		t.Errorf("handed back: taken_back = %v/%v, want false", info, show)
	}
	if _, err := f.run(t, "slice-resume", review.ID, "--note", "Rename it."); err != nil {
		t.Fatalf("slice-resume: %v", err)
	}
	if info, show, resumed := f.takenBackFlags(t, review.ID); !info || !show || resumed {
		t.Errorf("taken back with no PR: taken_back = %v/%v, resumed %v; want true, true, false", info, show, resumed)
	}
	handBack()
	if info, show, _ := f.takenBackFlags(t, review.ID); info || show {
		t.Errorf("handed back again: taken_back = %v/%v, want false", info, show)
	}

	if _, err := f.run(t, "slice-resume", f.slice.ID, "--note", "More."); err != nil {
		t.Fatalf("slice-resume: %v", err)
	}
	if info, show, resumed := f.takenBackFlags(t, f.slice.ID); !info || !show || !resumed {
		t.Errorf("resumed after approval: taken_back = %v/%v, resumed %v; want all true", info, show, resumed)
	}
}

// takenBack needs a Branch column to have cleared: a project with none is
// never read as taken back, and the hand-back question is not even asked.
func TestTakenBackNeedsABranchColumn(t *testing.T) {
	asked := false
	s := domain.Slice{Status: domain.SliceClaimed}
	if takenBack(s, false, func() bool { asked = true; return true }) || asked {
		t.Errorf("no Branch column: taken back, or asked %v", asked)
	}
}
