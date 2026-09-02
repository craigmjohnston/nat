import XCTest
@testable import NatKit

final class SliceCommitsTests: XCTestCase {
    func testDecodingADoc() throws {
        let json = """
        {
            "base": "main",
            "branch": "nat/example",
            "commits": [
                {"sha": "abcdef1234567890", "subject": "Fix the thing", "author": "craig", "date": "2026-03-01T12:00:00Z"}
            ]
        }
        """
        let doc = try JSONDecoder().decode(SliceCommitsDoc.self, from: json.data(using: .utf8)!)

        XCTAssertEqual(doc.base, "main")
        XCTAssertEqual(doc.branch, "nat/example")
        XCTAssertEqual(doc.commits.count, 1)
        XCTAssertEqual(doc.commits[0].sha, "abcdef1234567890")
        XCTAssertEqual(doc.commits[0].subject, "Fix the thing")
        XCTAssertEqual(doc.commits[0].author, "craig")
    }

    func testDecodingRefusesAnUnreadableDate() {
        let json = """
        {"sha": "abc", "subject": "s", "author": "a", "date": "not-a-date"}
        """
        XCTAssertThrowsError(try JSONDecoder().decode(SliceCommit.self, from: json.data(using: .utf8)!))
    }

    func testShortSHAIsTheFirstEightCharacters() {
        let long = SliceCommit(sha: "abcdef1234567890", subject: "s", author: "a", date: Date())
        XCTAssertEqual(long.shortSHA, "abcdef12")

        let short = SliceCommit(sha: "abc", subject: "s", author: "a", date: Date())
        XCTAssertEqual(short.shortSHA, "abc")
    }

    func testIDIsTheSHA() {
        let commit = SliceCommit(sha: "abc123", subject: "s", author: "a", date: Date())
        XCTAssertEqual(commit.id, "abc123")
    }

    func testEquality() {
        let date = Date(timeIntervalSince1970: 0)
        let a = SliceCommit(sha: "abc", subject: "s", author: "a", date: date)
        let b = SliceCommit(sha: "abc", subject: "s", author: "a", date: date)
        let c = SliceCommit(sha: "xyz", subject: "s", author: "a", date: date)
        XCTAssertEqual(a, b)
        XCTAssertNotEqual(a, c)
    }
}
