import Foundation
@testable import NatKit

/// A fake CommandRunning for testing that returns pre-configured fixture data.
final class FakeRunner: CommandRunning, @unchecked Sendable {
    enum Fixture {
        case infoWithAllFields
        case infoMinimal
        case paths
        case commandError
        case nonJSON
    }

    private var fixture: Fixture
    private var shouldFailWith: Error?

    init(fixture: Fixture = .infoWithAllFields) {
        self.fixture = fixture
    }

    func run(
        executable: String,
        arguments: [String],
        workingDirectory: String?,
        standardInput: Data?
    ) async throws -> (stdout: Data, stderr: Data, exitCode: Int32) {
        if let error = shouldFailWith {
            throw error
        }

        switch fixture {
        case .infoWithAllFields:
            return (fixtureInfoFull.data(using: .utf8)!, Data(), 0)
        case .infoMinimal:
            return (fixtureInfoMinimal.data(using: .utf8)!, Data(), 0)
        case .paths:
            return (fixturePaths.data(using: .utf8)!, Data(), 0)
        case .commandError:
            return (Data(), "Error: project not found".data(using: .utf8)!, 1)
        case .nonJSON:
            return ("This is not JSON".data(using: .utf8)!, Data(), 0)
        }
    }

    func setFixture(_ fixture: Fixture) {
        self.fixture = fixture
    }

    func setError(_ error: Error) {
        self.shouldFailWith = error
    }
}

// MARK: - Fixtures

let fixtureInfoFull = """
{
  "project": {
    "id": "proj-abc123",
    "name": "Example Project",
    "conventions": "Use TDD for all features"
  },
  "milestones": [
    {
      "id": "m1",
      "name": "Phase 1",
      "order": 1.0,
      "status": "Active"
    },
    {
      "id": "m2",
      "name": "Phase 2",
      "order": 2.0,
      "status": "Queued"
    }
  ],
  "slices": [
    {
      "id": "slice-1",
      "name": "Implement core API",
      "status": "In progress",
      "milestone_id": "m1",
      "assignee": "Alice",
      "pr": "https://github.com/org/repo/pull/42",
      "url": "https://notion.so/slice-1",
      "branch": "feature/api",
      "repo": "/path/to/repo",
      "depends_on": [],
      "blocked": false,
      "handed_back": true,
      "state": "awaiting review"
    },
    {
      "id": "slice-2",
      "name": "Add tests",
      "status": "Todo",
      "milestone_id": "m1",
      "assignee": "",
      "pr": "",
      "url": "https://notion.so/slice-2",
      "depends_on": ["slice-1"],
      "blocked": true,
      "handed_back": false
    },
    {
      "id": "slice-3",
      "name": "Future work",
      "status": "Todo",
      "milestone_id": "m2",
      "assignee": "",
      "pr": "",
      "url": "https://notion.so/slice-3",
      "depends_on": [],
      "blocked": false,
      "handed_back": false
    }
  ]
}
"""

let fixtureInfoMinimal = """
{
  "project": {
    "id": "proj-def456",
    "name": "Minimal Project",
    "conventions": ""
  },
  "milestones": [],
  "slices": []
}
"""

let fixturePaths = """
{
  "config": "/Users/alice/.config/notion-agent-tracker/config.json",
  "log_dir": "/Users/alice/Library/Logs/notion-agent-tracker",
  "nudge": "/Users/alice/Library/Logs/notion-agent-tracker/nudge"
}
"""
