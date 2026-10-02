import XCTest
@testable import NatKit

final class ThinScrollersTests: XCTestCase {
    func testContentThatFitsHasNoKnob() {
        XCTAssertNil(ScrollKnob(visible: 200, content: 200, scrolled: 0))
        XCTAssertNil(ScrollKnob(visible: 200, content: 120, scrolled: 0))
        XCTAssertNil(ScrollKnob(visible: 0, content: 120, scrolled: 0))
    }

    func testTheKnobIsTheVisibleShareOfTheTrack() {
        let knob = try! XCTUnwrap(ScrollKnob(visible: 200, content: 400, scrolled: 0))
        XCTAssertEqual(knob.length, 98)   // (200 - 4) * 200/400
        XCTAssertEqual(knob.offset, ScrollKnob.inset)
    }

    func testItRunsTheTrackAsTheContentScrolls() {
        let end = try! XCTUnwrap(ScrollKnob(visible: 200, content: 400, scrolled: 200))
        XCTAssertEqual(end.offset + end.length, 200 - ScrollKnob.inset)
        let past = try! XCTUnwrap(ScrollKnob(visible: 200, content: 400, scrolled: 900))
        XCTAssertEqual(past, end, "an overscroll pins it to the end")
        let before = try! XCTUnwrap(ScrollKnob(visible: 200, content: 400, scrolled: -50))
        XCTAssertEqual(before.offset, ScrollKnob.inset)
    }

    func testAVeryLongListStillHasAGraspableKnob() {
        XCTAssertEqual(ScrollKnob(visible: 200, content: 100_000, scrolled: 0)?.length, ScrollKnob.minLength)
    }

    func testADragMovesTheContentByTheKnobsTravel() {
        let knob = try! XCTUnwrap(ScrollKnob(visible: 200, content: 400, scrolled: 0))
        // The knob travels 196 - 98 = 98pt over 200pt of content.
        XCTAssertEqual(
            ScrollKnob.contentDistance(forDrag: 49, visible: 200, content: 400, knob: knob), 100, accuracy: 0.001)
    }

    func testAKnobWithNowhereToGoMovesNothing() {
        let knob = try! XCTUnwrap(ScrollKnob(visible: 20, content: 10_000, scrolled: 0))
        XCTAssertEqual(ScrollKnob.contentDistance(forDrag: 10, visible: 20, content: 10_000, knob: knob), 0)
    }

    /// Every scroll in the app carries the app's own scroll bar — or, like
    /// the chip picker's, none at all — so AppKit's never draws.
    func testEveryScrollViewHasThinScrollers() throws {
        let views = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent()
            .appendingPathComponent("Sources/NatApp")
        let files = FileManager.default.enumerator(at: views, includingPropertiesForKeys: nil)!
            .compactMap { $0 as? URL }.filter { $0.pathExtension == "swift" }
        XCTAssertFalse(files.isEmpty)
        for file in files {
            let source = try String(contentsOf: file, encoding: .utf8)
            let scrolls = source.components(separatedBy: "ScrollView(").count - 1
                + source.components(separatedBy: "ScrollView {").count - 1
                - source.components(separatedBy: "NSScrollView").count + 1
            let covered = source.components(separatedBy: ".thinScrollers(").count - 1
                + source.components(separatedBy: "showsIndicators: false").count - 1
            XCTAssertGreaterThanOrEqual(covered, scrolls, "\(file.lastPathComponent): a ScrollView without thinScrollers()")
        }
    }
}
