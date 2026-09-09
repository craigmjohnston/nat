import AppKit
import SwiftUI
import XCTest
@testable import NatKit

final class TrafficLightAlignerTests: XCTestCase {
    private let headerHeight: CGFloat = 40

    // MARK: - The geometry rule

    func testOriginCentresTheButtonAndMatchesTheLeadingInsetToTheVerticalOne() {
        let close = NSRect(x: 7, y: 6, width: 14, height: 16)
        let origin = TrafficLightAlignment.origin(
            frame: close,
            offsetFromClose: 0,
            headerHeight: 40,
            superviewHeight: 28,
            flipped: true
        )

        // (40 - 16) / 2 = 12 all round.
        XCTAssertEqual(origin.x, 12)
        XCTAssertEqual(origin.y, 12)
    }

    func testOriginKeepsEachButtonsPitchFromTheCloseButton() {
        let zoom = NSRect(x: 47, y: 6, width: 14, height: 16)
        let origin = TrafficLightAlignment.origin(
            frame: zoom,
            offsetFromClose: 40,
            headerHeight: 40,
            superviewHeight: 28,
            flipped: true
        )

        XCTAssertEqual(origin.x, 12 + 40)
    }

    func testOriginInAnUnflippedSuperviewMeasuresTheInsetFromTheTop() {
        let close = NSRect(x: 7, y: 6, width: 14, height: 16)
        let origin = TrafficLightAlignment.origin(
            frame: close,
            offsetFromClose: 0,
            headerHeight: 40,
            superviewHeight: 28,
            flipped: false
        )

        // 12 from the superview's top edge: 28 - 12 - 16 = 0.
        XCTAssertEqual(origin.y, 0)
    }

    // MARK: - Against a real window

