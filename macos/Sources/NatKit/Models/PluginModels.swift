import Foundation

// The plugin installer's answers — `nat plugin-list`, `plugin-install`,
// `plugin-uninstall` and the two source edits — field for field as
// `internal/plugins` and `internal/cli/plugins.go` write them. nat writes
// every field and every list, so these decode strictly: a shape that has
// drifted fails here rather than drawing as an empty tab.

/// `nat plugin-list --json`: every plugin source, every installed plugin and
/// every plugin a source offers.
public struct PluginListing: Codable, Equatable, Sendable {
    public let sources: [PluginSourceStatus]
    public let installed: [InstalledPlugin]
    public let available: [AvailablePlugin]

    public init(sources: [PluginSourceStatus], installed: [InstalledPlugin], available: [AvailablePlugin]) {
        self.sources = sources
        self.installed = installed
        self.available = available
    }
}

/// One plugin source as nat last read it: its latest release's version, or
/// why it could not be read — which is never the same as offering nothing.
public struct PluginSourceStatus: Codable, Equatable, Sendable, Identifiable {
    public let repo: String
    public let version: String
    public let error: String
    /// nat's own repository: always read, never removable.
    public let isDefault: Bool

    public var id: String { repo }

    enum CodingKeys: String, CodingKey {
        case repo, version, error
        case isDefault = "default"
    }

    public init(repo: String, version: String = "", error: String = "", isDefault: Bool = false) {
        self.repo = repo
        self.version = version
        self.error = error
        self.isDefault = isDefault
    }
}

/// How an installed plugin got where it is.
public enum PluginInstallKind: String, Codable, Equatable, Sendable {
    /// Installed by nat, with a record of where from.
    case managed
    /// In nat's plugins directory, put there by hand.
    case manual
    /// Found on PATH.
    case path
}

/// One installed plugin. `source`, `version` and `update` are a managed
/// install's alone; `update` is the newer version its source offers, empty
/// where there is none.
public struct InstalledPlugin: Codable, Equatable, Sendable, Identifiable {
    public let name: String
    public let path: String
    public let kind: PluginInstallKind
    public let source: String
    public let version: String
    public let update: String

    public var id: String { name }

    public init(
        name: String, path: String, kind: PluginInstallKind,
        source: String = "", version: String = "", update: String = ""
    ) {
        self.name = name
        self.path = path
        self.kind = kind
        self.source = source
        self.version = version
        self.update = update
    }

    /// What the row says in place of a version: the version nat installed,
    /// else how it got there — nat has no version for a plugin it did not
    /// install.
    public var versionLabel: String {
        switch kind {
        case .managed: version.isEmpty ? "installed by nat" : version
        case .manual: "manual"
        case .path: "on PATH"
        }
    }

    /// Whether its source has a newer release of it to install.
    public var hasUpdate: Bool { kind == .managed && !update.isEmpty }

    /// Whether nat may take it away: only one in its own plugins directory.
    public var isUninstallable: Bool { kind != .path }
}

/// One plugin a source offers, at that source's latest release.
public struct AvailablePlugin: Codable, Equatable, Sendable, Identifiable {
    public let name: String
    public let title: String
    public let description: String
    public let source: String
    public let version: String
    public let installed: Bool

    /// Two sources may offer a plugin of one name; each is its own row.
    public var id: String { "\(source)/\(name)" }

    public init(name: String, title: String, description: String, source: String, version: String, installed: Bool) {
        self.name = name
        self.title = title
        self.description = description
        self.source = source
        self.version = version
        self.installed = installed
    }

    /// Its title, else the name it installs as.
    public var displayTitle: String { title.isEmpty ? name : title }
}

/// `nat plugin-install --json`: what was installed, from where.
public struct PluginInstalled: Codable, Equatable, Sendable {
    public let name: String
    public let path: String
    public let source: String
    public let version: String
    public let sha256: String
    public let installedAt: String

    enum CodingKeys: String, CodingKey {
        case name, path, source, version, sha256
        case installedAt = "installed_at"
    }

    public init(name: String, path: String, source: String, version: String, sha256: String, installedAt: String) {
        self.name = name
        self.path = path
        self.source = source
        self.version = version
        self.sha256 = sha256
        self.installedAt = installedAt
    }
}

/// `nat plugin-uninstall --json`: the directory taken away.
public struct PluginUninstalled: Codable, Equatable, Sendable {
    public let name: String
    public let path: String

    public init(name: String, path: String) {
        self.name = name
        self.path = path
    }
}

/// `nat plugin-source-add|remove --json`: every source after the change.
public struct PluginSourceList: Codable, Equatable, Sendable {
    public let sources: [String]

    public init(sources: [String]) {
        self.sources = sources
    }
}

/// The shape of a plugin source, as `plugins.ValidRepo` holds it — here only
/// so the Add button can wait for something worth sending. nat validates it
/// again and its refusal is what is shown.
public enum PluginSourceRepo {
    public static func isValid(_ repo: String) -> Bool {
        let pattern = /[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?\/[A-Za-z0-9._-]+/
        guard repo.wholeMatch(of: pattern) != nil else { return false }
        let name = repo.split(separator: "/", maxSplits: 1).last.map(String.init) ?? ""
        return name != "." && name != ".."
    }
}
