import AppKit
import QuartzCore
import SwiftUI
import XCTest
@testable import NatKit

@MainActor
final class PulseTests: XCTestCase {
    /// The design's pulse, exactly: full down to 0.35 and back, 0.9s each
    /// way, eased in and out, forever.
    func testTheAnimationIsTheDesignsPulse() {
        let animation = Pulse.animation()

        XCTAssertEqual(animation.keyPath, "opacity")
        XCTAssertEqual(animation.fromValue as? Double, 1.0)
        XCTAssertEqual(animation.toValue as? Float, 0.35)
        XCTAssertEqual(animation.duration, 0.9)
        XCTAssertTrue(animation.autoreverses)
        XCTAssertEqual(animation.repeatCount, .infinity)
        XCTAssertEqual(animation.timingFunction, CAMediaTimingFunction(name: .easeInEaseOut))
        XCTAssertFalse(animation.isRemovedOnCompletion)
    }

    func testAHostCarriesThePulseOnItsOwnLayerFromTheStart() throws {
        let host = PulseHostView(rootView: Circle().frame(width: 7, height: 7))

        let layer = try XCTUnwrap(host.layer)
        let animation = try XCTUnwrap(layer.animation(forKey: Pulse.animationKey) as? CABasicAnimation)
        XCTAssertEqual(animation.keyPath, "opacity")
        XCTAssertEqual(layer.opacity, 1, "the model value stays full — only the animation dims it")
        XCTAssertTrue(host.hosting.superview === host)
    }

    func testAHostThatLostItsPulseGetsItBackOnLandingInAWindow() throws {
        let host = PulseHostView(rootView: Circle().frame(width: 7, height: 7))
        let layer = try XCTUnwrap(host.layer)
        layer.removeAllAnimations()

        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 50, height: 50),
            styleMask: [.borderless], backing: .buffered, defer: true)
        window.contentView?.addSubview(host)

        XCTAssertNotNil(host.layer?.animation(forKey: Pulse.animationKey))
        XCTAssertEqual(host.layer?.animationKeys()?.count, 1, "re-added once, not stacked")
    }

    /// A pulsing dot is part of its row: a click on it goes past it.
    func testAHostTakesNoClicks() {
        let host = PulseHostView(rootView: Circle().frame(width: 7, height: 7))
        host.frame = NSRect(x: 0, y: 0, width: 7, height: 7)

        XCTAssertNil(host.hitTest(NSPoint(x: 3, y: 3)))
    }

    /// The pulse takes the room its content would have taken unwrapped.
    func testPulsingSizesToItsContent() {
        let hosting = NSHostingView(rootView: Circle().frame(width: 7, height: 7).pulsing())

        XCTAssertEqual(hosting.fittingSize, CGSize(width: 7, height: 7))
    }

    /// The content inside the host draws in the environment it was put in,
    /// not a fresh one.
    func testPulsingCarriesTheEnvironmentIn() {
        let probe = EnvironmentProbe()
        let hosting = NSHostingView(rootView: probe.pulsing().environment(\.pulsesPaused, true))
        hosting.frame = NSRect(x: 0, y: 0, width: 20, height: 20)
        hosting.layoutSubtreeIfNeeded()

        XCTAssertEqual(probe.seen.value, true)
    }

    /// A pulse under a full-size-content window's titlebar — the
    /// breadcrumb's dot — draws where it was laid out, not pushed down
    /// past the titlebar by the window's safe area.
    func testAPulseUnderTheTitlebarDrawsInPlace() throws {
        let probe = FrameProbe()
        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 200, height: 200),
            styleMask: [.titled, .fullSizeContentView], backing: .buffered, defer: false)
        let content = try XCTUnwrap(window.contentView)
        let host = PulseHostView(rootView: probe)
        host.frame = NSRect(x: 10, y: content.bounds.height - 10, width: 7, height: 7)
        content.addSubview(host)
        host.layoutSubtreeIfNeeded()

        // `.global` is the window's, top-down: the host's own top edge.
        let top = content.bounds.height - host.frame.maxY
        XCTAssertEqual(probe.seen.value?.origin, CGPoint(x: 10, y: top))
    }

    // MARK: - Helpers

    private final class Box: @unchecked Sendable { var value: Bool? }
    private final class FrameBox: @unchecked Sendable { var value: CGRect? }

    /// Records where it is drawn in its hosting view.
    private struct FrameProbe: View {
        let seen = FrameBox()

        var body: some View {
            Color.clear.frame(width: 7, height: 7)
                .onGeometryChange(for: CGRect.self) { $0.frame(in: .global) } action: { seen.value = $0 }
        }
    }

    /// Records the `pulsesPaused` it is drawn with.
    private struct EnvironmentProbe: View {
        let seen = Box()
        @Environment(\.pulsesPaused) private var paused

        var body: some View {
            seen.value = paused
            return Color.clear.frame(width: 4, height: 4)
        }
    }
}
