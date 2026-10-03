import AppKit
import SwiftTerm

/// A diagnostic-only log of the agent pane's keyboard path: every key-down
/// the pane is offered and what it decided, and every byte run handed to the
/// pty. It exists to see the one hop no test exercises — from an `NSEvent`
/// to the pty write — and fixes nothing itself.
/// `docs/debugging/agent-pane-keys.md` says how to read it.
///
/// Entirely gated on `NAT_KEY_DEBUG=1`, read once: with it unset `enabled`
/// is false and `log` returns before building its message.
enum KeyDebug {
    static let enabled = ProcessInfo.processInfo.environment["NAT_KEY_DEBUG"] == "1"

    static func log(_ message: @autoclosure () -> String) {
        guard enabled else { return }
        NSLog("nat key-debug: %@", message())
    }

    /// Under `NAT_KEY_DEBUG_SYNTH=1` as well, posts shift+return, ctrl+return
    /// and a plain return to the app's own event queue, a few seconds after
    /// launch and a second apart — the same road through `sendEvent`, the
    /// local monitors and the first responder a hardware press takes, with
    /// no accessibility permission needed. A no-op otherwise.
    ///
    /// Timers in the run loop's common modes rather than a sleep in a
    /// `.task`: a dev binary's updater runs an app-modal alert at launch,
    /// and nothing scheduled in the default mode fires under it — the
    /// presses silently never happened, which reads as a pass.
    @MainActor
    static func synthesizeIfAsked() {
        guard enabled, ProcessInfo.processInfo.environment["NAT_KEY_DEBUG_SYNTH"] == "1" else { return }
        log("synth: armed")
        let presses: [(String, NSEvent.ModifierFlags)] = [("shift", .shift), ("control", .control), ("plain", [])]
        for (index, press) in presses.enumerated() {
            let timer = Timer(timeInterval: 4 + Double(index), repeats: false) { _ in
                MainActor.assumeIsolated { post(press.0, press.1) }
            }
            // Common modes, so a modal loop cannot hold the presses back.
            RunLoop.main.add(timer, forMode: .common)
        }
    }

    @MainActor
    private static func post(_ name: String, _ flags: NSEvent.ModifierFlags) {
        // A dev binary's updater puts up an app-modal alert at launch, which
        // would take the presses itself; a person clicks it away, so does this.
        if let modal = NSApp.modalWindow {
            log("synth: dismissing modal \"\(modal.title)\"")
            NSApp.abortModal()
            modal.orderOut(nil)
        }
        guard let window = NSApp.keyWindow ?? NSApp.windows.first(where: \.isVisible) else {
            log("synth: no window to post to")
            return
        }
        // A real press follows a click into the pane; stand in for the click
        // only where nothing in the window is the terminal yet, and say so.
        if !(window.firstResponder is LocalProcessTerminalView),
           let pane = terminal(in: window.contentView) {
            log("synth: focusing pane (first responder was \(window.firstResponder.map { String(describing: Swift.type(of: $0)) } ?? "nil"))")
            window.makeFirstResponder(pane)
        }
        log("synth: posting \(name)+return to window \(window.windowNumber)")
        for type in [NSEvent.EventType.keyDown, .keyUp] {
            if let event = NSEvent.keyEvent(
                with: type, location: .zero, modifierFlags: flags,
                timestamp: ProcessInfo.processInfo.systemUptime, windowNumber: window.windowNumber,
                context: nil, characters: "\r", charactersIgnoringModifiers: "\r",
                isARepeat: false, keyCode: 36
            ) {
                NSApp.postEvent(event, atStart: false)
            }
        }
    }

    @MainActor
    private static func terminal(in view: NSView?) -> LocalProcessTerminalView? {
        guard let view else { return nil }
        if let terminal = view as? LocalProcessTerminalView { return terminal }
        for subview in view.subviews {
            if let found = terminal(in: subview) { return found }
        }
        return nil
    }

    /// Bytes as a readable run: printable ASCII as itself, everything else
    /// escaped (`\r`, `\x1b`, …), so a CSI-u and a bare return can't be
    /// mistaken for one another.
    static func escaped(_ bytes: some Sequence<UInt8>) -> String {
        bytes.map { byte in
            switch byte {
            case 0x0d: "\\r"
            case 0x0a: "\\n"
            case 0x09: "\\t"
            case 0x5c: "\\\\"
            case 0x20...0x7e: String(UnicodeScalar(byte))
            default: String(format: "\\x%02x", byte)
            }
        }.joined()
    }
}
