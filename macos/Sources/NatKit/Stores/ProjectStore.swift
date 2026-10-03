import Foundation
import SwiftUI

/// A protocol for providing nat client functionality (allows injection for testing).
public protocol NatClientProtocol: Sendable {
    func info(projectID: String) async throws -> ProjectInfo
    func info(projectID: String, refresh: Bool) async throws -> ProjectInfo
    func status() async throws -> [AgentStatus]
    func usage() async throws -> UsageReading
    func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail
    func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff
    func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc
    func sliceFile(projectID: String, sliceRef: String, commit: String?, path: String, from: Int, to: Int?) async throws -> SliceFileLines
    func sliceEdit(projectID: String, sliceRef: String, description: String) async throws -> SliceEditResult
    func sliceLaunch(projectID: String, sliceRef: String, model: String?, effort: String?) async throws -> LaunchResult
    func agentSend(projectID: String, sliceRef: String, text: String) async throws -> Void
    func agentKill(projectID: String, sliceRef: String) async throws -> Void
    func agentKillWorkshop(projectID: String) async throws -> Void
    func sliceStatus(projectID: String, sliceRef: String) async throws -> SliceStatusResult
    func sliceApprove(projectID: String, sliceRef: String) async throws -> String
    func sliceRework(projectID: String, sliceRef: String, comments: String) async throws -> Void
    func sliceTriage(projectID: String, sliceRef: String, queue: [Int], fold: [Int], drop: [Int]) async throws -> TriageResult
    func sliceDiscardFollowUps(projectID: String, sliceRef: String) async throws -> TriageResult
    func prView(projectID: String, sliceRef: String) async throws -> PRDetail
    func prStatus(projectID: String) async throws -> PRStatusDoc
    func prMerge(projectID: String, sliceRef: String) async throws -> Void
    func prComment(projectID: String, sliceRef: String, body: String) async throws -> Void
    func prReviewers(projectID: String, sliceRef: String, add: [String], remove: [String]) async throws -> PRReviewers
    func workshopLaunch(projectID: String, model: String?, effort: String?, request: String?) async throws -> WorkshopLaunchResult
    func sliceAdd(projectID: String, title: String, milestone: String, description: String?) async throws -> SliceAddResult
    func configShow() async throws -> ConfigDoc
    func configSet(key: String, value: String) async throws -> Void
    func sessionLaunch(projectID: String, dir: String?, model: String?, effort: String?) async throws -> SessionLaunchResult
    func sessionList(projectID: String) async throws -> [Session]
    func sessionStatus(projectID: String, sessionID: String, discard: Bool) async throws -> SessionStatusDoc
    func sessionDiff(projectID: String, sessionID: String, branch: String?) async throws -> SliceDiff
    func sessionPRView(projectID: String, sessionID: String, prURL: String) async throws -> PRDetail
    func scratchOpen() async throws -> ScratchOpenResult
    func doneClear(projectID: String) async throws -> DoneClearResult
    func workspaceLaunch(workspaceID: String, model: String?, effort: String?, request: String) async throws -> WorkshopLaunchResult
    func agentKillWorkspace(workspaceID: String) async throws -> Void
    func projectOpenFolder(path: String) async throws -> ProjectEntry
    func planProposal(workspaceID: String) async throws -> PlanProposal?
    func planAccept(workspaceID: String, name: String) async throws -> PlanAccepted
    func planProposal(projectID: String) async throws -> PlanProposal?
    func planAccept(projectID: String) async throws -> PlanAccepted
    func notionSearch(query: String) async throws -> [NotionPlace]
    func projectMirror(projectID: String, parent: NotionPlace) async throws -> ProjectMirrored
    func info(projectID: String, refresh: Bool, expand: [String]) async throws -> ProjectInfo
    func containerShow(projectID: String, containerID: String) async throws -> ContainerShow
    func sourceAction(projectID: String, action: String, group: String?, container: String?, input: String?) async throws -> SourceActionResult
    func sourceList() async throws -> [SourcePlugin]
    func pluginList() async throws -> PluginListing
    func pluginInstall(name: String, source: String?, version: String?) async throws -> PluginInstalled
    func pluginUninstall(name: String, deleteProjects: Bool) async throws -> PluginUninstalled
    func pluginSourceAdd(repo: String) async throws -> PluginSourceList
    func pluginSourceRemove(repo: String) async throws -> PluginSourceList
    func sourceSetup(plugin: String, id: String, value: String) async throws -> PluginSetupResult
    func sliceAdd(projectID: String, title: String, container: String, description: String?) async throws -> SliceAddResult
    func projectCreate(name: String, repo: String?, description: String?, source: String?) async throws -> CreatedProject
}

