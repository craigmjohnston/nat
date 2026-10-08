import AppKit
import SwiftUI
import NatKit

/// A project's own settings (the project menu's Project settings…), as a
/// sheet on the main window: one grouped `Form` of stock controls — no
/// sidebar, no tabs, no app chrome, as the Settings window is — headed by the
/// project's name (a field, or a source project's plugin title as text),
/// then the working directory, Colour (and tag), the agents' models, merging
/// and the base branch, where the plan lives and the run commands. The form scrolls; Cancel and Save stay pinned at the foot. A
/// further per-project row is another `Section` (or row) here over another
/// `ProjectSettingsFields` field.
///
/// Save writes what changed (`ProjectSettingsModel.save`) and closes; a
/// refusal stays in the sheet under the row it came from, nothing changed.
struct ProjectSettingsView: View {
    /// What the project is called wherever gnat names it — for a source
    /// project its plugin's title, which the sheet shows and never edits.
    let projectName: String
    /// The project's short tag, the word on the Colour row's badge.
    let projectTag: String
    @State private var model: ProjectSettingsModel
    /// The model and effort choices, as Settings ▸ Agents offers them.
    @State private var agentOptions = AgentOptions.fallback
    @Environment(\.dismiss) private var dismiss

    init(projectName: String, projectTag: String, model: ProjectSettingsModel) {
        self.projectName = projectName
        self.projectTag = projectTag
        _model = State(initialValue: model)
    }

