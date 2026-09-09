package actions

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// todoPage is the slice ClaimSlice reads for the type of its Status column
// and for whether there is an Assignee to record: a Todo slice, held by
// nobody.
func todoPage(id string, assignee bool) *notion.Page {
	properties := map[string]notion.PropertyValue{notion.PropStatus: notion.NewSelect(notion.SliceTodo)}
	if assignee {
		properties[notion.PropAssignee] = notion.NewPeople()
	}
	return &notion.Page{ID: id, Properties: properties}
}

// TestClaimSlice covers the write: In progress on every project, and the
// Assignee only where the project tracks one and there is somebody
// configured to set it to.
func TestClaimSlice(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		assignee bool
		want     []string
	}{
		{"with an assignee column", "u1", true, []string{"u1"}},
		{"a project that tracks no assignee", "u1", false, nil},
		{"nobody configured to claim as", "", true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeClient{getPage: func(id string) (*notion.Page, error) { return todoPage(id, tt.assignee), nil }}

			if err := ClaimSlice(context.Background(), client, domain.Slice{ID: "s5", Name: "Info view"}, tt.userID); err != nil {
				t.Fatalf("ClaimSlice() = %v, want it to go through", err)
			}

			if len(client.updated) != 1 || client.updated[0].pageID != "s5" {
				t.Fatalf("writes = %+v, want exactly the slice claimed", client.updated)
			}
			props := client.updated[0].properties
			if name := props[notion.PropStatus].SelectName(); name != notion.SliceInProgress {
				t.Errorf("status = %q, want %q", name, notion.SliceInProgress)
			}
			if got := props[notion.PropAssignee].PeopleIDs(); !equalStrings(got, tt.want) {
				t.Errorf("assignee = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestClaimSliceReadFails covers the page's own read refusing: nothing is
// written, and the error names the slice.
func TestClaimSliceReadFails(t *testing.T) {
	client := &fakeClient{getPage: func(string) (*notion.Page, error) { return nil, errors.New("notion: 500") }}

	err := ClaimSlice(context.Background(), client, domain.Slice{ID: "s5", Name: "Info view"}, "u1")

	if err == nil || !strings.Contains(err.Error(), `claim "Info view": notion: 500`) {
		t.Errorf("err = %v, want the read's failure wrapped and the slice named", err)
	}
	if len(client.updated) != 0 {
		t.Errorf("wrote %v, want nothing after a failed read", client.updated)
	}
}

// TestClaimSliceWriteFails covers the write itself refusing.
func TestClaimSliceWriteFails(t *testing.T) {
	client := &fakeClient{
		getPage: func(id string) (*notion.Page, error) { return todoPage(id, true), nil },
		updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
			return nil, errors.New("notion: 500")
		},
	}

	err := ClaimSlice(context.Background(), client, domain.Slice{ID: "s5", Name: "Info view"}, "u1")

	if err == nil || !strings.Contains(err.Error(), `claim "Info view": notion: 500`) {
		t.Errorf("err = %v, want the write's failure wrapped and the slice named", err)
	}
}

// equalStrings is slices.Equal with nil and empty read as the same answer,
// which is all these tests need it for.
func equalStrings(got, want []string) bool {
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
