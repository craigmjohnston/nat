import AppKit
import SwiftUI

/// Where a scroll's knob sits along its track, on one axis: what
/// `thinScrollers()` draws, worked out apart from the drawing so it can be
/// held to numbers.
public struct ScrollKnob: Equatable, Sendable {
    /// The knob's start and length along the axis, in the scroll's own
    /// coordinates.
    public let offset: CGFloat
    public let length: CGFloat

    /// The knob's thickness across the axis, and the air between it and the
    /// scroll's edges.
    public static let thickness: CGFloat = 4
    public static let inset: CGFloat = 2
    /// The shortest a knob gets, however long the content.
    public static let minLength: CGFloat = 24

    /// The knob for a scroll `visible` long over content `content` long,
    /// scrolled `scrolled` along it — nil when everything fits, since there
    /// is then nothing to scroll and nothing to draw.
    public init?(visible: CGFloat, content: CGFloat, scrolled: CGFloat) {
        guard visible > 0, content > visible + 0.5 else { return nil }
        let track = visible - 2 * Self.inset
        let length = min(track, max(Self.minLength, track * visible / content))
        let progress = min(max(scrolled / (content - visible), 0), 1)
        self.offset = Self.inset + (track - length) * progress
        self.length = length
    }

    /// How far a drag of `distance` along the track moves the content.
    public static func contentDistance(
        forDrag distance: CGFloat, visible: CGFloat, content: CGFloat, knob: ScrollKnob
    ) -> CGFloat {
        let travel = visible - 2 * inset - knob.length
        guard travel > 0 else { return 0 }
        return distance * (content - visible) / travel
    }
}

public extension View {
    /// Applied to a `ScrollView`: the app's own scroll bar in place of
    /// AppKit's. A thin capsule drawn in SwiftUI, over the content's own
    /// trailing padding, which every scrolled list in the app leaves wider than
    /// the knob — so the space it needs is always held, whether the content
    /// scrolls or not, and nothing moves when it starts or stops scrolling.
    ///
    /// AppKit's scroller cannot be made to do this. Its on-screen drawing
    /// goes through its own layers, not a subclass's `drawKnob`, so a thinner
    /// scroller is drawn as the system's wide knob cropped to the thinner
    /// strip; and in the legacy style (the system's choice with a mouse in
    /// use, or Show scroll bars set to Always) it takes a gutter beside the
    /// content only while there is something to scroll.
    ///
    /// The knob follows the system's Show scroll bars the way AppKit's does:
    /// always there while the content overflows under the legacy style, and
    /// otherwise shown on scrolling or hover and faded out after. It can be
    /// dragged. `position` is the scroll's own binding where it already has
    /// one; without it the modifier keeps its own.
    func thinScrollers(_ axes: Axis.Set = .vertical, position: Binding<ScrollPosition>? = nil) -> some View {
        modifier(ThinScrollers(axes: axes, external: position))
    }
}

private struct ThinScrollers: ViewModifier {
    let axes: Axis.Set
    let external: Binding<ScrollPosition>?

    @State private var own = ScrollPosition()
    @State private var geometry: ScrollGeometry?
    @State private var lit = false
    @State private var hovering = false
    @State private var dragging = false
    @State private var dragOrigin: CGPoint?
    @State private var fade: Task<Void, Never>?
    @State private var legacy = NSScroller.preferredScrollerStyle == .legacy

    /// How long a knob stays up after the last scroll, in the overlay style.
    private static let linger = Duration.milliseconds(900)

    private var position: Binding<ScrollPosition> { external ?? $own }

    func body(content: Content) -> some View {
        content
            .scrollIndicators(.never)
            .modifier(OwnPosition(position: external == nil ? $own : nil))
            .onScrollGeometryChange(for: ScrollGeometry.self, of: { $0 }) { old, new in
                geometry = new
                if old.contentOffset != new.contentOffset { light() }
            }
            .onHover { inside in
                hovering = inside
                if inside { light() }
            }
            .onReceive(NotificationCenter.default.publisher(for: NSScroller.preferredScrollerStyleDidChangeNotification)) { _ in
                legacy = NSScroller.preferredScrollerStyle == .legacy
            }
            .overlay { knobs }
    }

