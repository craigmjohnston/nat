import XCTest
@testable import NatKit

final class NavigatorModelTests: XCTestCase {
    private func slice(
        status: String = "Todo", branch: String? = nil, handedBack: Bool = false, pr: String = "",
        blocked: Bool = false, fixing: Bool = false
    ) -> Slice {
        Slice(
            id: "s", name: "Slice", status: status, milestoneID: "M1", assignee: "", pr: pr, url: "",
            branch: branch, blocked: blocked, handedBack: handedBack, fixing: fixing)
    }

    private let prURL = "https://github.com/o/r/pull/40"

    // MARK: - Phase and defaults

    func testEachStateOpensTheDesignsSection() {
        let cases: [(Slice, AgentActivity?, Bool, NavigatorSection, MainPaneMode)] = [
            (slice(), nil, false, .thread, .empty),
            (slice(blocked: true), nil, false, .thread, .empty),
            (slice(status: "In progress"), .working, false, .thread, .terminal),
            (slice(status: "In progress"), .waiting, false, .thread, .terminal),
            (slice(status: "In progress", branch: "b", handedBack: true), nil, false, .changes, .diff),
            (slice(status: "In progress", branch: "b", pr: prURL), nil, false, .pr, .pr),
            (slice(status: "In progress", branch: "b", pr: prURL, fixing: true), .working, true, .thread, .terminal),
            (slice(status: "Done", branch: "b", pr: prURL), nil, false, .pr, .pr),
            (slice(status: "Done"), nil, false, .thread, .empty),
            (slice(status: "Done", branch: "b"), nil, false, .thread, .diff),
        ]
        for (index, (s, agent, _, phase, main)) in cases.enumerated() {
            let model = NavigatorModel(slice: s, agent: agent)
            XCTAssertEqual(model.phase, phase, "case \(index)")
            XCTAssertEqual(model.defaultOpen, [phase], "case \(index)")
            XCTAssertEqual(model.defaultMain, main, "case \(index)")
        }
    }

    func testSectionsAreLiveOnTheFactsTheyRead() {
        let todo = NavigatorModel(slice: slice(), agent: nil)
        XCTAssertTrue(todo.isLive(.thread), "it opens on the brief")
        XCTAssertFalse(todo.isLive(.changes))
        XCTAssertFalse(todo.isLive(.pr))
        XCTAssertFalse(todo.agentAvailable)
        XCTAssertFalse(todo.diffAvailable)

        let approved = NavigatorModel(slice: slice(status: "In progress", branch: "b", pr: prURL), agent: nil)
        XCTAssertTrue(approved.isLive(.changes))
        XCTAssertTrue(approved.isLive(.pr))
        XCTAssertTrue(approved.agentAvailable)
        XCTAssertTrue(approved.diffAvailable)
    }

    func testEachSectionPutsUpItsOwnMainView() {
        let todo = NavigatorModel(slice: slice(), agent: nil)
        XCTAssertEqual(NavigatorSection.allCases.map { todo.mainMode(for: $0) }, [nil, nil, nil, nil])

        let approved = NavigatorModel(slice: slice(status: "In progress", branch: "b", pr: prURL), agent: nil)
        XCTAssertEqual(NavigatorSection.allCases.map { approved.mainMode(for: $0) }, [.terminal, .diff, nil, .pr])

        let shown = NavigatorModel(
            slice: slice(status: "In progress", branch: "b", pr: prURL), agent: nil, hasVisuals: true)
        XCTAssertEqual(NavigatorSection.allCases.map { shown.mainMode(for: $0) }, [.terminal, .diff, .visuals, .pr])
    }

    func testVisualChangesAreLiveOnlyWithImagesAndSendOnlyToALiveAgent() {
        let none = NavigatorModel(slice: slice(status: "In progress"), agent: .working)
        XCTAssertFalse(none.isLive(.visuals))
        XCTAssertFalse(none.showsVisualActions)

        let handedIn = NavigatorModel(
            slice: slice(status: "In progress", branch: "b", handedBack: true), agent: nil, hasVisuals: true)
        XCTAssertTrue(handedIn.isLive(.visuals))
        XCTAssertFalse(handedIn.showsVisualActions, "no agent to send to")
        XCTAssertEqual(handedIn.phase, .changes, "images move neither the phase")
        XCTAssertEqual(handedIn.defaultMain, .diff, "nor the default view")

        let live = NavigatorModel(slice: slice(status: "In progress"), agent: .waiting, hasVisuals: true)
        XCTAssertTrue(live.showsVisualActions)
    }

    // MARK: - Header clicks

    func testTheChevronOnlyFolds() {
        let focus = NavigatorFocus(open: [.changes], main: .diff)
        XCTAssertEqual(focus.togglingFold(.changes), NavigatorFocus(open: [], main: .diff))
        XCTAssertEqual(focus.togglingFold(.pr), NavigatorFocus(open: [.changes, .pr], main: .diff))
    }

    func testAHeadOpensItsSectionAndPutsItsViewUp() {
        let focus = NavigatorFocus(open: [.thread], main: .terminal)
        XCTAssertEqual(focus.clickingHead(.changes, shows: .diff), NavigatorFocus(open: [.thread, .changes], main: .diff))
    }

