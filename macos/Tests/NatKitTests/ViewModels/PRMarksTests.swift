import XCTest
@testable import NatKit
@testable import NatFixtures

/// A pull request's conflict and failing checks: nat's words decoded, the
/// reading's maps, the sidebar's marks on both row kinds, and the PR
/// section's conflict notice.
final class PRMarksTests: XCTestCase {
    private func slice(
        _ id: String = "s-1", status: String = "In progress", pr: String = "https://github.test/o/r/pull/7",
        fixing: Bool = false, handedBack: Bool = false, branch: String? = nil
    ) -> Slice {
        Slice(
            id: id, name: "Slice \(id)", status: status, milestoneID: "M1", assignee: "", pr: pr, url: "",
            branch: branch, blocked: false, handedBack: handedBack, fixing: fixing)
    }

    // MARK: - Decoding

    func testPRStatusDecodesConflictingAndDefaultsItFalse() throws {
        let json = """
        {"slices": [
          {"slice_id": "s-1", "name": "A", "pr": "u", "readiness": "awaiting review", "conflicting": true, "base": "main"},
          {"slice_id": "s-2", "name": "B", "pr": "u", "readiness": "ready to merge", "conflicting": false},
          {"slice_id": "s-3", "name": "C", "pr": "u", "readiness": "unread"}
        ]}
        """
        let doc = try JSONDecoder().decode(PRStatusDoc.self, from: Data(json.utf8))
        XCTAssertEqual(doc.slices.map(\.conflicting), [true, false, false])
        XCTAssertEqual(doc.slices.map(\.base), ["main", nil, nil])
        XCTAssertEqual(try JSONDecoder().decode(PRStatusDoc.self, from: JSONEncoder().encode(doc)), doc)
    }

    // MARK: - The reading

    func testTheReadingSaysReadinessChecksAndConflicts() {
        let reading = PRReading(PRStatusDoc(slices: [
            PRStatusSlice(sliceID: "open", name: "", pr: "", readiness: PRStatusSlice.awaitingReview),
            PRStatusSlice(sliceID: "landed", name: "", pr: "", readiness: "unread"),
            PRStatusSlice(sliceID: "red", name: "", pr: "", readiness: PRStatusSlice.checksFailing),
            PRStatusSlice(
                sliceID: "both", name: "", pr: "", readiness: PRStatusSlice.checksFailing,
                checks: PRStatusChecks(verdict: "failing", failing: [PRStatusCheck(name: "test", url: "")]),
                conflicting: true, base: " main "),
            PRStatusSlice(sliceID: "x", name: "", pr: "", readiness: PRStatusSlice.readyToMerge, conflicting: true),
        ]))
        XCTAssertEqual(reading.readiness.keys.sorted(), ["both", "open", "red", "x"])
        XCTAssertEqual(reading.failingChecks, ["red": [], "both": ["test"]])
        XCTAssertEqual(reading.conflicts, ["both": BranchConflict(base: "main"), "x": BranchConflict(base: nil)])
        XCTAssertEqual(reading.marks, [
            "red": PRMarks(failingChecks: []),
            "both": PRMarks(failingChecks: ["test"], conflict: BranchConflict(base: "main")),
            "x": PRMarks(conflict: BranchConflict(base: nil)),
        ])
    }

    func testTheMarksSayWhatTheyMark() {
        XCTAssertEqual(BranchConflict(base: "main").help, "Conflicts with main")
        XCTAssertEqual(BranchConflict(base: "  ").help, "Merge conflicts")
        XCTAssertEqual(BranchConflict(base: nil).help, "Merge conflicts")
        XCTAssertEqual(PRMarks(failingChecks: ["test", "lint"]).checksHelp, "Checks failing: test, lint")
        XCTAssertEqual(PRMarks(failingChecks: []).checksHelp, "Checks failing")
        XCTAssertNil(PRMarks(conflict: BranchConflict(base: nil)).checksHelp)
        XCTAssertTrue(PRMarks.none.isEmpty)
        XCTAssertFalse(PRMarks(conflict: BranchConflict(base: nil)).isEmpty)
    }

    // MARK: - The sidebar

