import AppKit
import SwiftUI
import NatKit

/// The sheet the `+` menu's "New <source> project…" opens: a project whose
/// plan is a local one and whose milestones are the plugin's containers —
/// `nat project-create --source <name>`. A name and where its agents work;
/// the plugin keeps its own settings, so there is nothing else to ask.
/// Success ends where every other new project does, in `AppModel.addProject`.
struct NewSourceProjectSheetView: View {
    let plugin: SourcePlugin
    let onClose: () -> Void
    /// The new project's id and name.
    let onAdded: (String, String) -> Void

    @State private var name: String
    @State private var directory: String
    @State private var isSubmitting = false
    @State private var error: String?

    init(
        plugin: SourcePlugin, initialName: String = "", initialDirectory: String = "",
        onClose: @escaping () -> Void, onAdded: @escaping (String, String) -> Void
    ) {
        self.plugin = plugin
        self.onClose = onClose
        self.onAdded = onAdded
        _name = State(initialValue: initialName)
        _directory = State(initialValue: initialDirectory)
    }

    private var canSubmit: Bool {
        !isSubmitting && NewProjectModel.canCreate(name: name, directory: directory)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack(spacing: 8) {
                SourceIconView(icon: SourceIcon(symbol: plugin.iconSymbol, svg: plugin.describe?.iconSVG), size: 16)
                    .ink(.secondary)
                Text("New \(plugin.displayTitle) project")
                    .font(.system(size: Typo.headline, weight: .semibold))
                    .ink(.primary)
            }

            Text("Its \(plugin.describe?.containerNoun ?? "container")s come from \(plugin.displayTitle); "
                + "the \(plugin.describe?.taskNoun ?? "task")s under them are nat's own, in a plan on this Mac.")
                .font(.system(size: Typo.subhead))
                .ink(.tertiary)
                .fixedSize(horizontal: false, vertical: true)

            VStack(alignment: .leading, spacing: 6) {
                Text("Name")
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .ink(.secondary)
                TextField("Project name", text: $name)
                    .textFieldStyle(.roundedBorder)
                    .font(Typo.mono(size: Typo.code))
            }

            VStack(alignment: .leading, spacing: 6) {
                Text("Working directory")
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .ink(.secondary)
                HStack(spacing: 8) {
                    TextField("Where this project's agents work", text: $directory)
                        .textFieldStyle(.roundedBorder)
                        .font(Typo.mono(size: Typo.code))
                    Button("Choose\u{2026}", action: chooseDirectory)
                }
            }

            if let error {
                Text(error)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.danger)
            }

            HStack {
                Spacer()
                Button("Cancel", action: onClose)
                    .buttonStyle(SecondaryButtonStyle())
                    .keyboardShortcut(.cancelAction)
                Button(action: submit) {
                    AsyncActionLabel(isBusy: isSubmitting) { Text("Create") }
                }
                .buttonStyle(PrimaryButtonStyle())
                .keyboardShortcut(.defaultAction)
                .disabled(!canSubmit)
            }
        }
        .padding(20)
        .frame(width: 460)
    }

    /// The directory chooser, as the New Project sheet's.
    private func chooseDirectory() {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        panel.prompt = "Choose"
        guard panel.runModal() == .OK, let url = panel.url else { return }
        directory = url.path
    }

    private func submit() {
        Task {
            isSubmitting = true
            error = nil
            do {
                let project = try await NatClient().projectCreate(
                    name: name.trimmingCharacters(in: .whitespacesAndNewlines),
                    repo: directory.trimmingCharacters(in: .whitespacesAndNewlines),
                    description: nil, source: plugin.name)
                onAdded(project.id, project.name)
            } catch {
                // Nothing was recorded: the sheet stays up with what nat said.
                self.error = NewProjectModel.message(from: error)
            }
            isSubmitting = false
        }
    }
}
