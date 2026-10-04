import Foundation

/// One `config-set` write the settings scene's Save button would make:
/// the key exactly as `internal/cli/configset.go` names it, and the value to
/// write.
public struct ConfigChange: Equatable, Sendable {
    public let key: String
    public let value: String

    public init(key: String, value: String) {
        self.key = key
        self.value = value
    }
}

/// The settings scene's form, held as the strings a person typed rather than
/// the numbers or optionals `ConfigDoc` decodes them into — a field cleared
/// back to empty is "unset" and not a zero to render, mirroring the TUI
/// form's own `Settings` (`internal/tui/settings.go`).
public struct SettingsFields: Equatable, Sendable {
    public var pollSeconds: String
    public var workshopModel: String
    public var workshopEffort: String
    public var sliceModel: String
    public var sliceEffort: String

    /// Each tracked project's working directory, keyed by project ID —
    /// there is one field per project rather than one for "the active
    /// project", since a settings window has no notion of which project the
    /// board happens to be showing.
    public var projectWorkingDirs: [String: String]

    /// Each tracked project's run commands, keyed by project ID, as the
    /// table under its working directory edits them — a row being typed in
    /// included.
    public var projectRuns: [String: [RunCommand]]

    public init(
        pollSeconds: String,
        workshopModel: String,
        workshopEffort: String,
        sliceModel: String,
        sliceEffort: String,
        projectWorkingDirs: [String: String],
        projectRuns: [String: [RunCommand]] = [:]
    ) {
        self.pollSeconds = pollSeconds
        self.workshopModel = workshopModel
        self.workshopEffort = workshopEffort
        self.sliceModel = sliceModel
        self.sliceEffort = sliceEffort
        self.projectWorkingDirs = projectWorkingDirs
        self.projectRuns = projectRuns
    }

    /// Reads a loaded `ConfigDoc` into the form's own shape. An unset number
    /// comes back empty rather than as "0", the same rule `optionalNumber`
    /// applies on the Go side.
    public init(from config: ConfigDoc) {
        pollSeconds = config.pollSeconds == 0 ? "" : String(config.pollSeconds)
        workshopModel = config.workshopAgent.model ?? ""
        workshopEffort = config.workshopAgent.effort ?? ""
        sliceModel = config.sliceAgent.model ?? ""
        sliceEffort = config.sliceAgent.effort ?? ""
        projectWorkingDirs = config.projects.mapValues { $0.workingDir }
        projectRuns = config.projects.mapValues { $0.runs }
    }
}

/// The settings scene's one piece of logic worth testing on its own: diffing
/// what was loaded against what a person edited, into exactly the
/// `config-set` writes a Save button should make. The form view stays thin —
/// it holds a `SettingsFields` for editing and calls this once, on Save.
public enum SettingsModel {
    private static let keyPollSeconds = "poll_seconds"
    private static let keyWorkshopModel = "workshop_agent.model"
    private static let keyWorkshopEffort = "workshop_agent.effort"
    private static let keySliceModel = "slice_agent.model"
    private static let keySliceEffort = "slice_agent.effort"

    /// The key `config-set` reads a project's working directory from,
    /// mirroring `internal/cli/configset.go`'s own `project.<id>.working_dir`.
    public static func workingDirKey(projectID: String) -> String {
        "project.\(projectID).working_dir"
    }

    /// The key `config-set` reads a project's whole run list from, mirroring
    /// `internal/cli/configset.go`'s `project.<id>.runs`.
    public static func runsKey(projectID: String) -> String {
        "project.\(projectID).runs"
    }

    /// The `config-set` writes that would carry `edited` onto `original` —
    /// one per field that actually changed, nothing for a field left alone.
    /// Order is fixed (the scalar fields, then projects sorted by ID) so a
    /// save always writes in the same order twice.
    ///
    /// A project's runs are written whole, and only once every row has a
    /// label and a command: a row still being typed is no run to write, and
    /// nat would refuse it. Anything else nat refuses — a label used twice —
    /// comes back as that key's error.
    public static func changes(from original: SettingsFields, to edited: SettingsFields) -> [ConfigChange] {
        var changes: [ConfigChange] = []

        func addIfChanged(_ key: String, _ oldValue: String, _ newValue: String) {
            guard oldValue != newValue else { return }
            changes.append(ConfigChange(key: key, value: newValue))
        }

        addIfChanged(keyPollSeconds, original.pollSeconds, edited.pollSeconds)
        addIfChanged(keyWorkshopModel, original.workshopModel, edited.workshopModel)
        addIfChanged(keyWorkshopEffort, original.workshopEffort, edited.workshopEffort)
        addIfChanged(keySliceModel, original.sliceModel, edited.sliceModel)
        addIfChanged(keySliceEffort, original.sliceEffort, edited.sliceEffort)

        for projectID in edited.projectWorkingDirs.keys.sorted() {
            let newValue = edited.projectWorkingDirs[projectID] ?? ""
            let oldValue = original.projectWorkingDirs[projectID] ?? ""
            addIfChanged(workingDirKey(projectID: projectID), oldValue, newValue)
        }

        for projectID in edited.projectRuns.keys.sorted() {
            let runs = edited.projectRuns[projectID] ?? []
            guard runs.allSatisfy(\.isComplete) else { continue }
            addIfChanged(
                runsKey(projectID: projectID),
                (original.projectRuns[projectID] ?? []).configValue, runs.configValue)
        }

        return changes
    }

    /// Applies changes that were actually written back onto a copy of
    /// `fields` — what a Save button uses to move its "as last saved"
    /// baseline forward without re-reading the whole config, so a retried
    /// Save (after some keys failed) only reattempts the ones that did.
    public static func applying(_ changes: [ConfigChange], to fields: SettingsFields) -> SettingsFields {
        var result = fields
        for change in changes {
            switch change.key {
            case keyPollSeconds: result.pollSeconds = change.value
            case keyWorkshopModel: result.workshopModel = change.value
            case keyWorkshopEffort: result.workshopEffort = change.value
            case keySliceModel: result.sliceModel = change.value
            case keySliceEffort: result.sliceEffort = change.value
            default:
                if let projectID = projectID(from: change.key, suffix: ".working_dir") {
                    result.projectWorkingDirs[projectID] = change.value
                } else if let projectID = projectID(from: change.key, suffix: ".runs") {
                    result.projectRuns[projectID] = change.value.isEmpty
                        ? []
                        : (try? JSONDecoder().decode([RunCommand].self, from: Data(change.value.utf8))) ?? []
                }
            }
        }
        return result
    }

    /// The project ID a `project.<id><suffix>` key names, or nil for any
    /// other key.
    private static func projectID(from key: String, suffix: String) -> String? {
        let prefix = "project."
        guard key.hasPrefix(prefix), key.hasSuffix(suffix), key.count > prefix.count + suffix.count else {
            return nil
        }
        let start = key.index(key.startIndex, offsetBy: prefix.count)
        let end = key.index(key.endIndex, offsetBy: -suffix.count)
        return String(key[start..<end])
    }
}

extension RunCommand {
    /// Whether the row is a run nat would keep: a label and a command, both
    /// more than space.
    public var isComplete: Bool {
        !label.trimmingCharacters(in: .whitespaces).isEmpty
            && !command.trimmingCharacters(in: .whitespaces).isEmpty
    }
}
