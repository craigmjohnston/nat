import SwiftUI

/// The shimmer's own constants and the two pieces of arithmetic behind it,
/// kept apart from the view so they can be read — and tested — without
/// mounting anything.
///
/// A skeleton is the other half of `QuietLoadingView`'s bargain: where the
/// spinner is what a wait with no shape to it gets, this is what a wait
/// whose shape is already known gets — placeholder blocks laid out exactly
/// where the content will be, so the layout is settled before the content
/// lands and nothing jumps when it does.
public enum Skeleton {
    /// The block itself: the same barely-there wash a hover paints, which is
    /// what keeps a screenful of them from reading as content.
    public static let base = DesignTokens.labelQuaternary

    /// The sweep passing over it — brighter than the block and still far
    /// under anything the app draws as text.
    public static let highlight = DesignTokens.skeletonHighlight

    /// How long one pass takes. Slow enough to read as breathing rather than
    /// as something loading fast.
    public static let cycle: Double = 1.4

    /// How wide the sweep is as a fraction of the block it crosses.
    public static let highlightWidthRatio: CGFloat = 0.55

    /// Where the sweep sits for a phase running 0…1: one full block width
    /// off the leading edge at 0 and one full width past the trailing edge
    /// at 1, so the highlight is clear of the block at both ends of the
    /// cycle instead of parking on it when the animation restarts.
    public static func highlightOffset(phase: Double, width: CGFloat) -> CGFloat {
        CGFloat(phase * 2 - 1) * width
    }

    /// The sweep's animation, or nil where the user has asked for reduced
    /// motion — in which case the block is drawn flat and nothing moves at
    /// all, which is a placeholder still doing its whole job.
    public static func sweep(reduceMotion: Bool) -> Animation? {
        reduceMotion ? nil : .linear(duration: cycle).repeatForever(autoreverses: false)
    }
}

/// One placeholder block: a rounded rectangle in the skeleton wash with a
/// highlight sweeping across it, shaped and sized by its caller to stand in
/// for the thing that is coming — a row's glyph, a title, a paragraph line.
///
/// Callers build a skeleton out of these at the geometry of the real content
/// (the same heights, the same indents, the same leading axis), so the
/// arriving content replaces them without moving anything. `width: nil` fills
/// whatever the caller frames it in.
public struct SkeletonBlock: View {
    private let width: CGFloat?
    private let height: CGFloat
    private let cornerRadius: CGFloat

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var phase: Double = 0

    public init(width: CGFloat? = nil, height: CGFloat, cornerRadius: CGFloat = 3) {
        self.width = width
        self.height = height
        self.cornerRadius = cornerRadius
    }

    public var body: some View {
        let shape = RoundedRectangle(cornerRadius: cornerRadius, style: .continuous)
        return shape
            .fill(Skeleton.base)
            .frame(width: width, height: height)
            .overlay {
                if !reduceMotion {
                    GeometryReader { proxy in
                        LinearGradient(
                            colors: [.clear, Skeleton.highlight, .clear],
                            startPoint: .leading,
                            endPoint: .trailing
                        )
                        .frame(width: proxy.size.width * Skeleton.highlightWidthRatio)
                        .offset(x: Skeleton.highlightOffset(phase: phase, width: proxy.size.width))
                    }
                }
            }
            .clipShape(shape)
            .onAppear {
                guard !reduceMotion else { return }
                withAnimation(Skeleton.sweep(reduceMotion: false)) { phase = 1 }
            }
            // One block is a piece of a placeholder rather than anything to
            // read out; the view standing in for the content is what carries
            // the label saying it is loading.
            .accessibilityHidden(true)
    }
}

#Preview {
    VStack(alignment: .leading, spacing: 10) {
        SkeletonBlock(width: 140, height: 10)
        SkeletonBlock(width: 200, height: 10)
        SkeletonBlock(height: 10)
    }
    .padding(20)
    .frame(width: 300)
    .background(DesignTokens.windowBg)
}
