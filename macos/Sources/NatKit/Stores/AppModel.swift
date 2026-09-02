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

    /// Each handed-back slice's branch diff totals, for the NEEDS REVIEW
    /// rail's "+N −N" (app-wide, spans all projects, keyed by slice id —
    /// mirrors how `activityStore` is one store rather than one per project).
    public private(set) var reviewStatsStore: ReviewStatsStore?

    /// Whether the app has anywhere to show the board at all: no config file
    /// was found, or one was found naming no projects. Project creation stays
    /// with the TUI/CLI, so this is a dead end rather than a wizard — the
    /// window shows a welcome pane in its place until a "Check Again" re-runs
    /// `start()`.
    public private(set) var needsOnboarding: Bool = true

    /// Per-project selected slice IDs.
    private var selectedSliceIDs: [String: String?] = [:]

    /// Project stores keyed by project ID (lazily created).
    private var stores: [String: ProjectStore] = [:]

    private let configReader: ConfigReaderProtocol
    private let pollInterval: UInt64 // in seconds
    private var pollTask: Task<Void, Never>?
    private var nudgeWatcher: NudgeWatcher?

    /// The path config was last successfully loaded from, so `reloadConfig()`
    /// can re-read it without needing the paths resolved again.
    private var loadedConfigPath: String?

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
    ///
    /// No config file at all, or one naming no projects, leaves
    /// `needsOnboarding` true and does nothing else here: there is no board
    /// to show and no project to activate, and project creation is the
    /// TUI/CLI's job rather than a wizard of this app's own.
    public func start(configPath: String, nudgePath: String) async {
        do {
            let loadedConfig = try await configReader.readConfig(from: configPath)
            self.config = loadedConfig
            self.loadedConfigPath = configPath

            guard !loadedConfig.projects.isEmpty else {
                needsOnboarding = true
                return
            }
            needsOnboarding = false

            // Build project tabs from config, sorted by project name
            let sortedProjects = loadedConfig.projects.sorted { $0.key < $1.key }
            self.projectTabs = sortedProjects.map { (id: $0.key, name: $0.value.name) }

            // Create activity store (app-wide)
            let activityStore = ActivityStore()
            self.activityStore = activityStore
            self.reviewStatsStore = ReviewStatsStore()

            // Activate the first project (if any)
            if let firstProjectID = sortedProjects.first?.key {
                await activateProject(firstProjectID, nudgePath: nudgePath, config: loadedConfig)
            }
        } catch {
            // No config file to read from is the common case here, not a
            // crash-worthy one: it is exactly what a first run looks like.
            needsOnboarding = true
            NSLog("Failed to load config: %@", error.localizedDescription)
        }
    }

    /// Re-read config from wherever it was last successfully loaded, without
    /// touching project tabs, the active project or any timer: the settings
    /// scene calls this after a successful save so poll cadence, the model
    /// pairs and a project's working directory pick up the new values on
    /// their own next use, without restarting the app.
    ///
    /// Does nothing if config has never been loaded, or the re-read fails —
    /// the config already in hand is kept rather than dropped for a
    /// transient read error.
    public func reloadConfig() async {
        guard let path = loadedConfigPath else { return }
        guard let reloaded = try? await configReader.readConfig(from: path) else { return }
        self.config = reloaded
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
        await updateReviewStats(projectID: projectID, projectStore: projectStore)

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

    /// Manually refresh the current project — also the nudge watcher's own
    /// action, so an agent's hand-back reads that slice's stat in without
    /// waiting for the next poll.
    public func refresh() async {
        guard let projectStore = projectStore else { return }
        await projectStore.refresh()
        await updateReviewStats(projectID: projectStore.projectID, projectStore: projectStore)
        activityStore?.kick()
    }

    // MARK: - Private Helpers

    /// Feeds the active project's currently handed-back slices to
    /// `reviewStatsStore` — the store itself decides whether any of them are
    /// worth a fresh fetch (a branch it already has a tally for is left
    /// alone). A load that has not landed anything yet (no `projectInfo`) has
    /// nothing to feed it.
    private func updateReviewStats(projectID: String, projectStore: ProjectStore) async {
        guard let info = projectStore.state.projectInfo else { return }
        let handedBack = info.slices
            .filter { $0.handedBack }
            .map { ReviewStatsStore.HandedBackSlice(sliceID: $0.id, branch: $0.branch ?? "") }
        await reviewStatsStore?.update(projectID: projectID, handedBack: handedBack)
    }

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
        reviewStatsStore = nil
    }
}
