import XCTest
@testable import NatKit

final class NavigatorModelTests: XCTestCase {
    private func slice(
        status: String = "Todo", branch: String? = nil, handedBack: Bool = false, pr: String = "",
        blocked: Bool = false
    ) -> Slice {
        Slice(
            id: "s", name: "Slice", status: status, milestoneID: "M1", assignee: "", pr: pr, url: "",
            branch: branch, blocked: blocked, handedBack: handedBack)
    }

    private let prURL = "https://github.com/o/r/pull/40"

    // MARK: - Phase and defaults

    func testEachStateOpensTheDesignsSection() {
        let cases: [(Slice, AgentActivity?, Bool, NavigatorSection, MainPaneMode)] = [
            (slice(), nil, false, .brief, .empty),
            (slice(blocked: true), nil, false, .brief, .empty),
            (slice(status: "In progress"), .working, false, .thread, .terminal),
            (slice(status: "In progress"), .waiting, false, .thread, .terminal),
            (slice(status: "In progress", branch: "b", handedBack: true), nil, false, .changes, .diff),
            (slice(status: "In progress", branch: "b", pr: prURL), nil, false, .pr, .diff),
            (slice(status: "In progress", branch: "b", pr: prURL), .working, true, .thread, .terminal),
            (slice(status: "Done", branch: "b", pr: prURL), nil, false, .pr, .diff),
            (slice(status: "Done"), nil, false, .thread, .empty),
            (slice(status: "Done", branch: "b"), nil, false, .thread, .diff),
        ]
        for (index, (s, agent, fixing, phase, main)) in cases.enumerated() {
            let model = NavigatorModel(slice: s, agent: agent, fixLaunched: fixing)
            XCTAssertEqual(model.phase, phase, "case \(index)")
            XCTAssertEqual(model.defaultOpen, [phase], "case \(index)")
            XCTAssertEqual(model.defaultMain, main, "case \(index)")
        }
    }

    func testSectionsAreLiveOnTheFactsTheyRead() {
        let todo = NavigatorModel(slice: slice(), agent: nil, fixLaunched: false)
        XCTAssertTrue(todo.isLive(.brief))
        XCTAssertTrue(todo.isLive(.thread))
        XCTAssertFalse(todo.isLive(.changes))
        XCTAssertFalse(todo.isLive(.pr))
        XCTAssertFalse(todo.agentAvailable)
        XCTAssertFalse(todo.diffAvailable)

        let approved = NavigatorModel(slice: slice(status: "In progress", branch: "b", pr: prURL), agent: nil, fixLaunched: false)
        XCTAssertTrue(approved.isLive(.changes))
        XCTAssertTrue(approved.isLive(.pr))
        XCTAssertTrue(approved.agentAvailable)
        XCTAssertTrue(approved.diffAvailable)
    }

    // MARK: - Header actions

    func testLaunchIsOfferedBeforeLaunchAndToRelaunchAWorkingSlice() {
        let todo = NavigatorModel(slice: slice(), agent: nil, fixLaunched: false)
        XCTAssertTrue(todo.showsLaunch)
        XCTAssertTrue(todo.canLaunch)
        XCTAssertTrue(todo.launchIsPrimary)

        let blocked = NavigatorModel(slice: slice(blocked: true), agent: nil, fixLaunched: false)
        XCTAssertTrue(blocked.showsLaunch, "drawn disabled, as the design draws it")
        XCTAssertFalse(blocked.canLaunch)
        XCTAssertFalse(blocked.launchIsPrimary)

        let stalled = NavigatorModel(slice: slice(status: "In progress"), agent: nil, fixLaunched: false)
        XCTAssertTrue(stalled.showsLaunch)
        XCTAssertFalse(stalled.launchIsPrimary)

        let fixing = NavigatorModel(slice: slice(status: "In progress", pr: prURL), agent: nil, fixLaunched: true)
        XCTAssertTrue(fixing.showsLaunch)

        let live = NavigatorModel(slice: slice(status: "In progress"), agent: .working, fixLaunched: false)
        XCTAssertFalse(live.showsLaunch)

        for handed in [
            slice(status: "In progress", branch: "b", handedBack: true),
            slice(status: "In progress", pr: prURL),
            slice(status: "Done", pr: prURL),
        ] {
            XCTAssertFalse(NavigatorModel(slice: handed, agent: nil, fixLaunched: false).showsLaunch)
        }
        XCTAssertFalse(
            NavigatorModel(slice: slice(status: "In progress"), agent: .waiting, fixLaunched: false).showsLaunch)
    }

    func testReviewActionsAndMergeFollowTheState() {
        let review = NavigatorModel(slice: slice(status: "In progress", branch: "b", handedBack: true), agent: nil, fixLaunched: false)
        XCTAssertTrue(review.showsReviewActions)
        XCTAssertFalse(review.showsMerge)

        let approved = NavigatorModel(slice: slice(status: "In progress", pr: prURL), agent: nil, fixLaunched: false)
        XCTAssertFalse(approved.showsReviewActions)
        XCTAssertTrue(approved.showsMerge)

        let done = NavigatorModel(slice: slice(status: "Done", pr: prURL), agent: nil, fixLaunched: false)
        XCTAssertFalse(done.showsMerge)
    }

