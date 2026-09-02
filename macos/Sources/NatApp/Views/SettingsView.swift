import SwiftUI
import NatKit

/// The Settings scene (⌘,): the config file as one form, mirroring the Go
/// TUI's `S` — every field reachable from the one page rather than a section
/// apiece, since a settings window is somewhere the user arrives already
/// knowing which field they came for.
///
/// Save writes only the keys that changed, one `config-set` per key, and
/// surfaces each key's own refusal beside its field rather than as one banner
/// — `nat` refuses an out-of-bounds number with its own message, and that
/// message is the whole of what there is to say about it.
struct SettingsView: View {
    @Bindable var appModel: AppModel

    @State private var projectNames: [String: String] = [:]
    @State private var original: SettingsFields?
    @State private var edited = SettingsFields(
        agentSplitPercent: "", pollSeconds: "",
        workshopModel: "", workshopEffort: "",
        sliceModel: "", sliceEffort: "",
        projectWorkingDirs: [:]
    )
    @State private var isLoading = true
    @State private var loadError: String?
    @State private var isSaving = false
    @State private var fieldErrors: [String: String] = [:]
    @State private var savedNote: String?

    private var hasChanges: Bool {
        guard let original else { return false }
        return !SettingsModel.changes(from: original, to: edited).isEmpty
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text("Settings")
                .font(.system(size: 15, weight: .semibold))
                .foregroundStyle(DesignTokens.label)
                .padding(.horizontal, 20)
                .padding(.top, 20)
                .padding(.bottom, 12)

            Divider()

            Group {
                if isLoading {
                    ProgressView()
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else if let loadError {
                    VStack(spacing: 8) {
                        Text("Could not load configuration")
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundStyle(DesignTokens.label)
                        Text(loadError)
                            .font(.system(size: 11, weight: .regular))
                            .foregroundStyle(DesignTokens.systemRed)
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else {
                    form
                }
            }
            .frame(maxHeight: .infinity)

            Divider()

            footer
        }
        .frame(width: 480, height: 560)
        .background(DesignTokens.windowBg)
        .task {
            await load()
        }
    }

    private var form: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                ForEach(sortedProjectIDs, id: \.self) { projectID in
                    workingDirField(projectID: projectID)
                }

                numberField(
                    title: "Agent split",
                    description: "Percent of the window an agent's terminal takes beside the board; empty is 65. Applies at once.",
                    key: "agent_split_percent",
                    value: $edited.agentSplitPercent
                )

                numberField(
                    title: "Poll interval",
                    description: "Seconds between background refetches of the plan; empty is 30. Applies from the next poll.",
                    key: "poll_seconds",
                    value: $edited.pollSeconds
                )

                agentFields(
                    title: "Slice agent",
                    modelKey: "slice_agent.model",
                    effortKey: "slice_agent.effort",
                    model: $edited.sliceModel,
                    effort: $edited.sliceEffort
                )

                agentFields(
                    title: "Planning agent",
                    modelKey: "workshop_agent.model",
                    effortKey: "workshop_agent.effort",
                    model: $edited.workshopModel,
                    effort: $edited.workshopEffort
                )
            }
            .padding(20)
        }
    }

    private var footer: some View {
        HStack(spacing: 8) {
            if let savedNote {
                Text(savedNote)
                    .font(.system(size: 11, weight: .regular))
                    .foregroundStyle(DesignTokens.systemGreen)
            }

            Spacer()

            Button(action: { Task { await save() } }) {
                if isSaving {
                    ProgressView()
                        .scaleEffect(0.6, anchor: .center)
                        .frame(width: 40)
                } else {
                    Text("Save")
                        .frame(minWidth: 40)
                }
            }
            .disabled(isSaving || isLoading || !hasChanges)
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 12)
    }

    // MARK: - Field builders

    private var sortedProjectIDs: [String] {
        projectNames.keys.sorted { (projectNames[$0] ?? $0) < (projectNames[$1] ?? $1) }
    }

    private func workingDirField(projectID: String) -> some View {
        let key = SettingsModel.workingDirKey(projectID: projectID)
        return VStack(alignment: .leading, spacing: 4) {
            Text("\(projectNames[projectID] ?? projectID) working directory")
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(DesignTokens.labelSecondary)
            Text("Where its agents start, unless a slice names its own repo. Applies at the next launch.")
                .font(.system(size: 11, weight: .regular))
                .foregroundStyle(DesignTokens.labelTertiary)
            TextField("", text: workingDirBinding(projectID: projectID))
                .textFieldStyle(.roundedBorder)
                .font(.system(size: 12, weight: .regular, design: .monospaced))
            if let error = fieldErrors[key] {
                Text(error)
                    .font(.system(size: 11, weight: .regular))
                    .foregroundStyle(DesignTokens.systemRed)
            }
        }
    }

    private func numberField(title: String, description: String, key: String, value: Binding<String>) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(title)
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(DesignTokens.labelSecondary)
            Text(description)
                .font(.system(size: 11, weight: .regular))
                .foregroundStyle(DesignTokens.labelTertiary)
            TextField("", text: value)
                .textFieldStyle(.roundedBorder)
                .frame(width: 100)
            if let error = fieldErrors[key] {
                Text(error)
                    .font(.system(size: 11, weight: .regular))
                    .foregroundStyle(DesignTokens.systemRed)
            }
        }
    }

    private func agentFields(
        title: String,
        modelKey: String,
        effortKey: String,
        model: Binding<String>,
        effort: Binding<String>
    ) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(title)
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(DesignTokens.labelSecondary)
            Text("Model and effort a launch runs Claude Code as, unless the launch itself overrides them. Applies at the next launch.")
                .font(.system(size: 11, weight: .regular))
                .foregroundStyle(DesignTokens.labelTertiary)

            HStack(spacing: 12) {
                Picker("Model", selection: defaultableBinding(model)) {
                    Text("Default").tag("Default")
                    Text("sonnet").tag("sonnet")
                    Text("opus").tag("opus")
                    Text("haiku").tag("haiku")
                }
                .labelsHidden()
                .frame(width: 140)

                Picker("Effort", selection: defaultableBinding(effort)) {
                    Text("Default").tag("Default")
                    Text("low").tag("low")
                    Text("med").tag("med")
                    Text("high").tag("high")
                }
                .labelsHidden()
                .frame(width: 140)
            }

            if let error = fieldErrors[modelKey] {
                Text(error)
                    .font(.system(size: 11, weight: .regular))
                    .foregroundStyle(DesignTokens.systemRed)
            }
            if let error = fieldErrors[effortKey] {
                Text(error)
                    .font(.system(size: 11, weight: .regular))
                    .foregroundStyle(DesignTokens.systemRed)
            }
        }
    }

    /// A field cleared back to empty is "unset", the config file's own
    /// spelling of it — but a picker needs a real option selected, so
    /// "Default" stands in for "" on the way in and out.
    private func defaultableBinding(_ base: Binding<String>) -> Binding<String> {
        Binding(
            get: { base.wrappedValue.isEmpty ? "Default" : base.wrappedValue },
            set: { base.wrappedValue = $0 == "Default" ? "" : $0 }
        )
    }

    private func workingDirBinding(projectID: String) -> Binding<String> {
        Binding(
            get: { edited.projectWorkingDirs[projectID] ?? "" },
            set: { edited.projectWorkingDirs[projectID] = $0 }
        )
    }

    // MARK: - Loading and saving

    private func load() async {
        isLoading = true
        loadError = nil
        do {
            let doc = try await NatClient().configShow()
            projectNames = doc.projects.mapValues { $0.name }
            let fields = SettingsFields(from: doc)
            original = fields
            edited = fields
        } catch let error as NatError {
            loadError = error.localizedDescription
        } catch {
            loadError = error.localizedDescription
        }
        isLoading = false
    }

    private func save() async {
        guard let original else { return }
        isSaving = true
        savedNote = nil

        let changes = SettingsModel.changes(from: original, to: edited)
        var succeeded: [ConfigChange] = []
        var errors: [String: String] = [:]

        for change in changes {
            do {
                try await NatClient().configSet(key: change.key, value: change.value)
                succeeded.append(change)
            } catch let error as NatError {
                if case .commandFailed(let message) = error {
                    errors[change.key] = message
                } else {
                    errors[change.key] = error.localizedDescription
                }
            } catch {
                errors[change.key] = error.localizedDescription
            }
        }

        self.original = SettingsModel.applying(succeeded, to: original)
        self.fieldErrors = errors
        if errors.isEmpty {
            savedNote = "Settings saved."
        }

        await appModel.reloadConfig()
        isSaving = false
    }
}

#Preview {
    SettingsView(appModel: AppModel())
}
