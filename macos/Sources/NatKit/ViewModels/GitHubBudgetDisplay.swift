import Foundation

/// The status bar's GitHub readout, from the last reading's budget: nothing
/// while healthy; once nat throttles polling, what is left, with the
/// projection and the reset as its tooltip; once GitHub refused and nat
/// paused polling, when it resets.
public enum GitHubBudgetReadout: Equatable, Sendable {
    case healthy
    case throttled(remaining: Int, limit: Int, projected: Int?, resetAt: Date)
    case paused(until: Date)

    public init(_ rateLimit: GitHubRateLimit?) {
        guard let rateLimit else {
            self = .healthy
            return
        }
        if let until = rateLimit.pausedUntil {
            self = .paused(until: until)
        } else if rateLimit.throttled {
            self = .throttled(
                remaining: rateLimit.remaining, limit: rateLimit.limit,
                projected: rateLimit.projectedRemainingAtReset, resetAt: rateLimit.resetAt)
        } else {
            self = .healthy
        }
    }

    /// The clause the bar draws — nil while healthy.
    public func text(now: Date, calendar: Calendar = .current) -> String? {
        switch self {
        case .healthy:
            return nil
        case .throttled(let remaining, _, _, _):
            return "GitHub \u{00B7} \(remaining) left"
        case .paused(let until):
            return "GitHub limit \u{00B7} resets \(budgetClock(until, now: now, calendar: calendar))"
        }
    }

    /// The clause's tooltip.
    public func tooltip(now: Date, calendar: Calendar = .current) -> String? {
        switch self {
        case .healthy:
            return nil
        case .throttled(let remaining, let limit, let projected, let resetAt):
            let reset = budgetClock(resetAt, now: now, calendar: calendar)
            var text = "GitHub polling slowed to keep a reserve for actions: \(remaining) of \(limit) points left"
            if let projected { text += ", \(projected) projected at the reset at \(reset)" } else { text += ", resets \(reset)" }
            return text
        case .paused(let until):
            return "GitHub refused on its API limit; polling paused until "
                + "\(budgetClock(until, now: now, calendar: calendar)). Actions still run."
        }
    }

    /// The status bar's words for a throttled or paused reading — what
    /// Diagnostics adds after the value — nil while healthy.
    public var stateWord: String? {
        switch self {
        case .healthy: nil
        case .throttled: "throttled"
        case .paused: "paused"
        }
    }
}

/// A time as the budget's lines say it: today's time alone, `13:46`, with the
/// date before it where it is not today, `7 Oct 13:46`.
public func budgetClock(_ date: Date, now: Date, calendar: Calendar = .current) -> String {
    let formatter = DateFormatter()
    formatter.locale = Locale(identifier: "en_GB")
    formatter.calendar = calendar
    formatter.timeZone = calendar.timeZone
    formatter.dateFormat = calendar.isDate(date, inSameDayAs: now) ? "HH:mm" : "d MMM HH:mm"
    return formatter.string(from: date)
}

/// Settings ▸ About's Diagnostics rows, each a label and a value.
public enum DiagnosticsFormat {
    /// `GitHub budget`: the last reading's remaining of its limit and when it
    /// resets, the state after it where throttled or paused; before any
    /// reading this session, "no reading yet".
    public static func budget(_ rateLimit: GitHubRateLimit?, now: Date, calendar: Calendar = .current) -> String {
        guard let rateLimit else { return "no reading yet" }
        var line = "\(rateLimit.remaining) / \(rateLimit.limit) \u{00B7} resets "
            + budgetClock(rateLimit.resetAt, now: now, calendar: calendar)
        if let word = GitHubBudgetReadout(rateLimit).stateWord { line += " \u{00B7} \(word)" }
        return line
    }

    /// `gnat's usage this session`: `37 points · 29 readings · 8 actions`.
    public static func usage(points: Int, readings: Int, actions: Int) -> String {
        [count(points, "point"), count(readings, "reading"), count(actions, "action")].joined(separator: " \u{00B7} ")
    }

    /// `Session length`: `14m` under an hour, `2h 14m` under a day, `3d 2h`
    /// past one.
    public static func sessionLength(from launch: Date, to now: Date) -> String {
        let minutes = max(0, Int(now.timeIntervalSince(launch)) / 60)
        let days = minutes / (24 * 60)
        let hours = minutes / 60 % 24
        if days > 0 { return "\(days)d \(hours)h" }
        if hours > 0 { return "\(hours)h \(minutes % 60)m" }
        return "\(minutes)m"
    }

    private static func count(_ n: Int, _ noun: String) -> String {
        "\(n) \(noun)\(n == 1 ? "" : "s")"
    }
}
