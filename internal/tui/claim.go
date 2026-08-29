package tui

import (
	"context"
	"fmt"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// claimSlice takes a slice for the configured user: Status to In progress, and
// the Assignee set where the project has that column and the config names
// somebody to set it to. It is what the launch flow does before it starts a
// session, so the row says an agent has the slice from the moment the key is
// pressed rather than once a fresh Claude Code has got round to running
// start-slice.
//
// The page is read first for the type of its Status column, which a project
// converted in the Notion UI may have changed under the app — the same read the
// release and the approve make, for the same reason. It is also what says
// whether the slice carries an Assignee at all: a project without that column
// has nowhere to record one, and ownership is decided on status alone.
//
// Nothing checks the claim stuck, unlike the claim internal/cli makes: the
// agent's own start-slice reads the page a moment later and re-opens the slice
// only where this same user still holds it, so a race is settled before any
// agent is handed a brief.
func claimSlice(ctx context.Context, client NotionAPI, s domain.Slice, userID string) error {
	page, err := client.GetPage(ctx, s.ID)
	if err != nil {
		return fmt.Errorf("claim %q: %w", s.Name, err)
	}
	properties := map[string]notion.PropertyValue{
		notion.PropStatus: notion.NewChoice(page.Properties[notion.PropStatus].Type, notion.SliceInProgress),
	}
	if _, tracked := page.Properties[notion.PropAssignee]; tracked && userID != "" {
		properties[notion.PropAssignee] = notion.NewPeople(userID)
	}
	if _, err := client.UpdatePageProperties(ctx, s.ID, properties); err != nil {
		return fmt.Errorf("claim %q: %w", s.Name, err)
	}
	logging.Action("slice claimed at launch", "slice", s.ID, "name", s.Name, "user", userID)
	return nil
}
