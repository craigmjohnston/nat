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
/// where the eye rests in an empty pane.
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
                            .gridColumnAlignment(.trailing)
                        Text(shortcut.keys)
                            .font(Typo.mono(size: 12))
                            .tracking(1.5)
                            .ink(.quaternary)
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

    @State private var fileScroll = ScrollPosition(idType: String.self)
    @State private var anchor = DiffScrollAnchor()

    var body: some View {
        if diff.files.isEmpty {
            MainPaneNote(text: "Nothing to show — the branch matches its base")
        } else {
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 0, pinnedViews: [.sectionHeaders]) {
                    ForEach(diff.files) { file in
                        Section {
                            if !isCollapsed(file.path) {
                                box(file).rows
                            }
                        } header: {
                            DiffFileHeaderView(
                                file: file, isViewed: isViewed(file.path), isCollapsed: isCollapsed(file.path),
                                commentCount: store?.commentsByPath[file.path]?.count ?? 0,
                                showsViewed: showsViewed,
                                showsTopRule: file.id != diff.files.first?.id,
                                onToggleViewed: { onToggleViewed(file.path) },
                                onToggleCollapsed: { onToggleCollapsed(file.path) })
                            .id(file.path)
                        }
                    }
                    Text("\(diff.files.count) \(plural(diff.files.count, "file", "files"))")
                        .monoXS()
                        .ink(.secondary)
                        .padding(14)
                        .frame(maxWidth: .infinity)
                }
                .scrollTargetLayout()
                .inelastic()
            }
            .thinScrollers(position: $fileScroll)
            .scrollPosition($fileScroll, anchor: .top)
            .coordinateSpace(name: DiffRowFramesKey.space)
            .onPreferenceChange(DiffRowFramesKey.self) { frames in
                anchor.update(rows: frames.map { (key: $0.key, minY: $0.value.lowerBound, maxY: $0.value.upperBound) })
            }
            .onScrollGeometryChange(for: Bool.self) { $0.contentSize.height > $0.containerSize.height } action: { _, canScroll in
                anchor.canScroll = canScroll
            }
            // A width change re-wraps rows and the old offset lands on other
            // code, so the top line is put back after the re-wrap.
            .onGeometryChange(for: CGFloat.self) { $0.size.width } action: { old, new in
                guard old != new, let key = anchor.beginRestore() else { return }
                Task { @MainActor in
                    await Task.yield()
                    var transaction = Transaction()
                    transaction.disablesAnimations = true
                    withTransaction(transaction) { fileScroll.scrollTo(id: key.scrollID, anchor: .top) }
                    anchor.endRestore()
                }
            }
            .onChange(of: review?.scrollRequest?.token) { _, _ in
                guard let path = review?.scrollRequest?.path else { return }
                withAnimation(Motion.stateChange) { fileScroll.scrollTo(id: path, anchor: .top) }
            }
            .font(Typo.mono(size: Typo.code))
        }
    }

    private func box(_ file: DiffFileModel) -> DiffFileBoxView {
        DiffFileBoxView(
            file: file,
            numberWidth: diff.numberWidth,
            isViewed: isViewed(file.path),
            isCollapsed: isCollapsed(file.path),
            comments: store?.commentsByPath[file.path] ?? [],
            selection: review?.selection?.path == file.path ? review?.selection : nil,
            draft: review?.draft?.path == file.path ? review?.draft : nil,
            commentsEnabled: review != nil && (store?.commentsEditable ?? false),
            authorName: authorName,
            authorInitials: authorInitials,
            onToggleViewed: { onToggleViewed(file.path) },
            onToggleCollapsed: { onToggleCollapsed(file.path) },
            onRowClick: { row, shift in if store != nil { review?.handleRowClick(file: file, row: row, shift: shift) } },
            onOpenCommentEditor: { if let store { review?.openCommentEditor(store) } },
            onEditComment: { review?.editComment($0) },
            onDeleteComment: { comment in if let store { review?.deleteComment(comment, store: store) } },
            onSaveDraft: { text in if let store { review?.saveDraft(text, store: store) } },
            onCancelDraft: { review?.draft = nil }
        )
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
