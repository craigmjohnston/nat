import AppKit
import SwiftUI
import XCTest
@testable import NatKit

/// The diff canvas, hosted in a real window and driven the way the pointer
/// drives it: what it lays out, where it scrolls to, and what each click
/// asks of the review.
@MainActor
final class DiffCanvasViewTests: XCTestCase {
    private var window: NSWindow?

    override func tearDown() async throws {
        await MainActor.run {
            window?.close()
            window = nil
        }
        try await super.tearDown()
    }

    // MARK: - Fixtures

    private func row(_ id: String, _ n: Int, _ text: String, kind: DiffRow.Kind = .context) -> DiffRow {
        switch kind {
        case .added: DiffRow(id: id, kind: .added, oldNumber: nil, newNumber: n, prefix: "+", text: text)
        case .removed: DiffRow(id: id, kind: .removed, oldNumber: n, newNumber: nil, prefix: "-", text: text)
        case .hunkBreak: DiffRow(id: id, kind: .hunkBreak, oldNumber: nil, newNumber: nil, prefix: nil, text: text)
        default: DiffRow(id: id, kind: kind, oldNumber: n, newNumber: n, prefix: " ", text: text,
                         tokens: [TokenRun(kind: .text, length: text.utf8.count)])
        }
    }

    /// Three files of forty rows apiece, every kind of row among them, the
    /// second renamed.
    private var files: [DiffFileModel] {
        (0..<3).map { f in
            let rows = (0..<40).map { i -> DiffRow in
                let id = "f\(f)#\(i)"
                switch i % 10 {
                case 3: return row(id, i, "added line \(i)", kind: .added)
                case 5: return row(id, i, "removed line \(i)", kind: .removed)
                case 7: return row(id, i, "@@ -\(i) +\(i) @@", kind: .hunkBreak)
                case 9: return row(id, i, String(repeating: "long line \(i) ", count: 20))
                default: return row(id, i, "\tcontext line \(i)")
                }
            }
            return DiffFileModel(
                path: "dir/file\(f).swift", oldPath: f == 1 ? "dir/old\(f).swift" : "dir/file\(f).swift",
                adds: 4, dels: 4, described: false, rows: rows)
        }
    }

    private func makeCanvas(
        width: CGFloat = 600, height: CGFloat = 300, state: DiffCanvasState = DiffCanvasState(),
        files: [DiffFileModel]? = nil
    ) -> DiffCanvasView {
        let canvas = DiffCanvasView(frame: NSRect(x: 0, y: 0, width: width, height: height))
        let window = NSWindow(
            contentRect: canvas.frame, styleMask: [.borderless], backing: .buffered, defer: false)
        window.isReleasedWhenClosed = false
        window.contentView = canvas
        self.window = window
        canvas.update(files: files ?? self.files, state: state)
        canvas.layoutSubtreeIfNeeded()
        return canvas
    }

    private func resize(_ canvas: DiffCanvasView, width: CGFloat) {
        canvas.setFrameSize(NSSize(width: width, height: canvas.frame.height))
        canvas.needsLayout = true
        canvas.layoutSubtreeIfNeeded()
    }

    /// The item at the top of the view.
    private func topItem(_ canvas: DiffCanvasView) -> DiffLayout.Item {
        canvas.diffLayout.items[canvas.diffLayout.index(at: canvas.scrollOffset.y)]
    }

    private func scroll(_ canvas: DiffCanvasView, toItem item: DiffLayout.Item, plus delta: CGFloat = 0) {
        let index = canvas.diffLayout.index(of: item)!
        canvas.scrollView.contentView.setBoundsOrigin(NSPoint(x: 0, y: canvas.diffLayout.top(index) + delta))
    }

    /// A point in the viewport, as the window reports a click there.
    private func event(
        _ type: NSEvent.EventType, at point: NSPoint, in canvas: DiffCanvasView, flags: NSEvent.ModifierFlags = []
    ) -> NSEvent {
        NSEvent.mouseEvent(
            with: type, location: canvas.viewport.convert(point, to: nil), modifierFlags: flags,
            timestamp: 0, windowNumber: window?.windowNumber ?? 0, context: nil, eventNumber: 0,
            clickCount: 1, pressure: 1)!
    }

