import XCTest
@testable import NatKit

final class PathBootstrapTests: XCTestCase {

    // MARK: - loginPath(fromEnvListing:)

    func testLoginPathReadsThePATHLine() {
        let listing = "HOME=/Users/x\nPATH=/opt/homebrew/bin:/usr/bin\nSHELL=/bin/zsh\n"
        XCTAssertEqual(PathBootstrap.loginPath(fromEnvListing: listing), "/opt/homebrew/bin:/usr/bin")
    }

    func testLoginPathTakesTheLastPATHLine() {
        // A profile that prints something PATH-shaped is overtaken by env's
        // own line, which runs after every profile and so comes last.
        let listing = "PATH=/spoofed\nOTHER=1\nPATH=/opt/homebrew/bin:/usr/bin\n"
        XCTAssertEqual(PathBootstrap.loginPath(fromEnvListing: listing), "/opt/homebrew/bin:/usr/bin")
    }

    func testLoginPathIgnoresNoiseAndPrefixLookalikes() {
        // "NOT_PATH=" and a greeting are not PATH lines.
        let listing = "welcome to zsh\nNOT_PATH=/nope\nPATH=/usr/bin\n"
        XCTAssertEqual(PathBootstrap.loginPath(fromEnvListing: listing), "/usr/bin")
    }

    func testLoginPathIsNilWithNoPATHLine() {
        XCTAssertNil(PathBootstrap.loginPath(fromEnvListing: "HOME=/Users/x\n"))
    }

    func testLoginPathIsNilForAnEmptyValue() {
        XCTAssertNil(PathBootstrap.loginPath(fromEnvListing: "PATH=\n"))
    }

    // MARK: - composed(bundledDir:loginPath:current:)

    func testComposedPutsTheBundleFirstThenLoginThenCurrent() {
        XCTAssertEqual(
            PathBootstrap.composed(
                bundledDir: "/Applications/gnat.app/Contents/MacOS",
                loginPath: "/opt/homebrew/bin:/Users/x/go/bin",
                current: "/usr/bin:/bin"),
            "/Applications/gnat.app/Contents/MacOS:/opt/homebrew/bin:/Users/x/go/bin:/usr/bin:/bin")
    }

    func testComposedDeduplicatesKeepingTheFirstOccurrence() {
        XCTAssertEqual(
            PathBootstrap.composed(
                bundledDir: "/bundle",
                loginPath: "/usr/bin:/bundle:/opt/homebrew/bin",
                current: "/usr/bin:/bin"),
            "/bundle:/usr/bin:/opt/homebrew/bin:/bin")
    }

    func testComposedWithNoBundleIsLoginThenCurrent() {
        XCTAssertEqual(
            PathBootstrap.composed(bundledDir: nil, loginPath: "/opt/homebrew/bin", current: "/usr/bin"),
            "/opt/homebrew/bin:/usr/bin")
    }

    func testComposedWithOnlyTheCurrentPathKeepsIt() {
        // A shell that answered nothing changes nothing: launchd's own PATH
        // survives as it was.
        XCTAssertEqual(
            PathBootstrap.composed(bundledDir: nil, loginPath: nil, current: "/usr/bin:/bin"),
            "/usr/bin:/bin")
    }

    func testComposedDropsEmptySegments() {
        XCTAssertEqual(
            PathBootstrap.composed(bundledDir: nil, loginPath: "::/usr/bin:", current: nil),
            "/usr/bin")
    }

    func testComposedIsNilWithNothingToSay() {
        XCTAssertNil(PathBootstrap.composed(bundledDir: nil, loginPath: nil, current: nil))
        XCTAssertNil(PathBootstrap.composed(bundledDir: nil, loginPath: "", current: ""))
    }

    // MARK: - bundledNatDir

    func testBundledNatDirIsTheExecutablesDirectoryWhenNatSitsBesideIt() {
        let dir = PathBootstrap.bundledNatDir(
            executableURL: URL(fileURLWithPath: "/Applications/gnat.app/Contents/MacOS/gnat"),
            isExecutableFile: { $0 == "/Applications/gnat.app/Contents/MacOS/nat" })
        XCTAssertEqual(dir, "/Applications/gnat.app/Contents/MacOS")
    }

    func testBundledNatDirIsNilWhenTheBundleCarriesNoNat() {
        // The dev run's bare executable sits beside no nat.
        XCTAssertNil(PathBootstrap.bundledNatDir(
            executableURL: URL(fileURLWithPath: "/repo/macos/.build/debug/gnat"),
            isExecutableFile: { _ in false }))
    }

    func testBundledNatDirIsNilWithNoExecutableURL() {
        XCTAssertNil(PathBootstrap.bundledNatDir(executableURL: nil, isExecutableFile: { _ in true }))
    }

    // MARK: - bootstrap

    func testBootstrapComposesFromTheLoginShellAndApplies() {
        var askedShell: String?
        var applied: String?
        PathBootstrap.bootstrap(
            bundledDir: "/bundle",
            shell: "/bin/zsh",
            current: "/usr/bin",
            loginListing: { shell in
                askedShell = shell
                return "PATH=/opt/homebrew/bin\n"
            },
            apply: { applied = $0 })
        XCTAssertEqual(askedShell, "/bin/zsh")
        XCTAssertEqual(applied, "/bundle:/opt/homebrew/bin:/usr/bin")
    }

    func testBootstrapWithNoShellStillAppliesTheRest() {
        var applied: String?
        PathBootstrap.bootstrap(
            bundledDir: "/bundle",
            shell: nil,
            current: "/usr/bin",
            loginListing: { _ in XCTFail("no shell to ask"); return nil },
            apply: { applied = $0 })
        XCTAssertEqual(applied, "/bundle:/usr/bin")
    }

    func testBootstrapWithASilentShellFallsBackToWhatItHad() {
        var applied: String?
        PathBootstrap.bootstrap(
            bundledDir: nil,
            shell: "/bin/zsh",
            current: "/usr/bin:/bin",
            loginListing: { _ in nil },
            apply: { applied = $0 })
        XCTAssertEqual(applied, "/usr/bin:/bin")
    }

    func testBootstrapWithNothingToSaySetsNothing() {
        PathBootstrap.bootstrap(
            bundledDir: nil,
            shell: nil,
            current: nil,
            loginListing: { _ in nil },
            apply: { _ in XCTFail("nothing to apply") })
    }

    // MARK: - loginShellEnvListing (a real process, /bin/sh kept tiny)

    func testLoginShellEnvListingRunsTheShellForItsEnvironment() throws {
        let listing = try XCTUnwrap(PathBootstrap.loginShellEnvListing(shell: "/bin/sh"))
        XCTAssertNotNil(PathBootstrap.loginPath(fromEnvListing: listing))
    }

    func testLoginShellEnvListingIsNilForAShellThatCannotRun() {
        XCTAssertNil(PathBootstrap.loginShellEnvListing(shell: "/nonexistent/shell"))
    }

    // MARK: - environmentValue

    func testEnvironmentValueReadsSetenvLive() {
        setenv("NAT_PATH_BOOTSTRAP_TEST", "live", 1)
        defer { unsetenv("NAT_PATH_BOOTSTRAP_TEST") }
        XCTAssertEqual(PathBootstrap.environmentValue("NAT_PATH_BOOTSTRAP_TEST"), "live")
    }

    func testEnvironmentValueIsNilWhenUnset() {
        unsetenv("NAT_PATH_BOOTSTRAP_TEST_UNSET")
        XCTAssertNil(PathBootstrap.environmentValue("NAT_PATH_BOOTSTRAP_TEST_UNSET"))
    }
}
