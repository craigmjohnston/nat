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

    /// The project's shared diff cache, rather than a `DiffStore` local to
    /// this view — reading through the same instance across tab switches
    /// and slice reselection is what lets a branch already read this session
    /// show up instantly instead of behind a spinner again.
    private var store: DiffStore {
        appModel.diffStore(projectID: appModel.projectStore?.projectID ?? "")
    }

    @State private var selection: DiffSelection?
    @State private var draft: CommentDraft?

    @State private var isSending = false
    @State private var sendError: String?
    @State private var dropNotice: String?

    @State private var isApproving = false
    @State private var approveError: String?
    @State private var showApproveConfirm = false

    /// The file sidebar's width, draggable at its divider and remembered
    /// across launches — the default is the width it was fixed at before it
    /// was resizable.
    @AppStorage("diffSidebarWidth") private var sidebarWidth = 232.0

    /// Where the file column is scrolled to, by file path. The
    /// `scrollPosition`/`scrollTargetLayout` pair rather than a
    /// `ScrollViewReader`: the reader's `scrollTo` walks a lazy stack by
    /// estimated heights, and with every file box a different size it landed
    /// the sidebar's clicks somewhere near rather than at the file.
    @State private var fileScroll = ScrollPosition(idType: String.self)

    private var authorName: String { appModel.config?.assigneeUserName ?? "You" }
    private var authorInitials: String { initialsFor(appModel.config?.assigneeUserName) }

    var body: some View {
        VStack(spacing: 0) {
            // A reading already on screen wins over the state that replaced
            // it: a refresh keeps its diff (and says so with the footer's
            // busy mark), and a read that failed over one keeps it too, with
            // git's own words in a notice above the footer. Only a read with
            // nothing ever behind it draws the skeleton or the failure.
            if let diff = store.loadState.diff {
                content(for: diff)
            } else if case .failed = store.loadState {
                failedState
            } else {
                loadingState
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

    /// The branch's first read, drawn as the diff it is about to be — see
    /// `DiffSkeletonView`. A re-read never reaches this: it keeps the diff
    /// it has.
    private var loadingState: some View {
        DiffSkeletonView()
    }

    private var failedState: some View {
        VStack(spacing: 8) {
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 24, weight: .regular))
                .foregroundStyle(DesignTokens.systemRedInk(on: .window))

            Text("Failed to read the diff")
                .font(.system(size: Typo.body, weight: .regular))
                .foregroundStyle(DesignTokens.label)

            if let message = store.loadState.errorMessage {
                Text(message)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(DesignTokens.labelSecondary)
                    .lineLimit(3)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: 420)
            }

            Button("Retry") {
                Task { await refreshDiff() }
            }
            .buttonStyle(SecondaryButtonStyle())
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
                        .font(.system(size: Typo.body, weight: .regular))
                        .foregroundStyle(DesignTokens.labelSecondary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
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
                                    commentsEnabled: store.commentsEditable,
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
                            }
                        }
                        .scrollTargetLayout()
                        .padding(14)
                    }
                    .scrollPosition($fileScroll, anchor: .top)

                    DiffFileSidebarView(
                        files: diff.files,
                        isViewed: { store.isViewed($0) },
                        commentCount: { path in store.comments.filter { $0.path == path }.count },
                        commits: store.commits,
                        selectedCommit: store.selectedCommit,
                        onSelectCommit: { sha in Task { await store.selectCommit(sha) } },
                        onSelect: { path in
                            withAnimation(Motion.stateChange) {
                                fileScroll.scrollTo(id: path, anchor: .top)
                            }
                        }
                    )
                    .frame(width: sidebarWidth)
                    .rectBorder(width: 0.5, edges: [.leading], color: DesignTokens.separator(on: .window))
                    .overlay(alignment: .leading) {
                        PaneResizeHandle(width: $sidebarWidth, minWidth: 180, maxWidth: 420, edge: .leading)
                            .offset(x: -4.5)
                    }
                }

                footer(for: diff)
            }
        }
    }

    private func footer(for diff: DiffModel) -> some View {
        let viewedCount = diff.files.filter { store.isViewed($0.path) }.count
        let pendingCount = store.pendingCommentCount
        let commentsEditable = store.commentsEditable

        return VStack(spacing: 0) {
            // A read that failed over a diff already on screen: what is up is
            // the last good reading, and saying so is what stops it being
            // read as the branch's current state.
            if let staleMessage = store.loadState.errorMessage {
                inlineNotice("Showing the last reading — \(staleMessage)", color: DesignTokens.systemOrange)
            }
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
                // The busy mark holds its slot whether a read is running or
                // not, so a refresh admits to itself without moving the line
                // beside it.
                RefreshingMark(isRefreshing: store.isRefreshing)

                Text(footerLeftText(
                    pendingCount: pendingCount, viewedCount: viewedCount, total: diff.files.count,
                    commentsEditable: commentsEditable
                ))
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .monospacedDigit()
                    .foregroundStyle(DesignTokens.labelTertiary)

                Spacer()

                Button(action: { Task { await sendComments() } }) {
                    AsyncActionLabel(isBusy: isSending) {
                        Text("Send \(pendingCount) \(plural(pendingCount, "Comment", "Comments"))")
                            .font(.system(size: Typo.subhead, weight: .regular))
                            .monospacedDigit()
                    }
                }
                // Secondary rather than primary: what this footer confirms
                // is the approval beside it, and sending a review back is the
                // step before that rather than the pane's own submit.
                .buttonStyle(SecondaryButtonStyle())
                .disabled(pendingCount == 0 || isSending || !commentsEditable)
                .help(commentsEditable ? "" : "Comments are only sent while viewing All commits")

                // Only a hand-back still awaiting approval has anything to
                // approve — a Done slice's diff is the review continuing on
                // a pull request already open, and drawing the button there
                // would offer exactly what the CLI refuses.
                if slice.handedBack {
                    Button(action: { showApproveConfirm = true }) {
                        AsyncActionLabel(isBusy: isApproving) {
                            Text("Approve & Open PR…")
                                .font(.system(size: Typo.subhead, weight: .semibold))
                        }
                    }
                    .buttonStyle(PrimaryButtonStyle())
                    .disabled(pendingCount > 0 || isApproving || !commentsEditable)
                    .help(commentsEditable
                        ? approveHelp(pendingCount: pendingCount)
                        : "Approving is only available while viewing All commits")
                }
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
        // Without an icon of its own the dialog wears the app's, which for
        // an unbundled dev build is the generic document icon — a seal is
        // what the rail already marks review work with.
        .dialogIcon(Image(systemName: "checkmark.seal"))
    }

    private func inlineNotice(_ text: String, color: Color) -> some View {
        HStack {
            Text(text)
                .font(.system(size: Typo.subhead, weight: .regular))
                .monospacedDigit()
                .foregroundStyle(color)
                .lineLimit(2)
            Spacer()
        }
        .padding(.horizontal, 14)
        .padding(.top, 6)
    }

    private func footerLeftText(pendingCount: Int, viewedCount: Int, total: Int, commentsEditable: Bool) -> String {
        guard commentsEditable else {
            return "Viewing one commit — switch to All commits to comment or approve"
        }
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
        await store.fetchCommits(projectID: projectID, sliceRef: slice.id)
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
