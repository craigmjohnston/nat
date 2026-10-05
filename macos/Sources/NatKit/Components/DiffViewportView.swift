import AppKit
import CoreText
import SwiftUI

/// The faces the diff is drawn in, and the one measurement of them the
/// layout needs: how wide a column of code is.
@MainActor
final class DiffFonts {
    let code = Typo.monoNSFont(size: Typo.codeView)
    let header = Typo.monoNSFont(size: Typo.codeView)
    /// The design's mono `xs`: a hunk break, a header's tally, the closing
    /// line.
    let small = Typo.monoNSFont(size: Typo.codeView(12))
    let check = NSFont.systemFont(ofSize: 9)

    var charWidth: CGFloat {
        ("0" as NSString).size(withAttributes: [.font: code]).width
    }

    /// Where the first baseline sits below the top of a line `lineHeight`
    /// tall, the line's ink centred in it.
    func baseline(_ font: NSFont, lineHeight: CGFloat) -> CGFloat {
        (lineHeight - (font.ascender - font.descender)) / 2 + font.ascender
    }
}

/// What the diff draws with, by the role each colour plays — every one a
/// `DesignTokens` value, resolved against the appearance it is drawn under.
@MainActor
enum DiffInk {
    static let background = NSColor(DesignTokens.fill(.window))
    /// A file's header band: the pane's own ground, so the picked titlebar
    /// tab — drawn in that ground and open into the pane — runs on down into
    /// the first heading with no band of another colour between; its rules
    /// are what set a heading off.
    static let headerBand = NSColor(DesignTokens.fill(.window))
    static let rule = NSColor(DesignTokens.rule(.separator, on: .window))
    static let added = NSColor(DesignTokens.diffAddedRowBg(on: .window))
    static let removed = NSColor(DesignTokens.diffRemovedRowBg(on: .window))
    /// The number column beside an added or removed row: the same hue
    /// pressed harder, the stripe that marks the change now there is no
    /// +/- to.
    static let addedGutter = NSColor(DesignTokens.diffAddedGutterBg(on: .window))
    static let removedGutter = NSColor(DesignTokens.diffRemovedGutterBg(on: .window))
    static let selection = NSColor(DesignTokens.wash(.selection, tone: .accent, on: .window))
    /// A gap's band, its gutter cell — where its expand buttons sit — a step
    /// stronger, and stronger again under the pointer.
    static let gapBand = NSColor(DesignTokens.wash(.comment, tone: .accent, on: .window))
    static let gapButton = NSColor(DesignTokens.wash(.selection, tone: .accent, on: .window))
    static let gapButtonHover = NSColor(DesignTokens.wash(.chip, tone: .accent, on: .window))
    static let accent = NSColor(DesignTokens.ink(.accent, on: .window))
    static let primary = NSColor(DesignTokens.ink(.primary, on: .window))
    static let secondary = NSColor(DesignTokens.ink(.secondary, on: .window))
    static let tertiary = NSColor(DesignTokens.ink(.tertiary, on: .window))
    static let quaternary = NSColor(DesignTokens.ink(.quaternary, on: .window))
    static let success = NSColor(DesignTokens.ink(.success, on: .window))
    static let danger = NSColor(DesignTokens.ink(.danger, on: .window))
    static let checked = NSColor(DesignTokens.rowWash(selected: true, on: .window))
    static let control = NSColor(DesignTokens.fill(.control))
    static let controlBorder = NSColor(DesignTokens.rule(.border, on: .control))
    static let controlAccent = NSColor(DesignTokens.ink(.accent, on: .control))
    /// Code is inked for the `.card` ground the file box always drew it on.
    static let code = DesignTokens.ink(.primary, on: .card)
    static let described = DesignTokens.ink(.secondary, on: .card)

    private static let syntax: [TokenKind: NSColor] = Dictionary(
        uniqueKeysWithValues: [TokenKind.comment, .keyword, .string, .number, .name].map {
            ($0, NSColor(DiffSyntax.color(for: $0, defaultColor: code)))
        })
    private static let codeText = NSColor(code)
    private static let describedText = NSColor(described)

