import SwiftUI
import NatKit

/// What the main pane says with nothing to show.
struct MainPaneNote: View {
    let text: String

    var body: some View {
        Text(text)
            .font(.system(size: GnatMetrics.body))
            .ink(.secondary)
            .multilineTextAlignment(.center)
            .padding(24)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

/// The main pane with nothing selected, an editor's watermark: the gnat
/// mark, large and barely there, over the keyboard shortcuts that work with
/// nothing selected — each the menu bar's own. Set a little above centre,
/// where the eye rests in an empty pane. The two columns are held to one
/// width, so the gap between action and keys falls on the mark's own axis
/// rather than wherever the longest action happens to push it.
struct MainPaneEmptyState: View {
    private static let shortcuts: [(action: String, keys: String)] = [
        ("New slice", "\u{2318}N"),
        ("New milestone", "\u{2325}\u{2318}N"),
        ("New ad hoc session", "\u{2303}\u{2318}N"),
        ("New project", "\u{21E7}\u{2318}N"),
        ("Show or hide done items", "\u{21E7}\u{2318}."),
        ("Refresh", "\u{2318}R"),
    ]

    var body: some View {
        VStack(spacing: 36) {
            GnatMark(color: DesignTokens.ink(.quaternary, on: .window))
                .frame(width: 88, height: 88)
                .opacity(0.6)
            Grid(alignment: .leading, horizontalSpacing: 28, verticalSpacing: 9) {
                ForEach(Self.shortcuts, id: \.action) { shortcut in
                    GridRow {
                        Text(shortcut.action)
                            .font(.system(size: 12.5))
                            .ink(.tertiary)
                            .frame(maxWidth: .infinity, alignment: .trailing)
                        Text(shortcut.keys)
                            .font(Typo.mono(size: 12))
                            .tracking(1.5)
                            .ink(.quaternary)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
            }
        }
        .padding(24)
        .offset(y: -32)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

/// A live agent's terminal, full-bleed on the terminal ground, or the note
/// saying there is none.
struct AgentTerminalPane: View {
    let agent: AgentStatus?
    var emptyText = "No agent is running. Launch starts one, and its terminal opens here."
    var focusRequest = 0
    var sessionExists: () -> Bool = { true }
    @Environment(\.terminalStubbed) private var terminalStubbed

    var body: some View {
        if let agent {
            ZStack {
                DesignTokens.fill(.terminal)
                Group {
                    if terminalStubbed {
                        TerminalStubView(session: agent.session)
                    } else {
                        AgentTerminalHostView(
                            attachSpec: AttachSpec(session: agent.session),
                            sessionExists: sessionExists,
                            onExit: { _ in },
                            focusRequest: focusRequest
                        )
                        .id(agent.session)
                    }
                }
                .padding(.vertical, 14)
                .padding(.horizontal, 20)
            }
        } else {
            MainPaneNote(text: emptyText)
        }
    }
}

/// The design's continuous diff: every file one after another, its header
/// pinned while its rows scroll under it, folded by a click on that header.
/// On a review the rows take comments — marked, drafted and pending, all
/// through `review` — and each header carries its viewed mark.
struct ContinuousDiffView: View {
    let diff: DiffModel
    let isViewed: (String) -> Bool
    let isCollapsed: (String) -> Bool
    let onToggleViewed: (String) -> Void
    let onToggleCollapsed: (String) -> Void
    var showsViewed = true
    var review: DiffReview?
    var store: DiffStore?
    var authorName = "You"
    var authorInitials = ""

    /// View ▸ Wrap lines in diffs.
    @AppStorage(diffWrapsLinesKey) private var wrapsLines = true

    var body: some View {
        if diff.files.isEmpty {
            MainPaneNote(text: "Nothing to show — the branch matches its base")
        } else {
            DiffCanvasRepresentable(
                files: diff.files, state: state, attachments: attachments, actions: actions,
                review: review, store: store, authorName: authorName, authorInitials: authorInitials)
        }
    }

    private var state: DiffCanvasState {
        var state = DiffCanvasState()
        let paths = diff.files.map(\.path)
        state.viewed = Set(paths.filter(isViewed))
        state.collapsed = Set(paths.filter(isCollapsed))
        state.commentCounts = store?.commentsByPath.mapValues(\.count) ?? [:]
        state.selection = review?.selection.map { DiffCanvasSelection(path: $0.path, rowIDs: $0.rowIDs) }
        state.canComment = review != nil && store != nil && (store?.commentsEditable ?? false) && review?.draft == nil
        state.showsViewed = showsViewed
        state.wrap = wrapsLines
        return state
    }

    /// What goes under each row: the editor open on it, and its pending
    /// comments, keyed by the last row each covers.
    private var attachments: [String: DiffAttachmentContent] {
        var contents: [String: DiffAttachmentContent] = [:]
        for comment in store?.comments ?? [] {
            guard let last = comment.anchorRowIDs.last else { continue }
            contents[DiffLayout.key(path: comment.path, rowID: last), default: DiffAttachmentContent()].comments.append(comment)
        }
        if let draft = review?.draft, let last = draft.anchorRowIDs.last {
            contents[DiffLayout.key(path: draft.path, rowID: last), default: DiffAttachmentContent()].draft = draft
        }
        return contents
    }

    private var actions: DiffCanvasActions {
        var actions = DiffCanvasActions()
        let review = review, store = store
        let onToggleViewed = onToggleViewed, onToggleCollapsed = onToggleCollapsed
        actions.viewedToggled = { onToggleViewed($0) }
        actions.collapseToggled = { onToggleCollapsed($0) }
        // A diff with no store takes no marks — a session's diff.
        guard let review, let store else { return actions }
        actions.rowClicked = { file, row, shift in review.handleRowClick(file: file, row: row, shift: shift) }
        actions.rowsDragged = { file, rowIDs in review.handleRowDrag(file: file, rowIDs: rowIDs) }
        actions.commentRequested = { file, row, endsSelection in
            // A hovered row that is not the end of the marked run is marked
            // first, so the comment is about it.
            if !endsSelection { review.handleRowClick(file: file, row: row, shift: false) }
            review.openCommentEditor(store)
        }
        return actions
    }
}

/// A slice's main pane, under its heading: its terminal, its branch's diff,
/// its pull request's conversation, or the note.
struct SliceMainPane: View {
    @Bindable var appModel: AppModel
    let slice: Slice
    @Binding var mode: MainPaneMode
    let review: DiffReview

    private var nav: NavigatorModel {
        NavigatorModel(
            slice: slice, agent: appModel.activityStore?.agents[slice.id].map { AgentActivity($0.activity) },
            fixLaunched: appModel.fixLaunched[slice.id] != nil)
    }

    var body: some View {
        let nav = nav
        VStack(spacing: 0) {
            heading
            switch mode {
            case .terminal:
                if appModel.sliceActions.advance(for: slice.id)?.to == .agent {
                    AgentSkeletonView()
                } else {
                    AgentTerminalPane(
                        agent: appModel.activityStore?.agents[slice.id],
                        sessionExists: { appModel.activityStore?.agents[slice.id] != nil })
                }
            case .diff:
                diffPane
            case .pr:
                PRConversationPane(
                    store: appModel.prStore(projectID: appModel.projectStore?.projectID ?? ""),
                    expectedNumber: pullRequestNumber(slice.pr))
            case .empty:
                MainPaneNote(text: "The terminal opens here on launch.")
            }
        }
        .surface(.window)
    }

    /// The agent's model, effort and context on the left; the diff's commit
    /// switcher on the right; or nothing.
    private var heading: some View {
        MainPaneHeader {
            switch mode {
            case .terminal:
                AgentModelHeading(agent: appModel.activityStore?.agents[slice.id])
            case .diff:
                let store = review.store(appModel)
                Spacer(minLength: 0)
                DiffCommitsMenu(
                    commits: store.commits,
                    selectedCommit: store.selectedCommit,
                    onSelectCommit: { sha in Task { await store.selectCommit(sha) } },
                    bottomPadding: 0)
                    .fixedSize()
            case .pr, .empty:
                EmptyView()
            }
        }
    }

    @ViewBuilder
    private var diffPane: some View {
        let store = review.store(appModel)
        if let diff = store.loadState.diff {
            ContinuousDiffView(
                diff: diff,
                isViewed: { store.isViewed($0) },
                isCollapsed: { store.isCollapsed($0) },
                onToggleViewed: { store.toggleViewed($0) },
                onToggleCollapsed: { store.toggleCollapsed($0) },
                showsViewed: nav.showsReviewActions,
                review: review,
                store: store,
                authorName: appModel.config?.assigneeUserName ?? "You",
                authorInitials: initialsFor(appModel.config?.assigneeUserName))
        } else if let message = store.loadState.errorMessage {
            MainPaneNote(text: "Failed to read the diff — \(message)")
        } else {
            QuietLoadingView(label: "Reading the branch")
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }
}