extension NatClientProtocol {
    /// A plan read that may refresh the replica first: a conformer with no
    /// replica to refresh — every test double and the fixture client —
    /// answers it as the plain read, so none of them need say so.
    public func info(projectID: String, refresh: Bool) async throws -> ProjectInfo {
        try await info(projectID: projectID)
    }

    /// A file's own lines: only `NatClient`, the fixture client and the
    /// diff store's tests implement this, the same reasoning as
    /// `workspaceLaunch`.
    public func sliceFile(
        projectID: String, sliceRef: String, commit: String?, path: String, from: Int, to: Int?
    ) async throws -> SliceFileLines {
        throw NatError.commandFailed("slice-file: not supported by this client")
    }

    /// Reviewers: only `NatClient` and the fixture client implement this,
    /// the same reasoning as `workspaceLaunch`.
    public func prReviewers(projectID: String, sliceRef: String, add: [String], remove: [String]) async throws -> PRReviewers {
        throw NatError.commandFailed("pr-reviewers: not supported by this client")
    }

    /// Triaging follow-ups: only `NatClient` and the fixture client
    /// implement these, the same reasoning as `workspaceLaunch`.
    public func sliceTriage(projectID: String, sliceRef: String, queue: [Int], fold: [Int], drop: [Int]) async throws -> TriageResult {
        throw NatError.commandFailed("slice-triage: not supported by this client")
    }

    public func sliceDiscardFollowUps(projectID: String, sliceRef: String) async throws -> TriageResult {
        throw NatError.commandFailed("slice-triage --drop-all: not supported by this client")
    }

    /// The Untitled tab's planning agent: only `NatClient` and the fixture
    /// client implement these, so a test double for a store that never
    /// launches one need not say so — the same reason `sliceRework` is a
    /// default.
    public func workspaceLaunch(workspaceID: String, model: String?, effort: String?, request: String) async throws -> WorkshopLaunchResult {
        throw NatError.commandFailed("workshop-launch --workspace: not supported by this client")
    }

    public func agentKillWorkspace(workspaceID: String) async throws {
        throw NatError.commandFailed("agent-kill --workspace: not supported by this client")
    }

    /// Opening a plan folder as a project: same reasoning, only `NatClient`
    /// implements it.
    public func projectOpenFolder(path: String) async throws -> ProjectEntry {
        throw NatError.commandFailed("project-open-folder: not supported by this client")
    }

    /// The Untitled tab's proposal: same reasoning, only `NatClient` and the
    /// fixture client implement them.
    public func planProposal(workspaceID: String) async throws -> PlanProposal? {
        throw NatError.commandFailed("plan-proposal: not supported by this client")
    }

    public func planAccept(workspaceID: String, name: String) async throws -> PlanAccepted {
        throw NatError.commandFailed("plan-accept: not supported by this client")
    }

    /// A project workshop's proposal: same reasoning.
    public func planProposal(projectID: String) async throws -> PlanProposal? {
        throw NatError.commandFailed("plan-proposal --project: not supported by this client")
    }

    public func planAccept(projectID: String) async throws -> PlanAccepted {
        throw NatError.commandFailed("plan-accept --project: not supported by this client")
    }

    /// Making a project — what connecting a plugin does for its section:
    /// same reasoning, only `NatClient` and the fixture client implement it.
    public func projectCreate(name: String, repo: String?, description: String?, source: String?) async throws -> CreatedProject {
        throw NatError.commandFailed("project-create: not supported by this client")
    }

    /// Mirroring a local project into Notion: same reasoning, only `NatClient`
    /// and the fixture client implement them.
    public func notionSearch(query: String) async throws -> [NotionPlace] {
        throw NatError.commandFailed("notion-search: not supported by this client")
    }

    public func projectMirror(projectID: String, parent: NotionPlace) async throws -> ProjectMirrored {
        throw NatError.commandFailed("project-mirror: not supported by this client")
    }

    /// A conformer that never reworks a slice — every test double but the
    /// ones exercising the approve-over-comments flow — need not say so:
    /// `NatClient` and the fixture client are the two that implement it.
    public func sliceRework(projectID: String, sliceRef: String, comments: String) async throws {
        throw NatError.commandFailed("slice-rework: not supported by this client")
    }

