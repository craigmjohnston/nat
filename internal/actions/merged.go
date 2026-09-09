package actions

import (
	"context"
	"fmt"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// PRViewer is what settling a pull request needs of the GitHub CLI: the pull
// request's own reading, since a listing of what a repository has open cannot
// tell a merged pull request from one closed unmerged — both are simply
// absent. It is an interface for the reason [PRCreator] is.
type PRViewer interface {
	ViewPR(dir, ref string) (gh.PR, error)
}

// MarkDone moves a slice to Done: the one write that says its work is on
// main. Nothing but a merge reaches it — approving records the pull request
// and leaves the slice in progress, so the status on the page means the same
// thing everywhere the app reads it.
//
// The page is read first for the type of its Status column, which a project
// converted in the Notion UI may have changed under the app — the same read
// complete-slice makes for the same reason.
func MarkDone(ctx context.Context, client Client, s domain.Slice) error {
	page, err := client.GetPage(ctx, s.ID)
	if err != nil {
		return fmt.Errorf("mark %q Done: %w", s.Name, err)
	}
	properties := map[string]notion.PropertyValue{
		notion.PropStatus: notion.NewChoice(page.Properties[notion.PropStatus].Type, notion.SliceDone),
	}
	if _, err := client.UpdatePageProperties(ctx, s.ID, properties); err != nil {
		return fmt.Errorf("mark %q Done: %w", s.Name, err)
	}
	logging.Action("slice marked Done", "slice", s.ID, "name", s.Name)
	return nil
}

// SettleMerged asks GitHub what became of a pull request an open listing no
// longer names, and marks the slice Done where the answer is merged — how a
// merge made on GitHub itself, with nat not running to make it, still moves
// the slice. A pull request closed unmerged is the other thing absence means,
// and it is work going round again rather than work landed: the slice is left
// exactly as it is. Reports whether Done was written.
func SettleMerged(ctx context.Context, client Client, viewer PRViewer, s domain.Slice, dir string) (bool, error) {
	pr, err := viewer.ViewPR(dir, s.PRURL)
	if err != nil {
		return false, fmt.Errorf("read what became of %s: %w", s.PRURL, err)
	}
	if pr.State != gh.PRStateMerged {
		return false, nil
	}
	if err := MarkDone(ctx, client, s); err != nil {
		return false, err
	}
	return true, nil
}
