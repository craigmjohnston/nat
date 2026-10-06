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

    /// The tab each project's workshop pane has up, as last picked — see
    /// `workshopTab`. Set to Terminal by a launch and to Plan by a
    /// proposal's first arrival.
    private var workshopTabPicks: [String: WorkshopTab] = [:]

    /// The Plan tab's last ask to scroll to a proposed slice's box.
    public private(set) var workshopPlanScroll: WorkshopPlanScroll?

    /// The edits in the Plan section whose new brief is unfolded, by the
    /// edited task's title.
    public var expandedProposalEdits: Set<String> = []

    /// The Plan tab's boxes folded to their header, by proposed slice
    /// (`PlanProposal.sliceID`). Every box starts open.
    public var foldedProposedSlices: Set<String> = []

    /// The projects whose workshop has been opened and not yet launched or
    /// dismissed — each holding a "Workshop" row in Active while the
    /// user is elsewhere, so clicking away loses neither the row nor the
    /// draft under it. Per-project, beside `workshopSelectedProjects`; a
    /// launch that takes hands the row to the live agent, and the row's ✕
    /// (`closeWorkshopTab`) is the other way it goes. Kept across launches,
    /// with the three below, by `workshopCache`.
    public private(set) var workshopPinnedProjects: Set<String> = [] {
        didSet { workshopsChanged() }
    }

    /// The request each project's workshop was launched on, as it was sent —
    /// what the Brief section shows, read-only, once the draft it came from
    /// has been cleared by the launch — and, kept across launches, what it
    /// goes on showing for a session still running after a relaunch. An
    /// agent no run of this app launched has no request here to show.
    private var workshopRequests: [String: String] = [:] {
        didSet { workshopsChanged() }
    }

    /// The tabs whose workshop was running when the app last quit — restored
    /// with a request (`restoreWorkshops`), which a launch sets and a close
    /// clears — drawn as launched, "Reconnecting…", until the activity poll's
    /// first reading says whether the agent is still there
    /// (`settleReconnectingWorkshops`). Nothing of it is written anywhere:
    /// the kept request is the whole signal.
    public private(set) var reconnectingWorkshops: Set<String> = []

    /// The tabs whose workshop has had its plan accepted, its session still
    /// running — kept across launches, so an agent found gone after that is
    /// known to have left nothing unsaved (`EndedWorkshop.trash`). A fresh
    /// launch, and anything that closes the workshop, takes it off.
    private var workshopAcceptedTabs: Set<String> = [] {
        didSet { workshopsChanged() }
    }

    /// The tabs the last activity reading held a planning agent for: what an
    /// agent gone by the next reading is told from one never seen.
    @ObservationIgnored private var planningTabsLastRead: Set<String> = []

    /// The composer's typed-but-not-yet-launched request, per project — kept
    /// here rather than as `WorkshopPaneView`'s own `@State` so switching to a
    /// slice and back does not tear the composer down with it (`PaneView`
    /// mounts the workshop pane in a plain conditional). Kept through a
    /// launch, so an agent that ends before proposing hands the composer back
    /// with it (`EndedWorkshop.restoreBrief`); cleared only by closing or
    /// dismissing the workshop, or by its session ending after an accepted
    /// plan — never by navigating away — and kept across launches, saved as
    /// it is typed (debounced, `workshopsChanged`) and flushed on quit.
    private var workshopDrafts: [String: String] = [:] {
        didSet { workshopsChanged() }
    }

    /// The plan document chosen or dropped on an Untitled tab's starter card,
    /// per tab, held with the draft and cleared as it is.
    private var workshopPlanFiles: [String: PlanFile] = [:] {
        didSet { workshopsChanged() }
    }

    /// Each tab's workshop proposal and its Accept, by tab — an Untitled
    /// tab's read by its workspace, a project's by the project. See
    /// `ProposalState` for the rules that keep readings and an Accept from
    /// racing each other.
    public private(set) var proposalStates: [String: ProposalState] = [:]

    /// The plan each tab's workshop has proposed, by tab — what the
    /// workshop's Plan section draws. Replaced in place by a revised one.
    public var proposals: [String: PlanProposal] {
        proposalStates.compactMapValues(\.proposal)
    }

    /// The project name the user has typed over the agent's suggestion, per
    /// tab: absent while the field still shows the suggestion, so a revised
    /// proposal's own name follows it until the user has said otherwise.
    private var proposalNameEdits: [String: String] = [:]

    /// True while the tab on screen's Accept is under way, and what its last
    /// one refused with — drawn at the name field.
    public var proposalAccepting: Bool {
        activeProjectID.flatMap { proposalStates[$0]?.accepting } ?? false
    }
    public var proposalError: String? {
        activeProjectID.flatMap { proposalStates[$0]?.error }
    }

    /// What each accepted plan went in as, by project: the pane's "Plan
    /// accepted" state, shown until something is selected.
    public private(set) var acceptedPlans: [String: PlanAccepted] = [:]

    /// The projects still owed the "Mirror this plan to Notion?" card, read
    /// off `MirrorNudgeMemory` at launch and kept in step with it: armed by an
    /// accepted plan, disarmed by the card's ✕ and by the project mirroring.
    public private(set) var mirrorNudgePending: Set<String>

    /// Whether the picker sheet the card's "Choose page…" opens is up.
    public var mirrorPickerPresented = false

    @ObservationIgnored private let mirrorNudgeMemory: MirrorNudgeMemory
    /// What has been seen of each slice's Changes, Visual changes and PR —
    /// their New and Updated badges; see `SeenMemory`.
    @ObservationIgnored private let seenMemory: SeenMemory
    /// Which projects' tabs the user closed — see `ClosedTabMemory`.
    @ObservationIgnored private let closedTabMemory: ClosedTabMemory

    /// Bumped by "Keep workshopping"; the workshop terminal takes keyboard
    /// focus on each change.
    public private(set) var terminalFocusRequest = 0

    /// Where the nudge marker is, once `start` has been told: what the
    /// proposal watch polls. Nil before then, and no watch is started.
    @ObservationIgnored private var nudgePath: String?
    @ObservationIgnored private var proposalWatcher: NudgeWatcher?

    /// Ordered list of project tabs: (id, name). The Untitled ones among
    /// them are kept across launches (`workshopCache`).
    public private(set) var projectTabs: [(id: String, name: String)] = [] {
        didSet { workshopsChanged() }
    }

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

    /// How many Untitled tabs have been opened, so each gets an ID of its
    /// own: more than one may exist, and a closed one's ID is not reused.
    /// Restarts past the highest one restored from `workshopCache`.
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
        workspaceIDs[id] = newWorkspaceID(id)
        activeProjectID = id
        startProposalWatch()
        return id
    }

    /// Each Untitled tab's workspace id, minted with the tab: what a planning
    /// agent launched from it is keyed by, where a project's is keyed by the
    /// project, and what its `plan-propose` carries so a proposal routes back
    /// here. A fresh UUID rather than the tab's own `untitled-N`, which a
    /// later run may reuse once the tab is closed — a stale proposal file or
    /// session of a tab long gone must never be mistaken for this one's. An
    /// open tab's is kept across launches with the tab (`workshopCache`), so
    /// its planning session is found again under it.
    @ObservationIgnored private var workspaceIDs: [String: String] = [:] {
        didSet { workshopsChanged() }
    }

    /// A tab's workspace id — nil for a tab that is a project.
    public func workspaceID(forTab tabID: String) -> String? {
        workspaceIDs[tabID]
    }

    /// Whether an Untitled tab has a planning agent live: what closing it
    /// must ask about first, since the session does not outlive its tab.
    public func tabHasLiveWorkshop(_ tabID: String) -> Bool {
        guard let workspace = workspaceIDs[tabID] else { return false }
        return activityStore?.agents[TmuxSession.planTag(projectID: workspace)] != nil
    }

    /// Whether the Untitled tab on screen is showing its planning session —
    /// running, or launching — where it would otherwise show the starter
    /// card. The pane, and the rail's TODO explainer, both read it.
    public var untitledWorkshopVisible: Bool {
        activeTabIsUntitled && (planningAgent != nil || workshopLaunching || workshopReconnecting || workshopEnded)
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

    /// Every open project's `nat pr-status` reading, held per project — the
    /// sidebar's marks in every project, the PR section's notices and each
    /// tab's attention all read it.
    public private(set) var prStatusStore: PRStatusStore?

    /// The app's one read of GitHub: every open project in one `nat
    /// pr-status` on the poll tick, and a settle read after an action — see
    /// `GitHubReadingStore`. It feeds `prStatusStore`, the visible PR tab and
    /// the session rows; nothing else in the app reads GitHub on a timer.
    public private(set) var githubReadingStore: GitHubReadingStore?

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

    /// The runs started from this app whose sessions are still live, keyed
    /// by what they run for — a slice's ID, or `runKey(projectID:sliceID:)`
    /// of the project for a global run. A re-run replaces its key's entry;
    /// a session that ends drops it (`watchRun`).
    public private(set) var runs: [String: RunAttachment] = [:]

    /// The runs being started — the `nat run` in flight — by key, each with
    /// the label asked for (nat's default resolved to its scope's first run,
    /// "" where the scope lists none), so a run in flight is told from its
    /// siblings.
    public private(set) var runsStarting: [String: String] = [:]

    /// What the last `nat run` refused with, until dismissed or the next run.
    public private(set) var runError: String?

    /// Whether a run's tmux session is still there — tmux itself in the app,
    /// injectable so a test says when a run ends.
    @ObservationIgnored
    public var runSessionExists: @Sendable (String) async -> Bool = { await TmuxSession.exists($0) }

    /// How long `watchRun` waits between askings.
    @ObservationIgnored
    public var runWatchInterval: UInt64 = 2_000_000_000

    /// Whether the app has anywhere to show the board at all: no config file
    /// was found, or one was found naming no projects. The window shows a
    /// welcome pane in its place, which offers the same two ways onto the
    /// board the "+" tab does — `addProject(id:name:)` is where both of them
    /// end — and a "Check again" that re-runs `start()` for a workspace set
    /// up elsewhere in the meantime.
    public private(set) var needsOnboarding: Bool = true

    /// Per-project selected slice IDs.
    private var selectedSliceIDs: [String: String?] = [:]

    /// Per-project selected source containers, by container id — the third
    /// kind of selection beside slices and sessions (and the workshop),
    /// mutually exclusive with all of them.
    private var selectedContainerIDs: [String: String?] = [:]

    /// One container-detail cache per source project (lazily created), as
    /// `sliceDetailStores` is per project.
    private var containerStores: [String: ContainerStore] = [:]

    /// Each source project's lazy groups the user has opened, passed on every
    /// plan read of it (`info --expand`).
    public private(set) var sourceExpanded: [String: Set<String>] = [:]

    /// Every task-source plugin this machine has (`nat source-list`), read
    /// once at startup — what the `+` menu offers a new source project for.
    /// Empty where the read failed: a missing plugin is no reason to say so.
    public private(set) var sourcePlugins: [SourcePlugin] = []
    private var sourcePluginsLoaded = false
    /// The plugins a source project has been made for in this run, or is
    /// being made for now — so two readings landing together, or one landing
    /// before the config re-read shows the new project, make it once.
    private var sourceProjectsMade: Set<String> = []
    /// Whether a reading of the plugins, or of config, may go on to make a
    /// connected plugin's project. Only the app itself turns it on: a test
    /// drives this model over whatever `nat` the machine has, and must never
    /// write a project into its real config.
    private let makesSourceProjects: Bool
    /// Whether reading config may go on to ask nat for a colour for each
    /// project with none (`assignProjectColors`). Only the app turns it on,
    /// for `makesSourceProjects`' reason: a test must never write the
    /// machine's real config.
    private let assignsProjectColors: Bool
    /// The projects a colour has been asked for in this run, so two readings
    /// of config ask once — a second `auto` would choose afresh — and one nat
    /// refused waits for the next launch.
    private var colorsAsked: Set<String> = []

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

    /// One visual-changes store per project (lazily created), for the same
    /// reason — the Visual changes section and pane read through this.
    private var visualStores: [String: VisualStore] = [:]

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

    /// The Follow-ups sidebar's per-slice choices and the apply in flight —
    /// see `FollowUpStore`.
    public let followUpStore = FollowUpStore()

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

    /// Where the workshops are kept between launches — see
    /// `WorkshopSnapshot`. In memory unless the app says otherwise, so a
    /// test or a story never touches the real Application Support file.
    @ObservationIgnored private let workshopCache: WorkshopCaching

    /// How long a change to the workshops waits before it is written, so a
    /// brief being typed is written once it pauses rather than per key.
    /// Injectable so a test never waits.
    @ObservationIgnored private let workshopSaveWait: @MainActor @Sendable () async -> Void

    /// Whether `workshopCache` has been read this run. Nothing is written
    /// until it has: a write before would replace what is there with nothing.
    @ObservationIgnored private var workshopsRestored = false

    /// The write `workshopsChanged` has waiting, replaced by each change.
    @ObservationIgnored private var workshopSaveTask: Task<Void, Never>?

    private let pollInterval: UInt64 // in seconds
    /// Whether the GitHub reading runs on the poll tick — true in the app;
    /// false, as in tests, for readings taken only when asked for.
    private let readsGitHubOnATick: Bool
    /// How long a settle read waits after the action that asked for it.
    private let githubSettleDelay: Duration
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

    /// Mints an Untitled tab's workspace id, given the tab's own id: a fresh
    /// UUID unless a fixture pins one, so a story drawn twice names its
    /// planning session the same way twice.
    private let newWorkspaceID: @Sendable (String) -> String

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
        readsGitHubOnATick: Bool = false,
        githubSettleDelay: Duration = .seconds(5),
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
        newWorkspaceID: @escaping @Sendable (String) -> String = { _ in UUID().uuidString.lowercased() },
        visitHold: TimeInterval = agentVisitHold,
        toolsReady: @escaping @Sendable () -> Bool = {
            ["nat", "tmux", "gh", "ntn"].allSatisfy { BinaryLocator.status(of: $0).isFound }
        },
        mirrorNudgeMemory: MirrorNudgeMemory = .inMemory(),
        seenMemory: SeenMemory = .inMemory(),
        closedTabMemory: ClosedTabMemory = .inMemory(),
        workshopCache: WorkshopCaching = InMemoryWorkshopCache(),
        workshopSaveWait: @escaping @MainActor @Sendable () async -> Void = {
            try? await Task.sleep(nanoseconds: 500_000_000)
        },
        makesSourceProjects: Bool = false,
        assignsProjectColors: Bool = false
    ) {
        self.assignsProjectColors = assignsProjectColors
        self.workshopCache = workshopCache
        self.workshopSaveWait = workshopSaveWait
        self.makesSourceProjects = makesSourceProjects
        self.mirrorNudgeMemory = mirrorNudgeMemory
        self.seenMemory = seenMemory
        self.closedTabMemory = closedTabMemory
        self.mirrorNudgePending = mirrorNudgeMemory.pending
        self.toolsReady = toolsReady
        self.configReader = configReader
        self.planCache = planCache
        self.pollInterval = pollIntervalSeconds
        self.readsGitHubOnATick = readsGitHubOnATick
        self.githubSettleDelay = githubSettleDelay
        self.pathsProvider = pathsProvider
        self.clientFactory = clientFactory
        self.workshopLauncher = workshopLauncher
        self.activityStoreFactory = activityStoreFactory
        self.usageStoreFactory = usageStoreFactory
        self.launchSettleWait = launchSettleWait
        self.now = now
        self.newWorkspaceID = newWorkspaceID
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

    /// Reads the task-source plugins, once: `sourcePlugins` stays empty on a
    /// failed read. The source projects' tabs take their plugins' titles.
    public func loadSourcePlugins() async {
        guard !sourcePluginsLoaded else { return }
        sourcePluginsLoaded = true
        sourcePlugins = (try? await clientFactory().sourceList()) ?? []
        projectTabs = projectTabs.map { tab in
            config?.projects[tab.id]?.backend == .source ? (id: tab.id, name: tabName(tab.id, fallback: tab.name)) : tab
        }
        if makesSourceProjects { await ensureSourceProjects() }
    }

    /// What a project is called wherever gnat names it — its tab, its
    /// sidebar row, the breadcrumb: a source project's plugin's title
    /// (`sourcePlugins`' `displayTitle`, else the plugin's name), never the
    /// `name` its config entry may carry; any other project's config name.
    /// `fallback` where config has no such project.
    public func tabName(_ id: String, fallback: String = "") -> String {
        guard let entry = config?.projects[id] else { return fallback }
        if entry.backend == .source, let plugin = entry.source {
            return sourcePlugins.first { $0.name == plugin }?.displayTitle ?? plugin
        }
        return entry.name
    }

    /// Connecting a plugin makes its section: every plugin whose `describe`
    /// says it is connected (`SourceDescribe.isConnected` — for Shortcut, the
    /// token set) gets exactly one source project, made here the first time
    /// with the plugin's title for its name and no working directory — each
    /// of its tasks works out its own repository — and taken into the
    /// sidebar without being opened. A plugin with one already gets nothing,
    /// and so does every plugin before config has been read, since config is
    /// where the existing ones are found. A refusal concludes nothing: the
    /// next reading of the plugins tries again.
    public func ensureSourceProjects() async {
        guard let config else { return }
        let have = Set(config.projects.values.compactMap(\.source))
        for plugin in sourcePlugins {
            guard let describe = plugin.describe, describe.isConnected, !have.contains(plugin.name),
                  !sourceProjectsMade.contains(plugin.name) else { continue }
            sourceProjectsMade.insert(plugin.name)
            do {
                let created = try await clientFactory().projectCreate(
                    name: plugin.displayTitle, repo: nil, description: nil, source: plugin.name)
                await reloadConfig()
                await assignProjectColors()
                if !projectTabs.contains(where: { $0.id == created.id }) {
                    projectTabs.append((id: created.id, name: tabName(created.id, fallback: created.name)))
                    closedTabMemory.reopen(created.id)
                }
                loadBackgroundProject(created.id)
            } catch {
                sourceProjectsMade.remove(plugin.name)
                NSLog("AppModel: could not make the %@ source project: %@", plugin.name, error.localizedDescription)
            }
        }
    }

    /// Reads them again, after Settings ▸ Sources installed or took one away.
    public func reloadSourcePlugins() async {
        sourcePluginsLoaded = false
        await loadSourcePlugins()
    }

    /// The names of the projects a plugin is the source of, as config has
    /// them — what Settings ▸ Sources names before an uninstall deletes them.
    public func sourceProjectNames(of plugin: String) -> [String] {
        (config?.projects ?? [:]).filter { $0.value.source == plugin }.map { tabName($0.key) }.sorted()
    }

    /// Settings ▸ Sources installed, updated, uninstalled or set up a plugin.
    /// The projects nat deleted with it lose their tabs and stores first (and
    /// the plugin may make a fresh one if it is installed again); then the
    /// plugins are read again, and every source project of the plugin re-reads
    /// its plan — `info` asks the plugin for its tree every time, so the
    /// sidebar, menus and filter fields are the new binary's at once rather
    /// than at the next poll. A Notion or local project has no plugin to ask
    /// and is left alone.
    public func pluginChanged(_ change: PluginsModel.PluginChange) async {
        if !change.deletedProjectIDs.isEmpty {
            await projectsDeleted(change.deletedProjectIDs)
            sourceProjectsMade.remove(change.plugin)
        }
        await reloadSourcePlugins()
        // `.replica` (rereadSource's) is enough: `nat info` describes the
        // plugin and reads its sidebar on every read, `--refresh` or not —
        // that flag only pulls a Notion replica.
        let ids = (config?.projects ?? [:])
            .filter { $0.value.source == change.plugin && !change.deletedProjectIDs.contains($0.key) }
            .map(\.key).sorted()
        for id in ids {
            await rereadSource(projectID: id)
        }
    }

    /// Projects nat deleted: config re-read, and each one's tab, store and
    /// per-project state dropped, as a mirrored project's old ID is. The
    /// active project moves to the tab beside where it was — the scratch tab
    /// included, since it is a project like any other — and with no tab
    /// left, the board stops reading one.
    private func projectsDeleted(_ ids: [String]) async {
        await reloadConfig()
        let gone = Set(ids)
        let activeIndex = projectTabs.firstIndex { $0.id == activeProjectID }
        for id in ids { forgetProjectState(id) }
        projectTabs.removeAll { gone.contains($0.id) }
        guard let active = activeProjectID, gone.contains(active) else { return }
        if projectTabs.isEmpty {
            activeProjectID = nil
            pollTask?.cancel()
            pollTask = nil
            nudgeWatcher?.stop()
            nudgeWatcher = nil
        } else {
            await activateProject(projectTabs[min(activeIndex ?? 0, projectTabs.count - 1)].id)
        }
    }

    /// Drops what the app holds for one project that is no longer this ID:
    /// its store and every per-project reading keyed by it.
    private func forgetProjectState(_ id: String) {
        stores[id] = nil
        prStatusStore?.forget(projectID: id)
        containerStores[id] = nil
        sourceExpanded[id] = nil
        selectedSliceIDs[id] = nil
        selectedSessionIDs[id] = nil
        selectedContainerIDs[id] = nil
        workshopSelectedProjects.remove(id)
        workshopPinnedProjects.remove(id)
        workshopDrafts[id] = nil
        acceptedPlans[id] = nil
    }

    /// Start the app: load config, create project store, start timers.
    ///
    /// No config file at all, or one naming no projects, leaves
    /// `needsOnboarding` true and does nothing else here: there is no board
    /// to show and no project to activate until one is opened or created,
    /// which comes back through `addProject(id:name:)`.
    public func start(configPath: String, nudgePath: String) async {
        self.nudgePath = nudgePath
        startProposalWatch()
        // Off the startup path: `source-list` describes every plugin, and a
        // slow one is no reason to hold the board.
        if !sourcePluginsLoaded { Task { await loadSourcePlugins() } }
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
                    restoreWorkshops()
                    startWithUntitledTab()
                    if makesSourceProjects { await ensureSourceProjects() }
                } else {
                    needsOnboarding = true
                }
                return
            }
            needsOnboarding = false

            // Build project tabs from config, sorted by project ID, with the
            // scratch project (when config names one it also tracks) pinned
            // ahead of that sort, and the projects whose tabs the user closed
            // left out — unless that would leave no tab but scratch's, when
            // the closes are ignored for this launch rather than opening on
            // an empty board. A close of a project config no longer names is
            // forgotten here.
            self.scratchProjectID = loadedConfig.scratchProject.flatMap {
                loadedConfig.projects[$0] == nil ? nil : $0
            }
            closedTabMemory.prune(keeping: Set(loadedConfig.projects.keys))
            let closed = closedTabMemory.closed
            var sortedProjects = loadedConfig.projects.sorted { $0.key < $1.key }
            let open = sortedProjects.filter { $0.key == scratchProjectID || !closed.contains($0.key) }
            if open.contains(where: { $0.key != scratchProjectID }) {
                sortedProjects = open
            }
            if let scratch = scratchProjectID, let at = sortedProjects.firstIndex(where: { $0.key == scratch }) {
                sortedProjects.insert(sortedProjects.remove(at: at), at: 0)
            }
            self.projectTabs = sortedProjects.map { (id: $0.key, name: tabName($0.key, fallback: $0.value.name)) }
            restoreWorkshops()

            // Create activity store (app-wide), and start its poll at once:
            // its first `nat status` then runs beside the plan read, review
            // stats and reap below rather than after them, and that reading
            // is what a workshop restored as reconnecting waits on.
            let activityStore = makeActivityStore()
            self.activityStore = activityStore
            activityStore.kick()
            self.reviewStatsStore = ReviewStatsStore(client: clientFactory())
            self.prStatusStore = PRStatusStore(cache: planCache)
            self.sessionStore = SessionStore(client: clientFactory())
            startGitHubReading()
            startUsageStore()

            // Activate the first project (if any): the first real one, since
            // the scratch tab is somewhere to go rather than where to start —
            // unless it is all there is.
            let firstProjectID = sortedProjects.first { $0.key != scratchProjectID }?.key
                ?? sortedProjects.first?.key

            // Every other tab needs a loaded plan too — the sidebar draws
            // every project's tree, and a live agent on a project the user
            // has never clicked into still shows attention on its tab
            // (`attention(projectID:)` has nothing to attribute without one).
            // Each is loaded from its own cache and then refreshed in the
            // background, and started *before* the first project's
            // activation is awaited: that runs its read, its review stats and
            // a reaping sweep, and a tree waiting behind all of it is a tree
            // that unfolds onto "loading…" when its cache was on disk all along.
            for tab in projectTabs where tab.id != firstProjectID {
                loadBackgroundProject(tab.id)
            }
            if let firstProjectID {
                await activateProject(firstProjectID, nudgePath: nudgePath, config: loadedConfig)
            }
            // The first GitHub reading, off the startup path: the marks the
            // cache put up stand meanwhile. A background plan landing after
            // it asks for a settle read of its own (`loadBackgroundProject`).
            githubReadingStore?.readSoon()
            // The plugins' reading may have landed before config did, with
            // nowhere then to look for their projects.
            if makesSourceProjects { await ensureSourceProjects() }
            await assignProjectColors()
        } catch {
            // No config file to read from is the common case here, not a
            // crash-worthy one: it is exactly what a first run looks like.
            needsOnboarding = true
            NSLog("Failed to load config: %@", error.localizedDescription)
        }
    }

    /// The app-wide activity store, each reading wired to settle the
    /// workshops restored as reconnecting and those whose agent has ended.
    private func makeActivityStore() -> ActivityStore {
        let store = activityStoreFactory()
        store.onReading = { [weak self] in self?.planningAgentsRead() }
        return store
    }

    /// The board for a launch with no projects: the app-wide stores a first
    /// project would need (`addProject` finds them made), and one Untitled
    /// tab — not another when `start()` runs again with one already open.
    private func startWithUntitledTab() {
        needsOnboarding = false
        if activityStore == nil {
            activityStore = makeActivityStore()
            activityStore?.kick()
            reviewStatsStore = ReviewStatsStore(client: clientFactory())
            prStatusStore = PRStatusStore(cache: planCache)
            sessionStore = SessionStore(client: clientFactory())
            startGitHubReading()
        }
        if usageStore == nil {
            startUsageStore()
        }
        if let restored = projectTabs.first(where: { isUntitledTab($0.id) }) {
            if activeProjectID == nil { activeProjectID = restored.id }
        } else {
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

    /// gnat never picks a colour: every config project whose entry has none
    /// is given one by nat — one `config-set project.<id>.color auto` each,
    /// one at a time — and config is read again, which puts up their pucks.
    /// Each project is asked once a run; a refusal is logged and leaves it
    /// with no puck until the next launch. Off unless `assignsProjectColors`.
    public func assignProjectColors() async {
        guard assignsProjectColors, let config else { return }
        let bare = config.projects.filter { $0.value.color == nil && !colorsAsked.contains($0.key) }.map(\.key).sorted()
        guard !bare.isEmpty else { return }
        colorsAsked.formUnion(bare)
        let client = clientFactory()
        for id in bare {
            do {
                try await client.configSet(key: SettingsModel.colorKey(projectID: id), value: "auto")
            } catch {
                NSLog("AppModel: could not colour project %@: %@", id, error.localizedDescription)
            }
        }
        await reloadConfig()
    }

    /// Activate a project by ID, creating and loading its store lazily.
    public func activateProject(_ projectID: String, nudgePath: String, config: NatProjectConfig) async {
        activeProjectID = projectID

        // Create or retrieve the project store
        if stores[projectID] == nil {
            stores[projectID] = ProjectStore(projectID: projectID, client: clientFactory(), cache: planCache)
        }

        guard let projectStore = stores[projectID] else { return }

        // The last pull request reading first, so its marks are up before
        // the plan's fresh read and the reading's own.
        await prStatusStore?.restore(projectID: projectID)
        // Load the project store
        await projectStore.load()
        await updateReviewStats(projectID: projectID, projectStore: projectStore)
        // Its sessions: `session-list` asks GitHub nothing — each row's pull
        // requests are the ones the last GitHub reading kept.
        await sessionStore?.update(projectID: projectID)
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
    /// one slow project's read never holds up the rest of startup. Its pull
    /// requests' last reading is put up first, and once the plan has landed
    /// a settle read is asked for, where it has pull request work: every
    /// project loading at launch folds into that one reading.
    private func loadBackgroundProject(_ projectID: String) {
        if stores[projectID] == nil {
            stores[projectID] = ProjectStore(projectID: projectID, client: clientFactory(), cache: planCache)
        }
        guard let store = stores[projectID] else { return }
        Task {
            await prStatusStore?.restore(projectID: projectID)
            await store.load()
            if hasPullRequestWork(projectID: projectID) { githubReadingStore?.scheduleSettle(afterAction: false) }
        }
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

    /// Close a project's tab, and keep it closed: once the tab has gone the
    /// project is remembered in `ClosedTabMemory`, so later launches leave it
    /// out of the strip until the user opens it again from the "+" tab. The
    /// config entry is untouched — the project stays configured for every
    /// headless command and agent. A refused close records nothing, and an
    /// Untitled tab, which has no config entry to come back from, never is
    /// recorded. Closing the active tab activates its neighbour (the tab that
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
    ///
    /// An Untitled tab's planning session does not outlive it — nothing else
    /// could ever reach it again — so it is killed before the tab goes, and
    /// a session that will not die keeps the tab open and answers with why.
    /// Asking first is the caller's (`tabHasLiveWorkshop`), as the rail's ✕
    /// on the workshop row does.
    @discardableResult
    public func closeProject(_ projectID: String) async -> String? {
        guard projectID != scratchProjectID,
              closableTabCount > 1,
              let index = projectTabs.firstIndex(where: { $0.id == projectID }) else { return nil }
        if tabHasLiveWorkshop(projectID), let refusal = await killWorkshop(ofTab: projectID) {
            return refusal
        }
        forgetUntitledTab(projectID)
        workshopSelectedProjects.remove(projectID)
        workshopPinnedProjects.remove(projectID)
        let closing = Set((stores[projectID]?.state.projectInfo?.slices ?? []).map(\.id))
        await reapFinishedAgents(ignoringHoldsFor: closing)
        projectTabs.remove(at: index)
        // Its pull-request loop stops with the tab; reopening starts one.
        prStatusStore?.forget(projectID: projectID)
        if !isUntitledTab(projectID) { closedTabMemory.close(projectID) }
        if activeProjectID == projectID {
            let neighbour = projectTabs[min(index, projectTabs.count - 1)]
            await activateProject(neighbour.id)
        }
        return nil
    }

    /// The configured projects with no tab in the strip — every one the user
    /// closed — as the "+" tab's open picker offers them, in config order.
    /// Read off config itself rather than `nat project-list`, so a closed
    /// project can be opened again whatever the workspace listing says. The
    /// scratch project is never one: it is never closed.
    public var closedProjects: [ProjectListingEntry] {
        let open = Set(projectTabs.map(\.id))
        return (config?.projects ?? [:]).sorted { $0.key < $1.key }
            .filter { !open.contains($0.key) && $0.key != scratchProjectID }
            .map { ProjectListingEntry(
                id: $0.key, name: tabName($0.key, fallback: $0.value.name),
                configured: true, workingDir: $0.value.workingDir) }
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
            await assignProjectColors()
        }
        guard let config = config else { return }
        needsOnboarding = false

        // start() builds these for a config that named projects; a first
        // project on a machine that had none arrives here with neither.
        if activityStore == nil {
            activityStore = makeActivityStore()
            reviewStatsStore = ReviewStatsStore(client: clientFactory())
            prStatusStore = PRStatusStore(cache: planCache)
            sessionStore = SessionStore(client: clientFactory())
            startGitHubReading()
        }
        if usageStore == nil {
            startUsageStore()
        }

        let replaced = untitledID.flatMap { untitled in
            isUntitledTab(untitled) ? projectTabs.firstIndex(where: { $0.id == untitled }) : nil
        }
        if !projectTabs.contains(where: { $0.id == id }) {
            // The config's own name where it has one (a source project's
            // plugin's) — it is what every other tab is labelled with — and
            // what the command reported otherwise.
            let tab = (id: id, name: tabName(id, fallback: name))
            if let replaced {
                projectTabs[replaced] = tab
            } else {
                projectTabs.append(tab)
            }
        } else if let replaced {
            projectTabs.remove(at: replaced)
        }
        closedTabMemory.reopen(id)
        if let untitledID, replaced != nil { forgetUntitledTab(untitledID) }
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

    // MARK: - Every project at once (the sidebar)

    /// Each open project as the sidebar draws it, in `projectTabs` order: its
    /// plan as its own store last read it — every project's is loaded at
    /// start, not only the active one's — and where that read has got to.
    public var sidebarInputs: [SidebarProjectInput] {
        projectTabs.map { tab in
            let kind: SidebarProjectKind = isUntitledTab(tab.id)
                ? .untitled
                : (tab.id == scratchProjectID ? .scratch : .project)
            let state = stores[tab.id]?.state
            return SidebarProjectInput(
                id: tab.id, name: tab.name, kind: kind,
                plan: state?.projectInfo,
                isLoading: state?.isLoading ?? (kind != .untitled),
                errorMessage: state?.errorMessage,
                isSource: config?.projects[tab.id]?.backend == .source,
                color: projectColor(ofProject: tab.id))
        }
    }

    /// The colour a project's config entry holds — its puck's — nil for one
    /// nat has not coloured yet and for an Untitled tab, which has no entry.
    public func projectColor(ofProject projectID: String) -> ProjectColor? {
        config?.projects[projectID]?.color
    }

    /// One project's plan as its store last read it — nil before it lands.
    public func plan(projectID: String) -> ProjectInfo? {
        stores[projectID]?.state.projectInfo
    }

    /// The live planning agent of every open project that has one, keyed by
    /// project — each project's own scoped tag, and an Untitled tab's
    /// workspace tag under the tab's ID.
    public var planningAgents: [String: AgentActivity] {
        let agents = activityStore?.agents ?? [:]
        var found: [String: AgentActivity] = [:]
        for tab in projectTabs {
            let key = workspaceIDs[tab.id] ?? tab.id
            if let agent = agents[TmuxSession.planTag(projectID: key)] {
                found[tab.id] = AgentActivity(agent.activity)
            }
        }
        return found
    }

    /// The sidebar's model, over every open project.
    public var sidebarModel: SidebarModel {
        buildSidebarModel(
            projects: sidebarInputs,
            liveAgents: (activityStore?.agents ?? [:]).mapValues { AgentActivity($0.activity) },
            sessions: sessionStore?.sessions ?? [],
            sessionsProjectID: activeProjectID,
            planningAgents: planningAgents,
            pinnedWorkshops: workshopPinnedProjects,
            launchingWorkshop: workshopLaunching ? activeProjectID : nil,
            reconnectingWorkshops: reconnectingWorkshops,
            prMarks: prStatusStore?.marks ?? [:],
            proposedWorkshops: Set(proposals.keys))
    }

    /// What the navigator's titlebar names `selection` in the active project
    /// by — its Active row's tag, dot and liveness where it has one.
    public func titlebarIdentity(for selection: TitlebarSelection) -> TitlebarIdentity {
        NatKit.titlebarIdentity(
            for: selection, projectID: activeProjectID ?? "", active: sidebarModel.active,
            tags: sidebarTags(sidebarInputs))
    }

    // MARK: - Task sources

    /// A source project's fold as the sidebar draws it — nil for any other
    /// project, or one whose plan has not landed.
    public func source(ofProject projectID: String) -> SidebarSource? {
        sidebarModel.source(projectID: projectID)?.source
    }

    /// The container selected in the active project, as its fold draws it —
    /// nil with none selected or none drawn by that id.
    public var selectedContainer: SidebarContainer? {
        guard let id = selectedContainerID, let projectID = activeProjectID else { return nil }
        return source(ofProject: projectID)?.container(withID: id)
    }

    /// The title a container goes by: the plugin's own, where its tree lists
    /// it, else nat's cached milestone name, else its id.
    public func containerTitle(_ containerID: String, inProject projectID: String) -> String {
        if let row = source(ofProject: projectID)?.container(withID: containerID) { return row.title }
        return plan(projectID: projectID)?.milestones.first { $0.id == containerID }?.name ?? containerID
    }

    /// Select a source container, activating its project first.
    public func selectContainer(_ containerID: String, inProject projectID: String) async {
        await select(inProject: projectID) { $0.selectedContainerID = containerID }
    }

    /// The selected source container's id (per-project). Selecting one
    /// deselects the slice, the session and the workshop row — the sidebar
    /// draws exactly one selected row.
    public var selectedContainerID: String? {
        get {
            guard let activeID = activeProjectID else { return nil }
            return selectedContainerIDs[activeID] ?? nil
        }
        set {
            guard let activeID = activeProjectID else { return }
            selectedContainerIDs[activeID] = newValue
            if newValue != nil {
                selectedSliceIDs[activeID] = nil
                selectedSessionIDs[activeID] = nil
                workshopSelectedProjects.remove(activeID)
                workshopLaunchError = nil
            }
        }
    }

    /// The container-detail cache for one source project, created on first
    /// use — see `sliceDetailStore(projectID:)`.
    public func containerStore(projectID: String) -> ContainerStore {
        if let existing = containerStores[projectID] { return existing }
        let store = ContainerStore(projectID: projectID, client: clientFactory())
        containerStores[projectID] = store
        return store
    }

    /// Whether a source project's lazy group has been opened.
    public func isSourceGroupExpanded(_ groupID: String, inProject projectID: String) -> Bool {
        sourceExpanded[projectID]?.contains(groupID) ?? false
    }

    /// Opens or folds a source project's lazy group: the set goes on every
    /// later read of that plan, and the plan is read again now, so an opened
    /// group fills and a folded one stops being asked for.
    public func setSourceGroup(_ groupID: String, expanded: Bool, inProject projectID: String) async {
        var groups = sourceExpanded[projectID] ?? []
        if expanded { groups.insert(groupID) } else { groups.remove(groupID) }
        sourceExpanded[projectID] = groups
        guard let store = stores[projectID] else { return }
        store.expand = groups.sorted()
        await store.refresh(.replica)
    }

    /// Runs one of a source plugin's own actions — the source header's (no
    /// target), a group's or a container's — then reads the project's plan
    /// again (nat re-reads the plugin's tree after any action) and the
    /// container on screen. Answers with what went wrong, or nil.
    @discardableResult
    public func runSourceAction(
        projectID: String, action: SourceAction, group: String? = nil, container: String? = nil, input: String? = nil
    ) async -> String? {
        do {
            _ = try await clientFactory().sourceAction(
                projectID: projectID, action: action.id, group: group, container: container, input: input)
        } catch let error as NatError {
            if case .commandFailed(let message) = error { return message }
            return error.localizedDescription
        } catch {
            return error.localizedDescription
        }
        await rereadSource(projectID: projectID)
        if let container { await containerStore(projectID: projectID).fetch(containerID: container) }
        return nil
    }

    /// Reads a source project's plan — and so its plugin's tree — again: what
    /// an action ends on, and what an open filter editor asks for once while
    /// a field of it is still loading.
    public func rereadSource(projectID: String) async {
        if projectID == activeProjectID {
            await refresh(.replica)
        } else {
            await stores[projectID]?.refresh(.replica)
        }
    }

    /// Select a slice wherever it is filed: its project is made the active
    /// one first when it is not, which is what every per-project reading
    /// (detail, diff, pull request, sessions) is keyed by.
    public func selectSlice(_ sliceID: String, inProject projectID: String) async {
        await select(inProject: projectID) { $0.selectedSliceID = sliceID }
    }

    /// The slice whose Send back to agent a row menu asked for, until its
    /// navigator takes the request (`takeSendBackRequest`) and opens the
    /// editor, its note empty.
    public private(set) var sendBackRequest: String?

    /// A slice row menu's Send back to agent…: the slice selected, and its
    /// navigator asked to open the editor — set before the selection, so a
    /// navigator built for it finds the request waiting.
    public func requestSendBack(sliceID: String, inProject projectID: String) async {
        sendBackRequest = sliceID
        await selectSlice(sliceID, inProject: projectID)
    }

    /// Whether a send-back was asked for this slice — answered once, the
    /// request cleared as it is taken.
    public func takeSendBackRequest(sliceID: String) -> Bool {
        guard sendBackRequest == sliceID else { return false }
        sendBackRequest = nil
        return true
    }

    /// Select an ad hoc session of a project, activating it first.
    public func selectSession(_ sessionID: String, inProject projectID: String) async {
        await select(inProject: projectID) { $0.selectedSessionID = sessionID }
    }

    /// Select a project's workshop row, activating it first.
    public func selectWorkshop(inProject projectID: String) async {
        await select(inProject: projectID) { $0.workshopSelected = true }
        // A proposal written before the workshop was opened has no nudge
        // still to come.
        await refreshProposals()
    }

    /// Makes a selection in a project, switching to it first where it is not
    /// the active one. The switch and the selection both land at once, before
    /// the activation's reads: those take seconds, and a selection written
    /// only after them is a click that seems not to take — and, finishing
    /// after a later click has selected something else, one that overwrites
    /// that click with its own.
    private func select(inProject projectID: String, _ selection: (AppModel) -> Void) async {
        guard activeProjectID != projectID else {
            selection(self)
            return
        }
        if config != nil || isUntitledTab(projectID) { activeProjectID = projectID }
        selection(self)
        await activateProject(projectID)
    }

    /// Re-read every open project but the active one, in the background —
    /// the sidebar draws all of their plans, and a nudge or a poll tick is as
    /// much news for them as for the one on screen. A plan read only: the
    /// GitHub reading is `githubReadingStore`'s, on its own tick.
    private func refreshBackgroundProjects(_ read: PlanRead) {
        for tab in projectTabs where tab.id != activeProjectID && !isUntitledTab(tab.id) {
            guard let store = stores[tab.id] else { continue }
            Task { await store.refresh(read) }
        }
    }

    /// Whether a project's plan, as its store holds it, has anything for
    /// `pr-status` to read: a slice with a pull request, or a handed-back
    /// branch under review.
    private func hasPullRequestWork(projectID: String) -> Bool {
        guard let info = stores[projectID]?.state.projectInfo else { return false }
        return info.slices.contains(where: { !$0.pr.isEmpty || inReview($0) })
    }

    /// Makes and starts the one GitHub reading — on the poll tick where
    /// `readsGitHubOnATick`, else only when asked for.
    private func startGitHubReading() {
        let tick = readsGitHubOnATick ? Duration.seconds(Int64(config.map(pollSeconds) ?? pollInterval)) : nil
        let store = GitHubReadingStore(
            client: clientFactory(),
            request: { [weak self] in self?.githubReadingRequest() },
            deliver: { [weak self] reading in await self?.deliver(reading) },
            tick: tick, settleDelay: githubSettleDelay, now: now)
        githubReadingStore = store
        store.start()
    }

    /// What one GitHub reading asks: every open project tab with pull
    /// request work, the active one always (its sessions' branches ride the
    /// reading), in tab order — and the pull request on a visible PR tab in
    /// full, its project named too.
    private func githubReadingRequest() -> GitHubReadingStore.Request? {
        var projectIDs = projectTabs.map(\.id).filter {
            !isUntitledTab($0) && ($0 == activeProjectID || hasPullRequestWork(projectID: $0))
        }
        var detail: String?
        for (projectID, store) in prStores.sorted(by: { $0.key < $1.key }) {
            guard let url = store.detailURL else { continue }
            detail = url
            if !projectIDs.contains(projectID) { projectIDs.append(projectID) }
            break
        }
        guard !projectIDs.isEmpty else { return nil }
        return GitHubReadingStore.Request(projectIDs: projectIDs, detail: detail)
    }

    /// Hands a GitHub reading on: each open project's part to
    /// `prStatusStore`, the detail to the PR tab showing it, and the active
    /// project's session rows a fresh `session-list` — which reads the pull
    /// requests this reading kept, and asks GitHub nothing.
    private func deliver(_ reading: GitHubReading) async {
        let open = Set(projectTabs.map(\.id))
        for (projectID, doc) in reading.projects.sorted(by: { $0.key < $1.key }) where open.contains(projectID) {
            await prStatusStore?.apply(doc, projectID: projectID)
        }
        if let detail = reading.detail {
            for store in prStores.values { store.applyDetail(detail) }
        }
        if let active = activeProjectID, !isUntitledTab(active) {
            await sessionStore?.update(projectID: active)
        }
    }

    /// Asks for the GitHub reading's settle read, 5 seconds out — after the
    /// user asked for one. See `GitHubReadingStore`.
    public func scheduleGitHubReading() {
        githubReadingStore?.scheduleSettle()
    }

    /// An action that changed something on GitHub — approve, merge, comment,
    /// reviewers, re-run or cancel checks — has run: counted in gnat's spend
    /// this session, then the settle read after it.
    public func githubActionRan() {
        githubReadingStore?.actionRan()
    }

    /// The manual refresh: the plan read the nudge makes, and the GitHub
    /// reading's settle read.
    public func refreshByHand() async {
        await refresh()
        scheduleGitHubReading()
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
        let store = DiffStore(client: clientFactory(), seen: seenMemory)
        diffStores[projectID] = store
        return store
    }

    /// The visual-changes store for one project, created on first use — see
    /// `sliceDetailStore(projectID:)`.
    public func visualStore(projectID: String) -> VisualStore {
        if let existing = visualStores[projectID] { return existing }
        let store = VisualStore(client: clientFactory(), projectID: projectID, seen: seenMemory)
        visualStores[projectID] = store
        return store
    }

    /// The pull-request cache for one project, created on first use — see
    /// `sliceDetailStore(projectID:)`.
    public func prStore(projectID: String) -> PRStore {
        if let existing = prStores[projectID] { return existing }
        let store = PRStore(client: clientFactory(), seen: seenMemory, settle: { [weak self] in
            self?.githubActionRan()
        })
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
                selectedContainerIDs[activeID] = nil
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
                selectedContainerIDs[activeID] = nil
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
        // An Untitled tab's is keyed by its workspace, and takes nothing else:
        // the legacy bare session is a project's to attach.
        if let workspace = workspaceIDs[activeID] {
            let tag = TmuxSession.planTag(projectID: workspace)
            return agents[tag] != nil ? tag : nil
        }
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

    /// What the active project's workshop was launched on, as the Brief
    /// shows it once launched — nil where this run launched nothing there.
    public var workshopRequest: String? {
        activeProjectID.flatMap { workshopRequests[$0] }
    }

    /// Whether a project's workshop row is pinned to Active.
    public func isWorkshopPinned(_ projectID: String) -> Bool {
        workshopPinnedProjects.contains(projectID)
    }

    /// The ✕ on a pinned workshop row with nothing running: the row and its
    /// draft go, wherever the user is — it navigates nowhere. One with a live
    /// agent is `closeWorkshopTab`'s, after the caller has asked.
    public func dismissWorkshop(inProject projectID: String) {
        workshopPinnedProjects.remove(projectID)
        workshopDrafts[projectID] = nil
        workshopPlanFiles[projectID] = nil
        workshopRequests[projectID] = nil
        reconnectingWorkshops.remove(projectID)
        workshopAcceptedTabs.remove(projectID)
        workshopSelectedProjects.remove(projectID)
    }

    /// The plan document attached to the Untitled tab on screen, if any.
    public var workshopPlanFile: PlanFile? {
        activeProjectID.flatMap { workshopPlanFiles[$0] }
    }

    /// Attach a plan document to the Untitled tab on screen: picked with the
    /// starter's "Open plan from filesystem…" or dropped on the composer. Its
    /// content is handed to the planning agent with the launch, alongside the
    /// description. A file that is too large or not text is refused on the
    /// card's own error line and leaves whatever was attached as it was.
    public func attachPlanFile(_ url: URL) {
        guard let projectID = activeProjectID, isUntitledTab(projectID) else { return }
        do {
            workshopPlanFiles[projectID] = try PlanFile.read(url)
            workshopLaunchError = nil
        } catch let error as PlanFileError {
            workshopLaunchError = error.message
        } catch {
            workshopLaunchError = error.localizedDescription
        }
    }

    /// Take the attached plan document off the Untitled tab on screen.
    public func detachPlanFile() {
        guard let projectID = activeProjectID else { return }
        workshopPlanFiles[projectID] = nil
    }

    /// The starter's "From filesystem" tile: open the local plan a folder
    /// holds as a project, taking the Untitled tab over exactly as From Notion
    /// does. A folder with no plan is refused with what was looked for, on the
    /// card's error line, and the tab is left as it was.
    public func openPlanFolder(_ url: URL) async {
        guard let tabID = activeProjectID, isUntitledTab(tabID) else { return }
        workshopLaunchError = nil
        do {
            let entry = try await clientFactory().projectOpenFolder(path: url.path)
            await addProject(id: entry.id, name: entry.name, replacing: tabID)
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                workshopLaunchError = message
            } else {
                workshopLaunchError = error.localizedDescription
            }
        } catch {
            workshopLaunchError = error.localizedDescription
        }
    }

    // MARK: - Keeping workshops across launches

    /// The workshops as they stand, as `workshopCache` keeps them: every open
    /// Untitled tab with its workspace id, in strip order, and every tab with
    /// anything of a workshop to keep.
    var workshopSnapshot: WorkshopSnapshot {
        let untitled = projectTabs.compactMap { tab in
            workspaceIDs[tab.id].map { WorkshopSnapshot.UntitledTab(id: tab.id, workspaceID: $0) }
        }
        let ids = workshopPinnedProjects.union(workshopDrafts.keys)
            .union(workshopPlanFiles.keys).union(workshopRequests.keys).union(workshopAcceptedTabs)
        var workshops: [String: WorkshopSnapshot.Workshop] = [:]
        for id in ids {
            workshops[id] = WorkshopSnapshot.Workshop(
                pinned: workshopPinnedProjects.contains(id), draft: workshopDrafts[id],
                planFile: workshopPlanFiles[id], request: workshopRequests[id],
                accepted: workshopAcceptedTabs.contains(id) ? true : nil
            )
        }
        return WorkshopSnapshot(untitledTabs: untitled, workshops: workshops)
    }

    /// Bring back what the last run kept, once per run, as config is read:
    /// its Untitled tabs (at the end of the strip, under the workspace ids
    /// their planning sessions are keyed by) and every workshop of a tab that
    /// is here — a project config no longer names is dropped. A missing or
    /// unreadable file brings back nothing. From here on, changes are written.
    private func restoreWorkshops() {
        guard !workshopsRestored else { return }
        defer { workshopsRestored = true }
        guard let snapshot = workshopCache.read() else { return }
        for tab in snapshot.untitledTabs
        where isUntitledTab(tab.id) && !projectTabs.contains(where: { $0.id == tab.id }) {
            projectTabs.append((id: tab.id, name: Self.untitledName))
            workspaceIDs[tab.id] = tab.workspaceID
            if let n = Int(tab.id.dropFirst(Self.untitledPrefix.count)) {
                untitledOpened = max(untitledOpened, n)
            }
        }
        for (id, workshop) in snapshot.workshops {
            let here = isUntitledTab(id) ? workspaceIDs[id] != nil : config?.projects[id] != nil
            guard here else { continue }
            if workshop.pinned { workshopPinnedProjects.insert(id) }
            workshopDrafts[id] = workshop.draft
            workshopPlanFiles[id] = workshop.planFile
            workshopRequests[id] = workshop.request
            if workshop.request != nil { reconnectingWorkshops.insert(id) }
            if workshop.accepted == true { workshopAcceptedTabs.insert(id) }
        }
    }

    /// An activity reading has landed. Every workshop drawn from the last
    /// run's request stops being provisional — one whose planning agent the
    /// reading holds is simply live — and every launched workshop whose
    /// agent is gone (never there this run, or there at the last reading)
    /// degrades as `workshopAgentEnded` decides. A workshop with no request
    /// was closed, or never launched from here, and has nothing to keep.
    private func planningAgentsRead() {
        let present = Set(projectTabs.map(\.id).filter { planningStatus(forTab: $0) != nil })
        let ended = reconnectingWorkshops.union(planningTabsLastRead).subtracting(present)
        reconnectingWorkshops = []
        planningTabsLastRead = present
        for tabID in ended where workshopRequests[tabID] != nil {
            // Pinned at once, so the row holds its place while the proposal
            // is read.
            workshopPinnedProjects.insert(tabID)
            Task { await workshopAgentEnded(tabID) }
        }
    }

    /// A launched workshop's planning agent has ended on its own: its
    /// proposal is read, and the workshop degrades so nothing unsaved is
    /// lost (`EndedWorkshop`) — a pending plan kept up, Plan in front, with
    /// the request; with nothing proposed, the composer, its brief still in
    /// it; after an accepted plan, nothing left, and the workshop goes.
    public func workshopAgentEnded(_ tabID: String) async {
        let read = await readProposal(tabID: tabID)
        // Relaunched or closed while the proposal was read: not this.
        guard planningStatus(forTab: tabID) == nil, workshopRequests[tabID] != nil else { return }
        switch EndedWorkshop.decide(
            proposalRead: read, hasProposal: proposalStates[tabID]?.proposal != nil,
            accepted: workshopAcceptedTabs.contains(tabID)
        ) {
        case .keepPlan:
            workshopPinnedProjects.insert(tabID)
            workshopTabPicks[tabID] = .plan
        case .restoreBrief:
            workshopPinnedProjects.insert(tabID)
            // A workshop launched before drafts were kept through a launch
            // has only its request: that goes back into the composer.
            if (workshopDrafts[tabID] ?? "").isEmpty { workshopDrafts[tabID] = workshopRequests[tabID] }
            workshopRequests[tabID] = nil
            workshopAcceptedTabs.remove(tabID)
        case .trash:
            workshopDrafts[tabID] = nil
            workshopPlanFiles[tabID] = nil
            workshopRequests[tabID] = nil
            workshopAcceptedTabs.remove(tabID)
            workshopPinnedProjects.remove(tabID)
            if tabID == activeProjectID { workshopSelected = false }
        case .keepAll:
            workshopPinnedProjects.insert(tabID)
        }
    }

    /// Whether the workshop on screen is drawn from the last run's request,
    /// waiting on the first activity reading to confirm its agent.
    public var workshopReconnecting: Bool {
        activeProjectID.map(reconnectingWorkshops.contains) ?? false
    }

    /// Whether the workshop on screen is one whose agent has ended with a
    /// plan still up and unaccepted (`EndedWorkshop.keepPlan`): drawn as
    /// launched, its terminal empty, Keep workshopping starting a new agent.
    public var workshopEnded: Bool {
        planningAgent == nil && !workshopLaunching && !workshopReconnecting
            && workshopRequest != nil && activeProposal != nil
    }

    /// A tab's planning agent as the activity poll last saw it: the active
    /// tab's as `planningAgent` finds it (the legacy bare session included),
    /// any other's by its own scoped tag — an Untitled tab's by its
    /// workspace.
    public func planningStatus(forTab tabID: String) -> AgentStatus? {
        if tabID == activeProjectID { return planningAgent }
        return activityStore?.agents[TmuxSession.planTag(projectID: workspaceIDs[tabID] ?? tabID)]
    }

    /// What ending a tab's workshop session must ask first, or nil where
    /// it can end at once — `WorkshopEndRules` over the tab's planning
    /// agent, proposal and Accept.
    public func workshopEndConfirmation(forTab tabID: String) -> String? {
        WorkshopEndRules.confirmation(
            activity: planningStatus(forTab: tabID)?.activity,
            hasProposal: proposalStates[tabID]?.proposal != nil,
            accepting: proposalStates[tabID]?.accepting ?? false)
    }

    /// Something kept across launches changed: write it once the changes
    /// pause (`workshopSaveWait`), each change putting the write back.
    private func workshopsChanged() {
        guard workshopsRestored else { return }
        workshopSaveTask?.cancel()
        workshopSaveTask = Task { [weak self] in
            guard let wait = self?.workshopSaveWait else { return }
            await wait()
            guard !Task.isCancelled else { return }
            self?.flushWorkshops()
        }
    }

    /// Write the workshops now, ahead of any write still waiting — what the
    /// app does as it quits, so the last keystrokes are not lost to the
    /// debounce. Nothing before the cache has been read this run.
    public func flushWorkshops() {
        guard workshopsRestored else { return }
        workshopSaveTask?.cancel()
        workshopSaveTask = nil
        workshopCache.write(workshopSnapshot)
    }

    // MARK: - Proposal

    /// What a closed tab, or a handed-over Untitled one, leaves behind: its
    /// workspace id, draft, attached file and proposal.
    private func forgetUntitledTab(_ tabID: String) {
        workspaceIDs[tabID] = nil
        workshopDrafts[tabID] = nil
        workshopPlanFiles[tabID] = nil
        workshopPinnedProjects.remove(tabID)
        workshopRequests[tabID] = nil
        reconnectingWorkshops.remove(tabID)
        workshopAcceptedTabs.remove(tabID)
        // Discarded rather than removed, so a reading still in flight for
        // this tab finds it and is dropped as stale.
        proposalStates[tabID]?.discard()
        proposalNameEdits[tabID] = nil
    }

    /// Watch the nudge marker for `plan-propose`, which touches it: the same
    /// mtime watch the project's own refresh rides on, so a proposal reaches
    /// the Plan section within its one-second poll. Started with the app and
    /// running for its life, whatever tabs are open — a reading is cheap, and
    /// only tabs with a workshop going are read at all.
    private func startProposalWatch() {
        guard proposalWatcher == nil, let nudgePath else { return }
        let watcher = NudgeWatcher()
        watcher.start(path: nudgePath) { [weak self] in
            Task { @MainActor in
                await self?.refreshProposals()
            }
        }
        proposalWatcher = watcher
    }

    /// Read every workshop's proposal: each Untitled tab's by its workspace,
    /// and each project's whose workshop could have one — its planning agent
    /// live, its row pinned, or a proposal already on screen — by the
    /// project. Each reading lands through `ProposalState`, so one that
    /// finishes after a newer reading or across an Accept is dropped rather
    /// than drawn. One that will not read or parse is logged and concludes
    /// nothing — the agent will propose again.
    public func refreshProposals() async {
        for tabID in workspaceIDs.keys {
            await readProposal(tabID: tabID)
        }
        let planners = planningAgents
        for tab in projectTabs where !isUntitledTab(tab.id) {
            guard planners[tab.id] != nil || workshopPinnedProjects.contains(tab.id)
                || proposalStates[tab.id]?.proposal != nil
            else { continue }
            await readProposal(tabID: tab.id)
        }
    }

    /// One reading of one tab's proposal — an Untitled tab's by its
    /// workspace, a project's by the project: a ticket taken before nat is
    /// asked, handed back with what it found. False where nat could not be
    /// read, which concludes nothing.
    @discardableResult
    private func readProposal(tabID: String) async -> Bool {
        let ticket = proposalStates[tabID, default: ProposalState()].beginReading()
        let client = clientFactory()
        let found: PlanProposal?
        do {
            if let workspace = workspaceIDs[tabID] {
                found = try await client.planProposal(workspaceID: workspace)
            } else {
                found = try await client.planProposal(projectID: tabID)
            }
        } catch {
            NSLog("AppModel: could not read the proposal for %@: %@", tabID, error.localizedDescription)
            return false
        }
        let arriving = found != nil && proposalStates[tabID]?.proposal == nil
        // A proposal's first arrival puts the Plan tab up; a revision
        // replaces the plan in place under whichever tab is up.
        if proposalStates[tabID]?.land(ticket, found: found) == true, arriving {
            workshopTabPicks[tabID] = .plan
        }
        return true
    }

    /// The proposal the tab on screen holds, if any.
    public var activeProposal: PlanProposal? {
        activeProjectID.flatMap { proposalStates[$0]?.proposal }
    }

    /// The proposal a tab holds, if any, whether or not it is on screen.
    public func proposal(forTab tabID: String) -> PlanProposal? {
        proposalStates[tabID]?.proposal
    }

    /// The name field: the user's own text, or the agent's suggestion until
    /// they type one.
    public var proposalName: String {
        get {
            guard let tabID = activeProjectID else { return "" }
            return proposalNameEdits[tabID] ?? proposalStates[tabID]?.proposal?.name ?? ""
        }
        set {
            guard let tabID = activeProjectID, proposalStates[tabID]?.proposal != nil else { return }
            proposalNameEdits[tabID] = newValue
            proposalStates[tabID]?.clearError()
        }
    }

    /// "Keep workshopping": back to the terminal, the tree left as it is — a
    /// revised proposal replaces it in place.
    /// On a workshop whose agent has ended (`workshopEnded`), a new agent is
    /// started on the plan left up (`continueEndedWorkshop`).
    public func keepWorkshopping() {
        if workshopEnded {
            Task { await continueEndedWorkshop() }
            return
        }
        if let id = activeProjectID { workshopTabPicks[id] = .terminal }
        terminalFocusRequest += 1
    }

    /// Keep workshopping on a workshop whose agent has ended with a plan
    /// still up: a new planning agent on the same request, told the plan is
    /// there and how to read it, to take it up from where it was left. The
    /// Brief goes on showing the request as it was first sent.
    public func continueEndedWorkshop() async {
        guard workshopEnded, let tabID = activeProjectID, let request = workshopRequest else { return }
        let read = workspaceIDs[tabID].map { "nat plan-proposal --workspace \($0) --json" }
            ?? "nat plan-proposal --project \(tabID) --json"
        let continuing = [
            request,
            "A plan was proposed for this by an earlier planning session, which has since ended, and it is still "
                + "waiting for the user to accept it. Read it with `\(read)` and take it up from there: ask the "
                + "user what to change, and propose the revised plan with `nat plan-propose` as before.",
        ].filter { !$0.isEmpty }.joined(separator: "\n\n")
        await startWorkshop(tabID: tabID, shown: request, description: continuing)
    }

    /// Whether the workshop on screen has been launched — its agent live, or
    /// its launch under way: what takes the brief editor down and puts the
    /// terminal up in its place.
    /// A workshop restored as reconnecting counts as launched, so the pane
    /// keeps its launched layout rather than flashing the composer.
    public var workshopLaunched: Bool {
        planningAgent != nil || workshopLaunching || workshopReconnecting || workshopEnded
    }

    /// The workshop pane's tabs as they stand.
    public var workshopTabs: [WorkshopTab] {
        WorkshopTab.available(launched: workshopLaunched, hasProposal: activeProposal != nil)
    }

    /// The tab the workshop pane shows: the one last picked while it is
    /// there, else the first there is; nil while there are none.
    public var workshopTab: WorkshopTab? {
        let tabs = workshopTabs
        if let id = activeProjectID, let picked = workshopTabPicks[id], tabs.contains(picked) { return picked }
        return tabs.first
    }

    /// A workshop tab, or the Plan section's header: put that tab up. One
    /// the workshop does not have changes nothing.
    public func showWorkshopTab(_ tab: WorkshopTab) {
        guard let id = activeProjectID, workshopTabs.contains(tab) else { return }
        workshopTabPicks[id] = tab
    }

    /// An edit row in the Plan section: its new brief unfolded, or folded
    /// again.
    public func toggleProposalEdit(_ name: String) {
        if expandedProposalEdits.remove(name) == nil { expandedProposalEdits.insert(name) }
    }

    /// A Plan tab box's header: the box folded to it, or open again.
    public func toggleProposedSliceFold(_ sliceID: String) {
        if foldedProposedSlices.remove(sliceID) == nil { foldedProposedSlices.insert(sliceID) }
    }

    /// A slice row in the Plan section: the Plan tab up, scrolled to that
    /// slice's box, unfolded.
    public func showProposedSlice(_ sliceID: String) {
        showWorkshopTab(.plan)
        guard workshopTab == .plan else { return }
        foldedProposedSlices.remove(sliceID)
        workshopPlanScroll = WorkshopPlanScroll(sliceID: sliceID, token: (workshopPlanScroll?.token ?? 0) + 1)
    }

    /// "Accept plan": on an Untitled tab, make the proposal a local project
    /// named from the field, end the workshop session — accepting is the
    /// goodbye — and hand the tab over to the project. An empty name refuses
    /// at the field; a refusal from nat leaves the tab and its proposal as
    /// they were, with the reason at the field. On a project's tab it files
    /// the proposal into the project — `acceptProjectProposal`. Either way the
    /// Accept is one span of `ProposalState`, begun before nat is asked and
    /// ended once what it changed is on screen.
    public func acceptProposal() async {
        guard let tabID = activeProjectID, let state = proposalStates[tabID],
              state.proposal != nil, !state.accepting else { return }
        guard isUntitledTab(tabID) else {
            await acceptProjectProposal(tabID)
            return
        }
        guard let workspace = workspaceIDs[tabID] else { return }
        let name = proposalName.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !name.isEmpty else {
            proposalStates[tabID]?.refuse(ProposalText.emptyNameError)
            return
        }
        guard proposalStates[tabID]?.beginAccept() != nil else { return }
        do {
            let accepted = try await clientFactory().planAccept(workspaceID: workspace, name: name)
            // The plan is in and the project exists; a session that will not
            // die is logged rather than undoing that.
            if tabHasLiveWorkshop(tabID), let refusal = await killWorkshop(ofTab: tabID) {
                NSLog("AppModel: the workshop session outlived its accepted plan: %@", refusal)
            }
            acceptedPlans[accepted.project.id] = accepted
            // The card is owed from here until it is dismissed or answered.
            mirrorNudgeMemory.arm(accepted.project.id)
            mirrorNudgePending.insert(accepted.project.id)
            // The Accept stays under way until the tab is the project's: the
            // hand-over discards this tab's proposal state with the rest of it.
            await addProject(id: accepted.project.id, name: accepted.project.name, replacing: tabID)
            proposalStates[tabID]?.endAccept(refusal: nil)
            activityStore?.kick()
        } catch {
            proposalStates[tabID]?.endAccept(refusal: refusalMessage(error))
        }
    }

    /// A project workshop's Accept: `nat plan-accept --project` files the
    /// proposal into the project through plan-apply's own validation and
    /// drops it. The project's plan is then read from the replica — nat wrote
    /// the plan through it, so there is nothing to pull from the workspace —
    /// and only once that read has landed does the Accept end and the
    /// proposal go, so the tree holds the new milestones before the Plan
    /// section leaves, never neither. Unlike an Untitled tab's, the session is
    /// left running — the project was there before the workshop and is there
    /// after it, and the user may well keep planning. A refusal leaves the
    /// proposal on screen with nat's reason under it.
    private func acceptProjectProposal(_ projectID: String) async {
        guard proposalStates[projectID]?.beginAccept() != nil else { return }
        do {
            _ = try await clientFactory().planAccept(projectID: projectID)
            await stores[projectID]?.refresh(.replica)
            proposalStates[projectID]?.endAccept(refusal: nil)
            // The plan is kept. A session still running may go on planning;
            // one already gone has nothing left, and its workshop goes now.
            workshopAcceptedTabs.insert(projectID)
            if planningStatus(forTab: projectID) == nil, workshopRequests[projectID] != nil {
                await workshopAgentEnded(projectID)
            }
        } catch {
            proposalStates[projectID]?.endAccept(refusal: refusalMessage(error))
        }
    }

    /// What a failed nat call says, for the field under the proposal.
    private func refusalMessage(_ error: Error) -> String {
        if case NatError.commandFailed(let message) = error { return message }
        return error.localizedDescription
    }

    // MARK: - Mirroring to Notion

    /// Whether the rail draws the "Mirror this plan to Notion?" card: on the
    /// project of an accepted plan that still owes it, and only while the
    /// project is local — one that already mirrors is never asked, whatever
    /// the memory says.
    public var mirrorNudgeShown: Bool {
        guard let id = activeProjectID, !isUntitledTab(id),
              mirrorNudgePending.contains(id),
              config?.projects[id]?.backend == .local else { return false }
        return true
    }

    /// The card's ✕: the project is never asked again, on this Mac, ever.
    public func dismissMirrorNudge() {
        guard let id = activeProjectID else { return }
        mirrorNudgeMemory.disarm(id)
        mirrorNudgePending.remove(id)
    }

    /// The picker's own state, over the same client the rest of the app talks
    /// to nat through.
    public func makeNotionPicker() -> NotionPickerModel {
        NotionPickerModel(client: clientFactory())
    }

    /// The picker's "Create page": put the active local project into Notion
    /// under `place` (`nat project-mirror`). Answers nil once it has, and
    /// otherwise what nat refused with — for the picker to show, with the card
    /// left where it was: a failed mirror changes nothing here.
    ///
    /// The project's ID changes when it mirrors — a project in Notion is known
    /// by its page — so its tab is handed over to the new ID in the place it
    /// held, and what the app kept against the old one is dropped.
    public func mirrorActiveProject(into place: NotionPlace) async -> String? {
        guard let oldID = activeProjectID, !isUntitledTab(oldID) else {
            return "There is no project open to mirror."
        }
        do {
            let mirrored = try await clientFactory().projectMirror(projectID: oldID, parent: place)
            await projectMirrored(from: oldID, to: mirrored.project)
            return nil
        } catch let error as NatError {
            if case .commandFailed(let message) = error { return message }
            return error.localizedDescription
        } catch {
            return error.localizedDescription
        }
    }

    private func projectMirrored(from oldID: String, to project: ProjectEntry) async {
        mirrorNudgeMemory.disarm(oldID)
        mirrorNudgePending.remove(oldID)
        await reloadConfig()
        await assignProjectColors()
        forgetProjectState(oldID)
        let tab = (id: project.id, name: tabName(project.id, fallback: project.name))
        if let index = projectTabs.firstIndex(where: { $0.id == oldID }) {
            if projectTabs.contains(where: { $0.id == project.id }) {
                projectTabs.remove(at: index)
            } else {
                projectTabs[index] = tab
            }
        } else if !projectTabs.contains(where: { $0.id == project.id }) {
            projectTabs.append(tab)
        }
        closedTabMemory.reopen(project.id)
        await activateProject(project.id)
    }

    /// The accepted plan the pane is showing for the active project: while the
    /// project's tab is as accepting left it, before anything is selected.
    public var acceptedPlanShown: PlanAccepted? {
        guard let id = activeProjectID, let accepted = acceptedPlans[id],
              selectedSliceID == nil, selectedSessionID == nil, selectedContainerID == nil,
              !workshopSelected else { return nil }
        return accepted
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
                selectedContainerIDs[activeID] = nil
                // Opened with nothing running: pinned to Active until it is
                // launched or dismissed, so clicking away keeps its row.
                if planningAgent == nil, !workshopReconnecting { workshopPinnedProjects.insert(activeID) }
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
        let trimmed = request.trimmingCharacters(in: .whitespacesAndNewlines)
        // An Untitled tab has no project to workshop on, only what the
        // starter card was asked: the agent is told to start on it, so there
        // is nothing to launch without it.
        let workspace = workspaceIDs[projectID]
        let planFile = workspace != nil ? workshopPlanFiles[projectID] : nil
        if workspace != nil, trimmed.isEmpty, planFile == nil { return }
        workshopSelected = true
        guard planningAgent == nil, !workshopLaunching else { return }
        // What the Brief shows, read-only, from the moment Launch is pressed;
        // a launch that does not take takes it back off again.
        let shown = [trimmed, planFile.map { "Attached: \($0.name)" }]
            .compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: "\n\n")
        await startWorkshop(tabID: projectID, shown: shown, description: trimmed)
    }

    /// Start a planning agent on a tab, its Brief showing `shown` and the
    /// agent sent `description` (with an Untitled tab's attached plan file).
    /// The draft and file are kept, not cleared: if the agent ends before it
    /// proposes anything, the composer comes back with them in it
    /// (`EndedWorkshop.restoreBrief`); closing or dismissing the workshop is
    /// what discards them.
    private func startWorkshop(tabID projectID: String, shown: String, description: String) async {
        let workspace = workspaceIDs[projectID]
        let planFile = workspace != nil ? workshopPlanFiles[projectID] : nil
        workshopLaunching = true
        workshopLaunchError = nil
        workshopTabPicks[projectID] = .terminal
        workshopRequests[projectID] = shown
        // A fresh session has accepted nothing yet.
        workshopAcceptedTabs.remove(projectID)
        do {
            if let workspace {
                _ = try await clientFactory().workspaceLaunch(
                    workspaceID: workspace,
                    model: config?.workshopAgent?.model,
                    effort: config?.workshopAgent?.effort,
                    request: planFile?.request(description: description) ?? description
                )
            } else {
                _ = try await workshopLauncher(
                    projectID,
                    config?.workshopAgent?.model,
                    config?.workshopAgent?.effort,
                    description
                )
            }
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
            // The live agent's row takes the pinned one's place.
            if workshopLaunchError == nil { workshopPinnedProjects.remove(projectID) }
        } else {
            workshopRequests[projectID] = nil
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
        workshopLaunchError = "The workshop session was launched but has not appeared. Run `nat status` to check on it."
    }

    /// What a project's tab says needs attention — the count its pill draws
    /// and the state its dot takes, as one reading so the two cannot
    /// disagree. Nothing at all for a project whose plan has not landed:
    /// there are no slices to read anything off yet. Its count is the
    /// project's share of `dockAttention`, read off the same inputs.
    public func attention(projectID: String) -> ProjectAttention {
        guard let inputs = attentionInputs(projectID: projectID) else { return .none }
        return projectAttention(
            slices: inputs.slices, liveAgents: inputs.liveAgents, planningAgent: inputs.planning,
            prReading: inputs.prReading, sessions: inputs.sessions)
    }

    /// Everything waiting on the user across every open project, in tab
    /// order — what the dock badges, lists in its menu and bounces for.
    /// Computed off the same stores the pills are (each plan read, activity
    /// re-read and `pr-status` reading is observed through them), so it is
    /// never stale and needs no poll of its own.
    public var dockAttention: [AttentionItem] {
        projectTabs.flatMap { tab -> [AttentionItem] in
            guard let inputs = attentionInputs(projectID: tab.id) else { return [] }
            return attentionItems(
                projectID: tab.id, slices: inputs.slices, liveAgents: inputs.liveAgents,
                planningAgent: inputs.planning, prReading: inputs.prReading, sessions: inputs.sessions)
        }
    }

    /// The dock menu's groups over `dockAttention`, each row carrying its
    /// project's tag as the Active rows do.
    public var dockMenu: [DockMenuSection] {
        dockMenuSections(dockAttention, tags: sidebarTags(sidebarInputs))
    }

    /// Selects what a dock menu row names: its slice or session, or the
    /// planning agent's workshop — its project activated first.
    public func select(_ item: AttentionItem) async {
        switch item.subject {
        case .slice(let id): await selectSlice(id, inProject: item.projectID)
        case .session(let id): await selectSession(id, inProject: item.projectID)
        case .workshop: await selectWorkshop(inProject: item.projectID)
        }
    }

    /// One project's attention inputs, nil where its plan has not landed.
    ///
    /// The PR reading is the project's own last `pr-status` reading
    /// (`prStatusStore`) — absent, never wrong, for one not yet read, exactly
    /// as the rail does with no reading taken. The planning agent is the one the activity map
    /// attributes to this project by its own scoped tag; the bare legacy
    /// sentinel belongs to no project in particular and is nobody's tab.
    /// Sessions are read for the active project only, the one
    /// `sessionStore` holds.
    private func attentionInputs(projectID: String) -> (
        slices: [Slice], liveAgents: [String: AgentActivity], planning: AgentActivity?,
        prReading: PRReading, sessions: [Session]
    )? {
        guard let projectInfo = stores[projectID]?.state.projectInfo else { return nil }
        let agents = activityStore?.agents ?? [:]
        return (
            slices: projectInfo.slices,
            liveAgents: agents.mapValues { AgentActivity($0.activity) },
            planning: agents[TmuxSession.planTag(projectID: projectID)].map { AgentActivity($0.activity) },
            prReading: prStatusStore?.reading(projectID: projectID) ?? .empty,
            sessions: projectID == activeProjectID ? (sessionStore?.sessions ?? []) : []
        )
    }

    /// Manually refresh the current project — also the nudge watcher's own
    /// action, so an agent's hand-back reads that slice's stat in without
    /// waiting for the next poll.
    public func refresh(_ read: PlanRead = .pull) async {
        guard let projectStore = projectStore else { return }
        refreshBackgroundProjects(read)
        await projectStore.refresh(read)
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
        // The one on screen is read again instead, behind what it shows: an
        // agent proposing follow-ups writes nothing a plan read carries, so
        // this is what brings the Follow-ups sidebar up — and takes it down
        // once they are triaged.
        if let selectedSliceID {
            await sliceDetailStore(projectID: projectStore.projectID).fetch(sliceRef: selectedSliceID)
        }
        // A source container's detail the same way: every other reading
        // dropped, the one on screen read again behind what it shows.
        containerStores[projectStore.projectID]?.invalidateCache(keeping: selectedContainerID)
        if let selectedContainerID {
            await containerStore(projectID: projectStore.projectID).fetch(containerID: selectedContainerID)
        }
    }

    /// Applies the user's choices for one batch of a slice's follow-ups
    /// (`slice-triage`), then refreshes inside the apply, so the queued slices
    /// land in the plan, the batch's card goes, and every other batch's card
    /// is read with fresh indexes before it can apply.
    public func applyFollowUps(sliceID: String, batch: Int, followUps: [FollowUp]) async {
        guard let projectID = projectStore?.projectID else { return }
        await followUpStore.apply(
            projectID: projectID, sliceID: sliceID, batch: batch, followUps: followUps, client: clientFactory(),
            then: { await self.refresh() })
    }

    /// Send back to agent: a handed-back slice — in review, or at its open
    /// pull request — goes back to its agent for more. The record first, as
    /// `slice-triage` writes before its send: `nat slice-resume` files the
    /// note under a stamped `Resumed` and clears the Branch, so the slice reads
    /// as being worked again (`Slice.resumed`). Then the agent hears of it: a
    /// live one by `agent-send`, told why and ending in the `complete-slice
    /// --branch` hand-back on the branch the slice had; with none, the
    /// ordinary `slice-launch` — a relaunch, its prompt reading the Resumed
    /// off the page. A refused resume sends nothing; a send or launch that
    /// fails after it leaves the record standing, and its error is the one
    /// shown, so trying again resumes nothing twice (nat writes nothing on a
    /// slice already resumed) and only repeats the telling.
    ///
    /// Run as the one-shot `.sendBack`, its error kept for the action bar.
    /// Returns whether it went through.
    @discardableResult
    public func sendBack(slice: Slice, note: String, model: String? = nil, effort: String? = nil) async -> Bool {
        guard let projectID = projectStore?.projectID else { return false }
        let note = note.trimmingCharacters(in: .whitespacesAndNewlines)
        let client = clientFactory()
        let live = activityStore?.agents[slice.id] != nil
        var sent = false
        await sliceActions.run(.sendBack, sliceID: slice.id, select: { _ in }) {
            guard !note.isEmpty else { throw NatError.commandFailed("Say what the agent should change.") }
            try await client.sliceResume(projectID: projectID, sliceRef: slice.id, note: note)
            if live {
                let prompt = sendBackPrompt(
                    note: note, branch: slice.branch,
                    handBack: HandBackInstruction(projectID: projectID, sliceRef: slice.id))
                try await client.agentSend(projectID: projectID, sliceRef: slice.id, text: prompt)
            } else {
                _ = try await client.sliceLaunch(projectID: projectID, sliceRef: slice.id, model: model, effort: effort)
            }
            sent = true
        }
        await refresh()
        return sent
    }

    /// Drops every follow-up of one batch of a slice, then refreshes, as an
    /// apply does.
    public func discardFollowUps(sliceID: String, batch: Int, followUps: [FollowUp]) async {
        guard let projectID = projectStore?.projectID else { return }
        await followUpStore.discard(
            projectID: projectID, sliceID: sliceID, batch: batch, followUps: followUps, client: clientFactory(),
            then: { await self.refresh() })
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
                    self.githubActionRan()
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
        return await killWorkshop(ofTab: projectID)
    }

    /// `killWorkshopAgent` for any tab, not only the one on screen — an
    /// Untitled tab is killed by its workspace id, a project's by the project.
    private func killWorkshop(ofTab projectID: String) async -> String? {
        do {
            if let workspace = workspaceIDs[projectID] {
                try await clientFactory().agentKillWorkspace(workspaceID: workspace)
            } else {
                try await clientFactory().agentKillWorkshop(projectID: projectID)
            }
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
        workshopPlanFiles[projectID] = nil
        workshopRequests[projectID] = nil
        reconnectingWorkshops.remove(projectID)
        workshopAcceptedTabs.remove(projectID)
        workshopPinnedProjects.remove(projectID)
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

    /// Start an ad hoc session in the named project, activating it first —
    /// Active's `+` names one from its menu, and every project's own `+`
    /// (Scratch's included) names itself. A scratch project's session needs
    /// its `dir` chosen first, as `sessionNeedsFolder(inProject:)` says.
    public func launchSession(inProject projectID: String, dir: String? = nil) async {
        if activeProjectID != projectID { await activateProject(projectID) }
        await launchSession(dir: dir)
    }

    /// Whether a session in the named project must ask for a folder first —
    /// `newSessionNeedsFolder` for a project that is not yet the active one.
    public func sessionNeedsFolder(inProject projectID: String) -> Bool {
        isScratchTab(projectID)
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
    }

    private func startNudgeWatcher(for projectStore: ProjectStore, nudgePath: String) {
        let watcher = NudgeWatcher()
        watcher.start(path: nudgePath) { [weak self] in
            Task { @MainActor in
                // An agent marking itself waiting or working nudges too, and
                // its state should not wait on the plan read below.
                self?.activityStore?.reread()
                // A nudge is a write made on this machine, which nat made
                // through the replica: it is already there to read.
                await self?.refresh(.replica)
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
                        // The nudge watch brings proposals within a second;
                        // the poll is what catches one it never saw — a
                        // proposal already on disk when its workshop opened.
                        await self.refreshProposals()
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
        githubReadingStore?.stop()
        githubReadingStore = nil
        prStatusStore = nil
        sessionStore = nil
        sliceDetailStores = [:]
        containerStores = [:]
        diffStores = [:]
        visualStores = [:]
        prStores = [:]
        sessionDiffStores = [:]
        pickerMemory = PickerSelectionMemory()
    }
}

// MARK: - Run commands

/// A run started from this app: the tmux session `nat run` started it in —
/// held while it lives, so its button spins — and what it was started for.
public struct RunAttachment: Equatable, Sendable {
    public let session: String
    public let label: String
    public let projectID: String
    /// The slice a slice-scoped run was started on; nil for a global run.
    public let sliceID: String?

    public init(session: String, label: String, projectID: String, sliceID: String?) {
        self.session = session
        self.label = label
        self.projectID = projectID
        self.sliceID = sliceID
    }
}

/// A project as the titlebar's run tree lists it: its name and its runs.
public struct RunProject: Equatable, Identifiable, Sendable {
    public let id: String
    public let name: String
    public let runs: [RunCommand]

    public init(id: String, name: String, runs: [RunCommand]) {
        self.id = id
        self.name = name
        self.runs = runs
    }
}

extension AppModel {
    /// The key a run is held under: the slice's ID, or the project's own for
    /// a global run.
    public static func runKey(projectID: String, sliceID: String?) -> String {
        sliceID ?? "project:" + projectID
    }

    /// Every project with runs to offer from the titlebar, by name, each
    /// with its global runs — the titlebar's run tree.
    public var runProjects: [RunProject] {
        (config?.projects ?? [:]).compactMap { id, project in
            let runs = project.runs.globalRuns
            return runs.isEmpty ? nil : RunProject(id: id, name: tabName(id, fallback: project.name), runs: runs)
        }
        .sorted { ($0.name.lowercased(), $0.id) < ($1.name.lowercased(), $1.id) }
    }

    /// The project's runs the titlebar offers, as its config entry lists
    /// them — the first being the default.
    public func globalRuns(ofProject projectID: String) -> [RunCommand] {
        config?.projects[projectID]?.runs.globalRuns ?? []
    }

    /// The project's runs a handed-back slice's navigator offers.
    public func sliceRuns(ofProject projectID: String) -> [RunCommand] {
        config?.projects[projectID]?.runs.sliceRuns ?? []
    }

    /// Whether a run for that key is being started.
    public func isStartingRun(projectID: String, sliceID: String?) -> Bool {
        runsStarting[Self.runKey(projectID: projectID, sliceID: sliceID)] != nil
    }

    /// Whether the run with that label is being started or its session is
    /// still live — what a run offered anywhere is disabled for, so the same
    /// command is not doubled up. A guard of the UI's alone: nat itself
    /// restarts a live session asked for the same run again.
    public func isRunning(projectID: String, sliceID: String?, label: String) -> Bool {
        let key = Self.runKey(projectID: projectID, sliceID: sliceID)
        return runsStarting[key] == label || runs[key]?.label == label
    }

    /// Whether a run for that key is being started or its session is still
    /// live — what a run button spins for, there being no tab to show it.
    public func isRunBusy(projectID: String, sliceID: String?) -> Bool {
        isStartingRun(projectID: projectID, sliceID: sliceID)
            || runs[Self.runKey(projectID: projectID, sliceID: sliceID)] != nil
    }

    /// Whether any run at all is being started or still live — the
    /// titlebar's run button's spinner.
    public var anyRunBusy: Bool { !runsStarting.isEmpty || !runs.isEmpty }

    /// Start a run — `nat run`, `sliceID` nil for a global one and `label`
    /// nil for nat's default — and hold its session while it lives, in
    /// place of whatever that key held; no pane opens on it. A refusal is
    /// `runError`, in nat's own words.
    public func startRun(projectID: String, sliceID: String? = nil, label: String? = nil) async {
        let key = Self.runKey(projectID: projectID, sliceID: sliceID)
        let scope = sliceID == nil ? globalRuns(ofProject: projectID) : sliceRuns(ofProject: projectID)
        runsStarting[key] = label ?? scope.first?.label ?? ""
        runError = nil
        defer { runsStarting[key] = nil }
        do {
            let result = try await clientFactory().run(projectID: projectID, sliceRef: sliceID, label: label)
            runs[key] = RunAttachment(session: result.session, label: result.label, projectID: projectID, sliceID: sliceID)
            watchRun(key: key, session: result.session)
        } catch {
            runError = refusalMessage(error)
        }
    }

    /// Dismiss the last run's refusal.
    public func dismissRunError() { runError = nil }

    /// A run's session is gone: every key holding it lets it go, and its
    /// button stops spinning.
    public func runEnded(session: String) {
        runs = runs.filter { $0.value.session != session }
    }

    /// Ask after a run's session until it ends, or until its key is given to
    /// another run — what stops its button spinning once the command finishes.
    private func watchRun(key: String, session: String) {
        Task { [weak self] in
            while let self, self.runs[key]?.session == session {
                try? await Task.sleep(nanoseconds: self.runWatchInterval)
                guard self.runs[key]?.session == session else { return }
                if await !self.runSessionExists(session) { self.runEnded(session: session) }
            }
        }
    }
}
