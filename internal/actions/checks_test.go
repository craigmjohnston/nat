package actions

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
)

// checksStore is a task log in memory: Body is whatever has been filed, each
// record appended in the markdown both backends write.
type checksStore struct {
	bodies              map[string]string
	bodyErr, recordErr  error
	sentBack, failedRec []string
}

func (s *checksStore) Body(_ context.Context, id string) (string, error) {
	return s.bodies[id], s.bodyErr
}

func (s *checksStore) RecordSentBack(_ context.Context, id, text string) error {
	if s.recordErr != nil {
		return s.recordErr
	}
	s.sentBack = append(s.sentBack, id)
	s.bodies[id] += "\n### Sent back\n\n" + text + "\n"
	return nil
}

func (s *checksStore) RecordChecksFailed(_ context.Context, id, text string) error {
	if s.recordErr != nil {
		return s.recordErr
	}
	s.failedRec = append(s.failedRec, id)
	s.bodies[id] += "\n### Checks failed\n\n" + text + "\n"
	return nil
}

// promptSender records every turn typed and fails on demand.
type promptSender struct {
	err  error
	sent []string
}

func (p *promptSender) SendPrompt(session, text string) error {
	if p.err != nil {
		return p.err
	}
	p.sent = append(p.sent, session+": "+text)
	return nil
}

var redSlice = domain.Slice{ID: "s1", Name: "Red", Status: domain.SliceClaimed,
	PRURL: "https://github.test/pr/1", Branch: "slice/red"}

func red(urls ...string) []FailingChecks {
	var checks []gh.Check
	for i, u := range urls {
		checks = append(checks, gh.Check{Name: []string{"test", "lint", "deploy"}[i], State: "FAILURE", URL: u})
	}
	return []FailingChecks{{Slice: redSlice, Failing: checks}}
}

// A red reading with a live session sends exactly one prompt and files a Sent
// back; the same run URLs again send nothing; a new failing run sends again.
func TestNoticeFailingChecksNudgesALiveAgentOncePerFailure(t *testing.T) {
	st := &checksStore{bodies: map[string]string{"s1": "### Handed back\n\nDone.\n"}}
	sender := &promptSender{}
	live := map[string]string{"s1": "nat-s1"}
	ctx := context.Background()

	if !NoticeFailingChecks(ctx, st, sender, live, "proj", red("https://github.test/runs/1")) {
		t.Error("first red reading wrote nothing, want a Sent back")
	}
	if len(sender.sent) != 1 || len(st.sentBack) != 1 || len(st.failedRec) != 0 {
		t.Fatalf("sent %d, sent back %d, checks failed %d; want 1, 1, 0", len(sender.sent), len(st.sentBack), len(st.failedRec))
	}
	for _, want := range []string{"nat-s1: ", "- test: https://github.test/runs/1",
		"nat slice-checks s1 --log --project proj", "nat complete-slice s1 --branch slice/red"} {
		if !strings.Contains(sender.sent[0], want) {
			t.Errorf("prompt does not say %q:\n%s", want, sender.sent[0])
		}
	}
	if !strings.Contains(st.bodies["s1"], "- test: https://github.test/runs/1") {
		t.Errorf("record = %q, want the check and its URL", st.bodies["s1"])
	}

	if NoticeFailingChecks(ctx, st, sender, live, "proj", red("https://github.test/runs/1")) || len(sender.sent) != 1 {
		t.Errorf("the same failure again sent %d prompts, want still 1 and nothing written", len(sender.sent))
	}
	NoticeFailingChecks(ctx, st, sender, live, "proj", red("https://github.test/runs/2"))
	if len(sender.sent) != 2 || len(st.sentBack) != 2 {
		t.Errorf("a new failing run: sent %d, sent back %d, want 2, 2", len(sender.sent), len(st.sentBack))
	}
	NoticeFailingChecks(ctx, st, sender, live, "proj", red("https://github.test/runs/2", "https://github.test/runs/3"))
	if len(sender.sent) != 3 {
		t.Errorf("a second check failing too: sent %d, want 3", len(sender.sent))
	}
}

// With no session live the failure is filed as Checks failed, once; a check
// with no URL goes by its name.
func TestNoticeFailingChecksRecordsWithNoAgent(t *testing.T) {
	st := &checksStore{bodies: map[string]string{}}
	failing := []FailingChecks{{Slice: redSlice, Failing: []gh.Check{{Name: "deploy", State: "ERROR"}}}}
	if !NoticeFailingChecks(context.Background(), st, &promptSender{}, nil, "proj", failing) {
		t.Error("wrote nothing, want Checks failed")
	}
	if len(st.failedRec) != 1 || !strings.HasSuffix(st.bodies["s1"], "- deploy\n") {
		t.Errorf("recorded %v, body %q; want one Checks failed naming deploy", st.failedRec, st.bodies["s1"])
	}
	if NoticeFailingChecks(context.Background(), st, nil, nil, "proj", failing) || len(st.failedRec) != 1 {
		t.Errorf("the same failure recorded %d times, want once", len(st.failedRec))
	}
}

// A failed send writes nothing and the next reading retries; a failed record
// after a send is logged and leaves nothing written; an unreadable task log is
// passed over.
func TestNoticeFailingChecksFailures(t *testing.T) {
	ctx := context.Background()
	live := map[string]string{"s1": "nat-s1"}

	st := &checksStore{bodies: map[string]string{}}
	sender := &promptSender{err: errors.New("no pane")}
	if NoticeFailingChecks(ctx, st, sender, live, "proj", red("u1")) || len(st.sentBack) != 0 {
		t.Error("a failed send wrote a record, want nothing")
	}
	sender.err = nil
	if !NoticeFailingChecks(ctx, st, sender, live, "proj", red("u1")) || len(sender.sent) != 1 {
		t.Error("the next reading did not retry the send")
	}

	st = &checksStore{bodies: map[string]string{}, recordErr: errors.New("notion down")}
	sender = &promptSender{}
	if NoticeFailingChecks(ctx, st, sender, live, "proj", red("u1")) || len(sender.sent) != 1 {
		t.Errorf("a failed record after a send: sent %d, want 1 and nothing reported written", len(sender.sent))
	}
	if NoticeFailingChecks(ctx, st, nil, nil, "proj", red("u1")) {
		t.Error("a failed Checks failed record reported written")
	}

	st = &checksStore{bodies: map[string]string{}, bodyErr: errors.New("unreadable")}
	sender = &promptSender{}
	if NoticeFailingChecks(ctx, st, sender, live, "proj", red("u1")) || len(sender.sent) != 0 {
		t.Error("an unreadable task log was acted on, want it passed over")
	}
}

// A Sent back that is a review's own comments is a different failure from the
// one the reading found, and an empty record names nothing.
func TestSameFailure(t *testing.T) {
	checks := []gh.Check{{Name: "test", URL: "https://x/1"}, {Name: "Build / unit"}}
	if !sameFailure("The checks failed:\n\n- test: https://x/1\n- Build / unit", checks) {
		t.Error("the recorded failure did not match itself")
	}
	if sameFailure("Rename the helper.\n\n- and the test", checks) {
		t.Error("a review's comments matched a failure")
	}
	if sameFailure("", checks) {
		t.Error("an empty record matched a failure")
	}
	if sameFailure("- test: https://x/1\n- other", checks) {
		t.Error("a record of the same size naming another check matched")
	}
}
