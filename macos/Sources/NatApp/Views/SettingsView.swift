import AppKit
import SwiftUI
import NatKit

/// The Settings scene (⌘,), laid out as 1Password's settings are: a sidebar
/// of sections down the left under a "Settings" heading (the window has no
/// title bar, the traffic lights over the sidebar), each a tinted tile
/// beside its name, and the chosen section's form on the plain window ground
/// to its right — bold group headings over left-aligned labelled stock
/// controls. The window is
/// one fixed size whichever section is up (`SettingsLayout`); a section
/// taller than it scrolls.
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
/// The window's sections, in sidebar order, named so a story can open on one
/// other than General.
enum SettingsTab: Hashable, CaseIterable, Identifiable {
    case general, agents, sources, github, about

    var id: Self { self }

    var title: String {
        switch self {
        case .general: "General"
        case .agents: "Agents"
        case .sources: "Sources"
        case .github: "GitHub"
        case .about: "About"
        }
    }

    var symbol: String {
        switch self {
        case .general: "gearshape.fill"
        case .agents: "sparkles"
        case .sources: "puzzlepiece.extension.fill"
        case .github: "archivebox.fill"
        case .about: "info"
        }
    }

    /// The tile's ground, one hue per section so the column reads at a
    /// glance as the reference's does — fixed across palettes
    /// (`DesignTokens.tileNavy` and the rest), shaded top to bottom.
    var tint: LinearGradient {
        switch self {
        case .general: DesignTokens.tileNavy.gradient
        case .agents: DesignTokens.tileAmber.gradient
        case .sources: DesignTokens.tileAzure.gradient
        case .github: DesignTokens.tileGraphite.gradient
        case .about: DesignTokens.tileIndigo.gradient
        }
    }

    /// About stands apart from the settings proper, a rule above it.
    var startsGroup: Bool { self == .about }
}

struct SettingsView: View {
    @Bindable var appModel: AppModel

    /// What the form reads the config through and writes it back with — nat
    /// itself in the app, and a canned one in a story, which is the only way
    /// a settings screen can be drawn without spawning a `nat config show`
    /// against whatever machine is rendering it.
    var client: NatClientProtocol = NatClient()

    /// Sparkle, for About's Check for Updates — the app's own; a story has
    /// none, and draws the button disabled as a dev build's is.
    var updater: UpdaterViewModel?

    @State private var selectedTab: SettingsTab

    /// About's Diagnostics drawn open — a story's; the window opens it folded.
    private let diagnosticsExpanded: Bool

