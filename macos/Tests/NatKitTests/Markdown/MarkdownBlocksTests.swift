import XCTest
@testable import NatKit

final class MarkdownBlocksTests: XCTestCase {
    // MARK: - Tables

    func testProseWithNoTableIsOneBlock() {
        XCTAssertEqual(markdownBlocks("one\ntwo | three"), [.text("one\ntwo | three")])
        XCTAssertEqual(markdownBlocks(""), [.text("")])
    }

    func testATableSplitsTheProseAroundIt() {
        let text = """
        Before.
        | Name | Count | Note |
        |:-----|------:|:----:|
        | a | 1 | x |
        | b \\| c | 2 |
        After.
        """
        XCTAssertEqual(markdownBlocks(text), [
            .text("Before."),
            .table(MarkdownTable(
                header: ["Name", "Count", "Note"],
                alignments: [.leading, .trailing, .center],
                rows: [["a", "1", "x"], ["b | c", "2", ""]])),
            .text("After."),
        ])
    }

    func testOuterPipesAreOptionalAndExtraCellsAreDropped() {
        XCTAssertEqual(markdownBlocks("a | b\n--- | ---\n1 | 2 | 3"), [
            .table(MarkdownTable(header: ["a", "b"], alignments: [.leading, .leading], rows: [["1", "2"]])),
        ])
    }

    func testABlankLineEndsATable() {
        XCTAssertEqual(markdownBlocks("|a|\n|-|\n|1|\n\n|2|"), [
            .table(MarkdownTable(header: ["a"], alignments: [.leading], rows: [["1"]])),
            .text("\n|2|"),
        ])
    }

    func testNotATableWithoutAMatchingDelimiterRow() {
        XCTAssertEqual(markdownBlocks("a | b\n---"), [.text("a | b\n---")], "a delimiter one column short")
        XCTAssertEqual(markdownBlocks("a | b\nx | y"), [.text("a | b\nx | y")], "no dashes")
        XCTAssertEqual(markdownBlocks("a | b\n-x- | ---"), [.text("a | b\n-x- | ---")], "not only dashes")
        XCTAssertEqual(markdownBlocks("a | b\n: | ---"), [.text("a | b\n: | ---")], "colons alone")
    }

    func testNothingInsideAFenceIsATable() {
        let text = "```\n| a |\n|---|\n```"
        XCTAssertEqual(markdownBlocks(text), [.text(text)])
    }

    func testCellsKeepOtherEscapesAsWritten() {
        XCTAssertEqual(tableCells("| \\* | x\\ |"), ["\\*", "x\\"])
        XCTAssertEqual(tableCells("a \\|"), ["a |"])
        XCTAssertEqual(tableCells("| a | b\\"), ["a", "b\\"], "a trailing backslash escapes nothing")
    }

    // MARK: - Column widths

    func testColumnsThatFitKeepTheirWidths() {
        XCTAssertEqual(
            tableColumnWidths(natural: [50, 60], available: 300, expanded: []),
            [TableColumnWidth(width: 50, abbreviated: false), TableColumnWidth(width: 60, abbreviated: false)])
    }

    func testAColumnPastItsFairShareAndAThirdIsCut() {
        // Two columns: fair share is 150, a third 99 — the wider of the two caps.
        XCTAssertEqual(
            tableColumnWidths(natural: [40, 400], available: 300, expanded: []),
            [TableColumnWidth(width: 40, abbreviated: false), TableColumnWidth(width: 150, abbreviated: true)])
        // Five columns: fair share is 60, so a third (99) is the cap, and a
        // column past its share but under a third is left alone.
        let five = tableColumnWidths(natural: [90, 200, 10, 10, 10], available: 300, expanded: [])
        XCTAssertEqual(five[0], TableColumnWidth(width: 90, abbreviated: false))
        XCTAssertEqual(five[1], TableColumnWidth(width: 99, abbreviated: true))
    }

    func testAnExpandedColumnKeepsItsFullWidth() {
        XCTAssertEqual(
            tableColumnWidths(natural: [400], available: 300, expanded: [0]),
            [TableColumnWidth(width: 400, abbreviated: false)])
    }

    func testNoWidthToShareCutsNothing() {
        XCTAssertEqual(tableColumnWidths(natural: [400], available: 0, expanded: []),
                       [TableColumnWidth(width: 400, abbreviated: false)])
        XCTAssertEqual(tableColumnWidths(natural: [], available: 300, expanded: []), [])
    }

    // MARK: - Brief excerpt

    func testAShortBriefHasNoExcerpt() {
        XCTAssertNil(briefExcerpt("one two three", maxWords: 3))
        XCTAssertNil(briefExcerpt("  ", maxWords: 0))
    }

    func testALongBriefIsCutAfterItsWordsKeepingItsLines() {
        XCTAssertEqual(briefExcerpt("## Goal\n\nDo **this** now, then that.", maxWords: 4), "## Goal\n\nDo **this**\u{2026}")
        XCTAssertEqual(briefExcerpt("a b\n\nc", maxWords: 2), "a b\u{2026}")
    }
}
