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

    /// The composer's typed-but-not-yet-launched request, per project — kept
    /// here rather than as `WorkshopPaneView`'s own `@State` so switching to a
    /// slice and back does not tear the composer down with it (`PaneView`
    /// mounts the workshop pane in a plain conditional). Cleared only by a
    /// successful launch (`launchWorkshop(request:)`) or the ✕ closing the
    /// tab outright — never by navigating away, which is the one thing this
    /// exists to survive. In-memory only, like every other per-project piece
    /// of view state here; there is no draft to restore across app launches.
    private var workshopDrafts: [String: String] = [:]

    /// Ordered list of project tabs: (id, name).
    public private(set) var projectTabs: [(id: String, name: String)] = []

    /// The reserved scratch project, when config names one that is also among
    /// its projects: pinned first in `projectTabs`, drawn icon-only and never
    /// closable. A local project like any other underneath.
    public private(set) var scratchProjectID: String?

    /// How many tabs count toward the last-tab rule: every one but the scratch
    /// tab, which is always there and never closable, so it is neither a tab
    /// the user can close nor one that makes another safe to.
    public var closableTabCount: Int {
        projectTabs.filter { $0.id != scratchProjectID }.count
    }

    /// Whether a tab is the scratch tab.
    public func isScratchTab(_ projectID: String) -> Bool {
        projectID == scratchProjectID
    }

    /// The prefix of an Untitled tab's ID. Not a page ID or a plan's own —
    /// nothing else carries it, which is all `isUntitledTab` reads.
    static let untitledPrefix = "untitled-"

    /// What an Untitled tab is labelled with.
    public static let untitledName = "Untitled"

    /// How many Untitled tabs this run has opened, so each gets an ID of its
    /// own: more than one may exist, and a closed one's ID is not reused.
    private var untitledOpened = 0

    /// Whether a tab is an Untitled one — a tab backed by no project, which
    /// shows the starter card until a project is opened into it.
    public func isUntitledTab(_ projectID: String) -> Bool {
        projectID.hasPrefix(Self.untitledPrefix)
    }

    /// Whether the tab the user is on is an Untitled one — the rail's empty
    /// shape and the pane's starter card, and no store behind either.
    public var activeTabIsUntitled: Bool {
        activeProjectID.map(isUntitledTab) ?? false
    }

    /// Open a new Untitled tab at the end of the strip and switch to it. The
    /// "+" beside the strip and a launch with no projects both come here.
    /// Nothing is read or started: there is no project for a store to be of.
    @discardableResult
    public func openUntitledTab() -> String {
        untitledOpened += 1
        let id = Self.untitledPrefix + String(untitledOpened)
        projectTabs.append((id: id, name: Self.untitledName))
        activeProjectID = id
        return id
    }

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

    /// The active project's ad hoc sessions (app-wide store, active-project
    /// reading — mirrors `reviewStatsStore`'s own shape).
    public private(set) var sessionStore: SessionStore?

    /// Per-project selected session IDs, alongside `selectedSliceIDs` and
    /// `workshopSelectedProjects` — the rail draws exactly one selected row.
    private var selectedSessionIDs: [String: String?] = [:]

    /// Busy while a New Session launch is under way, and whatever it refused
    /// with — the rail button's own state, cleared by the next launch and by
    /// selecting anything else.
    /// The folder the last scratch-tab session was launched in, which the
    /// folder panel opens on next time. In memory only, like the rest of the
    /// per-session view state here: a relaunch starts the panel at its default.
    public private(set) var lastSessionFolder: String?

    /// Whether the New Session button must ask for a folder first: on the
    /// scratch tab, which has no fixed working directory worth launching in.
    /// Every other tab launches straight away in its own.
    public var newSessionNeedsFolder: Bool { activeTabIsScratch }

    /// Whether the tab the user is on is the scratch tab.
    public var activeTabIsScratch: Bool {
        activeProjectID.map(isScratchTab) ?? false
    }

    public private(set) var newSessionLaunching = false
    public private(set) var newSessionError: String?

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

    /// One ad hoc session diff cache per project (lazily created), for the
    /// same reason — `SessionDiffTabView` reads through this rather than
    /// owning a `SessionDiffStore` of its own.
    private var sessionDiffStores: [String: SessionDiffStore] = [:]

    /// Which pull request and which branch each ad hoc session's pickers last
    /// had selected, for the app session — read and written only through
    /// `selectedPickerID`/`selectPicker`.
    private var pickerMemory = PickerSelectionMemory()

    /// One pull-request cache per project (lazily created), for the same
    /// reason — `PRTabView` reads through this rather than owning a
    /// `PRStore` of its own.
    private var prStores: [String: PRStore] = [:]

    /// Every one-shot slice action's state (launch, approve, merge) and the
    /// stage move in flight, held here rather than in the tab a button lives
    /// on: an optimistic advance unmounts that tab mid-action.
    public let sliceActions = SliceActionTracker()

    /// Slices approved over pending review comments: the comments went to
    /// the agent and the slice was taken out of review (`slice-rework`), and
    /// the approve is owed once its next hand-back lands. The value says
    /// whether that rework has been *seen* — a refresh reading the slice as
    /// no longer handed back — because a plan read taken before the rework
    /// landed still shows the old hand-back, and approving on that would open
    /// the pull request over the unfixed work. Held in memory only: a
    /// restart forgets it, and the next hand-back is then reviewed as normal,
    /// which is the safe direction to lose it in.
    public private(set) var approvalsPending: [String: Bool] = [:]

    /// Remember a slice as approved-pending-fixes — called once its comments
    /// have been sent and it has been taken out of review.
    public func markApprovePending(sliceID: String) {
        approvalsPending[sliceID] = false
    }

    /// Whether a slice is waiting on its next hand-back to be approved.
    public func isApprovePending(sliceID: String) -> Bool {
        approvalsPending[sliceID] != nil
    }

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

    /// Whether every binary onboarding checks for is on the machine — what
    /// separates a launch with no projects that opens the starter card from
    /// one that shows the checklist. Injectable so a test says which without
    /// looking at the machine it runs on.
    private let toolsReady: @Sendable () -> Bool

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
        visitHold: TimeInterval = agentVisitHold,
        toolsReady: @escaping @Sendable () -> Bool = {
            ["nat", "tmux", "gh", "ntn"].allSatisfy { BinaryLocator.status(of: $0).isFound }
        }
    ) {
        self.toolsReady = toolsReady
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
        await prepareScratchProject()
        var configPath = NSHomeDirectory() + "/.config/notion-agent-tracker/config.json"
        var nudgePath = NSHomeDirectory() + "/Library/Logs/notion-agent-tracker/nudge"
        if let paths = try? await pathsProvider() {
            configPath = paths.config
            nudgePath = paths.nudge
        }
        await start(configPath: configPath, nudgePath: nudgePath)
    }

    /// Whether this launch has already opened and cleared the scratch project.
    /// `start()` runs again when onboarding finishes, and the clear is once per
    /// launch: a slice marked Done on the scratch tab stays there until the
    /// next launch rather than vanishing under the user.
    private var scratchPrepared = false

    /// `nat scratch-open`, then `nat done-clear` on what it named — both before
    /// the scratch project's first read, so the rail never draws a plan that is
    /// about to be emptied. Neither failure stops the launch: without the
    /// scratch project there is simply no scratch tab, and a failed clear leaves
    /// the rail drawing whatever is there. A binary too old to know either
    /// command lands in the first case.
    private func prepareScratchProject() async {
        guard !scratchPrepared else { return }
        let client = clientFactory()
        let scratch: ScratchOpenResult
        do {
            scratch = try await client.scratchOpen()
        } catch {
            NSLog("AppModel: could not open the scratch project: %@", error.localizedDescription)
            return
        }
        scratchPrepared = true
        do {
            _ = try await client.doneClear(projectID: scratch.id)
        } catch {
            NSLog("AppModel: could not clear the scratch project's Done work: %@", error.localizedDescription)
        }
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
                // A config naming no projects, on a machine with the whole
                // toolchain, is somewhere to start rather than something to
                // install: one Untitled tab, with the starter card. Without
                // the tools it is the onboarding pane's checklist, as before.
                if toolsReady() {
                    startWithUntitledTab()
                } else {
                    needsOnboarding = true
                }
                return
            }
            needsOnboarding = false

            // Build project tabs from config, sorted by project ID, with the
            // scratch project (when config names one it also tracks) pinned
            // ahead of that sort.
            var sortedProjects = loadedConfig.projects.sorted { $0.key < $1.key }
            self.scratchProjectID = loadedConfig.scratchProject.flatMap {
                loadedConfig.projects[$0] == nil ? nil : $0
            }
            if let scratch = scratchProjectID, let at = sortedProjects.firstIndex(where: { $0.key == scratch }) {
                sortedProjects.insert(sortedProjects.remove(at: at), at: 0)
            }
            self.projectTabs = sortedProjects.map { (id: $0.key, name: $0.value.name) }

            // Create activity store (app-wide)
            let activityStore = activityStoreFactory()
            self.activityStore = activityStore
            self.reviewStatsStore = ReviewStatsStore(client: clientFactory())
            self.sessionStore = SessionStore(client: clientFactory())
            startUsageStore()

            // Activate the first project (if any): the first real one, since
            // the scratch tab is somewhere to go rather than where to start —
            // unless it is all there is.
            let firstProjectID = sortedProjects.first { $0.key != scratchProjectID }?.key
                ?? sortedProjects.first?.key
            if let firstProjectID {
                await activateProject(firstProjectID, nudgePath: nudgePath, config: loadedConfig)
            }

            // Every other tab needs a loaded plan too, so a live agent on a
            // project the user has never clicked into still shows attention
            // on its tab (`attention(projectID:)` has nothing to attribute
            // without one) — loaded from each project's own cache and then
            // refreshed in the background, never blocking startup on it.
            for tab in projectTabs where tab.id != firstProjectID {
                loadBackgroundProject(tab.id)
            }
        } catch {
            // No config file to read from is the common case here, not a
            // crash-worthy one: it is exactly what a first run looks like.
            needsOnboarding = true
            NSLog("Failed to load config: %@", error.localizedDescription)
        }
    }

    /// The board for a launch with no projects: the app-wide stores a first
    /// project would need (`addProject` finds them made), and one Untitled
    /// tab — not another when `start()` runs again with one already open.
    private func startWithUntitledTab() {
        needsOnboarding = false
        if activityStore == nil {
            activityStore = activityStoreFactory()
            reviewStatsStore = ReviewStatsStore(client: clientFactory())
            sessionStore = SessionStore(client: clientFactory())
        }
        if usageStore == nil {
            startUsageStore()
        }
        if !projectTabs.contains(where: { isUntitledTab($0.id) }) {
            openUntitledTab()
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
        // An Untitled tab has no store to make or load, no poll of its own to
        // arm: the tab being on it is the whole of the activation.
        if isUntitledTab(projectID) {
            activeProjectID = projectID
            return
        }
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
        guard projectID != scratchProjectID,
              closableTabCount > 1,
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
    ///
    /// `replacing` names the Untitled tab the project was opened from: it
    /// becomes the project's tab, in the place it held, rather than one more
    /// tab beside it. A project that already has a tab of its own leaves the
    /// Untitled one simply closed.
    public func addProject(id: String, name: String, replacing untitledID: String? = nil) async {
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
            sessionStore = SessionStore(client: clientFactory())
        }
        if usageStore == nil {
            startUsageStore()
        }

        let replaced = untitledID.flatMap { untitled in
            isUntitledTab(untitled) ? projectTabs.firstIndex(where: { $0.id == untitled }) : nil
        }
        if !projectTabs.contains(where: { $0.id == id }) {
            // The config's own name where it has one — it is what every other
            // tab is labelled with — and what the command reported otherwise.
            let tab = (id: id, name: config.projects[id]?.name ?? name)
            if let replaced {
                projectTabs[replaced] = tab
            } else {
                projectTabs.append(tab)
            }
        } else if let replaced {
            projectTabs.remove(at: replaced)
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

    /// What a session's `picker` shows selected among `ids`: the choice made
    /// earlier this app session while it is still there, else `defaultID`.
    public func selectedPickerID(_ picker: SessionPicker, sessionID: String, among ids: [String], defaultID: String? = nil) -> String? {
        pickerMemory.resolved(for: picker.key(sessionID: sessionID), among: ids, defaultID: defaultID)
    }

    /// Remember `id` as what a session's `picker` shows selected.
    public func selectPicker(_ picker: SessionPicker, sessionID: String, id: String) {
        pickerMemory.select(id, for: picker.key(sessionID: sessionID))
    }

    /// The ad hoc session diff cache for one project, created on first use —
    /// see `sliceDetailStore(projectID:)`.
    public func sessionDiffStore(projectID: String) -> SessionDiffStore {
        if let existing = sessionDiffStores[projectID] { return existing }
        let store = SessionDiffStore(client: clientFactory())
        sessionDiffStores[projectID] = store
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
                selectedSessionIDs[activeID] = nil
            }
        }
    }

    /// The currently selected ad hoc session ID (per-project), alongside
    /// `selectedSliceID` and `workshopSelected` — the rail draws exactly one
    /// selected row, so selecting a session deselects the other two.
    public var selectedSessionID: String? {
        get {
            guard let activeID = activeProjectID else { return nil }
            return selectedSessionIDs[activeID] ?? nil
        }
        set {
            guard let activeID = activeProjectID else { return }
            selectedSessionIDs[activeID] = newValue
            if newValue != nil {
                selectedSliceIDs[activeID] = nil
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

    /// The live agent the pane is attached to — the selected slice's, the
    /// selected ad hoc session's, or the workshop's planning agent — as the
    /// activity poll last saw it. Nil with none live, which is what the
    /// status bar's model/effort/context readout draws nothing for.
    public var attachedAgent: AgentStatus? {
        let agents = activityStore?.agents ?? [:]
        if workshopSelected { return planningAgent }
        if let sliceID = selectedSliceID { return agents[sliceID] }
        if let sessionID = selectedSessionID,
           let session = sessionStore?.sessions.first(where: { $0.id == sessionID }) {
            return agents[session.tag]
        }
        return nil
    }

    /// The active project's workshop draft — what `WorkshopPaneView`'s
    /// composer binds to instead of its own local state, so the text
    /// survives the view being torn down and remounted by a tab switch.
    /// Empty (never nil) with no active project, mirroring how the composer
    /// itself has nothing to bind to then either.
    public var workshopDraft: String {
        get {
            guard let activeID = activeProjectID else { return "" }
            return workshopDrafts[activeID] ?? ""
        }
        set {
            guard let activeID = activeProjectID else { return }
            workshopDrafts[activeID] = newValue
        }
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
                selectedSessionIDs[activeID] = nil
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
    /// trimmed, and empty meaning a plain session. Skipped when one is already live (there is only
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
            // The one thing that discards the draft besides the ✕: a launch
            // that took is the request actually being used, so there is
            // nothing left in it worth keeping for the next visit.
            workshopDrafts[projectID] = nil
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
            prReadiness: projectID == activeProjectID ? (reviewStatsStore?.prReadiness ?? [:]) : [:],
            sessions: projectID == activeProjectID ? (sessionStore?.sessions ?? []) : []
        )
    }

    /// Manually refresh the current project — also the nudge watcher's own
    /// action, so an agent's hand-back reads that slice's stat in without
    /// waiting for the next poll.
    public func refresh() async {
        guard let projectStore = projectStore else { return }
        await projectStore.refresh()
        await settlePendingApprovals(projectStore: projectStore)
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

    /// Runs the approve a slice is owed when the plan just read shows it
    /// handed back again — `slice-approve` exactly as the Approve button runs
    /// it, its refusal (gh's) surfacing the same way on the Diff tab. The
    /// mark is dropped *before* the approve starts, so the nudge and the poll
    /// landing together cannot open it twice, and a slice that failed here is
    /// back to being reviewed by hand rather than retried behind the user's
    /// back. A slice found Done or with a pull request already open has
    /// nothing left to approve, and its mark goes too.
    private func settlePendingApprovals(projectStore: ProjectStore) async {
        guard !approvalsPending.isEmpty, let info = projectStore.state.projectInfo else { return }
        let projectID = projectStore.projectID
        for slice in info.slices {
            guard let reworkSeen = approvalsPending[slice.id] else { continue }
            if slice.status == "Done" || !slice.pr.isEmpty {
                approvalsPending[slice.id] = nil
            } else if !slice.handedBack {
                approvalsPending[slice.id] = true
            } else if reworkSeen {
                approvalsPending[slice.id] = nil
                let client = clientFactory()
                let sliceID = slice.id
                await sliceActions.run(.approve, sliceID: sliceID, select: { _ in }) {
                    _ = try await client.sliceApprove(projectID: projectID, sliceRef: sliceID)
                    await projectStore.refresh()
                }
            }
        }
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

    /// Ends the project's planning agent outright — `nat agent-kill
    /// --workshop`, alongside `killAgent(sliceID:)` above. Unlike that one,
    /// this is asked for by hand: the rail's ✕ on the workshop row is the
    /// only caller, closing a tab a session is still live on.
    ///
    /// Answers with the refusal's own first line where nat refused, and nil
    /// once the session is gone.
    @discardableResult
    public func killWorkshopAgent() async -> String? {
        guard let projectID = activeProjectID else { return "No project loaded" }
        do {
            try await clientFactory().agentKillWorkshop(projectID: projectID)
        } catch let error as NatError {
            if case .commandFailed(let message) = error { return message }
            return error.localizedDescription
        } catch {
            return error.localizedDescription
        }
        activityStore?.kick()
        return nil
    }

    /// The workshop row's ✕: with no live planning agent this is the whole
    /// of it — the draft is discarded and the row deselected, and
    /// `buildWorkshopEntry` then draws nothing at all. With one live, the
    /// caller is expected to have confirmed first (the app's usual pattern —
    /// this method does not ask), and the session is killed before the same
    /// draft-and-deselect happens; a refusal there is reported back rather
    /// than papered over; the row and draft are left exactly as they were so
    /// a session that would not die is not shown gone in the meantime.
    ///
    /// The deliberate asymmetry with `selectedSliceID`/`workshopSelected`
    /// deselecting alone: navigating *away* from the workshop keeps the
    /// draft (see `workshopDraft`), and this is the one explicit act that
    /// discards it.
    @discardableResult
    public func closeWorkshopTab() async -> String? {
        guard let projectID = activeProjectID else { return "No project loaded" }
        if planningAgent != nil {
            if let refusal = await killWorkshopAgent() { return refusal }
        }
        workshopDrafts[projectID] = nil
        workshopSelected = false
        return nil
    }

    // MARK: - Ad hoc sessions

    /// The New Session button: launch a bare Claude Code on the active
    /// project — `nat session-launch`. `dir` is where it runs, or nil for
    /// the project's own working directory. Busy while the launch is in
    /// flight, mirroring `launchWorkshop(request:)`'s own shape; unlike a
    /// workshop launch there is no live-reading to wait out afterwards — the
    /// session is minted before `session-launch` returns, so kicking the
    /// activity poll and refreshing the session list is enough for its row
    /// to appear.
    public func launchSession(dir: String? = nil) async {
        let onScratch = newSessionNeedsFolder
        guard let projectID = activeProjectID else { return }
        newSessionLaunching = true
        newSessionError = nil
        do {
            let agent = config?.sliceAgent
            let result = try await clientFactory().sessionLaunch(
                projectID: projectID, dir: dir, model: agent?.model, effort: agent?.effort
            )
            selectedSessionID = result.id
            if onScratch, let dir, !dir.isEmpty {
                lastSessionFolder = dir
            }
            await sessionStore?.update(projectID: projectID)
            activityStore?.kick()
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                newSessionError = message
            } else {
                newSessionError = error.localizedDescription
            }
        } catch {
            newSessionError = error.localizedDescription
        }
        newSessionLaunching = false
    }

    /// Read a session's branches and pull requests fresh — `nat
    /// session-status`, the PR tab's own reading. A plain read, unlike
    /// `endSession`/`discardSession`: it never ends the session (`discard`
    /// is always false), leaving whether the strongest fact — the merged/open
    /// state itself — has already ended it to the CLI's own next read.
    public func sessionStatus(projectID: String, sessionID: String) async throws -> SessionStatusDoc {
        try await clientFactory().sessionStatus(projectID: projectID, sessionID: sessionID, discard: false)
    }

    /// End an ad hoc session's agent outright — `nat agent-kill` on its own
    /// pane tag, the rail's own "End session" — and refresh the session list
    /// so the row settles into Needs review or DONE on its next reading
    /// rather than waiting for the poll.
    ///
    /// Answers with the refusal's own first line where nat refused, and nil
    /// once the session is gone.
    @discardableResult
    public func endSession(tag: String) async -> String? {
        guard let projectID = activeProjectID else { return "No project loaded" }
        do {
            try await clientFactory().agentKill(projectID: projectID, sliceRef: tag)
        } catch let error as NatError {
            if case .commandFailed(let message) = error { return message }
            return error.localizedDescription
        } catch {
            return error.localizedDescription
        }
        activityStore?.kick()
        await sessionStore?.update(projectID: projectID)
        return nil
    }

    /// Discard an ad hoc session — `nat session-status --discard`, the
    /// rail's confirmed "Discard": ends the session (and removes its
    /// worktree) even with a pull request still unmerged, so long as none is
    /// still open. A session still live, or with a pull request still open,
    /// is refused by the CLI, which passes straight through.
    ///
    /// Answers with the refusal's own first line where nat refused, and nil
    /// once discarded.
    @discardableResult
    public func discardSession(id: String) async -> String? {
        guard let projectID = activeProjectID else { return "No project loaded" }
        do {
            _ = try await clientFactory().sessionStatus(projectID: projectID, sessionID: id, discard: true)
        } catch let error as NatError {
            if case .commandFailed(let message) = error { return message }
            return error.localizedDescription
        } catch {
            return error.localizedDescription
        }
        if selectedSessionID == id {
            selectedSessionID = nil
        }
        await sessionStore?.update(projectID: projectID)
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
        // Ad hoc sessions ride the same cadence: every plan reload and the
        // poll's own tick, alongside the PR-readiness reading above.
        await sessionStore?.update(projectID: projectID)
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
        sessionStore = nil
        sliceDetailStores = [:]
        diffStores = [:]
        prStores = [:]
        sessionDiffStores = [:]
        pickerMemory = PickerSelectionMemory()
    }
}
