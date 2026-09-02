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

/// The Diff tab: the unified diff of a handed-back slice's branch, read
/// through `DiffStore` and drawn as one GitHub-style box per file beside a
/// file-list sidebar. Clicking a line (or shift-clicking to extend a range
/// within the same file) marks it as a comment's anchor; the review left
/// there is ephemeral — held only in `DiffStore.comments` — until "Send N
/// Comments" hands all of it to the agent as one prompt. Approving is
/// blocked while anything is still pending: a review with something left to
/// say is not one that approves the work.
struct DiffTabView: View {
    @Bindable var appModel: AppModel
    let slice: Slice
    var onApproved: () -> Void = {}

    @State private var store = DiffStore()

    @State private var selection: DiffSelection?
    @State private var draft: CommentDraft?

    @State private var isSending = false
    @State private var sendError: String?
    @State private var dropNotice: String?

    @State private var isApproving = false
    @State private var approveError: String?
    @State private var showApproveConfirm = false

    private var authorName: String { appModel.config?.assigneeUserName ?? "You" }
    private var authorInitials: String { initialsFor(appModel.config?.assigneeUserName) }

    var body: some View {
        VStack(spacing: 0) {
            switch store.loadState {
            case .idle, .loading:
                loadingState

            case .loaded(let diff):
                content(for: diff)

            case .failed:
                failedState
            }
        }
        .background(DesignTokens.windowBg)
        .task {
            await fetch()
        }
        .onChange(of: slice.id) { _, _ in
            selection = nil
            draft = nil
            Task { await fetch() }
        }
    }

    // MARK: - States

