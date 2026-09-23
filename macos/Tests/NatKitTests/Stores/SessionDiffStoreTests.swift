import XCTest
@testable import NatKit

private final class BranchRecordingClient: NatClientProtocol, @unchecked Sendable {
    private(set) var branches: [String?] = []
    func info(projectID: String) async throws -> ProjectInfo { throw NatError.missingOutput }
    func status() async throws -> [AgentStatus] { [] }
    func usage() async throws -> UsageReading { .empty }
    func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail { throw NatError.missingOutput }
    func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff { throw NatError.missingOutput }
    func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc { throw NatError.missingOutput }
    func sliceEdit(projectID: String, sliceRef: String, description: String) async throws -> SliceEditResult { throw NatError.missingOutput }
    func sliceLaunch(projectID: String, sliceRef: String, model: String?, effort: String?) async throws -> LaunchResult { throw NatError.missingOutput }
    func agentSend(projectID: String, sliceRef: String, text: String) async throws {}
    func agentKill(projectID: String, sliceRef: String) async throws {}
    func agentKillWorkshop(projectID: String) async throws {}
    func sliceStatus(projectID: String, sliceRef: String) async throws -> SliceStatusResult { throw NatError.missingOutput }
    func sliceApprove(projectID: String, sliceRef: String) async throws -> String { "" }
    func prView(projectID: String, sliceRef: String) async throws -> PRDetail { throw NatError.missingOutput }
    func prStatus(projectID: String) async throws -> PRStatusDoc { throw NatError.missingOutput }
    func prMerge(projectID: String, sliceRef: String) async throws {}
    func prComment(projectID: String, sliceRef: String, body: String) async throws {}
    func workshopLaunch(projectID: String, model: String?, effort: String?, request: String?) async throws -> WorkshopLaunchResult { throw NatError.missingOutput }
    func sliceAdd(projectID: String, title: String, milestone: String, description: String?) async throws -> SliceAddResult { throw NatError.missingOutput }
    func configShow() async throws -> ConfigDoc { throw NatError.missingOutput }
    func configSet(key: String, value: String) async throws {}

    func sessionDiff(projectID: String, sessionID: String, branch: String?) async throws -> SliceDiff {
        branches.append(branch)
        return SliceDiff(base: "main", branch: branch ?? "current", files: [])
    }
}

final class SessionDiffStoreTests: XCTestCase {
    @MainActor
    func testFetchReadsTheNamedBranchAndReReadsOnASwitch() async {
        let client = BranchRecordingClient()
        let store = SessionDiffStore(client: client)

        await store.fetch(projectID: "p", sessionID: "s")
        await store.fetch(projectID: "p", sessionID: "s")
        XCTAssertEqual(client.branches, [nil], "the checked-out branch, read once")

        store.toggleViewed("a.swift")
        await store.fetch(projectID: "p", sessionID: "s", branch: "session/other")
        XCTAssertEqual(client.branches, [nil, "session/other"])
        XCTAssertFalse(store.isViewed("a.swift"), "marks are of one diff's own files")

        await store.refresh(projectID: "p")
        XCTAssertEqual(client.branches, [nil, "session/other", "session/other"], "a refresh follows the picked branch")

        store.clear()
        await store.refresh(projectID: "p")
        XCTAssertEqual(client.branches.count, 3, "nothing to refresh once cleared")
    }
}
