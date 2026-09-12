import SwiftUI
import NatKit

/// The pane the rail's WORKSHOP row opens — the macOS app's answer to the
/// board's `w`. No workflow strip: a workshop session has no brief, no diff
/// and no pull request, so under the shared pane header the pane is the agent
/// view alone, the same embedded-terminal machinery a slice's Agent tab uses.
///
/// Presence is the activity poll's answer (`appModel.planningAgent`), so the
/// terminal attaches to the active project's planning agent whichever session
/// launched it — and to no other project's, since the workshop is scoped to a
/// project the way the rest of the pane is. With
/// none running the pane is one of two things: the composer — the board's own
/// "What do you want to workshop?", asked before any session starts, and also
/// where a launch failure is shown, over the request still typed and ready to
/// send again — or, from the moment Launch is pressed until there is an agent
/// to attach to, the launching indicator that says so.
struct WorkshopPaneView: View {
    @Bindable var appModel: AppModel
    @State private var request = ""
    /// A story draws the region rather than attaching to it — see
    /// `StorySeams`.
    @Environment(\.terminalStubbed) private var terminalStubbed

    var body: some View {
        VStack(spacing: 0) {
            // The same header every pane opens with — see `PaneHeader`.
            // No breadcrumb and nothing on the right: a workshop session is
            // filed under no milestone and runs through no pipeline.
            PaneHeader(title: "Workshop")

            content
        }
        .surface(.window)
    }

    @ViewBuilder
    private var content: some View {
        if let agent = appModel.planningAgent {
            // Terminal area — the same full-bleed panel the Agent tab draws.
            ZStack {
                DesignTokens.fill(.terminal)

                Group {
                    if terminalStubbed {
                        TerminalStubView(session: agent.session)
                    } else {
                        AgentTerminalHostView(
                            attachSpec: AttachSpec(session: agent.session),
                            sessionExists: { appModel.planningAgent != nil },
                            onExit: { _ in }
                        )
                        .id(agent.session)
                    }
                }
                .padding(.vertical, 14)
                .padding(.horizontal, 18)
            }
        } else if appModel.workshopLaunching {
            // Launch is pressed and the pane says so on the spot, rather than
            // sitting on the composer until the two-second activity poll
            // notices the session: `workshopLaunching` goes up on the first
            // line of the launch and stays up until the agent is there to
            // attach to — or the launch has failed, which is what puts the
            // composer back, request and all, with the error over it.
            launching
        } else {
            composer
        }
    }

    /// What the pane is between Launch and the terminal: no delay on this
    /// one, unlike `QuietLoadingView`'s deliberate 250ms — the wait is the
    /// answer to a key the user has just pressed, and the whole point of it
    /// is being seen immediately.
    private var launching: some View {
        VStack(spacing: 8) {
            ProgressView()
                .controlSize(.small)

            Text("Starting the workshop session…")
                .font(.system(size: Typo.subhead, weight: .regular))
                .ink(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .surface(.card)
    }

    /// The question the board's `w` form asks, as the pane's own content: the
    /// request typed here goes into the agent's prompt so the session starts
    /// on it, and an empty one launches on the project's pending wishlist
    /// when it has one, else a plain session — the CLI's own rule.
    private var composer: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("What do you want to workshop?")
                .font(.system(size: Typo.body, weight: .semibold))
                .ink(.primary)

            Text("Goes into the agent's prompt; empty starts on the pending wishlist, or a plain session.")
                .font(.system(size: Typo.subhead, weight: .regular))
                .ink(.secondary)

            TextEditor(text: $request)
                .disabled(appModel.workshopLaunching)
                .font(Typo.mono(size: Typo.body))
                .ink(.primary)
                .scrollContentBackground(.hidden)
                .padding(6)
                .frame(minHeight: 180, maxHeight: .infinity)
                .surface(.window, radius: 6)
                .overlay {
                    RoundedRectangle(cornerRadius: 6)
                        .stroke(DesignTokens.rule(.separator, on: .window), lineWidth: 1)
                }

            if let error = appModel.workshopLaunchError {
                Text(error)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.danger)
            }

            HStack {
                Spacer()

                Button(action: { Task { await appModel.launchWorkshop(request: request) } }) {
                    AsyncActionLabel(isBusy: appModel.workshopLaunching) {
                        Text("Launch")
                    }
                }
                .buttonStyle(PrimaryButtonStyle())
                .keyboardShortcut(.return, modifiers: .command)
                .disabled(appModel.workshopLaunching)
            }
        }
        // The editor takes whatever the pane has left: workshopping a plan is
        // paragraphs, and a box the size of a comment field is where the last
        // one made the user type into a keyhole. The column is wider for the
        // same reason, and capped rather than full-bleed so a maximised
        // window does not make lines nobody can read across.
        .frame(maxWidth: 820, maxHeight: .infinity, alignment: .topLeading)
        .padding(.horizontal, 40)
        .padding(.vertical, 28)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .surface(.card)
    }
}

#Preview {
    WorkshopPaneView(appModel: AppModel())
        .frame(width: 900, height: 600)
}
