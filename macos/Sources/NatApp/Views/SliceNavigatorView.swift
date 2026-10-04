import AppKit
import SwiftUI
import NatKit

/// A slice's navigator: Thread, Changes, Visual changes (only once images
/// are handed in) and PR, stacked, the Thread opening
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
    let visualReview: VisualReview
    /// The launch card's model and effort — the shell's, since the main
    /// pane's heading says what a launch will run as.
    @Binding var model: String
    @Binding var effort: String

    @State private var editingBrief = false
    @State private var agentOptions = AgentOptions.fallback
    @State private var launchWarning: String?
    @State private var showMergeConfirm = false
    /// Whether the Task header's run menu is open — a story's seam too.
    @State private var runMenuOpen = false

    private var projectID: String { appModel.projectStore?.projectID ?? "" }
    private var sliceRuns: [RunCommand] { appModel.sliceRuns(ofProject: projectID) }
    private var agent: AgentStatus? { appModel.activityStore?.agents[slice.id] }
    private var nav: NavigatorModel {
        NavigatorModel(
            slice: slice, agent: agent.map { AgentActivity($0.activity) },
            hasVisuals: !visuals.isEmpty)
    }
    /// The failing-checks notice, where the last PR reading has one.
    private var notice: ChecksNotice? {
        checksNotice(
            slice: slice, failing: appModel.reviewStatsStore?.failingChecks[slice.id],
            hasLiveAgent: agent != nil, events: detail.detail?.events)
    }
    private var detail: SliceDetailLoadState { appModel.sliceDetailStore(projectID: projectID).state(for: slice.id) }
    private var visuals: [VisualChange] { detail.detail?.visuals ?? [] }
    private var diffStore: DiffStore { review.store(appModel) }
    private var visualStore: VisualStore { visualReview.store(appModel) }
    private var prStore: PRStore { appModel.prStore(projectID: projectID) }
    private var milestoneName: String {
        appModel.projectStore?.state.projectInfo?.milestones.first { $0.id == slice.milestoneID }?.name
            ?? slice.milestoneID
    }

    var body: some View {
        let nav = nav
        NavigatorColumn(anyOpen: !open.isEmpty) {
            NavSectionView(
                label: nav.threadLabel, open: open.contains(.thread),
                selected: main == .terminal && nav.agentAvailable,
                onHead: { click(.thread) }, onFold: { fold(.thread) }
            ) {
                threadActions(nav)
            } content: {
                threadBody(nav)
            }
            // Changes and PR are only there once there is a branch, and a
            // pull request, to show.
            if nav.isLive(.changes) {
                NavSectionView(
                    label: "Changes", open: open.contains(.changes), selected: main == .diff,
                    onHead: { click(.changes) }, onFold: { fold(.changes) }
                ) {
                    if nav.showsReviewActions { reviewActions }
                } content: {
                    ChangesSectionBody(appModel: appModel, review: review, slice: slice, reviewing: nav.showsReviewActions) {
                        main = .diff
                    }
                }
            }
            // Visual changes is there only once the agent has handed in
            // images — absent, not greyed, before.
            if nav.isLive(.visuals) {
                NavSectionView(
                    label: NavigatorSection.visuals.label, open: open.contains(.visuals), selected: main == .visuals,
                    onHead: { click(.visuals) }, onFold: { fold(.visuals) }
                ) {
                    visualActions(nav)
                } content: {
                    VisualsSectionBody(appModel: appModel, review: visualReview, slice: slice, visuals: visuals) {
                        main = .visuals
                    }
                }
            }
            if nav.isLive(.pr) {
                NavSectionView(
                    label: "PR", open: open.contains(.pr), selected: main == .pr, status: nav.prStatus,
                    warning: notice?.text, onHead: { click(.pr) }, onFold: { fold(.pr) }
                ) {
                    PROpenInGitHubButton(store: prStore, expectedNumber: pullRequestNumber(slice.pr))
                    if nav.showsMerge { mergeAction }
                } content: {
                    prReading
                }
            }
        }
        .task(id: slice.id) {
            resetLaunchForm()
            agentOptions = await AgentOptionsCache.shared.resolve()
        }
        .task(id: "\(slice.id)|\(slice.pr)") {
            guard !slice.pr.isEmpty else { return }
            await prStore.fetch(projectID: projectID, sliceRef: slice.id)
            prStore.startPolling()
        }
        .onDisappear { prStore.stopPolling() }
        .task(id: "\(slice.id)|\(visuals.map(\.uri).joined(separator: "|"))") {
            await visualStore.load(sliceID: slice.id, visuals: visuals)
        }
        .focusedSceneValue(\.sliceMenu, menuActions(nav))
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
            Button(diffStore.pendingCommentCount > 0 ? "Send & approve on hand-back" : "Approve & open PR") {
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

    // MARK: - Menu

    /// The Slice menu and View's sections — each nil, and so disabled,
    /// exactly where the control it mirrors is.
    private func menuActions(_ nav: NavigatorModel) -> SliceMenuActions {
        let canLaunch = nav.showsLaunch
            && appModel.sliceActions.isEnabled(.launch, sliceID: slice.id, available: nav.canLaunch)
        let canMerge = nav.showsMerge
            && appModel.sliceActions.isEnabled(
                .merge, sliceID: slice.id, available: prStore.loadState.pr.map(mergeIsEnabled) ?? false)
        let prURL = URL(string: prStore.loadState.pr?.url ?? slice.pr)
        let notionURL = URL(string: slice.url) ?? NotionPageURL.forPage(slice.id)
        var actions = SliceMenuActions(title: slice.name)
        if canLaunch { actions.launch = launch }
        if slice.status == "Todo" && detail.detail != nil { actions.editBrief = { editingBrief = true } }
        if canMerge { actions.merge = { showMergeConfirm = true } }
        if !slice.pr.isEmpty, let prURL { actions.openPullRequest = { NSWorkspace.shared.open(prURL) } }
        if let notionURL { actions.openInNotion = { NSWorkspace.shared.open(notionURL) } }
        actions.showThread = { show(.thread) }
        if nav.isLive(.changes) { actions.showChanges = { show(.changes) } }
        if nav.isLive(.visuals) {
            // The titlebar tab's own path: open the section, put the images
            // up, never fold.
            actions.showVisuals = {
                apply(NavigatorFocus(open: open, main: main).showing(.visuals, shows: nav.mainMode(for: .visuals)))
            }
        }
        if nav.isLive(.pr) { actions.showPullRequest = { show(.pr) } }
        return actions
    }

    /// A View menu section item: the header click, but never folding a
    /// section whose view is already up.
    private func show(_ section: NavigatorSection) {
        guard !(open.contains(section) && main == nav.mainMode(for: section)) else { return }
        click(section)
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
        // Sections snap open and shut: no animation, everything shown or
        // hidden at once.
        open = focus.open
        if focus.main != main { main = focus.main }
    }

    // MARK: - Brief

    /// The Thread's first item: the brief, cut to its first few lines until
    /// asked for the rest, its Edit, and the slice's facts under it.
    private func briefItem(connector: LogConnector) -> some View {
        LogItem(symbol: briefSymbol, who: "Brief", connector: connector) {
            // Only a Todo task's brief can be edited; once launched there
            // is no Edit at all.
            if slice.status == "Todo" {
                Button("Edit") { editingBrief = true }
                    .buttonStyle(GnatLinkButtonStyle())
                    .disabled(detail.detail == nil)
                    .help("Edit the brief")
            }
        } content: {
            VStack(alignment: .leading, spacing: 0) {
                VStack(alignment: .leading, spacing: 4) {
                    if let detail = detail.detail {
                        if detail.brief.isEmpty {
                            Text("This task has no brief yet. What you write here becomes the agent's prompt.").ink(.secondary)
                        } else {
                            Excerpt(text: detail.brief) { shown in
                                Text(markdownAttributed(shown, size: Typo.scaled(13.5)))
                                    .ink(.primary)
                                    .textSelection(.enabled)
                            }
                        }
                    } else if let message = detail.errorMessage {
                        Text("The brief could not be read: \(message)").ink(.danger)
                    } else {
                        QuietLoadingView(label: "Reading the brief")
                            .frame(maxWidth: .infinity, minHeight: 60)
                    }
                }
                .font(.system(size: Typo.scaled(13.5)))
                .lineSpacing(2)

                facts.padding(.top, 8)
            }
        }
    }

    /// The brief's foot: its milestone, and what it depends on — one row
    /// per dependency, each its dot and name, selecting it on a click.
    private var facts: some View {
        let deps = dependencies
        return Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 3) {
            // A task under a source container: the container, opening in its
            // source where it has a URL, and its own facts, in the
            // milestone's place.
            if let container = detail.detail?.container {
                GridRow {
                    ThreadFactKey(containerNoun)
                    if let url = container.externalURL.flatMap(URL.init(string:)) {
                        Button(container.title) { NSWorkspace.shared.open(url) }
                            .buttonStyle(GnatLinkButtonStyle())
                            .lineLimit(1)
                            .help(url.absoluteString)
                    } else {
                        Text(container.title).ink(.primary).lineLimit(1)
                    }
                }
                SourceFactRows(facts: container.facts)
            } else {
                GridRow {
                    ThreadFactKey("milestone")
                    Text(milestoneName).ink(.primary).lineLimit(1)
                }
            }
            GridRow(alignment: .firstTextBaseline) {
                ThreadFactKey("depends on")
                if deps.isEmpty {
                    Text("none").ink(.secondary)
                } else {
                    VStack(alignment: .leading, spacing: 3) {
                        ForEach(deps) { dep in
                            taskRow(dep)
                        }
                    }
                }
            }
        }
        .monoXS()
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    /// What the project's source calls a container — "card".
    private var containerNoun: String {
        appModel.source(ofProject: projectID)?.containerNoun ?? "container"
    }

    /// Another slice of the plan as a row the user can go to: its dot and
    /// name, its detail on hover, a click selecting it — the brief's depends
    /// list and a note's `task` fact both draw one.
    private func taskRow(_ other: Slice) -> DependencyRow {
        DependencyRow(
            slice: other, state: state(of: other),
            live: appModel.activityStore?.agents[other.id] != nil,
            milestone: milestoneName(of: other),
            onSelect: { Task { await appModel.selectSlice(other.id, inProject: projectID) } })
    }

    /// `taskRow` for a slice named by id, nil where the plan has no such
    /// slice — what a Thread card's fact naming one is drawn with.
    private func taskRow(id: String) -> AnyView? {
        plan.first { $0.id == id }.map { AnyView(taskRow($0)) }
    }

    private var plan: [Slice] { appModel.projectStore?.state.projectInfo?.slices ?? [] }
    private var milestones: [Milestone] { appModel.projectStore?.state.projectInfo?.milestones ?? [] }
    private var dependencies: [Slice] { dependencySlices(slice.dependsOn, plan: plan) }

    private func state(of other: Slice) -> SliceDisplayState {
        displayState(
            for: other, agent: appModel.activityStore?.agents[other.id].map { AgentActivity($0.activity) })
    }

    private func milestoneName(of other: Slice) -> String {
        appModel.projectStore?.state.projectInfo?.milestones.first { $0.id == other.milestoneID }?.name
            ?? other.milestoneID
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
                HeaderActionLabel(title: launchMode(nav).actionTitle, systemImage: "arrow.right", isBusy: isLaunching)
            }
            .buttonStyle(GnatHeaderButtonStyle(primary: nav.launchIsPrimary))
            .disabled(!enabled)
            .onChange(of: nav.canLaunch, initial: true) { _, available in
                appModel.sliceActions.observe(.launch, sliceID: slice.id, available: available)
            }
        }
        // The project's slice-scoped runs, once the slice has handed back —
        // greyed once it is merged, its worktree being gone; spinning while
        // its run starts and for as long as the run's session lives.
        if slice.handedBack, !sliceRuns.isEmpty {
            RunSplitButton(
                runs: sliceRuns, isBusy: appModel.isRunBusy(projectID: projectID, sliceID: slice.id),
                menuOpen: $runMenuOpen,
                isRunning: { appModel.isRunning(projectID: projectID, sliceID: slice.id, label: $0) }
            ) { label in
                Task { await appModel.startRun(projectID: projectID, sliceID: slice.id, label: label) }
            }
            .frame(maxHeight: .infinity)
            .disabled(stage(for: slice, agent: nil) == .done)
        }
    }

    /// One item of the Thread's log, in the order `threadBody` draws them.
    private enum ThreadItem {
        case brief
        case event(ThreadEvent)
        /// The pending follow-ups, awaiting the user's decision — and when
        /// they were proposed, where the log says.
        case triage(Date?)
        case launch
    }

    /// The brief, what has happened since — follow-ups in their place among
    /// it, a proposal still awaiting a decision drawn as the item that takes
    /// one — and, while the slice can be launched, the launch item that says
    /// what comes next. Each item's rule runs on to the next; the last's,
    /// while an agent is live, runs on dashed.
    @ViewBuilder
    private func threadBody(_ nav: NavigatorModel) -> some View {
        let items = threadItems(nav)
        let live = agent != nil
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                VStack(alignment: .leading, spacing: LogMetrics.spacing) {
                    ForEach(Array(items.enumerated()), id: \.offset) { index, item in
                        let connector: LogConnector = index < items.count - 1 ? .solid : live ? .dashed : .none
                        switch item {
                        case .brief:
                            briefItem(connector: connector)
                        case .event(let event):
                            ThreadEventCard(event: event, connector: connector, taskRow: { taskRow(id: $0) })
                        case .triage(let when):
                            FollowUpCards(
                                appModel: appModel, slice: slice, followUps: followUps, milestone: milestoneName,
                                hasLiveAgent: agent != nil, when: when, connector: connector)
                        case .launch:
                            LaunchCard(
                                mode: launchMode(nav), model: $model, effort: $effort, options: agentOptions,
                                base: detail.detail?.base)
                        }
                    }
                }
                .taskLogPadding(live: live)
                if let launchError { NavNotice(text: launchError) }
                if let launchWarning { NavNotice(text: launchWarning, role: .warning) }
            }
        }
        .thinScrollers()
    }

    private func threadItems(_ nav: NavigatorModel) -> [ThreadItem] {
        let log = buildThreadEvents(
            slice: slice, agent: agent, brief: detail.detail?.brief, events: detail.detail?.events,
            plan: plan, milestones: milestones)
        var items: [ThreadItem] = [.brief]
        for event in log {
            if !event.awaitsTriage {
                items.append(.event(event))
            } else if !followUps.isEmpty {
                items.append(.triage(event.when))
            }
        }
        // A reading with no proposal in its log (a nat too old to report
        // one) still gets its pending follow-ups' item.
        if !followUps.isEmpty && !log.contains(where: \.awaitsTriage) { items.append(.triage(nil)) }
        if nav.showsLaunch { items.append(.launch) }
        return items
    }

    private func launchMode(_ nav: NavigatorModel) -> LaunchCard.Mode {
        if nav.state == .blocked {
            return .blocked(waitingOn: dependencies.filter { $0.status != "Done" }.map(\.name))
        }
        if nav.launchIsFix { return .fix }
        return nav.state.isLaunched ? .relaunch : .launch
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
                        .font(.system(size: 12, weight: .semibold))
                        .frame(width: 26)
                        .frame(maxHeight: .infinity)
                        .foregroundStyle(DesignTokens.ink(.primary, on: .chrome))
                        .hoverWash(cornerRadius: 0)
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

    // MARK: - Visual changes

    /// Send, while comments are pending on the slice's images — disabled
    /// while a send is out, or with no live agent to receive them.
    @ViewBuilder
    private func visualActions(_ nav: NavigatorModel) -> some View {
        let pending = visualStore.comments(for: slice.id).count
        if pending > 0 {
            Button(action: { Task { await visualReview.sendComments(appModel: appModel, slice: slice) } }) {
                HeaderActionLabel(
                    title: "Send \(pending) \(plural(pending, "comment", "comments"))",
                    systemImage: "arrow.right",
                    isBusy: visualReview.isSending)
            }
            .buttonStyle(GnatHeaderButtonStyle(primary: true))
            .disabled(visualReview.isSending || !nav.showsVisualActions)
            .help(nav.showsVisualActions ? "" : "No live agent to send the comments to")
        }
    }

    // MARK: - PR

    private var mergeAction: some View {
        let available = prStore.loadState.pr.map(mergeIsEnabled) ?? false
        return Button(action: { showMergeConfirm = true }) {
            HeaderActionLabel(
                title: "Merge", isBusy: appModel.sliceActions.isRunning(.merge, sliceID: slice.id), glyph: .merge)
        }
        .buttonStyle(GnatHeaderButtonStyle(primary: true))
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
    private var prReading: some View {
        if let pr = prStore.loadState.pr {
            PRSectionBody(
                pr: pr,
                reviewerStore: prStore,
                staleMessage: prStore.loadState.errorMessage,
                actionError: appModel.sliceActions.error(.merge, sliceID: slice.id),
                note: detail.detail?.container?.taskNote
            )
        } else if let message = prStore.loadState.errorMessage {
            NavProse {
                Text("The pull request could not be read: \(message)").ink(.danger)
                Button("Retry") { Task { await prStore.refresh(); prStore.startPolling() } }
                    .buttonStyle(GnatButtonStyle())
            }
        } else {
            QuietLoadingView(label: "Reading the pull request")
                .frame(maxWidth: .infinity, minHeight: 80)
        }
    }
}

/// The PR section's body: the readout — the checks and the review verdict.
/// The title, the description, the conversation and Open in GitHub are the
/// main pane's (`PRConversationPane`, `PROpenInGitHubButton`).
struct PRSectionBody: View {
    let pr: PRDetail
    /// The store reviewers are asked through — a slice's pull request. Nil
    /// for an ad hoc session's, which lists its requests but cannot edit
    /// them (`nat pr-reviewers` names a slice).
    var reviewerStore: PRStore?
    var staleMessage: String?
    var actionError: String?
    /// A source container's word on its tasks' pull requests (`task_note`),
    /// drawn small at the foot.
    var note: String?

    var body: some View {
        let verdict = reviewVerdict(reviewDecision: pr.reviewDecision)
        ScrollView {
            NavProse {
                if let staleMessage {
                    Text("The refresh failed, so this is the last reading: \(staleMessage)").ink(.warning)
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
                }

                NavHeading(text: "Review")
                Text(verdict.outcome == .passing
                     ? approvedBy(reviews: pr.reviews).map { "\(sentenceCase(verdict.word)) by \($0)" } ?? sentenceCase(verdict.word)
                     : sentenceCase(verdict.word))
                    .ink(.secondary)

                ReviewersBlock(pr: pr, store: reviewerStore)

                if let note {
                    Text(note)
                        .font(.system(size: GnatMetrics.xs))
                        .ink(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                        .padding(.top, 4)
                }
            }
        }
        .thinScrollers()
    }

    /// A check's line, led by a circle of its outcome: empty for one that
    /// did not run, dashed for one running, and filled — the only two in
    /// colour — for done and failed.
    /// Set in the pane's own sans at its body size, as the Review line is —
    /// a check's name ("CI / test") is a label, not code.
    private func checkLine(_ check: PRCheck) -> some View {
        let outcome = checkOutcome(state: check.state)
        let (symbol, role): (String, InkRole) = switch outcome {
        case .passing: ("checkmark.circle.fill", .success)
        case .failing: ("xmark.circle.fill", .danger)
        case .pending: ("circle.dashed", .secondary)
        case .skipped: ("circle", .tertiary)
        }
        return HStack(spacing: 6) {
            Image(systemName: symbol)
                .font(.system(size: 11, weight: .medium))
                .ink(role)
                .frame(width: 13)
            Text(check.name).ink(.primary).lineLimit(1)
            if outcome != .passing {
                Text("· \(checkStateWord(check.state))").ink(.secondary).lineLimit(1)
            }
        }
    }
}