    static func color(for kind: TokenKind, described: Bool) -> NSColor {
        syntax[kind] ?? (described ? describedText : codeText)
    }
}

/// One row's text, set once and drawn from then on: the code broken into
/// its lines, and the gutter's numbers.
struct DiffRowLines {
    var code: [CTLine] = []
    var number: CTLine?
}

/// The visible rect of the diff, drawn: whatever `DiffLayout` puts between
/// the top and bottom of the view, and the header of the file the top is in,
/// pinned over it. The canvas moves this to follow the scroll, so it is only
/// ever as big as the window shows.
final class DiffViewportView: NSView {
    weak var canvas: DiffCanvasView?
    /// A drag across rows: the file and row it began on.
    private var dragAnchor: (file: Int, row: Int)?
    private var dragLast: Int?
    /// Where the pointer is, for the gap control under it.
    private var hoverPoint: NSPoint?

    override var isFlipped: Bool { true }
    override var isOpaque: Bool { true }
    override var acceptsFirstResponder: Bool { true }

    override func viewDidChangeEffectiveAppearance() {
        super.viewDidChangeEffectiveAppearance()
        canvas?.lineCache.removeAll()
        needsDisplay = true
    }

    override func resetCursorRects() {
        addCursorRect(bounds, cursor: .arrow)
    }

    // MARK: - Drawing

    override func draw(_ dirtyRect: NSRect) {
        DiffInk.background.setFill()
        dirtyRect.fill()
        guard let canvas, let context = NSGraphicsContext.current?.cgContext else { return }
        let layout = canvas.diffLayout
        guard layout.items.count > 1 else { return }
        let top = canvas.scrollOffset.y
        let selected = Set(canvas.state.selection?.rowIDs ?? [])
        let selectedPath = canvas.state.selection?.path

        for index in layout.indices(from: top + dirtyRect.minY, to: top + dirtyRect.maxY) {
            let rect = NSRect(x: 0, y: layout.top(index) - top, width: bounds.width, height: layout.height(index))
            switch layout.items[index] {
            case .header(let file):
                drawHeader(canvas, file: file, y: rect.minY)
            case .row(let file, let row):
                let model = canvas.files[file]
                let isSelected = model.path == selectedPath && selected.contains(model.rows[row].id)
                drawRow(canvas, context: context, index: index, file: file, row: row, rect: rect, selected: isSelected)
            case .attachment:
                break
            case .footer:
                drawFooter(canvas, rect: rect)
            }
        }

        if let button = commentButton(canvas) {
            drawCommentButton(button.rect)
        }
        if let pinned = layout.pinnedHeader(at: top) {
            drawHeader(canvas, file: pinned.file, y: pinned.offset)
        }
    }

    private func drawRow(
        _ canvas: DiffCanvasView, context: CGContext, index: Int, file: Int, row: Int, rect: NSRect, selected: Bool
    ) {
        let model = canvas.files[file].rows[row]
        let metrics = canvas.metrics
        let fill: NSColor? = selected ? DiffInk.selection : model.kind == .added ? DiffInk.added
            : model.kind == .removed ? DiffInk.removed : nil
        if let fill {
            fill.setFill()
            rect.fill()
        }
        if !selected, let stripe = model.kind == .added ? DiffInk.addedGutter : model.kind == .removed ? DiffInk.removedGutter : nil {
            stripe.setFill()
            NSRect(x: 0, y: rect.minY, width: metrics.gutterWidth, height: rect.height).fill()
        }

        if model.kind == .hunkBreak {
            if let gap = model.gap { drawGap(gap, metrics: metrics, rect: rect) }
            drawString(
                model.text, font: canvas.fonts.small, color: DiffInk.tertiary,
                in: NSRect(x: metrics.textX, y: rect.minY, width: max(bounds.width - metrics.textX - 16, 0),
                           height: rect.height),
                truncating: .byTruncatingTail)
            return
        }

        let lines = canvas.lines(for: index, row: model)
        let baseline = rect.minY + metrics.rowPadding / 2 + canvas.fonts.baseline(canvas.fonts.code, lineHeight: metrics.lineHeight)
        context.saveGState()
        context.textMatrix = CGAffineTransform(scaleX: 1, y: -1)

        if let number = lines.number {
            draw(number, context: context, rightEdge: 8 + metrics.numberColumnWidth, baseline: baseline)
        }

        // Code scrolls sideways under a gutter that does not.
        context.clip(to: NSRect(x: metrics.textX - 1, y: rect.minY, width: bounds.width, height: rect.height))
        let x = metrics.textX - canvas.scrollOffset.x
        for (offset, line) in lines.code.enumerated() {
            context.textPosition = CGPoint(x: x, y: baseline + CGFloat(offset) * metrics.lineHeight)
            CTLineDraw(line, context)
        }
        context.restoreGState()
    }

