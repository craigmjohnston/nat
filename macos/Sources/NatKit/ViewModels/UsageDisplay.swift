import Foundation

/// A limit at or above this share of its window renders in the warning
/// tint — both its percent and its reset together, since the two are read as
/// one clause.
public let usageWarningThreshold = 80.0

/// One window's rendered clause — "Session 38% · resets 6:00 PM" — and
/// whether it crossed the warning threshold, which is drawn as one tint
/// across the whole clause.
public struct UsageWindowDisplay: Equatable {
    public let text: String
    public let warning: Bool

    public init(text: String, warning: Bool) {
        self.text = text
        self.warning = warning
    }
}

/// What the status bar's usage readout draws: zero, one or two window
/// clauses, in `five_hour`-then-`seven_day` order. Empty draws nothing at
/// all, per the brief's "if usage cannot be read, show nothing rather than
/// an error."
public struct UsageDisplay: Equatable {
    public let windows: [UsageWindowDisplay]

    public init(windows: [UsageWindowDisplay]) {
        self.windows = windows
    }

    public var isEmpty: Bool { windows.isEmpty }
}

/// Builds the status bar's usage display from a reading, dropping any window
/// whose `resetsAt` has already passed at `now` — a last-known reading holds
/// until its own reset, matching `/usage`'s own behaviour, and a window read
/// as expired is exactly as absent as one the payload never carried at all.
public func buildUsageDisplay(
    from reading: UsageReading?,
    now: Date = Date(),
    timeZone: TimeZone = .current
) -> UsageDisplay {
    guard let reading else { return UsageDisplay(windows: []) }

    var windows: [UsageWindowDisplay] = []
    if let fiveHour = reading.fiveHour, fiveHour.resetsAt > now {
        windows.append(windowDisplay(label: "Session", limit: fiveHour, resetFormat: usageTimeFormatter(timeZone: timeZone)))
    }
    if let sevenDay = reading.sevenDay, sevenDay.resetsAt > now {
        windows.append(windowDisplay(label: "Week", limit: sevenDay, resetFormat: usageWeekdayFormatter(timeZone: timeZone)))
    }
    return UsageDisplay(windows: windows)
}

private func windowDisplay(label: String, limit: UsageRateLimit, resetFormat: DateFormatter) -> UsageWindowDisplay {
    let percent = Int(limit.usedPercentage.rounded())
    let text = "\(label) \(percent)% · resets \(resetFormat.string(from: limit.resetsAt))"
    return UsageWindowDisplay(text: text, warning: limit.usedPercentage >= usageWarningThreshold)
}

/// "6:00 PM" — the five-hour window's own reset, precise to the minute since
/// it is always today or the next few hours.
private func usageTimeFormatter(timeZone: TimeZone) -> DateFormatter {
    let formatter = DateFormatter()
    formatter.dateFormat = "h:mm a"
    formatter.timeZone = timeZone
    formatter.locale = Locale(identifier: "en_US_POSIX")
    return formatter
}

/// "Tue" — the seven-day window's own reset, named by weekday rather than
/// clock time since it is always a few days out.
private func usageWeekdayFormatter(timeZone: TimeZone) -> DateFormatter {
    let formatter = DateFormatter()
    formatter.dateFormat = "EEE"
    formatter.timeZone = timeZone
    formatter.locale = Locale(identifier: "en_US_POSIX")
    return formatter
}
