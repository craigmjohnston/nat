import Foundation

/// A project's own settings sheet's form, held as the strings a person typed
/// — one field per per-project key, so a row added to the sheet is a field
/// here and a case in `ProjectSettingsModel.changes`/`applying`.
public struct ProjectSettingsFields: Equatable, Sendable {
    /// The project's name as its entry holds it — empty for a source
    /// project, whose plugin names it and whose sheet edits no name.
    public var name: String
    public var workingDir: String
    /// The project's colour — the picked swatch. Nil only where the entry has
    /// none yet, and nothing then is written until a swatch is picked.
    public var color: ProjectColor?
    /// The project's run commands in the order written: the first of each
    /// scope is that scope's default, so the order is part of the value.
    public var runs: [RunCommand]

    public init(name: String = "", workingDir: String, color: ProjectColor? = nil, runs: [RunCommand] = []) {
        self.name = name
        self.workingDir = workingDir
        self.color = color
        self.runs = runs
    }

    /// The project's entry as config holds it — empty for a project config
    /// does not name (or no config read yet), which is what the field shows.
    public init(projectID: String, config: NatProjectConfig?) {
        let entry = config?.projects[projectID]
        name = entry?.name ?? ""
        workingDir = entry?.workingDir ?? ""
        color = entry?.color
        runs = entry?.runs ?? []
    }
}

/// Where a project's plan lives, as the sheet's read-only Plan row says it.
/// Nothing in the sheet moves a plan: `nat project-mirror` is how a local
/// one goes into Notion.
public enum ProjectPlanLocation: Equatable, Sendable {
    /// In Notion, at the project's page — nil where the ID is no page ID.
    case notion(page: URL?)
    /// A plan file of nat's own, at the path `nat paths --project` gives;
    /// nil until that has answered, and where it could not.
    case local(file: String?)
    /// A local plan whose containers are this task-source plugin's.
    case source(plugin: String)

    /// The location config's entry for `projectID` says: a Notion project
    /// by its page, a local one with its file not yet read.
    public init(projectID: String, entry: ProjectConfig?) {
        switch entry?.backend ?? .notion {
        case .notion: self = .notion(page: NotionPageURL.forPage(projectID))
        case .local: self = .local(file: nil)
        case .source: self = .source(plugin: entry?.source ?? "")
        }
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
    /// Whether the sheet has a Colour row: not for the scratch project or a
    /// source project, which take no colour.
    public let takesColor: Bool
    /// Whether the name and run commands are edited here: not for a source
    /// project, which its plugin names and which has no working directory to
    /// run anything in.
    public let isSource: Bool
    /// Where the plan lives — a local plan's file filled in by `loadPlanFile`.
    public private(set) var plan: ProjectPlanLocation

    private let write: @Sendable (ConfigChange) async throws -> Void
    private let reload: @MainActor () async -> Void
    private let readPlanFile: @Sendable () async throws -> String?

    /// - Parameters:
    ///   - write: one `config-set` — `NatClient.configSet` in the app.
    ///   - reload: re-reads the app's config (`AppModel.reloadConfig`).
    ///   - readPlanFile: a local plan's file — `NatClient.planFile` in the app.
    public init(
        projectID: String,
        fields: ProjectSettingsFields,
        takesColor: Bool = true,
        isSource: Bool = false,
        plan: ProjectPlanLocation = .notion(page: nil),
        write: @escaping @Sendable (ConfigChange) async throws -> Void,
        reload: @escaping @MainActor () async -> Void,
        readPlanFile: @escaping @Sendable () async throws -> String? = { nil }
    ) {
        self.takesColor = takesColor
        self.isSource = isSource
        self.plan = plan
        self.projectID = projectID
        self.original = fields
        self.edited = fields
        self.write = write
        self.reload = reload
        self.readPlanFile = readPlanFile
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
            takesColor: config?.takesColor(projectID) ?? false,
            isSource: config?.projects[projectID]?.backend == .source,
            plan: ProjectPlanLocation(projectID: projectID, entry: config?.projects[projectID]),
            write: { try await client.configSet(key: $0.key, value: $0.value) },
            reload: reload,
            readPlanFile: { try await client.planFile(projectID: projectID) })
    }

