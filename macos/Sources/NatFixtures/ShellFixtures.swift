import Foundation
import NatKit

// The states the whole window has that no single pane's fixture covers: the
// planning agent behind the workshop pane, the first-run checklist's reading
// of the machine, and the review left pending on a diff.

extension Fixtures {
    /// The activity poll's reading with a planning agent in it as well as the
    /// two slice agents — keyed by the project-qualified plan tag, exactly as
    /// `AppModel.planningAgentKey` looks one up.
    ///
    /// A separate reading rather than an addition to `agentStatuses`, because
    /// most states of the board have no workshop session running and the rail
    /// draws no WORKSHOP section at all for them.
    public static var agentStatusesWithPlanner: [AgentStatus] {
        agentStatuses + [planningAgentStatus]
    }

    /// The active project's planning agent, working.
    public static var planningAgentStatus: AgentStatus {
        AgentStatus(
            sliceID: TmuxSession.planTag(projectID: projectID),
            session: TmuxSession.planSessionName(projectID: projectID),
            activity: .working
        )
    }

    /// What the onboarding checklist reads on a machine with the whole
    /// toolchain installed — pinned rather than looked up, so a first-run
    /// pane drawn here says the same thing on every machine.
    public static let toolsFound: [String: BinaryLocator.Status] = [
        "nat": .found("/opt/homebrew/bin/nat"),
        "tmux": .found("/opt/homebrew/bin/tmux"),
        "gh": .found("/opt/homebrew/bin/gh"),
        "ntn": .found("/opt/homebrew/bin/ntn"),
    ]

    /// The same machine before `nat` itself is installed — the pane's other
    /// shape, where there is nothing for the "+" sheet to run and the only
    /// way forward is a terminal.
    public static var toolsWithoutNat: [String: BinaryLocator.Status] {
        var statuses = toolsFound
        statuses["nat"] = .missing
        statuses["gh"] = .missing
        return statuses
    }

    /// What one of those maps answers for a binary it does not name, so a
    /// checklist that grows a fifth row still has something to draw.
    public static func toolStatus(
        _ binary: String, in statuses: [String: BinaryLocator.Status]
    ) -> BinaryLocator.Status {
        statuses[binary] ?? .missing
    }
}

extension Fixtures {
    /// Leaves `pendingComments` on a store that has already read the branch —
    /// the review a user has typed and not yet sent, which is a state of the
    /// store rather than of any reading and so cannot be canned as a value.
    ///
    /// The comments are set through `setComment` rather than assigned, so
    /// what lands is exactly what the diff pane's own comment box would have
    /// left there, sorting and all.
    @MainActor
    public static func seedPendingComments(into store: DiffStore) {
        for comment in pendingComments {
            store.setComment(
                path: comment.path, anchorRowIDs: comment.anchorRowIDs, text: comment.text)
        }
    }
}
