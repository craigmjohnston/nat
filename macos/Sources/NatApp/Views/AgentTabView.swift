import SwiftUI
import NatKit

/// The Agent tab view: shows an embedded terminal attached to a live agent,
/// or an empty state if no agent is running.
struct AgentTabView: View {
    @Bindable var appModel: AppModel
    let slice: Slice
    /// True while something is drawn over the whole board (the workshop
    /// overlay) — see `PaneView.isCovered` for why this tears the terminal
    /// down rather than merely leaving it painted over.
    var isCovered: Bool = false
    @State private var interruptError: String?
    @State private var lifecycle = TerminalLifecycle()

    private var liveAgent: AgentStatus? {
        guard let sliceID = appModel.selectedSliceID else { return nil }
        return appModel.activityStore?.agents[sliceID]
    }

    var body: some View {
        VStack(spacing: 0) {
            if let agent = liveAgent, !isCovered {
                // Terminal area: the dark surface reaches the pane's edges,
                // and the terminal itself is inset from it — a margin drawn
                // around a smaller rectangle would leave a lighter band at
                // the edges instead of the mock's own full-bleed panel.
                ZStack {
                    AgentTerminalHostView.backgroundColor

                    AgentTerminalHostView(
                        attachSpec: AttachSpec(session: agent.session),
                        sessionExists: { sessionStillExists() },
                        onExit: { reason in
                            lifecycle.handle(.processTerminated(sessionStillExists: false))
                        }
                    )
                    .id(agent.session) // Force recreation when session changes
                    .padding(.vertical, 14)
                    .padding(.horizontal, 18)
                }

                // Footer with buttons — the mock's own metrics: a ~34pt row,
                // 8pt of vertical padding and a hairline top border rather
                // than a bare Divider, matching every other pane border in
                // the app (see ProgressBorderView).
                HStack(spacing: 8) {
                    Spacer()

                    if let error = interruptError {
                        Text(error)
                            .font(.system(size: Typo.subhead, weight: .regular))
                            .foregroundStyle(DesignTokens.systemRed)
                    }

                    Button(action: openInTerminal) {
                        Text("Open in Terminal…")
                            .font(.system(size: Typo.subhead, weight: .regular))
                    }
                    .buttonStyle(.borderless)

                    Button(action: sendInterrupt) {
                        Text("Interrupt")
                            .font(.system(size: Typo.subhead, weight: .regular))
                    }
                    .buttonStyle(.borderless)
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 8)
                .frame(height: 34)
                .background(DesignTokens.controlBg)
                .rectBorder(width: 0.5, edges: [.top], color: DesignTokens.separator)
            } else {
                // Empty state
                VStack(spacing: 12) {
                    Image(systemName: "play.circle")
                        .font(.system(size: 40, weight: .regular))
                        .foregroundStyle(DesignTokens.labelSecondary)

                    Text("No agent is running on this slice")
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundStyle(DesignTokens.label)

                    Text("Launch one from the Brief tab")
                        .font(.system(size: 12, weight: .regular))
                        .foregroundStyle(DesignTokens.labelSecondary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .background(DesignTokens.controlBg)
            }
        }
        .background(DesignTokens.windowBg)
    }

    // MARK: - Actions

    private func openInTerminal() {
        guard let agent = liveAgent else { return }

        let spec = AttachSpec(session: agent.session)
        let scriptText = AttachCommandScript.generate(from: spec)

        // Write script to temp file
        let tempDir = FileManager.default.temporaryDirectory
        let scriptPath = tempDir.appendingPathComponent("nat-attach-\(agent.session).command")

        do {
            try scriptText.write(toFile: scriptPath.path, atomically: true, encoding: .utf8)

            // Make it executable
            try FileManager.default.setAttributes(
                [.posixPermissions: 0o755],
                ofItemAtPath: scriptPath.path
            )

            // Open with NSWorkspace (Terminal.app will run it)
            NSWorkspace.shared.open(scriptPath)
        } catch {
            interruptError = "Failed to open terminal: \(error.localizedDescription)"
        }
    }

    private func sendInterrupt() {
        guard let projectID = appModel.projectStore?.projectID else { return }

        interruptError = nil

        Task {
            do {
                try await NatClient().agentInterrupt(projectID: projectID, sliceRef: slice.id)
            } catch {
                await MainActor.run {
                    interruptError = error.localizedDescription
                }
            }
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
