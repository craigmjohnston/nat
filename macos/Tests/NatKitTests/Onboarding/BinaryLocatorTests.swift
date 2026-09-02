import XCTest
@testable import NatKit

final class BinaryLocatorTests: XCTestCase {
    func testResolvedPathPrefersThePathsEntries() {
        let path = BinaryLocator.resolvedPath(
            for: "nat",
            environment: ["PATH": "/nowhere:/somewhere/bin"],
            fileExists: { $0 == "/somewhere/bin/nat" }
        )
        XCTAssertEqual(path, "/somewhere/bin/nat")
    }

    func testResolvedPathFallsBackToKnownInstallLocations() {
        let path = BinaryLocator.resolvedPath(
            for: "gh",
            environment: ["PATH": "/nowhere"],
            fileExists: { $0 == "/opt/homebrew/bin/gh" }
        )
        XCTAssertEqual(path, "/opt/homebrew/bin/gh")
    }

    func testResolvedPathReturnsNilWhenFoundNowhere() {
        let path = BinaryLocator.resolvedPath(for: "ntn", environment: [:], fileExists: { _ in false })
        XCTAssertNil(path)
    }

    func testIsFoundTrue() {
        XCTAssertTrue(
            BinaryLocator.isFound("tmux", environment: ["PATH": "/usr/bin"], fileExists: { $0 == "/usr/bin/tmux" })
        )
    }

    func testIsFoundFalse() {
        XCTAssertFalse(
            BinaryLocator.isFound("tmux", environment: [:], fileExists: { _ in false })
        )
    }
}
