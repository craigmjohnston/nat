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
        XCTAssertNil(store.image(for: wide))

        await store.load(sliceID: slice, visuals: [wide, remote, wide])
        XCTAssertTrue(store.isLoaded([wide, remote]))
        XCTAssertEqual(store.image(for: wide)?.pixelSize, CGSize(width: 40, height: 20))
        XCTAssertEqual(store.image(for: remote), .unavailable)
        XCTAssertNil(store.image(for: remote)?.pixelSize)
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

    // MARK: - Viewed and folded

    func testMarkingViewedFoldsAndUnmarkingLeavesTheFold() {
        let store = VisualStore(client: FixtureNatClient())
        XCTAssertFalse(store.isViewed(sliceID: slice, wide))
        XCTAssertFalse(store.isCollapsed(sliceID: slice, wide))

        store.toggleViewed(sliceID: slice, wide)
        XCTAssertTrue(store.isViewed(sliceID: slice, wide))
        XCTAssertTrue(store.isCollapsed(sliceID: slice, wide), "viewed folds, GitHub-fashion")
        XCTAssertFalse(store.isViewed(sliceID: slice, tall), "one image never marks another")
        XCTAssertFalse(store.isViewed(sliceID: "other", wide))

        store.toggleViewed(sliceID: slice, wide)
        XCTAssertFalse(store.isViewed(sliceID: slice, wide))
        XCTAssertTrue(store.isCollapsed(sliceID: slice, wide), "un-marking leaves the fold alone")

        store.toggleCollapsed(sliceID: slice, wide)
        XCTAssertFalse(store.isCollapsed(sliceID: slice, wide))
        store.toggleCollapsed(sliceID: slice, tall)
        XCTAssertTrue(store.isCollapsed(sliceID: slice, tall))
        XCTAssertFalse(store.isViewed(sliceID: slice, tall), "folding is not viewing")
    }

    func testANewHandInDropsMarksOnImagesItReplaced() async {
        let store = VisualStore(client: FixtureNatClient())
        store.loader = { _ in .unavailable }
        await store.load(sliceID: slice, visuals: [wide, tall])
        store.toggleViewed(sliceID: slice, wide)
        store.toggleViewed(sliceID: slice, tall)

        await store.load(sliceID: slice, visuals: [wide, tall])
        XCTAssertTrue(store.isViewed(sliceID: slice, tall), "the same hand-in read again keeps its marks")

        let retaken = VisualChange(index: 2, name: "Tall", uri: "/tmp/tall-2.png")
        await store.load(sliceID: slice, visuals: [wide, retaken])
        XCTAssertTrue(store.isViewed(sliceID: slice, wide))
        XCTAssertTrue(store.isCollapsed(sliceID: slice, wide))
        XCTAssertFalse(store.isViewed(sliceID: slice, retaken), "a new image at the same index starts unviewed")
        XCTAssertFalse(store.isCollapsed(sliceID: slice, retaken))

        let rehashed = VisualChange(index: 1, name: "Wide", uri: wide.uri, hash: "new bytes")
        await store.load(sliceID: slice, visuals: [rehashed, retaken])
        XCTAssertFalse(store.isViewed(sliceID: slice, rehashed), "a re-render at the same path starts afresh")

        await store.load(sliceID: slice, visuals: [])
        XCTAssertNil(store.viewed[slice])
        XCTAssertNil(store.collapsed[slice])
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
        await store.load(sliceID: slice, visuals: [VisualChange(index: 1, name: "Wide", uri: wide.uri, hash: "b")])
        XCTAssertEqual(store.comments(for: slice), [], "a re-render at the same path is another image")
        set(store, wide, nil, "kept")
        await store.load(sliceID: slice, visuals: [])
        XCTAssertNil(store.comments[slice])
    }

    // MARK: - Re-renders, pairs and New

    func testAReRenderAtTheSamePathIsReadAgainAndTheOldOneDropped() async {
        let store = VisualStore(client: FixtureNatClient())
        let counter = Counter()
        store.loader = { _ in
            counter.bump()
            return .image(NSImage(size: CGSize(width: 1, height: 1)), pixelSize: CGSize(width: 10, height: 10))
        }
        let first = VisualChange(index: 1, name: "Wide", uri: "/tmp/wide.png", hash: "a")
        await store.load(sliceID: slice, visuals: [first])
        let shown = store.image(for: first)
        XCTAssertNotNil(shown)

        // What a nudge's re-read of the slice carries after a re-render: the
        // same URI with a new hash, which the views' load is keyed by too.
        let second = VisualChange(index: 1, name: "Wide", uri: "/tmp/wide.png", hash: "b")
        XCTAssertNotEqual(VisualChange.loadIdentity([first]), VisualChange.loadIdentity([second]))
        XCTAssertFalse(store.isLoaded([second]), "the re-render is not drawn from the old one's cache")
        await store.load(sliceID: slice, visuals: [second])
        XCTAssertEqual(counter.count, 2)
        XCTAssertNotEqual(store.image(for: second), shown)
        XCTAssertNil(store.loaded[first.imageKey], "an image no hand-in names any more is dropped")
    }

    func testAnImageAnotherSliceStillNamesIsKept() async {
        let store = VisualStore(client: FixtureNatClient())
        store.loader = { _ in .unavailable }
        await store.load(sliceID: slice, visuals: [wide])
        await store.load(sliceID: "other", visuals: [wide])
        await store.load(sliceID: slice, visuals: [])
        XCTAssertNotNil(store.image(for: wide))
    }

    func testAPairLoadsItsBeforeAndDrawsOnlyOnceBothAreIn() async {
        let store = VisualStore(client: FixtureNatClient())
        let pair = VisualChange(index: 1, name: "P", uri: "/tmp/after.png", hash: "a",
                                before: VisualBefore(uri: "/tmp/before.png", hash: "b"))
        XCTAssertFalse(store.isLoaded([pair]))
        store.loader = { uri in
            uri.hasSuffix("before.png") ? .unavailable : .image(NSImage(size: CGSize(width: 1, height: 1)), pixelSize: CGSize(width: 4, height: 4))
        }
        await store.load(sliceID: slice, visuals: [pair])
        XCTAssertTrue(store.isLoaded([pair]))
        XCTAssertEqual(store.beforeImage(for: pair), .unavailable)
        XCTAssertNil(store.beforeImage(for: wide), "an item that is no pair has no before")
        XCTAssertFalse(store.isComparable(pair), "an unavailable before draws the after alone")
        XCTAssertEqual(store.highlightRefusal(for: pair), "The before couldn't be opened")
        await store.toggleHighlight(sliceID: slice, visual: pair)
        XCTAssertFalse(store.isHighlighting(sliceID: slice, index: 1), "nothing to compare is never highlighted")
    }

    func testTheDividerAndTheBeforeAfterToggle() {
        let store = VisualStore(client: FixtureNatClient())
        XCTAssertEqual(store.divider(sliceID: slice, index: 1), 0.5, "a pair opens with the divider at the middle")
        XCTAssertNil(store.shownSide(sliceID: slice, index: 1))
        store.show(.before, sliceID: slice, index: 1)
        XCTAssertEqual(store.divider(sliceID: slice, index: 1), 1)
        XCTAssertEqual(store.shownSide(sliceID: slice, index: 1), .before)
        XCTAssertEqual(store.divider(sliceID: slice, index: 2), 0.5, "one pair's divider never moves another's")
        store.show(.after, sliceID: slice, index: 1)
        XCTAssertEqual(store.shownSide(sliceID: slice, index: 1), .after)
        store.setDivider(0.99, sliceID: slice, index: 1)
        XCTAssertNil(store.shownSide(sliceID: slice, index: 1), "a side is selected only at its end")
        store.setDivider(7, sliceID: slice, index: 1)
        XCTAssertEqual(store.divider(sliceID: slice, index: 1), 1, "clamped to the frame")
    }

    func testHighlightingComputesTheMaskOnceAndTogglesOff() async throws {
        let store = VisualStore(client: FixtureNatClient())
        let pair = VisualChange(index: 1, name: "P", uri: "/a", hash: "a", before: VisualBefore(uri: "/b", hash: "b"))
        let after = try solid(.red, width: 4, height: 2), before = try solid(.blue, width: 4, height: 2)
        store.loader = { uri in .image(uri == "/a" ? after : before, pixelSize: CGSize(width: 4, height: 2)) }
        await store.load(sliceID: slice, visuals: [pair])
        XCTAssertNil(store.mask(for: pair))
        await store.toggleHighlight(sliceID: slice, visual: pair)
        XCTAssertTrue(store.isHighlighting(sliceID: slice, index: 1))
        let mask = try XCTUnwrap(store.mask(for: pair))
        XCTAssertEqual(mask.count, 8, "every pixel differs")
        await store.loadMask(for: pair)
        XCTAssertEqual(store.mask(for: pair), mask, "computed once, cached by the two hashes")
        await store.toggleHighlight(sliceID: slice, visual: pair)
        XCTAssertFalse(store.isHighlighting(sliceID: slice, index: 1))

        let rebefore = VisualChange(index: 1, name: "P", uri: "/a", hash: "a", before: VisualBefore(uri: "/b", hash: "c"))
        await store.load(sliceID: slice, visuals: [rebefore])
        XCTAssertNil(store.mask(for: rebefore), "a new before is a new mask")
        XCTAssertTrue(store.masks.isEmpty, "and the old one is dropped with its image")
    }

    func testNewUntilSeenOnScreenOrMarkedViewed() {
        let seen = VisualSeenMemory.inMemory()
        let store = VisualStore(client: FixtureNatClient(), projectID: "p", seen: seen)
        let changed = VisualChange(index: 1, name: "Wide", uri: "/tmp/wide.png", hash: "a", changed: true)
        let other = VisualChange(index: 2, name: "Tall", uri: "/tmp/tall.png", hash: "b", changed: true)
        XCTAssertTrue(store.isNew(sliceID: slice, changed))
        XCTAssertFalse(store.isNew(sliceID: slice, wide), "an unchanged image is never new")
        XCTAssertTrue(store.anyNew(sliceID: slice, [wide, changed]))

        store.markSeen(sliceID: slice, changed)
        XCTAssertFalse(store.isNew(sliceID: slice, changed))
        XCTAssertFalse(store.anyNew(sliceID: slice, [wide, changed]))
        XCTAssertTrue(seen.isSeen(projectID: "p", sliceID: slice, changed), "remembered beyond the store")

        store.toggleViewed(sliceID: slice, other)
        XCTAssertFalse(store.isNew(sliceID: slice, other), "marking viewed sees it")

        let rerendered = VisualChange(index: 1, name: "Wide", uri: "/tmp/wide.png", hash: "c", changed: true)
        XCTAssertTrue(store.isNew(sliceID: slice, rerendered), "a re-render is new again")
        XCTAssertTrue(VisualStore(client: FixtureNatClient(), projectID: "q", seen: seen).isNew(sliceID: slice, changed),
                      "seen in one project is not seen in another")
    }

    func testALoadPrunesSeenMarksButNotOnAnEmptyHandIn() async {
        let seen = VisualSeenMemory.inMemory()
        let store = VisualStore(client: FixtureNatClient(), projectID: "p", seen: seen)
        store.loader = { _ in .unavailable }
        let changed = VisualChange(index: 1, name: "Wide", uri: "/tmp/wide.png", hash: "a", changed: true)
        store.markSeen(sliceID: slice, changed)
        await store.load(sliceID: slice, visuals: [])
        XCTAssertTrue(seen.isSeen(projectID: "p", sliceID: slice, changed), "a detail not yet read forgets nothing")
        await store.load(sliceID: slice, visuals: [tall])
        XCTAssertFalse(seen.isSeen(projectID: "p", sliceID: slice, changed))
    }

    private func solid(_ color: NSColor, width: Int, height: Int) throws -> NSImage {
        let rep = try XCTUnwrap(NSBitmapImageRep(
            bitmapDataPlanes: nil, pixelsWide: width, pixelsHigh: height, bitsPerSample: 8, samplesPerPixel: 4,
            hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0))
        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
        color.setFill()
        NSRect(x: 0, y: 0, width: width, height: height).fill()
        NSGraphicsContext.restoreGraphicsState()
        let image = NSImage(size: NSSize(width: width, height: height))
        image.addRepresentation(rep)
        return image
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