    /// The whole-branch diff, without naming a commit — `sliceDiff(projectID:sliceRef:commit:)`
    /// with `commit: nil`, kept as the two-argument spelling every caller
    /// asking for "the diff" (rather than one commit of it) already uses.
    public func sliceDiff(projectID: String, sliceRef: String) async throws -> SliceDiff {
        try await sliceDiff(projectID: projectID, sliceRef: sliceRef, commit: nil)
    }

    /// Default ad hoc session methods, so a mock client written for a store
    /// that never touches sessions (most of the pre-existing ones) does not
    /// have to stub four methods it will never be asked to answer — the same
    /// reason the two-argument `sliceDiff` above is a default rather than a
    /// second protocol requirement. `SessionStore`'s own tests, and
    /// `FixtureNatClient`, override every one of these for real.
    public func sessionLaunch(projectID: String, dir: String?, model: String?, effort: String?) async throws -> SessionLaunchResult {
        throw NatError.commandFailed("session-launch: not stubbed by this test client")
    }

    public func sessionList(projectID: String) async throws -> [Session] { [] }

    public func sessionStatus(projectID: String, sessionID: String, discard: Bool) async throws -> SessionStatusDoc {
        throw NatError.commandFailed("session-status: not stubbed by this test client")
    }

    public func sessionDiff(projectID: String, sessionID: String, branch: String?) async throws -> SliceDiff {
        throw NatError.commandFailed("session-diff: not stubbed by this test client")
    }

    public func sessionPRView(projectID: String, sessionID: String, prURL: String) async throws -> PRDetail {
        throw NatError.commandFailed("pr-view --session: not stubbed by this test client")
    }

    /// Defaults for the scratch project's two commands, for the same reason:
    /// only `AppModel.start()` calls them, and a mock written for any other
    /// store has nothing to say to either.
    public func scratchOpen() async throws -> ScratchOpenResult {
        throw NatError.commandFailed("scratch-open: not stubbed by this test client")
    }

    public func doneClear(projectID: String) async throws -> DoneClearResult {
        throw NatError.commandFailed("done-clear: not stubbed by this test client")
    }

    /// Task sources: only `NatClient` and the fixture client implement
    /// these, the same reasoning as `workspaceLaunch`. A plan read with lazy
    /// groups expanded is the plain one to a conformer with no source.
    public func info(projectID: String, refresh: Bool, expand: [String]) async throws -> ProjectInfo {
        try await info(projectID: projectID, refresh: refresh)
    }

    public func containerShow(projectID: String, containerID: String) async throws -> ContainerShow {
        throw NatError.commandFailed("container-show: not supported by this client")
    }

    public func sourceAction(
        projectID: String, action: String, group: String?, container: String?, input: String?
    ) async throws -> SourceActionResult {
        throw NatError.commandFailed("source-action: not supported by this client")
    }

    public func sourceList() async throws -> [SourcePlugin] {
        throw NatError.commandFailed("source-list: not supported by this client")
    }

    /// Plugin install: only `NatClient` and the fixture client implement
    /// these, the same reasoning as `workspaceLaunch`.
    public func pluginList() async throws -> PluginListing {
        throw NatError.commandFailed("plugin-list: not supported by this client")
    }

    public func pluginInstall(name: String, source: String?, version: String?) async throws -> PluginInstalled {
        throw NatError.commandFailed("plugin-install: not supported by this client")
    }

    public func pluginUninstall(name: String, deleteProjects: Bool) async throws -> PluginUninstalled {
        throw NatError.commandFailed("plugin-uninstall: not supported by this client")
    }

    public func pluginSourceAdd(repo: String) async throws -> PluginSourceList {
        throw NatError.commandFailed("plugin-source-add: not supported by this client")
    }

    public func pluginSourceRemove(repo: String) async throws -> PluginSourceList {
        throw NatError.commandFailed("plugin-source-remove: not supported by this client")
    }

    public func sourceSetup(plugin: String, id: String, value: String) async throws -> PluginSetupResult {
        throw NatError.commandFailed("source-setup: not supported by this client")
    }

    public func sliceAdd(projectID: String, title: String, container: String, description: String?) async throws -> SliceAddResult {
        throw NatError.commandFailed("slice-add --container: not supported by this client")
    }
}

// Make NatClient conform to the protocol
extension NatClient: NatClientProtocol {}

/// How a plan read treats the replica nat answers from.
public enum PlanRead: Sendable {
    /// The replica as it stands — a file read. What follows a write made on
    /// this machine: nat wrote it through the replica, so the replica already
    /// holds it, and pulling the workspace would only wait on news we made.
    case replica
    /// Let nat bring a stale replica up to date first (`nat info --refresh`)
    /// — the poll and the user's own refresh, which are how news made
    /// elsewhere arrives.
    case pull
}

