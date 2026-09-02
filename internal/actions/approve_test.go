package actions

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// prCall is one pull request a fakePRs was asked to open.
type prCall struct{ dir, branch, title, body string }

// fakePRs stands in for the GitHub CLI: it records what it was asked to open
// and answers with the URL — or the refusal — the test wants gh to have
// given.
type fakePRs struct {
	url  string
	err  error
	made []prCall
}

var _ PRCreator = (*fakePRs)(nil)

func (f *fakePRs) CreatePR(dir, branch, title, body string) (string, error) {
	f.made = append(f.made, prCall{dir, branch, title, body})
	return f.url, f.err
}

// block builds a paragraph-shaped block the way the API returns one.
func block(t *testing.T, blockType, text string) notion.Block {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"id":   "b1",
		"type": blockType,
		blockType: map[string]any{
			"rich_text": []map[string]any{{"type": "text", "plain_text": text}},
		},
	})
	if err != nil {
		t.Fatalf("marshal block: %v", err)
	}
	var b notion.Block
	if err := json.Unmarshal(encoded, &b); err != nil {
		t.Fatalf("unmarshal block: %v", err)
	}
	return b
}

// TestOpenPR covers the whole read-then-open: the description the agent left
// at hand-back is read off the slice page and given to gh as the pull
// request's title and body.
func TestOpenPR(t *testing.T) {
	client := &fakeClient{blocks: func(id string) ([]notion.Block, error) {
		if id != "hb" {
			t.Errorf("read the body of %q, want the slice being approved", id)
		}
		return []notion.Block{
			block(t, "heading_3", "Handed back"),
			block(t, "paragraph", "Did the work."),
			block(t, "heading_3", notion.PRDescriptionHeading),
			block(t, "paragraph", "Open the PR with the recorded description"),
			block(t, "paragraph", "What it does, and why."),
		}, nil
	}}
	prs := &fakePRs{url: "https://github.test/craig/nat/pull/9"}
	s := domain.Slice{ID: "hb", Name: "Approve action", Branch: "slice/approve"}

	url, err := OpenPR(context.Background(), client, prs, s, "/repo")

	if err != nil {
		t.Fatalf("OpenPR() = %v, want it to go through", err)
	}
	if url != prs.url {
		t.Errorf("url = %q, want %q", url, prs.url)
	}
	want := prCall{"/repo", "slice/approve", "Open the PR with the recorded description", "What it does, and why."}
	if len(prs.made) != 1 || prs.made[0] != want {
		t.Fatalf("gh was asked for %v, want %v", prs.made, want)
	}
}

// TestOpenPRWithoutARecordedDescription covers every hand-back written before
// there was a flag for one: nothing is read off the page as a title, so gh is
// given none and fills the pull request from the commits as it always did.
func TestOpenPRWithoutARecordedDescription(t *testing.T) {
	client := &fakeClient{blocks: func(string) ([]notion.Block, error) {
		return []notion.Block{block(t, "heading_3", "Handed back"), block(t, "paragraph", "Did the work.")}, nil
	}}
	prs := &fakePRs{}
	s := domain.Slice{ID: "hb", Branch: "slice/approve"}

	if _, err := OpenPR(context.Background(), client, prs, s, "/repo"); err != nil {
		t.Fatalf("OpenPR() = %v, want it to go through", err)
	}
	want := prCall{"/repo", "slice/approve", "", ""}
	if len(prs.made) != 1 || prs.made[0] != want {
		t.Fatalf("gh was asked for %v, want %v", prs.made, want)
	}
}

// TestOpenPRWithAnUnreadableDescription covers the page body failing to
// load: nothing is opened, because a pull request opened with the wrong
// title is not one this can open again.
func TestOpenPRWithAnUnreadableDescription(t *testing.T) {
	client := &fakeClient{blocks: func(string) ([]notion.Block, error) { return nil, errors.New("notion is down") }}
	prs := &fakePRs{}

	_, err := OpenPR(context.Background(), client, prs, domain.Slice{ID: "hb"}, "/repo")

	if err == nil || !strings.Contains(err.Error(), "read the pull request description") {
		t.Errorf("err = %v, want the read's failure named", err)
	}
	if len(prs.made) != 0 {
		t.Errorf("gh was asked for %v with the description unread", prs.made)
	}
}

