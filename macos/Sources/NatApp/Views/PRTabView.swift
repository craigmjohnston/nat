import AppKit
import SwiftUI
import NatKit
import NatFixtures

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

    @State private var showMergeConfirm = false

    /// Merging is held by the app's `SliceActionTracker`, so the one-shot
    /// rule (disabled once the merge has gone through, until it fails or the
    /// merge genuinely becomes available again) outlives this view.
    private var isMerging: Bool { appModel.sliceActions.isRunning(.merge, sliceID: slice.id) }
    private var mergeError: String? { appModel.sliceActions.error(.merge, sliceID: slice.id) }

    @State private var commentText = ""
    @State private var isSendingComment = false
    @State private var commentError: String?

    @State private var replyText = ""
    @State private var isSendingReply = false
    @State private var replyError: String?

    /// The sidebar's width, draggable at its divider and remembered across
    /// launches — the default is the width it was fixed at before it was
    /// resizable, and the same key `PRSidebarView` used to hold it under
    /// before its own frame/rule/resize became this tab's to wrap it in.
    @AppStorage("prSidebarWidth") private var sidebarWidth = 216.0
    @State private var liveSidebarWidth: Double?

    var body: some View {
        VStack(spacing: 0) {
            // A reading already on screen wins over the state that replaced
            // it: the five-second poll keeps its pull request (and says so
            // with the sidebar's pinned busy mark), and a `gh` that failed
            // one of those readings keeps it too, with its words in a notice
            // pinned there beside it. Only a read with nothing ever behind it
            // draws the skeleton or the failure.
            if let pr = store.loadState.pr {
                content(for: pr)
            } else if case .failed = store.loadState {
                failedState
            } else {
                loadingState
            }
        }
        .surface(.window)
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

    /// The pull request's first read, drawn as the screen it is about to be
    /// — see `PRSkeletonView`. A poll or a refresh never reaches this: it
    /// keeps the reading it has.
    private var loadingState: some View {
        PRSkeletonView()
    }

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
                Task { await refreshAndPoll() }
            }
            .buttonStyle(SecondaryButtonStyle())
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    // MARK: - Loaded content

    private func content(for pr: PRDetail) -> some View {
        HStack(spacing: 0) {
            mainColumn(for: pr)
            prSidebar(for: pr)
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
                .inelastic()
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

    private func branchLine(for pr: PRDetail) -> some View {
        Text("\(pr.headRefName) → \(pr.baseRefName)")
            .font(Typo.mono(size: Typo.code, weight: .regular))
            .ink(.tertiary)
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
                .card(radius: 10)
            }
        }
    }

    // MARK: - Sidebar (checks/review/changes rail, and the merge box's actions)

    /// The PR tab's own rail: the Merge/Open-in-GitHub actions atop it — the
    /// standing inspector-top slot every pane with a rail opens with — then
    /// `PRSidebarView`'s checks/review/changes, and a pinned foot for the
    /// merge box's own busy mark, its readiness heading, and any error.
    private func prSidebar(for pr: PRDetail) -> some View {
        let rollup = mergeRollup(mergeVerdicts(pr))

        return VStack(spacing: 0) {
            InspectorActionsBar {
                Button(action: { showMergeConfirm = true }) {
                    AsyncActionLabel(isBusy: isMerging) {
                        Text("Merge")
                    }
                }
                .buttonStyle(InspectorPrimaryButtonStyle())
                .disabled(!appModel.sliceActions.isEnabled(.merge, sliceID: slice.id, available: mergeIsEnabled(for: pr)))
                .onChange(of: mergeIsEnabled(for: pr), initial: true) { _, available in
                    appModel.sliceActions.observe(.merge, sliceID: slice.id, available: available)
                }

                Button(action: openInGitHub) {
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
                // A read that failed over a pull request already on screen:
                // what is up is the last good reading, and saying so is what
                // stops it being read as GitHub's current answer.
                if let staleMessage = store.loadState.errorMessage {
                    InspectorNotice(text: "Showing the last reading — \(staleMessage)", role: .warning)
                }
                if let mergeError {
                    InspectorNotice(text: mergeError, role: .danger)
                }

                HStack(spacing: 8) {
                    // Holds its slot whether a read is running or not, so the
                    // poll never moves the heading beside it.
                    RefreshingMark(isRefreshing: store.isRefreshing)

                    Image(systemName: footerMarkSymbolName(for: pr, rollup: rollup))
                        .font(.system(size: 12, weight: .medium))
                        .foregroundStyle(footerTint(for: pr, rollup: rollup))

                    Text(footerHeadingText(for: pr))
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .ink(.tertiary)
                        .lineLimit(2)

                    Spacer()
                }
            }
        }
        .frame(width: liveSidebarWidth ?? sidebarWidth)
        .rule(.separator, edges: [.leading], width: 0.5)
        .overlay(alignment: .leading) {
            PaneResizeHandle(width: sidebarWidth, liveWidth: $liveSidebarWidth, onCommit: { sidebarWidth = $0 }, minWidth: 170, maxWidth: 400, edge: .leading)
                .offset(x: -4.5)
        }
        .confirmationDialog(
            "Merge pull request #\(pr.number)?",
            isPresented: $showMergeConfirm,
            titleVisibility: .visible
        ) {
            Button("Merge") { Task { await performMerge() } }
            Button("Cancel", role: .cancel) {}
        }
        // Without an icon of its own the dialog wears the app's, which for
        // an unbundled dev build is the generic document icon.
        .dialogIcon(Image(systemName: "arrow.triangle.merge"))
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
                // Green verdicts but a merge state GitHub's button would not
                // yet offer (BLOCKED, BEHIND, still computing): say why the
                // button is off rather than "ready".
                if let refusal = mergeRefusal(pr) {
                    return "Not ready to merge — \(refusal)"
                }
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
        let store = store
        let appModel = appModel
        await appModel.sliceActions.run(.merge, sliceID: slice.id, select: { _ in }) {
            try await store.merge()
            // The rail still lists this slice as awaiting review off the
            // PR-readiness reading, and a merge writes nothing to Notion, so
            // no nudge will refresh it — take the reading now rather than
            // leaving "awaiting review" standing until the next poll.
            await appModel.refresh()
        }
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

/// One entry of the conversation timeline, the mock's `PRComment` shape: an
/// avatar circle with the author's initials, their name, what they did in
/// saying it — coloured by its tone, since a review's verdict is the entry's
/// whole point and an avatar cannot carry it — when, and the markdown they
/// wrote, aligned under the name rather than the avatar.
struct PRConversationEntryView: View {
    let entry: ConvoEntry

    /// The byline's avatar and the gap after it. Internal rather than
    /// private, so `PRSkeletonView` stands its entries in the very column
    /// these put the text in.
    static let avatarSize: CGFloat = 22
    static let avatarGap: CGFloat = 10

    var body: some View {
        HStack(alignment: .top, spacing: Self.avatarGap) {
            Text(authorInitials(entry.author))
                .font(.system(size: 9, weight: .semibold))
                .ink(.accent)
                .frame(width: Self.avatarSize, height: Self.avatarSize)
                .wash(.avatar)
                .clipShape(Circle())
                .padding(.top, 2)

            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 8) {
                    Text(entry.author)
                        .font(.system(size: Typo.subhead, weight: .semibold))
                        .ink(.primary)

                    Text(entry.verb)
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .foregroundStyle(entry.tone.tint)

                    Text(ago(Date().timeIntervalSince(entry.at)))
                        .font(.system(size: Typo.caption, weight: .regular))
                        .ink(.tertiary)
                }

                if !entry.body.isEmpty {
                    Text(markdownAttributed(entry.body, size: Typo.subhead))
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .ink(.secondary)
                        .lineSpacing(2)
                }
            }
        }
    }
}

