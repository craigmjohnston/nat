import XCTest
@testable import NatKit

final class TerminalGridSelectionTests: XCTestCase {
    /// Three rows of a fixture terminal, padded to a fixed width the way a
    /// real grid pads its unused columns with blanks.
    private static let fixtureRows = [
        "$ git status                                   ",
        "On branch slice/copy-and-paste-in-the-terminal ",
        "nothing to commit, working tree clean          ",
    ]

    // MARK: - One row

    func testASingleRowSelectionIsThatRowsSlice() {
        XCTAssertEqual(
            TerminalGridSelection.text(
                rows: ["$ git status                                   "],
                start: TerminalGridPosition(row: 0, col: 2),
                end: TerminalGridPosition(row: 0, col: 12)
            ),
            "git status"
        )
    }

    /// The end column trims to whatever selection width was dragged even
    /// when it lands inside the row's padding — the padding itself is not
    /// part of what was copied.
    func testASingleRowSelectionEndingInPaddingTrimsToTheContent() {
        XCTAssertEqual(
            TerminalGridSelection.text(
                rows: ["$ git status                                   "],
                start: TerminalGridPosition(row: 0, col: 0),
                end: TerminalGridPosition(row: 0, col: 40)
            ),
            "$ git status"
        )
    }

    // MARK: - Several rows

    func testAMultiRowSelectionIsOneLinePerRowJoinedByNewlines() {
        XCTAssertEqual(
            TerminalGridSelection.text(
                rows: Self.fixtureRows,
                start: TerminalGridPosition(row: 0, col: 2),
                end: TerminalGridPosition(row: 2, col: 7)
            ),
            "git status\nOn branch slice/copy-and-paste-in-the-terminal\nnothing"
        )
    }

    /// The first row is sliced from the start column to its end; the last is
    /// sliced from its start to the end column; every row between is kept
    /// whole but for its trailing padding.
    func testMiddleRowsAreKeptWholeButTrimmed() {
        XCTAssertEqual(
            TerminalGridSelection.text(
                rows: Self.fixtureRows,
                start: TerminalGridPosition(row: 0, col: 0),
                end: TerminalGridPosition(row: 2, col: 7)
            ),
            "$ git status\nOn branch slice/copy-and-paste-in-the-terminal\nnothing"
        )
    }

    // MARK: - Backwards drags

    /// A drag that ends above where it started reads exactly the same as one
    /// dragged the other way — the mouse can move in any direction.
    func testAnUpwardDragReadsTheSameAsADownwardOne() {
        let downward = TerminalGridSelection.text(
            rows: Self.fixtureRows,
            start: TerminalGridPosition(row: 0, col: 2),
            end: TerminalGridPosition(row: 2, col: 7)
        )
        let upward = TerminalGridSelection.text(
            rows: Self.fixtureRows,
            start: TerminalGridPosition(row: 2, col: 7),
            end: TerminalGridPosition(row: 0, col: 2)
        )
        XCTAssertEqual(downward, upward)
    }

    /// The same reversal within a single row.
    func testARightToLeftDragOnOneRowReadsInColumnOrder() {
        XCTAssertEqual(
            TerminalGridSelection.text(
                rows: ["$ git status                                   "],
                start: TerminalGridPosition(row: 0, col: 12),
                end: TerminalGridPosition(row: 0, col: 2)
            ),
            "git status"
        )
    }

    // MARK: - Edge cases

    func testAnEmptyGridReturnsAnEmptyString() {
        XCTAssertEqual(
            TerminalGridSelection.text(
                rows: [],
                start: TerminalGridPosition(row: 0, col: 0),
                end: TerminalGridPosition(row: 0, col: 5)
            ),
            ""
        )
    }

    /// A zero-width selection on one row — start and end at the same cell —
    /// selects nothing.
    func testAZeroWidthSelectionIsEmpty() {
        XCTAssertEqual(
            TerminalGridSelection.text(
                rows: ["$ git status"],
                start: TerminalGridPosition(row: 0, col: 4),
                end: TerminalGridPosition(row: 0, col: 4)
            ),
            ""
        )
    }

    /// A row that is nothing but padding trims to an empty line, which still
    /// takes its place between two newlines.
    func testABlankRowBetweenTwoOthersStaysAnEmptyLine() {
        XCTAssertEqual(
            TerminalGridSelection.text(
                rows: ["one", "                ", "two"],
                start: TerminalGridPosition(row: 0, col: 0),
                end: TerminalGridPosition(row: 2, col: 3)
            ),
            "one\n\ntwo"
        )
    }
}
