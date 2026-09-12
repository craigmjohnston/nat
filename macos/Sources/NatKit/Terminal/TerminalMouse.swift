import Foundation

/// Which clicks in the agent pane are the terminal's own, rather than the
/// program running inside it.
///
/// The pane is a tmux client, and tmux holds the mouse: its own `mouse`
/// option is on in the sessions nat makes and Claude Code asks for mouse
/// reporting besides, so an ordinary click is reported straight through and
/// whatever tmux and the agent make of it is theirs. One gesture has to be
/// kept back, though, and the reason is not taste.
///
/// nat binds tmux's `MouseDown1Pane` to open the OSC 8 hyperlink under the
/// mouse (`hyperlinkClickArgs` in `internal/agent/tmux.go`) — it has to,
/// because the Go TUI's own terminal widget cannot open a link at all. This
/// pane's terminal can. So a gesture both layers act on opens the link twice,
/// once from tmux on the press and once from here on the release, and there
/// is no way to tell the two apart after the fact: SwiftTerm reports an
/// activated link with no flag saying whether it came from an OSC 8 payload
/// or from its own detector, which is precisely the distinction tmux acts on.
///
/// So the link gesture here is the one tmux never sees. A command-modified
/// click is withheld from mouse reporting entirely, which leaves exactly one
/// layer opening exactly one link — and it is the gesture Terminal.app and
/// iTerm2 already use for a link, where a plain click goes to the program and
/// command+click does not.
public enum TerminalMouse {
    /// Whether a click carrying `modifiers` is the terminal's own to act on
    /// and to keep from the program inside it.
    ///
    /// Command and nothing about the other three: shift is already the
    /// emulator's own bypass, for selecting text through an application that
    /// has asked for the mouse, and control and option are modifiers tmux and
    /// the agent encode and read themselves.
    public static func isTerminalOwnClick(_ modifiers: TerminalKeyModifiers) -> Bool {
        modifiers.contains(.command)
    }
}
