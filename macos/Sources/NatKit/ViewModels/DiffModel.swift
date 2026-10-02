import Foundation
import SwiftUI

/// One row of a file's diff, ready to render.
///
/// A row's `id` is what a future inline comment would anchor to: for a row
/// that carries a real line number on either side it is built from the file's
/// path and those numbers alone, so it survives a re-read of the branch even
/// if the row's position in the list has moved — the same guarantee the Go
/// TUI's own line references give (`internal/tui/diffref.go`). A row with no
/// line number of its own (a hunk break, a line before a file's first hunk, a
/// described file's message) has nothing worth anchoring to, so its `id` is
/// only unique within this one read.
public struct DiffRow: Identifiable, Equatable, Sendable {
    /// What a row of a file's diff is, drawn one of five ways.
    public enum Kind: Equatable, Sendable {
        /// An unchanged line, present on both sides — or a line before a
        /// file's first hunk (a rename's own metadata, git's "no newline"
        /// note) that no hunk header numbers.
        case context
        /// A line the branch added: numbered on the new side alone.
        case added
        /// A line the branch removed: numbered on the old side alone.
        case removed
        /// The break left where a hunk ended and a later one began, standing
        /// in for the hunk header itself — the first hunk header of a file
        /// produces no row at all, since there is no gap above it to be the
        /// break after.
        case hunkBreak
        /// A line of a file git described rather than diffed (a binary file),
        /// kept as the one thing git wrote about it.
        case described
    }

    public let id: String
    public let kind: Kind
    /// The line's position in the file as it stood before the branch, or nil
    /// where the new side alone has it (an added line) or no hunk numbers it
    /// at all.
    public let oldNumber: Int?
    /// The line's position in the file as the branch leaves it, or nil where
    /// only the old side has it (a removed line) or no hunk numbers it.
    public let newNumber: Int?
    /// The line's leading `+`/`-`/` ` character, kept for the gutter's glyph.
    /// Nil where the line had none to strip: a hunk break (whose `text` is the
    /// hunk header itself), a described file's message, or a line before a
    /// file's first hunk.
    public let prefix: Character?
    /// The line's text with any leading `+`/`-`/` ` prefix removed. For a
    /// `hunkBreak` row this is the hunk header line verbatim, since that is
    /// what the break stands for.
    public let text: String
    /// The syntax runs `text` takes, one per stretch of a single colour — nil
    /// wherever the file has no matched language, or this row's own line took
    /// none (a hunk break, a described file's message). Threaded straight off
    /// `SliceDiffFile.tokens` at the same line index, so a re-lex never has to
    /// happen on this side: the Go side already lexed it once when the branch
    /// was read.
    public let tokens: [TokenRun]?
    /// The columns `text` takes on the diff's grid, tabs expanded, and
    /// whether every character in it is one column wide — measured once here
    /// so the layout can say how tall the row is at any width without
    /// reading the text again (`DiffText.lineCount`). Derived from `text`
    /// alone, so they take no part in `==`.
    public let columns: Int
    public let isNarrow: Bool
    /// For a `hunkBreak` row in a diff read for expanding: the lines it
    /// stands in for, which its controls reveal. Nil for every other row, and
    /// for a break with nothing behind it to reveal.
    public let gap: DiffGap?

    public static func == (lhs: DiffRow, rhs: DiffRow) -> Bool {
        lhs.id == rhs.id && lhs.kind == rhs.kind && lhs.oldNumber == rhs.oldNumber
            && lhs.newNumber == rhs.newNumber && lhs.prefix == rhs.prefix
            && lhs.text == rhs.text && lhs.tokens == rhs.tokens && lhs.gap == rhs.gap
    }

    public init(
        id: String,
        kind: Kind,
        oldNumber: Int?,
        newNumber: Int?,
        prefix: Character?,
        text: String,
        tokens: [TokenRun]? = nil,
        gap: DiffGap? = nil
    ) {
        self.id = id
        self.kind = kind
        self.oldNumber = oldNumber
        self.newNumber = newNumber
        self.prefix = prefix
        self.text = text
        self.tokens = tokens
        self.gap = gap
        (self.columns, self.isNarrow) = DiffText.measure(text)
    }
}

