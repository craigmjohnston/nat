import Foundation

/// Determines if a slice can be launched and helps build launch flags.
public struct LaunchPlan: Equatable, Sendable {
    public let canLaunch: Bool
    public let blockedBy: [String]?
    /// Whether the launch is a fix session: the slice is approved — in
    /// progress, its pull request recorded — so `nat slice-launch` sends the
    /// agent at the review rather than the brief (`actions.FixLaunch`).
    public let isFix: Bool

    /// Initialize from a slice, determining launchability and blockers if any.
    ///
    /// A slice is launchable with no live agent on it when it is Todo, or in
    /// progress — a relaunch, or a fix launch on an approved one. A live agent
    /// refuses either. Dependencies hold back work not yet out; a fix launch
    /// waits on the review alone, as nat's own does. Whether the pull request
    /// is still open is nat's to ask gh at launch.
    public init(for slice: Slice, hasLiveAgent: Bool) {
        let isFix = slice.status == "In progress" && !slice.pr.isEmpty
        let isLaunchableStatus = (slice.status == "Todo") || (slice.status == "In progress" && !hasLiveAgent)

        self.isFix = isFix
        self.canLaunch = isLaunchableStatus && (isFix || !slice.blocked)
        self.blockedBy = slice.blocked && !isFix ? slice.dependsOn : nil
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
