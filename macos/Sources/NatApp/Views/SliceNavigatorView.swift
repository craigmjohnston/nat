import AppKit
import SwiftUI
import NatKit

/// A slice's navigator: Thread, Changes, Visual changes (only once images
/// are handed in) and PR, stacked, the Thread opening
/// on the brief. Which are open and what the main pane shows are the shell's
/// (`open`, `main`) — the design's own pairing: a header puts its section's
/// view up (the Thread the terminal, Changes the diff, PR its conversation),
/// its chevron only folds. Under them all, pinned to the column's foot, the
/// action bar holds the slice's major actions — Send back to agent, Launch,
/// Approve, Merge — as `NavigatorModel.bar` decides them; the headers keep
/// only secondaries. A resumed slice keeps Changes, Visual changes and PR,
/// each carrying `NavigatorModel.resumedNotice`.
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
    /// Send back to agent's note while its editor is open; nil while shut.
    @State private var sendBackDraft: String?
    @Environment(\.sendBackOpen) private var sendBackOpen

    private var projectID: String { appModel.projectStore?.projectID ?? "" }
    private var agent: AgentStatus? { appModel.activityStore?.agents[slice.id] }
    private var nav: NavigatorModel {
        NavigatorModel(
            slice: slice, agent: agent.map { AgentActivity($0.activity) },
            hasVisuals: !visuals.isEmpty)
    }
    /// This project's last `pr-status` reading.
    private var prReadingOfProject: PRReading { appModel.prStatusStore?.reading(projectID: projectID) ?? .empty }
    /// The failing-checks notice, where the last PR reading has one.
    private var notice: ChecksNotice? {
        checksNotice(
            slice: slice, failing: prReadingOfProject.failingChecks[slice.id],
            hasLiveAgent: agent != nil, events: detail.detail?.events)
    }
    /// The conflict notice, where the last reading — the loaded pull request
    /// where it is this one, else `pr-status`'s — has one.
    private var conflictNotice: ConflictNotice? {
        NatKit.conflictNotice(
            slice: slice,
            conflict: conflict(
                reading: prReadingOfProject.conflicts[slice.id], detail: prStore.loadState.pr, prURL: slice.pr),
            hasLiveAgent: agent != nil)
    }
    /// The PR header's danger icon's tooltip: every notice that applies.
    private var prWarning: String? {
        let texts = [notice?.text, conflictNotice?.text].compactMap { $0 }
        return texts.isEmpty ? nil : texts.joined(separator: "\n")
    }
    /// The PR header's success mark's tooltip, where the last reading has the
    /// checks passing and the gate (`prMarks`) trusts it.
    private var prPassing: String? {
        prMarks(
            prReadingOfProject.marks[slice.id] ?? .none, for: slice, agent: agent.map { AgentActivity($0.activity) }
        ).passingHelp
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
                threadBody(nav)
            }
            // Changes and PR are only there once there is a branch, and a
            // pull request, to show.
            if nav.isLive(.changes) {
                NavSectionView(
                    label: "Changes", open: open.contains(.changes), selected: main == .diff,
                    // New or Updated while any file is, so a folded section
                    // says so.
                    status: diffStore.sectionStatus,
                    onHead: { click(.changes) }, onFold: { fold(.changes) }
                ) {
                    if nav.showsChangesSend { sendCommentsAction }
                } content: {
                    VStack(spacing: 0) {
                        if nav.worksAgain { NavNotice(text: NavigatorModel.resumedNotice, role: .warning) }
                        ChangesSectionBody(
                            appModel: appModel, review: review, slice: slice, reviewing: nav.showsReviewActions
                        ) {
                            main = .diff
                        }
                    }
                }
            }
            // Visual changes is there only once the agent has handed in
            // images — absent, not greyed, before.
            if nav.isLive(.visuals) {
                NavSectionView(
                    label: NavigatorSection.visuals.label, open: open.contains(.visuals), selected: main == .visuals,
                    // New or Updated while any image is, so a folded section
                    // says so.
                    status: visualStore.sectionStatus(sliceID: slice.id, visuals),
                    onHead: { click(.visuals) }, onFold: { fold(.visuals) }
                ) {
                    visualActions(nav)
                } content: {
                    VStack(spacing: 0) {
                        if nav.worksAgain { NavNotice(text: NavigatorModel.resumedNotice, role: .warning) }
                        VisualsSectionBody(appModel: appModel, review: visualReview, slice: slice, visuals: visuals) {
                            main = .visuals
                        }
                    }
                }
            }
            if nav.isLive(.pr) {
                NavSectionView(
                    label: "PR", open: open.contains(.pr), selected: main == .pr,
                    // Merged once Done; else Updated while the head has moved
                    // since the section was last open.
                    status: nav.prStatus ?? NavSectionStatus(prStore.badge(sliceID: slice.id)),
                    warning: prWarning, passing: prPassing, onHead: { click(.pr) }, onFold: { fold(.pr) }
                ) {
                    PROpenInGitHubButton(store: prStore, expectedNumber: pullRequestNumber(slice.pr))
                } content: {
                    prReading
                }
            }
        } footer: {
            actionBar(nav)
        }
        .task(id: slice.id) {
            resetLaunchForm()
            sendBackDraft = sendBackOpen ? sendBackReason(checks: notice, conflict: conflictNotice) : nil
            agentOptions = await AgentOptionsCache.shared.resolve()
        }
        .task(id: "\(slice.id)|\(slice.pr)") {
            guard !slice.pr.isEmpty else { return }
            await prStore.fetch(projectID: projectID, sliceRef: slice.id)
            prStore.startPolling()
        }
        .onDisappear { prStore.stopPolling() }
        // An open PR section — or its conversation up — is the pull request
        // seen, as it is read now.
        .onChange(of: "\(open.contains(.pr) || main == .pr)|\(prStore.loadState.pr?.headRefOid ?? "")", initial: true) {
            if open.contains(.pr) || main == .pr { prStore.markSeen(sliceID: slice.id) }
        }
        // Keyed by every image's hash as well as its URI, so a re-render
        // saved over the same path — which a nudge's re-read carries as a new
        // hash — loads afresh.
        .task(id: "\(slice.id)|\(VisualChange.loadIdentity(visuals))") {
            await visualStore.load(sliceID: slice.id, handIn: detail.detail?.visuals)
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
        let bar = bar(nav)
        let canLaunch = bar.button(.launch).map(isEnabled) ?? false
        let canMerge = bar.button(.merge).map(isEnabled) ?? false
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

    // MARK: - Action bar

    /// The pull request as read, where the reading is this slice's own.
    private var readPR: PRDetail? {
        prStore.loadState.pr.flatMap { pr in
            pullRequestNumber(slice.pr).map { $0 == pr.number } ?? true ? pr : nil
        }
    }

    /// Whether approving is available: a hand-back, read on All commits,
    /// with no send out.
    private var canApprove: Bool {
        slice.handedBack && diffStore.commentsEditable && !review.isSending
    }

    private var canMerge: Bool { readPR.map(mergeIsEnabled) ?? false }

    private func bar(_ nav: NavigatorModel) -> NavigatorBar {
        let state = readPR?.state
        return nav.bar(
            launchTitle: launchMode(nav).actionTitle, pendingComments: diffStore.pendingCommentCount,
            canApprove: canApprove, approveHelp: "Approving is only available while viewing All commits",
            canMerge: canMerge, prOpen: state != PRLifecycleState.merged && state != PRLifecycleState.closed)
    }

    private func kind(_ action: NavigatorBarAction) -> SliceActionKind {
        switch action {
        case .launch: return .launch
        case .approve: return .approve
        case .merge: return .merge
        case .sendBack: return .sendBack
        }
    }

    /// A bar button's own reading, under the one-shot gate.
    private func isEnabled(_ button: NavigatorBarButton) -> Bool {
        appModel.sliceActions.isEnabled(kind(button.action), sliceID: slice.id, available: button.enabled)
    }

    /// The bar at the column's foot: each action the slice has now, the
    /// primary trailing; "Task completed" alone for a Done slice.
    private func actionBar(_ nav: NavigatorModel) -> some View {
        VStack(spacing: 0) {
            if let draft = sendBackDraft, bar(nav).button(.sendBack) != nil {
                SendBackEditor(
                    text: Binding(get: { draft }, set: { sendBackDraft = $0 }),
                    hasLiveAgent: nav.hasLiveAgent,
                    isSending: appModel.sliceActions.isRunning(.sendBack, sliceID: slice.id),
                    error: appModel.sliceActions.error(.sendBack, sliceID: slice.id),
                    onCancel: { sendBackDraft = nil },
                    onSend: { sendBack(draft) })
            }
            actionButtons(nav)
        }
    }

    private func actionButtons(_ nav: NavigatorModel) -> some View {
        NavigatorActionBar {
            switch bar(nav) {
            case .completed:
                Text(NavigatorBar.completedText)
                    .font(.system(size: GnatMetrics.body))
                    .ink(.tertiary)
                    .padding(.horizontal, 12)
            case .buttons(let buttons):
                ForEach(buttons, id: \.title) { button in
                    Button(action: { press(button.action) }) {
                        HeaderActionLabel(
                            title: button.title, systemImage: glyph(button.action),
                            isBusy: appModel.sliceActions.isRunning(kind(button.action), sliceID: slice.id),
                            glyph: button.action == .merge ? .merge : nil)
                    }
                    .buttonStyle(GnatHeaderButtonStyle(primary: button.primary))
                    .disabled(!isEnabled(button))
                    .help(button.help ?? "")
                }
            }
        }
        // Each one-shot outlives its success only once the slice has moved
        // on: fed every availability change, drawn or not.
        .onChange(of: nav.canLaunch, initial: true) { _, available in
            appModel.sliceActions.observe(.launch, sliceID: slice.id, available: available)
        }
        .onChange(of: canApprove, initial: true) { _, available in
            appModel.sliceActions.observe(.approve, sliceID: slice.id, available: available)
        }
        .onChange(of: canMerge, initial: true) { _, available in
            appModel.sliceActions.observe(.merge, sliceID: slice.id, available: available)
        }
        .onChange(of: nav.showsSendBack && nav.canSendBack, initial: true) { _, available in
            appModel.sliceActions.observe(.sendBack, sliceID: slice.id, available: available)
        }
    }

    private func glyph(_ action: NavigatorBarAction) -> String? {
        switch action {
        case .launch: return "arrow.right"
        case .approve: return "checkmark"
        case .merge: return nil
        case .sendBack: return "arrow.uturn.left"
        }
    }

    private func press(_ action: NavigatorBarAction) {
        switch action {
        case .launch: launch()
        // With comments pending, the same confirmation says it sends them
        // and approves on the agent's next hand-back.
        case .approve: review.showApproveConfirm = true
        case .merge: showMergeConfirm = true
        // Opens the editor, prefilled with the pull request's own trouble
        // where it has any; a second press shuts it.
        case .sendBack:
            sendBackDraft = sendBackDraft == nil ? sendBackReason(checks: notice, conflict: conflictNotice) : nil
        }
    }

    /// Send back to agent: the record, then the agent told or launched
    /// (`AppModel.sendBack`); once it has gone, the editor shuts and the
    /// terminal comes up beside the Task log, where the work now is.
    private func sendBack(_ note: String) {
        let appModel = appModel, slice = slice
        let model = model.isEmpty ? nil : model
        let effort = effort.isEmpty ? nil : effort
        Task {
            guard await appModel.sendBack(slice: slice, note: note, model: model, effort: effort) else { return }
            sendBackDraft = nil
            open.insert(.thread)
            main = .terminal
        }
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

    /// One item of the Thread's log, in the order `threadBody` draws them.
    private enum ThreadItem {
        case brief
        case event(ThreadEvent)
        /// A quiet item, drawn folded to its header (`ThreadLogItem.folded`).
        case folded(ThreadEvent)
        /// A run of quiet items folded into one (`ThreadLogItem.group`).
        case group([ThreadEvent])
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
                        case .folded(let event):
                            ThreadEventCard(
                                event: event, connector: connector, collapsible: true, taskRow: { taskRow(id: $0) })
                        case .group(let events):
                            ThreadGroupCard(events: events, connector: connector, taskRow: { taskRow(id: $0) })
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

    /// The Task log's events, as nat reports them.
    private var threadLog: [ThreadEvent] {
        buildThreadEvents(
            slice: slice, agent: agent, brief: detail.detail?.brief, events: detail.detail?.events,
            plan: plan, milestones: milestones)
    }

    private func threadItems(_ nav: NavigatorModel) -> [ThreadItem] {
        let log = threadLog
        var items: [ThreadItem] = [.brief]
        // The quiet items folded, and three or more of them in a row folded
        // together into one.
        for item in threadLogItems(log) {
            switch item {
            case .card(let event):
                if !event.awaitsTriage {
                    items.append(.event(event))
                } else if !followUps.isEmpty {
                    items.append(.triage(event.when))
                }
            case .folded(let event):
                items.append(.folded(event))
            case .group(let events):
                items.append(.group(events))
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
        // A relaunch only where nat recorded an earlier launch.
        return launchIsRelaunch(log: threadLog, hasLiveAgent: nav.hasLiveAgent) ? .relaunch : .launch
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

    /// Send, while comments are pending on the diff — secondary, the bar's
    /// Approve with comments being the default.
    @ViewBuilder
    private var sendCommentsAction: some View {
        let pending = diffStore.pendingCommentCount
        let editable = diffStore.commentsEditable
        if pending > 0 {
            Button(action: { Task { await review.sendComments(appModel: appModel, slice: slice) } }) {
                HeaderActionLabel(
                    title: "Send \(pending) \(plural(pending, "comment", "comments"))",
                    systemImage: "arrow.right",
                    isBusy: review.isSending)
            }
            .buttonStyle(GnatHeaderButtonStyle(primary: false))
            .disabled(review.isSending || appModel.sliceActions.isRunning(.approve, sliceID: slice.id) || !editable)
            .help(editable ? "" : "Comments are only sent while viewing All commits")
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
            .buttonStyle(GnatHeaderButtonStyle(primary: false))
            .disabled(visualReview.isSending || !nav.showsVisualActions)
            .help(nav.showsVisualActions ? "" : "No live agent to send the comments to")
        }
    }

    // MARK: - PR

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

    private var prReading: some View {
        VStack(spacing: 0) {
            // Drawn from the project's reading too, so it shows before the
            // pull request itself has been read.
            if nav.worksAgain { NavNotice(text: NavigatorModel.resumedNotice, role: .warning) }
            if let conflictNotice {
                NavNotice(text: conflictNotice.text, role: .danger)
            }
            prBody
        }
    }

    @ViewBuilder
    private var prBody: some View {
        if let pr = prStore.loadState.pr {
            PRSectionBody(
                pr: pr,
                reviewerStore: prStore,
                checksStore: prStore,
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
    /// The store checks are re-run and cancelled through — a slice's pull
    /// request. Nil for an ad hoc session's, whose rows carry no controls
    /// (`nat slice-checks-rerun` names a slice).
    var checksStore: PRStore?
    /// A check drawn as under the pointer whether it is or not — a story's.
    var hoveredCheck: String?
    /// The check row under the pointer, whose re-run and cancel show.
    @State private var pointerCheck: String?
    var staleMessage: String?
    var actionError: String?
    /// A source container's word on its tasks' pull requests (`task_note`),
    /// drawn small at the foot.
    var note: String?

    var body: some View {
        let verdict = reviewVerdict(reviewDecision: pr.reviewDecision)
        let controls = ChecksControls(checks: pr.checks)
        VStack(spacing: 0) {
            if let notice = checksStore?.checksNotice {
                NavNotice(text: notice.text, role: notice.isError ? .danger : .secondary)
            }
            ScrollView {
                NavProse {
                    if let staleMessage {
                        Text("The refresh failed, so this is the last reading: \(staleMessage)").ink(.warning)
                    }
                    if let actionError {
                        Text(actionError).ink(.danger)
                    }

                    ChecksHeading(controls: controls, store: checksStore)
                    if pr.checks.isEmpty {
                        Text("No checks have run.").ink(.secondary)
                    } else {
                        VStack(alignment: .leading, spacing: 0) {
                            ForEach(Array(pr.checks.enumerated()), id: \.offset) { _, check in
                                checkRow(check, controls: controls)
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
    }

    /// A check's row: its line, then — on a slice's pull request with an
    /// Actions run behind any check — its re-run and cancel at the trailing
    /// edge, in the heading's columns. Under the pointer the whole row is
    /// washed, full bleed to the section's edges.
    ///
    /// As tall as a sidebar task row, and its controls drawn only under the
    /// pointer — or while one of them is under way, so a click never goes
    /// without its spinner. Hidden, never removed, so the name keeps its
    /// width as the pointer comes and goes (the sidebar `+`'s way).
    private func checkRow(_ check: PRCheck, controls: ChecksControls) -> some View {
        let source = checksStore?.checksActionSource
        let shows = hoveredCheck == check.name || pointerCheck == check.name
            || source == .rerun(check.name) || source == .cancel(check.name)
        return HStack(spacing: 6) {
            checkLine(check)
            Spacer(minLength: 4)
            if let checksStore, controls.hasControls {
                CheckRowControls(check: check, controls: controls, store: checksStore)
                    .opacity(shows ? 1 : 0)
                    .allowsHitTesting(shows)
                    .accessibilityHidden(!shows)
            }
        }
        .frame(height: GnatMetrics.sidebarRowHeight)
        .padding(.horizontal, 12)
        .gnatRow(washed: hoveredCheck == check.name)
        .onHover { inside in
            if inside { pointerCheck = check.name } else if pointerCheck == check.name { pointerCheck = nil }
        }
        .padding(.horizontal, -12)
    }

    /// A check's line, led by a circle of its outcome: empty for one that
    /// did not run, dashed for one running, and filled — the only two in
    /// colour — for done and failed.
    /// Set in the pane's own sans at its body size, as the Review line is —
    /// a check's name ("test") is a label, not code.
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
                .font(.system(size: GnatMetrics.treeGlyph, weight: .medium))
                .ink(role)
                .frame(width: GnatMetrics.treeGlyphColumn)
            Text(check.name).ink(.primary).lineLimit(1)
            if outcome != .passing {
                Text("· \(checkStateWord(check.state))").ink(.secondary).lineLimit(1)
            }
        }
    }
}
