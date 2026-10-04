import XCTest
import NatFixtures
@testable import NatKit

private final class RunStubRunner: CommandRunning, @unchecked Sendable {
    var stdout = ""
    private(set) var lastArguments: [String] = []

    func run(
        executable: String, arguments: [String], workingDirectory: String?, standardInput: Data?
    ) async throws -> (stdout: Data, stderr: Data, exitCode: Int32) {
        lastArguments = arguments
        return (Data(stdout.utf8), Data(), 0)
    }
}

final class RunCommandModelTests: XCTestCase {
    func testDecodesScopeWordsAndReadsAnAbsentOneAsBoth() throws {
        let json = #"[{"label":"Run","command":"make run","scope":"slice"},{"label":"Serve","command":"s","scope":"global"},{"label":"Play","command":"p"}]"#
        let runs = try JSONDecoder().decode([RunCommand].self, from: Data(json.utf8))
        XCTAssertEqual(runs.map(\.scope), [.slice, .global, .both])
        XCTAssertEqual(runs.globalRuns.map(\.label), ["Serve", "Play"])
        XCTAssertEqual(runs.sliceRuns.map(\.label), ["Run", "Play"])
    }

    func testConfigValueIsWhatConfigSetTakes() {
        XCTAssertEqual([RunCommand]().configValue, "")
        let value = [RunCommand(label: "Run", command: "make run", scope: .slice), RunCommand(label: "Play", command: "./p")]
            .configValue
        XCTAssertEqual(value, #"[{"command":"make run","label":"Run","scope":"slice"},{"command":"./p","label":"Play"}]"#)
    }

    func testConfigDocAndProjectConfigDecodeRunsAndTolerateTheirAbsence() throws {
        let doc = #"{"name":"nat","working_dir":"/w","runs":[{"label":"Run","command":"make run"}]}"#
        XCTAssertEqual(try JSONDecoder().decode(ConfigDocProject.self, from: Data(doc.utf8)).runs,
                       [RunCommand(label: "Run", command: "make run")])
        XCTAssertEqual(try JSONDecoder().decode(ConfigDocProject.self, from: Data(#"{"name":"n","working_dir":"/w"}"#.utf8)).runs, [])
        let entry = #"{"name":"nat","working_dir":"/w","runs":[{"label":"Run","command":"x","scope":"global"}]}"#
        let project = try JSONDecoder().decode(ProjectConfig.self, from: Data(entry.utf8))
        XCTAssertEqual(project.runs, [RunCommand(label: "Run", command: "x", scope: .global)])
        let plain = try JSONEncoder().encode(ProjectConfig(name: "n", workingDir: "/w"))
        XCTAssertFalse(String(decoding: plain, as: UTF8.self).contains("runs"))
    }

    func testTheRunTabSitsBesideTerminal() {
        XCTAssertEqual(TitlebarTab.withRun([.terminal, .changes]).map(\.id), ["pane.terminal", "pane.run", "pane.changes"])
        XCTAssertEqual(TitlebarTab.withRun([.changes]).map(\.id), ["pane.run", "pane.changes"])
    }

    func testNatClientRunPassesWhatItIsGiven() async throws {
        let runner = RunStubRunner()
        runner.stdout = #"{"session":"nat-run-x-run","label":"Run","command":"make run","dir":"/w"}"#
        let client = NatClient(commandRunner: runner)
        let result = try await client.run(projectID: "p", sliceRef: "s", label: "Run")
        XCTAssertEqual(result, RunResult(session: "nat-run-x-run", label: "Run", command: "make run", dir: "/w"))
        XCTAssertEqual(runner.lastArguments.suffix(8), ["run", "--project", "p", "--json", "--slice", "s", "--label", "Run"])
        _ = try await client.run(projectID: "p", sliceRef: nil, label: nil)
        XCTAssertEqual(runner.lastArguments.suffix(4), ["run", "--project", "p", "--json"])
    }
}

final class RunSettingsTests: XCTestCase {
    private func fields(_ runs: [RunCommand]) -> SettingsFields {
        SettingsFields(
            pollSeconds: "", workshopModel: "", workshopEffort: "", sliceModel: "", sliceEffort: "",
            projectWorkingDirs: ["p": "/w"], projectRuns: ["p": runs])
    }

    func testRunsAreWrittenWholeOnceEveryRowIsFilledIn() {
        let run = RunCommand(label: "Run", command: "make run", scope: .slice)
        let changes = SettingsModel.changes(from: fields([]), to: fields([run]))
        XCTAssertEqual(changes, [ConfigChange(key: "project.p.runs", value: [run].configValue)])

        let draft = SettingsModel.changes(from: fields([run]), to: fields([run, RunCommand(label: "", command: "x")]))
        XCTAssertEqual(draft, [], "a row still being typed is written with nothing")

        XCTAssertEqual(SettingsModel.changes(from: fields([run]), to: fields([])),
                       [ConfigChange(key: "project.p.runs", value: "")])
    }

    func testApplyingAWrittenListMovesTheBaseline() {
        let run = RunCommand(label: "Play", command: "./p")
        let moved = SettingsModel.applying([ConfigChange(key: "project.p.runs", value: [run].configValue)], to: fields([]))
        XCTAssertEqual(moved.projectRuns["p"], [run])
        let cleared = SettingsModel.applying([ConfigChange(key: "project.p.runs", value: "")], to: fields([run]))
        XCTAssertEqual(cleared.projectRuns["p"], [])
    }

    func testFromConfigDocReadsEachProjectsRuns() {
        XCTAssertEqual(SettingsFields(from: Fixtures.configDocWithRuns).projectRuns[Fixtures.projectID]?.count, 2)
    }
}

@MainActor
final class AppModelRunTests: XCTestCase {
    func testAGlobalRunIsHeldForEverySliceOfItsProjectUntilItEnds() async {
        let model = await Fixtures.startedAppModel(config: Fixtures.runsConfig)
        model.runSessionExists = { _ in true }
        XCTAssertEqual(model.globalRuns(ofProject: Fixtures.projectID).map(\.label), ["Serve", "Play"])
        XCTAssertEqual(model.sliceRuns(ofProject: Fixtures.projectID).map(\.label), ["Play", "Test"])
        XCTAssertEqual(model.globalRuns(ofProject: Fixtures.secondProjectID), [])

        await model.startRun(projectID: Fixtures.projectID)
        let run = model.run(forSlice: Fixtures.mergeBoxSliceID, inProject: Fixtures.projectID)
        XCTAssertEqual(run?.label, "Serve")
        XCTAssertNil(run?.sliceID)
        XCTAssertEqual(model.runShowRequest, 1)
        XCTAssertFalse(model.isStartingRun(projectID: Fixtures.projectID, sliceID: nil))

        await model.startRun(projectID: Fixtures.projectID, sliceID: Fixtures.mergeBoxSliceID, label: "Test")
        XCTAssertEqual(model.run(forSlice: Fixtures.mergeBoxSliceID, inProject: Fixtures.projectID)?.label, "Test",
                       "a slice's own run wins over its project's")

        model.runEnded(session: run!.session)
        XCTAssertNil(model.runs[AppModel.runKey(projectID: Fixtures.projectID, sliceID: nil)])
    }

    func testARunWhoseSessionEndsIsLetGo() async throws {
        let model = await Fixtures.startedAppModel(config: Fixtures.runsConfig)
        model.runWatchInterval = 1_000_000
        model.runSessionExists = { _ in false }
        await model.startRun(projectID: Fixtures.projectID)
        for _ in 0..<200 where !model.runs.isEmpty { try await Task.sleep(nanoseconds: 5_000_000) }
        XCTAssertTrue(model.runs.isEmpty)
    }

    func testARefusalIsKeptInNatsWordsUntilDismissed() async {
        let model = await Fixtures.startedAppModel(
            client: FixtureNatClient(behaviour: .refusing("run: the project has no global runs")), config: Fixtures.runsConfig)
        await model.startRun(projectID: Fixtures.projectID)
        XCTAssertEqual(model.runError, "run: the project has no global runs")
        XCTAssertTrue(model.runs.isEmpty)
        model.dismissRunError()
        XCTAssertNil(model.runError)
    }
}