    /// Where an item is drawn in the viewport, nudged inside it.
    private func point(of item: DiffLayout.Item, in canvas: DiffCanvasView, x: CGFloat = 200) -> NSPoint {
        let index = canvas.diffLayout.index(of: item)!
        return NSPoint(x: x, y: canvas.diffLayout.top(index) - canvas.scrollOffset.y + 5)
    }

    // MARK: - Laying out and scrolling

    func testTheDocumentIsExactlyAsTallAsTheLayout() {
        let canvas = makeCanvas()
        XCTAssertGreaterThan(canvas.diffLayout.totalHeight, 300)
        XCTAssertEqual(canvas.document.frame.height, canvas.diffLayout.totalHeight)
        XCTAssertEqual(canvas.document.frame.width, 600)
        XCTAssertEqual(canvas.viewport.frame, canvas.scrollView.contentView.bounds)
        XCTAssertEqual(canvas.metrics.numberDigits, 2)
    }

    func testAJumpToAFileLandsExactlyOnItsHeader() {
        let canvas = makeCanvas()
        canvas.scrollToFile("dir/file1.swift", animated: false)
        XCTAssertEqual(canvas.scrollOffset.y, canvas.diffLayout.top(canvas.diffLayout.headerIndex[1]))
        XCTAssertEqual(topItem(canvas), .header(file: 1))
        // The viewport follows the scroll it was moved by.
        XCTAssertEqual(canvas.viewport.frame.minY, canvas.scrollOffset.y)

        canvas.scrollToFile("dir/file2.swift", animated: false)
        XCTAssertEqual(topItem(canvas), .header(file: 2))

        // A header too near the end to reach the top: as near as the diff
        // can scroll.
        var state = canvas.state
        state.collapsed = ["dir/file2.swift"]
        canvas.update(files: files, state: state)
        canvas.scrollToFile("dir/file2.swift", animated: false)
        XCTAssertEqual(canvas.scrollOffset.y, canvas.diffLayout.totalHeight - 300)

        // A path that is not in the diff moves nothing.
        canvas.scrollToFile("nowhere.swift", animated: false)
        XCTAssertEqual(canvas.scrollOffset.y, canvas.diffLayout.totalHeight - 300)
    }

    func testAJumpAskedBeforeTheFirstLayoutWaitsForIt() {
        let canvas = DiffCanvasView(frame: NSRect(x: 0, y: 0, width: 600, height: 300))
        canvas.update(files: files, state: DiffCanvasState())
        canvas.scrollToFile("dir/file1.swift")
        canvas.layoutSubtreeIfNeeded()
        XCTAssertEqual(topItem(canvas), .header(file: 1))
    }

    func testAWidthChangeKeepsTheSameRowAtTheTop() {
        let canvas = makeCanvas()
        scroll(canvas, toItem: .row(file: 1, row: 19), plus: 10)
        XCTAssertEqual(topItem(canvas), .row(file: 1, row: 19))
        let wrapped = canvas.diffLayout.height(canvas.diffLayout.index(of: .row(file: 1, row: 19))!)

        resize(canvas, width: 360)
        XCTAssertEqual(topItem(canvas), .row(file: 1, row: 19))
        XCTAssertGreaterThan(canvas.diffLayout.height(canvas.diffLayout.index(of: .row(file: 1, row: 19))!), wrapped)
        XCTAssertEqual(canvas.document.frame.height, canvas.diffLayout.totalHeight)
    }

    func testFoldingTheFilePinnedAtTheTopKeepsItsHeaderThere() {
        let canvas = makeCanvas()
        scroll(canvas, toItem: .row(file: 1, row: 30))
        var state = canvas.state
        state.collapsed = ["dir/file1.swift"]
        canvas.update(files: files, state: state)
        XCTAssertEqual(topItem(canvas), .header(file: 1))
        XCTAssertEqual(canvas.scrollOffset.y, canvas.diffLayout.top(canvas.diffLayout.headerIndex[1]))
    }

    func testFoldingAnotherFileKeepsTheRowAtTheTop() {
        let canvas = makeCanvas()
        scroll(canvas, toItem: .row(file: 1, row: 10))
        var state = canvas.state
        state.collapsed = ["dir/file0.swift"]
        canvas.update(files: files, state: state)
        XCTAssertEqual(topItem(canvas), .row(file: 1, row: 10))
    }

