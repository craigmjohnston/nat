import AppKit
import SwiftUI
import NatKit

/// A slice's navigator: Thread, Changes and PR, stacked, the Thread opening
/// on the brief. Which are open and what the main pane shows are the shell's
/// (`open`, `main`) — the design's own pairing: a header puts its section's
/// view up (the Thread the terminal, Changes the diff, PR its conversation),
/// its chevron only folds.
struct SliceNavigatorView: View {
    @Bindable var appModel: AppModel
    let slice: Slice
    @Binding var open: Set<NavigatorSection>
    @Binding var main: MainPaneMode
    let review: DiffReview

    @State private var editingBrief = false
    @State private var model = ""
    @State private var effort = ""
    @State private var agentOptions = AgentOptions.fallback
    @State private var launchWarning: String?
    @State private var showMergeConfirm = false
    @State private var showsFullBrief = false

    private var projectID: String { appModel.projectStore?.projectID ?? "" }
    private var agent: AgentStatus? { appModel.activityStore?.agents[slice.id] }
    private var nav: NavigatorModel {
        NavigatorModel(
            slice: slice, agent: agent.map { AgentActivity($0.activity) },
            fixLaunched: appModel.fixLaunched[slice.id] != nil)
    }
    private var detail: SliceDetailLoadState { appModel.sliceDetailStore(projectID: projectID).state(for: slice.id) }
    private var diffStore: DiffStore { review.store(appModel) }
    private var prStore: PRStore { appModel.prStore(projectID: projectID) }
    private var milestoneName: String {
        appModel.projectStore?.state.projectInfo?.milestones.first { $0.id == slice.milestoneID }?.name
            ?? slice.milestoneID
    }

    var body: some View {
        let nav = nav
        NavigatorColumn(anyOpen: !open.isEmpty) {
            NavSectionView(
                label: "Thread", open: open.contains(.thread),
                selected: main == .terminal && nav.agentAvailable,
                onHead: { click(.thread) }, onFold: { fold(.thread) }
            ) {
                threadActions(nav)
            } content: {
                threadBody(nav)
            }
            NavSectionView(
                label: "Changes", open: open.contains(.changes), selected: main == .diff,
                live: nav.isLive(.changes), onHead: { click(.changes) }, onFold: { fold(.changes) }
            ) {
                if nav.showsReviewActions { reviewActions }
            } content: {
                ChangesSectionBody(appModel: appModel, review: review, slice: slice, reviewing: nav.showsReviewActions) {
                    main = .diff
                }
            }
            NavSectionView(
                label: "PR", open: open.contains(.pr), selected: main == .pr,
                live: nav.isLive(.pr), onHead: { click(.pr) }, onFold: { fold(.pr) }
            ) {
                if nav.showsMerge { mergeAction }
            } content: {
                prBody
            }
        }
        .task(id: slice.id) {
            resetLaunchForm()
            showsFullBrief = false
            agentOptions = await AgentOptionsCache.shared.resolve()
        }
        .task(id: "\(slice.id)|\(slice.pr)") {
            guard !slice.pr.isEmpty else { return }
            await prStore.fetch(projectID: projectID, sliceRef: slice.id)
            prStore.startPolling()
        }
        .onDisappear { prStore.stopPolling() }
        .sheet(isPresented: $editingBrief) {
            EditBriefSheetView(
                projectID: projectID, sliceID: slice.id, sliceName: slice.name,
                onClose: { editingBrief = false },
                onSaved: {
                    editingBrief = false
                    Task {
                        await appModel.sliceDetailStore(projectID: projectID).fetch(sliceRef: slice.id)
                        await appModel.refresh()
                    }
                }
            )
        }
        .confirmationDialog(
            approveQuestion, isPresented: Bindable(review).showApproveConfirm, titleVisibility: .visible
        ) {
            Button(diffStore.pendingCommentCount > 0 ? "Send & Approve on Hand-back" : "Approve & Open PR") {
                Task { await review.approve(appModel: appModel, slice: slice) }
            }
            Button("Cancel", role: .cancel) {}
        }
        .confirmationDialog(
            "Merge pull request #\(prStore.loadState.pr?.number ?? pullRequestNumber(slice.pr) ?? 0)?",
            isPresented: $showMergeConfirm, titleVisibility: .visible
        ) {
            Button("Merge") { Task { await merge() } }
            Button("Cancel", role: .cancel) {}
        }
    }

    // MARK: - Heads

    /// A header click — see `NavigatorFocus.clickingHead`.
    private func click(_ section: NavigatorSection) {
        apply(NavigatorFocus(open: open, main: main).clickingHead(section, shows: nav.mainMode(for: section)))
    }

    /// A chevron click: fold, nothing else.
    private func fold(_ section: NavigatorSection) {
        apply(NavigatorFocus(open: open, main: main).togglingFold(section))
    }

