import SwiftUI
import NatKit

// MARK: - Ad hoc sessions

/// An ad hoc session's navigator: Thread, Changes and PR, no Brief — a
/// session has no brief. Changes reads the session's own branches (picked
/// with the chip row the old Diff tab had), PR its own pull requests.
struct SessionNavigatorView: View {
    @Bindable var appModel: AppModel
    @Environment(\.clock) private var clock
    let session: Session
    @Binding var open: Set<NavigatorSection>
    @Binding var main: MainPaneMode
    let review: DiffReview

    @State private var branches: [String] = []
    @State private var branchesSessionID: String?

    private var projectID: String { appModel.projectStore?.projectID ?? "" }
    private var agent: AgentStatus? { appModel.activityStore?.agents[session.tag] }
    private var diffStore: SessionDiffStore { appModel.sessionDiffStore(projectID: projectID) }
    private var prStore: PRStore { appModel.prStore(projectID: projectID) }
    private var checkedOut: String? { branches.first }
    private var selectedBranch: String? {
        appModel.selectedPickerID(.branch, sessionID: session.id, among: branches, defaultID: checkedOut)
    }
    private var requestedBranch: String? { selectedBranch == checkedOut ? nil : selectedBranch }
    private var selectedPRURL: String? {
        appModel.selectedPickerID(.pullRequest, sessionID: session.id, among: session.prs.map(\.url))
    }

    var body: some View {
        NavigatorColumn(anyOpen: !open.isEmpty) {
            NavSectionView(
                label: "Session", open: open.contains(.thread), selected: main == .terminal,
                onHead: { click(.thread) }, onFold: { fold(.thread) }
            ) {
                ScrollView {
                    VStack(alignment: .leading, spacing: LogMetrics.spacing) {
                        ThreadEventCard(
                            event: ThreadEvent(
                                .launched, who: "Started", meta: ago(clock().timeIntervalSince(session.startedAt)),
                                facts: [session.branch.isEmpty ? ThreadFact("dir", session.dir) : ThreadFact("branch", session.branch)],
                                metaIsAction: false),
                            connector: .solid)
                        if let agent {
                            let waiting = AgentActivity(agent.activity) == .waiting
                            let reading = agentFacts(agent)
                            ThreadEventCard(
                                event: ThreadEvent(
                                    .agent, who: "Agent", meta: waiting ? "on standby" : "working",
                                    tone: waiting ? .hot : .accent,
                                    facts: reading.model + reading.context, isLive: true),
                                connector: .dashed)
                        } else {
                            ThreadEventCard(event: ThreadEvent(.agent, who: "Agent", meta: "ended"))
                        }
                    }
                    .taskLogPadding(live: agent != nil)
                }
                .thinScrollers()
            }
            NavSectionView(
                label: "Changes", open: open.contains(.changes), selected: main == .diff,
                onHead: { click(.changes) }, onFold: { fold(.changes) }
            ) {
                changesBody
            }
            if !session.prs.isEmpty {
                NavSectionView(
                    label: "PR", open: open.contains(.pr), selected: main == .pr,
                    onHead: { click(.pr) }, onFold: { fold(.pr) }
                ) {
                    PROpenInGitHubButton(store: prStore, expectedNumber: sessionSelectedPRNumber(appModel, session))
                } content: {
                    prBody
                }
            }
        }
        .task(id: "\(session.id)|\(requestedBranch ?? "")") {
            if branchesSessionID != session.id {
                branches = (try? await appModel.sessionStatus(projectID: projectID, sessionID: session.id))?
                    .branches.map(\.branch) ?? []
                branchesSessionID = session.id
            }
            await diffStore.fetch(projectID: projectID, sessionID: session.id, branch: requestedBranch)
        }
        .task(id: selectedPRURL) {
            guard let url = selectedPRURL else { return }
            await prStore.fetch(projectID: projectID, sliceRef: url, sessionID: session.id)
            prStore.startPolling()
        }
        .onDisappear { prStore.stopPolling() }
    }

    /// A header click — see `NavigatorFocus.clickingHead`. Every section of
    /// a session has a main-pane view of its own.
    private func click(_ section: NavigatorSection) {
        let shows: MainPaneMode = switch section {
        // A session builds no Visual changes header, so this is never
        // reached; the terminal is the harmless answer.
        case .thread, .visuals: .terminal
        case .changes: .diff
        case .pr: .pr
        }
        apply(NavigatorFocus(open: open, main: main).clickingHead(section, shows: shows))
    }