    @MainActor
    private func makeAlignedWindow() -> (NSWindow, TrafficLightAlignerNSView) {
        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 600, height: 400),
            styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView],
            backing: .buffered,
            defer: false
        )
        window.titlebarAppearsTransparent = true
        let aligner = TrafficLightAlignerNSView()
        aligner.headerHeight = headerHeight
        window.contentView?.addSubview(aligner)
        return (window, aligner)
    }

    /// A button's frame in the window's own coordinates, where the top of the
    /// window is y = frame.height — the space the header band is measured in.
    @MainActor
    private func windowFrame(of button: NSView) -> NSRect {
        button.superview!.convert(button.frame, to: nil)
    }

    @MainActor
    func testMountingCentresEveryButtonInTheHeaderBand() throws {
        let (window, _) = makeAlignedWindow()

        for type in [NSWindow.ButtonType.closeButton, .miniaturizeButton, .zoomButton] {
            let button = try XCTUnwrap(window.standardWindowButton(type))
            let frame = windowFrame(of: button)
            XCTAssertEqual(frame.midY, window.frame.height - headerHeight / 2,
                           "\(type) should be vertically centred in the header")
        }
    }

    @MainActor
    func testMountingGivesTheCloseButtonALeadingInsetEqualToTheVerticalOne() throws {
        let (window, _) = makeAlignedWindow()

        let close = try XCTUnwrap(window.standardWindowButton(.closeButton))
        let frame = windowFrame(of: close)
        XCTAssertEqual(frame.minX, (headerHeight - frame.height) / 2)
    }

    @MainActor
    func testMountingKeepsThePitchBetweenButtons() throws {
        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 600, height: 400),
            styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView],
            backing: .buffered,
            defer: false
        )
        let close = try XCTUnwrap(window.standardWindowButton(.closeButton))
        let mini = try XCTUnwrap(window.standardWindowButton(.miniaturizeButton))
        let zoom = try XCTUnwrap(window.standardWindowButton(.zoomButton))
        let pitches = [mini.frame.minX - close.frame.minX, zoom.frame.minX - mini.frame.minX]

        let aligner = TrafficLightAlignerNSView()
        aligner.headerHeight = headerHeight
        window.contentView?.addSubview(aligner)

        XCTAssertEqual(mini.frame.minX - close.frame.minX, pitches[0])
        XCTAssertEqual(zoom.frame.minX - mini.frame.minX, pitches[1])
    }

    /// AppKit putting a button back where it wants it is the reset the
    /// aligner listens for: moving one by hand posts the same frame
    /// notification, and the aligner should put it straight back.
    @MainActor
    func testAButtonMovedByAppKitIsRealigned() throws {
        let (window, _) = makeAlignedWindow()
        let close = try XCTUnwrap(window.standardWindowButton(.closeButton))
        let aligned = close.frame.origin

        close.setFrameOrigin(NSPoint(x: 7, y: 6))

        XCTAssertEqual(close.frame.origin, aligned)
    }

    /// The pass AppKit's reset triggers runs while the other buttons are
    /// still where the aligner put them: deriving their targets from the
    /// close button's just-reset frame would smear its offset into their
    /// pitch, one pass at a time.
    @MainActor
    func testAppKitResettingTheCloseButtonAloneLeavesTheOthersInPlace() throws {
        let (window, _) = makeAlignedWindow()
        let close = try XCTUnwrap(window.standardWindowButton(.closeButton))
        let mini = try XCTUnwrap(window.standardWindowButton(.miniaturizeButton))
        let zoom = try XCTUnwrap(window.standardWindowButton(.zoomButton))
        let alignedMini = mini.frame.origin
        let alignedZoom = zoom.frame.origin

        close.setFrameOrigin(NSPoint(x: 7, y: 6))

        XCTAssertEqual(mini.frame.origin, alignedMini)
        XCTAssertEqual(zoom.frame.origin, alignedZoom)
    }

    /// The mirror case: AppKit resets one of the outer buttons while the
    /// close button is still aligned. Measured against the aligned close
    /// frame, the reset x reads as the button's own offset and it stays put.
    @MainActor
    func testAppKitResettingTheZoomButtonAloneIsRealigned() throws {
        let (window, _) = makeAlignedWindow()
        let zoom = try XCTUnwrap(window.standardWindowButton(.zoomButton))
        let aligned = zoom.frame.origin

        zoom.setFrameOrigin(NSPoint(x: 47, y: 6))

        XCTAssertEqual(zoom.frame.origin, aligned)
    }

    /// A relayout that rebuilds the buttons moves views no frame observer is
    /// attached to, so the window's own notifications are the fallback that
    /// still triggers a pass. Simulated by silencing the button's frame
    /// notifications before knocking it out of place.
    @MainActor
    func testAWindowResizeRealignsAButtonWhoseNotificationsNeverFired() throws {
        let (window, _) = makeAlignedWindow()
        let zoom = try XCTUnwrap(window.standardWindowButton(.zoomButton))
        let aligned = zoom.frame.origin

        zoom.postsFrameChangedNotifications = false
        zoom.setFrameOrigin(NSPoint(x: 47, y: 6))
        XCTAssertNotEqual(zoom.frame.origin, aligned)
        NotificationCenter.default.post(name: NSWindow.didResizeNotification, object: window)

        XCTAssertEqual(zoom.frame.origin, aligned)
    }

    /// The style mask alone, since inserting `.fullScreen` into a real
    /// window's mask is refused by AppKit outside an actual transition.
    @MainActor
    func testFullScreenLeavesTheButtonsToTheSystem() {
        XCTAssertFalse(TrafficLightAlignerNSView.shouldAlign(styleMask: [.titled, .fullScreen]))
        XCTAssertTrue(TrafficLightAlignerNSView.shouldAlign(styleMask: [.titled, .fullSizeContentView]))
    }

    @MainActor
    func testInvisibleToHitTesting() {
        let aligner = TrafficLightAlignerNSView()
        aligner.frame = NSRect(x: 0, y: 0, width: 100, height: 100)

        XCTAssertNil(aligner.hitTest(NSPoint(x: 50, y: 50)))
    }

    /// `NSViewRepresentableContext` cannot be constructed directly, so the
    /// representable is exercised the way SwiftUI itself does it: mounted in
    /// a hosting view, then the aligner found in the AppKit hierarchy it
    /// produced.
    @MainActor
    func testRepresentableMountsTheAlignerWithItsHeaderHeight() {
        let hosting = NSHostingView(rootView: TrafficLightAlignerView(headerHeight: 40))
        hosting.frame = NSRect(x: 0, y: 0, width: 100, height: 100)
        hosting.layoutSubtreeIfNeeded()

        let aligner = findAligner(in: hosting)
        XCTAssertEqual(aligner?.headerHeight, 40)
    }

    /// `updateNSView` is exercised the way SwiftUI itself drives it — a new
    /// root view on the hosting view — since the representable context has
    /// no public initializer.
    @MainActor
    func testANewHeaderHeightIsReappliedThroughUpdate() throws {
        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 600, height: 400),
            styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView],
            backing: .buffered,
            defer: false
        )
        let hosting = NSHostingView(rootView: TrafficLightAlignerView(headerHeight: 40))
        window.contentView = hosting
        hosting.layoutSubtreeIfNeeded()
        let close = try XCTUnwrap(window.standardWindowButton(.closeButton))

        hosting.rootView = TrafficLightAlignerView(headerHeight: 60)
        hosting.layoutSubtreeIfNeeded()

        XCTAssertEqual(findAligner(in: hosting)?.headerHeight, 60)
        XCTAssertEqual(windowFrame(of: close).midY, window.frame.height - 30)
    }

    @MainActor
    func testAnUpdateBeforeAWindowExistsOnlyRecordsTheHeight() {
        let hosting = NSHostingView(rootView: TrafficLightAlignerView(headerHeight: 40))
        hosting.frame = NSRect(x: 0, y: 0, width: 100, height: 100)
        hosting.layoutSubtreeIfNeeded()

        hosting.rootView = TrafficLightAlignerView(headerHeight: 60)
        hosting.layoutSubtreeIfNeeded()

        XCTAssertEqual(findAligner(in: hosting)?.headerHeight, 60)
    }

    @MainActor
    private func findAligner(in view: NSView) -> TrafficLightAlignerNSView? {
        if let aligner = view as? TrafficLightAlignerNSView { return aligner }
        for subview in view.subviews {
            if let aligner = findAligner(in: subview) { return aligner }
        }
        return nil
    }
}