    private var loadingState: some View {
        VStack(spacing: 8) {
            if store.loadState.isLoading {
                ProgressView()
            }
            Text("Reading the diff of \(slice.branch ?? "the branch")…")
                .font(.system(size: 13, weight: .regular))
                .foregroundStyle(DesignTokens.labelSecondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private var failedState: some View {
        VStack(spacing: 8) {
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 24, weight: .regular))
                .foregroundStyle(DesignTokens.systemRed)

            Text("Failed to read the diff")
                .font(.system(size: 13, weight: .regular))
                .foregroundStyle(DesignTokens.label)

            if let message = store.loadState.errorMessage {
                Text(message)
                    .font(.system(size: 11, weight: .regular))
                    .foregroundStyle(DesignTokens.labelSecondary)
                    .lineLimit(3)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: 420)
            }

            Button("Retry") {
                Task { await refreshDiff() }
            }
            .buttonStyle(.bordered)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    // MARK: - Loaded content

    private func content(for diff: DiffModel) -> some View {
        VStack(spacing: 0) {
            if diff.files.isEmpty {
                VStack(spacing: 8) {
                    Image(systemName: "plus.forwardslash.minus")
                        .font(.system(size: 32, weight: .regular))
                        .foregroundStyle(DesignTokens.labelSecondary)

                    Text("Nothing to show — the branch matches its base")
                        .font(.system(size: 13, weight: .regular))
                        .foregroundStyle(DesignTokens.labelSecondary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                ScrollViewReader { proxy in
                    HStack(spacing: 0) {
                        ScrollView {
                            LazyVStack(alignment: .leading, spacing: 10) {
                                ForEach(diff.files) { file in
                                    DiffFileBoxView(
                                        file: file,
                                        numberWidth: diff.numberWidth,
                                        isViewed: store.isViewed(file.path),
                                        isCollapsed: store.isCollapsed(file.path),
                                        comments: store.comments.filter { $0.path == file.path },
                                        selection: selection?.path == file.path ? selection : nil,
                                        draft: draft?.path == file.path ? draft : nil,
                                        authorName: authorName,
                                        authorInitials: authorInitials,
                                        onToggleViewed: { store.toggleViewed(file.path) },
                                        onToggleCollapsed: { store.toggleCollapsed(file.path) },
                                        onRowClick: { row, shift in handleRowClick(file: file, row: row, shift: shift) },
                                        onOpenCommentEditor: openCommentEditor,
                                        onEditComment: editComment,
                                        onDeleteComment: deleteComment,
                                        onSaveDraft: saveDraft,
                                        onCancelDraft: { draft = nil }
                                    )
                                    .id(file.path)
                                }
                            }
                            .padding(14)
                        }

                        DiffFileSidebarView(
                            files: diff.files,
                            isViewed: { store.isViewed($0) },
                            commentCount: { path in store.comments.filter { $0.path == path }.count },
                            onSelect: { path in
                                withAnimation {
                                    proxy.scrollTo(path, anchor: .top)
                                }
                            }
                        )
                        .frame(width: 232)
                        .rectBorder(width: 0.5, edges: [.leading], color: DesignTokens.separator)
                    }
                }

                footer(for: diff)
            }
        }
    }

    private func footer(for diff: DiffModel) -> some View {
        let viewedCount = diff.files.filter { store.isViewed($0.path) }.count
        let pendingCount = store.pendingCommentCount

        return VStack(spacing: 0) {
            if let dropNotice {
                inlineNotice(dropNotice, color: DesignTokens.systemOrange)
            }
            if let sendError {
                inlineNotice(sendError, color: DesignTokens.systemRed)
            }
            if let approveError {
                inlineNotice(approveError, color: DesignTokens.systemRed)
            }

            Divider()
                .frame(height: 0.5)

            HStack(spacing: 8) {
                Text(footerLeftText(pendingCount: pendingCount, viewedCount: viewedCount, total: diff.files.count))
                    .font(.system(size: 12, weight: .regular))
                    .foregroundStyle(DesignTokens.labelTertiary)

                Spacer()

                Button(action: { Task { await sendComments() } }) {
                    Text("Send \(pendingCount) \(plural(pendingCount, "Comment", "Comments"))")
                        .font(.system(size: 12, weight: .regular))
                }
                .buttonStyle(.bordered)
                .disabled(pendingCount == 0 || isSending)

                Button(action: { showApproveConfirm = true }) {
                    Text("Approve & Open PR…")
                        .font(.system(size: 12, weight: .semibold))
                }
                .buttonStyle(.borderedProminent)
                .tint(DesignTokens.accent)
                .disabled(pendingCount > 0 || isApproving)
                .help(approveHelp(pendingCount: pendingCount))
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
        }
        .confirmationDialog(
            "Approve and open a pull request for \(diff.branch)?",
            isPresented: $showApproveConfirm,
            titleVisibility: .visible
        ) {
            Button("Approve & Open PR") { Task { await approve() } }
            Button("Cancel", role: .cancel) {}
        }
    }

    private func inlineNotice(_ text: String, color: Color) -> some View {
        HStack {
            Text(text)
                .font(.system(size: 11, weight: .regular))
                .foregroundStyle(color)
                .lineLimit(2)
            Spacer()
        }
        .padding(.horizontal, 14)
        .padding(.top, 6)
    }

    private func footerLeftText(pendingCount: Int, viewedCount: Int, total: Int) -> String {
        let viewedText = "\(viewedCount) of \(total) viewed"
        guard pendingCount > 0 else { return viewedText }
        return "\(pendingCount) pending \(plural(pendingCount, "comment", "comments")) · \(viewedText)"
    }

    private func approveHelp(pendingCount: Int) -> String {
        guard pendingCount > 0 else { return "" }
        return "Send or clear the \(pendingCount) pending \(plural(pendingCount, "comment", "comments")) first — " +
            "a review with something left to say is not one that approves the work."
    }

    // MARK: - Selection & comment editing

    private func handleRowClick(file: DiffFileModel, row: DiffRow, shift: Bool) {
        guard row.kind != .hunkBreak else { return }

        if shift, let current = selection, current.path == file.path, let anchorID = current.rowIDs.first,
           let anchorIndex = file.rows.firstIndex(where: { $0.id == anchorID }),
           let clickIndex = file.rows.firstIndex(where: { $0.id == row.id }) {
            let range = anchorIndex <= clickIndex ? anchorIndex...clickIndex : clickIndex...anchorIndex
            selection = DiffSelection(path: file.path, rowIDs: range.map { file.rows[$0].id })
        } else {
            selection = DiffSelection(path: file.path, rowIDs: [row.id])
        }

        // A fresh click that is not what the open draft is about abandons it.
        if let draft, draft.path != selection?.path || draft.anchorRowIDs != selection?.rowIDs {
            self.draft = nil
        }
    }

    private func openCommentEditor() {
        guard let selection else { return }
        let existing = store.comment(path: selection.path, anchorRowIDs: selection.rowIDs)?.text ?? ""
        draft = CommentDraft(path: selection.path, anchorRowIDs: selection.rowIDs, text: existing)
    }

    private func editComment(_ comment: PendingComment) {
        selection = DiffSelection(path: comment.path, rowIDs: comment.anchorRowIDs)
        draft = CommentDraft(path: comment.path, anchorRowIDs: comment.anchorRowIDs, text: comment.text)
    }

    private func deleteComment(_ comment: PendingComment) {
        store.deleteComment(id: comment.id)
        if draft?.path == comment.path && draft?.anchorRowIDs == comment.anchorRowIDs {
            draft = nil
        }
    }

    private func saveDraft(_ text: String) {
        guard let draft else { return }
        store.setComment(path: draft.path, anchorRowIDs: draft.anchorRowIDs, text: text)
        self.draft = nil
        selection = nil
    }

    // MARK: - Sending & approving

    private func sendComments() async {
        guard let projectID = appModel.projectStore?.projectID else { return }
        isSending = true
        sendError = nil
        do {
            _ = try await store.sendComments(projectID: projectID, sliceRef: slice.id)
        } catch {
            sendError = error.localizedDescription
        }
        isSending = false
    }

    private func approve() async {
        guard let projectID = appModel.projectStore?.projectID else { return }
        isApproving = true
        approveError = nil
        do {
            _ = try await store.approve(projectID: projectID, sliceRef: slice.id)
            isApproving = false
            await appModel.refresh()
            onApproved()
        } catch {
            isApproving = false
            approveError = error.localizedDescription
        }
    }

    // MARK: - Fetching

    private func fetch() async {
        guard let projectID = appModel.projectStore?.projectID else { return }
        await store.fetch(projectID: projectID, sliceRef: slice.id)
        updateDropNotice()
    }

    private func refreshDiff() async {
        await store.refresh()
        updateDropNotice()
    }

    private func updateDropNotice() {
        let n = store.lastDroppedCommentCount
        guard n > 0 else {
            dropNotice = nil
            return
        }
        dropNotice = "\(n) pending \(plural(n, "comment", "comments")) dropped — " +
            "\(plural(n, "its", "their")) lines changed."
    }
}

#Preview {
    let appModel = AppModel()
    let slice = Slice(
        id: "test-id",
        name: "Test Slice",
        status: "In progress",
        milestoneID: "m1",
        assignee: "Craig",
        pr: "",
        url: "https://example.com",
        branch: "feature/test",
        repo: "/path/to/repo",
        dependsOn: nil,
        blocked: false,
        handedBack: true
    )

    DiffTabView(appModel: appModel, slice: slice)
        .frame(width: 900, height: 500)
}
