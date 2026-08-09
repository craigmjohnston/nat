package tui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
)

// DefaultGlamourStyle is the glamour stylesheet the info view renders with.
// Glamour has no terminal-background detection, so one style is chosen rather
// than guessed at; dark is the one that reads on the terminals this runs in.
const DefaultGlamourStyle = "dark"

// infoState is how far the project page has got: nothing asked for yet, a fetch
// in flight, the body rendered, or a fetch that failed.
type infoState int

const (
	infoIdle infoState = iota
	infoLoading
	infoReady
	infoFailed
)

// Info is the read-only project screen: the project's Notion page body — the
// conventions agents work to — rendered as markdown and scrolled in a viewport.
//
// It holds the markdown rather than the rendered output, because glamour wraps
// to a fixed width: every resize re-renders from the source.
type Info struct {
	styles Styles
	vp     viewport.Model

	// style is the glamour stylesheet; a name glamour does not know falls back
	// to unrendered markdown rather than an empty screen.
	style string

	markdown string
	state    infoState
	err      error

	width, height int
}

// NewInfo returns an empty info screen, waiting for a page body to be loaded
// into it.
func NewInfo(styles Styles) Info {
	vp := viewport.New()
	return Info{styles: styles, vp: vp, style: DefaultGlamourStyle}
}

// infoKeys are the scrolling bindings the info screen lists in the help.
func infoKeys() []key.Binding {
	k := viewport.DefaultKeyMap()
	return []key.Binding{k.Up, k.Down, k.HalfPageUp, k.HalfPageDown, k.PageUp, k.PageDown}
}

// SetSize gives the viewport the space it has to draw in and re-renders the
// body to the new width, so a resize rewraps rather than truncates.
func (i *Info) SetSize(width, height int) {
	i.width, i.height = width, height
	i.vp.SetWidth(max(width, 1))
	i.vp.SetHeight(max(height, 1))
	i.render()
}

// Start marks a fetch as in flight, so the screen shows progress and the caller
// does not start a second one.
func (i *Info) Start() { i.state, i.err = infoLoading, nil }

// SetMarkdown shows a freshly fetched page body, from the top.
func (i *Info) SetMarkdown(markdown string) {
	i.markdown, i.state, i.err = markdown, infoReady, nil
	i.render()
	i.vp.GotoTop()
}

// Fail reports a fetch that did not come back, leaving whatever was on show
// alone — a failed refresh should not blank a page the user was reading.
func (i *Info) Fail(err error) { i.state, i.err = infoFailed, err }

// Idle reports whether the page body has yet to be asked for. A refresh puts
// the screen back here, so the next visit re-fetches.
func (i Info) Idle() bool { return i.state == infoIdle }

// Busy reports whether a fetch is in flight, which is what keeps the root
// model's spinner turning.
func (i Info) Busy() bool { return i.state == infoLoading }

// NeedsLoad reports whether opening the screen should fetch the page: it has
// never been fetched, or the last attempt failed and is worth another go.
func (i Info) NeedsLoad() bool { return i.state == infoIdle || i.state == infoFailed }

// Reset drops the loaded body, so the screen fetches again next time it is
// opened. The viewport keeps the size it was given.
func (i *Info) Reset() {
	i.markdown, i.state, i.err = "", infoIdle, nil
	i.render()
}

// render rebuilds the viewport's content from the markdown at the current
// width. Markdown glamour cannot render — an unknown stylesheet — is shown as
// it came, which is still readable.
func (i *Info) render() {
	if i.markdown == "" {
		i.vp.SetContent("")
		return
	}
	i.vp.SetContent(renderMarkdown(i.markdown, i.style, i.width))
}

// renderMarkdown runs markdown through glamour, word-wrapped to width, falling
// back to the source when glamour cannot render it — unrendered markdown is
// still readable, an empty screen is not.
func renderMarkdown(markdown, style string, width int) string {
	out, err := glamourRender(markdown, style, width)
	if err != nil {
		return markdown
	}
	return out
}

// glamourRender is the glamour call itself, kept apart so that both ways it can
// fail — a stylesheet glamour does not know, and a document it cannot convert —
// reach renderMarkdown's fallback down one path.
func glamourRender(markdown, style string, width int) (string, error) {
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(max(width, 1)),
	)
	if err != nil {
		return "", err
	}
	return r.Render(markdown)
}

// Update handles the screen's keys: the viewport's own scrolling. Everything
// else — leaving the screen, quitting — belongs to the root model.
func (i *Info) Update(msg tea.Msg) tea.Cmd {
	vp, cmd := i.vp.Update(msg)
	i.vp = vp
	return cmd
}

// View renders the screen. spinner is the root model's current frame, drawn
// while the fetch is in flight so the app turns one spinner rather than two.
func (i Info) View(spinner string) string {
	header := i.styles.Title.Render("Info")
	switch i.state {
	case infoLoading:
		return header + "\n\n" + spinner + " Loading the project page…"
	case infoFailed:
		return header + "\n\n" + i.styles.Error.Render(i.err.Error())
	default:
		if i.markdown == "" {
			return header + "\n\n" + i.styles.Faint.Render("The project page is empty.")
		}
		return header + "\n\n" + i.vp.View()
	}
}
