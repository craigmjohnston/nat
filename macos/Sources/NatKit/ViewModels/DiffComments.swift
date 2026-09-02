import Foundation

/// A review comment left on a run of one file's diff rows, held only in the
/// session — never written to Notion or GitHub, and sent to the agent as one
/// prompt with every other pending comment (mirrors the Go TUI's
/// `internal/tui/diffcomment.go`, whose `comment`/`Comment` this is the macOS
/// counterpart of).
///
/// `anchorRowIDs` are `DiffRow.id`s, in the order the rows are drawn, always
/// from the same file's rows and always contiguous in them — a comment is on
/// a run of whole lines of one file, never a stray selection across two.
/// They are what a re-read re-anchors onto (`DiffStore.reanchorComments`):
/// where every one of them can still be found, in the same run, the comment
/// survives; where they cannot, it is dropped, since guessing where it moved
/// to is worse than losing it.
public struct PendingComment: Identifiable, Equatable, Sendable {
    public let id: UUID
    public let path: String
    public let anchorRowIDs: [String]
    public var text: String

    public init(id: UUID = UUID(), path: String, anchorRowIDs: [String], text: String) {
        self.id = id
        self.path = path
        self.anchorRowIDs = anchorRowIDs
        self.text = text
    }
}

// MARK: - Prompt building

/// Builds the one turn every pending comment is delivered to the agent as:
/// what it is about, then each comment under the lines it was left on,
/// quoted as git wrote them — byte-for-byte the shape the Go TUI's
/// `commentPrompt`/`commentTitle` produce, so an agent handed a nat macOS
/// review reads the exact same prompt an agent handed a nat TUI review would.
///
/// `diff` is the current, already re-anchored diff: every comment's
/// `anchorRowIDs` is expected to still resolve against it (a comment that
/// cannot resolve — its file gone, or its lines no longer there — quotes
/// nothing and falls back to naming how many lines it was left on, exactly as
/// the Go side does for a comment its own read has failed to place).
public func commentsPrompt(_ comments: [PendingComment], diff: DiffModel) -> String {
    var rowsByPath: [String: [DiffRow]] = [:]
    for file in diff.files {
        rowsByPath[file.path] = file.rows
    }

    var out = "I have reviewed the diff of \(diff.branch) and left \(comments.count) " +
        "\(plural(comments.count, "comment", "comments")) on it. " +
        "Address every one of them, then push the branch again and tell me it is ready.\n"

    for comment in comments {
        let rows = anchoredRows(comment, rowsByPath: rowsByPath)
        let ref = lineRef(for: rows)
        let span = rows.isEmpty ? comment.anchorRowIDs.count : rows.count
        out += "\n## \(commentTitle(path: comment.path, ref: ref, span: span))\n\n"
        for row in rows {
            out += "> \(rawLine(row))\n"
        }
        out += "\n\(comment.text)\n"
    }
    return out
}

/// commentTitle names the lines a comment is about: where in the file they
/// sit, and how many of them there are where the reference cannot say —
/// mirrors `internal/tui/diffcomment.go`'s function of the same name exactly.
func commentTitle(path: String, ref: String, span: Int) -> String {
    if !ref.isEmpty {
        return "\(path), \(ref)"
    }
    return "\(path), \(span) \(plural(span, "line", "lines"))"
}

/// lineRef names where in the file a run of rows sits, in the words a
/// comment is addressed by: "line 42", "lines 42-45", and for a run that is
/// nothing but deletions the numbers the lines had before the branch removed
/// them — mirrors `internal/tui/diffref.go`'s `lineRef`. Unlike the Go side,
/// which reconstructs line numbers from a hunk header, `DiffRow` already
/// carries them, so this reads `newNumber`/`oldNumber` straight off the rows
/// in the run.
func lineRef(for rows: [DiffRow]) -> String {
    if let ref = rangeRef(rows.map(\.newNumber)) {
        return ref
    }
    if let ref = rangeRef(rows.map(\.oldNumber)) {
        return ref + " of the base"
    }
    return ""
}

/// rangeRef names the first and last numbered row of a run on one side, or
/// nothing at all where the side numbers none of them.
private func rangeRef(_ numbers: [Int?]) -> String? {
    let present = numbers.compactMap { $0 }
    guard let first = present.first, let last = present.last else { return nil }
    if first == last {
        return "line \(first)"
    }
    return "lines \(first)-\(last)"
}

/// anchoredRows resolves a comment's anchor IDs back into the rows of the
/// file they name, in the diff's own row order — the order the file's lines
/// are drawn in, not the order the IDs happen to be stored in.
private func anchoredRows(_ comment: PendingComment, rowsByPath: [String: [DiffRow]]) -> [DiffRow] {
    guard let rows = rowsByPath[comment.path] else { return [] }
    let anchored = Set(comment.anchorRowIDs)
    return rows.filter { anchored.contains($0.id) }
}

/// rawLine reconstructs the line exactly as git wrote it — prefix and all —
/// from a rendered row, since that is what the Go TUI quotes a comment's
/// lines as. A `hunkBreak` or `described` row's `text` is already verbatim
/// (a hunk header, or a binary file's one line about itself); every other row
/// had its leading `+`/`-`/space stripped when it was built, so it is put
/// back.
private func rawLine(_ row: DiffRow) -> String {
    switch row.kind {
    case .hunkBreak, .described:
        return row.text
    default:
        let prefix = row.prefix.map(String.init) ?? " "
        return prefix + row.text
    }
}

/// plural picks between two words by count, exactly as the Go TUI's helper
/// of the same name does. Public because the view layer's own footer and
/// notice text (`DiffTabView`) picks the same way.
public func plural(_ n: Int, _ one: String, _ many: String) -> String {
    n == 1 ? one : many
}

// MARK: - Avatar initials

/// The one or two letters a pending comment's avatar circle is drawn with:
/// the first letter of up to the first two words of a name, or "C" (for a
/// machine with nobody configured to be craig) where there is none.
public func initialsFor(_ name: String?) -> String {
    guard let name, !name.trimmingCharacters(in: .whitespaces).isEmpty else { return "C" }
    let letters = name.split(separator: " ").prefix(2).compactMap { $0.first }
    guard !letters.isEmpty else { return "C" }
    return String(letters).uppercased()
}
