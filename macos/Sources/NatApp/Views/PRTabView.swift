import AppKit
import SwiftUI
import NatKit

/// The PR tab: what GitHub says about a slice's pull request, read through
/// `PRStore` and drawn beside a checks/review/changes sidebar — the desktop
/// counterpart of the Go TUI's own PR screen (`internal/tui/prview.go`).
///
/// Two actions reach outside Notion here: the merge button, mirroring the
/// board's `m` key (`internal/tui/prmergeflow.go`), and the two comment
/// composers, which post a new top-level comment on the pull request through
/// `nat pr-comment` — GitHub's per-line review threads have a reply API of
/// their own that `nat` does not wrap, so neither composer is a threaded
/// reply whatever its placeholder says.
struct PRTabView: View {
    @Bindable var appModel: AppModel
    let slice: Slice

    /// The project's shared pull-request cache, rather than a `PRStore`
    /// local to this view — see `DiffTabView.store` for why.
    private var store: PRStore {
        appModel.prStore(projectID: appModel.projectStore?.projectID ?? "")
    }

    @State private var isMerging = false
    @State private var mergeError: String?
    @State private var showMergeConfirm = false

    @State private var commentText = ""
    @State private var isSendingComment = false
    @State private var commentError: String?

    @State private var replyText = ""
    @State private var isSendingReply = false
    @State private var replyError: String?

    var body: some View {
        VStack(spacing: 0) {
            switch store.loadState {
            case .idle, .loading:
                loadingState
            case .loaded(let pr):
                content(for: pr)
            case .failed:
                failedState
            }
        }
        .background(DesignTokens.windowBg)
        .task {
            await fetchAndPoll()
        }
        .onChange(of: slice.id) { _, _ in
            Task { await fetchAndPoll() }
        }
        .onDisappear {
            store.stopPolling()
        }
    }

    // MARK: - States

    private var loadingState: some View {
        QuietLoadingView(label: "Reading the pull request of \(slice.name)…")
    }

    private var failedState: some View {
        VStack(spacing: 8) {
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 24, weight: .regular))
                .foregroundStyle(DesignTokens.systemRed)