    func testANewDiffWithoutTheTopRowStartsAtTheTop() {
        let canvas = makeCanvas()
        scroll(canvas, toItem: .row(file: 2, row: 10))
        let other = [DiffFileModel(path: "else.swift", oldPath: "else.swift", adds: 1, dels: 0, described: false,
                                   rows: [row("e#1", 1, "x", kind: .added)])]
        canvas.update(files: other, state: canvas.state)
        XCTAssertEqual(canvas.scrollOffset.y, 0)
        XCTAssertEqual(canvas.metrics.numberDigits, 1)
    }

    func testUnwrappedTheDiffScrollsSideways() {
        var state = DiffCanvasState()
        state.wrap = false
        let canvas = makeCanvas(state: state)
        XCTAssertNil(canvas.diffLayout.wrapColumns)
        XCTAssertGreaterThan(canvas.document.frame.width, 600)
        canvas.scrollView.contentView.setBoundsOrigin(NSPoint(x: 120, y: 0))
        XCTAssertEqual(canvas.scrollOffset.x, 120)
        XCTAssertEqual(canvas.viewport.frame.minX, 120)

        // Wrapping again lays out to the width and brings the view back.
        state.wrap = true
        canvas.update(files: files, state: state)
        XCTAssertEqual(canvas.document.frame.width, 600)
        XCTAssertEqual(canvas.scrollOffset.x, 0)
    }

    func testAnUnchangedLayoutOnlyRedraws() {
        let canvas = makeCanvas()
        let tops = canvas.diffLayout.tops
        var state = canvas.state
        state.selection = DiffCanvasSelection(path: "dir/file0.swift", rowIDs: ["f0#1"])
        state.viewed = ["dir/file0.swift"]
        canvas.update(files: files, state: state)
        XCTAssertEqual(canvas.diffLayout.tops, tops)
        XCTAssertTrue(canvas.viewport.needsDisplay)
    }

    // MARK: - Attachments

    func testAnAttachmentIsLaidOutUnderItsRowAndFollowsTheScroll() {
        let canvas = makeCanvas()
        var height: CGFloat = 40
        canvas.measureAttachment = { _, _ in height }
        let card = NSView()
        let key = DiffLayout.key(path: "dir/file0.swift", rowID: "f0#2")
        canvas.setAttachments([key: card])

        let index = canvas.diffLayout.index(of: .attachment(file: 0, row: 2))!
        XCTAssertEqual(canvas.diffLayout.height(index), 40)
        XCTAssertFalse(card.isHidden)
        XCTAssertEqual(card.frame.minY, canvas.diffLayout.top(index))
        XCTAssertEqual(card.frame.minX, canvas.attachmentInset)
        XCTAssertEqual(card.frame.width, 600 - canvas.attachmentInset)

        // Scrolled well past it, it is hidden.
        canvas.scrollToFile("dir/file2.swift", animated: false)
        XCTAssertTrue(card.isHidden)

        // Grown, it is measured and laid out again on the next turn.
        height = 90
        canvas.attachmentDidResize(key)
        let grown = expectation(description: "relaid out")
        DispatchQueue.main.async { grown.fulfill() }
        wait(for: [grown], timeout: 1)
        XCTAssertEqual(canvas.diffLayout.height(canvas.diffLayout.index(of: .attachment(file: 0, row: 2))!), 90)

        // Taken away, so is its room.
        canvas.setAttachments([:])
        XCTAssertNil(card.superview)
        XCTAssertNil(canvas.diffLayout.index(of: .attachment(file: 0, row: 2)))
    }

    func testAnAttachmentOnAFoldedOrMissingRowIsHidden() {
        var state = DiffCanvasState()
        state.collapsed = ["dir/file0.swift"]
        let canvas = makeCanvas(state: state)
        canvas.measureAttachment = { _, _ in 30 }
        let folded = NSView(), stray = NSView()
        canvas.setAttachments([
            DiffLayout.key(path: "dir/file0.swift", rowID: "f0#2"): folded,
            DiffLayout.key(path: "gone.swift", rowID: "x"): stray,
        ])
        XCTAssertTrue(folded.isHidden)
        XCTAssertTrue(stray.isHidden)
        // A resize said of a key it does not hold is nothing to do.
        canvas.attachmentDidResize("unknown")
    }

    func testWithoutAMeasureAnAttachmentIsItsFittingHeight() {
        let canvas = makeCanvas()
        let card = NSView()
        canvas.setAttachments([DiffLayout.key(path: "dir/file0.swift", rowID: "f0#2"): card])
        XCTAssertEqual(canvas.diffLayout.height(canvas.diffLayout.index(of: .attachment(file: 0, row: 2))!), 0)
    }

