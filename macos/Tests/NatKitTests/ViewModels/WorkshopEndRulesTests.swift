import XCTest
@testable import NatKit

final class WorkshopEndRulesTests: XCTestCase {
    private func ask(_ activity: AgentActivityState?, proposal: Bool = false, accepting: Bool = false) -> String? {
        WorkshopEndRules.confirmation(activity: activity, hasProposal: proposal, accepting: accepting)
    }

    func testWorkingAsksWithTheWorkingMessage() {
        XCTAssertEqual(ask(.working), WorkshopEndRules.workingMessage)
    }

    func testAnUnreadableActivityIsTakenAsWorking() {
        XCTAssertEqual(ask(.unknown), WorkshopEndRules.workingMessage)
    }

    func testAProposalUpAsksWithTheProposalMessage() {
        XCTAssertEqual(ask(.waiting, proposal: true), WorkshopEndRules.proposalMessage)
        XCTAssertEqual(ask(.working, proposal: true), WorkshopEndRules.proposalMessage)
    }

    func testAnAcceptInFlightAsks() {
        XCTAssertEqual(ask(.waiting, accepting: true), WorkshopEndRules.proposalMessage)
    }

    func testWaitingWithNothingPendingEndsAtOnce() {
        XCTAssertNil(ask(.waiting))
    }

    func testNoAgentAsksNothing() {
        XCTAssertNil(ask(nil))
        XCTAssertNil(ask(nil, proposal: true))
    }
}
