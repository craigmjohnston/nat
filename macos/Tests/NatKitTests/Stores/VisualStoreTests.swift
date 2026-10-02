import AppKit
import XCTest
@testable import NatKit
import NatFixtures

@MainActor
final class VisualStoreTests: XCTestCase {
    private let slice = "slice-1"
    private let wide = VisualChange(index: 1, name: "Wide", uri: "/tmp/wide.png")
    private let tall = VisualChange(index: 2, name: "Tall", uri: "/tmp/tall.png")
    private let remote = VisualChange(index: 3, name: "Remote", uri: "https://example.com/x.png")

    func testTheAppKeepsOneStorePerProject() {
        let appModel = Fixtures.appModel()
        XCTAssertTrue(appModel.visualStore(projectID: "a") === appModel.visualStore(projectID: "a"))
        XCTAssertFalse(appModel.visualStore(projectID: "a") === appModel.visualStore(projectID: "b"))
    }

    // MARK: - Images

    func testTheLoaderSeamLoadsEachURIOnceAndPublishesThemTogether() async {
        let store = VisualStore(client: FixtureNatClient())
        let counter = Counter()
        let image = NSImage(size: CGSize(width: 4, height: 2))
        store.loader = { uri in
            counter.bump()
            return uri.hasPrefix("/") ? .image(image, pixelSize: CGSize(width: 40, height: 20)) : .unavailable
        }
        XCTAssertFalse(store.isLoaded([wide]))
        XCTAssertNil(store.image(for: wide.uri))

        await store.load(sliceID: slice, visuals: [wide, remote, wide])
        XCTAssertTrue(store.isLoaded([wide, remote]))
        XCTAssertEqual(store.image(for: wide.uri)?.pixelSize, CGSize(width: 40, height: 20))
        XCTAssertEqual(store.image(for: remote.uri), .unavailable)
        XCTAssertNil(store.image(for: remote.uri)?.pixelSize)
        XCTAssertEqual(counter.count, 2, "a URI given twice is read once")

        await store.load(sliceID: slice, visuals: [wide, remote])
        XCTAssertEqual(counter.count, 2, "nothing already loaded is read again")
    }

