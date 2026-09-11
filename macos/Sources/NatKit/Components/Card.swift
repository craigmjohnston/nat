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
        content()
            .surface(.card, radius: radius)
            .overlay {
                if border {
                    RoundedRectangle(cornerRadius: radius)
                        .stroke(DesignTokens.controlBorder(on: .card), lineWidth: 0.5)
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
        content()
            .surface(.field, radius: radius)
            .overlay {
                RoundedRectangle(cornerRadius: radius)
                    .stroke(DesignTokens.controlBorder(on: .field), lineWidth: 0.5)
            }
    }
}
