import XCTest
@testable import NatKit

final class PRDetailTests: XCTestCase {
    private func decode<T: Decodable>(_ type: T.Type, _ json: String) throws -> T {
        try JSONDecoder().decode(T.self, from: json.data(using: .utf8)!)
    }

    // MARK: - PRReview.submittedAt

    func testReviewSubmittedAtDecodesAnRFC3339Timestamp() throws {
        let review = try decode(PRReview.self, """
        {"author": "reviewer", "state": "APPROVED", "body": "", "submitted_at": "2026-03-01T12:00:00Z"}
        """)
        XCTAssertNotNil(review.submittedAt)
    }

    func testReviewSubmittedAtDecodesFractionalSeconds() throws {
        let review = try decode(PRReview.self, """
        {"author": "reviewer", "state": "APPROVED", "body": "", "submitted_at": "2026-03-01T12:00:00.500Z"}
        """)
        XCTAssertNotNil(review.submittedAt)
    }

    func testReviewSubmittedAtIsNilForGosZeroTimeMarker() throws {
        let review = try decode(PRReview.self, """
        {"author": "reviewer", "state": "PENDING", "body": "", "submitted_at": "0001-01-01T00:00:00Z"}
        """)
        XCTAssertNil(review.submittedAt, "the zero-time marker should read back as no time at all")
    }

    func testReviewSubmittedAtIsNilWhenTheKeyIsMissingEntirely() throws {
        let review = try decode(PRReview.self, """
        {"author": "reviewer", "state": "PENDING", "body": ""}
        """)
        XCTAssertNil(review.submittedAt)
    }

    func testReviewSubmittedAtIsNilForAnEmptyString() throws {
        let review = try decode(PRReview.self, """
        {"author": "reviewer", "state": "PENDING", "body": "", "submitted_at": ""}
        """)
        XCTAssertNil(review.submittedAt)
    }

    // MARK: - PRCommentEntry.createdAt

    func testCommentCreatedAtDecodesAnRFC3339Timestamp() throws {
        let comment = try decode(PRCommentEntry.self, """
        {"author": "craig", "body": "hi", "created_at": "2026-03-01T10:00:00Z", "url": "https://x"}
        """)
        XCTAssertEqual(comment.createdAt.timeIntervalSince1970, 1772359200)
    }

    func testCommentCreatedAtThrowsOnAnUnreadableTimestamp() {
        XCTAssertThrowsError(try decode(PRCommentEntry.self, """
        {"author": "craig", "body": "hi", "created_at": "not a date", "url": "https://x"}
        """))
    }

    func testCommentCreatedAtThrowsOnGosZeroTimeMarker() {
        // A comment always has a real creation time; the zero marker is
        // treated as unreadable here rather than silently accepted, since a
        // comment with no time at all would be one the timeline cannot place.
        XCTAssertThrowsError(try decode(PRCommentEntry.self, """
        {"author": "craig", "body": "hi", "created_at": "0001-01-01T00:00:00Z", "url": "https://x"}
        """))
    }

    // MARK: - PRDetail's optional change tally

    func testPRDetailDecodesWithoutTheOptionalTally() throws {
        let pr = try decode(PRDetail.self, """
        {
          "number": 1, "title": "t", "body": "", "state": "OPEN", "is_draft": false,
          "author": "a", "base_ref_name": "main", "head_ref_name": "h", "url": "https://x",
          "checks": [], "reviews": [], "comments": [],
          "review_decision": "", "mergeable": "", "merge_state_status": ""
        }
        """)
        XCTAssertNil(pr.additions)
        XCTAssertNil(pr.deletions)
        XCTAssertNil(pr.changedFiles)
        XCTAssertNil(pr.commits)
    }

    func testPRDetailDecodesWithTheOptionalTally() throws {
        let pr = try decode(PRDetail.self, """
        {
          "number": 1, "title": "t", "body": "", "state": "OPEN", "is_draft": false,
          "author": "a", "base_ref_name": "main", "head_ref_name": "h", "url": "https://x",
          "checks": [], "reviews": [], "comments": [],
          "review_decision": "", "mergeable": "", "merge_state_status": "",
          "additions": 10, "deletions": 2, "changed_files": 3, "commits": 4
        }
        """)
        XCTAssertEqual(pr.additions, 10)
        XCTAssertEqual(pr.deletions, 2)
        XCTAssertEqual(pr.changedFiles, 3)
        XCTAssertEqual(pr.commits, 4)
    }

    func testPRDetailMemberwiseInit() {
        let pr = PRDetail(
            number: 1, title: "t", body: "b", state: "OPEN", isDraft: true, author: "a",
            baseRefName: "main", headRefName: "h", url: "https://x",
            reviewDecision: "", mergeable: "", mergeStateStatus: ""
        )
        XCTAssertEqual(pr.number, 1)
        XCTAssertTrue(pr.checks.isEmpty)
        XCTAssertNil(pr.additions)
    }
}
