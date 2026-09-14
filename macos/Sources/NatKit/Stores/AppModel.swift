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
    /// The bare slice-ID sentinel `nat status` reported every planning agent
    /// under before they were scoped to a project — `agent.PlanSentinel` on
    /// the Go side. A planning agent launched now is keyed by its project
    /// (`TmuxSession.planTag(projectID:)`); this is only what a session
    /// started before the upgrade still answers to, and it belongs to no
    /// project, so any of them may attach it.
    public static let planSentinel = TmuxSession.planSentinel

    /// The current configuration.
    public private(set) var config: NatProjectConfig?

    /// True while a workshop launch is under way — the `nat workshop-launch`
    /// itself, and then the wait for the activity poll to report the session
    /// it started. It is held across both because the pane has nothing to
    /// draw in between: the command returning says a session exists, and the
    /// poll seeing it is what turns that into the attached terminal, so
    /// dropping the flag at the command would put the composer back on screen
    /// for the second or two the poll takes. Cleared by the agent appearing,
    /// and by a failure, which is what returns the user to the composer with
    /// the error over the request still typed there.
    public private(set) var workshopLaunching = false

    /// What the last workshop launch refused with — cleared by the next
    /// launch, and by selecting a slice, which is how the failure is
    /// dismissed.
    public private(set) var workshopLaunchError: String?

    /// The projects whose rail has the workshop row selected — per-project,
    /// like `selectedSliceIDs`, and mutually exclusive with a slice
    /// selection: the rail draws one selected row.
    private var workshopSelectedProjects: Set<String> = []

    /// Ordered list of project tabs: (id, name).
    public private(set) var projectTabs: [(id: String, name: String)] = []

    /// The ID of the currently active project.
    public private(set) var activeProjectID: String?

    /// Live agent activity (app-wide, spans all projects).
    public private(set) var activityStore: ActivityStore?

    /// The Claude account's own usage readout (app-wide, spans all projects
    /// like `activityStore` — it is a property of the logged-in account, not
    /// of any one tracked project).
    public private(set) var usageStore: UsageStore?

    /// Each handed-back slice's branch diff totals, for the review
    /// rail's "+N −N" (app-wide, spans all projects, keyed by slice id —
    /// mirrors how `activityStore` is one store rather than one per project).
    public private(set) var reviewStatsStore: ReviewStatsStore?

    /// Whether the app has anywhere to show the board at all: no config file
    /// was found, or one was found naming no projects. The window shows a
    /// welcome pane in its place, which offers the same two ways onto the
    /// board the "+" tab does — `addProject(id:name:)` is where both of them
    /// end — and a "Check Again" that re-runs `start()` for a workspace set
    /// up elsewhere in the meantime.
    public private(set) var needsOnboarding: Bool = true

    /// Per-project selected slice IDs.
    private var selectedSliceIDs: [String: String?] = [:]

    /// How long each visited slice's session is held from being reaped, keyed
    /// by slice ID and set to visit-time-plus-hold on every visit — the
    /// session reaper's guard against killing what was just looked at. A
    /// slice with no entry was never visited this run, which is what makes a
    /// session left over from a previous run dangling rather than merely
    /// idle: holds exist only within the current app run.
    private var heldUntil: [String: Date] = [:]

    /// Slice IDs a sweep has verified belong to a live In-progress session of
    /// a project this run has not got open — another window's project, most
    /// likely — so there is nothing to re-verify about them until the app
    /// restarts. Read fresh every sweep would cost a `nat slice-status` apiece
    /// forever. It holds only slices no open plan names, and is consulted
    /// only for those: a slice an open plan does name has this run watching
    /// its work end, and a cache entry that outlived that ending would keep
    /// its session alive forever — opening the project a cached slice belongs
    /// to is likewise what puts it back under the plan's own rule.
    private var verifiedElsewhere: Set<String> = []

    /// Project stores keyed by project ID (lazily created).
    private var stores: [String: ProjectStore] = [:]

    /// One slice-detail cache per project (lazily created) — shared by every
    /// `BriefTabView` for that project, rather than each tab view holding its
    /// own throwaway store that forgets everything the moment the user
    /// switches tabs or slices away and back.
    private var sliceDetailStores: [String: SliceDetailStore] = [:]

    /// One diff cache per project (lazily created), for the same reason —
    /// `DiffTabView` reads through this rather than owning a `DiffStore` of
    /// its own.
    private var diffStores: [String: DiffStore] = [:]

    /// One pull-request cache per project (lazily created), for the same
    /// reason — `PRTabView` reads through this rather than owning a
    /// `PRStore` of its own.
    private var prStores: [String: PRStore] = [:]

    private let configReader: ConfigReaderProtocol

    /// Where each project's last-good plan is kept between launches, handed
    /// to every `ProjectStore` this makes so the board draws from disk while
    /// the fresh read is in flight. Injectable so tests never touch the real
    /// Application Support directory.
    private let planCache: PlanCaching

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

    /// How `launchWorkshop(request:)` launches the planning agent — `nat
    /// workshop-launch` through the client. Injectable for the same reason
    /// `pathsProvider` is.
    private let workshopLauncher: @Sendable (
        _ projectID: String, _ model: String?, _ effort: String?, _ request: String?
    ) async throws -> WorkshopLaunchResult

    /// How every store this makes reaches nat. Injectable so a preview — or
    /// the coming gallery — can hand the whole app a canned client and get a
    /// board without a `nat` process anywhere near it; the default is the
    /// real one, which is what the app itself runs on.
    private let clientFactory: @Sendable () -> NatClientProtocol

    /// How the app-wide activity store is made. Injectable for the same
    /// reason `workshopLauncher` is: the default one polls tmux through the
    /// real client, and a test that wants to say what is running says it
    /// here.
    private let activityStoreFactory: @MainActor @Sendable () -> ActivityStore

    /// How the app-wide usage store is made. Injectable for the same reason
    /// `activityStoreFactory` is: the default probes and caches for real,
    /// and a test or the gallery hands a client (and, for a test, a cache)
    /// of its own here.
    private let usageStoreFactory: @MainActor @Sendable () -> UsageStore

    /// How `launchWorkshop(request:)` waits between askings, while a launched
    /// session has yet to show up in the activity poll's reading. Injectable
    /// so a test never waits a quarter of a second for anything.
    private let launchSettleWait: @MainActor @Sendable () async -> Void

    /// The clock the session reaper's visit holds are measured on. Injectable
    /// so a test can say what "five minutes later" means without waiting five
    /// minutes.
    private let now: @Sendable () -> Date

    /// How long a visited slice's session is held from being reaped —
    /// `agentVisitHold` unless a test says otherwise.
    private let visitHold: TimeInterval

    /// How many of those waits a launched session gets to appear in before
    /// the pane gives up on it and says so — thirty seconds at the default
    /// wait, which is long past the two the poll's own cadence costs.
    static let launchSettleAttempts = 120

    public init(
        configReader: ConfigReaderProtocol = FileConfigReader(),
        planCache: PlanCaching = DiskPlanCache(),
        pollIntervalSeconds: UInt64 = 30,
        pathsProvider: @escaping @Sendable () async throws -> NatPaths = { try await NatClient().paths() },
        workshopLauncher: @escaping @Sendable (String, String?, String?, String?) async throws -> WorkshopLaunchResult = {
            try await NatClient().workshopLaunch(projectID: $0, model: $1, effort: $2, request: $3)
        },
        clientFactory: @escaping @Sendable () -> NatClientProtocol = { NatClient() },
        activityStoreFactory: @escaping @MainActor @Sendable () -> ActivityStore = { ActivityStore() },
        usageStoreFactory: @escaping @MainActor @Sendable () -> UsageStore = { UsageStore() },
        launchSettleWait: @escaping @MainActor @Sendable () async -> Void = {
            try? await Task.sleep(nanoseconds: 250_000_000)
        },
        now: @escaping @Sendable () -> Date = { Date() },
        visitHold: TimeInterval = agentVisitHold
    ) {
        self.configReader = configReader
        self.planCache = planCache
        self.pollInterval = pollIntervalSeconds
        self.pathsProvider = pathsProvider
        self.clientFactory = clientFactory
        self.workshopLauncher = workshopLauncher
        self.activityStoreFactory = activityStoreFactory
        self.usageStoreFactory = usageStoreFactory
        self.launchSettleWait = launchSettleWait
        self.now = now
        self.visitHold = visitHold
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
    /// to show and no project to activate until one is opened or created,
    /// which comes back through `addProject(id:name:)`.
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
            let activityStore = activityStoreFactory()
            self.activityStore = activityStore
            self.reviewStatsStore = ReviewStatsStore(client: clientFactory())
            startUsageStore()

            // Activate the first project (if any)
            if let firstProjectID = sortedProjects.first?.key {
                await activateProject(firstProjectID, nudgePath: nudgePath, config: loadedConfig)
            }

            // Every other tab needs a loaded plan too, so a live agent on a
            // project the user has never clicked into still shows attention
            // on its tab (`attention(projectID:)` has nothing to attribute
            // without one) — loaded from each project's own cache and then
            // refreshed in the background, never blocking startup on it.
            for tab in projectTabs where tab.id != sortedProjects.first?.key {
                loadBackgroundProject(tab.id)
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
            stores[projectID] = ProjectStore(projectID: projectID, client: clientFactory(), cache: planCache)
        }

        guard let projectStore = stores[projectID] else { return }

        // Load the project store
        await projectStore.load()
        await updateReviewStats(projectID: projectID, projectStore: projectStore)
        // Every session a previous run left behind on a finished slice is
        // dangling, and this is the first sweep that sees them.
        await reapFinishedAgents()

        // Re-arm activity polling
        activityStore?.kick()

        // (Re)start nudge watcher and polling
        startNudgeWatcher(for: projectStore, nudgePath: nudgePath)
        startPolling(for: projectStore, seconds: pollSeconds(config))
    }

    /// Gives a background (never-activated) project's tab a loaded plan to
    /// attribute a live agent to, without the polling, nudge watcher or
    /// review-stats reading that only the active project gets: creates its
    /// store if it has none, then loads it — cache first, then a network
    /// refresh, `ProjectStore.load()`'s own shape — as an unawaited task, so
    /// one slow project's read never holds up the rest of startup.
    private func loadBackgroundProject(_ projectID: String) {
        if stores[projectID] == nil {
            stores[projectID] = ProjectStore(projectID: projectID, client: clientFactory(), cache: planCache)
        }
        guard let store = stores[projectID] else { return }
        Task { await store.load() }
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

    /// Close a project's tab for this session: the strip forgets it, the
    /// config does not — every configured project is a tab again at the next
    /// launch. Closing the active tab activates its neighbour (the tab that
    /// followed it, else the one before), and the last tab refuses to close:
    /// a board with no project is the onboarding screen's shape, and this is
    /// not onboarding.
    ///
    /// A closing tab's own dangling sessions get one final sweep first, with
    /// the closing tab's own slices' visit holds ignored — once this tab is
    /// gone, its plan stops being one the ordinary sweep considers at all, so
    /// this is the last chance for a while to catch what it was tracking, and
    /// a hold is about what the user just clicked away from, which closing
    /// this tab is not for any other tab's work. It runs before the tab is
    /// removed so every other open tab's plan is still in the mix, exactly as
    /// the ordinary sweep sees it: a session belonging to one of them is not
    /// mistaken for this tab's own dangling one.
    public func closeProject(_ projectID: String) async {
        guard projectTabs.count > 1,
              let index = projectTabs.firstIndex(where: { $0.id == projectID }) else { return }
        let closing = Set((stores[projectID]?.state.projectInfo?.slices ?? []).map(\.id))
        await reapFinishedAgents(ignoringHoldsFor: closing)
        projectTabs.remove(at: index)
        if activeProjectID == projectID {
            let neighbour = projectTabs[min(index, projectTabs.count - 1)]
            await activateProject(neighbour.id)
        }
    }

    /// Take a project just opened or created into the board: re-read config
    /// so the entry `project-open`/`project-create` wrote is in hand, give it
    /// a tab if it has none, and activate it. The two paths of the "+" tab
    /// end here, since what each produced is the same thing — one more entry
    /// in local config.
    ///
    /// The tab lands at the end of the strip rather than in the config's own
    /// order: it is where the user just made it, and the next launch is what
    /// files it away in order with the rest.
    ///
    /// A machine whose `start()` found no config at all — the onboarding
    /// pane's own state — has no config to re-read, so this is the start that
    /// was missed rather than a reload: there is a config file now. A start
    /// that still cannot read one leaves the board exactly as it was, since a
    /// board with no config behind it is the onboarding pane and not an empty
    /// plan.
    public func addProject(id: String, name: String) async {
        if config == nil || loadedConfigPath == nil {
            await start()
        } else {
            await reloadConfig()
        }
        guard let config = config else { return }
        needsOnboarding = false

        // start() builds these for a config that named projects; a first
        // project on a machine that had none arrives here with neither.
        if activityStore == nil {
            activityStore = activityStoreFactory()
            reviewStatsStore = ReviewStatsStore(client: clientFactory())
        }
        if usageStore == nil {
            startUsageStore()
        }

        if !projectTabs.contains(where: { $0.id == id }) {
            // The config's own name where it has one — it is what every other
            // tab is labelled with — and what the command reported otherwise.
            projectTabs.append((id: id, name: config.projects[id]?.name ?? name))
        }
        await activateProject(id)
    }

    /// Whether the active project's plan has landed and holds nothing — the
    /// state every project opened or created from the "+" tab starts in, and
    /// what the rail and the pane draw `EmptyProjectNote` for. A load still
    /// in flight, or one that failed, is not an empty plan: the rail reports
    /// either of those itself.
    public var activePlanIsEmpty: Bool {
        guard let info = projectStore?.state.projectInfo else { return false }
        return info.slices.isEmpty
    }

    /// Whether the active project has no working directory recorded — the
    /// state a project opened from the "+" tab starts in, since opening
    /// records where a plan lives and nothing about where its code does. The
    /// tab's empty state is what says so, and Settings is where it is given.
    ///
    /// False for a project config says nothing about at all: there is then no
    /// entry to be missing a directory, and the board has bigger problems to
    /// report than this one.
    public var activeProjectNeedsWorkingDir: Bool {
        guard let id = activeProjectID, let project = config?.projects[id] else { return false }
        return project.workingDir.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    /// The active project's store (computed property for backward compatibility).
    public var projectStore: ProjectStore? {
        guard let activeID = activeProjectID else { return nil }
        return stores[activeID]
    }

    /// The slice-detail cache for one project, created on first use — the
    /// same instance every call after that, so a slice's brief read once
    /// this session stays cached across tab switches and slice reselection.
    public func sliceDetailStore(projectID: String) -> SliceDetailStore {
        if let existing = sliceDetailStores[projectID] { return existing }
        let store = SliceDetailStore(projectID: projectID, client: clientFactory())
        sliceDetailStores[projectID] = store
        return store
    }

    /// The diff cache for one project, created on first use — see
    /// `sliceDetailStore(projectID:)`.
    public func diffStore(projectID: String) -> DiffStore {
        if let existing = diffStores[projectID] { return existing }
        let store = DiffStore(client: clientFactory())
        diffStores[projectID] = store
        return store
    }

    /// The pull-request cache for one project, created on first use — see
    /// `sliceDetailStore(projectID:)`.
    public func prStore(projectID: String) -> PRStore {
        if let existing = prStores[projectID] { return existing }
        let store = PRStore(client: clientFactory())
        prStores[projectID] = store
        return store
    }

    /// The currently selected slice ID (per-project). Selecting a slice
    /// deselects the workshop row — the rail draws one selected row — and
    /// dismisses any workshop launch failure, since looking away is how an
    /// error is put down.
    public var selectedSliceID: String? {
        get {
            guard let activeID = activeProjectID else { return nil }
            return selectedSliceIDs[activeID] ?? nil
        }
        set {
            guard let activeID = activeProjectID else { return }
            selectedSliceIDs[activeID] = newValue
            noteVisited(newValue)
            if newValue != nil {
                workshopSelectedProjects.remove(activeID)
                workshopLaunchError = nil
            }
        }
    }

    /// Stamps a slice as visited: its session is held from being reaped until
    /// `visitHold` from now, reset on every visit. The slice currently on
    /// screen is separately exempted outright by the reaper's own candidate
    /// rule, so what this hold is actually for is the window right after —
    /// long enough that clicking through the rail never kills the session the
    /// user is about to come back to.
    private func noteVisited(_ sliceID: String?) {
        guard let sliceID else { return }
        heldUntil[sliceID] = now().addingTimeInterval(visitHold)
    }

    // MARK: - Workshop

    /// The key the active project's planning agent sits at in
    /// `activityStore.agents` — its own project-qualified tag, or the bare
    /// legacy sentinel where that is what is running, since a session started
    /// before planning agents were scoped belongs to no project and every
    /// project may attach it. Nil when no project is active or none is live.
    public var planningAgentKey: String? {
        guard let activeID = activeProjectID, let agents = activityStore?.agents else { return nil }
        let scoped = TmuxSession.planTag(projectID: activeID)
        if agents[scoped] != nil { return scoped }
        if agents[Self.planSentinel] != nil { return Self.planSentinel }
        return nil
    }

    /// The active project's planning agent as the activity poll last saw it —
    /// nil while none runs. The live reading is the whole source of workshop
    /// presence, so an agent launched before this app started is found the same
    /// way one it launched itself is. Another project's planning agent is not
    /// this project's and is never drawn here: switching tabs switches the
    /// workshop with everything else.
    public var planningAgent: AgentStatus? {
        guard let key = planningAgentKey else { return nil }
        return activityStore?.agents[key]
    }

    /// Whether the active project's rail has the workshop row selected.
    /// Setting it true clears the slice selection — see `selectedSliceID`.
    public var workshopSelected: Bool {
        get {
            guard let activeID = activeProjectID else { return false }
            return workshopSelectedProjects.contains(activeID)
        }
        set {
            guard let activeID = activeProjectID else { return }
            if newValue {
                workshopSelectedProjects.insert(activeID)
                selectedSliceIDs[activeID] = nil
            } else {
                workshopSelectedProjects.remove(activeID)
            }
        }
    }

    /// The wand button: select the workshop row and nothing else. With a
    /// planning agent live the pane attaches to it; with none the pane opens
    /// on the composer asking what to workshop — the launch is the composer's
    /// own `launchWorkshop(request:)`, the board's `w` asking its question
    /// before any session starts.
    public func openWorkshop() {
        guard activeProjectID != nil else { return }
        workshopSelected = true
    }

    /// The composer's launch: start a planning agent on the active project
    /// with the config's workshop pair, the request folded into its prompt —
    /// trimmed, and empty meaning a plain session (or the wishlist, which is
    /// the CLI's own rule). Skipped when one is already live (there is only
    /// ever one — the CLI refuses a second). The activity poll is what turns
    /// a successful launch into a live row and an attached terminal, so it is
    /// kicked rather than the result being held here as a second source of
    /// truth.
    ///
    /// `workshopLaunching` goes up on the first line of the launch and stays
    /// up until there is something else for the pane to draw — the agent, or
    /// the failure — so the composer is gone the moment Launch is pressed and
    /// comes back only where the launch came to nothing.
    public func launchWorkshop(request: String) async {
        guard let projectID = activeProjectID else { return }
        workshopSelected = true
        guard planningAgent == nil, !workshopLaunching else { return }

        workshopLaunching = true
        workshopLaunchError = nil
        do {
            _ = try await workshopLauncher(
                projectID,
                config?.workshopAgent?.model,
                config?.workshopAgent?.effort,
                request.trimmingCharacters(in: .whitespacesAndNewlines)
            )
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                workshopLaunchError = message
            } else {
                workshopLaunchError = error.localizedDescription
            }
        } catch {
            workshopLaunchError = error.localizedDescription
        }
        // Kicked on failure too: "a planning agent is already live" means
        // there is a session the poll has not seen yet, and seeing it is
        // exactly what turns the refusal into the terminal.
        activityStore?.kick()

        // A launch that took leaves the pane with nothing to draw until the
        // poll reports the session, so the launching state is held across
        // that wait rather than flickering the composer back over it.
        if workshopLaunchError == nil {
            await settleOnPlanningAgent()
        }
        workshopLaunching = false
    }

    /// Wait for the activity poll to report the planning agent a launch just
    /// started, giving up after `launchSettleAttempts` and saying so. Giving
    /// up rather than waiting forever is the point: a session that started
    /// and exited on the spot, or a tmux the poll cannot read at all, would
    /// otherwise leave the pane launching for the rest of the session with
    /// nothing to launch.
    private func settleOnPlanningAgent() async {
        for _ in 0..<Self.launchSettleAttempts {
            if planningAgent != nil { return }
            await launchSettleWait()
        }
        guard planningAgent == nil else { return }
        workshopLaunchError = "the workshop session was launched but has not appeared — check `nat status`"
    }

    /// What a project's tab says needs attention — the count its pill draws
    /// and the state its dot takes, as one reading so the two cannot
    /// disagree. Nothing at all for a project whose plan has not landed:
    /// there are no slices to read anything off yet.
    ///
    /// The PR-readiness map is the active project's alone — one store, as
    /// `reviewStatsStore`'s comment says — so an inactive tab's dot has that
    /// refinement absent rather than wrong, exactly as the rail does with no
    /// reading taken. The planning agent is the one the activity map
    /// attributes to this project by its own scoped tag; the bare legacy
    /// sentinel belongs to no project in particular and is nobody's tab.
    public func attention(projectID: String) -> ProjectAttention {
        guard let projectInfo = stores[projectID]?.state.projectInfo else { return .none }

        let agents = activityStore?.agents ?? [:]
        let liveAgents = agents.mapValues { AgentActivity($0.activity) }
        let planning = agents[TmuxSession.planTag(projectID: projectID)]
            .map { AgentActivity($0.activity) }

        return projectAttention(
            slices: projectInfo.slices,
            liveAgents: liveAgents,
            planningAgent: planning,
            prReadiness: projectID == activeProjectID ? (reviewStatsStore?.prReadiness ?? [:]) : [:]
        )
    }

    /// Manually refresh the current project — also the nudge watcher's own
    /// action, so an agent's hand-back reads that slice's stat in without
    /// waiting for the next poll.
    public func refresh() async {
        guard let projectStore = projectStore else { return }
        await projectStore.refresh()
        await updateReviewStats(projectID: projectStore.projectID, projectStore: projectStore)
        await reapFinishedAgents()
        activityStore?.kick()
        // A slice's page may have changed underneath any reading of it taken
        // before this refresh landed — every cached reading but the one
        // currently on screen is dropped, so each re-reads fresh next time it
        // is actually selected rather than serving a possibly-stale brief
        // forever; the one on screen is left alone; blanking it here would
        // only cost the user their brief with nothing about to refetch it.
        sliceDetailStores[projectStore.projectID]?.invalidateCache(keeping: selectedSliceID)
    }

    // MARK: - Agent sessions

    /// End a slice's agent session outright — `nat agent-kill`, the one
    /// thing that actually reaps a session, since closing the Agent tab only
    /// detaches the viewer. The activity poll is re-armed straight after, so
    /// the session leaves the rail's live indicators on its next reading
    /// rather than at the next plan load.
    ///
    /// Answers with the refusal's own first line where nat refused, and nil
    /// where the session is gone. Nothing in the app asks for a kill by hand
    /// — `reapFinishedAgents` is the only caller — so a refusal is something
    /// to log rather than the app's error banner; `nat agent-kill` is where
    /// a kill is asked for outright.
    @discardableResult
    func killAgent(sliceID: String) async -> String? {
        guard let projectID = activeProjectID else { return "No project loaded" }
        do {
            try await clientFactory().agentKill(projectID: projectID, sliceRef: sliceID)
        } catch let error as NatError {
            if case .commandFailed(let message) = error { return message }
            return error.localizedDescription
        } catch {
            return error.localizedDescription
        }
        activityStore?.kick()
        return nil
    }

    /// Kills the agent sessions this run's open tabs have finished with —
    /// `agentSessionsToReap` is the candidate rule, `nat slice-status`, read
    /// fresh per candidate, the last word, and this is the sweep that applies
    /// both. It rides every open tab's own plan-load cadence: the app
    /// starting, a nudge landing and the poll ticking each run it for
    /// whichever tab they belong to, and `ignoringHoldsFor` is what
    /// `closeProject(_:)` asks for on a tab's way out — the closing tab's own
    /// slices' holds dropped, every other tab's left standing, since a hold
    /// is about what the user just clicked away from and closing one tab is
    /// not a click away from another's work.
    ///
    /// Every open tab's plan is in the mix, not only the active one's — a
    /// session dangling on a tab nobody has clicked to is exactly the kind
    /// this sweep exists to catch, and it would otherwise sit there until the
    /// user switched to that tab by hand. `projectTabs` is what "open" means;
    /// a project's `ProjectStore` can outlive its tab being closed (kept
    /// warm for a reopen), so reading `stores` directly would go on sweeping
    /// a project the user no longer has open at all.
    ///
    /// The activity reading is taken fresh rather than read off
    /// `activityStore`, whose first poll has not landed when the app has only
    /// just started — which is exactly the sweep that matters, since every
    /// session left running by a previous run is dangling. A reading that
    /// fails reaps nothing: a sweep is a kill, and nothing is killed on no
    /// news.
    ///
    /// Each candidate is then verified with a fresh `nat slice-status` and
    /// reaped only where that read says so (`verifiedForReap`) — closing the
    /// race the plan reading alone cannot: a session's own claim is always
    /// written before the session exists, so this fresh read can never show a
    /// phantom state the way the cached plan might. Only a candidate no open
    /// plan names is cached in `verifiedElsewhere` on an In-progress answer —
    /// another window's project's, with nothing here to watch it finish. One
    /// an open plan does name was nominated off a stale reading, which is
    /// exactly the race the fresh read exists to close, and is left uncached:
    /// the plan's next load stops nominating it by itself, and when its work
    /// really does end, that ending still has to be verified rather than
    /// found behind a cache entry that outlived it.
    private func reapFinishedAgents(ignoringHoldsFor unheld: Set<String> = []) async {
        let plans = projectTabs.compactMap { stores[$0.id]?.state.projectInfo }
        guard !plans.isEmpty, let projectID = activeProjectID else { return }
        var slicesByID: [String: Slice] = [:]
        for info in plans {
            for slice in info.slices { slicesByID[slice.id] = slice }
        }

        let client = clientFactory()
        guard let statuses = try? await client.status() else { return }

        var holds = heldUntil
        for id in unheld { holds.removeValue(forKey: id) }
        let candidates = agentSessionsToReap(
            agents: statuses,
            slicesByID: slicesByID,
            selectedSliceID: selectedSliceID,
            heldUntil: holds,
            now: now()
        )
        for sliceID in candidates {
            let inOpenPlan = slicesByID[sliceID] != nil
            if !inOpenPlan, verifiedElsewhere.contains(sliceID) { continue }
            let result = try? await client.sliceStatus(projectID: projectID, sliceRef: sliceID)
            guard verifiedForReap(result) else {
                if !inOpenPlan, case .found("In progress", false) = result {
                    verifiedElsewhere.insert(sliceID)
                }
                continue
            }
            if let refusal = await killAgent(sliceID: sliceID) {
                // Nothing to say to the user: nobody asked for this sweep,
                // and a session that would not die is one the next sweep
                // tries again on.
                NSLog("AppModel: could not reap the session for %@: %@", sliceID, refusal)
            }
        }
    }

    /// Makes and starts the app-wide usage store: the cache's last-known
    /// reading at once, then a fresh probe, then its own recurring timer.
    /// The probe runs detached from the caller — `start(configPath:nudgePath:)`
    /// and `addProject(id:name:)` both reach this while still setting up the
    /// rest of the app, and neither should wait out one probe's own timeout
    /// to finish doing so.
    private func startUsageStore() {
        let store = usageStoreFactory()
        self.usageStore = store
        Task { await store.start() }
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
        // The PR-readiness reading rides the same cadence the Go board's
        // does — every plan that lands — and is skipped the same way when no
        // slice has a pull request worth asking about.
        if info.slices.contains(where: { !$0.pr.isEmpty }) {
            await reviewStatsStore?.updatePRStatus(projectID: projectID)
        }
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
                        // The same load the refresh action makes — review
                        // stats and the PR-readiness reading included, so a
                        // pull request merged on GitHub (which writes nothing
                        // to Notion and so fires no nudge) leaves the NEEDS
                        // REVIEW rail within a poll interval rather than
                        // sitting there as "awaiting review" forever.
                        await self.refresh()
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
        usageStore?.stop()
        usageStore = nil
        reviewStatsStore = nil
        sliceDetailStores = [:]
        diffStores = [:]
        prStores = [:]
    }
}
