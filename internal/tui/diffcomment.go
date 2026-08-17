package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// The messages the review comments come back as. Both are local — nothing here
// touches Notion — but they still go through the update loop, because that is
// what clears the busy the form's completion set.
type (
	// commentSavedMsg carries what the comment box was closed on: the lines it
	// was about, and the text left on them. Empty text is a comment taken back.
	commentSavedMsg struct {
		path  string
		start int
		span  int
		text  string
	}
	// commentsSentMsg reports the pending comments handed to an agent's pane,
	// or the failure that stopped them getting there.
	commentsSentMsg struct {
		session string
		count   int
		err     error
	}
)

// CommentForm is the box behind c: what the user has to say about the lines
// the diff's cursor is on. It writes nothing anywhere — the comment is held in
// the session until it is sent to the agent — so it has no client and no page
// ID, only the lines it is about.
type CommentForm struct {
	form    *huh.Form
	heading string

	path  string
	start int
	span  int
	text  string
}

// newCommentForm returns the box for commenting on a run of a file's lines,
// opened on whatever was said about them before: a comment is edited by
// commenting again, and emptied to take it back.
func newCommentForm(theme huh.Theme, path, ref string, start, span int, text string) *CommentForm {
	f := &CommentForm{heading: "Comment on " + path, path: path, start: start, span: span, text: text}
	f.form = newForm(theme, huh.NewGroup(
		huh.NewText().
			Title(commentTitle(path, ref, span)).
			Description("Sent to the agent with every other comment; empty takes it back.").
			Value(&f.text),
	))
	return f
}

// commentTitle names the lines the box is about: where in the file they sit,
// and how many of them there are where the reference cannot say.
func commentTitle(path, ref string, span int) string {
	if ref != "" {
		return fmt.Sprintf("%s, %s", path, ref)
	}
	return fmt.Sprintf("%s, %d %s", path, span, plural(span, "line", "lines"))
}

// Init starts the form.
func (f *CommentForm) Init() tea.Cmd { return f.form.Init() }

// Update feeds a message to the form.
func (f *CommentForm) Update(msg tea.Msg) tea.Cmd {
	form, cmd := f.form.Update(msg)
	f.form = form.(*huh.Form)
	return cmd
}

// State is how far the form has got.
func (f *CommentForm) State() huh.FormState { return f.form.State }

// View renders the form.
func (f *CommentForm) View() string { return f.form.View() }

// Heading is the title drawn over the form.
func (f *CommentForm) Heading() string { return f.heading }

// SetSize gives the form the room the window leaves it.
func (f *CommentForm) SetSize(width, height int) {
	f.form = f.form.WithWidth(width).WithHeight(height)
}

// busyNote is nothing: a comment is recorded in the session as the box closes,
// so there is no work in flight for the status line to announce.
func (f *CommentForm) busyNote() string { return "" }

// save hands the comment back to the diff screen. It goes through a message
// rather than straight onto the screen because a completed form marks the app
// busy, and a message landing is what marks it idle again.
func (f *CommentForm) save(*App) tea.Cmd {
	msg := commentSavedMsg{path: f.path, start: f.start, span: f.span, text: f.text}
	return func() tea.Msg { return msg }
}

// diffKey handles the diff screen's review keys, reporting whether the key was
// one of them. They live here rather than on the screen itself because two of
// the three reach past it — one opens a form, the other talks to tmux — the way
// the board's own write keys do.
func (a *App) diffKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, a.diff.keys.Select):
		a.diff.ToggleSelect()
		return nil, true
	case key.Matches(msg, a.diff.keys.Comment):
		return a.commentFlow(), true
	case key.Matches(msg, a.diff.keys.Send):
		return a.sendCommentsFlow(), true
	}
	return nil, false
}