    private func apply(_ focus: NavigatorFocus) {
        withAnimation(Motion.stateChange) { open = focus.open }
        if focus.main != main { main = focus.main }
    }

    // MARK: - Brief

    /// The Thread's first item: the brief, cut to its first few lines until
    /// asked for the rest, its Edit, and the slice's facts as its foot.
    private var briefCard: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Text("Brief").monoXS(weight: .medium).ink(.secondary)
                Spacer(minLength: 0)
                Button("Edit") { editingBrief = true }
                    .buttonStyle(GnatLinkButtonStyle())
                    .disabled(slice.status != "Todo" || detail.detail == nil)
                    .help(slice.status == "Todo" ? "Edit the brief" : "Only a Todo slice's brief can be edited")
            }
            .padding(.horizontal, 10)
            .padding(.top, 8)

            VStack(alignment: .leading, spacing: 4) {
                if let detail = detail.detail {
                    if detail.brief.isEmpty {
                        Text("No brief yet — what you write here becomes the agent's prompt.").ink(.secondary)
                    } else {
                        let excerpt = briefExcerpt(detail.brief, maxWords: briefExcerptWords)
                        Text(markdownAttributed(
                            showsFullBrief ? detail.brief : excerpt ?? detail.brief, size: 13.5))
                            .ink(.primary)
                            .textSelection(.enabled)
                        if excerpt != nil {
                            Button(showsFullBrief ? "Show less" : "Show more") { showsFullBrief.toggle() }
                                .buttonStyle(GnatLinkButtonStyle())
                        }
                    }
                } else if let message = detail.errorMessage {
                    Text("The brief could not be read — \(message)").ink(.danger)
                } else {
                    QuietLoadingView(label: "Reading the brief")
                        .frame(maxWidth: .infinity, minHeight: 60)
                }
            }
            .font(.system(size: 13.5))
            .lineSpacing(2)
            .padding(.horizontal, 10)
            .padding(.top, 4)
            .padding(.bottom, 8)

            facts
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .clipShape(RoundedRectangle(cornerRadius: 4))
        .overlay {
            RoundedRectangle(cornerRadius: 4).strokeBorder(DesignTokens.rule(.separator, on: .window), lineWidth: 1)
        }
    }

    private var facts: some View {
        let deps = dependencyEntries(slice.dependsOn, plan: appModel.projectStore?.state.projectInfo?.slices ?? [])
        let branch = (slice.branch ?? "").isEmpty ? nil : slice.branch
        return Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 3) {
            GridRow {
                Text("milestone").ink(.tertiary)
                Text(milestoneName).ink(.primary).lineLimit(1)
            }
            GridRow {
                Text("depends").ink(.tertiary)
                Text(deps.isEmpty ? "none" : deps.map(\.name).joined(separator: ", "))
                    .ink(deps.isEmpty ? .secondary : .primary).lineLimit(1)
            }
            GridRow {
                Text("branch").ink(.tertiary)
                Text(branch ?? (slice.status == "Done" ? "none" : "assigned on launch"))
                    .ink(branch == nil ? .secondary : .primary)
                    .lineLimit(1).truncationMode(.middle)
            }
        }
        .monoXS()
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .frame(maxWidth: .infinity, alignment: .leading)
        .surface(.chrome)
        .overlay(alignment: .top) { DesignTokens.rule(.separator, on: .window).frame(height: 1) }
    }

    // MARK: - Thread

    private var isLaunching: Bool { appModel.sliceActions.isRunning(.launch, sliceID: slice.id) }
    private var launchError: String? { appModel.sliceActions.error(.launch, sliceID: slice.id) }
    private var followUps: [FollowUp] { detail.detail?.followUps ?? [] }

    @ViewBuilder
    private func threadActions(_ nav: NavigatorModel) -> some View {
        if nav.showsLaunch {
            let enabled = appModel.sliceActions.isEnabled(.launch, sliceID: slice.id, available: nav.canLaunch)
            Button(action: launch) {
                HeaderActionLabel(title: "Launch", systemImage: "arrow.right", isBusy: isLaunching)
            }
            .buttonStyle(GnatHeaderButtonStyle(primary: nav.launchIsPrimary))
            .disabled(!enabled)
            .onChange(of: nav.canLaunch, initial: true) { _, available in
                appModel.sliceActions.observe(.launch, sliceID: slice.id, available: available)
            }
        }
    }

    @ViewBuilder
    private func threadBody(_ nav: NavigatorModel) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                briefCard
                    .padding([.horizontal, .top], 6)
                if !nav.state.isLaunched && agent == nil {
                    NavProse {
                        if nav.state == .blocked {
                            let deps = dependencyEntries(
                                slice.dependsOn, plan: appModel.projectStore?.state.projectInfo?.slices ?? [])
                                .filter { !$0.done }.map(\.name)
                            (Text("Blocked on ") + Text(deps.joined(separator: ", ")).foregroundStyle(DesignTokens.ink(.primary, on: .window))
                                + Text(". Launch unlocks when that slice is done."))
                                .ink(.secondary)
                        } else {
                            Text("Launch starts Claude Code on this brief, in a worktree on a new branch. Its log appears here and the terminal opens on the right.")
                                .ink(.secondary)
                        }
                        launchChips
                    }
                } else {
                    VStack(spacing: 6) {
                        ForEach(Array(buildThreadEvents(slice: slice, agent: agent, brief: detail.detail?.brief).enumerated()), id: \.offset) {
                            ThreadEventCard(event: $0.element)
                        }
                        if !followUps.isEmpty {
                            FollowUpCards(appModel: appModel, slice: slice, followUps: followUps, milestone: milestoneName, hasLiveAgent: agent != nil)
                        }
                    }
                    .padding(6)
                    if nav.showsLaunch {
                        NavProse {
                            Text("No agent is running on it. Launch relaunches one on its branch, told it is continuing.")
                                .ink(.secondary)
                            launchChips
                        }
                    }
                }
                if let launchError { NavNotice(text: launchError) }
                if let launchWarning { NavNotice(text: launchWarning, role: .warning) }
            }
        }
        .inelastic()
    }

    private var launchChips: some View {
        HStack(spacing: 6) {
            NavChipMenu(title: model.isEmpty ? "default model" : model) {
                Button("Default") { model = "" }
                ForEach(agentOptions.models, id: \.self) { option in Button(option) { model = option } }
            }
            NavChipMenu(title: effort.isEmpty ? "default effort" : effort) {
                Button("Default") { effort = "" }
                ForEach(agentOptions.efforts, id: \.self) { option in Button(option) { effort = option } }
            }
        }
    }

    private func resetLaunchForm() {
        model = appModel.config?.sliceAgent?.model ?? ""
        effort = appModel.config?.sliceAgent?.effort ?? ""
        launchWarning = nil
    }

    private func launch() {
        let projectID = projectID, sliceRef = slice.id
        let model = model.isEmpty ? nil : model
        let effort = effort.isEmpty ? nil : effort
        let appModel = appModel
        launchWarning = nil
        Task {
            await appModel.sliceActions.run(.launch, sliceID: sliceRef, select: { tab in
                if tab == .agent {
                    main = .terminal
                    open.insert(.thread)
                }
            }) {
                let result = try await NatClient().sliceLaunch(
                    projectID: projectID, sliceRef: sliceRef, model: model, effort: effort)
                launchWarning = result.warning
                await appModel.refresh()
            }
        }
    }

    // MARK: - Changes

    private var reviewActions: some View {
        let pending = diffStore.pendingCommentCount
        let editable = diffStore.commentsEditable
        let canApprove = slice.handedBack && editable && !review.isSending
        let approving = appModel.sliceActions.isRunning(.approve, sliceID: slice.id)
        let approveEnabled = appModel.sliceActions.isEnabled(.approve, sliceID: slice.id, available: canApprove)
        return HStack(spacing: 0) {
            if pending > 0 {
                // With comments pending, sending them is the default; the
                // split ahead of it holds approving with them — they go to
                // the agent and the pull request opens on its next hand-back.
                Menu {
                    Button("Approve with comments") { review.showApproveConfirm = true }
                        .disabled(!approveEnabled)
                } label: {
                    Image(systemName: "ellipsis")
                        .font(.system(size: 11, weight: .semibold))
                        .frame(width: 26)
                        .frame(maxHeight: .infinity)
                        .foregroundStyle(DesignTokens.ink(.primary, on: .window))
                        .background(DesignTokens.fill(.window))
                        .overlay(alignment: .leading) {
                            DesignTokens.rule(.separator, on: .chrome).frame(width: 1)
                        }
                        .contentShape(Rectangle())
                }
                .menuStyle(.button)
                .buttonStyle(.plain)
                .menuIndicator(.hidden)
                .fixedSize(horizontal: true, vertical: false)
                .frame(maxHeight: .infinity)
                .disabled(review.isSending || approving)
                .help("Approve with the comments instead")

                Button(action: { Task { await review.sendComments(appModel: appModel, slice: slice) } }) {
                    HeaderActionLabel(
                        title: "Send \(pending) \(plural(pending, "comment", "comments"))",
                        systemImage: "arrow.right",
                        isBusy: review.isSending)
                }
                .buttonStyle(GnatHeaderButtonStyle(primary: true))
                .disabled(review.isSending || approving || !editable)
                .help(editable ? "" : "Comments are only sent while viewing All commits")
            } else {
                Button(action: { review.showApproveConfirm = true }) {
                    HeaderActionLabel(title: "Approve", systemImage: "checkmark", isBusy: approving)
                }
                .buttonStyle(GnatHeaderButtonStyle(primary: true))
                .disabled(!approveEnabled)
                .help(editable ? "" : "Approving is only available while viewing All commits")
            }
        }
        .onChange(of: canApprove, initial: true) { _, available in
            appModel.sliceActions.observe(.approve, sliceID: slice.id, available: available)
        }
    }

    private var approveQuestion: String {
        let pending = diffStore.pendingCommentCount
        let branch = diffStore.loadState.diff?.branch ?? slice.branch ?? "the branch"
        return pending > 0
            ? "Send \(pending) \(plural(pending, "comment", "comments")) to the agent, and open a pull request for \(branch) once it hands back?"
            : "Approve and open a pull request for \(branch)?"
    }

    // MARK: - PR

    private var mergeAction: some View {
        let available = prStore.loadState.pr.map(mergeIsEnabled) ?? false
        return Button(action: { showMergeConfirm = true }) {
            HeaderActionLabel(title: "Merge", isBusy: appModel.sliceActions.isRunning(.merge, sliceID: slice.id))
        }
        .buttonStyle(GnatHeaderButtonStyle())
        .disabled(!appModel.sliceActions.isEnabled(.merge, sliceID: slice.id, available: available))
        .onChange(of: available, initial: true) { _, available in
            appModel.sliceActions.observe(.merge, sliceID: slice.id, available: available)
        }
    }

    private func mergeIsEnabled(_ pr: PRDetail) -> Bool {
        guard pr.state != PRLifecycleState.merged, pr.state != PRLifecycleState.closed, !pr.isDraft else { return false }
        return mergeRefusal(pr) == nil
    }

    private func merge() async {
        let store = prStore
        let appModel = appModel
        await appModel.sliceActions.run(.merge, sliceID: slice.id, select: { _ in }) {
            try await store.merge()
            await appModel.refresh()
        }
        store.startPolling()
    }

    @ViewBuilder
    private var prBody: some View {
        if let pr = prStore.loadState.pr {
            PRSectionBody(
                pr: pr,
                staleMessage: prStore.loadState.errorMessage,
                actionError: appModel.sliceActions.error(.merge, sliceID: slice.id)
            )
        } else if let message = prStore.loadState.errorMessage {
            NavProse {
                Text("The pull request could not be read — \(message)").ink(.danger)
                Button("Retry") { Task { await prStore.refresh(); prStore.startPolling() } }
                    .buttonStyle(GnatButtonStyle())
            }
        } else {
            QuietLoadingView(label: "Reading the pull request")
                .frame(maxWidth: .infinity, minHeight: 80)
        }
    }
}

