import Foundation

/// Determines if a slice can be launched and helps build launch flags.
public struct LaunchPlan: Equatable, Sendable {
    public let canLaunch: Bool
    public let blockedBy: [String]?

    /// Initialize from a slice, determining launchability and blockers if any.
    ///
    /// A slice is launchable with no live agent on it when it is Todo, or in
    /// progress — a relaunch, a PR recorded or not (one resumed with no
    /// session to tell is launched again on the work so far). A live agent
    /// refuses either; so does a Done slice, as `nat slice-launch` does.
    /// Dependencies hold either back, as nat's own launch does.
    public init(for slice: Slice, hasLiveAgent: Bool) {
        let isLaunchableStatus = (slice.status == "Todo") || (slice.status == "In progress" && !hasLiveAgent)

        self.canLaunch = isLaunchableStatus && !slice.blocked
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
