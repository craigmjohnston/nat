import Foundation

/// Whether one re-run or cancel control is enabled, and the other checks
/// acting on it will stop: GitHub cancels whole runs, so a check's run still
/// going stops every sibling of it still queued or running.
public struct CheckControl: Equatable, Sendable {
    public let enabled: Bool
    public let stops: [String]

    public init(enabled: Bool, stops: [String] = []) {
        self.enabled = enabled
        self.stops = stops
    }
}

/// The PR section's re-run and cancel controls over a pull request's checks:
/// which are there, which enabled, and what each tooltip names — decided here
/// from the checks' states, `rerunnable` and `run`, so the view only draws.
///
/// nat cancels a run still going before it re-runs it, so nothing is disabled
/// for running; what is, is a check no Actions run is behind, and one that has
/// not started.
public struct ChecksControls: Equatable, Sendable {
    public let checks: [PRCheck]

    public init(checks: [PRCheck]) {
        self.checks = checks
    }

    /// GitHub's word for a check whose job a runner is working on.
    static let running = "IN_PROGRESS"

    /// Whether the check has finished (in any outcome) or is running —
    /// anything but queued and waiting.
    static func started(_ check: PRCheck) -> Bool {
        checkOutcome(state: check.state) != .pending || isRunning(check)
    }

    static func isRunning(_ check: PRCheck) -> Bool {
        check.state.trimmingCharacters(in: .whitespaces).uppercased() == running
    }

    static func isGoing(_ check: PRCheck) -> Bool {
        checkOutcome(state: check.state) == .pending
    }

    /// Whether the heading carries its two buttons at all.
    public var hasControls: Bool { checks.contains(where: \.rerunnable) }

    /// The checks acting on `check` would stop besides itself: its run's
    /// others still queued or running.
    public func siblingsStopped(by check: PRCheck) -> [String] {
        guard let run = check.run else { return [] }
        return checks.filter { $0.run == run && $0.name != check.name && Self.isGoing($0) }.map(\.name)
    }

    /// A row's re-run: once its job has finished or while it runs.
    public func rerun(_ check: PRCheck) -> CheckControl {
        let enabled = check.rerunnable && Self.started(check)
        return CheckControl(enabled: enabled, stops: enabled ? siblingsStopped(by: check) : [])
    }

    /// A row's cancel: only while its job is queued or running.
    public func cancel(_ check: PRCheck) -> CheckControl {
        let enabled = check.rerunnable && Self.isGoing(check)
        return CheckControl(enabled: enabled, stops: enabled ? siblingsStopped(by: check) : [])
    }

    /// The heading's re-run: disabled only while no re-runnable job has run
    /// and none is running.
    public var rerunAll: Bool {
        checks.contains { $0.rerunnable && Self.started($0) }
    }

    /// Re-run failed, in the heading's menu: only once a job has failed.
    public var rerunFailed: Bool {
        checks.contains { $0.rerunnable && checkOutcome(state: $0.state) == .failing }
    }

    /// The heading's cancel: only while a job is queued or running.
    public var cancelAll: Bool {
        checks.contains { $0.rerunnable && Self.isGoing($0) }
    }

    /// A row's re-run tooltip.
    public func rerunHelp(_ check: PRCheck) -> String {
        let control = rerun(check)
        guard check.rerunnable else { return "\(check.name) has no GitHub Actions run to re-run" }
        guard control.enabled else { return "\(check.name) has not started" }
        if Self.isGoing(check) || !control.stops.isEmpty {
            return "Cancel and re-run \(check.name)" + stopsClause(control.stops)
        }
        return "Re-run \(check.name)"
    }

    /// A row's cancel tooltip.
    public func cancelHelp(_ check: PRCheck) -> String {
        let control = cancel(check)
        guard check.rerunnable else { return "\(check.name) has no GitHub Actions run to cancel" }
        guard control.enabled else { return "\(check.name) is not running" }
        return "Cancel \(check.name)" + stopsClause(control.stops)
    }

    private func stopsClause(_ stops: [String]) -> String {
        stops.isEmpty ? "" : " — also stops \(listed(stops)), which share its run"
    }
}

/// What a re-run or cancel did, as the PR section's notice says it:
/// "Cancelled a and b, then re-ran a, b and c".
public func checksActionNotice(_ result: ChecksActionResult) -> String {
    var parts: [String] = []
    if !result.cancelled.isEmpty { parts.append("Cancelled \(listed(result.cancelled))") }
    if !result.rerun.isEmpty {
        parts.append(parts.isEmpty ? "Re-ran \(listed(result.rerun))" : "then re-ran \(listed(result.rerun))")
    }
    var text = parts.joined(separator: ", ")
    if !result.skipped.isEmpty {
        let skipped = "skipped \(listed(result.skipped)), which GitHub Actions does not run"
        text = text.isEmpty ? skipped.prefix(1).uppercased() + skipped.dropFirst() : text + "; " + skipped
    }
    return text.isEmpty ? "Nothing to do." : text + "."
}

/// Names in prose: "a", "a and b", "a, b and c".
func listed(_ names: [String]) -> String {
    switch names.count {
    case 0: return ""
    case 1: return names[0]
    default: return names.dropLast().joined(separator: ", ") + " and " + names[names.count - 1]
    }
}