    private func fold(_ section: NavigatorSection) {
        apply(NavigatorFocus(open: open, main: main).togglingFold(section))
    }

    private func apply(_ focus: NavigatorFocus) {
        // Sections snap open and shut: no animation, everything shown or
        // hidden at once.
        open = focus.open
        if focus.main != main { main = focus.main }
    }

    private var changesBody: some View {
        ScrollView {
            VStack(spacing: 0) {
                if branches.count > 1 {
                    ChipPickerView(model: ChipPickerModel(
                        chips: branchChips(branches, checkedOut: checkedOut), selectedID: selectedBranch
                    )) { appModel.selectPicker(.branch, sessionID: session.id, id: $0) }
                }
                if let diff = diffStore.loadState.diff {
                    ForEach(diff.files) { file in
                        HStack(spacing: 6) {
                            Text(file.path).font(Typo.mono(size: Typo.codeView)).ink(.primary)
                                .lineLimit(1).truncationMode(.head)
                                .frame(maxWidth: .infinity, alignment: .leading)
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
                            main = .diff
                        }
                    }
                } else if let message = diffStore.loadState.errorMessage {
                    NavNotice(text: "The diff could not be read: \(message)")
                } else {
                    QuietLoadingView(label: "Reading the branch").frame(maxWidth: .infinity, minHeight: 80)
                }
            }
            .padding(.vertical, 4)
        }
        .thinScrollers()
    }

    @ViewBuilder
    private var prBody: some View {
        VStack(spacing: 0) {
            if session.prs.count > 1 {
                ChipPickerView(model: ChipPickerModel(chips: pullRequestChips(session.prs), selectedID: selectedPRURL)) {
                    appModel.selectPicker(.pullRequest, sessionID: session.id, id: $0)
                }
            }
            let selected = session.prs.first { $0.url == selectedPRURL }
            if let pr = prStore.loadState.pr, pr.number == selected?.number {
                PRSectionBody(pr: pr, staleMessage: prStore.loadState.errorMessage)
            } else if let message = prStore.loadState.errorMessage {
                NavNotice(text: "The pull request could not be read: \(message)")
            } else {
                QuietLoadingView(label: "Reading the pull request").frame(maxWidth: .infinity, minHeight: 80)
            }
        }
    }
}

/// An ad hoc session's main pane, under the titlebar band: its agent's terminal,
/// its branch's diff, or its picked pull request's conversation.
struct SessionMainPane: View {
    @Bindable var appModel: AppModel
    let session: Session
    @Binding var mode: MainPaneMode
    let review: DiffReview

    var body: some View {
        let store = appModel.sessionDiffStore(projectID: appModel.projectStore?.projectID ?? "")
        VStack(spacing: 0) {
            switch mode {
            case .terminal, .empty, .visuals:
                AgentTerminalPane(
                    agent: appModel.activityStore?.agents[session.tag],
                    sessionExists: { appModel.activityStore?.agents[session.tag] != nil })
            case .diff:
                if let diff = store.loadState.diff {
                    ContinuousDiffView(
                        diff: diff,
                        isViewed: { store.isViewed($0) },
                        isCollapsed: { store.isCollapsed($0) },
                        onToggleViewed: { store.toggleViewed($0) },
                        onToggleCollapsed: { store.toggleCollapsed($0) },
                        review: review)
                } else if let message = store.loadState.errorMessage {
                    MainPaneNote(text: "The diff could not be read: \(message)")
                } else {
                    QuietLoadingView(label: "Reading the branch").frame(maxWidth: .infinity, maxHeight: .infinity)
                }
            case .pr:
                PRConversationPane(
                    store: appModel.prStore(projectID: appModel.projectStore?.projectID ?? ""),
                    expectedNumber: selectedPRNumber)
            }
        }
        .surface(.window)
    }

    private var selectedPRNumber: Int? { sessionSelectedPRNumber(appModel, session) }
}

/// The number of the pull request a session's PR section's picker has chosen.
@MainActor
private func sessionSelectedPRNumber(_ appModel: AppModel, _ session: Session) -> Int? {
    let url = appModel.selectedPickerID(.pullRequest, sessionID: session.id, among: session.prs.map(\.url))
    return session.prs.first { $0.url == url }?.number
}

// MARK: - The workshop

