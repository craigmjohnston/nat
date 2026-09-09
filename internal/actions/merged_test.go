package actions

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
)

// fakeViewer stands in for gh's pull request reading: it records what it was
// asked about and answers with the pull request — or the refusal — the test
// wants gh to have given.
type fakeViewer struct {
	pr     gh.PR
	err    error
	viewed []string
}

var _ PRViewer = (*fakeViewer)(nil)

func (f *fakeViewer) ViewPR(dir, ref string) (gh.PR, error) {
	f.viewed = append(f.viewed, dir+" "+ref)
	return f.pr, f.err
}

// TestMarkDone covers the whole write: Done in the shape the page's own
// Status column was read as, and nothing else touched.
func TestMarkDone(t *testing.T) {
	client := &fakeClient{getPage: func(id string) (*notion.Page, error) {
		return &notion.Page{ID: id, Properties: map[string]notion.PropertyValue{
			notion.PropStatus: {Type: notion.TypeStatus, Status: &notion.SelectOption{Name: notion.SliceInProgress}},
		}}, nil
	}}

	if err := MarkDone(context.Background(), client, domain.Slice{ID: "hb", Name: "Approve action"}); err != nil {
		t.Fatalf("MarkDone() = %v, want it to go through", err)
	}

	if len(client.updated) != 1 || client.updated[0].pageID != "hb" {
		t.Fatalf("wrote %+v, want exactly the slice", client.updated)
	}
	props := client.updated[0].properties
	if len(props) != 1 {
		t.Errorf("props = %v, want the Status column alone", props)
	}
	status := props[notion.PropStatus]
	if status.Status == nil || status.Status.Name != notion.SliceDone {
		t.Errorf("Status = %+v, want the status shape saying Done", status)
	}
}

// TestMarkDoneWritesASelectStatus covers the shape every project without a
// converted Status column is in: a plain select.
func TestMarkDoneWritesASelectStatus(t *testing.T) {
	client := &fakeClient{}
	if err := MarkDone(context.Background(), client, domain.Slice{ID: "hb", Name: "Approve action"}); err != nil {
		t.Fatalf("MarkDone() = %v, want it to go through", err)
	}
	status := client.updated[0].properties[notion.PropStatus]
	if status.Select == nil || status.Select.Name != notion.SliceDone {
		t.Errorf("Status = %+v, want the select shape saying Done", status)
	}
}

func TestMarkDoneReportsAFailedRead(t *testing.T) {
	client := &fakeClient{getPage: func(string) (*notion.Page, error) { return nil, errors.New("notion is down") }}

	err := MarkDone(context.Background(), client, domain.Slice{ID: "hb", Name: "Approve action"})

	if err == nil || !strings.Contains(err.Error(), `mark "Approve action" Done`) {
		t.Errorf("err = %v, want the read's failure named", err)
	}
	if len(client.updated) != 0 {
		t.Errorf("a failed read still wrote: %+v", client.updated)
	}
}

func TestMarkDoneReportsAFailedWrite(t *testing.T) {
	client := &fakeClient{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
		return nil, errors.New("notion is down")
	}}

	err := MarkDone(context.Background(), client, domain.Slice{ID: "hb", Name: "Approve action"})

	if err == nil || !strings.Contains(err.Error(), `mark "Approve action" Done`) {
		t.Errorf("err = %v, want the write's failure named", err)
	}
}

// TestSettleMergedMarksAMergedPRDone covers the merge nat was not running to
// witness: the pull request's own reading says merged, so the slice goes
// Done.
func TestSettleMergedMarksAMergedPRDone(t *testing.T) {
	client := &fakeClient{}
	viewer := &fakeViewer{pr: gh.PR{State: gh.PRStateMerged}}
	s := domain.Slice{ID: "hb", Name: "Approve action", PRURL: "https://github.test/pr/9"}

	done, err := SettleMerged(context.Background(), client, viewer, s, "/repo")

	if err != nil || !done {
		t.Fatalf("SettleMerged() = %v, %v, want Done written", done, err)
	}
	if want := []string{"/repo https://github.test/pr/9"}; len(viewer.viewed) != 1 || viewer.viewed[0] != want[0] {
		t.Errorf("viewed %v, want the slice's own pull request in its repo", viewer.viewed)
	}
	status := client.updated[0].properties[notion.PropStatus]
	if status.Select == nil || status.Select.Name != notion.SliceDone {
		t.Errorf("Status = %+v, want Done written", status)
	}
}

// TestSettleMergedLeavesAClosedPRAlone covers the other thing absence means:
// a pull request closed unmerged is work going round again, and the slice is
// left exactly as it is.
func TestSettleMergedLeavesAClosedPRAlone(t *testing.T) {
	client := &fakeClient{}
	viewer := &fakeViewer{pr: gh.PR{State: gh.PRStateClosed}}
	s := domain.Slice{ID: "hb", Name: "Approve action", PRURL: "https://github.test/pr/9"}

	done, err := SettleMerged(context.Background(), client, viewer, s, "/repo")

	if err != nil || done {
		t.Fatalf("SettleMerged() = %v, %v, want nothing written and no error", done, err)
	}
	if len(client.updated) != 0 {
		t.Errorf("a closed pull request still wrote: %+v", client.updated)
	}
}

// TestSettleMergedReportsAFailedReading covers gh refusing to answer: nothing
// is concluded from a reading that never happened.
func TestSettleMergedReportsAFailedReading(t *testing.T) {
	client := &fakeClient{}
	viewer := &fakeViewer{err: errors.New("gh is not signed in")}
	s := domain.Slice{ID: "hb", Name: "Approve action", PRURL: "https://github.test/pr/9"}

	done, err := SettleMerged(context.Background(), client, viewer, s, "/repo")

	if done || err == nil || !strings.Contains(err.Error(), "read what became of") {
		t.Errorf("SettleMerged() = %v, %v, want the reading's failure named", done, err)
	}
	if len(client.updated) != 0 {
		t.Errorf("a failed reading still wrote: %+v", client.updated)
	}
}

// TestSettleMergedReportsAFailedWrite covers the merge read and the status
// write refused: the failure is handed back so the caller retries on its next
// pass rather than counting the slice settled.
func TestSettleMergedReportsAFailedWrite(t *testing.T) {
	client := &fakeClient{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
		return nil, errors.New("notion is down")
	}}
	viewer := &fakeViewer{pr: gh.PR{State: gh.PRStateMerged}}
	s := domain.Slice{ID: "hb", Name: "Approve action", PRURL: "https://github.test/pr/9"}

	done, err := SettleMerged(context.Background(), client, viewer, s, "/repo")

	if done || err == nil || !strings.Contains(err.Error(), `mark "Approve action" Done`) {
		t.Errorf("SettleMerged() = %v, %v, want the write's failure named", done, err)
	}
}
