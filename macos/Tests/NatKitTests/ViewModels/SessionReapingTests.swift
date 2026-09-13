import XCTest
@testable import NatKit

final class SessionReapingTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_700_000_000)

    private func slice(_ id: String, status: String, pr: String = "") -> Slice {
        Slice(
            id: id, name: id, status: status, milestoneID: "m-1",
            assignee: "", pr: pr, url: "", blocked: false, handedBack: false
        )
    }

    private func agent(_ sliceID: String, activity: AgentActivityState = .waiting) -> AgentStatus {
        AgentStatus(sliceID: sliceID, session: "nat-\(sliceID)", activity: activity)
    }

    private func reap(
        agents: [AgentStatus],
        slices: [Slice],
        openPRSliceIDs: Set<String> = [],
        selected: String? = nil,
        leftAt: [String: Date] = [:]
    ) -> [String] {
        agentSessionsToReap(
            agents: agents.reduce(into: [:]) { $0[$1.sliceID] = $1 },
            slices: slices,
            openPRSliceIDs: openPRSliceIDs,
            selectedSliceID: selected,
            leftAt: leftAt,
            now: now,
            grace: 300
        )
    }

    /// A session left over from a previous run of the app: its slice was
    /// never selected, so there is no stamp and nothing to wait for.
    func testNeverSelectedDoneSliceIsReapedAtOnce() {
        let reaped = reap(agents: [agent("s-1")], slices: [slice("s-1", status: "Done")])

        XCTAssertEqual(reaped, ["s-1"])
    }

    func testDoneSliceIsLeftAloneInsideTheGracePeriod() {
        let reaped = reap(
            agents: [agent("s-1")],
            slices: [slice("s-1", status: "Done")],
            leftAt: ["s-1": now.addingTimeInterval(-60)]
        )

        XCTAssertEqual(reaped, [])
    }

    func testDoneSliceIsReapedOnceTheGracePeriodIsUp() {
        let reaped = reap(
            agents: [agent("s-1")],
            slices: [slice("s-1", status: "Done")],
            leftAt: ["s-1": now.addingTimeInterval(-301)]
        )

        XCTAssertEqual(reaped, ["s-1"])
    }

    func testTheSliceOnScreenIsNeverReaped() {
        let reaped = reap(
            agents: [agent("s-1")],
            slices: [slice("s-1", status: "Done")],
            selected: "s-1"
        )

        XCTAssertEqual(reaped, [])
    }

    func testAnUnfinishedSliceIsNeverReaped() {
        let reaped = reap(
            agents: [agent("s-1")],
            slices: [slice("s-1", status: "In progress")]
        )

        XCTAssertEqual(reaped, [])
    }

    /// A Done slice whose pull request is still open is the very slice a fix
    /// session runs on — `sliceWorkDone` says its work is not done yet.
    func testADoneSliceWithAnOpenPullRequestIsNeverReaped() {
        let reaped = reap(
            agents: [agent("s-1")],
            slices: [slice("s-1", status: "Done", pr: "https://github.test/pr/1")],
            openPRSliceIDs: ["s-1"]
        )

        XCTAssertEqual(reaped, [])
    }

    func testAnAgentMidTurnIsLeftToFinish() {
        let reaped = reap(
            agents: [agent("s-1", activity: .working)],
            slices: [slice("s-1", status: "Done")]
        )

        XCTAssertEqual(reaped, [])
    }

    func testASliceWithNoAgentRunningIsNotReported() {
        let reaped = reap(agents: [], slices: [slice("s-1", status: "Done")])

        XCTAssertEqual(reaped, [])
    }

    /// An agent whose slice is not this project's — another project's, or the
    /// planning agent, whose key is its project's tag rather than a slice.
    func testAnAgentOnNoSliceOfThisPlanIsNotReported() {
        let reaped = reap(agents: [agent("plan:proj-1")], slices: [slice("s-1", status: "Done")])

        XCTAssertEqual(reaped, [])
    }

    func testTheAnswerIsSorted() {
        let reaped = reap(
            agents: [agent("s-2"), agent("s-1")],
            slices: [slice("s-2", status: "Done"), slice("s-1", status: "Done")]
        )

        XCTAssertEqual(reaped, ["s-1", "s-2"])
    }
}

final class PRSettledTests: XCTestCase {
    private func pr(_ state: String) -> PRDetail {
        PRDetail(
            number: 1, title: "t", body: "", state: state, isDraft: false,
            author: "craig", baseRefName: "main", headRefName: "slice/x",
            url: "https://github.test/craig/nat/pull/1",
            reviewDecision: "", mergeable: "MERGEABLE", mergeStateStatus: "CLEAN"
        )
    }

    func testAMergedPullRequestIsSettled() {
        XCTAssertTrue(prIsSettled(pr(PRLifecycleState.merged)))
    }

    func testAClosedPullRequestIsSettled() {
        XCTAssertTrue(prIsSettled(pr(PRLifecycleState.closed)))
    }

    func testAnOpenPullRequestIsNot() {
        XCTAssertFalse(prIsSettled(pr("OPEN")))
    }

    /// A word this build does not know is not one of the two endings, so it
    /// is not settled — the direction to be wrong in when a kill rides on it.
    func testAWordThisBuildDoesNotKnowIsNot() {
        XCTAssertFalse(prIsSettled(pr("SOMETHING_NEW")))
    }
}