    /// A gap's band, and its controls stacked in the gutter, an equal share
    /// of the band apiece: ↓ the lines below the change above, ↑ those above
    /// the change below, ↕ the lot.
    private func drawGap(_ gap: DiffGap, metrics: DiffMetrics, rect: NSRect) {
        DiffInk.gapBand.setFill()
        rect.fill()
        let slotHeight = rect.height / CGFloat(gap.controls.count)
        for (slot, control) in gap.controls.enumerated() {
            let cell = NSRect(
                x: 0, y: rect.minY + CGFloat(slot) * slotHeight,
                width: metrics.gutterWidth, height: slotHeight)
            let hovered = hoverPoint.map { cell.contains($0) } ?? false
            (hovered ? DiffInk.gapButtonHover : DiffInk.gapButton).setFill()
            cell.fill()
            let symbol = switch control {
            case .down: "arrow.down"
            case .up: "arrow.up"
            case .all: "arrow.up.and.down"
            }
            drawSymbol(symbol, size: 11, weight: .semibold, color: hovered ? DiffInk.primary : DiffInk.accent, in: cell)
        }
    }

    private func draw(_ line: CTLine, context: CGContext, rightEdge: CGFloat, baseline: CGFloat) {
        let width = CTLineGetTypographicBounds(line, nil, nil, nil)
        context.textPosition = CGPoint(x: rightEdge - width, y: baseline)
        CTLineDraw(line, context)
    }

    private func drawFooter(_ canvas: DiffCanvasView, rect: NSRect) {
        let count = canvas.files.count
        drawString(
            "\(count) \(count == 1 ? "file" : "files")", font: canvas.fonts.small, color: DiffInk.secondary,
            in: rect.insetBy(dx: 14, dy: 14), alignment: .center)
    }

    // MARK: - Headers

    /// A file's header band: chevron, path, rename and comment mark from the
    /// leading edge; the tally against the trailing edge, just inside — on a
    /// review — the viewed box.
    private func drawHeader(_ canvas: DiffCanvasView, file: Int, y: CGFloat) {
        let model = canvas.files[file]
        let height = canvas.metrics.headerHeight
        let band = NSRect(x: 0, y: y, width: bounds.width, height: height)
        DiffInk.headerBand.setFill()
        band.fill()
        DiffInk.rule.setFill()
        // A header at the top — the first file's, or any file's pinned there —
        // sits on the pane heading's own line, and one under a folded file
        // on that header's bottom rule: two rules never meet back to back.
        if y > 0 && !Self.followsFoldedFile(canvas, file: file) {
            NSRect(x: 0, y: y, width: band.width, height: 1).fill()
        }
        NSRect(x: 0, y: band.maxY - 1, width: band.width, height: 1).fill()

        drawChevron(open: !canvas.state.collapsed.contains(model.path), at: NSPoint(x: 11, y: y + (height - 10) / 2))

        let layout = headerLayout(canvas, file: file, y: y)
        drawString(model.path, font: canvas.fonts.header, color: DiffInk.primary, in: layout.path, truncating: .byTruncatingHead)
        var x = layout.path.maxX + 8
        for (text, width) in layout.extras {
            drawString(text, font: canvas.fonts.small, color: DiffInk.secondary,
                       in: NSRect(x: x, y: y, width: width, height: height))
            x += width + 8
        }
        if layout.hasComments {
            drawSymbol("text.bubble.fill", size: 10, weight: .regular, color: DiffInk.secondary,
                       in: NSRect(x: x, y: y, width: 12, height: height))
        }
        drawString(layout.tallyText, font: canvas.fonts.small, color: DiffInk.secondary, in: layout.tally,
                   alignment: .right)
        if let viewed = layout.viewed {
            let checked = canvas.state.viewed.contains(model.path)
            let box = NSRect(x: viewed.minX, y: viewed.midY - 7, width: 14, height: 14)
            if checked {
                DiffInk.checked.setFill()
                NSBezierPath(roundedRect: box, xRadius: 3, yRadius: 3).fill()
            }
            (checked ? DiffInk.tertiary : DiffInk.quaternary).setStroke()
            let border = NSBezierPath(roundedRect: box.insetBy(dx: 0.5, dy: 0.5), xRadius: 2.5, yRadius: 2.5)
            border.lineWidth = 1
            border.stroke()
            if checked {
                drawString("\u{2713}", font: canvas.fonts.check, color: DiffInk.primary, in: box, alignment: .center)
            }
            drawString("viewed", font: canvas.fonts.small, color: checked ? DiffInk.primary : DiffInk.secondary,
                       in: NSRect(x: box.maxX + 6, y: y, width: viewed.maxX - box.maxX - 6, height: height))
        }
    }

