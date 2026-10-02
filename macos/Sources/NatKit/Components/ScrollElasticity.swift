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
///
/// Being the one seam every scroll in the app plants, it also puts the app's
/// own scroll bar on it — `ThinScroller`, in the overlay style whatever the
/// system's Show scroll bars setting says, so the bar sits in the content's
/// right padding rather than reserving a gutter beside it.
struct NoScrollElasticity: NSViewRepresentable {
    func makeNSView(context: Context) -> NSView { ElasticityOffView() }

    func updateNSView(_ nsView: NSView, context: Context) {
        (nsView as? ElasticityOffView)?.applyToEnclosingScrollView()
    }
}

final class ElasticityOffView: NSView {
    // Unsafe only so deinit can read it: it is written on the main thread alone.
    nonisolated(unsafe) private var styleObserver: NSObjectProtocol?
    /// Watches the scroll's own style, which AppKit sets back to the
    /// system's whenever it decides afresh what that is — not only on the
    /// public notification, and in the running app evidently after this
    /// view's own pass (a mouse in use makes the system's style the legacy
    /// one, which reserves a gutter beside the content).
    private var styleWatch: NSKeyValueObservation?
    private weak var watchedScrollView: NSScrollView?

    deinit {
        if let styleObserver { NotificationCenter.default.removeObserver(styleObserver) }
    }

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
        applyScrollers(to: scrollView)
        // AppKit puts every scroll back to the system's preferred style when
        // that setting (or the pointing device) changes; put ours back after.
        if styleObserver == nil {
            styleObserver = NotificationCenter.default.addObserver(
                forName: NSScroller.preferredScrollerStyleDidChangeNotification, object: nil, queue: nil
            ) { [weak self] _ in
                MainActor.assumeIsolated {
                    guard let self, let scrollView = self.enclosingScrollView else { return }
                    self.applyScrollers(to: scrollView)
                }
            }
        }
    }

    func applyScrollers(to scrollView: NSScrollView) {
        if !(scrollView.verticalScroller is ThinScroller) { scrollView.verticalScroller = ThinScroller() }
        if !(scrollView.horizontalScroller is ThinScroller) { scrollView.horizontalScroller = ThinScroller() }
        scrollView.scrollerStyle = .overlay
        guard watchedScrollView !== scrollView else { return }
        watchedScrollView = scrollView
        styleWatch = scrollView.observe(\.scrollerStyle, options: [.new]) { [weak self] scrollView, change in
            guard change.newValue != .overlay else { return }
            // After the setter that changed it has finished, not inside it.
            DispatchQueue.main.async {
                MainActor.assumeIsolated {
                    guard let self, self.enclosingScrollView === scrollView else { return }
                    self.applyScrollers(to: scrollView)
                }
            }
        }
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
