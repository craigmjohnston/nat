import Foundation

/// This month's GitHub artifact storage for the signed-in gh account, shared
/// out by project — `nat storage-usage --json`. GB are GB-months, as GitHub's
/// billing page counts them; every project is listed, one with no repository
/// or no storage at zero, and `other` is every repository no project claims.
public struct StorageUsage: Codable, Equatable, Sendable {
    public let login: String
    /// GitHub's name for the plan, lower-cased; empty where it named none.
    public let plan: String
    /// The storage the plan includes, in GB; zero where nat knows no figure.
    public let allowanceGB: Double
    public let year: Int
    public let month: Int
    public let daysLeft: Int
    public let totalGB: Double
    public let projects: [Project]
    public let other: Other

    public struct Project: Codable, Equatable, Sendable, Identifiable {
        public let id: String
        public let name: String
        /// The project's colour by name; nil for one that takes none.
        public let color: String?
        public let repos: [String]
        public let gb: Double

        public init(id: String, name: String, color: String?, repos: [String], gb: Double) {
            self.id = id
            self.name = name
            self.color = color
            self.repos = repos
            self.gb = gb
        }
    }

    public struct Other: Codable, Equatable, Sendable {
        public let gb: Double
        public let repos: [Repo]

        public init(gb: Double, repos: [Repo]) {
            self.gb = gb
            self.repos = repos
        }
    }

    public struct Repo: Codable, Equatable, Sendable {
        public let repo: String
        public let gb: Double

        public init(repo: String, gb: Double) {
            self.repo = repo
            self.gb = gb
        }
    }

    private enum CodingKeys: String, CodingKey {
        case login, plan, year, month, projects, other
        case allowanceGB = "allowance_gb"
        case daysLeft = "days_left"
        case totalGB = "total_gb"
    }

    public init(
        login: String, plan: String, allowanceGB: Double, year: Int, month: Int, daysLeft: Int,
        totalGB: Double, projects: [Project], other: Other
    ) {
        self.login = login
        self.plan = plan
        self.allowanceGB = allowanceGB
        self.year = year
        self.month = month
        self.daysLeft = daysLeft
        self.totalGB = totalGB
        self.projects = projects
        self.other = other
    }
}

/// What `nat storage-usage --json` answers: the reading, or — where gh lacks
/// the "user" scope GitHub's billing report needs — which command grants it.
/// The second is a choice the user has yet to make, not a failure.
public enum StorageUsageAnswer: Equatable, Sendable {
    case reading(StorageUsage)
    case needsScope(command: String)
}

extension StorageUsageAnswer: Decodable {
    private enum ScopeKeys: String, CodingKey {
        case needsScope = "needs_scope"
        case scopeCommand = "scope_command"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: ScopeKeys.self)
        if try c.decodeIfPresent(String.self, forKey: .needsScope) != nil {
            self = .needsScope(command: try c.decode(String.self, forKey: .scopeCommand))
        } else {
            self = .reading(try StorageUsage(from: decoder))
        }
    }
}
