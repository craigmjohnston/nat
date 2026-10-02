import Foundation

/// A run of markdown as the app draws it: prose (handed to
/// `markdownAttributed`) or a table, which `Text` cannot lay out and so is
/// drawn as a grid of its own.
public enum MarkdownBlock: Equatable, Sendable {
    case text(String)
    case table(MarkdownTable)
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

/// Splits markdown into prose and tables. A table is a header row, then a
/// delimiter row (`|---|:---:|`) of the same width, then every following line
/// that still carries a pipe. Nothing inside a code fence is a table.
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
