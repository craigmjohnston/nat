import AppKit
import SwiftTerm
import NatKit

/// Styles a SwiftTerm view from one of the app's palettes.
///
/// It lives here rather than in NatKit because SwiftTerm is NatApp's
/// dependency alone — NatKit carries the values (`Palette`'s terminal half,
/// where they are documented and asserted) and this is the one place they
/// are turned into the colours SwiftTerm takes. Nothing here decides
/// anything: the palette arrives already chosen.
enum TerminalTheme {
    /// Applies every colour a terminal draws with: the surface, the default
    /// foreground, the caret, the selection wash and the sixteen ANSI
    /// colours. All five have to move together — a light board framing a
    /// terminal whose ANSI palette is still Mocha's is exactly the thing
    /// this stops.
    @MainActor
    static func apply(_ palette: Palette, to view: TerminalView) {
        view.nativeBackgroundColor = NSColor(hex: palette.terminalBg)
        view.nativeForegroundColor = NSColor(hex: palette.terminalFg)
        view.caretColor = NSColor(hex: palette.terminalCursor)
        view.selectedTextBackgroundColor = NSColor(hex: palette.terminalSelection)
        view.installColors(palette.ansi.map(ansiColor))
    }

    /// One ANSI entry as SwiftTerm's own colour type, which counts its
    /// channels in sixteen bits rather than eight.
    private static func ansiColor(_ hex: String) -> SwiftTerm.Color {
        let color = NSColor(hex: hex)
        return SwiftTerm.Color(
            red8: UInt16(clamping: Int((color.redComponent * 255).rounded())),
            green8: UInt16(clamping: Int((color.greenComponent * 255).rounded())),
            blue8: UInt16(clamping: Int((color.blueComponent * 255).rounded()))
        )
    }
}
