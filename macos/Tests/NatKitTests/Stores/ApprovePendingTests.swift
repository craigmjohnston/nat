import XCTest
@testable import NatKit

/// A client whose plan the test rewrites between refreshes, and which
/// remembers every approve it was asked to make.
private final class ApproveClient: MockActivityClient, @unchecked Sendable {
    private let lock = NSLock()
    private var plan: ProjectInfo
    private var approves: [String] = []
    var approveError: Error?

    init(plan: ProjectInfo) {
        self.plan = plan
        super.init(response: .agents([]))
    }

    var approved: [String] { lock.withLock { approves } }

    func setPlan(_ plan: ProjectInfo) { lock.withLock { self.plan = plan } }

    override func info(projectID: String) async throws -> ProjectInfo {
        lock.withLock { plan }
    }

    override func sliceApprove(projectID: String, sliceRef: String) async throws -> String {
        lock.withLock { approves.append(sliceRef) }
        if let approveError { throw approveError }
        return "https://github.test/pr/1"
    }
}

final class ApprovePendingTests: XCTestCase {
    private static func plan(handedBack: Bool, status: String = "In progress", pr: String = "") -> ProjectInfo {
        ProjectInfo(
            project: Project(id: "proj-1", name: "Test", conventions: ""),
            milestones: [Milestone(id: "m-1", name: "M1", order: 1, status: "Active")],
            slices: [Slice(
                id: "s-1", name: "s-1", status: status, milestoneID: "m-1", assignee: "", pr: pr, url: "",
                branch: handedBack ? "slice/s-1" : nil, blocked: false, handedBack: handedBack
            )]
        )
    }

    @MainActor
    private func startedModel(client: ApproveClient) async -> AppModel {
        let config = NatProjectConfig(projects: [
            "proj-1": ProjectConfig(name: "proj-1", slicesDSID: "ds", workingDir: "/path")
        ])
        let model = AppModel(
            configReader: MockConfigReader(response: .success(config)),
            planCache: NullTestPlanCache(),
            pollIntervalSeconds: 3600,
            pathsProvider: { NatPaths(config: "/fake/config.json", logDir: "/fake", nudge: "/fake/nudge") },
            clientFactory: { client },
            activityStoreFactory: { ActivityStore(client: client) }
        )
        await model.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        return model
    }

    /// The whole flow: a read that still shows the old hand-back approves
    /// nothing, the rework being seen arms the mark, and the hand-back that
    /// follows approves exactly once.
    @MainActor
    func testTheNextHandBackAutoApprovesOnce() async {
        let client = ApproveClient(plan: Self.plan(handedBack: true))
        let model = await startedModel(client: client)

        model.markApprovePending(sliceID: "s-1")
        XCTAssertTrue(model.isApprovePending(sliceID: "s-1"))
        await model.refresh()
        XCTAssertEqual(client.approved, [], "a plan read from before the rework is not a fresh hand-back")

        client.setPlan(Self.plan(handedBack: false))
        await model.refresh()
        XCTAssertEqual(client.approved, [], "the agent is still fixing")

        client.setPlan(Self.plan(handedBack: true))
        await model.refresh()
        XCTAssertEqual(client.approved, ["s-1"])
        XCTAssertFalse(model.isApprovePending(sliceID: "s-1"))

        await model.refresh()
        XCTAssertEqual(client.approved, ["s-1"], "the mark is spent")
    }

    /// A slice nobody approved over comments is reviewed as normal.
    @MainActor
    func testWithNothingPendingNothingIsApproved() async {
        let client = ApproveClient(plan: Self.plan(handedBack: false))
        let model = await startedModel(client: client)
        await model.refresh()
        client.setPlan(Self.plan(handedBack: true))
        await model.refresh()
        XCTAssertEqual(client.approved, [])
    }

    /// The pending mark is the running app's own: a fresh app model (a
    /// restart) has none, so the same hand-back waits for review.
    @MainActor
    func testARestartForgetsTheMark() async {
        let client = ApproveClient(plan: Self.plan(handedBack: false))
        let first = await startedModel(client: client)
        first.markApprovePending(sliceID: "s-1")
        await first.refresh()

        let restarted = await startedModel(client: client)
        client.setPlan(Self.plan(handedBack: true))
        await restarted.refresh()
        XCTAssertEqual(client.approved, [])
        XCTAssertFalse(restarted.isApprovePending(sliceID: "s-1"))
    }

    /// gh's refusal at the delayed approve is shown as slice-approve's own,
    /// and the slice goes back to being reviewed by hand.
    @MainActor
    func testARefusedApproveSurfacesLikeTheButtonsAndIsNotRetried() async {
        let client = ApproveClient(plan: Self.plan(handedBack: false))
        client.approveError = NatError.commandFailed("gh: a pull request already exists")
        let model = await startedModel(client: client)
        model.markApprovePending(sliceID: "s-1")
        await model.refresh()
        client.setPlan(Self.plan(handedBack: true))
        await model.refresh()

        XCTAssertEqual(client.approved, ["s-1"])
        XCTAssertEqual(model.sliceActions.error(.approve, sliceID: "s-1"), "gh: a pull request already exists")
        XCTAssertFalse(model.isApprovePending(sliceID: "s-1"))
        await model.refresh()
        XCTAssertEqual(client.approved, ["s-1"])
    }

    /// A slice that is already Done or has its pull request has nothing left
    /// to approve, and its mark is dropped without an approve.
    @MainActor
    func testADoneOrAlreadyOpenedSliceDropsTheMark() async {
        for landed in [Self.plan(handedBack: true, status: "Done"), Self.plan(handedBack: true, pr: "https://x/pr/1")] {
            let client = ApproveClient(plan: landed)
            let model = await startedModel(client: client)
            model.markApprovePending(sliceID: "s-1")
            await model.refresh()
            XCTAssertFalse(model.isApprovePending(sliceID: "s-1"))
            XCTAssertEqual(client.approved, [])
        }
    }
}
