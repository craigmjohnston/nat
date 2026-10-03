import Foundation

/// Settings ▸ Sources' state: `nat plugin-list` as last read, which buttons
/// have a command running, and what nat last refused. The view binds to it
/// and draws; installing, updating, uninstalling and the source list are all
/// nat's — every action here is one `nat plugin-*` call, then the listing
/// read again, so what the tab shows is always what nat last said.
@MainActor
@Observable
public final class PluginsModel {
    /// The listing, nil until the first read lands.
    public private(set) var listing: PluginListing?

    /// Why the listing could not be read, shown in place of it.
    public private(set) var loadError: String?

    /// What the last action was refused with, shown above the groups until
    /// the next action.
    public private(set) var actionError: String?

    /// The actions running, by `PluginsModel.Action` — what puts a spinner in
    /// place of a button.
    public private(set) var running: Set<Action> = []

    /// The add field's text.
    public var newSource = ""

    /// One button's command, as the spinner it shows is keyed.
    public enum Action: Hashable, Sendable {
        case install(source: String, name: String)
        case update(name: String)
        case uninstall(name: String)
        case addSource
        case removeSource(repo: String)
    }

    @ObservationIgnored private let client: NatClientProtocol
    /// Told after a plugin is installed or taken away, so what else reads
    /// the installed plugins (the `+` menu's) reads them again.
    @ObservationIgnored private let pluginsChanged: @MainActor () async -> Void

    public init(client: NatClientProtocol, pluginsChanged: @escaping @MainActor () async -> Void = {}) {
        self.client = client
        self.pluginsChanged = pluginsChanged
    }

    /// Whether the add field holds something worth sending.
    public var canAddSource: Bool {
        PluginSourceRepo.isValid(newSource.trimmingCharacters(in: .whitespaces)) && !running.contains(.addSource)
    }

    /// Reads the listing. A failed read keeps whatever was drawn before it
    /// off the screen: a listing is what nat says now, and one it can no
    /// longer say is not one to show.
    public func load() async {
        do {
            listing = try await client.pluginList()
            loadError = nil
        } catch {
            listing = nil
            loadError = Self.message(error)
        }
    }

    /// The first read, only once: a tab shown again keeps its listing.
    public func loadIfNeeded() async {
        guard listing == nil, loadError == nil else { return }
        await load()
    }

    public func install(_ plugin: AvailablePlugin) async {
        await run(.install(source: plugin.source, name: plugin.name), changesPlugins: true) {
            _ = try await self.client.pluginInstall(name: plugin.name, source: plugin.source, version: nil)
        }
    }

    /// An update is an install from the source it came from, at its latest.
    public func update(_ plugin: InstalledPlugin) async {
        await run(.update(name: plugin.name), changesPlugins: true) {
            _ = try await self.client.pluginInstall(name: plugin.name, source: plugin.source, version: nil)
        }
    }

    public func uninstall(_ plugin: InstalledPlugin) async {
        await run(.uninstall(name: plugin.name), changesPlugins: true) {
            _ = try await self.client.pluginUninstall(name: plugin.name)
        }
    }

    /// Adds the field's source, clearing the field once nat took it.
    public func addSource() async {
        let repo = newSource.trimmingCharacters(in: .whitespaces)
        guard canAddSource else { return }
        await run(.addSource, changesPlugins: false) {
            _ = try await self.client.pluginSourceAdd(repo: repo)
            self.newSource = ""
        }
    }

    public func removeSource(_ repo: String) async {
        await run(.removeSource(repo: repo), changesPlugins: false) {
            _ = try await self.client.pluginSourceRemove(repo: repo)
        }
    }

    /// One action: its spinner up while nat runs, its refusal kept, and the
    /// listing read again either way — a refusal may still have changed
    /// something, and the tab draws only what nat says now.
    private func run(_ action: Action, changesPlugins: Bool, _ body: @MainActor () async throws -> Void) async {
        guard !running.contains(action) else { return }
        running.insert(action)
        actionError = nil
        do {
            try await body()
        } catch {
            actionError = Self.message(error)
        }
        await load()
        running.remove(action)
        if changesPlugins { await pluginsChanged() }
    }

    private static func message(_ error: Error) -> String {
        if case NatError.commandFailed(let message) = error { return message }
        return error.localizedDescription
    }
}