            Text("Failed to read the pull request")
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
                Task { await refreshAndPoll() }
            }
            .buttonStyle(.bordered)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    // MARK: - Loaded content

    private func content(for pr: PRDetail) -> some View {
        VStack(spacing: 0) {
            HStack(spacing: 0) {
                mainColumn(for: pr)
                PRSidebarView(pr: pr)
            }
            footer(for: pr)
        }
    }

    private func mainColumn(for pr: PRDetail) -> some View {
        VStack(spacing: 0) {
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    header(for: pr)
                    branchLine(for: pr)
                    descriptionSection(for: pr)
                    conversationSection(for: pr)
                }
                .padding(.horizontal, 22)
                .padding(.vertical, 18)
                .frame(maxWidth: .infinity, alignment: .leading)
            }

            Divider().frame(height: 0.5)

            // The composer pinned at the tab's own foot — GitHub's own
            // bottom-of-thread box. It posts through `pr-comment` exactly as
            // the compact one inside the conversation does; see the note
            // there for why neither is a threaded reply.
            PRComposerView(
                placeholder: "Leave a comment on the pull request…",
                compact: false,
                text: $commentText,
                isSending: isSendingComment,
                error: commentError,
                onSend: { Task { await sendComment() } }
            )
            .padding(.horizontal, 22)
            .padding(.vertical, 12)
        }
        .frame(maxWidth: .infinity)
    }

    private func header(for pr: PRDetail) -> some View {
        let chip = prStateChip(state: pr.state, isDraft: pr.isDraft)
        return HStack(spacing: 10) {
            Text(chip.label)
                .font(.system(size: Typo.subhead, weight: .semibold))
                .foregroundStyle(chip.tint)
                .padding(.horizontal, 10)
                .frame(height: 22)
                .background(chip.tint.opacity(0.18))
                .clipShape(Capsule())

            Text(pr.title)
                .font(.system(size: Typo.headline, weight: .semibold))
                .foregroundStyle(DesignTokens.label)
                .lineLimit(1)
                .truncationMode(.tail)

            Text("#\(pr.number)")
                .font(.system(size: Typo.body, weight: .regular))
                .monospacedDigit()
                .foregroundStyle(DesignTokens.labelTertiary)

            Spacer(minLength: 0)
        }
    }

    private func branchLine(for pr: PRDetail) -> some View {
        Text("\(pr.headRefName) → \(pr.baseRefName)")
            .font(.system(size: Typo.code, weight: .regular, design: .monospaced))
            .foregroundStyle(DesignTokens.labelTertiary)
    }

    private func descriptionSection(for pr: PRDetail) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("DESCRIPTION")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .foregroundStyle(DesignTokens.labelTertiary)

            let described = pr.body.trimmingCharacters(in: .whitespacesAndNewlines)
            if described.isEmpty {
                Text("This pull request has no description.")
                    .font(.system(size: Typo.body, weight: .regular))
                    .foregroundStyle(DesignTokens.labelSecondary)
            } else {
                Text(markdownAttributed(described))
                    .font(.system(size: Typo.body, weight: .regular))
                    .lineSpacing(2)
                    .foregroundStyle(DesignTokens.label)
                    .textSelection(.enabled)
            }
        }
    }

    private func conversationSection(for pr: PRDetail) -> some View {
        let entries = conversation(comments: pr.comments, reviews: pr.reviews)
        return VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 8) {
                Text("CONVERSATION")
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .foregroundStyle(DesignTokens.labelTertiary)
                if !entries.isEmpty {
                    Text(convoSummary(entries))
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .monospacedDigit()
                        .foregroundStyle(DesignTokens.labelTertiary)
                }
            }

            if entries.isEmpty {
                Text("Nothing has been said on this pull request.")
                    .font(.system(size: Typo.body, weight: .regular))
                    .foregroundStyle(DesignTokens.labelSecondary)
            } else {
                VStack(alignment: .leading, spacing: 12) {
                    ForEach(Array(entries.enumerated()), id: \.offset) { _, entry in
                        PRConversationEntryView(entry: entry)
                    }

                    // GitHub's per-line review threads have a reply API of
                    // their own, which `nat` does not wrap — this composer
                    // posts a new top-level comment on the pull request, the
                    // same as the one pinned at the tab's own foot, rather
                    // than a reply threaded onto whatever is above it.
                    PRComposerView(
                        placeholder: "Reply to this thread…",
                        compact: true,
                        text: $replyText,
                        isSending: isSendingReply,
                        error: replyError,
                        onSend: { Task { await sendReply() } }
                    )
                }
                .padding(12)
                .background(DesignTokens.controlBg)
                .clipShape(RoundedRectangle(cornerRadius: 10))
                .overlay(
                    RoundedRectangle(cornerRadius: 10)
                        .stroke(DesignTokens.controlBorder, lineWidth: 0.5)
                )
            }
        }
    }

    // MARK: - Footer (merge box)

    private func footer(for pr: PRDetail) -> some View {
        let rollup = mergeRollup(mergeVerdicts(pr))
        return VStack(spacing: 0) {
            if let mergeError {
                HStack {
                    Text(mergeError)
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .foregroundStyle(DesignTokens.systemRed)
                        .lineLimit(2)
                    Spacer()
                }
                .padding(.horizontal, 14)
                .padding(.top, 6)
            }

            Divider().frame(height: 0.5)

            HStack(spacing: 8) {
                Image(systemName: footerMarkSymbolName(for: pr, rollup: rollup))
                    .font(.system(size: 12, weight: .medium))
                    .foregroundStyle(footerTint(for: pr, rollup: rollup))

                Text(footerHeadingText(for: pr))
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(DesignTokens.labelTertiary)

                Spacer()

                Button(action: openInGitHub) {
                    HStack(spacing: 5) {
                        Text("Open in GitHub")
                        Image(systemName: "arrow.up.right.square")
                    }
                    .font(.system(size: Typo.subhead, weight: .regular))
                }
                .buttonStyle(.borderless)

                Button(action: { showMergeConfirm = true }) {
                    if isMerging {
                        ProgressView().scaleEffect(0.6)
                    } else {
                        Text("Merge")
                    }
                }
                .buttonStyle(.borderedProminent)
                .tint(DesignTokens.accent)
                .disabled(!mergeIsEnabled(for: pr) || isMerging)
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
        }
        .confirmationDialog(
            "Merge pull request #\(pr.number)?",
            isPresented: $showMergeConfirm,
            titleVisibility: .visible
        ) {
            Button("Merge") { Task { await performMerge() } }
            Button("Cancel", role: .cancel) {}
        }
    }

    private func mergeIsEnabled(for pr: PRDetail) -> Bool {
        guard pr.state != PRLifecycleState.merged, pr.state != PRLifecycleState.closed else { return false }
        guard !pr.isDraft else { return false }
        return mergeRefusal(pr) == nil
    }

    private func footerHeadingText(for pr: PRDetail) -> String {
        switch mergeBoxState(for: pr) {
        case .ended(let words, _):
            return sentenceCase(words)
        case .verdicts(let heading, let verdicts):
            switch heading.words {
            case "cannot merge":
                if let refusal = mergeRefusal(pr) {
                    return "Cannot merge — \(refusal)"
                }
                return "Cannot merge"
            case "not ready to merge":
                if let pendingVerdict = verdicts.first(where: { $0.outcome == .pending }) {
                    if pendingVerdict.label == "checks",
                        let firstPendingCheck = pr.checks.first(where: { checkOutcome(state: $0.state) == .pending }) {
                        return "Waiting on \(firstPendingCheck.name) — merges when green"
                    }
                    return "Waiting on \(pendingVerdict.label) — merges when green"
                }
                return "Not ready to merge"
            default:
                return "Ready to merge"
            }
        }
    }

    private func footerTint(for pr: PRDetail, rollup: CheckOutcome) -> Color {
        switch mergeBoxState(for: pr) {
        case .ended(_, let tint): return tint
        case .verdicts: return rollup.tint
        }
    }

    private func footerMarkSymbolName(for pr: PRDetail, rollup: CheckOutcome) -> String {
        switch mergeBoxState(for: pr) {
        case .ended: return "checkmark.circle.fill"
        case .verdicts: return rollup.markSymbolName
        }
    }

    private func openInGitHub() {
        guard let url = URL(string: store.loadState.pr?.url ?? slice.pr) else { return }
        NSWorkspace.shared.open(url)
    }

    private func performMerge() async {
        isMerging = true
        mergeError = nil
        do {
            try await store.merge()
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                mergeError = message
            } else {
                mergeError = error.localizedDescription
            }
        } catch {
            mergeError = error.localizedDescription
        }
        isMerging = false
        // The pull request may have just settled (merged) or may now have
        // nothing left pending — either way this is a no-op if polling
        // should not continue.
        store.startPolling()
    }

    // MARK: - Comments

    private func sendComment() async {
        isSendingComment = true
        commentError = nil
        do {
            try await store.comment(text: commentText)
            commentText = ""
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                commentError = message
            } else {
                commentError = error.localizedDescription
            }
        } catch {
            commentError = error.localizedDescription
        }
        isSendingComment = false
    }

    private func sendReply() async {
        isSendingReply = true
        replyError = nil
        do {
            try await store.comment(text: replyText)
            replyText = ""
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                replyError = message
            } else {
                replyError = error.localizedDescription
            }
        } catch {
            replyError = error.localizedDescription
        }
        isSendingReply = false
    }

    // MARK: - Fetching / polling

    private func fetchAndPoll() async {
        guard let projectID = appModel.projectStore?.projectID else { return }
        await store.fetch(projectID: projectID, sliceRef: slice.id)
        store.startPolling()
    }

    private func refreshAndPoll() async {
        await store.refresh()
        store.startPolling()
    }
}

