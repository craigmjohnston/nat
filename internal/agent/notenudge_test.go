package agent

import "testing"

// The nudge says a note arrived, its provenance and text verbatim, and that it
// is context rather than work — and runs nothing.
func TestNoteArrivedPrompt(t *testing.T) {
	got := NoteArrivedPrompt(`From "Draw it" (M2)`, "The menu moved.\nMind the toolbar.")
	want := "A note was just added to this slice's brief.\n\n" +
		"From \"Draw it\" (M2)\n\n" +
		"The menu moved.\nMind the toolbar.\n\n" +
		"It is context for the work that remains, not new work to do: carry on."
	if got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
	if cmds := natCommands(got); len(cmds) != 0 {
		t.Errorf("the note prompt runs %q, want no command", cmds)
	}
}
