import SwiftUI
import NatKit

/// The Changes section's body: the commit switcher (`DiffCommitsMenu`) over
/// the file list it filters, then one row per file — on a review, a viewed
/// box and the file's pending-comment count beside it — with its New or
/// Updated badge (`DiffStore.badge`) and its tally.
/// Picking a row scrolls the main pane's diff to that file and puts the diff
/// up.
struct ChangesSectionBody: View {
    @Bindable var appModel: AppModel
    let review: DiffReview
    let slice: Slice
    let reviewing: Bool
    let onPick: () -> Void

    private var store: DiffStore { review.store(appModel) }

    var body: some View {
        VStack(spacing: 0) {
            DiffCommitsMenu(
                commits: store.commits,
                selectedCommit: store.selectedCommit,
                onSelectCommit: { sha in Task { await store.selectCommit(sha) } },
                bottomPadding: 0)
                .padding(.horizontal, 8)
                .padding(.top, 8)
            ScrollView {
                VStack(spacing: 0) {
                    if let diff = store.loadState.diff {
                        if diff.files.isEmpty {
                            GnatNote(text: "nothing changed against the base", leading: 10)
                        }
                        ForEach(diff.files) { fileRow($0) }
                    } else if let message = store.loadState.errorMessage {
                        NavNotice(text: "The diff could not be read: \(message)")
                        Button("Retry") { Task { await review.refresh(appModel: appModel) } }
                            .buttonStyle(GnatButtonStyle())
                    } else {
                        QuietLoadingView(label: "Reading the branch")
                            .frame(maxWidth: .infinity, minHeight: 80)
                    }
                }
                .padding(.vertical, 4)
            }
            .thinScrollers()

            notices
        }
    }

    private func fileRow(_ file: DiffFileModel) -> some View {
        let viewed = store.isViewed(file.path)
        let comments = store.commentsByPath[file.path]?.count ?? 0
        return HStack(spacing: 6) {
            if reviewing {
                Button(action: { store.toggleViewed(file.path) }) {
                    ViewedCheckbox(checked: viewed)
                }
                .buttonStyle(.plain)
                .help(viewed ? "Mark not viewed" : "Mark viewed")
            }
            Text(file.path)
                .font(.system(size: Typo.scaled(13)))
                .ink(.primary)
                .lineLimit(1)
                .truncationMode(.head)
                .frame(maxWidth: .infinity, alignment: .leading)
            if let badge = store.badge(file.path) {
                SeenBadgeChip(badge: badge)
            }
            if reviewing && comments > 0 {
                Image(systemName: "text.bubble.fill")
                    .font(.system(size: 10))
                    .ink(.secondary)
                    .help("Pending comments")
            }
            HStack(spacing: 4) {
                Text("+\(file.adds)").ink(.success)
                Text("\u{2212}\(file.dels)").ink(.danger)
            }
            .monoXS()
        }
        .padding(.horizontal, 10)
        .frame(height: GnatMetrics.rowHeight)
        .gnatRow()
        .contentShape(Rectangle())
        .onTapGesture {
            review.requestScroll(to: file.path)
            onPick()
        }
    }

    @ViewBuilder
    private var notices: some View {
        if let stale = store.loadState.errorMessage, store.loadState.diff != nil {
            NavNotice(text: "The refresh failed, so this is the last reading: \(stale)", role: .warning)
        }
        if let drop = review.dropNotice { NavNotice(text: drop, role: .warning) }
        if let error = review.sendError { NavNotice(text: error) }
        if let error = appModel.sliceActions.error(.approve, sliceID: slice.id) { NavNotice(text: error) }
        if reviewing && !store.commentsEditable {
            NavNotice(text: "You are viewing one commit. Switch to All commits to comment or approve.", role: .secondary)
        }
    }
}

/// One batch of the agent's proposed follow-ups as one item of the Thread's
/// log, headed with how many there are — its icon and meta in the hue of what
/// waits on the user — each follow-up a row of it with its Queue / Fold in /
/// Drop picker, and under them the item's own two actions, Discard all and
/// Apply, which decide this batch alone. Its choices, apply and error are its
/// batch's own in `FollowUpStore`.
struct FollowUpCards: View {
    @Bindable var appModel: AppModel
    let slice: Slice
    let batch: Int
    /// This batch's pending follow-ups.
    let followUps: [FollowUp]
    let milestone: String
    let hasLiveAgent: Bool
    /// When the proposal was filed, off its log event's stamp — nil where
    /// the log has none.
    var when: Date?
    var connector: LogConnector = .none
    @Environment(\.clock) private var clock

