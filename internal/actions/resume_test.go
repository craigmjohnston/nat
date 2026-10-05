package actions

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
)

// resumeStore records each write Resume makes, in order, failing either on
// demand.
type resumeStore struct {
	recordErr, clearErr error
	writes              []string
}

func (s *resumeStore) RecordResumed(_ context.Context, id, note string) error {
	if s.recordErr != nil {
		return s.recordErr
	}
	s.writes = append(s.writes, "resumed "+id+": "+note)
	return nil
}

func (s *resumeStore) ClearBranch(_ context.Context, id string) error {
	if s.clearErr != nil {
		return s.clearErr
	}
	s.writes = append(s.writes, "clear "+id)
	return nil
}

// A handed-back slice is resumed: the Resumed goes on first, then the branch
// is cleared.
func TestResumeRecordsThenClearsTheBranch(t *testing.T) {
	st := &resumeStore{}
	s := domain.Slice{ID: "s1", Name: "Board", Status: domain.SliceClaimed, Branch: "slice/board", PRURL: "https://gh/pr/1"}
	wrote, err := Resume(context.Background(), st, s, "Add a footer.")
	if err != nil || !wrote {
		t.Fatalf("Resume = %v, %v; want true, nil", wrote, err)
	}
	if want := []string{"resumed s1: Add a footer.", "clear s1"}; !reflect.DeepEqual(st.writes, want) {
		t.Errorf("writes = %q, want %q", st.writes, want)
	}
}

// A slice in progress with no branch is work in progress already — resumed
// before, sent back, or never handed back — and nothing is written.
func TestResumeWritesNothingForWorkAlreadyInProgress(t *testing.T) {
	for _, s := range []domain.Slice{
		{ID: "s1", Status: domain.SliceClaimed, PRURL: "https://gh/pr/1"},
		{ID: "s1", Status: domain.SliceClaimed},
	} {
		st := &resumeStore{}
		if wrote, err := Resume(context.Background(), st, s, "more"); err != nil || wrote || len(st.writes) != 0 {
			t.Errorf("Resume(%+v) = %v, %v with writes %q; want false, nil, none", s, wrote, err, st.writes)
		}
	}
}

// Done is refused by name, saying the work is merged; any other status that
// is not in progress is refused too. Nothing is written for either.
func TestResumeRefusesASliceNotInProgress(t *testing.T) {
	st := &resumeStore{}
	_, err := Resume(context.Background(), st, domain.Slice{Name: "Board", Status: domain.SliceDone, StatusName: "Done", Branch: "b"}, "x")
	if err == nil || !strings.Contains(err.Error(), `"Board" is Done`) || !strings.Contains(err.Error(), "merged") {
		t.Errorf("Done: err = %v, want a refusal by name saying the work is merged", err)
	}
	_, err = Resume(context.Background(), st, domain.Slice{Name: "Board", Status: domain.SliceTodo, StatusName: "Todo"}, "x")
	if err == nil || !strings.Contains(err.Error(), `"Board" is Todo`) {
		t.Errorf("Todo: err = %v, want a refusal", err)
	}
	if len(st.writes) != 0 {
		t.Errorf("writes = %q, want none", st.writes)
	}
}

// A Resumed that fails leaves the branch where it was; a clear that fails is
// the command's error.
func TestResumeStopsAtAFailedRecord(t *testing.T) {
	s := domain.Slice{ID: "s1", Status: domain.SliceClaimed, Branch: "slice/board"}
	st := &resumeStore{recordErr: errors.New("notion down")}
	if _, err := Resume(context.Background(), st, s, "x"); err == nil || len(st.writes) != 0 {
		t.Errorf("failed record: err = %v, writes = %q; want the error and no clear", err, st.writes)
	}
	st = &resumeStore{clearErr: errors.New("notion down")}
	if _, err := Resume(context.Background(), st, s, "x"); err == nil {
		t.Error("failed clear: err = nil, want it")
	}
}