/// A run of a file's own lines the diff leaves out — above its first hunk,
/// between two, or after its last — and the controls that reveal it, as
/// GitHub's diff draws them: the next lines down from the change above, the
/// next lines up from the change below, or, where that would leave little
/// hidden, all of it at once.
///
/// Lines are numbered on the branch's side, which is the side `nat
/// slice-file` reads; every hidden line is context, on both sides, so its old
/// number is its new one plus `oldOffset`.
public struct DiffGap: Equatable, Sendable {
    /// What one control reveals.
    public enum Control: Equatable, Sendable {
        /// The lines just below the change above.
        case down
        /// The lines just above the change below.
        case up
        /// The whole gap.
        case all
    }

    /// How many lines one press reveals — GitHub's own step.
    public static let step = 20

    /// The first hidden line and the last — nil where the gap runs to the
    /// end of a file whose length is not yet known.
    public let first: Int
    public let last: Int?
    public let oldOffset: Int
    /// Whether the gap runs to the end of the file: the one after the last
    /// hunk.
    public let endsFile: Bool

    public init(first: Int, last: Int?, oldOffset: Int, endsFile: Bool = false) {
        self.first = first
        self.last = last
        self.oldOffset = oldOffset
        self.endsFile = endsFile
    }

    /// How many lines are hidden, where that is known.
    public var count: Int? { last.map { $0 - first + 1 } }

    /// The controls, top to bottom. A gap a single step would close takes
    /// one control for all of it; otherwise the gap above a file's first
    /// change only reveals upwards, towards it, the gap after its last only
    /// downwards, away from it, and a gap between two changes both ways.
    public var controls: [Control] {
        if let count, count <= Self.step { return [.all] }
        if first == 1 { return [.up] }
        if endsFile { return [.down] }
        return [.down, .up]
    }

    /// The lines a control reveals, as `nat slice-file` takes them: 1-based,
    /// inclusive, `to` nil for the rest of the file.
    public func range(for control: Control) -> (from: Int, to: Int?) {
        switch control {
        case .all:
            return (first, last)
        case .down:
            let to = first + Self.step - 1
            return (first, last.map { min($0, to) } ?? to)
        case .up:
            let last = last ?? first + Self.step - 1
            return (max(first, last - Self.step + 1), last)
        }
    }
}

/// One revealed line of a file: its text and, where the file has a
/// language, its syntax runs.
public struct RevealedLine: Equatable, Sendable {
    public let text: String
    public let tokens: [TokenRun]?

    public init(text: String, tokens: [TokenRun]? = nil) {
        self.text = text
        self.tokens = tokens
    }
}

/// One file's diff, parsed into rows ready to render.
public struct DiffFileModel: Identifiable, Equatable, Sendable {
    public let path: String
    public let oldPath: String
    public let adds: Int
    public let dels: Int
    public let described: Bool
    public let rows: [DiffRow]

    public var id: String { path }

    /// Whether the change moved the file — mirrors `SliceDiffFile.isRenamed`,
    /// carried onto the render-ready model so a view never has to reach back
    /// to the wire type to ask.
    public var isRenamed: Bool {
        !oldPath.isEmpty && oldPath != path
    }

    public init(
        path: String,
        oldPath: String,
        adds: Int,
        dels: Int,
        described: Bool,
        rows: [DiffRow]
    ) {
        self.path = path
        self.oldPath = oldPath
        self.adds = adds
        self.dels = dels
        self.described = described
        self.rows = rows
    }

    /// The file with `revealed` lines (by their number on the branch's
    /// side) put back in place of the gaps that hid them, as context rows
    /// numbered exactly as git would have numbered them — so a comment on one
    /// anchors as it would on any context line. What is still hidden stays a
    /// gap, split around what was revealed. `fileEnd` is the file's length
    /// where a read has said it: it bounds the gap after the last hunk, which
    /// goes once nothing is left in it.
    public func revealing(_ revealed: [Int: RevealedLine], fileEnd: Int?) -> DiffFileModel {
        guard rows.contains(where: { $0.gap != nil }) else { return self }
        var out: [DiffRow] = []
        out.reserveCapacity(rows.count + revealed.count)
        for row in rows {
            guard let gap = row.gap else {
                out.append(row)
                continue
            }
            out.append(contentsOf: revealRows(gap, header: row.text, revealed: revealed, fileEnd: fileEnd))
        }
        return DiffFileModel(path: path, oldPath: oldPath, adds: adds, dels: dels, described: described, rows: out)
    }

