import XCTest
@testable import NatKit

final class WorkflowTabsTests: XCTestCase {
    func testWorkflowTab_symbolNames() {
        XCTAssertEqual(WorkflowTab.brief.symbolName, "doc.text")
        XCTAssertEqual(WorkflowTab.agent.symbolName, "chevron.left.forwardslash.chevron.right")
        XCTAssertEqual(WorkflowTab.diff.symbolName, "plus.forwardslash.minus")
        XCTAssertEqual(WorkflowTab.pr.symbolName, "arrow.branch")
    }

    func testBuildWorkflowTabState_todoSliceNoAgent() {
        let slice = Slice(
            id: "s-1", name: "Task", status: "Todo", milestoneID: "m-1",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )

        let state = buildWorkflowTabState(for: slice, hasLiveAgent: false)

        XCTAssertEqual(state.reachable, [.brief])
        XCTAssertEqual(state.defaultTab, .brief)
    }

    func testBuildWorkflowTabState_withLiveAgent() {
        let slice = Slice(
            id: "s-1", name: "Task", status: "In progress", milestoneID: "m-1",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )

        let state = buildWorkflowTabState(for: slice, hasLiveAgent: true)

        XCTAssert(state.isReachable(.brief))
        XCTAssert(state.isReachable(.agent))
        XCTAssertFalse(state.isReachable(.diff))
        XCTAssertFalse(state.isReachable(.pr))
        XCTAssertEqual(state.defaultTab, .agent)
    }

    func testBuildWorkflowTabState_inProgress() {
        let slice = Slice(
            id: "s-1", name: "Task", status: "In progress", milestoneID: "m-1",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )

        let state = buildWorkflowTabState(for: slice, hasLiveAgent: false)

        XCTAssert(state.isReachable(.brief))
        XCTAssert(state.isReachable(.agent))
        XCTAssertEqual(state.defaultTab, .agent)
    }

    func testBuildWorkflowTabState_handedBack() {
        let slice = Slice(
            id: "s-1", name: "Task", status: "In progress", milestoneID: "m-1",
            assignee: "", pr: "", url: "", branch: "feature-x", blocked: false, handedBack: true
        )

        let state = buildWorkflowTabState(for: slice, hasLiveAgent: false)

        XCTAssert(state.isReachable(.brief))
        XCTAssert(state.isReachable(.agent))
        XCTAssert(state.isReachable(.diff))
        XCTAssertFalse(state.isReachable(.pr))
        XCTAssertEqual(state.defaultTab, .diff)
    }

    /// Completion is the slice's progress, not the tab on screen: a
    /// handed-back slice has been through Brief and Agent, and both stay
    /// ticked whichever tab the user is looking at — there is no "current
    /// tab" input for it to lose the tick to.
    func testIsComplete_marksTheStagesBehindTheSlicesProgress() {
        let slice = Slice(
            id: "s-1", name: "Task", status: "In progress", milestoneID: "m-1",
            assignee: "", pr: "", url: "", branch: "feature-x", blocked: false, handedBack: true
        )

        let state = buildWorkflowTabState(for: slice, hasLiveAgent: false)

        XCTAssert(state.isComplete(.brief))
        XCTAssert(state.isComplete(.agent))
        XCTAssertFalse(state.isComplete(.diff))
        XCTAssertFalse(state.isComplete(.pr))
    }

    /// A live agent moves the default tab back to Agent — where to look, not
    /// how far the slice got. The branch is still handed back, Diff is still
    /// unlocked, and the stages behind that stay ticked.
    func testIsComplete_liveAgentOnAHandedBackSliceKeepsAgentTicked() {
        let slice = Slice(
            id: "s-1", name: "Task", status: "In progress", milestoneID: "m-1",
            assignee: "", pr: "", url: "", branch: "feature-x", blocked: false, handedBack: true
        )

        let state = buildWorkflowTabState(for: slice, hasLiveAgent: true)

        XCTAssertEqual(state.defaultTab, .agent)
        XCTAssert(state.isReachable(.diff))
        XCTAssert(state.isComplete(.brief))
        XCTAssert(state.isComplete(.agent))
        XCTAssertFalse(state.isComplete(.diff))
        XCTAssertFalse(state.isComplete(.pr))
    }

