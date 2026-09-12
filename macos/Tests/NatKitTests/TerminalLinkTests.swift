import XCTest
@testable import NatKit

final class TerminalLinkTests: XCTestCase {
    /// Nothing on disk: a path is only ever resolved because a stub said it
    /// was there, so no test here depends on the machine it runs on.
    private func destination(_ link: String, existing: Set<String> = []) -> URL? {
        TerminalLink.destination(link, fileExists: { existing.contains($0) })
    }

    // MARK: - The schemes a click opens

    func testHTTPSURLOpensAsItself() {
        XCTAssertEqual(destination("https://example.test/pr/1")?.absoluteString, "https://example.test/pr/1")
    }

    func testHTTPURLOpens() {
        XCTAssertEqual(destination("http://example.test")?.scheme, "http")
    }

    func testMailtoOpens() {
        XCTAssertEqual(destination("mailto:someone@example.test")?.scheme, "mailto")
    }

    func testFileURLOpens() {
        XCTAssertEqual(destination("file:///tmp/report.md")?.scheme, "file")
    }

    /// A scheme is a scheme however it was written.
    func testASchemeIsMatchedCaseInsensitively() {
        XCTAssertNotNil(destination("HTTPS://example.test"))
    }

    /// The allowlist is the point: every other scheme an OSC 8 payload or the
    /// emulator's own detector can produce is refused rather than handed to
    /// whichever app registered it.
    func testOtherSchemesAreRefused() {
        for link in [
            "ssh://box.example.test",
            "git://example.test/repo.git",
            "javascript:alert(1)",
            "tel:+441234567890",
            "magnet:?xt=urn:btih:abc",
            "x-apple-something://do-a-thing"
        ] {
            XCTAssertNil(destination(link), "\(link) should not be openable")
        }
    }

    func testTheAllowlistIsTheFourSchemesAnAgentPrints() {
        XCTAssertEqual(TerminalLink.openableSchemes, ["http", "https", "mailto", "file"])
    }

    // MARK: - Paths, which is what a scheme-less payload is

    func testAnAbsolutePathThatExistsOpensAsAFileURL() {
        let url = destination("/tmp/report.md", existing: ["/tmp/report.md"])
        XCTAssertEqual(url?.scheme, "file")
        XCTAssertEqual(url?.path, "/tmp/report.md")
    }

    func testAnAbsolutePathThatIsNotThereOpensNothing() {
        XCTAssertNil(destination("/tmp/gone.md"))
    }

    func testATildeIsExpandedBeforeTheFileIsLookedFor() {
        let expanded = NSHomeDirectory() + "/notes.md"
        XCTAssertEqual(destination("~/notes.md", existing: [expanded])?.path, expanded)
    }

    /// A relative path would be opened against whatever directory the app
    /// happens to be running in, which is never where the agent meant.
    func testARelativePathOpensNothingEvenWhenItExists() {
        XCTAssertNil(destination("internal/tui/app.go", existing: ["internal/tui/app.go"]))
    }

    // MARK: - Nothing at all

    func testAnEmptyLinkOpensNothing() {
        XCTAssertNil(destination(""))
    }

    func testALinkOfNothingButWhitespaceOpensNothing() {
        XCTAssertNil(destination("  \n "))
    }

    /// Whitespace either side of a URL — a line's trailing spaces caught by a
    /// selection — is trimmed rather than making the URL unopenable.
    func testSurroundingWhitespaceIsTrimmed() {
        XCTAssertEqual(destination("  https://example.test  ")?.absoluteString, "https://example.test")
    }

    func testPlainProseOpensNothing() {
        XCTAssertNil(destination("just some words"))
    }

    /// The default lookup is the real file system, which the view bridge uses
    /// and nothing here should depend on: a path nothing could have created
    /// answers nil through it.
    func testTheDefaultLookupAsksTheFileSystem() {
        XCTAssertNil(TerminalLink.destination("/nat-no-such-path-4e9c1f/report.md"))
    }
}