    /// The working directory's `config-set` key.
    public var workingDirKey: String { SettingsModel.workingDirKey(projectID: projectID) }

    /// The colour's `config-set` key.
    public var colorKey: String { SettingsModel.colorKey(projectID: projectID) }

    /// The name's `config-set` key.
    public var nameKey: String { SettingsModel.nameKey(projectID: projectID) }

    /// The run commands' `config-set` key.
    public var runsKey: String { SettingsModel.runsKey(projectID: projectID) }

    /// Reads a local plan's file into `plan`, once; a failed read leaves it
    /// unknown — the row then says Local with no path to reveal.
    public func loadPlanFile() async {
        guard case .local(nil) = plan else { return }
        guard let file = try? await readPlanFile(), !file.isEmpty else { return }
        plan = .local(file: file)
    }

    /// A blank run at the foot of the list, offered in both places.
    public func addRun() {
        edited.runs.append(RunCommand(label: "", command: "", scope: .both))
    }

    /// Drops the run at `index`; an index past the list drops nothing.
    public func removeRun(at index: Int) {
        guard edited.runs.indices.contains(index) else { return }
        edited.runs.remove(at: index)
    }

    /// Moves runs as a list's drag does — which changes each scope's default
    /// where a run goes before the first of its scope.
    public func moveRuns(fromOffsets source: IndexSet, toOffset destination: Int) {
        edited.runs.move(fromOffsets: source, toOffset: destination)
    }

    /// Moves the run at `index` into the place of the run at `target` — what
    /// a row dropped onto another row means — the rows between shifting up
    /// or down by one. Either index past the list moves nothing.
    public func moveRun(_ index: Int, onto target: Int) {
        guard index != target, edited.runs.indices.contains(index), edited.runs.indices.contains(target) else { return }
        moveRuns(fromOffsets: IndexSet(integer: index), toOffset: target > index ? target + 1 : target)
    }

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
        let name = edited.name.trimmingCharacters(in: .whitespacesAndNewlines)
        if original.name != name {
            changes.append(ConfigChange(key: SettingsModel.nameKey(projectID: projectID), value: name))
        }
        if original.workingDir != edited.workingDir {
            changes.append(ConfigChange(key: SettingsModel.workingDirKey(projectID: projectID), value: edited.workingDir))
        }
        if original.color != edited.color, let color = edited.color {
            changes.append(ConfigChange(key: SettingsModel.colorKey(projectID: projectID), value: color.rawValue))
        }
        if original.runs != edited.runs {
            changes.append(ConfigChange(key: SettingsModel.runsKey(projectID: projectID), value: runsValue(edited.runs)))
        }
        return changes
    }

    /// The runs as `config-set project.<id>.runs` takes them: the whole list
    /// as a JSON array, each run as nat writes it (no scope for both), and
    /// the empty string — the key space's unset — for none.
    static func runsValue(_ runs: [RunCommand]) -> String {
        guard !runs.isEmpty else { return "" }
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
        guard let data = try? encoder.encode(runs) else { return "" }
        return String(decoding: data, as: UTF8.self)
    }

    /// The runs a `runsValue` carries back — none for the empty string.
    static func runs(fromValue value: String) -> [RunCommand] {
        guard !value.isEmpty else { return [] }
        return (try? JSONDecoder().decode([RunCommand].self, from: Data(value.utf8))) ?? []
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
            case SettingsModel.nameKey(projectID: projectID): result.name = change.value
            case SettingsModel.runsKey(projectID: projectID): result.runs = Self.runs(fromValue: change.value)
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
