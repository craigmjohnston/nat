import XCTest
@testable import NatKit

/// A stub `nat` that answers one canned stdout and records what it was asked
/// and sent over stdin.
private final class StubRunner: CommandRunning, @unchecked Sendable {
    var stdout = "{}"
    var stderr = ""
    var exitCode: Int32 = 0
    private(set) var lastArguments: [String] = []
    private(set) var lastStandardInput: Data?

    init(stdout: String = "{}") {
        self.stdout = stdout
    }

    func run(
        executable: String, arguments: [String], workingDirectory: String?, standardInput: Data?
    ) async throws -> (stdout: Data, stderr: Data, exitCode: Int32) {
        lastArguments = arguments
        lastStandardInput = standardInput
        return (Data(stdout.utf8), Data(stderr.utf8), exitCode)
    }
}

private let infoJSON = #"{"project": {"id": "p", "name": "Work", "conventions": ""}, "milestones": [], "slices": []}"#

/// The task-source commands' argument shapes, as `docs/design/task-sources`
/// spells them.
final class SourceNatClientTests: XCTestCase {
    func testInfoPassesEachExpandedGroup() async throws {
        let runner = StubRunner(stdout: infoJSON)
        _ = try await NatClient(commandRunner: runner).info(projectID: "p", refresh: true, expand: ["done", "seg-x"])
        XCTAssertEqual(runner.lastArguments, [
            "info", "--project", "p", "--json", "--refresh", "--expand", "done", "--expand", "seg-x",
        ])
    }

    func testInfoWithoutExpandIsUnchanged() async throws {
        let runner = StubRunner(stdout: infoJSON)
        _ = try await NatClient(commandRunner: runner).info(projectID: "p", refresh: false)
        XCTAssertEqual(runner.lastArguments, ["info", "--project", "p", "--json"])
    }

    func testContainerShow() async throws {
        let runner = StubRunner(stdout: #"{"container": {"id": "4821", "title": "Card"}, "tasks": []}"#)
        let show = try await NatClient(commandRunner: runner).containerShow(projectID: "p", containerID: "4821")
        XCTAssertEqual(runner.lastArguments, ["container-show", "4821", "--project", "p", "--json"])
        XCTAssertEqual(show.container.title, "Card")
    }

    func testContainerShowPassesThePluginsRefusal() async throws {
        let runner = StubRunner(stdout: "")
        runner.stderr = "source plugin demo: no card 9\nmore"
        runner.exitCode = 1
        do {
            _ = try await NatClient(commandRunner: runner).containerShow(projectID: "p", containerID: "9")
            XCTFail("expected a refusal")
        } catch NatError.commandFailed(let message) {
            XCTAssertEqual(message, "source plugin demo: no card 9")
        }
    }

    func testSourceActionOnTheHeaderWithNoInput() async throws {
        let runner = StubRunner(stdout: "{}")
        let result = try await NatClient(commandRunner: runner).sourceAction(
            projectID: "p", action: "refresh", group: nil, container: nil, input: nil)
        XCTAssertEqual(runner.lastArguments, ["source-action", "--project", "p", "--action", "refresh", "--json"])
        XCTAssertNil(runner.lastStandardInput)
        XCTAssertNil(result.message)
    }

    func testSourceActionOnAGroupWithAChoice() async throws {
        let runner = StubRunner(stdout: #"{"message": "Mine now shows unassigned cards"}"#)
        let result = try await NatClient(commandRunner: runner).sourceAction(
            projectID: "p", action: "segment-owner", group: "seg-mine", container: nil, input: "unassigned")
        XCTAssertEqual(runner.lastArguments, [
            "source-action", "--project", "p", "--action", "segment-owner",
            "--group", "seg-mine", "--input", "-", "--json",
        ])
        XCTAssertEqual(runner.lastStandardInput, Data("unassigned".utf8))
        XCTAssertEqual(result.message, "Mine now shows unassigned cards")
    }

    func testSourceActionOnAContainerSendsMultiLineInputOverStdin() async throws {
        let runner = StubRunner(stdout: "{}")
        _ = try await NatClient(commandRunner: runner).sourceAction(
            projectID: "p", action: "comment", group: "", container: "4821", input: "Line one\nLine two")
        XCTAssertEqual(runner.lastArguments, [
            "source-action", "--project", "p", "--action", "comment",
            "--container", "4821", "--input", "-", "--json",
        ])
        XCTAssertEqual(runner.lastStandardInput, Data("Line one\nLine two".utf8))
    }

    func testSourceListTakesNoProject() async throws {
        let runner = StubRunner(stdout: #"[{"name": "demo", "path": "/p", "error": "broken"}]"#)
        let plugins = try await NatClient(commandRunner: runner).sourceList()
        XCTAssertEqual(runner.lastArguments, ["source-list", "--json"])
        XCTAssertEqual(plugins, [SourcePlugin(name: "demo", path: "/p", error: "broken")])
    }

    func testProjectCreateWithASource() async throws {
        let runner = StubRunner(stdout: #"{"project": {"id": "p9", "name": "Work", "working_dir": "/w", "source": "demo"}}"#)
        let created = try await NatClient(commandRunner: runner).projectCreate(
            name: "Work", repo: "/w", description: "Cards first.", source: "demo")
        XCTAssertEqual(runner.lastArguments, [
            "project-create", "Work", "--json", "--repo", "/w", "--source", "demo", "--description", "-",
        ])
        XCTAssertEqual(runner.lastStandardInput, Data("Cards first.".utf8))
        XCTAssertEqual(created.source, "demo")
        XCTAssertEqual(created.workingDir, "/w")
    }

    func testProjectCreateWithAnEmptySourceIsAnOrdinaryCreate() async throws {
        let runner = StubRunner(stdout: #"{"project": {"id": "p9", "name": "N"}}"#)
        _ = try await NatClient(commandRunner: runner).projectCreate(name: "N", repo: nil, description: nil, source: "")
        XCTAssertEqual(runner.lastArguments, ["project-create", "N", "--json"])
    }
}