    /// The sheet over the app's own config and `nat`.
    init(
        appModel: AppModel, projectID: String, projectName: String, projectTag: String,
        client: NatClientProtocol = NatClient()
    ) {
        self.init(
            projectName: projectName, projectTag: projectTag,
            model: ProjectSettingsModel(
                projectID: projectID, config: appModel.config, client: client,
                reload: { await appModel.reloadConfig() }))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Form {
                Section {
                    nameRow
                } footer: {
                    if model.isSource {
                        footnote("Named by its source plugin, not here.")
                    }
                }
                Section {
                    workingDirRow
                } footer: {
                    footnote("Where this project's agents start, unless a task names its own repo. Applies at the next launch.")
                }
                if model.takesColor {
                    Section {
                        colorRow
                    }
                }
                agentsSection
                mergeSection
                Section {
                    planRow
                    if case .local(let file?) = model.plan {
                        planFileRow(file)
                    }
                } footer: {
                    if case .local = model.plan {
                        footnote("nat project-mirror puts a local plan into Notion.")
                    }
                }
                if !model.isSource {
                    runsSection
                }
            }
            .formStyle(.grouped)
            // The sheet's own ground behind the group, heading to buttons,
            // rather than a band of the form's.
            .scrollContentBackground(.hidden)
            Divider()
            HStack {
                Spacer()
                Button("Cancel", role: .cancel) { dismiss() }
                    .keyboardShortcut(.cancelAction)
                Button("Save") { save() }
                    .keyboardShortcut(.defaultAction)
                    .disabled(model.isSaving)
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 14)
        }
        .frame(width: 560, height: 640)
        .task { await model.loadPlanFile() }
        .task { await model.loadDefaultBase() }
        .task { agentOptions = await AgentOptionsCache.shared.resolve() }
    }

    /// The sheet's heading: the project's name as a field, or — a source
    /// project's, which its plugin gives — as text. nat's refusal of it (an
    /// empty name) under it.
    @ViewBuilder
    private var nameRow: some View {
        if model.isSource {
            LabeledContent("Name") {
                Text(projectName).font(.headline).lineLimit(1)
            }
        } else {
            LabeledContent("Name") {
                VStack(alignment: .trailing, spacing: 4) {
                    TextField("Name", text: $model.edited.name)
                        .labelsHidden()
                        .textFieldStyle(.roundedBorder)
                        .font(.system(size: Typo.input))
                        .multilineTextAlignment(.leading)
                        .frame(width: 300)
                        .onSubmit { save() }
                    refusal(model.nameKey)
                }
            }
        }
    }

    /// Where the plan lives, read-only: nothing here moves one. Notion with
    /// its page; a local plan's file, revealed in Finder; or the source
    /// plugin a source project's containers come from.
    @ViewBuilder
    private var planRow: some View {
        LabeledContent("Plan") {
            switch model.plan {
            case .notion(let page):
                HStack(spacing: 8) {
                    Text("Notion")
                    if let page {
                        Button("Open in Notion") { NSWorkspace.shared.open(page) }
                    }
                }
            case .local(let file):
                HStack(spacing: 8) {
                    Text("Local")
                    Button("Reveal in Finder") {
                        if let file { NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: file)]) }
                    }
                    .disabled(file == nil)
                }
            case .source(let plugin):
                Text("Source via \(projectName)").help(plugin)
            }
        }
    }

    /// A local plan's file, as `nat paths --project` gives it: cut at its
    /// head, so the file's own name stays, the whole path its tooltip.
    private func planFileRow(_ file: String) -> some View {
        LabeledContent("Plan file") {
            Text(file)
                .font(Typo.mono(size: Typo.caption))
                .lineLimit(1)
                .truncationMode(.head)
                .textSelection(.enabled)
                .help(file)
        }
    }

    /// The run commands, one row each — label, command in the mono face,
    /// scope — with remove beside each and add under them. A row dragged by
    /// its grip onto another takes that one's place: the first run of each
    /// scope is its default. Saved whole, as one `config-set`, and nat's
    /// refusal of the list under it with the rows as typed.
    private var runsSection: some View {
        Section {
            ForEach(model.edited.runs.indices, id: \.self) { index in
                runRow(index)
            }
            HStack {
                Button("Add Run", systemImage: "plus") { model.addRun() }
                Spacer()
            }
        } header: {
            Text("Run commands")
        } footer: {
            VStack(alignment: .leading, spacing: 4) {
                refusal(model.runsKey)
                footnote("The titlebar's run button offers Global runs, a handed-back task's Slice runs; Both, either. The first of each is its default.")
            }
        }
    }

    private func runRow(_ index: Int) -> some View {
        let run = runBinding(index)
        return HStack(spacing: 8) {
            Image(systemName: "line.3.horizontal")
                .ink(.secondary)
                .help("Drag to reorder")
                .draggable(String(index))
            TextField("Label", text: run.label)
                .labelsHidden()
                .textFieldStyle(.roundedBorder)
                .font(.system(size: Typo.input))
                .frame(width: 96)
            TextField("Command", text: run.command)
                .labelsHidden()
                .textFieldStyle(.roundedBorder)
                .font(Typo.mono(size: Typo.input))
                .help(run.wrappedValue.command)
            Picker("Scope", selection: run.scope) {
                Text("Both").tag(RunScope.both)
                Text("Global").tag(RunScope.global)
                Text("Slice").tag(RunScope.slice)
            }
            .labelsHidden()
            .pickerStyle(.menu)
            .fixedSize()
            Button("Remove", systemImage: "minus.circle") { model.removeRun(at: index) }
                .labelStyle(.iconOnly)
                .buttonStyle(.borderless)
        }
        .dropDestination(for: String.self) { items, _ in
            guard let from = items.first.flatMap(Int.init) else { return false }
            model.moveRun(from, onto: index)
            return true
        }
    }

    /// The run at `index`, read and written only while it is there: a row
    /// can redraw once after its run was removed.
    private func runBinding(_ index: Int) -> Binding<RunCommand> {
        Binding(
            get: { model.edited.runs.indices.contains(index) ? model.edited.runs[index] : RunCommand(label: "", command: "") },
            set: { if model.edited.runs.indices.contains(index) { model.edited.runs[index] = $0 } })
    }

    private func footnote(_ text: String) -> some View {
        Text(text)
            .font(.footnote)
            .ink(.secondary)
            .multilineTextAlignment(.leading)
            .frame(maxWidth: .infinity, alignment: .leading)
    }

    /// nat's refusal of `key` where the last Save had one.
    @ViewBuilder
    private func refusal(_ key: String) -> some View {
        if let error = model.errors[key] {
            Label(error, systemImage: "exclamationmark.triangle.fill")
                .font(.footnote)
                .ink(.danger)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    /// A path is as long as it is, so the field is as wide as the row allows
    /// beside its label and says the rest in its tooltip — and beside
    /// it the button every native path row has, since typing a path out is
    /// not how anyone picks a directory. nat's refusal of it, where the last
    /// Save had one, under it.
    private var workingDirRow: some View {
        LabeledContent("Working directory") {
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 8) {
                    TextField("Working directory", text: $model.edited.workingDir)
                        .labelsHidden()
                        .textFieldStyle(.roundedBorder)
                        .font(.system(size: Typo.input))
                        .multilineTextAlignment(.leading)
                        // Wide for a path, and fixed, so the label keeps its
                        // line beside it: the tooltip says the rest.
                        .frame(width: 220)
                        .help(model.edited.workingDir)
                        .onSubmit { save() }
                    Button("Choose\u{2026}") { chooseDirectory() }
                }
                refusal(model.workingDirKey)
            }
        }
    }

    /// One swatch per project colour, in nat's order — the picked one ringed
    /// in the accent — then the project's badge as the sidebar will draw it. No "auto":
    /// a project always has a colour, and the one ringed first is the one
    /// its entry holds. nat's refusal, where the last Save had one, under it.
    private var colorRow: some View {
        LabeledContent("Colour") {
            VStack(alignment: .trailing, spacing: 4) {
                HStack(spacing: 6) {
                    ForEach(ProjectColor.allCases, id: \.self) { color in
                        ColorSwatch(color: color, selected: model.edited.color == color) {
                            model.edited.color = color
                        }
                    }
                    if let color = model.edited.color {
                        ProjectBadgeView(tag: model.previewTag(shown: projectTag), color: color, name: projectName)
                            .padding(.leading, 6)
                    }
                    TextField("Tag", text: $model.edited.tag, prompt: Text(projectTag))
                        .labelsHidden()
                        .textFieldStyle(.roundedBorder)
                        .font(Typo.mono(size: Typo.input))
                        .frame(width: 52)
                        .help("1 to 3 letters or digits; empty takes the one its name gives.")
                        .onSubmit { save() }
                }
                refusal(model.colorKey)
                refusal(model.tagKey)
            }
        }
    }

    /// The slice and planning agents' model and effort, the pickers Settings
    /// ▸ Agents uses, each led by a Default that names the global value it
    /// falls through to.
    private var agentsSection: some View {
        Section {
            agentRows(title: "Slice agent", model: $model.edited.sliceModel, effort: $model.edited.sliceEffort,
                      global: model.globalSliceAgent, modelKey: model.sliceModelKey, effortKey: model.sliceEffortKey)
            agentRows(title: "Planning agent", model: $model.edited.workshopModel, effort: $model.edited.workshopEffort,
                      global: model.globalWorkshopAgent, modelKey: model.workshopModelKey,
                      effortKey: model.workshopEffortKey)
        } header: {
            Text("Agents")
        } footer: {
            footnote("Over Settings ▸ Agents for this project alone. Applies at the next launch.")
        }
    }

    @ViewBuilder
    private func agentRows(
        title: String, model modelValue: Binding<String>, effort: Binding<String>, global: AgentModel,
        modelKey: String, effortKey: String
    ) -> some View {
        LabeledContent("\(title) model") {
            VStack(alignment: .trailing, spacing: 4) {
                ModelPicker(value: modelValue, options: agentOptions.models,
                            defaultTitle: ProjectSettingsModel.defaultTitle(global.model)) { text in
                    TextField("Model ID", text: text)
                        .labelsHidden()
                        .textFieldStyle(.roundedBorder)
                        .font(.system(size: Typo.input))
                }
                .frame(width: 200, alignment: .trailing)
                refusal(modelKey)
            }
        }
        LabeledContent("\(title) effort") {
            VStack(alignment: .trailing, spacing: 4) {
                Picker("", selection: effort) {
                    Text(ProjectSettingsModel.defaultTitle(global.effort)).tag("")
                    ForEach(withStored(effort.wrappedValue, in: agentOptions.efforts), id: \.self) { option in
                        Text(option).tag(option)
                    }
                }
                .labelsHidden()
                .pickerStyle(.menu)
                .frame(width: 200, alignment: .trailing)
                refusal(effortKey)
            }
        }
    }

    /// The stored value is a choice too where it is none of the known ones:
    /// nat takes any word, and a picker without it would lose it.
    private func withStored(_ stored: String, in options: [String]) -> [String] {
        guard !stored.isEmpty, !options.contains(stored) else { return options }
        return options + [stored]
    }

    /// How the project's pull requests merge, whether the branch goes with
    /// the merge, and the branch they are cut from and merge into.
    private var mergeSection: some View {
        Section {
            LabeledContent("Merge method") {
                VStack(alignment: .trailing, spacing: 4) {
                    Picker("", selection: $model.edited.shownMergeMethod) {
                        ForEach(ProjectMergeMethod.allCases) { method in
                            Text(method.title).tag(method)
                        }
                    }
                    .labelsHidden()
                    .pickerStyle(.menu)
                    .fixedSize()
                    refusal(model.mergeMethodKey)
                }
            }
            LabeledContent("Delete branch") {
                VStack(alignment: .trailing, spacing: 4) {
                    Toggle("Delete the branch after merging", isOn: $model.edited.deleteBranch)
                        .toggleStyle(.checkbox)
                    refusal(model.deleteBranchKey)
                }
            }
            LabeledContent("Base branch") {
                VStack(alignment: .trailing, spacing: 4) {
                    TextField("Base branch", text: $model.edited.baseBranch,
                              prompt: Text(model.baseBranchPlaceholder))
                        .labelsHidden()
                        .textFieldStyle(.roundedBorder)
                        .font(Typo.mono(size: Typo.input))
                        .multilineTextAlignment(.leading)
                        .frame(width: 200)
                        .onSubmit { save() }
                    refusal(model.baseBranchKey)
                }
            }
        } header: {
            Text("Merging")
        } footer: {
            footnote("Tasks are cut from, reviewed against and opened as pull requests into the base branch; empty is the repository's own default.")
        }
    }

    private func save() {
        Task {
            if await model.save() { dismiss() }
        }
    }

    /// The open panel as a settings window opens one: directories only,
    /// started wherever the field already points when that is a directory
    /// that exists, and writing what was chosen into the field — saved with
    /// the rest by Save. A cancelled panel writes nothing at all.
    ///
    /// `NSOpenPanel` rather than `fileImporter`: there is no presentation
    /// state to hold, and the app is not sandboxed.
    private func chooseDirectory() {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        panel.prompt = "Choose"
        if let start = existingDirectory(model.edited.workingDir) {
            panel.directoryURL = start
        }
        guard panel.runModal() == .OK, let url = panel.url else { return }
        model.edited.workingDir = url.path
    }

    private func existingDirectory(_ path: String) -> URL? {
        guard !path.isEmpty else { return nil }
        var isDirectory: ObjCBool = false
        guard FileManager.default.fileExists(atPath: path, isDirectory: &isDirectory),
              isDirectory.boolValue else { return nil }
        return URL(fileURLWithPath: path)
    }
}

/// One colour of the Colour row: a filled circle in the colour's tint on
/// the sheet's ground, ringed in the accent while picked, named in its
/// tooltip and to accessibility.
private struct ColorSwatch: View {
    let color: ProjectColor
    let selected: Bool
    let pick: () -> Void

    var body: some View {
        Button(action: pick) {
            Circle()
                .fill(DesignTokens.projectInk(color, on: .window))
                .frame(width: 14, height: 14)
                .padding(3)
                .overlay {
                    Circle().strokeBorder(DesignTokens.accent, lineWidth: 2).opacity(selected ? 1 : 0)
                }
                .contentShape(Circle())
        }
        .buttonStyle(.plain)
        .help(color.rawValue.capitalized)
        .accessibilityLabel(color.rawValue.capitalized)
        .accessibilityAddTraits(selected ? .isSelected : [])
    }
}
