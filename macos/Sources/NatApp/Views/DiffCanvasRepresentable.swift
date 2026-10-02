import AppKit
import SwiftUI
import NatKit

/// What is drawn under one row of the diff: the comment editor open on it,
/// then its pending comments.
struct DiffAttachmentContent: Equatable {
    var draft: CommentDraft?
    var comments: [PendingComment] = []
}

/// `DiffCanvasView` in SwiftUI: the diff itself drawn by AppKit, the
/// comments and the editor under its rows still SwiftUI's, each hosted in a
/// view of its own that the canvas places and measures.
struct DiffCanvasRepresentable: NSViewRepresentable {
    let files: [DiffFileModel]
    let state: DiffCanvasState
    let attachments: [String: DiffAttachmentContent]
    let actions: DiffCanvasActions
    let review: DiffReview?
    let store: DiffStore?
    let authorName: String
    let authorInitials: String

    func makeCoordinator() -> Coordinator { Coordinator() }

    func makeNSView(context: Context) -> DiffCanvasView {
        let canvas = DiffCanvasView()
        let coordinator = context.coordinator
        canvas.measureAttachment = { key, width in coordinator.measure(key, width: width) }
        coordinator.canvas = canvas
        return canvas
    }

    func updateNSView(_ canvas: DiffCanvasView, context: Context) {
        canvas.actions = actions
        canvas.update(files: files, state: state)
        context.coordinator.sync(
            attachments, review: review, store: store, authorName: authorName, authorInitials: authorInitials)
        if let review, let request = review.scrollRequest, request.token > review.handledScrollToken {
            review.handledScrollToken = request.token
            canvas.scrollToFile(request.path)
        }
    }

    @MainActor
    final class Coordinator {
        weak var canvas: DiffCanvasView?
        private var hosts: [String: NSHostingController<DiffAttachmentView>] = [:]
        private var contents: [String: DiffAttachmentContent] = [:]

        func measure(_ key: String, width: CGFloat) -> CGFloat {
            hosts[key]?.sizeThatFits(in: CGSize(width: width, height: 10_000)).height ?? 0
        }

        /// Brings the hosted attachments in line with `wanted`: a new key
        /// gets a view, a gone one loses it, and a changed one is redrawn in
        /// place, so an editor keeps what has been typed into it.
        func sync(
            _ wanted: [String: DiffAttachmentContent], review: DiffReview?, store: DiffStore?,
            authorName: String, authorInitials: String
        ) {
            guard let canvas else { return }
            var changed = false
            for key in hosts.keys where wanted[key] == nil {
                hosts[key] = nil
                contents[key] = nil
                changed = true
            }
            for (key, content) in wanted where contents[key] != content || hosts[key] == nil {
                let view = DiffAttachmentView(
                    content: content, review: review, store: store, authorName: authorName, authorInitials: authorInitials,
                    onHeight: { [weak canvas] _ in canvas?.attachmentDidResize(key) })
                if let host = hosts[key] {
                    host.rootView = view
                    canvas.attachmentDidResize(key)
                } else {
                    let host = NSHostingController(rootView: view)
                    host.sizingOptions = []
                    hosts[key] = host
                    changed = true
                }
                contents[key] = content
            }
            if changed { canvas.setAttachments(hosts.mapValues(\.view)) }
        }
    }
}

/// One row's attachment: the editor, then the cards, each inset from the
/// trailing edge as the file box always drew them. Says when its height
/// changes — an editor growing as it is typed in — so the diff can make room.
struct DiffAttachmentView: View {
    let content: DiffAttachmentContent
    let review: DiffReview?
    let store: DiffStore?
    let authorName: String
    let authorInitials: String
    let onHeight: (CGFloat) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let draft = content.draft {
                CommentEditorView(
                    initialText: draft.text,
                    onSave: { text in if let store { review?.saveDraft(text, store: store) } },
                    onCancel: { review?.draft = nil })
                .padding(.trailing, 16)
                .padding(.vertical, 8)
            }
            ForEach(content.comments) { comment in
                PendingCommentCardView(
                    comment: comment, authorName: authorName, authorInitials: authorInitials,
                    onEdit: { review?.editComment(comment) },
                    onDelete: { if let store { review?.deleteComment(comment, store: store) } })
                .padding(.trailing, 16)
                .padding(.top, 4)
                .padding(.bottom, 8)
            }
        }
        .fixedSize(horizontal: false, vertical: true)
        .onGeometryChange(for: CGFloat.self) { $0.size.height } action: { onHeight($0) }
        // Wide as it is given, but only as tall as it is: its ideal height
        // is what the canvas measures and lays the diff out with.
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .surface(.window)
    }
}
