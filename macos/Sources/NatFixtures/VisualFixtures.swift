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
    public static let visualsSliceDetail = SliceDetail(
        id: sliceDetail.id, name: sliceDetail.name, url: sliceDetail.url, status: sliceDetail.status,
        milestone: sliceDetail.milestone, assignee: sliceDetail.assignee, branch: sliceDetail.branch,
        repo: sliceDetail.repo, base: sliceDetail.base, pr: sliceDetail.pr, dependsOn: sliceDetail.dependsOn,
        blocked: sliceDetail.blocked, handedBack: sliceDetail.handedBack, state: sliceDetail.state,
        brief: sliceDetail.brief, visuals: visualChanges)

    /// `sliceDetails` with the handed-back slice's images added.
    public static var visualsSliceDetails: [String: SliceDetail] {
        sliceDetails.merging([mergeBoxSliceID: visualsSliceDetail]) { _, new in new }
    }

    /// The pixel sizes the two drawn renders stand for: a wide window and a
    /// tall sidebar.
    public static let visualPixelSizes: [String: CGSize] = [
        visualChanges[0].uri: CGSize(width: 1440, height: 900),
        visualChanges[1].uri: CGSize(width: 800, height: 1200),
    ]

    /// The loader a story swaps in: the two renders drawn procedurally —
    /// panes and rows as filled rounded rectangles, and a title — so no file
    /// is read; anything else is unavailable, as the real loader has it.
    public static let visualImageLoader: @Sendable (String) -> VisualImage = { uri in
        guard let size = visualPixelSizes[uri] else { return .unavailable }
        let title = visualChanges.first { $0.uri == uri }?.name ?? ""
        let image = NSImage(size: size, flipped: true) { rect in
            drawRender(in: rect, title: title)
            return true
        }
        return .image(image, pixelSize: size)
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
    /// a card, in the theme's own colours, with the title across the band.
    private static func drawRender(in rect: CGRect, title: String) {
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
            x: column.maxX + unit * 4, y: unit * 10, width: rect.width - column.maxX - unit * 7, height: rect.height * 0.4)
        fill(DesignTokens.fill(.card), card, radius: unit)
        fill(DesignTokens.ink(.success, on: .card),
             CGRect(x: card.minX + unit * 3, y: card.minY + unit * 3, width: unit * 3, height: unit * 3), radius: unit * 1.5)
        fill(DesignTokens.accentDim(on: .card),
             CGRect(x: card.minX + unit * 3, y: card.maxY - unit * 8, width: card.width * 0.35, height: unit * 5), radius: unit)
    }
}
