import XCTest
@testable import NatKit

final class DependencyLineTests: XCTestCase {
    private func slice(_ id: String, _ name: String, status: String) -> Slice {
        Slice(
            id: id, name: name, status: status, milestoneID: "m-1",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )
    }

    func testNoDependencies() {
        XCTAssertEqual(
            dependencyLine(dependsOn: nil, blocked: false, plan: []),
            "Nothing blocks this slice"
        )
        XCTAssertEqual(
            dependencyLine(dependsOn: [], blocked: false, plan: []),
            "Nothing blocks this slice"
        )
    }

    /// Blocked with no dependency to name at all — the page said blocked and
    /// nothing else, so that is the whole line.
    func testBlockedWithoutDependencies() {
        XCTAssertEqual(
            dependencyLine(dependsOn: nil, blocked: true, plan: []),
            "Blocked"
        )
    }

    /// A blocked slice names the dependencies still unfinished — the ones
    /// actually being waited on — and not the ones already Done.
    func testBlockedListsOnlyUnfinishedDependencies() {
        let plan = [
            slice("dep-1", "Fix the parser", status: "Todo"),
            slice("dep-2", "Ship the client", status: "Done"),
            slice("dep-3", "Wire the poller", status: "In progress"),
        ]
        XCTAssertEqual(
            dependencyLine(dependsOn: ["dep-1", "dep-2", "dep-3"], blocked: true, plan: plan),
            "Waits on Fix the parser, Wire the poller"
        )
    }

    /// An unblocked slice with dependencies names them all and says they are
    /// done, so the line still says why the slice was gated.
    func testUnblockedNamesTheWholeListAsDone() {
        let plan = [
            slice("dep-1", "Fix the parser", status: "Done"),
            slice("dep-2", "Ship the client", status: "Done"),
        ]
        XCTAssertEqual(
            dependencyLine(dependsOn: ["dep-1", "dep-2"], blocked: false, plan: plan),
            "Waits on Fix the parser, Ship the client — all done"
        )
    }

    /// Relation IDs come dashed and the plan's may not (or the reverse); the
    /// match strips them, so the same page always names itself.
    func testMatchesIDsWhateverTheirDashes() {
        let plan = [slice("3b738308f654815fa843dce9c020efb4", "Fix the parser", status: "Todo")]
        XCTAssertEqual(
            dependencyLine(
                dependsOn: ["3b738308-f654-815f-a843-dce9c020efb4"], blocked: true, plan: plan),
            "Waits on Fix the parser"
        )
    }

    /// A dependency the plan cannot name is left unlisted rather than
    /// guessed at; with nothing nameable, the count is all there is to say.
    func testFallsBackToTheCountWhenNothingCanBeNamed() {
        XCTAssertEqual(
            dependencyLine(dependsOn: ["gone-1"], blocked: true, plan: []),
            "Waits on 1 slice"
        )
        XCTAssertEqual(
            dependencyLine(dependsOn: ["gone-1", "gone-2"], blocked: false, plan: []),
            "Waits on 2 slices"
        )
    }

    /// Blocked, but every nameable dependency is Done — what remains of the
    /// wait is pages nobody can read, so the count fallback covers it.
    func testBlockedWithOnlyFinishedNameableDependencies() {
        let plan = [slice("dep-1", "Fix the parser", status: "Done")]
        XCTAssertEqual(
            dependencyLine(dependsOn: ["dep-1", "gone-2"], blocked: true, plan: plan),
            "Waits on 2 slices"
        )
    }

    // MARK: - dependencyEntries

    func testEntriesEmptyForNoDependencies() {
        XCTAssertTrue(dependencyEntries(nil, plan: []).isEmpty)
        XCTAssertTrue(dependencyEntries([], plan: []).isEmpty)
    }

    /// Resolves in the order the relation names them, each with its own
    /// finished flag rather than one collapsed line.
    func testEntriesResolveInOrderWithDoneFlags() {
        let plan = [
            slice("dep-1", "Fix the parser", status: "Todo"),
            slice("dep-2", "Ship the client", status: "Done"),
            slice("dep-3", "Wire the poller", status: "In progress"),
        ]
        let entries = dependencyEntries(["dep-2", "dep-1", "dep-3"], plan: plan)
        XCTAssertEqual(entries.map(\.name), ["Ship the client", "Fix the parser", "Wire the poller"])
        XCTAssertEqual(entries.map(\.done), [true, false, false])
    }

    /// Relation IDs come dashed and the plan's may not (or the reverse); the
    /// match strips them, so the same page always resolves.
    func testEntriesMatchIDsWhateverTheirDashes() {
        let plan = [slice("3b738308f654815fa843dce9c020efb4", "Fix the parser", status: "Todo")]
        let entries = dependencyEntries(["3b738308-f654-815f-a843-dce9c020efb4"], plan: plan)
        XCTAssertEqual(entries.map(\.name), ["Fix the parser"])
        XCTAssertEqual(entries.map(\.done), [false])
    }

    /// An ID the plan cannot name is skipped rather than guessed at.
    func testEntriesSkipUnresolvableIDs() {
        let plan = [slice("dep-1", "Fix the parser", status: "Todo")]
        let entries = dependencyEntries(["dep-1", "gone-2"], plan: plan)
        XCTAssertEqual(entries.map(\.name), ["Fix the parser"])
    }
}