    /// Whether the file before `file` is folded to its header — whose bottom
    /// rule is then the one rule between the two headers.
    static func followsFoldedFile(_ canvas: DiffCanvasView, file: Int) -> Bool {
        file > 0 && canvas.state.collapsed.contains(canvas.files[file - 1].path)
    }

    struct HeaderLayout {
        var path: NSRect
        /// What follows the path: the rename note, where there is one.
        var extras: [(String, CGFloat)]
        var hasComments: Bool
        /// The `+N −N` tally, right-aligned against the trailing edge — just
        /// inside the viewed toggle where there is one.
        var tally: NSRect
        var tallyText: String
        /// The viewed toggle — box and word — where the header has one.
        var viewed: NSRect?
    }

    /// Where a header's pieces go across the band: from the trailing edge the
    /// viewed toggle, then the tally; the path takes what the rest leaves it,
    /// cut from its head so the file's own name stays.
    func headerLayout(_ canvas: DiffCanvasView, file: Int, y: CGFloat) -> HeaderLayout {
        let model = canvas.files[file]
        let height = canvas.metrics.headerHeight
        let small = canvas.fonts.small
        var extras: [(String, CGFloat)] = []
        if model.isRenamed { extras.append(("was \(model.oldPath)", width("was \(model.oldPath)", small))) }
        let tallyText = "+\(model.adds) \u{2212}\(model.dels)"
        let hasComments = (canvas.state.commentCounts[model.path] ?? 0) > 0

        var trailing = bounds.width - 16
        var viewed: NSRect?
        if canvas.state.showsViewed {
            let viewedWidth = 14 + 6 + width("viewed", small)
            trailing -= viewedWidth
            viewed = NSRect(x: trailing, y: y, width: viewedWidth, height: height)
            trailing -= 8
        }
        let tallyWidth = width(tallyText, small)
        let tally = NSRect(x: trailing - tallyWidth, y: y, width: tallyWidth, height: height)
        trailing -= tallyWidth + 8
        let extrasWidth = extras.reduce(0) { $0 + $1.1 + 8 } + (hasComments ? 12 + 8 : 0)
        let pathX: CGFloat = 30
        let pathWidth = min(width(model.path, canvas.fonts.header), max(trailing - pathX - extrasWidth, 0))
        return HeaderLayout(
            path: NSRect(x: pathX, y: y, width: pathWidth, height: height),
            extras: extras, hasComments: hasComments, tally: tally, tallyText: tallyText, viewed: viewed)
    }

