import XCTest
@testable import NatKit

final class ChipPickerTests: XCTestCase {
    private func pr(_ number: Int, _ state: String, title: String = "A change") -> SessionPR {
        SessionPR(number: number, title: title, url: "https://x/pull/\(number)", state: state)
    }

    // MARK: - Chips

    func testPullRequestChipsCarryNumberTitleAndState() {
        let chips = pullRequestChips([pr(3, "OPEN"), pr(2, "MERGED"), pr(1, "CLOSED"), pr(0, "WHATEVER")])
        XCTAssertEqual(chips.map(\.id), ["https://x/pull/3", "https://x/pull/2", "https://x/pull/1", "https://x/pull/0"])
        XCTAssertEqual(chips.map(\.lead), ["#3", "#2", "#1", "#0"])
        XCTAssertEqual(chips.map(\.state), [.open, .merged, .closed, .open], "an unknown state reads as open")
        XCTAssertEqual(PickerChipState.merged.word, "merged")
        XCTAssertEqual(PickerChipState.closed.word, "closed")
        XCTAssertEqual(PickerChipState.open.word, "open")
    }

    func testTitlesAreTruncatedWithAnEllipsis() {
        XCTAssertEqual(truncatedPickerTitle("  short  "), "short")
        let long = String(repeating: "a", count: 40)
        let cut = truncatedPickerTitle(long)
        XCTAssertEqual(cut.count, pickerTitleLimit)
        XCTAssertTrue(cut.hasSuffix("…"))
        XCTAssertEqual(truncatedPickerTitle(String(repeating: "b", count: pickerTitleLimit)).count, pickerTitleLimit)
    }

    func testBranchChipsMarkTheCheckedOutOne() {
        let chips = branchChips(["b", "a"], checkedOut: "b")
        XCTAssertEqual(chips.map(\.mark), ["checked out", nil])
        XCTAssertEqual(branchChips(["b", "a"], checkedOut: nil).map(\.mark), [nil, nil])
    }

    func testPickerIsVisibleOnlyWithSeveralChips() {
        XCTAssertFalse(ChipPickerModel(chips: [], selectedID: nil).isVisible)
        XCTAssertFalse(ChipPickerModel(chips: pullRequestChips([pr(1, "OPEN")]), selectedID: nil).isVisible)
        XCTAssertTrue(ChipPickerModel(chips: pullRequestChips([pr(1, "OPEN"), pr(2, "OPEN")]), selectedID: nil).isVisible)
    }

    // MARK: - Selection memory

    func testResolvedPrefersTheRememberedChoice() {
        var memory = PickerSelectionMemory()
        let ids = ["a", "b", "c"]
        XCTAssertEqual(memory.resolved(for: "k", among: ids), "a", "the first with no default")
        XCTAssertEqual(memory.resolved(for: "k", among: ids, defaultID: "b"), "b")
        memory.select("c", for: "k")
        XCTAssertEqual(memory.resolved(for: "k", among: ids, defaultID: "b"), "c")
        XCTAssertEqual(memory.resolved(for: "other", among: ids, defaultID: "b"), "b", "per key")
    }

    func testResolvedFallsBackWhenTheChoiceIsGone() {
        var memory = PickerSelectionMemory()
        memory.select("gone", for: "k")
        XCTAssertEqual(memory.resolved(for: "k", among: ["a", "b"], defaultID: "b"), "b")
        XCTAssertEqual(memory.resolved(for: "k", among: ["a", "b"], defaultID: "missing"), "a")
        XCTAssertNil(memory.resolved(for: "k", among: []))
        memory.select("a", for: "k")
        memory.forget("k")
        XCTAssertEqual(memory.resolved(for: "k", among: ["a", "b"], defaultID: "b"), "b")
    }

    func testPickerKeysAreDistinctPerSessionAndPicker() {
        XCTAssertNotEqual(SessionPicker.pullRequest.key(sessionID: "s"), SessionPicker.branch.key(sessionID: "s"))
        XCTAssertNotEqual(SessionPicker.branch.key(sessionID: "s"), SessionPicker.branch.key(sessionID: "t"))
    }

