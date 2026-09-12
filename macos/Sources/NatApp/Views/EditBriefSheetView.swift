import SwiftUI
import NatKit

/// The sheet the rail's "Edit Description…" opens: one Todo slice's brief,
/// read back off its page and written whole — the same `nat slice-edit` the
/// Brief tab's own "Edit…" runs, reached from the tree rather than from the
/// pane.
///
/// A sheet rather than the Brief tab's inline editor because the rail's menu
/// is opened on a row the user need not have selected: sending them to
/// another pane to type is a different action from the one they picked. What
/// it writes, and what the CLI refuses, are the Brief tab's exactly.
struct EditBriefSheetView: View {
    let projectID: String
    let sliceID: String
    let sliceName: String
    let onClose: () -> Void
    let onSaved: () -> Void

    /// The brief as typed. Empty until the read lands, which is why `isLoading`
    /// is its own state: an empty editor and an unread one look alike, and
    /// saving the first over the second would blank a page nobody had read.
    @State private var brief: String = ""
    @State private var isLoading = true
    @State private var isSaving = false
    @State private var error: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Edit Description")
                .font(.system(size: Typo.headline, weight: .semibold))
                .ink(.primary)

            Text(sliceName)
                .font(.system(size: Typo.subhead, weight: .regular))
                .ink(.tertiary)
                .lineLimit(1)

            TextEditor(text: $brief)
                .font(Typo.mono(size: Typo.subhead))
                .scrollContentBackground(.hidden)
                .padding(6)
                .surface(.field)
                .cornerRadius(6)
                .frame(height: 220)
                .disabled(isLoading)
                .opacity(isLoading ? 0.5 : 1)

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

                Button(action: save) {
                    AsyncActionLabel(isBusy: isSaving) {
                        Text("Save")
                    }
                }
                .buttonStyle(PrimaryButtonStyle())
                .keyboardShortcut(.defaultAction)
                .disabled(isLoading || isSaving)
            }
        }
        .padding(20)
        .frame(width: 520)
        .task { await load() }
    }

    private func load() async {
        do {
            let detail = try await NatClient().sliceShow(projectID: projectID, sliceRef: sliceID)
            brief = detail.brief
        } catch {
            self.error = commandMessage(of: error)
        }
        isLoading = false
    }

    private func save() {
        Task {
            isSaving = true
            error = nil
            do {
                _ = try await NatClient().sliceEdit(
                    projectID: projectID, sliceRef: sliceID, description: brief)
                onSaved()
            } catch {
                self.error = commandMessage(of: error)
            }
            isSaving = false
        }
    }

    /// nat's own first stderr line where there is one, and the generic
    /// description otherwise — the same unwrapping every other caller does.
    private func commandMessage(of error: Error) -> String {
        if case NatError.commandFailed(let message) = error {
            return message
        }
        return error.localizedDescription
    }
}

#Preview {
    EditBriefSheetView(
        projectID: "proj-1",
        sliceID: "slice-1",
        sliceName: "Write the UI",
        onClose: {},
        onSaved: {}
    )
}
