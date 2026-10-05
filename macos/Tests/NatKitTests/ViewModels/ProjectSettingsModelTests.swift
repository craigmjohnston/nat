import XCTest
@testable import NatKit
import NatFixtures

@MainActor
final class ProjectSettingsModelTests: XCTestCase {
    /// Every `config-set` the model made, and whether config was re-read —
    /// nothing reaches nat or the machine's config.
    private final class Calls: @unchecked Sendable {
        var writes: [ConfigChange] = []
        var reloads = 0
        var refusal: String?
    }

    private func model(workingDir: String = "/repo", calls: Calls) -> ProjectSettingsModel {
        ProjectSettingsModel(
            projectID: "p1",
            fields: ProjectSettingsFields(workingDir: workingDir),
            write: { change in
                if let refusal = calls.refusal { throw NatError.commandFailed(refusal) }
                calls.writes.append(change)
            },
            reload: { calls.reloads += 1 })
    }

    func testFieldsReadTheProjectsEntryFromConfig() {
        let fields = ProjectSettingsFields(projectID: Fixtures.projectID, config: Fixtures.config)
        XCTAssertEqual(fields.workingDir, "/Users/craig/Projects/notion-agent-tracker")
    }

    func testFieldsForAProjectConfigDoesNotNameAreEmpty() {
        XCTAssertEqual(ProjectSettingsFields(projectID: "elsewhere", config: Fixtures.config).workingDir, "")
        XCTAssertEqual(ProjectSettingsFields(projectID: "p1", config: nil).workingDir, "")
    }

    func testAnUnchangedDirectoryWritesNothing() async {
        let calls = Calls()
        let model = model(calls: calls)

        let closed = await model.save()

        XCTAssertTrue(closed)
        XCTAssertTrue(calls.writes.isEmpty)
        XCTAssertEqual(calls.reloads, 0)
    }

    func testAChangedDirectoryWritesExactlyOneConfigSetUnderTheProjectsKey() async {
        let calls = Calls()
        let model = model(calls: calls)
        model.edited.workingDir = "/elsewhere"

        let closed = await model.save()

        XCTAssertTrue(closed)
        XCTAssertEqual(calls.writes, [ConfigChange(key: "project.p1.working_dir", value: "/elsewhere")])
        XCTAssertEqual(model.original.workingDir, "/elsewhere")
        XCTAssertTrue(model.changes.isEmpty)
        XCTAssertTrue(model.errors.isEmpty)
        XCTAssertEqual(calls.reloads, 1, "config is re-read so launches and Reveal use the new path")
    }

    func testARefusalIsSurfacedAndConfigLeftAsRead() async {
        let calls = Calls()
        calls.refusal = "working_dir: no such directory"
        let model = model(calls: calls)
        model.edited.workingDir = "/missing"

        let closed = await model.save()

        XCTAssertFalse(closed)
        XCTAssertEqual(model.errors, [model.workingDirKey: "working_dir: no such directory"])
        XCTAssertEqual(model.original.workingDir, "/repo")
        XCTAssertEqual(model.edited.workingDir, "/missing", "the edit is kept to correct")
        XCTAssertEqual(calls.reloads, 0)
    }

    func testARetryAfterARefusalClearsTheError() async {
        let calls = Calls()
        calls.refusal = "refused"
        let model = model(calls: calls)
        model.edited.workingDir = "/next"
        _ = await model.save()

        calls.refusal = nil
        let closed = await model.save()

        XCTAssertTrue(closed)
        XCTAssertTrue(model.errors.isEmpty)
        XCTAssertEqual(calls.writes.map(\.value), ["/next"])
    }

    func testAnErrorThatIsNoRefusalIsSurfacedByItsDescription() async {
        let model = ProjectSettingsModel(
            projectID: "p1", fields: ProjectSettingsFields(workingDir: ""),
            write: { _ in throw NatError.missingOutput },
            reload: {})
        model.edited.workingDir = "/x"

        let closed = await model.save()

        XCTAssertFalse(closed)
        XCTAssertEqual(model.errors[model.workingDirKey], NatError.missingOutput.localizedDescription)
    }

    /// Over the fixture client, as the app builds it: the write is a
    /// `config-set` of the project's key, and a refusing client's message
    /// lands beside it.
    func testOverAClientWritesConfigSet() async {
        let client = FixtureNatClient()
        let model = ProjectSettingsModel(projectID: Fixtures.projectID, config: Fixtures.config, client: client, reload: {})
        model.edited.workingDir = "/elsewhere"

        _ = await model.save()

        XCTAssertEqual(client.writes, ["config-set project.\(Fixtures.projectID).working_dir"])

        let refusing = ProjectSettingsModel(
            projectID: Fixtures.projectID, config: Fixtures.config,
            client: FixtureNatClient(behaviour: .refusing("nope")), reload: {})
        refusing.edited.workingDir = "/elsewhere"
        _ = await refusing.save()
        XCTAssertEqual(refusing.errors[refusing.workingDirKey], "nope")
    }

    func testApplyingIgnoresAKeyTheSheetDoesNotHold() {
        let fields = ProjectSettingsFields(workingDir: "/repo")
        let result = ProjectSettingsModel.applying(
            [ConfigChange(key: "project.other.working_dir", value: "/x"), ConfigChange(key: "poll_seconds", value: "5")],
            projectID: "p1", to: fields)
        XCTAssertEqual(result, fields)
    }
}