/// Renders markdown the same way the Brief tab does: full syntax, with a
/// plain-text fallback should parsing fail rather than an empty view.
func markdownAttributed(_ text: String) -> AttributedString {
    (try? AttributedString(markdown: text, options: .init(interpretedSyntax: .full))) ?? AttributedString(text)
}

/// One entry of the conversation timeline: a mark and colour for its tone, who
/// said it and when, and — for everything but a bare verdict — the markdown
/// they wrote, nested slightly under the entry's own line.
struct PRConversationEntryView: View {
    let entry: ConvoEntry

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 8) {
                Image(systemName: entry.tone.markSymbolName)
                    .font(.system(size: 12, weight: .medium))
                    .foregroundStyle(entry.tone.tint)

                Text(entry.author)
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .foregroundStyle(DesignTokens.label)

                Text(entry.verb)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(entry.tone.tint)

                Text(ago(Date().timeIntervalSince(entry.at)))
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(DesignTokens.labelTertiary)
            }

            if !entry.body.isEmpty {
                Text(markdownAttributed(entry.body))
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(DesignTokens.labelSecondary)
                    .lineSpacing(1)
                    .textSelection(.enabled)
                    .padding(.leading, 19)
            }
        }
    }
}

/// The pull request's own comment box, matching the mock's `PRComposer`
/// chrome (a rounded field with a toolbar row under it, the send button in
/// accent) but functional: it posts through `pr-comment` rather than sitting
/// there as decoration. Used twice — full-size at the tab's own foot, compact
/// at the conversation's — with the same mechanics either way; see the call
/// sites for why neither is a threaded reply.
struct PRComposerView: View {
    let placeholder: String
    var compact: Bool = false
    @Binding var text: String
    let isSending: Bool
    let error: String?
    let onSend: () -> Void

