import XCTest
@testable import NatKit

final class SliceRemovalRulesTests: XCTestCase {
    /// Only a slice in progress has work to cancel.
    func testCancelIsOfferedOnlyInProgress() {
        XCTAssertTrue(SliceRemovalRules.canCancel(status: "In progress"))
        for status in ["Todo", "Done", "", nil] as [String?] {
            XCTAssertFalse(SliceRemovalRules.canCancel(status: status), "\(status ?? "nil") offers no cancel")
        }
    }

    /// The delete confirmation says what goes with the page: a Done slice's
    /// record, a slice in progress's agent and work, and otherwise the page
    /// alone.
    func testDeleteMessageSaysWhatGoes() {
        XCTAssertTrue(SliceRemovalRules.deleteMessage(status: "Done").contains("record of finished work"))
        let inProgress = SliceRemovalRules.deleteMessage(status: "In progress")
        XCTAssertTrue(inProgress.contains("Its agent is stopped"))
        XCTAssertTrue(inProgress.contains("worktree and branch"))
        XCTAssertTrue(inProgress.contains("not yet on a pull request"))
        XCTAssertEqual(SliceRemovalRules.deleteMessage(status: "Todo"), "The page goes to Notion's trash.")
        XCTAssertEqual(SliceRemovalRules.deleteMessage(status: nil), "The page goes to Notion's trash.")
    }

    /// The cancel confirmation names everything discarded and what is left,
    /// and its button is never the alert's own "Cancel".
    func testCancelWording() {
        for phrase in ["agent is stopped", "branch and worktree", "back to Todo", "pull request is left"] {
            XCTAssertTrue(SliceRemovalRules.cancelMessage.contains(phrase), phrase)
        }
        XCTAssertNotEqual(SliceRemovalRules.cancelButton, "Cancel")
    }
}
