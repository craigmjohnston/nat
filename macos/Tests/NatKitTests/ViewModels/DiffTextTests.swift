import XCTest
@testable import NatKit

final class DiffTextTests: XCTestCase {
    private typealias Run = DiffSyntax.Run

    private func texts(_ lines: [[Run]]) -> [String] {
        lines.map { $0.map(\.text).joined() }
    }

    // MARK: - Widths

    func testCharacterWidths() {
        XCTAssertEqual(DiffText.width(of: "a"), 1)
        XCTAssertEqual(DiffText.width(of: "日"), 2)
        XCTAssertEqual(DiffText.width(of: "한"), 2)
        XCTAssertEqual(DiffText.width(of: "Ａ"), 2) // fullwidth A
        XCTAssertEqual(DiffText.width(of: "🎉"), 2)
        XCTAssertEqual(DiffText.width(of: "é"), 1)
    }

    func testMeasureExpandsTabsToTheNextStop() {
        XCTAssertEqual(DiffText.measure("\tx").columns, 5)
        XCTAssertEqual(DiffText.measure("ab\tx").columns, 5)
        XCTAssertEqual(DiffText.measure("abcd\tx").columns, 9)
        XCTAssertTrue(DiffText.measure("a\tb").isNarrow)
    }

    func testMeasureFlagsAWideCharacter() {
        let measured = DiffText.measure("a日")
        XCTAssertEqual(measured.columns, 3)
        XCTAssertFalse(measured.isNarrow)
    }

    // MARK: - Line counts

    func testLineCountIsOneWithoutALimitOrWithinIt() {
        XCTAssertEqual(DiffText.lineCount("abcdef", columns: 6, isNarrow: true, limit: nil), 1)
        XCTAssertEqual(DiffText.lineCount("abcdef", columns: 6, isNarrow: true, limit: 6), 1)
        XCTAssertEqual(DiffText.lineCount("", columns: 0, isNarrow: true, limit: 4), 1)
        XCTAssertEqual(DiffText.lineCount("abc", columns: 3, isNarrow: true, limit: 0), 1)
    }

    func testLineCountOfANarrowLineIsArithmetic() {
        XCTAssertEqual(DiffText.lineCount("abcdefg", columns: 7, isNarrow: true, limit: 3), 3)
        XCTAssertEqual(DiffText.lineCount("abcdef", columns: 6, isNarrow: true, limit: 3), 2)
    }

    func testLineCountOfAWideLineIsWhatTheWrapMakesOfIt() {
        // "aa日日" is six columns: arithmetic says two lines of three, but
        // neither 日 fits beside "aa" or beside the other, so it is three.
        let text = "aa日日"
        let measured = DiffText.measure(text)
        XCTAssertEqual(DiffText.lineCount(text, columns: measured.columns, isNarrow: measured.isNarrow, limit: 3), 3)
        XCTAssertEqual(DiffText.wrap([Run(text: text, kind: .text)], limit: 3).count, 3)
    }

    /// Layout and drawing agree for every narrow line at every limit: the
    /// guarantee that keeps the diff from jumping.
    func testLineCountAgreesWithTheWrapForNarrowLines() {
        let lines = ["", "a", "\t\tdeep", "func f(x: Int) -> Int { x * 2 }", String(repeating: "ab\t", count: 9)]
        for text in lines {
            let measured = DiffText.measure(text)
            for limit in 1...20 {
                XCTAssertEqual(
                    DiffText.lineCount(text, columns: measured.columns, isNarrow: measured.isNarrow, limit: limit),
                    DiffText.wrap([Run(text: text, kind: .text)], limit: limit).count,
                    "\(text.debugDescription) at \(limit)")
            }
        }
    }

    // MARK: - Wrapping

    func testWrapWithoutALimitIsOneLineWithTabsExpanded() {
        let lines = DiffText.wrap([Run(text: "a\tb", kind: .keyword)], limit: nil)
        XCTAssertEqual(lines, [[Run(text: "a   b", kind: .keyword)]])
    }

    func testWrapBreaksAtTheLimitKeepingEachRunsKind() {
        let runs = [Run(text: "func", kind: .keyword), Run(text: " ", kind: .text), Run(text: "name", kind: .name)]
        let lines = DiffText.wrap(runs, limit: 3)
        XCTAssertEqual(texts(lines), ["fun", "c n", "ame"])
        XCTAssertEqual(lines[1], [Run(text: "c", kind: .keyword), Run(text: " ", kind: .text), Run(text: "n", kind: .name)])
    }

    func testWrapBreaksATabAcrossTheLimitLikeSpaces() {
        XCTAssertEqual(texts(DiffText.wrap([Run(text: "ab\tc", kind: .text)], limit: 3)), ["ab ", " c"])
    }

    func testWrapMovesAStraddlingWideCharacterWhole() {
        XCTAssertEqual(texts(DiffText.wrap([Run(text: "ab日", kind: .text)], limit: 3)), ["ab", "日"])
    }

    func testWrapOfAnEmptyLineIsOneEmptyLine() {
        XCTAssertEqual(DiffText.wrap([Run(text: "", kind: .text)], limit: 3), [[]])
        XCTAssertEqual(DiffText.wrap([], limit: nil), [[]])
    }
}
