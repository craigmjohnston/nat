import AppKit
import SwiftUI
import NatKit

/// An ad hoc session's PR tab: the first pull request `nat session-status`
/// reports across every branch the session has been on, or an empty state
/// naming its branch when there is none yet.
///
/// Lighter than the slice PR tab's own `PRTabView`: a session's pull request
/// is read through `session-status` rather than `gh pr view`, which gives
/// only the fields gh's list API carries (number, title, state, URL, merge
/// date) — not the full detail (description, checks, reviewers, comments)
/// `PRTabView` draws. Merging and commenting on a session's pull request are
/// done on GitHub itself for now; picking among a session's several
/// branches and pull requests is the next slice's own.
struct SessionPRTabView: View {
    @Bindable var appModel: AppModel
    let session: Session

    @State private var status: SessionStatusDoc?
    @State private var loadFailed = false
    @State private var isLoading = false

    private var firstPR: SessionPR? {
        status?.firstPR
    }

    var body: some View {
        VStack(spacing: 0) {
            if let pr = firstPR {
                content(for: pr)
            } else if isLoading && status == nil {
                loadingState
            } else if loadFailed {
                failedState
            } else {
                emptyState
            }
        }
        .surface(.window)
        .task {
            await fetch()
        }
        .onChange(of: session.id) { _, _ in
            Task { await fetch() }
        }
    }

    private var loadingState: some View {
        VStack(spacing: 8) {
            ProgressView()
            Text("Reading the session's pull requests…")
                .font(.system(size: Typo.subhead, weight: .regular))
                .ink(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private var failedState: some View {
        VStack(spacing: 8) {
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 24, weight: .regular))
                .ink(.danger)

            Text("Failed to read the session's pull requests")
                .font(.system(size: Typo.body, weight: .regular))
                .ink(.primary)

            Button("Retry") {
                Task { await fetch() }
            }
            .buttonStyle(SecondaryButtonStyle())
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    /// No pull request read yet for this session's branch — naming it, so
    /// there is something on screen to say what a pull request would be
    /// opened against.
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

    private func content(for pr: SessionPR) -> some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack(spacing: 8) {
                Text("#\(pr.number)")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .monospacedDigit()
                    .ink(.tertiary)

                stateBadge(for: pr.state)

                Spacer()

                Button("Open on GitHub") {
                    if let url = URL(string: pr.url) {
                        NSWorkspace.shared.open(url)
                    }
                }
                .buttonStyle(SecondaryButtonStyle())
            }

            Text(pr.title)
                .font(.system(size: Typo.headline, weight: .semibold))
                .ink(.primary)

            if let mergedAt = pr.mergedAt {
                Text("Merged \(mergedAt.formatted(date: .abbreviated, time: .shortened))")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.secondary)
            }

            Spacer()
        }
        .padding(20)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }

    private func stateBadge(for state: String) -> some View {
        let (label, role): (String, InkRole) = {
            switch state {
            case "MERGED": return ("Merged", .accent)
            case "OPEN": return ("Open", .success)
            default: return ("Closed", .danger)
            }
        }()
        return Text(label)
            .font(.system(size: Typo.subhead, weight: .semibold))
            .ink(role)
            .padding(.horizontal, 8)
            .padding(.vertical, 2)
            .background(RoundedRectangle(cornerRadius: 5).fill(DesignTokens.wash(.selection, tone: .accent, on: .window)))
    }

    private func fetch() async {
        guard let projectID = appModel.projectStore?.projectID else { return }
        isLoading = true
        loadFailed = false
        do {
            status = try await appModel.sessionStatus(projectID: projectID, sessionID: session.id)
        } catch {
            loadFailed = true
        }
        isLoading = false
    }
}

#Preview("With a pull request") {
    let appModel = AppModel()
    let session = Session(
        id: "session-1", tag: "session:proj:session-1", live: false,
        startedAt: Date(), dir: "/Users/craig/Projects/scratch", branch: "session/fixture",
        prs: [SessionPR(number: 12, title: "Ad hoc fixture change", url: "https://github.com/x/y/pull/12", state: "OPEN")]
    )
    SessionPRTabView(appModel: appModel, session: session)
        .frame(width: 700, height: 400)
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
