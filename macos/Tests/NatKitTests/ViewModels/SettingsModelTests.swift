import XCTest
@testable import NatKit

final class SettingsModelTests: XCTestCase {
    private func fields(
        poll: String = "",
        workshopModel: String = "", workshopEffort: String = "",
        sliceModel: String = "", sliceEffort: String = "",
        projects: [String: String] = [:]
    ) -> SettingsFields {
        SettingsFields(
            pollSeconds: poll,
            workshopModel: workshopModel, workshopEffort: workshopEffort,
            sliceModel: sliceModel, sliceEffort: sliceEffort,
            projectWorkingDirs: projects
        )
    }

    func testFromConfigDocReadsUnsetNumbersAsEmpty() {
        let doc = ConfigDoc(
            agentSplitPercent: 0, pollSeconds: 0,
            workshopAgent: AgentModel(), sliceAgent: AgentModel(),
            projects: [:]
        )

        let fields = SettingsFields(from: doc)

        XCTAssertEqual(fields.pollSeconds, "")
        XCTAssertEqual(fields.workshopModel, "")
        XCTAssertEqual(fields.sliceEffort, "")
    }

    func testFromConfigDocReadsSetValues() {
        let doc = ConfigDoc(
            agentSplitPercent: 70, pollSeconds: 45,
            workshopAgent: AgentModel(model: "sonnet", effort: "low"),
            sliceAgent: AgentModel(model: "opus", effort: "high"),
            projects: ["p1": ConfigDocProject(name: "Project 1", workingDir: "/repo")]
        )

        let fields = SettingsFields(from: doc)

        XCTAssertEqual(fields.pollSeconds, "45")
        XCTAssertEqual(fields.workshopModel, "sonnet")
        XCTAssertEqual(fields.workshopEffort, "low")
        XCTAssertEqual(fields.sliceModel, "opus")
        XCTAssertEqual(fields.sliceEffort, "high")
        XCTAssertEqual(fields.projectWorkingDirs["p1"], "/repo")
    }

    func testNoChangesProducesNoWrites() {
        let original = fields(poll: "30", projects: ["p1": "/repo"])
        let changes = SettingsModel.changes(from: original, to: original)
        XCTAssertTrue(changes.isEmpty)
    }

    func testChangedPollSecondsProducesOneWrite() {
        let original = fields(poll: "30")
        let edited = fields(poll: "45")

        let changes = SettingsModel.changes(from: original, to: edited)

        XCTAssertEqual(changes, [ConfigChange(key: "poll_seconds", value: "45")])
    }

    func testClearingAFieldWritesEmptyString() {
        let original = fields(poll: "30")
        let edited = fields(poll: "")

        let changes = SettingsModel.changes(from: original, to: edited)

        XCTAssertEqual(changes, [ConfigChange(key: "poll_seconds", value: "")])
    }

    func testEveryScalarFieldChangeProducesItsOwnKey() {
        let original = fields()
        let edited = fields(
            poll: "45",
            workshopModel: "sonnet", workshopEffort: "low",
            sliceModel: "opus", sliceEffort: "high"
        )

        let changes = SettingsModel.changes(from: original, to: edited)

        XCTAssertEqual(Set(changes.map(\.key)), Set([
            "poll_seconds",
            "workshop_agent.model", "workshop_agent.effort",
            "slice_agent.model", "slice_agent.effort"
        ]))
    }

    func testProjectWorkingDirChangeUsesTheProjectKey() {
        let original = fields(projects: ["p1": "/old"])
        let edited = fields(projects: ["p1": "/new"])

        let changes = SettingsModel.changes(from: original, to: edited)

        XCTAssertEqual(changes, [ConfigChange(key: "project.p1.working_dir", value: "/new")])
    }

    func testMultipleProjectChangesAreSortedByID() {
        let original = fields(projects: ["z-proj": "/old-z", "a-proj": "/old-a"])
        let edited = fields(projects: ["z-proj": "/new-z", "a-proj": "/new-a"])

        let changes = SettingsModel.changes(from: original, to: edited)

        XCTAssertEqual(changes, [
            ConfigChange(key: "project.a-proj.working_dir", value: "/new-a"),
            ConfigChange(key: "project.z-proj.working_dir", value: "/new-z")
        ])
    }

    func testUnchangedProjectProducesNoWrite() {
        let original = fields(projects: ["p1": "/repo", "p2": "/repo2"])
        let edited = fields(projects: ["p1": "/repo", "p2": "/changed"])

        let changes = SettingsModel.changes(from: original, to: edited)

        XCTAssertEqual(changes, [ConfigChange(key: "project.p2.working_dir", value: "/changed")])
    }

    func testWorkingDirKeyFormat() {
        XCTAssertEqual(SettingsModel.workingDirKey(projectID: "abc-123"), "project.abc-123.working_dir")
    }

    func testApplyingMovesTheBaselineForwardOnlyForSucceededKeys() {
        let original = fields(poll: "30", projects: ["p1": "/old", "p2": "/kept"])
        let succeeded = [
            ConfigChange(key: "project.p1.working_dir", value: "/new")
        ]

        let result = SettingsModel.applying(succeeded, to: original)

        XCTAssertEqual(result.pollSeconds, "30")
        XCTAssertEqual(result.projectWorkingDirs["p2"], "/kept")
        XCTAssertEqual(result.projectWorkingDirs["p1"], "/new")
    }

    func testApplyingHandlesEveryScalarKey() {
        let original = fields()
        let all = [
            ConfigChange(key: "poll_seconds", value: "45"),
            ConfigChange(key: "workshop_agent.model", value: "sonnet"),
            ConfigChange(key: "workshop_agent.effort", value: "low"),
            ConfigChange(key: "slice_agent.model", value: "opus"),
            ConfigChange(key: "slice_agent.effort", value: "high")
        ]

        let result = SettingsModel.applying(all, to: original)

        XCTAssertEqual(result.pollSeconds, "45")
        XCTAssertEqual(result.workshopModel, "sonnet")
        XCTAssertEqual(result.workshopEffort, "low")
        XCTAssertEqual(result.sliceModel, "opus")
        XCTAssertEqual(result.sliceEffort, "high")
    }

    /// A key the form no longer writes — `agent_split_percent`, which stays
    /// in nat's config for the TUI — is neither a scalar field nor a
    /// project's working directory, and moves nothing.
    func testApplyingIgnoresAKeyTheFormDoesNotHold() {
        let original = fields(poll: "30", projects: ["p1": "/repo"])

        let result = SettingsModel.applying(
            [ConfigChange(key: "agent_split_percent", value: "70")], to: original
        )

        XCTAssertEqual(result, original)
    }

    func testApplyingWithNoChangesReturnsFieldsUnchanged() {
        let original = fields(poll: "30", projects: ["p1": "/repo"])
        let result = SettingsModel.applying([], to: original)
        XCTAssertEqual(result, original)
    }
}