    // MARK: - The file header

    /// The tally sits against the trailing edge — just inside the viewed
    /// toggle where there is one — and the path's room stops short of it;
    /// the rename note stays after the path.
    func testTheTallyIsRightAlignedInsideTheViewedToggle() throws {
        let canvas = makeCanvas()
        let renamed = canvas.viewport.headerLayout(canvas, file: 1, y: 0)
        let viewed = try XCTUnwrap(renamed.viewed)
        XCTAssertEqual(renamed.tallyText, "+4 \u{2212}4")
        XCTAssertEqual(renamed.tally.maxX, viewed.minX - 8)
        XCTAssertLessThanOrEqual(renamed.path.maxX, renamed.tally.minX - 8)
        XCTAssertEqual(renamed.extras.map(\.0), ["was dir/old1.swift"])

        var state = DiffCanvasState()
        state.showsViewed = false
        let plain = makeCanvas(state: state)
        let layout = plain.viewport.headerLayout(plain, file: 0, y: 0)
        XCTAssertNil(layout.viewed)
        XCTAssertEqual(layout.tally.maxX, plain.viewport.bounds.width - 16, "against the trailing edge")
        XCTAssertEqual(layout.extras.count, 0)
    }

    // MARK: - What is under the pointer

    func testHitTesting() {
        var state = DiffCanvasState()
        state.canComment = true
        let canvas = makeCanvas(state: state)
        let viewport = canvas.viewport
        XCTAssertEqual(viewport.hit(at: point(of: .header(file: 0), in: canvas)), .header(file: 0, viewed: false))
        XCTAssertEqual(viewport.hit(at: point(of: .header(file: 0), in: canvas, x: 560)), .header(file: 0, viewed: true))
        XCTAssertEqual(viewport.hit(at: point(of: .row(file: 0, row: 1), in: canvas)), .row(file: 0, row: 1))

        // A row under the pointer shows the comment button, and the button
        // takes the click.
        viewport.mouseMoved(with: event(.mouseMoved, at: point(of: .row(file: 0, row: 1), in: canvas), in: canvas))
        XCTAssertEqual(canvas.hoveredItem, canvas.diffLayout.index(of: .row(file: 0, row: 1)))
        XCTAssertEqual(
            viewport.hit(at: point(of: .row(file: 0, row: 1), in: canvas, x: 580)),
            .commentButton(file: 0, row: 1, endsSelection: false))
        // (AppKit builds no entered/exited event this way; the handler reads
        // nothing off it.)
        viewport.mouseExited(with: event(.mouseMoved, at: .zero, in: canvas))
        XCTAssertNil(canvas.hoveredItem)

        // Scrolled into file 1, its header is pinned over whatever is at
        // the top.
        scroll(canvas, toItem: .row(file: 1, row: 20))
        XCTAssertEqual(viewport.hit(at: NSPoint(x: 200, y: 5)), .header(file: 1, viewed: false))
        canvas.scrollToFile("dir/file2.swift", animated: false)
        let footer = canvas.diffLayout.index(of: .footer)!
        XCTAssertNil(viewport.hit(at: NSPoint(x: 200, y: canvas.diffLayout.top(footer) - canvas.scrollOffset.y + 5)))
    }

    func testTheCommentButtonOnTheEndOfAMarkedRun() {
        var state = DiffCanvasState()
        state.canComment = true
        state.selection = DiffCanvasSelection(path: "dir/file0.swift", rowIDs: ["f0#1", "f0#2"])
        let canvas = makeCanvas(state: state)
        XCTAssertEqual(
            canvas.viewport.hit(at: point(of: .row(file: 0, row: 2), in: canvas, x: 580)),
            .commentButton(file: 0, row: 2, endsSelection: true))
        // Off while a comment is being written.
        state.canComment = false
        canvas.update(files: files, state: state)
        XCTAssertEqual(canvas.viewport.hit(at: point(of: .row(file: 0, row: 2), in: canvas, x: 580)), .row(file: 0, row: 2))
    }

    // MARK: - Clicks

    private final class Asked {
        var clicked: [(String, Bool)] = []
        var dragged: [[String]] = []
        var commented: [(String, Bool)] = []
        var viewed: [String] = []
        var collapsed: [String] = []
    }

