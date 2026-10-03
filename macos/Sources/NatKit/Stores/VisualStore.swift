import AppKit
import Foundation

/// One handed-in image as the pane draws it: the image and its size in
/// pixels, or the word that it could not be opened — a URI that is not a
/// local path, or a file that is not there or not an image.
///
/// `@unchecked Sendable` because `NSImage` is not: an image is only ever
/// read after the load that made it has handed it over, and never mutated.
public enum VisualImage: @unchecked Sendable, Equatable {
    case image(NSImage, pixelSize: CGSize)
    case unavailable

    /// The image's size in pixels, nil where there is no image.
    public var pixelSize: CGSize? {
        if case .image(_, let size) = self { return size }
        return nil
    }

    public static func == (a: VisualImage, b: VisualImage) -> Bool {
        switch (a, b) {
        case let (.image(x, sx), .image(y, sy)): return x === y && sx == sy
        case (.unavailable, .unavailable): return true
        default: return false
        }
    }
}

/// The Visual changes section's state, one per project like `DiffStore`:
/// the handed-in images loaded by URI, each image's own zoom, and the
/// comments pending on each slice's images until they are sent.
///
/// Nothing here is written anywhere: the images are read from where the agent
/// left them, and the comments live only in the session — cleared once a send
/// has reached the agent, kept when it fails.
@MainActor
@Observable
public final class VisualStore {
    /// The zoom an image starts at — fitted to the pane's width — and the
    /// bounds and step every zoom stays within.
    public static let fitZoom: CGFloat = 1.0
    public static let minZoom: CGFloat = 0.25
    public static let maxZoom: CGFloat = 4.0
    public static let zoomStep: CGFloat = 1.25

    /// Every image loaded so far, by URI.
    public private(set) var loaded: [String: VisualImage] = [:]

    /// Each image's zoom, by slice ID then visual index; absent is `fitZoom`.
    public private(set) var zooms: [String: [Int: CGFloat]] = [:]

    /// The comments pending on each slice's images, by slice ID, in the
    /// order they are sent and drawn: by image, then those on the whole image
    /// first, then top to bottom and left to right.
    public private(set) var comments: [String: [PendingVisualComment]] = [:]

    /// Which images have been marked viewed, and which are folded to their
    /// header, by slice ID then visual index — the screen's own state, never
    /// written anywhere, as `DiffStore`'s viewed and collapsed files are.
    public private(set) var viewed: [String: Set<Int>] = [:]
    public private(set) var collapsed: [String: Set<Int>] = [:]

    /// How a URI becomes an image — a seam a story or test swaps after first
    /// access (nothing is read until `load`), so `AppModel` takes no new
    /// parameter for it. Runs off the main actor.
    public var loader: @Sendable (String) -> VisualImage = VisualStore.fileLoader

    private let client: NatClientProtocol

    /// The URI each index carried at the last load, by slice — what tells a
    /// newer hand-in's image apart from the one a viewed mark was left on.
    private var seenURIs: [String: [Int: String]] = [:]

    public init(client: NatClientProtocol = NatClient()) {
        self.client = client
    }

    /// The default loader: a bare absolute path or a `file://` URI read from
    /// disk, its pixel size from its first bitmap; anything else — another
    /// scheme, a missing file, a file that is no image — is unavailable. No
    /// network is ever touched.
    public nonisolated static func fileLoader(_ uri: String) -> VisualImage {
        let path: String
        if uri.hasPrefix("file://") {
            guard let url = URL(string: uri), url.isFileURL else { return .unavailable }
            path = url.path
        } else if uri.hasPrefix("/") {
            path = uri
        } else {
            return .unavailable
        }
        guard let image = NSImage(contentsOfFile: path) else { return .unavailable }
        let rep = image.representations.first
        let size = rep.map { CGSize(width: $0.pixelsWide, height: $0.pixelsHigh) } ?? image.size
        guard size.width > 0, size.height > 0 else { return .unavailable }
        return .image(image, pixelSize: size)
    }

    // MARK: - Images

    /// Load every image of a slice's hand-in not already loaded, off the main
    /// actor, and publish them together — so nothing draws until every size
    /// is known and no list height ever shifts. Comments left on images the
    /// hand-in no longer carries are dropped, and so are their viewed marks
    /// and folds: a new hand-in starts afresh.
    public func load(sliceID: String, visuals: [VisualChange]) async {
        let current = Set(visuals.map { "\($0.index)\u{0}\($0.uri)" })
        if let pending = comments[sliceID] {
            let kept = pending.filter { current.contains("\($0.index)\u{0}\($0.uri)") }
            comments[sliceID] = kept.isEmpty ? nil : kept
        }
        // A viewed mark is keyed by index alone, so an index whose URI
        // changed is a different image and loses it too.
        let uris = Dictionary(visuals.map { ($0.index, $0.uri) }, uniquingKeysWith: { first, _ in first })
        if seenURIs[sliceID] != uris {
            let kept = Set(uris.keys.filter { seenURIs[sliceID]?[$0] == uris[$0] })
            viewed[sliceID] = viewed[sliceID].map { $0.intersection(kept) }.flatMap { $0.isEmpty ? nil : $0 }
            collapsed[sliceID] = collapsed[sliceID].map { $0.intersection(kept) }.flatMap { $0.isEmpty ? nil : $0 }
            seenURIs[sliceID] = uris
        }
        var seen = Set(loaded.keys)
        let missing = visuals.map(\.uri).filter { seen.insert($0).inserted }
        guard !missing.isEmpty else { return }
        let loader = self.loader
        let results = await Task.detached { missing.map { ($0, loader($0)) } }.value
        for (uri, image) in results {
            loaded[uri] = image
        }
    }

