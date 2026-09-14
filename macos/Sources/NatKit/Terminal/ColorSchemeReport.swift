import SwiftUI

/// The report a terminal sends when the colour scheme behind it changes —
/// half of the live re-theme protocol Claude Code speaks when launched with
/// `theme: "auto"` (`agent.agentCommand` on the Go side): `CSI ?997;1n` for
/// dark, `CSI ?997;2n` for light. Verified empirically against a real
/// `claude` process — receiving this on its stdin is what makes it re-probe
/// OSC 11 and re-theme live, no restart.
///
/// gnat's embedded terminal is what has to send it: it stands in for a real
/// terminal on the far end of the attach client's pty (`AgentTerminalHostView`
/// runs SwiftTerm's `LocalProcessTerminalView` directly against `tmux
/// attach-session`), and nothing else is watching gnat's own appearance
/// change to report it on Claude Code's behalf.
public enum ColorSchemeReport {
    /// The escape sequence to send for `colorScheme`.
    public static func escape(for colorScheme: ColorScheme) -> String {
        colorScheme == .dark ? "\u{1b}[?997;1n" : "\u{1b}[?997;2n"
    }

    /// Whether a report is worth sending: `last` is the scheme the terminal
    /// host last saw, nil before its own first reading — `theme: "auto"`
    /// already probes for itself against the palette that first reading
    /// applied, so nothing is sent for it, only for an actual change after.
    /// `attached` is whether the attach client's pty is actually live to
    /// receive one — typed at a session still starting, or one already
    /// gone, it has nowhere useful to land.
    public static func shouldReport(from last: ColorScheme?, to current: ColorScheme, attached: Bool) -> Bool {
        guard let last else { return false }
        return attached && last != current
    }
}
