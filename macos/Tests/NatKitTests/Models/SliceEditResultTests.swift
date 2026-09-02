import XCTest
@testable import NatKit

final class SliceEditResultTests: XCTestCase {
    func testDecoding() throws {
        let json = """
        {
            "id": "slice-1",
            "name": "Write the UI",
            "url": "https://notion.so/slice-1",
            "brief": "New brief text"
        }
        """
        let result = try JSONDecoder().decode(SliceEditResult.self, from: json.data(using: .utf8)!)

        XCTAssertEqual(result.id, "slice-1")
        XCTAssertEqual(result.name, "Write the UI")
        XCTAssertEqual(result.url, "https://notion.so/slice-1")
        XCTAssertEqual(result.brief, "New brief text")
    }

    func testDecodingWithoutURLDefaultsToEmpty() throws {
        let json = """
        {
            "id": "slice-1",
            "name": "Write the UI",
            "brief": "New brief text"
        }
        """
        let result = try JSONDecoder().decode(SliceEditResult.self, from: json.data(using: .utf8)!)

        XCTAssertEqual(result.url, "")
    }

    func testEquality() {
        let a = SliceEditResult(id: "s1", name: "N", url: "https://x", brief: "b")
        let b = SliceEditResult(id: "s1", name: "N", url: "https://x", brief: "b")
        let c = SliceEditResult(id: "s2", name: "N", url: "https://x", brief: "b")
        XCTAssertEqual(a, b)
        XCTAssertNotEqual(a, c)
    }
}