// TestOpenPRReportsAGhFailure covers gh itself refusing: the failure is
// returned as-is, since it is gh's own reason and nothing here has anything
// to add to it.
func TestOpenPRReportsAGhFailure(t *testing.T) {
	client := &fakeClient{}
	prs := &fakePRs{err: errors.New(`a pull request for branch "slice/approve" already exists`)}

	_, err := OpenPR(context.Background(), client, prs, domain.Slice{ID: "hb", Branch: "slice/approve"}, "/repo")

	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("err = %v, want gh's own reason", err)
	}
}

// TestPRTitleBody pins the split gh is given: the first line titles the pull
// request and the rest is its body, a one-line description is a title alone,
// and no description at all is neither.
func TestPRTitleBody(t *testing.T) {
	for _, tt := range []struct{ name, in, title, body string }{
		{"title and body", "Title line\n\nThe body.\n", "Title line", "The body."},
		{"title alone", "  Title line  ", "Title line", ""},
		{"nothing at all", "  \n ", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			title, body := PRTitleBody(tt.in)
			if title != tt.title || body != tt.body {
				t.Errorf("PRTitleBody(%q) = %q, %q, want %q, %q", tt.in, title, body, tt.title, tt.body)
			}
		})
	}
}

// TestRecordPR covers the whole write: the pull request URL and Done go on
// together, in the shape the page's own Status column was read as.
func TestRecordPR(t *testing.T) {
	client := &fakeClient{getPage: func(id string) (*notion.Page, error) {
		return &notion.Page{ID: id, Properties: map[string]notion.PropertyValue{
			notion.PropStatus: {Type: notion.TypeStatus, Status: &notion.SelectOption{Name: notion.SliceInProgress}},
		}}, nil
	}}
	s := domain.Slice{ID: "hb", Name: "Approve action"}

	if err := RecordPR(context.Background(), client, s, "https://github.test/pr/9"); err != nil {
		t.Fatalf("RecordPR() = %v, want it to go through", err)
	}

	if len(client.updated) != 1 || client.updated[0].pageID != "hb" {
		t.Fatalf("wrote %+v, want exactly the slice", client.updated)
	}
	props := client.updated[0].properties
	if got := props[notion.PropPR].URL; got != "https://github.test/pr/9" {
		t.Errorf("PR = %q, want the recorded url", got)
	}
	status := props[notion.PropStatus]
	if status.Status == nil || status.Status.Name != notion.SliceDone {
		t.Errorf("Status = %+v, want the status shape saying Done", status)
	}
}

// TestRecordPRWritesASelectStatus covers the shape every project without a
// converted Status column is in: a plain select.
func TestRecordPRWritesASelectStatus(t *testing.T) {
	client := &fakeClient{}
	if err := RecordPR(context.Background(), client, domain.Slice{ID: "hb", Name: "Approve action"}, "https://github.test/pr/9"); err != nil {
		t.Fatalf("RecordPR() = %v, want it to go through", err)
	}
	status := client.updated[0].properties[notion.PropStatus]
	if status.Select == nil || status.Select.Name != notion.SliceDone {
		t.Errorf("Status = %+v, want the select shape saying Done", status)
	}
}

// TestRecordPRReportsAFailedRead and TestRecordPRReportsAFailedWrite cover
// the one half-done state the action has: a pull request opened and not
// recorded.
func TestRecordPRReportsAFailedRead(t *testing.T) {
	client := &fakeClient{getPage: func(string) (*notion.Page, error) { return nil, errors.New("notion is down") }}

	err := RecordPR(context.Background(), client, domain.Slice{ID: "hb", Name: "Approve action"}, "https://github.test/pr/9")

	if err == nil || !strings.Contains(err.Error(), `record the pull request for "Approve action"`) {
		t.Errorf("err = %v, want the read's failure named", err)
	}
}

func TestRecordPRReportsAFailedWrite(t *testing.T) {
	client := &fakeClient{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
		return nil, errors.New("notion is down")
	}}

	err := RecordPR(context.Background(), client, domain.Slice{ID: "hb", Name: "Approve action"}, "https://github.test/pr/9")

	if err == nil || !strings.Contains(err.Error(), `record the pull request for "Approve action"`) {
		t.Errorf("err = %v, want the write's failure named", err)
	}
}
