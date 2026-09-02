import XCTest
@testable import NatKit

final class DiffModelTests: XCTestCase {
    // MARK: - Helpers

    private func file(
        path: String = "a.go",
        oldPath: String = "a.go",
        adds: Int = 0,
        dels: Int = 0,
        described: Bool = false,
        lines: [String]
    ) -> SliceDiffFile {
        SliceDiffFile(path: path, oldPath: oldPath, adds: adds, dels: dels, described: described, lines: lines)
    }

    // MARK: - Noise suppression

    func testHeaderLinesProduceNoRows() {
        let f = file(lines: [
            "diff --git a/a.go b/a.go",
            "index 1111111..2222222 100644",
            "--- a/a.go",
            "+++ b/a.go",
            "@@ -1,1 +1,1 @@",
            "-old",
            "+new"
        ])
        let rows = diffRows(for: f)
        // Only the two content lines: the header, blob, and path lines are
        // noise, and the one hunk header here is the file's first (dropped).
        XCTAssertEqual(rows.count, 2)
        XCTAssertEqual(rows[0].kind, .removed)
        XCTAssertEqual(rows[1].kind, .added)
    }

    func testFirstHunkHeaderProducesNoRowLaterOneBreaks() {
        let f = file(lines: [
            "diff --git a/a.go b/a.go",
            "index 1111111..2222222 100644",
            "--- a/a.go",
            "+++ b/a.go",
            "@@ -1,2 +1,2 @@",
            " one",
            " two",
            "@@ -10,1 +10,2 @@ some context",
            " ten",
            "+eleven"
        ])
        let rows = diffRows(for: f)
        // one, two, <break>, ten, eleven
        XCTAssertEqual(rows.count, 5)
        XCTAssertEqual(rows[0].kind, .context)
        XCTAssertEqual(rows[1].kind, .context)
        XCTAssertEqual(rows[2].kind, .hunkBreak)
        XCTAssertEqual(rows[2].text, "@@ -10,1 +10,2 @@ some context")
        XCTAssertNil(rows[2].oldNumber)
        XCTAssertNil(rows[2].newNumber)
        XCTAssertEqual(rows[3].kind, .context)
        XCTAssertEqual(rows[4].kind, .added)
    }

    // MARK: - Line numbering

    func testContextAddedRemovedAreNumbered() {
        let f = file(lines: [
            "diff --git a/a.go b/a.go",
            "--- a/a.go",
            "+++ b/a.go",
            "@@ -4,4 +4,5 @@",
            " unchanged",
            "-removed line",
            "+added line one",
            "+added line two",
            " trailing"
        ])
        let rows = diffRows(for: f)
        XCTAssertEqual(rows.count, 5)

        XCTAssertEqual(rows[0].kind, .context)
        XCTAssertEqual(rows[0].oldNumber, 4)
        XCTAssertEqual(rows[0].newNumber, 4)
        XCTAssertEqual(rows[0].text, "unchanged")
        XCTAssertEqual(rows[0].prefix, " ")

        XCTAssertEqual(rows[1].kind, .removed)
        XCTAssertEqual(rows[1].oldNumber, 5)
        XCTAssertNil(rows[1].newNumber)
        XCTAssertEqual(rows[1].text, "removed line")
        XCTAssertEqual(rows[1].prefix, "-")

        XCTAssertEqual(rows[2].kind, .added)
        XCTAssertNil(rows[2].oldNumber)
        XCTAssertEqual(rows[2].newNumber, 5)
        XCTAssertEqual(rows[2].text, "added line one")

        XCTAssertEqual(rows[3].kind, .added)
        XCTAssertEqual(rows[3].newNumber, 6)

        XCTAssertEqual(rows[4].kind, .context)
        XCTAssertEqual(rows[4].oldNumber, 6)
        XCTAssertEqual(rows[4].newNumber, 7)
    }

    func testBlankContextLineHasEmptyText() {
        let f = file(lines: [
            "diff --git a/a.go b/a.go",
            "--- a/a.go",
            "+++ b/a.go",
            "@@ -1,2 +1,2 @@",
            " one",
            "",
            " two"
        ])
        let rows = diffRows(for: f)
        XCTAssertEqual(rows.count, 3)
        XCTAssertEqual(rows[1].text, "")
        XCTAssertEqual(rows[1].kind, .context)
        XCTAssertEqual(rows[1].oldNumber, 2)
        XCTAssertEqual(rows[1].newNumber, 2)
    }

    func testNoNewlineMarkerIsUnnumberedContext() {
        let f = file(lines: [
            "diff --git a/a.go b/a.go",
            "--- a/a.go",
            "+++ b/a.go",
            "@@ -1,1 +1,1 @@",
            "-old",
            "\\ No newline at end of file",
            "+new"
        ])
        let rows = diffRows(for: f)
        XCTAssertEqual(rows.count, 3)
        XCTAssertEqual(rows[1].kind, .context)
        XCTAssertNil(rows[1].oldNumber)
        XCTAssertNil(rows[1].newNumber)
        XCTAssertNil(rows[1].prefix)
        XCTAssertEqual(rows[1].text, "\\ No newline at end of file")
    }

