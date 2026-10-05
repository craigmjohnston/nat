import XCTest
@testable import NatKit

final class WorkflowStageTests: XCTestCase {
    private func slice(
        status: String, branch: String?, handedBack: Bool, pr: String, id: String = "s-1", resumed: Bool = false
    ) -> Slice {
        Slice(
            id: id, name: "Task", status: status, milestoneID: "m-1", assignee: "", pr: pr, url: "",
            branch: branch, blocked: false, handedBack: handedBack, resumed: resumed
        )
    }

    /// The expected stage for one combination of the facts, written out from
    /// the transition table rather than from the implementation.
    private func expected(status: String, handedBack: Bool, pr: Bool, mark: Bool) -> WorkflowStage {
        switch status {
        case "Done": return .done
        case "In progress":
            // Resumed is nat's reading, and working whatever the PR says.
            if mark { return .working }
            if pr { return .pr }
            return handedBack ? .review : .working
        default: return .todo
        }
    }

    /// Every combination of status, branch, PR, live agent and nat's resumed: the
    /// stage, and the tab it lands on. A live agent must change nothing.
    func testEveryCombinationOfFactsHasTheTabsStage() {
        var count = 0
        for status in ["Todo", "In progress", "Done"] {
            for hasBranch in [false, true] {
                for hasPR in [false, true] {
                    for agent in [nil, AgentActivity.working, .waiting] {
                        for mark in [false, true] {
                            // The slice's own reading: handed back is an In
                            // progress slice with a branch and no PR.
                            let handedBack = status == "In progress" && hasBranch && !hasPR
                            let s = slice(
                                status: status, branch: hasBranch ? "slice/x" : nil,
                                handedBack: handedBack, pr: hasPR ? "https://pr/1" : "", resumed: mark
                            )
                            let want = expected(status: status, handedBack: handedBack, pr: hasPR, mark: mark)
                            let got = stage(for: s, agent: agent)
                            XCTAssertEqual(got, want, "\(status) branch:\(hasBranch) pr:\(hasPR) \(String(describing: agent)) mark:\(mark)")

                            let tab: WorkflowTab
                            switch want {
                            case .todo: tab = .brief
                            case .working: tab = .agent
                            case .review: tab = .diff
                            case .pr: tab = .pr
                            case .done: tab = hasPR ? .pr : .brief
                            }
                            XCTAssertEqual(got.tab(for: s), tab)
                            XCTAssertEqual(
                                buildWorkflowTabState(for: s, hasLiveAgent: agent != nil).defaultTab,
                                tab
                            )
                            count += 1
                        }
                    }
                }
            }
        }
        XCTAssertEqual(count, 72)
    }

    /// Adding a case without a tab fails here: every case must have one, and
    /// the set of cases is pinned so the table in the doc comment is kept.
    func testEveryStageHasATab() {
        XCTAssertEqual(WorkflowStage.allCases.count, 5)
        for stage in WorkflowStage.allCases {
            let tab = stage.tab(hasPR: true)
            XCTAssertTrue(WorkflowTab.allCases.contains(tab), "\(stage)")
        }
        XCTAssertEqual(WorkflowStage.done.tab(hasPR: false), .brief)
    }

    func testAcceptanceScenarios() {
        let handedBack = slice(status: "In progress", branch: "b", handedBack: true, pr: "")
        XCTAssertEqual(stage(for: handedBack, agent: .waiting).tab(for: handedBack), .diff)

        let approved = slice(status: "In progress", branch: "b", handedBack: false, pr: "https://pr/1")
        XCTAssertEqual(stage(for: approved, agent: .working).tab(for: approved), .pr)

        let done = slice(status: "Done", branch: "b", handedBack: false, pr: "https://pr/1", resumed: true)
        XCTAssertEqual(stage(for: done, agent: .working).tab(for: done), .pr)

        // Reworked: Branch cleared, so back to working, then review again.
        let reworked = slice(status: "In progress", branch: nil, handedBack: false, pr: "")
        XCTAssertEqual(stage(for: reworked, agent: .working).tab(for: reworked), .agent)
        XCTAssertEqual(stage(for: handedBack, agent: .working).tab(for: handedBack), .diff)

        // Resumed after approval: Branch cleared, the PR kept — working, its
        // agent's terminal the tab, until the hand-back that follows puts it
        // back at its pull request.
        let resumed = slice(status: "In progress", branch: nil, handedBack: false, pr: "https://pr/1", resumed: true)
        XCTAssertEqual(stage(for: resumed, agent: nil), .working)
        XCTAssertEqual(stage(for: resumed, agent: nil).tab(for: resumed), .agent)
        let backAgain = slice(status: "In progress", branch: "b", handedBack: false, pr: "https://pr/1")
        XCTAssertEqual(stage(for: backAgain, agent: .waiting), .pr)
        // A project with no Branch column holds a PR and no branch, and nat
        // does not call that resumed: it stays at its pull request.
        let noBranchColumn = slice(status: "In progress", branch: nil, handedBack: false, pr: "https://pr/1")
        XCTAssertEqual(stage(for: noBranchColumn, agent: nil), .pr)
    }

