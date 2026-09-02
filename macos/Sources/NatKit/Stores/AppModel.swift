import Foundation
import SwiftUI

/// Protocol for reading the configuration file.
public protocol ConfigReaderProtocol: Sendable {
    func readConfig(from path: String) async throws -> NatProjectConfig
}

/// Default implementation that reads from the filesystem.
public struct FileConfigReader: ConfigReaderProtocol {
    public init() {}

    public func readConfig(from path: String) async throws -> NatProjectConfig {
        let data = try Data(contentsOf: URL(fileURLWithPath: path))
        let decoder = JSONDecoder()
        return try decoder.decode(NatProjectConfig.self, from: data)
    }
}

/// The top-level application state, observable and main-thread-only.
@MainActor
@Observable
public final class AppModel {
    /// The current configuration.
    public private(set) var config: NatProjectConfig?

    /// Ordered list of project tabs: (id, name).
    public private(set) var projectTabs: [(id: String, name: String)] = []

    /// The ID of the currently active project.
    public private(set) var activeProjectID: String?

    /// Live agent activity (app-wide, spans all projects).
    public private(set) var activityStore: ActivityStore?

    /// Per-project selected slice IDs.
    private var selectedSliceIDs: [String: String?] = [:]

    /// Project stores keyed by project ID (lazily created).
    private var stores: [String: ProjectStore] = [:]

    private let configReader: ConfigReaderProtocol
    private let pollInterval: UInt64 // in seconds
    private var pollTask: Task<Void, Never>?
    private var nudgeWatcher: NudgeWatcher?

    /// How the zero-argument `start()` finds the config and nudge files:
    /// `nat paths --json` through the same client every other read uses, so a
    /// NAT_BIN override reaches it too. Injectable so tests never spawn one.
    private let pathsProvider: @Sendable () async throws -> NatPaths

    public init(
        configReader: ConfigReaderProtocol = FileConfigReader(),
        pollIntervalSeconds: UInt64 = 30,
        pathsProvider: @escaping @Sendable () async throws -> NatPaths = { try await NatClient().paths() }
    ) {
        self.configReader = configReader
        self.pollInterval = pollIntervalSeconds
        self.pathsProvider = pathsProvider
    }

    /// Start the app resolving the config and nudge paths from `nat paths`,
    /// falling back to nat's own defaults when the binary on PATH is too old
    /// to answer (or missing): the paths are derivable, and a board that
    /// cannot ask still has a config to read.
    public func start() async {
        var configPath = NSHomeDirectory() + "/.config/notion-agent-tracker/config.json"
        var nudgePath = NSHomeDirectory() + "/Library/Logs/notion-agent-tracker/nudge"
        if let paths = try? await pathsProvider() {
            configPath = paths.config
            nudgePath = paths.nudge
        }
        await start(configPath: configPath, nudgePath: nudgePath)
    }

    /// Start the app: load config, create project store, start timers.
    public func start(configPath: String, nudgePath: String) async {
        // Load config
        do {
            let loadedConfig = try await configReader.readConfig(from: configPath)
            self.config = loadedConfig

            // Build project tabs from config, sorted by project name
            let sortedProjects = loadedConfig.projects.sorted { $0.key < $1.key }
            self.projectTabs = sortedProjects.map { (id: $0.key, name: $0.value.name) }

            // Create activity store (app-wide)
            let activityStore = ActivityStore()
            self.activityStore = activityStore

            // Activate the first project (if any)
            if let firstProjectID = sortedProjects.first?.key {
                await activateProject(firstProjectID, nudgePath: nudgePath, config: loadedConfig)
            }
        } catch {
            // Log error, but don't crash
            NSLog("Failed to load config: %@", error.localizedDescription)
        }
    }

    /// Activate a project by ID, creating and loading its store lazily.
    public func activateProject(_ projectID: String, nudgePath: String, config: NatProjectConfig) async {
        activeProjectID = projectID

        // Create or retrieve the project store
        if stores[projectID] == nil {
            stores[projectID] = ProjectStore(projectID: projectID)
        }

        guard let projectStore = stores[projectID] else { return }

        // Load the project store
        await projectStore.load()

        // Re-arm activity polling
        activityStore?.kick()

        // (Re)start nudge watcher and polling
        startNudgeWatcher(for: projectStore, nudgePath: nudgePath)
        startPolling(for: projectStore, seconds: pollSeconds(config))
    }

    /// Activate a project by ID (public convenience).
    public func activateProject(_ projectID: String) async {
        guard let config = config else { return }

        var nudgePath = NSHomeDirectory() + "/Library/Logs/notion-agent-tracker/nudge"
        if let paths = try? await pathsProvider() {
            nudgePath = paths.nudge
        }

        await activateProject(projectID, nudgePath: nudgePath, config: config)
    }

    /// The active project's store (computed property for backward compatibility).
    public var projectStore: ProjectStore? {
        guard let activeID = activeProjectID else { return nil }
        return stores[activeID]
    }

    /// The currently selected slice ID (per-project).
    public var selectedSliceID: String? {
        get {
            guard let activeID = activeProjectID else { return nil }
            return selectedSliceIDs[activeID] ?? nil
        }
        set {
            guard let activeID = activeProjectID else { return }
            selectedSliceIDs[activeID] = newValue
        }
    }

    /// Return the count of live agents in a given project.
    public func liveCount(projectID: String) -> Int {
        guard let projectStore = stores[projectID] else { return 0 }
        guard let projectInfo = projectStore.state.projectInfo else { return 0 }

        let sliceIDs = Set(projectInfo.slices.map { $0.id })
        var count = 0
        for (sliceID, _) in activityStore?.agents ?? [:] {
            if sliceIDs.contains(sliceID) {
                count += 1
            }
        }
        return count
    }

    /// Manually refresh the current project.
    public func refresh() async {
        guard let projectStore = projectStore else { return }
        await projectStore.refresh()
        activityStore?.kick()
    }

    // MARK: - Private Helpers

    private func startNudgeWatcher(for projectStore: ProjectStore, nudgePath: String) {
        let watcher = NudgeWatcher()
        watcher.start(path: nudgePath) { [weak self] in
            Task { @MainActor in
                await self?.refresh()
            }
        }
        self.nudgeWatcher = watcher
    }

    /// The poll cadence is the config's own poll_seconds where it names one,
    /// exactly as the TUI reads it, and the init's default otherwise.
    private func pollSeconds(_ config: NatProjectConfig) -> UInt64 {
        if let s = config.pollSeconds, s > 0 { return UInt64(s) }
        return pollInterval
    }

    private func startPolling(for projectStore: ProjectStore, seconds: UInt64) {
        // Cancel any existing poll task
        pollTask?.cancel()

        let pollInterval = seconds
        pollTask = Task {
            while !Task.isCancelled {
                do {
                    // Sleep for the poll interval
                    try await Task.sleep(nanoseconds: pollInterval * 1_000_000_000)

                    if !Task.isCancelled {
                        await projectStore.refresh()
                    }
                } catch {
                    // Task was cancelled; exit the loop
                    break
                }
            }
        }
    }

    /// Clean up resources when the app model is no longer needed.
    public func cleanup() {
        pollTask?.cancel()
        pollTask = nil
        nudgeWatcher?.stop()
        nudgeWatcher = nil
        activityStore?.stop()
        activityStore = nil
    }
}