    private func drawChevron(open: Bool, at origin: NSPoint) {
        let points: [(CGFloat, CGFloat)] = open ? [(2, 3.5), (5, 6.5), (8, 3.5)] : [(3.5, 2), (6.5, 5), (3.5, 8)]
        let path = NSBezierPath()
        path.move(to: NSPoint(x: origin.x + points[0].0, y: origin.y + points[0].1))
        for point in points.dropFirst() { path.line(to: NSPoint(x: origin.x + point.0, y: origin.y + point.1)) }
        path.lineWidth = 1.6
        path.lineCapStyle = .round
        path.lineJoinStyle = .round
        DiffInk.tertiary.setStroke()
        path.stroke()
    }

    // MARK: - The comment button

    /// The "+" over the trailing end of a row: on the row under the pointer,
    /// and on the end of a marked run, while a comment can be started.
    func commentButton(_ canvas: DiffCanvasView) -> (rect: NSRect, file: Int, row: Int, endsSelection: Bool)? {
        guard canvas.state.canComment else { return nil }
        let layout = canvas.diffLayout
        var candidates: [Int] = []
        if let hovered = canvas.hoveredItem { candidates.append(hovered) }
        if let selection = canvas.state.selection, let last = selection.rowIDs.last,
           let file = canvas.files.firstIndex(where: { $0.path == selection.path }),
           let row = canvas.files[file].rows.firstIndex(where: { $0.id == last }),
           let index = layout.index(of: .row(file: file, row: row)) {
            candidates.append(index)
        }
        // The hovered row's button wins where both show; the marked run's is
        // drawn too, but only one is ever under the pointer to be clicked.
        for index in candidates where layout.items.indices.contains(index) {
            guard case .row(let file, let row) = layout.items[index],
                  canvas.files[file].rows[row].kind != .hunkBreak else { continue }
            let rowID = canvas.files[file].rows[row].id
            let ends = canvas.state.selection?.path == canvas.files[file].path && canvas.state.selection?.rowIDs.last == rowID
            let rect = NSRect(
                x: bounds.width - 8 - 24, y: layout.top(index) - canvas.scrollOffset.y + 1,
                width: 24, height: canvas.metrics.rowMinHeight - 2)
            return (rect, file, row, ends)
        }
        return nil
    }

    private func drawCommentButton(_ rect: NSRect) {
        let shape = NSBezierPath(roundedRect: rect, xRadius: 4, yRadius: 4)
        DiffInk.control.setFill()
        shape.fill()
        DiffInk.controlBorder.setStroke()
        let border = NSBezierPath(roundedRect: rect.insetBy(dx: 0.25, dy: 0.25), xRadius: 4, yRadius: 4)
        border.lineWidth = 0.5
        border.stroke()
        drawSymbol("plus.bubble", size: 12, weight: .medium, color: DiffInk.controlAccent, in: rect)
    }

    // MARK: - Text helpers

    private func width(_ text: String, _ font: NSFont) -> CGFloat {
        ceil((text as NSString).size(withAttributes: [.font: font]).width)
    }

    private func drawString(
        _ text: String, font: NSFont, color: NSColor, in rect: NSRect,
        truncating: NSLineBreakMode = .byClipping, alignment: NSTextAlignment = .left
    ) {
        let style = NSMutableParagraphStyle()
        style.lineBreakMode = truncating
        style.alignment = alignment
        let lineHeight = ceil(font.ascender - font.descender + font.leading)
        let line = NSRect(x: rect.minX, y: rect.midY - lineHeight / 2, width: rect.width, height: lineHeight)
        (text as NSString).draw(
            with: line, options: [.usesLineFragmentOrigin, .truncatesLastVisibleLine],
            attributes: [.font: font, .foregroundColor: color, .paragraphStyle: style])
    }

    private func drawSymbol(_ name: String, size: CGFloat, weight: NSFont.Weight, color: NSColor, in rect: NSRect) {
        let configuration = NSImage.SymbolConfiguration(pointSize: size, weight: weight)
            .applying(NSImage.SymbolConfiguration(paletteColors: [NSColor(cgColor: color.cgColor) ?? color]))
        guard let image = NSImage(systemSymbolName: name, accessibilityDescription: nil)?
            .withSymbolConfiguration(configuration) else { return }
        let size = image.size
        image.draw(in: NSRect(x: rect.midX - size.width / 2, y: rect.midY - size.height / 2,
                              width: size.width, height: size.height))
    }

