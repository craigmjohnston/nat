import AppKit
import SwiftUI
import NatKit
import NatFixtures

/// The PR tab: what GitHub says about a slice's pull request, read through
/// `PRStore` and drawn beside a checks/review/changes sidebar — the desktop
/// counterpart of the Go TUI's own PR screen (`internal/tui/prview.go`).
///
/// Two actions reach outside Notion here: the merge button, mirroring the
/// board's `m` key (`internal/tui/prmergeflow.go`), and the comment composer
/// at the conversation's foot, which posts a new top-level comment on the
/// pull request through `nat pr-comment` — GitHub's per-line review threads
/// have a reply API of their own that `nat` does not wrap, so it is never a
/// threaded reply to whatever sits above it.
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

    /// The sidebar's width, draggable at its divider and remembered across
    /// launches — the default is the width it was fixed at before it was
    /// resizable, and the same key `PRSidebarView` used to hold it under
    /// before its own frame/rule/resize became this tab's to wrap it in.
    @AppStorage("prSidebarWidth") private var sidebarWidth = 216.0
    @State private var liveSidebarWidth: Double?

    /// The description pane's height over the conversation, draggable at the
    /// divider between them and remembered across launches, as the sidebar's
    /// width is. Clamped where it is drawn — see `PRSplitMetrics`.
    @AppStorage(PRSplitMetrics.storageKey) private var descriptionHeight = PRSplitMetrics.defaultUpper
    @State private var liveDescriptionHeight: Double?

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

    /// Two stacked panes, each scrolling on its own: what the pull request
    /// *is* (header, branch, description) over what has been *said* on it
    /// (the conversation, ending in the comment box), with a draggable
    /// divider between them.
    private func mainColumn(for pr: PRDetail) -> some View {
        PRSplitView(
            upperHeight: descriptionHeight,
            liveUpperHeight: $liveDescriptionHeight,
            onCommit: { descriptionHeight = $0 }
        ) {
            VStack(alignment: .leading, spacing: 16) {
                header(for: pr)
                branchLine(for: pr)
                descriptionSection(for: pr)
            }
        } lower: {
            conversationSection(for: pr)
        }
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
                composer
            } else {
                VStack(alignment: .leading, spacing: 12) {
                    ForEach(Array(entries.enumerated()), id: \.offset) { _, entry in
                        PRConversationEntryView(entry: entry)
                    }
                    composer
                }
                .padding(12)
                .card(radius: 10)
            }
        }
    }

    /// The one comment box, GitHub's own bottom-of-thread one, at the end of
    /// the conversation and scrolling with it. GitHub's per-line review
    /// threads have a reply API of their own, which `nat` does not wrap, so
    /// this posts a new top-level comment rather than a reply threaded onto
    /// whatever is above it.
    private var composer: some View {
        PRComposerView(
            placeholder: "Leave a comment on the pull request…",
            text: $commentText,
            isSending: isSendingComment,
            error: commentError,
            onSend: { Task { await sendComment() } }
        )
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

/// The main column's split: where its divider starts, and the floors that
/// keep either pane from collapsing under a drag or a short window. Named
/// rather than inline so `PRSkeletonView` splits at the very same height.
enum PRSplitMetrics {
    /// The `@AppStorage` key the description pane's height persists under.
    static let storageKey = "prDescriptionHeight"
    /// Room for the header, branch line and a few lines of description.
    static let defaultUpper: Double = 200
    /// The header and branch line, and a line of description under them.
    static let minUpper: Double = 90
    /// The section label and the comment box at its floor, with a line of
    /// conversation over them.
    static let minLower: Double = 150
    /// Each pane's insets — the reading column's own, as one scroll had them.
    static let horizontalPadding: CGFloat = 22
    static let verticalPadding: CGFloat = 18
}

/// The PR tab's main column, split: `upper` over `lower`, each in its own
/// scroll, with a draggable divider between them. The upper pane's height is
/// the caller's to persist; this only clamps it to the room there is and
/// moves it under a drag. Shared with `PRSkeletonView`, so a first load
/// splits exactly where the loaded pane will.
struct PRSplitView<Upper: View, Lower: View>: View {
    let upperHeight: Double
    @Binding var liveUpperHeight: Double?
    let onCommit: (Double) -> Void
    @ViewBuilder let upper: Upper
    @ViewBuilder let lower: Lower

    var body: some View {
        GeometryReader { proxy in
            let available = proxy.size.height
            let height = paneSplitHeight(
                stored: liveUpperHeight ?? upperHeight,
                available: available,
                minUpper: PRSplitMetrics.minUpper,
                minLower: PRSplitMetrics.minLower
            )
            VStack(spacing: 0) {
                pane { upper }
                    .frame(height: height)
                // The rule hangs off the lower pane's top rather than the
                // upper's bottom: `rectBorder` stacks a bottom edge under a
                // row with no height of its own, so it lands at the top.
                pane { lower }
                    .frame(maxHeight: .infinity)
                    .rule(.separator, edges: [.top], width: 0.5)
            }
            .overlay(alignment: .top) {
                PaneRowResizeHandle(
                    height: upperHeight,
                    liveHeight: $liveUpperHeight,
                    onCommit: onCommit,
                    minHeight: PRSplitMetrics.minUpper,
                    maxHeight: max(PRSplitMetrics.minUpper, available - PRSplitMetrics.minLower),
                    edge: .bottom
                )
                .offset(y: height - 4.5)
            }
        }
        .frame(maxWidth: .infinity)
    }

    private func pane<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        ScrollView {
            content()
                .padding(.horizontal, PRSplitMetrics.horizontalPadding)
                .padding(.vertical, PRSplitMetrics.verticalPadding)
                .frame(maxWidth: .infinity, alignment: .leading)
                .inelastic()
        }
    }
}