    private func wire(_ canvas: DiffCanvasView) -> Asked {
        let asked = Asked()
        var actions = DiffCanvasActions()
        actions.rowClicked = { _, row, shift in asked.clicked.append((row.id, shift)) }
        actions.rowsDragged = { _, ids in asked.dragged.append(ids) }
        actions.commentRequested = { _, row, ends in asked.commented.append((row.id, ends)) }
        actions.viewedToggled = { asked.viewed.append($0) }
        actions.collapseToggled = { asked.collapsed.append($0) }
        canvas.actions = actions
        return asked
    }

    func testEachClickAsksForItsAction() {
        var state = DiffCanvasState()
        state.canComment = true
        let canvas = makeCanvas(state: state)
        let asked = wire(canvas)
        let viewport = canvas.viewport

        viewport.mouseDown(with: event(.leftMouseDown, at: point(of: .header(file: 0), in: canvas), in: canvas))
        viewport.mouseDown(with: event(.leftMouseDown, at: point(of: .header(file: 0), in: canvas, x: 560), in: canvas))
        viewport.mouseDown(with: event(.leftMouseDown, at: point(of: .row(file: 0, row: 1), in: canvas), in: canvas))
        viewport.mouseUp(with: event(.leftMouseUp, at: point(of: .row(file: 0, row: 1), in: canvas), in: canvas))
        viewport.mouseDown(with: event(
            .leftMouseDown, at: point(of: .row(file: 0, row: 2), in: canvas), in: canvas, flags: .shift))
        // A hunk break takes no click.
        viewport.mouseDown(with: event(.leftMouseDown, at: point(of: .row(file: 0, row: 7), in: canvas), in: canvas))
        // Nor does the closing line, at the very bottom.
        canvas.scrollView.contentView.setBoundsOrigin(NSPoint(x: 0, y: canvas.diffLayout.totalHeight - 300))
        viewport.mouseDown(with: event(.leftMouseDown, at: NSPoint(x: 200, y: 295), in: canvas))
        canvas.scrollView.contentView.setBoundsOrigin(.zero)
        viewport.mouseMoved(with: event(.mouseMoved, at: point(of: .row(file: 0, row: 4), in: canvas), in: canvas))
        viewport.mouseDown(with: event(.leftMouseDown, at: point(of: .row(file: 0, row: 4), in: canvas, x: 580), in: canvas))

        XCTAssertEqual(asked.collapsed, ["dir/file0.swift"])
        XCTAssertEqual(asked.viewed, ["dir/file0.swift"])
        XCTAssertEqual(asked.clicked.map(\.0), ["f0#1", "f0#2"])
        XCTAssertEqual(asked.clicked.map(\.1), [false, true])
        XCTAssertEqual(asked.commented.map(\.0), ["f0#4"])
        XCTAssertTrue(window?.firstResponder === viewport)
    }

    /// A file with a gap between two changes, a line, and a small gap.
    private var gappedFiles: [DiffFileModel] {
        let between = DiffRow(
            id: "g1", kind: .hunkBreak, oldNumber: nil, newNumber: nil, prefix: nil, text: "@@ -1 +1 @@",
            gap: DiffGap(first: 10, last: 90, oldOffset: 0))
        let small = DiffRow(
            id: "g2", kind: .hunkBreak, oldNumber: nil, newNumber: nil, prefix: nil, text: "",
            gap: DiffGap(first: 92, last: 95, oldOffset: 0))
        return [DiffFileModel(
            path: "a.swift", oldPath: "a.swift", adds: 1, dels: 0, described: false,
            rows: [between, row("r", 91, "line"), small, row("x", 96, "added", kind: .added)])]
    }