// commentFlow opens the comment box on the lines the diff's cursor is on, or
// the range being marked from it. Only a diff on screen has lines to comment
// on: there is nothing to say about a read that is still in flight.
func (a *App) commentFlow() tea.Cmd {
	path, start, span, text, ok := a.diff.Selection()
	if !ok {
		return a.showToast("Move to a line of the diff to comment on it.", sevWarning)
	}
	return a.openForm(newCommentForm(a.styles.FormTheme, path, a.diff.SelectionRef(path, start, span),
		start, span, text))
}

// commentSaved records what the box was closed on. A comment that came back
// empty is one taken back, which the diff itself works out from the text.
func (a *App) commentSaved(msg commentSavedMsg) (tea.Model, tea.Cmd) {
	a.busy, a.note = false, ""
	a.diff.SetComment(msg.path, msg.start, msg.span, msg.text)
	if strings.TrimSpace(msg.text) == "" {
		return a, a.showToast("Comment removed.", sevWarning)
	}
	n := a.diff.Pending()
	return a, a.showToast(fmt.Sprintf("%d %s pending — s sends them to the agent.",
		n, plural(n, "comment", "comments")), sevSuccess)
}

// sendCommentsFlow hands every pending comment to the agent working the slice
// whose branch is on show, in one prompt: a review is one turn of the
// conversation, not one per line.
//
// It needs a live session to send to, which is the same thing the attach key
// needs — an agent that has exited is not there to be told anything, and the
// comments stay pending until there is one that is.
func (a *App) sendCommentsFlow() tea.Cmd {
	if a.launcher == nil {
		return nil
	}
	comments := a.diff.Comments()
	if len(comments) == 0 {
		return a.showToast("No comments to send — press c to leave one.", sevWarning)
	}
	name, branch, _ := a.diff.Target()
	session := a.live[a.diff.SliceID()]
	if session == "" {
		return a.showToast(fmt.Sprintf("No agent session is running for %q — the comments are still here.",
			name), sevWarning)
	}
	return sendComments(a.launcher, session, commentPrompt(branch, comments), len(comments))
}

// sendComments types the prompt at the agent's pane and submits it.
func sendComments(l AgentLauncher, session, prompt string, count int) tea.Cmd {
	return func() tea.Msg {
		if err := l.SendPrompt(session, prompt); err != nil {
			return commentsSentMsg{session: session, err: err}
		}
		return commentsSentMsg{session: session, count: count}
	}
}

// commentsSent reports the send. The comments are cleared only once they have
// actually reached the pane: a send that failed leaves them pending, because
// they are held nowhere else and retyping a review is not a thing to ask of
// anybody.
func (a *App) commentsSent(msg commentsSentMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return a, a.showToast(fmt.Sprintf("Could not send the comments to %s: %v", msg.session, msg.err), sevError)
	}
	a.diff.ClearComments()
	return a, a.showToast(fmt.Sprintf("Sent %d %s to %s.",
		msg.count, plural(msg.count, "comment", "comments"), msg.session), sevSuccess)
}

// commentPrompt is the one turn the pending comments are delivered as: what it
// is about, then each comment under the lines it was left on.
//
// The lines are quoted as git wrote them, prefix and all, beside the numbers
// they sit at in the file: the numbers are what the agent opens the file by, and
// the text is what it checks it has landed in the right place.
func commentPrompt(branch string, comments []Comment) string {
	var b strings.Builder
	fmt.Fprintf(&b, "I have reviewed the diff of %s and left %d %s on it. "+
		"Address every one of them, then push the branch again and tell me it is ready.\n",
		branch, len(comments), plural(len(comments), "comment", "comments"))
	for _, c := range comments {
		fmt.Fprintf(&b, "\n## %s\n\n", commentTitle(c.Path, c.Ref, len(c.Lines)))
		for _, line := range c.Lines {
			fmt.Fprintf(&b, "> %s\n", line)
		}
		fmt.Fprintf(&b, "\n%s\n", c.Text)
	}
	return b.String()
}
