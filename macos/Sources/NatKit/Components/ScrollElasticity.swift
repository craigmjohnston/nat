import AppKit
import ObjectiveC

/// The rubber band, off — in every scroll in the app. Every scroll here is a
/// list of the user's own work — a plan, a diff, a conversation — and the
/// bounce past either end of one says the list has more to show when it has
/// not, which on a rail of three sections that each scroll within a share of
/// the column is three different lies at once.
///
/// It is AppKit's own switch rather than SwiftUI's `scrollBounceBehavior`,
/// which only settles what an under-full scroll does: a list longer than its
/// box goes on bouncing at both ends whatever that is set to. And it is set
/// on `NSScrollView` itself, for all of them, rather than by a modifier at
/// each call site: a per-site seam has to be planted in the right place (the
/// scroll's content, not the scroll) in every scroll ever written, and a
/// text editor's or a settings form's scroll has no content of ours to plant
/// it in. So every scroll view loses its elasticity as it joins a window,
/// and a later attempt to give it back — SwiftUI re-applying its own
/// defaults on an update — is held to none.
public enum ScrollElasticity {
    /// Swaps the two setters and the move-to-window hook in, once. Called
    /// at launch, before any window — the app's and the gallery's alike.
    @MainActor public static func disableEverywhere() {
        guard !installed else { return }
        installed = true
        swap(#selector(setter: NSScrollView.verticalScrollElasticity),
             #selector(NSScrollView.nat_setVerticalScrollElasticity(_:)))
        swap(#selector(setter: NSScrollView.horizontalScrollElasticity),
             #selector(NSScrollView.nat_setHorizontalScrollElasticity(_:)))
        swap(#selector(NSView.viewDidMoveToWindow),
             #selector(NSScrollView.nat_viewDidMoveToWindow))
    }

    @MainActor private static var installed = false

    @MainActor private static func swap(_ original: Selector, _ replacement: Selector) {
        guard let originalMethod = class_getInstanceMethod(NSScrollView.self, original),
              let replacementMethod = class_getInstanceMethod(NSScrollView.self, replacement) else { return }
        // `viewDidMoveToWindow` is NSView's; added to NSScrollView first, so
        // the swap changes the scroll's own method and no other view's.
        if class_addMethod(
            NSScrollView.self, original,
            method_getImplementation(originalMethod), method_getTypeEncoding(originalMethod)
        ), let added = class_getInstanceMethod(NSScrollView.self, original) {
            method_exchangeImplementations(added, replacementMethod)
        } else {
            method_exchangeImplementations(originalMethod, replacementMethod)
        }
    }
}

extension NSScrollView {
    // After the swap each of these names the original, so the calls below
    // reach AppKit's own implementation rather than recursing.

    @objc fileprivate func nat_setVerticalScrollElasticity(_ elasticity: NSScrollView.Elasticity) {
        nat_setVerticalScrollElasticity(.none)
    }

    @objc fileprivate func nat_setHorizontalScrollElasticity(_ elasticity: NSScrollView.Elasticity) {
        nat_setHorizontalScrollElasticity(.none)
    }

    /// Both ways round: a scroll that can only move vertically still
    /// stretches horizontally under a trackpad's sideways drift.
    @objc fileprivate func nat_viewDidMoveToWindow() {
        nat_viewDidMoveToWindow()
        verticalScrollElasticity = .none
        horizontalScrollElasticity = .none
    }
}
