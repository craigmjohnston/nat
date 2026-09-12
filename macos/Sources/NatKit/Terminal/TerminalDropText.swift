import Foundation

/// What dropping — or pasting — files onto the agent terminal types into it.
///
/// A native terminal answers a drop by inserting the dropped files' paths,
/// escaped the way a shell needs them, followed by a space; that is the
/// gesture Claude Code's own drag-and-drop is documented against, and
/// reproducing it exactly is what makes the embedded pane behave like the
/// terminal a user dropped a screenshot into before.
///
/// The bytes are never anything but text: the pane is a pseudo-terminal, so
/// a file can only reach the agent as a path it goes and reads.
public enum TerminalDropText {
    /// The characters that go through unescaped: the letters, the digits, and
    /// the punctuation a path is actually made of. Everything else — a space
    /// above all, but also the quotes, the brackets and the glob characters —
    /// takes a backslash, which is what Terminal.app and iTerm2 both insert
    /// and what Claude Code reads back as one path rather than several words.
    private static let unescaped = Set("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/._-+,:@%=~")

    /// One path, escaped.
    public static func escape(_ path: String) -> String {
        var out = ""
        for character in path {
            if !unescaped.contains(character) {
                out.append("\\")
            }
            out.append(character)
        }
        return out
    }

    /// The text a drop of `paths` types: each path escaped, separated by
    /// spaces, and with a trailing space so whatever is typed next is a word
    /// of its own — and the empty string for a drop that named no file at
    /// all, which types nothing rather than a stray space.
    public static func text(forPaths paths: [String]) -> String {
        let named = paths.filter { !$0.isEmpty }
        guard !named.isEmpty else {
            return ""
        }
        return named.map(escape).joined(separator: " ") + " "
    }
}
