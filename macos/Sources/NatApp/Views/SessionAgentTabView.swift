import SwiftUI
import NatKit

/// An ad hoc session's Agent tab: the embedded terminal on its own tmux
/// session while live, or an empty state naming its branch once the agent
/// is gone — `AgentTabView`'s own shape, keyed by the session's pane tag
/// (`ActivityStore.agents[session.tag]`) rather than by a slice ID.
struct SessionAgentTabView: View {
    @Bindable var appModel: AppModel
    let session: Session
    @State private var lifecycle = TerminalLifecycle()
    /// A story draws the region rather than attaching to it — see
    /// `StorySeams`.
    @Environment(\.terminalStubbed) private var terminalStubbed

    private var liveAgent: AgentStatus? {
        appModel.activityStore?.agents[session.tag]
    }

    var body: some View {
        VStack(spacing: 0) {
            if let agent = liveAgent {
                ZStack {
                    DesignTokens.fill(.terminal)

                    Group {
                        if terminalStubbed {
                            TerminalStubView(session: agent.session)
                        } else {
                            AgentTerminalHostView(
                                attachSpec: AttachSpec(session: agent.session),
                                sessionExists: { liveAgent != nil },
                                onExit: { _ in
                                    lifecycle.handle(.processTerminated(sessionStillExists: false))
                                }
                            )
                            .id(agent.session)
                        }
                    }
                    .padding(.vertical, 14)
                    .padding(.horizontal, 18)
                }
            } else {
                VStack(spacing: 12) {
                    Image(systemName: "play.circle")
                        .font(.system(size: 40, weight: .regular))
                        .ink(.secondary)

                    Text("No agent is running on this session")
                        .font(.system(size: 13, weight: .semibold))
                        .ink(.primary)

                    Text(session.label)
                        .font(.system(size: 12, weight: .regular))
                        .ink(.secondary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .surface(.card)
            }
        }
        .surface(.window)
    }
}

#Preview {
    let appModel = AppModel()
    let session = Session(
        id: "session-1", tag: "session:proj:session-1", live: false,
        startedAt: Date(), dir: "/Users/craig/Projects/scratch", branch: "session/fixture"
    )
    SessionAgentTabView(appModel: appModel, session: session)
        .frame(height: 400)
}
