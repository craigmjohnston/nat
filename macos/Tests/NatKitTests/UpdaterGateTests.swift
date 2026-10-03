import XCTest
@testable import NatKit

final class UpdaterGateTests: XCTestCase {
    func testABundledAppStartsTheUpdater() {
        XCTAssertTrue(UpdaterGate.shouldStart(bundleURL: URL(fileURLWithPath: "/Applications/gnat.app")))
    }

    /// A dev executable's "bundle" is the directory it sits in.
    func testADevExecutableDoesNot() {
        XCTAssertFalse(UpdaterGate.shouldStart(bundleURL: URL(fileURLWithPath: "/repo/macos/.build/debug")))
    }
}
