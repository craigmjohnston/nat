import XCTest
@testable import NatKit
import NatFixtures

/// Send back to agent: what it offers to say, what the agent is told, and the
/// order it writes in — the record (`slice-resume`) before the agent hears.
@MainActor
final class SendBackTests: XCTestCase {
    private var approved: Slice { Fixtures.slice(Fixtures.approveSliceID) }

    // MARK: - The words

    func testTheReasonIsThePullRequestsOwnTrouble() {
        XCTAssertEqual(sendBackReason(checks: nil, conflict: nil), "")
        let checks = ChecksNotice(checks: ["CI / test", "lint"], action: .sendBack)
        XCTAssertEqual(
            sendBackReason(checks: checks, conflict: nil), "Checks are failing on the pull request: CI / test, lint.")
        XCTAssertEqual(
            sendBackReason(checks: ChecksNotice(checks: [], action: .none), conflict: nil),
            "Checks are failing on the pull request.")
        let conflict = ConflictNotice(conflict: BranchConflict(base: "main"), action: .sendBack)
        XCTAssertEqual(
            sendBackReason(checks: nil, conflict: conflict),
            "The branch conflicts with main: merge main in and resolve the conflicts.")
        XCTAssertEqual(
            sendBackReason(checks: checks, conflict: ConflictNotice(conflict: BranchConflict(base: nil), action: .none)),
            "Checks are failing on the pull request: CI / test, lint. "
                + "The branch conflicts with its base: merge its base in and resolve the conflicts.")
    }

    func testThePromptSaysWhyThenEndsInTheHandBackOnTheSameBranch() {
        let handBack = HandBackInstruction(projectID: "p", sliceRef: "s")
        let prompt = sendBackPrompt(note: "  Rename the helper.\n", branch: "slice/x", handBack: handBack)
        XCTAssertTrue(prompt.hasPrefix("I am sending this task back to you for more work:\n\nRename the helper.\n\n"))
        XCTAssertTrue(prompt.hasSuffix(
            "nat complete-slice s --project p --branch slice/x --summary '<what you changed>'\n"))
        XCTAssertTrue(
            sendBackPrompt(note: "x", branch: nil, handBack: handBack).contains("--branch <your branch>"),
            "no branch recorded: the agent fills it in")
        XCTAssertTrue(sendBackPrompt(note: "x", branch: "", handBack: handBack).contains("--branch <your branch>"))
    }

    func testSendBackIsAOneShotWithNoStageOfItsOwn() {
        XCTAssertNil(SliceActionKind.sendBack.advance)
        XCTAssertEqual(NavigatorBarButton.sendBackTitle, "Send back to agent")
    }

    // MARK: - The flow

    func testWithNoAgentItResumesThenLaunches() async {
        let client = FixtureNatClient(agents: [])
        let appModel = await Fixtures.startedAppModel(client: client)
        let sent = await appModel.sendBack(slice: approved, note: "  Fix the test.  ")
        XCTAssertTrue(sent)
        XCTAssertEqual(
            client.writes,
            ["slice-resume \(Fixtures.approveSliceID) Fix the test.", "slice-launch \(Fixtures.approveSliceID)"],
            "the record first, the note trimmed")
        XCTAssertNil(appModel.sliceActions.error(.sendBack, sliceID: Fixtures.approveSliceID))
    }

    func testWithALiveAgentItResumesThenTellsIt() async {
        let client = FixtureNatClient(agents: Fixtures.approvedAgentStatuses)
        let appModel = await Fixtures.startedAppModel(client: client)
        for _ in 0..<50 where appModel.activityStore?.agents[Fixtures.approveSliceID] == nil {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        let sent = await appModel.sendBack(slice: approved, note: "Fix the test.")
        XCTAssertTrue(sent)
        XCTAssertEqual(
            client.writes,
            ["slice-resume \(Fixtures.approveSliceID) Fix the test.", "agent-send \(Fixtures.approveSliceID)"])
    }

    func testAnEmptyNoteSendsNothingAndSaysWhy() async {
        let client = FixtureNatClient(agents: [])
        let appModel = await Fixtures.startedAppModel(client: client)
        let sent = await appModel.sendBack(slice: approved, note: " \n ")
        XCTAssertFalse(sent)
        XCTAssertEqual(client.writes, [])
        XCTAssertEqual(
            appModel.sliceActions.error(.sendBack, sliceID: Fixtures.approveSliceID), "Say what the agent should change.")
    }

    func testARefusedResumeTellsNoAgent() async {
        let client = FixtureNatClient(behaviour: .refusing("slice is Done"))
        let appModel = Fixtures.appModel(client: client)
        // Unstarted, so it has no project to send for: nothing is asked.
        let unstarted = await appModel.sendBack(slice: approved, note: "x")
        XCTAssertFalse(unstarted)
        await Fixtures.start(appModel)
        let refused = await appModel.sendBack(slice: approved, note: "x")
        XCTAssertFalse(refused)
        XCTAssertEqual(appModel.sliceActions.error(.sendBack, sliceID: Fixtures.approveSliceID), "slice is Done")
    }
}