/// The PR section's body: the readout — the checks and the review verdict,
/// then Open in GitHub. The description and the conversation are the main
/// pane's (`PRConversationPane`).
struct PRSectionBody: View {
    let pr: PRDetail
    var staleMessage: String?
    var actionError: String?

    var body: some View {
        let verdict = reviewVerdict(reviewDecision: pr.reviewDecision)
        ScrollView {
            NavProse {
                if let staleMessage {
                    Text("Showing the last reading — \(staleMessage)").ink(.warning)
                }
                if let actionError {
                    Text(actionError).ink(.danger)
                }

                NavHeading(text: "Checks")
                if pr.checks.isEmpty {
                    Text("No checks have run.").ink(.secondary)
                } else {
                    VStack(alignment: .leading, spacing: 3) {
                        ForEach(Array(pr.checks.enumerated()), id: \.offset) { _, check in
                            checkLine(check)
                        }
                    }
                    .monoXS()
                }

                NavHeading(text: "Review")
                Text(verdict.outcome == .passing
                     ? approvedBy(reviews: pr.reviews).map { "\(sentenceCase(verdict.word)) by \($0)" } ?? sentenceCase(verdict.word)
                     : sentenceCase(verdict.word))
                    .ink(.secondary)

                Button("Open in GitHub") {
                    if let url = URL(string: pr.url) { NSWorkspace.shared.open(url) }
                }
                .buttonStyle(GnatButtonStyle())
                .padding(.top, 2)
            }
        }
        .inelastic()
    }

    private func checkLine(_ check: PRCheck) -> some View {
        let outcome = checkOutcome(state: check.state)
        let (glyph, role): (String, InkRole) = switch outcome {
        case .passing: ("\u{2713}", .success)
        case .failing: ("\u{2717}", .danger)
        case .pending: ("\u{25D0}", .hot)
        case .skipped: ("\u{25CB}", .secondary)
        }
        return HStack(spacing: 6) {
            Text(glyph).ink(role)
            Text(check.name).ink(.primary).lineLimit(1)
            if outcome != .passing {
                Text("· \(checkStateWord(check.state))").ink(.secondary).lineLimit(1)
            }
        }
    }
}
