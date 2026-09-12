import XCTest
@testable import NatKit
@testable import NatFixtures

/// The canned client and the app built over it — the half of the fixtures
/// that is behaviour rather than data, and so the half that can be wrong in a
/// way reading it would not show.
final class FixtureClientTests: XCTestCase {

    func testAnsweringClientHandsBackTheFixtures() async throws {
        let client = FixtureNatClient()
        let plan = try await client.info(projectID: Fixtures.projectID)
        XCTAssertEqual(plan, Fixtures.projectInfo)
        let agents = try await client.status()
        XCTAssertEqual(agents, Fixtures.agentStatuses)
        let detail = try await client.sliceShow(projectID: Fixtures.projectID, sliceRef: Fixtures.cacheSliceID)
        XCTAssertEqual(detail, Fixtures.blockedSliceDetail)
        // A slice no fixture names still gets a brief rather than a refusal.
        let fallback = try await client.sliceShow(projectID: Fixtures.projectID, sliceRef: "nobody")
        XCTAssertEqual(fallback, Fixtures.sliceDetail)
        let diff = try await client.sliceDiff(projectID: Fixtures.projectID, sliceRef: Fixtures.mergeBoxSliceID)
        XCTAssertEqual(diff, Fixtures.sliceDiff)
        let oneCommit = try await client.sliceDiff(
            projectID: Fixtures.projectID, sliceRef: Fixtures.mergeBoxSliceID, commit: Fixtures.commits[0].sha)
        XCTAssertEqual(oneCommit, Fixtures.smallSliceDiff)
        let commits = try await client.sliceCommits(projectID: Fixtures.projectID, sliceRef: Fixtures.mergeBoxSliceID)
        XCTAssertEqual(commits, Fixtures.commitsDoc)
        let pr = try await client.prView(projectID: Fixtures.projectID, sliceRef: Fixtures.approveSliceID)
        XCTAssertEqual(pr, Fixtures.prGreen)
        let status = try await client.prStatus(projectID: Fixtures.projectID)
        XCTAssertEqual(status, Fixtures.prStatusDoc)
        let config = try await client.configShow()
        XCTAssertEqual(config, Fixtures.configDoc)
    }

    func testWritesAreRememberedRatherThanPerformed() async throws {
        let client = FixtureNatClient()
        XCTAssertTrue(client.writes.isEmpty)

        let edited = try await client.sliceEdit(
            projectID: Fixtures.projectID, sliceRef: Fixtures.mergeBoxSliceID, description: "new brief")
        XCTAssertEqual(edited.brief, "new brief")
        let launch = try await client.sliceLaunch(
            projectID: Fixtures.projectID, sliceRef: Fixtures.mergeBoxSliceID, model: nil, effort: nil)
        XCTAssertEqual(launch.session, TmuxSession.name(forSlicePageID: Fixtures.mergeBoxSliceID))
        try await client.agentSend(projectID: Fixtures.projectID, sliceRef: Fixtures.mergeBoxSliceID, text: "hi")
        let approved = try await client.sliceApprove(
            projectID: Fixtures.projectID, sliceRef: Fixtures.mergeBoxSliceID)
        XCTAssertEqual(approved, Fixtures.prURL)
        try await client.prMerge(projectID: Fixtures.projectID, sliceRef: Fixtures.approveSliceID)
        try await client.prComment(
            projectID: Fixtures.projectID, sliceRef: Fixtures.approveSliceID, body: "looks good")
        let workshop = try await client.workshopLaunch(
            projectID: Fixtures.projectID, model: nil, effort: nil, request: nil)
        XCTAssertEqual(workshop.session, TmuxSession.planSessionName(projectID: Fixtures.projectID))
        let added = try await client.sliceAdd(
            projectID: Fixtures.projectID, title: "A new slice", milestone: "M3: View gallery", description: nil)
        XCTAssertEqual(added.name, "A new slice")
        try await client.configSet(key: "poll_seconds", value: "30")

        XCTAssertEqual(client.writes, [
            "slice-edit \(Fixtures.mergeBoxSliceID)",
            "slice-launch \(Fixtures.mergeBoxSliceID)",
            "agent-send \(Fixtures.mergeBoxSliceID)",
            "slice-approve \(Fixtures.mergeBoxSliceID)",
            "pr-merge \(Fixtures.approveSliceID)",
            "pr-comment \(Fixtures.approveSliceID)",
            "workshop-launch \(Fixtures.projectID)",
            "slice-add A new slice",
            "config-set poll_seconds",
        ])
    }