    /// - Parameters:
    ///   - initialTab: Which section the window opens on — General for the
    ///     window itself, and whichever section a story wants to show.
    ///   - plugins: The Sources section's model, already driven — a story's,
    ///     to draw what a Save came to; the window makes its own.
    ///   - diagnosticsExpanded: About's Diagnostics drawn open, for a story.
    ///   - storage: The GitHub section's model, already read — a story's; the
    ///     window makes its own, read when the section is first shown.
    init(
        appModel: AppModel, client: NatClientProtocol = NatClient(), updater: UpdaterViewModel? = nil,
        initialTab: SettingsTab = .general, plugins: PluginsModel? = nil, diagnosticsExpanded: Bool = false,
        storage: StorageUsageModel? = nil
    ) {
        self.appModel = appModel
        self.diagnosticsExpanded = diagnosticsExpanded
        self.client = client
        self.updater = updater
        _selectedTab = State(initialValue: initialTab)
        _storage = State(initialValue: storage ?? StorageUsageModel(client: client))
        _plugins = State(initialValue: plugins ?? PluginsModel(
            client: client, projectsUsing: { [appModel] in appModel.sourceProjectNames(of: $0) }
        ) { [appModel] change in
            await appModel.pluginChanged(change)
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
    /// The two text sizes, the theme's kind of preference and kept the same
    /// way: `UserDefaults`, live, never `nat config-set`.
    @AppStorage(TypeSize.uiStorageKey) private var storedUISize = TypeSize.defaultUI
    @AppStorage(TypeSize.monoStorageKey) private var storedMonoSize = TypeSize.defaultMono

    @State private var original: SettingsFields?
    @State private var edited = SettingsFields(
        pollSeconds: "",
        workshopModel: "", workshopEffort: "",
        sliceModel: "", sliceEffort: ""
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

    /// The GitHub section: `nat storage-usage`, read when first shown.
    @State private var storage: StorageUsageModel

    /// nat's own version for About, read once that section is first shown:
    /// nil until then, and `natVersionFailed` where the read was refused.
    @State private var natVersion: String?
    @State private var natVersionFailed = false

    var body: some View {
        // One fixed size rather than each section's own height: a split
        // window resizing under the pointer as the sidebar is clicked down
        // is not what a sidebar-list settings window does.
        HStack(spacing: 0) {
            SettingsSidebar(selection: $selectedTab)
                .frame(width: SettingsLayout.sidebarWidth)
            Divider()
            detail
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        }
        // No title bar: the panes run to the window's top edge, the traffic
        // lights over the sidebar, and the sidebar carries the heading. The
        // bar is still there, transparent and untitled (so it drags and its
        // buttons work), and SwiftUI still reserves its height as a top safe
        // area — given back here, the panes keeping the bar's height clear
        // themselves, so a story, drawn in a window with no bar at all, lays
        // out as the real window does. The window is sized to the ideal
        // height plus that inset, so the ideal is the window's height less
        // the bar's; the max lets a story's barless window have it all. (A
        // measured inset fed back into the frame oscillated, 0 and 28 in
        // turn, as the window resized under it.)
        .frame(width: SettingsLayout.windowSize.width)
        .frame(maxHeight: .infinity)
        .ignoresSafeArea(.container, edges: .top)
        .frame(
            minHeight: SettingsLayout.windowSize.height - SettingsLayout.titlebarHeight,
            idealHeight: SettingsLayout.windowSize.height - SettingsLayout.titlebarHeight,
            maxHeight: SettingsLayout.windowSize.height)
        .toolbar(removing: .title)
        .toolbarBackgroundVisibility(.hidden, for: .windowToolbar)
        .navigationTitle("Settings")
        // About gnat's request, taken whether it was waiting for this
        // window or arrives while it is open on another section.
        .onChange(of: appModel.settingsAboutRequested, initial: true) {
            if appModel.takeSettingsAboutRequest() { selectedTab = .about }
        }
        .task { await load() }
        .task { agentOptions = await AgentOptionsCache.shared.resolve() }
        // A window closed on a field still focused would otherwise take that
        // edit with it: the focus change never arrives, because the view is
        // gone. The commit chain outlives the view, so this one lands.
        .onDisappear { commit() }
    }

    // MARK: - Tabs

    @ViewBuilder
    private var detail: some View {
        switch selectedTab {
        case .general: generalTab
        case .agents: agentsTab
        case .sources: sourcesTab
        case .github: githubTab
        case .about: aboutTab
        }
    }

    private var generalTab: some View {
        VStack(alignment: .leading, spacing: SettingsLayout.groupSpacing) {
            settingsGroup(
                "Appearance",
                footer: "The palettes the app draws with, the agent terminal included. System switches between the two as the Mac does. Applies at once."
            ) {
                settingRow(title: "Theme") {
                    Picker("Theme", selection: themeBinding) {
                        ForEach(Theme.allCases) { theme in
                            Text(theme.title).tag(theme)
                        }
                    }
                    .labelsHidden()
                    .pickerStyle(.segmented)
                    // Wide enough for its three words and no wider, rather
                    // than stretched across the value column.
                    .fixedSize()
                }
                paletteRow(title: "Light theme", dark: false, stored: $storedLightPalette)
                paletteRow(title: "Dark theme", dark: true, stored: $storedDarkPalette)
            }

            settingsGroup(
                "Text",
                footer: "Text size is a sidebar row's, and the rest of the app's type follows it in proportion. Code size is the agent terminal's and the diff's. Applies at once."
            ) {
                sizeRow(title: "Text size", range: TypeSize.uiRange, stored: $storedUISize)
                sizeRow(title: "Code size", range: TypeSize.monoRange, stored: $storedMonoSize)
            }
        }
        .settingsForm()
    }

    private var agentsTab: some View {
        VStack(alignment: .leading, spacing: SettingsLayout.groupSpacing) {
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

    /// The task-source plugins: what is installed, what the plugin sources
    /// offer, and the sources themselves — all `nat plugin-list`, read the
    /// first time the tab is shown and again after every button, each of
    /// which is one `nat plugin-*` call (`PluginsModel`).
    private var sourcesTab: some View {
        VStack(alignment: .leading, spacing: SettingsLayout.groupSpacing) {
            if let listing = plugins.listing {
                if let error = plugins.actionError {
                    errorLabel(error)
                }
                installedSection(listing)
                availableSection(listing)
                pluginSourcesSection(listing)
            } else {
                settingsGroup("Task sources") {
                    if let error = plugins.loadError {
                        errorLabel(error)
                    } else {
                        SettingsLoadingRow(text: "Looking for plugins…")
                    }
                }
            }
        }
        .settingsForm()
        .task { await plugins.loadIfNeeded() }
        .alert(
            plugins.pendingUninstall.map { "Uninstall \($0.plugin.name) and delete its projects?" } ?? "",
            isPresented: Binding(
                get: { plugins.pendingUninstall != nil },
                set: { if !$0 { plugins.cancelUninstall() } }),
            presenting: plugins.pendingUninstall
        ) { pending in
            // The alert's own dismissal clears the pending uninstall before a
            // task runs, so the button hands over the one it was drawn for.
            Button("Delete and Uninstall", role: .destructive) {
                Task { await plugins.confirmUninstall(pending) }
            }
            Button("Cancel", role: .cancel) { plugins.cancelUninstall() }
        } message: { pending in
            Text("These projects and their tasks are deleted with it:\n" + pending.projects.joined(separator: "\n"))
        }
    }

    private func installedSection(_ listing: PluginListing) -> some View {
        settingsGroup(
            "Installed",
            footer: "Connecting a plugin \u{2014} setting what it asks for here \u{2014} adds its section to the sidebar, "
                + "its cards from that service. Updates come from the sources below. A plugin marked manual or on "
                + "PATH was installed outside gnat and is left as you put it."
        ) {
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
                .font(.system(size: Typo.input))
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
        settingsGroup("Available") {
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
        }
    }

    private func pluginSourcesSection(_ listing: PluginListing) -> some View {
        settingsGroup(
            "Plugin sources",
            footer: "GitHub repositories that publish plugins. nat's own is always checked first."
        ) {
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
                    .font(.system(size: Typo.input))
                    .labelsHidden()
                    .onSubmit { Task { await plugins.addSource() } }
                if plugins.running.contains(.addSource) {
                    ProgressView().controlSize(.small)
                }
                Button("Add") { Task { await plugins.addSource() } }
                    .disabled(!plugins.canAddSource)
            }
        }
    }

    /// GitHub: this month's artifact storage against the plan's allowance, a
    /// segment of the bar per project in its colour and a grey one for every
    /// other repository, each project's figure listed under it. Read the first
    /// time the section is shown and again on Refresh — never polled.
    private var githubTab: some View {
        VStack(alignment: .leading, spacing: SettingsLayout.groupSpacing) {
            settingsGroup(
                "Artifact storage",
                footer: "GitHub Actions artifacts kept this month, as GitHub's billing page counts them, against "
                    + "what your plan includes. A project's share is its repositories'; caches are not counted."
            ) {
                StorageUsageSection(model: storage)
            }
        }
        .settingsForm()
        .task { await storage.loadIfNeeded() }
    }

    /// gnat itself: the icon, the name, its version and build, the embedded
    /// nat's version under them, Check for Updates through Sparkle and the
    /// repository. Nothing here is editable.
    private var aboutTab: some View {
        VStack(spacing: 6) {
            AppIconImage()
                .frame(width: 128, height: 128)
                .padding(.bottom, 6)
            Text("gnat")
                .font(.title.bold())
            Text(AppVersion(infoDictionary: Bundle.main.infoDictionary).label)
                .ink(.secondary)
                .textSelection(.enabled)
            Text(natVersionLine)
                .ink(.secondary)
                .textSelection(.enabled)
            Group {
                if let updater {
                    CheckForUpdatesView(model: updater, title: "Check for Updates\u{2026}")
                } else {
                    Button("Check for Updates\u{2026}") {}
                        .disabled(true)
                }
            }
            .padding(.top, 12)
            Link("github.com/craigmjohnston/nat", destination: SettingsLayout.repositoryURL)
                .padding(.top, 8)
            DiagnosticsFoldout(store: appModel.githubReadingStore, initiallyExpanded: diagnosticsExpanded)
                .frame(maxWidth: SettingsLayout.diagnosticsWidth)
                .padding(.top, 24)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .padding(SettingsLayout.detailInsets)
        .task { await loadNatVersion() }
    }

    /// The embedded nat's version — blank while the read is out (it is one
    /// short process), and saying so where nat would not answer.
    private var natVersionLine: String {
        if natVersionFailed { return "nat version unknown" }
        guard let natVersion else { return " " }
        return "nat \(natVersion)"
    }

    private func loadNatVersion() async {
        guard natVersion == nil else { return }
        do {
            natVersion = try await client.natVersion()
            natVersionFailed = false
        } catch {
            natVersionFailed = true
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
    private func configSection(
        _ title: String,
        footer: String? = nil,
        @ViewBuilder content: () -> some View
    ) -> some View {
        // Nothing to footnote while the section is holding a wait or a
        // refusal instead of the fields the footnote is about.
        settingsGroup(title, footer: isLoading || loadError != nil ? nil : footer) {
            if isLoading {
                SettingsLoadingRow()
            } else if let loadError {
                Label(loadError, systemImage: "exclamationmark.triangle.fill")
                    .ink(.danger)
                    .fixedSize(horizontal: false, vertical: true)
            } else {
                content()
            }
        }
    }

    /// One group of a section: its heading in bold body text, the reference's
    /// `Defaults` and `Keyboard Shortcuts`, its rows left-aligned under it,
    /// then the footnote saying what the rows mean beyond their own names.
    private func settingsGroup(
        _ title: String,
        footer: String? = nil,
        @ViewBuilder content: () -> some View
    ) -> some View {
        VStack(alignment: .leading, spacing: SettingsLayout.rowSpacing) {
            Text(title)
                .font(.body.bold())
            content()
            if let footer {
                sectionFootnote(footer)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    /// What a section's fields mean beyond their own names: footnote-sized,
    /// left-aligned, secondary — the caption a settings window puts under a
    /// group rather than beside a control.
    private func sectionFootnote(_ text: String) -> some View {
        Text(text)
            .font(.footnote)
            .ink(.secondary)
            .multilineTextAlignment(.leading)
            .fixedSize(horizontal: false, vertical: true)
            .frame(maxWidth: .infinity, alignment: .leading)
    }

    /// One row of a group: the field's name in a fixed label column and its
    /// control left-aligned after it, on one line — and, only where the last
    /// write of this key was refused, what `nat` said about it under the
    /// control it was refused from.
    private func settingRow(
        title: String,
        key: String? = nil,
        @ViewBuilder control: () -> some View
    ) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            Text(title)
                .lineLimit(1)
                .truncationMode(.tail)
                .help(title)
                .frame(width: SettingsLayout.labelWidth, alignment: .leading)
            VStack(alignment: .leading, spacing: 4) {
                control()
                if let key, let error = fieldErrors[key] {
                    Label(error, systemImage: "exclamationmark.triangle.fill")
                        .font(.footnote)
                        .ink(.danger)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    @ViewBuilder
    private func agentRows(
        modelKey: String,
        effortKey: String,
        model: Binding<String>,
        effort: Binding<String>
    ) -> some View {
        // Both pickers, in both agents' groups, in one frame, so all four
        // are one width.
        settingRow(title: "Model", key: modelKey) {
            ModelPicker(value: model, options: agentOptions.models, commit: commit) { text in
                commitField(text, width: FieldWidth.model)
            }
            .frame(width: FieldWidth.model, alignment: .trailing)
            .help("An alias (\(agentOptions.models.joined(separator: ", "))), Custom for a full model ID, or Default to leave it to Claude Code.")
        }

        settingRow(title: "Effort", key: effortKey) {
            defaultablePicker(effort, options: agentOptions.efforts)
                .frame(width: FieldWidth.model, alignment: .trailing)
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

    /// One text size: a points field and a stepper beside it, both writing
    /// the stored value clamped to `range`, so neither can store a size the
    /// app would not draw at.
    private func sizeRow(title: String, range: ClosedRange<Int>, stored: Binding<Int>) -> some View {
        let clamped = Binding(
            get: { min(max(stored.wrappedValue, range.lowerBound), range.upperBound) },
            set: { stored.wrappedValue = min(max($0, range.lowerBound), range.upperBound) }
        )
        return settingRow(title: title) {
            HStack(spacing: 4) {
                TextField(title, value: clamped, format: .number)
                    .font(.system(size: Typo.input))
                    .labelsHidden()
                    .multilineTextAlignment(.trailing)
                    .frame(width: FieldWidth.size)
                Text("pt")
                    .ink(.secondary)
                    .padding(.trailing, 2)
                Stepper(title, value: clamped, in: range)
                    .labelsHidden()
            }
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

    // MARK: - Loading and saving

    private func load() async {
        isLoading = true
        loadError = nil
        do {
            let doc = try await client.configShow()
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

/// The window's metrics, read off the reference: a 200pt sidebar of 32pt
/// tiles under its own heading in a 760×560 window with no title bar, and
/// the detail pane's generous insets.
enum SettingsLayout {
    static let windowSize = CGSize(width: 760, height: 560)
    /// About's Diagnostics foldout: wide enough for a label and its value on
    /// one line, centred under the rest of the section.
    static let diagnosticsWidth: CGFloat = 540
    static let sidebarWidth: CGFloat = 200
    static let tileSize: CGFloat = 32
    static let tileCornerRadius: CGFloat = 8
    static let rowCornerRadius: CGFloat = 8
    /// The band the hidden title bar leaves the traffic lights in: the
    /// standard bar's height, which both panes keep clear above their own
    /// content now that it runs to the window's top edge.
    static let titlebarHeight: CGFloat = 28
    /// The sidebar's "Settings" heading: well over the old title's size and
    /// weight, so it reads as the pane's heading, a little clear of the
    /// traffic lights' band and further clear of the first row.
    static let headingSize: CGFloat = 21
    static let headingTopGap: CGFloat = 6
    static let headingGap: CGFloat = 12
    /// The detail's top inset sets its first group heading's baseline level
    /// with the sidebar heading's, so the two read as one line.
    static let detailInsets = EdgeInsets(top: titlebarHeight + 14, leading: 32, bottom: 28, trailing: 32)
    static let groupSpacing: CGFloat = 28
    static let rowSpacing: CGFloat = 10
    static let labelWidth: CGFloat = 150
    static let repositoryURL = URL(string: "https://github.com/craigmjohnston/nat")!
}

/// The one width a control here is pinned to. Everything else sizes to its
/// own content, left-aligned in the value column; a small numeric field is
/// the exception, since a field for two digits drawn the width of the
/// column is what no settings window has.
private enum FieldWidth {
    static let size: CGFloat = 44
    static let model: CGFloat = 160
}

/// The column of sections down the window's left: a tinted tile and the
/// section's name per row, the selected row filled in the accent with its
/// name in the accent's own ink, the one under the pointer washed — and a
/// rule above About, which is about the app rather than a setting of it.
private struct SettingsSidebar: View {
    @Binding var selection: SettingsTab
    @State private var hovered: SettingsTab?

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            // The pane's heading, below the traffic lights rather than beside
            // them, over the tiles' own leading edge.
            Text("Settings")
                .font(.system(size: SettingsLayout.headingSize, weight: .bold))
                .padding(.horizontal, 4)
                .padding(.bottom, SettingsLayout.headingGap)
            ForEach(SettingsTab.allCases) { tab in
                if tab.startsGroup {
                    Divider()
                        .padding(.vertical, 6)
                        .padding(.horizontal, 6)
                }
                row(tab)
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 10)
        .padding(.top, SettingsLayout.titlebarHeight + SettingsLayout.headingTopGap)
        .padding(.bottom, 12)
        .frame(maxHeight: .infinity, alignment: .top)
        .background(SidebarMaterial())
    }

    private func row(_ tab: SettingsTab) -> some View {
        let selected = tab == selection
        return Button { selection = tab } label: {
            HStack(spacing: 10) {
                Image(systemName: tab.symbol)
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundStyle(DesignTokens.tileGlyph)
                    .frame(width: SettingsLayout.tileSize, height: SettingsLayout.tileSize)
                    .background(tab.tint, in: RoundedRectangle(cornerRadius: SettingsLayout.tileCornerRadius))
                    .overlay(
                        RoundedRectangle(cornerRadius: SettingsLayout.tileCornerRadius)
                            .strokeBorder(DesignTokens.tileStroke, lineWidth: 0.5))
                Text(tab.title)
                    .foregroundStyle(selected ? DesignTokens.accentText : DesignTokens.label)
                Spacer(minLength: 0)
            }
            .padding(4)
            .background(
                RoundedRectangle(cornerRadius: SettingsLayout.rowCornerRadius)
                    .fill(rowFill(tab, selected: selected)))
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .onHover { hovered = $0 ? tab : (hovered == tab ? nil : hovered) }
    }

    private func rowFill(_ tab: SettingsTab, selected: Bool) -> Color {
        if selected { return DesignTokens.accent }
        if hovered == tab { return DesignTokens.rowWash(selected: false, on: .chrome) }
        return Color.clear
    }
}

/// The sidebar's ground: AppKit's own sidebar material, as a source list
/// stands on.
private struct SidebarMaterial: NSViewRepresentable {
    func makeNSView(context: Context) -> NSVisualEffectView {
        let view = NSVisualEffectView()
        view.material = .sidebar
        view.blendingMode = .behindWindow
        view.state = .followsWindowActiveState
        return view
    }

    func updateNSView(_ nsView: NSVisualEffectView, context: Context) {}
}

/// The app's icon as the dock shows it — the paper icon in light, the
/// dark-navy one in dark (`NatApp.iconImage`), else whatever AppKit holds.
private struct AppIconImage: View {
    @Environment(\.colorScheme) private var colorScheme

    var body: some View {
        Image(nsImage: NatApp.iconImage(dark: colorScheme == .dark) ?? NSApplication.shared.applicationIconImage)
            .resizable()
            .interpolation(.high)
            .aspectRatio(contentMode: .fit)
    }
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
            // window on the Mac sets its own in — at the ramp's input size,
            // the one every field in the app shares.
            .font(.system(size: Typo.input))
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
    /// What every section's form is: its groups on the plain window ground,
    /// at the reference's insets, scrolling only where the section is taller
    /// than the fixed window.
    func settingsForm() -> some View {
        ScrollView {
            padding(SettingsLayout.detailInsets)
                .frame(maxWidth: .infinity, alignment: .topLeading)
        }
        .scrollBounceBehavior(.basedOnSize)
        .thinScrollers()
    }
}

#Preview {
    SettingsView(appModel: AppModel())
}

/// About's Diagnostics: a stock `DisclosureGroup`, folded on every opening of
/// the window (its state is the view's own, never kept), whose body is a
/// recessed well with a hairline border holding one row per line — GitHub's
/// budget as the last reading left it, gnat's own spend this session (the two
/// side by side tell whether it is gnat burning the token), and how long the
/// app has been open, ticking once a minute. All of it is what the GitHub
/// reading's store already keeps; nothing here asks nat anything.
struct DiagnosticsFoldout: View {
    let store: GitHubReadingStore?
    /// Open from the start — a gallery story's seam; the window always opens
    /// it folded.
    var initiallyExpanded = false
    @State private var expanded: Bool?
    @Environment(\.clock) private var clock

    private var isExpanded: Binding<Bool> {
        Binding(get: { expanded ?? initiallyExpanded }, set: { expanded = $0 })
    }

    var body: some View {
        DisclosureGroup("Diagnostics", isExpanded: isExpanded) {
            TimelineView(.everyMinute) { _ in
                let now = clock()
                VStack(alignment: .leading, spacing: 6) {
                    row("GitHub budget", DiagnosticsFormat.budget(store?.rateLimit, now: now))
                    row("gnat\u{2019}s usage this session", DiagnosticsFormat.usage(
                        points: store?.sessionPoints ?? 0, readings: store?.sessionReadings ?? 0,
                        actions: store?.sessionActions ?? 0))
                    row("Session length", store.map { DiagnosticsFormat.sessionLength(from: $0.launchedAt, to: now) } ?? "\u{2014}")
                }
                .padding(10)
                .frame(maxWidth: .infinity, alignment: .leading)
                .surface(.rowAlt, radius: 6)
                .overlay(RoundedRectangle(cornerRadius: 6)
                    .strokeBorder(DesignTokens.rule(.separator, on: .rowAlt), lineWidth: 0.5))
                .padding(.top, 6)
            }
        }
    }

    private func row(_ label: String, _ value: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            Text(label)
                .ink(.secondary)
            Spacer(minLength: 12)
            Text(value)
                // The app's mono face at the section's body size.
                .font(Typo.mono(size: NSFont.systemFontSize))
                .textSelection(.enabled)
                .lineLimit(1)
        }
    }
}
