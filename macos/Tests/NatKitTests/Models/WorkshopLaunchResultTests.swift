import XCTest
@testable import NatKit

final class WorkshopLaunchResultTests: XCTestCase {
    func testDecoding() throws {
        let json = """
        {
            "session": "nat-plan",
            "workdir": "/path/to/repo",
            "wishlist": false
        }
        """

        let data = json.data(using: .utf8)!
        let result = try JSONDecoder().decode(WorkshopLaunchResult.self, from: data)

        XCTAssertEqual(result.session, "nat-plan")
        XCTAssertEqual(result.workdir, "/path/to/repo")
        XCTAssertFalse(result.wishlist)
    }

    func testEquality() {
        let a = WorkshopLaunchResult(session: "nat-plan", workdir: "/path", wishlist: true)
        let b = WorkshopLaunchResult(session: "nat-plan", workdir: "/path", wishlist: true)
        let c = WorkshopLaunchResult(session: "nat-plan", workdir: "/path", wishlist: false)

        XCTAssertEqual(a, b)
        XCTAssertNotEqual(a, c)
    }
}
