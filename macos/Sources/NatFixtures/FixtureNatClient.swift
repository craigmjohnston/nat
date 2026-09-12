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

    /// Every write this client was asked to make, in order — a preview never
    /// looks, and a test asserting that a button reached the client does.
    private let recorded = Recorder()

    public init(
        behaviour: Behaviour = .answering,
        plan: ProjectInfo = Fixtures.projectInfo,
        agents: [AgentStatus] = Fixtures.agentStatuses,
        diff: SliceDiff = Fixtures.sliceDiff,
        pr: PRDetail = Fixtures.prGreen
    ) {
        self.behaviour = behaviour
        self.plan = plan
        self.agents = agents
        self.diff = diff
        self.pr = pr
    }

    /// The writes this client was asked to make, oldest first.
    public var writes: [String] { recorded.all() }

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

    public func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail {
        try await answer(Fixtures.sliceDetails[sliceRef] ?? Fixtures.sliceDetail)
    }

    public func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff {
        // One commit of the branch is a smaller reading than the whole of it,
        // which is the difference the dropdown exists to show.
        try await answer(commit == nil ? diff : Fixtures.smallSliceDiff)
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

    public func configShow() async throws -> ConfigDoc {
        try await answer(Fixtures.configDoc)
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

    public func sliceLaunch(projectID: String, sliceRef: String, model: String?, effort: String?) async throws -> LaunchResult {
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
}
