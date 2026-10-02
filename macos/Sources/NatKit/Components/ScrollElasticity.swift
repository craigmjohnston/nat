import AppKit
import SwiftUI

/// The rubber band, off. Every scroll in the app is a list of the user's own
/// work — a plan, a diff, a conversation — and the bounce past either end of
/// one says the list has more to show when it has not, which on a rail of
/// three sections that each scroll within a share of the column is three
/// different lies at once.
///
/// It is AppKit's own switch rather than SwiftUI's `scrollBounceBehavior`,
/// which only settles what an under-full scroll does: a list longer than its
/// box goes on bouncing at both ends whatever that is set to. The seam is a
/// zero-sized view planted in the scroll's *content*, since that is what is
/// inside the `NSScrollView` and so what can find it — a view hung off the
/// `ScrollView` itself would walk up past it and find whatever scroll the
/// whole thing happens to sit in.
struct NoScrollElasticity: NSViewRepresentable {
    func makeNSView(context: Context) -> NSView { ElasticityOffView() }

    func updateNSView(_ nsView: NSView, context: Context) {
        (nsView as? ElasticityOffView)?.applyToEnclosingScrollView()
    }
}

final class ElasticityOffView: NSView {
    override func viewDidMoveToSuperview() {
        super.viewDidMoveToSuperview()
        applyToEnclosingScrollView()
    }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        applyToEnclosingScrollView()
    }

    /// Both ways round: a scroll that can only move vertically still
    /// stretches horizontally under a trackpad's sideways drift.
    func applyToEnclosingScrollView() {
        guard let scrollView = enclosingScrollView else { return }
        scrollView.verticalScrollElasticity = .none
        scrollView.horizontalScrollElasticity = .none
    }
}

public extension View {
    /// Applied to a `ScrollView`'s content — not to the `ScrollView` — this
    /// takes the elasticity off the scroll it is inside. See
    /// `NoScrollElasticity`.
    func inelastic() -> some View {
        background(NoScrollElasticity().frame(width: 0, height: 0))
    }
}
