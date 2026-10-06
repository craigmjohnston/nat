import SwiftUI
import NatKit

/// A run of one file's rows currently marked as a comment's anchor — purely
/// transient view state (never persisted; `PendingComment` is what persists
/// once the run actually has something said about it). `rowIDs` are ordered
/// by the file's own row order and always contiguous within it.
struct DiffSelection: Equatable {
    let path: String
    var rowIDs: [String]
}

/// The comment box open below a selection's last row: a new comment being
/// written, or an existing pending one reopened for editing — its own
/// anchor, so saving replaces what was there rather than adding beside it.
struct CommentDraft: Equatable {
    var path: String
    var anchorRowIDs: [String]
    var text: String
}

/// The review of a handed-back branch, shared between the two places the
/// design splits it across: the navigator's Changes section (the file list,
/// Send and Approve) and the main pane's continuous diff (the rows a comment
/// is anchored to). What it holds is the old Diff tab's own view state —
/// the marked rows, the open draft, the send in flight and what refused —
/// lifted out so both halves read and write the one copy. The pending
/// comments themselves stay where they always were, in `DiffStore`.
@MainActor
@Observable
final class DiffReview {
    var selection: DiffSelection?
    var draft: CommentDraft?
    var isSending = false
    var sendError: String?
    var dropNotice: String?
    var showApproveConfirm = false
    /// The file the main pane is asked to scroll to, bumped with a token so
    /// asking for the same file twice still scrolls.
    private(set) var scrollRequest: (path: String, token: Int)?
    /// The last request the diff acted on — so a diff that opens because of
    /// a request still acts on it, and one reopened later does not act on an
    /// old one again.
    @ObservationIgnored var handledScrollToken = 0

    func requestScroll(to path: String) {
        scrollRequest = (path, (scrollRequest?.token ?? 0) + 1)
    }

    /// Forget what was marked and drafted — a different slice is on screen.
    func reset() {
        selection = nil
        draft = nil
        sendError = nil
        dropNotice = nil
        showApproveConfirm = false
    }

    func store(_ appModel: AppModel) -> DiffStore {
        appModel.diffStore(projectID: appModel.projectStore?.projectID ?? "")
    }

    // MARK: - Marking and drafting

    func handleRowClick(file: DiffFileModel, row: DiffRow, shift: Bool) {
        guard row.kind != .hunkBreak else { return }
        if shift, let current = selection, current.path == file.path, let anchorID = current.rowIDs.first,
           let anchorIndex = file.rows.firstIndex(where: { $0.id == anchorID }),
           let clickIndex = file.rows.firstIndex(where: { $0.id == row.id }) {
            let range = anchorIndex <= clickIndex ? anchorIndex...clickIndex : clickIndex...anchorIndex
            mark(DiffSelection(path: file.path, rowIDs: range.map { file.rows[$0].id }))
        } else {
            mark(DiffSelection(path: file.path, rowIDs: [row.id]))
        }
    }

    /// A drag across a file's rows marks the run it covers, already in the
    /// file's order.
    func handleRowDrag(file: DiffFileModel, rowIDs: [String]) {
        guard !rowIDs.isEmpty else { return }
        mark(DiffSelection(path: file.path, rowIDs: rowIDs))
    }

    private func mark(_ marked: DiffSelection) {
        selection = marked
        // A fresh mark that is not what the open draft is about abandons it.
        if let draft, draft.path != marked.path || draft.anchorRowIDs != marked.rowIDs {
            self.draft = nil
        }
    }

    func openCommentEditor(_ store: DiffStore) {
        guard let selection else { return }
        let existing = store.comment(path: selection.path, anchorRowIDs: selection.rowIDs)?.text ?? ""
        draft = CommentDraft(path: selection.path, anchorRowIDs: selection.rowIDs, text: existing)
    }

    func editComment(_ comment: PendingComment) {
        selection = DiffSelection(path: comment.path, rowIDs: comment.anchorRowIDs)
        draft = CommentDraft(path: comment.path, anchorRowIDs: comment.anchorRowIDs, text: comment.text)
    }

    func deleteComment(_ comment: PendingComment, store: DiffStore) {
        store.deleteComment(id: comment.id)
        if draft?.path == comment.path && draft?.anchorRowIDs == comment.anchorRowIDs {
            draft = nil
        }
    }

    func saveDraft(_ text: String, store: DiffStore) {
        guard let draft else { return }
        store.setComment(path: draft.path, anchorRowIDs: draft.anchorRowIDs, text: text)
        self.draft = nil
        selection = nil
    }

    // MARK: - Reading

    func fetch(appModel: AppModel, slice: Slice) async {
        guard let projectID = appModel.projectStore?.projectID else { return }
        let store = store(appModel)
        await store.fetch(projectID: projectID, sliceRef: slice.id)
        updateDropNotice(store)
        await store.fetchCommits(projectID: projectID, sliceRef: slice.id)
    }

    func refresh(appModel: AppModel) async {
        let store = store(appModel)
        await store.refresh()
        updateDropNotice(store)
    }

    private func updateDropNotice(_ store: DiffStore) {
        let n = store.lastDroppedCommentCount
        dropNotice = n > 0
            ? "\(n) pending \(plural(n, "comment was", "comments were")) dropped because \(plural(n, "its", "their")) lines changed."
            : nil
    }

    // MARK: - Sending and approving

    func sendComments(appModel: AppModel, slice: Slice) async {
        guard let projectID = appModel.projectStore?.projectID else { return }
        isSending = true
        sendError = nil
        do {
            // A resumed slice's agent is already at work and hands back of
            // its own accord: no hand-back line, no rework.
            _ = try await store(appModel).sendComments(
                projectID: projectID, sliceRef: slice.id, handedBack: slice.handedBack)
            await appModel.refresh()
        } catch {
            sendError = error.localizedDescription
        }
        isSending = false
    }

    /// Approving with comments pending opens no pull request now: the
    /// comments go to the agent, the slice is taken out of review, and the
    /// approve is owed to the slice's next hand-back (`approvalsPending`).
    func approve(appModel: AppModel, slice: Slice) async {
        guard let projectID = appModel.projectStore?.projectID else { return }
        let store = store(appModel)
        if store.pendingCommentCount > 0 {
            isSending = true
            sendError = nil
            do {
                try await store.sendComments(projectID: projectID, sliceRef: slice.id, approving: true)
                appModel.markApprovePending(sliceID: slice.id)
                await appModel.refresh()
            } catch {
                sendError = error.localizedDescription
            }
            isSending = false
            return
        }
        let sliceRef = slice.id
        await appModel.sliceActions.run(.approve, sliceID: sliceRef, select: { _ in }) {
            _ = try await store.approve(projectID: projectID, sliceRef: sliceRef)
            await appModel.refresh()
            // The pull request exists now; what GitHub says about it — its
            // mergeability, its checks — comes a few seconds after.
            appModel.githubActionRan()
        }
    }
}
