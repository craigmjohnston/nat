import XCTest
@testable import NatKit

final class ConfigDocTests: XCTestCase {
    func testDecodingWithModelPairsSet() throws {
        let json = """
        {
          "agent_split_percent": 65,
          "poll_seconds": 30,
          "workshop_agent": {"model": "sonnet", "effort": "low"},
          "slice_agent": {"model": "opus", "effort": "high"},
          "projects": {
            "proj-1": {"name": "Example Project", "working_dir": "/path/to/repo"}
          }
        }
        """

        let data = json.data(using: .utf8)!
        let doc = try JSONDecoder().decode(ConfigDoc.self, from: data)

        XCTAssertEqual(doc.agentSplitPercent, 65)
        XCTAssertEqual(doc.pollSeconds, 30)
        XCTAssertEqual(doc.workshopAgent.model, "sonnet")
        XCTAssertEqual(doc.workshopAgent.effort, "low")
        XCTAssertEqual(doc.sliceAgent.model, "opus")
        XCTAssertEqual(doc.sliceAgent.effort, "high")
        XCTAssertEqual(doc.projects.count, 1)
        XCTAssertEqual(doc.projects["proj-1"]?.name, "Example Project")
        XCTAssertEqual(doc.projects["proj-1"]?.workingDir, "/path/to/repo")
    }

    // Go's `omitempty` drops both fields of an unset model pair entirely, so
    // the object decodes as `{}` — both fields should come back nil rather
    // than fail the whole read.
    func testDecodingWithUnsetModelPairsAndNoProjects() throws {
        let json = """
        {
          "agent_split_percent": 0,
          "poll_seconds": 0,
          "workshop_agent": {},
          "slice_agent": {},
          "projects": {}
        }
        """

        let data = json.data(using: .utf8)!
        let doc = try JSONDecoder().decode(ConfigDoc.self, from: data)

        XCTAssertEqual(doc.agentSplitPercent, 0)
        XCTAssertEqual(doc.pollSeconds, 0)
        XCTAssertNil(doc.workshopAgent.model)
        XCTAssertNil(doc.workshopAgent.effort)
        XCTAssertNil(doc.sliceAgent.model)
        XCTAssertNil(doc.sliceAgent.effort)
        XCTAssertTrue(doc.projects.isEmpty)
    }

    func testEquality() {
        let a = ConfigDoc(
            agentSplitPercent: 65, pollSeconds: 30,
            workshopAgent: AgentModel(model: "sonnet", effort: nil),
            sliceAgent: AgentModel(),
            projects: ["p1": ConfigDocProject(name: "P1", workingDir: "/p1")]
        )
        let b = a
        var c = a
        c = ConfigDoc(
            agentSplitPercent: 70, pollSeconds: a.pollSeconds,
            workshopAgent: a.workshopAgent, sliceAgent: a.sliceAgent, projects: a.projects
        )

        XCTAssertEqual(a, b)
        XCTAssertNotEqual(a, c)
    }
}
