import XCTest
@testable import NatKit
import NatFixtures

/// An action whose effect on the pip is known is drawn at once: every send to
/// a possibly-waiting agent expects it working before its `nat` call, and
/// withdraws the expectation where the call is refused.
@MainActor
final class AppModelExpectationTests: XCTestCase {
    /// The fixture slice whose agent reads waiting.
    private let waitingID = Fixtures.activitySliceID
    private var waitingSlice: Slice { Fixtures.slice(Fixtures.activitySliceID) }

    /// A started app whose activity reading has landed, and a record of
    /// whether the waiting agent was expected working as each write went out.
    private func started(_ client: FixtureNatClient) async -> (AppModel, Box) {
        let appModel = await Fixtures.startedAppModel(client: client)
        for _ in 0..<100 where appModel.activityStore?.agents[waitingID] == nil {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        XCTAssertEqual(appModel.activityStore?.activity(for: waitingID), .waiting)
        let seen = Box()
        let id = waitingID
        client.observeWrites { [weak appModel] call in
            seen.calls.append((call, appModel?.activityStore?.expectations[id] != nil))
        }
        return (appModel, seen)
    }

    final class Box: @unchecked Sendable {
        var calls: [(String, Bool)] = []
        var allExpected: Bool { !calls.isEmpty && calls.allSatisfy(\.1) }
    }

    private func assertDrawnWorking(_ appModel: AppModel, _ seen: Box, file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertTrue(seen.allExpected, "expected before every call: \(seen.calls)", file: file, line: line)
        XCTAssertEqual(appModel.activityStore?.activity(for: waitingID), .working, file: file, line: line)
    }

    private func assertWithdrawn(_ appModel: AppModel, _ seen: Box, file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertTrue(seen.allExpected, "expected before the call: \(seen.calls)", file: file, line: line)
        XCTAssertNil(appModel.activityStore?.expectations[waitingID], file: file, line: line)
        XCTAssertEqual(appModel.activityStore?.activity(for: waitingID), .waiting, file: file, line: line)
    }

    // MARK: - Follow-ups

    func testApplyingFollowUpsExpectsTheAgentWorking() async {
        let (appModel, seen) = await started(FixtureNatClient())
        await appModel.applyFollowUps(sliceID: waitingID, batch: 1, followUps: [])
        assertDrawnWorking(appModel, seen)
    }

    func testARefusedApplyWithdrawsTheExpectation() async {
        let client = FixtureNatClient()
        let (appModel, seen) = await started(client)
        client.refuseWrites("nope")
        await appModel.applyFollowUps(sliceID: waitingID, batch: 1, followUps: [])
        assertWithdrawn(appModel, seen)
    }

    func testDiscardingFollowUpsExpectsTheAgentWorkingAndARefusalWithdraws() async {
        let client = FixtureNatClient()
        let (appModel, seen) = await started(client)
        await appModel.discardFollowUps(sliceID: waitingID, batch: 1, followUps: [])
        assertDrawnWorking(appModel, seen)

        appModel.activityStore?.withdraw(waitingID)
        client.refuseWrites("nope")
        await appModel.discardFollowUps(sliceID: waitingID, batch: 1, followUps: [])
        assertWithdrawn(appModel, seen)
    }

    // MARK: - Send back

    func testSendBackExpectsTheAgentWorking() async {
        let (appModel, seen) = await started(FixtureNatClient())
        let sent = await appModel.sendBack(slice: waitingSlice, note: "Fix the test.")
        XCTAssertTrue(sent)
        XCTAssertEqual(seen.calls.map(\.0), ["slice-resume \(waitingID) Fix the test.", "agent-send \(waitingID)"])
        assertDrawnWorking(appModel, seen)
    }

    func testARefusedSendBackWithdrawsTheExpectation() async {
        let client = FixtureNatClient()
        let (appModel, seen) = await started(client)
        client.refuseWrites("slice is Done")
        let sent = await appModel.sendBack(slice: waitingSlice, note: "Fix the test.")
        XCTAssertFalse(sent)
        assertWithdrawn(appModel, seen)
    }

    // MARK: - Comments

    private func pendDiffComment(_ appModel: AppModel) async {
        let store = appModel.diffStore(projectID: Fixtures.projectID)
        await store.fetch(projectID: Fixtures.projectID, sliceRef: waitingID)
        guard let file = store.loadState.diff?.files.first, let row = file.rows.first else {
            return XCTFail("no diff to comment on")
        }
        store.setComment(path: file.path, anchorRowIDs: [row.id], text: "clamp this")
    }

    func testSendingDiffCommentsExpectsTheAgentWorking() async throws {
        let (appModel, seen) = await started(FixtureNatClient())
        await pendDiffComment(appModel)
        let count = try await appModel.sendDiffComments(slice: waitingSlice, handedBack: false)
        XCTAssertEqual(count, 1)
        XCTAssertEqual(seen.calls.map(\.0), ["agent-send \(waitingID)"])
        assertDrawnWorking(appModel, seen)
    }

    func testARefusedDiffCommentSendWithdrawsTheExpectation() async {
        let client = FixtureNatClient()
        let (appModel, seen) = await started(client)
        await pendDiffComment(appModel)
        client.refuseWrites("no agent")
        do {
            _ = try await appModel.sendDiffComments(slice: waitingSlice, handedBack: false)
            XCTFail("the refusal is the caller's")
        } catch {}
        assertWithdrawn(appModel, seen)
    }

    private func pendVisualComment(_ appModel: AppModel) {
        appModel.visualStore(projectID: Fixtures.projectID).setComment(
            sliceID: waitingID, visual: Fixtures.visualChanges[0], point: nil,
            imageSize: CGSize(width: 100, height: 50), text: "too tight")
    }

    func testSendingVisualCommentsExpectsTheAgentWorking() async throws {
        let (appModel, seen) = await started(FixtureNatClient())
        pendVisualComment(appModel)
        let count = try await appModel.sendVisualComments(slice: waitingSlice)
        XCTAssertEqual(count, 1)
        assertDrawnWorking(appModel, seen)
    }

    func testARefusedVisualCommentSendWithdrawsTheExpectation() async {
        let client = FixtureNatClient()
        let (appModel, seen) = await started(client)
        pendVisualComment(appModel)
        client.refuseWrites("no agent")
        do {
            _ = try await appModel.sendVisualComments(slice: waitingSlice)
            XCTFail("the refusal is the caller's")
        } catch {}
        assertWithdrawn(appModel, seen)
    }

    func testCommentSendsWithNoProjectSendNothing() async throws {
        let appModel = Fixtures.appModel(client: FixtureNatClient())
        let diff = try await appModel.sendDiffComments(slice: waitingSlice, handedBack: false)
        let visual = try await appModel.sendVisualComments(slice: waitingSlice)
        XCTAssertEqual(diff + visual, 0)
    }

    // MARK: - The terminal's Enter

    func testAnEnterAtAWaitingAgentExpectsItWorkingAndNoOtherDoes() async {
        let (appModel, _) = await started(FixtureNatClient())
        appModel.terminalSubmitted(agentKey: Fixtures.diffPaneSliceID)
        XCTAssertNil(appModel.activityStore?.expectations[Fixtures.diffPaneSliceID], "a working agent is left alone")
        appModel.terminalSubmitted(agentKey: "no-such-agent")
        XCTAssertNil(appModel.activityStore?.expectations["no-such-agent"])

        appModel.terminalSubmitted(agentKey: waitingID)
        XCTAssertEqual(appModel.activityStore?.activity(for: waitingID), .working)
    }

    // MARK: - Readers

    /// The sidebar, the tab's attention and the dock read through the
    /// overlay, so an expected agent stops counting as waiting at once.
    func testAttentionAndTheSidebarReadThroughTheExpectation() async {
        let (appModel, _) = await started(FixtureNatClient())
        let waitingBefore = appModel.dockAttention.filter { $0.kind == .waiting }.count
        let countBefore = appModel.attention(projectID: Fixtures.projectID).count
        XCTAssertGreaterThan(waitingBefore, 0)

        appModel.activityStore?.expectWorking(waitingID)

        XCTAssertEqual(appModel.dockAttention.filter { $0.kind == .waiting }.count, waitingBefore - 1)
        XCTAssertEqual(appModel.attention(projectID: Fixtures.projectID).count, countBefore - 1)
        XCTAssertEqual(appModel.sidebarModel.active.first { $0.targetID == waitingID }?.state, .working)
    }
}
