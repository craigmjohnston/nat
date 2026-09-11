import SwiftUI
import NatKit

/// The Settings scene (⌘,), built as a macOS settings window is built: a
/// `TabView` of toolbar tabs over grouped forms of stock controls, sized by
/// what it holds rather than to a frame of its own — so it is the shape the
/// user already knows from every other settings window on the Mac, and the
/// app's own chrome (`DesignTokens`, `Typo`, hand-drawn dividers) stops at
/// its door.
///
/// Underneath it is the same config file the hand-built form edited: fields
/// read from `nat config-show`, and a write of exactly the keys that changed,
/// one `config-set` per key, with each key's own refusal shown beside the
/// field that caused it — `nat` refuses an out-of-bounds number with its own
/// message, and that message is the whole of what there is to say about it.
///
/// What went is the Save button. A settings window applies what it is told
/// when it is told it, so a field commits on Return or when it loses focus
/// and a picker commits on the choice; the diff is what makes that cheap,
/// since a commit that changed nothing writes nothing. Commits are chained
/// (`commit()`) rather than run as they arrive, so two fields committed in
/// quick succession — tabbing from one to the next — cannot both diff against
/// a baseline the first has yet to move.
///
/// A row is a row: the field's name in the left column and its control alone
/// in the right, on one line, at a fixed width so every row of a tab lines
/// up — which is the shape System Settings, Safari and Xcode all draw. What
/// a field means that its own name does not say is a section's footnote
/// rather than a paragraph per row, since three rows of one section rarely
/// have three different things to say and a caption in the value column
/// drags the control out of its column and wraps it right-aligned.
struct SettingsView: View {
    @Bindable var appModel: AppModel

    /// The theme, which is this app's own preference rather than one of
    /// nat's: it is written to `UserDefaults` the moment it is picked and
    /// takes effect at once, so it is no part of the config form's diff and
    /// is shown whatever became of the config read — including while that
    /// read is still in flight, or has failed.
    @AppStorage(Theme.storageKey) private var storedTheme = Theme.system.rawValue

    @State private var projectNames: [String: String] = [:]
    @State private var original: SettingsFields?
    @State private var edited = SettingsFields(
        pollSeconds: "",
        workshopModel: "", workshopEffort: "",
        sliceModel: "", sliceEffort: "",
        projectWorkingDirs: [:]
    )
    @State private var isLoading = true
    @State private var loadError: String?
    @State private var fieldErrors: [String: String] = [:]
    @State private var saveChain: Task<Void, Never>?

    var body: some View {
        TabView {
            generalTab
                .tabItem { Label("General", systemImage: "gearshape") }
            agentsTab
                .tabItem { Label("Agents", systemImage: "sparkles") }
            projectsTab
                .tabItem { Label("Projects", systemImage: "folder") }
        }
        // Width alone: the height is the tab's own, which is what makes the
        // window resize to each tab the way a settings window does.
        .frame(width: 520)
        .task { await load() }
        // A window closed on a field still focused would otherwise take that
        // edit with it: the focus change never arrives, because the view is
        // gone. The commit chain outlives the view, so this one lands.
        .onDisappear { commit() }
    }

    // MARK: - Tabs

    private var generalTab: some View {
        Form {
            Section {
                settingRow(title: "Theme") {
                    Picker("Theme", selection: themeBinding) {
                        ForEach(Theme.allCases) { theme in
                            Text(theme.title).tag(theme)
                        }
                    }
                    .labelsHidden()
                    .pickerStyle(.segmented)
                    .frame(width: FieldWidth.segments)
                }
            } footer: {
                sectionFootnote("The palette the app draws with, the agent terminal included. Applies at once.")
            }

            configSection(
                "Board",
                footer: "Seconds between background refetches of the plan; empty is 30. Applies from the next poll."
            ) {
                settingRow(title: "Poll interval", key: SettingsKey.pollSeconds) {
                    commitField($edited.pollSeconds, width: FieldWidth.number)
                }
            }
        }
        .settingsForm()
    }

    private var agentsTab: some View {
        Form {
            configSection(
                "Slice agent",
                footer: "Which Claude Code a slice's agent runs as, and how hard it thinks, unless the launch itself overrides them. Applies at the next launch."
            ) {
                agentRows(
                    modelKey: SettingsKey.sliceModel,
                    effortKey: SettingsKey.sliceEffort,
                    model: $edited.sliceModel,
                    effort: $edited.sliceEffort
                )
            }

            configSection(
                "Planning agent",
                footer: "The same, for the agent a workshop launch runs."
            ) {
                agentRows(
                    modelKey: SettingsKey.workshopModel,
                    effortKey: SettingsKey.workshopEffort,
                    model: $edited.workshopModel,
                    effort: $edited.workshopEffort
                )
            }
        }
        .settingsForm()
    }

