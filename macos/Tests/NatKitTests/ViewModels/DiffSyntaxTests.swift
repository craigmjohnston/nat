import XCTest
import SwiftUI
@testable import NatKit

final class DiffSyntaxTests: XCTestCase {
    private typealias Run = DiffSyntax.Run

    // MARK: - No tokens: falls all the way back

    func testNilTokensFallsBackToOnePlainRun() {
        XCTAssertEqual(DiffSyntax.runs("hello world", tokens: nil), [Run(text: "hello world", kind: .text)])
    }

    func testEmptyTokensFallsBackToOnePlainRun() {
        XCTAssertEqual(DiffSyntax.runs("hello", tokens: []), [Run(text: "hello", kind: .text)])
    }

    // MARK: - Runs

    func testRunsSliceTheLineByTheirKinds() {
        let tokens: [TokenRun] = [
            TokenRun(kind: .keyword, length: 4), // "func"
            TokenRun(kind: .text, length: 1), // " "
            TokenRun(kind: .name, length: 1) // "f"
        ]
        XCTAssertEqual(DiffSyntax.runs("func f", tokens: tokens), [
            Run(text: "func", kind: .keyword), Run(text: " ", kind: .text), Run(text: "f", kind: .name),
        ])
    }

    func testEachKindMapsToADistinctPalette() {
        // Comment/keyword/string/number/name are each their own colour, and
        // .text falls back to whatever the row's own default is — the few
        // colours the diff viewer uses, on purpose.
        XCTAssertEqual(DiffSyntax.color(for: .comment, defaultColor: .white), DesignTokens.labelTertiary)
        XCTAssertEqual(DiffSyntax.color(for: .keyword, defaultColor: .white), DesignTokens.accent)
        XCTAssertEqual(DiffSyntax.color(for: .string, defaultColor: .white), DesignTokens.systemYellow)
        XCTAssertEqual(DiffSyntax.color(for: .number, defaultColor: .white), DesignTokens.systemTeal)
        XCTAssertEqual(DiffSyntax.color(for: .name, defaultColor: .white), DesignTokens.systemBlue)
        XCTAssertEqual(DiffSyntax.color(for: .text, defaultColor: .white), .white)
    }

    // MARK: - Multi-byte characters

    func testMultiByteCharacterRunSlicesOnAByteBoundary() {
        // "日本語" is 3 bytes per character in UTF-8 (9 bytes total), followed
        // by a 5-byte ASCII word — the run lengths are byte counts, not
        // character counts, and must still land the text back together
        // whole and in order.
        let tokens: [TokenRun] = [
            TokenRun(kind: .string, length: 9), // "日本語"
            TokenRun(kind: .text, length: 1), // " "
            TokenRun(kind: .name, length: 5) // "hello"
        ]
        XCTAssertEqual(DiffSyntax.runs("日本語 hello", tokens: tokens), [
            Run(text: "日本語", kind: .string), Run(text: " ", kind: .text), Run(text: "hello", kind: .name),
        ])
    }

    func testEmojiRunSlicesOnAByteBoundary() {
        // An emoji can take 4 bytes in UTF-8, and does not decompose into a
        // separate Unicode scalar the way some multi-byte characters do.
        let tokens: [TokenRun] = [
            TokenRun(kind: .name, length: 4), // "🎉"
            TokenRun(kind: .keyword, length: 4) // "done"
        ]
        XCTAssertEqual(DiffSyntax.runs("🎉done", tokens: tokens), [
            Run(text: "🎉", kind: .name), Run(text: "done", kind: .keyword),
        ])
    }

    // MARK: - Malformed runs fall back rather than crash

    func testRunsShorterThanTheLineFallBackPlain() {
        let tokens: [TokenRun] = [TokenRun(kind: .keyword, length: 2)] // "hello" is 5 bytes
        XCTAssertEqual(DiffSyntax.runs("hello", tokens: tokens), [Run(text: "hello", kind: .text)])
    }

    func testRunsLongerThanTheLineFallBackPlain() {
        let tokens: [TokenRun] = [TokenRun(kind: .keyword, length: 50)]
        XCTAssertEqual(DiffSyntax.runs("hi", tokens: tokens), [Run(text: "hi", kind: .text)])
    }

    func testANegativeRunLengthFallsBackPlain() {
        let tokens: [TokenRun] = [TokenRun(kind: .keyword, length: -1)]
        XCTAssertEqual(DiffSyntax.runs("hi", tokens: tokens), [Run(text: "hi", kind: .text)])
    }

    func testARunThatSplitsAMultiByteCharacterFallsBackRatherThanCrash() {
        // "日" is 3 bytes; a run of length 1 or 2 lands inside it rather than
        // on its boundary, which is not valid UTF-8 on its own.
        let tokens: [TokenRun] = [TokenRun(kind: .text, length: 1), TokenRun(kind: .text, length: 2)]
        XCTAssertEqual(DiffSyntax.runs("日", tokens: tokens), [Run(text: "日", kind: .text)])
    }
}
