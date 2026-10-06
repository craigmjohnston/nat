import AppKit
import SwiftTerm
import XCTest

/// `FirstLayoutTerminalView.setFrameSize` drops a frame set at the size the
/// view already has, because of two behaviours that are AppKit's and
/// SwiftTerm's rather than gnat's. These pin both against the real thing:
/// should either change, a test here fails and the guard can be reconsidered.
@MainActor
final class SameSizeFrameTests: XCTestCase {
    /// AppKit calls `setFrameSize` for a frame whose size has not changed —
    /// which is what every SwiftUI layout pass over the pane sets.
    func testAppKitCallsSetFrameSizeForAnUnchangedSize() {
        let view = CountingView(frame: NSRect(x: 0, y: 0, width: 100, height: 100))
        view.calls = 0

        view.frame = NSRect(x: 0, y: 0, width: 100, height: 100)
        view.frame = NSRect(x: 5, y: 0, width: 100, height: 100)

        XCTAssertEqual(view.calls, 2, "AppKit stopped calling setFrameSize for an unchanged size")
    }

    /// SwiftTerm marks the whole terminal for redrawing on any
    /// `setFrameSize`, the size unchanged or not.
    func testSwiftTermRedrawsTheWholeViewForAnUnchangedSize() {
        let view = RecordingTerminalView(frame: NSRect(x: 0, y: 0, width: 400, height: 300))
        view.wholeViewRedraws = 0

        view.setFrameSize(view.frame.size)

        XCTAssertEqual(view.wholeViewRedraws, 1, "SwiftTerm no longer redraws on a same-size frame")
    }

    /// Counts whole-view redraw requests. `needsDisplay` itself cannot be
    /// read back here: a view in no window does not keep it.
    private final class RecordingTerminalView: TerminalView {
        var wholeViewRedraws = 0
        override var needsDisplay: Bool {
            get { super.needsDisplay }
            set {
                if newValue { wholeViewRedraws += 1 }
                super.needsDisplay = newValue
            }
        }
    }

    private final class CountingView: NSView {
        var calls = 0
        override func setFrameSize(_ newSize: NSSize) {
            calls += 1
            super.setFrameSize(newSize)
        }
    }
}
