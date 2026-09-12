import XCTest
@testable import NatKit

final class BinaryLocatorTests: XCTestCase {
    func testResolvedPathPrefersThePathsEntries() {
        // A dev run outside any bundle — the one resolution that reaches PATH
        // for nat at all.
        let path = BinaryLocator.resolvedPath(
            for: "nat",
            environment: ["PATH": "/nowhere:/somewhere/bin"],
            fileExists: { $0 == "/somewhere/bin/nat" },
            natResolution: { .searchPath }
        )
        XCTAssertEqual(path, "/somewhere/bin/nat")
    }

    // MARK: - nat reads NatBinary's resolution

    func testNatTakesTheBundledBinaryOverAnythingOnPath() {
        // PATH holds a nat and the bundle holds one: the bundle's is the
        // answer, so the checklist says what the runtime will actually run.
        let status = BinaryLocator.status(
            of: "nat",
            environment: ["PATH": "/somewhere/bin"],
            fileExists: { _ in true },
            natResolution: { .bundled("/Applications/gnat.app/Contents/MacOS/nat") }
        )
        XCTAssertEqual(status, .found("/Applications/gnat.app/Contents/MacOS/nat"))
    }

    func testNatTakesTheOverrideAsWritten() {
        let status = BinaryLocator.status(
            of: "nat",
            environment: ["PATH": "/somewhere/bin"],
            fileExists: { _ in true },
            natResolution: { .override("/dev/nat") }
        )
        XCTAssertEqual(status, .found("/dev/nat"))
    }

    func testNatInADamagedBundleIsNeitherFoundNorSearchedForOnPath() {
        let expected = "/Applications/gnat.app/Contents/MacOS/nat"
        let status = BinaryLocator.status(
            of: "nat",
            environment: ["PATH": "/somewhere/bin"],
            fileExists: { _ in true },
            natResolution: { .damagedInstall(expected: expected) }
        )
        XCTAssertEqual(status, .damagedInstall(expected: expected))
        XCTAssertFalse(status.isFound)
        XCTAssertNil(
            BinaryLocator.resolvedPath(
                for: "nat",
                environment: ["PATH": "/somewhere/bin"],
                fileExists: { _ in true },
                natResolution: { .damagedInstall(expected: expected) }
            )
        )
        XCTAssertFalse(
            BinaryLocator.isFound(
                "nat",
                environment: ["PATH": "/somewhere/bin"],
                fileExists: { _ in true },
                natResolution: { .damagedInstall(expected: expected) }
            )
        )
    }

    func testOtherBinariesIgnoreTheNatResolutionEntirely() {
        // The bundle carries nat and nothing else, so tmux is PATH's question
        // however nat resolved.
        let status = BinaryLocator.status(
            of: "tmux",
            environment: ["PATH": "/usr/bin"],
            fileExists: { $0 == "/usr/bin/tmux" },
            natResolution: { .damagedInstall(expected: "/nope/nat") }
        )
        XCTAssertEqual(status, .found("/usr/bin/tmux"))
    }

    func testStatusIsMissingWhenFoundNowhere() {
        let status = BinaryLocator.status(
            of: "ntn", environment: [:], fileExists: { _ in false })
        XCTAssertEqual(status, .missing)
        XCTAssertFalse(status.isFound)
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
