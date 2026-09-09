import AppKit
import SwiftUI

/// Recenters the traffic lights in the shell's custom title bar. With the
/// system title bar hidden the header row is the title bar, but macOS goes on
/// laying the close/minimize/zoom buttons out for the standard bar's height —
/// above the centre of a taller header, and at the standard leading inset
/// rather than one that matches. Mounted (invisibly) in the shell, this view
/// moves them so each button is vertically centred in the header band and the
/// leading inset equals the vertical one: even space all round.
public struct TrafficLightAlignerView: NSViewRepresentable {
    let headerHeight: CGFloat

    public init(headerHeight: CGFloat) {
        self.headerHeight = headerHeight
    }

    public func makeNSView(context: Context) -> TrafficLightAlignerNSView {
        let view = TrafficLightAlignerNSView()
        view.headerHeight = headerHeight
        return view
    }

    public func updateNSView(_ nsView: TrafficLightAlignerNSView, context: Context) {
        nsView.headerHeight = headerHeight
        if let window = nsView.window {
            nsView.align(window)
        }
    }
}

/// The geometry alone, kept pure so the placement rule is testable without a
/// window: the inset that centres a button's height in the header band is
/// applied on the leading side too, and each button sits at the offset from
/// the close button that AppKit's own layout gave it. The offset is a number
/// captured once rather than a frame read live, because an align pass can run
/// mid-way through AppKit's own relayout — the close button reset, the others
/// still where the last pass put them — and offsets measured across that
/// mixed state smear the reset into the pitch, one pass at a time.
enum TrafficLightAlignment {
    static func origin(
        frame: NSRect,
        offsetFromClose: CGFloat,
        headerHeight: CGFloat,
        superviewHeight: CGFloat,
        flipped: Bool
    ) -> NSPoint {
        let inset = (headerHeight - frame.height) / 2
        let x = inset + offsetFromClose
        let y = flipped ? inset : superviewHeight - inset - frame.height
        return NSPoint(x: x, y: y)
    }
}

/// Internal rather than private so the tests can reach it. AppKit re-lays the
/// buttons out on occasions of its own — a resize, a return from full screen —
/// and every one of them moves a button, so the buttons' own frame
/// notifications are the main signal; the window's resize and full-screen-exit
/// notifications back them up, for a relayout that rebuilds the buttons and so
/// moves views no observer is attached to. Re-applying is idempotent, an
/// already-aligned button being given the origin it has.
public final class TrafficLightAlignerNSView: NSView {
    var headerHeight: CGFloat = 0
    private var aligning = false

    /// The close button the current offsets and observers belong to. AppKit
    /// replacing the buttons is detected by this identity changing, and a
    /// fresh set is adopted — offsets recaptured, observers moved over.
    private var adoptedClose: ObjectIdentifier?
    private var offsets: [NSWindow.ButtonType: CGFloat] = [:]

    private static let buttonTypes: [NSWindow.ButtonType] = [
        .closeButton, .miniaturizeButton, .zoomButton,
    ]

    override public func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        NotificationCenter.default.removeObserver(self)
        adoptedClose = nil
        guard let window else { return }
        align(window)
    }

    @objc private func buttonFrameDidChange() {
        guard let window else { return }
        align(window)
    }

    @objc private func windowDidRelayout() {
        guard let window else { return }
        align(window)
    }

    /// Captures each button's offset from the close button and observes its
    /// frame, once per set of buttons: on first sight the frames are AppKit's
    /// own, and after that alignment preserves the offsets, so what is read
    /// here is always the pitch AppKit chose.
    private func adoptButtonsIfNeeded(_ window: NSWindow, close: NSView) {
        guard adoptedClose != ObjectIdentifier(close) else { return }
        // A previous set's observers point at buttons no longer in the
        // window; everything, the window's own notifications included, is
        // re-registered here so one place owns the wiring.
        NotificationCenter.default.removeObserver(self)
        adoptedClose = ObjectIdentifier(close)
        for name in [NSWindow.didResizeNotification, NSWindow.didExitFullScreenNotification] {
            NotificationCenter.default.addObserver(
                self,
                selector: #selector(windowDidRelayout),
                name: name,
                object: window
            )
        }
        offsets = [:]
        for type in Self.buttonTypes {
            guard let button = window.standardWindowButton(type) else { continue }
            offsets[type] = button.frame.minX - close.frame.minX
            button.postsFrameChangedNotifications = true
            NotificationCenter.default.addObserver(
                self,
                selector: #selector(buttonFrameDidChange),
                name: NSView.frameDidChangeNotification,
                object: button
            )
        }
    }

    func align(_ window: NSWindow) {
        // Our own setFrameOrigin posts the very notification that re-enters
        // here; the guard keeps one pass at a time.
        guard !aligning else { return }
        aligning = true
        defer { aligning = false }
        guard Self.shouldAlign(styleMask: window.styleMask) else { return }
        guard let close = window.standardWindowButton(.closeButton),
              let superview = close.superview else { return }
        adoptButtonsIfNeeded(window, close: close)
        for type in Self.buttonTypes {
            guard let button = window.standardWindowButton(type) else { continue }
            button.setFrameOrigin(TrafficLightAlignment.origin(
                frame: button.frame,
                offsetFromClose: offsets[type] ?? 0,
                headerHeight: headerHeight,
                superviewHeight: superview.bounds.height,
                flipped: superview.isFlipped
            ))
        }
    }

    /// Full screen hands the buttons to the auto-revealing strip along the
    /// top of the screen; they are not the header's to place there. A pure
    /// function of the style mask, since a test cannot put a real window
    /// into full screen.
    static func shouldAlign(styleMask: NSWindow.StyleMask) -> Bool {
        !styleMask.contains(.fullScreen)
    }

    /// Invisible to clicks: the view is about the buttons alone, and a
    /// background that swallowed a press would take it from the board.
    override public func hitTest(_ point: NSPoint) -> NSView? {
        nil
    }
}
