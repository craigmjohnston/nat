import XCTest
@testable import NatKit

/// A `PlanCaching` held in memory, for the tests of everything that reads
/// through one — see `ProjectStoreTests` and `AppModelTests`.
final class FakePlanCache: PlanCaching, @unchecked Sendable {
    private(set) var reads: [String] = []
    private(set) var writes: [(projectID: String, info: ProjectInfo)] = []
    var stored: [String: ProjectInfo]

    init(stored: [String: ProjectInfo] = [:]) {
        self.stored = stored
    }

    func read(projectID: String) async -> ProjectInfo? {
        reads.append(projectID)
        return stored[projectID]
    }

    func write(_ info: ProjectInfo, projectID: String) async {
        writes.append((projectID: projectID, info: info))
        stored[projectID] = info
    }
}

final class PlanCacheTests: XCTestCase {
    private var directory: URL!
    private var cache: DiskPlanCache!

    private static func info(name: String, sliceName: String = "Slice") -> ProjectInfo {
        ProjectInfo(
            project: Project(id: "proj-1", name: name, conventions: "Be kind"),
            milestones: [Milestone(id: "M1", name: "M1", order: 0, status: "Active")],
            slices: [
                Slice(
                    id: "slice-1",
                    name: sliceName,
                    status: "Todo",
                    milestoneID: "M1",
                    assignee: "",
                    pr: "",
                    url: "https://example.com/s",
                    branch: "slice/one",
                    repo: "/tmp/repo",
                    dependsOn: ["slice-0"],
                    blocked: false,
                    handedBack: true,
                    state: nil
                )
            ]
        )
    }

    override func setUpWithError() throws {
        try super.setUpWithError()
        directory = URL(fileURLWithPath: NSTemporaryDirectory(), isDirectory: true)
            .appendingPathComponent("plan-cache-tests-" + UUID().uuidString, isDirectory: true)
        cache = DiskPlanCache(directory: directory)
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: directory)
        try super.tearDownWithError()
    }

    func testRoundTrip() async {
        let info = Self.info(name: "Tracker")
        await cache.write(info, projectID: "proj-1")

        let read = await cache.read(projectID: "proj-1")
        XCTAssertEqual(read, info)
    }

    func testWriteReplacesTheLastPlan() async {
        await cache.write(Self.info(name: "Old"), projectID: "proj-1")
        await cache.write(Self.info(name: "New"), projectID: "proj-1")

        let read = await cache.read(projectID: "proj-1")
        XCTAssertEqual(read?.project.name, "New")
    }

    func testPlansAreKeyedByProject() async {
        await cache.write(Self.info(name: "One"), projectID: "proj-1")
        await cache.write(Self.info(name: "Two"), projectID: "proj-2")

        let one = await cache.read(projectID: "proj-1")
        let two = await cache.read(projectID: "proj-2")
        XCTAssertEqual(one?.project.name, "One")
        XCTAssertEqual(two?.project.name, "Two")
    }

    func testMissingFileReadsAsNothing() async {
        let read = await cache.read(projectID: "never-written")
        XCTAssertNil(read)
    }

    func testCorruptFileReadsAsNothing() async throws {
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        try Data("{ not a plan".utf8).write(to: cache.fileURL(projectID: "proj-1"))

        let read = await cache.read(projectID: "proj-1")
        XCTAssertNil(read)
    }

    func testFileNameSlugsTheProjectID() {
        XCTAssertEqual(
            cache.fileURL(projectID: "3b738308-f654-811c").lastPathComponent,
            "3b738308-f654-811c.json"
        )
        XCTAssertEqual(
            cache.fileURL(projectID: "../../etc/passwd").lastPathComponent,
            "etc-passwd.json"
        )
        XCTAssertEqual(cache.fileURL(projectID: "///").lastPathComponent, "project.json")
        XCTAssertEqual(cache.fileURL(projectID: "a b").lastPathComponent, "a-b.json")
    }

    func testDefaultDirectoryIsUnderTheBundleID() {
        let defaultCache = DiskPlanCache()
        XCTAssertEqual(defaultCache.directory.lastPathComponent, "plans")
        XCTAssertEqual(
            defaultCache.directory.deletingLastPathComponent().lastPathComponent,
            DiskPlanCache.bundleID
        )
        XCTAssertEqual(DiskPlanCache.defaultDirectory, defaultCache.directory)
    }
}

/// A latch a test holds a client's read at: it says when the read was asked
/// for, and lets it answer only once the test has looked at the store.
actor Gate {
    private var asked = false
    private var askedWaiters: [CheckedContinuation<Void, Never>] = []
    private var opened = false
    private var openWaiters: [CheckedContinuation<Void, Never>] = []

    func markAsked() {
        asked = true
        for waiter in askedWaiters { waiter.resume() }
        askedWaiters = []
    }

    func waitUntilAsked() async {
        if asked { return }
        await withCheckedContinuation { askedWaiters.append($0) }
    }

    func open() {
        opened = true
        for waiter in openWaiters { waiter.resume() }
        openWaiters = []
    }

    func waitUntilOpen() async {
        if opened { return }
        await withCheckedContinuation { openWaiters.append($0) }
    }
}
