import XCTest
@testable import NatKit

final class ScratchEmptyNoteTests: XCTestCase {
    func testTheNoteReadsAsOneSentenceWithBothLinks() {
        XCTAssertEqual(
            String(ScratchEmptyNote.markdown.characters),
            "No tasks. Use a workshop agent or add one yourself.")
        let links = ScratchEmptyNote.markdown.runs.compactMap(\.link).compactMap(ScratchEmptyNote.Link.init)
        XCTAssertEqual(links, [.workshop, .addSlice])
    }

    func testAnyOtherURLIsNotALink() {
        XCTAssertNil(ScratchEmptyNote.Link(URL(string: "https://example.com")!))
        XCTAssertNil(ScratchEmptyNote.Link(URL(string: "gnat-scratch:elsewhere")!))
    }
}
