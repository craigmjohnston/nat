import SwiftUI
import NatKit

/// The pane the rail's WORKSHOP row opens — the macOS app's answer to the
/// board's `w`. No workflow strip: a workshop session has no brief, no diff
/// and no pull request, so under the title row the pane is the agent view
/// alone, the same embedded-terminal machinery a slice's Agent tab uses.
///
/// Presence is the activity poll's answer (`appModel.planningAgent`), so the
/// terminal attaches to a planning agent whichever session launched it. With
/// none running the pane is the composer — the board's own "What do you want
/// to workshop?", asked before any session starts — which is also where a
/// launch failure is shown, over the request still typed and ready to send
/// again.
struct WorkshopPaneView: View {
    @Bindable var appModel: AppModel
    @State private var request = ""

    var body: some View {
        VStack(spacing: 0) {
            // Title row, the same chrome a slice's pane opens with — minus
            // the tab strip there is nothing to fill it with.
            VStack(spacing: 0) {
                HStack(spacing: 14) {
                    Text("Workshop")
                        .font(.system(size: Typo.body, weight: .semibold))
                        .foregroundStyle(DesignTokens.label)
                        .lineLimit(1)

                    Spacer()
                }
                .frame(height: 46)
                .padding(.horizontal, 14)

                Divider()
                    .frame(height: 0.5)
                    .foregroundStyle(DesignTokens.separator)
            }

            content
        }
        .background(DesignTokens.windowBg)
    }

    @ViewBuilder
    private var content: some View {
        if let agent = appModel.planningAgent {
            // Terminal area — the same full-bleed panel the Agent tab draws.
            ZStack {
                AgentTerminalHostView.backgroundColor

                AgentTerminalHostView(
                    attachSpec: AttachSpec(session: agent.session),
                    sessionExists: { appModel.planningAgent != nil },
                    onExit: { _ in }
                )
                .id(agent.session)
                .padding(.vertical, 14)
                .padding(.horizontal, 18)
            }
        } else {
            // A launch keeps the composer exactly where it is rather than
            // swapping it for a spinner: what the user typed is still what
            // the session is starting on, and a pane that blanked and then
            // came back as a terminal would have thrown the request off
            // screen for the second it takes. The Launch button's own busy
            // mark is what says it is under way — see `AsyncActionLabel`.
            composer
        }
    }

    /// The question the board's `w` form asks, as the pane's own content: the
    /// request typed here goes into the agent's prompt so the session starts
    /// on it, and an empty one launches on the project's pending wishlist
    /// when it has one, else a plain session — the CLI's own rule.
    private var composer: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("What do you want to workshop?")
                .font(.system(size: Typo.body, weight: .semibold))
                .foregroundStyle(DesignTokens.label)

            Text("Goes into the agent's prompt; empty starts on the pending wishlist, or a plain session.")
                .font(.system(size: Typo.subhead, weight: .regular))
                .foregroundStyle(DesignTokens.labelSecondary)

            TextEditor(text: $request)
                .disabled(appModel.workshopLaunching)
                .font(.system(size: Typo.body, weight: .regular))
                .foregroundStyle(DesignTokens.label)
                .scrollContentBackground(.hidden)
                .padding(6)
                .frame(height: 140)
                .background(DesignTokens.windowBg)
                .clipShape(RoundedRectangle(cornerRadius: 6))
                .overlay(
                    RoundedRectangle(cornerRadius: 6)
                        .stroke(DesignTokens.separator, lineWidth: 1)
                )

            if let error = appModel.workshopLaunchError {
                Text(error)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(DesignTokens.systemRed)
            }

            HStack {
                Spacer()

                Button(action: { Task { await appModel.launchWorkshop(request: request) } }) {
                    AsyncActionLabel(isBusy: appModel.workshopLaunching) {
                        Text("Launch")
                    }
                }
                .keyboardShortcut(.return, modifiers: .command)
                .disabled(appModel.workshopLaunching)
            }
        }
        .frame(maxWidth: 560)
        .padding(.horizontal, 40)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(DesignTokens.controlBg)
    }
}

#Preview {
    WorkshopPaneView(appModel: AppModel())
        .frame(width: 900, height: 600)
}
