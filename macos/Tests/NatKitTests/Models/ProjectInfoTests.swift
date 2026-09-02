import XCTest
@testable import NatKit

final class ProjectInfoTests: XCTestCase {
    func testProjectInfoDecoding() throws {
        let json = """
        {
          "project": {
            "id": "proj-123",
            "name": "Test Project",
            "conventions": "Use TDD"
          },
          "milestones": [
            {
              "id": "m1",
              "name": "Phase 1",
              "order": 1.0,
              "status": "Active"
            }
          ],
          "slices": [
            {
              "id": "s1",
              "name": "First slice",
              "status": "In progress",
              "milestone_id": "m1",
              "assignee": "Alice",
              "pr": "",
              "url": "https://notion.so/s1",
              "blocked": false,
              "handed_back": false
            }
          ]
        }
        """

        let data = json.data(using: .utf8)!
        let info = try JSONDecoder().decode(ProjectInfo.self, from: data)

        XCTAssertEqual(info.project.id, "proj-123")
        XCTAssertEqual(info.project.name, "Test Project")
        XCTAssertEqual(info.project.conventions, "Use TDD")
        XCTAssertEqual(info.milestones.count, 1)
        XCTAssertEqual(info.slices.count, 1)
    }

    func testSliceWithAllOptionalFields() throws {
        let json = """
        {
          "id": "s1",
          "name": "Full slice",
          "status": "In progress",
          "milestone_id": "m1",
          "assignee": "Alice",
          "pr": "https://github.com/org/repo/pull/123",
          "url": "https://notion.so/s1",
          "branch": "feature/add-thing",
          "repo": "/path/to/repo",
          "depends_on": ["s0", "s2"],
          "blocked": true,
          "handed_back": true,
          "state": "awaiting review"
        }
        """

        let data = json.data(using: .utf8)!
        let slice = try JSONDecoder().decode(Slice.self, from: data)

        XCTAssertEqual(slice.id, "s1")
        XCTAssertEqual(slice.branch, "feature/add-thing")
        XCTAssertEqual(slice.repo, "/path/to/repo")
        XCTAssertEqual(slice.dependsOn, ["s0", "s2"])
        XCTAssertEqual(slice.blocked, true)
        XCTAssertEqual(slice.handedBack, true)
        XCTAssertEqual(slice.state, .awaitingReview)
    }

    func testSliceWithoutOptionalFields() throws {
        let json = """
        {
          "id": "s2",
          "name": "Minimal slice",
          "status": "Todo",
          "milestone_id": "m1",
          "assignee": "",
          "pr": "",
          "url": "https://notion.so/s2",
          "blocked": false,
          "handed_back": false
        }
        """

        let data = json.data(using: .utf8)!
        let slice = try JSONDecoder().decode(Slice.self, from: data)

        XCTAssertNil(slice.branch)
        XCTAssertNil(slice.repo)
        XCTAssertNil(slice.dependsOn)
        XCTAssertNil(slice.state)
    }

    func testMilestoneDecoding() throws {
        let json = """
        {
          "id": "m-uuid",
          "name": "Beta Release",
          "order": 2.5,
          "status": "Queued"
        }
        """

        let data = json.data(using: .utf8)!
        let milestone = try JSONDecoder().decode(Milestone.self, from: data)

        XCTAssertEqual(milestone.id, "m-uuid")
        XCTAssertEqual(milestone.name, "Beta Release")
        XCTAssertEqual(milestone.order, 2.5)
        XCTAssertEqual(milestone.status, "Queued")
    }

    func testNatPathsDecoding() throws {
        let json = """
        {
          "config": "/Users/alice/.config/notion-agent-tracker/config.json",
          "log_dir": "/Users/alice/Library/Logs/notion-agent-tracker",
          "nudge": "/Users/alice/Library/Logs/notion-agent-tracker/nudge"
        }
        """

        let data = json.data(using: .utf8)!
        let paths = try JSONDecoder().decode(NatPaths.self, from: data)

        XCTAssertEqual(paths.config, "/Users/alice/.config/notion-agent-tracker/config.json")
        XCTAssertEqual(paths.logDir, "/Users/alice/Library/Logs/notion-agent-tracker")
        XCTAssertEqual(paths.nudge, "/Users/alice/Library/Logs/notion-agent-tracker/nudge")
    }

    func testNatProjectConfigDecoding() throws {
        let json = """
        {
          "projects": {
            "proj-1": {
              "name": "Project A",
              "slices_ds_id": "ds-1",
              "working_dir": "/path/to/project-a"
            }
          },
          "agent_split_percent": 65,
          "poll_seconds": 30,
          "workshop_agent": {
            "model": "claude-opus",
            "effort": "high"
          },
          "slice_agent": {
            "model": "claude-sonnet",
            "effort": "medium"
          }
        }
        """

        let data = json.data(using: .utf8)!
        let config = try JSONDecoder().decode(NatProjectConfig.self, from: data)

        XCTAssertEqual(config.projects.count, 1)
        XCTAssertEqual(config.projects["proj-1"]?.name, "Project A")
        XCTAssertEqual(config.agentSplitPercent, 65)
        XCTAssertEqual(config.pollSeconds, 30)
        XCTAssertEqual(config.workshopAgent?.model, "claude-opus")
        XCTAssertEqual(config.sliceAgent?.effort, "medium")
    }

    func testAgentModelEmpty() {
        let emptyModel = AgentModel()
        XCTAssertTrue(emptyModel.isEmpty)

        let modelWithValue = AgentModel(model: "claude-opus")
        XCTAssertFalse(modelWithValue.isEmpty)
    }

    func testProjectConfigDecoding() throws {
        let json = """
        {
          "name": "My Project",
          "slices_ds_id": "data-source-123",
          "working_dir": "/Users/alice/projects/my-project"
        }
        """

        let data = json.data(using: .utf8)!
        let config = try JSONDecoder().decode(ProjectConfig.self, from: data)

        XCTAssertEqual(config.name, "My Project")
        XCTAssertEqual(config.slicesDSID, "data-source-123")
        XCTAssertEqual(config.workingDir, "/Users/alice/projects/my-project")
    }

    func testProjectInfoEquality() {
        let project1 = Project(id: "p1", name: "Test", conventions: "")
        let project2 = Project(id: "p1", name: "Test", conventions: "")
        XCTAssertEqual(project1, project2)

        let info1 = ProjectInfo(project: project1, milestones: [], slices: [])
        let info2 = ProjectInfo(project: project2, milestones: [], slices: [])
        XCTAssertEqual(info1, info2)
    }
}
