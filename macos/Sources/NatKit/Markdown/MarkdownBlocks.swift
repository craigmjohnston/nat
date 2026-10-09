import Foundation

/// A run of markdown as the app draws it: prose (handed to
/// `markdownAttributed`), a table, which `Text` cannot lay out and so is
/// drawn as a grid of its own, or a GitHub `<details>` fold.
public enum MarkdownBlock: Equatable, Sendable {
    case text(String)
    case table(MarkdownTable)
    case details(MarkdownDetails)
}

/// A `<details>` element as GitHub draws one: its `<summary>` line, always
/// shown, and the markdown it folds away — open from the start when the tag
/// said `open`.
public struct MarkdownDetails: Equatable, Sendable {
    public let summary: String
    public let body: String
    public let open: Bool

    public init(summary: String, body: String, open: Bool = false) {
        self.summary = summary
        self.body = body
        self.open = open
    }
}

/// A GitHub-flavoured table: its header cells, each column's alignment, and
/// its body rows, every row padded or cut to the header's width as GitHub
/// does.
public struct MarkdownTable: Equatable, Sendable {
    public enum Alignment: Equatable, Sendable { case leading, center, trailing }

    public let header: [String]
    public let alignments: [Alignment]
    public let rows: [[String]]

    public init(header: [String], alignments: [Alignment], rows: [[String]]) {
        self.header = header
        self.alignments = alignments
        self.rows = rows
    }
}

/// Splits markdown into prose, tables and `<details>` folds. A table is a
/// header row, then a delimiter row (`|---|:---:|`) of the same width, then
/// every following line that still carries a pipe. A fold runs from a line
/// opening `<details` to the `</details>` that balances it. Nothing inside a
/// code fence is either.
public func markdownBlocks(_ text: String) -> [MarkdownBlock] {
    let lines = text.split(separator: "\n", omittingEmptySubsequences: false).map(String.init)
    var blocks: [MarkdownBlock] = []
    var prose: [String] = []
    var inFence = false
    var index = 0

    func flushProse() {
        guard !prose.isEmpty else { return }
        blocks.append(.text(prose.joined(separator: "\n")))
        prose = []
    }

    while index < lines.count {
        let line = lines[index]
        let trimmed = line.trimmingCharacters(in: .whitespaces)
        if trimmed.hasPrefix("```") || trimmed.hasPrefix("~~~") { inFence.toggle() }
        if !inFence, trimmed.lowercased().hasPrefix("<details") {
            let (details, next) = detailsBlock(lines, from: index)
            flushProse()
            blocks.append(.details(details))
            index = next
            continue
        }
        if !inFence, index + 1 < lines.count, line.contains("|"),
           let alignments = delimiterRow(lines[index + 1]) {
            let header = tableCells(line)
            if header.count == alignments.count {
                var rows: [[String]] = []
                var next = index + 2
                while next < lines.count, lines[next].contains("|"),
                      !lines[next].trimmingCharacters(in: .whitespaces).isEmpty {
                    rows.append(fitted(tableCells(lines[next]), to: header.count))
                    next += 1
                }
                flushProse()
                blocks.append(.table(MarkdownTable(header: header, alignments: alignments, rows: rows)))
                index = next
                continue
            }
        }
        prose.append(line)
        index += 1
    }
    flushProse()
    return blocks
}

