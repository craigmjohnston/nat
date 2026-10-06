import Foundation
import NatKit

/// The fixtures handed back through the very protocol every store already
/// reads, so a whole board can be canned without a single `nat` process.
///
/// Nothing here spawns anything, sleeps or touches the disk: every read
/// answers from `Fixtures` immediately, and every write is remembered rather
/// than performed, so a preview that presses a button neither hangs nor
/// changes anything. `failing` is the same client with every read refusing,
/// which is how the error states are reached through a store rather than
/// written into one.
public final class FixtureNatClient: NatClientProtocol, @unchecked Sendable {
    /// What every read does: answer from the fixtures, or refuse with the
    /// message a real refusal would carry.
    public enum Behaviour: Sendable {
        case answering
        case refusing(String)
        /// Every call waits and never lands. A view over such a client draws
        /// its loading state for as long as it is up, which is what a
        /// skeleton story is: the state a real load passes through in a
        /// tenth of a second, held still long enough to look at.
        case hanging
    }

    public let behaviour: Behaviour
    /// The fixture project's plan — swapped by `setPlan` as a story moves a
    /// slice on mid-life.
    private let planBox: Box<ProjectInfo>
    private var plan: ProjectInfo { planBox.get() }
    /// Plans for projects other than the fixture's own, by project ID — what
    /// a multi-project sidebar reads for each of the others. A project with
    /// none here reads `plan`.
    private let otherPlans: [String: ProjectInfo]
    private let agents: [AgentStatus]
    /// The planning agents Untitled tabs have launched here, on top of the
    /// fixed `agents` — a launch puts one in the next reading and a kill
    /// takes it out, as tmux would.
    private let workspaceAgents = Box<[AgentStatus]>([])
    /// The plan a workshop has proposed, as `plan-proposal` reads it back —
    /// nil until something sets one, as a workshop that has not drafted yet.
    private let proposalBox = Box<PlanProposal?>(nil)
    /// What each project's own workshop has proposed, by project.
    private let projectProposals = Box<[String: PlanProposal]>([:])
    /// Set once an Accept should never come back — the accept-in-flight
    /// story's state, held still.
    private let acceptHangs = Box<Bool>(false)
    /// The same for a workshop launch — the launching story's state.
    private let launchHangs = Box<Bool>(false)
    /// The same for `status` — the activity poll's first reading never
    /// landing, the reconnecting story's state.
    private let statusHangs = Box<Bool>(false)
    /// The same for a checks re-run or cancel — the mid-call story's state.
    private let checksHang = Box<Bool>(false)
    private let acceptRefusal = Box<String?>(nil)
    private let diff: SliceDiff
    private let pr: PRDetail
    private let config: ConfigDoc
    private let usageReading: UsageReading
    private let sessionsList: [Session]
    /// What `slice-show` answers, by slice — `Fixtures.sliceDetails` unless a
    /// story wants another reading of one (a slice with follow-ups pending).
    private let details: [String: SliceDetail]
    /// What `pr-status` answers — `Fixtures.prStatusDoc` unless a story
    /// reads another.
    private let prStatusDoc: PRStatusDoc
    /// What `pr-status` answers for a project other than the fixture's own
    /// reading — `.some(nil)` a reading that fails — set at init or later, as
    /// a test moves a project's pull requests on.
    private let prStatusByProject: Box<[String: PRStatusDoc?]>
    /// What `plugin-list` answers — the same listing after every install,
    /// since nothing here is installed.
    private let plugins: PluginListing
    /// What `source-list` answers.
    private let sources: [SourcePlugin]
    /// The setup fields `source-setup` has set, as `plugin/field`.
    private let setUpFields = Box<Set<String>>([])

    /// Every write this client was asked to make, in order — a preview never
    /// looks, and a test asserting that a button reached the client does.
    private let recorded = Recorder()

    /// Every project `info` was asked for, in order — reads are not writes,
    /// so they are kept apart from `writes`; a test asserting a plan was read
    /// again looks here.
    private let infoRecorded = Recorder()

    /// The projects `info` has read, in order.
    public var infoReads: [String] { infoRecorded.all() }

