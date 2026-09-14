import Foundation

/// One point in the terminal's character grid — a row and a column, both
/// zero-based and both counted the way the terminal counts them.
///
/// The view resolves a click to one of these before anything here sees it,
/// so this type carries no opinion about scroll offset or pixel geometry —
/// only the two integers `TerminalGridSelection` needs.
public struct TerminalGridPosition: Equatable, Sendable {
    public var row: Int
    public var col: Int

    public init(row: Int, col: Int) {
        self.row = row
        self.col = col
    }
}

/// Turns a selected range of the terminal's character grid into the text
/// Cmd+C puts on the pasteboard.
///
/// A drag can move in any direction — up, left, backwards over itself — so
/// `start` and `end` need not already be in reading order; putting them in
/// order is this function's first job. What comes after is the format the
/// brief asks for: one line per grid row spanned, each line's trailing
/// whitespace trimmed, the lines joined with `\n`. Written as a pure
/// function of already-read grid rows rather than of a live `SwiftTerm`
/// view, so it is testable against fixtures without a pty behind it.
public enum TerminalGridSelection {
    /// The text selected by a range from `start` to `end`, given the grid
    /// rows it spans.
    ///
    /// `rows` holds exactly the rows from the lower of `start.row`/`end.row`
    /// through the higher, in that order, each a full-width string as the
    /// terminal drew it — reading it off the grid and slicing it to that
    /// exact span is the view's job, done once per drag rather than once per
    /// pixel this function is called from.
    public static func text(rows: [String], start: TerminalGridPosition, end: TerminalGridPosition) -> String {
        guard !rows.isEmpty else { return "" }

        var top = start
        var bottom = end
        if top.row > bottom.row || (top.row == bottom.row && top.col > bottom.col) {
            swap(&top, &bottom)
        }
        guard bottom.row - top.row + 1 == rows.count else { return "" }

        let lines = rows.enumerated().map { offset, row -> String in
            let characters = Array(row)
            let absoluteRow = top.row + offset
            let lowerBound = absoluteRow == top.row ? min(max(0, top.col), characters.count) : 0
            let upperBound = absoluteRow == bottom.row ? min(max(0, bottom.col), characters.count) : characters.count
            guard lowerBound < upperBound else { return "" }
            return trimTrailingWhitespace(String(characters[lowerBound..<upperBound]))
        }
        return lines.joined(separator: "\n")
    }

    /// Trailing spaces and tabs only — a terminal row pads its unused
    /// columns with blanks, and those are never part of what was copied.
    private static func trimTrailingWhitespace(_ line: String) -> String {
        var characters = Substring(line)
        while let last = characters.last, last == " " || last == "\t" {
            characters.removeLast()
        }
        return String(characters)
    }
}
