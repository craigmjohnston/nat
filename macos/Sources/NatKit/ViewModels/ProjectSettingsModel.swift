import Foundation

/// A project's own settings sheet's form, held as the strings a person typed
/// — one field per per-project key, so a row added to the sheet is a field
/// here and a case in `ProjectSettingsModel.changes`/`applying`.
public struct ProjectSettingsFields: Equatable, Sendable {
    public var workingDir: String
    /// The project's colour — the picked swatch. Nil only where the entry has
    /// none yet, and nothing then is written until a swatch is picked.
    public var color: ProjectColor?

    public init(workingDir: String, color: ProjectColor? = nil) {
        self.workingDir = workingDir
        self.color = color
    }

    /// The project's entry as config holds it — empty for a project config
    /// does not name (or no config read yet), which is what the field shows.
    public init(projectID: String, config: NatProjectConfig?) {
        workingDir = config?.projects[projectID]?.workingDir ?? ""
        color = config?.projects[projectID]?.color
    }
}

/// The project settings sheet (the project menu's Project settings…): the
/// project's fields as config read them, the edits made over them, and the
/// Save that writes the difference — Settings' own rule, one `nat config-set
/// <key> <value>` per changed key, a refusal kept beside its key with the
/// baseline left as read, and config re-read once anything landed so every
/// launch and Reveal uses the new value at once. The view only binds.
@MainActor
@Observable
public final class ProjectSettingsModel {
    public let projectID: String
    /// What config held when the sheet opened, moved forward only by writes
    /// that landed.
    public private(set) var original: ProjectSettingsFields
    public var edited: ProjectSettingsFields
    /// nat's refusal of the last Save, by key.
    public private(set) var errors: [String: String] = [:]
    public private(set) var isSaving = false

    private let write: @Sendable (ConfigChange) async throws -> Void
    private let reload: @MainActor () async -> Void

    /// - Parameters:
    ///   - write: one `config-set` — `NatClient.configSet` in the app.
    ///   - reload: re-reads the app's config (`AppModel.reloadConfig`).
    public init(
        projectID: String,
        fields: ProjectSettingsFields,
        write: @escaping @Sendable (ConfigChange) async throws -> Void,
        reload: @escaping @MainActor () async -> Void
    ) {
        self.projectID = projectID
        self.original = fields
        self.edited = fields
        self.write = write
        self.reload = reload
    }

    /// The sheet over `client`, reading the project's fields from `config`.
    public convenience init(
        projectID: String,
        config: NatProjectConfig?,
        client: NatClientProtocol,
        reload: @escaping @MainActor () async -> Void
    ) {
        self.init(
            projectID: projectID,
            fields: ProjectSettingsFields(projectID: projectID, config: config),
            write: { try await client.configSet(key: $0.key, value: $0.value) },
            reload: reload)
    }

    /// The working directory's `config-set` key.
    public var workingDirKey: String { SettingsModel.workingDirKey(projectID: projectID) }

    /// The colour's `config-set` key.
    public var colorKey: String { SettingsModel.colorKey(projectID: projectID) }

    /// The writes Save would make now.
    public var changes: [ConfigChange] {
        Self.changes(projectID: projectID, from: original, to: edited)
    }

    /// The `config-set` writes that carry `edited` onto `original`: one per
    /// field that changed, in field order.
    public static func changes(
        projectID: String, from original: ProjectSettingsFields, to edited: ProjectSettingsFields
    ) -> [ConfigChange] {
        var changes: [ConfigChange] = []
        if original.workingDir != edited.workingDir {
            changes.append(ConfigChange(key: SettingsModel.workingDirKey(projectID: projectID), value: edited.workingDir))
        }
        if original.color != edited.color, let color = edited.color {
            changes.append(ConfigChange(key: SettingsModel.colorKey(projectID: projectID), value: color.rawValue))
        }
        return changes
    }

    /// `fields` with the writes that landed applied — a key this sheet does
    /// not hold moves nothing.
    public static func applying(
        _ changes: [ConfigChange], projectID: String, to fields: ProjectSettingsFields
    ) -> ProjectSettingsFields {
        var result = fields
        for change in changes {
            switch change.key {
            case SettingsModel.workingDirKey(projectID: projectID): result.workingDir = change.value
            case SettingsModel.colorKey(projectID: projectID): result.color = ProjectColor(rawValue: change.value)
            default: break
            }
        }
        return result
    }

    /// Writes every change, each its own `config-set`; true once all of them
    /// landed (nothing to write included), which is when the sheet closes.
    /// A refused key keeps its edit and nat's message; a second Save while
    /// one is under way does nothing.
    public func save() async -> Bool {
        guard !isSaving else { return false }
        let pending = changes
        guard !pending.isEmpty else {
            errors = [:]
            return true
        }
        isSaving = true
        defer { isSaving = false }

        var landed: [ConfigChange] = []
        var refused: [String: String] = [:]
        for change in pending {
            do {
                try await write(change)
                landed.append(change)
            } catch NatError.commandFailed(let message) {
                refused[change.key] = message
            } catch {
                refused[change.key] = error.localizedDescription
            }
        }
        original = Self.applying(landed, projectID: projectID, to: original)
        errors = refused
        if !landed.isEmpty { await reload() }
        return refused.isEmpty
    }
}
