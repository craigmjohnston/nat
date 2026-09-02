import Foundation

/// Determines if a slice can be launched and helps build launch flags.
public struct LaunchPlan: Equatable, Sendable {
    public let canLaunch: Bool
    public let blockedBy: [String]?

    /// Initialize from a slice, determining launchability and blockers if any.
    public init(for slice: Slice, hasLiveAgent: Bool) {
        // A slice is launchable if:
        // - Status is Todo (not started), OR
        // - Status is In progress AND no live agent is running
        // AND it is not blocked by dependencies

        let isNotBlocked = !slice.blocked
        let isLaunchableStatus = (slice.status == "Todo") || (slice.status == "In progress" && !hasLiveAgent)

        self.canLaunch = isNotBlocked && isLaunchableStatus
        self.blockedBy = slice.blocked ? slice.dependsOn : nil
    }

    /// Build command-line flags for model and effort.
    ///
    /// Returns an array of arguments that should be appended to the nat command.
    /// If both model and effort are nil, returns empty array.
    public static func buildFlags(model: String?, effort: String?) -> [String] {
        var flags: [String] = []
        if let model = model, !model.isEmpty, model != "Default" {
            flags.append(contentsOf: ["--model", model])
        }
        if let effort = effort, !effort.isEmpty, effort != "Default" {
            flags.append(contentsOf: ["--effort", effort])
        }
        return flags
    }
}
