import XCTest
@testable import NatKit

/// The sidebar's folds: static defaults, the user's folds kept across
/// launches in `FoldMemory`, and the selection revealed — every fold over its
/// row opened — once per selection, as VS Code and Zed reveal the active
/// file. Every memory here is in-memory or over a throwaway suite, never the
/// real defaults.
@MainActor
final class SidebarFoldsTests: XCTestCase {
    // MARK: - FoldMemory

    func testAnInMemoryMemoryKeepsWhatItIsGivenAndNoOtherDoes() {
        let memory = FoldMemory.inMemory()
        XCTAssertEqual(memory.folds, [:])

        memory.save(["p:a": false, "work": true])

        XCTAssertEqual(memory.folds, ["p:a": false, "work": true])
        XCTAssertEqual(FoldMemory.inMemory().folds, [:], "each in-memory memory is its own")
    }

    func testFoldsOutlastALaunchThroughTheDefaults() throws {
        let suite = "SidebarFoldsTests-\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }

        let first = SidebarFolds(memory: FoldMemory(defaults: defaults))
        first.set("m:a/M1", folded: true)
        first.reveal(["p:a"])

        let relaunched = SidebarFolds(memory: FoldMemory(defaults: defaults))
        XCTAssertFalse(relaunched.isOpen("m:a/M1"))
        XCTAssertTrue(relaunched.isOpen("p:a", byDefault: false))
        XCTAssertEqual(
            defaults.dictionary(forKey: FoldMemory.storageKey) as? [String: Bool], ["m:a/M1": true, "p:a": false])
    }

    // MARK: - Folding

    func testAKeyWithNoFoldTakesItsDefault() {
        let folds = SidebarFolds()
        XCTAssertTrue(folds.isOpen("work"))
        XCTAssertFalse(folds.isOpen("p:a", byDefault: false))
    }

    func testSettingAFoldRecordsItOverTheDefault() {
        let memory = FoldMemory.inMemory()
        let folds = SidebarFolds(memory: memory)

        folds.set("work", folded: true)
        folds.set("p:a", folded: false)

        XCTAssertFalse(folds.isOpen("work"))
        XCTAssertTrue(folds.isOpen("p:a", byDefault: false))
        XCTAssertEqual(memory.folds, ["work": true, "p:a": false])
    }

    func testARenamedMilestoneKeepsItsFold() {
        let memory = FoldMemory.inMemory()
        let folds = SidebarFolds(memory: memory)
        folds.set("m:a/Old", folded: true)

        folds.rename("m:a/Old", to: "m:a/New")
        folds.rename("m:a/Never", to: "m:a/Other")

        XCTAssertEqual(memory.folds, ["m:a/New": true], "a key with no fold carries nothing over")
        XCTAssertFalse(folds.isOpen("m:a/New"))
    }

    func testRevealOpensEachKeyAndFoldsNothing() {
        let memory = FoldMemory.inMemory()
        let folds = SidebarFolds(memory: memory)
        folds.set("m:a/M2", folded: false)

        folds.reveal(["work", "p:a", "m:a/M1"])

        XCTAssertEqual(memory.folds, ["work": false, "p:a": false, "m:a/M1": false, "m:a/M2": false])
    }

    func testASeedStandsOverTheMemoryAndIsRecordedOnlyWithTheNextChange() {
        let memory = FoldMemory.inMemory()
        memory.save(["work": true, "p:a": true])

        let folds = SidebarFolds(memory: memory, seed: ["p:a": false])

        XCTAssertFalse(folds.isOpen("work"))
        XCTAssertTrue(folds.isOpen("p:a", byDefault: false))
        XCTAssertEqual(memory.folds, ["work": true, "p:a": true])
        folds.set("scratch", folded: false)
        XCTAssertEqual(memory.folds, ["work": true, "p:a": false, "scratch": false])
    }

    // MARK: - Following the selection

    private let selected = SidebarSelection(projectID: "a", sliceID: "1")

    func testASelectionIsRevealedOnceAndAFoldOverItStands() {
        let folds = SidebarFolds()
        folds.follow(selected, keys: ["work", "p:a", "m:a/M1"])
        XCTAssertTrue(folds.isOpen("m:a/M1", byDefault: false))

        folds.set("m:a/M1", folded: true)
        folds.follow(selected, keys: ["work", "p:a", "m:a/M1"])

        XCTAssertFalse(folds.isOpen("m:a/M1"), "a refresh of the same selection reveals nothing again")
    }

    func testANewSelectionIsRevealedAgainAndSoIsAReturnToTheOldOne() {
        let folds = SidebarFolds()
        folds.follow(selected, keys: ["m:a/M1"])
        folds.set("m:a/M1", folded: true)

        folds.follow(SidebarSelection(projectID: "a", sliceID: "2"), keys: ["m:a/M2"])
        XCTAssertTrue(folds.isOpen("m:a/M2", byDefault: false))
        XCTAssertFalse(folds.isOpen("m:a/M1"), "the old selection's fold is left as the user made it")

        folds.follow(selected, keys: ["m:a/M1"])
        XCTAssertTrue(folds.isOpen("m:a/M1"))
    }

    func testASelectionNotYetDrawnIsRevealedOnceItIs() {
        let folds = SidebarFolds()
        folds.follow(selected, keys: nil)
        XCTAssertEqual(folds.folded, [:])

        folds.follow(selected, keys: ["p:a"])
        XCTAssertTrue(folds.isOpen("p:a", byDefault: false))

        folds.set("p:a", folded: true)
        folds.follow(selected, keys: ["p:a"])
        XCTAssertFalse(folds.isOpen("p:a"), "found once is revealed once")
    }

