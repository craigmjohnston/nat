import AppKit
import SwiftUI
import NatKit

/// The sheet the "+" tab opens: the two ways a project comes to be on the
/// board, side by side under one switch — open one the workspace already has,
/// or create a new one. Both end the same way, in `AppModel.addProject`: what
/// each produced is one more entry in local config.
///
/// The two paths are kept apart on purpose. Opening is a pick from a list and
/// nothing to type; creating is a form and reaches Notion. A listing that
/// fails takes only its own half down — the sheet opens on Create instead and
/// says why the picker is empty — because a workspace whose projects database
/// cannot be read is exactly a machine that still needs to make a project.
struct NewProjectSheetView: View {
    let onClose: () -> Void

    /// What a successful open or create hands back: the page ID `--project`
    /// takes and the name to label its tab with.
    let onAdded: (String, String) -> Void

    enum Mode: String, CaseIterable {
        case open = "Open Existing"
        case create = "Create New"
    }

    @State private var mode: Mode = .open

    // The open path.
    @State private var listing = ProjectListing(projects: [])
    @State private var listingError: String?
    @State private var isListing = true
    @State private var selectedProjectID = ""

    // The create path.
    @State private var name = ""
    @State private var directory = ""
    @State private var descriptionText = ""

    // Both.
    @State private var isSubmitting = false
    @State private var error: String?

    private var openable: [ProjectListingEntry] { NewProjectModel.openable(listing) }

    private var canSubmit: Bool {
        guard !isSubmitting else { return false }
        switch mode {
        case .open:
            return NewProjectModel.canOpen(selection: selectedProjectID, in: listing)
        case .create:
            return NewProjectModel.canCreate(name: name, directory: directory)
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Add a Project")
                .font(.system(size: Typo.headline, weight: .semibold))
                .foregroundStyle(DesignTokens.label)

            Picker("", selection: $mode) {
                ForEach(Mode.allCases, id: \.self) { mode in
                    Text(mode.rawValue).tag(mode)
                }
            }
            .pickerStyle(.segmented)
            .labelsHidden()

            switch mode {
            case .open:
                openForm
            case .create:
                createForm
            }

            if let error {
                Text(error)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(DesignTokens.systemRed)
            }

            HStack {
                Spacer()

                Button("Cancel") {
                    onClose()
                }
                .buttonStyle(SecondaryButtonStyle())
                .keyboardShortcut(.cancelAction)

                Button(action: submit) {
                    AsyncActionLabel(isBusy: isSubmitting) {
                        Text(mode == .open ? "Open" : "Create")
                            .frame(minWidth: 60)
                    }
                }
                .buttonStyle(PrimaryButtonStyle())
                .keyboardShortcut(.defaultAction)
                .disabled(!canSubmit)
            }
        }
        .padding(20)
        .frame(width: 460)
        .task { await loadListing() }
    }

    // MARK: - Open

    @ViewBuilder
    private var openForm: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Workspace Project")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .foregroundStyle(DesignTokens.labelSecondary)

            if isListing {
                QuietLoadingView(label: "Reading the workspace…")
                    .frame(height: 60)
            } else if openable.isEmpty {
                Text(listingError
                    ?? listing.note
                    ?? "Every project in the workspace is already on this Mac. Create a new one instead.")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(listingError == nil ? DesignTokens.labelTertiary : DesignTokens.systemYellow)
                    .fixedSize(horizontal: false, vertical: true)
            } else {
                Picker("Project", selection: $selectedProjectID) {
                    Text("Select a project").tag("")
                    ForEach(openable) { entry in
                        Text(entry.name).tag(entry.id)
                    }
                }
                .labelsHidden()

                Text("Its plan is read straight away. Where its code lives is this Mac's own answer — set a working directory in Settings before launching an agent on it.")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(DesignTokens.labelTertiary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    // MARK: - Create

    @ViewBuilder
    private var createForm: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Name")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .foregroundStyle(DesignTokens.labelSecondary)
            TextField("Project name", text: $name)
                .textFieldStyle(.roundedBorder)
        }

        VStack(alignment: .leading, spacing: 6) {
            Text("Repository")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .foregroundStyle(DesignTokens.labelSecondary)
            HStack(spacing: 8) {
                TextField("Where this project's agents work", text: $directory)
                    .textFieldStyle(.roundedBorder)
                Button("Choose…", action: chooseDirectory)
            }
        }

        VStack(alignment: .leading, spacing: 6) {
            Text("Conventions")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .foregroundStyle(DesignTokens.labelSecondary)
            Text("Optional. Becomes the project page's body — what every agent reads first.")
                .font(.system(size: Typo.subhead, weight: .regular))
                .foregroundStyle(DesignTokens.labelTertiary)
            TextEditor(text: $descriptionText)
                .font(.system(size: Typo.subhead, weight: .regular))
                .scrollContentBackground(.hidden)
                .padding(6)
                .background(DesignTokens.fieldBg)
                .cornerRadius(6)
                .frame(height: 100)
        }
    }

    /// The directory chooser: a windowed app has no working directory worth
    /// defaulting from, so the one `project-create` would fall back to is
    /// never the right answer and the panel is how the real one arrives.
    private func chooseDirectory() {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        panel.prompt = "Choose"
        guard panel.runModal() == .OK, let url = panel.url else { return }
        directory = url.path
    }

    // MARK: - Actions

    private func loadListing() async {
        isListing = true
        do {
            listing = try await NatClient().projectList()
        } catch {
            // Only the open half is lost: the sheet opens on Create, which is
            // what a machine with no readable workspace listing needs anyway.
            listingError = NewProjectModel.message(from: error)
            mode = .create
        }
        isListing = false
    }

    private func submit() {
        Task {
            isSubmitting = true
            error = nil
            do {
                switch mode {
                case .open:
                    let entry = try await NatClient().projectOpen(pageRef: selectedProjectID)
                    onAdded(entry.id, entry.name)
                case .create:
                    let project = try await NatClient().projectCreate(
                        name: name.trimmingCharacters(in: .whitespacesAndNewlines),
                        repo: directory.trimmingCharacters(in: .whitespacesAndNewlines),
                        description: descriptionText.trimmingCharacters(in: .whitespacesAndNewlines)
                    )
                    onAdded(project.id, project.name)
                }
            } catch {
                // Nothing was recorded — both commands write config only once
                // they have something whole to write — so the sheet stays up
                // with what nat said and the fields as they were.
                self.error = NewProjectModel.message(from: error)
            }
            isSubmitting = false
        }
    }
}

#Preview {
    NewProjectSheetView(onClose: {}, onAdded: { _, _ in })
}
