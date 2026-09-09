import AppKit
import XCTest
@testable import NatKit

final class TitlebarDoubleClickTests: XCTestCase {
    func testMinimizeSettingMinimizes() {
        XCTAssertEqual(TitlebarDoubleClick.action(for: "Minimize"), .minimize)
    }

    func testNoneSettingDoesNothing() {
        XCTAssertEqual(TitlebarDoubleClick.action(for: "None"), .none)
    }

    /// Zoom is the answer for the unset default, for "Maximize", and for
    /// any value this build does not know — "Fill" included, which is the
    /// system's newer name for much the same thing.
    func testEverythingElseZooms() {
        XCTAssertEqual(TitlebarDoubleClick.action(for: nil), .zoom)
        XCTAssertEqual(TitlebarDoubleClick.action(for: "Maximize"), .zoom)
        XCTAssertEqual(TitlebarDoubleClick.action(for: "Fill"), .zoom)
    }

    @MainActor
    func testPerformOnNoWindowIsANoOp() {
        TitlebarDoubleClick.perform(on: nil)
    }
}
