import Foundation

/// The modifier keys a terminal key press can carry, said in NatKit's own
/// terms rather than AppKit's.
///
/// It exists so the decision below can be tested without an `NSEvent`: the
/// view bridge maps `NSEvent.ModifierFlags` onto this and nothing else about
/// a key press crosses into NatKit.
public struct TerminalKeyModifiers: OptionSet, Sendable, Hashable {
    public let rawValue: Int

    public init(rawValue: Int) {
        self.rawValue = rawValue
    }

    public static let shift = TerminalKeyModifiers(rawValue: 1 << 0)
    public static let control = TerminalKeyModifiers(rawValue: 1 << 1)
    public static let option = TerminalKeyModifiers(rawValue: 1 << 2)
    public static let command = TerminalKeyModifiers(rawValue: 1 << 3)
}

/// The bytes a modified enter has to be written as by hand, copied from the
/// Go TUI's `internal/tui/agentview.go` — `shiftEnterBytes` and
/// `ctrlEnterBytes`.
///
/// An emulator left to its own devices sends an ordinary carriage return for
/// all three enters, and the modifier is the whole point of two of them in
/// Claude Code, where shift+enter breaks the line and ctrl+enter accepts. So
/// they are written as their CSI-u encodings instead, and the attach client
/// advertises `extkeys` (see `AttachSpec.viewerFeatures`) so the tmux on the
/// far end passes them through rather than folding them back to a return.
public enum TerminalKeyEncoding {
    /// CSI-u for enter with shift held: keycode 13, modifier bitmask 1 (shift)
    /// plus 1, which is how CSI-u numbers a modifier set.
    public static let shiftEnter = "\u{1b}[13;2u"

    /// CSI-u for enter with control held — the same encoding with control's
    /// bit 4 in place of shift's 1.
    public static let ctrlEnter = "\u{1b}[13;5u"

    /// The bytes to send for an enter carrying `modifiers`, or nil for one
    /// the emulator should encode itself.
    ///
    /// The match is on the whole modifier set rather than on a bit being
    /// present, exactly as the Go TUI's keymap matches: a plain enter is the
    /// emulator's to encode, and so is any combination neither of the two
    /// keys the board forwards stands for — writing a guess for shift+ctrl or
    /// for option+enter would be inventing a key Claude Code was never told
    /// about.
    public static func returnKey(_ modifiers: TerminalKeyModifiers) -> String? {
        if modifiers == .shift {
            return shiftEnter
        }
        if modifiers == .control {
            return ctrlEnter
        }
        return nil
    }
}
