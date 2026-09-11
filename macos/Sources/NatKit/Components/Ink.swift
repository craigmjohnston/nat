import SwiftUI

/// What a piece of text is *for*, which is the only thing a call site should
/// have to say about its colour.
///
/// The tiers and the tones are one vocabulary rather than two because a call
/// site is choosing between them — a row's title is `.primary` and its error
/// is `.danger` — and because both answer the same question of the ground
/// they land on. What they are not is colour names: `.success` is the word
/// because `systemGreen` says what a colour looks like in one theme and
/// nothing about what it means, and a theme whose success colour is not green
/// would have to be argued with.
public enum InkRole: Sendable {
    /// Titles and row labels.
    case primary
    /// Supporting text.
    case secondary
    /// Meta lines and timestamps.
    case tertiary
    /// The disabled glyph and the empty-slot rule — never words to read, and
    /// so the one role held to no contrast bar.
    case quaternary
    /// What is written on a filled accent control.
    case onAccent

    case accent, success, danger, warning, info
}

extension View {
    /// Colour text by what it is for, against whatever ground it is on.
    ///
    /// This is the whole public colour vocabulary for text. There is no
    /// `foregroundStyle(DesignTokens.…)` left in the view layer, and no ground
    /// to pass: `Card` said what it was, and this reads it.
    public func ink(_ role: InkRole) -> some View {
        modifier(InkModifier(role: role))
    }
}

struct InkModifier: ViewModifier {
    let role: InkRole
    @Environment(\.ground) private var ground

    func body(content: Content) -> some View {
        content.foregroundStyle(DesignTokens.ink(role, on: ground))
    }
}

extension View {
    /// Lay a wash under something: a selected row, an avatar's disc, a diff
    /// row's fill. The tone says which hue and the role says how hard it is
    /// pressed; the ground comes from whatever declared it.
    public func wash(_ role: WashRole, tone: Tone = .accent) -> some View {
        modifier(WashModifier(role: role, tone: tone))
    }
}

struct WashModifier: ViewModifier {
    let role: WashRole
    let tone: Tone
    @Environment(\.ground) private var ground

    func body(content: Content) -> some View {
        content.background(DesignTokens.wash(role, tone: tone.chipTint, on: ground))
    }
}