    /// Every project `pr-status` was asked for, in order — a test of the
    /// reading's own cadence counts here.
    private let prStatusRecorded = Recorder()
    /// The projects `pr-status` has read, in order, one entry a project.
    public var prStatusReads: [String] { prStatusRecorded.all() }
    /// Every `pr-status` run, its projects joined with commas and any
    /// `--detail` after a space — one entry a run.
    private let prStatusRunRecorded = Recorder()
    public var prStatusRuns: [String] { prStatusRunRecorded.all() }
    /// Every `pr-view` of a slice's pull request, by slice ref.
    private let prViewRecorded = Recorder()
    public var prViewReads: [String] { prViewRecorded.all() }
    /// Every `session-list`, by project.
    private let sessionListRecorded = Recorder()
    public var sessionListReads: [String] { sessionListRecorded.all() }
    /// The rate limit `pr-status` reads — none unless a test says.
    private let rateLimit = Box<GitHubRateLimit?>(nil)
    /// Set while `pr-status` reads are held mid-call.
    private let prStatusHeld = Box(false)

    /// Set once a caller wants every `sliceDiff` read from here on to refuse
    /// — armed rather than counted, since a story's own setup (`AppModel`
    /// startup reads a handed-back slice's diff for its review stats before
    /// a story ever touches its `DiffStore`) would throw off any count from
    /// the client's own first call. `answering` until armed, whatever
    /// `behaviour` says otherwise, which is the point: everything else about
    /// the client stays as canned as it always was, and only the read a
    /// story is stale-testing turns over.
    private let diffFailureMessage = Box<String?>(nil)
    private let doneClearFailureMessage = Box<String?>(nil)

    public init(
        behaviour: Behaviour = .answering,
        plan: ProjectInfo = Fixtures.projectInfo,
        otherPlans: [String: ProjectInfo] = [Fixtures.secondProjectID: Fixtures.secondProjectInfo],
        agents: [AgentStatus] = Fixtures.agentStatuses,
        diff: SliceDiff = Fixtures.sliceDiff,
        pr: PRDetail = Fixtures.prGreen,
        config: ConfigDoc = Fixtures.configDoc,
        usage: UsageReading = Fixtures.usageReading,
        sessions: [Session] = Fixtures.sessions,
        details: [String: SliceDetail] = Fixtures.sliceDetails,
        plugins: PluginListing = Fixtures.pluginListing,
        sources: [SourcePlugin] = Fixtures.sourcePlugins,
        prStatus: PRStatusDoc = Fixtures.prStatusDoc,
        prStatusByProject: [String: PRStatusDoc] = [:]
    ) {
        self.prStatusDoc = prStatus
        self.prStatusByProject = Box(prStatusByProject.mapValues { Optional($0) })
        self.plugins = plugins
        self.sources = sources
        self.behaviour = behaviour
        self.planBox = Box(plan)
        self.otherPlans = otherPlans
        self.agents = agents
        self.diff = diff
        self.pr = pr
        self.config = config
        self.usageReading = usage
        self.sessionsList = sessions
        self.details = details
    }

    /// Say what the fixture project's plan reads from here on, as a write
    /// would have moved it.
    public func setPlan(_ plan: ProjectInfo) {
        planBox.set(plan)
    }

    /// Say what the workshop has proposed, as `plan-propose` would have.
    public func setProposal(_ proposal: PlanProposal?) {
        proposalBox.set(proposal)
    }

    /// Say what a project's own workshop has proposed.
    public func setProposal(_ proposal: PlanProposal?, forProject projectID: String) {
        var all = projectProposals.get()
        all[projectID] = proposal
        projectProposals.set(all)
    }

    /// Hold every Accept from now on, so the app stays mid-accept.
    public func holdAccepts() {
        acceptHangs.set(true)
    }

    /// Refuse every project Accept from now on with `message`, as nat refuses
    /// a proposal the plan has moved out from under.
    public func refuseAccepts(_ message: String) {
        acceptRefusal.set(message)
    }

    /// Hold every checks re-run and cancel from now on, so the PR section
    /// stays mid-call.
    public func holdChecksActions() {
        checksHang.set(true)
    }

    /// Hold every `pr-status` read from now on mid-call, until `releasePRStatus`.
    public func holdPRStatus() {
        prStatusHeld.set(true)
    }

    /// Let every held `pr-status` read answer.
    public func releasePRStatus() {
        prStatusHeld.set(false)
    }

