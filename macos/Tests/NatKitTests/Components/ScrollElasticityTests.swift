import AppKit
import SwiftUI
import XCTest
@testable import NatKit

/// `inelastic()` really does reach the scroll it is planted in. A source
/// scan can only say the modifier was written; what matters is that the
/// `NSScrollView` behind a SwiftUI `ScrollView` comes back with its
/// elasticity off, so the seam is hosted and then read.
@MainActor
final class ScrollElasticityTests: XCTestCase {
    /// The modifier's own view, hung inside a real `NSScrollView`: what it
    /// finds by walking up is the scroll it is inside, and both axes go.
    func testItTakesTheElasticityOffTheScrollItIsIn() {
        let scrollView = NSScrollView(frame: NSRect(x: 0, y: 0, width: 200, height: 200))
        let document = NSView(frame: NSRect(x: 0, y: 0, width: 200, height: 600))
        scrollView.documentView = document
        XCTAssertEqual(scrollView.verticalScrollElasticity, .automatic)

        document.addSubview(ElasticityOffView())

        XCTAssertEqual(scrollView.verticalScrollElasticity, .none)
        XCTAssertEqual(scrollView.horizontalScrollElasticity, .none)
    }

    /// A view in no scroll at all is the ordinary case for the first moment
    /// of a layout, and it passes over rather than falling over.
    func testAViewInNoScrollDoesNothing() {
        let loose = NSView(frame: .zero)
        loose.addSubview(ElasticityOffView())
        XCTAssertNil(loose.enclosingScrollView)
    }

    /// And the whole thing end to end: a SwiftUI `ScrollView` whose content
    /// carries `inelastic()`, hosted and laid out, leaves AppKit's own
    /// scroll with no bounce in it.
    func testAHostedScrollViewEndsUpInelastic() throws {
        let hosting = NSHostingView(
            rootView: ScrollView {
                VStack {
                    ForEach(0..<40, id: \.self) { index in
                        Text("row \(index)")
                    }
                }
                .inelastic()
            }
        )
        hosting.frame = NSRect(x: 0, y: 0, width: 200, height: 200)

        let window = NSWindow(
            contentRect: hosting.frame,
            styleMask: [.borderless],
            backing: .buffered,
            defer: false
        )
        window.contentView = hosting
        hosting.layoutSubtreeIfNeeded()

        guard let scrollView = firstScrollView(in: hosting) else {
            throw XCTSkip("SwiftUI drew this ScrollView without an NSScrollView behind it")
        }
        XCTAssertEqual(scrollView.verticalScrollElasticity, .none)
        XCTAssertEqual(scrollView.horizontalScrollElasticity, .none)
    }

    private func firstScrollView(in view: NSView) -> NSScrollView? {
        if let scrollView = view as? NSScrollView { return scrollView }
        for subview in view.subviews {
            if let found = firstScrollView(in: subview) { return found }
        }
        return nil
    }
}
