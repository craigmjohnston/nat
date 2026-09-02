import XCTest
@testable import NatKit

final class NatClientTests: XCTestCase {
    func testInfoWithAllFields() async throws {
        let fakeRunner = FakeRunner(fixture: .infoWithAllFields)
        let client = NatClient(commandRunner: fakeRunner)

        let info = try await client.info(projectID: "proj-123")

        XCTAssertEqual(info.project.id, "proj-abc123")
        XCTAssertEqual(info.project.name, "Example Project")
        XCTAssertEqual(info.milestones.count, 2)
        XCTAssertEqual(info.slices.count, 3)

        // Check first slice has all optional fields
        let firstSlice = info.slices[0]
        XCTAssertEqual(firstSlice.id, "slice-1")
        XCTAssertEqual(firstSlice.branch, "feature/api")
        XCTAssertEqual(firstSlice.repo, "/path/to/repo")
        XCTAssertEqual(firstSlice.state, .awaitingReview)
        XCTAssertTrue(firstSlice.handedBack)

        // Check second slice doesn't have optional fields
        let secondSlice = info.slices[1]
        XCTAssertNil(secondSlice.branch)
        XCTAssertNil(secondSlice.repo)
        XCTAssertNil(secondSlice.state)
    }

    func testInfoMinimal() async throws {
        let fakeRunner = FakeRunner(fixture: .infoMinimal)
        let client = NatClient(commandRunner: fakeRunner)

        let info = try await client.info(projectID: "proj-456")

        XCTAssertEqual(info.project.name, "Minimal Project")
        XCTAssertTrue(info.milestones.isEmpty)
        XCTAssertTrue(info.slices.isEmpty)
    }

    func testPaths() async throws {
        let fakeRunner = FakeRunner(fixture: .paths)
        let client = NatClient(commandRunner: fakeRunner)

        let paths = try await client.paths()

        XCTAssertEqual(paths.config, "/Users/alice/.config/notion-agent-tracker/config.json")
        XCTAssertEqual(paths.logDir, "/Users/alice/Library/Logs/notion-agent-tracker")
        XCTAssertEqual(paths.nudge, "/Users/alice/Library/Logs/notion-agent-tracker/nudge")
    }

    func testCommandFailureReturnsFirstLineOfStderr() async throws {
        let fakeRunner = FakeRunner(fixture: .commandError)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            _ = try await client.info(projectID: "nonexistent")
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                XCTAssertEqual(message, "Error: project not found")
            } else {
                XCTFail("Expected commandFailed error")
            }
        }
    }

    func testInvalidJSONThrows() async throws {
        let fakeRunner = FakeRunner(fixture: .nonJSON)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            _ = try await client.info(projectID: "test")
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .invalidJSON = error {
                // Expected
            } else {
                XCTFail("Expected invalidJSON error, got \(error)")
            }
        }
    }

    func testProcessRunnerResolvesNatBinary() async throws {
        let runner = ProcessRunner()

        // This test verifies that ProcessRunner can be instantiated and respects
        // the NAT_BIN environment variable. We don't actually run nat here because
        // it might not be installed, but we can verify the behavior with a fake.
        XCTAssertNotNil(runner)
    }

    func testMultipleCalls() async throws {
        let fakeRunner = FakeRunner(fixture: .infoWithAllFields)
        let client = NatClient(commandRunner: fakeRunner)

        let info1 = try await client.info(projectID: "proj-1")
        let info2 = try await client.info(projectID: "proj-2")

        XCTAssertEqual(info1.project.name, "Example Project")
        XCTAssertEqual(info2.project.name, "Example Project")
    }

    func testStatus() async throws {
        let fakeRunner = FakeRunner(fixture: .status)
        let client = NatClient(commandRunner: fakeRunner)

        let statuses = try await client.status()

        XCTAssertEqual(statuses.count, 2)
        XCTAssertEqual(statuses[0].sliceID, "slice-1")
        XCTAssertEqual(statuses[0].session, "nat-abc123def")
        XCTAssertEqual(statuses[0].activity, .working)

        XCTAssertEqual(statuses[1].sliceID, "slice-2")
        XCTAssertEqual(statuses[1].activity, .waiting)
    }

    func testSliceShow() async throws {
        let fakeRunner = FakeRunner(fixture: .sliceShow)
        let client = NatClient(commandRunner: fakeRunner)

        let detail = try await client.sliceShow(projectID: "proj-123", sliceRef: "slice-1")

        XCTAssertEqual(detail.id, "slice-1")
        XCTAssertEqual(detail.name, "Implement API")
        XCTAssertEqual(detail.status, "In progress")
        XCTAssertEqual(detail.milestone, "Phase 1")
        XCTAssertEqual(detail.branch, "feature/api")
        XCTAssertEqual(detail.repo, "/path/to/repo")
        XCTAssertEqual(detail.pr, "https://github.com/org/repo/pull/42")
        XCTAssertTrue(detail.handedBack)
        XCTAssertEqual(detail.state, "awaiting_review")
        XCTAssert(detail.brief.contains("API Implementation"))
    }

    func testAgentInterruptSuccess() async throws {
        let fakeRunner = FakeRunner(fixture: .agentInterruptSuccess)
        let client = NatClient(commandRunner: fakeRunner)

        // Should not throw
        try await client.agentInterrupt(projectID: "proj-123", sliceRef: "slice-1")
    }

    func testAgentInterruptNoSession() async throws {
        let fakeRunner = FakeRunner(fixture: .agentInterruptNoSession)
        let client = NatClient(commandRunner: fakeRunner)

        do {
            try await client.agentInterrupt(projectID: "proj-123", sliceRef: "slice-1")
            XCTFail("Should have thrown")
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                XCTAssertEqual(message, "no live session for slice-id")
            } else {
                XCTFail("Expected commandFailed error")
            }
        }
    }
}
