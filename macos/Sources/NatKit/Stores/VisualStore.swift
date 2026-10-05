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
/// the handed-in images loaded by identity (URI and hash), each item's own
/// zoom and — for a pair — its divider and highlight, what has been viewed,
/// folded and seen, and the comments pending on each slice's images until
/// they are sent.
///
/// Only what has been seen is written anywhere (`SeenMemory`): the images
/// are read from where the agent left them, and the comments live only in the
/// session — cleared once a send has reached the agent, kept when it fails.
@MainActor
@Observable
public final class VisualStore {
    /// The zoom an image starts at — fitted to the pane's width — and the
    /// bounds and step every zoom stays within.
    public static let fitZoom: CGFloat = 1.0
    public static let minZoom: CGFloat = 0.25
    public static let maxZoom: CGFloat = 4.0
    public static let zoomStep: CGFloat = 1.25

    /// Every image loaded so far, by `VisualChange.imageKey`/`beforeKey` — a
    /// URI and the hash it was handed in with, so a re-render saved over the
    /// same path is read again.
    public private(set) var loaded: [String: VisualImage] = [:]

    /// Each image's zoom, by slice ID then visual index; absent is `fitZoom`.
    public private(set) var zooms: [String: [Int: CGFloat]] = [:]

    /// Each pair's divider, by slice ID then visual index; absent is
    /// `VisualCompare.middle`. Session-only, as zoom is.
    public private(set) var dividers: [String: [Int: CGFloat]] = [:]

    /// The pairs whose differences are highlighted, by slice ID.
    public private(set) var highlighted: [String: Set<Int>] = [:]

    /// Each pair's difference mask, by its before's and after's keys — the
    /// two hashes — computed once.
    public private(set) var masks: [String: VisualMask] = [:]

    /// The comments pending on each slice's images, by slice ID, in the
    /// order they are sent and drawn: by image, then those on the whole image
    /// first, then top to bottom and left to right.
    public private(set) var comments: [String: [PendingVisualComment]] = [:]

    /// Which items have been marked viewed, and which are folded to their
    /// header, by slice ID then `VisualChange.identity` — the screen's own
    /// state, never written anywhere, as `DiffStore`'s viewed and collapsed
    /// files are. An item handed in again with a new hash is a new identity,
    /// and starts afresh.
    public private(set) var viewed: [String: Set<String>] = [:]
    public private(set) var collapsed: [String: Set<String>] = [:]

    /// How a URI becomes an image — a seam a story or test swaps after first
    /// access (nothing is read until `load`), so `AppModel` takes no new
    /// parameter for it. Runs off the main actor.
    public var loader: @Sendable (String) -> VisualImage = VisualStore.fileLoader

    private let client: NatClientProtocol
    private let projectID: String
    private let seen: SeenMemory

    /// The image keys each slice's last hand-in names — what decides which
    /// loaded images are still wanted.
    private var handIns: [String: Set<String>] = [:]

    /// Bumped on every seen mark written, so a view reading `badge` redraws:
    /// the memory itself is not observable.
    private var seenTick = 0