/// The composer's own metrics. Named rather than inline for the reason
/// `ButtonMetrics` is, and for one more: `PRSkeletonView` reserves the band
/// this opens at, and a skeleton that guessed it would hand the reading
/// column rows the composer then took back.
enum PRComposerMetrics {
    /// What the editor is at least, and at most, in each of its two sizes.
    static func editorMinHeight(compact: Bool) -> CGFloat { compact ? 22 : 36 }
    static func editorMaxHeight(compact: Bool) -> CGFloat { compact ? 70 : 120 }
    /// The inset over the editor, which is the only one that differs between
    /// them.
    static func editorTopPadding(compact: Bool) -> CGFloat { compact ? 5 : 7 }
    /// The send row under it: the button's own height and the insets around
    /// the row.
    static let sendRowHeight: CGFloat = 22
    static let sendRowTopPadding: CGFloat = 5
    static let sendRowBottomPadding: CGFloat = 7
    /// The field's corner.
    static let cornerRadius: CGFloat = 8

    /// What the box comes to at its floor: the editor at its own minimum,
    /// the send row under it, and the insets around both.
    static func height(compact: Bool) -> CGFloat {
        chrome + editorMinHeight(compact: compact)
    }

    /// And at its ceiling, which is what the one at the tab's foot actually
    /// opens at: the editor is flexible, so a `VStack` with room to spare
    /// gives it every point up to its maximum.
    static func maxHeight(compact: Bool) -> CGFloat {
        chrome + editorMaxHeight(compact: compact)
    }

