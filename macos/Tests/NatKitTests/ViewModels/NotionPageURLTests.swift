import XCTest
@testable import NatKit

final class NotionPageURLTests: XCTestCase {
    /// A dashed page ID, as `nat` prints one: Notion resolves the compact
    /// form, so the dashes come out.
    func testForPage_dashedIDLosesItsDashes() {
        let url = NotionPageURL.forPage("3b738308-f654-811c-948d-e1fb36f71df3")

        XCTAssertEqual(url?.absoluteString, "https://www.notion.so/3b738308f654811c948de1fb36f71df3")
    }

    /// One copied out of a URL is already compact and passes through.
    func testForPage_compactIDPassesThrough() {
        let url = NotionPageURL.forPage("3b738308f654811c948de1fb36f71df3")

        XCTAssertEqual(url?.absoluteString, "https://www.notion.so/3b738308f654811c948de1fb36f71df3")
    }

    func testForPage_surroundingWhitespaceIsIgnored() {
        let url = NotionPageURL.forPage("  3b738308f654811c948de1fb36f71df3\n")

        XCTAssertEqual(url?.absoluteString, "https://www.notion.so/3b738308f654811c948de1fb36f71df3")
    }

    /// Nothing to link to: the menu drops the item rather than opening a
    /// search page.
    func testForPage_emptyIsNoURL() {
        XCTAssertNil(NotionPageURL.forPage(""))
        XCTAssertNil(NotionPageURL.forPage("   "))
        XCTAssertNil(NotionPageURL.forPage("-"))
    }

    /// Anything that is not a page ID is not linked either — a fixture's
    /// stand-in, a title, a URL already.
    func testForPage_aNonIDIsNoURL() {
        XCTAssertNil(NotionPageURL.forPage("slice-1"))
        XCTAssertNil(NotionPageURL.forPage("https://notion.so/abc"))
    }
}
