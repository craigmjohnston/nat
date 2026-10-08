import Foundation

/// Which Claude Code this machine has and the newest released —
/// `nat claude-version --json`. Either side nat could not read is absent, and
/// then `updateAvailable` is false.
public struct ClaudeVersion: Codable, Equatable, Sendable {
    public let installed: String?
    public let latest: String?
    public let updateAvailable: Bool

    private enum CodingKeys: String, CodingKey {
        case installed, latest
        case updateAvailable = "update_available"
    }

    public init(installed: String?, latest: String?, updateAvailable: Bool) {
        self.installed = installed
        self.latest = latest
        self.updateAvailable = updateAvailable
    }

    /// The status bar's notice — "Claude Code 2.1.295 available" — only
    /// where nat says a newer one exists, else nil and nothing is drawn.
    public var notice: String? {
        guard updateAvailable, let latest else { return nil }
        return "Claude Code \(latest) available"
    }
}

/// What `nat claude-update --json` answers: `claude update`'s own output.
public struct ClaudeUpdateResult: Codable, Equatable, Sendable {
    public let output: String

    public init(output: String) {
        self.output = output
    }
}
