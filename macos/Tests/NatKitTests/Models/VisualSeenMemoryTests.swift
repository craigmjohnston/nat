import XCTest
@testable import NatKit

final class VisualSeenMemoryTests: XCTestCase {
    private let a = VisualChange(index: 1, name: "A", uri: "/a.png", hash: "1", changed: true)
    private let b = VisualChange(index: 2, name: "B", uri: "/b.png", hash: "2", changed: true)

    func testMarksAreKeptPerProjectSliceAndIdentityInTheDefaults() throws {
        let defaults = try XCTUnwrap(UserDefaults(suiteName: "VisualSeenMemoryTests-\(UUID().uuidString)"))
        let memory = VisualSeenMemory(defaults: defaults)
        XCTAssertFalse(memory.isSeen(projectID: "p", sliceID: "s", a))
        memory.markSeen(projectID: "p", sliceID: "s", a)
        memory.markSeen(projectID: "p", sliceID: "s", a)
        XCTAssertEqual(defaults.stringArray(forKey: VisualSeenMemory.storageKey)?.count, 1, "seen twice is one mark")
        XCTAssertTrue(VisualSeenMemory(defaults: defaults).isSeen(projectID: "p", sliceID: "s", a), "survives a relaunch")
        XCTAssertFalse(memory.isSeen(projectID: "p", sliceID: "t", a))
        XCTAssertFalse(memory.isSeen(projectID: "q", sliceID: "s", a))
        XCTAssertFalse(memory.isSeen(projectID: "p", sliceID: "s",
                                     VisualChange(index: 1, name: "A", uri: "/a.png", hash: "9", changed: true)))
    }

    func testRetainDropsOnlyThatSlicesMarksOnItemsGone() {
        let memory = VisualSeenMemory.inMemory()
        memory.markSeen(projectID: "p", sliceID: "s", a)
        memory.markSeen(projectID: "p", sliceID: "s", b)
        memory.markSeen(projectID: "p", sliceID: "t", a)
        memory.retain(projectID: "p", sliceID: "s", [b])
        XCTAssertFalse(memory.isSeen(projectID: "p", sliceID: "s", a))
        XCTAssertTrue(memory.isSeen(projectID: "p", sliceID: "s", b))
        XCTAssertTrue(memory.isSeen(projectID: "p", sliceID: "t", a))
        memory.retain(projectID: "p", sliceID: "s", [b])
        XCTAssertTrue(memory.isSeen(projectID: "p", sliceID: "s", b))
    }
}
