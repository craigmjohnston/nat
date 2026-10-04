package agent

import "fmt"

// NoteArrivedPrompt is the one turn typed at a live agent whose slice has just
// had a note added to its brief: that the note is there, where it came from
// (provenance as `slice-note` composed it — already a slice's name or a
// person's, never an ID), the note itself verbatim, and that it is context for
// the work that remains rather than new work. The brief an agent is handed at
// launch is never re-read mid-session, so without this turn the note would
// reach only the slice's next session.
//
// It names no command at all: a note asks nothing to be done.
func NoteArrivedPrompt(provenance, note string) string {
	return fmt.Sprintf("A note was just added to this slice's brief.\n\n%s\n\n%s\n\n"+
		"It is context for the work that remains, not new work to do: carry on.", provenance, note)
}
