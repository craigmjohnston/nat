import AppKit
import SwiftUI
import NatKit

/// An ad hoc session's PR tab: one of the pull requests its branches have
/// opened, read in full through `PRStore` (`nat pr-view --session`) and drawn
/// beside the checks/review/changes sidebar the slice PR tab draws.
///
/// A session can open several — `session-list` reports them across every
/// branch it has been on — and with more than one, a `ChipPickerView` above
/// the content chooses which is shown, remembered per session for the app
/// session. With one, no picker is drawn. There is no merge button and no
/// composer here: a session's pull requests are merged and discussed on
/// GitHub itself, and "Open in GitHub" goes there.
struct SessionPRTabView: View {
    @Bindable var appModel: AppModel
    let session: Session

    private var store: PRStore {
        appModel.prStore(projectID: appModel.projectStore?.projectID ?? "")
    }

    @AppStorage("prSidebarWidth") private var sidebarWidth = 216.0

    private var selectedURL: String? {
        appModel.selectedPickerID(.pullRequest, sessionID: session.id, among: session.prs.map(\.url))
    }

    private var selectedPR: SessionPR? {
        session.prs.first { $0.url == selectedURL }
    }

    private var picker: ChipPickerModel {
        ChipPickerModel(chips: pullRequestChips(session.prs), selectedID: selectedURL)
    }

    var body: some View {
        VStack(spacing: 0) {
            if let selected = selectedPR {
                ChipPickerView(model: picker) { url in
                    appModel.selectPicker(.pullRequest, sessionID: session.id, id: url)
                }

                // A reading already on screen wins over the state that
                // replaced it, exactly as on the slice PR tab — but only one
                // of *this* pull request: the store is shared with the slice
                // tab, and the moment before its fetch lands may still hold
                // whatever it read last.
                if let pr = store.loadState.pr, pr.number == selected.number {
                    content(for: pr, url: selected.url)
                } else if case .failed = store.loadState {
                    failedState
                } else {
                    PRSkeletonView()
                }
            } else {
                emptyState
            }
        }
        .surface(.window)
        .task(id: selectedURL) {
            await fetchAndPoll()
        }
        .onDisappear {
            store.stopPolling()
        }
    }

    // MARK: - States

    private var failedState: some View {
        VStack(spacing: 8) {
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 24, weight: .regular))
                .ink(.danger)

