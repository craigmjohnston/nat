import AppKit
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
/// The scene's four tabs, named so a story can open on one other than
/// General — the tab builder's own `TabView` selection has no other seam.
enum SettingsTab: Hashable {
    case general, agents, projects, sources
}

struct SettingsView: View {
    @Bindable var appModel: AppModel

    /// What the form reads the config through and writes it back with — nat
    /// itself in the app, and a canned one in a story, which is the only way
    /// a settings screen can be drawn without spawning a `nat config show`
    /// against whatever machine is rendering it.
    var client: NatClientProtocol = NatClient()

    @State private var selectedTab: SettingsTab

    /// - Parameters:
    ///   - initialTab: Which tab the scene opens on — General for the window
    ///     itself, and whichever tab's own story wants to show for a gallery
    ///     capture.
    ///   - plugins: The Sources tab's model, already driven — a story's, to
    ///     draw what a Save came to; the window makes its own.
    init(
        appModel: AppModel, client: NatClientProtocol = NatClient(), initialTab: SettingsTab = .general,
        plugins: PluginsModel? = nil
    ) {
        self.appModel = appModel
        self.client = client
        _selectedTab = State(initialValue: initialTab)
        _plugins = State(initialValue: plugins ?? PluginsModel(client: client) { [appModel] in
            await appModel.reloadSourcePlugins()
        })
    }

    /// The theme, which is this app's own preference rather than one of
    /// nat's: it is written to `UserDefaults` the moment it is picked and
    /// takes effect at once, so it is no part of the config form's diff and
    /// is shown whatever became of the config read — including while that
    /// read is still in flight, or has failed.
    @AppStorage(Theme.storageKey) private var storedTheme = Theme.system.rawValue
    @AppStorage(PaletteChoice.darkStorageKey) private var storedDarkPalette = PaletteChoice.defaultDark.rawValue
    @AppStorage(PaletteChoice.lightStorageKey) private var storedLightPalette = PaletteChoice.defaultLight.rawValue

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

    /// The models and effort levels the Agents tab offers, read once from
    /// `AgentOptionsCache` — the fallback list until that read lands, since a
    /// blank picker while it is in flight would be worse than the stale-but-
    /// reasonable answer it starts with.
    @State private var agentOptions = AgentOptions.fallback

    /// The Sources tab: `nat plugin-list` and the buttons over it.
    @State private var plugins: PluginsModel

