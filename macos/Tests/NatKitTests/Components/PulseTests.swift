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

    // MARK: - Helpers

    private final class Box: @unchecked Sendable { var value: Bool? }

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
