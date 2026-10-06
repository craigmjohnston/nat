import AppKit
import QuartzCore
import SwiftUI

/// The design's pulse: down to a third and back, slowly, forever — 0.9s each
/// way, eased in and out — run by the render server, not by SwiftUI.
///
/// Two ways of drawing it in SwiftUI were tried and both cost gnat a
/// SwiftUI update and a Core Animation commit every display frame, up to
/// 120 a second, for as long as any agent was live — a third of a core in
/// gnat and a quarter of one in WindowServer, idle, by the profiler. A
/// `repeatForever` animation was the first, and it had a second fault: it
/// rode the transaction any move of the view's frame landed in, so a dot
/// its parent re-laid — the breadcrumb's, as its measured widths arrived —
/// slid back and forth between the two places forever. `TimelineView`
/// computing each frame's opacity fixed the slide and kept the cost.
///
/// So the pulse is a `CABasicAnimation` on the opacity of a layer the pulse
/// owns (`PulseHostView`'s), added once: the render server interpolates it
/// with no work from gnat at all, and since it animates only `opacity` the
/// layer's place stays whatever SwiftUI's layout says — no frame move can
/// ride it.
public enum Pulse {
    /// One way of the pulse, in seconds.
    public static let halfPeriod: CFTimeInterval = 0.9
    /// The opacity the pulse comes down to.
    public static let floor: Float = 0.35
    /// The key the animation is added under, and checked for.
    public static let animationKey = "gnat.pulse"

    /// Full to `floor` and back over `2 × halfPeriod`, eased in and out each
    /// way, forever. Not removed on completion, which it never reaches, so
    /// nothing short of the layer going takes it off.
    public static func animation() -> CABasicAnimation {
        let animation = CABasicAnimation(keyPath: "opacity")
        animation.fromValue = 1.0
        animation.toValue = floor
        animation.duration = halfPeriod
        animation.autoreverses = true
        animation.repeatCount = .infinity
        animation.timingFunction = CAMediaTimingFunction(name: .easeInEaseOut)
        animation.isRemovedOnCompletion = false
        return animation
    }
}

/// Draws `content` pulsing: the content in an `NSHostingView` inside a view
/// whose own layer carries `Pulse.animation()`.
///
/// A host for the content rather than an animation laid beside it: the
/// opacity has to be the content's own, and the only layer SwiftUI hands
/// out to put an animation on is a platform view's, whose opacity reaches
/// nothing but what that view itself draws. Hosting the content costs one
/// `NSHostingView` a
/// live agent, which is a handful at most; in exchange the dot and the
/// workshop's glyph both pulse through this one path, whatever they draw.
/// The window's environment goes in with the content (`updateNSView`), so a
/// palette's ink resolves inside as it would outside.
public struct Pulsing<Content: View>: NSViewRepresentable {
    let content: Content

    public init(_ content: Content) {
        self.content = content
    }

    public func makeNSView(context: Context) -> PulseHostView<PulseRoot<Content>> {
        PulseHostView(rootView: PulseRoot(content: content, environment: context.environment))
    }

    public func updateNSView(_ nsView: PulseHostView<PulseRoot<Content>>, context: Context) {
        nsView.hosting.rootView = PulseRoot(content: content, environment: context.environment)
    }

    /// The content's own size, so the pulse takes exactly the room the
    /// content would have taken unwrapped.
    public func sizeThatFits(
        _ proposal: ProposedViewSize, nsView: PulseHostView<PulseRoot<Content>>, context: Context
    ) -> CGSize? {
        nsView.hosting.intrinsicContentSize
    }
}

/// `Pulsing`'s content with the environment it was drawn in — an
/// `NSHostingView` starts from a fresh one otherwise.
public struct PulseRoot<Content: View>: View {
    let content: Content
    let environment: EnvironmentValues

    public var body: some View {
        content.environment(\.self, environment)
    }
}

extension View {
    /// This view, pulsing (`Pulse`).
    public func pulsing() -> some View {
        Pulsing(self).allowsHitTesting(false)
    }
}

/// The layer `Pulse.animation()` runs on, with the content hosted inside.
///
/// The animation is re-added whenever the view lands in a window without it,
/// should AppKit have dropped it in a move, and the view takes no clicks: a
/// pulsing dot in a row is part of the row, so a click on it goes on to the
/// row's own SwiftUI gestures.
public final class PulseHostView<Root: View>: NSView {
    public let hosting: NSHostingView<Root>

    public init(rootView: Root) {
        hosting = NSHostingView(rootView: rootView)
        super.init(frame: .zero)
        wantsLayer = true
        // Sized by this view's frame, never by constraints of its own (the
        // min/max options add required ones that would fight the autoresizing
        // mask): the intrinsic size is only read, by `Pulsing.sizeThatFits`.
        hosting.sizingOptions = [.intrinsicContentSize]
        // No safe area: a hosting view under a full-size-content titlebar
        // (the breadcrumb's dot) is otherwise inset by the titlebar's height
        // and draws its content that far below where it was laid out.
        hosting.safeAreaRegions = []
        hosting.autoresizingMask = [.width, .height]
        hosting.frame = bounds
        addSubview(hosting)
        ensurePulse()
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) {
        fatalError("PulseHostView is made in code")
    }

    override public func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        ensurePulse()
    }

    override public func hitTest(_ point: NSPoint) -> NSView? {
        nil
    }

    func ensurePulse() {
        guard let layer, layer.animation(forKey: Pulse.animationKey) == nil else { return }
        layer.add(Pulse.animation(), forKey: Pulse.animationKey)
    }
}
