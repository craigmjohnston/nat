import AppKit
import SwiftUI
import NatKit

extension Fixtures {
    /// What the handed-back slice's agent handed in: two renders, at paths
    /// no file is ever read from (`visualImageLoader` draws them), and one
    /// URI the app does not open, for the placeholder card.
    public static let visualChanges: [VisualChange] = [
        VisualChange(index: 1, name: "Merge box, all three verdicts passing",
                     uri: "/Users/craig/Projects/notion-agent-tracker/.render/merge-box-passing.png"),
        VisualChange(index: 2, name: "Merge box in the narrow PR sidebar",
                     uri: "/Users/craig/Projects/notion-agent-tracker/.render/merge-box-narrow.png"),
        VisualChange(index: 3, name: "The design's own mock, for comparison",
                     uri: "https://example.com/gnat/merge-box-mock.png"),
    ]

    /// The handed-back slice's detail with those images on it.
    public static let visualsSliceDetail = detail(visuals: visualChanges)

    /// `sliceDetails` with the handed-back slice's images added.
    public static var visualsSliceDetails: [String: SliceDetail] {
        sliceDetails.merging([mergeBoxSliceID: visualsSliceDetail]) { _, new in new }
    }

    /// A second hand-in on the same slice, as nat reads it: the first render
    /// re-rendered at the same path (a new hash, changed — and not yet seen),
    /// the second re-rendered too but already seen (`seedSeenVisual`), and
    /// the URI unchanged since the hand-in before.
    public static let visualChangesWithNews: [VisualChange] = [
        VisualChange(index: 1, name: visualChanges[0].name, uri: visualChanges[0].uri, hash: "9f2c41", changed: true),
        VisualChange(index: 2, name: visualChanges[1].name, uri: visualChanges[1].uri, hash: "47be0a", changed: true),
        VisualChange(index: 3, name: visualChanges[2].name, uri: visualChanges[2].uri),
    ]

    /// The handed-back slice's detail with that second hand-in on it.
    public static var visualsWithNewsSliceDetails: [String: SliceDetail] {
        sliceDetails.merging([mergeBoxSliceID: detail(visuals: visualChangesWithNews)]) { _, new in new }
    }

    /// Mark the second of `visualChangesWithNews` seen, as having been on
    /// screen once.
    @MainActor
    public static func seedSeenVisual(into store: VisualStore) {
        store.markSeen(sliceID: mergeBoxSliceID, visualChangesWithNews[1])
    }

    /// A pair: the merge box re-rendered, judged against the render from
    /// before the change, both the same size.
    public static let visualPair = VisualChange(
        index: 1, name: "Merge box, before and after the checks line",
        uri: "/Users/craig/Projects/notion-agent-tracker/.render/merge-box-after.png", hash: "c0ffee",
        before: VisualBefore(uri: "/Users/craig/Projects/notion-agent-tracker/.render/merge-box-before.png", hash: "decade"),
        changed: true)

    /// A pair whose before was rendered at another window size, so its pixels
    /// cannot be compared one for one.
    public static let visualPairMismatched = VisualChange(
        index: 1, name: "Merge box, before at the old window size",
        uri: visualPair.uri, hash: visualPair.hash,
        before: VisualBefore(uri: "/Users/craig/Projects/notion-agent-tracker/.render/merge-box-before-small.png", hash: "facade"),
        changed: true)

    /// The handed-back slice's detail with one hand-in of `visuals`.
    public static func detail(visuals: [VisualChange]) -> SliceDetail {
        SliceDetail(
            id: sliceDetail.id, name: sliceDetail.name, url: sliceDetail.url, status: sliceDetail.status,
            milestone: sliceDetail.milestone, assignee: sliceDetail.assignee, branch: sliceDetail.branch,
            repo: sliceDetail.repo, base: sliceDetail.base, pr: sliceDetail.pr, dependsOn: sliceDetail.dependsOn,
            blocked: sliceDetail.blocked, handedBack: sliceDetail.handedBack, state: sliceDetail.state,
            brief: sliceDetail.brief, visuals: visuals)
    }

