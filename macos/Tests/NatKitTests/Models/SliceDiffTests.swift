import XCTest
@testable import NatKit

final class SliceDiffTests: XCTestCase {
    func testDecodingAFullFile() throws {
        let json = """
        {
            "base": "main",
            "branch": "nat/example",
            "files": [
                {
                    "path": "a.go",
                    "old_path": "a.go",
                    "adds": 2,
                    "dels": 1,
                    "described": false,
                    "lines": ["diff --git a/a.go b/a.go", "-old", "+new"]
                }
            ]
        }
        """
        let diff = try JSONDecoder().decode(SliceDiff.self, from: json.data(using: .utf8)!)

        XCTAssertEqual(diff.base, "main")
        XCTAssertEqual(diff.branch, "nat/example")
        XCTAssertEqual(diff.files.count, 1)

        let file = diff.files[0]
        XCTAssertEqual(file.path, "a.go")
        XCTAssertEqual(file.oldPath, "a.go")
        XCTAssertEqual(file.adds, 2)
        XCTAssertEqual(file.dels, 1)
        XCTAssertFalse(file.described)
        XCTAssertEqual(file.lines, ["diff --git a/a.go b/a.go", "-old", "+new"])
        XCTAssertFalse(file.isRenamed)
    }

    func testDecodingWithoutOldPathDefaultsToEmpty() throws {
        // The Go side omits old_path entirely when it would be empty
        // (`omitempty`); decoding must not fail just because the key is gone.
        let json = """
        {
            "path": "a.go",
            "adds": 0,
            "dels": 0,
            "described": false,
            "lines": []
        }
        """
        let file = try JSONDecoder().decode(SliceDiffFile.self, from: json.data(using: .utf8)!)

        XCTAssertEqual(file.oldPath, "")
        XCTAssertFalse(file.isRenamed, "an empty old_path is not a rename, whatever else is true")
    }

    func testIsRenamedIsTrueOnlyWhenOldPathDiffersAndIsNonEmpty() {
        let renamed = SliceDiffFile(path: "new.go", oldPath: "old.go", adds: 0, dels: 0, described: false, lines: [])
        XCTAssertTrue(renamed.isRenamed)

        let created = SliceDiffFile(path: "new.go", oldPath: "new.go", adds: 1, dels: 0, described: false, lines: [])
        XCTAssertFalse(created.isRenamed)

        let noOldPath = SliceDiffFile(path: "new.go", adds: 1, dels: 0, described: false, lines: [])
        XCTAssertFalse(noOldPath.isRenamed)
    }

    func testEncodingOmitsNothingAndRoundTrips() throws {
        let original = SliceDiff(
            base: "main",
            branch: "nat/example",
            files: [
                SliceDiffFile(path: "a.go", oldPath: "old.go", adds: 3, dels: 2, described: true, lines: ["x", "y"])
            ]
        )
        let data = try JSONEncoder().encode(original)
        let decoded = try JSONDecoder().decode(SliceDiff.self, from: data)
        XCTAssertEqual(decoded, original)
    }

    func testEquality() {
        let a = SliceDiff(base: "main", branch: "b", files: [])
        let b = SliceDiff(base: "main", branch: "b", files: [])
        let c = SliceDiff(base: "main", branch: "other", files: [])
        XCTAssertEqual(a, b)
        XCTAssertNotEqual(a, c)
    }

    // MARK: - Syntax highlighting: language and tokens

    func testDecodingLanguageAndTokens() throws {
        let json = """
        {
            "path": "a.go",
            "adds": 1,
            "dels": 0,
            "described": false,
            "lines": ["+func f() {}"],
            "language": "Go",
            "tokens": [[["keyword", 4], ["text", 8]]]
        }
        """
        let file = try JSONDecoder().decode(SliceDiffFile.self, from: json.data(using: .utf8)!)

        XCTAssertEqual(file.language, "Go")
        XCTAssertEqual(file.tokens, [[TokenRun(kind: .keyword, length: 4), TokenRun(kind: .text, length: 8)]])
    }

    func testDecodingWithoutLanguageOrTokensDefaultsToAbsent() throws {
        // Both keys are omitted on the wire for an unmatched or described
        // file — decoding must not fail just because they are gone.
        let json = """
        {
            "path": "a.go",
            "adds": 0,
            "dels": 0,
            "described": false,
            "lines": []
        }
        """
        let file = try JSONDecoder().decode(SliceDiffFile.self, from: json.data(using: .utf8)!)

        XCTAssertEqual(file.language, "")
        XCTAssertNil(file.tokens)
    }

    func testTokenRunRoundTrips() throws {
        let run = TokenRun(kind: .string, length: 7)
        let data = try JSONEncoder().encode(run)
        XCTAssertEqual(String(data: data, encoding: .utf8), "[\"string\",7]")

        let decoded = try JSONDecoder().decode(TokenRun.self, from: data)
        XCTAssertEqual(decoded, run)
    }

    func testTokenRunRefusesAWrongShapedArray() {
        let json = "[\"string\", 7, 9]"
        XCTAssertThrowsError(try JSONDecoder().decode(TokenRun.self, from: json.data(using: .utf8)!))

        let short = "[\"string\"]"
        XCTAssertThrowsError(try JSONDecoder().decode(TokenRun.self, from: short.data(using: .utf8)!))
    }
}
