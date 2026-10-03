import SwiftUI
import NatKit

// MARK: - Ad hoc sessions

/// An ad hoc session's navigator: Thread, Changes and PR, no Brief — a
/// session has no brief. Changes reads the session's own branches (picked
/// with the chip row the old Diff tab had), PR its own pull requests.
struct SessionNavigatorView: View {
    @Bindable var appModel: AppModel
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
                    VStack(spacing: 6) {
                        ThreadEventCard(event: ThreadEvent(
                            .launched, who: "Started", meta: ago(Date().timeIntervalSince(session.startedAt)),
                            facts: [session.branch.isEmpty ? ThreadFact("dir", session.dir) : ThreadFact("branch", session.branch)]))
                        if let agent {
                            let waiting = AgentActivity(agent.activity) == .waiting
                            let reading = agentFacts(agent)
                            ThreadEventCard(event: ThreadEvent(
                                .agent, who: "Agent", meta: waiting ? "waiting for you" : "working",
                                tone: waiting ? .hot : .accent,
                                facts: reading.model + reading.context))
                        } else {
                            ThreadEventCard(event: ThreadEvent(.agent, who: "Agent", meta: "ended"))
                        }
                    }
                    .padding(6)
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
                            Text(file.path).font(Typo.mono(size: 13)).ink(.primary)
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
                    NavNotice(text: "The diff could not be read — \(message)")
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
                NavNotice(text: "The pull request could not be read — \(message)")
            } else {
                QuietLoadingView(label: "Reading the pull request").frame(maxWidth: .infinity, minHeight: 80)
            }
        }
    }
}

/// An ad hoc session's main pane, under its titlebar segment: its agent's terminal,
/// its branch's diff, or its picked pull request's conversation.
struct SessionMainPane: View {
    @Bindable var appModel: AppModel
    let session: Session
    @Binding var mode: MainPaneMode
    let review: DiffReview
    var tabs: [MainPaneTab] = []
    var onTab: (MainPaneTab) -> Void = { _ in }

