import AppKit
import CoreText
import SwiftUI

/// View ▸ Wrap lines in diffs, as `UserDefaults` holds it: on unless turned
/// off.
public let diffWrapsLinesKey = "diffWrapsLines"

/// A run of one file's rows marked in the diff, in the file's own row order.
public struct DiffCanvasSelection: Equatable, Sendable {
    public var path: String
    public var rowIDs: [String]

    public init(path: String, rowIDs: [String]) {
        self.path = path
        self.rowIDs = rowIDs
    }
}

/// What the diff draws over the diff itself: the review's state.
public struct DiffCanvasState: Equatable, Sendable {
    public var viewed: Set<String> = []
    public var collapsed: Set<String> = []
    public var commentCounts: [String: Int] = [:]
    public var selection: DiffCanvasSelection?
    /// Whether a new comment can be started at all — the hover button's
    /// gate: off while one is being written, and off on a diff that takes
    /// no comments.
    public var canComment = false
    public var showsViewed = true
    public var wrap = true
    /// A file's New or Updated badge (`SeenBadge`), by path, drawn on its
    /// header after the path.
    public var badges: [String: SeenBadge] = [:]

    public init() {}
}

/// What a click in the diff asks for. The canvas decides nothing about the
/// review itself: it says which row, and the review decides.
@MainActor
public struct DiffCanvasActions {
    public var rowClicked: @MainActor (DiffFileModel, DiffRow, _ shift: Bool) -> Void = { _, _, _ in }
    /// A drag across rows: the run from where it started to where it is,
    /// in the file's own order.
    public var rowsDragged: @MainActor (DiffFileModel, [String]) -> Void = { _, _ in }
    public var commentRequested: @MainActor (DiffFileModel, DiffRow, _ endsSelection: Bool) -> Void = { _, _, _ in }
    public var viewedToggled: @MainActor (String) -> Void = { _ in }
    public var collapseToggled: @MainActor (String) -> Void = { _ in }
    /// A gap's control pressed: reveal what it offers.
    public var gapExpanded: @MainActor (DiffFileModel, DiffGap, DiffGap.Control) -> Void = { _, _, _ in }
    /// The files whose rows are on screen, by path — seen, for their badges.
    /// Said whenever the view moves or what it shows changes.
    public var filesShown: @MainActor ([String]) -> Void = { _ in }

    public init() {}
}

/// The continuous diff, drawn by AppKit from an exact layout.
///
/// Every file one after another, its header pinned while its rows scroll
/// under it. The scroll view's document is an empty view exactly as tall as
/// `DiffLayout` says the diff is; over it, a view the size of the visible
/// rect draws whatever the layout puts there, and is moved to follow the
/// scroll. Nothing is laid out that is not on screen and nothing is
/// estimated: a row's height is arithmetic, the scroller is the true
/// proportion of the true height, and a jump to a file lands on its header
/// because its offset is known before the jump starts.
///
/// Comments and the comment editor are the caller's own views
/// (`setAttachments`), placed under the row they belong to and measured by
/// `measureAttachment`; whatever grows inside one says so through
/// `attachmentDidResize`.
@MainActor
public final class DiffCanvasView: NSView {
    public private(set) var files: [DiffFileModel] = []
    public private(set) var state = DiffCanvasState()
    public var actions = DiffCanvasActions()
    /// How tall an attachment's view is at a width.
    public var measureAttachment: (@MainActor (String, CGFloat) -> CGFloat)?

    public private(set) var diffLayout = DiffLayout(files: [], width: 0)
    public private(set) var metrics = DiffMetrics(codeSize: Typo.codeView)

    let scrollView = NSScrollView()
    let document = DiffDocumentView()
    let viewport = DiffViewportView()
    let verticalKnob = DiffKnobView(axis: .vertical)
    let horizontalKnob = DiffKnobView(axis: .horizontal)

    private var pathIndex: [String: Int] = [:]
    /// The files last said to be on screen (`actions.filesShown`), so a
    /// scroll within them says nothing again; forgotten on every update, so
    /// a fresh reading of what is on screen is said even unscrolled.
    private var lastShownFiles: [String]?
    private var attachmentViews: [String: NSView] = [:]
    private var attachmentHeights: [String: CGFloat] = [:]
    private var layoutWidth: CGFloat = -1
    private var relayoutScheduled = false

    let fonts = DiffFonts()
    var lineCache: [Int: DiffRowLines] = [:]
    /// The row under the pointer, as an item index.
    var hoveredItem: Int?