            Text("Failed to read the pull request")
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
                Task {
                    await store.refresh()
                    store.startPolling()
                }
            }
            .buttonStyle(SecondaryButtonStyle())
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    /// No pull request yet on any branch — naming the session's branch, so
    /// there is something on screen to say what one would be opened from.
    private var emptyState: some View {
        VStack(spacing: 8) {
            Image(systemName: "arrow.branch")
                .font(.system(size: 32, weight: .regular))
                .ink(.secondary)

            Text("No pull request yet")
                .font(.system(size: Typo.body, weight: .regular))
                .ink(.primary)

            Text(session.branch.isEmpty ? session.label : session.branch)
                .font(Typo.mono(size: Typo.subhead))
                .ink(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    // MARK: - Loaded content

    private func content(for pr: PRDetail, url: String) -> some View {
        HStack(spacing: 0) {
            mainColumn(for: pr)
            sidebar(for: pr, url: url)
        }
    }

    private func mainColumn(for pr: PRDetail) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                header(for: pr)

                Text("\(pr.headRefName) → \(pr.baseRefName)")
                    .font(Typo.mono(size: Typo.code, weight: .regular))
                    .ink(.tertiary)

                descriptionSection(for: pr)
                conversationSection(for: pr)
            }
            .padding(.horizontal, 22)
            .padding(.vertical, 18)
            .frame(maxWidth: .infinity, alignment: .leading)
            .inelastic()
        }
        .frame(maxWidth: .infinity)
    }

    private func header(for pr: PRDetail) -> some View {
        let chip = prStateChip(state: pr.state, isDraft: pr.isDraft, on: .window)
        return HStack(spacing: 10) {
            HStack(spacing: 5) {
                Image(systemName: "arrow.branch")
                    .font(.system(size: 11, weight: .semibold))
                Text(sentenceCase(chip.label))
                    .font(.system(size: Typo.subhead, weight: .semibold))
            }
            .foregroundStyle(chip.tint)
            .padding(.horizontal, 10)
            .frame(height: 22)
            .background(chip.wash)
            .clipShape(Capsule())

            Text(pr.title)
                .font(.system(size: Typo.headline, weight: .semibold))
                .ink(.primary)
                .lineLimit(1)
                .truncationMode(.tail)

            Text("#\(pr.number)")
                .font(.system(size: Typo.subhead, weight: .regular))
                .monospacedDigit()
                .ink(.tertiary)

            Spacer(minLength: 0)
        }
    }

    private func descriptionSection(for pr: PRDetail) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("DESCRIPTION")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .ink(.tertiary)

            let described = pr.body.trimmingCharacters(in: .whitespacesAndNewlines)
            if described.isEmpty {
                Text("This pull request has no description.")
                    .font(.system(size: Typo.body, weight: .regular))
                    .ink(.secondary)
            } else {
                Text(markdownAttributed(described, size: Typo.body))
                    .font(.system(size: Typo.body, weight: .regular))
                    .lineSpacing(2)
                    .ink(.secondary)
            }
        }
    }

    private func conversationSection(for pr: PRDetail) -> some View {
        let entries = conversation(comments: pr.comments, reviews: pr.reviews)
        return VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 8) {
                Text("CONVERSATION")
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .ink(.tertiary)
                if !entries.isEmpty {
                    Text(convoSummary(entries))
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .monospacedDigit()
                        .ink(.tertiary)
                }
            }

            if entries.isEmpty {
                Text("Nothing has been said on this pull request.")
                    .font(.system(size: Typo.body, weight: .regular))
                    .ink(.secondary)
            } else {
                VStack(alignment: .leading, spacing: 12) {
                    ForEach(Array(entries.enumerated()), id: \.offset) { _, entry in
                        PRConversationEntryView(entry: entry)
                    }
                }
                .padding(12)
                .card(radius: 10)
            }
        }
    }

    private func sidebar(for pr: PRDetail, url: String) -> some View {
        VStack(spacing: 0) {
            InspectorActionsBar {
                Button(action: { open(url) }) {
                    HStack(spacing: 5) {
                        Text("Open in GitHub")
                        Image(systemName: "arrow.up.right.square")
                    }
                    .font(.system(size: Typo.subhead, weight: .regular))
                }
                .buttonStyle(InspectorSecondaryButtonStyle())
            }

            PRSidebarView(pr: pr)

            InspectorStatusFoot {
                if let staleMessage = store.loadState.errorMessage {
                    InspectorNotice(text: "Showing the last reading — \(staleMessage)", role: .warning)
                }

                HStack(spacing: 8) {
                    RefreshingMark(isRefreshing: store.isRefreshing)
                    Spacer()
                }
            }
        }
        .frame(width: sidebarWidth)
        .rule(.separator, edges: [.leading], width: 0.5)
    }

    // MARK: - Fetching

    private func open(_ url: String) {
        guard let url = URL(string: url) else { return }
        NSWorkspace.shared.open(url)
    }

    private func fetchAndPoll() async {
        guard let projectID = appModel.projectStore?.projectID, let selected = selectedPR else { return }
        await store.fetch(projectID: projectID, sliceRef: selected.url, sessionID: session.id)
        store.startPolling()
    }
}

#Preview("With two pull requests") {
    let appModel = AppModel()
    let session = Session(
        id: "session-1", tag: "session:proj:session-1", live: false,
        startedAt: Date(), dir: "/Users/craig/Projects/scratch", branch: "session/fixture",
        prs: [
            SessionPR(number: 12, title: "Ad hoc fixture change", url: "https://github.com/x/y/pull/12", state: "OPEN"),
            SessionPR(number: 11, title: "The earlier one", url: "https://github.com/x/y/pull/11", state: "MERGED"),
        ]
    )
    SessionPRTabView(appModel: appModel, session: session)
        .frame(width: 900, height: 500)
}

#Preview("No pull request") {
    let appModel = AppModel()
    let session = Session(
        id: "session-2", tag: "session:proj:session-2", live: false,
        startedAt: Date(), dir: "/Users/craig/Projects/scratch", branch: "session/no-pr"
    )
    SessionPRTabView(appModel: appModel, session: session)
        .frame(width: 700, height: 400)
}