    /// Hold every workshop launch from now on, so the app stays mid-launch.
    public func holdLaunches() {
        launchHangs.set(true)
    }

    /// Arms every `status` read from now on to never come back.
    public func holdStatus() {
        statusHangs.set(true)
    }

    /// The writes this client was asked to make, oldest first.
    public var writes: [String] { recorded.all() }

    /// Arms every `sliceDiff` read from now on to refuse with `message` —
    /// see `diffFailureMessage`. Called after whatever has already read
    /// successfully (an `AppModel`'s own startup included), so a story can
    /// build a stale-read notice on top of a load that is known to have
    /// landed.
    public func armDiffFailure(_ message: String) {
        diffFailureMessage.set(message)
    }

    /// The one place a read decides whether to answer or refuse, so every
    /// method below reads the same way.
    private func answer<T>(_ value: @autoclosure () -> T) async throws -> T {
        switch behaviour {
        case .answering:
            return value()
        case .refusing(let message):
            throw NatError.commandFailed(message)
        case .hanging:
            try await Self.never()
        }
    }

    private func record(_ call: String) async throws {
        switch behaviour {
        case .answering:
            recorded.append(call)
        case .refusing(let message):
            throw NatError.commandFailed(message)
        case .hanging:
            try await Self.never()
        }
    }

    /// A call that does not come back. It sleeps rather than suspending
    /// forever so a cancelled task — a view going away, a test ending — is
    /// let go of rather than leaked; the sleep's own length is past any
    /// render or any test, so nothing ever reaches the end of it.
    private static func never() async throws -> Never {
        try await Task.sleep(for: .seconds(86_400))
        throw CancellationError()
    }

    // MARK: - Reads

    public func info(projectID: String) async throws -> ProjectInfo {
        try await info(projectID: projectID, refresh: false, expand: [])
    }

    /// The source project (`Fixtures.sourceProjectID`) answers its own plan,
    /// its lazy Done group listing cards only where `expand` opens it; every
    /// other project reads as it always has.
    public func info(projectID: String, refresh: Bool, expand: [String]) async throws -> ProjectInfo {
        infoRecorded.append(projectID)
        if projectID == Fixtures.sourceProjectID, otherPlans[projectID] == nil {
            return try await answer(Fixtures.sourceProjectInfo(expand: expand))
        }
        return try await answer(otherPlans[projectID] ?? plan)
    }

    public func containerShow(projectID: String, containerID: String) async throws -> ContainerShow {
        try await answer(Fixtures.sourceContainerShow(id: containerID))
    }

    public func sourceList() async throws -> [SourcePlugin] {
        try await answer(sources)
    }

    /// A release-shaped version, so Settings ▸ About draws what a shipped
    /// app's does rather than a dev build's `devel`.
    public func natVersion() async throws -> String {
        try await answer("0.48.0")
    }

    /// The listing, with every field `source-setup` has set reading `set`
    /// from then on — as the plugin's describe would.
    public func pluginList() async throws -> PluginListing {
        let done = setUpFields.get()
        let installed = plugins.installed.map { p in
            InstalledPlugin(
                name: p.name, path: p.path, kind: p.kind, source: p.source, version: p.version, update: p.update,
                setup: p.setup.map { done.contains("\(p.name)/\($0.id)") ? $0.with(set: true) : $0 },
                describeError: p.describeError)
        }
        return try await answer(PluginListing(sources: plugins.sources, installed: installed, available: plugins.available))
    }

    /// Records the plugin and field only — never the value, which stands for
    /// a token — and answers as the Shortcut plugin does.
    public func sourceSetup(plugin: String, id: String, value: String) async throws -> PluginSetupResult {
        try await record("source-setup \(plugin) --id \(id)")
        var done = setUpFields.get()
        done.insert("\(plugin)/\(id)")
        setUpFields.set(done)
        return PluginSetupResult(message: "Logged in to scratch as Craig Scratch")
    }

    /// Records the project made — a connected plugin's section, here — and
    /// answers with an id of its own.
    public func projectCreate(name: String, repo: String?, description: String?, source: String?) async throws -> CreatedProject {
        try await record("project-create \(name)" + (source.map { " --source \($0)" } ?? ""))
        return CreatedProject(id: "f1x8500c-0000-4000-8000-\(source ?? "project")", name: name, source: source)
    }

