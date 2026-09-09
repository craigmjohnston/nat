import XCTest
@testable import NatKit

/// Ports of `internal/tui/diffref_test.go`'s `TestLineRef` and
/// `internal/tui/diffcomment_test.go`'s `TestCommentPrompt` — the acceptance
/// test for `commentsPrompt` is that it produces byte-identical wording to
/// the Go TUI's own `commentPrompt`/`commentTitle`/`lineRef`, since the same
/// agent may be handed a review from either.
final class DiffCommentsTests: XCTestCase {
    // MARK: - lineRef, ported from TestLineRef

    /// Builds the same two-hunk section `internal/tui/diffref_test.go`'s
    /// `refSection` diffs, run through our own production `diffRows(for:)` —
    /// the same builder `DiffModel` uses on a real `nat slice-diff` read —
    /// rather than hand-built rows, so the row/number mapping this test
    /// checks `lineRef` against is the one the app would actually produce.
    private func refSectionRows() -> [DiffRow] {
        let file = SliceDiffFile(
            path: "main.go",
            oldPath: "main.go",
            adds: 3,
            dels: 2,
            described: false,
            lines: [
                "diff --git a/main.go b/main.go",
                "index 1111111..2222222 100644",
                "--- a/main.go",
                "+++ b/main.go",
                "@@ -10,4 +10,5 @@ func main() {",
                " \tfirst",
                "-\tremoved",
                "+\tadded",
                "+\talso added",
                " \tlast",
                "@@ -40,2 +41,2 @@ func other() {",
                " \tcontext",
                "-\tgone"
            ]
        )
        return diffRows(for: file)
    }

    /// The rows `refSectionRows()` produces, in order: `first`(old 10/new 10),
    /// `removed`(old 11), `added`(new 11), `also added`(new 12),
    /// `last`(old 12/new 13), a hunk break, `context`(old 40/new 41),
    /// `gone`(old 41). Noise headers and the file's first hunk header
    /// produce no row at all, exactly as a real read's would not.
    func testRefSectionRowsMatchWhatGitWrote() {
        let rows = refSectionRows()
        XCTAssertEqual(rows.count, 8)
        XCTAssertEqual(rows[5].kind, .hunkBreak)
    }

    func testLineRefMatchesGoTUIReferenceFormat() {
        let rows = refSectionRows()

        // "a context line" — TestLineRef(start: 5, span: 1) == "line 10"
        XCTAssertEqual(lineRef(for: [rows[0]]), "line 10")

        // "a removed line alone" — (start: 6, span: 1) == "line 11 of the base"
        XCTAssertEqual(lineRef(for: [rows[1]]), "line 11 of the base")

        // "an added line" — (start: 7, span: 1) == "line 11"
        XCTAssertEqual(lineRef(for: [rows[2]]), "line 11")

        // "a run across a removal" — (start: 6, span: 3) == "lines 11-12"
        XCTAssertEqual(lineRef(for: Array(rows[1...3])), "lines 11-12")

        // "the whole first hunk" — (start: 4, span: 6) == "lines 10-13"
        // (the raw span includes the hunk header itself, which produces no
        // row and so contributes no number either way)
        XCTAssertEqual(lineRef(for: Array(rows[0...4])), "lines 10-13")

        // "a header line" — nothing numbers it on either side, so the
        // reference is empty; the hunk-break row is our own stand-in.
        XCTAssertEqual(lineRef(for: [rows[5]]), "")

        // "the second hunk's context" — (start: 11, span: 1) == "line 41"
        XCTAssertEqual(lineRef(for: [rows[6]]), "line 41")

        // "the second hunk's removal" — (start: 12, span: 1) == "line 41 of the base"
        XCTAssertEqual(lineRef(for: [rows[7]]), "line 41 of the base")
    }

    /// Ports `TestDiffCommentMarksItsLines`'s own assertion on `Ref`: a
    /// comment on a removal and the addition that replaced it names the one
    /// line the branch actually leaves there — "line 13" — even though the
    /// run covers two raw lines, one of which has no line on the new side at
    /// all.
    func testLineRefPrefersTheNewSideOverARemovalItReplaced() {
        let removed = DiffRow(id: "a#12#_", kind: .removed, oldNumber: 12, newNumber: nil, prefix: "-", text: "old line")
        let added = DiffRow(id: "a#_#13", kind: .added, oldNumber: nil, newNumber: 13, prefix: "+", text: "new line")
        XCTAssertEqual(lineRef(for: [removed, added]), "line 13")
    }

    // MARK: - commentTitle

    func testCommentTitleNamesTheLinesOrCountsThem() {
        XCTAssertEqual(commentTitle(path: "main.go", ref: "line 12", span: 1), "main.go, line 12")
        XCTAssertEqual(commentTitle(path: "other.go", ref: "", span: 1), "other.go, 1 line")
        XCTAssertEqual(commentTitle(path: "other.go", ref: "", span: 3), "other.go, 3 lines")
    }

    // MARK: - commentsPrompt, ported from TestCommentPrompt

