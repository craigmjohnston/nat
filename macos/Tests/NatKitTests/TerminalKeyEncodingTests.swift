import XCTest
@testable import NatKit

final class TerminalKeyEncodingTests: XCTestCase {
    // MARK: - The bytes themselves

    /// The two constants are the Go TUI's `shiftEnterBytes` and
    /// `ctrlEnterBytes` verbatim. Written out as escapes here rather than
    /// compared to the constants they came from, so a typo in either one is
    /// the failure rather than agreeing with itself.
    func testShiftEnterIsTheCSIuEncodingOfEnterWithShift() {
        XCTAssertEqual(TerminalKeyEncoding.shiftEnter, "\u{1b}[13;2u")
    }

    func testCtrlEnterIsTheCSIuEncodingOfEnterWithControl() {
        XCTAssertEqual(TerminalKeyEncoding.ctrlEnter, "\u{1b}[13;5u")
    }

    // MARK: - Which modifier sets are encoded by hand

    func testShiftAloneEncodesShiftEnter() {
        XCTAssertEqual(TerminalKeyEncoding.returnKey(.shift), TerminalKeyEncoding.shiftEnter)
    }

    func testControlAloneEncodesCtrlEnter() {
        XCTAssertEqual(TerminalKeyEncoding.returnKey(.control), TerminalKeyEncoding.ctrlEnter)
    }

    /// A plain enter is the emulator's own to encode — it is the one enter a
    /// carriage return is right for.
    func testNoModifierIsLeftToTheEmulator() {
        XCTAssertNil(TerminalKeyEncoding.returnKey([]))
    }

    /// The match is on the whole set, so a combination neither key stands for
    /// is left alone rather than guessed at.
    func testOtherModifierSetsAreLeftToTheEmulator() {
        let sets: [TerminalKeyModifiers] = [
            .option,
            .command,
            [.shift, .control],
            [.shift, .option],
            [.shift, .command],
            [.control, .command],
            [.shift, .control, .option, .command]
        ]
        for modifiers in sets {
            XCTAssertNil(
                TerminalKeyEncoding.returnKey(modifiers),
                "modifier set \(modifiers.rawValue) should be left to the emulator"
            )
        }
    }

    // MARK: - The modifier set

    func testTheFourModifiersAreDistinctBits() {
        let all: TerminalKeyModifiers = [.shift, .control, .option, .command]
        XCTAssertEqual(all.rawValue, 0b1111)
        XCTAssertTrue(all.contains(.shift))
        XCTAssertTrue(all.contains(.command))
    }

    func testAModifierSetIsBuiltFromItsRawValue() {
        XCTAssertEqual(TerminalKeyModifiers(rawValue: 1 << 1), .control)
    }
}
