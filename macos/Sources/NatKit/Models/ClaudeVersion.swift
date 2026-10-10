import Foundation

/// How `nat claude-update` will update Claude Code, as nat decides it —
/// `claude-version`'s `update_method` and `homebrew_cask`. gnat never works
/// it out itself.
public enum ClaudeUpdateMethod: Equatable, Sendable {
    /// `brew upgrade <cask>`: claude lives in Homebrew's Caskroom.
    case homebrew(cask: String)
    /// `claude update`: Claude Code's own updater.
    case claudeUpdater

    /// The update window's words for it.
    public var description: String {
        switch self {
        case .homebrew(let cask): "Homebrew (cask \(cask))"
        case .claudeUpdater: "Claude Code"
        }
    }
}

/// Which Claude Code this machine has and the newest released —
/// `nat claude-version --json`. Either side nat could not read is absent, and
/// then `updateAvailable` is false. `updateMethod` is nil only from a nat
/// that predates it.
public struct ClaudeVersion: Codable, Equatable, Sendable {
    public let installed: String?
    public let latest: String?
    public let updateAvailable: Bool
    public let updateMethod: ClaudeUpdateMethod?

    private enum CodingKeys: String, CodingKey {
        case installed, latest
        case updateAvailable = "update_available"
        case updateMethod = "update_method"
        case homebrewCask = "homebrew_cask"
    }

    public init(installed: String?, latest: String?, updateAvailable: Bool, updateMethod: ClaudeUpdateMethod? = nil) {
        self.installed = installed
        self.latest = latest
        self.updateAvailable = updateAvailable
        self.updateMethod = updateMethod
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        installed = try c.decodeIfPresent(String.self, forKey: .installed)
        latest = try c.decodeIfPresent(String.self, forKey: .latest)
        updateAvailable = try c.decode(Bool.self, forKey: .updateAvailable)
        switch try c.decodeIfPresent(String.self, forKey: .updateMethod) {
        case "homebrew":
            updateMethod = .homebrew(cask: try c.decodeIfPresent(String.self, forKey: .homebrewCask) ?? "")
        case "claude":
            updateMethod = .claudeUpdater
        default:
            updateMethod = nil
        }
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encodeIfPresent(installed, forKey: .installed)
        try c.encodeIfPresent(latest, forKey: .latest)
        try c.encode(updateAvailable, forKey: .updateAvailable)
        switch updateMethod {
        case .homebrew(let cask):
            try c.encode("homebrew", forKey: .updateMethod)
            try c.encode(cask, forKey: .homebrewCask)
        case .claudeUpdater:
            try c.encode("claude", forKey: .updateMethod)
        case nil:
            break
        }
    }

    /// The status bar's notice — "Claude Code 2.1.295 available" — only
    /// where nat says a newer one exists, else nil and nothing is drawn.
    public var notice: String? {
        guard updateAvailable, let latest else { return nil }
        return "Claude Code \(latest) available"
    }

    /// The installed version for the update window, "unknown" where nat
    /// could not read it.
    public var installedText: String { installed ?? "unknown" }
}

/// What `nat claude-update --json` answers: `claude update`'s own output.
public struct ClaudeUpdateResult: Codable, Equatable, Sendable {
    public let output: String

    public init(output: String) {
        self.output = output
    }
}
