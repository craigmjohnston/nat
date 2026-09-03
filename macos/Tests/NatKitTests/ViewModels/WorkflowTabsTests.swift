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