/// The workshop's navigator, the same for an Untitled tab and a project:
/// Brief — the request, Plan before and End session after — over Plan —
/// what the agent has proposed, with Accept beside Keep workshopping.
struct WorkshopNavigatorView: View {
    @Bindable var appModel: AppModel
    let projectName: String
    @State private var folded: Set<String> = []
    @State private var confirmingEnd = false
    @State private var endError: String?

    var body: some View {
        // There is no Plan section until there is a plan to show in it.
        let proposal = appModel.activeProposal
        NavigatorColumn(anyOpen: !folded.contains("brief") || (proposal != nil && !folded.contains("plan"))) {
            NavSectionView(label: "Brief", open: !folded.contains("brief"), onHead: { toggle("brief") }) {
                briefActions
            } content: {
                ScrollView { briefContent }.thinScrollers()
            }
            if let proposal {
                NavSectionView(
                    label: "Plan", open: !folded.contains("plan"), selected: appModel.workshopTab == .plan,
                    onHead: clickPlanHead, onFold: { toggle("plan") }
                ) {
                    planActions
                } content: {
                    ScrollView { planContent(proposal) }.thinScrollers()
                }
            }
        }
        // A proposal arriving opens its section, whatever an earlier one's
        // was left as.
        .onChange(of: proposal == nil) { _, gone in
            if !gone { folded.remove("plan") }
        }
        .focusedSceneValue(\.workshopMenu, WorkshopMenuActions(
            showTerminal: appModel.workshopTabs.contains(.terminal) ? { appModel.showWorkshopTab(.terminal) } : nil,
            showPlan: appModel.workshopTabs.contains(.plan) ? { appModel.showWorkshopTab(.plan) } : nil))
        .alert("End the workshop session?", isPresented: $confirmingEnd) {
            Button("End session", role: .destructive) {
                Task { endError = await appModel.closeWorkshopTab() }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("The planning agent is still running. Ending it ends its session; the draft goes with it.")
        }
    }

    /// Sections snap open and shut, as a slice's do.
    private func toggle(_ section: String) {
        if folded.contains(section) { folded.remove(section) } else { folded.insert(section) }
    }

    /// The Plan header, as a slice section's: opens the section and puts
    /// the Plan tab up — or folds it, when it is open with the tab already
    /// up. With no Plan tab (no session) it only folds or unfolds.
    private func clickPlanHead() {
        guard appModel.workshopTabs.contains(.plan), folded.contains("plan") || appModel.workshopTab != .plan else {
            return toggle("plan")
        }
        folded.remove("plan")
        appModel.showWorkshopTab(.plan)
    }

    // MARK: - Brief

    @ViewBuilder
    private var briefActions: some View {
        if appModel.planningAgent != nil {
            Button(action: { confirmingEnd = true }) { HeaderActionLabel(title: "End session") }
                .buttonStyle(GnatHeaderButtonStyle())
        } else {
            Button(action: { Task { await appModel.launchWorkshop(request: appModel.workshopDraft) } }) {
                HeaderActionLabel(title: "Plan", systemImage: "arrow.right", isBusy: appModel.workshopLaunching)
            }
            .buttonStyle(GnatHeaderButtonStyle(primary: true))
            .keyboardShortcut(.return, modifiers: .command)
            .disabled(appModel.workshopLaunching)
        }
    }

    @ViewBuilder
    private var briefContent: some View {
        if appModel.workshopLaunched {
            NavProse {
                if let request = appModel.workshopRequest {
                    if request.isEmpty {
                        Text("This planning session was started without a request.").ink(.secondary)
                    } else {
                        Excerpt(text: request) { shown in
                            Text(markdownAttributed(shown, size: GnatMetrics.body))
                                .ink(.primary)
                                .textSelection(.enabled)
                        }
                    }
                } else {
                    Text("This session was started before gnat opened. You can find its request in the terminal.")
                        .ink(.secondary)
                }
                if appModel.activeProposal == nil {
                    Text("The plan will appear here when the agent proposes one.").ink(.tertiary)
                }
                if let endError { Text(endError).ink(.danger) }
            }
        } else {
            NavProse {
                Text("Describe the changes you want to make in the editor on the right. You can list several and the agent will plan milestones and tasks for them in one go.")
                    .ink(.secondary)
                Text("Use \u{2318}\u{21A9} to send the brief to an agent.").ink(.tertiary)
                if let error = appModel.workshopLaunchError { Text(error).ink(.danger) }
            }
        }
    }

    // MARK: - Plan

    @ViewBuilder
    private var planActions: some View {
        Button(action: { appModel.keepWorkshopping() }) { HeaderActionLabel(title: ProposalText.keepLabel) }
            .buttonStyle(GnatHeaderButtonStyle())
            .disabled(appModel.proposalAccepting)
        Button(action: { Task { await appModel.acceptProposal() } }) {
            HeaderActionLabel(title: "Accept", systemImage: "checkmark", isBusy: appModel.proposalAccepting)
        }
        .buttonStyle(GnatHeaderButtonStyle(primary: true))
        .disabled(appModel.proposalAccepting)
    }

    /// The proposal: its counts, on an Untitled tab the name field, the
    /// caption, then the proposed tree in the sidebar's own rows — each
    /// milestone a folder over its slices, every one Todo, the ones the
    /// proposal creates marked NEW. A revised
    /// proposal replaces it in place. A slice row scrolls the Plan tab to
    /// that slice's box.
    private func planContent(_ proposal: PlanProposal) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            NavProse {
                NavHeading(text: ProposalText.counts(milestones: proposal.milestoneCount, slices: proposal.sliceCount))
                if appModel.activeTabIsUntitled {
                    TextField("Project name", text: Binding(
                        get: { appModel.proposalName }, set: { appModel.proposalName = $0 }))
                        .textFieldStyle(.plain)
                        .font(Typo.mono(size: Typo.input))
                        .padding(.horizontal, 8)
                        .padding(.vertical, 5)
                        .overlay {
                            RoundedRectangle(cornerRadius: 4).strokeBorder(DesignTokens.rule(.border, on: .window), lineWidth: 1)
                        }
                        .onSubmit { Task { await appModel.acceptProposal() } }
                        .disabled(appModel.proposalAccepting)
                    Text(appModel.proposalError ?? ProposalText.acceptCaption(name: appModel.proposalName))
                        .ink(appModel.proposalError == nil ? .secondary : .danger)
                } else {
                    Text(appModel.proposalError ?? ProposalText.projectAcceptCaption(project: projectName))
                        .ink(appModel.proposalError == nil ? .secondary : .danger)
                }
                if !proposal.removals.isEmpty {
                    Text(ProposalText.removalWarning(count: proposal.removals.count)).ink(.warning)
                }
            }
            ForEach(proposal.folders, id: \.milestoneID) { folder in
                TreeMilestoneLine(
                    name: folder.title, count: "\(folder.slices.count)", indent: 12, isNew: folder.isNew)
                ForEach(folder.slices, id: \.sliceID) { slice in
                    TreeSliceLine(title: slice.name, state: .todo, indent: 20)
                        .onTapGesture { appModel.showProposedSlice(slice.sliceID) }
                }
            }
            if proposal.changesBoard { boardChanges(proposal) }
        }
        .padding(.bottom, 8)
    }

    /// What the proposal does to tasks already planned, under what it
    /// creates: a struck-through row per removal, a row per move naming
    /// where it goes, a row per edit that unfolds its new brief.
    @ViewBuilder
    private func boardChanges(_ proposal: PlanProposal) -> some View {
        NavProse { NavHeading(text: ProposalText.boardChangesHeading) }
        if !proposal.removals.isEmpty {
            ProposalChangeGroupLine(label: ProposalText.removeLabel, systemImage: "trash", count: proposal.removals.count)
            ForEach(proposal.removals, id: \.self) { name in
                ProposalChangeLine(title: name, struck: true)
            }
        }
        if !proposal.moves.isEmpty {
            ProposalChangeGroupLine(label: ProposalText.moveLabel, systemImage: "arrow.right", count: proposal.moves.count)
            ForEach(proposal.moves, id: \.name) { move in
                ProposalChangeLine(title: move.name)
                Text(ProposalText.moveDestination(move.milestone))
                    .monoXS().ink(.tertiary).lineLimit(1)
                    .padding(.leading, 38)
                    .padding(.trailing, 10)
                    .padding(.bottom, 6)
            }
        }
        if !proposal.edits.isEmpty {
            ProposalChangeGroupLine(label: ProposalText.editLabel, systemImage: "pencil", count: proposal.edits.count)
            ForEach(proposal.edits, id: \.name) { edit in
                let open = appModel.expandedProposalEdits.contains(edit.name)
                ProposalChangeLine(title: edit.name, disclosure: open)
                    .onTapGesture { appModel.toggleProposalEdit(edit.name) }
                if open {
                    MarkdownView(text: edit.brief, size: GnatMetrics.body)
                        .padding(.leading, 38)
                        .padding(.trailing, 12)
                        .padding(.vertical, 6)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
        }
    }
}

/// The head of one kind of change in the Plan section — Remove, Move or
/// Edit — in a milestone line's place: its glyph, its label, its count.
private struct ProposalChangeGroupLine: View {
    let label: String
    let systemImage: String
    let count: Int

    var body: some View {
        HStack(spacing: 7) {
            Image(systemName: systemImage)
                .font(.system(size: 11))
                .ink(.tertiary)
                .frame(width: GnatMetrics.treeFolderColumn)
            Text(label)
                .font(.system(size: GnatMetrics.body))
                .ink(.secondary)
            Spacer(minLength: 0)
            Text("\(count)").monoXS().ink(.tertiary)
        }
        .padding(.leading, 12)
        .padding(.trailing, 10)
        .frame(height: GnatMetrics.sidebarRowHeight)
    }
}

/// One task already planned that the proposal changes, as a slice line of
/// the tree: struck through where it is removed, a disclosure chevron where
/// its brief is replaced.
private struct ProposalChangeLine: View {
    let title: String
    var struck = false
    var disclosure: Bool?

    var body: some View {
        HStack(spacing: 6) {
            StateDot(state: .todo).frame(width: GnatMetrics.treeGlyphColumn)
            Text(title)
                .font(.system(size: GnatMetrics.body))
                .strikethrough(struck)
                .ink(struck ? .tertiary : .secondary)
                .lineLimit(1)
            Spacer(minLength: 4)
            if let disclosure {
                Image(systemName: disclosure ? "chevron.down" : "chevron.right")
                    .font(.system(size: 9, weight: .semibold))
                    .ink(.tertiary)
                    // One width for both, which differ: the column holds still.
                    .frame(width: GnatMetrics.treeGlyphColumn)
            }
        }
        .padding(.leading, 20)
        .padding(.trailing, 10)
        .frame(height: GnatMetrics.sidebarRowHeight)
        .contentShape(Rectangle())
    }
}

/// The workshop's main pane: before launch, the brief editor, the whole
/// height of the pane, with no tabs; from launch on, under the titlebar
/// band's Terminal and Plan tabs (`AppModel.workshopTab`), the planning
/// agent's terminal or the proposal read brief by brief.
struct WorkshopMainPane: View {
    @Bindable var appModel: AppModel

    var body: some View {
        VStack(spacing: 0) {
            if appModel.workshopTab == .plan, let proposal = appModel.activeProposal {
                WorkshopPlanView(
                    proposal: proposal, scroll: appModel.workshopPlanScroll, folded: appModel.foldedProposedSlices,
                    onToggle: { appModel.toggleProposedSliceFold($0) })
            } else if appModel.workshopLaunched {
                AgentTerminalPane(
                    agent: appModel.planningAgent,
                    emptyText: appModel.workshopLaunching ? "Starting the workshop session\u{2026}" : nil,
                    focusRequest: appModel.terminalFocusRequest,
                    sessionExists: { appModel.planningAgent != nil })
            } else {
                WorkshopBriefEditor(text: $appModel.workshopDraft) {
                    Task { await appModel.launchWorkshop(request: appModel.workshopDraft) }
                }
            }
        }
        .surface(.window)
    }
}

/// The Plan tab: the proposal as the briefs it files, read like the Changes
/// view reads a diff — one box per proposed slice, in plan order, under a
/// heading per milestone, each box a header strip with the slice's title
/// over its brief as rendered markdown, folding to the strip on a click. A
/// revised proposal replaces it in place; `scroll` is the Plan section's ask
/// to bring a slice's box to the top.
private struct WorkshopPlanView: View {
    let proposal: PlanProposal
    let scroll: WorkshopPlanScroll?
    /// The boxes folded to their header, by `PlanProposal.sliceID`.
    let folded: Set<String>
    let onToggle: (String) -> Void

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    ForEach(Array(proposal.milestones.enumerated()), id: \.offset) { index, milestone in
                        ProposedMilestoneHeading(milestone: milestone)
                        ForEach(Array(milestone.slices.enumerated()), id: \.offset) { sliceIndex, slice in
                            let id = PlanProposal.sliceID(milestone: index, slice: sliceIndex)
                            ProposedSliceBox(
                                slice: slice, folded: folded.contains(id),
                                followsFolded: sliceIndex > 0
                                    && folded.contains(PlanProposal.sliceID(milestone: index, slice: sliceIndex - 1)),
                                onToggle: { onToggle(id) })
                                .id(id)
                        }
                    }
                }
                .padding(.bottom, 24)
            }
            .thinScrollers()
            .task(id: scroll) {
                guard let scroll else { return }
                proxy.scrollTo(scroll.sliceID, anchor: .top)
            }
        }
    }
}