    /// Ports `internal/tui/diffcomment_test.go`'s `TestCommentPrompt`
    /// byte-for-byte: the same branch name, the same two comments (one with
    /// a numbered reference, one without), and the same expected substrings.
    func testCommentPromptMatchesGoTUIFormat() {
        let fooRow = DiffRow(id: "main.go#_#12", kind: .added, oldNumber: nil, newNumber: 12, prefix: "+", text: "\tfoo()")
        let mainFile = DiffFileModel(path: "main.go", oldPath: "main.go", adds: 1, dels: 0, described: false, rows: [fooRow])

        let binaryRow = DiffRow(id: "other.go#row0", kind: .described, oldNumber: nil, newNumber: nil, prefix: nil, text: "binary")
        let otherFile = DiffFileModel(path: "other.go", oldPath: "other.go", adds: 0, dels: 0, described: true, rows: [binaryRow])

        let diff = DiffModel(base: "main", branch: "slice/review", files: [mainFile, otherFile])
        let comments = [
            PendingComment(path: "main.go", anchorRowIDs: [fooRow.id], text: "call bar instead"),
            PendingComment(path: "other.go", anchorRowIDs: [binaryRow.id], text: "and this")
        ]

        let got = commentsPrompt(comments, diff: diff)

        for want in [
            "I have reviewed the diff of slice/review and left 2 comments on it.",
            "## main.go, line 12",
            "> +\tfoo()",
            "call bar instead",
            "## other.go, 1 line",
            "and this"
        ] {
            XCTAssertTrue(got.contains(want), "prompt is missing \(want.debugDescription):\n\(got)")
        }
    }

    /// A single comment says "1 comment", not "1 comments" — `plural` is
    /// exercised through the prompt's own opening line.
    func testCommentPromptSingularCount() {
        let row = DiffRow(id: "a.go#_#1", kind: .added, oldNumber: nil, newNumber: 1, prefix: "+", text: "hello")
        let file = DiffFileModel(path: "a.go", oldPath: "a.go", adds: 1, dels: 0, described: false, rows: [row])
        let diff = DiffModel(base: "main", branch: "nat/example", files: [file])
        let comment = PendingComment(path: "a.go", anchorRowIDs: [row.id], text: "nice")

        let got = commentsPrompt([comment], diff: diff)
        XCTAssertTrue(got.contains("left 1 comment on it."))
    }

    /// `commentsPrompt` quotes comments in exactly the order it is handed
    /// them — the ordering itself is `DiffStore.comments`'s job (covered by
    /// `DiffStoreTests.testCommentsOrderedByFileThenPositionWithinFile`,
    /// mirroring `TestDiffCommentsAreOrderedByFileAndLine`), the same split
    /// of responsibility the Go TUI has between `Comments()` and
    /// `commentPrompt`. This checks the prompt builder holds up its half:
    /// whatever order it receives is the order it writes out.
    func testCommentPromptPreservesTheOrderItIsGiven() {
        let earlyRow = DiffRow(id: "a.go#_#1", kind: .added, oldNumber: nil, newNumber: 1, prefix: "+", text: "one")
        let lateRow = DiffRow(id: "a.go#_#2", kind: .added, oldNumber: nil, newNumber: 2, prefix: "+", text: "two")
        let fileA = DiffFileModel(
            path: "a.go", oldPath: "a.go", adds: 2, dels: 0, described: false, rows: [earlyRow, lateRow]
        )
        let bRow = DiffRow(id: "b.go#_#1", kind: .added, oldNumber: nil, newNumber: 1, prefix: "+", text: "hi")
        let fileB = DiffFileModel(path: "b.go", oldPath: "b.go", adds: 1, dels: 0, described: false, rows: [bRow])
        let diff = DiffModel(base: "main", branch: "nat/example", files: [fileA, fileB])

        // Already in file/line order, as DiffStore.comments would hand them.
        let comments = [
            PendingComment(path: "a.go", anchorRowIDs: [earlyRow.id], text: "the earlier line"),
            PendingComment(path: "a.go", anchorRowIDs: [lateRow.id], text: "the later line"),
            PendingComment(path: "b.go", anchorRowIDs: [bRow.id], text: "the second file")
        ]

        let got = commentsPrompt(comments, diff: diff)
        let earlierRange = got.range(of: "the earlier line")
        let laterRange = got.range(of: "the later line")
        let secondFileRange = got.range(of: "the second file")
        XCTAssertNotNil(earlierRange)
        XCTAssertNotNil(laterRange)
        XCTAssertNotNil(secondFileRange)
        XCTAssertTrue(earlierRange!.lowerBound < laterRange!.lowerBound)
        XCTAssertTrue(laterRange!.lowerBound < secondFileRange!.lowerBound)
    }

    // MARK: - initialsFor

    func testInitialsForTakesTheFirstLetterOfUpToTwoWords() {
        XCTAssertEqual(initialsFor("Craig Johnston"), "CJ")
        XCTAssertEqual(initialsFor("Craig"), "C")
        XCTAssertEqual(initialsFor("Craig Middle Johnston"), "CM")
        XCTAssertEqual(initialsFor(nil), "C")
        XCTAssertEqual(initialsFor(""), "C")
        XCTAssertEqual(initialsFor("   "), "C")
    }

    // MARK: - PendingComment

    func testPendingCommentEquality() {
        let a = PendingComment(id: UUID(), path: "a.go", anchorRowIDs: ["1"], text: "x")
        let b = PendingComment(id: a.id, path: "a.go", anchorRowIDs: ["1"], text: "x")
        let c = PendingComment(id: UUID(), path: "a.go", anchorRowIDs: ["1"], text: "x")
        XCTAssertEqual(a, b)
        XCTAssertNotEqual(a, c, "a fresh UUID should not compare equal even with the same lines and text")
    }
}