    /// Active rows and tree rows carry the same marks, in every project,
    /// only while the slice stands at its pull request.
    func testBothRowKindsCarryTheMarksAtThePRStageOnly() {
        let plan = ProjectInfo(
            project: Project(id: "p", name: "P", conventions: ""),
            milestones: [Milestone(id: "M1", name: "M1", order: 0, status: "Active")],
            slices: [slice("a"), slice("b", fixing: true), slice("c", pr: ""), slice("d", status: "Done")])
        let other = ProjectInfo(
            project: Project(id: "q", name: "Q", conventions: ""),
            milestones: [Milestone(id: "M1", name: "M1", order: 0, status: "Active")],
            slices: [slice("e")])
        let both = PRMarks(failingChecks: ["test"], conflict: BranchConflict(base: "main"))
        let model = buildSidebarModel(
            projects: [
                SidebarProjectInput(id: "p", name: "P", plan: plan), SidebarProjectInput(id: "q", name: "Q", plan: other),
            ],
            liveAgents: ["b": .waiting, "c": .working],
            prMarks: [
                "a": both, "b": PRMarks(failingChecks: ["lint"]), "c": both, "d": both,
                "e": PRMarks(conflict: BranchConflict(base: nil)),
            ])
        let active = Dictionary(uniqueKeysWithValues: model.active.map { ($0.targetID, $0.marks) })
        XCTAssertEqual(active["a"], both)
        XCTAssertEqual(active["b"], PRMarks(failingChecks: ["lint"]), "under a fix, its agent waiting")
        XCTAssertEqual(active["c"], PRMarks.none, "a working slice has no pull request to mark")
        XCTAssertEqual(active["e"], PRMarks(conflict: BranchConflict(base: nil)), "another project's")

        let tree = Dictionary(uniqueKeysWithValues: model.projects.flatMap { project in
            (project.milestones + project.doneMilestones).flatMap(\.slices).map { ($0.sliceID, $0.marks) }
        })
        XCTAssertEqual(tree["a"], both)
        XCTAssertEqual(tree["b"], PRMarks(failingChecks: ["lint"]))
        XCTAssertEqual(tree["c"], PRMarks.none)
        XCTAssertEqual(tree["d"], PRMarks.none, "a Done slice carries neither")
        XCTAssertEqual(tree["e"], PRMarks(conflict: BranchConflict(base: nil)))
    }

    // MARK: - The PR section

    func testALoadedPullRequestDecidesTheConflict() {
        let reading = BranchConflict(base: "main")
        let url = Fixtures.prURL
        XCTAssertEqual(conflict(reading: reading, detail: nil, prURL: url), reading, "nothing loaded: the reading")
        XCTAssertNil(conflict(reading: reading, detail: Fixtures.prGreen, prURL: url), "fresher, and mergeable")
        XCTAssertEqual(
            conflict(reading: nil, detail: Fixtures.prConflicting, prURL: url), BranchConflict(base: "main"),
            "fresher, and conflicting")
        XCTAssertEqual(
            conflict(reading: reading, detail: Fixtures.prGreen, prURL: "https://github.test/o/r/pull/1"), reading,
            "another pull request's detail says nothing about this one")
    }

    func testTheNoticeSaysWhatToDo() {
        let main = BranchConflict(base: "main")
        let noAgent = conflictNotice(slice: slice(), conflict: main, hasLiveAgent: false)
        XCTAssertEqual(noAgent, ConflictNotice(conflict: main, action: .launchFix))
        XCTAssertEqual(
            noAgent?.text, "This branch conflicts with main — launch a fix agent to merge main in and resolve them.")

        let live = conflictNotice(slice: slice(fixing: true), conflict: main, hasLiveAgent: true)
        XCTAssertEqual(live?.action, .liveAgent)
        XCTAssertEqual(
            live?.text, "This branch conflicts with main — the live agent has it: ask it to merge main in and resolve them.")

        XCTAssertEqual(
            ConflictNotice(conflict: BranchConflict(base: nil), action: .none).text,
            "This branch conflicts with its base.")
    }

    func testTheNoticeIsDrawnOnlyAtThePRStageOrUnderAFix() {
        let main = BranchConflict(base: "main")
        XCTAssertNil(conflictNotice(slice: slice(), conflict: nil, hasLiveAgent: false), "mergeable or unread")
        XCTAssertNil(conflictNotice(slice: slice(status: "Done"), conflict: main, hasLiveAgent: false))
        XCTAssertNil(
            conflictNotice(slice: slice(pr: "", handedBack: true, branch: "b"), conflict: main, hasLiveAgent: false),
            "a handed-back branch with no pull request is not this notice's")
        XCTAssertNotNil(conflictNotice(slice: slice(fixing: true), conflict: main, hasLiveAgent: false))
    }
}
