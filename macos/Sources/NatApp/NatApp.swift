import SwiftUI
import NatKit

@main
struct NatApp: App {
    var body: some Scene {
        WindowGroup("nat") {
            // NAT_TERM_SESSION is a debug affordance only: it lets a session
            // name be smoke-tested against a real tmux session before the
            // Agent tab has anywhere of its own to launch one from. Anyone
            // launching NatApp normally sees the placeholder ContentView.
            if let session = ProcessInfo.processInfo.environment["NAT_TERM_SESSION"] {
                AgentTerminalDebugView(session: session)
            } else {
                ContentView()
            }
        }
    }
}

/// The debug host for NAT_TERM_SESSION: attaches the terminal view straight
/// to the named session, with a real (if minimal) way of telling a detach
/// from the session having gone — `tmux has-session` run after the attach
/// process ends. That check is plain command-running with no branching
/// logic of its own worth testing in NatKit, unlike the state it feeds.
struct AgentTerminalDebugView: View {
    let session: String

    var body: some View {
        AgentTerminalHostView(
            attachSpec: AttachSpec(session: session),
            sessionExists: { Self.tmuxHasSession(session) },
            onExit: { reason in
                NSLog("nat: terminal for session \(session) exited: \(reason)")
            }
        )
        .ignoresSafeArea()
    }

    private static func tmuxHasSession(_ session: String) -> Bool {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/env")
        process.arguments = [AttachSpec.executable, "has-session", "-t", session]
        do {
            try process.run()
            process.waitUntilExit()
            return process.terminationStatus == 0
        } catch {
            return false
        }
    }
}

struct ContentView: View {
    var body: some View {
        ZStack {
            DesignTokens.windowBg
                .ignoresSafeArea()

            VStack(spacing: 16) {
                Text("nat")
                    .font(.system(size: 32, weight: .semibold))
                    .foregroundStyle(DesignTokens.label)

                Text("board loading will land here")
                    .font(.system(size: 14, weight: .regular))
                    .foregroundStyle(DesignTokens.labelSecondary)
            }
        }
    }
}
