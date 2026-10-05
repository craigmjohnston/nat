import XCTest
@testable import NatKit

final class SettingsModelTests: XCTestCase {
    private func fields(
        poll: String = "",
        workshopModel: String = "", workshopEffort: String = "",
        sliceModel: String = "", sliceEffort: String = ""
    ) -> SettingsFields {
        SettingsFields(
            pollSeconds: poll,
            workshopModel: workshopModel, workshopEffort: workshopEffort,
            sliceModel: sliceModel, sliceEffort: sliceEffort
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
    }

    func testNoChangesProducesNoWrites() {
        let original = fields(poll: "30")
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

    func testWorkingDirKeyFormat() {
        XCTAssertEqual(SettingsModel.workingDirKey(projectID: "abc-123"), "project.abc-123.working_dir")
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
    /// in nat's config for the TUI — or a project's working directory (the
    /// project settings sheet's) is no field here, and moves nothing.
    func testApplyingIgnoresAKeyTheFormDoesNotHold() {
        let original = fields(poll: "30")

        let result = SettingsModel.applying(
            [
                ConfigChange(key: "agent_split_percent", value: "70"),
                ConfigChange(key: "project.p1.working_dir", value: "/new")
            ],
            to: original
        )

        XCTAssertEqual(result, original)
    }

    func testApplyingWithNoChangesReturnsFieldsUnchanged() {
        let original = fields(poll: "30")
        let result = SettingsModel.applying([], to: original)
        XCTAssertEqual(result, original)
    }
}
