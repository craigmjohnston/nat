import XCTest
@testable import NatKit
@testable import NatFixtures

/// A pull request's conflict and failing checks: nat's words decoded, the
/// reading's maps, the sidebar's marks on both row kinds, and the PR
/// section's conflict notice.
final class PRMarksTests: XCTestCase {
    private func slice(
        _ id: String = "s-1", status: String = "In progress", pr: String = "https://github.test/o/r/pull/7",
        resumed: Bool = false, handedBack: Bool = false, branch: String? = nil
    ) -> Slice {
        Slice(
            id: id, name: "Slice \(id)", status: status, milestoneID: "M1", assignee: "", pr: pr, url: "",
            branch: branch, blocked: false, handedBack: handedBack, resumed: resumed)
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
        XCTAssertEqual(reading.runningChecks, ["pending"])
        XCTAssertEqual(reading.marks, [
            "green": PRMarks(checksPassing: true),
            "pending": PRMarks(checksRunning: true),
            "red": PRMarks(failingChecks: []),
            "x": PRMarks(conflict: BranchConflict(base: nil), checksPassing: true),
        ])
    }

    func testThePassingTickIsKeptOnlyWhereItCanBeTrusted() {
        let green = PRMarks(checksPassing: true)
        let tick = PRMarks(checksPassing: true)
        XCTAssertEqual(prMarks(green, for: slice()), tick, "at the PR stage, whatever its agent reads")
        XCTAssertEqual(prMarks(green, for: slice(resumed: true)), .none, "resumed")
        XCTAssertEqual(prMarks(green, for: slice(status: "Done")), .none, "Done")
        XCTAssertEqual(prMarks(green, for: slice(pr: "", handedBack: true, branch: "b")), .none, "in review")
        XCTAssertEqual(prMarks(green, for: slice(pr: "")), .none, "working, sent back")
        XCTAssertEqual(prMarks(.none, for: slice()), .none, "no reading, or not passing")

        let conflicted = PRMarks(conflict: BranchConflict(base: "main"), checksPassing: true)
        XCTAssertEqual(
            prMarks(conflicted, for: slice()), PRMarks(conflict: BranchConflict(base: "main")),
            "a conflict draws alone")
        let contradictory = PRMarks(failingChecks: ["test"], checksPassing: true)
        XCTAssertEqual(prMarks(contradictory, for: slice()), PRMarks(failingChecks: ["test"]))

        let both = PRMarks(failingChecks: ["test"], conflict: BranchConflict(base: "main"))
        XCTAssertEqual(prMarks(both, for: slice()), both, "failing and conflict as read")
    }

    func testTheRunningMarkKeepsThePassingTicksGate() {
        let running = PRMarks(checksRunning: true)
        XCTAssertEqual(PRStatusSlice.checksPending, "pending")
        XCTAssertEqual(prMarks(running, for: slice()), running, "at the PR stage, whatever its agent reads")
        XCTAssertEqual(prMarks(running, for: slice(resumed: true)), .none, "resumed")
        XCTAssertEqual(prMarks(running, for: slice(status: "Done")), .none, "Done")
        XCTAssertEqual(
            prMarks(PRMarks(failingChecks: ["test"], checksRunning: true), for: slice()),
            PRMarks(failingChecks: ["test"]), "failing wins")
        XCTAssertEqual(
            prMarks(PRMarks(conflict: BranchConflict(base: "main"), checksRunning: true), for: slice()),
            PRMarks(conflict: BranchConflict(base: "main")), "a conflict draws alone")
        XCTAssertEqual(
            prMarks(PRMarks(checksRunning: true, heldFailingChecks: ["test"]), for: slice()), running,
            "handed back, the checks running again: the held failure goes")
        XCTAssertEqual(running.runningHelp, "Checks running")
        XCTAssertNil(PRMarks(checksPassing: true).runningHelp)
        XCTAssertFalse(running.isEmpty)
    }

    /// Work kicked back to the agent draws its failing checks alone — as
    /// read, or held while the fix's checks run — until it is handed back.
    func testAResumedSliceKeepsOnlyItsFailingChecks() {
        let resumed = slice(resumed: true)
        XCTAssertEqual(
            prMarks(PRMarks(failingChecks: ["test"], conflict: BranchConflict(base: "main")), for: resumed),
            PRMarks(failingChecks: ["test"]), "failing as read, the conflict not")
        XCTAssertEqual(
            prMarks(PRMarks(checksRunning: true, heldFailingChecks: ["test"]), for: resumed),
            PRMarks(failingChecks: ["test"]), "held while the fix's checks run")
        XCTAssertEqual(prMarks(PRMarks(conflict: BranchConflict(base: "main")), for: resumed), .none)
        XCTAssertFalse(PRMarks(heldFailingChecks: []).isEmpty)
    }

