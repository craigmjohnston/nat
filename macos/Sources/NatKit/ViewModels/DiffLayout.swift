import Foundation

/// The fixed geometry the continuous diff is laid out on: the numbers the
/// canvas draws to and the layout counts heights with, kept in one value so
/// the two can never drift apart.
public struct DiffMetrics: Equatable, Sendable {
    /// A file's header band, its bottom rule included: the 32pt band every
    /// other heading in the window is, and the rule under it.
    public var headerHeight: CGFloat = 33
    /// A row of a file's body at least — one line of code, its padding
    /// included.
    public var rowMinHeight: CGFloat = 21
    /// The padding above and below a row's lines, together.
    public var rowPadding: CGFloat = 2
    /// One line of code, and one column of it.
    public var lineHeight: CGFloat = 19
    public var charWidth: CGFloat = 7.8
    /// The widest line number anywhere in the diff, in digits.
    public var numberDigits: Int = 1
    /// What the gutter allows a digit — a little over the face's own
    /// advance, as the gutter has always been drawn, so the code starts
    /// where it always started.
    public var digitWidth: CGFloat = 8.9
    /// The room kept after a line's last column, where the comment button
    /// sits over it.
    public var trailingInset: CGFloat = 8
    /// The "N files" line closing the diff.
    public var footerHeight: CGFloat = 44

    public init() {}

    /// The line-number column: the digits, and a little air — never
    /// narrower than a gap's expand button, which it also holds.
    public var numberColumnWidth: CGFloat {
        max(CGFloat(numberDigits) * digitWidth + 4, 16)
    }

    /// The gutter: the one number column — each line's number on the
    /// branch's side, a removed line's left blank and coloured, as GitHub
    /// and delta draw a unified diff — and its padding either side.
    public var gutterWidth: CGFloat {
        numberColumnWidth + 16
    }

    /// Where the code starts. No +/- column: a row's colour says what the
    /// change did to it.
    public var textX: CGFloat { gutterWidth + 10 }

    /// How many columns of code fit across `width`.
    public func wrapColumns(width: CGFloat) -> Int {
        max(1, Int(((width - textX - trailingInset) / charWidth).rounded(.down)))
    }
}

/// The continuous diff laid out once, exactly: every header, row, comment
/// and the closing line with the height it will be drawn at and where it
/// starts, worked out from the text before anything is drawn.
///
/// Nothing is estimated, which is the whole of why this exists. The old view
/// was a lazy stack that knew only the heights of the rows it had already
/// drawn and guessed at the rest, so the content's height — and the offset
/// of every row below the guess — moved as it scrolled, and a scroll to a
/// file aimed at where a guess put it. Here the height of a row is
/// arithmetic on its columns, the offset of anything is a prefix sum, and
/// finding what is at an offset is a binary search: a hundred thousand rows
/// lay out in a few milliseconds, at any width.
public struct DiffLayout: Sendable {
    /// One thing the diff draws, top to bottom.
    public enum Item: Hashable, Sendable {
        /// A file's header band.
        case header(file: Int)
        /// One row of a file's body.
        case row(file: Int, row: Int)
        /// What is drawn under a row: its comments, and the editor open on
        /// it — measured by the view that draws them, not here.
        case attachment(file: Int, row: Int)
        /// The "N files" line closing the diff.
        case footer

        public var file: Int? {
            switch self {
            case .header(let file), .row(let file, _), .attachment(let file, _): file
            case .footer: nil
            }
        }
    }

    public let items: [Item]
    /// Where each item starts; one longer than `items`, its last entry the
    /// height of the whole diff.
    public let tops: [CGFloat]
    /// Each file's header, as an index into `items`.
    public let headerIndex: [Int]
    /// Each file's rows, as indices into `items` — empty for a collapsed
    /// file.
    let rowIndex: [[Int]]
    /// The columns a line breaks at, or nil when lines do not wrap.
    public let wrapColumns: Int?
    /// How wide the diff is drawn: the width asked for, or — with lines not
    /// wrapping — wider, to the end of the longest line.
    public let contentWidth: CGFloat
    public let metrics: DiffMetrics

    public var totalHeight: CGFloat { tops.last ?? 0 }