    /// One gap's rows once `revealed` is put into it: context rows for what
    /// is revealed, and a gap row for each run still hidden — the one next to
    /// the change below keeping the hunk header the gap was drawn with.
    private func revealRows(_ gap: DiffGap, header: String, revealed: [Int: RevealedLine], fileEnd: Int?) -> [DiffRow] {
        var rows: [DiffRow] = []
        func context(_ n: Int, _ line: RevealedLine) {
            let old = n + gap.oldOffset
            rows.append(DiffRow(
                id: anchoredID(path, old: old, new: n), kind: .context, oldNumber: old, newNumber: n,
                prefix: " ", text: line.text, tokens: line.tokens))
        }
        /// A run still hidden: the gap's last run keeps its header (next to
        /// the change below) or its place at the end of the file.
        func hidden(_ first: Int, _ last: Int?, isLast: Bool) {
            let run = DiffGap(first: first, last: last, oldOffset: gap.oldOffset, endsFile: gap.endsFile && isLast)
            rows.append(DiffRow(
                id: "\(path)#gap#\(first)", kind: .hunkBreak, oldNumber: nil, newNumber: nil, prefix: nil,
                text: isLast ? header : "", gap: run))
        }

        guard let end = gap.last ?? fileEnd else {
            // Running to an end not yet known: what has been revealed runs
            // down from the change above, and the rest is still hidden.
            var n = gap.first
            while let line = revealed[n] {
                context(n, line)
                n += 1
            }
            hidden(n, nil, isLast: true)
            return rows
        }
        var runStart: Int?
        for n in gap.first...max(gap.first, end) where n <= end {
            if let line = revealed[n] {
                if let start = runStart {
                    hidden(start, n - 1, isLast: false)
                    runStart = nil
                }
                context(n, line)
            } else if runStart == nil {
                runStart = n
            }
        }
        if let start = runStart {
            hidden(start, end, isLast: true)
        }
        return rows
    }
}

/// A whole diff, parsed into render-ready files.
public struct DiffModel: Equatable, Sendable {
    public let base: String
    public let branch: String
    public let files: [DiffFileModel]

    public init(base: String, branch: String, files: [DiffFileModel]) {
        self.base = base
        self.branch = branch
        self.files = files
    }

    /// The digit width of the largest line number anywhere in the diff, so a
    /// file's code starts at the same column in every box rather than
    /// shifting from one to the next — mirrors `internal/tui/diffbox.go`'s
    /// `numberWidth`, read once across the whole diff rather than per file.
    public var numberWidth: Int {
        var widest = 0
        for file in files {
            for row in file.rows {
                if let n = row.oldNumber { widest = max(widest, n) }
                if let n = row.newNumber { widest = max(widest, n) }
            }
        }
        return max(String(widest).count, 1)
    }
}

// MARK: - Building

/// Builds a render-ready `DiffModel` from the wire response of
/// `nat slice-diff --json`. `expandable` reads it for a viewer that can
/// reveal the lines between hunks (`nat slice-file`): every gap — above the
/// first hunk, between each pair, after the last — becomes a break carrying
/// its `DiffGap`.
public func buildDiffModel(from diff: SliceDiff, expandable: Bool = false) -> DiffModel {
    DiffModel(
        base: diff.base,
        branch: diff.branch,
        files: diff.files.map { buildDiffFileModel(from: $0, expandable: expandable) }
    )
}

/// Builds one file's render-ready rows from its wire lines, applying the same
/// noise rules as the Go TUI's `internal/tui/diffnoise.go`: the file header,
/// the blob line, and the two path lines produce no row at all (the box's own
/// header row and the gutter already say what they say); the first hunk
/// header of a file produces no row either, since there is no gap above it to
/// be the break after, and every later one becomes a `hunkBreak` row.
///
/// A described (binary) file keeps every line git wrote about it as a plain
/// `described` row — there is no hunk to number them by. A line before a
/// file's first hunk that is not one of the dropped headers (a rename's
/// "similarity index"/"rename from"/"rename to", or an unrecognised header) is
/// drawn like a context line, but numbered by neither side, since no hunk has
/// claimed it yet.
func buildDiffFileModel(from file: SliceDiffFile, expandable: Bool = false) -> DiffFileModel {
    DiffFileModel(
        path: file.path,
        oldPath: file.oldPath,
        adds: file.adds,
        dels: file.dels,
        described: file.described,
        rows: diffRows(for: file, expandable: expandable)
    )
}

