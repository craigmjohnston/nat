import XCTest
@testable import NatKit

final class DiffLayoutTests: XCTestCase {
    /// Round numbers, so offsets can be read off by eye: a 10pt column, a
    /// 20pt line, rows padded to 22, code starting at `textX`.
    private var metrics: DiffMetrics {
        var metrics = DiffMetrics()
        metrics.headerHeight = 30
        metrics.rowMinHeight = 22
        metrics.rowPadding = 2
        metrics.lineHeight = 20
        metrics.charWidth = 10
        metrics.numberDigits = 1
        metrics.digitWidth = 10
        metrics.trailingInset = 8
        metrics.footerHeight = 40
        return metrics
    }

    private func row(_ id: String, _ text: String, kind: DiffRow.Kind = .context) -> DiffRow {
        DiffRow(id: id, kind: kind, oldNumber: 1, newNumber: 1, prefix: " ", text: text)
    }

    private func file(_ path: String, _ rows: [DiffRow]) -> DiffFileModel {
        DiffFileModel(path: path, oldPath: path, adds: 0, dels: 0, described: false, rows: rows)
    }

    /// A width that fits exactly `columns` columns of code.
    private func width(columns: Int) -> CGFloat {
        metrics.textX + CGFloat(columns) * metrics.charWidth + metrics.trailingInset
    }

    private var files: [DiffFileModel] {
        [
            file("a.swift", [row("a1", "short"), row("a2", String(repeating: "x", count: 25))]),
            file("b.swift", [row("b1", "one"), row("@@", "@@ -1 +1 @@ " + String(repeating: "y", count: 40), kind: .hunkBreak)]),
        ]
    }

    // MARK: - Metrics

    func testMetricsGeometry() {
        let m = metrics
        XCTAssertEqual(m.numberColumnWidth, 14)
        XCTAssertEqual(m.gutterWidth, 14 * 2 + 6 + 16)
        XCTAssertEqual(m.glyphX, m.gutterWidth + 12)
        XCTAssertEqual(m.textX, m.glyphX + 13)
        XCTAssertEqual(m.wrapColumns(width: width(columns: 10)), 10)
        XCTAssertEqual(m.wrapColumns(width: width(columns: 10) + 9), 10)
        XCTAssertEqual(m.wrapColumns(width: 0), 1)
    }

    // MARK: - Laying out

    func testItemsAndOffsetsAreExact() {
        let layout = DiffLayout(files: files, width: width(columns: 10), metrics: metrics)
        XCTAssertEqual(layout.items, [
            .header(file: 0), .row(file: 0, row: 0), .row(file: 0, row: 1),
            .header(file: 1), .row(file: 1, row: 0), .row(file: 1, row: 1), .footer,
        ])
        // a2 is 25 columns at 10 apiece: three lines, 3 * 20 + 2.
        XCTAssertEqual(layout.tops, [0, 30, 52, 114, 144, 166, 188, 228])
        XCTAssertEqual(layout.totalHeight, 228)
        XCTAssertEqual(layout.headerIndex, [0, 3])
        XCTAssertEqual(layout.wrapColumns, 10)
        XCTAssertEqual(layout.contentWidth, width(columns: 10))
        XCTAssertEqual(layout.height(2), 62)
    }

    func testAHunkBreakIsOneLineHoweverLong() {
        let layout = DiffLayout(files: files, width: width(columns: 10), metrics: metrics)
        XCTAssertEqual(layout.height(5), 22)
    }

    func testUnwrappedRowsAreOneLineAndTheContentAsWideAsTheLongest() {
        let layout = DiffLayout(files: files, width: width(columns: 10), wrap: false, metrics: metrics)
        XCTAssertNil(layout.wrapColumns)
        XCTAssertEqual(layout.height(2), 22)
        // The hunk break's 52 columns are the widest.
        XCTAssertEqual(layout.contentWidth, width(columns: 52))
        XCTAssertEqual(DiffLayout(files: files, width: 2000, wrap: false, metrics: metrics).contentWidth, 2000)
    }

    func testACollapsedFileIsItsHeaderAlone() {
        let layout = DiffLayout(files: files, collapsed: ["a.swift"], width: width(columns: 10), metrics: metrics)
        XCTAssertEqual(layout.items.prefix(2), [.header(file: 0), .header(file: 1)])
        XCTAssertEqual(layout.tops[1], 30)
        XCTAssertNil(layout.index(of: .row(file: 0, row: 0)))
        XCTAssertEqual(layout.index(of: .row(file: 1, row: 0)), 2)
    }