    public func pluginInstall(name: String, source: String?, version: String?) async throws -> PluginInstalled {
        try await record("plugin-install \(name) --source \(source ?? "")")
        return PluginInstalled(
            name: name, path: "/Users/craig/.config/notion-agent-tracker/plugins/\(name)/nat-source-\(name)",
            source: source ?? "craigmjohnston/nat", version: version ?? "1.0.57",
            sha256: String(repeating: "0", count: 64), installedAt: "2026-10-03T12:00:00Z")
    }

    /// Records the uninstall; `deleteProjects` answers with the config's
    /// source projects of the plugin, by id, as deleted (nothing is).
    public func pluginUninstall(name: String, deleteProjects: Bool) async throws -> PluginUninstalled {
        try await record("plugin-uninstall \(name)" + (deleteProjects ? " --delete-projects" : ""))
        let deleted = deleteProjects
            ? config.projects.filter { $0.value.source == name }.sorted { $0.key < $1.key }
                .map { PluginUninstalled.DeletedProject(id: $0.key, name: $0.value.name) }
            : []
        return PluginUninstalled(
            name: name, path: "/Users/craig/.config/notion-agent-tracker/plugins/\(name)", projectsDeleted: deleted)
    }

    public func pluginSourceAdd(repo: String) async throws -> PluginSourceList {
        try await record("plugin-source-add \(repo)")
        return PluginSourceList(sources: plugins.sources.map(\.repo) + [repo])
    }

    public func pluginSourceRemove(repo: String) async throws -> PluginSourceList {
        try await record("plugin-source-remove \(repo)")
        return PluginSourceList(sources: plugins.sources.map(\.repo).filter { $0 != repo })
    }

    public func status() async throws -> [AgentStatus] {
        if statusHangs.get() { try await Self.never() }
        return try await answer(agents + workspaceAgents.get())
    }

    public func usage() async throws -> UsageReading {
        try await answer(usageReading)
    }

    public func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail {
        try await answer(details[sliceRef] ?? Fixtures.sourceSliceDetails[sliceRef] ?? Fixtures.sliceDetail)
    }

    public func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff {
        if let message = diffFailureMessage.get() {
            throw NatError.commandFailed(message)
        }
        // One commit of the branch is a smaller reading than the whole of it,
        // which is the difference the dropdown exists to show.
        return try await answer(commit == nil ? diff : Fixtures.smallSliceDiff)
    }

    /// A file's own lines, made up: every fixture file is 400 lines of
    /// unremarkable code, which is all an expanded gap needs to show.
    public func sliceFile(
        projectID: String, sliceRef: String, commit: String?, path: String, from: Int, to: Int?
    ) async throws -> SliceFileLines {
        let total = 400
        let last = min(to ?? total, total)
        let lines = from > last ? [] : (from...last).map { "    let unchanged\($0) = context(\($0))" }
        return try await answer(SliceFileLines(path: path, from: from, total: total, lines: lines))
    }

    public func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc {
        try await answer(Fixtures.commitsDoc)
    }

    public func prView(projectID: String, sliceRef: String) async throws -> PRDetail {
        prViewRecorded.append(sliceRef)
        return try await answer(pr)
    }

    /// Every project named reads its own doc — `prStatusByProject`'s, else
    /// the fixture's — and `detail` reads the fixture's pull request. A
    /// project set to fail fails the whole run, as one failed document does.
    public func prStatus(projectIDs: [String], detail: String?) async throws -> GitHubReading {
        for id in projectIDs { prStatusRecorded.append(id) }
        prStatusRunRecorded.append(projectIDs.joined(separator: ",") + (detail.map { " " + $0 } ?? ""))
        while prStatusHeld.get() { try await Task.sleep(for: .milliseconds(1)) }
        var projects: [String: PRStatusDoc] = [:]
        for id in projectIDs {
            if let doc = prStatusByProject.get()[id] {
                guard let doc else { throw NatError.commandFailed("gh could not be read") }
                projects[id] = doc
            } else {
                projects[id] = prStatusDoc
            }
        }
        return try await answer(GitHubReading(
            projects: projects, rateLimit: rateLimit.get(), detail: detail == nil ? nil : pr))
    }

    /// Say what rate limit `pr-status` reads from here on.
    public func setRateLimit(_ limit: GitHubRateLimit?) {
        rateLimit.set(limit)
    }