    // MARK: - Add-only file

    func testAddOnlyFileIsAllAddedRows() {
        let f = file(
            path: "internal/tui/newfile.go", oldPath: "internal/tui/newfile.go",
            adds: 2, dels: 0,
            lines: [
                "diff --git a/internal/tui/newfile.go b/internal/tui/newfile.go",
                "index 0000000..3a1f1d4",
                "--- /dev/null",
                "+++ b/internal/tui/newfile.go",
                "@@ -0,0 +1,2 @@",
                "+package tui",
                "+"
            ]
        )
        let model = buildDiffFileModel(from: f)
        XCTAssertEqual(model.rows.count, 2)
        XCTAssertTrue(model.rows.allSatisfy { $0.kind == .added })
        XCTAssertEqual(model.rows[0].newNumber, 1)
        XCTAssertEqual(model.rows[1].newNumber, 2)
        XCTAssertFalse(model.isRenamed)
    }

    // MARK: - Rename

    func testRenameMetadataLinesAreUnnumberedContextBeforeTheHunk() {
        let f = file(
            path: "new_name.go", oldPath: "old_name.go",
            adds: 1, dels: 1,
            lines: [
                "diff --git a/old_name.go b/new_name.go",
                "similarity index 66%",
                "rename from old_name.go",
                "rename to new_name.go",
                "index 686ae1c..1a1a054 100644",
                "--- a/old_name.go",
                "+++ b/new_name.go",
                "@@ -1,1 +1,1 @@",
                "-old",
                "+new"
            ]
        )
        let model = buildDiffFileModel(from: f)
        XCTAssertTrue(model.isRenamed)
        // similarity/rename from/rename to are drawn (not noise-dropped), but
        // numbered by nothing since no hunk has started yet.
        XCTAssertEqual(model.rows.count, 5)
        XCTAssertEqual(model.rows[0].text, "similarity index 66%")
        XCTAssertEqual(model.rows[0].kind, .context)
        XCTAssertNil(model.rows[0].oldNumber)
        XCTAssertEqual(model.rows[1].text, "rename from old_name.go")
        XCTAssertEqual(model.rows[2].text, "rename to new_name.go")
        XCTAssertEqual(model.rows[3].kind, .removed)
        XCTAssertEqual(model.rows[4].kind, .added)
    }

    func testNonRenamedFileHasEqualOldAndNewPath() {
        let f = file(path: "a.go", oldPath: "a.go", lines: ["diff --git a/a.go b/a.go"])
        let model = buildDiffFileModel(from: f)
        XCTAssertFalse(model.isRenamed)
    }

    // MARK: - Described (binary) file

    func testDescribedFileKeepsItsMessageAsPlainRows() {
        let f = file(
            path: "docs/shot.png", oldPath: "docs/shot.png",
            described: true,
            lines: [
                "diff --git a/docs/shot.png b/docs/shot.png",
                "index 5555555..6666666 100644",
                "Binary files a/docs/shot.png and b/docs/shot.png differ"
            ]
        )
        let model = buildDiffFileModel(from: f)
        XCTAssertEqual(model.rows.count, 1)
        XCTAssertEqual(model.rows[0].kind, .described)
        XCTAssertEqual(model.rows[0].text, "Binary files a/docs/shot.png and b/docs/shot.png differ")
        XCTAssertNil(model.rows[0].oldNumber)
        XCTAssertNil(model.rows[0].newNumber)
        XCTAssertNil(model.rows[0].prefix)
    }

    // MARK: - Stable line IDs

    func testAnchoredRowsShareIDAcrossParsesOfTheSameFile() {
        let lines = [
            "diff --git a/a.go b/a.go",
            "--- a/a.go",
            "+++ b/a.go",
            "@@ -1,1 +1,1 @@",
            "-old",
            "+new"
        ]
        let a = diffRows(for: file(lines: lines))
        let b = diffRows(for: file(lines: lines))
        XCTAssertEqual(a[0].id, b[0].id)
        XCTAssertEqual(a[1].id, b[1].id)
        XCTAssertNotEqual(a[0].id, a[1].id)
    }

    func testAnchoredRowIDsDifferByFilePath() {
        let lines = [
            "diff --git a/a.go b/a.go",
            "--- a/a.go",
            "+++ b/a.go",
            "@@ -1,1 +1,1 @@",
            " same"
        ]
        let a = diffRows(for: file(path: "a.go", oldPath: "a.go", lines: lines))
        let b = diffRows(for: file(path: "b.go", oldPath: "b.go", lines: lines))
        XCTAssertNotEqual(a[0].id, b[0].id)
    }

