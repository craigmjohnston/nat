import XCTest
@testable import NatKit

final class WorkflowStageTests: XCTestCase {
    private func slice(
        status: String, branch: String?, handedBack: Bool, pr: String, id: String = "s-1"
    ) -> Slice {
        Slice(
            id: id, name: "Task", status: status, milestoneID: "m-1", assignee: "", pr: pr, url: "",
            branch: branch, blocked: false, handedBack: handedBack
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

    /// Every combination of status, branch, PR, live agent and fix mark: the
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
                                handedBack: handedBack, pr: hasPR ? "https://pr/1" : ""
                            )
                            let want = expected(status: status, handedBack: handedBack, pr: hasPR, mark: mark)
                            let got = stage(for: s, agent: agent, fixLaunched: mark)
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
                                buildWorkflowTabState(for: s, hasLiveAgent: agent != nil, fixLaunched: mark).defaultTab,
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
        XCTAssertEqual(stage(for: handedBack, agent: .waiting, fixLaunched: false).tab(for: handedBack), .diff)

        let approved = slice(status: "In progress", branch: "b", handedBack: false, pr: "https://pr/1")
        XCTAssertEqual(stage(for: approved, agent: .working, fixLaunched: false).tab(for: approved), .pr)

        let done = slice(status: "Done", branch: "b", handedBack: false, pr: "https://pr/1")
        XCTAssertEqual(stage(for: done, agent: .working, fixLaunched: true).tab(for: done), .pr)

        // Reworked: Branch cleared, so back to working, then review again.
        let reworked = slice(status: "In progress", branch: nil, handedBack: false, pr: "")
        XCTAssertEqual(stage(for: reworked, agent: .working, fixLaunched: false).tab(for: reworked), .agent)
        XCTAssertEqual(stage(for: handedBack, agent: .working, fixLaunched: false).tab(for: handedBack), .diff)
    }

    func testRailAgreesWithTheStage() {
        let fixing = slice(status: "In progress", branch: nil, handedBack: false, pr: "https://pr/1")
        XCTAssertTrue(isActiveSlice(fixing, fixLaunched: true))
        XCTAssertFalse(isReviewSlice(fixing, fixLaunched: true))
        XCTAssertTrue(isReviewSlice(fixing))
        XCTAssertFalse(isActiveSlice(fixing))
        XCTAssertEqual(inFlightSliceIDs(slices: [fixing], fixLaunched: ["s-1"]), ["s-1"])

        let info = ProjectInfo(
            project: Project(id: "p", name: "P", conventions: ""),
            milestones: [Milestone(id: "m-1", name: "M1", order: 1, status: "Active")],
            slices: [fixing]
        )
        let entries = buildRailModel(
            from: info, liveAgents: ["s-1": .waiting], fixLaunched: ["s-1"]
        ).active
        XCTAssertEqual(entries.map(\.displayState), ["Waiting for input"])
        XCTAssertEqual(
            buildRailModel(from: info, liveAgents: ["s-1": .waiting]).active.map(\.displayState),
            ["Needs review"]
        )
    }

    func testAttentionCountsAStageOnce() {
        let handedBack = slice(status: "In progress", branch: "b", handedBack: true, pr: "")
        let attention = projectAttention(slices: [handedBack], liveAgents: ["s-1": .waiting])
        XCTAssertEqual(attention.count, 1)
        let fixing = slice(status: "In progress", branch: nil, handedBack: false, pr: "https://pr/1")
        XCTAssertEqual(
            projectAttention(
                slices: [fixing], liveAgents: [:], prReadiness: ["s-1": PRStatusSlice.readyToMerge],
                fixLaunched: ["s-1"]
            ).count,
            0
        )
    }
}

final class FixLaunchedTests: XCTestCase {
    @MainActor
    func testMarkClearsOnlyAfterTheSessionWasSeenAndThenIsGone() {
        let model = AppModel()
        model.markFixLaunched(sliceID: "s-1")
        XCTAssertEqual(model.fixLaunchedSliceIDs, ["s-1"])

        model.settleFixLaunched(liveSliceIDs: [])
        XCTAssertEqual(model.fixLaunchedSliceIDs, ["s-1"], "no session yet: launch has not shown up in tmux")

        model.settleFixLaunched(liveSliceIDs: ["s-1", "other"])
        XCTAssertEqual(model.fixLaunchedSliceIDs, ["s-1"])

        model.settleFixLaunched(liveSliceIDs: ["other"])
        XCTAssertTrue(model.fixLaunchedSliceIDs.isEmpty)
    }

    @MainActor
    func testActivityPollDrivesTheClearing() async {
        let client = MockActivityClient(response: .agents([]))
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
        model.markFixLaunched(sliceID: "s-1")
        model.settleFixLaunched(liveSliceIDs: ["s-1"])
        model.activityStore?.kick()
        for _ in 0..<100 where !model.fixLaunchedSliceIDs.isEmpty {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        XCTAssertTrue(model.fixLaunchedSliceIDs.isEmpty)
    }
}