    // MARK: - Pointer

    enum Hit: Equatable {
        case header(file: Int, viewed: Bool)
        case commentButton(file: Int, row: Int, endsSelection: Bool)
        case row(file: Int, row: Int)
        /// One of a gap's controls: its button, or anywhere along its slot.
        case gap(file: Int, row: Int, control: DiffGap.Control)
    }

    /// What is under a point of the view: the pinned header first, since it
    /// is drawn over whatever scrolls beneath it.
    func hit(at point: NSPoint) -> Hit? {
        guard let canvas else { return nil }
        let layout = canvas.diffLayout
        guard layout.items.count > 1 else { return nil }
        let top = canvas.scrollOffset.y
        if let pinned = layout.pinnedHeader(at: top),
           point.y >= pinned.offset, point.y < pinned.offset + canvas.metrics.headerHeight {
            return headerHit(canvas, file: pinned.file, y: pinned.offset, point: point)
        }
        if let button = commentButton(canvas), button.rect.contains(point) {
            return .commentButton(file: button.file, row: button.row, endsSelection: button.endsSelection)
        }
        let index = layout.index(at: top + point.y)
        switch layout.items[index] {
        case .header(let file):
            return headerHit(canvas, file: file, y: layout.top(index) - top, point: point)
        case .row(let file, let row):
            if let gap = canvas.files[file].rows[row].gap {
                let slot = Int((top + point.y - layout.top(index)) / (layout.height(index) / CGFloat(gap.controls.count)))
                return .gap(file: file, row: row, control: gap.controls[min(max(slot, 0), gap.controls.count - 1)])
            }
            return .row(file: file, row: row)
        case .attachment, .footer:
            return nil
        }
    }

    private func headerHit(_ canvas: DiffCanvasView, file: Int, y: CGFloat, point: NSPoint) -> Hit {
        let viewed = headerLayout(canvas, file: file, y: y).viewed?.contains(point) ?? false
        return .header(file: file, viewed: viewed)
    }

    override func mouseDown(with event: NSEvent) {
        window?.makeFirstResponder(self)
        dragAnchor = nil
        guard let canvas, let hit = hit(at: convert(event.locationInWindow, from: nil)) else { return }
        switch hit {
        case .header(let file, let viewed):
            let path = canvas.files[file].path
            viewed ? canvas.actions.viewedToggled(path) : canvas.actions.collapseToggled(path)
        case .commentButton(let file, let row, let endsSelection):
            canvas.actions.commentRequested(canvas.files[file], canvas.files[file].rows[row], endsSelection)
        case .gap(let file, let row, let control):
            if let gap = canvas.files[file].rows[row].gap {
                canvas.actions.gapExpanded(canvas.files[file], gap, control)
            }
        case .row(let file, let row):
            let model = canvas.files[file]
            guard model.rows[row].kind != .hunkBreak else { return }
            let shift = event.modifierFlags.contains(.shift)
            canvas.actions.rowClicked(model, model.rows[row], shift)
            if !shift {
                dragAnchor = (file, row)
                dragLast = row
            }
        }
    }

    override func mouseDragged(with event: NSEvent) {
        guard let canvas, let anchor = dragAnchor else { return }
        autoscroll(with: event)
        let point = convert(event.locationInWindow, from: nil)
        let row = rowNear(canvas, file: anchor.file, y: canvas.scrollOffset.y + point.y)
        guard row != dragLast else { return }
        dragLast = row
        let model = canvas.files[anchor.file]
        let range = min(anchor.row, row)...max(anchor.row, row)
        canvas.actions.rowsDragged(model, range.map { model.rows[$0].id })
    }

    override func mouseUp(with event: NSEvent) {
        dragAnchor = nil
        dragLast = nil
    }