    private var store: FollowUpStore { appModel.followUpStore }
    private var choices: [Int: FollowUpChoice] { store.choices(sliceID: slice.id, batch: batch) }
    /// This batch's own apply or discard, which spins its button.
    private var isApplying: Bool { store.isApplying(sliceID: slice.id, batch: batch) }
    /// Any batch's of the slice, which holds every card's controls.
    private var isBusy: Bool { store.isApplying(sliceID: slice.id) }

    var body: some View {
        LogItem(
            symbol: ThreadEventKind.followUps.symbol, iconRole: .hot, who: "Agent",
            meta: "proposed \(followUps.count) follow-up\(followUps.count == 1 ? "" : "s")", metaRole: .hot,
            when: when.map { threadTimestamp($0, now: clock()) }, connector: connector
        ) {
            VStack(alignment: .leading, spacing: 0) {
                ForEach(Array(followUps.enumerated()), id: \.element.index) { offset, followUp in
                    VStack(alignment: .leading, spacing: 6) {
                        Text(followUp.title)
                            .font(.system(size: Typo.scaled(13.5)))
                            .ink(.primary)
                            .fixedSize(horizontal: false, vertical: true)
                        Excerpt(text: followUp.brief) { shown in
                            Text(shown)
                                .font(.system(size: Typo.scaled(13)))
                                .ink(.secondary)
                                .fixedSize(horizontal: false, vertical: true)
                                .textSelection(.enabled)
                        }
                        picker(followUp, position: offset + 1)
                    }
                    .padding(.vertical, 8)
                    .overlay(alignment: .top) {
                        if offset > 0 { DesignTokens.rule(.separator, on: .window).frame(height: 1) }
                    }
                }
                actions
            }
        }
    }

    /// What stands in the way, if anything, over Discard all and Apply.
    private var actions: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let notice {
                Text(notice.text)
                    .font(.system(size: Typo.scaled(13)))
                    .ink(notice.role)
                    .fixedSize(horizontal: false, vertical: true)
            }
            HStack(spacing: 6) {
                Spacer(minLength: 0)
                Button("Discard all") {
                    let sliceID = slice.id, batch = batch, followUps = followUps
                    Task { await appModel.discardFollowUps(sliceID: sliceID, batch: batch, followUps: followUps) }
                }
                .buttonStyle(GnatButtonStyle())
                .disabled(isBusy)
                Button(action: apply) {
                    HeaderActionLabel(title: "Apply", systemImage: "checkmark", isBusy: isApplying)
                }
                .buttonStyle(GnatButtonStyle(primary: true))
                .disabled(!canApply || isBusy)
            }
        }
        .padding(.top, 8)
        .frame(maxWidth: .infinity, alignment: .leading)
        .overlay(alignment: .top) { DesignTokens.rule(.separator, on: .window).frame(height: 1) }
    }

    private var canApply: Bool {
        FollowUpStore.canApply(followUps: followUps, choices: choices, hasLiveAgent: hasLiveAgent)
    }

    private func apply() {
        let sliceID = slice.id, batch = batch, followUps = followUps
        Task { await appModel.applyFollowUps(sliceID: sliceID, batch: batch, followUps: followUps) }
    }

    /// What the foot says above the actions: only what stands in the way —
    /// a refusal, or no live agent to fold anything into.
    private var notice: (text: String, role: InkRole)? {
        if let error = store.error(sliceID: slice.id, batch: batch) { return (error, .danger) }
        if !hasLiveAgent {
            return ("No live agent, so nothing can be folded in. Relaunch the task first, or queue it instead.", .warning)
        }
        return nil
    }

    private func picker(_ followUp: FollowUp, position: Int) -> some View {
        let selection = Binding<FollowUpChoice?>(
            get: { choices[position] },
            set: { store.setChoice($0, sliceID: slice.id, batch: batch, position: position) }
        )
        return Picker("", selection: selection) {
            ForEach(FollowUpChoice.allCases, id: \.self) { choice in
                Text(choice.rawValue)
                    .tag(Optional(choice))
                    .selectionDisabled(choice == .fold && !hasLiveAgent)
            }
        }
        .labelsHidden()
        .pickerStyle(.segmented)
        .disabled(isBusy)
        .accessibilityLabel(followUp.title)
    }
}

/// The viewed checkbox: a small rounded box, ticked once a file is viewed —
/// the file list's and the diff header's alike.
struct ViewedCheckbox: View {
    @Environment(\.ground) private var ground
    let checked: Bool

    var body: some View {
        Text(checked ? "\u{2713}" : "")
            .font(.system(size: 9))
            .ink(.primary)
            .frame(width: 14, height: 14)
            .background(
                RoundedRectangle(cornerRadius: 3)
                    .fill(checked ? DesignTokens.rowWash(selected: true, on: ground) : .clear))
            .overlay {
                RoundedRectangle(cornerRadius: 3).strokeBorder(
                    DesignTokens.ink(checked ? .tertiary : .quaternary, on: ground), lineWidth: 1)
            }
    }
}
