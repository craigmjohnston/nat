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
                .overlay(alignment: .topTrailing) { endSessionControl }
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

    /// The one action the tab has beyond watching: end the session. Closing
    /// the tab only detaches the viewer — the session goes on running, which
    /// is right while there is work in it and is how a finished slice's
    /// session sits on the tmux server forever — so ending one has to be
    /// asked for, and this is where.
    ///
    /// It is a control on the terminal rather than a band under it: the
    /// terminal is the whole of this tab, and a strip of chrome for one
    /// button would take a row off it on every slice. It does not ask before
    /// killing either — the button says what it does, and an agent whose
    /// session ended is relaunched from the Brief tab.
    @ViewBuilder
    private var endSessionControl: some View {
        VStack(alignment: .trailing, spacing: 6) {
            Button(action: performKill) {
                AsyncActionLabel(isBusy: isKilling) {
                    Label("End session", systemImage: "stop.circle")
                        .labelStyle(.titleAndIcon)
                }
                .font(.system(size: Typo.caption, weight: .medium))
                .ink(.secondary)
                .padding(.horizontal, 8)
                .padding(.vertical, 4)
            }
            .buttonStyle(.plain)
            .hoverWash()
            .disabled(isKilling)

            if let killError {
                Text(killError)
                    .font(.system(size: Typo.caption, weight: .regular))
                    .ink(.danger)
                    .lineLimit(2)
                    .multilineTextAlignment(.trailing)
                    .frame(maxWidth: 260, alignment: .trailing)
            }
        }
        .padding(.top, 6)
        .padding(.trailing, 8)
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
