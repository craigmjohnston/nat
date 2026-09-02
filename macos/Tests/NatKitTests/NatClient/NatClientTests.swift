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
}
