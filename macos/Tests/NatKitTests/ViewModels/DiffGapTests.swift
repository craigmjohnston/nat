import XCTest
@testable import NatKit

/// The gaps a diff read for expanding leaves between its hunks: where they
/// are, which controls each offers, what each control reveals, and how the
/// revealed lines go back in.
final class DiffGapTests: XCTestCase {
    // MARK: - Controls and ranges

    func testASmallGapRevealsAllAtOnce() {
        let gap = DiffGap(first: 10, last: 29, oldOffset: 0)
        XCTAssertEqual(gap.count, 20)
        XCTAssertEqual(gap.controls, [.all])
        XCTAssertEqual(gap.range(for: .all).from, 10)
        XCTAssertEqual(gap.range(for: .all).to, 29)
    }

    func testTheGapAboveTheFirstChangeOnlyRevealsUpwards() {
        let gap = DiffGap(first: 1, last: 40, oldOffset: 0)
        XCTAssertEqual(gap.controls, [.up])
        XCTAssertEqual(gap.range(for: .up).from, 21)
        XCTAssertEqual(gap.range(for: .up).to, 40)
    }

    func testTheGapAfterTheLastChangeOnlyRevealsDownwards() {
        let unknown = DiffGap(first: 50, last: nil, oldOffset: 2, endsFile: true)
        XCTAssertNil(unknown.count)
        XCTAssertEqual(unknown.controls, [.down])
        XCTAssertEqual(unknown.range(for: .down).from, 50)
        XCTAssertEqual(unknown.range(for: .down).to, 69)
        XCTAssertNil(unknown.range(for: .all).to)

        let known = DiffGap(first: 50, last: 60, oldOffset: 2, endsFile: true)
        XCTAssertEqual(known.controls, [.all])
        XCTAssertEqual(known.range(for: .down).to, 60)
    }

    func testAGapBetweenTwoChangesRevealsBothWays() {
        let gap = DiffGap(first: 10, last: 100, oldOffset: -3)
        XCTAssertEqual(gap.controls, [.down, .up])
        XCTAssertEqual(gap.range(for: .down).from, 10)
        XCTAssertEqual(gap.range(for: .down).to, 29)
        XCTAssertEqual(gap.range(for: .up).from, 81)
        XCTAssertEqual(gap.range(for: .up).to, 100)
        // An unbounded gap reads its up step from where it starts.
        XCTAssertEqual(DiffGap(first: 5, last: nil, oldOffset: 0).range(for: .up).from, 5)
    }

    // MARK: - Reading the gaps off a diff

    private func file(_ lines: [String], path: String = "a.swift") -> SliceDiffFile {
        SliceDiffFile(path: path, oldPath: path, adds: 1, dels: 1, described: false, lines: lines)
    }

    private let twoHunks = [
        "diff --git a/a.swift b/a.swift", "--- a/a.swift", "+++ b/a.swift",
        "@@ -5,2 +5,3 @@ func a()", " five", "+new", " six",
        "@@ -40,1 +41,1 @@ func b()", "-old", "+fresh",
    ]

    func testAnExpandableReadPutsAGapAroundEveryHunk() {
        let rows = diffRows(for: file(twoHunks), expandable: true)
        let gaps = rows.compactMap(\.gap)
        XCTAssertEqual(gaps, [
            DiffGap(first: 1, last: 4, oldOffset: 0),
            DiffGap(first: 8, last: 40, oldOffset: -1),
            DiffGap(first: 42, last: nil, oldOffset: -1, endsFile: true),
        ])
        XCTAssertEqual(rows.first?.text, "@@ -5,2 +5,3 @@ func a()")
        XCTAssertEqual(rows.first?.id, "a.swift#gap#1")
        XCTAssertEqual(rows.last?.text, "")
    }

    func testAPlainReadKeepsItsBreaksWithoutGaps() {
        let rows = diffRows(for: file(twoHunks))
        XCTAssertTrue(rows.allSatisfy { $0.gap == nil })
        XCTAssertEqual(rows.filter { $0.kind == .hunkBreak }.count, 1)
    }

