import XCTest
@testable import NatKit
@testable import NatFixtures

/// A pull request's failing checks and conflict reach the sidebar in every
/// open project — not just the active one — and stay until a newer reading of
/// that project no longer says them.
@MainActor
final class AppModelPRMarksTests: XCTestCase {
    private let red = Fixtures.secondRedSliceID
    private let conflicting = Fixtures.secondConflictingSliceID

    private func client() -> FixtureNatClient {
        FixtureNatClient(
            otherPlans: [Fixtures.secondProjectID: Fixtures.secondProjectInfoWithPRs],
            prStatusByProject: [Fixtures.secondProjectID: Fixtures.secondProjectPRStatus])
    }

    /// The second project is never activated: its plan and its reading are
    /// both taken in the background.
    private func started(_ client: FixtureNatClient, planCache: PlanCaching = NullPlanCache()) async -> AppModel {
        let model = await Fixtures.startedAppModel(
            client: client, config: Fixtures.twoProjectConfig, planCache: planCache)
        for _ in 0..<500 where model.prStatusStore?.readings[Fixtures.secondProjectID] == nil {
            await Task.yield()
        }
        return model
    }

    private func activeMarks(_ model: AppModel, _ id: String) -> PRMarks? {
        model.sidebarModel.active.first { $0.targetID == id }?.marks
    }

    private func treeMarks(_ model: AppModel, _ id: String) -> PRMarks? {
        model.sidebarModel.projects.flatMap(\.milestones).flatMap(\.slices).first { $0.sliceID == id }?.marks
    }

    private let redMarks = PRMarks(failingChecks: ["CI / build"])
    private let conflictMarks = PRMarks(conflict: BranchConflict(base: "main"))

    func testABackgroundProjectsPullRequestsAreMarkedWithoutOpeningIt() async {
        let model = await started(client())
        XCTAssertEqual(model.activeProjectID, Fixtures.projectID)

        XCTAssertEqual(activeMarks(model, red), redMarks)
        XCTAssertEqual(treeMarks(model, red), redMarks)
        XCTAssertEqual(activeMarks(model, conflicting), conflictMarks)
        XCTAssertEqual(treeMarks(model, conflicting), conflictMarks)
        // Its tab counts the red and the conflicting pull requests beside its
        // handed-back branch — the dock's share of that project exactly.
        XCTAssertEqual(model.attention(projectID: Fixtures.secondProjectID).count, 3)
        let second = model.dockAttention.filter { $0.projectID == Fixtures.secondProjectID }
        XCTAssertEqual(second.map(\.kind), [.review, .checksFailed, .conflict])
        XCTAssertEqual(second.count, model.attention(projectID: Fixtures.secondProjectID).count)
        XCTAssertEqual(
            model.dockAttention.count,
            model.projectTabs.map { model.attention(projectID: $0.id).count }.reduce(0, +))
        XCTAssertEqual(
            model.dockMenu.map(\.heading),
            Array(Set(model.dockAttention.map(\.kind))).sorted().map(\.heading))
    }

    /// Choosing a dock menu row selects its slice, activating its project.
    func testSelectingAnAttentionItemSelectsItsSlice() async {
        let model = await started(client())
        let item = AttentionItem(kind: .checksFailed, subject: .slice(red), name: "red", projectID: Fixtures.secondProjectID)

        await model.select(item)

        XCTAssertEqual(model.activeProjectID, Fixtures.secondProjectID)
        XCTAssertEqual(model.selectedSliceID, red)

        // The planning agent's selects its workshop; a session's, the session.
        await model.select(AttentionItem(
            kind: .waiting, subject: .workshop, name: "Workshop", projectID: Fixtures.projectID))
        XCTAssertEqual(model.activeProjectID, Fixtures.projectID)
        XCTAssertTrue(model.workshopSelected)
        await model.select(AttentionItem(
            kind: .waiting, subject: .session(Fixtures.liveSessionID), name: "s", projectID: Fixtures.projectID))
        XCTAssertEqual(model.selectedSessionID, Fixtures.liveSessionID)
    }

    /// Switching away and back, and a reading that fails, clear nothing; a
    /// later reading that reads green and mergeable does.
    func testOnlyANewerReadingClearsAMark() async {
        let client = client()
        let model = await started(client)

        await model.activateProject(Fixtures.secondProjectID)
        await model.activateProject(Fixtures.projectID)
        XCTAssertEqual(activeMarks(model, red), redMarks, "switching projects clears no mark")
        XCTAssertEqual(activeMarks(model, conflicting), conflictMarks)

        client.setPRStatus(nil, forProject: Fixtures.secondProjectID)
        await model.activateProject(Fixtures.secondProjectID)
        XCTAssertEqual(activeMarks(model, red), redMarks, "a failed pr-status clears no mark")
        XCTAssertEqual(treeMarks(model, conflicting), conflictMarks)

        client.setPRStatus(PRStatusDoc(slices: [
            PRStatusSlice(
                sliceID: red, name: "", pr: "", readiness: PRStatusSlice.readyToMerge,
                checks: PRStatusChecks(verdict: "passing"), base: "main"),
            PRStatusSlice(
                sliceID: conflicting, name: "", pr: "", readiness: PRStatusSlice.awaitingReview,
                checks: PRStatusChecks(verdict: "passing"), base: "main"),
        ]), forProject: Fixtures.secondProjectID)
        await model.refresh()
        // The trouble cleared, and the green tick in its place.
        XCTAssertEqual(activeMarks(model, red), PRMarks(checksPassing: true))
        XCTAssertEqual(treeMarks(model, conflicting), PRMarks(checksPassing: true))
    }

