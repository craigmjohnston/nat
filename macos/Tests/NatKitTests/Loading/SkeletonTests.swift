import AppKit
import SwiftUI
import XCTest
@testable import NatKit

final class SkeletonTests: XCTestCase {
    func testHighlightStartsClearOfTheLeadingEdge() {
        XCTAssertEqual(Skeleton.highlightOffset(phase: 0, width: 100), -100)
    }

    func testHighlightEndsClearOfTheTrailingEdge() {
        XCTAssertEqual(Skeleton.highlightOffset(phase: 1, width: 100), 100)
    }

    func testHighlightCrossesTheBlockAtTheMiddleOfTheCycle() {
        XCTAssertEqual(Skeleton.highlightOffset(phase: 0.5, width: 100), 0)
    }

    func testHighlightScalesWithTheBlockItCrosses() {
        XCTAssertEqual(Skeleton.highlightOffset(phase: 0.25, width: 40), -20)
    }

    func testSweepRepeatsWhenMotionIsAllowed() {
        XCTAssertEqual(Skeleton.sweep(reduceMotion: false),
                       .linear(duration: Skeleton.cycle).repeatForever(autoreverses: false))
    }

    func testSweepIsNoAnimationAtAllUnderReducedMotion() {
        XCTAssertNil(Skeleton.sweep(reduceMotion: true))
    }

    func testHighlightIsNarrowerThanTheBlockItCrosses() {
        XCTAssertGreaterThan(Skeleton.highlightWidthRatio, 0)
        XCTAssertLessThan(Skeleton.highlightWidthRatio, 1)
    }

    /// The block is a view with no output to assert on, so it is exercised
    /// the way SwiftUI itself does it: mounted, laid out, and checked for
    /// the size its caller framed it at — which is the whole contract a
    /// skeleton has, since standing in for content at exactly its geometry
    /// is the point.
    @MainActor
    func testBlockTakesTheSizeItIsGiven() {
        let hosting = NSHostingView(rootView: SkeletonBlock(width: 120, height: 9))
        hosting.layoutSubtreeIfNeeded()

        XCTAssertEqual(hosting.intrinsicContentSize.width, 120, accuracy: 0.5)
        XCTAssertEqual(hosting.intrinsicContentSize.height, 9, accuracy: 0.5)
    }

    @MainActor
    func testBlockWithNoWidthFillsWhatItIsFramedIn() {
        let hosting = NSHostingView(
            rootView: SkeletonBlock(height: 9).frame(width: 200, height: 9)
        )
        hosting.layoutSubtreeIfNeeded()

        XCTAssertEqual(hosting.intrinsicContentSize.width, 200, accuracy: 0.5)
    }
}
