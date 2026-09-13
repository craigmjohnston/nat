import XCTest
@testable import NatKit

final class SessionReapingTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_700_000_000)

    private func slice(_ id: String, status: String) -> Slice {
        Slice(
            id: id, name: id, status: status, milestoneID: "m-1",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )
    }

    private func agent(_ sliceID: String, activity: AgentActivityState = .waiting) -> AgentStatus {
        AgentStatus(sliceID: sliceID, session: "nat-\(sliceID)", activity: activity)
    }

    private func reap(
        agents: [AgentStatus],
        slicesByID: [String: Slice],
        selected: String? = nil,
        heldUntil: [String: Date] = [:]
    ) -> [String] {
        agentSessionsToReap(
            agents: agents,
            slicesByID: slicesByID,
            selectedSliceID: selected,
            heldUntil: heldUntil,
            now: now
        )
    }

    // MARK: - The candidate rule

    /// A session left over from a previous run of the app: its slice was
    /// never visited, so there is no hold and nothing to wait for.
    func testNeverVisitedDoneSliceIsACandidateAtOnce() {
        let reaped = reap(agents: [agent("s-1")], slicesByID: ["s-1": slice("s-1", status: "Done")])

        XCTAssertEqual(reaped, ["s-1"])
    }

    func testTodoSliceIsACandidateToo() {
        let reaped = reap(agents: [agent("s-1")], slicesByID: ["s-1": slice("s-1", status: "Todo")])

        XCTAssertEqual(reaped, ["s-1"])
    }

    /// A slice absent from every open project's plan — another project's
    /// session, or one whose slice has since been deleted — is a candidate
    /// exactly as one present with the wrong status is.
    func testASliceAbsentFromEveryOpenPlanIsACandidate() {
        let reaped = reap(agents: [agent("s-1")], slicesByID: [:])

        XCTAssertEqual(reaped, ["s-1"])
    }

    func testAnInProgressSliceIsNeverACandidate() {
        let reaped = reap(agents: [agent("s-1")], slicesByID: ["s-1": slice("s-1", status: "In progress")])

        XCTAssertEqual(reaped, [])
    }

    func testAnAgentMidTurnIsLeftToFinish() {
        let reaped = reap(
            agents: [agent("s-1", activity: .working)],
            slicesByID: ["s-1": slice("s-1", status: "Done")]
        )

        XCTAssertEqual(reaped, [])
    }

    func testTheSliceOnScreenIsNeverACandidate() {
        let reaped = reap(
            agents: [agent("s-1")],
            slicesByID: ["s-1": slice("s-1", status: "Done")],
            selected: "s-1"
        )

        XCTAssertEqual(reaped, [])
    }

    func testAHeldSliceIsNotACandidateWhileTheHoldRuns() {
        let reaped = reap(
            agents: [agent("s-1")],
            slicesByID: ["s-1": slice("s-1", status: "Done")],
            heldUntil: ["s-1": now.addingTimeInterval(60)]
        )

        XCTAssertEqual(reaped, [])
    }

    func testASliceIsACandidateOnceItsHoldHasExpired() {
        let reaped = reap(
            agents: [agent("s-1")],
            slicesByID: ["s-1": slice("s-1", status: "Done")],
            heldUntil: ["s-1": now.addingTimeInterval(-1)]
        )

        XCTAssertEqual(reaped, ["s-1"])
    }

    /// A planning agent's session is tagged by its project rather than by a
    /// slice, and is never a candidate here: it belongs to no slice for a
    /// status to disagree with, and reaping it is not this rule's job.
    func testAPlanningAgentSessionIsNeverACandidate() {
        let reaped = reap(agents: [agent("plan:proj-1")], slicesByID: [:])

        XCTAssertEqual(reaped, [])

        let legacy = reap(agents: [agent("plan")], slicesByID: [:])
        XCTAssertEqual(legacy, [])
    }

    func testASliceWithNoAgentRunningIsNotReported() {
        let reaped = reap(agents: [], slicesByID: ["s-1": slice("s-1", status: "Done")])

        XCTAssertEqual(reaped, [])
    }

    func testTheAnswerIsSorted() {
        let reaped = reap(
            agents: [agent("s-2"), agent("s-1")],
            slicesByID: ["s-1": slice("s-1", status: "Done"), "s-2": slice("s-2", status: "Done")]
        )

        XCTAssertEqual(reaped, ["s-1", "s-2"])
    }
}

final class VerifiedForReapTests: XCTestCase {
    func testInProgressIsKept() {
        XCTAssertFalse(verifiedForReap(.found(status: "In progress", trashed: false)))
    }

    func testDoneIsReaped() {
        XCTAssertTrue(verifiedForReap(.found(status: "Done", trashed: false)))
    }

    func testTodoIsReaped() {
        XCTAssertTrue(verifiedForReap(.found(status: "Todo", trashed: false)))
    }

    /// A page trashed while it read In progress is reaped anyway: whatever
    /// the status column still says, nothing is claiming the session on a
    /// page that has been thrown away.
    func testATrashedInProgressPageIsReaped() {
        XCTAssertTrue(verifiedForReap(.found(status: "In progress", trashed: true)))
    }

    func testAGonePageIsReaped() {
        XCTAssertTrue(verifiedForReap(.gone))
    }

    /// A read that failed answers no: nothing is killed on no news, since the
    /// one question standing between a session and a kill did not get an
    /// answer.
    func testAFailedReadIsKept() {
        XCTAssertFalse(verifiedForReap(nil))
    }
}