    /// A background project's reading follows its plan's own refreshes.
    func testABackgroundRefreshTakesAFreshReading() async {
        let client = client()
        let model = await started(client)
        client.setPRStatus(PRStatusDoc(slices: []), forProject: Fixtures.secondProjectID)
        await model.refresh()
        for _ in 0..<500 where !(model.prStatusStore?.reading(projectID: Fixtures.secondProjectID).doc.slices.isEmpty ?? false) {
            await Task.yield()
        }
        XCTAssertEqual(activeMarks(model, red), PRMarks.none)
    }

    /// The last reading is kept on disk: a launch whose fresh reading fails
    /// still draws the marks.
    func testTheCachedReadingIsDrawnAtLaunch() async {
        let cache = PRStatusOnlyCache(stored: [Fixtures.secondProjectID: Fixtures.secondProjectPRStatus])
        let client = client()
        client.setPRStatus(nil, forProject: Fixtures.secondProjectID)
        let model = await Fixtures.startedAppModel(
            client: client, config: Fixtures.twoProjectConfig, planCache: cache)
        for _ in 0..<500 where treeMarks(model, red) == nil || treeMarks(model, red) == PRMarks.none {
            await Task.yield()
        }
        XCTAssertEqual(treeMarks(model, red), redMarks)
        XCTAssertEqual(activeMarks(model, conflicting), conflictMarks)
    }

    /// A project whose only slice under review is a handed-back branch with no
    /// pull request is read all the same — nat tests that branch — and the
    /// branch's conflict marks it; a plan with neither is never read.
    func testAHandBackWithNoPullRequestIsReadAndMarked() async {
        let id = "f1x75111-0000-4000-8000-0000000000b1"
        func plan(_ slices: [Slice]) -> ProjectInfo {
            ProjectInfo(
                project: Fixtures.secondProjectInfo.project, milestones: Fixtures.secondProjectInfo.milestones,
                slices: slices)
        }
        let review = Slice(
            id: id, name: "Rebase me", status: "In progress", milestoneID: "Detail overhaul", assignee: "", pr: "",
            url: "", branch: "slice/rebase-me", blocked: false, handedBack: true)
        let doc = PRStatusDoc(slices: [], branches: [
            PRStatusBranch(sliceID: id, name: "Rebase me", branch: "slice/rebase-me", base: "origin/main", conflicting: true),
        ])
        let model = await started(FixtureNatClient(
            otherPlans: [Fixtures.secondProjectID: plan([review])],
            prStatusByProject: [Fixtures.secondProjectID: doc]))
        let marks = PRMarks(conflict: BranchConflict(base: "origin/main"))
        XCTAssertEqual(activeMarks(model, id), marks)
        XCTAssertEqual(treeMarks(model, id), marks)

        let working = Slice(
            id: id, name: "Rebase me", status: "In progress", milestoneID: "Detail overhaul", assignee: "", pr: "",
            url: "", branch: nil, blocked: false, handedBack: false)
        let quiet = await Fixtures.startedAppModel(
            client: FixtureNatClient(
                otherPlans: [Fixtures.secondProjectID: plan([working])],
                prStatusByProject: [Fixtures.secondProjectID: doc]),
            config: Fixtures.twoProjectConfig, planCache: NullPlanCache())
        for _ in 0..<200 { await Task.yield() }
        XCTAssertNil(quiet.prStatusStore?.readings[Fixtures.secondProjectID], "nothing to ask nat about")
    }

    /// A project that is no longer this ID drops its reading with the rest.
    func testCleanupDropsTheStore() async {
        let model = await started(client())
        model.cleanup()
        XCTAssertNil(model.prStatusStore)
    }
}

/// A cache holding `pr-status` readings and no plan, immutable — so the
/// app's concurrent reads of it at launch (every project's plan and reading
/// at once) share nothing mutable, as `FakePlanCache` would.
private struct PRStatusOnlyCache: PlanCaching {
    let stored: [String: PRStatusDoc]

    func read(projectID: String) async -> ProjectInfo? { nil }
    func write(_ info: ProjectInfo, projectID: String) async {}
    func readPRStatus(projectID: String) async -> PRStatusDoc? { stored[projectID] }
    func writePRStatus(_ doc: PRStatusDoc, projectID: String) async {}
}