    /// Whether every image of a hand-in is loaded, which is when the pane
    /// draws them.
    public func isLoaded(_ visuals: [VisualChange]) -> Bool {
        visuals.allSatisfy { loaded[$0.uri] != nil }
    }

    /// One image as loaded, nil before its load lands.
    public func image(for uri: String) -> VisualImage? {
        loaded[uri]
    }

    // MARK: - Zoom

    /// An image's zoom — 1 is fitted to the pane's width.
    public func zoom(sliceID: String, index: Int) -> CGFloat {
        zooms[sliceID]?[index] ?? Self.fitZoom
    }

    /// Set one image's zoom, clamped; no other image's changes.
    public func setZoom(_ zoom: CGFloat, sliceID: String, index: Int) {
        zooms[sliceID, default: [:]][index] = min(max(zoom, Self.minZoom), Self.maxZoom)
    }

    public func zoomIn(sliceID: String, index: Int) {
        setZoom(zoom(sliceID: sliceID, index: index) * Self.zoomStep, sliceID: sliceID, index: index)
    }

    public func zoomOut(sliceID: String, index: Int) {
        setZoom(zoom(sliceID: sliceID, index: index) / Self.zoomStep, sliceID: sliceID, index: index)
    }

    /// Back to fitted to the pane's width.
    public func resetZoom(sliceID: String, index: Int) {
        zooms[sliceID]?[index] = nil
    }

    // MARK: - Viewed and folded

    public func isViewed(sliceID: String, index: Int) -> Bool {
        viewed[sliceID]?.contains(index) ?? false
    }

    public func isCollapsed(sliceID: String, index: Int) -> Bool {
        collapsed[sliceID]?.contains(index) ?? false
    }

    /// Toggle an image's viewed mark. Marking it viewed folds it too,
    /// GitHub-fashion, as `DiffStore.toggleViewed` does; un-marking it leaves
    /// its fold as it was.
    public func toggleViewed(sliceID: String, index: Int) {
        if isViewed(sliceID: sliceID, index: index) {
            viewed[sliceID]?.remove(index)
        } else {
            viewed[sliceID, default: []].insert(index)
            collapsed[sliceID, default: []].insert(index)
        }
    }

    /// Toggle an image's own fold, whether or not it is viewed.
    public func toggleCollapsed(sliceID: String, index: Int) {
        if isCollapsed(sliceID: sliceID, index: index) {
            collapsed[sliceID]?.remove(index)
        } else {
            collapsed[sliceID, default: []].insert(index)
        }
    }

    // MARK: - Comments

    /// The comments pending on a slice's images, in send order.
    public func comments(for sliceID: String) -> [PendingVisualComment] {
        comments[sliceID] ?? []
    }

    /// Add a comment, or — given the `id` of one already pending — replace
    /// its text; text that trims to nothing takes it back, as
    /// `DiffStore.setComment` does.
    @discardableResult
    public func setComment(
        sliceID: String, id: UUID? = nil, visual: VisualChange,
        point: CGPoint?, imageSize: CGSize, text: String
    ) -> PendingVisualComment? {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        var list = comments[sliceID] ?? []
        let existing = id.flatMap { id in list.firstIndex { $0.id == id } }
        defer { comments[sliceID] = list.isEmpty ? nil : list }
        if trimmed.isEmpty {
            if let existing { list.remove(at: existing) }
            return nil
        }
        if let existing {
            list[existing].text = trimmed
            return list[existing]
        }
        let comment = PendingVisualComment(
            index: visual.index, name: visual.name, uri: visual.uri,
            point: point, imageSize: imageSize, text: trimmed)
        list.append(comment)
        list.sort(by: Self.sendOrder)
        return comment
    }

    /// Take back one pending comment outright — the trash icon on its card.
    public func deleteComment(sliceID: String, id: UUID) {
        guard var list = comments[sliceID] else { return }
        list.removeAll { $0.id == id }
        comments[sliceID] = list.isEmpty ? nil : list
    }

    /// Send every pending comment on a slice's images to its agent as one
    /// prompt, then clear them — only once the send has landed, so a failed
    /// one keeps every comment. Where the slice is handed back, the prompt
    /// ends with the hand-back and the slice is taken out of review
    /// (`slice-rework`), as Changes does; where it is not, the agent is still
    /// working and will hand back of its own accord, so neither is done.
    @discardableResult
    public func sendComments(projectID: String, sliceRef: String, branch: String?, handedBack: Bool) async throws -> Int {
        let pending = comments(for: sliceRef)
        guard !pending.isEmpty else { return 0 }
        let handBack = handedBack ? HandBackInstruction(projectID: projectID, sliceRef: sliceRef) : nil
        let prompt = visualCommentsPrompt(pending, branch: branch, handBack: handBack)
        try await client.agentSend(projectID: projectID, sliceRef: sliceRef, text: prompt)
        comments[sliceRef] = nil
        if handedBack {
            try await client.sliceRework(
                projectID: projectID, sliceRef: sliceRef, comments: visualCommentsRecord(pending))
        }
        return pending.count
    }

    /// By image, then the whole-image comments first, then top to bottom and
    /// left to right.
    private static func sendOrder(_ a: PendingVisualComment, _ b: PendingVisualComment) -> Bool {
        if a.index != b.index { return a.index < b.index }
        switch (a.point, b.point) {
        case (nil, nil): return false
        case (nil, _): return true
        case (_, nil): return false
        case let (pa?, pb?):
            if pa.y != pb.y { return pa.y < pb.y }
            return pa.x < pb.x
        }
    }
}
