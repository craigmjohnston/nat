import XCTest
@testable import NatKit

final class SliceAddResultTests: XCTestCase {
    func testDecoding() throws {
        let json = """
        {
            "id": "slice-new",
            "name": "Add the settings scene",
            "status": "Todo",
            "milestone_id": "Phase 1",
            "milestone_name": "Phase 1",
            "repo": "/path/to/repo",
            "url": "https://notion.so/slice-new"
        }
        """

        let data = json.data(using: .utf8)!
        let result = try JSONDecoder().decode(SliceAddResult.self, from: data)

        XCTAssertEqual(result.id, "slice-new")
        XCTAssertEqual(result.name, "Add the settings scene")
        XCTAssertEqual(result.status, "Todo")
        XCTAssertEqual(result.milestoneID, "Phase 1")
        XCTAssertEqual(result.milestoneName, "Phase 1")
        XCTAssertEqual(result.repo, "/path/to/repo")
        XCTAssertEqual(result.url, "https://notion.so/slice-new")
    }

    func testEquality() {
        let a = SliceAddResult(id: "s1", name: "N", status: "Todo", milestoneID: "m1", milestoneName: "M1", repo: "/r", url: "https://x")
        let b = SliceAddResult(id: "s1", name: "N", status: "Todo", milestoneID: "m1", milestoneName: "M1", repo: "/r", url: "https://x")
        let c = SliceAddResult(id: "s2", name: "N", status: "Todo", milestoneID: "m1", milestoneName: "M1", repo: "/r", url: "https://x")

        XCTAssertEqual(a, b)
        XCTAssertNotEqual(a, c)
    }
}