/// The fold opening at `start`: every line up to the one whose
/// `</details>` closes it (or the end, as a browser would), its `<summary>`
/// pulled out and the rest kept as its body. Returns the line after it.
private func detailsBlock(_ lines: [String], from start: Int) -> (MarkdownDetails, Int) {
    var depth = 0
    var end = lines.count - 1
    for index in start..<lines.count {
        let lower = lines[index].lowercased()
        depth += lower.components(separatedBy: "<details").count - 1
        depth -= lower.components(separatedBy: "</details>").count - 1
        if depth <= 0 {
            end = index
            break
        }
    }
    var inner = lines[start...end].joined(separator: "\n")
    // The opening tag, whatever attributes it carries.
    let openTag = inner.range(of: ">").map { inner[..<$0.upperBound] } ?? Substring(inner)
    let isOpen = openTag.lowercased().range(of: #"\bopen\b"#, options: .regularExpression) != nil
    inner = String(inner[openTag.endIndex...])
    if let close = inner.range(of: "</details>", options: [.caseInsensitive, .backwards]) {
        inner = String(inner[..<close.lowerBound])
    }
    var summary = "Details"
    if let match = inner.range(of: #"<summary[^>]*>[\s\S]*?</summary>"#, options: [.regularExpression, .caseInsensitive]) {
        let tag = String(inner[match])
        summary = tag
            .replacingOccurrences(of: #"<[^>]+>"#, with: "", options: .regularExpression)
            .trimmingCharacters(in: .whitespacesAndNewlines)
        inner.removeSubrange(match)
    }
    let body = inner.trimmingCharacters(in: .whitespacesAndNewlines)
    return (MarkdownDetails(summary: summary, body: body, open: isOpen), end + 1)
}

/// A row's cells: split on unescaped pipes, the outer pipes dropped, each
/// cell trimmed and an escaped pipe kept as a pipe.
func tableCells(_ line: String) -> [String] {
    var trimmed = Substring(line.trimmingCharacters(in: .whitespaces))
    if trimmed.hasPrefix("|") { trimmed = trimmed.dropFirst() }
    if trimmed.hasSuffix("|") && !trimmed.hasSuffix("\\|") { trimmed = trimmed.dropLast() }
    var cells: [String] = []
    var cell = ""
    var escaped = false
    for character in trimmed {
        if escaped {
            cell.append(character == "|" ? "|" : "\\\(character)")
            escaped = false
        } else if character == "\\" {
            escaped = true
        } else if character == "|" {
            cells.append(cell.trimmingCharacters(in: .whitespaces))
            cell = ""
        } else {
            cell.append(character)
        }
    }
    if escaped { cell.append("\\") }
    cells.append(cell.trimmingCharacters(in: .whitespaces))
    return cells
}

private func delimiterRow(_ line: String) -> [MarkdownTable.Alignment]? {
    guard line.contains("-") else { return nil }
    let cells = tableCells(line)
    var alignments: [MarkdownTable.Alignment] = []
    for cell in cells {
        let left = cell.hasPrefix(":"), right = cell.hasSuffix(":")
        let dashes = cell.drop { $0 == ":" }.reversed().drop { $0 == ":" }
        guard !dashes.isEmpty, dashes.allSatisfy({ $0 == "-" }) else { return nil }
        alignments.append(left && right ? .center : right ? .trailing : .leading)
    }
    return alignments
}

private func fitted(_ cells: [String], to count: Int) -> [String] {
    cells.count >= count ? Array(cells.prefix(count)) : cells + Array(repeating: "", count: count - cells.count)
}

/// One table column's laid-out width, and whether that cut it short.
public struct TableColumnWidth: Equatable, Sendable {
    public let width: Double
    public let abbreviated: Bool

    public init(width: Double, abbreviated: Bool) {
        self.width = width
        self.abbreviated = abbreviated
    }
}

/// The share of the available width past which an over-wide column is cut.
public let tableColumnCapShare = 0.33

/// Each column's width: its natural one, unless it takes more than its fair
/// share of `available` *and* more than a third of it — then it is cut to
/// whichever of those two is wider, and marked abbreviated. A column the
/// user has expanded keeps its natural width whatever it is; the table
/// scrolls sideways for whatever no longer fits.
public func tableColumnWidths(natural: [Double], available: Double, expanded: Set<Int>) -> [TableColumnWidth] {
    guard !natural.isEmpty, available > 0 else {
        return natural.map { TableColumnWidth(width: $0, abbreviated: false) }
    }
    let fair = available / Double(natural.count)
    let cap = max(fair, available * tableColumnCapShare)
    return natural.enumerated().map { index, width in
        guard !expanded.contains(index), width > cap else {
            return TableColumnWidth(width: width, abbreviated: false)
        }
        return TableColumnWidth(width: cap, abbreviated: true)
    }
}

/// The first `maxWords` words of a brief, the way it was written — its line
/// breaks and markdown kept — closed with an ellipsis. Nil when the brief is
/// no longer than that, so there is nothing more to show.
public func briefExcerpt(_ text: String, maxWords: Int) -> String? {
    var words = 0
    var inWord = false
    for index in text.indices {
        let isSpace = text[index].isWhitespace
        if !isSpace && !inWord {
            if words == maxWords {
                let kept = text[..<index].trimmingCharacters(in: .whitespacesAndNewlines)
                return kept + "\u{2026}"
            }
            words += 1
        }
        inWord = !isSpace
    }
    return nil
}

/// Words that fill about four lines of the navigator at its usual width.
public let briefExcerptWords = 30

/// The most words a brief's summary paragraph may run to — nat's
/// `domain.MaxBriefOpeningWords`, which refuses a planner's brief whose
/// first paragraph is longer.
public let briefSummaryWords = 60

/// A brief's summary: its first paragraph — the lines from the first
/// non-blank one up to the next blank one — where more follows it and it is
/// at most `briefSummaryWords` words. Nil otherwise: a brief of one
/// paragraph has no summary apart from itself, and a long first paragraph
/// was written before briefs opened on one.
public func briefSummary(_ text: String) -> String? {
    var summary: [Substring] = []
    var rest = false
    for line in text.split(separator: "\n", omittingEmptySubsequences: false) {
        let blank = line.allSatisfy(\.isWhitespace)
        if summary.isEmpty {
            if !blank { summary.append(line) }
        } else if blank {
            rest = true
        } else if rest {
            let joined = summary.joined(separator: "\n")
            let words = joined.split(whereSeparator: \.isWhitespace).count
            return words <= briefSummaryWords ? joined.trimmingCharacters(in: .whitespaces) : nil
        } else {
            summary.append(line)
        }
    }
    return nil
}

/// What the Brief card shows folded: the brief's summary paragraph where it
/// has one, else its first `briefExcerptWords` words. Nil where the whole
/// brief is no longer than that, so there is no Show more.
public func briefCardExcerpt(_ text: String) -> String? {
    briefSummary(text) ?? briefExcerpt(text, maxWords: briefExcerptWords)
}