    func testAnAttachmentFollowsItsRowAtItsHeight() {
        let key = DiffLayout.key(path: "a.swift", rowID: "a1")
        let layout = DiffLayout(files: files, attachmentHeights: [key: 50], width: width(columns: 10), metrics: metrics)
        XCTAssertEqual(layout.items[2], .attachment(file: 0, row: 0))
        XCTAssertEqual(layout.top(2), 52)
        XCTAssertEqual(layout.height(2), 50)
        XCTAssertEqual(layout.index(of: .attachment(file: 0, row: 0)), 2)
        XCTAssertNil(layout.index(of: .attachment(file: 0, row: 1)))
        XCTAssertEqual(layout.index(of: .row(file: 0, row: 1)), 3)
    }

    func testAnEmptyDiffIsTheFooterAlone() {
        let layout = DiffLayout(files: [], width: 100, metrics: metrics)
        XCTAssertEqual(layout.items, [.footer])
        XCTAssertNil(layout.pinnedHeader(at: 0))
    }

    // MARK: - Finding things

    func testIndexAtAnOffset() {
        let layout = DiffLayout(files: files, width: width(columns: 10), metrics: metrics)
        XCTAssertEqual(layout.index(at: -5), 0)
        XCTAssertEqual(layout.index(at: 0), 0)
        XCTAssertEqual(layout.index(at: 29.9), 0)
        XCTAssertEqual(layout.index(at: 30), 1)
        XCTAssertEqual(layout.index(at: 113), 2)
        XCTAssertEqual(layout.index(at: 10_000), 6)
    }

    func testIndicesBetweenTwoOffsets() {
        let layout = DiffLayout(files: files, width: width(columns: 10), metrics: metrics)
        XCTAssertEqual(layout.indices(from: 0, to: 30), 0...0)
        XCTAssertEqual(layout.indices(from: 10, to: 60), 0...2)
        XCTAssertEqual(layout.indices(from: 200, to: 900), 6...6)
    }

    func testIndexOfEachKindOfItem() {
        let layout = DiffLayout(files: files, width: width(columns: 10), metrics: metrics)
        XCTAssertEqual(layout.index(of: .header(file: 1)), 3)
        XCTAssertEqual(layout.index(of: .row(file: 1, row: 1)), 5)
        XCTAssertEqual(layout.index(of: .footer), 6)
        XCTAssertNil(layout.index(of: .header(file: 5)))
        XCTAssertNil(layout.index(of: .row(file: 5, row: 0)))
        XCTAssertNil(layout.index(of: .row(file: 0, row: 9)))
        XCTAssertEqual(DiffLayout.Item.footer.file, nil)
        XCTAssertEqual(DiffLayout.Item.attachment(file: 2, row: 0).file, 2)
    }

    func testThePinnedHeaderIsPushedUpByTheNext() {
        let layout = DiffLayout(files: files, width: width(columns: 10), metrics: metrics)
        XCTAssertEqual(layout.pinnedHeader(at: 0)?.file, 0)
        XCTAssertEqual(layout.pinnedHeader(at: 0)?.offset, 0)
        XCTAssertEqual(layout.pinnedHeader(at: 60)?.offset, 0)
        // b's header starts at 114: from 84 on it pushes a's up.
        XCTAssertEqual(layout.pinnedHeader(at: 100)?.file, 0)
        XCTAssertEqual(layout.pinnedHeader(at: 100)?.offset, -16)
        XCTAssertEqual(layout.pinnedHeader(at: 114)?.file, 1)
        // The closing line pushes the last one up the same way, and past it
        // nothing is pinned.
        XCTAssertEqual(layout.pinnedHeader(at: 170)?.offset, -12)
        XCTAssertNil(layout.pinnedHeader(at: 190))
        XCTAssertNil(layout.pinnedHeader(at: -1))
    }

    // MARK: - Scale

    /// A hundred thousand rows lay out in tens of milliseconds even in a
    /// debug build — what keeps a live resize of a huge diff smooth.
    func testAHundredThousandRowsLayOutQuickly() {
        let line = String(repeating: "let value = compute(input) ", count: 4)
        let big = (0..<200).map { f in
            file("f\(f).swift", (0..<500).map { row("\(f)#\($0)", line) })
        }
        measure {
            _ = DiffLayout(files: big, width: 900, metrics: metrics)
        }
    }
}
