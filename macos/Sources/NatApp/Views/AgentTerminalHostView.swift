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
        //
        // It is also what a Claude Code inside the pane reads an OSC 11
        // query's answer from: SwiftTerm's `nativeBackgroundColor` setter
        // writes straight through to `terminal.backgroundColor`, which is
        // what `getColors(source:)`'s default implementation answers a
        // query with — so the probe `theme: "auto"` sends at startup, and
        // any later one this view's own `notifyAppearanceChange` prompts,
        // always reads gnat's current chrome.
        TerminalTheme.apply(DesignTokens.palette(for: colorScheme), to: nsView)
        context.coordinator.notifyAppearanceChange(colorScheme, on: nsView)
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

        /// The colour scheme this coordinator last saw, so a report is sent
        /// only on an actual change — nil until the view's first
        /// `updateNSView`, which is what keeps that first call from sending
        /// one: `theme: "auto"` already probes for itself at startup
        /// (`agent.agentCommand`), on the palette `makeNSView` already
        /// applied before the attach process ever started, so a report
        /// there would tell Claude Code nothing it had not already asked.
        private var lastColorScheme: ColorScheme?

        init(sessionExists: @escaping () -> Bool, onExit: @escaping (TerminalExitReason) -> Void) {
            self.sessionExists = sessionExists
            self.onExit = onExit
        }

        /// Reports a colour-scheme change to whatever is attached to view's
        /// pty, if colorScheme is not what this coordinator last saw and the
        /// attach is actually live — a report typed at a session still
        /// starting, or one already gone, has nowhere useful to land.
        ///
        /// The report goes straight to the pty (`send(txt:)`, the same
        /// method a modified enter's CSI-u encoding goes through above) —
        /// this is gnat's own appearance changing, not the user or the
        /// agent doing anything to the pane, so it is not something
        /// SwiftTerm's emulator should interpret, only relay: tmux forwards
        /// it into the attached pane exactly like any other client input,
        /// and Claude Code reads it off its stdin.
        func notifyAppearanceChange(_ colorScheme: ColorScheme, on view: LocalProcessTerminalView) {
            defer { lastColorScheme = colorScheme }
            let attached = if case .attached = lifecycle.state { true } else { false }
            if ColorSchemeReport.shouldReport(from: lastColorScheme, to: colorScheme, attached: attached) {
                view.send(txt: ColorSchemeReport.escape(for: colorScheme))
            }
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

    /// The local monitor that answers a modified enter while this pane holds
    /// the keyboard, installed for as long as this view has a window.
    ///
    /// `performKeyEquivalent` was tried first and does not work: AppKit only
    /// offers that hook a *modified* key equivalent — one carrying command —
    /// so a bare shift+return never reaches it at all and goes straight to
    /// SwiftTerm's own `keyDown`, which cannot be overridden from outside
    /// that module since SwiftTerm declares it `public` rather than `open`.
    /// A local `NSEvent` monitor is offered every key-down in the app before
    /// any responder sees it, `keyDown` included, which is what catching an
    /// unmodified-equivalent key like shift+return actually needs.
    ///
    /// `nonisolated(unsafe)`: `deinit` runs off the main actor even for a
    /// main-actor-isolated class, and removing the monitor there is the only
    /// way to guarantee it on every teardown path rather than just the ones
    /// that go through `viewDidMoveToWindow`. Safe here because nothing but
    /// this view — always on the main thread — ever touches the property.
    private nonisolated(unsafe) var keyDownMonitor: Any?

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        removeKeyDownMonitor()
        guard window != nil else { return }
        keyDownMonitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [weak self] event in
            self?.interceptModifiedEnter(event) ?? event
        }
    }

    deinit {
        if let keyDownMonitor {
            NSEvent.removeMonitor(keyDownMonitor)
        }
    }

    private func removeKeyDownMonitor() {
        if let keyDownMonitor {
            NSEvent.removeMonitor(keyDownMonitor)
        }
        keyDownMonitor = nil
    }

    /// Sends a modified enter as its CSI-u encoding, since the emulator would
    /// send an ordinary carriage return for all three enters and Claude Code
    /// reads that as "submit" — which is exactly what shift+enter must not do.
    ///
    /// Only the two combinations `TerminalKeyEncoding` names are taken; every
    /// other key press, a plain enter included, is returned unchanged so it
    /// falls through to SwiftTerm's own `keyDown`.
    ///
    /// Answers only while this pane itself has the keyboard: the monitor is
    /// offered every key-down in the app regardless of which view — or
    /// window — has focus, so a shift+enter typed into a sheet's text field,
    /// or into another window entirely, is not the agent's and is returned
    /// unchanged.
    ///
    /// `event.window` is checked against this view's own window before
    /// `hasKeyboardFocus` — a window's `firstResponder` is not cleared when
    /// it resigns key, so a stale one left over from before another window
    /// took focus would otherwise read as this pane still having it.
    private func interceptModifiedEnter(_ event: NSEvent) -> NSEvent? {
        guard event.window === window,
              hasKeyboardFocus,
              Self.returnKeyCodes.contains(event.keyCode),
              let bytes = TerminalKeyEncoding.returnKey(Self.modifiers(of: event))
        else {
            return event
        }
        send(txt: bytes)
        return nil
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

    // MARK: - Selection: plain drags win over mouse reporting

    /// Set the moment a plain (non-command) drag begins, cleared on mouseUp.
    /// Distinguishes a drag from a click that never moved, since only a
    /// mouseDragged event tells the two apart — a click alone leaves mouse
    /// reporting exactly as it was.
    private var isRunningLocalSelectionDrag = false

    /// Keeps a command-modified click out of mouse reporting, so tmux — and
    /// the agent behind it — never sees the gesture this pane opens links on;
    /// and clears an existing selection on a plain click that does not drag.
    ///
    /// Without the command carve-out the press would reach tmux, whose
    /// `MouseDown1Pane` binding opens the OSC 8 hyperlink under the mouse,
    /// and the release would reach `requestOpenLink` above: one link, two
    /// opens, two browser tabs. `TerminalMouse` carries the whole reasoning.
    /// A plain click is untouched here and goes where it always went, tmux's
    /// own hyperlink binding included — SwiftTerm's own click-clearing code
    /// never runs for it, since mouse reporting is on for every agent
    /// session and its `mouseDown` returns before reaching that code, so it
    /// is done by hand here instead.
    override func mouseDown(with event: NSEvent) {
        let modifiers = Self.modifiers(of: event)
        guard !TerminalMouse.isTerminalOwnClick(modifiers) else { return }
        if event.clickCount == 1, !modifiers.contains(.shift), selection.active {
            selection.active = false
            setNeedsDisplay(bounds)
        }
        super.mouseDown(with: event)
    }

    /// Forces a plain drag to extend the local selection instead of
    /// reporting motion to tmux, which otherwise always wins: an agent
    /// session's tmux runs with its own `mouse` option on
    /// (`internal/agent/tmux.go`'s `mouseOnArgs`), so SwiftTerm treats every
    /// drag as mouse-tracking traffic for the pane and never reaches its own
    /// selection code at all. Selection wins for a plain drag, full stop —
    /// no modifier scheme, per the brief; a command-modified drag is left
    /// alone since that gesture is the link click's, not selection's.
    ///
    /// Turning reporting off for the whole gesture rather than restoring it
    /// between events is also what keeps streaming output from clearing the
    /// selection mid-drag: SwiftTerm only preserves a manual selection
    /// across a `linefeed` while `allowMouseReporting` reads false, and an
    /// agent's pane never stops producing output for the length of a drag.
    override func mouseDragged(with event: NSEvent) {
        guard !TerminalMouse.isTerminalOwnClick(Self.modifiers(of: event)) else {
            super.mouseDragged(with: event)
            return
        }
        if !isRunningLocalSelectionDrag {
            isRunningLocalSelectionDrag = true
            allowMouseReporting = false
        }
        super.mouseDragged(with: event)
    }

    /// Restores mouse reporting once a forced selection drag ends, after
    /// SwiftTerm's own mouseUp has run — so a drag that stayed local the
    /// whole time never reports the stray release tmux would otherwise
    /// receive with no matching motion before it.
    override func mouseUp(with event: NSEvent) {
        defer {
            if isRunningLocalSelectionDrag {
                isRunningLocalSelectionDrag = false
                allowMouseReporting = true
            }
        }
        super.mouseUp(with: event)
    }

    /// Keeps a selection across streaming output. SwiftTerm's own `linefeed`
    /// clears the selection on every line of output whenever
    /// `allowMouseReporting` is true — which it is again the moment a drag
    /// ends — so an agent's pane wiped a shift+drag selection moments after
    /// release. A click (`mouseDown`) or a new drag is what dismisses it here.
    override func linefeed(source: Terminal) {}

    // MARK: - Copy and paste

    /// Puts the active selection on the pasteboard as plain text, read
    /// through `TerminalGridSelection` rather than SwiftTerm's own
    /// `getSelectedText()` — that merges wrapped rows into one line with no
    /// trim, where the brief wants one line per grid row with each line's
    /// trailing whitespace trimmed.
    override func copy(_ sender: Any) {
        guard selection.active else { return }
        let start = TerminalGridPosition(row: selection.start.row, col: selection.start.col)
        let end = TerminalGridPosition(row: selection.end.row, col: selection.end.col)
        let rows = (min(start.row, end.row)...max(start.row, end.row)).map(gridRowText)
        let text = TerminalGridSelection.text(rows: rows, start: start, end: end)
        guard !text.isEmpty else { return }

        let pasteboard = NSPasteboard.general
        pasteboard.clearContents()
        pasteboard.setString(text, forType: .string)
    }

    /// Pasting files — a Finder copy, or anything else that puts file URLs on
    /// the pasteboard — types their paths, exactly as dropping them does.
    ///
    /// Otherwise, sends the pasteboard's plain text to the session, bracketed
    /// when the running program has asked for it — through
    /// `TerminalPasteEncoding` rather than SwiftTerm's own `paste(_:)`, so
    /// the encoding is the one piece tested against fixtures.
    ///
    /// An image held as raw data rather than as a file is nobody's business
    /// here: a pseudo-terminal carries no bytes but text, and Claude Code's
    /// own ctrl+v reads the Mac's clipboard directly — a key that reaches it
    /// through this pane unchanged, since it is not a command-key gesture.
    override func paste(_ sender: Any) {
        if type(into: Self.filePaths(on: .general)) {
            return
        }
        guard let text = NSPasteboard.general.string(forType: .string) else { return }
        send(txt: TerminalPasteEncoding.send(text, bracketed: terminal.bracketedPasteMode))
    }

    /// One full-width row of the grid at `absoluteRow`, an absolute buffer
    /// row exactly as `selection.start`/`end` count it — the same numbering
    /// `calculateMouseHit` builds those from. `getCharacter(col:row:)` counts
    /// rows from the top of the *visible* screen instead and adds the
    /// scroll offset back in itself, so that offset is subtracted here first
    /// to land on the same row either way.
    private func gridRowText(absoluteRow: Int) -> String {
        let viewportRow = absoluteRow - terminal.buffer.yDisp
        var characters: [Character] = []
        characters.reserveCapacity(terminal.cols)
        for col in 0..<terminal.cols {
            characters.append(terminal.getCharacter(col: col, row: viewportRow) ?? " ")
        }
        return String(characters)
    }

    // MARK: - Files dropped and pasted

    override func draggingEntered(_ sender: any NSDraggingInfo) -> NSDragOperation {
        Self.filePaths(on: sender.draggingPasteboard).isEmpty ? [] : .copy
    }

    override func performDragOperation(_ sender: any NSDraggingInfo) -> Bool {
        type(into: Self.filePaths(on: sender.draggingPasteboard))
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
