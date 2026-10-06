import AppKit
import SwiftUI
import NatKit

/// A project's own settings (the project menu's Project settings…), as a
/// sheet on the main window: titled with the project's name, then one
/// grouped `Form` of stock controls — no sidebar, no tabs, no app chrome, as
/// the Settings window is. A further per-project row is another `Section`
/// (or row) here over another `ProjectSettingsFields` field.
///
/// Save writes what changed (`ProjectSettingsModel.save`) and closes; a
/// refusal stays in the sheet under the row it came from, nothing changed.
struct ProjectSettingsView: View {
    let projectName: String
    @State private var model: ProjectSettingsModel
    @Environment(\.dismiss) private var dismiss

    init(projectName: String, model: ProjectSettingsModel) {
        self.projectName = projectName
        _model = State(initialValue: model)
    }

    /// The sheet over the app's own config and `nat`.
    init(appModel: AppModel, projectID: String, projectName: String, client: NatClientProtocol = NatClient()) {
        self.init(
            projectName: projectName,
            model: ProjectSettingsModel(
                projectID: projectID, config: appModel.config, client: client,
                reload: { await appModel.reloadConfig() }))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(projectName)
                .font(.headline)
                .lineLimit(1)
                .padding(.horizontal, 20)
                .padding(.top, 18)
            Form {
                Section {
                    workingDirRow
                } footer: {
                    Text("Where this project's agents start, unless a task names its own repo. Applies at the next launch.")
                        .font(.footnote)
                        .ink(.secondary)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                if model.takesColor {
                    Section {
                        colorRow
                    }
                }
            }
            .formStyle(.grouped)
            // The sheet's own ground behind the group, heading to buttons,
            // rather than a band of the form's.
            .scrollContentBackground(.hidden)
            .scrollDisabled(true)
            .fixedSize(horizontal: false, vertical: true)
            HStack {
                Spacer()
                Button("Cancel", role: .cancel) { dismiss() }
                    .keyboardShortcut(.cancelAction)
                Button("Save") { save() }
                    .keyboardShortcut(.defaultAction)
                    .disabled(model.isSaving)
            }
            .padding(.horizontal, 20)
            .padding(.bottom, 18)
        }
        .frame(width: 520)
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
                if let error = model.errors[model.workingDirKey] {
                    Label(error, systemImage: "exclamationmark.triangle.fill")
                        .font(.footnote)
                        .ink(.danger)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
        }
    }

    /// One swatch per project colour, in nat's order — the picked one ringed
    /// in the accent — then the puck as the sidebar will draw it. No "auto":
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
                        ProjectPuck(color: color)
                            .padding(.leading, 6)
                    }
                }
                if let error = model.errors[model.colorKey] {
                    Label(error, systemImage: "exclamationmark.triangle.fill")
                        .font(.footnote)
                        .ink(.danger)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
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
