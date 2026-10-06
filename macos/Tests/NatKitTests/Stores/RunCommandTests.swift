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

@MainActor
final class AppModelRunTests: XCTestCase {
    func testASourceProjectsRunProjectTakesNoBadge() async {
        let source = Fixtures.sourceConfig
        var projects = source.projects
        let work = projects[Fixtures.sourceProjectID]!
        projects[Fixtures.sourceProjectID] = ProjectConfig(
            name: work.name, workingDir: work.workingDir, backend: .source, source: "demo",
            runs: [RunCommand(label: "Run", command: "make run")])
        let config = NatProjectConfig(
            projects: projects, agentSplitPercent: source.agentSplitPercent, pollSeconds: source.pollSeconds,
            assigneeUserName: source.assigneeUserName)
        let model = await Fixtures.startedAppModel(config: config)
        let project = model.runProjects.first { $0.id == Fixtures.sourceProjectID }
        XCTAssertEqual(project?.tag, "", "a source project takes no badge")
        XCTAssertNil(project?.color)
    }

    func testARunIsHeldAndItsButtonBusyUntilItEnds() async {
        let model = await Fixtures.startedAppModel(config: Fixtures.runsConfig)
        model.runSessionExists = { _ in true }
        XCTAssertEqual(model.globalRuns(ofProject: Fixtures.projectID).map(\.command), ["./scripts/play.sh --windowed", "go run ."])
        XCTAssertEqual(model.sliceRuns(ofProject: Fixtures.projectID).map(\.command), ["./scripts/play.sh --windowed", "go run . --sandbox"])
        XCTAssertEqual(model.runProjects.map(\.name), ["gnat", "notion-agent-tracker"])
        XCTAssertEqual(model.runProjects.last?.runs.map(\.label), ["Play", "Board"])
        XCTAssertEqual(model.runProjects.map(\.tag), ["GNA", "NOT"], "each project's badge word, sidebarTags'")
        XCTAssertEqual(
            model.runProjects.map(\.color),
            [Fixtures.runsConfig.projects[Fixtures.secondProjectID]?.color,
             Fixtures.runsConfig.projects[Fixtures.projectID]?.color])

        XCTAssertFalse(model.anyRunBusy)
        await model.startRun(projectID: Fixtures.projectID)
        let run = model.runs[AppModel.runKey(projectID: Fixtures.projectID, sliceID: nil)]
        XCTAssertEqual(run?.label, "Play")
        XCTAssertNil(run?.sliceID)
        XCTAssertFalse(model.isStartingRun(projectID: Fixtures.projectID, sliceID: nil))
        XCTAssertTrue(model.isRunBusy(projectID: Fixtures.projectID, sliceID: nil), "busy while its session lives")
        XCTAssertFalse(model.isRunBusy(projectID: Fixtures.projectID, sliceID: Fixtures.mergeBoxSliceID),
                       "a project's run is not a slice's")
        XCTAssertTrue(model.anyRunBusy)

        await model.startRun(projectID: Fixtures.projectID, sliceID: Fixtures.mergeBoxSliceID, label: "Board")
        XCTAssertEqual(model.runs[Fixtures.mergeBoxSliceID]?.label, "Board")
        XCTAssertTrue(model.isRunBusy(projectID: Fixtures.projectID, sliceID: Fixtures.mergeBoxSliceID))

        model.runEnded(session: run!.session)
        XCTAssertNil(model.runs[AppModel.runKey(projectID: Fixtures.projectID, sliceID: nil)])
        XCTAssertFalse(model.isRunBusy(projectID: Fixtures.projectID, sliceID: nil))
        XCTAssertTrue(model.anyRunBusy, "the slice's run is still live")
    }

    /// The run asked for is reported running — nat's default by its scope's
    /// first label — and its siblings are not, until its session ends.
    func testTheRunningRunIsToldFromItsSiblings() async {
        let model = await Fixtures.startedAppModel(config: Fixtures.runsConfig)
        model.runSessionExists = { _ in true }
        let p = Fixtures.projectID, s = Fixtures.mergeBoxSliceID

        await model.startRun(projectID: p, label: "Board")
        XCTAssertTrue(model.isRunning(projectID: p, sliceID: nil, label: "Board"))
        XCTAssertFalse(model.isRunning(projectID: p, sliceID: nil, label: "Play"))
        XCTAssertFalse(model.isRunning(projectID: p, sliceID: s, label: "Board"), "a slice's runs are another key")

        await model.startRun(projectID: p, sliceID: s)
        XCTAssertTrue(model.isRunning(projectID: p, sliceID: s, label: "Play"), "the default is the scope's first")
        XCTAssertFalse(model.isRunning(projectID: p, sliceID: s, label: "Board"))

        model.runEnded(session: model.runs[s]!.session)
        XCTAssertFalse(model.isRunning(projectID: p, sliceID: s, label: "Play"))
        XCTAssertTrue(model.isRunning(projectID: p, sliceID: nil, label: "Board"))
    }

    /// While `nat run` is in flight the label asked for is already running,
    /// its siblings not; a run that never lands clears it.
    func testARunStartingIsRunningUntilItFails() async throws {
        let model = Fixtures.appModel(client: FixtureNatClient(behaviour: .hanging), config: Fixtures.runsConfig)
        let p = Fixtures.projectID
        let start = Task { await model.startRun(projectID: p, label: "Board") }
        for _ in 0..<200 where !model.isStartingRun(projectID: p, sliceID: nil) {
            try await Task.sleep(nanoseconds: 5_000_000)
        }
        XCTAssertTrue(model.isRunning(projectID: p, sliceID: nil, label: "Board"))
        XCTAssertFalse(model.isRunning(projectID: p, sliceID: nil, label: "Play"))
        start.cancel()
        await start.value
        XCTAssertFalse(model.isRunning(projectID: p, sliceID: nil, label: "Board"))
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
        XCTAssertFalse(model.isRunning(projectID: Fixtures.projectID, sliceID: nil, label: "Play"), "a refused run is not running")
        model.dismissRunError()
        XCTAssertNil(model.runError)
    }
}

@MainActor
final class RunProjectsTests: XCTestCase {
    func testAProjectWithNoRunsIsLeftOutOfTheTree() async {
        let model = await Fixtures.startedAppModel(config: Fixtures.twoProjectConfig)
        XCTAssertEqual(model.runProjects, [])
    }
}
