package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/craigmjohnston/notion-agent-tracker/internal/domain"
	"github.com/craigmjohnston/notion-agent-tracker/internal/notion"
)

// The messages the add/edit flow comes back as.
type (
	// sliceBodyMsg carries the body of the slice about to be edited, already
	// converted to markdown, or the fetch that failed instead.
	sliceBodyMsg struct {
		slice    domain.Slice
		markdown string
		err      error
	}
	// sliceSavedMsg reports a finished write: note is what the status bar says
	// when it worked, err what stopped it.
	sliceSavedMsg struct {
		note string
		err  error
	}
)

// sliceFormMode says whether a form creates a slice or rewrites one.
type sliceFormMode int

const (
	sliceFormAdd sliceFormMode = iota
	sliceFormEdit
)

// SliceForm is the modal for writing a slice: its title, the brief that becomes
// the page body, and the optional repo override. Both modes ask for the same
// three things, so they share one form and differ only in where the answers go.
type SliceForm struct {
	mode    sliceFormMode
	form    *huh.Form
	heading string

	// milestoneID is the milestone an added slice is filed under; sliceID and
	// its name identify the page an edited one rewrites.
	milestoneID string
	sliceID     string

	// The values bound to the form's fields.
	title       string
	description string
	repo        string
}

// newAddSliceForm returns an empty form for a new slice under a milestone.
func newAddSliceForm(m domain.Milestone) *SliceForm {
	f := &SliceForm{mode: sliceFormAdd, milestoneID: m.ID, heading: "Add a slice to " + m.Name}
	f.form = f.build()
	return f
}

// newEditSliceForm returns a form filled in with a slice as it stands, with
// description being its page body as markdown.
func newEditSliceForm(s domain.Slice, description string) *SliceForm {
	f := &SliceForm{
		mode:        sliceFormEdit,
		sliceID:     s.ID,
		heading:     "Edit " + s.Name,
		title:       s.Name,
		description: description,
		repo:        s.Repo,
	}
	f.form = f.build()
	return f
}

// build assembles the form over the model's own fields, so a completed form
// leaves its answers where the caller can read them.
func (f *SliceForm) build() *huh.Form {
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Title").
			Value(&f.title).
			Validate(required("a title")),
		huh.NewText().
			Title("Brief").
			Description("Becomes the slice's page body — what the agent works from.").
			Value(&f.description),
		huh.NewInput().
			Title("Repo").
			Description("Working directory override; blank uses the project's own.").
			Value(&f.repo),
	))
}

// Init starts the form.
func (f *SliceForm) Init() tea.Cmd { return f.form.Init() }

// Update feeds a message to the form.
func (f *SliceForm) Update(msg tea.Msg) tea.Cmd {
	form, cmd := f.form.Update(msg)
	f.form = form.(*huh.Form)
	return cmd
}

// State is how far the form has got: the caller acts on completion and abort.
func (f *SliceForm) State() huh.FormState { return f.form.State }

// View renders the form.
func (f *SliceForm) View() string { return f.form.View() }

// loadSliceBody fetches the page body of the slice about to be edited, as the
// markdown the form pre-fills its brief with.
func loadSliceBody(client NotionAPI, s domain.Slice) tea.Cmd {
	return func() tea.Msg {
		blocks, err := client.GetBlockChildren(context.Background(), s.ID)
		if err != nil {
			return sliceBodyMsg{slice: s, err: fmt.Errorf("load slice body: %w", err)}
		}
		return sliceBodyMsg{slice: s, markdown: strings.TrimSpace(notion.Markdown(blocks))}
	}
}

// createSlice files a new slice under a milestone. It starts Todo and
// unassigned, which is what makes it something an agent can pick up.
func createSlice(client NotionAPI, slicesDSID, milestoneID, title, description, repo string) tea.Cmd {
	title, repo = strings.TrimSpace(title), strings.TrimSpace(repo)
	return func() tea.Msg {
		properties := map[string]notion.PropertyValue{
			notion.PropName:      notion.NewTitle(title),
			notion.PropStatus:    notion.NewSelect(notion.SliceTodo),
			notion.PropMilestone: notion.NewRelation(milestoneID),
			notion.PropRepo:      notion.NewRichText(repo),
		}
		_, err := client.CreatePage(context.Background(), notion.DataSourceParent(slicesDSID),
			properties, paragraphBlocks(description))
		if err != nil {
			return sliceSavedMsg{err: fmt.Errorf("create slice: %w", err)}
		}
		return sliceSavedMsg{note: fmt.Sprintf("Added %q.", title)}
	}
}

// editSlice rewrites a slice: its properties first, then its body. The
// milestone is left alone — moving a slice is its own flow — and so is the
// status, which only the workflow changes.
func editSlice(client NotionAPI, sliceID, title, description, repo string) tea.Cmd {
	title, repo = strings.TrimSpace(title), strings.TrimSpace(repo)
	return func() tea.Msg {
		ctx := context.Background()
		properties := map[string]notion.PropertyValue{
			notion.PropName: notion.NewTitle(title),
			notion.PropRepo: notion.NewRichText(repo),
		}
		if _, err := client.UpdatePageProperties(ctx, sliceID, properties); err != nil {
			return sliceSavedMsg{err: fmt.Errorf("update slice: %w", err)}
		}
		if err := replaceBody(ctx, client, sliceID, description); err != nil {
			return sliceSavedMsg{err: err}
		}
		return sliceSavedMsg{note: fmt.Sprintf("Updated %q.", title)}
	}
}

// replaceBody rewrites a page's content with the brief. Notion has no
// replace-content call, so the old blocks are trashed one by one and the new
// ones appended; only the top level is walked, because a trashed block takes
// its children with it.
func replaceBody(ctx context.Context, client NotionAPI, pageID, description string) error {
	blocks, err := client.GetBlockChildren(ctx, pageID)
	if err != nil {
		return fmt.Errorf("read slice body: %w", err)
	}
	for _, b := range blocks {
		if err := client.DeleteBlock(ctx, b.ID); err != nil {
			return fmt.Errorf("clear slice body: %w", err)
		}
	}
	children := paragraphBlocks(description)
	if len(children) == 0 {
		return nil
	}
	if _, err := client.AppendBlockChildren(ctx, pageID, children); err != nil {
		return fmt.Errorf("write slice body: %w", err)
	}
	return nil
}

// paragraphBlocks turns a brief into the blocks a slice page is written as: one
// paragraph per blank-line-separated chunk, blank chunks dropped. Bodies
// written here are paragraphs and nothing else — the form edits plain text, and
// a body that round-trips through it should come back as what was typed.
func paragraphBlocks(description string) []map[string]any {
	var blocks []map[string]any
	for _, chunk := range strings.Split(strings.ReplaceAll(description, "\r\n", "\n"), "\n\n") {
		text := strings.TrimSpace(chunk)
		if text == "" {
			continue
		}
		blocks = append(blocks, map[string]any{
			"object": "block",
			"type":   "paragraph",
			"paragraph": map[string]any{
				"rich_text": []map[string]any{{
					"type": "text",
					"text": map[string]any{"content": text},
				}},
			},
		})
	}
	return blocks
}