    /// A Done slice whose pull request is still open: the branch is still
    /// there to read — approving opened the pull request, it did not end the
    /// review — and the slice's own state is the pull request, so that is
    /// the tab a click lands on, a lingering agent session notwithstanding:
    /// a session can outlive the slice it was launched on, and an idle one
    /// must not steal the landing from the stage the slice is actually at.
    func testBuildWorkflowTabState_doneSliceWithPRLandsOnPRAndKeepsItsDiff() {
        let slice = Slice(
            id: "s-1", name: "Task", status: "Done", milestoneID: "m-1",
            assignee: "", pr: "https://github.com/x/y/pull/9", url: "",
            branch: "slice/task", blocked: false, handedBack: false
        )

        for hasLiveAgent in [false, true] {
            let state = buildWorkflowTabState(for: slice, hasLiveAgent: hasLiveAgent)
            XCTAssertEqual(state.defaultTab, .pr, "live agent: \(hasLiveAgent)")
            XCTAssert(state.isReachable(.diff), "live agent: \(hasLiveAgent)")
            XCTAssert(state.isReachable(.pr))
        }
    }

    /// A Done slice that never recorded a branch — finished before there was
    /// a Branch column — has nothing for the Diff tab to read.
    func testBuildWorkflowTabState_doneSliceWithoutABranchKeepsDiffLocked() {
        let slice = Slice(
            id: "s-1", name: "Task", status: "Done", milestoneID: "m-1",
            assignee: "", pr: "https://github.com/x/y/pull/9", url: "", blocked: false, handedBack: false
        )

        let state = buildWorkflowTabState(for: slice, hasLiveAgent: false)

        XCTAssertFalse(state.isReachable(.diff))
        XCTAssertEqual(state.defaultTab, .pr)
    }

    func testIsComplete_freshSliceHasCompletedNothing() {
        let slice = Slice(
            id: "s-1", name: "Task", status: "Todo", milestoneID: "m-1",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )

        let state = buildWorkflowTabState(for: slice, hasLiveAgent: false)

        for tab in state.tabs {
            XCTAssertFalse(state.isComplete(tab))
        }
    }

    func testIsComplete_prRecordedTicksEverythingBeforeIt() {
        let slice = Slice(
            id: "s-1", name: "Task", status: "Done", milestoneID: "m-1",
            assignee: "", pr: "https://github.com/x/y/pull/1", url: "", blocked: false, handedBack: false
        )

        let state = buildWorkflowTabState(for: slice, hasLiveAgent: false)

        XCTAssert(state.isComplete(.brief))
        XCTAssert(state.isComplete(.agent))
        XCTAssert(state.isComplete(.diff))
        XCTAssertFalse(state.isComplete(.pr))
    }

    func testBuildWorkflowTabState_withPR() {
        let slice = Slice(
            id: "s-1", name: "Task", status: "Done", milestoneID: "m-1",
            assignee: "", pr: "https://github.com/...", url: "", blocked: false, handedBack: false
        )

        let state = buildWorkflowTabState(for: slice, hasLiveAgent: false)

        XCTAssert(state.isReachable(.brief))
        XCTAssertFalse(state.isReachable(.agent))
        XCTAssertFalse(state.isReachable(.diff))
        XCTAssert(state.isReachable(.pr))
        XCTAssertEqual(state.defaultTab, .pr)
    }

    func testBuildWorkflowTabState_allReachable() {
        let slice = Slice(
            id: "s-1", name: "Task", status: "In progress", milestoneID: "m-1",
            assignee: "", pr: "https://github.com/...", url: "", branch: "feature-x", blocked: false, handedBack: true
        )

        let state = buildWorkflowTabState(for: slice, hasLiveAgent: true)

        XCTAssert(state.isReachable(.brief))
        XCTAssert(state.isReachable(.agent))
        XCTAssert(state.isReachable(.diff))
        XCTAssert(state.isReachable(.pr))
        XCTAssertEqual(state.defaultTab, .agent)
    }

    func testBuildWorkflowTabState_handedBackWithoutLiveAgent() {
        let slice = Slice(
            id: "s-1", name: "Task", status: "In progress", milestoneID: "m-1",
            assignee: "", pr: "", url: "", branch: "feature-x", blocked: false, handedBack: true
        )

        let state = buildWorkflowTabState(for: slice, hasLiveAgent: false)

        XCTAssertEqual(state.defaultTab, .diff)
    }

    func testBuildWorkflowTabState_emptyPRString() {
        let slice = Slice(
            id: "s-1", name: "Task", status: "Done", milestoneID: "m-1",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )

        let state = buildWorkflowTabState(for: slice, hasLiveAgent: false)

        XCTAssertFalse(state.isReachable(.pr))
    }

    func testBuildWorkflowTabState_allTabs() {
        let slice = Slice(
            id: "s-1", name: "Task", status: "Todo", milestoneID: "m-1",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )

        let state = buildWorkflowTabState(for: slice, hasLiveAgent: false)

        XCTAssertEqual(state.tabs, WorkflowTab.allCases)
    }
}
