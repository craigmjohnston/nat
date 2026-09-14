import XCTest
@testable import NatKit

final class TerminalPasteEncodingTests: XCTestCase {
    func testPlainPasteIsSentAsIs() {
        XCTAssertEqual(
            TerminalPasteEncoding.send("git status", bracketed: false),
            "git status"
        )
    }

    /// The whole reason bracketed paste exists: a newline in an unbracketed
    /// paste goes through unchanged, but Claude Code reads that plain
    /// carriage return as "submit".
    func testPlainPastePreservesNewlinesAsIs() {
        XCTAssertEqual(
            TerminalPasteEncoding.send("line one\nline two", bracketed: false),
            "line one\nline two"
        )
    }

    func testBracketedPasteWrapsTheTextInTheMarkers() {
        XCTAssertEqual(
            TerminalPasteEncoding.send("git status", bracketed: true),
            "\u{1b}[200~git status\u{1b}[201~"
        )
    }

    func testBracketedPastePreservesEmbeddedNewlines() {
        XCTAssertEqual(
            TerminalPasteEncoding.send("line one\nline two", bracketed: true),
            "\u{1b}[200~line one\nline two\u{1b}[201~"
        )
    }

    func testBracketedEmptyPasteIsJustTheMarkers() {
        XCTAssertEqual(
            TerminalPasteEncoding.send("", bracketed: true),
            "\u{1b}[200~\u{1b}[201~"
        )
    }

    func testPlainEmptyPasteIsEmpty() {
        XCTAssertEqual(TerminalPasteEncoding.send("", bracketed: false), "")
    }
}
