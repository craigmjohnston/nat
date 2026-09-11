import SwiftUI

/// What a view is drawn on, carried down the view tree.
///
/// Colour in this app is relative: a rule, a wash and a chip's word are all
/// the theme's colours mixed into the ground behind them, so every one of them
/// needs to know what that ground is. Passing it at each call site works and
/// is what this replaced, but it is the wrong shape — a container that changes
/// its own fill silently invalidates every `on:` written inside it, and there
/// were about a hundred and ninety of them.
///
/// Declared once by whatever paints the ground, it cannot fall out of step:
/// `Card` says `.card` and everything inside it, however deeply nested, is
/// coloured against a card.
private struct GroundKey: EnvironmentKey {
    /// The window's own ground, which is what a view not inside anything else
    /// is drawn on.
    static let defaultValue: Ground = .window
}

extension EnvironmentValues {
    public var ground: Ground {
        get { self[GroundKey.self] }
        set { self[GroundKey.self] = newValue }
    }
}

extension View {
    /// Paint a ground and declare it: the fill, the corner radius if it has
    /// one, and the `ground` every descendant resolves its colours against.
    ///
    /// The two halves are one call on purpose. Painting without declaring is
    /// how a card ends up with text coloured for the window behind it, and
    /// declaring without painting is a claim about a surface nobody drew.
    public func surface(_ ground: Ground, radius: CGFloat? = nil) -> some View {
        modifier(SurfaceModifier(ground: ground, radius: radius))
    }
}

struct SurfaceModifier: ViewModifier {
    let ground: Ground
    let radius: CGFloat?

    func body(content: Content) -> some View {
        Group {
            if let radius {
                content
                    .background(DesignTokens.fill(ground))
                    .clipShape(RoundedRectangle(cornerRadius: radius))
            } else {
                content.background(DesignTokens.fill(ground))
            }
        }
        .environment(\.ground, ground)
    }
}
