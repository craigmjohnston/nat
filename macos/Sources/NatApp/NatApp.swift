import SwiftUI
import NatKit

@main
struct NatApp: App {
    @State private var appModel = AppModel()

    var body: some Scene {
        // The mock's canvas is 1360×840 and every metric in it was chosen at
        // that size — opening there is what makes the proportions read as
        // designed.
        WindowGroup("nat") {
            // NAT_TERM_SESSION is a debug affordance only: it lets a session
            // name be smoke-tested against a real tmux session before the
            // Agent tab has anywhere of its own to launch one from. Anyone
            // launching NatApp normally sees the full shell view.
            if let session = ProcessInfo.processInfo.environment["NAT_TERM_SESSION"] {
                AgentTerminalDebugView(session: session)
            } else {
                WindowShellView(appModel: appModel)
                    .task { await Self.snapshotIfAsked(appModel) }
            }
        }
        // The header row IS the title bar (WindowShellView reserves room for
        // the traffic lights and makes itself draggable) — hiding the system
        // one is what lets the project tabs sit where the mock puts them,
        // flush with the top of the window rather than below a bar of their
        // own.
        .windowStyle(.hiddenTitleBar)
        .defaultSize(width: 1360, height: 840)

        Settings {
            SettingsView(appModel: appModel)
        }
    }

    /// NAT_SNAPSHOT is the headless eye on the window: with it set to a file
    /// path, the app waits for the first loads to land, renders the shell
    /// offscreen at the mock's canvas size, writes the PNG there and exits.
    /// It exists because screencapture needs a permission a build agent does
    /// not have, and a screen nobody can look at is a screen nobody checks.
    @MainActor
    private static func snapshotIfAsked(_ appModel: AppModel) async {
        guard let path = ProcessInfo.processInfo.environment["NAT_SNAPSHOT"] else { return }
        for _ in 0..<30 {
            try? await Task.sleep(nanoseconds: 1_000_000_000)
            guard let state = appModel.projectStore?.state else { continue }
            if state.projectInfo != nil || state.errorMessage != nil { break }
        }
        NSLog("nat snapshot: store state = %@",
              String(describing: appModel.projectStore?.state).prefix(300) as CVarArg)
        if let want = ProcessInfo.processInfo.environment["NAT_SNAPSHOT_SELECT"],
           let info = appModel.projectStore?.state.projectInfo {
            appModel.selectedSliceID = want == "first"
                ? info.slices.first(where: { $0.status != "Done" })?.id
                : want
            try? await Task.sleep(nanoseconds: 3_000_000_000)
        }
        try? await Task.sleep(nanoseconds: 1_000_000_000)
        // The window's own drawn pixels, not an offscreen ImageRenderer pass:
        // the renderer skips scrollable containers' content, and a snapshot
        // that lies about the window is worse than none.
        if let window = NSApp.windows.first(where: { $0.isVisible }),
           let view = window.contentView {
            window.setContentSize(NSSize(width: 1360, height: 840))
            window.layoutIfNeeded()
            try? await Task.sleep(nanoseconds: 1_000_000_000)
            if let rep = view.bitmapImageRepForCachingDisplay(in: view.bounds) {
                view.cacheDisplay(in: view.bounds, to: rep)
                if let png = rep.representation(using: .png, properties: [:]) {
                    try? png.write(to: URL(fileURLWithPath: path))
                }
            }
        }
        exit(0)
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
