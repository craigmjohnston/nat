import SwiftUI
import NatKit
import NatFixtures

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
/// Comments" hands all of it to the agent as one prompt. Approving with
/// comments pending sends them the same way and owes the approve to the
/// slice's next hand-back, rather than opening a pull request over work
/// the review has just said needs fixing.
struct DiffTabView: View {
    @Bindable var appModel: AppModel
    let slice: Slice

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

    @State private var showApproveConfirm = false

    /// Approving is held by the app's `SliceActionTracker`, not by this view:
    /// approving advances the pane to the PR stage at once, unmounting this
    /// tab, and a failure returns to a fresh one that must still find the
    /// error — and the one-shot rule — where the action left them.
    private var isApproving: Bool { appModel.sliceActions.isRunning(.approve, sliceID: slice.id) }
    private var approveError: String? { appModel.sliceActions.error(.approve, sliceID: slice.id) }

    /// The file sidebar's width, draggable at its divider and remembered
    /// across launches — the default is the width it was fixed at before it
    /// was resizable.
    @AppStorage("diffSidebarWidth") private var sidebarWidth = 232.0
    @State private var liveSidebarWidth: Double?

    /// Where the file column is scrolled to, by file path. The
    /// `scrollPosition`/`scrollTargetLayout` pair rather than a
    /// `ScrollViewReader`: the reader's `scrollTo` walks a lazy stack by
    /// estimated heights, and with every file box a different size it landed
    /// the sidebar's clicks somewhere near rather than at the file.
    @State private var fileScroll = ScrollPosition(idType: String.self)

