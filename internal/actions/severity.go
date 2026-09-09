package actions

// Severity is how loudly a launch or an approve has something to say about
// where it landed: worked outright, worked but is worth a note, or failed.
// tui's own severity is this type under another name, so a toast reads the
// same whichever key produced it.
type Severity int

const (
	SevSuccess Severity = iota
	SevWarning
	SevError
)