    func testEverySectionHasItsLabel() {
        XCTAssertEqual(NavigatorSection.allCases.map(\.label), ["Brief", "Thread", "Changes", "PR"])
    }

    // MARK: - The Thread

    private func agent(_ activity: AgentActivityState, model: String? = nil, effort: String? = nil, context: Double? = nil) -> AgentStatus {
        AgentStatus(sliceID: "s", session: "nat-s", activity: activity, model: model, effort: effort, contextPercent: context)
    }

    func testAnUnlaunchedSliceHasNoThread() {
        XCTAssertTrue(buildThreadEvents(slice: slice(), agent: nil, brief: nil).isEmpty)
    }

    func testAWorkingSliceReadsLaunchAndItsAgent() {
        let events = buildThreadEvents(
            slice: slice(status: "In progress", branch: "slice/x"),
            agent: agent(.working, model: "Opus 5.5", effort: "medium", context: 36.6),
            brief: nil)
        XCTAssertEqual(events, [
            ThreadEvent(who: "Launched", body: "Opus 5.5 · medium", foot: "slice/x"),
            ThreadEvent(who: "Agent", meta: "working", tone: .accent, foot: "ctx 37%"),
        ])
    }

    func testAWaitingAgentIsHotAndAnUnreadReadingSaysNothing() {
        let events = buildThreadEvents(slice: slice(status: "In progress"), agent: agent(.waiting, model: ""), brief: nil)
        XCTAssertEqual(events, [
            ThreadEvent(who: "Launched"),
            ThreadEvent(who: "Agent", meta: "waiting for you", tone: .hot),
        ])
    }

    func testAHandedBackSliceCarriesItsLastNote() {
        let brief = """
        Do the thing.

        ## Handed back

        First pass.

        ## Handed back

        Second pass, **done**.

        ## PR description

        Not a note.
        """
        let events = buildThreadEvents(
            slice: slice(status: "In progress", branch: "b", handedBack: true), agent: nil, brief: brief)
        XCTAssertEqual(events.last, ThreadEvent(who: "Agent", meta: "handed back", body: "Second pass, **done**."))
    }

    func testAnApprovedAndMergedSliceReadsToTheEnd() {
        let events = buildThreadEvents(slice: slice(status: "Done", branch: "b", pr: prURL), agent: nil, brief: nil)
        XCTAssertEqual(events.map(\.who), ["Launched", "Agent", "You", "Merged"])
        XCTAssertEqual(events[2], ThreadEvent(who: "You", meta: "approved", foot: "PR #40 → main"))
    }

    func testAPullRequestWithNoNumberStillReadsApproved() {
        let events = buildThreadEvents(slice: slice(status: "In progress", pr: "https://example.com/x"), agent: nil, brief: nil)
        XCTAssertEqual(events.last, ThreadEvent(who: "You", meta: "approved", foot: "PR opened"))
    }

    func testASliceClosedWithNoBranchSaysClosedWithItsSummary() {
        let brief = "Look into it.\n\n### Summary\n\nNothing to change; wrote it up.\n"
        let events = buildThreadEvents(slice: slice(status: "Done"), agent: nil, brief: brief)
        XCTAssertEqual(events, [
            ThreadEvent(who: "Launched"),
            ThreadEvent(who: "Closed", body: "Nothing to change; wrote it up."),
        ])
        XCTAssertEqual(
            buildThreadEvents(slice: slice(status: "Done"), agent: nil, brief: nil).last,
            ThreadEvent(who: "Closed"))
    }

    func testADoneSliceWithABranchButNoPullRequestWasMerged() {
        let events = buildThreadEvents(slice: slice(status: "Done", branch: "b"), agent: nil, brief: nil)
        XCTAssertEqual(events.map(\.who), ["Launched", "Agent", "Merged"])
    }

    func testSummaryNotesAreTheLastSummarySection() {
        XCTAssertEqual(summaryNote("## Summary\nfirst\n## Summary\nsecond"), "second")
        XCTAssertNil(summaryNote("## Handed back\nnot a summary"))
    }

    // MARK: - Helpers

    func testPullRequestNumbersComeOffTheURL() {
        XCTAssertEqual(pullRequestNumber("https://github.com/o/r/pull/214"), 214)
        XCTAssertEqual(pullRequestNumber("https://github.com/o/r/pull/7/files"), 7)
        XCTAssertNil(pullRequestNumber("https://github.com/o/r"))
        XCTAssertNil(pullRequestNumber("https://github.com/o/r/pull/"))
    }

    func testHandBackNotesAreTheLastSectionOnly() {
        XCTAssertNil(handBackNote("No heading at all."))
        XCTAssertNil(handBackNote("## Handed back\n\n"), "an empty section is no note")
        XCTAssertEqual(handBackNote("# Title\n## handed back\nnote\n### detail\nmore"), "note\n### detail\nmore",
                       "a deeper heading belongs to the note; the match ignores case")
        XCTAssertEqual(handBackNote("## Handed back\nfirst\n## Other\nx"), "first")
        XCTAssertEqual(handBackNote("#Handed back\nnot a heading"), nil)
    }
}
