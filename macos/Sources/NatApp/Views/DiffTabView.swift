import SwiftUI
import NatKit

/// The Diff tab: the unified diff of a handed-back slice's branch, read
/// through `DiffStore` and drawn as one GitHub-style box per file beside a
/// file-list sidebar. Read-only — nothing here writes anything; inline
/// comments, sending them to the agent, and the approve footer are a later
/// task, so the row model already carries the stable line identities they
/// will anchor to (`DiffRow.id`), but nothing reads them yet.
struct DiffTabView: View {
    @Bindable var appModel: AppModel
    let slice: Slice
    @State private var store = DiffStore()

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
                Task { await store.refresh() }
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
                                        onToggleViewed: { store.toggleViewed(file.path) },
                                        onToggleCollapsed: { store.toggleCollapsed(file.path) }
                                    )
                                    .id(file.path)
                                }
                            }
                            .padding(14)
                        }

                        DiffFileSidebarView(
                            files: diff.files,
                            isViewed: { store.isViewed($0) },
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
        return VStack(spacing: 0) {
            Divider()
                .frame(height: 0.5)

            HStack(spacing: 8) {
                Text("\(viewedCount) of \(diff.files.count) viewed")
                    .font(.system(size: 12, weight: .regular))
                    .foregroundStyle(DesignTokens.labelTertiary)

                Spacer()

                Button(action: {}) {
                    Text("Approve & Open PR…")
                        .font(.system(size: 12, weight: .semibold))
                }
                .buttonStyle(.borderedProminent)
                .tint(DesignTokens.accent)
                .disabled(true)
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
        }
    }

    // MARK: - Fetching

    private func fetch() async {
        guard let projectID = appModel.projectStore?.projectID else { return }
        await store.fetch(projectID: projectID, sliceRef: slice.id)
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
