import XCTest
import SwiftUI
@testable import NatKit

final class DiffSyntaxTests: XCTestCase {
    private let defaultColor = DesignTokens.label

    /// The plain text of an `AttributedString`, for asserting the runs
    /// reassemble to exactly the input.
    private func plainText(_ s: AttributedString) -> String {
        String(s.characters)
    }

    private func colors(_ s: AttributedString) -> [Color?] {
        s.runs.map { $0.foregroundColor }
    }

    // MARK: - No tokens: falls all the way back

    func testNilTokensFallsBackToOneUncolouredRun() {
        let result = DiffSyntax.attributedLine("hello world", tokens: nil, defaultColor: defaultColor)
        XCTAssertEqual(plainText(result), "hello world")
        XCTAssertEqual(colors(result), [defaultColor])
    }

    func testEmptyTokensFallsBackToOneUncolouredRun() {
        let result = DiffSyntax.attributedLine("hello", tokens: [], defaultColor: defaultColor)
        XCTAssertEqual(plainText(result), "hello")
        XCTAssertEqual(colors(result), [defaultColor])
    }

    // MARK: - Run -> AttributedString mapping

    func testRunsMapToTheirKindsColours() {
        let tokens: [TokenRun] = [
            TokenRun(kind: .keyword, length: 4), // "func"
            TokenRun(kind: .text, length: 1), // " "
            TokenRun(kind: .name, length: 1) // "f"
        ]
        let result = DiffSyntax.attributedLine("func f", tokens: tokens, defaultColor: defaultColor)

        XCTAssertEqual(plainText(result), "func f")
        XCTAssertEqual(colors(result), [
            DiffSyntax.color(for: .keyword, defaultColor: defaultColor),
            defaultColor,
            DiffSyntax.color(for: .name, defaultColor: defaultColor)
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
        let text = "日本語 hello"
        let tokens: [TokenRun] = [
            TokenRun(kind: .string, length: 9), // "日本語"
            TokenRun(kind: .text, length: 1), // " "
            TokenRun(kind: .name, length: 5) // "hello"
        ]
        let result = DiffSyntax.attributedLine(text, tokens: tokens, defaultColor: defaultColor)

        XCTAssertEqual(plainText(result), text)
        XCTAssertEqual(colors(result), [
            DiffSyntax.color(for: .string, defaultColor: defaultColor),
            defaultColor,
            DiffSyntax.color(for: .name, defaultColor: defaultColor)
        ])
    }

    func testEmojiRunSlicesOnAByteBoundary() {
        // An emoji can take 4 bytes in UTF-8, and does not decompose into a
        // separate Unicode scalar the way some multi-byte characters do.
        let text = "🎉done"
        let tokens: [TokenRun] = [
            TokenRun(kind: .name, length: 4), // "🎉"
            TokenRun(kind: .keyword, length: 4) // "done"
        ]
        let result = DiffSyntax.attributedLine(text, tokens: tokens, defaultColor: defaultColor)

        XCTAssertEqual(plainText(result), text)
        XCTAssertEqual(colors(result), [
            DiffSyntax.color(for: .name, defaultColor: defaultColor),
            DiffSyntax.color(for: .keyword, defaultColor: defaultColor)
        ])
    }

    // MARK: - Malformed runs fall back rather than crash

    func testRunsShorterThanTheLineFallBackUncoloured() {
        let tokens: [TokenRun] = [TokenRun(kind: .keyword, length: 2)] // "hello" is 5 bytes
        let result = DiffSyntax.attributedLine("hello", tokens: tokens, defaultColor: defaultColor)
        XCTAssertEqual(plainText(result), "hello")
        XCTAssertEqual(colors(result), [defaultColor])
    }

    func testRunsLongerThanTheLineFallBackUncoloured() {
        let tokens: [TokenRun] = [TokenRun(kind: .keyword, length: 50)]
        let result = DiffSyntax.attributedLine("hi", tokens: tokens, defaultColor: defaultColor)
        XCTAssertEqual(plainText(result), "hi")
        XCTAssertEqual(colors(result), [defaultColor])
    }

    func testANegativeRunLengthFallsBackUncoloured() {
        let tokens: [TokenRun] = [TokenRun(kind: .keyword, length: -1)]
        let result = DiffSyntax.attributedLine("hi", tokens: tokens, defaultColor: defaultColor)
        XCTAssertEqual(plainText(result), "hi")
        XCTAssertEqual(colors(result), [defaultColor])
    }

    func testARunThatSplitsAMultiByteCharacterFallsBackRatherThanCrash() {
        // "日" is 3 bytes; a run of length 1 or 2 lands inside it rather than
        // on its boundary, which is not valid UTF-8 on its own.
        let tokens: [TokenRun] = [TokenRun(kind: .text, length: 1), TokenRun(kind: .text, length: 2)]
        let result = DiffSyntax.attributedLine("日", tokens: tokens, defaultColor: defaultColor)
        XCTAssertEqual(plainText(result), "日")
        XCTAssertEqual(colors(result), [defaultColor])
    }
}
