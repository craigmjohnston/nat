import SwiftUI

/// The diff viewer's syntax highlighting: turning one line's `[TokenRun]`s
/// (byte-length runs over the line's content, exactly as
/// `internal/cli/difftokens.go` lexed it on the Go side) into an
/// `AttributedString` a `Text` can draw — mirrors the Go TUI's
/// `internal/tui/diffsyntax.go` in spirit, but there is no lexing to do here:
/// the runs already say what colour each stretch takes, so this is purely the
/// mapping from a run's kind to a colour, and the byte-safe slicing to apply
/// it.
///
/// Few colours, on purpose, matching the TUI's own choices: a keyword takes
/// the accent, a string takes the pending yellow rather than the green an
/// added line already means, a number takes the system teal, a name (a
/// function, a class, a tag — anything chroma calls one) takes the system
/// blue, a comment fades to the tertiary label, and anything else is the
/// row's own default foreground. The diff's added/removed wash stays under
/// the row wherever it is drawn; nothing here touches a background.
public enum DiffSyntax {
    /// The colour one run's kind is drawn in, `defaultColor` standing in for
    /// `.text` — the row's own foreground (label, or the dimmer label used
    /// for a described file's message), so a file with no highlighting at all
    /// still reads exactly as it always did.
    public static func color(for kind: TokenKind, defaultColor: Color) -> Color {
        switch kind {
        case .text: return defaultColor
        case .comment: return DesignTokens.labelTertiary
        case .keyword: return DesignTokens.accent
        case .string: return DesignTokens.systemYellow
        case .number: return DesignTokens.systemTeal
        case .name: return DesignTokens.systemBlue
        }
    }

    /// Builds one line's `AttributedString`, colouring each byte-length run
    /// in its kind's colour and leaving everything else in `defaultColor`.
    ///
    /// `tokens` is nil or empty for a line the Go side never lexed (no
    /// matched language, or a line outside a hunk) — such a line renders
    /// exactly as it always did, uncoloured. A run list whose lengths do not
    /// consume `text` exactly, byte for byte — too short, too long, or one
    /// that lands off a UTF-8 character boundary — is the wire disagreeing
    /// with what it is describing; rather than draw a mis-sliced line (or
    /// crash slicing invalid UTF-8), this falls all the way back to the plain,
    /// uncoloured line, the same as a file with no language at all.
    public static func attributedLine(_ text: String, tokens: [TokenRun]?, defaultColor: Color) -> AttributedString {
        guard let tokens, !tokens.isEmpty else {
            return plain(text, color: defaultColor)
        }

        let bytes = Array(text.utf8)
        var offset = 0
        var pieces: [AttributedString] = []
        pieces.reserveCapacity(tokens.count)

        for run in tokens {
            let end = offset + run.length
            guard run.length >= 0, offset >= 0, end <= bytes.count else {
                return plain(text, color: defaultColor)
            }
            guard let runText = String(bytes: bytes[offset..<end], encoding: .utf8) else {
                return plain(text, color: defaultColor)
            }
            var piece = AttributedString(runText)
            piece.foregroundColor = color(for: run.kind, defaultColor: defaultColor)
            pieces.append(piece)
            offset = end
        }

        // The runs must account for every byte of the line — a short count
        // (or one that overshoots, already refused above) is the same kind of
        // disagreement, so it falls back exactly the same way.
        guard offset == bytes.count else {
            return plain(text, color: defaultColor)
        }

        return pieces.reduce(into: AttributedString()) { $0 += $1 }
    }

    private static func plain(_ text: String, color: Color) -> AttributedString {
        var s = AttributedString(text)
        s.foregroundColor = color
        return s
    }
}
