import Foundation

/// How a line of code occupies the diff's monospaced grid: how many columns
/// it takes, and how it breaks into lines a given number of columns wide.
///
/// The layout and the drawing both come through here, which is the point: a
/// row's height is worked out from `columns` long before the row is drawn,
/// and `wrap` is what then draws it, so the two can never disagree about how
/// many lines a row is — the disagreement a text system's own line breaking
/// would leave room for, and what made the old diff jump as rows it had
/// guessed at were measured.
///
/// Breaking is by character, not by word: the grid is the code's own, and a
/// line broken at the column it runs out at keeps every later column where
/// the eye expects it.
public enum DiffText {
    /// The columns a tab advances to the next multiple of.
    public static let tabWidth = 4

    /// The columns one character takes: two for a wide one (CJK, Hangul,
    /// fullwidth forms, an emoji drawn as one), one for anything else.
    public static func width(of character: Character) -> Int {
        guard let scalar = character.unicodeScalars.first else { return 1 }
        if scalar.properties.isEmojiPresentation { return 2 }
        switch scalar.value {
        case 0x1100...0x115F, 0x2E80...0x303E, 0x3041...0x33FF, 0x3400...0x4DBF, 0x4E00...0x9FFF,
             0xA000...0xA4CF, 0xAC00...0xD7A3, 0xF900...0xFAFF, 0xFE30...0xFE4F, 0xFF00...0xFF60,
             0xFFE0...0xFFE6, 0x1F300...0x1F64F, 0x1F900...0x1F9FF, 0x20000...0x3FFFD:
            return 2
        default:
            return 1
        }
    }

    /// What the layout needs of a line to know its height at any width: its
    /// columns, and whether every character is one column wide — the case,
    /// nearly always, where the line count is plain arithmetic.
    public static func measure(_ text: String) -> (columns: Int, isNarrow: Bool) {
        var column = 0
        var isNarrow = true
        for character in text {
            if character == "\t" {
                column += tabWidth - column % tabWidth
            } else {
                let width = width(of: character)
                if width != 1 { isNarrow = false }
                column += width
            }
        }
        return (column, isNarrow)
    }

    /// The lines a line breaks into at `limit` columns apiece: one at the
    /// least, since an empty line still takes a row. A narrow line's count is
    /// arithmetic on its columns; a line with a wide character in it is
    /// broken for real, since a wide character that would straddle the limit
    /// moves whole to the next line and the arithmetic cannot see that.
    public static func lineCount(_ text: String, columns: Int, isNarrow: Bool, limit: Int?) -> Int {
        guard let limit, limit > 0, columns > limit else { return 1 }
        guard isNarrow else { return wrap([DiffSyntax.Run(text: text, kind: .text)], limit: limit).count }
        return (columns + limit - 1) / limit
    }

    /// Breaks a line's coloured runs into lines at most `limit` columns wide
    /// (no limit: one line), tabs expanded to spaces so what is drawn is what
    /// was counted. A wide character that would straddle the limit starts the
    /// next line instead. Always at least one line, possibly empty.
    public static func wrap(_ runs: [DiffSyntax.Run], limit: Int?) -> [[DiffSyntax.Run]] {
        var lines: [[DiffSyntax.Run]] = []
        var line: [DiffSyntax.Run] = []
        var pending = ""
        var pendingKind = TokenKind.text
        var column = 0
        var lineStart = 0

        func flushPending() {
            if !pending.isEmpty { line.append(DiffSyntax.Run(text: pending, kind: pendingKind)) }
            pending = ""
        }

        for run in runs {
            flushPending()
            pendingKind = run.kind
            for character in run.text {
                let isTab = character == "\t"
                let width = isTab ? tabWidth - column % tabWidth : DiffText.width(of: character)
                if let limit, limit > 0 {
                    // A tab is spaces: it breaks across the limit like them.
                    if isTab {
                        for _ in 0..<width {
                            if column - lineStart >= limit {
                                flushPending()
                                lines.append(line)
                                line = []
                                lineStart = column
                            }
                            pending.append(" ")
                            column += 1
                        }
                        continue
                    }
                    if column - lineStart + width > limit, column > lineStart {
                        flushPending()
                        lines.append(line)
                        line = []
                        lineStart = column
                    }
                }
                if isTab {
                    pending.append(String(repeating: " ", count: width))
                } else {
                    pending.append(character)
                }
                column += width
            }
        }
        flushPending()
        lines.append(line)
        return lines
    }
}