private let gitFileMarker = "diff --git "
private let gitBlobMarker = "index "
private let gitOldMarker = "--- "
private let gitNewMarker = "+++ "

func diffRows(for file: SliceDiffFile, expandable: Bool = false) -> [DiffRow] {
    var rows: [DiffRow] = []
    var inHunk = false
    var nextOld = 0
    var nextNew = 0
    // A side a hunk starts at 0 is a side the file is not on: an added file
    // (old) or a deleted one (new) is its whole self in the one hunk, with
    // nothing around it to reveal.
    var wholeFile = false

    for (lineIndex, line) in file.lines.enumerated() {
        let tokens = tokenRuns(file, at: lineIndex)

        if let hunk = hunkStarts(line) {
            wholeFile = hunk.old == 0 || hunk.new == 0
            let gap: DiffGap? = if !expandable || wholeFile {
                nil
            } else if inHunk {
                hunk.new > nextNew ? DiffGap(first: nextNew, last: hunk.new - 1, oldOffset: nextOld - nextNew) : nil
            } else {
                hunk.new > 1 ? DiffGap(first: 1, last: hunk.new - 1, oldOffset: hunk.old - hunk.new) : nil
            }
            if let gap {
                rows.append(DiffRow(
                    id: "\(file.path)#gap#\(gap.first)", kind: .hunkBreak,
                    oldNumber: nil, newNumber: nil, prefix: nil, text: line, gap: gap))
            } else if inHunk {
                rows.append(DiffRow(
                    id: unanchoredID(file.path, rows.count),
                    kind: .hunkBreak,
                    oldNumber: nil,
                    newNumber: nil,
                    prefix: nil,
                    text: line
                ))
            }
            inHunk = true
            nextOld = hunk.old
            nextNew = hunk.new
            continue
        }

        if !inHunk && isNoiseHeader(line) {
            continue
        }

        if file.described {
            rows.append(DiffRow(
                id: unanchoredID(file.path, rows.count),
                kind: .described,
                oldNumber: nil,
                newNumber: nil,
                prefix: nil,
                text: line
            ))
            continue
        }

        if !inHunk {
            // A rename's own metadata, git's "no newline" note above any hunk,
            // or any other line no hunk header will ever number.
            rows.append(DiffRow(
                id: unanchoredID(file.path, rows.count),
                kind: .context,
                oldNumber: nil,
                newNumber: nil,
                prefix: nil,
                text: line,
                tokens: tokens
            ))
            continue
        }

        rows.append(contentRow(path: file.path, line: line, tokens: tokens, nextOld: &nextOld, nextNew: &nextNew))
    }

    // After the last hunk, the rest of the file — however much there is,
    // which only reading it will say. A file whose last line git noted has
    // no newline ends right there.
    if expandable, inHunk, !file.described, !wholeFile, file.lines.last?.hasPrefix("\\") != true {
        rows.append(DiffRow(
            id: "\(file.path)#gap#\(nextNew)", kind: .hunkBreak, oldNumber: nil, newNumber: nil, prefix: nil,
            text: "", gap: DiffGap(first: nextNew, last: nil, oldOffset: nextOld - nextNew, endsFile: true)))
    }
    return rows
}

/// The syntax runs the Go side lexed for one line of a file's own `lines`, by
/// index — nil where the file carries no `tokens` at all (no matched
/// language, or a described file), or where that particular index is out of
/// bounds of a `tokens` array that (against the wire's own contract) turned
/// out shorter than `lines`.
private func tokenRuns(_ file: SliceDiffFile, at lineIndex: Int) -> [TokenRun]? {
    guard let tokens = file.tokens, lineIndex >= 0, lineIndex < tokens.count else { return nil }
    return tokens[lineIndex]
}