    /// What each drawn render stands for: its pixel size, the title across
    /// it, and whether it is a before — drawn as the render was before the
    /// change, its card shorter and its checks dot not yet green.
    private static let visualRenders: [String: (size: CGSize, title: String, before: Bool)] = [
        visualChanges[0].uri: (CGSize(width: 1440, height: 900), visualChanges[0].name, false),
        visualChanges[1].uri: (CGSize(width: 800, height: 1200), visualChanges[1].name, false),
        visualPair.uri: (CGSize(width: 1440, height: 900), "Merge box", false),
        visualPair.before!.uri: (CGSize(width: 1440, height: 900), "Merge box", true),
        visualPairMismatched.before!.uri: (CGSize(width: 1200, height: 760), "Merge box", true),
    ]

    /// The pixel sizes the drawn renders stand for: a wide window, a tall
    /// sidebar, and the pairs' renders.
    public static let visualPixelSizes: [String: CGSize] = visualRenders.mapValues(\.size)

    /// The loader a story swaps in: the renders drawn procedurally — panes
    /// and rows as filled rounded rectangles, and a title — so no file is
    /// read; anything else is unavailable, as the real loader has it.
    public static let visualImageLoader: @Sendable (String) -> VisualImage = { uri in
        guard let render = visualRenders[uri] else { return .unavailable }
        let image = NSImage(size: render.size, flipped: true) { rect in
            drawRender(in: rect, title: render.title, before: render.before)
            return true
        }
        return .image(image, pixelSize: render.size)
    }

    /// A pending comment at a point on the first render, and one on the
    /// second as a whole.
    public static let pendingVisualComments: [(visual: VisualChange, point: CGPoint?, text: String)] = [
        (visualChanges[0], CGPoint(x: 412, y: 300), "The checks line should sit flush with the review line above it."),
        (visualChanges[1], nil, "In the narrow sidebar the heading wraps — keep it to one line and truncate."),
    ]

    /// Leave those comments on the handed-back slice.
    @MainActor
    public static func seedPendingVisualComments(into store: VisualStore) {
        for comment in pendingVisualComments {
            store.setComment(
                sliceID: mergeBoxSliceID, visual: comment.visual, point: comment.point,
                imageSize: visualPixelSizes[comment.visual.uri] ?? .zero, text: comment.text)
        }
    }

    /// A fake render: a window ground, a header band, a column of rows and
    /// a card, in the theme's own colours, with the title across the band —
    /// as a `before`, its card shorter and its dot amber.
    private static func drawRender(in rect: CGRect, title: String, before: Bool = false) {
        func fill(_ color: Color, _ r: CGRect, radius: CGFloat = 0) {
            NSColor(color).setFill()
            NSBezierPath(roundedRect: r, xRadius: radius, yRadius: radius).fill()
        }
        let unit = rect.width / 100
        fill(DesignTokens.fill(.window), rect)
        fill(DesignTokens.fill(.chrome), CGRect(x: 0, y: 0, width: rect.width, height: unit * 6))
        let text = NSAttributedString(string: title, attributes: [
            .font: NSFont.systemFont(ofSize: unit * 2.6, weight: .semibold),
            .foregroundColor: NSColor(DesignTokens.ink(.primary, on: .chrome)),
        ])
        text.draw(at: CGPoint(x: unit * 3, y: unit * 1.4))
        let column = CGRect(x: unit * 3, y: unit * 10, width: rect.width * 0.3, height: rect.height - unit * 14)
        fill(DesignTokens.fill(.chrome), column, radius: unit)
        var y = column.minY + unit * 2
        while y + unit * 4 < column.maxY {
            fill(DesignTokens.rowWash(selected: false, on: .chrome),
                 CGRect(x: column.minX + unit * 2, y: y, width: column.width - unit * 4, height: unit * 3), radius: unit / 2)
            y += unit * 5
        }
        let card = CGRect(
            x: column.maxX + unit * 4, y: unit * 10, width: rect.width - column.maxX - unit * 7,
            height: rect.height * (before ? 0.32 : 0.4))
        fill(DesignTokens.fill(.card), card, radius: unit)
        fill(DesignTokens.ink(before ? .warning : .success, on: .card),
             CGRect(x: card.minX + unit * 3, y: card.minY + unit * 3, width: unit * 3, height: unit * 3), radius: unit * 1.5)
        fill(DesignTokens.accentDim(on: .card),
             CGRect(x: card.minX + unit * 3, y: card.maxY - unit * 8, width: card.width * 0.35, height: unit * 5), radius: unit)
    }
}
