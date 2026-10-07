import AppKit
import SwiftUI
import XCTest
@testable import NatKit

@MainActor
final class ContextMenuRegionTests: XCTestCase {
    /// Stands in for SwiftUI's hosting view: answers a menu from
    /// `menu(for:)` and records each event it was asked with.
    private final class MenuAnsweringView: NSView {
        var answer: NSMenu?
        var asked: [NSEvent] = []

        override func menu(for event: NSEvent) -> NSMenu? {
            asked.append(event)
            return answer
        }
    }

    private var window: NSWindow!
    private var host: MenuAnsweringView!
    private var row: NSView!
    private var region: ContextMenuRegionView!
    private let rowMenu = NSMenu(title: "row")

    override func setUp() {
        super.setUp()
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 400, height: 300),
                          styleMask: [.titled], backing: .buffered, defer: false)
        window.isReleasedWhenClosed = false
        host = MenuAnsweringView(frame: NSRect(x: 0, y: 0, width: 400, height: 300))
        rowMenu.addItem(withTitle: "Launch agent", action: nil, keyEquivalent: "")
        host.answer = rowMenu
        // A row inside the host with no menu of its own, so the walk has to
        // go up to find the host's — as a click on a sidebar row's text does.
        row = NSView(frame: NSRect(x: 0, y: 200, width: 200, height: 20))
        host.addSubview(row)
        // The region covers the left half, as the sidebar does the window's.
        region = ContextMenuRegionView(frame: NSRect(x: 0, y: 0, width: 200, height: 300))
        host.addSubview(region, positioned: .below, relativeTo: row)
        window.contentView = host
    }

    override func tearDown() {
        region.removeFromSuperview()
        window.close()
        window = nil
        super.tearDown()
    }

    private func event(_ type: NSEvent.EventType, at point: NSPoint, flags: NSEvent.ModifierFlags = []) -> NSEvent {
        NSEvent.mouseEvent(with: type, location: point, modifierFlags: flags, timestamp: 0,
                           windowNumber: window.windowNumber, context: nil, eventNumber: 0,
                           clickCount: 1, pressure: 1)!
    }

    private func handle(_ event: NSEvent) -> (NSEvent?, [(NSMenu, NSEvent, NSView)]) {
        var presented: [(NSMenu, NSEvent, NSView)] = []
        let out = ContextMenuRegion.handle(event) { presented.append(($0, $1, $2)) }
        return (out, presented)
    }

    func testARightClickOnARowAsksMenuForAndPresentsWhatItAnswers() throws {
        let click = event(.rightMouseDown, at: NSPoint(x: 50, y: 210))

        let (out, presented) = handle(click)

        XCTAssertNil(out, "a click it presents for is consumed, so SwiftUI cannot present a second menu")
        XCTAssertEqual(host.asked.count, 1)
        XCTAssertTrue(host.asked.first === click)
        let only = try XCTUnwrap(presented.first)
        XCTAssertEqual(presented.count, 1)
        XCTAssertTrue(only.0 === rowMenu)
        XCTAssertTrue(only.1 === click)
        XCTAssertTrue(only.2 === host, "presented for the view that answered, so its items reach SwiftUI")
    }

    func testAControlClickIsPresentedTheSameWay() {
        let click = event(.leftMouseDown, at: NSPoint(x: 50, y: 210), flags: .control)

        let (out, presented) = handle(click)

        XCTAssertNil(out)
        XCTAssertEqual(presented.count, 1)
        XCTAssertTrue(presented.first?.0 === rowMenu)
    }

    func testAPlainLeftClickIsPassedOnUnasked() {
        let click = event(.leftMouseDown, at: NSPoint(x: 50, y: 210))

        let (out, presented) = handle(click)

        XCTAssertTrue(out === click)
        XCTAssertTrue(presented.isEmpty)
        XCTAssertTrue(host.asked.isEmpty)
    }

    func testARightClickOutsideTheRegionIsLeftToAppKit() {
        let click = event(.rightMouseDown, at: NSPoint(x: 300, y: 210))

        let (out, presented) = handle(click)

        XCTAssertTrue(out === click)
        XCTAssertTrue(presented.isEmpty)
        XCTAssertTrue(host.asked.isEmpty)
    }

    func testARightClickWhereNothingAnswersAMenuIsPassedOn() {
        host.answer = nil
        let click = event(.rightMouseDown, at: NSPoint(x: 50, y: 210))

        let (out, presented) = handle(click)

        XCTAssertTrue(out === click)
        XCTAssertTrue(presented.isEmpty)
    }

    func testARegionLeavingItsWindowStopsClaimingClicks() {
        region.removeFromSuperview()
        let click = event(.rightMouseDown, at: NSPoint(x: 50, y: 210))

        let (out, presented) = handle(click)

        XCTAssertTrue(out === click)
        XCTAssertTrue(presented.isEmpty)
    }

    func testARightClickInAnotherWindowIsNotThisRegions() {
        let other = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 400, height: 300),
                             styleMask: [.titled], backing: .buffered, defer: false)
        other.isReleasedWhenClosed = false
        defer { other.close() }
        let otherHost = MenuAnsweringView(frame: NSRect(x: 0, y: 0, width: 400, height: 300))
        otherHost.answer = rowMenu
        other.contentView = otherHost
        let click = NSEvent.mouseEvent(with: .rightMouseDown, location: NSPoint(x: 50, y: 210),
                                       modifierFlags: [], timestamp: 0, windowNumber: other.windowNumber,
                                       context: nil, eventNumber: 0, clickCount: 1, pressure: 1)!

        let (out, presented) = handle(click)

        XCTAssertTrue(out === click)
        XCTAssertTrue(presented.isEmpty)
    }

    func testTheRegionIsNeverHit() {
        XCTAssertNil(region.hitTest(NSPoint(x: 10, y: 10)))
    }

    func testTheRepresentableMakesARegionView() {
        let hosting = NSHostingView(rootView: Color.clear.frame(width: 50, height: 50).background(ContextMenuRegion()))
        window.contentView = hosting
        hosting.layoutSubtreeIfNeeded()

        XCTAssertTrue(ContextMenuRegion.regions.allObjects.contains { $0.isDescendant(of: hosting) })
    }

    func testInstallIsIdempotent() {
        ContextMenuRegion.install()
        ContextMenuRegion.install()
    }
}
