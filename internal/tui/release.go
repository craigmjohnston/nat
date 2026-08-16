package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// The choices the release prompt offers, in the order they read. Releasing
// comes first, so a single enter takes it; the way out is beside it as well as
// on esc, the way the approve prompt has it.
var releaseChoices = []string{"release", "cancel"}

const (
	choiceRelease = iota
	choiceCancelRelease
)

// releaseSliceFlow anchors the release prompt to the slice the cursor is on.
// Only a slice in progress can be released: that is the one state a slice gets
// stuck in, and a Todo slice is already where releasing would put it.
//
// A slice with an agent still running on it is refused. Releasing one out from
// under a working session is how two sessions end up on one branch, and the
// board already knows which slices have an agent — so the refusal is a toast
// naming the way out, rather than a question the user could answer wrongly.
func (a *App) releaseSliceFlow() tea.Cmd {
	if !a.canWrite() {
		return nil
	}
	s, ok := a.board.SelectedSlice()
	if !ok {
		return a.showConfirm("Move to a slice to release it.", sevWarning)
	}
	if s.Status != domain.SliceClaimed {
		return a.showConfirm(fmt.Sprintf("%q is %s — only a slice in progress can be released.",
			s.Name, statusWord(s)), sevWarning)
	}
	if _, live := a.live[s.ID]; live {
		return a.showToast(fmt.Sprintf("An agent is still running on %q — stop it before releasing the slice.",
			s.Name), sevWarning)
	}
	return a.openPrompt(releaseChoices, func(choice int) tea.Cmd {
		return a.releaseChosen(s, choice)
	})
}

// releaseChosen is what answering the prompt does. Cancelling says nothing —
// nothing was in flight, and the row is as it was.
func (a *App) releaseChosen(s domain.Slice, choice int) tea.Cmd {
	if choice != choiceRelease {
		return nil
	}
	a.busy, a.note = true, releaseNote
	return releaseSlice(a.client, s, a.cfg.AssigneeUserName)
}

// releaseNote is what the status bar says while the release is in flight.
const releaseNote = "Releasing the slice…"

// releasedLine is the one line a release leaves on the page, so a slice that
// went round twice reads as having done so. It is worded exactly as the
// headless command's, since the two are the same act by different routes.
func releasedLine(assignee string) string {
	return fmt.Sprintf("Released back to Todo by %s: the session working it ended without finishing it.", assignee)
}

// releaseSlice hands the slice back to the plan: Todo, held by nobody, and a
// line on the page saying so. Nothing else is touched — the brief, the
// dependencies, the repo and any branch already pushed are what the next
// session picks the slice up with.
//
// The page is read first for the type of its Status column, which a project
// converted in the Notion UI may have changed under the app — the same read
// complete-slice and the approve key make, for the same reason. It is also
// what says whether the slice carries an Assignee at all: a project without
// that column has none to clear.
//
// The line goes on before the status does, which is the order release-slice
// writes in: a slice still in progress carrying the line can be released by
// running the key again, whereas one already back at Todo would refuse to have
// a line added to it.
func releaseSlice(client NotionAPI, s domain.Slice, assignee string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		fail := func(err error) tea.Msg {
			return sliceSavedMsg{err: fmt.Errorf("release %q: %w", s.Name, err)}
		}
		page, err := client.GetPage(ctx, s.ID)
		if err != nil {
			return fail(err)
		}
		if _, err := client.AppendBlockChildren(ctx, s.ID,
			[]map[string]any{paragraphBlock(releasedLine(assignee))}); err != nil {
			return fail(err)
		}
		properties := map[string]notion.PropertyValue{
			notion.PropStatus: notion.NewChoice(page.Properties[notion.PropStatus].Type, notion.SliceTodo),
		}
		if _, held := page.Properties[notion.PropAssignee]; held {
			properties[notion.PropAssignee] = notion.NewPeople()
		}
		if _, err := client.UpdatePageProperties(ctx, s.ID, properties); err != nil {
			return fail(err)
		}
		return sliceSavedMsg{note: fmt.Sprintf("Released %q back to Todo.", s.Name), sliceID: s.ID}
	}
}

// paragraphBlock is one paragraph of plain text, which is the whole of what a
// release writes onto a page.
func paragraphBlock(text string) map[string]any {
	return map[string]any{
		"object": "block",
		"type":   "paragraph",
		"paragraph": map[string]any{
			"rich_text": []map[string]any{{
				"type": "text",
				"text": map[string]any{"content": text},
			}},
		},
	}
}
