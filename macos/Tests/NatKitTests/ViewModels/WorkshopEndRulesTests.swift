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

    // MARK: - An agent that has ended

    func testAnEndedAgentsPlanUpIsKept() {
        XCTAssertEqual(EndedWorkshop.decide(proposalRead: true, hasProposal: true, accepted: false), .keepPlan)
        XCTAssertEqual(EndedWorkshop.decide(proposalRead: true, hasProposal: true, accepted: true), .keepPlan,
                       "proposed again after an accept: still unkept")
        XCTAssertEqual(EndedWorkshop.decide(proposalRead: false, hasProposal: true, accepted: false), .keepPlan,
                       "one already on screen")
    }

    func testNothingProposedRestoresTheBrief() {
        XCTAssertEqual(EndedWorkshop.decide(proposalRead: true, hasProposal: false, accepted: false), .restoreBrief)
    }

    func testAnAcceptedPlanWithNothingSinceIsTrashed() {
        XCTAssertEqual(EndedWorkshop.decide(proposalRead: true, hasProposal: false, accepted: true), .trash)
    }

    func testAnUnreadableProposalConcludesNothing() {
        XCTAssertEqual(EndedWorkshop.decide(proposalRead: false, hasProposal: false, accepted: true), .keepAll)
        XCTAssertEqual(EndedWorkshop.decide(proposalRead: false, hasProposal: false, accepted: false), .keepAll)
    }
}