    /// Say what `pr-status` reads for one project from here on — nil, a
    /// reading that fails.
    public func setPRStatus(_ doc: PRStatusDoc?, forProject projectID: String) {
        var all = prStatusByProject.get()
        all[projectID] = .some(doc)
        prStatusByProject.set(all)
    }

    public func sliceStatus(projectID: String, sliceRef: String) async throws -> SliceStatusResult {
        try await answer(.found(status: "In progress", trashed: false))
    }

    public func configShow() async throws -> ConfigDoc {
        try await answer(config)
    }

    // MARK: - Writes

    public func sliceEdit(projectID: String, sliceRef: String, description: String) async throws -> SliceEditResult {
        try await record("slice-edit \(sliceRef)")
        return SliceEditResult(
            id: sliceRef,
            name: Fixtures.sliceDetail.name,
            url: Fixtures.sliceDetail.url,
            brief: description
        )
    }

    public func sliceLaunch(
        projectID: String, sliceRef: String, model: String?, effort: String?
    ) async throws -> LaunchResult {
        try await record("slice-launch \(sliceRef)")
        return LaunchResult(
            session: TmuxSession.name(forSlicePageID: sliceRef),
            workdir: "/Users/craig/Projects/notion-agent-tracker.worktrees/slice-fixture",
            branch: "slice/fixture",
            warning: nil
        )
    }

    public func agentSend(projectID: String, sliceRef: String, text: String) async throws {
        try await record("agent-send \(sliceRef)")
    }

    public func agentKill(projectID: String, sliceRef: String) async throws {
        try await record("agent-kill \(sliceRef)")
    }

    public func agentKillWorkshop(projectID: String) async throws {
        try await record("agent-kill --workshop")
    }

    public func sliceApprove(projectID: String, sliceRef: String) async throws -> String {
        try await record("slice-approve \(sliceRef)")
        return Fixtures.prURL
    }

    public func sliceRework(projectID: String, sliceRef: String, comments: String) async throws {
        try await record("slice-rework \(sliceRef)")
    }

    public func sliceResume(projectID: String, sliceRef: String, note: String) async throws {
        try await record("slice-resume \(sliceRef) \(note)")
    }

    public func sliceTriage(projectID: String, sliceRef: String, queue: [Int], fold: [Int], drop: [Int]) async throws -> TriageResult {
        try await record("slice-triage \(sliceRef) queue=\(queue) fold=\(fold) drop=\(drop)")
        let followUps = details[sliceRef]?.followUps ?? []
        let titles = { (indexes: [Int]) in followUps.filter { indexes.contains($0.index) }.map(\.title) }
        return TriageResult(
            queued: titles(queue).map { .init(title: $0, id: "queued-\($0.count)", url: "") },
            folded: titles(fold),
            dropped: titles(drop)
        )
    }

    public func prMerge(projectID: String, sliceRef: String) async throws {
        try await record("pr-merge \(sliceRef)")
    }

    public func prComment(projectID: String, sliceRef: String, body: String) async throws {
        try await record("pr-comment \(sliceRef)")
    }

    public func prEdit(projectID: String, sliceRef: String, body: String) async throws {
        try await record("pr-edit \(sliceRef)")
    }

    /// The fixture repository's collaborators, the pull request's own
    /// requests edited by whatever was asked — answered, not remembered.
    public func prReviewers(
        projectID: String, sliceRef: String, add: [String], remove: [String]
    ) async throws -> PRReviewers {
        if !add.isEmpty || !remove.isEmpty {
            try await record("pr-reviewers \(sliceRef) +\(add.joined(separator: ",")) -\(remove.joined(separator: ","))")
        }
        let requested = (pr.reviewRequests + add).filter { !remove.contains($0) }
        return try await answer(PRReviewers(
            pr: pr.url, requested: requested,
            candidates: Fixtures.collaborators.filter { $0 != pr.author && !requested.contains($0) }))
    }

