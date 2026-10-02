import SwiftUI
import NatKit

/// The skeleton blocks and the agent stage's first frame: drawn as the
/// content they stand in for rather than as a spinner in an empty rectangle,
/// so what arrives lands on a layout that is already right.

// MARK: - Shared pieces

/// The type a placeholder line stands in for: the font the real text is set
/// in, which the line's row is measured by, and the point size that font was
/// asked for at, which its block's thickness is derived from.
///
/// A pair rather than a `Font` alone, since a `Font` cannot be asked how big
/// it is — and the whole point of naming the type at all is that the line
/// takes the room the real run of it will take.
struct SkeletonType {
    let font: Font
    let size: CGFloat

    static func system(_ size: CGFloat, weight: Font.Weight = .regular) -> SkeletonType {
        SkeletonType(font: .system(size: size, weight: weight), size: size)
    }

    static func mono(_ size: CGFloat) -> SkeletonType {
        SkeletonType(font: Typo.mono(size: size), size: size)
    }

    /// How thick the block is drawn — the ink of a line rather than its whole
    /// box; see `SkeletonLayout.lineThickness(forTextOf:)`.
    var thickness: CGFloat { SkeletonLayout.lineThickness(forTextOf: size) }
}

/// One placeholder line of prose, taking exactly the room one line of the
/// type it stands in for takes: a hidden run of that very font is what gives
/// the row its height, so the text that replaces it lands on the row the
/// block held rather than a point or two off it.
///
/// Its width is its share of the space its parent has left for it, measured
/// through a `GeometryReader` rather than the `containerRelativeFrame` the
/// rail's own blocks use: a container is the scroll view or the window, and a
/// line inside a card inside a scroll view has fixed insets between it and
/// either of them, so a fraction of the container runs out past the card it
/// is drawn in. The reader sits in an overlay on the sizer rather than around
/// it, which is what has it measure the width this line actually has instead
/// of whatever a greedy reader is proposed.
struct SkeletonTextLine: View {
    let width: Double
    var type: SkeletonType = .system(Typo.body)
    var cornerRadius: CGFloat = 3

    var body: some View {
        Text(" ")
            .font(type.font)
            .hidden()
            .frame(maxWidth: .infinity, alignment: .leading)
            .overlay(alignment: .leading) {
                GeometryReader { proxy in
                    SkeletonBlock(
                        width: SkeletonLayout.lineWidth(width, in: proxy.size.width),
                        height: type.thickness,
                        cornerRadius: cornerRadius
                    )
                    .frame(maxHeight: .infinity, alignment: .center)
                }
            }
    }
}

/// A placeholder for a short run of type whose width is known rather than a
/// share of the row — a line number, a ± tally, a button's label. Its row is
/// the type's own line, measured the way `SkeletonTextLine`'s is, so the
/// block sits where the run's ink will sit instead of at the top of a row it
/// only half fills.
struct SkeletonTextRun: View {
    let width: CGFloat
    var type: SkeletonType = .system(Typo.subhead)
    var cornerRadius: CGFloat = 2

    var body: some View {
        Text(" ")
            .font(type.font)
            .hidden()
            .frame(width: width)
            .overlay {
                SkeletonBlock(width: width, height: type.thickness, cornerRadius: cornerRadius)
            }
    }
}

/// A run of them — one paragraph, one comment's body, one section's values —
/// at the `lineSpacing` the type they stand in for is set at, so the run
/// comes to the height the real paragraph comes to.
struct SkeletonParagraph: View {
    let lines: SkeletonLines
    var type: SkeletonType = .system(Typo.body)
    /// The `lineSpacing` on the real `Text`, which SwiftUI adds between its
    /// lines and nowhere else — hence a `VStack` spacing of exactly it.
    var lineSpacing: CGFloat = 2

    var body: some View {
        VStack(alignment: .leading, spacing: lineSpacing) {
            ForEach(Array(lines.enumerated()), id: \.offset) { _, width in
                SkeletonTextLine(width: width, type: type)
            }
        }
    }
}

/// The busy mark a pane wears while a read runs over content already on
/// screen: a small spinner in a slot that is there whether it is spinning or
/// not, so admitting to the refresh moves nothing beside it. Not the rule
/// `AsyncActionLabel` follows for a button — a button grows by its spinner,
/// because a press is what started the work and the growth says so; nobody
/// pressed anything for a background read.
struct RefreshingMark: View {
    let isRefreshing: Bool

    var body: some View {
        BusySlot(isBusy: isRefreshing, label: "Refreshing…")
    }
}

/// The Agent stage between pressing Launch Agent and the session appearing:
/// the terminal's own surface and inset, with placeholder output lines.
struct AgentSkeletonView: View {
    var body: some View {
        ZStack(alignment: .topLeading) {
            DesignTokens.fill(.terminal)

            VStack(alignment: .leading, spacing: 6) {
                ForEach(Array(AgentSkeleton.lines.enumerated()), id: \.offset) { _, width in
                    SkeletonTextLine(width: width, type: .mono(Typo.code))
                }
            }
            .padding(.vertical, 14)
            .padding(.horizontal, 18)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(AgentSkeleton.accessibilityLabel)
    }
}

#Preview("Agent") {
    AgentSkeletonView()
        .frame(width: 900, height: 560)
}