    func testTheFileLoaderReadsLocalPathsAndNothingElse() throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: dir) }
        let rep = try XCTUnwrap(NSBitmapImageRep(
            bitmapDataPlanes: nil, pixelsWide: 30, pixelsHigh: 10, bitsPerSample: 8, samplesPerPixel: 4,
            hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0))
        let png = dir.appendingPathComponent("shot.png")
        try XCTUnwrap(rep.representation(using: .png, properties: [:])).write(to: png)
        let junk = dir.appendingPathComponent("junk.png")
        try Data("not an image".utf8).write(to: junk)

        XCTAssertEqual(VisualStore.fileLoader(png.path).pixelSize, CGSize(width: 30, height: 10))
        XCTAssertEqual(VisualStore.fileLoader(png.absoluteString).pixelSize, CGSize(width: 30, height: 10))
        XCTAssertEqual(VisualStore.fileLoader(junk.path), .unavailable)
        XCTAssertEqual(VisualStore.fileLoader(dir.appendingPathComponent("gone.png").path), .unavailable)
        XCTAssertEqual(VisualStore.fileLoader("https://example.com/x.png"), .unavailable, "no network")
        XCTAssertEqual(VisualStore.fileLoader("relative/x.png"), .unavailable)
        XCTAssertEqual(VisualStore.fileLoader("file://"), .unavailable)
    }

    func testImagesCompareByIdentity() {
        let a = NSImage(size: CGSize(width: 1, height: 1)), b = NSImage(size: CGSize(width: 1, height: 1))
        let size = CGSize(width: 1, height: 1)
        XCTAssertEqual(VisualImage.image(a, pixelSize: size), .image(a, pixelSize: size))
        XCTAssertNotEqual(VisualImage.image(a, pixelSize: size), .image(b, pixelSize: size))
        XCTAssertNotEqual(VisualImage.image(a, pixelSize: size), .unavailable)
    }

    // MARK: - Zoom

    func testZoomIsPerImageAndClamped() {
        let store = VisualStore(client: FixtureNatClient())
        XCTAssertEqual(store.zoom(sliceID: slice, index: 1), 1)

        store.zoomIn(sliceID: slice, index: 1)
        XCTAssertEqual(store.zoom(sliceID: slice, index: 1), 1.25)
        XCTAssertEqual(store.zoom(sliceID: slice, index: 2), 1, "zooming one image never moves another")
        XCTAssertEqual(store.zoom(sliceID: "other", index: 1), 1, "nor the same index on another slice")

        store.zoomOut(sliceID: slice, index: 2)
        XCTAssertEqual(store.zoom(sliceID: slice, index: 2), 0.8)
        XCTAssertEqual(store.zoom(sliceID: slice, index: 1), 1.25)

        store.setZoom(100, sliceID: slice, index: 1)
        XCTAssertEqual(store.zoom(sliceID: slice, index: 1), VisualStore.maxZoom)
        store.setZoom(0.01, sliceID: slice, index: 1)
        XCTAssertEqual(store.zoom(sliceID: slice, index: 1), VisualStore.minZoom)

        store.resetZoom(sliceID: slice, index: 1)
        XCTAssertEqual(store.zoom(sliceID: slice, index: 1), 1)
        XCTAssertEqual(store.zoom(sliceID: slice, index: 2), 0.8)
    }

    // MARK: - Comments

    private func set(_ store: VisualStore, _ visual: VisualChange, _ point: CGPoint?, _ text: String, id: UUID? = nil)
        -> PendingVisualComment? {
        store.setComment(sliceID: slice, id: id, visual: visual, point: point, imageSize: CGSize(width: 100, height: 50), text: text)
    }

    func testCommentsAreOrderedByImageThenWholeImageThenPosition() {
        let store = VisualStore(client: FixtureNatClient())
        set(store, tall, CGPoint(x: 1, y: 1), "tall point")
        set(store, wide, CGPoint(x: 50, y: 20), "lower right")
        set(store, wide, CGPoint(x: 10, y: 20), "lower left")
        set(store, wide, CGPoint(x: 90, y: 5), "upper")
        set(store, wide, nil, "whole")
        set(store, wide, nil, "whole again")
        XCTAssertEqual(store.comments(for: slice).map(\.text),
                       ["whole", "whole again", "upper", "lower left", "lower right", "tall point"])
        XCTAssertEqual(store.comments(for: "other"), [])
    }

    func testACommentIsReplacedByIDAndRemovedByEmptyText() throws {
        let store = VisualStore(client: FixtureNatClient())
        let made = try XCTUnwrap(set(store, wide, CGPoint(x: 3, y: 4), "  first  "))
        XCTAssertEqual(made.text, "first")
        XCTAssertEqual(made.placement, "at (3, 4)")

        let edited = set(store, wide, CGPoint(x: 3, y: 4), "second", id: made.id)
        XCTAssertEqual(edited?.id, made.id)
        XCTAssertEqual(store.comments(for: slice).map(\.text), ["second"])

        XCTAssertNil(set(store, wide, nil, "   "), "an empty new comment is no comment")
        XCTAssertEqual(store.comments(for: slice).count, 1)

        XCTAssertNil(set(store, wide, CGPoint(x: 3, y: 4), "", id: made.id))
        XCTAssertEqual(store.comments(for: slice), [])
        XCTAssertNil(store.comments[slice], "an emptied slice leaves nothing behind")
    }

    func testDeletingAComment() throws {
        let store = VisualStore(client: FixtureNatClient())
        let a = try XCTUnwrap(set(store, wide, nil, "a"))
        let b = try XCTUnwrap(set(store, tall, nil, "b"))
        XCTAssertEqual(a.placement, "whole image")
        store.deleteComment(sliceID: slice, id: a.id)
        XCTAssertEqual(store.comments(for: slice).map(\.id), [b.id])
        store.deleteComment(sliceID: "other", id: b.id)
        store.deleteComment(sliceID: slice, id: b.id)
        XCTAssertNil(store.comments[slice])
    }

    func testANewHandInDropsCommentsOnImagesItNoLongerCarries() async {
        let store = VisualStore(client: FixtureNatClient())
        store.loader = { _ in .unavailable }
        set(store, wide, nil, "kept")
        set(store, tall, nil, "dropped")
        await store.load(sliceID: slice, visuals: [wide, VisualChange(index: 2, name: "Tall", uri: "/tmp/new.png")])
        XCTAssertEqual(store.comments(for: slice).map(\.text), ["kept"])
        await store.load(sliceID: slice, visuals: [])
        XCTAssertNil(store.comments[slice])
    }

    // MARK: - Sending

    func testSendingToAHandedBackSliceTakesItOutOfReview() async throws {
        let client = FixtureNatClient()
        let store = VisualStore(client: client)
        set(store, wide, CGPoint(x: 1, y: 2), "fix it")
        set(store, tall, nil, "and this")
        let sent = try await store.sendComments(projectID: "p", sliceRef: slice, branch: "slice/x", handedBack: true)
        XCTAssertEqual(sent, 2)
        XCTAssertEqual(client.writes, ["agent-send \(slice)", "slice-rework \(slice)"])
        XCTAssertEqual(store.comments(for: slice), [])
    }

    func testSendingWhileTheAgentStillWorksLeavesTheSliceAlone() async throws {
        let client = FixtureNatClient()
        let store = VisualStore(client: client)
        set(store, wide, nil, "fix it")
        let sent = try await store.sendComments(projectID: "p", sliceRef: slice, branch: nil, handedBack: false)
        XCTAssertEqual(sent, 1)
        XCTAssertEqual(client.writes, ["agent-send \(slice)"])
    }

    func testNothingPendingSendsNothing() async throws {
        let client = FixtureNatClient()
        let sent = try await VisualStore(client: client).sendComments(
            projectID: "p", sliceRef: slice, branch: "b", handedBack: true)
        XCTAssertEqual(sent, 0)
        XCTAssertEqual(client.writes, [])
    }

    func testAFailedSendKeepsEveryComment() async {
        let store = VisualStore(client: FixtureNatClient(behaviour: .refusing("no session")))
        set(store, wide, nil, "fix it")
        do {
            try await store.sendComments(projectID: "p", sliceRef: slice, branch: "b", handedBack: true)
            XCTFail("want the send's failure")
        } catch {}
        XCTAssertEqual(store.comments(for: slice).map(\.text), ["fix it"])
    }
}

/// A count a `@Sendable` loader can bump from off the main actor.
private final class Counter: @unchecked Sendable {
    private let lock = NSLock()
    private var value = 0
    var count: Int { lock.withLock { value } }
    func bump() { lock.withLock { value += 1 } }
}