    /// A failure is held over checks running again, from the reading before
    /// or the one before that, and dropped by any other verdict.
    func testTheReadingHoldsAFailureWhileTheChecksRunAgain() {
        func pr(_ verdict: String, failing: [String] = []) -> PRStatusDoc {
            PRStatusDoc(slices: [PRStatusSlice(
                sliceID: "s", name: "", pr: "",
                readiness: verdict == "failing" ? PRStatusSlice.checksFailing : PRStatusSlice.awaitingReview,
                checks: PRStatusChecks(verdict: verdict, failing: failing.map { PRStatusCheck(name: $0, url: "") }))])
        }
        let red = PRReading(pr("failing", failing: ["test"]))
        XCTAssertEqual(red.heldFailingChecks, [:], "a failure as read is not held")
        let rerun = PRReading(pr("pending"), after: red)
        XCTAssertEqual(rerun.heldFailingChecks, ["s": ["test"]])
        XCTAssertEqual(rerun.marks["s"], PRMarks(checksRunning: true, heldFailingChecks: ["test"]))
        XCTAssertEqual(PRReading(pr("pending"), after: rerun).heldFailingChecks, ["s": ["test"]], "still running")
        XCTAssertEqual(PRReading(pr("passing"), after: rerun).heldFailingChecks, [:], "passed")
        XCTAssertEqual(PRReading(pr("pending")).heldFailingChecks, [:], "nothing before it")
        XCTAssertEqual(
            PRReading(pr("pending"), after: PRReading(pr("passing"))).heldFailingChecks, [:], "never failed")
    }