    func testAGapsControlsEachRevealTheirOwnWay() throws {
        let canvas = makeCanvas(files: gappedFiles)
        var pressed: [(String, DiffGap.Control)] = []
        var actions = DiffCanvasActions()
        actions.gapExpanded = { file, gap, control in pressed.append(("\(file.path)@\(gap.first)", control)) }
        actions.rowClicked = { _, _, _ in XCTFail("a gap is no row to mark") }
        canvas.actions = actions
        let viewport = canvas.viewport
        let rowHeight = canvas.metrics.rowMinHeight

        let top = point(of: .row(file: 0, row: 0), in: canvas, x: 10)
        viewport.mouseDown(with: event(.leftMouseDown, at: top, in: canvas))
        viewport.mouseDown(with: event(
            .leftMouseDown, at: NSPoint(x: 300, y: top.y + rowHeight), in: canvas))
        viewport.mouseDown(with: event(.leftMouseDown, at: point(of: .row(file: 0, row: 2), in: canvas), in: canvas))

        XCTAssertEqual(pressed.map(\.0), ["a.swift@10", "a.swift@10", "a.swift@92"])
        XCTAssertEqual(pressed.map(\.1), [.down, .up, .all])

        // The control under the pointer lights up, and goes out as it leaves.
        viewport.mouseMoved(with: event(.mouseMoved, at: top, in: canvas))
        try assertDraws(canvas)
        viewport.mouseMoved(with: event(.mouseMoved, at: point(of: .row(file: 0, row: 1), in: canvas), in: canvas))
        try assertDraws(canvas)
    }

    func testADragMarksTheRunFromWhereItBegan() {
        let canvas = makeCanvas(height: 900)
        let asked = wire(canvas)
        let viewport = canvas.viewport
        viewport.mouseDown(with: event(.leftMouseDown, at: point(of: .row(file: 0, row: 4), in: canvas), in: canvas))
        viewport.mouseDragged(with: event(.leftMouseDragged, at: point(of: .row(file: 0, row: 6), in: canvas), in: canvas))
        // The same row again asks nothing new.
        viewport.mouseDragged(with: event(.leftMouseDragged, at: point(of: .row(file: 0, row: 6), in: canvas), in: canvas))
        // Back up past the start, the run turns round on it.
        viewport.mouseDragged(with: event(.leftMouseDragged, at: point(of: .row(file: 0, row: 2), in: canvas), in: canvas))
        // Up onto the file's own header: its first row.
        viewport.mouseDragged(with: event(.leftMouseDragged, at: point(of: .header(file: 0), in: canvas), in: canvas))
        // Down into the next file: this one's last row.
        viewport.mouseDragged(with: event(.leftMouseDragged, at: point(of: .row(file: 1, row: 1), in: canvas), in: canvas))
        viewport.mouseUp(with: event(.leftMouseUp, at: .zero, in: canvas))
        // After the button is up, a drag is no drag.
        viewport.mouseDragged(with: event(.leftMouseDragged, at: point(of: .row(file: 0, row: 3), in: canvas), in: canvas))

        XCTAssertEqual(asked.dragged, [
            ["f0#4", "f0#5", "f0#6"],
            ["f0#2", "f0#3", "f0#4"],
            ["f0#0", "f0#1", "f0#2", "f0#3", "f0#4"],
            (4...39).map { "f0#\($0)" },
        ])
    }

    func testADragFromTheNextFileUpStopsAtItsFirstRow() {
        let canvas = makeCanvas(height: 900)
        let asked = wire(canvas)
        scroll(canvas, toItem: .header(file: 1), plus: -400)
        let viewport = canvas.viewport
        viewport.mouseDown(with: event(.leftMouseDown, at: point(of: .row(file: 1, row: 2), in: canvas), in: canvas))
        viewport.mouseDragged(with: event(.leftMouseDragged, at: point(of: .row(file: 0, row: 38), in: canvas), in: canvas))
        XCTAssertEqual(asked.dragged, [["f1#0", "f1#1", "f1#2"]])
    }

    func testCopyWritesTheMarkedRowsText() {
        var state = DiffCanvasState()
        state.selection = DiffCanvasSelection(path: "dir/file0.swift", rowIDs: ["f0#3", "f0#4"])
        let canvas = makeCanvas(state: state)
        let pasteboard = NSPasteboard(name: NSPasteboard.Name("DiffCanvasViewTests-\(UUID())"))
        defer { pasteboard.releaseGlobally() }
        canvas.viewport.pasteboard = pasteboard
        canvas.viewport.copy(nil)
        XCTAssertEqual(pasteboard.string(forType: .string), "added line 3\n\tcontext line 4")
    }

    // MARK: - Knobs