    /// What nat would say re-running the fixture pull request's checks: a
    /// run with any check still going cancelled first (its going checks named)
    /// and re-run whole; otherwise the mode's own checks.
    public func sliceChecksRerun(projectID: String, sliceRef: String, mode: ChecksRerunMode) async throws -> ChecksActionResult {
        if checksHang.get() { try await Self.never() }
        try await record("slice-checks-rerun \(sliceRef) \(mode)")
        let actions = pr.checks.filter(\.rerunnable)
        let touched: [PRCheck] = switch mode {
        case .all: actions
        case .failed: actions.filter { checkOutcome(state: $0.state) == .failing }
        case .checks(let names): actions.filter { names.contains($0.name) }
        }
        var cancelled: [String] = []
        var rerun: [String] = []
        for run in touched.compactMap(\.run).reduce(into: [String](), { if !$0.contains($1) { $0.append($1) } }) {
            let ofRun = actions.filter { $0.run == run }
            let going = ofRun.filter { checkOutcome(state: $0.state) == .pending }
            if going.isEmpty {
                rerun += touched.filter { $0.run == run }.map(\.name)
            } else {
                cancelled += going.map(\.name)
                rerun += ofRun.map(\.name)
            }
        }
        return ChecksActionResult(cancelled: cancelled, rerun: rerun)
    }

    /// What nat would say cancelling the fixture pull request's runs still
    /// going: every check still going in each run touched.
    public func sliceChecksCancel(projectID: String, sliceRef: String, checks: [String]) async throws -> ChecksActionResult {
        if checksHang.get() { try await Self.never() }
        try await record("slice-checks-cancel \(sliceRef) \(checks)")
        let going = pr.checks.filter { $0.rerunnable && checkOutcome(state: $0.state) == .pending }
        let runs = Set(checks.isEmpty ? going.compactMap(\.run) : pr.checks.filter { checks.contains($0.name) }.compactMap(\.run))
        return ChecksActionResult(cancelled: going.filter { $0.run.map(runs.contains) ?? false }.map(\.name))
    }

    public func workshopLaunch(
        projectID: String, model: String?, effort: String?, request: String?
    ) async throws -> WorkshopLaunchResult {
        if launchHangs.get() { try await Self.never() }
        try await record("workshop-launch \(projectID)")
        return WorkshopLaunchResult(
            session: TmuxSession.planSessionName(projectID: projectID),
            workdir: "/Users/craig/Projects/notion-agent-tracker"
        )
    }

    public func workspaceLaunch(
        workspaceID: String, model: String?, effort: String?, request: String
    ) async throws -> WorkshopLaunchResult {
        if launchHangs.get() { try await Self.never() }
        try await record("workshop-launch --workspace \(workspaceID)")
        workspaceAgents.set(workspaceAgents.get() + [AgentStatus(
            sliceID: TmuxSession.planTag(projectID: workspaceID),
            session: TmuxSession.planSessionName(projectID: workspaceID),
            activity: .working
        )])
        return WorkshopLaunchResult(
            session: TmuxSession.planSessionName(projectID: workspaceID),
            workdir: "/Users/craig/.local/state/notion-agent-tracker/workspaces/\(workspaceID)"
        )
    }

    public func agentKillWorkspace(workspaceID: String) async throws {
        try await record("agent-kill --workshop --workspace \(workspaceID)")
        let tag = TmuxSession.planTag(projectID: workspaceID)
        workspaceAgents.set(workspaceAgents.get().filter { $0.sliceID != tag })
    }

    public func planProposal(workspaceID: String) async throws -> PlanProposal? {
        try await answer(proposalBox.get())
    }

    public func planProposal(projectID: String) async throws -> PlanProposal? {
        try await answer(projectProposals.get()[projectID])
    }

    public func planAccept(projectID: String) async throws -> PlanAccepted {
        if acceptHangs.get() { try await Self.never() }
        if let refusal = acceptRefusal.get() { throw NatError.commandFailed(refusal) }
        try await record("plan-accept --project \(projectID)")
        let proposal = projectProposals.get()[projectID]
        setProposal(nil, forProject: projectID)
        return PlanAccepted(
            project: ProjectEntry(id: projectID, name: (otherPlans[projectID] ?? plan).project.name),
            milestones: proposal?.milestoneCount ?? 0,
            slices: proposal?.sliceCount ?? 0
        )
    }

