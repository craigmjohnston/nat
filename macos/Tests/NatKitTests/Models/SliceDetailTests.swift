import XCTest
@testable import NatKit

final class SliceDetailTests: XCTestCase {
    func testSliceDetailDecodingMinimal() throws {
        let json = """
        {
            "id": "slice-123",
            "name": "Implement feature",
            "url": "https://notion.so/slice-123",
            "status": "In progress",
            "milestone": "Milestone 1",
            "assignee": "Craig",
            "branch": "feature/test",
            "blocked": false,
            "handed_back": false,
            "brief": "Do the thing"
        }
        """

        let data = json.data(using: .utf8)!
        let decoder = JSONDecoder()
        let detail = try decoder.decode(SliceDetail.self, from: data)

        XCTAssertEqual(detail.id, "slice-123")
        XCTAssertEqual(detail.name, "Implement feature")
        XCTAssertEqual(detail.status, "In progress")
        XCTAssertEqual(detail.milestone, "Milestone 1")
        XCTAssertEqual(detail.brief, "Do the thing")
        XCTAssertEqual(detail.branch, "feature/test")
        XCTAssertFalse(detail.blocked)
        XCTAssertFalse(detail.handedBack)
    }

    func testSliceDetailDecodingWithAllFields() throws {
        let json = """
        {
            "id": "slice-456",
            "name": "Review PR",
            "url": "https://notion.so/slice-456",
            "status": "Done",
            "milestone": "Milestone 2",
            "assignee": "Jane",
            "branch": "review/pr",
            "repo": "/path/to/repo",
            "pr": "https://github.com/owner/repo/pull/42",
            "depends_on": ["slice-123", "slice-124"],
            "blocked": true,
            "handed_back": true,
            "state": "awaiting_review",
            "brief": "# Brief\\nWith markdown"
        }
        """

        let data = json.data(using: .utf8)!
        let decoder = JSONDecoder()
        let detail = try decoder.decode(SliceDetail.self, from: data)

        XCTAssertEqual(detail.id, "slice-456")
        XCTAssertEqual(detail.name, "Review PR")
        XCTAssertEqual(detail.status, "Done")
        XCTAssertEqual(detail.branch, "review/pr")
        XCTAssertEqual(detail.repo, "/path/to/repo")
        XCTAssertEqual(detail.pr, "https://github.com/owner/repo/pull/42")
        XCTAssertEqual(detail.dependsOn, ["slice-123", "slice-124"])
        XCTAssertTrue(detail.blocked)
        XCTAssertTrue(detail.handedBack)
        XCTAssertEqual(detail.state, "awaiting_review")
    }

    func testSliceDetailDecodingWithoutOptionalFields() throws {
        let json = """
        {
            "id": "slice-789",
            "name": "Test slice",
            "url": "https://notion.so/slice-789",
            "status": "Todo",
            "milestone": "Milestone 3",
            "assignee": "",
            "blocked": false,
            "handed_back": false,
            "brief": ""
        }
        """

        let data = json.data(using: .utf8)!
        let decoder = JSONDecoder()
        let detail = try decoder.decode(SliceDetail.self, from: data)

        XCTAssertNil(detail.branch)
        XCTAssertNil(detail.repo)
        XCTAssertNil(detail.pr)
        XCTAssertNil(detail.dependsOn)
        XCTAssertNil(detail.state)
    }

    func testSliceDetailEquality() {
        let detail1 = SliceDetail(
            id: "slice-1",
            name: "Test",
            url: "https://notion.so/slice-1",
            status: "Todo",
            milestone: "M1",
            assignee: "Craig",
            blocked: false,
            handedBack: false,
            brief: "Brief"
        )

        let detail2 = SliceDetail(
            id: "slice-1",
            name: "Test",
            url: "https://notion.so/slice-1",
            status: "Todo",
            milestone: "M1",
            assignee: "Craig",
            blocked: false,
            handedBack: false,
            brief: "Brief"
        )

        let detail3 = SliceDetail(
            id: "slice-2",
            name: "Test",
            url: "https://notion.so/slice-2",
            status: "Todo",
            milestone: "M1",
            assignee: "Craig",
            blocked: false,
            handedBack: false,
            brief: "Brief"
        )

        XCTAssertEqual(detail1, detail2)
        XCTAssertNotEqual(detail1, detail3)
    }
}