    /// Everything the box is but its editor.
    private static var chrome: CGFloat {
        editorTopPadding(compact: false) + sendRowTopPadding + sendRowHeight + sendRowBottomPadding
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
                        // Laid where the editor's own first line starts —
                        // its 2pt padding plus the text view's 5pt line
                        // fragment padding, and no top offset — so the caret
                        // blinks exactly at the placeholder's first letter.
                        Text(placeholder)
                            // The editor's own font, since the placeholder
                            // stands exactly where the first typed letter
                            // will: a proportional one would sit a hair off
                            // the caret it is drawn behind.
                            .font(Typo.mono(size: Typo.subhead))
                            .ink(.tertiary)
                            .padding(.leading, 7)
                            .allowsHitTesting(false)
                    }
                    TextEditor(text: $text)
                        .font(Typo.mono(size: Typo.subhead))
                        .scrollContentBackground(.hidden)
                        .frame(
                            minHeight: PRComposerMetrics.editorMinHeight(compact: compact),
                            maxHeight: PRComposerMetrics.editorMaxHeight(compact: compact)
                        )
                        .padding(.horizontal, 2)
                }
                .padding(.horizontal, 9)
                .padding(.top, PRComposerMetrics.editorTopPadding(compact: compact))

                // The mock's toolbar row also draws textformat and paperclip
                // icons here; neither has anything real to do — gh has no API
                // for comment attachments — and a control that does nothing is
                // worse than the mock losing two glyphs, so only the send
                // button is drawn.
                HStack(spacing: 8) {
                    Spacer()

                    Button(action: onSend) {
                        Group {
                            if isSending {
                                // Sized down to the slot rather than laid out
                                // at the control's own size, which
                                // `scaleEffect` draws smaller without ever
                                // shrinking: an unframed spinner here made the
                                // send button — and the composer under it —
                                // grow while a comment was posting.
                                ProgressView()
                                    .controlSize(.small)
                                    .scaleEffect(0.55)
                                    .frame(width: 10, height: 10)
                            } else {
                                Image(systemName: "paperplane.fill")
                                    .font(.system(size: 12, weight: .medium))
                                    .ink(.onAccent)
                            }
                        }
                        .frame(width: 24, height: PRComposerMetrics.sendRowHeight)
                        .background(canSend ? DesignTokens.accent : DesignTokens.accentMuted(on: .field))
                        .clipShape(RoundedRectangle(cornerRadius: 6))
                    }
                    .buttonStyle(.plain)
                    .disabled(!canSend)
                    .help("Send")
                }
                .padding(.horizontal, 8)
                .padding(.top, PRComposerMetrics.sendRowTopPadding)
                .padding(.bottom, PRComposerMetrics.sendRowBottomPadding)
            }
            .field(radius: PRComposerMetrics.cornerRadius)

            if let error {
                Text(error)
                    .font(.system(size: Typo.caption, weight: .regular))
                    .ink(.danger)
                    .lineLimit(2)
            }
        }
    }
}

#Preview("Ready to merge") {
    @Previewable @State var appModel = Fixtures.appModel()
    let slice = Fixtures.slices.first { $0.id == Fixtures.approveSliceID }!

    PRTabView(appModel: appModel, slice: slice)
        .frame(width: 900, height: 560)
        .task { await Fixtures.start(appModel) }
}

#Preview("Failing checks") {
    @Previewable @State var appModel = Fixtures.appModel(
        client: FixtureNatClient(pr: Fixtures.prFailingChecks)
    )
    let slice = Fixtures.slices.first { $0.id == Fixtures.approveSliceID }!

    PRTabView(appModel: appModel, slice: slice)
        .frame(width: 900, height: 560)
        .task { await Fixtures.start(appModel) }
}

#Preview("Conflicting") {
    @Previewable @State var appModel = Fixtures.appModel(
        client: FixtureNatClient(pr: Fixtures.prConflicting)
    )
    let slice = Fixtures.slices.first { $0.id == Fixtures.approveSliceID }!

    PRTabView(appModel: appModel, slice: slice)
        .frame(width: 900, height: 560)
        .task { await Fixtures.start(appModel) }
}

#Preview("Merged") {
    @Previewable @State var appModel = Fixtures.appModel(
        client: FixtureNatClient(pr: Fixtures.prMerged)
    )
    let slice = Fixtures.slices.first { $0.id == Fixtures.approveSliceID }!

    PRTabView(appModel: appModel, slice: slice)
        .frame(width: 900, height: 560)
        .task { await Fixtures.start(appModel) }
}