    func testRailAgreesWithTheStage() {
        let resumed = slice(status: "In progress", branch: nil, handedBack: false, pr: "https://pr/1", resumed: true)
        let approved = slice(status: "In progress", branch: nil, handedBack: false, pr: "https://pr/1")
        XCTAssertTrue(isActiveSlice(resumed))
        XCTAssertFalse(isReviewSlice(resumed))
        XCTAssertTrue(isReviewSlice(approved))
        XCTAssertFalse(isActiveSlice(approved))
        XCTAssertEqual(inFlightSliceIDs(slices: [resumed]), ["s-1"])

        let info = ProjectInfo(
            project: Project(id: "p", name: "P", conventions: ""),
            milestones: [Milestone(id: "m-1", name: "M1", order: 1, status: "Active")],
            slices: [resumed]
        )
        let entries = buildRailModel(from: info, liveAgents: ["s-1": .waiting]).active
        XCTAssertEqual(entries.map(\.displayState), ["Waiting for input"])
        let approvedInfo = ProjectInfo(project: info.project, milestones: info.milestones, slices: [approved])
        XCTAssertEqual(
            buildRailModel(from: approvedInfo, liveAgents: ["s-1": .waiting]).active.map(\.displayState),
            ["Needs review"]
        )
    }

    func testAttentionCountsAStageOnce() {
        let handedBack = slice(status: "In progress", branch: "b", handedBack: true, pr: "")
        let attention = projectAttention(slices: [handedBack], liveAgents: ["s-1": .waiting])
        XCTAssertEqual(attention.count, 1)
        let resumed = slice(status: "In progress", branch: nil, handedBack: false, pr: "https://pr/1", resumed: true)
        XCTAssertEqual(
            projectAttention(
                slices: [resumed], liveAgents: [:], prReadiness: ["s-1": PRStatusSlice.readyToMerge]
            ).count,
            0
        )
    }

    /// A pull request read failing its checks counts once, at the PR stage —
    /// waiting agent and all — and not on a slice anywhere else: a resumed
    /// one's red reading is of a commit its agent is replacing.
    func testAttentionCountsFailingChecksOnce() {
        let red = ["s-1": PRStatusSlice.checksFailing]
        let approved = slice(status: "In progress", branch: nil, handedBack: false, pr: "https://pr/1")
        let resumed = slice(status: "In progress", branch: nil, handedBack: false, pr: "https://pr/1", resumed: true)
        let done = slice(status: "Done", branch: nil, handedBack: false, pr: "https://pr/1")
        XCTAssertEqual(projectAttention(slices: [approved], liveAgents: [:], prReadiness: red).count, 1)
        XCTAssertEqual(projectAttention(slices: [approved], liveAgents: [:], prReadiness: red).role, .review)
        XCTAssertEqual(projectAttention(slices: [approved], liveAgents: ["s-1": .waiting], prReadiness: red).count, 1)
        XCTAssertEqual(projectAttention(slices: [resumed], liveAgents: [:], prReadiness: red).count, 0)
        XCTAssertEqual(projectAttention(slices: [done], liveAgents: [:], prReadiness: red).count, 0)
    }

    /// An In progress slice with a PR and no live agent is an ordinary
    /// relaunch — its dependencies hold it back as any launch's do, as nat's
    /// own launch does now there is no fix launch; a live agent refuses, and
    /// a Done slice is never launched.
    func testLaunchPlanTreatsAnApprovedSliceAsAnOrdinaryRelaunch() {
        let approved = Slice(
            id: "s-1", name: "T", status: "In progress", milestoneID: "m", assignee: "", pr: "https://pr/1", url: "",
            blocked: false, handedBack: false)
        XCTAssertTrue(LaunchPlan(for: approved, hasLiveAgent: false).canLaunch)
        XCTAssertFalse(LaunchPlan(for: approved, hasLiveAgent: true).canLaunch)
        let blockedApproved = Slice(
            id: "s-1", name: "T", status: "In progress", milestoneID: "m", assignee: "", pr: "https://pr/1", url: "",
            dependsOn: ["s-0"], blocked: true, handedBack: false)
        XCTAssertFalse(LaunchPlan(for: blockedApproved, hasLiveAgent: false).canLaunch)
        XCTAssertEqual(LaunchPlan(for: blockedApproved, hasLiveAgent: false).blockedBy, ["s-0"])
        let done = Slice(
            id: "s-3", name: "T", status: "Done", milestoneID: "m", assignee: "", pr: "https://pr/1", url: "",
            blocked: false, handedBack: false)
        XCTAssertFalse(LaunchPlan(for: done, hasLiveAgent: false).canLaunch)
    }

}