    func testDraggingTheKnobScrolls() {
        let canvas = makeCanvas()
        canvas.knobDrag(.vertical, translation: 0, ended: false)
        canvas.knobDrag(.vertical, translation: 40, ended: false)
        XCTAssertGreaterThan(canvas.scrollOffset.y, 40)
        canvas.knobDrag(.vertical, translation: 0, ended: true)
        canvas.knobHovered(true)
        canvas.knobHovered(false)
        canvas.mouseEntered(with: event(.mouseMoved, at: .zero, in: canvas))
        canvas.mouseExited(with: event(.mouseMoved, at: .zero, in: canvas))
    }

    func testTheKnobFadesAfterItsPauseAndCanBeDraggedByHand() {
        let canvas = makeCanvas()
        canvas.knobLinger = .milliseconds(10)
        canvas.lightKnobs()
        XCTAssertTrue(canvas.knobsLit)
        let faded = expectation(description: "faded")
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) { faded.fulfill() }
        wait(for: [faded], timeout: 2)
        XCTAssertFalse(canvas.knobsLit)

        // The knob's own drag, measured in the canvas.
        let knob = canvas.verticalKnob
        knob.mouseDown(with: event(.leftMouseDown, at: NSPoint(x: 595, y: 10), in: canvas))
        knob.mouseDragged(with: event(.leftMouseDragged, at: NSPoint(x: 595, y: 60), in: canvas))
        XCTAssertGreaterThan(canvas.scrollOffset.y, 50)
        knob.mouseUp(with: event(.leftMouseUp, at: NSPoint(x: 595, y: 60), in: canvas))
        XCTAssertTrue(canvas.knobsLit)

