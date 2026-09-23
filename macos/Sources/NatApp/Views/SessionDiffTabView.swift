import SwiftUI
import NatKit
import NatFixtures

/// An ad hoc session's Diff tab: `nat session-diff`'s reading of one of its
/// branches — the checked-out one until another is picked from the
/// `ChipPickerView` above it, which is hidden while the session has only the
/// one — drawn with the same file-box-and-sidebar machinery the slice Diff
/// tab uses — but with no approve action and no comment composer, since a
/// session has no hand-back to approve and no review this diff is itself the
/// subject of.
struct SessionDiffTabView: View {
    @Bindable var appModel: AppModel
    let session: Session

    private var store: SessionDiffStore {
        appModel.sessionDiffStore(projectID: appModel.projectStore?.projectID ?? "")
    }

    @State private var fileScroll = ScrollPosition(idType: String.self)

    /// Every branch the session has been on, as `session-status` last read
    /// them, and the session that reading was of — the checked-out one first.
    @State private var branches: [String] = []
    @State private var branchesSessionID: String?

    /// The branch checked out in the session's worktree now: the default
    /// choice, and the one `session-status` lists first.
    private var checkedOut: String? { branches.first }

    private var selectedBranch: String? {
        appModel.selectedPickerID(.branch, sessionID: session.id, among: branches, defaultID: checkedOut)
    }

    /// What `session-diff --branch` is asked for: nothing for the default,
    /// which `session-diff` resolves itself to the worktree's own branch.
    private var requestedBranch: String? {
        selectedBranch == checkedOut ? nil : selectedBranch
    }

    private var picker: ChipPickerModel {
        ChipPickerModel(chips: branchChips(branches, checkedOut: checkedOut), selectedID: selectedBranch)
    }

    var body: some View {
        VStack(spacing: 0) {
            ChipPickerView(model: picker) { branch in
                appModel.selectPicker(.branch, sessionID: session.id, id: branch)
            }

            if let diff = store.loadState.diff {
                content(for: diff)
            } else if case .failed = store.loadState {
                failedState
            } else {
                loadingState
            }
        }
        .surface(.window)
        .task(id: "\(session.id)|\(requestedBranch ?? "")") {
            await fetch()
        }
    }

    private var loadingState: some View {
        DiffSkeletonView()
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
                Task { await refresh() }
            }
            .buttonStyle(SecondaryButtonStyle())
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

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
                                    comments: [],
                                    selection: nil,
                                    draft: nil,
                                    commentsEnabled: false,
                                    authorName: "",
                                    authorInitials: "",
                                    onToggleViewed: { store.toggleViewed(file.path) },
                                    onToggleCollapsed: { store.toggleCollapsed(file.path) },
                                    onRowClick: { _, _ in },
                                    onOpenCommentEditor: {},
                                    onEditComment: { _ in },
                                    onDeleteComment: { _ in },
                                    onSaveDraft: { _ in },
                                    onCancelDraft: {}
                                )
                            }
                        }
                        .scrollTargetLayout()
                        .padding(14)
                        .inelastic()
                    }
                    .scrollPosition($fileScroll, anchor: .top)

                    diffSidebar(for: diff)
                }
            }
        }
    }

    private func diffSidebar(for diff: DiffModel) -> some View {
        let viewedCount = diff.files.filter { store.isViewed($0.path) }.count

        return VStack(spacing: 0) {
            DiffFileSidebarView(
                files: diff.files,
                isViewed: { store.isViewed($0) },
                onSelect: { path in
                    withAnimation(Motion.stateChange) {
                        fileScroll.scrollTo(id: path, anchor: .top)
                    }
                }
            )

            InspectorStatusFoot {
                if let staleMessage = store.loadState.errorMessage {
                    InspectorNotice(text: "Showing the last reading — \(staleMessage)", role: .warning)
                }

                HStack(spacing: 8) {
                    RefreshingMark(isRefreshing: store.isRefreshing)

                    Text("\(viewedCount) of \(diff.files.count) viewed")
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .monospacedDigit()
                        .ink(.tertiary)

                    Spacer()
                }
            }
        }
        .frame(width: 232)
        .rule(.separator, edges: [.leading], width: 0.5)
    }

    /// Read the session's branches once per session — a read that fails
    /// leaves none, and so no picker and the default branch's diff — then the
    /// diff of the branch selected. The selection is read after the branches
    /// land, since a remembered one only means something among them.
    private func fetch() async {
        guard let projectID = appModel.projectStore?.projectID else { return }
        if branchesSessionID != session.id {
            await loadBranches(projectID: projectID)
        }
        await store.fetch(projectID: projectID, sessionID: session.id, branch: requestedBranch)
    }

    private func loadBranches(projectID: String) async {
        let sessionID = session.id
        let doc = try? await appModel.sessionStatus(projectID: projectID, sessionID: sessionID)
        branches = doc?.branches.map(\.branch) ?? []
        branchesSessionID = sessionID
    }

    private func refresh() async {
        guard let projectID = appModel.projectStore?.projectID else { return }
        await loadBranches(projectID: projectID)
        await store.refresh(projectID: projectID)
    }
}

#Preview {
    let appModel = Fixtures.appModel()
    let session = Fixtures.liveSession
    SessionDiffTabView(appModel: appModel, session: session)
        .frame(width: 900, height: 500)
}
