import Foundation

// The plugin installer's answers — `nat plugin-list`, `plugin-install`,
// `plugin-uninstall`, the two source edits and `source-setup` — field for
// field as `internal/plugins` and `internal/cli` write them. nat writes every
// field and every list, so these decode strictly: a shape that has drifted
// fails here rather than drawing as an empty tab.

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
/// where there is none. `setup` is what its describe asks to be set (empty
/// where it asks nothing or would not describe), and `describeError` why it
/// would not describe — the plugin's own stderr line where it wrote one.
public struct InstalledPlugin: Codable, Equatable, Sendable, Identifiable {
    public let name: String
    public let path: String
    public let kind: PluginInstallKind
    public let source: String
    public let version: String
    public let update: String
    public let setup: [PluginSetupField]
    public let describeError: String

    public var id: String { name }

    enum CodingKeys: String, CodingKey {
        case name, path, kind, source, version, update, setup
        case describeError = "describe_error"
    }

    public init(
        name: String, path: String, kind: PluginInstallKind,
        source: String = "", version: String = "", update: String = "",
        setup: [PluginSetupField] = [], describeError: String = ""
    ) {
        self.name = name
        self.path = path
        self.kind = kind
        self.source = source
        self.version = version
        self.update = update
        self.setup = setup
        self.describeError = describeError
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

/// One thing a plugin needs set before it works — its describe's `setup`
/// entry. gnat draws it and hands the value to `nat source-setup` on stdin;
/// what the value means is the plugin's business alone.
public struct PluginSetupField: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let label: String
    /// `secret` (drawn masked) or `text`.
    public let input: String
    /// Where to find the value, or empty.
    public let hint: String

    enum CodingKeys: String, CodingKey {
        case id, label, input, hint
    }

    public init(id: String, label: String, input: String, hint: String = "") {
        self.id = id
        self.label = label
        self.input = input
        self.hint = hint
    }

    /// `hint` is the one field the protocol lets a plugin leave out.
    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        label = try c.decode(String.self, forKey: .label)
        input = try c.decode(String.self, forKey: .input)
        hint = try c.decodeIfPresent(String.self, forKey: .hint) ?? ""
    }

    /// Whether the field is drawn masked.
    public var isSecret: Bool { input == "secret" }
}

/// `nat source-setup --json`: what the plugin said of the value.
public struct PluginSetupResult: Codable, Equatable, Sendable {
    public let message: String

    public init(message: String) {
        self.message = message
    }
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
