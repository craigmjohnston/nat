import XCTest
@testable import NatKit

/// The proposal's state machine: a reading lands only if nothing has
/// happened since it began.
final class ProposalStateTests: XCTestCase {
    private let first = PlanProposal(name: "first", milestones: [.init(name: "M1", slices: ["A"])])
    private let second = PlanProposal(name: "second", milestones: [.init(name: "M1", slices: ["B"])])

    func testAReadingLands() {
        var state = ProposalState()
        let ticket = state.beginReading()
        XCTAssertTrue(state.land(ticket, found: first))
        XCTAssertEqual(state.proposal, first)
    }

    func testAnOlderReadingFinishingLastIsDropped() {
        var state = ProposalState()
        let older = state.beginReading()
        let newer = state.beginReading()

        XCTAssertTrue(state.land(newer, found: second))
        XCTAssertFalse(state.land(older, found: first), "it began before the reading that landed")
        XCTAssertEqual(state.proposal, second)
    }

    func testNoFileClearsTheProposal() {
        var state = ProposalState()
        state.land(state.beginReading(), found: first)
        XCTAssertTrue(state.land(state.beginReading(), found: nil))
        XCTAssertNil(state.proposal, "accepted elsewhere: the file is the one source")
    }

    func testNothingLandsDuringAnAcceptAndNothingBegunBeforeItsEndLandsAfter() {
        var state = ProposalState()
        state.land(state.beginReading(), found: first)
        let before = state.beginReading()

        XCTAssertEqual(state.beginAccept(), first)
        XCTAssertTrue(state.accepting)
        XCTAssertNil(state.beginAccept(), "one Accept at a time")
        let during = state.beginReading()
        XCTAssertFalse(state.land(during, found: first), "nothing lands while accepting")

        state.endAccept(refusal: nil)
        XCTAssertNil(state.proposal)
        XCTAssertFalse(state.accepting)
        XCTAssertFalse(state.land(before, found: first), "read before the Accept: the accepted proposal stays gone")
        XCTAssertFalse(state.land(during, found: first), "read during it: it may have seen the file before nat dropped it")

        XCTAssertTrue(state.land(state.beginReading(), found: second), "a reading begun after lands")
        XCTAssertEqual(state.proposal, second)
    }

    func testARefusedAcceptKeepsTheProposalWithTheReason() {
        var state = ProposalState()
        state.land(state.beginReading(), found: first)
        _ = state.beginAccept()

        state.endAccept(refusal: "outgrown")

        XCTAssertEqual(state.proposal, first)
        XCTAssertEqual(state.error, "outgrown")
        XCTAssertFalse(state.accepting)
    }

    func testThereIsNothingToAcceptWithoutAProposalAndNoAcceptToEnd() {
        var state = ProposalState()
        XCTAssertNil(state.beginAccept())
        state.endAccept(refusal: "ignored")
        XCTAssertNil(state.error)
    }

    func testADiscardedTabDropsReadingsStillInFlight() {
        var state = ProposalState()
        state.land(state.beginReading(), found: first)
        let inFlight = state.beginReading()
        _ = state.beginAccept()

        state.discard()

        XCTAssertNil(state.proposal)
        XCTAssertFalse(state.accepting)
        XCTAssertFalse(state.land(inFlight, found: first))
    }

    /// A withdraw takes the proposal and its refusal down, and a reading
    /// already in flight cannot bring it back; a later one can.
    func testAWithdrawClearsAndDropsReadingsStillInFlight() {
        var state = ProposalState()
        state.land(state.beginReading(), found: first)
        state.refuse("name it")
        let inFlight = state.beginReading()

        XCTAssertTrue(state.withdraw())

        XCTAssertNil(state.proposal)
        XCTAssertNil(state.error)
        XCTAssertFalse(state.land(inFlight, found: first), "a stale reading is dropped")
        XCTAssertTrue(state.land(state.beginReading(), found: second), "the agent's next proposal lands")
        XCTAssertEqual(state.proposal, second)
    }

    /// Nothing up, or a proposal being accepted: a withdraw changes nothing.
    func testAWithdrawIsANoOpWithNothingUpOrWhileAccepting() {
        var empty = ProposalState()
        XCTAssertFalse(empty.withdraw())

        var state = ProposalState()
        state.land(state.beginReading(), found: first)
        _ = state.beginAccept()
        XCTAssertFalse(state.withdraw())
        XCTAssertEqual(state.proposal, first)
        XCTAssertTrue(state.accepting)
    }

    func testErrorsAreSetAndClearedAndANewProposalClearsThem() {
        var state = ProposalState()
        state.land(state.beginReading(), found: first)
        state.refuse("name it")
        XCTAssertEqual(state.error, "name it")
        state.clearError()
        XCTAssertNil(state.error)

        state.refuse("name it")
        state.land(state.beginReading(), found: first)
        XCTAssertEqual(state.error, "name it", "the same proposal again leaves the refusal standing")
        state.land(state.beginReading(), found: second)
        XCTAssertNil(state.error, "a revised proposal takes it down")
    }
}