    func testRefusingClientRefusesEveryCall() async {
        let client = FixtureNatClient(behaviour: .refusing("nope"))
        await assertRefuses { _ = try await client.info(projectID: Fixtures.projectID) }
        await assertRefuses { _ = try await client.status() }
        await assertRefuses { _ = try await client.sliceShow(projectID: "p", sliceRef: "s") }
        await assertRefuses { _ = try await client.sliceDiff(projectID: "p", sliceRef: "s") }
        await assertRefuses { _ = try await client.sliceCommits(projectID: "p", sliceRef: "s") }
        await assertRefuses { _ = try await client.prView(projectID: "p", sliceRef: "s") }
        await assertRefuses { _ = try await client.prStatus(projectID: "p") }
        await assertRefuses { _ = try await client.configShow() }
        await assertRefuses { _ = try await client.sliceEdit(projectID: "p", sliceRef: "s", description: "d") }
        await assertRefuses { _ = try await client.sliceLaunch(projectID: "p", sliceRef: "s", model: nil, effort: nil) }
        await assertRefuses { try await client.agentSend(projectID: "p", sliceRef: "s", text: "t") }
        await assertRefuses { _ = try await client.sliceApprove(projectID: "p", sliceRef: "s") }
        await assertRefuses { try await client.prMerge(projectID: "p", sliceRef: "s") }
        await assertRefuses { try await client.prComment(projectID: "p", sliceRef: "s", body: "b") }
        await assertRefuses { _ = try await client.workshopLaunch(projectID: "p", model: nil, effort: nil, request: nil) }
        await assertRefuses { _ = try await client.sliceAdd(projectID: "p", title: "t", milestone: "m", description: nil) }
        await assertRefuses { try await client.configSet(key: "k", value: "v") }
        XCTAssertTrue(client.writes.isEmpty)
    }

    /// The client the skeleton stories are drawn over: every call waits, and
    /// the only way out is cancelling it.
    func testHangingClientNeverAnswers() async {
        let client = FixtureNatClient(behaviour: .hanging)
        let read = Task { try await client.info(projectID: Fixtures.projectID) }
        let write = Task { try await client.configSet(key: "k", value: "v") }
        // Long enough that an answering client would have answered many
        // times over, and short enough to be no wait at all.
        try? await Task.sleep(for: .milliseconds(50))
        XCTAssertFalse(read.isCancelled)
        read.cancel()
        write.cancel()
        let readResult = await read.result
        let writeResult = await write.result
        XCTAssertThrowsError(try readResult.get())
        XCTAssertThrowsError(try writeResult.get())
        // A call that never lands never records anything either.
        XCTAssertTrue(client.writes.isEmpty)
    }

    /// The rail's skeleton state, as a story reaches it: started, and still
    /// loading with nothing to show.
    @MainActor
    func testLoadingAppModelStaysLoading() async {
        let model = Fixtures.loadingAppModel()
        defer { model.cleanup() }
        try? await Task.sleep(for: .milliseconds(50))

        XCTAssertFalse(model.needsOnboarding)
        XCTAssertEqual(model.activeProjectID, Fixtures.projectID)
        XCTAssertTrue(model.projectStore?.state.isLoading ?? false)
        XCTAssertNil(model.projectStore?.state.projectInfo)
        XCTAssertNil(model.projectStore?.state.errorMessage)
    }

