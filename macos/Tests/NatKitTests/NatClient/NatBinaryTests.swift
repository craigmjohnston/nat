import XCTest
@testable import NatKit

final class NatBinaryTests: XCTestCase {

    private let appExecutable = URL(fileURLWithPath: "/Applications/gnat.app/Contents/MacOS/gnat")
    private let appBundle = URL(fileURLWithPath: "/Applications/gnat.app")
    private var bundledNat: String { "/Applications/gnat.app/Contents/MacOS/nat" }

    // MARK: - NAT_BIN outranks everything

    func testOverrideWinsOverTheBundledBinary() {
        XCTAssertEqual(
            NatBinary.resolve(
                override: "/Users/x/dev/nat",
                executableURL: appExecutable,
                bundleURL: appBundle,
                isExecutableFile: { _ in true }),
            .override("/Users/x/dev/nat"))
    }

    func testOverrideWinsInADamagedBundleToo() {
        // The override is the dev saying which nat to run, so it answers
        // before the bundle is looked at at all.
        XCTAssertEqual(
            NatBinary.resolve(
                override: "/Users/x/dev/nat",
                executableURL: appExecutable,
                bundleURL: appBundle,
                isExecutableFile: { _ in false }),
            .override("/Users/x/dev/nat"))
    }

    func testAnEmptyOverrideSaysNothing() {
        // An unset variable and one set to nothing are the same absence.
        XCTAssertEqual(
            NatBinary.resolve(
                override: "",
                executableURL: appExecutable,
                bundleURL: appBundle,
                isExecutableFile: { $0 == self.bundledNat }),
            .bundled(bundledNat))
    }

    // MARK: - The bundled binary, by absolute path

    func testThePackagedAppTakesTheBinaryBesideIt() {
        XCTAssertEqual(
            NatBinary.resolve(
                override: nil,
                executableURL: appExecutable,
                bundleURL: appBundle,
                isExecutableFile: { $0 == self.bundledNat }),
            .bundled(bundledNat))
    }

    func testADevBuildSittingBesideANatTakesItToo() {
        // Nothing about the bundle decides this: a binary beside the
        // executable is the one that was built with it wherever it runs.
        XCTAssertEqual(
            NatBinary.resolve(
                override: nil,
                executableURL: URL(fileURLWithPath: "/repo/.build/debug/gnat"),
                bundleURL: URL(fileURLWithPath: "/repo/.build/debug"),
                isExecutableFile: { $0 == "/repo/.build/debug/nat" }),
            .bundled("/repo/.build/debug/nat"))
    }

    // MARK: - A packaged app with no nat is broken, not a PATH search

    func testAPackagedAppMissingItsNatIsADamagedInstall() {
        XCTAssertEqual(
            NatBinary.resolve(
                override: nil,
                executableURL: appExecutable,
                bundleURL: appBundle,
                isExecutableFile: { _ in false }),
            .damagedInstall(expected: bundledNat))
    }

    // MARK: - Only a bare executable falls through to PATH

    func testABareExecutableOutsideAnyBundleSearchesPath() {
        XCTAssertEqual(
            NatBinary.resolve(
                override: nil,
                executableURL: URL(fileURLWithPath: "/repo/.build/debug/gnat"),
                bundleURL: URL(fileURLWithPath: "/repo/.build/debug"),
                isExecutableFile: { _ in false }),
            .searchPath)
    }

    func testAProcessThatCannotSayWhereItRunsFromSearchesPath() {
        XCTAssertEqual(
            NatBinary.resolve(
                override: nil,
                executableURL: nil,
                bundleURL: appBundle,
                isExecutableFile: { _ in false }),
            .searchPath)
    }

    func testNoBundleURLIsNoBundleToBeDamaged() {
        XCTAssertEqual(
            NatBinary.resolve(
                override: nil,
                executableURL: appExecutable,
                bundleURL: nil,
                isExecutableFile: { _ in false }),
            .searchPath)
    }
}
