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
                label: "Thread", open: open.contains(.thread), selected: main == .terminal,
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
                .inelastic()
            }
            NavSectionView(
                label: "Changes", open: open.contains(.changes), selected: main == .diff,
                onHead: { click(.changes) }, onFold: { fold(.changes) }
            ) {
                changesBody
            }
            NavSectionView(
                label: "PR", open: open.contains(.pr), selected: main == .pr, live: !session.prs.isEmpty,
                onHead: { click(.pr) }, onFold: { fold(.pr) }
            ) {
                prBody
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
        case .thread: .terminal
        case .changes: .diff
        case .pr: .pr
        }
        apply(NavigatorFocus(open: open, main: main).clickingHead(section, shows: shows))
    }

    private func fold(_ section: NavigatorSection) {
        apply(NavigatorFocus(open: open, main: main).togglingFold(section))
    }

    private func apply(_ focus: NavigatorFocus) {
        withAnimation(Motion.stateChange) { open = focus.open }
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
        .inelastic()
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

/// An ad hoc session's main pane, under its heading: its agent's terminal,
/// its branch's diff, or its picked pull request's conversation.
struct SessionMainPane: View {
    @Bindable var appModel: AppModel
    let session: Session
    @Binding var mode: MainPaneMode
    let review: DiffReview

    var body: some View {
        let store = appModel.sessionDiffStore(projectID: appModel.projectStore?.projectID ?? "")
        VStack(spacing: 0) {
            MainPaneHeader {
                if mode == .terminal || mode == .empty {
                    AgentModelHeading(agent: appModel.activityStore?.agents[session.tag])
                }
            }
            switch mode {
            case .terminal, .empty:
                AgentTerminalPane(
                    agent: appModel.activityStore?.agents[session.tag],
                    emptyText: "No agent is running on this session.",
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
                let url = appModel.selectedPickerID(.pullRequest, sessionID: session.id, among: session.prs.map(\.url))
                PRConversationPane(
                    store: appModel.prStore(projectID: appModel.projectStore?.projectID ?? ""),
                    expectedNumber: session.prs.first { $0.url == url }?.number)
            }
        }
        .surface(.window)
    }
}

// MARK: - The workshop

/// The workshop's navigator: one section, always open, holding what the
/// planning agent is at — the request to start it on, its launch, the
/// running note, or the plan it proposed with Accept beside Keep
/// workshopping.
struct WorkshopNavigatorView: View {
    @Bindable var appModel: AppModel
    let projectName: String
    @State private var confirmingEnd = false
    @State private var endError: String?

    var body: some View {
        NavigatorColumn(anyOpen: true) {
            NavSectionView(label: "Plan", open: true, onHead: {}) {
                actions
            } content: {
                ScrollView { content }.inelastic()
            }
        }
        .alert("End the workshop session?", isPresented: $confirmingEnd) {
            Button("End Session", role: .destructive) {
                Task { endError = await appModel.closeWorkshopTab() }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("The planning agent is still running. Ending it ends its session; the draft goes with it.")
        }
    }

    @ViewBuilder
    private var actions: some View {
        if appModel.activeProposal != nil {
            Button(action: { appModel.keepWorkshopping() }) { HeaderActionLabel(title: ProposalText.keepLabel) }
                .buttonStyle(GnatHeaderButtonStyle())
            Button(action: { Task { await appModel.acceptProposal() } }) {
                HeaderActionLabel(title: "Accept", systemImage: "checkmark", isBusy: appModel.proposalAccepting)
            }
            .buttonStyle(GnatHeaderButtonStyle(primary: true))
            .disabled(appModel.proposalAccepting)
        } else if appModel.planningAgent != nil {
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
    private var content: some View {
        if let proposal = appModel.activeProposal {
            NavProse {
                NavHeading(text: ProposalText.counts(milestones: proposal.milestoneCount, slices: proposal.sliceCount))
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
                Text(appModel.proposalError ?? ProposalText.acceptCaption(name: appModel.proposalName))
                    .ink(appModel.proposalError == nil ? .secondary : .danger)
                Text("The proposed tree is drawn under this project in the sidebar.").ink(.secondary)
            }
        } else if appModel.planningAgent != nil {
            NavProse {
                Text("The planning agent is running in the terminal on the right. When it proposes a plan, the plan appears here to accept.")
                    .ink(.secondary)
                if let endError { Text(endError).ink(.danger) }
            }
        } else if appModel.workshopLaunching {
            NavProse { Text("Starting the workshop session\u{2026}").ink(.secondary) }
        } else {
            NavProse {
                Text("Describe the changes you want to make. You can list several and the agent will plan milestones and slices for them in one go.")
                    .ink(.secondary)
                TextEditor(text: $appModel.workshopDraft)
                    .font(Typo.mono(size: 13))
                    .scrollContentBackground(.hidden)
                    .padding(4)
                    .frame(minHeight: 180)
                    .overlay {
                        RoundedRectangle(cornerRadius: 4).strokeBorder(DesignTokens.rule(.border, on: .window), lineWidth: 1)
                    }
                if let error = appModel.workshopLaunchError { Text(error).ink(.danger) }
            }
        }
    }
}

/// The workshop's main pane: the planning agent's terminal.
struct WorkshopMainPane: View {
    @Bindable var appModel: AppModel

    var body: some View {
        VStack(spacing: 0) {
            MainPaneHeader { AgentModelHeading(agent: appModel.planningAgent) }
            AgentTerminalPane(
                agent: appModel.planningAgent,
                emptyText: appModel.workshopLaunching
                    ? "Starting the workshop session\u{2026}"
                    : "The planning agent's terminal opens here on launch.",
                focusRequest: appModel.terminalFocusRequest,
                sessionExists: { appModel.planningAgent != nil })
        }
        .surface(.window)
    }
}