    func testAHeadAlreadyOpenWithItsViewUpFolds() {
        let focus = NavigatorFocus(open: [.changes], main: .diff)
        XCTAssertEqual(focus.clickingHead(.changes, shows: .diff), NavigatorFocus(open: [], main: .diff))
    }

    func testAnOpenHeadWhoseViewIsNotUpPutsItUpAndStaysOpen() {
        let focus = NavigatorFocus(open: [.changes, .thread], main: .terminal)
        XCTAssertEqual(focus.clickingHead(.changes, shows: .diff), NavigatorFocus(open: [.changes, .thread], main: .diff))
    }

    func testAHeadWithNoViewOfItsOwnJustFolds() {
        let focus = NavigatorFocus(open: [.thread], main: .empty)
        XCTAssertEqual(focus.clickingHead(.thread, shows: nil), NavigatorFocus(open: [], main: .empty))
        XCTAssertEqual(NavigatorFocus(open: [], main: .empty).clickingHead(.thread, shows: nil),
                       NavigatorFocus(open: [.thread], main: .empty))
    }

    // MARK: - Main-pane tabs

    func testATabOpensItsSectionAndPutsItsViewUpWithoutEverFolding() {
        let focus = NavigatorFocus(open: [.thread], main: .terminal)
        XCTAssertEqual(focus.showing(.changes, shows: .diff), NavigatorFocus(open: [.thread, .changes], main: .diff))
        XCTAssertEqual(focus.showing(.thread, shows: .terminal), focus, "already up: stays open")
        XCTAssertEqual(focus.showing(.pr, shows: nil), focus, "no view of its own: nothing changes")
    }

    func testEachTabStandsForItsSectionsView() {
        XCTAssertEqual(MainPaneTab.allCases.map(\.label), ["Terminal", "Changes", "Visual changes", "PR"])
        XCTAssertEqual(MainPaneTab.allCases.map(\.section), [.thread, .changes, .visuals, .pr])
        XCTAssertEqual(MainPaneTab.allCases.map(\.mode), [.terminal, .diff, .visuals, .pr])
    }

    func testTheVisualChangesTabShowsOnlyWithImagesBetweenChangesAndPR() {
        let reviewed = slice(status: "In progress", branch: "b", pr: prURL)
        XCTAssertEqual(NavigatorModel(slice: reviewed, agent: nil).tabs,
                       [.terminal, .changes, .pr])
        XCTAssertEqual(NavigatorModel(slice: reviewed, agent: nil, hasVisuals: true).tabs,
                       [.terminal, .changes, .visuals, .pr])
        XCTAssertEqual(NavigatorModel(slice: slice(status: "In progress"), agent: .working,
                                      hasVisuals: true).tabs, [.terminal, .visuals])
        XCTAssertEqual(NavigatorModel(slice: slice(), agent: nil, hasVisuals: true).tabs,
                       [.visuals], "images handed in on a slice with no agent or branch")
    }

    func testASlicesTabsAreTheSectionsThatPutAViewUp() {
        XCTAssertEqual(NavigatorModel(slice: slice(), agent: nil).tabs, [])
        XCTAssertEqual(NavigatorModel(slice: slice(status: "In progress"), agent: .working).tabs,
                       [.terminal])
        XCTAssertEqual(NavigatorModel(slice: slice(status: "In progress", branch: "b", handedBack: true),
                                      agent: nil).tabs, [.terminal, .changes])
        XCTAssertEqual(NavigatorModel(slice: slice(status: "In progress", branch: "b", pr: prURL),
                                      agent: nil).tabs, [.terminal, .changes, .pr])
    }

    func testASessionsTabsWaitForAPRBeforeShowingOne() {
        XCTAssertEqual(MainPaneTab.forSession(hasPRs: false), [.terminal, .changes])
        XCTAssertEqual(MainPaneTab.forSession(hasPRs: true), [.terminal, .changes, .pr])
    }

    // MARK: - Header actions

    func testLaunchIsOfferedBeforeLaunchAndToRelaunchAWorkingSlice() {
        let todo = NavigatorModel(slice: slice(), agent: nil)
        XCTAssertTrue(todo.showsLaunch)
        XCTAssertTrue(todo.canLaunch)
        XCTAssertTrue(todo.launchIsPrimary)

        let blocked = NavigatorModel(slice: slice(blocked: true), agent: nil)
        XCTAssertTrue(blocked.showsLaunch, "drawn disabled, as the design draws it")
        XCTAssertFalse(blocked.canLaunch)
        XCTAssertFalse(blocked.launchIsPrimary)

        let stalled = NavigatorModel(slice: slice(status: "In progress"), agent: nil)
        XCTAssertTrue(stalled.showsLaunch)
        XCTAssertFalse(stalled.launchIsPrimary)

        let fixing = NavigatorModel(slice: slice(status: "In progress", pr: prURL, fixing: true), agent: nil)
        XCTAssertTrue(fixing.showsLaunch)
        XCTAssertFalse(fixing.launchIsFix, "a fix already under way relaunches")

        // Approved, at its pull request with nobody on it: a fix launch.
        let approved = NavigatorModel(slice: slice(status: "In progress", pr: prURL), agent: nil)
        XCTAssertTrue(approved.showsLaunch)
        XCTAssertTrue(approved.canLaunch)
        XCTAssertTrue(approved.launchIsFix)
        XCTAssertFalse(NavigatorModel(slice: slice(status: "In progress", pr: prURL), agent: .working).showsLaunch)

        let live = NavigatorModel(slice: slice(status: "In progress"), agent: .working)
        XCTAssertFalse(live.showsLaunch)

        for handed in [
            slice(status: "In progress", branch: "b", handedBack: true),
            slice(status: "Done", pr: prURL),
        ] {
            XCTAssertFalse(NavigatorModel(slice: handed, agent: nil).showsLaunch)
        }
        XCTAssertFalse(
            NavigatorModel(slice: slice(status: "In progress"), agent: .waiting).showsLaunch)
    }