    private var canSend: Bool {
        !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !isSending
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            VStack(alignment: .leading, spacing: 0) {
                ZStack(alignment: .topLeading) {
                    if text.isEmpty {
                        Text(placeholder)
                            .font(.system(size: Typo.subhead, weight: .regular))
                            .foregroundStyle(DesignTokens.labelTertiary)
                            .padding(.top, 8)
                            .padding(.leading, 5)
                            .allowsHitTesting(false)
                    }
                    TextEditor(text: $text)
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .scrollContentBackground(.hidden)
                        .frame(minHeight: compact ? 22 : 36, maxHeight: compact ? 70 : 120)
                        .padding(.horizontal, 2)
                }
                .padding(.horizontal, 9)
                .padding(.top, compact ? 5 : 7)

                HStack(spacing: 8) {
                    Image(systemName: "textformat")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundStyle(DesignTokens.labelTertiary)
                    Image(systemName: "paperclip")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundStyle(DesignTokens.labelTertiary)

                    Spacer()

                    Button(action: onSend) {
                        Group {
                            if isSending {
                                ProgressView().scaleEffect(0.5)
                            } else {
                                Image(systemName: "paperplane.fill")
                                    .font(.system(size: 12, weight: .medium))
                                    .foregroundStyle(DesignTokens.accentText)
                            }
                        }
                        .frame(width: 24, height: 22)
                        .background(canSend ? DesignTokens.accent : DesignTokens.accent.opacity(0.4))
                        .clipShape(RoundedRectangle(cornerRadius: 6))
                    }
                    .buttonStyle(.plain)
                    .disabled(!canSend)
                    .help("Send")
                }
                .padding(.horizontal, 8)
                .padding(.top, 5)
                .padding(.bottom, 7)
            }
            .background(DesignTokens.fieldBg)
            .clipShape(RoundedRectangle(cornerRadius: 8))
            .overlay(
                RoundedRectangle(cornerRadius: 8)
                    .stroke(DesignTokens.controlBorder, lineWidth: 0.5)
            )

            if let error {
                Text(error)
                    .font(.system(size: Typo.caption, weight: .regular))
                    .foregroundStyle(DesignTokens.systemRed)
                    .lineLimit(2)
            }
        }
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
        pr: "https://github.com/example/repo/pull/1",
        url: "https://example.com",
        branch: "feature/test",
        repo: "/path/to/repo",
        dependsOn: nil,
        blocked: false,
        handedBack: false
    )

    PRTabView(appModel: appModel, slice: slice)
        .frame(width: 900, height: 560)
}