/// One row of a line inside a hunk: added, removed, context, or (for git's own
/// "\ No newline at end of file" note) an unnumbered context row about the
/// line above rather than a line of either side.
private func contentRow(
    path: String, line: String, tokens: [TokenRun]?, nextOld: inout Int, nextNew: inout Int
) -> DiffRow {
    switch line.first {
    case "+":
        let n = nextNew
        nextNew += 1
        return DiffRow(
            id: anchoredID(path, old: nil, new: n),
            kind: .added,
            oldNumber: nil,
            newNumber: n,
            prefix: "+",
            text: String(line.dropFirst()),
            tokens: tokens
        )
    case "-":
        let n = nextOld
        nextOld += 1
        return DiffRow(
            id: anchoredID(path, old: n, new: nil),
            kind: .removed,
            oldNumber: n,
            newNumber: nil,
            prefix: "-",
            text: String(line.dropFirst()),
            tokens: tokens
        )
    case "\\":
        // git's "No newline at end of file": about the line above, not a line
        // of either side, and numbered by neither.
        return DiffRow(
            id: "\(path)#nonewline#\(nextOld)#\(nextNew)",
            kind: .context,
            oldNumber: nil,
            newNumber: nil,
            prefix: nil,
            text: line
        )
    default:
        // A context line (leading space), or the blank line git writes for an
        // empty one: the same line on both sides.
        let old = nextOld
        let new = nextNew
        nextOld += 1
        nextNew += 1
        return DiffRow(
            id: anchoredID(path, old: old, new: new),
            kind: .context,
            oldNumber: old,
            newNumber: new,
            prefix: " ",
            text: line.isEmpty ? "" : String(line.dropFirst()),
            tokens: tokens
        )
    }
}

/// A stable identity for a row that carries a real line number on at least one
/// side: the file it belongs to and the numbers themselves, so a comment
/// anchored to it survives a re-read of the branch.
private func anchoredID(_ path: String, old: Int?, new: Int?) -> String {
    "\(path)#\(old.map(String.init) ?? "_")#\(new.map(String.init) ?? "_")"
}

/// A unique-for-this-read identity for a row with no line number of its own —
/// a hunk break, a described file's message, a line before a file's first
/// hunk — built from its position among the file's other unnumbered rows
/// rather than anything that would survive a re-read.
private func unanchoredID(_ path: String, _ index: Int) -> String {
    "\(path)#row\(index)"
}

/// Whether a line is one of the four git writes only above a file's first
/// hunk: its own header, the blob line, and the two path lines. Recognised
/// only there, the way the Go TUI's `lineRoles` reads them — inside a hunk
/// every line carries its own +/-/space prefix, and a removed line reading
/// "--- x" is three characters the branch took out rather than a path.
private func isNoiseHeader(_ line: String) -> Bool {
    line.hasPrefix(gitFileMarker) || line.hasPrefix(gitBlobMarker) ||
        line.hasPrefix(gitOldMarker) || line.hasPrefix(gitNewMarker)
}

// MARK: - Hunk headers

/// The first line either side of a hunk covers, read off its header —
/// "@@ -12,7 +13,9 @@" — mirroring `internal/tui/diffref.go`'s `hunkStarts`.
/// A line that starts with "@@" but this cannot make sense of is not treated
/// as a hunk header at all.
private func hunkStarts(_ line: String) -> (old: Int, new: Int)? {
    guard line.hasPrefix("@@") else { return nil }
    let fields = line.split(separator: " ", omittingEmptySubsequences: true)
    guard fields.count >= 3 else { return nil }
    guard let old = sideStart(String(fields[1]), sign: "-"),
          let new = sideStart(String(fields[2]), sign: "+") else {
        return nil
    }
    return (old, new)
}

/// The first line one side of a hunk covers, read from its "-12,7" or "+12"
/// field. A field with no count ("+12") is git's shorthand for exactly one
/// line, which numbering only needs the start of either way.
private func sideStart(_ field: String, sign: Character) -> Int? {
    guard field.first == sign else { return nil }
    let rest = field.dropFirst()
    let head = rest.split(separator: ",", maxSplits: 1, omittingEmptySubsequences: false).first ?? Substring()
    guard let start = Int(head), start >= 0 else { return nil }
    return start
}
