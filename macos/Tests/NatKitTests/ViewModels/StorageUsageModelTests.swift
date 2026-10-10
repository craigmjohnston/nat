import XCTest
import NatFixtures
@testable import NatKit

/// A stub `nat` that answers one canned stdout and records what it was asked.
private final class StorageStubRunner: CommandRunning, @unchecked Sendable {
    let stdout: String
    private(set) var lastArguments: [String] = []

    init(stdout: String) {
        self.stdout = stdout
    }

    func run(
        executable: String, arguments: [String], workingDirectory: String?, standardInput: Data?
    ) async throws -> (stdout: Data, stderr: Data, exitCode: Int32) {
        lastArguments = arguments
        return (Data(stdout.utf8), Data(), 0)
    }
}

/// `nat storage-usage --json` as `internal/cli/storageusage.go` writes it.
private let storageJSON = #"""
{
  "login": "octo",
  "plan": "pro",
  "allowance_gb": 1,
  "year": 2026,
  "month": 10,
  "days_left": 22,
  "total_gb": 0.75,
  "projects": [
    {"id": "p1", "name": "nat", "color": "blue", "repos": ["octo/nat"], "gb": 0.5},
    {"id": "p2", "name": "Cards", "repos": ["octo/cards"], "gb": 0.1},
    {"id": "p3", "name": "loose", "color": "green", "repos": [], "gb": 0}
  ],
  "other": {"gb": 0.15, "repos": [{"repo": "octo/old", "gb": 0.15}]}
}
"""#

/// A stub `nat` answering storage-usage from a queue — stdout on success,
/// stderr and a non-zero exit on a refusal, or a thrown error — counting the
/// calls.
private final class QueuedStorageRunner: CommandRunning, @unchecked Sendable {
    enum Answer {
        case reading(StorageUsage)
        case needsScope(String)
        case refusal(String)
        case thrown(Error)
    }

    var answers: [Answer]
    private(set) var calls = 0

    init(_ answers: [Answer]) {
        self.answers = answers
    }

    func run(
        executable: String, arguments: [String], workingDirectory: String?, standardInput: Data?
    ) async throws -> (stdout: Data, stderr: Data, exitCode: Int32) {
        calls += 1
        switch answers.removeFirst() {
        case .reading(let usage): return (try JSONEncoder().encode(usage), Data(), 0)
        case .needsScope(let command):
            return (Data(#"{"needs_scope": "user", "scope_command": "\#(command)"}"#.utf8), Data(), 0)
        case .refusal(let message): return (Data(), Data(message.utf8), 1)
        case .thrown(let error): throw error
        }
    }
}

private func usage(
    allowance: Double = 1, total: Double = 0.75, other: Double = 0.15, month: Int = 10, daysLeft: Int = 22,
    projects: [StorageUsage.Project]? = nil
) -> StorageUsage {
    StorageUsage(
        login: "octo", plan: "pro", allowanceGB: allowance, year: 2026, month: month, daysLeft: daysLeft,
        totalGB: total,
        projects: projects ?? [
            .init(id: "p1", name: "nat", color: "blue", repos: ["octo/nat"], gb: 0.5),
            .init(id: "p2", name: "Cards", color: nil, repos: ["octo/cards"], gb: 0.1),
            .init(id: "p3", name: "loose", color: "green", repos: [], gb: 0),
        ],
        other: .init(gb: other, repos: [.init(repo: "octo/old", gb: other)]))
}

@MainActor
final class StorageUsageModelTests: XCTestCase {
    func testClientRunsStorageUsageAndDecodesNatsJSON() async throws {
        let runner = StorageStubRunner(stdout: storageJSON)
        let read = try await NatClient(commandRunner: runner).storageUsage()
        XCTAssertEqual(runner.lastArguments, ["storage-usage", "--json"])
        XCTAssertEqual(read, .reading(usage()))

        let scope = StorageStubRunner(stdout: #"{"needs_scope": "user", "scope_command": "gh auth refresh -h github.com -s user"}"#)
        let needs = try await NatClient(commandRunner: scope).storageUsage()
        XCTAssertEqual(needs, .needsScope(command: "gh auth refresh -h github.com -s user"))
    }

    func testAMissingScopeIsItsOwnStateAndCheckingAgainReadsIt() async {
        let client = QueuedStorageRunner([.needsScope("gh auth refresh -h github.com -s user"), .reading(usage())])
        let model = StorageUsageModel(client: NatClient(commandRunner: client))
        await model.loadIfNeeded()
        XCTAssertEqual(model.state, .needsScope(command: "gh auth refresh -h github.com -s user"))
        XCTAssertTrue(model.state.isNeedsScope)
        await model.refresh()
        XCTAssertEqual(model.state, .loaded(usage()))
        XCTAssertFalse(model.state.isNeedsScope)
    }

    func testLoadsOnceAndRefreshesOnAsking() async {
        let client = QueuedStorageRunner([.reading(usage()), .reading(usage(total: 0.9))])
        let model = StorageUsageModel(client: NatClient(commandRunner: client))
        XCTAssertEqual(model.state, .loading)

        await model.loadIfNeeded()
        await model.loadIfNeeded()
        XCTAssertEqual(client.calls, 1, "a section shown again keeps its reading")
        XCTAssertEqual(model.state, .loaded(usage()))

        await model.refresh()
        XCTAssertEqual(client.calls, 2)
        XCTAssertEqual(model.state, .loaded(usage(total: 0.9)))
        XCTAssertFalse(model.isReading)
    }

    func testAFailedReadShowsNatsWordsAndReplacesTheLastReading() async {
        let client = QueuedStorageRunner([
            .reading(usage()),
            .refusal("gh is not signed in"),
            .thrown(CocoaError(.fileNoSuchFile)),
        ])
        let model = StorageUsageModel(client: NatClient(commandRunner: client))
        await model.loadIfNeeded()
        await model.refresh()
        XCTAssertEqual(model.state, .failed("gh is not signed in"))
        await model.refresh()
        guard case .failed(let message) = model.state else { return XCTFail("state = \(model.state)") }
        XCTAssertFalse(message.isEmpty)
    }

    func testOneReadAtATime() async {
        let client = FixtureNatClient(behaviour: .hanging)
        let model = StorageUsageModel(client: client)
        let first = Task { await model.refresh() }
        while !model.isReading { await Task.yield() }
        await model.refresh()  // returns at once: a read is already out
        XCTAssertTrue(model.isReading)
        first.cancel()
    }

    func testSegmentsAreTheProjectsHoldingStorageThenOther() {
        let segments = StorageUsageModel.segments(usage())
        XCTAssertEqual(segments.map(\.id), ["p1", "p2", StorageUsageModel.otherID])
        XCTAssertEqual(segments.map(\.color), [.blue, nil, nil])
        XCTAssertEqual(segments.map(\.isOther), [false, false, true])
        XCTAssertEqual(segments[0].fraction, 0.5, accuracy: 1e-9)
        XCTAssertEqual(segments[2].fraction, 0.15, accuracy: 1e-9)
        XCTAssertEqual(segments[2].name, "Other repositories")
    }

    func testAMonthOverItsAllowanceFillsTheBar() {
        let over = usage(total: 2, other: 1.4)
        XCTAssertTrue(StorageUsageModel.isOver(over))
        XCTAssertFalse(StorageUsageModel.isOver(usage()))
        let total = StorageUsageModel.segments(over).map(\.fraction).reduce(0, +)
        XCTAssertEqual(total, 1, accuracy: 1e-9)
    }

    func testNoAllowanceScalesByTheTotal() {
        let unknown = usage(allowance: 0)
        XCTAssertEqual(StorageUsageModel.segments(unknown)[0].fraction, 0.5 / 0.75, accuracy: 1e-9)
        XCTAssertFalse(StorageUsageModel.isOver(unknown))
        XCTAssertEqual(StorageUsageModel.summary(unknown), "0.75 GB used \u{2014} the plan's allowance is not known")
    }

    func testNothingStoredDrawsNoSegments() {
        let empty = usage(allowance: 0, total: 0, other: 0, projects: [])
        XCTAssertEqual(StorageUsageModel.segments(empty), [])
        XCTAssertEqual(StorageUsageModel.legend(empty).map(\.id), [StorageUsageModel.otherID])
        let noOther = usage(other: 0)
        XCTAssertEqual(StorageUsageModel.segments(noOther).map(\.id), ["p1", "p2"])
    }

    func testLegendListsEveryProjectThenOther() {
        let legend = StorageUsageModel.legend(usage())
        XCTAssertEqual(legend.map(\.name), ["nat", "Cards", "loose", "Other repositories"])
        XCTAssertEqual(legend.map(\.figure), ["0.50 GB", "0.10 GB", "0.00 GB", "0.15 GB"])
        XCTAssertEqual(legend.map(\.color), [.blue, nil, .green, nil])
        XCTAssertEqual(legend[3].repos, ["octo/old"])
        XCTAssertEqual(legend[2].repos, [])
    }

    func testSummaryAndDaysLeft() {
        XCTAssertEqual(StorageUsageModel.summary(usage()), "0.75 GB of 1.00 GB used")
        XCTAssertEqual(StorageUsageModel.daysLeft(usage()), "22 days left in October")
        XCTAssertEqual(StorageUsageModel.daysLeft(usage(month: 2, daysLeft: 1)), "1 day left in February")
        XCTAssertEqual(StorageUsageModel.daysLeft(usage(month: 0, daysLeft: 3)), "3 days left in the month")
    }
}
