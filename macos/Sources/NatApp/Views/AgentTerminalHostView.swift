import SwiftUI
import AppKit
import SwiftTerm
import NatKit

/// Hosts a `SwiftTerm.LocalProcessTerminalView` attached to a running
/// agent's tmux session — the macOS app's answer to the Go TUI's
/// `agentview.go`, which runs `AttachClientCmd` on a `vterm.Session` and
/// draws it beside the board.
///
/// Every decision lives in NatKit and is tested there: the argv and
/// environment an attach needs come from `AttachSpec`, and what state the
/// terminal is in comes from `TerminalLifecycle`. This view only wires
/// SwiftTerm's callbacks to those two.
public struct AgentTerminalHostView: NSViewRepresentable {
    /// The background the terminal area renders on — the dark terminal
    /// panel from the design mock's `AgentTerminalView` (`#121216`), kept as
    /// a constant here until the surrounding chrome (tabs, footer, the
    /// composer row) is its own later task.
    public static let backgroundHex = "121216"

    private let attachSpec: AttachSpec

    /// Answers whether the tmux session `attachSpec` names still exists,
    /// asked only after the attach process has ended — the process's own
    /// exit status cannot tell a detach from the session dying, so the
    /// terminal's caller is the one asking tmux, not this view guessing.
    private let sessionExists: () -> Bool

    /// Called once the attach process has ended, with which of the two the
    /// termination turned out to be.
    private let onExit: (TerminalExitReason) -> Void

    public init(
        attachSpec: AttachSpec,
        sessionExists: @escaping () -> Bool,
        onExit: @escaping (TerminalExitReason) -> Void
    ) {
        self.attachSpec = attachSpec
        self.sessionExists = sessionExists
        self.onExit = onExit
    }

    public func makeCoordinator() -> Coordinator {
        Coordinator(sessionExists: sessionExists, onExit: onExit)
    }

    public func makeNSView(context: Context) -> LocalProcessTerminalView {
        let view = LocalProcessTerminalView(frame: .zero)
        view.nativeBackgroundColor = NSColor(hex: Self.backgroundHex)
        view.processDelegate = context.coordinator
        context.coordinator.attach(view, spec: attachSpec)
        return view
    }

    public func updateNSView(_ nsView: LocalProcessTerminalView, context: Context) {
        // Resizing is handled by AppKit's own layout of the view plus
        // SwiftTerm's sizeChanged callback telling the pty its new
        // dimensions; there is nothing this binding needs to push down on
        // every SwiftUI update.
    }

    public static func dismantleNSView(_ nsView: LocalProcessTerminalView, coordinator: Coordinator) {
        // Tearing the SwiftUI view down is a tab switch, not a "close the
        // agent": detaching ends this attach's client only, and the tmux
        // session it was attached to goes on running.
        coordinator.detach()
    }

    /// Bridges SwiftTerm's `LocalProcessTerminalViewDelegate` callbacks to
    /// `TerminalLifecycle` and starts the attach process itself. Kept
    /// separate from the view so it can own the one piece of state
    /// (`lifecycle`) SwiftUI's value-type view cannot.
    @MainActor
    public final class Coordinator: NSObject, @MainActor LocalProcessTerminalViewDelegate {
        private var lifecycle = TerminalLifecycle()
        private let sessionExists: () -> Bool
        private let onExit: (TerminalExitReason) -> Void
        private weak var view: LocalProcessTerminalView?

        init(sessionExists: @escaping () -> Bool, onExit: @escaping (TerminalExitReason) -> Void) {
            self.sessionExists = sessionExists
            self.onExit = onExit
        }

        /// Starts the attach process on view, using spec for its argv and
        /// environment.
        func attach(_ view: LocalProcessTerminalView, spec: AttachSpec) {
            self.view = view
            lifecycle.handle(.startRequested)

            let environment = AttachSpec.environment(from: ProcessInfo.processInfo.environment)
                .map { name, value in "\(name)=\(value)" }

            view.startProcess(
                executable: AttachSpec.executable,
                args: spec.arguments,
                environment: environment
            )
            lifecycle.handle(.processLaunched)
        }

        /// Ends this attach's client without touching the tmux session it
        /// was attached to — exactly what tabbing away from the terminal
        /// wants, and what a later reattach recreates the process over.
        func detach() {
            view?.terminate()
        }

        public func processTerminated(source: TerminalView, exitCode: Int32?) {
            let stillThere = sessionExists()
            let next = lifecycle.handle(.processTerminated(sessionStillExists: stillThere))
            if case .exited(let reason) = next {
                onExit(reason)
            }
        }

        public func sizeChanged(source: LocalProcessTerminalView, newCols: Int, newRows: Int) {}

        public func setTerminalTitle(source: LocalProcessTerminalView, title: String) {}

        public func hostCurrentDirectoryUpdate(source: TerminalView, directory: String?) {}
    }
}

extension NSColor {
    /// A convenience mirroring `NatKit.Color(hex:)`, for the one native
    /// AppKit color this view sets directly on SwiftTerm's view rather than
    /// through SwiftUI.
    convenience init(hex: String) {
        var rgb: UInt64 = 0
        Scanner(string: hex).scanHexInt64(&rgb)
        self.init(
            srgbRed: CGFloat((rgb >> 16) & 0xFF) / 255,
            green: CGFloat((rgb >> 8) & 0xFF) / 255,
            blue: CGFloat(rgb & 0xFF) / 255,
            alpha: 1
        )
    }
}
