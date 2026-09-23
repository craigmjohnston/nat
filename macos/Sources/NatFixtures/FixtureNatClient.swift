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
    private let plan: ProjectInfo
    private let agents: [AgentStatus]
    private let diff: SliceDiff
    private let pr: PRDetail
    private let config: ConfigDoc
    private let usageReading: UsageReading
    private let sessionsList: [Session]

    /// Every write this client was asked to make, in order — a preview never
    /// looks, and a test asserting that a button reached the client does.
    private let recorded = Recorder()

    /// Set once a caller wants every `sliceDiff` read from here on to refuse
    /// — armed rather than counted, since a story's own setup (`AppModel`
    /// startup reads a handed-back slice's diff for its review stats before
    /// a story ever touches its `DiffStore`) would throw off any count from
    /// the client's own first call. `answering` until armed, whatever
    /// `behaviour` says otherwise, which is the point: everything else about
    /// the client stays as canned as it always was, and only the read a
    /// story is stale-testing turns over.
    private let diffFailureMessage = Box<String?>(nil)

    public init(
        behaviour: Behaviour = .answering,
        plan: ProjectInfo = Fixtures.projectInfo,
        agents: [AgentStatus] = Fixtures.agentStatuses,
        diff: SliceDiff = Fixtures.sliceDiff,
        pr: PRDetail = Fixtures.prGreen,
        config: ConfigDoc = Fixtures.configDoc,
        usage: UsageReading = Fixtures.usageReading,
        sessions: [Session] = Fixtures.sessions
    ) {
        self.behaviour = behaviour
        self.plan = plan
        self.agents = agents
        self.diff = diff
        self.pr = pr
        self.config = config
        self.usageReading = usage
        self.sessionsList = sessions
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
        try await answer(plan)
    }

    public func status() async throws -> [AgentStatus] {
        try await answer(agents)
    }

    public func usage() async throws -> UsageReading {
        try await answer(usageReading)
    }

    public func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail {
        try await answer(Fixtures.sliceDetails[sliceRef] ?? Fixtures.sliceDetail)
    }

    public func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff {
        if let message = diffFailureMessage.get() {
            throw NatError.commandFailed(message)
        }
        // One commit of the branch is a smaller reading than the whole of it,
        // which is the difference the dropdown exists to show.
        return try await answer(commit == nil ? diff : Fixtures.smallSliceDiff)
    }

    public func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc {
        try await answer(Fixtures.commitsDoc)
    }

    public func prView(projectID: String, sliceRef: String) async throws -> PRDetail {
        try await answer(pr)
    }

    public func prStatus(projectID: String) async throws -> PRStatusDoc {
        try await answer(Fixtures.prStatusDoc)
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

    public func prMerge(projectID: String, sliceRef: String) async throws {
        try await record("pr-merge \(sliceRef)")
    }

    public func prComment(projectID: String, sliceRef: String, body: String) async throws {
        try await record("pr-comment \(sliceRef)")
    }

    public func workshopLaunch(
        projectID: String, model: String?, effort: String?, request: String?
    ) async throws -> WorkshopLaunchResult {
        try await record("workshop-launch \(projectID)")
        return WorkshopLaunchResult(
            session: TmuxSession.planSessionName(projectID: projectID),
            workdir: "/Users/craig/Projects/notion-agent-tracker",
            wishlist: false
        )
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

    public func configSet(key: String, value: String) async throws {
        try await record("config-set \(key)")
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

    public func sessionList(projectID: String) async throws -> [Session] {
        try await answer(sessionsList)
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
            branches: [SessionBranchStatus(branch: session.branch, stale: false, prs: session.prs)]
        )
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