    private var projectsTab: some View {
        Form {
            configSection(
                "Working directories",
                footer: "Where a project's agents start, unless a slice names its own repo. Applies at the next launch."
            ) {
                if sortedProjectIDs.isEmpty {
                    Text("No projects are tracked on this Mac yet.")
                        .ink(.secondary)
                } else {
                    ForEach(sortedProjectIDs, id: \.self) { projectID in
                        workingDirRow(projectID: projectID)
                    }
                }
            }
        }
        .settingsForm()
    }

    // MARK: - Rows

    /// The rows of a section that edits the config file, or — while that read
    /// is in flight or after it failed — what became of it instead, since a
    /// section of empty fields would read as a config with nothing in it.
    @ViewBuilder
    private func configSection(
        _ title: String,
        footer: String? = nil,
        @ViewBuilder content: () -> some View
    ) -> some View {
        Section {
            if isLoading {
                SettingsLoadingRow()
            } else if let loadError {
                Label(loadError, systemImage: "exclamationmark.triangle.fill")
                    .ink(.danger)
                    .fixedSize(horizontal: false, vertical: true)
            } else {
                content()
            }
        } header: {
            Text(title)
        } footer: {
            // Nothing to footnote while the section is holding a wait or a
            // refusal instead of the fields the footnote is about.
            if let footer, !isLoading, loadError == nil {
                sectionFootnote(footer)
            }
        }
    }

    /// What a section's fields mean beyond their own names: footnote-sized,
    /// left-aligned, secondary — the caption a settings window puts under a
    /// group rather than beside a control.
    private func sectionFootnote(_ text: String) -> some View {
        Text(text)
            .font(.footnote)
            .ink(.secondary)
            .fixedSize(horizontal: false, vertical: true)
            .frame(maxWidth: .infinity, alignment: .leading)
    }

