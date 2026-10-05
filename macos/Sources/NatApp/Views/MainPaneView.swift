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

/// A notice across the top of the main pane, over what it shows — a resumed
/// slice's warning that the diff, images or pull request beneath are of work
/// the agent is redoing (`NavigatorModel.resumedNotice`). The navigator's
/// own `NavNotice`, ruled off from the pane under it.
struct MainPaneNotice: View {
    let text: String
    var role: InkRole = .warning

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 12))
                .ink(role)
            Text(text)
                .font(.system(size: Typo.scaled(13)))
                .ink(role)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 8)
        .frame(maxWidth: .infinity, alignment: .leading)
        .rule(.separator, edges: [.bottom], width: 1)
    }
}

/// The main pane with nothing selected, an editor's watermark: the gnat
/// mark, large and barely there, over the keyboard shortcuts that work with
/// nothing selected — each the menu bar's own. Set a little above centre,
/// where the eye rests in an empty pane. The two columns are held to one
/// width, so the gap between action and keys falls on the mark's own axis
/// rather than wherever the longest action happens to push it.
///
/// With a selection whose main pane has nothing to show — no agent yet —
/// it is the mark alone (`showsShortcuts` off), centred: the shortcuts are
/// the nothing-selected ones.
struct MainPaneEmptyState: View {
    var showsShortcuts = true

    private static let shortcuts: [(action: String, keys: String)] = [
        ("New task", "\u{2318}N"),
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
            if showsShortcuts { shortcutGrid }
        }
        .padding(24)
        .offset(y: showsShortcuts ? -32 : 0)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private var shortcutGrid: some View {
        Grid(alignment: .leading, horizontalSpacing: 28, verticalSpacing: 9) {
            ForEach(Self.shortcuts, id: \.action) { shortcut in
                GridRow {
                    Text(shortcut.action)
                        .font(.system(size: Typo.scaled(12.5)))
                        .ink(.tertiary)
                        .frame(maxWidth: .infinity, alignment: .trailing)
                    Text(shortcut.keys)
                        .font(Typo.mono(size: Typo.subhead))
                        .tracking(1.5)
                        .ink(.quaternary)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
        }
    }
}

/// A live agent's terminal, full-bleed on the terminal ground — or, with
/// none, the empty pane's mark, or `emptyText` where there is something to
/// say instead.
struct AgentTerminalPane: View {
    let session: String?
    var emptyText: String?
    var focusRequest = 0
    var sessionExists: () -> Bool = { true }
    @Environment(\.terminalStubbed) private var terminalStubbed

    init(
        agent: AgentStatus?, emptyText: String? = nil, focusRequest: Int = 0,
        sessionExists: @escaping () -> Bool = { true }
    ) {
        self.session = agent?.session
        self.emptyText = emptyText
        self.focusRequest = focusRequest
        self.sessionExists = sessionExists
    }

    var body: some View {
        if let session {
            ZStack {
                DesignTokens.fill(.terminal)
                Group {
                    if terminalStubbed {
                        TerminalStubView(session: session)
                    } else {
                        AgentTerminalHostView(
                            attachSpec: AttachSpec(session: session),
                            sessionExists: sessionExists,
                            onExit: { _ in },
                            focusRequest: focusRequest
                        )
                        .id(session)
                    }
                }
                .padding(.vertical, 14)
                .padding(.horizontal, 20)
            }
        } else if let emptyText {
            MainPaneNote(text: emptyText)
        } else {
            MainPaneEmptyState(showsShortcuts: false)
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
            MainPaneNote(text: "The branch matches its base, so there is nothing to show")
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
        if let store {
            state.badges = Dictionary(
                paths.compactMap { path in store.badge(path).map { (path, $0) } }, uniquingKeysWith: { first, _ in first })
        }
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
        // A file whose rows have been on screen is seen. After the view has
        // drawn, never during — the store is observed by what is drawing.
        actions.filesShown = { paths in
            Task { @MainActor in paths.forEach(store.markSeen) }
        }
        actions.gapExpanded = { file, gap, control in
            Task { await store.expand(path: file.path, gap: gap, control: control) }
        }
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

/// A slice's main pane, under the titlebar band: its terminal, its
/// branch's diff, its pull request's conversation, or the note.
struct SliceMainPane: View {
    @Bindable var appModel: AppModel
    let slice: Slice
    @Binding var mode: MainPaneMode
    let review: DiffReview
    let visualReview: VisualReview

    /// Nil while the slice's detail has not loaded (`VisualsPane.handIn`).
    private var visuals: [VisualChange]? {
        appModel.sliceDetailStore(projectID: appModel.projectStore?.projectID ?? "")
            .state(for: slice.id).detail?.visuals
    }

    private var nav: NavigatorModel {
        NavigatorModel(
            slice: slice, agent: appModel.activityStore?.agents[slice.id].map { AgentActivity($0.activity) })
    }

    var body: some View {
        VStack(spacing: 0) {
            // A resumed slice's diff, images and pull request are of work the
            // agent is redoing: said once, across the top of each.
            if nav.worksAgain && (mode == .diff || mode == .visuals || mode == .pr) {
                MainPaneNotice(text: NavigatorModel.resumedNotice)
            }
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
            case .visuals:
                VisualsPane(
                    appModel: appModel, review: visualReview, slice: slice, handIn: visuals,
                    authorName: appModel.config?.assigneeUserName ?? "You")
            case .pr:
                PRConversationPane(
                    store: appModel.prStore(projectID: appModel.projectStore?.projectID ?? ""),
                    expectedNumber: pullRequestNumber(slice.pr))
            case .empty:
                MainPaneEmptyState(showsShortcuts: false)
            }
        }
        .surface(.window)
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
            MainPaneNote(text: "The diff could not be read: \(message)")
        } else {
            QuietLoadingView(label: "Reading the branch")
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }
}