    public func planAccept(workspaceID: String, name: String) async throws -> PlanAccepted {
        if acceptHangs.get() { try await Self.never() }
        try await record("plan-accept --workspace \(workspaceID) --name \(name)")
        let proposal = proposalBox.get()
        proposalBox.set(nil)
        return PlanAccepted(
            project: ProjectEntry(id: Fixtures.acceptedProjectID, name: name),
            milestones: proposal?.milestoneCount ?? 0,
            slices: proposal?.sliceCount ?? 0
        )
    }

    public func notionSearch(query: String) async throws -> [NotionPlace] {
        try await record("notion-search \(query)")
        let needle = query.lowercased()
        return Fixtures.notionPlaces.filter { needle.isEmpty || $0.title.lowercased().contains(needle) }
    }

    public func projectMirror(projectID: String, parent: NotionPlace) async throws -> ProjectMirrored {
        try await record("project-mirror \(projectID) --parent \(parent.id)")
        return ProjectMirrored(
            project: ProjectEntry(id: Fixtures.mirroredProjectID, name: "rust-importer", slicesDSID: "f1x75111-ds"),
            replaced: projectID, milestones: 4, slices: 14)
    }

    public func sliceAdd(projectID: String, title: String, milestone: String, description: String?) async throws -> SliceAddResult {
        try await record("slice-add \(title)")
        return SliceAddResult(
            id: "f1x75111-0000-4000-8000-000000000099",
            name: title,
            status: "Todo",
            milestoneID: milestone,
            milestoneName: milestone,
            repo: "",
            url: "https://notion.so/f1x75111000040008000000000000099"
        )
    }

    public func sliceAdd(projectID: String, title: String, container: String, description: String?) async throws -> SliceAddResult {
        try await record("slice-add \(title) --container \(container)")
        return SliceAddResult(
            id: "f1x7500c-0000-4000-8000-000000000099", name: title, status: "Todo",
            milestoneID: container, milestoneName: container, repo: "", url: "nat://f1x7500c-0000-4000-8000-000000000099")
    }

    public func configSet(key: String, value: String) async throws {
        try await record("config-set \(key)")
    }

    /// Remembered by action and target, and answered with the message a
    /// plugin would give.
    public func sourceAction(
        projectID: String, action: String, group: String?, container: String?, input: String?
    ) async throws -> SourceActionResult {
        let target = group.map { " --group \($0)" } ?? container.map { " --container \($0)" } ?? ""
        try await record("source-action \(action)\(target)")
        return SourceActionResult(message: "Ran \(action).")
    }

    // MARK: - Scratch project

    public func scratchOpen() async throws -> ScratchOpenResult {
        try await record("scratch-open")
        return ScratchOpenResult(id: Fixtures.scratchProjectID, created: false)
    }

    public func doneClear(projectID: String) async throws -> DoneClearResult {
        try await record("done-clear \(projectID)")
        if let message = doneClearFailureMessage.get() {
            throw NatError.commandFailed(message)
        }
        return DoneClearResult()
    }

    /// Arms every `doneClear` from here on to refuse, for a test that checks
    /// the launch survives a failed clear.
    public func armDoneClearFailure(_ message: String) {
        doneClearFailureMessage.set(message)
    }

    // MARK: - Ad hoc sessions

    public func sessionLaunch(
        projectID: String, dir: String?, model: String?, effort: String?
    ) async throws -> SessionLaunchResult {
        try await record("session-launch \(dir ?? "")")
        let id = "f1x75e55-0000-4000-8000-000000000001"
        return SessionLaunchResult(
            session: "nat-session-f1x75e55",
            tag: "session:\(projectID):\(id)",
            id: id,
            dir: dir ?? Fixtures.configDoc.projects[projectID]?.workingDir ?? "",
            branch: "session/f1x75e55"
        )
    }

    public func run(projectID: String, sliceRef: String?, label: String?) async throws -> RunResult {
        try await record("run \(sliceRef ?? "") \(label ?? "")")
        let runs = Fixtures.runs
        let offered = sliceRef == nil ? runs.globalRuns : runs.sliceRuns
        let run = offered.first { $0.label == label } ?? offered.first ?? RunCommand(label: label ?? "Run", command: "make run")
        return RunResult(
            session: "nat-run-f1x7ure5-\(run.label.lowercased())", label: run.label, command: run.command,
            dir: sliceRef == nil ? "/repos/nat.worktrees/run-main" : "/repos/nat.worktrees/slice-\(sliceRef ?? "")")
    }

