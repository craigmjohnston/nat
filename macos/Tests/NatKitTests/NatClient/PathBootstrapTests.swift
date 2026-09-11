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

    func testComposedPutsFallbacksLastAndDeduplicates() {
        // A fallback the shell already named is not repeated, and one it did
        // not goes after everything real — a real PATH entry always wins.
        XCTAssertEqual(
            PathBootstrap.composed(
                bundledDir: nil,
                loginPath: "/opt/homebrew/bin:/Users/x/go/bin",
                current: "/usr/bin",
                fallbacks: ["/opt/homebrew/bin", "/Users/x/.local/bin"]),
            "/opt/homebrew/bin:/Users/x/go/bin:/usr/bin:/Users/x/.local/bin")
    }

    func testComposedWithOnlyFallbacksStillComposes() {
        // Even a process with no PATH at all gets the floor.
        XCTAssertEqual(
            PathBootstrap.composed(
                bundledDir: nil, loginPath: nil, current: nil, fallbacks: ["/opt/homebrew/bin"]),
            "/opt/homebrew/bin")
    }

    // MARK: - wellKnownDirs

    func testWellKnownDirsAreTheInstallLocationsUnderTheGivenHome() {
        XCTAssertEqual(
            PathBootstrap.wellKnownDirs(home: "/Users/x"),
            ["/opt/homebrew/bin", "/usr/local/bin", "/Users/x/.local/bin", "/Users/x/go/bin"])
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
            fallbacks: [],
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
            fallbacks: [],
            loginListing: { _ in XCTFail("no shell to ask"); return nil },
            inBackground: { _ in XCTFail("no shell to retry either") },
            apply: { applied = $0 })
        XCTAssertEqual(applied, "/bundle:/usr/bin")
    }

    func testBootstrapWithASilentShellStillHasTheFloor() {
        // The regression this floor exists for: a login shell that timed out
        // at launch used to leave launchd's bare PATH standing for the whole
        // process lifetime, and every nat it spawned unable to find ntn. The
        // retry is scheduled but deliberately left unrun here — the floor
        // must stand before it, not wait for it.
        var applied: [String] = []
        var scheduled = 0
        PathBootstrap.bootstrap(
            bundledDir: nil,
            shell: "/bin/zsh",
            current: "/usr/bin:/bin",
            fallbacks: ["/opt/homebrew/bin", "/Users/x/.local/bin"],
            loginListing: { _ in nil },
            retryListing: { _ in nil },
            inBackground: { _ in scheduled += 1 },
            apply: { applied.append($0) })
        XCTAssertEqual(applied, ["/usr/bin:/bin:/opt/homebrew/bin:/Users/x/.local/bin"])
        XCTAssertEqual(scheduled, 1)
    }

    func testBootstrapRetriesASilentShellAndAppliesTheLateAnswer() {
        var applied: [String] = []
        var work: (() -> Void)?
        PathBootstrap.bootstrap(
            bundledDir: "/bundle",
            shell: "/bin/zsh",
            current: "/usr/bin",
            fallbacks: ["/opt/homebrew/bin"],
            loginListing: { _ in nil },
            retryListing: { shell in
                XCTAssertEqual(shell, "/bin/zsh")
                return "PATH=/opt/homebrew/bin:/Users/x/.rvm/bin\n"
            },
            inBackground: { work = $0 },
            apply: { applied.append($0) })
        // The floor stands the moment bootstrap returns; the late answer
        // re-composes in the documented order — bundle, login, current,
        // fallbacks — with the fallback the shell also named said once.
        XCTAssertEqual(applied, ["/bundle:/usr/bin:/opt/homebrew/bin"])
        work?()
        XCTAssertEqual(applied, [
            "/bundle:/usr/bin:/opt/homebrew/bin",
            "/bundle:/opt/homebrew/bin:/Users/x/.rvm/bin:/usr/bin",
        ])
    }

    func testBootstrapRetryThatHearsNothingAppliesNothingMore() {
        var applyCount = 0
        var work: (() -> Void)?
        PathBootstrap.bootstrap(
            bundledDir: nil,
            shell: "/bin/zsh",
            current: "/usr/bin",
            fallbacks: [],
            loginListing: { _ in nil },
            retryListing: { _ in nil },
            inBackground: { work = $0 },
            apply: { _ in applyCount += 1 })
        XCTAssertEqual(applyCount, 1)
        work?()
        XCTAssertEqual(applyCount, 1)
    }

    func testBootstrapWithAnAnsweredShellSchedulesNoRetry() {
        PathBootstrap.bootstrap(
            bundledDir: nil,
            shell: "/bin/zsh",
            current: "/usr/bin",
            fallbacks: [],
            loginListing: { _ in "PATH=/opt/homebrew/bin\n" },
            retryListing: { _ in XCTFail("the shell answered"); return nil },
            inBackground: { _ in XCTFail("nothing to schedule") },
            apply: { _ in })
    }

    func testBootstrapDoesNotRetryAListingWithNoPATHLine() {
        // A listing without a PATH line is still the shell's answer, and
        // asking again would only hear it again.
        PathBootstrap.bootstrap(
            bundledDir: nil,
            shell: "/bin/zsh",
            current: "/usr/bin",
            fallbacks: [],
            loginListing: { _ in "HOME=/Users/x\n" },
            inBackground: { _ in XCTFail("the shell answered, PATH line or not") },
            apply: { _ in })
    }

    func testBootstrapWithNothingToSaySetsNothing() {
        PathBootstrap.bootstrap(
            bundledDir: nil,
            shell: nil,
            current: nil,
            fallbacks: [],
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

    func testLoginShellEnvListingIsNilForAShellStillGoingAtTheTimeout() throws {
        // A stand-in shell that sleeps past the cap: the wait gives up and
        // the listing is nil rather than late.
        let script = FileManager.default.temporaryDirectory
            .appendingPathComponent("nat-slow-shell-\(UUID().uuidString).sh")
        try "#!/bin/sh\nsleep 5\n".write(to: script, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes(
            [.posixPermissions: 0o755], ofItemAtPath: script.path)
        defer { try? FileManager.default.removeItem(at: script) }
        XCTAssertNil(PathBootstrap.loginShellEnvListing(shell: script.path, timeout: 0.2))
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