    func testACheckRowLeadsWithItsOutcomesMark() {
        XCTAssertEqual(CheckRowMark(.passing), CheckRowMark(.passing))
        XCTAssertEqual(CheckRowMark(.passing).symbol, "checkmark.circle.fill")
        XCTAssertEqual(CheckRowMark(.passing).role, .success)
        XCTAssertEqual(CheckRowMark(.failing).symbol, "xmark.circle.fill")
        XCTAssertEqual(CheckRowMark(.failing).role, .danger)
        XCTAssertEqual(CheckRowMark(.pending).symbol, PRMarks.runningSymbol, "the sidebar's running mark")
        XCTAssertEqual(CheckRowMark(.pending).role, .secondary, "neutral, never a warning")
        XCTAssertEqual(CheckRowMark(.skipped).symbol, "slash.circle")
        XCTAssertEqual(CheckRowMark(.skipped).role, .tertiary)
        XCTAssertEqual(PRMarks.runningSymbol, "ellipsis.circle.fill")
        XCTAssertEqual(PRMarks.runningOutlineSymbol, "ellipsis.circle")
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
                slice("a"), slice("b"), slice("c", pr: ""), slice("d", status: "Done"),
                slice("g"), slice("h"), slice("i", resumed: true), slice("j", resumed: true),
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
                "i": PRMarks(checksPassing: true), "j": PRMarks(checksRunning: true, heldFailingChecks: ["test"]),
                "a": both, "b": PRMarks(failingChecks: ["lint"]), "c": both, "d": both,
                "e": PRMarks(conflict: BranchConflict(base: nil)),
            ])
        let active = Dictionary(uniqueKeysWithValues: model.active.map { ($0.targetID, $0.marks) })
        XCTAssertEqual(active["a"], both)
        XCTAssertEqual(active["b"], PRMarks(failingChecks: ["lint"]), "at its pull request, its agent waiting")
        XCTAssertEqual(active["c"], PRMarks.none, "a working slice has no pull request to mark")
        XCTAssertEqual(active["e"], PRMarks(conflict: BranchConflict(base: nil)), "another project's")
        XCTAssertEqual(active["g"], PRMarks(checksPassing: true), "green at the PR stage")
        XCTAssertEqual(active["h"], PRMarks(checksPassing: true), "green, its agent left reading working")
        XCTAssertEqual(active["i"], PRMarks.none, "green, but resumed")
        XCTAssertEqual(active["j"], PRMarks(failingChecks: ["test"]), "resumed, its fix's checks running")

        let tree = Dictionary(uniqueKeysWithValues: model.projects.flatMap { project in
            (project.milestones + project.doneMilestones).flatMap(\.slices).map { ($0.sliceID, $0.marks) }
        })
        XCTAssertEqual(tree["a"], both)
        XCTAssertEqual(tree["b"], PRMarks(failingChecks: ["lint"]))
        XCTAssertEqual(tree["c"], PRMarks.none)
        XCTAssertEqual(tree["d"], PRMarks.none, "a Done slice carries neither")
        XCTAssertEqual(tree["e"], PRMarks(conflict: BranchConflict(base: nil)))
        XCTAssertEqual(tree["g"], PRMarks(checksPassing: true))
        XCTAssertEqual(tree["h"], PRMarks(checksPassing: true))
        XCTAssertEqual(tree["j"], PRMarks(failingChecks: ["test"]))
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
        XCTAssertEqual(noAgent, ConflictNotice(conflict: main, action: .sendBack))
        XCTAssertEqual(
            noAgent?.text, "This branch conflicts with main — send it back to the agent to merge main in and resolve them.")

        let live = conflictNotice(slice: slice(), conflict: main, hasLiveAgent: true)
        XCTAssertEqual(live?.action, .liveAgent)
        XCTAssertEqual(
            live?.text, "This branch conflicts with main — the live agent has it: ask it to merge main in and resolve them.")

        XCTAssertEqual(
            ConflictNotice(conflict: BranchConflict(base: nil), action: .none).text,
            "This branch conflicts with its base.")
    }

    func testTheNoticeIsDrawnOnlyAtThePRStage() {
        let main = BranchConflict(base: "main")
        XCTAssertNil(conflictNotice(slice: slice(), conflict: nil, hasLiveAgent: false), "mergeable or unread")
        XCTAssertNil(conflictNotice(slice: slice(status: "Done"), conflict: main, hasLiveAgent: false))
        XCTAssertNil(
            conflictNotice(slice: slice(pr: "", handedBack: true, branch: "b"), conflict: main, hasLiveAgent: false),
            "a handed-back branch with no pull request is not this notice's")
        XCTAssertNil(conflictNotice(slice: slice(resumed: true), conflict: main, hasLiveAgent: false), "resumed")
        XCTAssertNotNil(conflictNotice(slice: slice(), conflict: main, hasLiveAgent: false))
    }

    // MARK: - A handed-back branch with no pull request

    private func inReview(_ id: String = "s-1") -> Slice {
        slice(id, pr: "", handedBack: true, branch: "slice/\(id)")
    }

    func testPRStatusDecodesBranchesAndDefaultsThemEmpty() throws {
        let json = """
        {"slices": [], "branches": [
          {"slice_id": "s-1", "name": "A", "branch": "slice/a", "base": "origin/main", "conflicting": true}
        ]}
        """
        let doc = try JSONDecoder().decode(PRStatusDoc.self, from: Data(json.utf8))
        XCTAssertEqual(doc.branches, [
            PRStatusBranch(sliceID: "s-1", name: "A", branch: "slice/a", base: "origin/main", conflicting: true),
        ])
        XCTAssertEqual(try JSONDecoder().decode(PRStatusDoc.self, from: JSONEncoder().encode(doc)), doc)
        let older = try JSONDecoder().decode(PRStatusDoc.self, from: Data(#"{"slices": []}"#.utf8))
        XCTAssertEqual(older.branches, [], "an older nat sends no branches")
    }

    /// A conflicted branch is marked, a clean one is not; a branch nat could
    /// not test never reaches the reading.
    func testTheReadingMarksAConflictedBranchAndNotACleanOne() {
        let reading = PRReading(PRStatusDoc(slices: [], branches: [
            PRStatusBranch(sliceID: "bad", name: "", branch: "b", base: "origin/main", conflicting: true),
            PRStatusBranch(sliceID: "ok", name: "", branch: "c", base: "origin/main", conflicting: false),
        ]))
        XCTAssertEqual(reading.branchConflicts, ["bad": BranchConflict(base: "origin/main")])
        XCTAssertEqual(reading.conflicts, [:], "no pull request conflicts")
        XCTAssertEqual(reading.marks, ["bad": PRMarks(conflict: BranchConflict(base: "origin/main"))])
    }

    /// In review, a slice's rows carry its branch's conflict and nothing else
    /// of a reading; with none, nothing.
    func testAConflictedHandBackIsMarkedInReview() {
        let conflict = BranchConflict(base: "origin/main")
        let marks = PRMarks(failingChecks: ["stale"], conflict: conflict, checksPassing: true)
        XCTAssertEqual(prMarks(marks, for: inReview()), PRMarks(conflict: conflict))
        XCTAssertEqual(prMarks(.none, for: inReview()), .none)
        XCTAssertEqual(
            prMarks(PRMarks(conflict: conflict), for: slice(pr: "", branch: nil)), .none,
            "work in progress is not in review")
    }

    func testTheBranchNoticeSaysToRebase() {
        let base = BranchConflict(base: "origin/main")
        let noAgent = branchConflictNotice(slice: inReview(), conflict: base, hasLiveAgent: false)
        XCTAssertEqual(noAgent, ConflictNotice(conflict: base, action: .sendBack, hasPullRequest: false))
        XCTAssertEqual(
            noAgent?.text,
            "This branch conflicts with origin/main — send it back to the agent to rebase it on origin/main and resolve them.")
        XCTAssertEqual(
            branchConflictNotice(slice: inReview(), conflict: base, hasLiveAgent: true)?.text,
            "This branch conflicts with origin/main — the live agent has it: ask it to rebase it on origin/main and resolve them.")
    }

    /// Only a slice in review draws it: an unknown reading (no conflict in
    /// hand) draws nothing, and an approved slice keeps the PR's own notice.
    func testTheBranchNoticeIsDrawnOnlyInReview() {
        let base = BranchConflict(base: "origin/main")
        XCTAssertNil(branchConflictNotice(slice: inReview(), conflict: nil, hasLiveAgent: false), "clean or untested")
        XCTAssertNil(branchConflictNotice(slice: slice(), conflict: base, hasLiveAgent: false), "approved")
        XCTAssertNil(branchConflictNotice(slice: slice(status: "Done"), conflict: base, hasLiveAgent: false))
    }
}
