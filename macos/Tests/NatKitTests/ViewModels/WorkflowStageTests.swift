import XCTest
@testable import NatKit

final class WorkflowStageTests: XCTestCase {
    private func slice(
        status: String, branch: String?, handedBack: Bool, pr: String, id: String = "s-1", fixing: Bool = false
    ) -> Slice {
        Slice(
            id: id, name: "Task", status: status, milestoneID: "m-1", assignee: "", pr: pr, url: "",
            branch: branch, blocked: false, handedBack: handedBack, fixing: fixing
        )
    }

    /// The expected stage for one combination of the facts, written out from
    /// the transition table rather than from the implementation.
    private func expected(status: String, handedBack: Bool, pr: Bool, mark: Bool) -> WorkflowStage {
        switch status {
        case "Done": return .done
        case "In progress":
            if pr { return mark ? .fixing : .pr }
            return handedBack ? .review : .working
        default: return .todo
        }
    }

    /// Every combination of status, branch, PR, live agent and nat's fixing: the
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
                                handedBack: handedBack, pr: hasPR ? "https://pr/1" : "", fixing: mark
                            )
                            let want = expected(status: status, handedBack: handedBack, pr: hasPR, mark: mark)
                            let got = stage(for: s, agent: agent)
                            XCTAssertEqual(got, want, "\(status) branch:\(hasBranch) pr:\(hasPR) \(String(describing: agent)) mark:\(mark)")

                            let tab: WorkflowTab
                            switch want {
                            case .todo: tab = .brief
                            case .working, .fixing: tab = .agent
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
        XCTAssertEqual(WorkflowStage.allCases.count, 6)
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

        let done = slice(status: "Done", branch: "b", handedBack: false, pr: "https://pr/1", fixing: true)
        XCTAssertEqual(stage(for: done, agent: .working).tab(for: done), .pr)

        // Reworked: Branch cleared, so back to working, then review again.
        let reworked = slice(status: "In progress", branch: nil, handedBack: false, pr: "")
        XCTAssertEqual(stage(for: reworked, agent: .working).tab(for: reworked), .agent)
        XCTAssertEqual(stage(for: handedBack, agent: .working).tab(for: handedBack), .diff)
    }

    func testRailAgreesWithTheStage() {
        let fixing = slice(status: "In progress", branch: nil, handedBack: false, pr: "https://pr/1", fixing: true)
        let approved = slice(status: "In progress", branch: nil, handedBack: false, pr: "https://pr/1")
        XCTAssertTrue(isActiveSlice(fixing))
        XCTAssertFalse(isReviewSlice(fixing))
        XCTAssertTrue(isReviewSlice(approved))
        XCTAssertFalse(isActiveSlice(approved))
        XCTAssertEqual(inFlightSliceIDs(slices: [fixing]), ["s-1"])

        let info = ProjectInfo(
            project: Project(id: "p", name: "P", conventions: ""),
            milestones: [Milestone(id: "m-1", name: "M1", order: 1, status: "Active")],
            slices: [fixing]
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
        let fixing = slice(status: "In progress", branch: nil, handedBack: false, pr: "https://pr/1", fixing: true)
        XCTAssertEqual(
            projectAttention(
                slices: [fixing], liveAgents: [:], prReadiness: ["s-1": PRStatusSlice.readyToMerge]
            ).count,
            0
        )
    }

    /// A pull request read failing its checks counts once, at the PR stage or
    /// under a fix — waiting agent and all — and not on a slice anywhere else.
    func testAttentionCountsFailingChecksOnce() {
        let red = ["s-1": PRStatusSlice.checksFailing]
        let approved = slice(status: "In progress", branch: nil, handedBack: false, pr: "https://pr/1")
        let fixing = slice(status: "In progress", branch: nil, handedBack: false, pr: "https://pr/1", fixing: true)
        let done = slice(status: "Done", branch: nil, handedBack: false, pr: "https://pr/1")
        XCTAssertEqual(projectAttention(slices: [approved], liveAgents: [:], prReadiness: red).count, 1)
        XCTAssertEqual(projectAttention(slices: [approved], liveAgents: [:], prReadiness: red).role, .review)
        XCTAssertEqual(projectAttention(slices: [fixing], liveAgents: ["s-1": .waiting], prReadiness: red).count, 1)
        XCTAssertEqual(projectAttention(slices: [done], liveAgents: [:], prReadiness: red).count, 0)
    }

    /// A fix launch: an approved slice with no live agent launches whatever
    /// its dependencies say; a live agent still refuses.
    func testLaunchPlanAdmitsAFixLaunch() {
        let approved = Slice(
            id: "s-1", name: "T", status: "In progress", milestoneID: "m", assignee: "", pr: "https://pr/1", url: "",
            dependsOn: ["s-0"], blocked: true, handedBack: false)
        let plan = LaunchPlan(for: approved, hasLiveAgent: false)
        XCTAssertTrue(plan.canLaunch)
        XCTAssertTrue(plan.isFix)
        XCTAssertNil(plan.blockedBy)
        XCTAssertFalse(LaunchPlan(for: approved, hasLiveAgent: true).canLaunch)
        let blocked = Slice(
            id: "s-2", name: "T", status: "Todo", milestoneID: "m", assignee: "", pr: "", url: "",
            dependsOn: ["s-0"], blocked: true, handedBack: false)
        XCTAssertFalse(LaunchPlan(for: blocked, hasLiveAgent: false).canLaunch)
        XCTAssertFalse(LaunchPlan(for: blocked, hasLiveAgent: false).isFix)
    }

}
