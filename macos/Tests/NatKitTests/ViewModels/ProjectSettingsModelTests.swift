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

    private func model(workingDir: String = "/repo", color: ProjectColor? = .teal, calls: Calls) -> ProjectSettingsModel {
        ProjectSettingsModel(
            projectID: "p1",
            fields: ProjectSettingsFields(workingDir: workingDir, color: color),
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

    // MARK: - Colour

    func testTheColourIsReadFromTheProjectsEntry() {
        XCTAssertEqual(ProjectSettingsFields(projectID: Fixtures.projectID, config: Fixtures.config).color, .teal)
        XCTAssertNil(ProjectSettingsFields(projectID: "elsewhere", config: Fixtures.config).color)
        XCTAssertNil(ProjectSettingsFields(projectID: "p1", config: nil).color)
    }

    func testAnUnchangedColourWritesNothing() async {
        let calls = Calls()
        let model = model(calls: calls)
        model.edited.color = .teal

        let closed = await model.save()
        XCTAssertTrue(closed)
        XCTAssertTrue(calls.writes.isEmpty)
    }

    func testAPickedColourWritesExactlyOneConfigSet() async {
        let calls = Calls()
        let model = model(calls: calls)
        model.edited.color = .purple

        let closed = await model.save()
        XCTAssertTrue(closed)
        XCTAssertEqual(calls.writes, [ConfigChange(key: "project.p1.color", value: "purple")])
        XCTAssertEqual(model.colorKey, "project.p1.color")
        XCTAssertEqual(model.original.color, .purple)
        XCTAssertTrue(model.changes.isEmpty)
        XCTAssertEqual(calls.reloads, 1, "config is re-read, which repaints every badge")
    }

    /// A project with no colour yet writes none until a swatch is picked.
    func testNoColourWritesNothingUntilOneIsPicked() async {
        let calls = Calls()
        let model = model(color: nil, calls: calls)
        model.edited.workingDir = "/elsewhere"

        let closed = await model.save()
        XCTAssertTrue(closed)
        XCTAssertEqual(calls.writes.map(\.key), ["project.p1.working_dir"])
    }

    func testARefusedColourKeepsTheBaseline() async {
        let calls = Calls()
        calls.refusal = "config-set: project.p1.color wants one of …"
        let model = model(calls: calls)
        model.edited.color = .red

        let closed = await model.save()
        XCTAssertFalse(closed)
        XCTAssertEqual(model.errors, [model.colorKey: "config-set: project.p1.color wants one of …"])
        XCTAssertEqual(model.original.color, .teal)
        XCTAssertEqual(model.edited.color, .red)
        XCTAssertEqual(calls.reloads, 0)
    }

    func testBothFieldsChangedAreTwoWritesInFieldOrder() {
        let original = ProjectSettingsFields(workingDir: "/a", color: .red)
        let edited = ProjectSettingsFields(workingDir: "/b", color: .blue)
        let changes = ProjectSettingsModel.changes(projectID: "p1", from: original, to: edited)
        XCTAssertEqual(changes, [
            ConfigChange(key: "project.p1.working_dir", value: "/b"),
            ConfigChange(key: "project.p1.color", value: "blue"),
        ])
        XCTAssertEqual(ProjectSettingsModel.applying(changes, projectID: "p1", to: original), edited)
    }

    /// The sheet has a Colour row only for a project that takes a colour.
    func testOnlyAProjectThatTakesAColourHasTheRow() {
        var projects = Fixtures.config.projects
        projects["s"] = ProjectConfig(name: "Scratch", workingDir: "/")
        projects["w"] = ProjectConfig(name: "Work", workingDir: "", backend: .source, source: "demo")
        let config = NatProjectConfig(projects: projects, scratchProject: "s")
        func sheet(_ id: String, _ config: NatProjectConfig?) -> ProjectSettingsModel {
            ProjectSettingsModel(projectID: id, config: config, client: FixtureNatClient(), reload: {})
        }
        XCTAssertTrue(sheet(Fixtures.projectID, config).takesColor)
        XCTAssertFalse(sheet("s", config).takesColor)
        XCTAssertFalse(sheet("w", config).takesColor)
        XCTAssertFalse(sheet("p1", nil).takesColor)
    }

    // MARK: - Name

    private func model(fields: ProjectSettingsFields, calls: Calls) -> ProjectSettingsModel {
        ProjectSettingsModel(
            projectID: "p1", fields: fields,
            write: { change in
                if let refusal = calls.refusal { throw NatError.commandFailed(refusal) }
                calls.writes.append(change)
            },
            reload: { calls.reloads += 1 })
    }

    func testTheNameIsReadFromTheProjectsEntry() {
        XCTAssertEqual(ProjectSettingsFields(projectID: Fixtures.projectID, config: Fixtures.config).name, "notion-agent-tracker")
        XCTAssertEqual(ProjectSettingsFields(projectID: "p1", config: nil).name, "")
    }

    func testARenameWritesTheTrimmedName() async {
        let calls = Calls()
        let model = model(fields: ProjectSettingsFields(name: "nat", workingDir: "/repo"), calls: calls)
        model.edited.name = "  gnat "

        let closed = await model.save()

        XCTAssertTrue(closed)
        XCTAssertEqual(calls.writes, [ConfigChange(key: "project.p1.name", value: "gnat")])
        XCTAssertEqual(model.nameKey, "project.p1.name")
        XCTAssertEqual(model.original.name, "gnat")
        XCTAssertTrue(model.changes.isEmpty)
        XCTAssertEqual(calls.reloads, 1, "config is re-read, which renames the tab, row and breadcrumb")
    }

    func testANameChangedOnlyBySpacesWritesNothing() {
        let calls = Calls()
        let model = model(fields: ProjectSettingsFields(name: "nat", workingDir: "/repo"), calls: calls)
        model.edited.name = " nat "
        XCTAssertTrue(model.changes.isEmpty)
    }

    /// nat refuses an empty name; the sheet sends it and keeps the refusal.
    func testARefusedNameKeepsItsEditAndMessage() async {
        let calls = Calls()
        calls.refusal = "config-set: project.p1.name wants a name, given none"
        let model = model(fields: ProjectSettingsFields(name: "nat", workingDir: "/repo"), calls: calls)
        model.edited.name = ""

        let closed = await model.save()

        XCTAssertFalse(closed)
        XCTAssertEqual(model.errors, [model.nameKey: "config-set: project.p1.name wants a name, given none"])
        XCTAssertEqual(model.original.name, "nat")
        XCTAssertEqual(model.edited.name, "")
        XCTAssertEqual(calls.reloads, 0)
    }

    // MARK: - Run commands

    private let scopedRuns = [
        RunCommand(label: "Run", command: "go run .", scope: .global),
        RunCommand(label: "Test", command: "go test ./...", scope: .both),
        RunCommand(label: "Run", command: "make dev /tmp", scope: .slice),
    ]

    func testTheRunsAreReadFromTheProjectsEntry() {
        let config = NatProjectConfig(projects: ["p1": ProjectConfig(name: "n", workingDir: "/", runs: scopedRuns)])
        XCTAssertEqual(ProjectSettingsFields(projectID: "p1", config: config).runs, scopedRuns)
        XCTAssertEqual(ProjectSettingsFields(projectID: "p1", config: nil).runs, [])
    }

    /// The whole list in one `config-set`, as nat writes it — scope left off
    /// for both — and the value read back is the list it was made from.
    func testRunsRoundTripScopedAndScopelessRunsUnchanged() {
        let value = ProjectSettingsModel.runsValue(scopedRuns)
        XCTAssertEqual(value, #"[{"command":"go run .","label":"Run","scope":"global"},"#
            + #"{"command":"go test ./...","label":"Test"},"#
            + #"{"command":"make dev /tmp","label":"Run","scope":"slice"}]"#)
        XCTAssertEqual(ProjectSettingsModel.runs(fromValue: value), scopedRuns)
        XCTAssertEqual(ProjectSettingsModel.runsValue([]), "")
        XCTAssertEqual(ProjectSettingsModel.runs(fromValue: ""), [])
    }

    func testUnchangedRunsWriteNothing() {
        let model = model(fields: ProjectSettingsFields(workingDir: "/", runs: scopedRuns), calls: Calls())
        model.edited.runs = scopedRuns
        XCTAssertTrue(model.changes.isEmpty)
    }

    func testAnEditedRunWritesTheWholeListOnce() async {
        let calls = Calls()
        let model = model(fields: ProjectSettingsFields(workingDir: "/", runs: scopedRuns), calls: calls)
        model.edited.runs[1].command = "go test -race ./..."
        model.addRun()
        model.edited.runs[3].label = "Lint"
        model.edited.runs[3].command = "golangci-lint run"

        let closed = await model.save()

        XCTAssertTrue(closed)
        XCTAssertEqual(calls.writes.map(\.key), ["project.p1.runs"])
        XCTAssertEqual(model.runsKey, "project.p1.runs")
        let written = ProjectSettingsModel.runs(fromValue: calls.writes[0].value)
        XCTAssertEqual(written.map(\.command), ["go run .", "go test -race ./...", "make dev /tmp", "golangci-lint run"])
        XCTAssertEqual(written[3].scope, .both, "a new run is offered in both places")
        XCTAssertEqual(model.original.runs, written)
        XCTAssertTrue(model.changes.isEmpty)
    }

    func testReorderChangesTheOrderWritten() async {
        let calls = Calls()
        let model = model(fields: ProjectSettingsFields(workingDir: "/", runs: scopedRuns), calls: calls)
        model.moveRuns(fromOffsets: IndexSet(integer: 2), toOffset: 0)

        _ = await model.save()

        XCTAssertEqual(ProjectSettingsModel.runs(fromValue: calls.writes[0].value).map(\.scope), [.slice, .global, .both])
    }

    func testARowDroppedOntoAnotherTakesItsPlace() {
        let model = model(fields: ProjectSettingsFields(workingDir: "/", runs: scopedRuns), calls: Calls())
        model.moveRun(0, onto: 2)
        XCTAssertEqual(model.edited.runs.map(\.command), ["go test ./...", "make dev /tmp", "go run ."])
        model.moveRun(2, onto: 0)
        XCTAssertEqual(model.edited.runs, scopedRuns)
        model.moveRun(1, onto: 1)
        model.moveRun(0, onto: 3)
        model.moveRun(7, onto: 0)
        XCTAssertEqual(model.edited.runs, scopedRuns, "onto itself or past the list moves nothing")
    }

    func testRemovingEveryRunUnsetsThem() async {
        let calls = Calls()
        let model = model(fields: ProjectSettingsFields(workingDir: "/", runs: [scopedRuns[0]]), calls: calls)
        model.removeRun(at: 5)
        XCTAssertEqual(model.edited.runs.count, 1, "an index past the list removes nothing")
        model.removeRun(at: 0)

        _ = await model.save()

        XCTAssertEqual(calls.writes, [ConfigChange(key: "project.p1.runs", value: "")])
        XCTAssertEqual(model.original.runs, [])
    }

    /// nat's `ValidRuns` refusal stays under the section, the rows as typed.
    func testARefusedRunsListKeepsTheRowsAsTyped() async {
        let calls = Calls()
        calls.refusal = "config-set: run 2: a label offered twice"
        let model = model(fields: ProjectSettingsFields(workingDir: "/", runs: scopedRuns), calls: calls)
        model.edited.runs[1].label = "Run"
        let typed = model.edited.runs

        let closed = await model.save()

        XCTAssertFalse(closed)
        XCTAssertEqual(model.errors, [model.runsKey: "config-set: run 2: a label offered twice"])
        XCTAssertEqual(model.edited.runs, typed)
        XCTAssertEqual(model.original.runs, scopedRuns)
    }

    /// `applying` moves only the field of each key that landed.
    func testApplyingMovesOnlyItsOwnKey() {
        let fields = ProjectSettingsFields(name: "nat", workingDir: "/repo", color: .teal, runs: scopedRuns)
        let renamed = ProjectSettingsModel.applying(
            [ConfigChange(key: "project.p1.name", value: "gnat")], projectID: "p1", to: fields)
        XCTAssertEqual(renamed, ProjectSettingsFields(name: "gnat", workingDir: "/repo", color: .teal, runs: scopedRuns))
        let rerun = ProjectSettingsModel.applying(
            [ConfigChange(key: "project.p1.runs", value: "")], projectID: "p1", to: fields)
        XCTAssertEqual(rerun, ProjectSettingsFields(name: "nat", workingDir: "/repo", color: .teal, runs: []))
        let elsewhere = ProjectSettingsModel.applying(
            [ConfigChange(key: "project.p2.name", value: "x"), ConfigChange(key: "project.p2.runs", value: "")],
            projectID: "p1", to: fields)
        XCTAssertEqual(elsewhere, fields)
    }

    /// One refusal among several writes: the others land, the refused key
    /// keeps its edit.
    func testARefusalAmongSeveralLandsTheRest() async {
        final class Refusing: @unchecked Sendable { var writes: [ConfigChange] = [] }
        let calls = Refusing()
        let model = ProjectSettingsModel(
            projectID: "p1", fields: ProjectSettingsFields(name: "nat", workingDir: "/repo", runs: scopedRuns),
            write: { change in
                if change.key == "project.p1.runs" { throw NatError.commandFailed("bad runs") }
                calls.writes.append(change)
            },
            reload: {})
        model.edited.name = "gnat"
        model.edited.runs = []

        _ = await model.save()

        XCTAssertEqual(calls.writes.map(\.key), ["project.p1.name"])
        XCTAssertEqual(model.original.name, "gnat")
        XCTAssertEqual(model.original.runs, scopedRuns)
        XCTAssertEqual(model.changes.map(\.key), ["project.p1.runs"])
    }

    // MARK: - Plan

    func testThePlanLocationFollowsTheBackend() {
        var projects = Fixtures.config.projects
        projects["l"] = ProjectConfig(name: "Here", workingDir: "/", backend: .local)
        projects["w"] = ProjectConfig(name: "", workingDir: "", backend: .source, source: "shortcut")
        let config = NatProjectConfig(projects: projects)
        func sheet(_ id: String) -> ProjectSettingsModel {
            ProjectSettingsModel(projectID: id, config: config, client: FixtureNatClient(), reload: {})
        }
        XCTAssertEqual(sheet(Fixtures.projectID).plan, .notion(page: NotionPageURL.forPage(Fixtures.projectID)))
        XCTAssertFalse(sheet(Fixtures.projectID).isSource)
        XCTAssertEqual(sheet("l").plan, .local(file: nil))
        XCTAssertEqual(sheet("w").plan, .source(plugin: "shortcut"))
        XCTAssertTrue(sheet("w").isSource)
        XCTAssertEqual(ProjectPlanLocation(projectID: "x", entry: nil), .notion(page: nil))
    }

    func testALocalPlansFileIsReadOnce() async {
        final class Reads: @unchecked Sendable { var count = 0 }
        let reads = Reads()
        let model = ProjectSettingsModel(
            projectID: "p1", fields: ProjectSettingsFields(workingDir: "/"), plan: .local(file: nil),
            write: { _ in }, reload: {},
            readPlanFile: { reads.count += 1; return "/plans/p1.db" })

        await model.loadPlanFile()
        await model.loadPlanFile()

        XCTAssertEqual(model.plan, .local(file: "/plans/p1.db"))
        XCTAssertEqual(reads.count, 1)
    }

    func testAFailedPlanFileReadLeavesItUnknown() async {
        let model = ProjectSettingsModel(
            projectID: "p1", fields: ProjectSettingsFields(workingDir: "/"), plan: .local(file: nil),
            write: { _ in }, reload: {},
            readPlanFile: { throw NatError.missingOutput })
        await model.loadPlanFile()
        XCTAssertEqual(model.plan, .local(file: nil))
    }

    func testANotionPlanReadsNoFile() async {
        let model = ProjectSettingsModel(
            projectID: "p1", fields: ProjectSettingsFields(workingDir: "/"), plan: .notion(page: nil),
            write: { _ in }, reload: {},
            readPlanFile: { XCTFail("no file to read"); return nil })
        await model.loadPlanFile()
        XCTAssertEqual(model.plan, .notion(page: nil))
    }

    /// Over the fixture client, a local plan's file is `nat paths`' answer.
    func testOverAClientALocalPlansFileIsNatsPath() async {
        let config = NatProjectConfig(projects: ["l": ProjectConfig(name: "Here", workingDir: "/", backend: .local)])
        let model = ProjectSettingsModel(projectID: "l", config: config, client: FixtureNatClient(), reload: {})
        await model.loadPlanFile()
        XCTAssertEqual(model.plan, .local(file: Fixtures.planFile(projectID: "l")))
    }
}
