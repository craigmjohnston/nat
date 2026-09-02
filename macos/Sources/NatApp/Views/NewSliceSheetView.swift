import SwiftUI
import NatKit

/// The sheet the plus toolbar button opens: a small native form for filing
/// one new slice under a milestone of the active plan, Todo and unassigned —
/// the macOS app's answer to `nat slice-add`. It stays thin: the one thing
/// worth deciding here is when "Add Slice" is enabled, which is a title and a
/// milestone both present.
struct NewSliceSheetView: View {
    let projectID: String
    let milestones: [Milestone]
    let onClose: () -> Void
    let onCreated: () -> Void

    @State private var title: String = ""
    @State private var selectedMilestone: String = ""
    @State private var description: String = ""
    @State private var isSubmitting = false
    @State private var error: String?

    private var canSubmit: Bool {
        !isSubmitting
            && !title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            && !selectedMilestone.isEmpty
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("New Slice")
                .font(.system(size: 15, weight: .semibold))
                .foregroundStyle(DesignTokens.label)

            VStack(alignment: .leading, spacing: 6) {
                Text("Title")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundStyle(DesignTokens.labelSecondary)
                TextField("Slice title", text: $title)
                    .textFieldStyle(.roundedBorder)
            }

            VStack(alignment: .leading, spacing: 6) {
                Text("Milestone")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundStyle(DesignTokens.labelSecondary)
                Picker("Milestone", selection: $selectedMilestone) {
                    Text("Select a milestone").tag("")
                    ForEach(milestones) { milestone in
                        Text(milestone.name).tag(milestone.name)
                    }
                }
                .labelsHidden()
            }

            VStack(alignment: .leading, spacing: 6) {
                Text("Description")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundStyle(DesignTokens.labelSecondary)
                Text("Optional. Becomes the slice page's brief.")
                    .font(.system(size: 11, weight: .regular))
                    .foregroundStyle(DesignTokens.labelTertiary)
                TextEditor(text: $description)
                    .font(.system(size: 12, weight: .regular))
                    .scrollContentBackground(.hidden)
                    .padding(6)
                    .background(DesignTokens.fieldBg)
                    .cornerRadius(6)
                    .frame(height: 100)
            }

            if let error {
                Text(error)
                    .font(.system(size: 11, weight: .regular))
                    .foregroundStyle(DesignTokens.systemRed)
            }

            HStack {
                Spacer()

                Button("Cancel") {
                    onClose()
                }
                .keyboardShortcut(.cancelAction)

                Button(action: submit) {
                    if isSubmitting {
                        ProgressView()
                            .scaleEffect(0.6, anchor: .center)
                            .frame(width: 60)
                    } else {
                        Text("Add Slice")
                            .frame(minWidth: 60)
                    }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(!canSubmit)
            }
        }
        .padding(20)
        .frame(width: 420)
    }

    private func submit() {
        let trimmedTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedDescription = description.trimmingCharacters(in: .whitespacesAndNewlines)

        Task {
            isSubmitting = true
            error = nil

            do {
                _ = try await NatClient().sliceAdd(
                    projectID: projectID,
                    title: trimmedTitle,
                    milestone: selectedMilestone,
                    description: trimmedDescription.isEmpty ? nil : trimmedDescription
                )
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
