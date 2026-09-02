import SwiftUI
import NatKit

/// The full-pane overlay the wand toolbar button opens: launches the
/// planning agent for the active project on appearance and, once launched,
/// hosts it in the same embedded-terminal machinery a slice's Agent tab uses
/// — the macOS app's answer to the board's `W`, which the Go TUI shows
/// beside the board rather than over it only because it has a board pane of
/// its own to show it in.
///
/// The workshop session is not a slice's, so it gets a header of its own
/// ("Workshop — <project>") in place of the Agent tab's footer, with a Close
/// button that only detaches this view's client — the tmux session, and the
/// planning agent in it, goes on running exactly as `AgentTerminalHostView`'s
/// own dismantle already guarantees for a slice's terminal.
struct WorkshopOverlayView: View {
    let projectID: String
    let projectName: String
    let model: String?
    let effort: String?
    let onClose: () -> Void

    @State private var session: String?
    @State private var isLaunching = true
    @State private var launchError: String?

    var body: some View {
        VStack(spacing: 0) {
            header

            Divider()
                .frame(height: 0.5)

            content
        }
        .background(DesignTokens.windowBg)
        .task {
            await launch()
        }
    }

    private var header: some View {
        HStack(spacing: 10) {
            Text("Workshop — \(projectName)")
                .font(.system(size: 13, weight: .semibold))
                .foregroundStyle(DesignTokens.label)

            if let launchError {
                Text(launchError)
                    .font(.system(size: 11, weight: .regular))
                    .foregroundStyle(DesignTokens.systemRed)
                    .lineLimit(2)
            }

            Spacer()

            Button("Close") {
                onClose()
            }
            .buttonStyle(.borderless)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 8)
        .background(DesignTokens.controlBg)
    }

    @ViewBuilder
    private var content: some View {
        if let session {
            AgentTerminalHostView(
                attachSpec: AttachSpec(session: session),
                sessionExists: { true },
                onExit: { _ in onClose() }
            )
            .id(session)
        } else if isLaunching {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            VStack(spacing: 12) {
                Image(systemName: "exclamationmark.triangle")
                    .font(.system(size: 32, weight: .regular))
                    .foregroundStyle(DesignTokens.labelSecondary)

                Text("Could not launch the workshop agent")
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundStyle(DesignTokens.label)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    private func launch() async {
        do {
            let result = try await NatClient().workshopLaunch(projectID: projectID, model: model, effort: effort)
            session = result.session
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                launchError = message
            } else {
                launchError = error.localizedDescription
            }
        } catch {
            launchError = error.localizedDescription
        }
        isLaunching = false
    }
}

#Preview {
    WorkshopOverlayView(
        projectID: "proj-1",
        projectName: "Example Project",
        model: nil,
        effort: nil,
        onClose: {}
    )
    .frame(width: 900, height: 600)
}
