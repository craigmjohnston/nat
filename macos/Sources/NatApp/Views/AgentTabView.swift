import SwiftUI
import NatKit

/// The Agent tab view: shows an embedded terminal attached to a live agent,
/// or an empty state if no agent is running.
struct AgentTabView: View {
    @Bindable var appModel: AppModel
    let slice: Slice
    @State private var lifecycle = TerminalLifecycle()
    /// The kill in flight, and what nat said if it refused — the button's own
    /// state, since ending a session is something the user asked for here and
    /// not news the whole app needs.
    @State private var isKilling = false
    @State private var killError: String?
    @State private var confirmingKill = false
    /// A story draws the region rather than attaching to it — see
    /// `StorySeams`.
    @Environment(\.terminalStubbed) private var terminalStubbed

    private var liveAgent: AgentStatus? {
        guard let sliceID = appModel.selectedSliceID else { return nil }
        return appModel.activityStore?.agents[sliceID]
    }

    var body: some View {
        VStack(spacing: 0) {
            if let agent = liveAgent {
                // Terminal area: the dark surface reaches the pane's edges,
                // and the terminal itself is inset from it — a margin drawn
                // around a smaller rectangle would leave a lighter band at
                // the edges instead of the mock's own full-bleed panel.
                ZStack {
                    DesignTokens.fill(.terminal)

                    Group {
                        if terminalStubbed {
                            TerminalStubView(session: agent.session)
                        } else {
                            AgentTerminalHostView(
                                attachSpec: AttachSpec(session: agent.session),
                                sessionExists: { sessionStillExists() },
                                onExit: { reason in
                                    lifecycle.handle(.processTerminated(sessionStillExists: false))
                                }
                            )
                            .id(agent.session) // Force recreation when session changes
                        }
                    }
                    .padding(.vertical, 14)
                    .padding(.horizontal, 18)
                }

                sessionBar(agent)
            } else {
                // Empty state
                VStack(spacing: 12) {
                    Image(systemName: "play.circle")
                        .font(.system(size: 40, weight: .regular))
                        .ink(.secondary)

                    Text("No agent is running on this slice")
                        .font(.system(size: 13, weight: .semibold))
                        .ink(.primary)

                    Text("Launch one from the Brief tab")
                        .font(.system(size: 12, weight: .regular))
                        .ink(.secondary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .surface(.card)
            }
        }
        .surface(.window)
    }

    /// The band under the terminal: which session this is, and the one action
    /// that ends it. Closing the tab only detaches the viewer — the session
    /// goes on running, which is right while there is work in it and is how a
    /// finished slice's session sits on the tmux server forever — so ending
    /// one is a thing that has to be asked for, and this is where.
    ///
    /// It asks first, because a session with a turn in flight loses that turn.
    @ViewBuilder
    private func sessionBar(_ agent: AgentStatus) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 8) {
                Text(agent.session)
                    .font(Typo.mono(size: Typo.caption))
                    .ink(.tertiary)
                    .lineLimit(1)

                Spacer()

                Button(action: { confirmingKill = true }) {
                    AsyncActionLabel(isBusy: isKilling) {
                        Text("End session")
                    }
                }
                .buttonStyle(SecondaryButtonStyle())
                .disabled(isKilling)
                .confirmationDialog(
                    "End the agent session for this slice?",
                    isPresented: $confirmingKill
                ) {
                    Button("End session", role: .destructive) { performKill() }
                    Button("Cancel", role: .cancel) {}
                } message: {
                    Text("The tmux session is killed. Anything the agent is part way through is lost.")
                }
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 8)

            if let killError {
                HStack(spacing: 8) {
                    Image(systemName: "exclamationmark.circle.fill")
                        .ink(.danger)
                        .font(.system(size: 12, weight: .medium))
                    Text(killError)
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .ink(.danger)
                        .lineLimit(2)
                    Spacer()
                }
                .padding(.horizontal, 14)
                .padding(.bottom, 8)
            }
        }
        .frame(maxWidth: .infinity)
        .overlay(alignment: .top) {
            Rule(.hairline)
        }
        .surface(.band)
    }

    private func performKill() {
        Task {
            isKilling = true
            killError = nil
            killError = await appModel.killAgent(sliceID: slice.id)
            isKilling = false
        }
    }

    private func sessionStillExists() -> Bool {
        guard liveAgent != nil else { return false }
        // In a real implementation, we'd check with tmux; for now assume if it's
        // in the ActivityStore map, it still exists (the store will remove it within 2s)
        return true
    }
}

#Preview {
    let appModel = AppModel()
    let slice = Slice(
        id: "test-id",
        name: "Test Slice",
        status: "In progress",
        milestoneID: "m1",
        assignee: "Craig",
        pr: "",
        url: "https://example.com",
        branch: "feature/test",
        repo: "/path/to/repo",
        dependsOn: nil,
        blocked: false,
        handedBack: false
    )

    AgentTabView(appModel: appModel, slice: slice)
        .frame(height: 400)
}