    var body: some View {
        let store = appModel.sessionDiffStore(projectID: appModel.projectStore?.projectID ?? "")
        VStack(spacing: 0) {
            MainPaneTitlebar(tabs: tabs, selected: mode, onTab: onTab) {
                switch mode {
                case .terminal, .empty, .visuals:
                    AgentModelHeading(agent: appModel.activityStore?.agents[session.tag])
                case .pr:
                    PROpenInGitHubButton(
                        store: appModel.prStore(projectID: appModel.projectStore?.projectID ?? ""),
                        expectedNumber: selectedPRNumber)
                case .diff:
                    EmptyView()
                }
            }
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
                    MainPaneNote(text: "Failed to read the diff — \(message)")
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

    /// The number of the pull request the PR section's picker has chosen.
    private var selectedPRNumber: Int? {
        let url = appModel.selectedPickerID(.pullRequest, sessionID: session.id, among: session.prs.map(\.url))
        return session.prs.first { $0.url == url }?.number
    }
}

// MARK: - The workshop

/// Whether the workshop on screen has been launched — its agent live, or its
/// launch under way: what moves the brief from the editor into the Brief
/// section, read-only, and puts the terminal up in its place.
@MainActor
private func workshopLaunched(_ appModel: AppModel) -> Bool {
    appModel.planningAgent != nil || appModel.workshopLaunching
}

/// The workshop's navigator, the same for an Untitled tab and a project:
/// Brief — the request, Launch before and End session after — over Plan —
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
                NavSectionView(label: "Plan", open: !folded.contains("plan"), onHead: { toggle("plan") }) {
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

    // MARK: - Brief

    @ViewBuilder
    private var briefActions: some View {
        if appModel.planningAgent != nil {
            Button(action: { confirmingEnd = true }) { HeaderActionLabel(title: "End session") }
                .buttonStyle(GnatHeaderButtonStyle())
        } else {
            Button(action: { Task { await appModel.launchWorkshop(request: appModel.workshopDraft) } }) {
                HeaderActionLabel(title: "Launch", systemImage: "arrow.right", isBusy: appModel.workshopLaunching)
            }
            .buttonStyle(GnatHeaderButtonStyle(primary: true))
            .keyboardShortcut(.return, modifiers: .command)
            .disabled(appModel.workshopLaunching)
        }
    }

    @ViewBuilder
    private var briefContent: some View {
        if workshopLaunched(appModel) {
            NavProse {
                if let request = appModel.workshopRequest {
                    if request.isEmpty {
                        Text("Launched with no request — a plain planning session.").ink(.secondary)
                    } else {
                        Excerpt(text: request) { shown in
                            Text(markdownAttributed(shown, size: GnatMetrics.body))
                                .ink(.primary)
                                .textSelection(.enabled)
                        }
                    }
                } else {
                    Text("This session was launched before gnat was; what it was asked is in the terminal.")
                        .ink(.secondary)
                }
                if appModel.activeProposal == nil {
                    Text("The plan appears here when the agent proposes it.").ink(.tertiary)
                }
                if let endError { Text(endError).ink(.danger) }
            }
        } else {
            NavProse {
                Text("Describe the changes you want to make in the editor on the right. You can list several and the agent will plan milestones and tasks for them in one go.")
                    .ink(.secondary)
                Text("\u{2318}\u{21A9} launches.").ink(.tertiary)
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
    /// milestone a folder over its slices, every one Todo. A revised
    /// proposal replaces it in place.
    private func planContent(_ proposal: PlanProposal) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            NavProse {
                NavHeading(text: ProposalText.counts(milestones: proposal.milestoneCount, slices: proposal.sliceCount))
                if appModel.activeTabIsUntitled {
                    TextField("Project name", text: Binding(
                        get: { appModel.proposalName }, set: { appModel.proposalName = $0 }))
                        .textFieldStyle(.plain)
                        .font(Typo.mono(size: GnatMetrics.body))
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
            }
            ForEach(proposal.folders, id: \.milestoneID) { folder in
                TreeMilestoneLine(name: folder.title, count: "\(folder.slices.count)", indent: 12)
                ForEach(folder.slices, id: \.sliceID) { slice in
                    TreeSliceLine(title: slice.name, state: .todo, indent: 20)
                }
            }
        }
        .padding(.bottom, 8)
    }
}

/// The workshop's main pane, with no tabs: before launch, the brief editor,
/// the whole height of the pane; from launch on, the planning agent's
/// terminal under its model heading.
struct WorkshopMainPane: View {
    @Bindable var appModel: AppModel

    var body: some View {
        VStack(spacing: 0) {
            if workshopLaunched(appModel) {
                MainPaneTitlebar { AgentModelHeading(agent: appModel.planningAgent) }
                AgentTerminalPane(
                    agent: appModel.planningAgent,
                    emptyText: appModel.workshopLaunching ? "Starting the workshop session\u{2026}" : nil,
                    focusRequest: appModel.terminalFocusRequest,
                    sessionExists: { appModel.planningAgent != nil })
            } else {
                MainPaneTitlebar {
                    Text("\u{2318}\u{21A9} to launch").monoXS().ink(.tertiary)
                }
                WorkshopBriefEditor(text: $appModel.workshopDraft) {
                    Task { await appModel.launchWorkshop(request: appModel.workshopDraft) }
                }
            }
        }
        .surface(.window)
    }
}

/// The brief being written, edge to edge in the main pane, with a
/// placeholder until anything is typed and focus on arrival.
private struct WorkshopBriefEditor: View {
    @Binding var text: String
    let onLaunch: () -> Void
    @FocusState private var focused: Bool

    var body: some View {
        ZStack(alignment: .topLeading) {
            TextEditor(text: $text)
                .font(Typo.mono(size: Typo.code))
                .lineSpacing(3)
                .scrollContentBackground(.hidden)
                .focused($focused)
                .padding(.horizontal, 14)
                .padding(.vertical, 12)
            if text.isEmpty {
                Text("What should the plan cover? Describe the changes — several at once is fine.")
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