    func testNoSelectionRevealsNothing() {
        let folds = SidebarFolds()
        folds.follow(nil, keys: nil)
        XCTAssertEqual(folds.folded, [:])
    }

    func testASeedStandsOverTheFirstReveal() {
        let folds = SidebarFolds(seed: ["p:a": true])
        folds.follow(selected, keys: nil)
        folds.follow(selected, keys: ["work", "p:a"])

        XCTAssertFalse(folds.isOpen("p:a"), "a story's fold is one made after the starting selection was shown")
        XCTAssertTrue(folds.isOpen("work", byDefault: false))

        folds.follow(SidebarSelection(projectID: "a", sliceID: "2"), keys: ["p:a"])
        XCTAssertTrue(folds.isOpen("p:a"), "only the first reveal")
    }

    // MARK: - The folds over a row

    private func row(_ id: String, project: String = "a") -> SidebarSliceRow {
        SidebarSliceRow(sliceID: id, projectID: project, title: id, state: .todo, live: false)
    }

    private func milestone(_ name: String, _ ids: [String], project: String = "a") -> SidebarMilestone {
        SidebarMilestone(name: name, done: 0, total: ids.count, slices: ids.map { row($0, project: project) })
    }

    private var model: SidebarModel {
        let project = SidebarProject(
            id: "a", name: "A", kind: .project, status: .loaded,
            milestones: [milestone("M1", ["1"]), milestone("", ["2"])],
            doneMilestones: [milestone("Old", ["3"])], needsYou: 0)
        let scratch = SidebarProject(
            id: "s", name: "Scratch", kind: .scratch, status: .loaded,
            milestones: [milestone("Try", ["5"], project: "s")], needsYou: 0, loose: [row("4", project: "s")])
        func container(_ id: String, _ task: String) -> SidebarContainer {
            SidebarContainer(id: id, title: id, tasks: [row(task, project: "w")], needsYou: 0)
        }
        let source = SidebarSource(
            name: "shortcut", title: "Shortcut", tag: "SC", icon: SourceIcon(symbol: ""),
            containerNoun: "card", taskNoun: "task",
            groups: [
                SidebarSourceGroup(id: "mine", label: "Mine", containers: [container("c1", "t1")]),
                SidebarSourceGroup(
                    id: "ready", label: "Ready",
                    children: [SidebarSourceGroup(id: "soon", label: "Soon", containers: [container("c2", "t2")])]),
                SidebarSourceGroup(id: "later", label: "Later", lazy: true, containers: [container("c3", "t3")]),
            ])
        return SidebarModel(
            active: [],
            projects: [project],
            sources: [
                SidebarProject(id: "w", name: "Work", kind: .project, status: .loaded, milestones: [], needsYou: 0,
                               source: source),
                SidebarProject(id: "v", name: "Unread", kind: .project, status: .loading, milestones: [], needsYou: 0),
            ],
            scratch: scratch)
    }

    private func keys(_ project: String, slice: String? = nil, session: String? = nil) -> [String]? {
        sidebarRevealKeys(
            for: SidebarSelection(projectID: project, sliceID: slice, sessionID: session), in: model,
            endedSessions: ["ended"])
    }

    func testASliceInALiveMilestoneOpensItsSectionProjectAndMilestone() {
        XCTAssertEqual(keys("a", slice: "1"), ["work", "p:a", "m:a/M1"])
        XCTAssertEqual(keys("a", slice: "2"), ["work", "p:a", "m:a/"], "the unfiled milestone has its own key")
    }

    func testASliceInADoneMilestoneOpensTheDoneFolderToo() {
        XCTAssertEqual(keys("a", slice: "3"), ["work", "p:a", "d:a", "m:a/Old"])
    }

    func testAScratchSliceOpensScratchAndItsMilestoneIfItHasOne() {
        XCTAssertEqual(keys("s", slice: "4"), ["scratch"], "a loose slice sits under no folder")
        XCTAssertEqual(keys("s", slice: "5"), ["scratch", "m:s/Try"])
    }

    func testASourceTaskOpensItsFoldAndTheGroupsOverIt() {
        XCTAssertEqual(keys("w", slice: "t1"), ["s:w", "g:w/mine"])
        XCTAssertEqual(keys("w", slice: "t2"), ["s:w", "g:w/ready", "g:w/soon"])
        XCTAssertEqual(keys("w", slice: "t3"), ["s:w"], "a lazy group listing it is already expanded")
        XCTAssertNil(keys("w", slice: "gone"))
        XCTAssertNil(keys("v", slice: "t1"), "a source fold with no tree yet draws nothing")
    }

    func testAnEndedSessionOpensTheAdHocSessionsFolder() {
        XCTAssertEqual(keys("a", session: "ended"), ["work", "p:a", "m:a/~sessions"])
        XCTAssertEqual(keys("s", session: "ended"), ["scratch", "m:s/~sessions"])
        XCTAssertEqual(keys("a", session: "live"), ["work", "p:a"], "a live session is drawn in Active alone")
        XCTAssertEqual(keys("w", session: "ended"), ["s:w"], "a source fold draws no sessions")
    }

    func testARowNotDrawnYetHasNoKeys() {
        XCTAssertNil(keys("a", slice: "9"))
        XCTAssertNil(keys("s", slice: "9"))
        XCTAssertNil(keys("elsewhere", slice: "1"))
        XCTAssertNil(keys("elsewhere"))
    }

    func testAProjectAloneSelectedOpensItsOwnTree() {
        XCTAssertEqual(keys("a"), ["work", "p:a"])
        XCTAssertEqual(keys("s"), ["scratch"])
        XCTAssertEqual(keys("w"), ["s:w"])
    }
}
