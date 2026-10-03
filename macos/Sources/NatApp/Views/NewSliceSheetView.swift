import SwiftUI
import NatKit

/// The sheet the plus toolbar button opens: a small native form for filing
/// one new slice under a milestone of the active plan, Todo and unassigned —
/// the macOS app's answer to `nat slice-add`. It stays thin: the one thing
/// worth deciding here is when "Add slice" is enabled, which is a title and a
/// milestone both present.
struct NewSliceSheetView: View {
    let projectID: String
    let milestones: [Milestone]
    /// The milestone the sheet opens on, for the rail's "New slice…" — a
    /// menu opened on a folder has already said which milestone it means, so
    /// asking again would be the sheet forgetting where it was opened. Empty
    /// is the toolbar button's own answer: nothing said, so nothing picked.
    var initialMilestone: String = ""
    /// The scratch project's sheet: a slice there may go under no milestone,
    /// which nat files under the project's unfiled one.
    var milestoneOptional = false
    /// A source project's container the task goes under — fixed, in place of
    /// the milestone picker, and filed with `slice-add --container`.
    var container: Container?
    let onClose: () -> Void
    let onCreated: () -> Void

    /// The container a source project's `+` opened the sheet on.
    struct Container {
        let id: String
        let title: String
        /// What the source calls one — "card".
        let noun: String
    }

    @State private var title: String = ""
    @State private var selectedMilestone: String = ""
    @State private var description: String = ""
    @State private var isSubmitting = false
    @State private var error: String?

    private var canSubmit: Bool {
        !isSubmitting
            && !title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            && (milestoneOptional || container != nil || !selectedMilestone.isEmpty)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("New task")
                .font(.system(size: Typo.headline, weight: .semibold))
                .ink(.primary)

            VStack(alignment: .leading, spacing: 6) {
                Text("Title")
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .ink(.secondary)
                TextField("Task title", text: $title)
                    .textFieldStyle(.roundedBorder)
                    .font(Typo.mono(size: Typo.input))
            }

            if let container {
                VStack(alignment: .leading, spacing: 6) {
                    Text(container.noun.prefix(1).uppercased() + container.noun.dropFirst())
                        .font(.system(size: Typo.subhead, weight: .semibold))
                        .ink(.secondary)
                    HStack(spacing: 7) {
                        Image(systemName: SourceGlyph.container)
                            .font(.system(size: 11))
                            .ink(.tertiary)
                        Text(container.title).ink(.primary).lineLimit(1)
                    }
                    .font(.system(size: Typo.code))
                }
            } else {
                VStack(alignment: .leading, spacing: 6) {
                    Text("Milestone")
                        .font(.system(size: Typo.subhead, weight: .semibold))
                        .ink(.secondary)
                    Picker("Milestone", selection: $selectedMilestone) {
                        Text(milestoneOptional ? "No milestone" : "Select a milestone").tag("")
                        ForEach(milestones.filter { !$0.unfiled }) { milestone in
                            Text(milestone.name).tag(milestone.name)
                        }
                    }
                    .labelsHidden()
                }
            }

            VStack(alignment: .leading, spacing: 6) {
                Text("Description")
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .ink(.secondary)
                Text("Optional. Becomes the task page's brief.")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.tertiary)
                TextEditor(text: $description)
                    .font(Typo.mono(size: Typo.input))
                    .scrollContentBackground(.hidden)
                    .padding(6)
                    .surface(.field)
                    .cornerRadius(6)
                    .frame(height: 100)
            }

            if let error {
                Text(error)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.danger)
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
                        Text("Add task")
                    }
                }
                .buttonStyle(PrimaryButtonStyle())
                .keyboardShortcut(.defaultAction)
                .disabled(!canSubmit)
            }
        }
        .padding(20)
        .frame(width: 420)
        // Seeded here rather than in the field's own initial value: the
        // picker's selection is `@State`, which takes its value once, and
        // the sheet is built before the milestone it was opened on is known
        // to it.
        .onAppear {
            if selectedMilestone.isEmpty, milestones.contains(where: { $0.name == initialMilestone }) {
                selectedMilestone = initialMilestone
            }
        }
    }

    private func submit() {
        let trimmedTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedDescription = description.trimmingCharacters(in: .whitespacesAndNewlines)

        Task {
            isSubmitting = true
            error = nil

            do {
                let description = trimmedDescription.isEmpty ? nil : trimmedDescription
                if let container {
                    _ = try await NatClient().sliceAdd(
                        projectID: projectID, title: trimmedTitle, container: container.id, description: description)
                } else {
                    _ = try await NatClient().sliceAdd(
                        projectID: projectID,
                        title: trimmedTitle,
                        milestone: selectedMilestone,
                        description: description
                    )
                }
                onCreated()
            } catch let natError as NatError {
                if case .commandFailed(let message) = natError {
                    error = message
                } else {
                    error = natError.localizedDescription
                }
            } catch {
                self.error = error.localizedDescription
            }

            isSubmitting = false
        }
    }
}

#Preview {
    NewSliceSheetView(
        projectID: "proj-1",
        milestones: [
            Milestone(id: "m1", name: "Phase 1", order: 0, status: "Active"),
            Milestone(id: "m2", name: "Phase 2", order: 1, status: "Queued")
        ],
        onClose: {},
        onCreated: {}
    )
}
