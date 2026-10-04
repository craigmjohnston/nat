import XCTest
import NatFixtures
@testable import NatKit

/// A stub `nat` that answers one canned stdout and records what it was asked.
private final class PluginStubRunner: CommandRunning, @unchecked Sendable {
    let stdout: String
    private(set) var lastArguments: [String] = []
    private(set) var lastStandardInput: Data?

    init(stdout: String) {
        self.stdout = stdout
    }

    func run(
        executable: String, arguments: [String], workingDirectory: String?, standardInput: Data?
    ) async throws -> (stdout: Data, stderr: Data, exitCode: Int32) {
        lastArguments = arguments
        lastStandardInput = standardInput
        return (Data(stdout.utf8), Data(), 0)
    }
}

/// `nat plugin-list --json` exactly as `internal/cli/plugins.go` writes it.
private let listingJSON = #"""
{
  "sources": [
    {"repo": "craigmjohnston/nat", "version": "1.0.2", "error": "", "default": true},
    {"repo": "dead/source", "version": "", "error": "read plugin source dead/source: GET …: 404 Not Found", "default": false}
  ],
  "installed": [
    {"name": "demo", "path": "/c/plugins/demo/nat-source-demo", "kind": "managed", "source": "craigmjohnston/nat", "version": "1.0.1", "update": "1.0.2",
     "setup": [{"id": "token", "label": "API token", "input": "secret", "hint": "Settings ▸ Tokens", "set": true}, {"id": "team", "label": "Team", "input": "text"}], "describe_error": ""},
    {"name": "hand", "path": "/c/plugins/hand/nat-source-hand", "kind": "manual", "source": "", "version": "", "update": "", "setup": [], "describe_error": "hand: broken"},
    {"name": "onpath", "path": "/bin/nat-source-onpath", "kind": "path", "source": "", "version": "", "update": "", "setup": [], "describe_error": ""}
  ],
  "available": [
    {"name": "demo", "title": "Demo", "description": "A demo.", "source": "craigmjohnston/nat", "version": "1.0.2", "installed": true}
  ]
}
"""#

final class PluginModelsTests: XCTestCase {
    func testThePluginListingDecodes() throws {
        let listing = try JSONDecoder().decode(PluginListing.self, from: Data(listingJSON.utf8))
        XCTAssertEqual(listing.sources, [
            PluginSourceStatus(repo: "craigmjohnston/nat", version: "1.0.2", isDefault: true),
            PluginSourceStatus(repo: "dead/source", error: "read plugin source dead/source: GET …: 404 Not Found"),
        ])
        XCTAssertEqual(listing.installed.map(\.kind), [.managed, .manual, .path])
        XCTAssertEqual(listing.installed.map(\.versionLabel), ["1.0.1", "manual", "on PATH"])
        XCTAssertEqual(listing.installed.map(\.hasUpdate), [true, false, false])
        XCTAssertEqual(listing.installed.map(\.isUninstallable), [true, true, false])
        XCTAssertEqual(listing.available, [AvailablePlugin(
            name: "demo", title: "Demo", description: "A demo.", source: "craigmjohnston/nat", version: "1.0.2", installed: true)])
        XCTAssertEqual(listing.available[0].id, "craigmjohnston/nat/demo")
        XCTAssertEqual(listing.sources[0].id, "craigmjohnston/nat")
        XCTAssertEqual(listing.installed[0].id, "demo")
        // A setup field's hint may be left out; the rest are always written.
        XCTAssertEqual(listing.installed[0].setup, [
            PluginSetupField(id: "token", label: "API token", input: "secret", hint: "Settings ▸ Tokens", set: true),
            PluginSetupField(id: "team", label: "Team", input: "text"),
        ])
        XCTAssertEqual(listing.installed[0].setup.map(\.set), [true, nil], "set is optional, and absent is nil")
        XCTAssertEqual(listing.installed[0].setup.map(\.isSecret), [true, false])
        XCTAssertEqual(listing.installed.map(\.describeError), ["", "hand: broken", ""])
    }

    func testLabelsForWhatNatCannotSay() {
        XCTAssertEqual(InstalledPlugin(name: "x", path: "/p", kind: .managed).versionLabel, "installed by nat")
        XCTAssertEqual(AvailablePlugin(name: "x", title: "", description: "", source: "a/b", version: "1", installed: false).displayTitle, "x")
    }

    func testPluginSourceRepoShape() {
        for ok in ["craigmjohnston/nat", "a/b", "Some-One/my.repo_2"] {
            XCTAssertTrue(PluginSourceRepo.isValid(ok), ok)
        }
        for bad in ["", "nat", "a/b/c", "-a/b", "a-/b", "a/..", "a/.", "a b/c", "https://github.com/a/b", "a/"] {
            XCTAssertFalse(PluginSourceRepo.isValid(bad), bad)
        }
    }

    // MARK: - NatClient

    func testPluginListTakesNoProject() async throws {
        let runner = PluginStubRunner(stdout: listingJSON)
        let listing = try await NatClient(commandRunner: runner).pluginList()
        XCTAssertEqual(runner.lastArguments, ["plugin-list", "--json"])
        XCTAssertEqual(listing.installed.count, 3)
    }

    func testPluginInstallArguments() async throws {
        let runner = PluginStubRunner(stdout: #"{"name": "demo", "path": "/p", "source": "a/b", "version": "2", "sha256": "ab", "installed_at": "2026-10-03T12:00:00Z"}"#)
        let client = NatClient(commandRunner: runner)
        let installed = try await client.pluginInstall(name: "demo", source: "a/b", version: "2")
        XCTAssertEqual(runner.lastArguments, ["plugin-install", "demo", "--source", "a/b", "--version", "2", "--json"])
        XCTAssertEqual(installed, PluginInstalled(name: "demo", path: "/p", source: "a/b", version: "2", sha256: "ab", installedAt: "2026-10-03T12:00:00Z"))
        _ = try await client.pluginInstall(name: "demo", source: nil, version: "")
        XCTAssertEqual(runner.lastArguments, ["plugin-install", "demo", "--json"])
    }

    func testPluginUninstallAndSourceArguments() async throws {
        // An older nat sends no projects_deleted: none.
        let gone = PluginStubRunner(stdout: #"{"name": "demo", "path": "/c/plugins/demo"}"#)
        let uninstalled = try await NatClient(commandRunner: gone).pluginUninstall(name: "demo", deleteProjects: false)
        XCTAssertEqual(gone.lastArguments, ["plugin-uninstall", "demo", "--json"])
        XCTAssertEqual(uninstalled, PluginUninstalled(name: "demo", path: "/c/plugins/demo"))

        let deleting = PluginStubRunner(stdout: #"{"name": "demo", "path": "/c/plugins/demo", "projects_deleted": [{"id": "p1", "name": "Work"}]}"#)
        let deleted = try await NatClient(commandRunner: deleting).pluginUninstall(name: "demo", deleteProjects: true)
        XCTAssertEqual(deleting.lastArguments, ["plugin-uninstall", "demo", "--delete-projects", "--json"])
        XCTAssertEqual(deleted.projectsDeleted, [PluginUninstalled.DeletedProject(id: "p1", name: "Work")])

        let sources = PluginStubRunner(stdout: #"{"sources": ["craigmjohnston/nat", "a/b"]}"#)
        let client = NatClient(commandRunner: sources)
        let added = try await client.pluginSourceAdd(repo: "a/b")
        XCTAssertEqual(sources.lastArguments, ["plugin-source-add", "a/b", "--json"])
        XCTAssertEqual(added, PluginSourceList(sources: ["craigmjohnston/nat", "a/b"]))
        _ = try await client.pluginSourceRemove(repo: "a/b")
        XCTAssertEqual(sources.lastArguments, ["plugin-source-remove", "a/b", "--json"])
    }

    /// The value — a token — goes on stdin alone: the arguments name the
    /// plugin and the field and nothing else.
    func testSourceSetupSendsTheValueOnStdinOnly() async throws {
        let runner = PluginStubRunner(stdout: #"{"message": "Logged in to scratch as Craig"}"#)
        let result = try await NatClient(commandRunner: runner).sourceSetup(plugin: "shortcut", id: "token", value: "s3cret-tok")
        XCTAssertEqual(result, PluginSetupResult(message: "Logged in to scratch as Craig"))
        XCTAssertEqual(runner.lastArguments, ["source-setup", "shortcut", "--id", "token", "--json"])
        XCTAssertFalse(runner.lastArguments.contains { $0.contains("s3cret") })
        XCTAssertEqual(runner.lastStandardInput, Data("s3cret-tok".utf8))
    }

    func testAClientWithNoPluginsRefusesThePluginCommands() async {
        let client = MockActivityClient(response: .agents([]))
        let calls: [(String, () async throws -> Void)] = [
            ("plugin-list", { _ = try await client.pluginList() }),
            ("plugin-install", { _ = try await client.pluginInstall(name: "x", source: nil, version: nil) }),
            ("plugin-uninstall", { _ = try await client.pluginUninstall(name: "x", deleteProjects: false) }),
            ("plugin-source-add", { _ = try await client.pluginSourceAdd(repo: "a/b") }),
            ("plugin-source-remove", { _ = try await client.pluginSourceRemove(repo: "a/b") }),
            ("source-setup", { _ = try await client.sourceSetup(plugin: "x", id: "token", value: "v") }),
        ]
        for (command, call) in calls {
            do {
                try await call()
                XCTFail("\(command): expected a refusal")
            } catch NatError.commandFailed(let message) {
                XCTAssertTrue(message.hasPrefix(command), message)
            } catch { XCTFail("\(error)") }
        }
    }

    // MARK: - Fixture client

    func testTheFixtureClientAnswersAndRecords() async throws {
        let client = FixtureNatClient(plugins: Fixtures.pluginListingEmpty)
        let listing = try await client.pluginList()
        XCTAssertEqual(listing, Fixtures.pluginListingEmpty)
        let installed = try await client.pluginInstall(name: "demo", source: nil, version: nil)
        XCTAssertEqual(installed.source, "craigmjohnston/nat")
        _ = try await client.pluginUninstall(name: "demo", deleteProjects: false)
        let added = try await client.pluginSourceAdd(repo: "a/b")
        XCTAssertEqual(added.sources, ["craigmjohnston/nat", "a/b"])
        let removed = try await client.pluginSourceRemove(repo: "craigmjohnston/nat")
        XCTAssertEqual(removed.sources, [])
        let setUp = try await client.sourceSetup(plugin: "shortcut", id: "token", value: "s3cret")
        XCTAssertEqual(setUp.message, "Logged in to scratch as Craig Scratch")
        XCTAssertEqual(client.writes, [
            "plugin-install demo --source ",
            "plugin-uninstall demo",
            "plugin-source-add a/b",
            "plugin-source-remove craigmjohnston/nat",
            "source-setup shortcut --id token",
        ])
        XCTAssertTrue(Fixtures.pluginListing.installed.contains { $0.hasUpdate })
    }
}

@MainActor
final class PluginsModelTests: XCTestCase {
    func testLoadOnceAndEveryAction() async {
        let client = FixtureNatClient()
        var changed: [PluginsModel.PluginChange] = []
        let model = PluginsModel(client: client, pluginsChanged: { changed.append($0) })

        await model.loadIfNeeded()
        XCTAssertEqual(model.listing, Fixtures.pluginListing)
        XCTAssertNil(model.loadError)
        await model.loadIfNeeded()

        let demo = Fixtures.pluginListing.installed[0]
        let shortcut = Fixtures.pluginListing.available[1]
        await model.install(shortcut)
        await model.update(demo)
        await model.uninstall(demo)
        XCTAssertEqual(changed, [
            PluginsModel.PluginChange(plugin: "shortcut"), PluginsModel.PluginChange(plugin: "demo"),
            PluginsModel.PluginChange(plugin: "demo"),
        ], "each names its plugin; no project used demo, so none was deleted")
        XCTAssertNil(model.pendingUninstall, "nothing to ask with no project using it")

        XCTAssertFalse(model.canAddSource)
        model.newSource = " someone/plugins "
        XCTAssertTrue(model.canAddSource)
        await model.addSource()
        XCTAssertEqual(model.newSource, "")
        await model.addSource()
        await model.removeSource("someone/plugins")
        XCTAssertEqual(changed.count, 3, "a source edit installs nothing")

        XCTAssertEqual(client.writes, [
            "plugin-install shortcut --source craigmjohnston/nat",
            "plugin-install demo --source craigmjohnston/nat",
            "plugin-uninstall demo",
            "plugin-source-add someone/plugins",
            "plugin-source-remove someone/plugins",
        ])
        XCTAssertTrue(model.running.isEmpty)
        XCTAssertNil(model.actionError)
    }

    /// A plugin some project uses is not uninstalled on the click: the
    /// projects are named and the user asked, and only a yes sends
    /// `--delete-projects`. What nat deleted reaches whoever reads plugins.
    func testUninstallingAPluginInUseAsksFirst() async {
        let config = ConfigDoc(
            agentSplitPercent: 45, pollSeconds: 60, workshopAgent: AgentModel(model: nil, effort: nil),
            sliceAgent: AgentModel(model: nil, effort: nil),
            projects: ["p1": ConfigDocProject(name: "Work", workingDir: "", backend: .source, source: "demo")])
        let client = FixtureNatClient(config: config)
        var changed: [PluginsModel.PluginChange] = []
        let model = PluginsModel(client: client, projectsUsing: { $0 == "demo" ? ["Work"] : [] }) { changed.append($0) }
        let demo = Fixtures.pluginListing.installed[0]

        await model.uninstall(demo)
        XCTAssertEqual(model.pendingUninstall, PluginsModel.PendingUninstall(plugin: demo, projects: ["Work"]))
        XCTAssertTrue(client.writes.isEmpty, "nothing runs before the user says")
        XCTAssertTrue(changed.isEmpty)

        model.cancelUninstall()
        XCTAssertNil(model.pendingUninstall)
        XCTAssertTrue(client.writes.isEmpty, "cancel runs nothing")

        await model.uninstall(demo)
        guard let pending = model.pendingUninstall else { return XCTFail("not asked") }
        await model.confirmUninstall(pending)
        XCTAssertNil(model.pendingUninstall)
        XCTAssertEqual(client.writes, ["plugin-uninstall demo --delete-projects"])
        XCTAssertEqual(changed, [PluginsModel.PluginChange(plugin: "demo", deletedProjectIDs: ["p1"])])
    }

    func testRefusalsAreShownNotSwallowed() async {
        let model = PluginsModel(client: FixtureNatClient(behaviour: .refusing("nat said no")))
        await model.loadIfNeeded()
        XCTAssertNil(model.listing)
        XCTAssertEqual(model.loadError, "nat said no")

        await model.uninstall(Fixtures.pluginListing.installed[0])
        XCTAssertEqual(model.actionError, "nat said no")
        XCTAssertTrue(model.running.isEmpty)
    }

    func testAnActionAlreadyRunningIsNotStartedTwice() async {
        let client = FixtureNatClient(behaviour: .hanging)
        let model = PluginsModel(client: client)
        let demo = Fixtures.pluginListing.installed[0]
        let first = Task { await model.uninstall(demo) }
        while !model.running.contains(.uninstall(name: "demo")) { await Task.yield() }
        await model.uninstall(demo)
        XCTAssertTrue(model.running.contains(.uninstall(name: "demo")))
        first.cancel()
    }

    func testSaveSetupClearsTheFieldAndKeepsWhatThePluginSaid() async {
        let client = FixtureNatClient(plugins: Fixtures.pluginListingShortcut)
        let model = PluginsModel(client: client)
        let key = PluginsModel.SetupKey(plugin: "shortcut", field: "token")

        XCTAssertFalse(model.canSave(key))
        model.setupValues[key] = " \n"
        XCTAssertFalse(model.canSave(key), "a blank value is nothing to save")
        await model.saveSetup(plugin: "shortcut", field: "token")
        XCTAssertTrue(client.writes.isEmpty)

        model.setupValues[key] = "s3cret"
        XCTAssertTrue(model.canSave(key))
        await model.saveSetup(plugin: "shortcut", field: "token")
        XCTAssertEqual(model.setupValues[key], "")
        XCTAssertEqual(model.setupOutcomes[key], .saved("Logged in to scratch as Craig Scratch"))
        XCTAssertEqual(
            model.listing?.installed.first?.setup.first?.set, true,
            "the listing is read again, and the field now reads set")
        XCTAssertEqual(Fixtures.pluginListingShortcut.installed.first?.setup.first?.set, false)
        XCTAssertEqual(client.writes, ["source-setup shortcut --id token"])
        XCTAssertTrue(model.running.isEmpty)
        XCTAssertNil(model.actionError, "a setup answer is the field's, not the tab's")
    }

    /// A value saved may be what connects the plugin, so whoever reads the
    /// plugins hears of it; a refusal changes nothing to hear of.
    func testSaveSetupTellsWhoeverReadsThePlugins() async {
        var told = 0
        let model = PluginsModel(client: FixtureNatClient(plugins: Fixtures.pluginListingShortcut), pluginsChanged: {
            XCTAssertEqual($0, PluginsModel.PluginChange(plugin: "shortcut"))
            told += 1
        })
        let key = PluginsModel.SetupKey(plugin: "shortcut", field: "token")
        model.setupValues[key] = "s3cret"
        await model.saveSetup(plugin: "shortcut", field: "token")
        XCTAssertEqual(told, 1)

        let refusing = PluginsModel(client: FixtureNatClient(behaviour: .refusing("no")), pluginsChanged: { _ in told += 1 })
        refusing.setupValues[key] = "wrong"
        await refusing.saveSetup(plugin: "shortcut", field: "token")
        XCTAssertEqual(told, 1)
    }

    func testSaveSetupKeepsARefusalAndTheValue() async {
        let model = PluginsModel(client: FixtureNatClient(behaviour: .refusing("shortcut: token stored, but Shortcut refused it")))
        let key = PluginsModel.SetupKey(plugin: "shortcut", field: "token")
        model.setupValues[key] = "wrong"
        await model.saveSetup(plugin: "shortcut", field: "token")
        XCTAssertEqual(model.setupOutcomes[key], .refused("shortcut: token stored, but Shortcut refused it"))
        XCTAssertEqual(model.setupValues[key], "wrong", "kept to correct")
        XCTAssertNil(model.actionError)
        XCTAssertTrue(model.running.isEmpty)
    }

    /// A plugin that answers nothing still reads as saved.
    func testSaveSetupWithNoMessage() async {
        let model = PluginsModel(client: NatClient(commandRunner: PluginStubRunner(stdout: #"{"message": ""}"#)))
        let key = PluginsModel.SetupKey(plugin: "p", field: "f")
        model.setupValues[key] = "v"
        await model.saveSetup(plugin: "p", field: "f")
        XCTAssertEqual(model.setupOutcomes[key], .saved("Saved."))
    }

    func testReloadSourcePluginsReadsThemAgain() async {
        let app = await Fixtures.startedAppModel()
        await app.reloadSourcePlugins()
        XCTAssertEqual(app.sourcePlugins, Fixtures.sourcePlugins)
    }
}
