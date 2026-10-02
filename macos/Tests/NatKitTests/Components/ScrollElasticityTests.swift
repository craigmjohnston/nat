import AppKit
import SwiftUI
import XCTest
@testable import NatKit

/// `ScrollElasticity.disableEverywhere()` really does reach every scroll: a
/// plain `NSScrollView`, one asked for its bounce back, and the one behind a
/// hosted SwiftUI `ScrollView` all come back with their elasticity off.
@MainActor
final class ScrollElasticityTests: XCTestCase {
    override func setUp() {
        super.setUp()
        ScrollElasticity.disableEverywhere()
    }

    /// A scroll view joining a window loses its bounce on both axes.
    func testAScrollViewInAWindowIsInelastic() {
        let scrollView = NSScrollView(frame: NSRect(x: 0, y: 0, width: 200, height: 200))
        let window = NSWindow(contentRect: scrollView.frame, styleMask: [.borderless], backing: .buffered, defer: false)
        window.contentView = scrollView

        XCTAssertEqual(scrollView.verticalScrollElasticity, .none)
        XCTAssertEqual(scrollView.horizontalScrollElasticity, .none)
    }

    /// Asking for the bounce back is held to none.
    func testSettingElasticityIsHeldToNone() {
        let scrollView = NSScrollView(frame: .zero)
        scrollView.verticalScrollElasticity = .allowed
        scrollView.horizontalScrollElasticity = .automatic

        XCTAssertEqual(scrollView.verticalScrollElasticity, .none)
        XCTAssertEqual(scrollView.horizontalScrollElasticity, .none)
    }

    /// Installing twice swaps nothing back.
    func testInstallingTwiceKeepsItOff() {
        ScrollElasticity.disableEverywhere()
        let scrollView = NSScrollView(frame: .zero)
        scrollView.verticalScrollElasticity = .allowed
        XCTAssertEqual(scrollView.verticalScrollElasticity, .none)
    }

    /// Other views moving to a window are untouched by the hook.
    func testOtherViewsStillMoveToWindows() {
        let view = NSView(frame: NSRect(x: 0, y: 0, width: 10, height: 10))
        let window = NSWindow(contentRect: view.frame, styleMask: [.borderless], backing: .buffered, defer: false)
        window.contentView = view
        XCTAssertIdentical(view.window, window)
    }

    /// And end to end: a SwiftUI `ScrollView`, hosted and laid out with no
    /// modifier of ours, leaves AppKit's own scroll with no bounce in it.
    func testAHostedScrollViewEndsUpInelastic() throws {
        let hosting = NSHostingView(
            rootView: ScrollView {
                VStack {
                    ForEach(0..<40, id: \.self) { index in
                        Text("row \(index)")
                    }
                }
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
