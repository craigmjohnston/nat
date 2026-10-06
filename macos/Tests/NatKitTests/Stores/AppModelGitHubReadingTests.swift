import XCTest
@testable import NatKit
@testable import NatFixtures

/// The app's one read of GitHub, counted at the client: `pr-status` runs, the
/// PR tab's `pr-view`s and `session-list`s. A nudge makes none of them; a
/// tick makes one `pr-status` whatever the number of tabs; an action makes one
/// settle read, however many come inside its window; an open PR tab makes no
/// `pr-view` after its first load.
@MainActor
final class AppModelGitHubReadingTests: XCTestCase {
    private func started(settleDelay: Duration = .zero) async -> (AppModel, FixtureNatClient) {
        let client = FixtureNatClient(
            otherPlans: [Fixtures.secondProjectID: Fixtures.secondProjectInfoWithPRs],
            prStatusByProject: [Fixtures.secondProjectID: Fixtures.secondProjectPRStatus])
        let model = await Fixtures.startedAppModel(
            client: client, config: Fixtures.twoProjectConfig, githubSettleDelay: settleDelay)
        for _ in 0..<500 where model.prStatusStore?.readings[Fixtures.secondProjectID] == nil {
            await Task.yield()
        }
        await model.githubReadingStore?.idle()
        return (model, client)
    }

    private struct Counts: Equatable {
        let prStatus: Int
        let prView: Int
        let sessionList: Int
    }

    private func counts(_ client: FixtureNatClient) -> Counts {
        Counts(prStatus: client.prStatusRuns.count, prView: client.prViewReads.count,
               sessionList: client.sessionListReads.count)
    }

    func testANudgeReadsNoGitHub() async {
        let (model, client) = await started()
        let before = counts(client)

        await model.refresh(.replica)

        XCTAssertEqual(counts(client), before, "a nudge refreshes the plan only")
    }

    func testATickIsOnePRStatusWhateverTheTabCount() async {
        let (model, client) = await started()
        XCTAssertEqual(model.projectTabs.count, 2)
        let before = client.prStatusRuns.count

        await model.githubReadingStore?.read()

        XCTAssertEqual(client.prStatusRuns.count, before + 1)
        XCTAssertEqual(
            Set(client.prStatusRuns.last?.split(separator: ",").map(String.init) ?? []),
            [Fixtures.projectID, Fixtures.secondProjectID])
        XCTAssertEqual(client.sessionListReads.last, Fixtures.projectID, "the session rows re-read what it kept")
    }

    /// Two approves inside the settle window are one reading — and the manual
    /// refresh, and a merge, ask for the same read.
    func testActionsInsideTheWindowAreOneSettleRead() async {
        let (model, client) = await started(settleDelay: .milliseconds(200))
        let before = client.prStatusRuns.count

        model.scheduleGitHubReading()
        model.scheduleGitHubReading()
        await model.refreshByHand()
        let prStore = model.prStore(projectID: Fixtures.projectID)
        await prStore.fetch(projectID: Fixtures.projectID, sliceRef: Fixtures.approveSliceID)
        try? await prStore.merge()
        XCTAssertEqual(client.prStatusRuns.count, before, "nothing reads before the window ends")
        XCTAssertTrue(model.githubReadingStore?.isSettlePending ?? false)

        await model.githubReadingStore?.idle()
        XCTAssertEqual(client.prStatusRuns.count, before + 1)
    }

    /// An open PR tab's pull request rides the reading as its detail: after
    /// its first load, no `pr-view`.
    func testAnOpenPRTabMakesNoViewAfterItsFirstLoad() async {
        let (model, client) = await started()
        let prStore = model.prStore(projectID: Fixtures.projectID)
        await prStore.fetch(projectID: Fixtures.projectID, sliceRef: Fixtures.approveSliceID)
        prStore.setVisible(true)
        XCTAssertEqual(client.prViewReads, [Fixtures.approveSliceID])

        await model.githubReadingStore?.read()
        await model.githubReadingStore?.read()

        XCTAssertEqual(client.prStatusRuns.last?.hasSuffix(" " + Fixtures.prGreen.url), true)
        XCTAssertEqual(client.prViewReads, [Fixtures.approveSliceID], "no pr-view after the first load")
        XCTAssertEqual(prStore.loadState.pr, Fixtures.prGreen)

        prStore.setVisible(false)
        await model.githubReadingStore?.read()
        XCTAssertFalse(client.prStatusRuns.last?.contains(" ") ?? true, "a hidden tab is no detail")
    }
}
