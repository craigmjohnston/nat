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
    /// The window's appearance, read so this view is re-evaluated when the
    /// theme changes: SwiftTerm's view is an AppKit one that resolves no
    /// dynamic colour of its own, so the palette has to be pushed onto it,
    /// and `updateNSView` only runs for a view that depends on something
    /// that changed.
    @Environment(\.colorScheme) private var colorScheme

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
        TerminalTheme.apply(DesignTokens.palette(for: colorScheme), to: view)
        view.processDelegate = context.coordinator
        // Link tracking, said out loud rather than left to SwiftTerm's
        // defaults. `.implicit` is the part that matters: it finds a URL an
        // agent simply printed as well as one it wrapped in an OSC 8
        // hyperlink, and a printed URL is nearly always what Claude Code
        // writes — without it a bare URL is not a link to click at all.
        //
        // `.hoverWithModifier` is SwiftTerm's own default and is kept
        // deliberately: it makes command+click the gesture that opens a link,
        // which is the one gesture tmux does not also open it on. See
        // `TerminalMouse` for why that matters and `mouseDown` below for the
        // other half of it.
        view.linkReporting = .implicit
        view.linkHighlightMode = .hoverWithModifier
        // Files dropped onto the pane. SwiftTerm registers no dragged type of
        // its own, so without this AppKit never offers the view a drop at
        // all; the view's own `performDragOperation` is what turns one into
        // the paths it types.
        view.registerForDraggedTypes([.fileURL])
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
        //
        // The theme is the one thing that does: the surface, the default
        // foreground, the caret and the sixteen ANSI colours are plain
        // AppKit colours SwiftTerm resolved once, so a light board would go
        // on framing a dark terminal without this. Re-applying the palette
        // the view already has is a no-op that costs a redraw, which is
        // what every other update this method sees is.
        TerminalTheme.apply(DesignTokens.palette(for: colorScheme), to: nsView)
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

            // ProcessInfo is a snapshot that may predate PathBootstrap's
            // setenv; PATH is re-read live so the attach client carries the
            // composed one, like every other child this app spawns.
            var base = AttachSpec.environment(from: ProcessInfo.processInfo.environment)
            if let path = PathBootstrap.environmentValue("PATH") {
                base["PATH"] = path
            }
            let environment = base.map { name, value in "\(name)=\(value)" }

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
///
/// It is also where the three gestures SwiftTerm leaves to its host are
/// answered, since each of them is an override on the view rather than a
/// delegate callback: a modified enter, a clicked link, and files dropped or
/// pasted onto the pane. Every decision any of them makes is NatKit's —
/// `TerminalKeyEncoding`, `TerminalLink`, `TerminalDropText` — so what is
/// here is the AppKit event and nothing more.
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

    // MARK: - The two enters

    /// `kVK_Return` and `kVK_ANSI_KeypadEnter`, the two keys that mean enter
    /// on a Mac keyboard. Named here rather than importing Carbon for two
    /// integers, and read as key codes rather than as characters so the
    /// answer does not depend on the keyboard layout.
    private static let returnKeyCodes: Set<UInt16> = [36, 76]

    /// Sends a modified enter as its CSI-u encoding, since the emulator would
    /// send an ordinary carriage return for all three enters and Claude Code
    /// reads that as "submit" — which is exactly what shift+enter must not do.
    ///
    /// Only the two combinations `TerminalKeyEncoding` names are taken; every
    /// other key press, a plain enter included, falls through to SwiftTerm's
    /// own `keyDown`.
    ///
    /// `performKeyEquivalent` rather than `keyDown`: SwiftTerm declares its
    /// `keyDown` `public` rather than `open`, so it cannot be overridden from
    /// outside that module at all. The key-equivalent hook is the one AppKit
    /// offers the view hierarchy *before* the key reaches the first
    /// responder's `keyDown` — it is how a default button answers a plain
    /// return — which is exactly the interception this needs, and it is
    /// `open`. Consuming the event is what `true` says.
    ///
    /// It is offered to the whole hierarchy regardless of who has the
    /// keyboard, so this only answers while the pane itself does: a
    /// shift+enter typed into a sheet's text field elsewhere in the window is
    /// not the agent's.
    override func performKeyEquivalent(with event: NSEvent) -> Bool {
        guard hasKeyboardFocus,
              Self.returnKeyCodes.contains(event.keyCode),
              let bytes = TerminalKeyEncoding.returnKey(Self.modifiers(of: event))
        else {
            return super.performKeyEquivalent(with: event)
        }
        send(txt: bytes)
        return true
    }

    /// Whether this pane is where typing currently goes — itself, or any view
    /// SwiftTerm keeps inside it for input.
    private var hasKeyboardFocus: Bool {
        guard let responder = window?.firstResponder as? NSView else { return false }
        return responder === self || responder.isDescendant(of: self)
    }

    /// AppKit's modifier flags as NatKit's, reading the four keys the
    /// encoding is about and ignoring the rest — caps lock and the numeric
    /// pad's own flag are not modifiers a key encoding has an opinion on, and
    /// keypad enter carries the latter.
    private static func modifiers(of event: NSEvent) -> TerminalKeyModifiers {
        let flags = event.modifierFlags
        var out: TerminalKeyModifiers = []
        if flags.contains(.shift) { out.insert(.shift) }
        if flags.contains(.control) { out.insert(.control) }
        if flags.contains(.option) { out.insert(.option) }
        if flags.contains(.command) { out.insert(.command) }
        return out
    }

    // MARK: - Links

    /// Opens a link activated by command+click with the Mac's own handler.
    ///
    /// `LocalProcessTerminalView` already opens whatever it is handed; this
    /// overrides that to go through `TerminalLink`, so what an agent's pane
    /// can open on one gesture is a decision written down and tested rather
    /// than "any scheme at all".
    override func requestOpenLink(source: TerminalView, link: String, params: [String: String]) {
        guard let url = TerminalLink.destination(link) else { return }
        NSWorkspace.shared.open(url)
    }

    /// Keeps a command-modified click out of mouse reporting, so tmux — and
    /// the agent behind it — never sees the gesture this pane opens links on.
    ///
    /// Without this the press would reach tmux, whose `MouseDown1Pane`
    /// binding opens the OSC 8 hyperlink under the mouse, and the release
    /// would reach `requestOpenLink` above: one link, two opens, two browser
    /// tabs. `TerminalMouse` carries the whole reasoning. A plain click is
    /// untouched and goes where it always went, tmux's own hyperlink binding
    /// included.
    override func mouseDown(with event: NSEvent) {
        guard !TerminalMouse.isTerminalOwnClick(Self.modifiers(of: event)) else { return }
        super.mouseDown(with: event)
    }

    // MARK: - Files dropped and pasted

    override func draggingEntered(_ sender: any NSDraggingInfo) -> NSDragOperation {
        Self.filePaths(on: sender.draggingPasteboard).isEmpty ? [] : .copy
    }

    override func performDragOperation(_ sender: any NSDraggingInfo) -> Bool {
        type(into: Self.filePaths(on: sender.draggingPasteboard))
    }

    /// Pasting files — a Finder copy, or anything else that puts file URLs on
    /// the pasteboard — types their paths, exactly as dropping them does. A
    /// pasteboard holding no file falls through to SwiftTerm's own paste,
    /// which is the text one.
    ///
    /// An image held as raw data rather than as a file is nobody's business
    /// here: a pseudo-terminal carries no bytes but text, and Claude Code's
    /// own ctrl+v reads the Mac's clipboard directly — a key that reaches it
    /// through this pane unchanged, since it is not a command-key gesture.
    override func paste(_ sender: Any) {
        if type(into: Self.filePaths(on: .general)) {
            return
        }
        super.paste(sender)
    }

    /// Types the paths, answering whether there was anything to type.
    private func type(into paths: [String]) -> Bool {
        let text = TerminalDropText.text(forPaths: paths)
        guard !text.isEmpty else { return false }
        send(txt: text)
        return true
    }

    /// The files a pasteboard names, and nothing else it happens to hold.
    ///
    /// Gated on the pasteboard actually advertising file URLs, so a copied
    /// string that reads like a path stays a string: pasting text is the
    /// paste this pane already did correctly.
    private static func filePaths(on pasteboard: NSPasteboard) -> [String] {
        guard pasteboard.types?.contains(.fileURL) == true else { return [] }
        let read = pasteboard.readObjects(
            forClasses: [NSURL.self],
            options: [.urlReadingFileURLsOnly: true]
        )
        guard let urls = read as? [URL] else { return [] }
        return urls.filter(\.isFileURL).map(\.path)
    }
}