/// A milestone's heading in the Plan tab: its name, NEW where the proposal
/// creates it, and how many tasks it files.
private struct ProposedMilestoneHeading: View {
    let milestone: PlanProposal.Milestone

    var body: some View {
        HStack(spacing: 8) {
            Text(milestone.name)
                .font(.system(size: GnatMetrics.body, weight: .semibold))
                .ink(.primary)
            if milestone.isNew { Chip("New", tone: .accent, size: .small) }
            Spacer(minLength: 0)
            Text("\(milestone.slices.count) \(milestone.slices.count == 1 ? "task" : "tasks")")
                .monoXS().ink(.tertiary)
        }
        .padding(.horizontal, 14)
        .padding(.top, 22)
        .padding(.bottom, 10)
    }
}

/// One proposed slice in the Plan tab, in the diff file box's chrome: a
/// header strip drawn as `DiffViewportView.drawHeader` draws a file's — its
/// band and rules, the chevron at 11, the title at 30 in the header's mono —
/// then, unless folded to it, one quiet line naming what it waits on, where it
/// waits on anything, and its brief. The rules follow the diff's: a bottom
/// rule always, a top one only where no folded box's bottom rule is already
/// above it, so two never meet.
private struct ProposedSliceBox: View {
    let slice: PlanProposal.ProposedSlice
    var folded = false
    /// The box above is folded: its bottom rule is the one between the two.
    var followsFolded = false
    let onToggle: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            header
            if !folded { content }
        }
    }

    private var header: some View {
        HStack(spacing: 8) {
            DisclosureChevron(open: !folded)
            Text(slice.name)
                .font(Typo.mono(size: Typo.codeView))
                .ink(.primary)
                .lineLimit(1)
                .truncationMode(.head)
            Spacer(minLength: 0)
        }
        .padding(.leading, 10)
        .padding(.trailing, 16)
        // The band, its bottom rule included, as `DiffLayout` counts it.
        .frame(height: DiffMetrics().headerHeight)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(DesignTokens.fill(.window))
        .overlay(alignment: .top) {
            if !followsFolded { DesignTokens.rule(.separator, on: .window).frame(height: 1) }
        }
        .overlay(alignment: .bottom) { DesignTokens.rule(.separator, on: .window).frame(height: 1) }
        .contentShape(Rectangle())
        .onTapGesture(perform: onToggle)
        .help(folded ? "Show the brief" : "Fold to the title")
    }

    private var content: some View {
        VStack(alignment: .leading, spacing: 10) {
            if !slice.dependsOn.isEmpty {
                Text("Waits on " + slice.dependsOn.joined(separator: ", "))
                    .font(.system(size: Typo.subhead))
                    .ink(.tertiary)
                    .textSelection(.enabled)
            }
            if slice.brief.isEmpty {
                Text("This task has a title but no brief.")
                    .font(.system(size: GnatMetrics.body))
                    .ink(.tertiary)
            } else {
                MarkdownView(text: slice.brief, size: GnatMetrics.body)
            }
        }
        .padding(.horizontal, 14)
        .padding(.top, 12)
        .padding(.bottom, 18)
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// The brief being written, edge to edge in the main pane, with a
/// placeholder until anything is typed — ending on the launch shortcut — and
/// focus on arrival.
private struct WorkshopBriefEditor: View {
    @Binding var text: String
    let onLaunch: () -> Void
    @FocusState private var focused: Bool

    var body: some View {
        ZStack(alignment: .topLeading) {
            TextEditor(text: $text)
                .font(Typo.mono(size: Typo.input))
                .lineSpacing(3)
                .scrollContentBackground(.hidden)
                .focused($focused)
                .padding(.horizontal, 14)
                .padding(.vertical, 12)
            if text.isEmpty {
                Text("What should the plan cover? Describe the changes you want. You can list several at once. Press \u{2318}\u{21A9} to start planning.")
                    .font(Typo.mono(size: Typo.code))
                    .ink(.tertiary)
                    .padding(.horizontal, 19)
                    .padding(.vertical, 12)
                    .allowsHitTesting(false)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .onAppear { focused = true }
    }
}
