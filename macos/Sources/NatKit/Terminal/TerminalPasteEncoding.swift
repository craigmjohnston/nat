import Foundation

/// What Cmd+V sends to the underlying session.
///
/// Bracketed paste is the escape sequence that tells the program on the far
/// end "this whole block is one paste, not keystrokes" — the program turns
/// it on itself (`ESC [ ? 2004 h`), read here as a plain `Bool` the view
/// already has from the terminal's own `bracketedPasteMode`, so this stays a
/// function of a string and a flag rather than of a live terminal.
///
/// Claude Code's input box depends on it: unbracketed, every newline in a
/// multi-line paste arrives as its own carriage return, and Claude Code
/// reads a carriage return as "submit" — a pasted prompt would submit itself
/// line by line instead of landing as one block to edit.
public enum TerminalPasteEncoding {
    /// `ESC [ 200 ~` — the bracketed-paste start marker (mode 2004).
    public static let bracketedStart = "\u{1b}[200~"

    /// `ESC [ 201 ~` — the bracketed-paste end marker.
    public static let bracketedEnd = "\u{1b}[201~"

    /// The text to send for pasting `text`: wrapped in the bracketed-paste
    /// markers when `bracketed` is true, sent exactly as given otherwise.
    /// Newlines in `text` are never altered either way — bracketed paste
    /// exists precisely so the program on the far end can tell a pasted
    /// newline from an Enter key press, not so this rewrites one into the
    /// other.
    public static func send(_ text: String, bracketed: Bool) -> String {
        guard bracketed else { return text }
        return bracketedStart + text + bracketedEnd
    }
}