    /// Lays out `files` at `width`. A collapsed file is its header alone; an
    /// attachment's height is `attachmentHeights` for its row's key
    /// (`Self.key(path:rowID:)`), and a row with none has no attachment.
    public init(
        files: [DiffFileModel],
        collapsed: Set<String> = [],
        attachmentHeights: [String: CGFloat] = [:],
        width: CGFloat,
        wrap: Bool = true,
        metrics: DiffMetrics = DiffMetrics()
    ) {
        let limit = wrap ? metrics.wrapColumns(width: width) : nil
        var items: [Item] = []
        var tops: [CGFloat] = []
        var headerIndex: [Int] = []
        var rowIndices: [[Int]] = []
        var y: CGFloat = 0
        var widestColumns = 0

        for (fileIndex, file) in files.enumerated() {
            headerIndex.append(items.count)
            items.append(.header(file: fileIndex))
            tops.append(y)
            y += metrics.headerHeight
            var fileRows: [Int] = []
            defer { rowIndices.append(fileRows) }
            guard !collapsed.contains(file.path) else { continue }
            fileRows.reserveCapacity(file.rows.count)
            for (rowIndex, row) in file.rows.enumerated() {
                fileRows.append(items.count)
                items.append(.row(file: fileIndex, row: rowIndex))
                tops.append(y)
                y += Self.rowHeight(row, limit: limit, metrics: metrics)
                widestColumns = max(widestColumns, row.columns)
                if !attachmentHeights.isEmpty, let height = attachmentHeights[Self.key(path: file.path, rowID: row.id)] {
                    items.append(.attachment(file: fileIndex, row: rowIndex))
                    tops.append(y)
                    y += height
                }
            }
        }
        items.append(.footer)
        tops.append(y)
        y += metrics.footerHeight
        tops.append(y)

        self.items = items
        self.tops = tops
        self.headerIndex = headerIndex
        self.rowIndex = rowIndices
        self.wrapColumns = limit
        self.metrics = metrics
        self.contentWidth = wrap
            ? width
            : max(width, metrics.textX + CGFloat(widestColumns) * metrics.charWidth + metrics.trailingInset)
    }

    /// The key an attachment is known by: its row, qualified by its file,
    /// since a row's id alone repeats between files.
    public static func key(path: String, rowID: String) -> String {
        "\(path)\u{0}\(rowID)"
    }

    /// A row's height at `limit` columns: a hunk break is cut short rather
    /// than wrapped — one line, or one per control a gap stacks.
    static func rowHeight(_ row: DiffRow, limit: Int?, metrics: DiffMetrics) -> CGFloat {
        let lines = row.kind == .hunkBreak
            ? row.gap?.controls.count ?? 1
            : DiffText.lineCount(row.text, columns: row.columns, isNarrow: row.isNarrow, limit: limit)
        return max(metrics.rowMinHeight, CGFloat(lines) * metrics.lineHeight + metrics.rowPadding)
    }

    public func top(_ index: Int) -> CGFloat { tops[index] }
    public func height(_ index: Int) -> CGFloat { tops[index + 1] - tops[index] }

    /// The item drawn at `y`, clamped to the first and last.
    public func index(at y: CGFloat) -> Int {
        var low = 0, high = items.count - 1
        while low < high {
            let mid = (low + high + 1) / 2
            if tops[mid] <= y { low = mid } else { high = mid - 1 }
        }
        return low
    }

    /// The items any part of which falls between `minY` and `maxY`.
    public func indices(from minY: CGFloat, to maxY: CGFloat) -> ClosedRange<Int> {
        let first = index(at: minY)
        var last = index(at: maxY)
        if last > first, tops[last] >= maxY { last -= 1 }
        return first...last
    }

    /// The index of an item, or nil where it is not laid out (a row of a
    /// collapsed file, a row with no attachment, a file past the last).
    public func index(of item: Item) -> Int? {
        switch item {
        case .footer:
            return items.count - 1
        case .header(let file):
            return headerIndex.indices.contains(file) ? headerIndex[file] : nil
        case .row(let file, let row):
            guard rowIndex.indices.contains(file), rowIndex[file].indices.contains(row) else { return nil }
            return rowIndex[file][row]
        case .attachment(let file, let row):
            guard let index = index(of: .row(file: file, row: row)), items[index + 1] == item else { return nil }
            return index + 1
        }
    }

    /// The file whose header is pinned at the top of a view scrolled to
    /// `y`, and how far up the next file's header has pushed it (zero or
    /// less) — nil above the first header or once the closing line is
    /// reached.
    public func pinnedHeader(at y: CGFloat) -> (file: Int, offset: CGFloat)? {
        guard !headerIndex.isEmpty, y >= 0 else { return nil }
        let item = items[index(at: y)]
        guard let file = item.file else { return nil }
        let nextTop = file + 1 < headerIndex.count ? tops[headerIndex[file + 1]] : tops[items.count - 1]
        return (file, min(0, nextTop - y - metrics.headerHeight))
    }
}