    func testAnAddedOrDeletedFileHasNothingAroundIt() {
        let added = file(["diff --git a/n b/n", "@@ -0,0 +1,2 @@", "+one", "+two"])
        let deleted = file(["diff --git a/n b/n", "@@ -1,2 +0,0 @@", "-one", "-two"])
        XCTAssertTrue(diffRows(for: added, expandable: true).allSatisfy { $0.gap == nil })
        XCTAssertTrue(diffRows(for: deleted, expandable: true).allSatisfy { $0.gap == nil })
    }

    func testAFileThatEndsWithoutANewlineHasNoGapAfterIt() {
        let lines = ["diff --git a/a b/a", "@@ -1,1 +1,1 @@", "-x", "+y", "\\ No newline at end of file"]
        XCTAssertTrue(diffRows(for: file(lines), expandable: true).allSatisfy { $0.gap == nil })
    }

    func testAHunkStartingAtTheTopHasNoGapAboveIt() {
        let lines = ["diff --git a/a b/a", "@@ -1,1 +1,1 @@", "-x", "+y"]
        let gaps = diffRows(for: file(lines), expandable: true).compactMap(\.gap)
        XCTAssertEqual(gaps, [DiffGap(first: 2, last: nil, oldOffset: 0, endsFile: true)])
    }

    // MARK: - Revealing

    private var model: DiffFileModel { buildDiffFileModel(from: file(twoHunks), expandable: true) }

    private func lines(_ range: ClosedRange<Int>) -> [Int: RevealedLine] {
        Dictionary(uniqueKeysWithValues: range.map { ($0, RevealedLine(text: "line \($0)", tokens: [TokenRun(kind: .text, length: 4)])) })
    }

    func testRevealedLinesComeBackAsContextNumberedAsGitWouldHave() {
        let revealed = model.revealing(lines(8...12), fileEnd: nil)
        let row = revealed.rows.first { $0.newNumber == 8 }
        XCTAssertEqual(row?.kind, .context)
        XCTAssertEqual(row?.oldNumber, 7)
        XCTAssertEqual(row?.id, "a.swift#7#8")
        XCTAssertEqual(row?.text, "line 8")
        XCTAssertEqual(row?.tokens, [TokenRun(kind: .text, length: 4)])
        // What is left hidden is still a gap, keeping the hunk header.
        let rest = revealed.rows.first { $0.gap?.first == 13 }
        XCTAssertEqual(rest?.gap, DiffGap(first: 13, last: 40, oldOffset: -1))
        XCTAssertEqual(rest?.text, "@@ -40,1 +41,1 @@ func b()")
    }

    func testRevealingBothEndsLeavesTheMiddleHidden() {
        let revealed = model.revealing(lines(8...10).merging(lines(38...40)) { $1 }, fileEnd: nil)
        let gaps = revealed.rows.compactMap(\.gap)
        XCTAssertTrue(gaps.contains(DiffGap(first: 11, last: 37, oldOffset: -1)))
        XCTAssertEqual(revealed.rows.first { $0.gap?.first == 11 }?.text, "")
    }

    func testRevealingAWholeGapRemovesIt() {
        let revealed = model.revealing(lines(1...4), fileEnd: nil)
        XCTAssertFalse(revealed.rows.contains { $0.gap?.first == 1 })
        XCTAssertEqual(revealed.rows.prefix(4).map(\.newNumber), [1, 2, 3, 4])
    }

    func testTheGapAfterTheLastHunkFollowsTheFilesLength() {
        // Unknown length: what is revealed runs down from the change.
        let partial = model.revealing(lines(42...45), fileEnd: nil)
        XCTAssertEqual(partial.rows.last?.gap, DiffGap(first: 46, last: nil, oldOffset: -1, endsFile: true))
        // Known: the gap is bounded by it, and goes once it is all revealed.
        let bounded = model.revealing(lines(42...45), fileEnd: 50)
        XCTAssertEqual(bounded.rows.last?.gap, DiffGap(first: 46, last: 50, oldOffset: -1, endsFile: true))
        let done = model.revealing(lines(42...50), fileEnd: 50)
        XCTAssertNil(done.rows.last?.gap)
        // A file shorter than the diff said is the lines that are there.
        let shorter = model.revealing([:], fileEnd: 41)
        XCTAssertNil(shorter.rows.last?.gap)
    }

    func testAFileWithNoGapsIsUnchanged() {
        let plain = buildDiffFileModel(from: file(twoHunks))
        XCTAssertEqual(plain.revealing(lines(1...4), fileEnd: 10), plain)
    }
}