    var body: some View {
        // The macOS 15 tab builder rather than `.tabItem`, which is the
        // current spelling of the same thing: the settings window's toolbar
        // comes out `.preference` either way — read off `NSApp`'s own window
        // at runtime — so the tabs already have the per-item metrics
        // Safari's do, and there is no style to force.
        TabView(selection: $selectedTab) {
            Tab("General", systemImage: "gearshape", value: SettingsTab.general) {
                generalTab
            }
            Tab("Agents", systemImage: "sparkles", value: SettingsTab.agents) {
                agentsTab
            }
            Tab("Projects", systemImage: "folder", value: SettingsTab.projects) {
                projectsTab
            }
            Tab("Sources", systemImage: "puzzlepiece.extension", value: SettingsTab.sources) {
                sourcesTab
            }
        }
        // Width alone: the height is the tab's own, which is what makes the
        // window resize to each tab the way a settings window does.
        .frame(width: 520)
        .task { await load() }
        .task { agentOptions = await AgentOptionsCache.shared.resolve() }
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
                    // Wide enough for its three words and no wider: a stock
                    // segmented control in a grouped form is the width of
                    // what it holds, trailing-aligned in the value column,
                    // rather than stretched across it.
                    .fixedSize()
                }
                paletteRow(title: "Light theme", dark: false, stored: $storedLightPalette)
                paletteRow(title: "Dark theme", dark: true, stored: $storedDarkPalette)
            } footer: {
                sectionFootnote("The palettes the app draws with, the agent terminal included. System switches between the two as the Mac does. Applies at once.")
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
                "Task agent",
                footer: "Which Claude Code a task's agent runs as, and how hard it thinks, unless the launch itself overrides them. Applies at the next launch."
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
                footer: "Where a project's agents start, unless a task names its own repo. Applies at the next launch."
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

    /// The task-source plugins: what is installed, what the plugin sources
    /// offer, and the sources themselves — all `nat plugin-list`, read the
    /// first time the tab is shown and again after every button, each of
    /// which is one `nat plugin-*` call (`PluginsModel`).
    private var sourcesTab: some View {
        Form {
            if let listing = plugins.listing {
                if let error = plugins.actionError {
                    Section {
                        errorLabel(error)
                    }
                }
                installedSection(listing)
                availableSection(listing)
                pluginSourcesSection(listing)
            } else {
                Section {
                    if let error = plugins.loadError {
                        errorLabel(error)
                    } else {
                        SettingsLoadingRow(text: "Looking for plugins…")
                    }
                } header: {
                    Text("Task sources")
                }
            }
        }
        .settingsForm()
        .task { await plugins.loadIfNeeded() }
    }

    private func installedSection(_ listing: PluginListing) -> some View {
        Section {
            if listing.installed.isEmpty {
                Text("No task-source plugins installed.")
                    .ink(.secondary)
            }
            ForEach(listing.installed) { plugin in
                VStack(alignment: .leading, spacing: 8) {
                    InstalledPluginRow(
                        plugin: plugin,
                        updating: plugins.running.contains(.update(name: plugin.name)),
                        uninstalling: plugins.running.contains(.uninstall(name: plugin.name)),
                        update: { Task { await plugins.update(plugin) } },
                        uninstall: { Task { await plugins.uninstall(plugin) } }
                    )
                    if !plugin.describeError.isEmpty {
                        Label(plugin.describeError, systemImage: "exclamationmark.triangle.fill")
                            .font(.footnote)
                            .ink(.warning)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    ForEach(plugin.setup) { field in
                        setupFieldRow(plugin: plugin.name, field: field)
                    }
                }
            }
        } header: {
            Text("Installed")
        } footer: {
            sectionFootnote(
                "Each plugin adds a \u{201C}New \u{2026} project\u{201D} item to the + menu, for a project whose cards "
                    + "come from that service. Updates come from the sources below. A plugin marked manual or on "
                    + "PATH was installed outside gnat and is left as you put it.")
        }
    }

    /// One of a plugin's setup fields beneath its row: whether the plugin
    /// holds a value for it (where it says), the label, a secure field (or a
    /// plain one for `text`) and Save, the hint under them, then what the
    /// last Save came to. The value goes to `nat source-setup` on
    /// stdin, through `PluginsModel`.
    private func setupFieldRow(plugin: String, field: PluginSetupField) -> some View {
        let key = PluginsModel.SetupKey(plugin: plugin, field: field.id)
        let value = Binding(
            get: { plugins.setupValues[key] ?? "" },
            set: { plugins.setupValues[key] = $0 })
        let save = { Task { await plugins.saveSetup(plugin: plugin, field: field.id) } }
        // A field already set is one a value would replace, and says so.
        let prompt = field.set == true ? Text("Replace \u{2026}") : nil
        return VStack(alignment: .leading, spacing: 4) {
            switch field.set {
            case false:
                Label("\(field.label) not set", systemImage: "exclamationmark.triangle.fill")
                    .font(.footnote)
                    .ink(.warning)
            case true:
                Label("\(field.label) set", systemImage: "checkmark")
                    .font(.footnote)
                    .ink(.secondary)
            case nil:
                EmptyView()
            }
            HStack(spacing: 8) {
                Text(field.label)
                Group {
                    if field.isSecret {
                        SecureField(field.label, text: value, prompt: prompt)
                    } else {
                        TextField(field.label, text: value, prompt: prompt)
                    }
                }
                .textFieldStyle(.roundedBorder)
                .labelsHidden()
                .onSubmit { _ = save() }
                PluginActionButton(title: "Save", running: plugins.running.contains(.setup(key))) { _ = save() }
                    .disabled(!plugins.canSave(key))
            }
            if !field.hint.isEmpty {
                Text(field.hint)
                    .font(.footnote)
                    .ink(.secondary)
            }
            switch plugins.setupOutcomes[key] {
            case .saved(let message):
                Label(message, systemImage: "checkmark.circle.fill")
                    .font(.footnote)
                    .ink(.success)
                    .fixedSize(horizontal: false, vertical: true)
            case .refused(let message):
                Label(message, systemImage: "exclamationmark.triangle.fill")
                    .font(.footnote)
                    .ink(.danger)
                    .fixedSize(horizontal: false, vertical: true)
            case nil:
                EmptyView()
            }
        }
    }

    private func availableSection(_ listing: PluginListing) -> some View {
        Section {
            if listing.available.isEmpty {
                Text("No plugin source offers a plugin yet.")
                    .ink(.secondary)
            }
            ForEach(listing.available) { plugin in
                AvailablePluginRow(
                    plugin: plugin,
                    installing: plugins.running.contains(.install(source: plugin.source, name: plugin.name)),
                    install: { Task { await plugins.install(plugin) } }
                )
            }
        } header: {
            Text("Available")
        }
    }

    private func pluginSourcesSection(_ listing: PluginListing) -> some View {
        Section {
            ForEach(listing.sources) { source in
                PluginSourceRow(
                    source: source,
                    removing: plugins.running.contains(.removeSource(repo: source.repo)),
                    remove: { Task { await plugins.removeSource(source.repo) } }
                )
            }
            HStack(spacing: 8) {
                TextField("Plugin source", text: $plugins.newSource, prompt: Text("owner/repo"))
                    .textFieldStyle(.roundedBorder)
                    .labelsHidden()
                    .onSubmit { Task { await plugins.addSource() } }
                if plugins.running.contains(.addSource) {
                    ProgressView().controlSize(.small)
                }
                Button("Add") { Task { await plugins.addSource() } }
                    .disabled(!plugins.canAddSource)
            }
        } header: {
            Text("Plugin sources")
        } footer: {
            sectionFootnote("GitHub repositories that publish plugins. nat's own is always checked first.")
        }
    }

    private func errorLabel(_ message: String) -> some View {
        Label(message, systemImage: "exclamationmark.triangle.fill")
            .ink(.danger)
            .fixedSize(horizontal: false, vertical: true)
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
            // A grouped form sets its footers' multi-line alignment trailing;
            // a caption that wraps reads ragged-right, as System Settings'.
            .multilineTextAlignment(.leading)
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
            ModelPicker(value: model, options: agentOptions.models, commit: commit) { text in
                commitField(text, width: FieldWidth.model)
            }
            .frame(width: FieldWidth.model)
            .help("An alias (\(agentOptions.models.joined(separator: ", "))), Custom for a full model ID, or Default to leave it to Claude Code.")
        }

        settingRow(title: "Effort", key: effortKey) {
            defaultablePicker(effort, options: agentOptions.efforts)
        }
    }

    /// A path is as long as it is, so the field takes the whole value column
    /// rather than a stub of it and says the rest in its tooltip — and beside
    /// it the button every native path row has, since typing a path out is
    /// not how anyone picks a directory.
    private func workingDirRow(projectID: String) -> some View {
        let path = workingDirBinding(projectID: projectID)
        return settingRow(
            title: projectNames[projectID] ?? projectID,
            key: SettingsModel.workingDirKey(projectID: projectID)
        ) {
            HStack(spacing: 8) {
                commitField(path)
                    // An ideal width well short of any real path, so the
                    // row's own label and control stay on one line — a
                    // field asking for the width of what it holds is what
                    // sends `LabeledContent` into its stacked layout — and
                    // then all the width the value column has left.
                    .frame(minWidth: 0, idealWidth: 160, maxWidth: .infinity)
                    .help(path.wrappedValue)
                Button("Choose…") { chooseDirectory(into: path) }
            }
        }
    }

    /// The open panel as a settings window opens one: directories only,
    /// started wherever the field already points when that is a directory
    /// that exists, and writing what was chosen into the very binding the
    /// field edits — so a choice commits exactly as a typed path does, and
    /// one that changed nothing writes nothing, the diff having no change to
    /// find. A cancelled panel writes nothing at all.
    ///
    /// `NSOpenPanel` rather than `fileImporter`: there is no presentation
    /// state to hold per row, and the app is not sandboxed.
    private func chooseDirectory(into path: Binding<String>) {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        panel.prompt = "Choose"
        if let start = existingDirectory(path.wrappedValue) {
            panel.directoryURL = start
        }
        guard panel.runModal() == .OK, let url = panel.url else { return }
        path.wrappedValue = url.path
        commit()
    }

    private func existingDirectory(_ path: String) -> URL? {
        guard !path.isEmpty else { return nil }
        var isDirectory: ObjCBool = false
        guard FileManager.default.fileExists(atPath: path, isDirectory: &isDirectory),
              isDirectory.boolValue else { return nil }
        return URL(fileURLWithPath: path)
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

    /// One slot's palette picker: the palettes of that scheme, by name.
    private func paletteRow(title: String, dark: Bool, stored: Binding<String>) -> some View {
        settingRow(title: title) {
            Picker(title, selection: Binding(
                get: { PaletteChoice(stored: stored.wrappedValue, dark: dark) },
                set: { stored.wrappedValue = $0.rawValue }
            )) {
                ForEach(PaletteChoice.choices(dark: dark)) { choice in
                    Text(choice.title).tag(choice)
                }
            }
            .labelsHidden()
            .pickerStyle(.menu)
            .fixedSize()
        }
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
            let doc = try await client.configShow()
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
                try await client.configSet(key: change.key, value: change.value)
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

/// The one width a control here is pinned to. Everything else sizes to its
/// own content and trailing-aligns in the value column, as a stock control
/// in a grouped form does; a small numeric field is the exception, since a
/// field for two digits drawn the width of the column is what no settings
/// window has.
private enum FieldWidth {
    static let number: CGFloat = 80
    static let model: CGFloat = 160
}

/// The wait on the config read, as one row of the form rather than a hole
/// the height of a section. It reveals late the way `QuietLoadingView` does
/// — a read that lands inside `LoadingDelay` never shows anything — but by
/// fading in rather than by appearing, since a row that arrives resizes the
/// section it is in and the window around it.
private struct SettingsLoadingRow: View {
    var text = "Loading configuration…"
    @State private var isRevealed = false

    var body: some View {
        HStack(spacing: 8) {
            ProgressView()
                .controlSize(.small)
            Text(text)
                .ink(.secondary)
        }
        .opacity(isRevealed ? 1 : 0)
        .task { isRevealed = await LoadingDelay().shouldReveal() }
    }
}

/// A button that is a spinner while its command runs — the stock small
/// progress indicator in its place, so the row keeps its shape.
private struct PluginActionButton: View {
    let title: String
    let running: Bool
    var role: ButtonRole?
    let action: () -> Void

    var body: some View {
        if running {
            ProgressView()
                .controlSize(.small)
        } else {
            Button(title, role: role, action: action)
        }
    }
}

/// One installed plugin: its name and version (or how it got there when nat
/// did not install it) over the path it runs from, then Update where its
/// source has a newer release, and Uninstall where it is nat's to remove.
private struct InstalledPluginRow: View {
    let plugin: InstalledPlugin
    let updating: Bool
    let uninstalling: Bool
    let update: () -> Void
    let uninstall: () -> Void

    var body: some View {
        HStack(alignment: .center, spacing: 10) {
            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 6) {
                    Text(plugin.name)
                    Text(plugin.versionLabel)
                        .font(.footnote)
                        .ink(.secondary)
                }
                Text(plugin.path)
                    .font(.footnote)
                    .ink(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .help(plugin.path)
            }
            Spacer(minLength: 0)
            if plugin.hasUpdate {
                PluginActionButton(title: "Update to \(plugin.update)", running: updating, action: update)
            }
            if plugin.isUninstallable {
                PluginActionButton(title: "Uninstall", running: uninstalling, action: uninstall)
            }
        }
        .padding(.vertical, 2)
    }
}

/// One plugin a source offers: its title and version, what it is, and where
/// from — with Install, or a quiet "Installed" for one already here.
private struct AvailablePluginRow: View {
    let plugin: AvailablePlugin
    let installing: Bool
    let install: () -> Void

    var body: some View {
        HStack(alignment: .center, spacing: 10) {
            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 6) {
                    Text(plugin.displayTitle)
                    Text(plugin.version)
                        .font(.footnote)
                        .ink(.secondary)
                }
                if !plugin.description.isEmpty {
                    Text(plugin.description)
                        .font(.footnote)
                        .ink(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                Text(plugin.source)
                    .font(.footnote)
                    .ink(.tertiary)
            }
            Spacer(minLength: 0)
            if plugin.installed {
                Text("Installed")
                    .font(.footnote)
                    .ink(.secondary)
            } else {
                PluginActionButton(title: "Install", running: installing, action: install)
            }
        }
        .padding(.vertical, 2)
    }
}

/// One plugin source: the repository and its latest release, or nat's reason
/// it could not be read; nat's own marked, every other removable.
private struct PluginSourceRow: View {
    let source: PluginSourceStatus
    let removing: Bool
    let remove: () -> Void

    var body: some View {
        HStack(alignment: .center, spacing: 10) {
            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 6) {
                    Text(source.repo)
                    if !source.version.isEmpty {
                        Text(source.version)
                            .font(.footnote)
                            .ink(.secondary)
                    }
                }
                if !source.error.isEmpty {
                    Label(source.error, systemImage: "exclamationmark.triangle.fill")
                        .font(.footnote)
                        .ink(.danger)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            Spacer(minLength: 0)
            if source.isDefault {
                Text("Default")
                    .font(.footnote)
                    .ink(.secondary)
            } else {
                PluginActionButton(title: "Remove", running: removing, action: remove)
            }
        }
        .padding(.vertical, 2)
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
            // The system font, not the app's monospaced face: a settings
            // window's fields are set in the face every other settings
            // window on the Mac sets its own in.
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
