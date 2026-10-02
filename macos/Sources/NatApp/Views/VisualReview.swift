import SwiftUI
import NatKit

/// The comment box open on one handed-in image: a new comment at a point (or
/// on the whole image, `point` nil), or a pending one reopened (`id`), so
/// saving replaces it rather than adding beside it.
struct VisualDraft: Equatable {
    var id: UUID?
    var visual: VisualChange
    var point: CGPoint?
    var imageSize: CGSize
    var text: String
}

/// The review of a slice's handed-in images, shared between the navigator's
/// Visual changes section (the image list and Send) and the main pane's image
/// list (where comments are left) — `DiffReview`'s analogue. The pending
/// comments themselves live in `VisualStore`.
@MainActor
@Observable
final class VisualReview {
    var draft: VisualDraft?
    var isSending = false
    var sendError: String?
    /// The image the main pane is asked to scroll to, bumped with a token so
    /// asking for the same one twice still scrolls.
    private(set) var scrollRequest: (index: Int, token: Int)?
    /// The last request the pane acted on — see `DiffReview`.
    @ObservationIgnored var handledScrollToken = 0

    func requestScroll(to index: Int) {
        scrollRequest = (index, (scrollRequest?.token ?? 0) + 1)
    }

    /// Forget the draft and any refusal — a different slice is on screen.
    func reset() {
        draft = nil
        sendError = nil
    }

    func store(_ appModel: AppModel) -> VisualStore {
        appModel.visualStore(projectID: appModel.projectStore?.projectID ?? "")
    }

    // MARK: - Drafting

    func openDraft(_ visual: VisualChange, point: CGPoint?, imageSize: CGSize) {
        draft = VisualDraft(visual: visual, point: point, imageSize: imageSize, text: "")
    }

    func editComment(_ comment: PendingVisualComment, visual: VisualChange) {
        draft = VisualDraft(
            id: comment.id, visual: visual, point: comment.point, imageSize: comment.imageSize, text: comment.text)
    }

    func deleteComment(_ comment: PendingVisualComment, sliceID: String, store: VisualStore) {
        store.deleteComment(sliceID: sliceID, id: comment.id)
        if draft?.id == comment.id { draft = nil }
    }

    func saveDraft(_ text: String, sliceID: String, store: VisualStore) {
        guard let draft else { return }
        store.setComment(
            sliceID: sliceID, id: draft.id, visual: draft.visual, point: draft.point,
            imageSize: draft.imageSize, text: text)
        self.draft = nil
    }

    // MARK: - Sending

    func sendComments(appModel: AppModel, slice: Slice) async {
        guard let projectID = appModel.projectStore?.projectID else { return }
        isSending = true
        sendError = nil
        do {
            _ = try await store(appModel).sendComments(
                projectID: projectID, sliceRef: slice.id, branch: slice.branch, handedBack: slice.handedBack)
            await appModel.refresh()
        } catch {
            sendError = error.localizedDescription
        }
        isSending = false
    }
}
