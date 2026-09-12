import XCTest
@testable import NatKit

final class TerminalDropTextTests: XCTestCase {
    // MARK: - One path

    /// The characters a path is ordinarily made of go through untouched, so
    /// the common drop types exactly the path the Finder would show.
    func testAnOrdinaryPathIsTypedAsItIs() {
        XCTAssertEqual(
            TerminalDropText.escape("/Users/craig/Projects/nat/docs/design-v2.1_final.png"),
            "/Users/craig/Projects/nat/docs/design-v2.1_final.png"
        )
    }

    /// A space is why any of this escaping exists: unescaped, one dropped file
    /// reads as several words.
    func testSpacesAreEscaped() {
        XCTAssertEqual(
            TerminalDropText.escape("/Users/craig/Screen Shot 1.png"),
            "/Users/craig/Screen\\ Shot\\ 1.png"
        )
    }

    func testTheOtherCharactersAShellWouldReadAreEscaped() {
        XCTAssertEqual(TerminalDropText.escape("/tmp/a'b\"c.png"), "/tmp/a\\'b\\\"c.png")
        XCTAssertEqual(TerminalDropText.escape("/tmp/(1).png"), "/tmp/\\(1\\).png")
        XCTAssertEqual(TerminalDropText.escape("/tmp/a*b?.png"), "/tmp/a\\*b\\?.png")
        XCTAssertEqual(TerminalDropText.escape("/tmp/a$b&c.png"), "/tmp/a\\$b\\&c.png")
        XCTAssertEqual(TerminalDropText.escape("/tmp/back\\slash"), "/tmp/back\\\\slash")
    }

    /// A name outside ASCII takes a backslash under the same rule, which is
    /// harmless — a shell reads `\é` as `é` — and keeps the rule one line.
    func testACharacterOutsideTheSafeSetIsEscapedWhateverItIs() {
        XCTAssertEqual(TerminalDropText.escape("/tmp/café.png"), "/tmp/caf\\é.png")
    }

    // MARK: - What a drop types

    func testOnePathIsTypedWithATrailingSpace() {
        XCTAssertEqual(TerminalDropText.text(forPaths: ["/tmp/a.png"]), "/tmp/a.png ")
    }

    func testSeveralPathsAreSeparatedBySpaces() {
        XCTAssertEqual(
            TerminalDropText.text(forPaths: ["/tmp/a.png", "/tmp/b c.png"]),
            "/tmp/a.png /tmp/b\\ c.png "
        )
    }

    /// A drop that named no file types nothing at all — not even the trailing
    /// space, which would land in the composer as a stray character.
    func testADropOfNoFilesTypesNothing() {
        XCTAssertEqual(TerminalDropText.text(forPaths: []), "")
    }

    func testAnEmptyPathIsNotAFile() {
        XCTAssertEqual(TerminalDropText.text(forPaths: ["", "/tmp/a.png", ""]), "/tmp/a.png ")
    }

    func testADropOfNothingButEmptyPathsTypesNothing() {
        XCTAssertEqual(TerminalDropText.text(forPaths: ["", ""]), "")
    }
}
