import XCTest
@testable import NatKit

final class SeenMemoryTests: XCTestCase {
    func testTheRuleNothingBeforeTheFirstLookThenNewAndUpdated() {
        XCTAssertNil(seenBadge(snapshot: nil, item: "a", fingerprint: "1"), "never looked at")
        XCTAssertEqual(seenBadge(snapshot: [:], item: "a", fingerprint: "1"), .new)
        XCTAssertEqual(seenBadge(snapshot: ["a": "0"], item: "a", fingerprint: "1"), .updated)
        XCTAssertNil(seenBadge(snapshot: ["a": "1"], item: "a", fingerprint: "1"))
    }

    func testSnapshotsAreKeptPerProjectSliceAndSectionInTheDefaults() throws {
        let defaults = try XCTUnwrap(UserDefaults(suiteName: "SeenMemoryTests-\(UUID().uuidString)"))
        let memory = SeenMemory(defaults: defaults)
        XCTAssertNil(memory.snapshot(projectID: "p", sliceID: "s", .changes))
        memory.markSeen(projectID: "p", sliceID: "s", .changes, item: "a", fingerprint: "1")
        XCTAssertNil(memory.snapshot(projectID: "p", sliceID: "s", .changes), "a mark before the first look is none")

        memory.baseline(projectID: "p", sliceID: "s", .changes, ["a": "1", "b": "2"])
        memory.baseline(projectID: "p", sliceID: "s", .changes, ["z": "9"])
        XCTAssertEqual(memory.snapshot(projectID: "p", sliceID: "s", .changes), ["a": "1", "b": "2"], "a first look once")
        XCTAssertEqual(
            SeenMemory(defaults: defaults).snapshot(projectID: "p", sliceID: "s", .changes), ["a": "1", "b": "2"],
            "survives a relaunch")
        XCTAssertNil(memory.snapshot(projectID: "p", sliceID: "s", .visuals))
        XCTAssertNil(memory.snapshot(projectID: "p", sliceID: "t", .changes))
        XCTAssertNil(memory.snapshot(projectID: "q", sliceID: "s", .changes))

        XCTAssertEqual(memory.badge(projectID: "p", sliceID: "s", .changes, item: "a", fingerprint: "7"), .updated)
        memory.markSeen(projectID: "p", sliceID: "s", .changes, item: "a", fingerprint: "7")
        memory.markSeen(projectID: "p", sliceID: "s", .changes, item: "c", fingerprint: "3")
        XCTAssertEqual(memory.snapshot(projectID: "p", sliceID: "s", .changes), ["a": "7", "b": "2", "c": "3"])
        XCTAssertNil(memory.badge(projectID: "p", sliceID: "s", .changes, item: "a", fingerprint: "7"))
    }

    func testRetainDropsOnlyThatSectionsItemsGone() {
        let memory = SeenMemory.inMemory()
        memory.retain(projectID: "p", sliceID: "s", .visuals, items: [])
        XCTAssertNil(memory.snapshot(projectID: "p", sliceID: "s", .visuals), "nothing to prune makes no snapshot")
        memory.baseline(projectID: "p", sliceID: "s", .visuals, ["A": "1", "B": "2"])
        memory.baseline(projectID: "p", sliceID: "t", .visuals, ["A": "1"])
        memory.retain(projectID: "p", sliceID: "s", .visuals, items: ["B"])
        XCTAssertEqual(memory.snapshot(projectID: "p", sliceID: "s", .visuals), ["B": "2"])
        XCTAssertEqual(memory.snapshot(projectID: "p", sliceID: "t", .visuals), ["A": "1"])
        memory.retain(projectID: "p", sliceID: "s", .visuals, items: ["B"])
        XCTAssertEqual(memory.snapshot(projectID: "p", sliceID: "s", .visuals), ["B": "2"])
    }

    func testASectionsStatusIsNewOverUpdated() {
        XCTAssertNil(NavSectionStatus.of([nil, nil]))
        XCTAssertEqual(NavSectionStatus.of([nil, .updated]), .updated)
        XCTAssertEqual(NavSectionStatus.of([.updated, .new]), .new)
        XCTAssertEqual(NavSectionStatus(SeenBadge.new), .new)
        XCTAssertEqual(NavSectionStatus(SeenBadge.updated), .updated)
        XCTAssertNil(NavSectionStatus(nil))
        XCTAssertEqual([NavSectionStatus.merged, .new, .updated].map(\.label), ["Merged", "New", "Updated"])
    }
}