    func testNullPlanCacheRemembersNothing() async {
        let cache = NullPlanCache()
        await cache.write(Fixtures.projectInfo, projectID: Fixtures.projectID)
        let read = await cache.read(projectID: Fixtures.projectID)
        XCTAssertNil(read)
    }

    func testFixtureConfigReaderIgnoresThePath() async throws {
        let reader = FixtureConfigReader()
        let read = try await reader.readConfig(from: "/nowhere")
        XCTAssertEqual(read, Fixtures.config)
        let empty = FixtureConfigReader(config: Fixtures.emptyConfig)
        let readEmpty = try await empty.readConfig(from: "/nowhere")
        XCTAssertEqual(readEmpty, Fixtures.emptyConfig)
    }

    @MainActor
    func testStartedAppModelIsAWholeBoard() async {
        let model = await Fixtures.startedAppModel()
        defer { model.cleanup() }

        XCTAssertFalse(model.needsOnboarding)
        XCTAssertEqual(model.activeProjectID, Fixtures.projectID)
        XCTAssertEqual(model.projectTabs.map(\.id), [Fixtures.projectID])
        XCTAssertEqual(model.projectStore?.state.projectInfo, Fixtures.projectInfo)
        XCTAssertEqual(model.config, Fixtures.config)
        XCTAssertEqual(model.reviewStatsStore?.stats, Fixtures.reviewStats)
        XCTAssertEqual(model.reviewStatsStore?.fileCounts, Fixtures.reviewFileCounts)
        XCTAssertEqual(model.reviewStatsStore?.prReadiness, Fixtures.prReadiness)

        // The per-slice stores it makes read the canned client too.
        let diffStore = model.diffStore(projectID: Fixtures.projectID)
        await diffStore.fetch(projectID: Fixtures.projectID, sliceRef: Fixtures.mergeBoxSliceID)
        XCTAssertEqual(diffStore.loadState.diff, Fixtures.diffModel)

        let prStore = model.prStore(projectID: Fixtures.projectID)
        await prStore.fetch(projectID: Fixtures.projectID, sliceRef: Fixtures.approveSliceID)
        XCTAssertEqual(prStore.loadState.pr, Fixtures.prGreen)

        let detailStore = model.sliceDetailStore(projectID: Fixtures.projectID)
        await detailStore.fetch(sliceRef: Fixtures.mergeBoxSliceID)
        XCTAssertEqual(detailStore.state(for: Fixtures.mergeBoxSliceID).detail, Fixtures.sliceDetail)
    }

    @MainActor
    func testStartedAppModelOverARefusingClient() async {
        let model = await Fixtures.startedAppModel(
            client: FixtureNatClient(behaviour: .refusing(Fixtures.loadErrorMessage)))
        defer { model.cleanup() }

        XCTAssertFalse(model.needsOnboarding)
        XCTAssertNil(model.projectStore?.state.projectInfo)
        XCTAssertNotNil(model.projectStore?.state.errorMessage)
    }

    @MainActor
    func testStartedAppModelOnAConfigNamingNoProject() async {
        let model = await Fixtures.startedAppModel(config: Fixtures.emptyConfig)
        defer { model.cleanup() }

        XCTAssertTrue(model.needsOnboarding)
        XCTAssertNil(model.activeProjectID)
    }

    @MainActor
    func testUnstartedAppModelIsUnstarted() {
        let model = Fixtures.appModel()
        defer { model.cleanup() }
        XCTAssertTrue(model.needsOnboarding)
        XCTAssertNil(model.projectStore)
    }

    private func assertRefuses(
        file: StaticString = #filePath,
        line: UInt = #line,
        _ body: () async throws -> Void
    ) async {
        do {
            try await body()
            XCTFail("a refusing client answered", file: file, line: line)
        } catch {
            XCTAssertTrue(error is NatError, file: file, line: line)
        }
    }
}
