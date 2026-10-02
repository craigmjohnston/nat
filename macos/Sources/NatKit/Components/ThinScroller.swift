import AppKit

/// The app's scroll bar: a thin capsule of a knob and nothing else — no
/// slot, no track, no border — at a fixed width under either scroller style,
/// so hovering never swells it. Always the overlay style (see
/// `ElasticityOffView`), so it takes no gutter of its own: it is drawn in the
/// content's own right padding, which is wider than it.
final class ThinScroller: NSScroller {
    /// The scroller's whole width — the knob plus the air either side of it.
    static let width: CGFloat = 8
    /// The air between the knob and the scroller's edges.
    static let knobInset: CGFloat = 2

    override class var isCompatibleWithOverlayScrollers: Bool { true }

    override class func scrollerWidth(
        for controlSize: NSControl.ControlSize, scrollerStyle: NSScroller.Style
    ) -> CGFloat {
        width
    }

    override func drawKnobSlot(in slotRect: NSRect, highlight flag: Bool) {}

    override func drawKnob() {
        let knob = rect(for: .knob).insetBy(dx: Self.knobInset, dy: Self.knobInset)
        guard knob.width > 0, knob.height > 0 else { return }
        let radius = min(knob.width, knob.height) / 2
        DesignTokens.scrollerKnob.setFill()
        NSBezierPath(roundedRect: knob, xRadius: radius, yRadius: radius).fill()
    }
}