    // MARK: - Knob state

    private(set) var knobsLit = false
    private var knobHover = false
    private var knobDragging = false
    private var knobFade: Task<Void, Never>?
    private var legacyScrollers = NSScroller.preferredScrollerStyle == .legacy
    /// How long the knobs stay up after the last scroll, in the overlay
    /// style — `thinScrollers()`'s own pause.
    var knobLinger = Duration.milliseconds(900)

    public override init(frame: NSRect) {
        super.init(frame: frame)
        metrics.charWidth = fonts.charWidth

        scrollView.drawsBackground = false
        scrollView.hasVerticalScroller = false
        scrollView.hasHorizontalScroller = false
        scrollView.verticalScrollElasticity = .none
        scrollView.horizontalScrollElasticity = .none
        scrollView.automaticallyAdjustsContentInsets = false
        scrollView.documentView = document
        document.addSubview(viewport)
        viewport.canvas = self
        addSubview(scrollView)

        for knob in [verticalKnob, horizontalKnob] {
            knob.canvas = self
            knob.alphaValue = 0
            addSubview(knob)
        }

        scrollView.contentView.postsBoundsChangedNotifications = true
        NotificationCenter.default.addObserver(
            self, selector: #selector(scrolled), name: NSView.boundsDidChangeNotification, object: scrollView.contentView)
        NotificationCenter.default.addObserver(
            self, selector: #selector(scrollerStyleChanged),
            name: NSScroller.preferredScrollerStyleDidChangeNotification, object: nil)
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("init(coder:) is not supported") }

    public override var isFlipped: Bool { true }

    // MARK: - Input

    /// Takes the diff and the review's state. Only what changed is redone: a
    /// new selection or viewed mark redraws, a fold or a new diff lays out
    /// again, keeping the code at the top of the view where it was.
    public func update(files: [DiffFileModel], state: DiffCanvasState) {
        let filesChanged = files != self.files
        let old = self.state
        // Where the view is, said before the files it is said in change.
        var anchor = currentAnchor()
        // A fold of the file pinned at the top keeps its header where it is,
        // rather than the row under it that the fold just hid.
        let toggled = old.collapsed.symmetricDifference(state.collapsed)
        if let pinned = diffLayout.pinnedHeader(at: scrollOffset.y), toggled.contains(self.files[pinned.file].path) {
            anchor = Anchor(path: self.files[pinned.file].path, rowID: nil, isAttachment: false, fraction: 0)
        }
        self.files = files
        self.state = state
        lastShownFiles = nil

        if filesChanged {
            pathIndex = Dictionary(files.enumerated().map { ($1.path, $0) }, uniquingKeysWith: { first, _ in first })
            metrics.numberDigits = Self.numberDigits(files)
            lineCache.removeAll()
        }
        if filesChanged || old.collapsed != state.collapsed || old.wrap != state.wrap {
            relayout(keeping: anchor)
        } else {
            viewport.needsDisplay = true
            reportShownFiles()
        }
    }

    /// The views drawn under rows, keyed by `DiffLayout.key(path:rowID:)`.
    /// A view already placed under the same key is kept, not replaced, so
    /// whatever it holds (a half-written comment) survives.
    public func setAttachments(_ views: [String: NSView]) {
        var changed = false
        for (key, view) in attachmentViews where views[key] !== view {
            view.removeFromSuperview()
            attachmentViews[key] = nil
            attachmentHeights[key] = nil
            changed = true
        }
        for (key, view) in views where attachmentViews[key] == nil {
            attachmentViews[key] = view
            viewport.addSubview(view)
            attachmentHeights[key] = measure(key)
            changed = true
        }
        if changed { relayout(keeping: currentAnchor()) }
    }

    /// An attachment's content changed height: measured again, and laid out
    /// again if it did — on the next turn of the run loop, since this is
    /// typically said from inside a SwiftUI layout pass.
    public func attachmentDidResize(_ key: String) {
        guard attachmentViews[key] != nil, !relayoutScheduled else { return }
        relayoutScheduled = true
        DispatchQueue.main.async { [weak self] in
            guard let self else { return }
            relayoutScheduled = false
            var changed = false
            for key in attachmentViews.keys {
                let height = measure(key)
                if abs(height - (attachmentHeights[key] ?? 0)) > 0.5 {
                    attachmentHeights[key] = height
                    changed = true
                }
            }
            if changed { relayout(keeping: currentAnchor()) }
        }
    }

    /// Scrolls so `path`'s header is at the top — exactly there, since its
    /// offset is known — or as near as the end of the diff allows.
    /// Asked before the first layout, the jump waits for it.
    public func scrollToFile(_ path: String, animated: Bool = true) {
        guard layoutWidth > 0 else {
            pendingScroll = path
            return
        }
        guard let file = pathIndex[path], let index = diffLayout.index(of: .header(file: file)) else { return }
        scroll(to: NSPoint(x: 0, y: diffLayout.top(index)), animated: animated)
    }

    private var pendingScroll: String?

    // MARK: - Geometry

    var scrollOffset: NSPoint { scrollView.contentView.bounds.origin }
    var viewportSize: NSSize { scrollView.contentView.bounds.size }

    public override func layout() {
        super.layout()
        scrollView.frame = bounds
        if scrollView.contentSize.width != layoutWidth {
            relayout(keeping: currentAnchor())
        } else {
            sizeDocument()
        }
    }

    /// Where the view is, said in terms that survive a new layout: the item
    /// at the top, and how far into it.
    struct Anchor {
        let path: String
        /// nil for the file's header.
        let rowID: String?
        let isAttachment: Bool
        let fraction: CGFloat
    }

    private func currentAnchor() -> Anchor? {
        guard !files.isEmpty, diffLayout.items.count > 1 else { return nil }
        let y = scrollOffset.y
        let index = diffLayout.index(at: y)
        let fraction = diffLayout.height(index) > 0 ? (y - diffLayout.top(index)) / diffLayout.height(index) : 0
        switch diffLayout.items[index] {
        case .header(let file):
            return Anchor(path: files[file].path, rowID: nil, isAttachment: false, fraction: fraction)
        case .row(let file, let row):
            return Anchor(path: files[file].path, rowID: files[file].rows[row].id, isAttachment: false, fraction: fraction)
        case .attachment(let file, let row):
            return Anchor(path: files[file].path, rowID: files[file].rows[row].id, isAttachment: true, fraction: fraction)
        case .footer:
            return nil
        }
    }

    private func offset(of anchor: Anchor) -> CGFloat? {
        guard let file = pathIndex[anchor.path] else { return nil }
        var item = DiffLayout.Item.header(file: file)
        if let rowID = anchor.rowID, let row = files[file].rows.firstIndex(where: { $0.id == rowID }) {
            item = anchor.isAttachment ? .attachment(file: file, row: row) : .row(file: file, row: row)
        }
        let index = diffLayout.index(of: item) ?? diffLayout.index(of: .header(file: file))
        guard let index else { return nil }
        return diffLayout.top(index) + diffLayout.height(index) * min(max(anchor.fraction, 0), 1)
    }

    /// Lays the diff out again at the current width, then puts `anchor` —
    /// typically whatever was at the top before — back at the top: the top
    /// of the diff where there is none, or it is no longer in the diff.
    func relayout(keeping anchor: Anchor?) {
        let width = scrollView.contentSize.width
        if width != layoutWidth {
            for key in attachmentViews.keys { attachmentHeights[key] = measure(key, width: width) }
        }
        layoutWidth = width
        diffLayout = DiffLayout(
            files: files, collapsed: state.collapsed, attachmentHeights: attachmentHeights,
            width: max(width, 1), wrap: state.wrap, metrics: metrics)
        lineCache.removeAll()
        sizeDocument()
        let y = anchor.flatMap(offset(of:)) ?? 0
        scroll(to: NSPoint(x: state.wrap ? 0 : scrollOffset.x, y: y), animated: false)
        if let path = pendingScroll, width > 0 {
            pendingScroll = nil
            scrollToFile(path, animated: false)
        }
        viewportMoved()
    }

    private func sizeDocument() {
        let size = viewportSize
        let height = max(diffLayout.totalHeight, size.height)
        let width = max(diffLayout.contentWidth, size.width)
        if document.frame.size != NSSize(width: width, height: height) {
            document.setFrameSize(NSSize(width: width, height: height))
        }
        viewportMoved()
    }

    private func scroll(to point: NSPoint, animated: Bool) {
        let size = viewportSize
        let target = NSPoint(
            x: min(max(point.x, 0), max(document.frame.width - size.width, 0)),
            y: min(max(point.y, 0), max(document.frame.height - size.height, 0)))
        let clip = scrollView.contentView
        if animated, let duration = Motion.stateChangeDuration {
            NSAnimationContext.runAnimationGroup { context in
                context.duration = duration
                context.timingFunction = CAMediaTimingFunction(name: .easeOut)
                clip.animator().setBoundsOrigin(target)
            }
        } else {
            clip.setBoundsOrigin(target)
        }
        scrollView.reflectScrolledClipView(clip)
    }

    @objc private func scrolled() {
        viewportMoved()
        lightKnobs()
    }

    /// The visible rect moved or changed size: the viewport follows it and
    /// draws afresh, the attachments and knobs move with the content.
    func viewportMoved() {
        let visible = scrollView.contentView.bounds
        if viewport.frame != visible { viewport.frame = visible }
        viewport.needsDisplay = true
        placeAttachments()
        placeKnobs()
        viewport.refreshHover()
        reportShownFiles()
    }

    /// Say which files' rows are on screen, where that has changed.
    private func reportShownFiles() {
        let visible = scrollView.contentView.bounds
        guard visible.height > 0 else { return }
        let paths = diffLayout.shownFiles(from: visible.minY, to: visible.maxY)
            .filter { files.indices.contains($0) }.map { files[$0].path }
        guard paths != lastShownFiles else { return }
        lastShownFiles = paths
        if !paths.isEmpty { actions.filesShown(paths) }
    }

    private func measure(_ key: String, width: CGFloat? = nil) -> CGFloat {
        let width = (width ?? scrollView.contentSize.width) - attachmentInset
        return max(0, measureAttachment?(key, max(width, 1)) ?? attachmentViews[key]?.fittingSize.height ?? 0)
    }

    /// Where an attachment starts: at the gutter's far edge, its hairline
    /// included, so a comment lines up with the code rather than the box.
    var attachmentInset: CGFloat { metrics.gutterWidth + 0.5 }

    private func placeAttachments() {
        guard !attachmentViews.isEmpty else { return }
        let size = viewportSize
        let y = scrollOffset.y
        for (key, view) in attachmentViews {
            guard let (file, row) = attachmentRow(key),
                  let index = diffLayout.index(of: .attachment(file: file, row: row)) else {
                view.isHidden = true
                continue
            }
            let frame = NSRect(
                x: attachmentInset, y: diffLayout.top(index) - y,
                width: max(size.width - attachmentInset, 0), height: diffLayout.height(index))
            let onScreen = frame.maxY > 0 && frame.minY < size.height
            if view.isHidden == onScreen { view.isHidden = !onScreen }
            if onScreen, view.frame != frame { view.frame = frame }
        }
    }

    private func attachmentRow(_ key: String) -> (Int, Int)? {
        let parts = key.split(separator: "\u{0}", maxSplits: 1, omittingEmptySubsequences: false)
        guard parts.count == 2, let file = pathIndex[String(parts[0])] else { return nil }
        let rowID = String(parts[1])
        guard let row = files[file].rows.firstIndex(where: { $0.id == rowID }) else { return nil }
        return (file, row)
    }

    static func numberDigits(_ files: [DiffFileModel]) -> Int {
        var widest = 0
        for file in files {
            for row in file.rows {
                widest = max(widest, row.oldNumber ?? 0, row.newNumber ?? 0)
            }
        }
        return max(String(widest).count, 1)
    }

    // MARK: - Knobs

    private func placeKnobs() {
        let size = viewportSize
        let offset = scrollOffset
        let strip = ScrollKnob.thickness + 2 * ScrollKnob.inset
        if let knob = ScrollKnob(visible: size.height, content: document.frame.height, scrolled: offset.y) {
            verticalKnob.isHidden = false
            verticalKnob.knob = knob
            verticalKnob.frame = NSRect(
                x: bounds.width - strip, y: knob.offset - ScrollKnob.inset,
                width: strip, height: knob.length + 2 * ScrollKnob.inset)
        } else {
            verticalKnob.isHidden = true
        }
        if let knob = ScrollKnob(visible: size.width, content: document.frame.width, scrolled: offset.x) {
            horizontalKnob.isHidden = false
            horizontalKnob.knob = knob
            horizontalKnob.frame = NSRect(
                x: knob.offset - ScrollKnob.inset, y: bounds.height - strip,
                width: knob.length + 2 * ScrollKnob.inset, height: strip)
        } else {
            horizontalKnob.isHidden = true
        }
        showKnobs(legacyScrollers || knobsLit || knobDragging)
    }

    private func showKnobs(_ shown: Bool) {
        let alpha: CGFloat = shown ? 1 : 0
        for knob in [verticalKnob, horizontalKnob] where knob.alphaValue != alpha {
            NSAnimationContext.runAnimationGroup { context in
                context.duration = 0.2
                knob.animator().alphaValue = alpha
            }
        }
    }

    /// Shows the knobs, and in the overlay style fades them after a pause.
    func lightKnobs() {
        knobsLit = true
        showKnobs(true)
        knobFade?.cancel()
        knobFade = Task { @MainActor [weak self] in
            try? await Task.sleep(for: self?.knobLinger ?? .zero)
            guard let self, !Task.isCancelled, !knobHover, !knobDragging else { return }
            knobsLit = false
            showKnobs(legacyScrollers)
        }
    }

    func knobHovered(_ inside: Bool) {
        knobHover = inside
        if inside { lightKnobs() }
    }

    private var knobDragOrigin: NSPoint?

    func knobDrag(_ axis: Axis, translation: CGFloat, ended: Bool) {
        if ended {
            knobDragging = false
            knobDragOrigin = nil
            lightKnobs()
            return
        }
        knobDragging = true
        let origin = knobDragOrigin ?? scrollOffset
        knobDragOrigin = origin
        let knob = axis == .vertical ? verticalKnob.knob : horizontalKnob.knob
        guard let knob else { return }
        let size = viewportSize
        let moved = ScrollKnob.contentDistance(
            forDrag: translation,
            visible: axis == .vertical ? size.height : size.width,
            content: axis == .vertical ? document.frame.height : document.frame.width,
            knob: knob)
        scroll(
            to: axis == .vertical ? NSPoint(x: origin.x, y: origin.y + moved) : NSPoint(x: origin.x + moved, y: origin.y),
            animated: false)
    }

    @objc private func scrollerStyleChanged() {
        legacyScrollers = NSScroller.preferredScrollerStyle == .legacy
        placeKnobs()
    }

    public override func updateTrackingAreas() {
        super.updateTrackingAreas()
        for area in trackingAreas { removeTrackingArea(area) }
        addTrackingArea(NSTrackingArea(
            rect: .zero, options: [.mouseEnteredAndExited, .activeInKeyWindow, .inVisibleRect], owner: self))
    }

    public override func mouseEntered(with event: NSEvent) { knobHovered(true) }
    public override func mouseExited(with event: NSEvent) { knobHovered(false) }
}

/// The scroll view's document: as big as the diff, and drawing nothing — the
/// viewport over it does. Kept off responsive scrolling, which would scroll
/// a layer ahead of the viewport's redraw and show its edge.
final class DiffDocumentView: NSView {
    override var isFlipped: Bool { true }
    override class var isCompatibleWithResponsiveScrolling: Bool { false }
}

/// The app's thin scroll knob over the diff — `thinScrollers()`'s, drawn in
/// AppKit since the diff is.
final class DiffKnobView: NSView {
    let axis: Axis
    weak var canvas: DiffCanvasView?
    var knob: ScrollKnob? { didSet { if knob != oldValue { needsDisplay = true } } }
    /// Where a drag began, in the canvas — which, unlike the knob, does not
    /// move as the drag scrolls.
    private var dragStart: NSPoint?

    init(axis: Axis) {
        self.axis = axis
        super.init(frame: .zero)
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("init(coder:) is not supported") }

    override var isFlipped: Bool { true }

    override func draw(_ dirtyRect: NSRect) {
        let rect = bounds.insetBy(dx: ScrollKnob.inset, dy: ScrollKnob.inset)
        NSColor(DesignTokens.scrollerKnob).setFill()
        NSBezierPath(roundedRect: rect, xRadius: ScrollKnob.thickness / 2, yRadius: ScrollKnob.thickness / 2).fill()
    }

    override func mouseDown(with event: NSEvent) {
        dragStart = canvas?.convert(event.locationInWindow, from: nil)
        canvas?.knobDrag(axis, translation: 0, ended: false)
    }

    override func mouseDragged(with event: NSEvent) {
        guard let dragStart, let canvas else { return }
        let now = canvas.convert(event.locationInWindow, from: nil)
        canvas.knobDrag(axis, translation: axis == .vertical ? now.y - dragStart.y : now.x - dragStart.x, ended: false)
    }

    override func mouseUp(with event: NSEvent) {
        dragStart = nil
        canvas?.knobDrag(axis, translation: 0, ended: true)
    }
}