    public init(client: NatClientProtocol = NatClient(), projectID: String = "", seen: SeenMemory = .inMemory()) {
        self.client = client
        self.projectID = projectID
        self.seen = seen
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

    /// Load every image of a slice's hand-in — befores included — not already
    /// loaded at its URI and hash, off the main actor, and publish them
    /// together, so nothing draws until every size is known and no list
    /// height ever shifts. Images no slice's hand-in still names are dropped,
    /// and so are their masks. Comments left on images the hand-in no longer
    /// carries are dropped, and so are viewed marks and folds on items it no
    /// longer carries as they were: a re-render starts afresh.
    public func load(sliceID: String, visuals: [VisualChange]) async {
        let current = Set(visuals.map { Self.commentKey($0.index, $0.imageKey) })
        if let pending = comments[sliceID] {
            let kept = pending.filter { current.contains(Self.commentKey($0.index, VisualChange.key(uri: $0.uri, hash: $0.hash))) }
            comments[sliceID] = kept.isEmpty ? nil : kept
        }
        let identities = Set(visuals.map(\.identity))
        viewed[sliceID] = viewed[sliceID].map { $0.intersection(identities) }.flatMap { $0.isEmpty ? nil : $0 }
        collapsed[sliceID] = collapsed[sliceID].map { $0.intersection(identities) }.flatMap { $0.isEmpty ? nil : $0 }
        // An empty hand-in is also what a slice reads as before its detail
        // has loaded, so what was seen — which outlives the session — is
        // pruned, and a first look taken, only against a hand-in that names
        // something.
        if !visuals.isEmpty {
            seen.retain(projectID: projectID, sliceID: sliceID, .visuals, items: Set(visuals.map(\.name)))
            seen.baseline(projectID: projectID, sliceID: sliceID, .visuals, Self.snapshot(visuals))
            seenTick += 1
        }

        handIns[sliceID] = Set(visuals.flatMap { $0.imageKeys.map(\.key) })
        let wanted = handIns.values.reduce(into: Set<String>()) { $0.formUnion($1) }
        loaded = loaded.filter { wanted.contains($0.key) }
        masks = masks.filter { key, _ in
            let parts = key.split(separator: "\u{3}", omittingEmptySubsequences: false).map(String.init)
            return parts.allSatisfy(wanted.contains)
        }

        var asked = Set(loaded.keys)
        let missing = visuals.flatMap(\.imageKeys).filter { asked.insert($0.key).inserted }
        guard !missing.isEmpty else { return }
        let loader = self.loader
        let results = await Task.detached { missing.map { ($0.key, loader($0.uri)) } }.value
        for (key, image) in results {
            loaded[key] = image
        }
    }

    /// `load(sliceID:visuals:)` over a slice's hand-in as its detail reads it,
    /// nil while that detail has not loaded — which loads nothing and drops
    /// nothing. A slice whose cached detail was dropped is drawn with no
    /// hand-in on its way back to the screen, and loading that as one naming
    /// nothing would throw away every pending comment, viewed mark and fold
    /// on it; a hand-in that really is empty still clears them.
    public func load(sliceID: String, handIn visuals: [VisualChange]?) async {
        guard let visuals else { return }
        await load(sliceID: sliceID, visuals: visuals)
    }

    /// Whether every image of a hand-in, befores included, is loaded, which
    /// is when the pane draws them.
    public func isLoaded(_ visuals: [VisualChange]) -> Bool {
        visuals.allSatisfy { $0.imageKeys.allSatisfy { loaded[$0.key] != nil } }
    }

    /// An item's image as loaded, nil before its load lands.
    public func image(for visual: VisualChange) -> VisualImage? {
        loaded[visual.imageKey]
    }

    /// A pair's before as loaded, nil before its load lands or for an item
    /// that is no pair.
    public func beforeImage(for visual: VisualChange) -> VisualImage? {
        visual.beforeKey.flatMap { loaded[$0] }
    }

    private static func commentKey(_ index: Int, _ imageKey: String) -> String {
        "\(index)\u{0}\(imageKey)"
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

    // MARK: - Pairs

    /// Whether a pair can be compared at all: both its images open. An item
    /// that is no pair, or one whose before could not be opened, draws its
    /// image alone.
    public func isComparable(_ visual: VisualChange) -> Bool {
        image(for: visual)?.pixelSize != nil && beforeImage(for: visual)?.pixelSize != nil
    }

    /// A pair's divider, from 0 (the after whole) to 1 (the before whole).
    public func divider(sliceID: String, index: Int) -> CGFloat {
        dividers[sliceID]?[index] ?? VisualCompare.middle
    }

    /// Move a pair's divider, clamped to the frame.
    public func setDivider(_ divider: CGFloat, sliceID: String, index: Int) {
        dividers[sliceID, default: [:]][index] = VisualCompare.clamp(divider)
    }

    /// The Before / After toggle: the divider to the far end, showing `side`
    /// whole.
    public func show(_ side: VisualCompareSide, sliceID: String, index: Int) {
        setDivider(VisualCompare.divider(showing: side), sliceID: sliceID, index: index)
    }

    /// The side the Before / After toggle shows selected — one only while
    /// the divider is at its end.
    public func shownSide(sliceID: String, index: Int) -> VisualCompareSide? {
        VisualCompare.side(showing: divider(sliceID: sliceID, index: index))
    }

    /// Why a pair's differences cannot be highlighted, nil where they can.
    public func highlightRefusal(for visual: VisualChange) -> String? {
        VisualCompare.highlightRefusal(after: image(for: visual), before: beforeImage(for: visual))
    }

    public func isHighlighting(sliceID: String, index: Int) -> Bool {
        highlighted[sliceID]?.contains(index) ?? false
    }

    /// Turn a pair's highlight on or off. Turning it on computes the pair's
    /// mask, off the main actor, where it is not already cached; a pair that
    /// cannot be compared is never highlighted.
    public func toggleHighlight(sliceID: String, visual: VisualChange) async {
        if isHighlighting(sliceID: sliceID, index: visual.index) {
            highlighted[sliceID]?.remove(visual.index)
            return
        }
        guard highlightRefusal(for: visual) == nil else { return }
        highlighted[sliceID, default: []].insert(visual.index)
        await loadMask(for: visual)
    }

    /// A pair's difference mask, nil until it is computed.
    public func mask(for visual: VisualChange) -> VisualMask? {
        Self.maskKey(visual).flatMap { masks[$0] }
    }

    /// Compute a pair's difference mask where it is not already cached.
    public func loadMask(for visual: VisualChange) async {
        guard let key = Self.maskKey(visual), masks[key] == nil, highlightRefusal(for: visual) == nil,
              case .image(let after, let size)? = image(for: visual),
              case .image(let before, _)? = beforeImage(for: visual)
        else { return }
        // The images cross to the computation as the one value, not
        // separately captured: `NSImage` is not `Sendable`, and is only read.
        let pair = VisualImagePair(before: before, after: after)
        let mask = await Task.detached {
            VisualCompare.differenceMask(before: pair.before, after: pair.after, pixelSize: size)
        }.value
        if let mask { masks[key] = mask }
    }

    private static func maskKey(_ visual: VisualChange) -> String? {
        visual.beforeKey.map { "\($0)\u{3}\(visual.imageKey)" }
    }

    // MARK: - New and Updated

    /// An item's badge (`SeenMemory`'s one rule): New for an image by a name
    /// the user had not seen on this slice, Updated for one handed in again
    /// under a name they had seen with different content — its image or its
    /// before — and none the first time the section is ever loaded, or for
    /// what they have seen as it is.
    public func badge(sliceID: String, _ visual: VisualChange) -> SeenBadge? {
        _ = seenTick
        return seen.badge(projectID: projectID, sliceID: sliceID, .visuals, item: visual.name, fingerprint: visual.identity)
    }

    /// The section header's badge: New while any item is, else Updated while
    /// any item is.
    public func sectionStatus(sliceID: String, _ visuals: [VisualChange]) -> NavSectionStatus? {
        .of(visuals.map { badge(sliceID: sliceID, $0) })
    }

    /// The user has seen an item as it now is: its section has been on screen
    /// in the image list, or it has been marked viewed.
    public func markSeen(sliceID: String, _ visual: VisualChange) {
        guard badge(sliceID: sliceID, visual) != nil else { return }
        seen.markSeen(projectID: projectID, sliceID: sliceID, .visuals, item: visual.name, fingerprint: visual.identity)
        seenTick += 1
    }

    /// A hand-in as a seen snapshot: each image's name at its identity.
    private static func snapshot(_ visuals: [VisualChange]) -> [String: String] {
        Dictionary(visuals.map { ($0.name, $0.identity) }, uniquingKeysWith: { _, last in last })
    }

    // MARK: - Viewed and folded

    public func isViewed(sliceID: String, _ visual: VisualChange) -> Bool {
        viewed[sliceID]?.contains(visual.identity) ?? false
    }

    public func isCollapsed(sliceID: String, _ visual: VisualChange) -> Bool {
        collapsed[sliceID]?.contains(visual.identity) ?? false
    }

    /// Toggle an item's viewed mark. Marking it viewed folds it too,
    /// GitHub-fashion, as `DiffStore.toggleViewed` does, and sees it;
    /// un-marking it leaves its fold as it was.
    public func toggleViewed(sliceID: String, _ visual: VisualChange) {
        if isViewed(sliceID: sliceID, visual) {
            viewed[sliceID]?.remove(visual.identity)
        } else {
            viewed[sliceID, default: []].insert(visual.identity)
            collapsed[sliceID, default: []].insert(visual.identity)
            markSeen(sliceID: sliceID, visual)
        }
    }

    /// Toggle an item's own fold, whether or not it is viewed.
    public func toggleCollapsed(sliceID: String, _ visual: VisualChange) {
        if isCollapsed(sliceID: sliceID, visual) {
            collapsed[sliceID]?.remove(visual.identity)
        } else {
            collapsed[sliceID, default: []].insert(visual.identity)
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
            index: visual.index, name: visual.name, uri: visual.uri, hash: visual.hash,
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

/// A pair's two images, carried together to the mask's computation off the
/// main actor — `@unchecked Sendable` for `VisualImage`'s reason.
private struct VisualImagePair: @unchecked Sendable {
    let before: NSImage
    let after: NSImage
}
