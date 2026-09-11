import AppKit
import SwiftTerm
import NatKit

/// Styles a SwiftTerm view from one of the app's palettes.
///
/// It lives here rather than in NatKit because SwiftTerm is NatApp's
/// dependency alone — NatKit carries the values (`Palette`'s terminal half
/// and `TerminalType`, where they are documented and asserted) and this is
/// the one place they are turned into the colours and the font SwiftTerm
/// takes. Nothing here decides anything: the palette arrives already
/// chosen, and so does the type.
enum TerminalTheme {
    /// Applies everything a terminal draws with: its type, then every
    /// colour — the surface, the default foreground, the caret, the
    /// selection wash and the sixteen ANSI colours. The colours all have to
    /// move together — a light board framing a terminal whose ANSI palette
    /// is still Mocha's is exactly the thing this stops — and the type has
    /// to be said at all, since SwiftTerm's own defaults are a font and a
    /// rasterisation nothing else in the window uses.
    @MainActor
    static func apply(_ palette: Palette, to view: TerminalView) {
        // The font is what the cell grid is measured from, so assigning it
        // resets SwiftTerm's glyph caches, drops the selection and resizes
        // the terminal. This runs on every SwiftUI update of the host view,
        // so it is assigned only when it would actually change something —
        // where the colours below are idempotent and cost a redraw.
        let font = TerminalType.font
        if view.font != font {
            view.font = font
        }
        // Stem dilation, which is off: see `TerminalType.smoothsFonts` for
        // what it was doing to the pane. Plain storage on SwiftTerm's side,
        // picked up by the next draw, which the colours below ask for.
        view.fontSmoothing = TerminalType.smoothsFonts

        view.nativeBackgroundColor = NSColor(hex: palette.terminalBg.hex)
        view.nativeForegroundColor = NSColor(hex: palette.terminalFg.hex)
        view.caretColor = NSColor(hex: palette.terminalCursor.hex)
        view.selectedTextBackgroundColor = NSColor(hex: palette.terminalSelection.hex)
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