    func testUnanchoredRowsAreUniqueWithinAFile() {
        let f = file(
            path: "docs/shot.png", oldPath: "docs/shot.png",
            described: true,
            lines: [
                "diff --git a/docs/shot.png b/docs/shot.png",
                "Binary files a/docs/shot.png and b/docs/shot.png differ",
                "GIT binary patch"
            ]
        )
        let rows = diffRows(for: f)
        XCTAssertEqual(rows.count, 2)
        XCTAssertNotEqual(rows[0].id, rows[1].id)
    }

    // MARK: - Malformed hunk header

    func testMalformedHunkHeaderIsNotTreatedAsAHunkStart() {
        let f = file(lines: [
            "diff --git a/a.go b/a.go",
            "--- a/a.go",
            "+++ b/a.go",
            "@@ not a real header @@",
            " context line"
        ])
        let rows = diffRows(for: f)
        // Neither dropped nor numbered: drawn like any other unrecognised
        // pre-hunk line.
        XCTAssertEqual(rows.count, 2)
        XCTAssertEqual(rows[0].kind, .context)
        XCTAssertNil(rows[0].oldNumber)
        XCTAssertEqual(rows[1].kind, .context)
        XCTAssertNil(rows[1].oldNumber)
    }

    // MARK: - Whole-model building

    func testBuildDiffModelCarriesBaseAndBranchAndMapsEveryFile() {
        let diff = SliceDiff(
            base: "main",
            branch: "nat/example",
            files: [
                file(path: "a.go", oldPath: "a.go", adds: 1, dels: 0, lines: [
                    "diff --git a/a.go b/a.go",
                    "--- a/a.go",
                    "+++ b/a.go",
                    "@@ -0,0 +1,1 @@",
                    "+hello"
                ]),
                file(path: "b.go", oldPath: "b.go", adds: 0, dels: 1, lines: [
                    "diff --git a/b.go b/b.go",
                    "--- a/b.go",
                    "+++ b/b.go",
                    "@@ -1,1 +0,0 @@",
                    "-bye"
                ])
            ]
        )
        let model = buildDiffModel(from: diff)
        XCTAssertEqual(model.base, "main")
        XCTAssertEqual(model.branch, "nat/example")
        XCTAssertEqual(model.files.count, 2)
        XCTAssertEqual(model.files[0].path, "a.go")
        XCTAssertEqual(model.files[0].adds, 1)
        XCTAssertEqual(model.files[1].path, "b.go")
        XCTAssertEqual(model.files[1].dels, 1)
    }

    // MARK: - Shared gutter width

    func testNumberWidthIsTheWidestNumberAcrossEveryFile() {
        let small = file(path: "a.go", oldPath: "a.go", lines: [
            "diff --git a/a.go b/a.go", "--- a/a.go", "+++ b/a.go",
            "@@ -1,1 +1,1 @@", " one"
        ])
        let big = file(path: "b.go", oldPath: "b.go", lines: [
            "diff --git a/b.go b/b.go", "--- a/b.go", "+++ b/b.go",
            "@@ -998,3 +998,3 @@", " a", " b", " c"
        ])
        let model = DiffModel(base: "main", branch: "b", files: [
            buildDiffFileModel(from: small), buildDiffFileModel(from: big)
        ])
        // The big file's third context line lands at 1000, four digits wide.
        XCTAssertEqual(model.numberWidth, 4)
    }

    func testNumberWidthWithNoNumberedRowsIsAtLeastOne() {
        let f = file(described: true, lines: ["diff --git a/a.go b/a.go", "Binary files differ"])
        let model = DiffModel(base: "main", branch: "b", files: [buildDiffFileModel(from: f)])
        XCTAssertEqual(model.numberWidth, 1)
    }

    // MARK: - Real-shaped fixture (decoded from the FakeRunner's slice-diff JSON)

    func testRealShapedFixtureParsesEveryFileWithoutCrashing() throws {
        let data = fixtureSliceDiff.data(using: .utf8)!
        let diff = try JSONDecoder().decode(SliceDiff.self, from: data)
        let model = buildDiffModel(from: diff)

        XCTAssertEqual(model.files.count, 4)

        let board = model.files[0]
        // Two hunks, so exactly one hunkBreak row.
        XCTAssertEqual(board.rows.filter { $0.kind == .hunkBreak }.count, 1)
        XCTAssertEqual(board.rows.filter { $0.kind == .added }.count, 2)
        XCTAssertEqual(board.rows.filter { $0.kind == .removed }.count, 1)

        let renamed = model.files[2]
        XCTAssertTrue(renamed.isRenamed)

        let binary = model.files[3]
        XCTAssertTrue(binary.described)
        XCTAssertTrue(binary.rows.allSatisfy { $0.kind == .described })
    }
}