    @ViewBuilder
    private var knobs: some View {
        if let geometry {
            ZStack(alignment: .topLeading) {
                if axes.contains(.vertical), let knob = verticalKnob(geometry) {
                    knobView(along: .vertical, knob, geometry)
                }
                if axes.contains(.horizontal), let knob = horizontalKnob(geometry) {
                    knobView(along: .horizontal, knob, geometry)
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
            .opacity(legacy || lit || dragging ? 1 : 0)
            .animation(.easeOut(duration: 0.2), value: legacy || lit || dragging)
        }
    }

    private func verticalKnob(_ g: ScrollGeometry) -> ScrollKnob? {
        ScrollKnob(visible: g.containerSize.height, content: contentLength(g, .vertical), scrolled: scrolled(g, .vertical))
    }

    private func horizontalKnob(_ g: ScrollGeometry) -> ScrollKnob? {
        ScrollKnob(visible: g.containerSize.width, content: contentLength(g, .horizontal), scrolled: scrolled(g, .horizontal))
    }

    private func contentLength(_ g: ScrollGeometry, _ axis: Axis) -> CGFloat {
        axis == .vertical
            ? g.contentSize.height + g.contentInsets.top + g.contentInsets.bottom
            : g.contentSize.width + g.contentInsets.leading + g.contentInsets.trailing
    }

    private func scrolled(_ g: ScrollGeometry, _ axis: Axis) -> CGFloat {
        axis == .vertical ? g.contentOffset.y + g.contentInsets.top : g.contentOffset.x + g.contentInsets.leading
    }

    private func knobView(along axis: Axis, _ knob: ScrollKnob, _ g: ScrollGeometry) -> some View {
        let vertical = axis == .vertical
        // The hit area is the whole strip the knob runs in, so a 4pt knob is
        // not a 4pt target.
        let strip = ScrollKnob.thickness + 2 * ScrollKnob.inset
        return Capsule()
            .fill(DesignTokens.scrollerKnob)
            .frame(width: vertical ? ScrollKnob.thickness : knob.length,
                   height: vertical ? knob.length : ScrollKnob.thickness)
            .padding(ScrollKnob.inset)
            .frame(width: vertical ? strip : knob.length + 2 * ScrollKnob.inset,
                   height: vertical ? knob.length + 2 * ScrollKnob.inset : strip)
            .contentShape(Rectangle())
            .offset(x: vertical ? g.containerSize.width - strip : knob.offset - ScrollKnob.inset,
                    y: vertical ? knob.offset - ScrollKnob.inset : g.containerSize.height - strip)
            .gesture(
                DragGesture(minimumDistance: 0)
                    .onChanged { drag in
                        dragging = true
                        let origin = dragOrigin ?? g.contentOffset
                        if dragOrigin == nil { dragOrigin = origin }
                        let moved = ScrollKnob.contentDistance(
                            forDrag: vertical ? drag.translation.height : drag.translation.width,
                            visible: vertical ? g.containerSize.height : g.containerSize.width,
                            content: contentLength(g, axis), knob: knob)
                        var snap = Transaction()
                        snap.disablesAnimations = true
                        withTransaction(snap) {
                            if vertical {
                                position.wrappedValue.scrollTo(y: max(0, origin.y + moved))
                            } else {
                                position.wrappedValue.scrollTo(x: max(0, origin.x + moved))
                            }
                        }
                    }
                    .onEnded { _ in
                        dragging = false
                        dragOrigin = nil
                        light()
                    }
            )
    }

    /// Shows the knob, and in the overlay style fades it after a pause.
    private func light() {
        lit = true
        fade?.cancel()
        fade = Task { @MainActor in
            try? await Task.sleep(for: Self.linger)
            guard !Task.isCancelled, !hovering, !dragging else { return }
            lit = false
        }
    }
}

/// The modifier's own scroll position, where the scroll has none of its own
/// — a second `.scrollPosition` on a scroll that already has one would
/// override it.
private struct OwnPosition: ViewModifier {
    let position: Binding<ScrollPosition>?

    func body(content: Content) -> some View {
        if let position {
            content.scrollPosition(position)
        } else {
            content
        }
    }
}