    /// The row at the top of the column, restored after a width change.
    @State private var anchor = DiffScrollAnchor()

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
        .surface(.window)
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
        DiffSkeletonView(handedBack: slice.handedBack)
    }

    private var failedState: some View {
        VStack(spacing: 8) {
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 24, weight: .regular))
                .ink(.danger)

            Text("Failed to read the diff")
                .font(.system(size: Typo.body, weight: .regular))
                .ink(.primary)

            if let message = store.loadState.errorMessage {
                Text(message)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.secondary)
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
                        .ink(.secondary)

                    Text("Nothing to show — the branch matches its base")
                        .font(.system(size: Typo.body, weight: .regular))
                        .ink(.secondary)
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
                                    comments: store.commentsByPath[file.path] ?? [],
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
                        .inelastic()
                    }
                    .scrollPosition($fileScroll, anchor: .top)
                    .coordinateSpace(name: DiffRowFramesKey.space)
                    .onPreferenceChange(DiffRowFramesKey.self) { frames in
                        anchor.update(rows: frames.map { (key: $0.key, minY: $0.value.lowerBound, maxY: $0.value.upperBound) })
                    }
                    .onScrollGeometryChange(for: Bool.self) { geometry in
                        geometry.contentSize.height > geometry.containerSize.height
                    } action: { _, canScroll in
                        anchor.canScroll = canScroll
                    }
                    // The column's width moves with the sidebar's divider, the
                    // rail's and the window: rows re-wrap and the old offset
                    // lands on other code, so the top line is put back.
                    .onGeometryChange(for: CGFloat.self) { $0.size.width } action: { old, new in
                        guard old != new, let key = anchor.beginRestore() else { return }
                        Task { @MainActor in
                            // After the re-wrap has laid out, not before.
                            await Task.yield()
                            var transaction = Transaction()
                            transaction.disablesAnimations = true
                            withTransaction(transaction) {
                                fileScroll.scrollTo(id: key.scrollID, anchor: .top)
                            }
                            anchor.endRestore()
                        }
                    }

                    diffSidebar(for: diff)
                }
            }
        }
    }

    /// The file list's own rail: the Send/Approve actions atop it — the
    /// standing inspector-top slot every pane with a rail opens with — then
    /// the file list itself, and a pinned foot for the busy mark, the
    /// viewed/pending line, and whatever notice is live.
    private func diffSidebar(for diff: DiffModel) -> some View {
        let viewedCount = diff.files.filter { store.isViewed($0.path) }.count
        let pendingCount = store.pendingCommentCount
        let commentsEditable = store.commentsEditable
        let canApprove = slice.handedBack && commentsEditable && !isSending

        return VStack(spacing: 0) {
            InspectorActionsBar {
                // Only a hand-back still awaiting approval has anything to
                // approve — a Done slice's diff is the review continuing on
                // a pull request already open, and drawing the button there
                // would offer exactly what the CLI refuses.
                if slice.handedBack {
                    Button(action: { showApproveConfirm = true }) {
                        AsyncActionLabel(isBusy: isApproving || isSending) {
                            Text("Approve & Open PR…")
                                .font(.system(size: Typo.subhead, weight: .semibold))
                        }
                    }
                    .buttonStyle(InspectorPrimaryButtonStyle())
                    .disabled(!appModel.sliceActions.isEnabled(.approve, sliceID: slice.id, available: canApprove))
                    .help(commentsEditable
                        ? Self.approveHelp(pendingCount: pendingCount)
                        : "Approving is only available while viewing All commits")
                    .onChange(of: canApprove, initial: true) { _, available in
                        appModel.sliceActions.observe(.approve, sliceID: slice.id, available: available)
                    }
                }

                // Secondary rather than primary: what this rail confirms is
                // the approval above it, and sending a review back is the
                // step before that rather than the pane's own submit.
                Button(action: { Task { await sendComments() } }) {
                    AsyncActionLabel(isBusy: isSending) {
                        Text("Send \(pendingCount) \(plural(pendingCount, "Comment", "Comments"))")
                            .font(.system(size: Typo.subhead, weight: .regular))
                            .monospacedDigit()
                    }
                }
                .buttonStyle(InspectorSecondaryButtonStyle())
                .disabled(pendingCount == 0 || isSending || !commentsEditable)
                .help(commentsEditable ? "" : "Comments are only sent while viewing All commits")
            }

            DiffFileSidebarView(
                files: diff.files,
                isViewed: { store.isViewed($0) },
                commentCount: { path in store.commentsByPath[path]?.count ?? 0 },
                commits: store.commits,
                selectedCommit: store.selectedCommit,
                onSelectCommit: { sha in Task { await store.selectCommit(sha) } },
                onSelect: { path in
                    withAnimation(Motion.stateChange) {
                        fileScroll.scrollTo(id: path, anchor: .top)
                    }
                }
            )

            InspectorStatusFoot {
                // A read that failed over a diff already on screen: what is
                // up is the last good reading, and saying so is what stops
                // it being read as the branch's current state.
                if let staleMessage = store.loadState.errorMessage {
                    InspectorNotice(text: "Showing the last reading — \(staleMessage)", role: .warning)
                }
                if let dropNotice {
                    InspectorNotice(text: dropNotice, role: .warning)
                }
                if let sendError {
                    InspectorNotice(text: sendError, role: .danger)
                }
                if let approveError {
                    InspectorNotice(text: approveError, role: .danger)
                }

                HStack(spacing: 8) {
                    // The busy mark holds its slot whether a read is running
                    // or not, so a refresh admits to itself without moving
                    // the line beside it.
                    RefreshingMark(isRefreshing: store.isRefreshing)

                    Text(footerLeftText(
                        pendingCount: pendingCount, viewedCount: viewedCount, total: diff.files.count,
                        commentsEditable: commentsEditable
                    ))
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .monospacedDigit()
                        .ink(.tertiary)

                    Spacer()
                }
            }
        }
        .frame(width: liveSidebarWidth ?? sidebarWidth)
        .rule(.separator, edges: [.leading], width: 0.5)
        .overlay(alignment: .leading) {
            PaneResizeHandle(width: sidebarWidth, liveWidth: $liveSidebarWidth, onCommit: { sidebarWidth = $0 }, minWidth: 180, maxWidth: 420, edge: .leading)
                .offset(x: -4.5)
        }
        .confirmationDialog(
            pendingCount > 0
                ? "Send \(pendingCount) \(plural(pendingCount, "comment", "comments")) to the agent, and open a pull request for \(diff.branch) once it hands back?"
                : "Approve and open a pull request for \(diff.branch)?",
            isPresented: $showApproveConfirm,
            titleVisibility: .visible
        ) {
            Button(pendingCount > 0 ? "Send & Approve on Hand-back" : "Approve & Open PR") {
                Task { await approve() }
            }
            Button("Cancel", role: .cancel) {}
        }
        // Without an icon of its own the dialog wears the app's, which for
        // an unbundled dev build is the generic document icon — a seal is
        // what the rail already marks review work with.
        .dialogIcon(Image(systemName: "checkmark.seal"))
    }

    private static func approveHelp(pendingCount: Int) -> String {
        guard pendingCount > 0 else { return "" }
        return "Sends the \(pendingCount) pending \(plural(pendingCount, "comment", "comments")) to the agent; " +
            "the pull request opens on its next hand-back, without another review."
    }

    private func footerLeftText(pendingCount: Int, viewedCount: Int, total: Int, commentsEditable: Bool) -> String {
        guard commentsEditable else {
            return "Viewing one commit — switch to All commits to comment or approve"
        }
        let viewedText = "\(viewedCount) of \(total) viewed"
        guard pendingCount > 0 else { return viewedText }
        return "\(pendingCount) pending \(plural(pendingCount, "comment", "comments")) · \(viewedText)"
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
            await appModel.refresh()
        } catch {
            sendError = error.localizedDescription
        }
        isSending = false
    }

    private func approve() async {
        guard let projectID = appModel.projectStore?.projectID else { return }
        if store.pendingCommentCount > 0 {
            await approveOverComments(projectID: projectID)
            return
        }
        let sliceRef = slice.id
        let store = store
        let appModel = appModel
        await appModel.sliceActions.run(.approve, sliceID: sliceRef, select: { _ in }) {
            _ = try await store.approve(projectID: projectID, sliceRef: sliceRef)
            await appModel.refresh()
        }
    }

    /// Approving with comments pending opens no pull request now: the
    /// comments go to the agent, the slice is taken out of review, and the
    /// approve is left owed to the slice's next hand-back (`AppModel`'s
    /// `approvalsPending`). Marked pending only once both writes have
    /// landed — a failed send leaves the comments and the review as they
    /// were, and a rework that failed after the send says so, since the agent
    /// has its instructions but nothing will approve what it hands back.
    private func approveOverComments(projectID: String) async {
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

#Preview("Handed back") {
    @Previewable @State var appModel = Fixtures.appModel()
    let slice = Fixtures.slices.first { $0.id == Fixtures.mergeBoxSliceID }!

    DiffTabView(appModel: appModel, slice: slice)
        .frame(width: 900, height: 500)
        .task { await Fixtures.start(appModel) }
}

#Preview("Unreadable branch") {
    @Previewable @State var appModel = Fixtures.appModel(
        client: FixtureNatClient(behaviour: .refusing(Fixtures.diffErrorMessage))
    )
    let slice = Fixtures.slices.first { $0.id == Fixtures.mergeBoxSliceID }!

    DiffTabView(appModel: appModel, slice: slice)
        .frame(width: 900, height: 500)
        .task { await Fixtures.start(appModel) }
}