/// Manages loading and refreshing project information.
///
/// Reads never overlap and never go missing: a read asked for while another
/// is in flight is not dropped but owed, and every request made during one
/// read is answered by the one read that follows it. So when `load` returns,
/// the plan on hand is from a read that began after it was called — a write
/// awaited before it is in what is drawn, whatever else was reading at the
/// time. Errors fall back to the previous successful load, and a refresh keeps
/// showing the previous data while it reloads.
@MainActor
@Observable
public final class ProjectStore {
    public private(set) var projectID: String
    public private(set) var state: LoadState = .idle
    private let client: NatClientProtocol
    private let cache: PlanCaching
    private var isLoadInFlight = false

    /// The read state machine's counters: every `load` call takes the next
    /// request number, and `answered` is the highest request a read that
    /// began after it has landed for. A call is done once `answered` reaches
    /// its own number.
    private var requested = 0
    private var answered = 0
    /// Whether any request still owed asked for a pull.
    private var owedPull = false
    private var waiters: [(request: Int, continuation: CheckedContinuation<Void, Never>)] = []

    /// How many `load` calls are waiting on a read still to come — a test's
    /// way to know its requests are queued before it lets a read finish.
    var owedLoads: Int { waiters.count }

    /// Whether the cache has already been asked about this project. It is
    /// asked once, before the first read lands: after that the plan in hand
    /// is fresher than anything on disk, so a later load has nothing to seed
    /// from and every refresh that fails keeps what it has anyway.
    private var cacheConsulted = false

    /// Whether a read has landed yet — a pull is only asked for once there is
    /// a plan on screen to wait behind; the first read is always the replica.
    private var hasRead = false

    /// A source project's lazy groups the user has opened, passed on every
    /// read (`info --expand`) so an opened group stays listed. Empty for
    /// every other project, whose reads it does not change.
    public var expand: [String] = []

    public init(
        projectID: String,
        client: NatClientProtocol = NatClient(),
        cache: PlanCaching = DiskPlanCache()
    ) {
        self.projectID = projectID
        self.client = client
        self.cache = cache
    }

    /// Load project information, and return once a read that began after
    /// this call has landed.
    ///
    /// While a read is in flight the request is owed rather than run: the
    /// in-flight read may have begun before whatever the caller is waiting
    /// to see, so one more read follows it, answering every request made
    /// meanwhile at once (a pull if any of them asked for one).
    public func load(_ read: PlanRead = .pull) async {
        requested += 1
        let request = requested
        if read == .pull { owedPull = true }
        guard !isLoadInFlight else {
            await withCheckedContinuation { waiters.append((request, $0)) }
            return
        }

        isLoadInFlight = true
        while answered < requested {
            let covers = requested
            let pull = owedPull
            owedPull = false
            await readOnce(pull: pull)
            answered = covers
            let done = waiters.filter { $0.request <= covers }
            waiters.removeAll { $0.request <= covers }
            for waiter in done { waiter.continuation.resume() }
        }
        isLoadInFlight = false
    }

    private func readOnce(pull: Bool) async {
        // Only a first load shows as loading, and only where there is
        // nothing to show in the meantime: the last plan this project was
        // read as is on disk, and seeding it here is what puts the board on
        // screen at once. From there it is the ordinary refresh — a plan
        // already in hand stays up until the fresh one lands, so the board
        // never blanks under a poll, a nudge, or a launch.
        if state.projectInfo == nil, !cacheConsulted {
            cacheConsulted = true
            if let cached = await cache.read(projectID: projectID) {
                state = .loaded(cached)
            }
        }
        if state.projectInfo == nil {
            state = .loading
        }

        do {
            // The first read takes the replica as it stands — a file read,
            // nothing to wait on — and a pull asked for after it lets nat
            // bring a stale one up to date first, behind what is already drawn.
            let info = expand.isEmpty
                ? try await client.info(projectID: projectID, refresh: pull && hasRead)
                : try await client.info(projectID: projectID, refresh: pull && hasRead, expand: expand)
            hasRead = true
            state = .loaded(info)
            // Every read that lands is what the next launch starts from.
            await cache.write(info, projectID: projectID)
        } catch {
            let previousInfo = state.projectInfo
            state = .failed(error.localizedDescription, previous: previousInfo)
        }
    }

    /// Refresh project information, keeping the previous data visible while
    /// reloading — `load` by another name.
    public func refresh(_ read: PlanRead = .pull) async {
        await load(read)
    }
}
