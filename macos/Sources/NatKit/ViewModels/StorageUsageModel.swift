import Foundation

/// Settings ▸ GitHub's state: `nat storage-usage` as last read, read once
/// when the section is first shown and again on the refresh button — never
/// polled, and nothing to do with the GraphQL budget. Everything the section
/// draws — the bar's segments, the legend's rows, the summary — is worked
/// out here from nat's reading; the view only draws it.
@MainActor
@Observable
public final class StorageUsageModel {
    /// Where the reading stands.
    public enum State: Equatable, Sendable {
        case loading
        case loaded(StorageUsage)
        /// gh lacks the "user" scope the billing report needs: the command
        /// that grants it, for the user to run — or not.
        case needsScope(command: String)
        /// nat's refusal, in its words — gh not signed in, the scope
        /// missing, GitHub unreachable.
        case failed(String)
    }

    public private(set) var state: State = .loading

    /// A read is out — the refresh button's spinner.
    public private(set) var isReading = false

    /// Whether any read has been asked for, so a section shown again keeps
    /// what it read.
    @ObservationIgnored private var hasRead = false
    @ObservationIgnored private let client: NatClientProtocol

    public init(client: NatClientProtocol) {
        self.client = client
    }

    /// The first read, only once.
    public func loadIfNeeded() async {
        guard !hasRead else { return }
        await refresh()
    }

    /// Reads again. A reading already on screen stays there while the new one
    /// is out; a failure replaces it, since a stale figure under a refresh
    /// that failed would read as current.
    public func refresh() async {
        guard !isReading else { return }
        hasRead = true
        isReading = true
        defer { isReading = false }
        do {
            switch try await client.storageUsage() {
            case .reading(let usage): state = .loaded(usage)
            case .needsScope(let command): state = .needsScope(command: command)
            }
        } catch {
            if case NatError.commandFailed(let message) = error {
                state = .failed(message)
            } else {
                state = .failed(error.localizedDescription)
            }
        }
    }

    // MARK: - What the section draws

    /// One stretch of the bar: a project in its colour, or Other in grey.
    public struct Segment: Equatable, Sendable, Identifiable {
        public let id: String
        public let name: String
        /// The project's colour; nil for Other, or a project that takes none.
        public let color: ProjectColor?
        public let isOther: Bool
        /// Its share of the bar's full width, 0…1.
        public let fraction: Double
    }

    /// One line of the legend: a project or Other, with its figure.
    public struct LegendRow: Equatable, Sendable, Identifiable {
        public let id: String
        public let name: String
        public let color: ProjectColor?
        public let isOther: Bool
        /// "0.42 GB".
        public let figure: String
        /// The repositories behind the row, for its tooltip.
        public let repos: [String]
    }

    /// The id of Other's segment and legend row; never a project's ID.
    public static let otherID = "_other"

    /// The bar's segments, largest project first, then Other — only those
    /// holding anything. The bar's full width is the allowance, or the total
    /// where that is more (or no allowance is known), so a month over its
    /// allowance fills the bar rather than running off it.
    public static func segments(_ usage: StorageUsage) -> [Segment] {
        let scale = max(usage.allowanceGB, usage.totalGB)
        guard scale > 0 else { return [] }
        var out = usage.projects.filter { $0.gb > 0 }.map {
            Segment(
                id: $0.id, name: $0.name, color: ProjectColor(word: $0.color), isOther: false,
                fraction: $0.gb / scale)
        }
        if usage.other.gb > 0 {
            out.append(Segment(
                id: otherID, name: otherName, color: nil, isOther: true, fraction: usage.other.gb / scale))
        }
        return out
    }

    /// The legend: every project, in nat's order, then Other.
    public static func legend(_ usage: StorageUsage) -> [LegendRow] {
        usage.projects.map {
            LegendRow(
                id: $0.id, name: $0.name, color: ProjectColor(word: $0.color), isOther: false,
                figure: gb($0.gb), repos: $0.repos)
        } + [LegendRow(
            id: otherID, name: otherName, color: nil, isOther: true,
            figure: gb(usage.other.gb), repos: usage.other.repos.map(\.repo))]
    }

    /// The total against the allowance: "0.62 GB of 1.00 GB used".
    public static func summary(_ usage: StorageUsage) -> String {
        guard usage.allowanceGB > 0 else {
            return "\(gb(usage.totalGB)) used \u{2014} the plan's allowance is not known"
        }
        return "\(gb(usage.totalGB)) of \(gb(usage.allowanceGB)) used"
    }

    /// Whether the month has used more than the plan includes.
    public static func isOver(_ usage: StorageUsage) -> Bool {
        usage.allowanceGB > 0 && usage.totalGB > usage.allowanceGB
    }

    /// "22 days left in October".
    public static func daysLeft(_ usage: StorageUsage) -> String {
        let monthName = (1...12).contains(usage.month) ? monthNames[usage.month - 1] : "the month"
        let days = usage.daysLeft == 1 ? "1 day" : "\(usage.daysLeft) days"
        return "\(days) left in \(monthName)"
    }

    /// A figure as GitHub's billing page writes one: two decimals and GB.
    public static func gb(_ value: Double) -> String {
        String(format: "%.2f GB", value)
    }

    static let otherName = "Other repositories"

    /// The months in English, whatever the Mac's language: the app's words
    /// are all English.
    private static let monthNames: [String] = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        return f.standaloneMonthSymbols
    }()
}

extension StorageUsageModel.State {
    /// Whether gh lacks the scope — the refresh button reads Check Again.
    public var isNeedsScope: Bool {
        if case .needsScope = self { return true }
        return false
    }
}