        // A change of the system's scroller style is taken up.
        NotificationCenter.default.post(name: NSScroller.preferredScrollerStyleDidChangeNotification, object: nil)
        XCTAssertFalse(DiffDocumentView.isCompatibleWithResponsiveScrolling)
    }

    func testAnAnimatedJumpEndsOnTheHeader() {
        let canvas = makeCanvas()
        canvas.scrollToFile("dir/file1.swift")
        let landed = expectation(description: "landed")
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) { landed.fulfill() }
        wait(for: [landed], timeout: 2)
        XCTAssertEqual(canvas.scrollOffset.y, canvas.diffLayout.top(canvas.diffLayout.headerIndex[1]))
    }

    func testAnAttachmentOrTheClosingLineAtTheTopIsKeptThroughAResize() {
        let canvas = makeCanvas()
        canvas.measureAttachment = { _, _ in 120 }
        canvas.setAttachments([DiffLayout.key(path: "dir/file1.swift", rowID: "f1#2"): NSView()])
        scroll(canvas, toItem: .attachment(file: 1, row: 2), plus: 30)
        resize(canvas, width: 420)
        XCTAssertEqual(topItem(canvas), .attachment(file: 1, row: 2))

        // The end of the diff, which has no anchor of its own, is wherever
        // the end is once laid out again.
        canvas.scrollView.contentView.setBoundsOrigin(NSPoint(x: 0, y: canvas.diffLayout.totalHeight - 300))
        resize(canvas, width: 600)
        XCTAssertLessThanOrEqual(canvas.scrollOffset.y, canvas.diffLayout.totalHeight - 300)
    }

    func testAHeaderBelowTheTopIsClickedWhereItIs() {
        let canvas = makeCanvas(height: 1400)
        let asked = wire(canvas)
        canvas.viewport.mouseDown(with: event(.leftMouseDown, at: point(of: .header(file: 1), in: canvas), in: canvas))
        XCTAssertEqual(asked.collapsed, ["dir/file1.swift"])
    }

    func testADragDownIntoTheClosingLineReachesTheLastRow() {
        let canvas = makeCanvas(height: 900)
        let asked = wire(canvas)
        canvas.scrollView.contentView.setBoundsOrigin(NSPoint(x: 0, y: canvas.diffLayout.totalHeight - 900))
        let viewport = canvas.viewport
        viewport.mouseDown(with: event(.leftMouseDown, at: point(of: .row(file: 2, row: 36), in: canvas), in: canvas))
        viewport.mouseDragged(with: event(.leftMouseDragged, at: NSPoint(x: 200, y: 895), in: canvas))
        XCTAssertEqual(asked.dragged, [(36...39).map { "f2#\($0)" }])
    }

    func testThePointerOnTheCommentButtonKeepsItsRowHovered() {
        var state = DiffCanvasState()
        state.canComment = true
        let canvas = makeCanvas(state: state)
        let row = point(of: .row(file: 0, row: 1), in: canvas)
        canvas.viewport.mouseMoved(with: event(.mouseMoved, at: row, in: canvas))
        let hovered = canvas.hoveredItem
        canvas.viewport.mouseMoved(with: event(.mouseMoved, at: NSPoint(x: 580, y: row.y), in: canvas))
        XCTAssertEqual(canvas.hoveredItem, hovered)
        XCTAssertTrue(canvas.viewport.acceptsFirstResponder)
    }

    // MARK: - Drawing

    /// Every part of the diff drawn at least once: wrapped and not, folded,
    /// marked, hovered, viewed, with the comment button and a pinned header
    /// being pushed off by the next — none of which may come out blank.
    func testEveryStateDraws() throws {
        var state = DiffCanvasState()
        state.canComment = true
        state.viewed = ["dir/file1.swift"]
        state.commentCounts = ["dir/file1.swift": 2]
        state.selection = DiffCanvasSelection(path: "dir/file1.swift", rowIDs: ["f1#3", "f1#4", "f1#5"])
        let canvas = makeCanvas(state: state)
        let header = canvas.diffLayout.top(canvas.diffLayout.headerIndex[1])
        canvas.scrollView.contentView.setBoundsOrigin(NSPoint(x: 0, y: header - 15))
        try assertDraws(canvas)

        state.wrap = false
        state.collapsed = ["dir/file0.swift"]
        canvas.update(files: files, state: state)
        canvas.viewportMoved()
        canvas.viewport.viewDidChangeEffectiveAppearance()
        try assertDraws(canvas)

        // The closing line, and an attachment's room left for its view.
        canvas.measureAttachment = { _, _ in 30 }
        canvas.setAttachments([DiffLayout.key(path: "dir/file2.swift", rowID: "f2#38"): NSView()])
        canvas.scrollView.contentView.setBoundsOrigin(NSPoint(x: 0, y: canvas.diffLayout.totalHeight - 300))
        try assertDraws(canvas)

        canvas.update(files: [], state: DiffCanvasState())
        try assertDraws(canvas)
    }

    /// A later file's header pinned at the very top draws no rule along its
    /// top edge: the pane heading's own line is already there, and a second
    /// one under it reads as a double border.
    func testAPinnedHeaderAtTheTopDrawsNoTopRule() throws {
        let canvas = makeCanvas()
        let header = canvas.diffLayout.top(canvas.diffLayout.headerIndex[1])
        canvas.scrollView.contentView.setBoundsOrigin(NSPoint(x: 0, y: header + 10))
        let viewport = canvas.viewport
        let rep = try XCTUnwrap(viewport.bitmapImageRepForCachingDisplay(in: viewport.bounds))
        viewport.cacheDisplay(in: viewport.bounds, to: rep)
        let x = rep.pixelsWide - 2
        XCTAssertEqual(rep.colorAt(x: x, y: 0), rep.colorAt(x: x, y: 6))
    }

    /// A fling through a 50,000-row diff, every frame drawn afresh: a
    /// hundred viewports, each a thousand points further down, as many
    /// rows set and drawn for the first time as a scroll can ask for.
    func testScrollingAHugeDiffDrawsEachFrameQuickly() throws {
        let line = "    let value = compute(input, label: \"a line long enough to wrap once in the pane\")"
        let huge = (0..<200).map { f in
            DiffFileModel(path: "f\(f).swift", oldPath: "f\(f).swift", adds: 0, dels: 0, described: false,
                          rows: (0..<250).map { row("\(f)#\($0)", $0, line) })
        }
        let canvas = makeCanvas(width: 700, height: 900)
        canvas.update(files: huge, state: DiffCanvasState())
        let viewport = canvas.viewport
        let rep = try XCTUnwrap(viewport.bitmapImageRepForCachingDisplay(in: viewport.bounds))
        measure {
            for step in 0..<100 {
                canvas.scrollView.contentView.setBoundsOrigin(NSPoint(x: 0, y: CGFloat(step) * 1000))
                viewport.cacheDisplay(in: viewport.bounds, to: rep)
            }
        }
    }

    private func assertDraws(_ canvas: DiffCanvasView) throws {
        let viewport = canvas.viewport
        let rep = try XCTUnwrap(viewport.bitmapImageRepForCachingDisplay(in: viewport.bounds))
        viewport.cacheDisplay(in: viewport.bounds, to: rep)
        XCTAssertGreaterThan(rep.pixelsWide, 0)
    }
}
