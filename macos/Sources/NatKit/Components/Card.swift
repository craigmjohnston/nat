import SwiftUI

/// A raised surface with a border: the rail's notice, a PR section, a diff
/// file box, the brief's panels.
///
/// It existed fifteen times as three hand-written modifiers — a fill, a
/// rounded clip and a stroked overlay — across six files, each free to pick
/// its own radius and its own border weight, and each leaving whatever it
/// contained to guess what it was drawn on. That is the same way five
/// different submit buttons happened before `PrimaryButtonStyle`, and the
/// same cure: the shape is stated once and the ground it declares travels
/// with it.
extension View {
    /// A raised surface with a border: ground, radius and edge in one call.
    /// `Card` is this modifier with a container around it, for the call sites
    /// where a wrapper reads better than a suffix.
    public func card(radius: CGFloat = 10, border: RuleWeight = .border) -> some View {
        surface(.card, radius: radius)
            .overlay {
                RoundedRectangle(cornerRadius: radius)
                    .stroke(DesignTokens.rule(border, on: .card), lineWidth: border == .border ? 0.5 : 1)
            }
    }

    /// The well text is typed into.
    public func field(radius: CGFloat = 8, border: RuleWeight = .border) -> some View {
        surface(.field, radius: radius)
            .overlay {
                RoundedRectangle(cornerRadius: radius)
                    .stroke(DesignTokens.rule(border, on: .field), lineWidth: border == .border ? 0.5 : 1)
            }
    }

    /// A control's own face: a pill, a menu button.
    public func control(radius: CGFloat = 6, border: RuleWeight = .border) -> some View {
        surface(.control, radius: radius)
            .overlay {
                RoundedRectangle(cornerRadius: radius)
                    .stroke(DesignTokens.rule(border, on: .control), lineWidth: border == .border ? 0.5 : 1)
            }
    }
}

public struct Card<Content: View>: View {
    var radius: CGFloat
    var border: Bool
    @ViewBuilder var content: () -> Content

    public init(radius: CGFloat = 10, border: Bool = true, @ViewBuilder content: @escaping () -> Content) {
        self.radius = radius
        self.border = border
        self.content = content
    }

    public var body: some View {
        Group {
            if border {
                content().card(radius: radius)
            } else {
                content().surface(.card, radius: radius)
            }
        }
    }
}

/// A band laid over the window ground: a pane's header, the brief's footer,
/// the notice row under it. Half a card, so it reads as part of the pane
/// rather than as a card of its own.
public struct Band<Content: View>: View {
    @ViewBuilder var content: () -> Content

    public init(@ViewBuilder content: @escaping () -> Content) {
        self.content = content
    }

    public var body: some View {
        content().surface(.band)
    }
}

/// The well text is typed into.
public struct Field<Content: View>: View {
    var radius: CGFloat
    @ViewBuilder var content: () -> Content

    public init(radius: CGFloat = 8, @ViewBuilder content: @escaping () -> Content) {
        self.radius = radius
        self.content = content
    }

    public var body: some View {
        content().field(radius: radius)
    }
}
