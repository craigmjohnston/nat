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

    // A mixed config — some projects in Notion, some kept in a file of nat's
    // own — is what the app has to load once a machine tracks both, and the
    // half it has never heard of must not take the other half down with it.
    func testDecodingAMixedConfig() throws {
        let json = """
        {
          "agent_split_percent": 0,
          "poll_seconds": 0,
          "workshop_agent": {},
          "slice_agent": {},
          "projects": {
            "proj-1": {"name": "In Notion", "working_dir": "/a", "backend": "notion"},
            "proj-2": {"name": "On this machine", "working_dir": "/b", "backend": "local",
                       "plan_dir": "/plans"}
          }
        }
        """

        let doc = try JSONDecoder().decode(ConfigDoc.self, from: json.data(using: .utf8)!)

        XCTAssertEqual(doc.projects.count, 2)
        XCTAssertEqual(doc.projects["proj-1"]?.backend, "notion")
        XCTAssertEqual(doc.projects["proj-1"]?.isLocal, false)
        XCTAssertEqual(doc.projects["proj-1"]?.planDir, "")
        XCTAssertEqual(doc.projects["proj-2"]?.backend, "local")
        XCTAssertEqual(doc.projects["proj-2"]?.isLocal, true)
        XCTAssertEqual(doc.projects["proj-2"]?.planDir, "/plans")
    }

    // A listing printed by a nat that had never heard of the choice names no
    // backend at all, which is a project kept in Notion because that is all
    // there was to keep it in.
    func testAProjectWithNoBackendReadsAsNotion() throws {
        let json = """
        {"agent_split_percent": 0, "poll_seconds": 0,
         "workshop_agent": {}, "slice_agent": {},
         "projects": {"proj-1": {"name": "Old", "working_dir": "/a"}}}
        """

        let doc = try JSONDecoder().decode(ConfigDoc.self, from: json.data(using: .utf8)!)

        XCTAssertEqual(doc.projects["proj-1"]?.backend, "notion")
        XCTAssertEqual(doc.projects["proj-1"]?.isLocal, false)
    }

    // A word a later nat invented is not the local one, and reading it as
    // local would claim a plan file that is not there.
    func testAnUnknownBackendReadsAsNotion() throws {
        let p = ConfigDocProject(name: "P", workingDir: "/a", backend: "someday")
        XCTAssertFalse(p.isLocal)
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
