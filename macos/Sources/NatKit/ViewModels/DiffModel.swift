import Foundation

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

    public init(
        id: String,
        kind: Kind,
        oldNumber: Int?,
        newNumber: Int?,
        prefix: Character?,
        text: String
    ) {
        self.id = id
        self.kind = kind
        self.oldNumber = oldNumber
        self.newNumber = newNumber
        self.prefix = prefix
        self.text = text
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
/// `nat slice-diff --json`.
public func buildDiffModel(from diff: SliceDiff) -> DiffModel {
    DiffModel(
        base: diff.base,
        branch: diff.branch,
        files: diff.files.map(buildDiffFileModel)
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
func buildDiffFileModel(from file: SliceDiffFile) -> DiffFileModel {
    DiffFileModel(
        path: file.path,
        oldPath: file.oldPath,
        adds: file.adds,
        dels: file.dels,
        described: file.described,
        rows: diffRows(for: file)
    )
}

private let gitFileMarker = "diff --git "
private let gitBlobMarker = "index "
private let gitOldMarker = "--- "
private let gitNewMarker = "+++ "

func diffRows(for file: SliceDiffFile) -> [DiffRow] {
    var rows: [DiffRow] = []
    var inHunk = false
    var nextOld = 0
    var nextNew = 0

    for line in file.lines {
        if let hunk = hunkStarts(line) {
            if inHunk {
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
                text: line
            ))
            continue
        }

        rows.append(contentRow(path: file.path, line: line, nextOld: &nextOld, nextNew: &nextNew))
    }

    return rows
}

/// One row of a line inside a hunk: added, removed, context, or (for git's own
/// "\ No newline at end of file" note) an unnumbered context row about the
/// line above rather than a line of either side.
private func contentRow(path: String, line: String, nextOld: inout Int, nextNew: inout Int) -> DiffRow {
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
            text: String(line.dropFirst())
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
            text: String(line.dropFirst())
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
            text: line.isEmpty ? "" : String(line.dropFirst())
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