    public func sessionList(projectID: String) async throws -> [Session] {
        sessionListRecorded.append(projectID)
        return try await answer(sessionsList)
    }

    public func sessionStatus(projectID: String, sessionID: String, discard: Bool) async throws -> SessionStatusDoc {
        let session = sessionsList.first { $0.id == sessionID } ?? Fixtures.liveSession
        try await record("session-status \(sessionID)")
        return SessionStatusDoc(
            id: session.id,
            live: session.live,
            ended: session.ended,
            dir: session.dir,
            branch: session.branch,
            branches: Fixtures.sessionBranchNames(for: session).enumerated().map { index, branch in
                // One pull request to a branch, in order; a session with
                // fewer branches than pull requests keeps the rest on its last.
                let names = Fixtures.sessionBranchNames(for: session)
                let prs = session.prs.enumerated().filter { min($0.offset, names.count - 1) == index }.map(\.element)
                return SessionBranchStatus(branch: branch, stale: false, prs: prs)
            }
        )
    }

    /// The fixture pull request, made over as the one asked for: the same
    /// checks, reviews and conversation under the number, title, state and URL
    /// the session's own listing gave it.
    public func sessionPRView(projectID: String, sessionID: String, prURL: String) async throws -> PRDetail {
        let listed = sessionsList.flatMap(\.prs).first { $0.url == prURL }
        return try await answer(PRDetail(
            number: listed?.number ?? pr.number,
            title: listed?.title ?? pr.title,
            body: pr.body,
            state: listed?.state ?? pr.state,
            isDraft: false,
            author: pr.author,
            baseRefName: pr.baseRefName,
            headRefName: pr.headRefName,
            url: prURL,
            checks: pr.checks,
            reviews: pr.reviews,
            comments: pr.comments,
            reviewDecision: pr.reviewDecision,
            mergeable: pr.mergeable,
            mergeStateStatus: pr.mergeStateStatus,
            additions: pr.additions,
            deletions: pr.deletions,
            changedFiles: pr.changedFiles,
            commits: pr.commits
        ))
    }

    public func sessionDiff(projectID: String, sessionID: String, branch: String?) async throws -> SliceDiff {
        try await answer(Fixtures.sliceDiff)
    }
}

/// The recorded writes, behind a lock so the client stays `Sendable` without
/// being an actor — every method of `NatClientProtocol` is `async` and a
/// store may well call two at once.
private final class Recorder: @unchecked Sendable {
    private let lock = NSLock()
    private var calls: [String] = []

    func append(_ call: String) {
        lock.lock()
        defer { lock.unlock() }
        calls.append(call)
    }

    func all() -> [String] {
        lock.lock()
        defer { lock.unlock() }
        return calls
    }
}

/// A single value behind a lock — `diffFailureMessage`'s own storage, set
/// once and read on every `sliceDiff` call after.
private final class Box<T>: @unchecked Sendable {
    private let lock = NSLock()
    private var value: T

    init(_ value: T) {
        self.value = value
    }

    func get() -> T {
        lock.lock()
        defer { lock.unlock() }
        return value
    }

    func set(_ newValue: T) {
        lock.lock()
        defer { lock.unlock() }
        value = newValue
    }
}

extension Fixtures {
    /// `nat config show --json` for the fixture config.
    public static var configDoc: ConfigDoc {
        ConfigDoc(
            agentSplitPercent: 45,
            pollSeconds: 3600,
            workshopAgent: AgentModel(model: "sonnet", effort: nil),
            sliceAgent: AgentModel(model: "opus", effort: "high"),
            projects: [
                projectID: ConfigDocProject(
                    name: "notion-agent-tracker",
                    workingDir: "/Users/craig/Projects/notion-agent-tracker"
                ),
            ]
        )
    }

    /// The same config with the slice agent's model set to a full model ID
    /// rather than one of `AgentOptions`' own aliases — what the Settings
    /// Agents story shows the model picker's Custom state over.
    public static var configDocWithCustomModel: ConfigDoc {
        let doc = configDoc
        return ConfigDoc(
            agentSplitPercent: doc.agentSplitPercent,
            pollSeconds: doc.pollSeconds,
            workshopAgent: doc.workshopAgent,
            sliceAgent: AgentModel(model: "claude-sonnet-5", effort: "high"),
            projects: doc.projects
        )
    }
}