    /// One row of a grouped form: the field's name in the left column and
    /// its control alone in the right, on one line — and, only where the
    /// last write of this key was refused, what `nat` said about it under
    /// the control it was refused from.
    private func settingRow(
        title: String,
        key: String? = nil,
        @ViewBuilder control: () -> some View
    ) -> some View {
        LabeledContent {
            VStack(alignment: .leading, spacing: 4) {
                control()
                if let key, let error = fieldErrors[key] {
                    Label(error, systemImage: "exclamationmark.triangle.fill")
                        .font(.footnote)
                        .ink(.danger)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
        } label: {
            Text(title)
        }
    }

    @ViewBuilder
    private func agentRows(
        modelKey: String,
        effortKey: String,
        model: Binding<String>,
        effort: Binding<String>
    ) -> some View {
        settingRow(title: "Model", key: modelKey) {
            defaultablePicker(model, options: ["sonnet", "opus", "haiku"])
        }

        settingRow(title: "Effort", key: effortKey) {
            defaultablePicker(effort, options: ["low", "med", "high"])
        }
    }

    private func workingDirRow(projectID: String) -> some View {
        settingRow(
            title: projectNames[projectID] ?? projectID,
            key: SettingsModel.workingDirKey(projectID: projectID)
        ) {
            commitField(workingDirBinding(projectID: projectID), width: FieldWidth.path)
                .font(.system(.body, design: .monospaced))
        }
    }

    // MARK: - Controls

    /// A text field that writes what it holds when the user is done with it:
    /// on Return, and on the focus moving off it, which is the two ways a
    /// person finishes with a field in a settings window.
    private func commitField(_ text: Binding<String>, width: CGFloat? = nil) -> some View {
        CommitTextField(text: text, width: width, commit: commit)
    }

    /// A picker over the values `nat` takes for a key, plus the one it takes
    /// for "say nothing": a field cleared back to empty is unset, the config
    /// file's own spelling of it, and a picker needs a real option selected,
    /// so "Default" stands in for "" on the way in and out.
    ///
    /// Whatever the config already holds is an option too, wherever that is
    /// not one of the known ones: `config-set` takes any string for these
    /// keys — the TUI's own form is free text — and a picker that did not
    /// carry the stored value would show a blank selection for it and lose it
    /// to the first other choice made.
    private func defaultablePicker(_ value: Binding<String>, options: [String]) -> some View {
        Picker("", selection: defaultableBinding(value)) {
            Text("Default").tag(defaultTag)
            ForEach(withStored(value.wrappedValue, in: options), id: \.self) { option in
                Text(option).tag(option)
            }
        }
        .labelsHidden()
        .frame(width: FieldWidth.picker)
        .onChange(of: value.wrappedValue) { commit() }
    }

    private func withStored(_ stored: String, in options: [String]) -> [String] {
        guard !stored.isEmpty, !options.contains(stored) else { return options }
        return options + [stored]
    }

    private var defaultTag: String { "Default" }

    private func defaultableBinding(_ base: Binding<String>) -> Binding<String> {
        Binding(
            get: { base.wrappedValue.isEmpty ? defaultTag : base.wrappedValue },
            set: { base.wrappedValue = $0 == defaultTag ? "" : $0 }
        )
    }

    /// The stored string as the enum the picker selects over, so an unwritten
    /// or unrecognised value arrives as `system` rather than as a selection
    /// matching no option.
    private var themeBinding: Binding<Theme> {
        Binding(
            get: { Theme(stored: storedTheme) },
            set: { storedTheme = $0.rawValue }
        )
    }

    private func workingDirBinding(projectID: String) -> Binding<String> {
        Binding(
            get: { edited.projectWorkingDirs[projectID] ?? "" },
            set: { edited.projectWorkingDirs[projectID] = $0 }
        )
    }

    private var sortedProjectIDs: [String] {
        projectNames.keys.sorted { (projectNames[$0] ?? $0) < (projectNames[$1] ?? $1) }
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

    /// Queues a save behind whatever save is already running. Two fields
    /// committed in the same breath would otherwise both diff against the
    /// baseline the first has not moved yet, and write the first key twice.
    private func commit() {
        let previous = saveChain
        saveChain = Task {
            await previous?.value
            await save()
        }
    }

    private func save() async {
        guard let original else { return }
        let changes = SettingsModel.changes(from: original, to: edited)
        guard !changes.isEmpty else { return }

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

        await appModel.reloadConfig()
    }
}

/// The widths the controls are pinned to, so the rows of a tab line up
/// down the value column instead of each one sizing to its own content.
private enum FieldWidth {
    static let segments: CGFloat = 240
    static let number: CGFloat = 80
    static let picker: CGFloat = 140
    static let path: CGFloat = 260
}

/// The wait on the config read, as one row of the form rather than a hole
/// the height of a section. It reveals late the way `QuietLoadingView` does
/// — a read that lands inside `LoadingDelay` never shows anything — but by
/// fading in rather than by appearing, since a row that arrives resizes the
/// section it is in and the window around it.
private struct SettingsLoadingRow: View {
    @State private var isRevealed = false

    var body: some View {
        HStack(spacing: 8) {
            ProgressView()
                .controlSize(.small)
            Text("Loading configuration…")
                .ink(.secondary)
        }
        .opacity(isRevealed ? 1 : 0)
        .task { isRevealed = await LoadingDelay().shouldReveal() }
    }
}

/// The keys the form writes, as `internal/cli/configset.go` names them —
/// here rather than in the rows so a row and the error shown under it cannot
/// name the key differently.
private enum SettingsKey {
    static let pollSeconds = "poll_seconds"
    static let workshopModel = "workshop_agent.model"
    static let workshopEffort = "workshop_agent.effort"
    static let sliceModel = "slice_agent.model"
    static let sliceEffort = "slice_agent.effort"
}

/// A `TextField` that tells its owner when the user has finished with it:
/// Return, or the focus moving elsewhere. Kept as a view of its own because
/// the focus state has to belong to the field rather than to the form.
private struct CommitTextField: View {
    @Binding var text: String
    let width: CGFloat?
    let commit: () -> Void

    @FocusState private var isFocused: Bool

    var body: some View {
        TextField("", text: $text)
            .textFieldStyle(.roundedBorder)
            // The value column is trailing-aligned, and a field left to
            // inherit that alignment right-aligns the text inside itself.
            .multilineTextAlignment(.leading)
            .frame(width: width)
            .focused($isFocused)
            .onSubmit { commit() }
            .onChange(of: isFocused) { _, focused in
                if !focused { commit() }
            }
    }
}

private extension View {
    /// What every one of the tabs' forms is: the platform's grouped form,
    /// scrolling only where the pane it is in is too small to hold it — the
    /// window sizes to the form, so usually it is not.
    func settingsForm() -> some View {
        formStyle(.grouped)
            .scrollBounceBehavior(.basedOnSize)
    }
}

#Preview {
    SettingsView(appModel: AppModel())
}
