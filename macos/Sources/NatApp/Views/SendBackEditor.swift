import SwiftUI
import NatKit

/// Send back to agent's own few lines, opened over the action bar by its
/// button: what the agent should change — prefilled with the pull request's
/// trouble where it has any (`sendBackReason`) — then Cancel and Send back.
/// Drawn in the column rather than as a popover, so the gallery can render
/// it, as the visual comment box is. What sending does is
/// `AppModel.sendBack`'s: the note recorded with `nat slice-resume`, then the
/// live agent told, or one launched.
struct SendBackEditor: View {
    @Binding var text: String
    /// Whether an agent is live to be told — else Send back launches one,
    /// and says so.
    let hasLiveAgent: Bool
    let isSending: Bool
    let error: String?
    let onCancel: () -> Void
    let onSend: () -> Void

    @FocusState private var focused: Bool

    private var canSend: Bool {
        !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !isSending
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(hasLiveAgent
                 ? "What should the agent change? It is told at once, and hands the task back when it is done."
                 : "What should the agent change? No agent is running, so one is launched on the work so far.")
                .font(.system(size: Typo.scaled(12.5)))
                .ink(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            TextField("What to change", text: $text, axis: .vertical)
                .font(Typo.mono(size: Typo.input))
                .textFieldStyle(.plain)
                .lineLimit(2...6)
                .focused($focused)
                .padding(.horizontal, 8)
                .padding(.vertical, 6)
                .surface(.field, radius: 5)
                .overlay(RoundedRectangle(cornerRadius: 5).strokeBorder(DesignTokens.rule(.border, on: .field), lineWidth: 1))
                .onSubmit { if canSend { onSend() } }
            if let error {
                Text(error)
                    .font(.system(size: Typo.scaled(12.5)))
                    .ink(.danger)
                    .fixedSize(horizontal: false, vertical: true)
                    .textSelection(.enabled)
            }
            HStack(spacing: 6) {
                Spacer(minLength: 0)
                Button("Cancel", action: onCancel)
                    .buttonStyle(GnatButtonStyle())
                    .keyboardShortcut(.cancelAction)
                Button(action: onSend) {
                    HeaderActionLabel(title: "Send back", systemImage: "arrow.uturn.left", isBusy: isSending)
                }
                .buttonStyle(GnatButtonStyle(primary: true))
                .disabled(!canSend)
            }
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(DesignTokens.fill(.chrome))
        .overlay(alignment: .top) {
            DesignTokens.rule(.separator, on: .chrome).frame(height: 1).offset(y: -1)
        }
        .onAppear { focused = true }
    }
}
