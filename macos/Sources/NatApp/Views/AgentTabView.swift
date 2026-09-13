import SwiftUI
import NatKit

/// The Agent tab view: shows an embedded terminal attached to a live agent,
/// or an empty state if no agent is running.
struct AgentTabView: View {
    @Bindable var appModel: AppModel
    let slice: Slice
    @State private var lifecycle = TerminalLifecycle()
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