    func testReviewActionsAndMergeFollowTheState() {
        let review = NavigatorModel(slice: slice(status: "In progress", branch: "b", handedBack: true), agent: nil)
        XCTAssertTrue(review.showsReviewActions)
        XCTAssertFalse(review.showsMerge)

        let approved = NavigatorModel(slice: slice(status: "In progress", pr: prURL), agent: nil)
        XCTAssertFalse(approved.showsReviewActions)
        XCTAssertTrue(approved.showsMerge)

        let done = NavigatorModel(slice: slice(status: "Done", pr: prURL), agent: nil)
        XCTAssertFalse(done.showsMerge)
    }

    func testThePRHeaderSaysMergedOnlyOnceTheSliceIsDoneWithAPR() {
        let merged = NavigatorModel(slice: slice(status: "Done", pr: prURL), agent: nil)
        XCTAssertEqual(merged.prStatus, .merged)
        XCTAssertEqual(merged.prStatus?.label, "Merged")

        // A fix session on an approved slice is fixing, not done, and so
        // not merged.
        let fixing = NavigatorModel(slice: slice(status: "In progress", pr: prURL, fixing: true), agent: .working)
        XCTAssertNil(fixing.prStatus)

        let approved = NavigatorModel(slice: slice(status: "In progress", pr: prURL), agent: nil)
        XCTAssertNil(approved.prStatus)

        let closed = NavigatorModel(slice: slice(status: "Done"), agent: nil)
        XCTAssertNil(closed.prStatus)
    }

    func testEverySectionHasItsLabel() {
        XCTAssertEqual(NavigatorSection.allCases.map(\.label), ["Task", "Changes", "Visual changes", "PR"])
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
            ThreadEvent(.launched, who: "Launched", facts: [
                ThreadFact("model", "Opus 5.5"), ThreadFact("effort", "medium"), ThreadFact("branch", "slice/x"),
            ]),
            ThreadEvent(.agent, who: "Agent", meta: "working", tone: .accent, facts: [ThreadFact("context", "37%")]),
        ])
    }

    func testAWaitingAgentIsHotAndAnUnreadReadingSaysNothing() {
        let events = buildThreadEvents(slice: slice(status: "In progress"), agent: agent(.waiting, model: ""), brief: nil)
        XCTAssertEqual(events, [
            ThreadEvent(.launched, who: "Launched"),
            ThreadEvent(.agent, who: "Agent", meta: "waiting for you", tone: .hot),
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
        XCTAssertEqual(events.last, ThreadEvent(.handedBack, who: "Agent", meta: "handed back", body: "Second pass, **done**."))
    }

    func testAnApprovedAndMergedSliceReadsToTheEnd() {
        let events = buildThreadEvents(slice: slice(status: "Done", branch: "b", pr: prURL), agent: nil, brief: nil)
        XCTAssertEqual(events.map(\.who), ["Launched", "Agent", "You", "Merged"])
        XCTAssertEqual(events[2], ThreadEvent(
            .approved, who: "You", meta: "approved", facts: [ThreadFact("pr", "#40"), ThreadFact("into", "main")]))
    }

    func testAPullRequestWithNoNumberStillReadsApproved() {
        let events = buildThreadEvents(slice: slice(status: "In progress", pr: "https://example.com/x"), agent: nil, brief: nil)
        XCTAssertEqual(events.last, ThreadEvent(
            .approved, who: "You", meta: "approved", facts: [ThreadFact("pr", "https://example.com/x")]))
    }

    func testASliceClosedWithNoBranchSaysClosedWithItsSummary() {
        let brief = "Look into it.\n\n### Summary\n\nNothing to change; wrote it up.\n"
        let events = buildThreadEvents(slice: slice(status: "Done"), agent: nil, brief: brief)
        XCTAssertEqual(events, [
            ThreadEvent(.launched, who: "Launched"),
            ThreadEvent(.closed, who: "Closed", body: "Nothing to change; wrote it up."),
        ])
        XCTAssertEqual(
            buildThreadEvents(slice: slice(status: "Done"), agent: nil, brief: nil).last,
            ThreadEvent(.closed, who: "Closed"))
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
