import XCTest
@testable import NatKit

final class GalleryCommandTests: XCTestCase {
    /// argv's first element is the executable and is never a flag, so every
    /// case below spells it out rather than passing flags alone.
    private func parse(_ args: String...) throws -> GalleryCommand? {
        try GalleryCommand.parse(["gnat"] + args)
    }

    // MARK: - The app, launched

    func testNoArgumentsIsNotAGalleryRun() throws {
        XCTAssertNil(try parse())
    }

    /// AppKit and Xcode both pass arguments of their own to a launched app.
    /// A line naming no gallery flag is the app's, whatever else is on it.
    func testArgumentsTheGalleryDoesNotKnowAreNotItsBusiness() throws {
        XCTAssertNil(try parse("-NSDocumentRevisionsDebugMode", "YES"))
    }

    // MARK: - The three commands

    func testList() throws {
        XCTAssertEqual(try parse("--list"), .list)
    }

    func testOneStory() throws {
        XCTAssertEqual(
            try parse("--story", "window-shell", "--out", "/tmp/a.png"),
            .one(story: "window-shell", out: "/tmp/a.png"))
    }

    /// The flags commute: the parse is a scan rather than a shape.
    func testOneStoryWithTheFlagsTheOtherWayRound() throws {
        XCTAssertEqual(
            try parse("--out", "/tmp/a.png", "--story", "window-shell"),
            .one(story: "window-shell", out: "/tmp/a.png"))
    }

    func testAll() throws {
        XCTAssertEqual(try parse("--all", "--out", "/tmp/gallery"), .all(directory: "/tmp/gallery"))
    }

    // MARK: - Refusals

    func testStoryWithNoValue() {
        assertRefused(try parse("--story"), .missingValue(flag: "--story"))
    }

    /// A flag after `--story` is another flag and not a story name — taking
    /// it as one would render nothing and write a file called `--out`.
    func testStoryFollowedByAnotherFlag() {
        assertRefused(try parse("--story", "--out", "/tmp/a.png"), .missingValue(flag: "--story"))
    }

    func testOutWithNoValue() {
        assertRefused(try parse("--story", "a", "--out"), .missingValue(flag: "--out"))
    }

    func testStoryAndAllTogether() {
        assertRefused(try parse("--all", "--story", "a", "--out", "/tmp"), .storyAndAll)
    }

    func testListWithAll() {
        assertRefused(try parse("--list", "--all"), .listWithOthers)
    }

    func testListWithStory() {
        assertRefused(try parse("--list", "--story", "a"), .listWithOthers)
    }

    func testListWithOut() {
        assertRefused(try parse("--list", "--out", "/tmp/a.png"), .listWithOthers)
    }

    func testStoryWithNoOut() {
        assertRefused(try parse("--story", "window-shell"), .missingOut)
    }

    func testAllWithNoOut() {
        assertRefused(try parse("--all"), .missingOut)
    }

    func testOutWithNothingToWrite() {
        assertRefused(try parse("--out", "/tmp/a.png"), .outWithoutTarget)
    }

    func testUnknownArgumentOnAGalleryLine() {
        assertRefused(try parse("--list", "--verbose"), .unknown(argument: "--verbose"))
    }

    // MARK: - What a refusal says

    /// Every refusal is printed rather than matched on, so each one has to
    /// say something. The exact wording is not the contract; that none is
    /// empty, and that no two read the same, is.
    func testEveryRefusalSaysSomethingOfItsOwn() {
        let errors: [GalleryCommandError] = [
            .missingValue(flag: "--story"), .storyAndAll, .listWithOthers,
            .missingOut, .outWithoutTarget, .unknown(argument: "--verbose"),
        ]
        let said = errors.map(\.description)
        for message in said {
            XCTAssertFalse(message.isEmpty)
        }
        XCTAssertEqual(Set(said).count, said.count, "two refusals read the same")
    }

    private func assertRefused(
        _ expression: @autoclosure () throws -> GalleryCommand?,
        _ expected: GalleryCommandError,
        file: StaticString = #filePath, line: UInt = #line
    ) {
        do {
            let command = try expression()
            XCTFail("expected \(expected), parsed \(String(describing: command))", file: file, line: line)
        } catch let error as GalleryCommandError {
            XCTAssertEqual(error, expected, file: file, line: line)
        } catch {
            XCTFail("unexpected error \(error)", file: file, line: line)
        }
    }
}
