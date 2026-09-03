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

    /// `backgroundHex` as a SwiftUI `Color`, for the surface the caller lays
    /// full-bleed behind this view once the terminal itself is inset from
    /// the pane's edges — the padding belongs to the terminal, not to a
    /// margin around a smaller dark rectangle.
    public static let backgroundColor = Color(hex: backgroundHex)

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
        let view = FirstLayoutTerminalView(frame: .zero)
        view.nativeBackgroundColor = NSColor(hex: Self.backgroundHex)
        view.processDelegate = context.coordinator
        // `makeNSView` runs before AppKit has laid this view out at all, so
        // starting the process here would open the pty at SwiftTerm's
        // default ~80 columns and let tmux wrap its whole backlog to that
        // width — the resize that follows, once the pane's real size
        // arrives, is the visible reflow this was reported over. Waiting for
        // the view's first nonzero layout is what lets the pty open at the
        // column count the pane already has.
        view.onFirstRealLayout = { [weak view] in
            guard let view else { return }
            context.coordinator.attach(view, spec: attachSpec)
        }
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
        ///
        /// Guarded by the lifecycle's own state rather than trusted to be
        /// called once: `view`'s first-layout callback already fires at most
        /// once, but a second start landing here regardless — were that
        /// guarantee ever loosened — would open a second pty against the
        /// same session. Only `.idle` and `.exited` are states a
        /// `.startRequested` actually moves out of; `.attaching` and
        /// `.attached` mean a process is already starting or running.
        func attach(_ view: LocalProcessTerminalView, spec: AttachSpec) {
            switch lifecycle.state {
            case .idle, .exited:
                break
            case .attaching, .attached:
                return
            }

            self.view = view
            lifecycle.handle(.startRequested)

            let environment = AttachSpec.environment(from: ProcessInfo.processInfo.environment)
                .map { name, value in "\(name)=\(value)" }

            view.startProcess(
                executable: AttachSpec.resolvedExecutable(),
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

/// A `LocalProcessTerminalView` that answers one question `makeNSView` cannot:
/// when has AppKit actually given this view a real size? SwiftUI hands back
/// the view at `.zero` and sizes it afterward, so anything that wants the
/// pane's true size — starting a pty at the column count it should have from
/// the first paint, rather than SwiftTerm's own default — has to wait for a
/// callback `LocalProcessTerminalView` does not otherwise give.
///
/// `setFrameSize` rather than `layout()`: it is the hook `TerminalView`
/// itself already resizes the terminal from (`processSizeChange`, which
/// updates `terminal.cols`/`rows` from the new frame), so by the time
/// `super.setFrameSize` returns here, the dimensions a `startProcess` reads
/// via `getWindowSize()` already match this size.
final class FirstLayoutTerminalView: LocalProcessTerminalView {
    /// Fired once, the first time AppKit sets this view to a real, nonzero
    /// size. Never fires again after that — a later resize is a plain
    /// resize, which SwiftTerm's own `sizeChanged` delegate callback already
    /// reports to the pty.
    var onFirstRealLayout: (() -> Void)?
    private var hasFiredFirstLayout = false

    override func setFrameSize(_ newSize: NSSize) {
        super.setFrameSize(newSize)
        guard !hasFiredFirstLayout, newSize.width > 0, newSize.height > 0 else { return }
        hasFiredFirstLayout = true
        onFirstRealLayout?()
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