/// The composer's own metrics. Named rather than inline for the reason
/// `ButtonMetrics` is, and for one more: `PRSkeletonView` reserves the band
/// the box lays out in, and a skeleton that guessed it would hand the
/// conversation rows the composer then took back.
enum PRComposerMetrics {
    /// What the editor is at least — one line — and at most. Inside the
    /// conversation's scroll the editor grows with its text between the two,
    /// and only scrolls itself once a comment is taller than the ceiling.
    static let editorMinHeight: CGFloat = 36
    static let editorMaxHeight: CGFloat = 60
    /// The inset over the editor.
    static let editorTopPadding: CGFloat = 7
    /// The send row under it: the button's own height and the insets around
    /// the row.
    static let sendRowHeight: CGFloat = 22
    static let sendRowTopPadding: CGFloat = 5
    static let sendRowBottomPadding: CGFloat = 7
    /// The field's corner.
    static let cornerRadius: CGFloat = 8

    /// What the box comes to empty, which is how a pull request opens: the
    /// editor at its floor, the send row under it, and the insets around both.
    static var height: CGFloat {
        editorTopPadding + editorMinHeight + sendRowTopPadding + sendRowHeight + sendRowBottomPadding
    }
}

/// The pull request's own comment box, matching the mock's `PRComposer`
/// chrome (a rounded field with a toolbar row under it, the send button in
/// accent) but functional: it posts through `pr-comment` rather than sitting
/// there as decoration. It sits at the end of the conversation, inside its
/// scroll; see the call site for why it is not a threaded reply.
struct PRComposerView: View {
    let placeholder: String
    @Binding var text: String
    let isSending: Bool
    let error: String?
    let onSend: () -> Void

    private var canSend: Bool {
        !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !isSending
    }

    /// What the hidden sizer lays out: the text as typed, with a space after
    /// a trailing newline (or in place of nothing) so the line the caret has
    /// just opened is counted too.
    private var sizingText: String {
        text.isEmpty || text.hasSuffix("\n") ? text + " " : text
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            VStack(alignment: .leading, spacing: 0) {
                // The editor is sized by a hidden copy of its own text rather
                // than by a frame of its own: inside a scroll a `TextEditor`
                // is offered no height to fill and would sit at whichever
                // bound its frame named, so the copy — wrapped at the text
                // view's own 5pt line fragment padding — measures the lines,
                // clamped to the editor's floor and ceiling, and the editor
                // is laid over exactly that.
                Text(sizingText)
                    .font(Typo.mono(size: Typo.subhead))
                    .padding(.horizontal, 5)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(
                        minHeight: PRComposerMetrics.editorMinHeight,
                        maxHeight: PRComposerMetrics.editorMaxHeight,
                        alignment: .top
                    )
                    .hidden()
                    .overlay(alignment: .topLeading) { editor }
                    .padding(.horizontal, 2)
                        .padding(.top, PRComposerMetrics.editorTopPadding)

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

    /// The editor itself, with the placeholder under it while it is empty.
    private var editor: some View {
        ZStack(alignment: .topLeading) {
            if text.isEmpty {
                // Laid where the editor's own first line starts — the text
                // view's 5pt line fragment padding, and no top offset — so
                // the caret blinks exactly at the placeholder's first letter.
                Text(placeholder)
                    // The editor's own font, since the placeholder stands
                    // exactly where the first typed letter will: a
                    // proportional one would sit a hair off the caret it is
                    // drawn behind.
                    .font(Typo.mono(size: Typo.subhead))
                    .ink(.tertiary)
                    .padding(.leading, 5)
                    .allowsHitTesting(false)
            }
            TextEditor(text: $text)
                .font(Typo.mono(size: Typo.subhead))
                .scrollContentBackground(.hidden)
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