    /// The row of `file` a drag at `y` has reached — its first or last where
    /// the drag has left the file.
    private func rowNear(_ canvas: DiffCanvasView, file: Int, y: CGFloat) -> Int {
        let layout = canvas.diffLayout
        let rows = canvas.files[file].rows.count
        switch layout.items[layout.index(at: y)] {
        case .row(let at, let row), .attachment(let at, let row):
            if at == file { return row }
            return at < file ? 0 : rows - 1
        case .header(let at):
            return at <= file ? 0 : rows - 1
        case .footer:
            return rows - 1
        }
    }

    override func updateTrackingAreas() {
        super.updateTrackingAreas()
        for area in trackingAreas { removeTrackingArea(area) }
        addTrackingArea(NSTrackingArea(
            rect: .zero, options: [.mouseMoved, .mouseEnteredAndExited, .activeInKeyWindow, .inVisibleRect],
            owner: self))
    }

    override func mouseMoved(with event: NSEvent) {
        setHover(at: convert(event.locationInWindow, from: nil))
    }

    override func mouseExited(with event: NSEvent) {
        setHover(at: nil)
    }

    /// The pointer stood still while the rows moved under it.
    func refreshHover() {
        guard let window, canvas?.hoveredItem != nil || bounds.contains(convert(window.mouseLocationOutsideOfEventStream, from: nil))
        else { return }
        let point = convert(window.mouseLocationOutsideOfEventStream, from: nil)
        setHover(at: bounds.contains(point) ? point : nil)
    }

    private func setHover(at point: NSPoint?) {
        guard let canvas else { return }
        var hovered: Int?
        let found = point.flatMap { hit(at: $0) }
        if case .row(let file, let row)? = found {
            hovered = canvas.diffLayout.index(of: .row(file: file, row: row))
        } else if case .commentButton? = found {
            hovered = canvas.hoveredItem
        }
        // A gap redraws as the pointer crosses it, so the control under it
        // lights up.
        let overGap: Bool = if case .gap? = found { true } else { false }
        let wasOverGap = hoverPoint != nil
        hoverPoint = overGap ? point : nil
        if overGap || wasOverGap { needsDisplay = true }
        if hovered != canvas.hoveredItem {
            canvas.hoveredItem = hovered
            needsDisplay = true
        }
    }

    // MARK: - Copying

    /// ⌘C: the marked rows' text, one line apiece.
    @objc func copy(_ sender: Any?) {
        guard let canvas, let selection = canvas.state.selection,
              let file = canvas.files.first(where: { $0.path == selection.path }) else { return }
        let marked = Set(selection.rowIDs)
        let text = file.rows.filter { marked.contains($0.id) }.map(\.text).joined(separator: "\n")
        pasteboard.clearContents()
        pasteboard.setString(text, forType: .string)
    }

    /// Where ⌘C writes: the system's pasteboard, but for a test.
    var pasteboard = NSPasteboard.general
}

extension DiffCanvasView {
    /// A row's text set for drawing, from the cache or set now: syntax
    /// coloured, broken at the layout's columns.
    func lines(for index: Int, row: DiffRow) -> DiffRowLines {
        if let cached = lineCache[index] { return cached }
        if lineCache.count > 4000 { lineCache.removeAll(keepingCapacity: true) }
        let described = row.kind == .described
        let wrapped = DiffText.wrap(DiffSyntax.runs(row.text, tokens: row.tokens), limit: diffLayout.wrapColumns)
        var lines = DiffRowLines()
        lines.code = wrapped.map { runs in
            let text = NSMutableAttributedString()
            for run in runs {
                text.append(NSAttributedString(string: run.text, attributes: [
                    .font: fonts.code,
                    NSAttributedString.Key(kCTForegroundColorAttributeName as String):
                        DiffInk.color(for: run.kind, described: described).cgColor,
                ]))
            }
            return CTLineCreateWithAttributedString(text)
        }
        lines.number = row.newNumber.map { numberLine($0) }
        lineCache[index] = lines
        return lines
    }

    private func numberLine(_ number: Int) -> CTLine {
        CTLineCreateWithAttributedString(NSAttributedString(string: String(number), attributes: [
            .font: fonts.code,
            NSAttributedString.Key(kCTForegroundColorAttributeName as String): DiffInk.tertiary.cgColor,
        ]))
    }
}
