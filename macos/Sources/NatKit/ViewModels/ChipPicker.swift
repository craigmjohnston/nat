import Foundation

/// Where a pull request stands, in the attention vocabulary the rail and the
/// project tabs already speak: open, merged, closed. A state this build does
/// not recognise reads as open, the word for a pull request that is neither
/// of the other two — `prStateChip`'s own rule.
public enum PickerChipState: Equatable, Sendable {
    case open, merged, closed

    public init(prState: String) {
        switch prState {
        case PRLifecycleState.merged: self = .merged
        case PRLifecycleState.closed: self = .closed
        default: self = .open
        }
    }

    public var word: String {
        switch self {
        case .open: return "open"
        case .merged: return "merged"
        case .closed: return "closed"
        }
    }
}

/// One choice in a `ChipPicker`: what it is called, and whatever it says
/// besides — a pull request's number and state, or a branch's "checked out"
/// mark. `id` is what a selection is remembered by, so it is the pull
/// request's URL or the branch's name rather than anything drawn.
public struct PickerChip: Equatable, Identifiable, Sendable {
    public let id: String
    /// The short lead-in ahead of the title — a pull request's "#12".
    public let lead: String?
    public let title: String
    public let state: PickerChipState?
    /// A word marking the chip as the default choice — "checked out" — or nil.
    public let mark: String?

    public init(id: String, lead: String? = nil, title: String, state: PickerChipState? = nil, mark: String? = nil) {
        self.id = id
        self.lead = lead
        self.title = title
        self.state = state
        self.mark = mark
    }
}

/// The picker's whole reading: which chips it draws and which is selected.
/// One choice is no choice, so a picker of fewer than two chips is not drawn
/// at all.
public struct ChipPickerModel: Equatable, Sendable {
    public let chips: [PickerChip]
    public let selectedID: String?

    public init(chips: [PickerChip], selectedID: String?) {
        self.chips = chips
        self.selectedID = selectedID
    }

    public var isVisible: Bool { chips.count > 1 }
}

/// How many characters of a pull request's title its chip keeps, so a row of
/// three fits a pane.
public let pickerTitleLimit = 28

/// `title` cut to `limit` characters with an ellipsis where it was cut.
public func truncatedPickerTitle(_ title: String, limit: Int = pickerTitleLimit) -> String {
    let trimmed = title.trimmingCharacters(in: .whitespacesAndNewlines)
    guard trimmed.count > limit else { return trimmed }
    return String(trimmed.prefix(max(limit - 1, 1))).trimmingCharacters(in: .whitespaces) + "…"
}

/// The chips for a session's pull requests, in the order `session-status`
/// reported them.
public func pullRequestChips(_ prs: [SessionPR]) -> [PickerChip] {
    prs.map {
        PickerChip(
            id: $0.url,
            lead: "#\($0.number)",
            title: truncatedPickerTitle($0.title),
            state: PickerChipState(prState: $0.state)
        )
    }
}

/// The chips for a session's branches. The branch checked out now is the
/// default and says so; `checkedOut` is nil where the worktree's branch could
/// not be read, which leaves every chip unmarked.
public func branchChips(_ branches: [String], checkedOut: String?) -> [PickerChip] {
    branches.map {
        PickerChip(id: $0, title: $0, mark: $0 == checkedOut ? "checked out" : nil)
    }
}

/// What a session's picker choices are remembered by for the app session:
/// one selection per session per picker. A chosen id that is no longer among
/// the options — the branch was deleted, the pull request fell off the list —
/// falls back to the default rather than selecting nothing.
public struct PickerSelectionMemory: Equatable, Sendable {
    private var chosen: [String: String] = [:]

    public init() {}

    public mutating func select(_ id: String, for key: String) {
        chosen[key] = id
    }

    public mutating func forget(_ key: String) {
        chosen[key] = nil
    }

    /// The id to show selected: the remembered one while it is still an
    /// option, else `defaultID`, else the first option, else nil.
    public func resolved(for key: String, among ids: [String], defaultID: String? = nil) -> String? {
        if let remembered = chosen[key], ids.contains(remembered) { return remembered }
        if let defaultID, ids.contains(defaultID) { return defaultID }
        return ids.first
    }
}

/// The memory key for a session's PR picker or branch picker.
public enum SessionPicker: String, Sendable {
    case pullRequest
    case branch

    public func key(sessionID: String) -> String { "\(rawValue):\(sessionID)" }
}

/// What the pane stepper says of a session's PR stage: the count of pull
/// requests still open as its badge (none when none are), and whether it is
/// complete — green only once there is a pull request and every one of them
/// has merged.
public struct SessionPRStage: Equatable, Sendable {
    public let openCount: Int
    public let isComplete: Bool

    public init(prs: [SessionPR]) {
        openCount = prs.filter(\.isOpen).count
        isComplete = !prs.isEmpty && prs.allSatisfy { PickerChipState(prState: $0.state) == .merged }
    }
}

/// The rail row's own words for a session's pull requests when it has more
/// than one — "3 PRs · 1 open" — and nil with one or none, where the row says
/// nothing extra.
public func sessionPRSummary(_ prs: [SessionPR]) -> String? {
    guard prs.count > 1 else { return nil }
    let open = prs.filter(\.isOpen).count
    return "\(prs.count) PRs · \(open) open"
}
