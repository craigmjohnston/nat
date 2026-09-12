import XCTest
@testable import NatKit

final class TerminalMouseTests: XCTestCase {
    /// The link gesture, and the one click withheld from tmux — which is what
    /// keeps tmux's own hyperlink binding and this pane's opener from both
    /// opening the same link.
    func testACommandClickIsTheTerminalsOwn() {
        XCTAssertTrue(TerminalMouse.isTerminalOwnClick(.command))
    }

    func testCommandWithAnotherModifierIsStillTheTerminalsOwn() {
        XCTAssertTrue(TerminalMouse.isTerminalOwnClick([.command, .shift]))
        XCTAssertTrue(TerminalMouse.isTerminalOwnClick([.command, .control, .option]))
    }

    /// A plain click goes where it always went: through mouse reporting to
    /// tmux and the agent, tmux's hyperlink binding included.
    func testAPlainClickGoesToTheProgram() {
        XCTAssertFalse(TerminalMouse.isTerminalOwnClick([]))
    }

    /// Shift is the emulator's own selection bypass and control and option
    /// are encoded and read by tmux and the agent, so none of the three is
    /// this pane's to take.
    func testTheOtherModifiersGoToTheProgram() {
        let sets: [TerminalKeyModifiers] = [.shift, .control, .option, [.shift, .control], [.control, .option]]
        for modifiers in sets {
            XCTAssertFalse(
                TerminalMouse.isTerminalOwnClick(modifiers),
                "modifier set \(modifiers.rawValue) should reach the program"
            )
        }
    }
}
