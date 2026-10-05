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

    func testTheReadingMarksPassingChecksByVerdict() {
        func pr(_ id: String, _ verdict: String?, conflicting: Bool = false) -> PRStatusSlice {
            PRStatusSlice(
                sliceID: id, name: "", pr: "",
                readiness: verdict == "failing" ? PRStatusSlice.checksFailing : PRStatusSlice.awaitingReview,
                checks: verdict.map { PRStatusChecks(verdict: $0) }, conflicting: conflicting)
        }
        let reading = PRReading(PRStatusDoc(slices: [
            pr("green", PRStatusSlice.checksPassing), pr("pending", "pending"), pr("none", "none"),
            pr("red", "failing"), pr("unread", nil), pr("x", PRStatusSlice.checksPassing, conflicting: true),
        ]))
        XCTAssertEqual(reading.passingChecks, ["green", "x"])
        XCTAssertEqual(reading.marks, [
            "green": PRMarks(checksPassing: true),
            "red": PRMarks(failingChecks: []),
            "x": PRMarks(conflict: BranchConflict(base: nil), checksPassing: true),
        ])
    }

    func testThePassingTickIsKeptOnlyWhereItCanBeTrusted() {
        let green = PRMarks(checksPassing: true)
        let tick = PRMarks(checksPassing: true)
        XCTAssertEqual(prMarks(green, for: slice(), agent: nil), tick, "at the PR stage, no agent")
        XCTAssertEqual(prMarks(green, for: slice(), agent: .waiting), tick, "an idle agent left from hand-back")
        XCTAssertEqual(prMarks(green, for: slice(), agent: .working), .none, "a working agent may push")
        XCTAssertEqual(prMarks(green, for: slice(fixing: true), agent: nil), .none, "under a fix")
        XCTAssertEqual(prMarks(green, for: slice(status: "Done"), agent: nil), .none, "Done")
        XCTAssertEqual(
            prMarks(green, for: slice(pr: "", handedBack: true, branch: "b"), agent: nil), .none, "in review")
        XCTAssertEqual(prMarks(green, for: slice(pr: ""), agent: nil), .none, "working, sent back")
        XCTAssertEqual(prMarks(.none, for: slice(), agent: nil), .none, "no reading, or not passing")

        let conflicted = PRMarks(conflict: BranchConflict(base: "main"), checksPassing: true)
        XCTAssertEqual(
            prMarks(conflicted, for: slice(), agent: nil), PRMarks(conflict: BranchConflict(base: "main")),
            "a conflict draws alone")
        let contradictory = PRMarks(failingChecks: ["test"], checksPassing: true)
        XCTAssertEqual(prMarks(contradictory, for: slice(), agent: nil), PRMarks(failingChecks: ["test"]))

        let both = PRMarks(failingChecks: ["test"], conflict: BranchConflict(base: "main"))
        XCTAssertEqual(prMarks(both, for: slice(), agent: .working), both, "failing and conflict as before")
        XCTAssertEqual(prMarks(both, for: slice(fixing: true), agent: .working), both)
    }

    func testTheMarksSayWhatTheyMark() {
        XCTAssertEqual(PRMarks(checksPassing: true).passingHelp, "Checks passing")
        XCTAssertNil(PRMarks(failingChecks: []).passingHelp)
        XCTAssertFalse(PRMarks(checksPassing: true).isEmpty)
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
            slices: [
                slice("a"), slice("b", fixing: true), slice("c", pr: ""), slice("d", status: "Done"),
                slice("g"), slice("h"), slice("i", fixing: true),
            ])
        let other = ProjectInfo(
            project: Project(id: "q", name: "Q", conventions: ""),
            milestones: [Milestone(id: "M1", name: "M1", order: 0, status: "Active")],
            slices: [slice("e")])
        let both = PRMarks(failingChecks: ["test"], conflict: BranchConflict(base: "main"))
        let model = buildSidebarModel(
            projects: [
                SidebarProjectInput(id: "p", name: "P", plan: plan), SidebarProjectInput(id: "q", name: "Q", plan: other),
            ],
            liveAgents: ["b": .waiting, "c": .working, "h": .working],
            prMarks: [
                "g": PRMarks(checksPassing: true), "h": PRMarks(checksPassing: true),
                "i": PRMarks(checksPassing: true),
                "a": both, "b": PRMarks(failingChecks: ["lint"]), "c": both, "d": both,
                "e": PRMarks(conflict: BranchConflict(base: nil)),
            ])
        let active = Dictionary(uniqueKeysWithValues: model.active.map { ($0.targetID, $0.marks) })
        XCTAssertEqual(active["a"], both)
        XCTAssertEqual(active["b"], PRMarks(failingChecks: ["lint"]), "under a fix, its agent waiting")
        XCTAssertEqual(active["c"], PRMarks.none, "a working slice has no pull request to mark")
        XCTAssertEqual(active["e"], PRMarks(conflict: BranchConflict(base: nil)), "another project's")
        XCTAssertEqual(active["g"], PRMarks(checksPassing: true), "green at the PR stage")
        XCTAssertEqual(active["h"], PRMarks.none, "green, but its agent is working")
        XCTAssertEqual(active["i"], PRMarks.none, "green, but under a fix")

        let tree = Dictionary(uniqueKeysWithValues: model.projects.flatMap { project in
            (project.milestones + project.doneMilestones).flatMap(\.slices).map { ($0.sliceID, $0.marks) }
        })
        XCTAssertEqual(tree["a"], both)
        XCTAssertEqual(tree["b"], PRMarks(failingChecks: ["lint"]))
        XCTAssertEqual(tree["c"], PRMarks.none)
        XCTAssertEqual(tree["d"], PRMarks.none, "a Done slice carries neither")
        XCTAssertEqual(tree["e"], PRMarks(conflict: BranchConflict(base: nil)))
        XCTAssertEqual(tree["g"], PRMarks(checksPassing: true))
        XCTAssertEqual(tree["h"], PRMarks.none)
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
