import SwiftUI
import XCTest
@testable import NatKit

final class MarkdownTests: XCTestCase {
    private func rendered(_ text: String) -> String {
        String(markdownAttributed(text, size: 14).characters)
    }

    // MARK: - Newlines survive

    func testParagraphsKeepTheirBlankLine() {
        XCTAssertEqual(rendered("One.\n\nTwo."), "One.\n\nTwo.")
    }

    func testASingleNewlineIsALineBreakNotASpace() {
        XCTAssertEqual(rendered("first line\nsecond line"), "first line\nsecond line")
    }

    // MARK: - Blocks

    func testBulletMarkersBecomeBullets() {
        XCTAssertEqual(rendered("- one\n- two"), "• one\n• two")
        XCTAssertEqual(rendered("* star\n+ plus"), "• star\n• plus")
    }

    func testNestedBulletsKeepTheirIndent() {
        XCTAssertEqual(rendered("- outer\n  - inner"), "• outer\n  • inner")
    }

    func testOrderedItemsKeepTheirOwnNumbers() {
        XCTAssertEqual(rendered("1. first\n2. second"), "1. first\n2. second")
    }

    func testADashWithNoSpaceIsNotABullet() {
        XCTAssertEqual(rendered("-not a list"), "-not a list")
    }

    func testHeadingLosesItsHashesAndGainsItsType() throws {
        let attr = markdownAttributed("## Section\n\nBody.", size: 14)
        XCTAssertEqual(String(attr.characters), "Section\n\nBody.")
        let headingRun = try XCTUnwrap(attr.runs.first)
        XCTAssertEqual(headingRun.font, .system(size: 16, weight: .semibold))
    }

    func testHeadingLevelsRampDown() throws {
        let h1 = try XCTUnwrap(markdownAttributed("# Title", size: 14).runs.first)
        XCTAssertEqual(h1.font, .system(size: 18, weight: .semibold))
        let h3 = try XCTUnwrap(markdownAttributed("### Deep", size: 14).runs.first)
        XCTAssertEqual(h3.font, .system(size: 15, weight: .semibold))
    }

    func testSevenHashesIsNotAHeading() {
        XCTAssertEqual(rendered("####### too deep"), "####### too deep")
    }

    func testHashesWithoutASpaceAreNotAHeading() {
        XCTAssertEqual(rendered("#tag"), "#tag")
    }

    func testCodeFenceContentIsVerbatimAndMonospaced() throws {
        let attr = markdownAttributed("```go\nfunc main() {}\n\t**not bold**\n```", size: 14)
        XCTAssertEqual(String(attr.characters), "func main() {}\n\t**not bold**")
        let run = try XCTUnwrap(attr.runs.first)
        XCTAssertEqual(run.font, Typo.mono(size: 13))
    }

    /// An inline code span takes the app's own face too — `Text` would
    /// otherwise draw the parser's code intent in the system's monospaced
    /// font, which is the one face this app ships its own to replace.
    func testInlineCodeSpansTakeTheAppsMonospacedFace() throws {
        let attr = markdownAttributed("run `nat info` first", size: 14)
        XCTAssertEqual(String(attr.characters), "run nat info first")
        let code = try XCTUnwrap(attr.runs.first {
            $0.inlinePresentationIntent?.contains(.code) == true
        })
        XCTAssertEqual(code.font, Typo.mono(size: 13))
    }

    /// And the prose around it does not: only the span is said over.
    func testProseAroundACodeSpanKeepsItsOwnFont() {
        let attr = markdownAttributed("run `nat info` first", size: 14)
        let prose = attr.runs.filter { $0.inlinePresentationIntent?.contains(.code) != true }
        XCTAssertFalse(prose.isEmpty)
        for run in prose {
            XCTAssertNil(run.font)
        }
    }

    func testTildeFencesCloseTildeFences() {
        XCTAssertEqual(rendered("~~~\ncode\n~~~\nafter"), "code\nafter")
    }

    func testAnUnclosedFenceStaysCode() {
        XCTAssertEqual(rendered("```\nstill code"), "still code")
    }

    // MARK: - Inline syntax still parses

    func testInlineEmphasisSurvivesInsideALine() throws {
        let attr = markdownAttributed("some **bold** words", size: 14)
        XCTAssertEqual(String(attr.characters), "some bold words")
        let boldRun = attr.runs.first { $0.inlinePresentationIntent?.contains(.stronglyEmphasized) == true }
        XCTAssertNotNil(boldRun)
    }

    func testInlineEmphasisInsideAListItem() {
        XCTAssertEqual(rendered("- has *emphasis*"), "• has emphasis")
    }

    func testEmptyInputRendersEmpty() {
        XCTAssertEqual(rendered(""), "")
    }
}
