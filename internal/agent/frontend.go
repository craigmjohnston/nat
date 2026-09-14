package agent

import "fmt"

// Frontend says which surface the user launched a session from: the TUI
// board, or gnat, the macOS app. It is carried through the launch rather
// than detected, because nothing on the agent's side of a launch — a tmux
// session, a `nat` subprocess — can tell the two apart on its own.
//
// The zero value means unspecified: a hand-run `nat slice-launch` or
// `nat workshop-launch` with no --frontend flag claims nothing about where
// the user is, and the prompt it writes reads exactly as it did before this
// type existed.
type Frontend string

const (
	FrontendTUI  Frontend = "tui"
	FrontendGnat Frontend = "gnat"
)

// ParseFrontend validates a --frontend flag value against the enum. Empty is
// valid and means unspecified — the flag is optional, and its absence is a
// value in its own right, not an error.
func ParseFrontend(s string) (Frontend, error) {
	switch Frontend(s) {
	case "", FrontendTUI, FrontendGnat:
		return Frontend(s), nil
	default:
		return "", fmt.Errorf("--frontend must be %q or %q, given %q", FrontendTUI, FrontendGnat, s)
	}
}

// frontendNote is the sentence a prompt opens with when the launch says
// which surface the user is on — the one piece of user-facing guidance every
// template states outright rather than only adjusting around, so an agent
// reading only the first paragraph already knows. Empty for an unspecified
// launch, which says nothing here just as it said nothing before Frontend
// existed.
func frontendNote(f Frontend) string {
	switch f {
	case FrontendTUI:
		return "The user is driving this from the TUI board.\n\n"
	case FrontendGnat:
		return "The user is driving this from gnat, the macOS app.\n\n"
	default:
		return ""
	}
}
