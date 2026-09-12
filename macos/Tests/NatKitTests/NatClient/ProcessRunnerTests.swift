import XCTest
@testable import NatKit

/// What `nat` the runner actually spawns. Every case drives the resolution
/// through the seam rather than through a real bundle, and spawns a binary
/// that is on every Mac — the point is which path was used, not what it said.
final class ProcessRunnerTests: XCTestCase {

    func testNatRunsTheResolvedBundledBinaryByAbsolutePath() async throws {
        // PATH is deliberately a directory holding nothing: a packaged app's
        // nat is found beside the app whatever PATH says.
        let previous = PathBootstrap.environmentValue("PATH")
        setenv("PATH", "/nowhere", 1)
        defer { setenv("PATH", previous ?? "", 1) }

        let runner = ProcessRunner(natResolution: { .bundled("/bin/echo") })
        let result = try await runner.run(
            executable: "nat", arguments: ["bundled"], workingDirectory: nil, standardInput: nil)

        XCTAssertEqual(result.exitCode, 0)
        XCTAssertEqual(String(decoding: result.stdout), "bundled\n")
    }

    func testNatRunsTheOverrideWhenOneIsResolved() async throws {
        let runner = ProcessRunner(natResolution: { .override("/bin/echo") })
        let result = try await runner.run(
            executable: "nat", arguments: ["override"], workingDirectory: nil, standardInput: nil)

        XCTAssertEqual(String(decoding: result.stdout), "override\n")
    }

    func testADamagedInstallRefusesToRunAnotherNat() async {
        // The refusal is the point: a nat on PATH is exactly what must not be
        // reached from a bundle that lost its own.
        let previous = PathBootstrap.environmentValue("PATH")
        setenv("PATH", "/bin", 1)
        defer { setenv("PATH", previous ?? "", 1) }

        let expected = "/Applications/gnat.app/Contents/MacOS/nat"
        let runner = ProcessRunner(natResolution: { .damagedInstall(expected: expected) })
        do {
            _ = try await runner.run(
                executable: "nat", arguments: [], workingDirectory: nil, standardInput: nil)
            XCTFail("a damaged install should refuse to run any nat")
        } catch let error as NatError {
            guard case .bundledBinaryMissing(let path) = error else {
                return XCTFail("expected bundledBinaryMissing, got \(error)")
            }
            XCTAssertEqual(path, expected)
            XCTAssertEqual(
                error.errorDescription,
                "This copy of gnat is missing the nat it was built with "
                    + "(expected at \(expected)). Reinstall gnat: running another "
                    + "nat would be a version the app was never built against.")
        } catch {
            XCTFail("expected a NatError, got \(error)")
        }
    }

    func testADevRunFallsThroughToPath() async throws {
        let previous = PathBootstrap.environmentValue("PATH")
        setenv("PATH", "/nowhere:\(pathDir)", 1)
        defer { setenv("PATH", previous ?? "", 1) }

        let runner = ProcessRunner(natResolution: { .searchPath })
        let result = try await runner.run(
            executable: natOnPath, arguments: ["from-path"], workingDirectory: nil,
            standardInput: nil)

        XCTAssertEqual(String(decoding: result.stdout), "from-path\n")
    }

    func testAnAbsoluteExecutableIsRunAsGiven() async throws {
        // Nothing is resolved for a path that is already one — nat included,
        // which is what every caller past the first resolution hands over.
        let runner = ProcessRunner(natResolution: { .damagedInstall(expected: "/nope") })
        let result = try await runner.run(
            executable: "/bin/echo", arguments: ["absolute"], workingDirectory: nil,
            standardInput: nil)

        XCTAssertEqual(String(decoding: result.stdout), "absolute\n")
    }

    func testAnotherToolIsUnaffectedByTheNatResolution() async throws {
        let previous = PathBootstrap.environmentValue("PATH")
        setenv("PATH", "/bin", 1)
        defer { setenv("PATH", previous ?? "", 1) }

        let runner = ProcessRunner(natResolution: { .damagedInstall(expected: "/nope/nat") })
        let result = try await runner.run(
            executable: "echo", arguments: ["tool"], workingDirectory: nil, standardInput: nil)

        XCTAssertEqual(String(decoding: result.stdout), "tool\n")
    }

    /// `echo` under a name nat is not, so a PATH search is what has to have
    /// found it.
    private let natOnPath = "echo"
    private var pathDir: String { "/bin" }
}

private extension String {
    init(decoding data: Data) {
        self = String(data: data, encoding: .utf8) ?? ""
    }
}