    @MainActor
    func testAppModelRemembersPerSession() {
        let model = AppModel()
        XCTAssertEqual(model.selectedPickerID(.branch, sessionID: "s", among: ["a", "b"], defaultID: "a"), "a")
        model.selectPicker(.branch, sessionID: "s", id: "b")
        XCTAssertEqual(model.selectedPickerID(.branch, sessionID: "s", among: ["a", "b"], defaultID: "a"), "b")
        XCTAssertEqual(model.selectedPickerID(.branch, sessionID: "t", among: ["a", "b"], defaultID: "a"), "a")
    }

    // MARK: - Stepper stage

    func testPRStageBadgeIsTheOpenCountAndGreenOnlyWhenAllMerged() {
        XCTAssertEqual(SessionPRStage(prs: []).openCount, 0)
        XCTAssertFalse(SessionPRStage(prs: []).isComplete, "no pull request is not merged")

        let mixed = SessionPRStage(prs: [pr(1, "OPEN"), pr(2, "MERGED"), pr(3, "CLOSED"), pr(4, "OPEN")])
        XCTAssertEqual(mixed.openCount, 2)
        XCTAssertFalse(mixed.isComplete)

        XCTAssertFalse(SessionPRStage(prs: [pr(1, "MERGED"), pr(2, "CLOSED")]).isComplete)
        XCTAssertTrue(SessionPRStage(prs: [pr(1, "MERGED"), pr(2, "MERGED")]).isComplete)
    }

    func testSessionTabStateCarriesTheBadgeAndLeavesDiffUnchanged() {
        let none = buildSessionTabState()
        XCTAssertTrue(none.badges.isEmpty)
        XCTAssertFalse(none.isComplete(.pr))
        XCTAssertTrue(none.isComplete(.diff), "Diff keeps the default reading")

        let open = buildSessionTabState(prs: [pr(1, "OPEN"), pr(2, "MERGED")])
        XCTAssertEqual(open.badges[.pr], 1)
        XCTAssertFalse(open.isComplete(.pr))
        XCTAssertTrue(open.isComplete(.diff))

        let merged = buildSessionTabState(prs: [pr(1, "MERGED"), pr(2, "MERGED")])
        XCTAssertNil(merged.badges[.pr])
        XCTAssertTrue(merged.isComplete(.pr))
    }

    // MARK: - Rail row

    func testSummaryNamesTheCountOnlyWithSeveralPRs() {
        XCTAssertNil(sessionPRSummary([]))
        XCTAssertNil(sessionPRSummary([pr(1, "OPEN")]))
        XCTAssertEqual(sessionPRSummary([pr(1, "OPEN"), pr(2, "MERGED"), pr(3, "CLOSED")]), "3 PRs · 1 open")
    }

    func testRailRowsNameTheCount() {
        let prs = [pr(1, "OPEN"), pr(2, "MERGED")]
        let info = ProjectInfo(project: Project(id: "p", name: "P", conventions: ""), milestones: [], slices: [])
        func session(_ id: String) -> Session {
            Session(id: id, tag: "session:p:\(id)", live: false, startedAt: Date(), dir: "/tmp", branch: "b", prs: prs)
        }

        let review = buildRailModel(from: info, liveAgents: [:], sessions: [session("r")])
        XCTAssertEqual(review.active.first?.detail.last, "2 PRs · 1 open")

        let live = buildRailModel(from: info, liveAgents: ["session:p:l": .working], sessions: [session("l")])
        XCTAssertEqual(live.active.first?.detail, ["b", "2 PRs · 1 open"])

        let done = Session(
            id: "d", tag: "session:p:d", live: false, startedAt: Date(), dir: "/tmp", branch: "b",
            prs: [pr(1, "MERGED"), pr(2, "MERGED")])
        let doneModel = buildRailModel(from: info, liveAgents: [:], sessions: [done])
        XCTAssertEqual(doneModel.doneSessions.first?.detail.last, "2 PRs · 0 open")
    }
}
