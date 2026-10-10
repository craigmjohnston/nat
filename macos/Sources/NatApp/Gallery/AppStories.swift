import SwiftUI
import NatKit
import NatFixtures

/// The stories the gallery ships with.
///
/// It lives in `NatApp` rather than in `NatKit` for the same reason the views
/// do: a story is a view, and the views are here. What it draws is entirely
/// `NatFixtures` — a pinned clock, a canned plan, a client that answers from
/// memory — so a run reaches no Notion, no `nat` and no tmux, and renders the
/// same pixels on any machine.
///
/// The catalog is the app's own shape: the window in each phase a slice goes
/// through (the gnat design's own states — todo, working, waiting, review,
/// pr, blocked, done, plus resumed), then the screens that are not a slice
/// (workshop, sessions, the Untitled starter, onboarding), then the sidebar
/// and the status bar on their own, then the smaller pieces and settings.
///
/// Two regions are drawn rather than run — the agent terminal and the
/// onboarding checklist; see `StorySeams`. Pulses are held still so a capture
/// never lands mid-fade.
@MainActor
enum AppStories {
    /// The design's own canvas: every metric in the shell was drawn at this.
    private static let window = CGSize(width: 1320, height: 820)

    /// The main pane on its own, at what the window leaves it.
    private static let pane = CGSize(width: 730, height: 760)

    /// The sidebar on its own, at its default width and the window's height.
    private static let sidebar = CGSize(width: 260, height: 820)

    /// The whole window, held still and with the terminal drawn.
    private static func shell(
        _ appModel: AppModel, folds: [String: Bool] = [:], focus: NavigatorFocus? = nil,
        containerFocus: ContainerFocus? = nil, sendBackOpen: Bool = false
    ) -> some View {
        WindowShellView(appModel: appModel, sidebarFolds: folds, focus: focus, containerFocus: containerFocus)
            .environment(\.terminalStubbed, true)
            .environment(\.pulsesPaused, true)
            .environment(\.sendBackOpen, sendBackOpen)
    }

    /// The board with the Work source project beside the two others, its
    /// card (or one of its tasks) selected and read — Projects folded so the
    /// source fold is in view.
    private static func sourceShell(
        container: String? = nil, task: String? = nil, containerFocus: ContainerFocus? = nil
    ) async -> some View {
        let appModel = await Fixtures.startedAppModel(config: Fixtures.sourceConfig)
        let projectID = Fixtures.sourceProjectID
        if let container {
            await appModel.selectContainer(container, inProject: projectID)
            await appModel.containerStore(projectID: projectID).fetch(containerID: container)
        }
        if let task {
            await appModel.selectSlice(task, inProject: projectID)
            await appModel.sliceDetailStore(projectID: projectID).fetch(sliceRef: task)
            let slice = Fixtures.sourceTasks.first { $0.id == task }
            if let slice, slice.handedBack || !(slice.branch ?? "").isEmpty {
                await appModel.diffStore(projectID: projectID).fetch(projectID: projectID, sliceRef: task)
            }
            if let slice, !slice.pr.isEmpty {
                await appModel.prStore(projectID: projectID).fetch(projectID: projectID, sliceRef: task)
            }
        }
        return shell(appModel, folds: ["work": true], containerFocus: containerFocus)
    }

    /// The sidebar alone over the Work source project — Projects folded
    /// unless `folded` says otherwise — the first card selected.
    /// Four projects — two tracked, each its own colour, and the Work source
    /// project and Scratch, which take none, every fold open — with Active
    /// rows across them.
    private static func projectColoursSidebar() async -> some View {
        var projects = Fixtures.sourceConfig.projects
        projects[Fixtures.scratchProjectID] = Fixtures.scratchConfig.projects[Fixtures.scratchProjectID]
        let config = NatProjectConfig(
            projects: projects, agentSplitPercent: 45, pollSeconds: 3600,
            assigneeUserName: "Craig Johnston", scratchProject: Fixtures.scratchProjectID)
        let appModel = await Fixtures.startedAppModel(config: config)
        appModel.selectedSliceID = Fixtures.mergeBoxSliceID
        return SidebarView(appModel: appModel, folded: ["scratch": false, "p:\(Fixtures.secondProjectID)": true])
            .environment(\.pulsesPaused, true)
    }

    /// A Shortcut card's project badge, as the stories draw it.
    static let storyCardBadge = Fixtures.sourceMobileApp

    /// Every project colour's badge, then the no-colour chip, then a Shortcut
    /// card's badge as it is drawn outside the Shortcut section — the logo,
    /// then its project — on the sidebar's ground.
    private static func projectBadgeRow() -> some View {
        HStack(spacing: 6) {
            ForEach(ProjectColor.allCases, id: \.self) { color in
                ProjectBadgeView(tag: String(color.rawValue.prefix(3)).uppercased(), color: color)
            }
            ProjectBadgeView(tag: "SCR", color: nil)
            CardMarkView(badge: storyCardBadge, icon: Fixtures.shortcutIcon)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(DesignTokens.fill(.header))
        .environment(\.ground, .header)
    }

    private static func sourceSidebar(
        client: FixtureNatClient = FixtureNatClient(), folded: [String: Bool] = ["work": true],
        hoveredContainer: String? = nil, hoveredGroup: String? = nil
    ) async -> some View {
        let appModel = await Fixtures.startedAppModel(client: client, config: Fixtures.sourceConfig)
        await appModel.selectContainer(Fixtures.sourceCardID, inProject: Fixtures.sourceProjectID)
        return SidebarView(
            appModel: appModel, folded: folded,
            hoveredContainer: hoveredContainer.map { (Fixtures.sourceProjectID, $0) },
            hoveredGroup: hoveredGroup.map { (Fixtures.sourceProjectID, $0) })
            .environment(\.pulsesPaused, true)
    }

    /// The filter editor as it opens from its button, alone — a real popover
    /// is a window of its own, which no render of the main one can show.
    private static func filterPopover(_ action: SourceAction?) -> some View {
        SourceFilterPopover(action: action ?? Fixtures.sourceFilterAction([:]), onCancel: {}, onApply: { _ in })
            .surface(.window)
    }

    /// The window on one slice, the fixture plan beside the second project's,
    /// with the live readings the fixtures carry, waiting for those readings
    /// to land so the slice is drawn in the state it is a story about.
    /// The PR view alone over a fixture pull request, with an entry's reply
    /// open (`replyTo`, its index in the conversation) or the description
    /// being edited.
    private static func prConversation(
        _ pr: PRDetail = Fixtures.prGreen, replyTo index: Int? = nil, replyText: String = "",
        editing: String? = nil
    ) async -> some View {
        let store = PRStore(client: FixtureNatClient(pr: pr))
        await store.fetch(projectID: Fixtures.projectID, sliceRef: Fixtures.approveSliceID)
        let entries = conversation(comments: pr.comments, reviews: pr.reviews)
        let reply = index.map { (key: entries[$0].replyKey, text: replyText) }
        return PRConversationPane(store: store, expectedNumber: pr.number, reply: reply, editingDescription: editing)
            .surface(.window)
    }

    private static func slicePane(
        _ sliceID: String, agents: [AgentStatus] = Fixtures.agentStatuses, plan: ProjectInfo = Fixtures.projectInfo,
        prStatus: PRStatusDoc = Fixtures.prStatusDoc, pr: PRDetail = Fixtures.prGreen,
        details: [String: SliceDetail] = Fixtures.sliceDetails, focus: NavigatorFocus? = nil,
        config: NatProjectConfig = Fixtures.twoProjectConfig, sendBackOpen: Bool = false,
        seen: SeenMemory = .inMemory(),
        configure: @MainActor (AppModel) async -> Void = { _ in }
    ) async -> some View {
        let appModel = await Fixtures.startedAppModel(
            client: FixtureNatClient(plan: plan, agents: agents, pr: pr, details: details, prStatus: prStatus),
            config: config, seenMemory: seen)
        appModel.selectedSliceID = sliceID
        for _ in 0..<50 where !agents.isEmpty && appModel.activityStore?.agents.isEmpty != false {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        await appModel.sliceDetailStore(projectID: Fixtures.projectID).fetch(sliceRef: sliceID)
        let store = appModel.diffStore(projectID: Fixtures.projectID)
        let slice = plan.slices.first { $0.id == sliceID } ?? Fixtures.slice(sliceID)
        // A resumed slice's branch is read by its agent branch, its Branch
        // cleared.
        if slice.handedBack || !(slice.branch ?? "").isEmpty || slice.resumed || slice.takenBack {
            await store.fetch(projectID: Fixtures.projectID, sliceRef: sliceID)
        }
        if !slice.pr.isEmpty {
            await appModel.prStore(projectID: Fixtures.projectID).fetch(projectID: Fixtures.projectID, sliceRef: sliceID)
        }
        await configure(appModel)
        return shell(appModel, focus: focus, sendBackOpen: sendBackOpen)
    }

    /// Two projects with pull request trouble: the first's approved slice red
    /// and conflicting, the second ("gnat", never activated) with one red and
    /// one conflicting — waited for until its background reading has landed.
    /// `prStatus` is the first project's reading.
    @MainActor
    private static func prMarksAppModel(
        prStatus: PRStatusDoc = Fixtures.prStatusChecksFailingAndConflicting,
        agents: [AgentStatus] = Fixtures.agentStatuses
    ) async -> AppModel {
        let appModel = await Fixtures.startedAppModel(
            client: FixtureNatClient(
                otherPlans: [Fixtures.secondProjectID: Fixtures.secondProjectInfoWithPRs], agents: agents,
                prStatus: prStatus,
                prStatusByProject: [Fixtures.secondProjectID: Fixtures.secondProjectPRStatus]),
            config: Fixtures.twoProjectConfig)
        for _ in 0..<100 where appModel.prStatusStore?.readings[Fixtures.secondProjectID] == nil {
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        return appModel
    }

    /// The status bar over an app whose GitHub readings carry `limit`.
    private static func githubBudgetStatusBar(_ limit: GitHubRateLimit) async -> some View {
        let client = FixtureNatClient(plan: statusBarPlan, agents: Fixtures.agentStatuses, usage: Fixtures.usageReading)
        client.setRateLimit(limit)
        return StatusBarView(appModel: await Fixtures.startedAppModel(client: client))
    }

    /// Settings ▸ About, Diagnostics open: launched 2h 14m before the
    /// gallery's clock, a throttled reading in, and a tally of readings and
    /// actions behind it.
    private static func aboutDiagnostics() async -> some View {
        let client = FixtureNatClient()
        client.setRateLimit(GitHubRateLimit(
            limit: 5000, remaining: 412, resetAt: Fixtures.now.addingTimeInterval(46 * 60),
            projectedRemainingAtReset: 120, throttled: true, pollAfterSeconds: 300, cost: 1))
        let launched = Fixtures.now.addingTimeInterval(-(2 * 3600 + 14 * 60))
        let appModel = await Fixtures.startedAppModel(client: client, now: { launched })
        for _ in 0..<3 { appModel.githubActionRan() }
        await appModel.githubReadingStore?.idle()
        for _ in 0..<4 { await appModel.githubReadingStore?.read() }
        return SettingsView(appModel: appModel, client: client, initialTab: .about, diagnosticsExpanded: true)
    }

    /// The size the PR section's Checks stories are drawn at: the
    /// navigator's width.
    private static let checksSize = CGSize(width: 330, height: 330)

    /// The PR section's body over `checks`, read through a `PRStore` on the
    /// fixture client — `act` run on the store before it is drawn.
    private static func checksSection(
        _ checks: [PRCheck], hovered: String? = nil,
        act: @MainActor (PRStore, FixtureNatClient) async -> Void = { _, _ in }
    ) async -> some View {
        let client = FixtureNatClient(pr: Fixtures.pr(checks: checks))
        let store = PRStore(client: client)
        await store.fetch(projectID: Fixtures.projectID, sliceRef: Fixtures.approveSliceID)
        await act(store, client)
        store.setVisible(false)
        return PRSectionBody(
            pr: store.loadState.pr ?? Fixtures.pr(checks: checks), checksStore: store, hoveredCheck: hovered)
            .frame(width: checksSize.width, height: checksSize.height, alignment: .top)
            .surface(.window)
    }

    /// Starts an action whose nat call the fixture client holds, and gives it
    /// long enough to reach that call — the state a story about an action in
    /// flight is drawn in.
    private static func startHeld(_ action: @escaping @MainActor () async -> Void) async {
        Task { @MainActor in await action() }
        try? await Task.sleep(nanoseconds: 50_000_000)
    }

    /// Selects the last row of Active, once the activity poll's first
    /// reading has filled it.
    private static func selectLastActiveRow(_ appModel: AppModel) async {
        for _ in 0..<50 where appModel.activityStore?.agents.isEmpty != false {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        guard let last = appModel.sidebarModel.active.last else { return }
        await appModel.selectSlice(last.targetID, inProject: last.projectID)
    }

    /// Once the activity poll has read the fixture's waiting agent, expects
    /// it working — the state just after a send, before nat's marker moves.
    private static func expectWaitingAgentWorking(_ appModel: AppModel) async {
        for _ in 0..<50 where appModel.activityStore?.agents[Fixtures.activitySliceID] == nil {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        appModel.activityStore?.expectWorking(Fixtures.activitySliceID)
    }

    /// Waits for the activity poll's first reading to report the active
    /// project's planning agent, so a workshop story is drawn with it live.
    private static func settleOnPlanner(_ appModel: AppModel) async {
        for _ in 0..<50 where appModel.planningAgent == nil {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
    }

    /// A project's workshop, its agent live, holding the fixture proposal —
    /// `accepting` with an Accept under way that never lands, `scrollTo` the
    /// Plan section's row for that proposed slice clicked.
    private static func projectProposalShell(
        _ proposal: PlanProposal = Fixtures.proposal, accepting: Bool, scrollTo: String? = nil,
        expandEdits: Set<String> = [], folded: Set<String> = []
    ) async -> some View {
        let client = FixtureNatClient(agents: Fixtures.agentStatusesWithPlanner)
        client.setProposal(proposal, forProject: Fixtures.projectID)
        let appModel = await Fixtures.startedAppModel(client: client)
        await settleOnPlanner(appModel)
        appModel.workshopSelected = true
        await appModel.refreshProposals()
        if let scrollTo { appModel.showProposedSlice(scrollTo) }
        appModel.expandedProposalEdits = expandEdits
        appModel.foldedProposedSlices = folded
        if accepting {
            client.holdAccepts()
            await startHeld { await appModel.acceptProposal() }
        }
        return shell(appModel)
    }

    /// The approved slice at its open pull request, the PR section open and
    /// its conversation up, sent back to its live agent once the pane is up —
    /// `AppModel.sendBack` as the editor's Send or the bar's Fix runs it, the plan then
    /// reading the slice resumed.
    private static func resumedFromPullRequest() async -> some View {
        let client = FixtureNatClient(
            plan: Fixtures.projectInfo, agents: Fixtures.approvedAgentStatuses,
            details: Fixtures.resumedVisualsSliceDetails)
        let appModel = await Fixtures.startedAppModel(client: client, config: Fixtures.twoProjectConfig)
        appModel.selectedSliceID = Fixtures.approveSliceID
        for _ in 0..<50 where appModel.activityStore?.agents.isEmpty != false {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        await appModel.prStore(projectID: Fixtures.projectID)
            .fetch(projectID: Fixtures.projectID, sliceRef: Fixtures.approveSliceID)
        let slice = Fixtures.slice(Fixtures.approveSliceID)
        Task {
            try? await Task.sleep(for: .milliseconds(200))
            client.setPlan(Fixtures.resumedProjectInfo)
            _ = await appModel.sendBack(slice: slice, note: "Fix the failing test.", model: nil, effort: nil)
        }
        return shell(appModel, focus: NavigatorFocus(open: [.pr], main: .pr))
    }

    /// A project's workshop that was running when the app last quit, drawn
    /// from the kept request while the activity poll's first reading never
    /// lands — the start kicked off, not awaited, since it waits on that
    /// reading's twin (the reaper's) too.
    private static func reconnectingWorkshop() async -> AppModel {
        let client = FixtureNatClient()
        client.holdStatus()
        let appModel = Fixtures.appModel(
            client: client,
            workshopCache: InMemoryWorkshopCache(WorkshopSnapshot(
                workshops: [Fixtures.projectID: .init(request: "Split the importer into a reader and a writer.")])))
        Task { await Fixtures.start(appModel) }
        for _ in 0..<50 where appModel.plan(projectID: Fixtures.projectID) == nil {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        appModel.workshopSelected = true
        return appModel
    }

    /// A project's workshop that was running at quit, whose agent the first
    /// activity reading finds gone with its plan still up: kept, Plan in
    /// front (`EndedWorkshop.keepPlan`).
    private static func endedWorkshop() async -> AppModel {
        let client = FixtureNatClient()
        client.setProposal(Fixtures.proposal, forProject: Fixtures.projectID)
        let appModel = Fixtures.appModel(
            client: client,
            workshopCache: InMemoryWorkshopCache(WorkshopSnapshot(
                workshops: [Fixtures.projectID: .init(request: "Split the importer into a reader and a writer.")])))
        await Fixtures.start(appModel)
        appModel.workshopSelected = true
        for _ in 0..<50 where !appModel.workshopEnded {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        return appModel
    }

    /// The sidebar over a project whose live workshop has a proposal up —
    /// `hovered` drawing its Active row under the pointer.
    private static func workshopPlanReadySidebar(hovered: Bool) async -> some View {
        let client = FixtureNatClient(agents: Fixtures.agentStatusesWithPlanner)
        client.setProposal(Fixtures.proposal, forProject: Fixtures.projectID)
        let appModel = await Fixtures.startedAppModel(client: client)
        await settleOnPlanner(appModel)
        await appModel.refreshProposals()
        return SidebarView(appModel: appModel, hoveredActiveRow: hovered ? Fixtures.projectID : nil)
            .environment(\.pulsesPaused, true)
    }

    /// The handed-back slice with its images handed in, the Visual changes
    /// section open and the image list up — `live` adding a waiting agent so
    /// there is one to send comments to.
    private static func visualsPane(live: Bool, seeded: Bool) async -> some View {
        let sliceID = Fixtures.mergeBoxSliceID
        let agents = Fixtures.agentStatuses + (live ? [AgentStatus(
            sliceID: sliceID, session: TmuxSession.name(forSlicePageID: sliceID), activity: .waiting)] : [])
        return await slicePane(
            sliceID, agents: agents, details: Fixtures.visualsSliceDetails,
            focus: NavigatorFocus(open: [.visuals], main: .visuals)
        ) { appModel in
            let store = appModel.visualStore(projectID: Fixtures.projectID)
            store.loader = Fixtures.visualImageLoader
            await store.load(sliceID: sliceID, visuals: Fixtures.visualChanges)
            if seeded {
                Fixtures.seedPendingVisualComments(into: store)
                store.toggleViewed(sliceID: sliceID, Fixtures.visualChanges[0])
            }
        }
    }

    /// The handed-back slice after a second hand-in, the first seen: the
    /// first render re-rendered, folded so its section has not been on screen
    /// — Updated on its row and its header — the second and third as they
    /// were, and a fourth render the first hand-in did not have: New, and so
    /// New on the section's header.
    private static func visualsNewPane() async -> some View {
        let sliceID = Fixtures.mergeBoxSliceID
        return await slicePane(
            sliceID, details: Fixtures.visualsWithNewsSliceDetails,
            focus: NavigatorFocus(open: [.visuals], main: .visuals)
        ) { appModel in
            let store = appModel.visualStore(projectID: Fixtures.projectID)
            store.loader = Fixtures.visualImageLoader
            await Fixtures.seedSeenVisuals(into: store)
            await store.load(sliceID: sliceID, visuals: Fixtures.visualChangesWithNews)
            store.toggleCollapsed(sliceID: sliceID, Fixtures.visualChangesWithNews[0])
        }
    }

    /// The fixture plan with a dozen more slices in flight — what a sidebar
    /// with more running than fits looks like.
    /// A titlebar band story's width: a 330pt navigator beside a 730pt
    /// main pane, the window less its sidebar.
    private static let bandWidth = GnatMetrics.navigatorWidth + 730

    private static let longBandTitle =
        "Rework the navigator and main pane titlebars into one band, tabs right-aligned, the title ellipsizing into the gap"

    /// A slice name long enough that the band's room runs out at an
    /// ordinary window width, short enough that each stage of giving way
    /// (`BreadcrumbFit`) is reached before the next.
    private static let fitBandTitle = "Persist an unlaunched workshop across relaunches"

    private static let bandAgent = AgentStatus(
        sliceID: Fixtures.diffPaneSliceID, session: "nat-1", activity: .working,
        model: "Sonnet 5", effort: "high", contextPercent: 42, contextTokens: 84_120)

    /// The titlebar band as the shell lays it out over a 330pt navigator:
    /// the breadcrumb, its last crumb a live `GNA` selection (or `identity`),
    /// then the tabs at the trailing edge.
    private static func band(
        tabs: [MainPaneTab], selected: MainPaneMode?, crumbs: TitlebarCrumbs, state: SliceDisplayState = .working,
        identity: TitlebarIdentity? = nil, hoveredTab: MainPaneTab? = nil, runs: Bool = false,
        runBusy: Bool = false, runHovered: Bool = false,
        projectColor: ProjectColor? = Fixtures.config.projects[Fixtures.projectID]?.color
    ) -> some View {
        TitlebarBand(
            navigatorWidth: GnatMetrics.navigatorWidth, tabs: tabs.map(\.titlebarTab),
            selected: tabs.first { $0.mode == selected }?.titlebarTab.id,
            hoveredTab: hoveredTab?.titlebarTab.id,
            trailing: runs
                ? AnyView(RunSplitButton(runs: Fixtures.runs.sliceRuns, isBusy: runBusy, menuOpen: .constant(false)) { _ in }
                    .frame(maxHeight: .infinity)
                    .transformEnvironment(\.hoverForced) { if runHovered { $0 = true } })
                : nil
        ) {
            TitlebarBreadcrumb(
                crumbs: crumbs,
                identity: identity ?? TitlebarIdentity(tag: "GNA", state: state, live: true, title: crumbs.title),
                projectColor: projectColor,
                openPicker: .constant(nil)
            ) { _ in EmptyView() }
        }
    }

    /// The sidebar's own titlebar segment over a project with runs — and,
    /// where `treeOpen`, the run tree drawn under the play button, since a
    /// real popover is a window of its own no render of this one can show.
    /// `running`: the project's Board run started and still live, so the
    /// tree greys it and the play button spins.
    private static func titlebarRun(treeOpen: Bool, running: Bool = false) async -> some View {
        let appModel = await Fixtures.startedAppModel(config: Fixtures.runsConfig)
        if running {
            appModel.runSessionExists = { _ in true }
            await appModel.startRun(projectID: Fixtures.projectID, label: "Board")
        }
        return VStack(alignment: .leading, spacing: 0) {
            SidebarView(appModel: appModel, showsTitlebar: true)
                .frame(width: 260, height: GnatMetrics.titlebarHeight, alignment: .top)
                .clipped()
                .environment(\.pulsesPaused, true)
            if treeOpen {
                RunTreePicker(
                    projects: appModel.runProjects, openProjectID: Fixtures.projectID,
                    isRunning: { appModel.isRunning(projectID: $0, sliceID: nil, label: $1) }
                ) { _, _ in }
                    .clipShape(RoundedRectangle(cornerRadius: 8))
                    .overlay(RoundedRectangle(cornerRadius: 8).stroke(DesignTokens.rule(.separator, on: .header), lineWidth: 1))
                    .padding(.top, 6)
                    .padding(.leading, 12)
            }
            Spacer(minLength: 0)
        }
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .surface(.window)
    }

    /// A slice's crumbs in the fixture project, under M2.
    private static func sliceCrumbs(_ title: String) -> TitlebarCrumbs {
        TitlebarCrumbs(project: Fixtures.project.name, parent: "M2: Review flow", title: title)
    }

    /// The handed-back slice's Changes section body alone, at the
    /// navigator's width, its branch and commits read.
    private static func changesSection() async -> some View {
        let appModel = await Fixtures.startedAppModel(
            client: FixtureNatClient(agents: []), config: Fixtures.twoProjectConfig)
        let slice = Fixtures.slice(Fixtures.mergeBoxSliceID)
        appModel.selectedSliceID = slice.id
        await appModel.diffStore(projectID: Fixtures.projectID).fetch(projectID: Fixtures.projectID, sliceRef: slice.id)
        return ChangesSectionBody(appModel: appModel, review: DiffReview(), slice: slice, reviewing: true) {}
            .surface(.window)
    }

    private static let crowdedPlan = ProjectInfo(
        project: Fixtures.project,
        milestones: Fixtures.milestones,
        slices: Fixtures.slices + (1...12).map { n in
            Slice(
                id: "f1x75222-0000-4000-8000-0000000000\(String(format: "%02d", n))",
                name: "Working slice \(n)",
                status: "In progress",
                milestoneID: "M2: Review flow",
                assignee: "Craig Johnston",
                pr: "",
                url: "",
                blocked: false,
                handedBack: false
            )
        }
    )

    /// The fixture plan with two of M2's slices marked Done, so the status
    /// bar has a started milestone alongside an untouched one.
    private static let statusBarPlan = ProjectInfo(
        project: Fixtures.project,
        milestones: Fixtures.milestones,
        slices: Fixtures.slices.map { slice in
            guard slice.id == Fixtures.diffPaneSliceID || slice.id == Fixtures.activitySliceID else {
                return slice
            }
            return Slice(
                id: slice.id, name: slice.name, status: "Done", milestoneID: slice.milestoneID,
                assignee: slice.assignee, pr: slice.pr, url: slice.url, branch: slice.branch, repo: slice.repo,
                dependsOn: slice.dependsOn, blocked: slice.blocked, handedBack: slice.handedBack)
        }
    )

    /// The status bar's plan (M2 partly done, M3 untouched) and an untouched
    /// M4 after it, for the milestones' default folds.
    private static let defaultFoldsPlan = ProjectInfo(
        project: Fixtures.project,
        milestones: Fixtures.milestones + [Milestone(id: "M4: Polish", name: "M4: Polish", order: 3, status: "Queued")],
        slices: statusBarPlan.slices + (1...2).map { n in
            Slice(
                id: "f1x75222-0000-4000-8000-0000000004\(String(format: "%02d", n))",
                name: "Polish pass \(n)", status: "Todo", milestoneID: "M4: Polish",
                assignee: "", pr: "", url: "", blocked: false, handedBack: false)
        }
    )

    /// A scratch plan: one slice added with no milestone (so under the
    /// unfiled one), one under a milestone of its own.
    private static let unfiledScratchPlan = ProjectInfo(
        project: Project(id: Fixtures.scratchProjectID, name: "Scratch", conventions: ""),
        milestones: [
            Milestone(id: "Spikes", name: "Spikes", order: 0, status: "Active"),
            Milestone(id: "Unfiled", name: "Unfiled", order: 1, status: "Queued", unfiled: true),
        ],
        slices: [
            Slice(id: "f1x7aaaa-0000-4000-8000-000000000001", name: "Try the new tmux hooks", status: "Todo",
                  milestoneID: "Unfiled", assignee: "", pr: "", url: "", blocked: false, handedBack: false),
            Slice(id: "f1x7aaaa-0000-4000-8000-000000000002", name: "Look at the release log", status: "Todo",
                  milestoneID: "Unfiled", assignee: "", pr: "", url: "", blocked: false, handedBack: false),
            Slice(id: "f1x7aaaa-0000-4000-8000-000000000003", name: "Profile the diff read", status: "Todo",
                  milestoneID: "Spikes", assignee: "", pr: "", url: "", blocked: false, handedBack: false),
        ])

    /// A scratch plan with two slices under way, one under no milestone and
    /// one under a milestone, and one still to do.
    private static let activeScratchPlan = ProjectInfo(
        project: Project(id: Fixtures.scratchProjectID, name: "Scratch", conventions: ""),
        milestones: unfiledScratchPlan.milestones,
        slices: [
            Slice(id: "f1x7aaaa-0000-4000-8000-000000000001", name: "Try the new tmux hooks", status: "In progress",
                  milestoneID: "Unfiled", assignee: "Craig Johnston", pr: "", url: "", blocked: false, handedBack: false),
            Slice(id: "f1x7aaaa-0000-4000-8000-000000000002", name: "Look at the release log", status: "Todo",
                  milestoneID: "Unfiled", assignee: "", pr: "", url: "", blocked: false, handedBack: false),
            Slice(id: "f1x7aaaa-0000-4000-8000-000000000003", name: "Profile the diff read", status: "In progress",
                  milestoneID: "Spikes", assignee: "Craig Johnston", pr: "", url: "", blocked: false, handedBack: false),
        ])

    /// The sidebar with Scratch folded over the given scratch plan: what
    /// Active makes of its work.
    private static func scratchActiveSidebar(_ scratchPlan: ProjectInfo) async -> some View {
        let appModel = await Fixtures.startedAppModel(
            client: scratchClient(scratchPlan), config: Fixtures.scratchConfigWithSecondProject)
        return SidebarView(appModel: appModel, folded: ["work": true])
            .environment(\.pulsesPaused, true)
    }

    /// The breadcrumb's tree picker opened on Scratch: its row last, its
    /// icon in the folder's place and its word, no badge.
    private static func scratchCrumbTreePicker() async -> some View {
        let appModel = await Fixtures.startedAppModel(
            client: scratchClient(activeScratchPlan), config: Fixtures.scratchConfigWithSecondProject)
        let tree = CrumbTree(model: appModel.sidebarModel, projectID: Fixtures.scratchProjectID, milestone: "Spikes")
        return CrumbTreePicker(tree: tree, onPick: { _ in })
            .environment(\.pulsesPaused, true)
    }

    /// The run tree with Scratch among the projects with runs, opened on it.
    private static func scratchRunTree() async -> some View {
        var projects = Fixtures.runsConfig.projects
        projects[Fixtures.scratchProjectID] = ProjectConfig(
            name: "Scratch", slicesDSID: "", workingDir: "/Users/craig",
            runs: [RunCommand(label: "Notes", command: "open ~/notes.md", scope: .global)])
        let config = NatProjectConfig(
            projects: projects, agentSplitPercent: 45, pollSeconds: 3600,
            assigneeUserName: "Craig Johnston", scratchProject: Fixtures.scratchProjectID)
        let appModel = await Fixtures.startedAppModel(client: scratchClient(activeScratchPlan), config: config)
        return RunTreePicker(
            projects: appModel.runProjects, openProjectID: Fixtures.scratchProjectID, isRunning: { _, _ in false }
        ) { _, _ in }
            .environment(\.pulsesPaused, true)
    }

    /// A scratch project with nothing in it.
    private static let emptyScratchPlan = ProjectInfo(
        project: Project(id: Fixtures.scratchProjectID, name: "Scratch", conventions: ""),
        milestones: [], slices: [])

    nonisolated private static func scratchClient(_ scratchPlan: ProjectInfo) -> FixtureNatClient {
        FixtureNatClient(otherPlans: [
            Fixtures.secondProjectID: Fixtures.secondProjectInfo, Fixtures.scratchProjectID: scratchPlan,
        ])
    }

    /// The breadcrumb's tree picker as a source task's container crumb opens
    /// it: projects, the source project's cards (no segment column), the
    /// card's tasks.
    private static func sourceCrumbTreePicker() async -> some View {
        let appModel = await Fixtures.startedAppModel(config: Fixtures.sourceConfig)
        let tree = CrumbTree(
            model: appModel.sidebarModel, projectID: Fixtures.sourceProjectID, container: Fixtures.sourceCardID)
        return CrumbTreePicker(tree: tree, onPick: { _ in })
            .environment(\.pulsesPaused, true)
    }

    /// The breadcrumb's tree picker, as a milestone crumb opens it.
    private static func crumbTreePicker() async -> some View {
        let appModel = await Fixtures.startedAppModel(
            client: FixtureNatClient(agents: Fixtures.agentStatuses), config: Fixtures.twoProjectConfig)
        for _ in 0..<50 where appModel.activityStore?.agents.isEmpty != false {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        let tree = CrumbTree(model: appModel.sidebarModel, projectID: Fixtures.projectID, milestone: "M2: Review flow")
        return CrumbTreePicker(tree: tree, onPick: { _ in })
            .environment(\.pulsesPaused, true)
    }

    /// The sidebar with Scratch open over the given scratch plan.
    private static func scratchSidebar(_ scratchPlan: ProjectInfo) async -> some View {
        let appModel = await Fixtures.startedAppModel(
            client: scratchClient(scratchPlan), config: Fixtures.scratchConfigWithSecondProject)
        return SidebarView(appModel: appModel, folded: ["scratch": false])
            .environment(\.pulsesPaused, true)
    }

    /// An app model on the activity slice with its follow-ups pending — its
    /// three, or as `detail` has them — and its agent waiting, the choices
    /// given (by batch, then by place in the batch) already made.
    private static func followUpsModel(
        detail: SliceDetail = Fixtures.followUpsSliceDetail, choices: [Int: [Int: FollowUpChoice]]
    ) async -> AppModel {
        let sliceID = Fixtures.activitySliceID
        let details = Fixtures.sliceDetails.merging([sliceID: detail]) { _, new in new }
        let appModel = await Fixtures.startedAppModel(
            client: FixtureNatClient(agents: Fixtures.agentStatuses, details: details),
            config: Fixtures.twoProjectConfig)
        appModel.selectedSliceID = sliceID
        await appModel.sliceDetailStore(projectID: Fixtures.projectID).fetch(sliceRef: sliceID)
        for _ in 0..<50 where appModel.activityStore?.agents[sliceID] == nil {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        for (batch, batchChoices) in choices {
            for (position, choice) in batchChoices {
                appModel.followUpStore.setChoice(choice, sliceID: sliceID, batch: batch, position: position)
            }
        }
        return appModel
    }

    static let catalog = StoryCatalog([

        // MARK: - The window, one slice in each phase

        // MARK: - The navigator's action bar

        Story(
            name: "action-bar-launch",
            summary: "A Todo slice: the bar at the navigator's foot holds Launch agent, primary; the Task header "
                + "carries nothing.",
            size: window
        ) {
            await slicePane(Fixtures.fixturesSliceID)
        },

        Story(
            name: "action-bar-launch-blocked",
            summary: "A Todo slice blocked on a dependency: Launch agent drawn disabled, the launch item "
                + "quietened above it.",
            size: window
        ) {
            await slicePane(Fixtures.cacheSliceID)
        },

        Story(
            name: "action-bar-approve",
            summary: "A handed-back slice with no comments: the bar reads Approve changes, primary; the Changes "
                + "header is empty.",
            size: window
        ) {
            await slicePane(Fixtures.mergeBoxSliceID)
        },

        Story(
            name: "action-bar-approve-with-comments",
            summary: "The same review with comments pending: the bar's button becomes Approve with comments, and "
                + "Send 2 comments sits in the Changes header, secondary.",
            size: window
        ) {
            await slicePane(Fixtures.mergeBoxSliceID) { appModel in
                Fixtures.seedPendingComments(into: appModel.diffStore(projectID: Fixtures.projectID))
            }
        },

        Story(
            name: "action-bar-merge",
            summary: "An approved slice with its agent still live from the hand-back: Merge PR alone in the bar, "
                + "primary — Send back to agent is the row menu's; Open in GitHub titled in the PR header.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: Fixtures.approvedAgentStatuses,
                focus: NavigatorFocus(open: [.pr], main: .pr))
        },

        Story(
            name: "action-bar-merge-no-agent",
            summary: "An approved slice with no agent and a clean pull request: Merge PR alone in the bar, primary "
                + "— no Launch, and no Send back to agent, which the row menu offers.",
            size: window
        ) {
            await slicePane(Fixtures.approveSliceID, agents: [], focus: NavigatorFocus(open: [.pr], main: .pr))
        },

        Story(
            name: "action-bar-merge-mergeability-unknown",
            summary: "An approved slice with no agent, its checks green and GitHub still working out "
                + "mergeability (UNKNOWN, BLOCKED on nothing else): Merge PR enabled — only what GitHub positively "
                + "says stands in the way greys it.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: [], pr: Fixtures.prMergeabilityUnknown,
                focus: NavigatorFocus(open: [.pr], main: .pr))
        },

        Story(
            name: "action-bar-split-menu",
            summary: "The action bar's Fix split button's menu, drawn on its own as a popover's content: the action "
                + "Fix took the place of — Approve changes — to press instead.",
            size: CGSize(width: 240, height: 60)
        ) {
            HeaderSplitMenuList(items: [
                HeaderSplitMenuList.Item(title: "Approve changes", systemImage: "checkmark") {},
            ])
            .surface(.window)
        },

        Story(
            name: "action-bar-fallback-launch",
            summary: "An agent still working with no branch yet: nothing to press, so the bar shows Launch agent "
                + "disabled, its tooltip saying the agent is still working.",
            size: window
        ) {
            await slicePane(Fixtures.diffPaneSliceID)
        },

        Story(
            name: "action-bar-fallback-approve",
            summary: "An agent still working on a recorded branch: Changes is the latest section, so the bar shows "
                + "Approve changes disabled, waiting on the agent's hand-back.",
            size: window
        ) {
            await slicePane(Fixtures.diffPaneSliceID, plan: Fixtures.branchedWorkingProjectInfo)
        },

        Story(
            name: "action-bar-fallback-merge",
            summary: "An agent live on a slice whose pull request reads merged on GitHub, not yet settled: no "
                + "Merge to offer, so the bar shows Merge PR disabled, the pull request no longer open.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: Fixtures.approvedAgentStatuses,
                pr: Fixtures.prGreenMergedOnGitHub, focus: NavigatorFocus(open: [.pr], main: .pr))
        },

        Story(
            name: "action-bar-completed",
            summary: "A merged slice: the bar holds no button, only the quiet Task completed.",
            size: window
        ) {
            await slicePane(Fixtures.shellSliceID)
        },

        Story(
            name: "window-review",
            summary: "A handed-back slice: Changes open, Approve changes in the action bar, the continuous diff in the main pane.",
            size: window
        ) {
            await slicePane(Fixtures.mergeBoxSliceID)
        },

        Story(
            name: "window-review-conflicting",
            summary: "A handed-back slice with no pull request whose branch nat tested conflicting with origin/main, no agent on it: the conflict mark on its Active and tree rows, and a Conflict badge (the same merge glyph) on the Changes header whose tooltip names origin/main and says to send it back to the agent to rebase; the bar's primary is Resolve conflicts, a split button with Approve changes behind its chevron.",
            size: window
        ) {
            await slicePane(Fixtures.mergeBoxSliceID, agents: [], prStatus: Fixtures.prStatusBranchConflicting)
        },

        Story(
            name: "window-review-comments",
            summary: "The same review with comments pending: the count on Send, secondary in the Changes header, Approve with comments in the bar, the dot on the file row, the inline cards in the diff.",
            size: window
        ) {
            await slicePane(Fixtures.mergeBoxSliceID) { appModel in
                Fixtures.seedPendingComments(into: appModel.diffStore(projectID: Fixtures.projectID))
            }
        },

        Story(
            name: "window-review-expanded",
            summary: "The review with its first file opened out: the gap above its first change revealed whole, "
                + "the next twenty lines down from it revealed and the rest still a gap with its controls.",
            size: window
        ) {
            await slicePane(Fixtures.mergeBoxSliceID) { appModel in
                let store = appModel.diffStore(projectID: Fixtures.projectID)
                guard let file = store.loadState.diff?.files.first else { return }
                let gaps = file.rows.compactMap(\.gap)
                if let top = gaps.first { await store.expand(path: file.path, gap: top, control: .all) }
                if gaps.count > 1 { await store.expand(path: file.path, gap: gaps[1], control: .down) }
            }
        },

        Story(
            name: "window-visuals",
            summary: "A review whose agent handed in images: Visual changes open with a thumbnail per image, "
                + "the image list up at 100% under the first one's pinned header, the third a placeholder card.",
            size: window
        ) {
            await visualsPane(live: false, seeded: false)
        },

        Story(
            name: "window-visuals-comments",
            summary: "The same with a live agent and comments pending, the first image marked viewed — ticked "
                + "in its row and header, folded in the pane — the whole-image comment on the second, and Send 2 comments.",
            size: window
        ) {
            await visualsPane(live: true, seeded: true)
        },

        Story(
            name: "window-visuals-new",
            summary: "A second hand-in: the first render re-rendered at the same path and not yet seen wears New "
                + "on its row, on its (folded) header and on the Visual changes header; the second, re-rendered "
                + "but already seen, and the unchanged third wear none.",
            size: window
        ) {
            await visualsNewPane()
        },

        Story(
            name: "visuals-pair",
            summary: "A pair as one image: before left of the divider, after right of it, the divider at the "
                + "middle with its handle, neither Before nor After selected, a pin on the after's pixels.",
            size: pane
        ) {
            await VisualsPaneStory.pair(Fixtures.visualPair)
        },

        Story(
            name: "visuals-pair-highlight",
            summary: "The same pair with Highlight differences on: the pixels that differ — the taller card, the "
                + "dot gone green — tinted over both sides.",
            size: pane
        ) {
            await VisualsPaneStory.pair(Fixtures.visualPair, highlight: true)
        },

        Story(
            name: "visuals-pair-before",
            summary: "The same pair toggled to Before: the divider at the far right, the before whole, Before "
                + "selected.",
            size: pane
        ) {
            await VisualsPaneStory.pair(Fixtures.visualPair, show: .before)
        },

        Story(
            name: "visuals-pair-size-mismatch",
            summary: "A pair whose before is smaller than its after: both from the top-leading corner in a frame "
                + "the larger of each, and Highlight differences disabled, its tooltip saying why.",
            size: pane
        ) {
            await VisualsPaneStory.pair(Fixtures.visualPairMismatched)
        },

        Story(
            name: "visuals-zoomed",
            summary: "The image list alone, tall: the first image at 200% scrolled to its middle, its pin still on "
                + "its spot, the second still fitted at 100%.",
            size: CGSize(width: pane.width, height: 1500)
        ) {
            await VisualsPaneStory.make(zoomFirst: 2, draft: nil)
        },

        Story(
            name: "visuals-comment-editor",
            summary: "The image list alone with the comment box open where the first image was clicked, "
                + "under a pending pin.",
            size: pane
        ) {
            await VisualsPaneStory.make(zoomFirst: 1, draft: CGPoint(x: 900, y: 180))
        },

        Story(
            name: "visuals-comment-editor-image-foot",
            summary: "The comment box open at a point near the first image's bottom edge: floated over the pane, "
                + "it runs past the image's bottom border unclipped, the pin uncovered above it.",
            size: pane
        ) {
            await VisualsPaneStory.make(zoomFirst: 1, draft: CGPoint(x: 700, y: 860))
        },

        Story(
            name: "visuals-comment-editor-zoomed-edge",
            summary: "The first image at 200% scrolled to its right end, the comment box open near that edge: "
                + "clamped to the pane's right inset, the pin visible above it.",
            size: pane
        ) {
            await VisualsPaneStory.make(zoomFirst: 2, draft: CGPoint(x: 1420, y: 300), anchor: .trailing)
        },

        Story(
            name: "visuals-comment-editor-pane-foot",
            summary: "The comment box open at a point on the second image, low in the pane where below would run "
                + "off it: the box sits above the pin, wholly on screen.",
            size: pane
        ) {
            await VisualsPaneStory.make(zoomFirst: 1, draft: CGPoint(x: 400, y: 80), on: 1)
        },

        Story(
            name: "diff-comment-button",
            summary: "A marked line in the diff: the comment button laid over the end of the line on a face of its own, the code under it unwrapped.",
            size: CGSize(width: 520, height: 260)
        ) {
            DiffCommentButtonStory()
        },

        Story(
            name: "diff-stress",
            summary: "A 300-file, 51,000-row diff jumped to its 150th file: the header pinned exactly at the top, long lines wrapped, tabs and wide characters on the grid.",
            size: CGSize(width: 900, height: 640)
        ) {
            DiffStressStory(wrap: true)
        },

        Story(
            name: "diff-folds",
            summary: "The review's diff with its first two files folded over each other, the third open under them and the last folded under it: one rule between any two headers, and between the last folded header and the closing line.",
            size: CGSize(width: 900, height: 640)
        ) {
            DiffFoldsStory()
        },

        Story(
            name: "diff-stress-unwrapped",
            summary: "The same diff with View ▸ Wrap lines in diffs off: every row one line, the long ones running past the pane's edge.",
            size: CGSize(width: 900, height: 640)
        ) {
            DiffStressStory(wrap: false)
        },

        Story(
            name: "window-todo",
            summary: "A Todo slice: the Thread open on its brief, cut short with Show more, Launch primary in its header, the main pane's note.",
            size: window
        ) {
            await slicePane(Fixtures.fixturesSliceID)
        },

        Story(
            name: "window-blocked",
            summary: "A blocked slice: its dot hollow and dim, Launch disabled, and the Thread's launch item quietened, its chips disabled, over what it waits on.",
            size: window
        ) {
            await slicePane(Fixtures.cacheSliceID)
        },

        Story(
            name: "window-blocked-several",
            summary: "A slice blocked on two slices with a third already done: the brief's depends list one row apiece, each with its dot, and the launch item naming the two still open.",
            size: window
        ) {
            let plan = ProjectInfo(
                project: Fixtures.project, milestones: Fixtures.milestones,
                slices: Fixtures.slices.map { slice in
                    guard slice.id == Fixtures.cacheSliceID else { return slice }
                    return Slice(
                        id: slice.id, name: slice.name, status: slice.status, milestoneID: slice.milestoneID,
                        assignee: slice.assignee, pr: slice.pr, url: slice.url,
                        dependsOn: [Fixtures.commentsSliceID, Fixtures.fixturesSliceID, Fixtures.shellSliceID],
                        blocked: true, handedBack: false)
                })
            let appModel = await Fixtures.startedAppModel(
                client: FixtureNatClient(plan: plan), config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = Fixtures.cacheSliceID
            await appModel.sliceDetailStore(projectID: Fixtures.projectID).fetch(sliceRef: Fixtures.cacheSliceID)
            return shell(appModel)
        },

        Story(
            name: "window-relaunch",
            summary: "A slice in progress whose agent is gone: the Thread's log, then the launch item, the header offering Relaunch.",
            size: window
        ) {
            await slicePane(Fixtures.diffPaneSliceID, agents: [])
        },

        Story(
            name: "window-working",
            summary: "A slice with its agent working: Thread open on its log, the terminal in the main pane.",
            size: window
        ) {
            await slicePane(Fixtures.diffPaneSliceID)
        },

        Story(
            name: "window-waiting",
            summary: "A slice whose agent waits for the user: hot in Active and in the Thread.",
            size: window
        ) {
            await slicePane(Fixtures.activitySliceID)
        },

        Story(
            name: "window-pr",
            summary: "An approved slice: PR open on its checks and review, Open in GitHub in the header, Merge PR in the action bar, its description and conversation in the main pane.",
            size: window
        ) {
            await slicePane(Fixtures.approveSliceID)
        },

        Story(
            name: "window-resumed",
            summary: "An approved slice sent back to its agent, read off the record (nat's resumed): the Task log ending on Work resumed and the live agent, the terminal up, its Active row working and pulsing; no Approve or Merge in the bar.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: Fixtures.approvedAgentStatuses, plan: Fixtures.resumedProjectInfo,
                details: Fixtures.resumedVisualsSliceDetails
            ) { appModel in
                appModel.visualStore(projectID: Fixtures.projectID).loader = Fixtures.visualImageLoader
            }
        },

        Story(
            name: "window-resumed-notices",
            summary: "The resumed slice with Changes, Visual changes and PR open and the diff up: each section's header wears Reworking (its tooltip the full warning), and the main pane over the diff says the agent is working on this again.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: Fixtures.approvedAgentStatuses, plan: Fixtures.resumedProjectInfo,
                details: Fixtures.resumedVisualsSliceDetails,
                focus: NavigatorFocus(open: [.changes, .visuals, .pr], main: .diff)
            ) { appModel in
                appModel.visualStore(projectID: Fixtures.projectID).loader = Fixtures.visualImageLoader
            }
        },

        Story(
            name: "window-resumed-pr-open",
            summary: "The approved slice at its open pull request, the PR section open and the conversation up, then sent back to its live agent (slice-resume, agent-send, the plan re-read resumed): the navigator lands on Task with the terminal up, and the PR section stays — folded, wearing Reworking — with its PR tab in the titlebar; no Merge in the bar.",
            size: window
        ) {
            await resumedFromPullRequest()
        },

        Story(
            name: "window-taken-back",
            summary: "A review sent back to its agent before any pull request (nat's taken_back): working again, Changes kept open on the diff, its header wearing Reworking and the main pane's banner saying the agent is working on this again; no PR section.",
            size: window
        ) {
            await slicePane(
                Fixtures.mergeBoxSliceID, plan: Fixtures.takenBackProjectInfo,
                focus: NavigatorFocus(open: [.changes], main: .diff))
        },

        Story(
            name: "window-resumed-badges",
            summary: "The resumed slice after its agent pushed, Changes and Visual changes open, the terminal up: a file the user had not seen New and one changed since Updated in the Changes list, the re-rendered image Updated and the added one New, Reworking then New on both headers; the folded PR section's header Reworking then Updated, its head moved.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: Fixtures.approvedAgentStatuses, plan: Fixtures.resumedProjectInfo,
                details: Fixtures.resumedVisualsSliceDetails, focus: NavigatorFocus(open: [.changes, .visuals], main: .terminal),
                seen: Fixtures.seenBeforeResume()
            ) { appModel in
                appModel.visualStore(projectID: Fixtures.projectID).loader = Fixtures.visualImageLoader
            }
        },

        Story(
            name: "window-pr-updated",
            summary: "An approved slice whose pull request's head has moved since the user last opened its PR section, the Task log up: the PR section's header wears Updated until it is opened.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: Fixtures.approvedAgentStatuses,
                focus: NavigatorFocus(open: [.thread], main: .terminal), seen: Fixtures.seenBeforeResume())
        },

        Story(
            name: "window-task-log-resumed",
            summary: "An approved slice sent back and handed back again: its Task log reads the hand-back, Work resumed with why as its body and its time, then the new hand-back, then the approve.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: [], details: Fixtures.resumedHandedBackSliceDetails,
                focus: NavigatorFocus(open: [.thread], main: .pr))
        },

        Story(
            name: "window-pr-send-back",
            summary: "An approved slice with no agent, the row menu's Send back to agent… picked: its editor over the action bar, the field empty for the user's own note, saying an agent will be launched; the bar under it still Merge PR alone.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: [], focus: NavigatorFocus(open: [.thread], main: .pr),
                sendBackOpen: true)
        },

        Story(
            name: "window-review-send-back",
            summary: "A handed-back slice in review with its agent live, the row menu's Send back to agent… picked: the editor over the bar, empty, saying the agent is told at once; Approve changes under it.",
            size: window
        ) {
            await slicePane(
                Fixtures.mergeBoxSliceID,
                agents: Fixtures.agentStatuses + [
                    AgentStatus(
                        sliceID: Fixtures.mergeBoxSliceID,
                        session: TmuxSession.name(forSlicePageID: Fixtures.mergeBoxSliceID), activity: .waiting),
                ],
                sendBackOpen: true)
        },

        Story(
            name: "window-pr-checks-failing",
            summary: "An approved slice whose pull request reads checks failing, no agent on it, its PR section open: a danger icon on the PR header whose tooltip names the check; no notice in the PR body, and the bar's primary is Fix failing checks in Merge's place — no split, GitHub refusing a merge over a failing check.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: [], prStatus: Fixtures.prStatusChecksFailing, pr: Fixtures.prFailingChecks,
                details: Fixtures.checksFailedSliceDetails, focus: NavigatorFocus(open: [.pr], main: .pr))
        },

        Story(
            name: "window-pr-checks-agent-told",
            summary: "The same red pull request with its agent live and the nudge on record: the PR header's danger icon, whose tooltip says the failing check was sent to the agent to fix; no notice over the Task log, whose item reads Checks failed and ends its body Sent to the agent to fix.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: Fixtures.approvedAgentStatuses,
                prStatus: Fixtures.prStatusChecksFailing, pr: Fixtures.prFailingChecks, details: Fixtures.checksNudgedSliceDetails)
        },

        Story(
            name: "window-pr-checks-fixing",
            summary: "The red pull request's slice resumed on the nudge, its agent's fix pushed and the checks running again, not yet handed back: the PR header keeps the danger icon (sent to the agent to fix) and its sidebar rows the checks' danger mark, not the running mark.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: Fixtures.approvedAgentStatuses, plan: Fixtures.fixingChecksProjectInfo,
                prStatus: Fixtures.prStatusChecksRunning, pr: Fixtures.prChecksRunning,
                details: Fixtures.checksNudgedSliceDetails, focus: NavigatorFocus(open: [.pr], main: .pr))
        },

        Story(
            name: "window-pr-checks-controls",
            summary: "An approved slice's PR section open over checks in every state — passed, failed, running, queued, and one Vercel reported — each row a sidebar task row's height: the heading's re-run and cancel over a checklist at the trailing edge, the rows' own pair hidden until the pointer is on one.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: [], pr: Fixtures.pr(checks: Fixtures.mixedChecks),
                focus: NavigatorFocus(open: [.pr], main: .pr))
        },

        Story(
            name: "pr-checks-controls",
            summary: "The Checks block alone, no row under the pointer: rows a sidebar task row's height, their name at its size, the outcome glyph in the tree's dot column; no row's re-run or cancel drawn, the heading's both enabled.",
            size: checksSize
        ) {
            await checksSection(Fixtures.mixedChecks)
        },

        Story(
            name: "pr-checks-row-hovered",
            summary: "The running macOS check's row under the pointer: the row wash runs full bleed to the section's edges, its re-run and cancel — both enabled — shown in the heading's columns, the other rows' still hidden.",
            size: checksSize
        ) {
            await checksSection(Fixtures.mixedChecks, hovered: "macOS App CI / test")
        },

        Story(
            name: "pr-checks-nothing-run",
            summary: "Every Actions job still queued: the heading's re-run disabled (nothing has run), its cancel enabled; the hovered test row's re-run disabled and cancel enabled.",
            size: checksSize
        ) {
            await checksSection(Fixtures.queuedChecks, hovered: "CI / test")
        },

        Story(
            name: "pr-checks-mid-call",
            summary: "A re-run of the running macOS check under way, the pointer gone from its row: its re-run still shown as a spinner, its cancel with it, disabled; every other row's pair hidden.",
            size: checksSize
        ) {
            await checksSection(Fixtures.mixedChecks) { store, client in
                client.holdChecksActions()
                await startHeld { await store.rerunChecks(.checks(["macOS App CI / test"]), from: .rerun("macOS App CI / test")) }
            }
        },

        Story(
            name: "pr-checks-cancelled-then-reran",
            summary: "After re-running the running macOS check: the section's notice says nat cancelled it and its queued sibling first, then re-ran both.",
            size: checksSize
        ) {
            await checksSection(Fixtures.mixedChecks) { store, _ in
                await store.rerunChecks(.checks(["macOS App CI / test"]), from: .rerun("macOS App CI / test"))
            }
        },

        Story(
            name: "window-task-log-checks-failed",
            summary: "An approved slice's Task log with a Checks failed entry: its own glyph in danger, the failed check and its run; no notice over the log, and the folded PR header carries the danger icon.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: [], prStatus: Fixtures.prStatusChecksFailing, pr: Fixtures.prFailingChecks,
                details: Fixtures.checksFailedSliceDetails, focus: NavigatorFocus(open: [.thread], main: .pr))
        },

        Story(
            name: "sidebar-checks-failing",
            summary: "The sidebar with the approved slice's pull request read checks failing: a danger mark on its Active row, the check named under the pointer.",
            size: sidebar
        ) {
            let appModel = await Fixtures.startedAppModel(
                client: FixtureNatClient(prStatus: Fixtures.prStatusChecksFailing), config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = Fixtures.approveSliceID
            return SidebarView(appModel: appModel).environment(\.pulsesPaused, true)
        },

        Story(
            name: "sidebar-pr-marks",
            summary: "Pull request marks across two projects, the second never opened: the approved slice's Active row carries both the checks' danger mark and the conflict mark, its tree row the conflict mark alone; in gnat, one slice's Active row carries the checks mark (its tree row none) and another's rows the conflict mark alone.",
            size: sidebar
        ) {
            let appModel = await prMarksAppModel()
            let open: [String: Bool] = [
                "p:\(Fixtures.projectID)": false, "p:\(Fixtures.secondProjectID)": false,
                "m:\(Fixtures.projectID)/M2: Review flow": false, "m:\(Fixtures.secondProjectID)/Detail overhaul": false,
                "m:\(Fixtures.projectID)/~sessions": true,
            ]
            return SidebarView(appModel: appModel, folded: open).environment(\.pulsesPaused, true)
        },

        Story(
            name: "sidebar-pr-marks-passing",
            summary: "As sidebar-pr-marks, but the approved slice's pull request reads mergeable with every check passed: its Active row carries the green passing mark in the danger mark's slot, its tree row no checks mark; gnat's red and conflicting rows are as before.",
            size: sidebar
        ) {
            let appModel = await prMarksAppModel(prStatus: Fixtures.prStatusChecksPassing)
            let open: [String: Bool] = [
                "p:\(Fixtures.projectID)": false, "p:\(Fixtures.secondProjectID)": false,
                "m:\(Fixtures.projectID)/M2: Review flow": false, "m:\(Fixtures.secondProjectID)/Detail overhaul": false,
                "m:\(Fixtures.projectID)/~sessions": true,
            ]
            return SidebarView(appModel: appModel, folded: open).environment(\.pulsesPaused, true)
        },

        Story(
            name: "sidebar-pr-marks-passing-agent-left",
            summary: "As sidebar-pr-marks-passing, with the agent left in the approved slice's pane after its hand-back — which reads as working: its Active row still carries the green passing mark.",
            size: sidebar
        ) {
            let appModel = await prMarksAppModel(
                prStatus: Fixtures.prStatusChecksPassing, agents: Fixtures.approvedAgentStatuses)
            let open: [String: Bool] = [
                "p:\(Fixtures.projectID)": false, "p:\(Fixtures.secondProjectID)": false,
                "m:\(Fixtures.projectID)/M2: Review flow": false, "m:\(Fixtures.secondProjectID)/Detail overhaul": false,
                "m:\(Fixtures.projectID)/~sessions": true,
            ]
            return SidebarView(appModel: appModel, folded: open).environment(\.pulsesPaused, true)
        },

        Story(
            name: "window-pr-checks-passing",
            summary: "An approved slice whose pull request reads mergeable with every check passed, no agent on it, its PR section open: a green passing mark on the PR header where the danger icon would be.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: [], prStatus: Fixtures.prStatusChecksPassing, pr: Fixtures.prGreen,
                focus: NavigatorFocus(open: [.pr], main: .pr))
        },

        Story(
            name: "window-pr-checks-stale-detail",
            summary: "As window-pr-checks-passing, but the pull request's own view was last read with its checks still going (one passed, one running, one queued, one skipped): the Checks list follows the batched reading the green header mark comes from, every check passed and the skipped one struck through.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: [], prStatus: Fixtures.prStatusChecksPassing, pr: Fixtures.prChecksRunning,
                focus: NavigatorFocus(open: [.pr], main: .pr))
        },

        Story(
            name: "sidebar-pr-marks-running",
            summary: "As sidebar-pr-marks, but the approved slice's pull request has its checks still running: its Active row carries the neutral in-progress mark (an ellipsis circle) in the checks' slot, its tree row no checks mark; gnat's red and conflicting rows are as before.",
            size: sidebar
        ) {
            let appModel = await prMarksAppModel(prStatus: Fixtures.prStatusChecksRunning)
            let open: [String: Bool] = [
                "p:\(Fixtures.projectID)": false, "p:\(Fixtures.secondProjectID)": false,
                "m:\(Fixtures.projectID)/M2: Review flow": false, "m:\(Fixtures.secondProjectID)/Detail overhaul": false,
                "m:\(Fixtures.projectID)/~sessions": true,
            ]
            return SidebarView(appModel: appModel, folded: open).environment(\.pulsesPaused, true)
        },

        Story(
            name: "window-pr-checks-running",
            summary: "An approved slice whose pull request's checks are still running, no agent on it, its PR section open: the neutral in-progress mark (an ellipsis circle) on the PR header where the passing mark would be; the running and queued checks lead with the same mark, the skipped one with a slashed circle, faded and struck through.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: [], prStatus: Fixtures.prStatusChecksRunning, pr: Fixtures.prChecksRunning,
                focus: NavigatorFocus(open: [.pr], main: .pr))
        },

        Story(
            name: "sidebar-milestone-hovered",
            summary: "A milestone row under the pointer: washed as a project or slice row is, its folder given way to the fold chevron, as a project row's does, and its count given way to its three-dot button in the same slot.",
            size: sidebar
        ) {
            let appModel = await prMarksAppModel()
            let key = "m:\(Fixtures.secondProjectID)/Detail overhaul"
            return SidebarView(
                appModel: appModel, folded: ["p:\(Fixtures.secondProjectID)": false, key: false], hoveredMilestone: key
            ).environment(\.pulsesPaused, true)
        },

        Story(
            name: "window-pr-conflicting",
            summary: "An approved slice whose pull request conflicts with main, no agent on it, its PR section open: a Conflict badge (the merge glyph) on the PR header whose tooltip says the branch conflicts with main and to send it back to the agent; nothing in the PR body; the bar's primary is Resolve conflicts in Merge's place, no split — GitHub refuses a conflicting merge.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: [], prStatus: Fixtures.prStatusConflicting, pr: Fixtures.prConflicting,
                focus: NavigatorFocus(open: [.pr], main: .pr))
        },

        Story(
            name: "window-pr-conflicting-checks-failing",
            summary: "The same pull request conflicting and red at once: the PR header wears the Conflict badge and the failing checks' danger icon, each with its own tooltip; the PR body shows its failed checks; the bar's one button reads Fix checks and conflicts.",
            size: window
        ) {
            await slicePane(
                Fixtures.approveSliceID, agents: [], prStatus: Fixtures.prStatusChecksFailingAndConflicting,
                pr: Fixtures.prFailingChecksConflicting, details: Fixtures.checksFailedSliceDetails,
                focus: NavigatorFocus(open: [.pr], main: .pr))
        },

        Story(
            name: "window-done",
            summary: "A Done slice: struck through in the tree, its PR section open on the merged pull request, Merged beside the PR heading.",
            size: window
        ) {
            await slicePane(Fixtures.shellSliceID, agents: [])
        },

        Story(
            name: "window-done-closed",
            summary: "A slice closed straight to Done with no branch: the Thread ends Closed with "
                + "the agent's summary cut short with Show more, and Changes and PR stay greyed out.",
            size: window
        ) {
            let id = "f1x75111-0000-4000-8000-0000000000c1"
            let closed = Slice(
                id: id, name: "Survey how other boards draw a review", status: "Done",
                milestoneID: "M2: Review flow", assignee: "Craig Johnston", pr: "", url: "",
                blocked: false, handedBack: false)
            let plan = ProjectInfo(
                project: Fixtures.project, milestones: Fixtures.milestones, slices: Fixtures.slices + [closed])
            var details = Fixtures.sliceDetails
            details[id] = SliceDetail(
                id: id, name: closed.name, url: "", status: "Done", milestone: "M2: Review flow",
                assignee: "Craig Johnston", branch: nil, repo: nil, pr: nil, dependsOn: nil, blocked: false,
                handedBack: false, state: nil,
                brief: "Look at how three other review tools lay out a pull request.\n\n"
                    + "### Summary\n\nNo code to change. Wrote the comparison up on the milestone's page; "
                    + "the merge box should lead with the worst verdict. All three put the checks above the "
                    + "conversation and none of them repeat the review decision in the button itself, which "
                    + "is the one place ours currently disagrees.")
            let appModel = await Fixtures.startedAppModel(
                client: FixtureNatClient(plan: plan, agents: [], details: details), config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = id
            await appModel.sliceDetailStore(projectID: Fixtures.projectID).fetch(sliceRef: id)
            return shell(appModel)
        },

        Story(
            name: "window-brief-summary",
            summary: "A Todo slice whose brief opens on a summary paragraph: the Brief card shows that "
                + "sentence alone, the detail behind Show more.",
            size: window
        ) {
            let id = "f1x75111-0000-4000-8000-0000000000c2"
            let todo = Slice(
                id: id, name: "Make shift+enter insert a newline", status: "Todo",
                milestoneID: "M2: Review flow", assignee: "", pr: "", url: "",
                blocked: false, handedBack: false)
            let plan = ProjectInfo(
                project: Fixtures.project, milestones: Fixtures.milestones, slices: Fixtures.slices + [todo])
            var details = Fixtures.sliceDetails
            details[id] = SliceDetail(
                id: id, name: todo.name, url: "", status: "Todo", milestone: "M2: Review flow",
                assignee: "", branch: nil, repo: nil, pr: nil, dependsOn: nil, blocked: false,
                handedBack: false, state: nil,
                brief: "Shift+enter in the agent terminal starts a new line instead of sending the message.\n\n"
                    + "- What is settled: plain enter still sends; shift+enter and option+enter both insert "
                    + "a newline, as they do in Claude Code's own terminal.\n"
                    + "- Out of scope: the workshop's brief editor.\n"
                    + "- Where to look: probably the terminal's key handling.\n\n"
                    + "Done when: typing shift+enter in a slice's terminal moves to a new line and sends nothing.")
            let appModel = await Fixtures.startedAppModel(
                client: FixtureNatClient(plan: plan, agents: [], details: details), config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = id
            await appModel.sliceDetailStore(projectID: Fixtures.projectID).fetch(sliceRef: id)
            return shell(appModel)
        },

        Story(
            name: "window-pr-long-description",
            summary: "A pull request whose description runs long: cut to three times the brief's length, with Show more.",
            size: window
        ) {
            let green = Fixtures.prGreen
            let pr = PRDetail(
                number: green.number, title: green.title,
                body: green.body + "\n\n" + String(repeating: "The heading is read off the same verdicts the rows are, so the two can never tell a different story about whether this merges. ", count: 4),
                state: green.state, isDraft: green.isDraft, author: green.author, baseRefName: green.baseRefName,
                headRefName: green.headRefName, url: green.url, checks: green.checks, reviews: green.reviews,
                comments: green.comments, reviewDecision: green.reviewDecision, mergeable: green.mergeable,
                mergeStateStatus: green.mergeStateStatus, additions: green.additions, deletions: green.deletions,
                changedFiles: green.changedFiles, commits: green.commits)
            let appModel = await Fixtures.startedAppModel(
                client: FixtureNatClient(pr: pr), config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = Fixtures.approveSliceID
            await appModel.prStore(projectID: Fixtures.projectID).fetch(projectID: Fixtures.projectID, sliceRef: Fixtures.approveSliceID)
            return shell(appModel)
        },

        Story(
            name: "window-done-hidden",
            summary: "View \u{25B8} Hide done items: no done slices under the milestones, no ended sessions and no Done folder.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel(config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = Fixtures.fixturesSliceID
            return shell(appModel).environment(\.showsDoneItems, false)
        },

        Story(
            name: "dependency-detail",
            summary: "The detail a depends row shows the moment the pointer is over it: the slice's whole name, where it stands, its milestone and pull request.",
            size: CGSize(width: 260, height: 170)
        ) {
            DependencyDetailView(
                slice: Fixtures.slice(Fixtures.approveSliceID), state: .pr, live: false, milestone: "M2: Review flow")
                .surface(.window)
        },

        Story(
            name: "window-followups",
            summary: "A waiting agent that proposed three follow-ups: their triage item last in the Thread, its dashed tail under it, Apply at its foot.",
            size: window
        ) {
            shell(await followUpsModel(choices: [1: [1: .queue, 2: .fold, 3: .drop]]))
        },

        Story(
            name: "window-followups-two-batches",
            summary: "Two batches of follow-ups pending at once, a note between them: each its own triage item in "
                + "its place in the log, with its own items, choices, Discard all and Apply — the first fully "
                + "decided and ready to apply, the second with one choice made.",
            size: CGSize(width: window.width, height: 1300)
        ) {
            shell(await followUpsModel(
                detail: Fixtures.twoBatchesSliceDetail,
                choices: [1: [1: .queue, 2: .fold, 3: .drop], 2: [1: .drop]]))
        },

        Story(
            name: "window-followups-decided-and-pending",
            summary: "A first batch decided (one queued, one folded in, one dismissed) drawn as its record, its "
                + "proposal and decisions folded into one group of four, then a second batch still pending as its "
                + "own triage item.",
            size: CGSize(width: window.width, height: 1000)
        ) {
            shell(await followUpsModel(detail: Fixtures.decidedAndPendingSliceDetail, choices: [:]))
        },

        Story(
            name: "window-light",
            summary: "The review window in the design's light palette.",
            size: window,
            colorScheme: .light
        ) {
            await slicePane(Fixtures.mergeBoxSliceID)
        },

        Story(
            name: "window-no-selection",
            summary: "A project with nothing selected: the navigator's note and the main pane's.",
            size: window
        ) {
            shell(await Fixtures.startedAppModel(config: Fixtures.twoProjectConfig))
        },

        Story(
            name: "window-crowded",
            summary: "A dozen slices in flight: Active grows and the Projects tree scrolls under it.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel(
                client: FixtureNatClient(plan: crowdedPlan, agents: Fixtures.agentStatuses),
                config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = Fixtures.mergeBoxSliceID
            return shell(appModel)
        },

        Story(
            name: "window-nothing-selected",
            summary: "Nothing selected: no breadcrumb, the navigator's note centred, and the main pane's quiet empty state.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel(
                client: FixtureNatClient(agents: Fixtures.agentStatuses), config: Fixtures.twoProjectConfig)
            return shell(appModel)
        },

        Story(
            name: "crumb-tree-picker",
            summary: "The breadcrumb's tree picker opened from a milestone crumb: projects, the project's milestones, the milestone's slices.",
            size: CGSize(width: 693, height: 320)
        ) {
            await crumbTreePicker()
        },

        Story(
            name: "crumb-tree-picker-source",
            summary: "The tree picker opened from a source task\u{2019}s card crumb: projects, the source "
                + "project\u{2019}s cards \u{2014} every card once, no segment column \u{2014} then the card\u{2019}s tasks.",
            size: CGSize(width: 693, height: 320)
        ) {
            await sourceCrumbTreePicker()
        },

        Story(
            name: "window-sidebar-folded",
            summary: "Active and the second project folded away: the hot count stays on the Active heading.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel(config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = Fixtures.mergeBoxSliceID
            return shell(appModel, folds: ["active": true, "p:\(Fixtures.secondProjectID)": true])
        },

        Story(
            name: "window-sidebar-default-folds",
            summary: "A fresh launch's milestone folds: M2, partly done, open; M3 open for the selected slice in it; M4, untouched, folded to its head.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel(
                client: FixtureNatClient(plan: defaultFoldsPlan), config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = defaultFoldsPlan.slices.first { $0.milestoneID == "M3: View gallery" }?.id
            return shell(appModel)
        },

        Story(
            name: "window-projects-folded",
            summary: "Projects folded away: its heading pins to the sidebar's foot, Active above it.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel(config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = Fixtures.mergeBoxSliceID
            return shell(appModel, folds: ["work": true])
        },

        // MARK: - The window, on what is not a slice

        Story(
            name: "window-workshop",
            summary: "A project's workshop with its planning agent live and nothing proposed yet: Brief alone, read-only with End session and a line saying the plan appears here, no Plan section yet; the titlebar's pulsing dot and its Terminal tab alone; the planning terminal.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel(
                client: FixtureNatClient(agents: Fixtures.agentStatusesWithPlanner))
            await settleOnPlanner(appModel)
            appModel.workshopSelected = true
            return shell(appModel)
        },

        Story(
            name: "window-workshop-reconnecting",
            summary: "A project's workshop running when the app last quit, before the first activity reading: its Active row a launching one captioned Reconnecting…, the Brief showing the kept request with no Plan button, the Terminal tab alone, the pane's quiet Reconnecting to the planning agent note.",
            size: window
        ) {
            shell(await reconnectingWorkshop())
        },

        Story(
            name: "window-workshop-ended",
            summary: "A project's workshop whose planning agent ended with a plan still up and unaccepted: the row pinned with Plan ready, the Brief showing the request with no Plan or End session, Terminal and Plan tabs with Plan up; Keep workshopping starts a new agent on the plan.",
            size: window
        ) {
            shell(await endedWorkshop())
        },

        Story(
            name: "window-workshop-ended-terminal",
            summary: "As window-workshop-ended, the Terminal tab up: the empty pane saying the agent has ended and Keep workshopping starts a new one.",
            size: window
        ) {
            let appModel = await endedWorkshop()
            appModel.showWorkshopTab(.terminal)
            return shell(appModel)
        },

        Story(
            name: "sidebar-workshop-reconnecting",
            summary: "As window-workshop-reconnecting, the sidebar alone: the Workshop row in Active, launching tint, Reconnecting… before its ✕.",
            size: sidebar
        ) {
            SidebarView(appModel: await reconnectingWorkshop()).environment(\.pulsesPaused, true)
        },

        Story(
            name: "workshop-composer",
            summary: "A project's workshop before launch: Brief alone in the middle, no Plan section yet, no titlebar tabs, the brief editor full-height on the right, Plan in Brief's header.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel()
            appModel.workshopSelected = true
            appModel.workshopDraft = "Split the importer into a reader and a writer, and add a dry-run flag."
            return shell(appModel)
        },

        Story(
            name: "workshop-launching",
            summary: "A project's workshop mid-launch: Plan busy, the brief read-only, the Terminal tab alone, the terminal pane starting.",
            size: window
        ) {
            let client = FixtureNatClient()
            client.holdLaunches()
            let appModel = await Fixtures.startedAppModel(client: client)
            appModel.workshopSelected = true
            appModel.workshopDraft = "Split the importer into a reader and a writer, and add a dry-run flag."
            await startHeld { await appModel.launchWorkshop(request: appModel.workshopDraft) }
            return shell(appModel)
        },

        Story(
            name: "workshop-proposal",
            summary: "A project's workshop that proposed a plan: the Plan section open on the proposed tree, Accept and Keep workshopping in its header, nothing of it in the sidebar; Terminal and Plan tabs, Plan up, each proposed task's brief boxed under its milestone.",
            size: window
        ) {
            await projectProposalShell(accepting: false)
        },

        Story(
            name: "workshop-proposal-scrolled",
            summary: "The Plan section's row for a later task clicked: the Plan tab scrolled to that task's box — M2's reconcile task, waiting on two others by name.",
            size: window
        ) {
            await projectProposalShell(accepting: false, scrollTo: PlanProposal.sliceID(milestone: 1, slice: 1))
        },

        Story(
            name: "workshop-proposal-folded",
            summary: "The Plan tab with some boxes folded to their header: M1's first two folded over each other, the third open, the last folded over M2's heading; M2's first folded under its heading, the second open, the last folded. One rule between any two headers, a header and the brief under it, or a brief and the header after it.",
            size: window
        ) {
            await projectProposalShell(accepting: false, folded: [
                PlanProposal.sliceID(milestone: 0, slice: 0), PlanProposal.sliceID(milestone: 0, slice: 1),
                PlanProposal.sliceID(milestone: 0, slice: 3), PlanProposal.sliceID(milestone: 1, slice: 0),
                PlanProposal.sliceID(milestone: 1, slice: 2),
            ])
        },

        Story(
            name: "workshop-proposal-revision",
            summary: "A project's proposal filing slices into milestones it already has: the new milestone first, then each existing one holding only its proposed slices; the count names what Accept creates; the Plan tab up, only the new milestone marked NEW.",
            size: window
        ) {
            await projectProposalShell(Fixtures.revisionProposal, accepting: false)
        },

        Story(
            name: "workshop-proposal-superseding",
            summary: "A project's proposal that supersedes work already planned: under the created work, Changes to tasks already planned — a struck-through removal, a move naming its destination, an edit unfolded to its new brief, a rename alone named as it will be, renamed from its old title, with no disclosure; the warning that Accept removes a task sits above the tree. Taller than the window so the whole Plan section shows.",
            size: CGSize(width: window.width, height: 1240)
        ) {
            await projectProposalShell(
                Fixtures.supersedingProposal, accepting: false, expandEdits: ["Cache the plan on disk"])
        },

        Story(
            name: "workshop-accepting",
            summary: "A project's proposal mid-Accept: Accept busy, Keep workshopping disabled.",
            size: window
        ) {
            await projectProposalShell(accepting: true)
        },

        Story(
            name: "sidebar-workshop-plan-ready",
            summary: "A project's workshop that proposed a plan, seen from the sidebar: its Active row wears the green ✓ Plan ready badge between the marks' slot and its ✕, the row's height and the ✕ where they always are.",
            size: sidebar
        ) {
            await workshopPlanReadySidebar(hovered: false)
        },

        Story(
            name: "sidebar-workshop-plan-ready-hovered",
            summary: "As sidebar-workshop-plan-ready, the workshop row under the pointer: washed, the badge and the ✕ where they were.",
            size: sidebar
        ) {
            await workshopPlanReadySidebar(hovered: true)
        },

        Story(
            name: "window-workshop-pinned",
            summary: "A workshop opened and not launched, then a task clicked: its row stays in Active, with its ✕, while the task is on screen.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel(
                client: FixtureNatClient(agents: Fixtures.agentStatuses), config: Fixtures.twoProjectConfig)
            appModel.workshopSelected = true
            appModel.workshopDraft = "Tidy the review flow."
            appModel.selectedSliceID = Fixtures.mergeBoxSliceID
            return shell(appModel)
        },

        Story(
            name: "window-task-log",
            summary: "A merged task's log: launched (with its time), handed back three times, sent back twice, "
                + "three follow-ups triaged — the proposal and its three decisions folded into one group of four "
                + "other items — then approved and merged.",
            size: window
        ) {
            await slicePane(
                Fixtures.shellSliceID, agents: [], details: Fixtures.taskLogSliceDetails,
                focus: NavigatorFocus(open: [.thread], main: .diff))
        },

        Story(
            name: "window-task-log-whole",
            summary: "The same log in a window tall enough to show every item, its folds open: the group of the "
                + "proposal and its three decided follow-ups, each still folded, then the approve and the merge.",
            size: CGSize(width: window.width, height: 1500)
        ) {
            await slicePane(
                Fixtures.shellSliceID, agents: [], details: Fixtures.taskLogSliceDetails,
                focus: NavigatorFocus(open: [.thread], main: .diff))
                .environment(\.threadFoldsOpen, true)
        },

        Story(
            name: "window-task-log-notes",
            summary: "An in-progress task's log with two notes on its brief, drawn with its folds open, each headed \"Another agent left a note\": one from a task on the plan, its task fact the depends-on row (dot, name, hover, click to go), and one from a person, its source fact plain text. Each item is stamped at its header's end — the time for today's, the day for this year's, the year too for last year's — Launched too, with the launch nat recorded.",
            size: window
        ) {
            await slicePane(
                Fixtures.activitySliceID, agents: [], details: Fixtures.notedSliceDetails,
                focus: NavigatorFocus(open: [.thread], main: .diff))
                .environment(\.threadFoldsOpen, true)
        },

        Story(
            name: "window-task-log-cancelled",
            summary: "An in-progress task's log after a cancel: launched, handed back, then \"Craig Johnston cancelled to Todo, work discarded\" under its own glyph and time, then relaunched from the brief alone.",
            size: window
        ) {
            await slicePane(
                Fixtures.activitySliceID, agents: [], details: Fixtures.cancelledSliceDetails,
                focus: NavigatorFocus(open: [.thread], main: .diff))
        },

        Story(
            name: "window-task-log-folds",
            summary: "An in-progress task's log as it first draws: Launched with its time, the hand-back open, "
                + "then a run of six quiet items (three notes, a blocked hand-in, a triaged proposal and its "
                + "decision) folded into one group — stacked icon, its count in italic, the span of its times — "
                + "then the send-back open and a lone note folded to its header; Relaunch, as nat recorded a launch.",
            size: window
        ) {
            await slicePane(
                Fixtures.activitySliceID, agents: [], details: Fixtures.groupedSliceDetails,
                focus: NavigatorFocus(open: [.thread], main: .diff))
        },

        Story(
            name: "window-task-log-folds-open",
            summary: "The same log with its folds open: the group's header, then its six items folded to icon, "
                + "title and time in one recessed well a step in from it, the log's rule running on past the well, "
                + "and the well's last line, \"Hide 6 items\", that folds it again; the lone note open below.",
            size: window
        ) {
            await slicePane(
                Fixtures.activitySliceID, agents: [], details: Fixtures.groupedSliceDetails,
                focus: NavigatorFocus(open: [.thread], main: .diff))
                .environment(\.threadFoldsOpen, true)
        },

        Story(
            name: "task-log-fold-hover",
            summary: "A folded group and a folded note under the pointer: each one's icon becomes its chevron, "
                + "pointing right while folded, in the icon's own slot, so nothing on the row moves.",
            size: CGSize(width: 330, height: 120)
        ) {
            let items = threadLogItems(buildThreadEvents(
                slice: Fixtures.slice(Fixtures.activitySliceID), agent: nil, brief: nil,
                events: Fixtures.groupedTaskLogEvents))
            let group = items.compactMap { item -> [ThreadEvent]? in
                if case .group(let events) = item { return events }
                return nil
            }
            let folded = items.compactMap { item -> ThreadEvent? in
                if case .folded(let event) = item { return event }
                return nil
            }
            return VStack(alignment: .leading, spacing: LogMetrics.spacing) {
                ThreadGroupCard(events: group.first ?? [], connector: .solid)
                if let note = folded.last { ThreadEventCard(event: note, collapsible: true) }
            }
            .taskLogPadding()
            .frame(maxHeight: .infinity, alignment: .top)
            .surface(.window)
            .environment(\.hoverForced, true)
        },

        Story(
            name: "window-task-log-note-todo",
            summary: "A Todo task never launched, with one note on its brief from a task on the plan: the log is that note alone, folded to its header and stamped with today's time, then the Launch action — no Launched item claims a launch that never happened.",
            size: window
        ) {
            await slicePane(
                Fixtures.fixturesSliceID, agents: [], details: Fixtures.notedTodoSliceDetails,
                focus: NavigatorFocus(open: [.thread], main: .diff))
        },

        Story(
            name: "sidebar-last-active-selected",
            summary: "The last Active row selected: its highlight the ordinary row height, the line under Active not drawn.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel(
                client: FixtureNatClient(agents: Fixtures.agentStatuses), config: Fixtures.twoProjectConfig)
            await selectLastActiveRow(appModel)
            return shell(appModel)
        },

        Story(
            name: "window-session-live",
            summary: "An ad hoc session with its agent running: Active lists it, Thread and the terminal.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel(client: FixtureNatClient(
                agents: Fixtures.agentStatuses + [Fixtures.sessionAgentStatus], sessions: [Fixtures.liveSession]))
            appModel.selectedSessionID = Fixtures.liveSession.id
            return shell(appModel)
        },

        Story(
            name: "window-session-review",
            summary: "An ad hoc session whose agent has exited with a pull request open, and an ended one under its project.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel(client: FixtureNatClient(
                sessions: [Fixtures.reviewSession, Fixtures.doneSession]))
            appModel.selectedSessionID = Fixtures.reviewSession.id
            return shell(appModel)
        },

        Story(
            name: "window-untitled",
            summary: "A launch with no projects: one Untitled project row and the starter card across the navigator and main pane.",
            size: window
        ) {
            shell(await Fixtures.startedAppModel(config: Fixtures.emptyConfig, toolsReady: true))
        },

        Story(
            name: "window-untitled-plan-file",
            summary: "The starter card with a plan file attached.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel(config: Fixtures.emptyConfig, toolsReady: true)
            let url = FileManager.default.temporaryDirectory.appendingPathComponent("habit-tracker-plan.md")
            try? "# Habit tracker\n".write(to: url, atomically: true, encoding: .utf8)
            appModel.attachPlanFile(url)
            return shell(appModel)
        },

        Story(
            name: "window-untitled-workshop",
            summary: "An Untitled project after Workshop: the planning agent's terminal and Brief alone, no Plan section until a proposal.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel(config: Fixtures.emptyConfig, toolsReady: true)
            appModel.workshopDraft = "A habit tracker with streaks."
            await appModel.launchWorkshop(request: appModel.workshopDraft)
            return shell(appModel)
        },

        Story(
            name: "untitled-workshop-launching",
            summary: "An Untitled project mid-launch from the starter card: the workshop's layout with Plan busy and the request read-only.",
            size: window
        ) {
            let client = FixtureNatClient()
            client.holdLaunches()
            let appModel = await Fixtures.startedAppModel(client: client, config: Fixtures.emptyConfig, toolsReady: true)
            appModel.workshopDraft = "A habit tracker with streaks."
            await startHeld { await appModel.launchWorkshop(request: appModel.workshopDraft) }
            return shell(appModel)
        },

        Story(
            name: "untitled-accepting",
            summary: "An Untitled project's proposal mid-Accept: Accept busy, the name field and Keep workshopping disabled.",
            size: window
        ) {
            let client = FixtureNatClient()
            client.setProposal(Fixtures.proposal)
            let appModel = await Fixtures.startedAppModel(client: client, config: Fixtures.emptyConfig, toolsReady: true)
            appModel.workshopDraft = "A Rust rewrite of the importer."
            await appModel.launchWorkshop(request: appModel.workshopDraft)
            await appModel.refreshProposals()
            client.holdAccepts()
            await startHeld { await appModel.acceptProposal() }
            return shell(appModel)
        },

        Story(
            name: "window-untitled-proposal",
            summary: "An Untitled project whose workshop proposed a plan: the name field, then the proposed tree in the Plan section, Accept and Keep workshopping; nothing of it in the sidebar.",
            size: window
        ) {
            let client = FixtureNatClient()
            client.setProposal(Fixtures.proposal)
            let appModel = await Fixtures.startedAppModel(client: client, config: Fixtures.emptyConfig, toolsReady: true)
            appModel.workshopDraft = "A Rust rewrite of the importer."
            await appModel.launchWorkshop(request: appModel.workshopDraft)
            await appModel.refreshProposals()
            return shell(appModel)
        },

        Story(
            name: "window-plan-accepted",
            summary: "The proposal just accepted: the project's own tree, the Notion nudge at the sidebar's foot, Plan accepted in the main pane.",
            size: window
        ) {
            let client = FixtureNatClient()
            client.setProposal(Fixtures.proposal)
            let appModel = await Fixtures.startedAppModel(client: client, config: Fixtures.acceptedConfig, toolsReady: true)
            appModel.openUntitledTab()
            appModel.workshopDraft = "A Rust rewrite of the importer."
            await appModel.launchWorkshop(request: appModel.workshopDraft)
            await appModel.refreshProposals()
            await appModel.acceptProposal()
            return shell(appModel)
        },

        Story(
            name: "window-onboarding",
            summary: "First run with the toolchain installed: the welcome pane and its way in.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel(config: Fixtures.emptyConfig)
            return WindowShellView(appModel: appModel)
                .environment(\.toolStatus, { Fixtures.toolStatus($0, in: Fixtures.toolsFound) })
        },

        Story(
            name: "window-onboarding-missing-tools",
            summary: "First run with nat and gh missing: the checklist's other shape.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel(config: Fixtures.emptyConfig)
            return WindowShellView(appModel: appModel)
                .environment(\.toolStatus, { Fixtures.toolStatus($0, in: Fixtures.toolsWithoutNat) })
        },

        // MARK: - The sidebar

        Story(
            name: "sidebar-loaded",
            summary: "The sidebar over two projects: Active needs-you first, the active project's tree open, the other folded with its activity pip on its folder's shoulder.",
            size: sidebar
        ) {
            let appModel = await Fixtures.startedAppModel(config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = Fixtures.mergeBoxSliceID
            return SidebarView(appModel: appModel).environment(\.pulsesPaused, true)
        },

        Story(
            name: "sidebar-project-colours",
            summary: "Every project's colour as its badge: each Active row reads badge, slash, state dot, "
                + "title, and each PROJECTS row folder, name, then its badge at the row's trailing edge. "
                + "Scratch's rows nest under one Scratch row, its icon and word; the source project takes no badge: its card is an "
                + "Active row of its own — the Shortcut logo and the card's project, MOB, then the card — its "
                + "tasks nested under it. Fold headings carry none.",
            size: sidebar
        ) {
            await projectColoursSidebar()
        },

        Story(
            name: "sidebar-project-colours-light",
            summary: "As sidebar-project-colours, in the light theme: each badge's ink and wash in the light palette's hue.",
            size: sidebar,
            colorScheme: .light
        ) {
            await projectColoursSidebar()
        },

        Story(
            name: "project-badges",
            summary: "All eight project colours as badges on the sidebar's ground, in nat's order, then the "
                + "quiet grey chip a project with no colour takes, then a source project's — its plugin's icon "
                + "then its tag on the same chip, wider: eight hues spread evenly round the circle, no two alike.",
            size: CGSize(width: 380, height: 40)
        ) {
            projectBadgeRow()
        },

        Story(
            name: "project-badges-light",
            summary: "As project-badges, in the light theme: the light set's darker hues on their washes.",
            size: CGSize(width: 380, height: 40),
            colorScheme: .light
        ) {
            projectBadgeRow()
        },

        Story(
            name: "sidebar-slice-hover",
            summary: "The loaded sidebar with one slice row of the tree under the pointer: the hover wash, "
                + "square and edge to edge, a step lighter than the selected row's, and its three-dot "
                + "button at the trailing edge.",
            size: sidebar
        ) {
            let appModel = await Fixtures.startedAppModel(config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = Fixtures.mergeBoxSliceID
            return SidebarView(appModel: appModel, hoveredSlice: Fixtures.commentsSliceID)
                .environment(\.pulsesPaused, true)
        },

        Story(
            name: "sidebar-project-hovered",
            summary: "A folded project row under the pointer: its three-dot button beside its `+`, both "
                + "showing; the open project above shows its `+` alone, its three-dot hidden in a kept slot.",
            size: sidebar
        ) {
            let appModel = await Fixtures.startedAppModel(config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = Fixtures.mergeBoxSliceID
            return SidebarView(
                appModel: appModel, folded: ["p:\(Fixtures.secondProjectID)": true],
                hoveredProject: Fixtures.secondProjectID
            ).environment(\.pulsesPaused, true)
        },

        Story(
            name: "sidebar-active-optimistic",
            summary: "The loaded sidebar just after a send to the agent waiting on the user: its Active row is "
                + "drawn working from the expectation while its reading still says waiting.",
            size: sidebar
        ) {
            let appModel = await Fixtures.startedAppModel(config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = Fixtures.mergeBoxSliceID
            await expectWaitingAgentWorking(appModel)
            return SidebarView(appModel: appModel).environment(\.pulsesPaused, true)
        },

        Story(
            name: "sidebar-active-hover",
            summary: "The loaded sidebar with one Active row under the pointer: the same square hover wash the "
                + "tree\u{2019}s rows take, a step lighter than the selected row\u{2019}s.",
            size: sidebar
        ) {
            let appModel = await Fixtures.startedAppModel(config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = Fixtures.mergeBoxSliceID
            return SidebarView(appModel: appModel, hoveredActiveRow: Fixtures.diffPaneSliceID)
                .environment(\.pulsesPaused, true)
        },

        Story(
            name: "sidebar-scrolled",
            summary: "The sidebar, short, its Projects tree scrolled down: the active project's row pinned at the "
                + "top of the tree over its milestones, the rows scrolling under it hidden behind it.",
            size: CGSize(width: sidebar.width, height: 420)
        ) {
            let appModel = await Fixtures.startedAppModel(config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = Fixtures.mergeBoxSliceID
            return SidebarView(appModel: appModel, folded: ["active": true], treeAnchor: .center)
                .environment(\.pulsesPaused, true)
        },

        Story(
            name: "sidebar-scratch",
            summary: "The scratch project as its own fold under Projects: its milestones at a project row's depth.",
            size: sidebar
        ) {
            SidebarView(
                appModel: await Fixtures.startedAppModel(config: Fixtures.scratchConfigWithSecondProject),
                folded: ["scratch": false])
                .environment(\.pulsesPaused, true)
        },

        Story(
            name: "sidebar-scratch-unfiled",
            summary: "Scratch slices added with no milestone: loose at the head of the fold, above its one milestone, with no folder of their own.",
            size: sidebar
        ) {
            await scratchSidebar(unfiledScratchPlan)
        },

        Story(
            name: "sidebar-active-scratch",
            summary: "Two scratch slices under way: one Scratch row in Active \u{2014} its icon and the word "
                + "Scratch, no chip \u{2014} with the two nested under it, each its dot and title alone.",
            size: sidebar
        ) {
            await scratchActiveSidebar(activeScratchPlan)
        },

        Story(
            name: "sidebar-active-no-scratch",
            summary: "No scratch slice under way: Active draws no Scratch row.",
            size: sidebar
        ) {
            await scratchActiveSidebar(unfiledScratchPlan)
        },

        Story(
            name: "crumb-tree-picker-scratch",
            summary: "The tree picker opened on Scratch: the projects, then Scratch \u{2014} its icon in the "
                + "folder\u{2019}s place and the word Scratch, no badge \u{2014} its milestone, its slices.",
            size: CGSize(width: 693, height: 320)
        ) {
            await scratchCrumbTreePicker()
        },

        Story(
            name: "titlebar-run-menu-scratch",
            summary: "The run tree with Scratch among the projects: its icon and the word Scratch, no badge.",
            size: CGSize(width: 500, height: 240)
        ) {
            await scratchRunTree()
        },

        Story(
            name: "sidebar-scratch-empty",
            summary: "An empty Scratch: the note under its heading, linking the workshop agent and adding a slice.",
            size: sidebar
        ) {
            await scratchSidebar(emptyScratchPlan)
        },

        Story(
            name: "sidebar-scratch-projects-folded",
            summary: "Projects folded with Scratch open: Scratch takes the height Projects gave up.",
            size: sidebar
        ) {
            SidebarView(
                appModel: await Fixtures.startedAppModel(config: Fixtures.scratchConfigWithSecondProject),
                folded: ["work": true, "scratch": false])
                .environment(\.pulsesPaused, true)
        },

        Story(
            name: "sidebar-scratch-folded",
            summary: "Scratch folded, as it starts, with Projects open: its heading pins to the sidebar's foot.",
            size: sidebar
        ) {
            SidebarView(
                appModel: await Fixtures.startedAppModel(config: Fixtures.scratchConfigWithSecondProject))
                .environment(\.pulsesPaused, true)
        },

        Story(
            name: "sidebar-all-folded",
            summary: "Active, Projects and Scratch all folded: the two lower headings pinned to the foot.",
            size: sidebar
        ) {
            SidebarView(
                appModel: await Fixtures.startedAppModel(config: Fixtures.scratchConfigWithSecondProject),
                folded: ["active": true, "work": true, "scratch": true])
                .environment(\.pulsesPaused, true)
        },

        Story(
            name: "sidebar-skeleton",
            summary: "The sidebar on a first load that has not landed.",
            size: sidebar
        ) {
            SidebarView(appModel: Fixtures.loadingAppModel())
        },

        Story(
            name: "sidebar-empty",
            summary: "A project with nothing queued: nothing running, no slices.",
            size: sidebar
        ) {
            SidebarView(appModel: await Fixtures.startedAppModel(
                client: FixtureNatClient(plan: Fixtures.emptyProjectInfo, agents: [])))
        },

        Story(
            name: "sidebar-error",
            summary: "The first read of the plan failed: the note under the project, with Retry.",
            size: sidebar
        ) {
            SidebarView(appModel: await Fixtures.startedAppModel(
                client: FixtureNatClient(behaviour: .refusing(Fixtures.loadErrorMessage))))
        },

        Story(
            name: "sidebar-state-dots",
            summary: "Every slice dot side by side as sidebar rows: todo, blocked, working (live and not), "
                + "waiting, review, pr open and done, with a folded project's needs-you dot.",
            size: CGSize(width: 300, height: 330)
        ) {
            StateDotsStory()
        },

        Story(
            name: "sidebar-state-dots-light",
            summary: "The same dots in the light theme.",
            size: CGSize(width: 300, height: 330),
            colorScheme: .light
        ) {
            StateDotsStory()
        },

        // MARK: - Task sources

        Story(
            name: "sidebar-source",
            summary: "The Work source project's own fold, headed by the plugin's title, never the project's: "
                + "its icon, filter button (filled: the section narrows) and header menu, Doing with its cards "
                + "and their tasks, the Mine and Board segments as top-level groups (one card in both), the lazy "
                + "Done folded with its count; a card with tasks drawn as the stacked card, one with none as the "
                + "single card; project badges one fixed width, a card with no project none; the first card "
                + "selected. Projects, folded, pins to the foot.",
            size: sidebar
        ) {
            await sourceSidebar()
        },

        Story(
            name: "sidebar-source-hover",
            summary: "A card row under the pointer: its estimate shows, and the + takes the badge's fixed "
                + "slot, centred where the badge was, rather than pushing it left — the title does not move.",
            size: sidebar
        ) {
            await sourceSidebar(hoveredContainer: Fixtures.sourceBoardCardID)
        },

        Story(
            name: "sidebar-source-segment-hover",
            summary: "A segment row under the pointer: its filter button (filled in the accent, the segment "
                + "narrowing) beside its menu, both hidden otherwise.",
            size: sidebar
        ) {
            await sourceSidebar(hoveredGroup: "ready/board")
        },

        Story(
            name: "sidebar-source-projects-open",
            summary: "Projects and the source fold both open: each its own section with its own scroll, "
                + "Projects taking the room and the source fold its natural height under it.",
            size: sidebar
        ) {
            await sourceSidebar(folded: [:])
        },

        Story(
            name: "sidebar-source-folded",
            summary: "The source fold folded with Projects open: Projects takes the room, the folded "
                + "source heading pins to the foot.",
            size: sidebar
        ) {
            await sourceSidebar(folded: ["s:\(Fixtures.sourceProjectID)": true])
        },

        Story(
            name: "sidebar-source-all-folded",
            summary: "Projects and the source fold both folded: their headings stack at the foot in their "
                + "usual order, Projects then the source, Active alone at the top.",
            size: sidebar
        ) {
            await sourceSidebar(folded: ["work": true, "s:\(Fixtures.sourceProjectID)": true])
        },

        Story(
            name: "sidebar-source-error",
            summary: "The plugin could not be read: its error as the fold's note, and every card with tasks "
                + "under nat's own Other containers group.",
            size: sidebar
        ) {
            await sourceSidebar(client: Fixtures.failedSourceClient())
        },

        Story(
            name: "window-container",
            summary: "A card selected: its card mark (the Shortcut logo and MOB), a slash, the card glyph and title in the titlebar, Story open with its facts "
                + "and tasks and New task, Comments and Links folded with their counts; the story and its "
                + "comments with the composer in the main pane, Open in Demo source at the trailing edge.",
            size: window
        ) {
            await sourceShell(container: Fixtures.sourceCardID)
        },

        Story(
            name: "window-container-links",
            summary: "The same card with its Links section's header clicked: the links in the section, "
                + "a pull request by its branch glyph and state, a document by its arrow out, and the list in the main pane.",
            size: window
        ) {
            await sourceShell(
                container: Fixtures.sourceCardID,
                containerFocus: ContainerFocus(open: ["links"], main: .section("links")))
        },

        Story(
            name: "window-source-task-brief",
            summary: "A Todo task under a card: the brief's facts lead with the card, opening in its "
                + "source, and the card's own facts in the milestone's place; the status bar reads card / task.",
            size: window
        ) {
            await sourceShell(task: Fixtures.sourceTodoTaskID)
        },

        Story(
            name: "window-source-task-pr",
            summary: "A handed-back task with its pull request open: the PR section ends on the card's own "
                + "note about what merging does.",
            size: window
        ) {
            await sourceShell(task: Fixtures.sourceReviewTaskID)
        },

        Story(
            name: "source-filter-popover",
            summary: "A segment's Filter…, opened from its row's filter button: Team, Project, State (a "
                + "segment's alone), Epic and Labels over the workspace's choices, opened on the segment's own "
                + "(team Board); where the section sets a field, Any names what it falls through to "
                + "(\u{201C}Any (section\u{2019}s: Mobile App)\u{201D}).",
            size: CGSize(width: 360, height: 280)
        ) {
            filterPopover(Fixtures.sourceGroups()[1].menu.first { $0.input == .filter })
        },

        Story(
            name: "source-filter-popover-section",
            summary: "The section header's Filter…, opened from the header's filter button: four fields, no "
                + "State, narrowing every list the fold draws (here, the Mobile App project), with nothing "
                + "wider to fall through to.",
            size: CGSize(width: 360, height: 240)
        ) {
            filterPopover(Fixtures.sourceInfo().menu.first { $0.input == .filter })
        },

        Story(
            name: "source-filter-popover-loading",
            summary: "The editor opened before the plugin's background fetch of the epic list has landed: "
                + "Epic says it is loading, and the other fields work regardless.",
            size: CGSize(width: 360, height: 280)
        ) {
            filterPopover(Fixtures.sourceGroups(epicsLoading: true)[1].menu.first { $0.input == .filter })
        },

        // MARK: - The titlebar band

        Story(
            name: "titlebar-band-slice",
            summary: "The titlebar band over a slice: the breadcrumb at the navigator\u{2019}s inset \u{2014} the project\u{2019}s badge, "
                + "milestone, then the slice\u{2019}s dot and title with no second badge, the project crumb naming it "
                + "already \u{2014} no rule at the split, the tabs filling from the trailing edge (PR, Changes, "
                + "Terminal left to right, Terminal rightmost) and nothing beside them.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(tabs: [.terminal, .changes, .pr], selected: .terminal, crumbs: sliceCrumbs("Draw the box"))
        },

        Story(
            name: "titlebar-band-run",
            summary: "A handed-back slice\u{2019}s band in a project with runs: the run split button the "
                + "band\u{2019}s rightmost item, the tabs \u{2014} PR, Visual changes, Changes, Terminal \u{2014} "
                + "right-aligned against it.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: [.terminal, .changes, .visuals, .pr], selected: .diff, crumbs: sliceCrumbs("Draw the box"),
                state: .review, runs: true)
        },

        Story(
            name: "titlebar-band-run-narrow",
            summary: "The same band in a narrow window: the run button keeps its width and the tabs give way, cut "
                + "at their leading edge at the navigator split.",
            size: CGSize(width: 640, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: [.terminal, .changes, .visuals, .pr], selected: .diff, crumbs: sliceCrumbs("Draw the box"),
                state: .review, runs: true)
        },

        Story(
            name: "titlebar-band-run-terminal",
            summary: "The run button beside a picked rightmost tab: Terminal stands open on the window\u{2019}s "
                + "ground, its trailing edge closed by the same line as every tab\u{2019}s leading one.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: [.terminal, .changes, .visuals, .pr], selected: .terminal, crumbs: sliceCrumbs("Draw the box"),
                state: .review, runs: true)
        },

        Story(
            name: "titlebar-band-run-busy",
            summary: "The run button while its run starts: the spinner in the play glyph\u{2019}s slot before "
                + "the label, which keeps its place.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: [.terminal, .changes, .visuals, .pr], selected: .diff, crumbs: sliceCrumbs("Draw the box"),
                state: .review, runs: true, runBusy: true)
        },

        Story(
            name: "titlebar-band-run-hover",
            summary: "The run button under the pointer: the row wash behind its main part and its chevron, "
                + "each the band\u{2019}s full height, the divider between them full height too and drawn over "
                + "the wash, as the tabs\u{2019} lines are.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: [.terminal, .changes, .visuals, .pr], selected: .diff, crumbs: sliceCrumbs("Draw the box"),
                state: .review, runs: true, runHovered: true)
        },

        Story(
            name: "titlebar-band-tab-hover",
            summary: "The slice\u{2019}s band with its Changes tab under the pointer: the row wash on the header "
                + "behind that one tab, the picked Terminal tab standing open on the window\u{2019}s ground.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: [.terminal, .changes, .pr], selected: .terminal, crumbs: sliceCrumbs("Draw the box"),
                hoveredTab: .changes)
        },

        Story(
            name: "titlebar-band-long-title",
            summary: "A long task name runs on past the navigator\u{2019}s width into the gap, the project crumb "
                + "its badge, the tabs still at the right and never left of the split.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(tabs: [.terminal, .changes, .pr], selected: .diff, crumbs: sliceCrumbs(longBandTitle))
        },

        Story(
            name: "titlebar-band-long-title-narrow",
            summary: "The same band in a narrower window: too little room for the name at 80% with any crumb "
                + "before it, so just the Active row\u{2019}s line \u{2014} badge, slash, dot, name ellipsized, the chevron beside it.",
            size: CGSize(width: 760, height: GnatMetrics.titlebarHeight)
        ) {
            band(tabs: [.terminal, .changes, .pr], selected: .diff, crumbs: sliceCrumbs(longBandTitle))
        },

        Story(
            name: "titlebar-band-fit-title",
            summary: "Room running out, first step: the task\u{2019}s name ellipsizes, still showing at least 80% of itself; the project\u{2019}s badge and the milestone whole.",
            size: CGSize(width: 780, height: GnatMetrics.titlebarHeight)
        ) {
            band(tabs: [.terminal, .changes, .pr], selected: .diff, crumbs: sliceCrumbs(fitBandTitle))
        },

        Story(
            name: "titlebar-band-fit-milestone",
            summary: "Second step: the name held at 80%, the milestone ellipsizes, down to half of itself; the badge stays.",
            size: CGSize(width: 680, height: GnatMetrics.titlebarHeight)
        ) {
            band(tabs: [.terminal, .changes, .pr], selected: .diff, crumbs: sliceCrumbs(fitBandTitle))
        },

        Story(
            name: "titlebar-band-fit-minimal",
            summary: "Past every floor: no breadcrumb, just the Active row\u{2019}s line \u{2014} the GNA badge, a slash, the state dot, the name, which alone ellipsizes.",
            size: CGSize(width: 620, height: GnatMetrics.titlebarHeight)
        ) {
            band(tabs: [.terminal, .changes, .pr], selected: .diff, crumbs: sliceCrumbs(fitBandTitle))
        },

        Story(
            name: "titlebar-band-project-colour",
            summary: "The band over a slice of a pink project: the project crumb is its badge alone, GNA in pink "
                + "on a pink wash, then its slash \u{2014} no folder glyph and no name \u{2014} from the navigator\u{2019}s inset.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: [.terminal, .changes, .pr], selected: .terminal, crumbs: sliceCrumbs("Draw the box"),
                projectColor: .pink)
        },

        Story(
            name: "titlebar-band-scratch",
            summary: "The band over a scratch slice: the project crumb is Scratch\u{2019}s icon and the word "
                + "Scratch in the quiet ink, no chip, then its slash.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: [.terminal, .changes, .pr], selected: .terminal,
                crumbs: TitlebarCrumbs(project: "Scratch", parent: "Spikes", title: "Profile the diff read"),
                identity: TitlebarIdentity(
                    tag: "SCR", state: .working, live: true, title: "Profile the diff read", isScratch: true),
                projectColor: nil)
        },

        Story(
            name: "titlebar-band-scratch-minimal",
            summary: "The same band past every floor: the Active row\u{2019}s line, Scratch\u{2019}s icon and "
                + "word, a slash, the dot and the name.",
            size: CGSize(width: 560, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: [.terminal, .changes, .pr], selected: .terminal,
                crumbs: TitlebarCrumbs(project: "Scratch", parent: "Spikes", title: fitBandTitle),
                identity: TitlebarIdentity(
                    tag: "SCR", state: .working, live: true, title: fitBandTitle, isScratch: true),
                projectColor: nil)
        },

        Story(
            name: "titlebar-band-workshop",
            summary: "The workshop\u{2019}s band: the project\u{2019}s badge then the wand and Workshop, no second badge, and no tabs.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: [], selected: nil,
                crumbs: TitlebarCrumbs(parent: Fixtures.project.name, parentKind: .project, title: workshopRowTitle),
                identity: TitlebarIdentity(
                    tag: "GNA", state: .working, live: true, title: workshopRowTitle, symbol: workshopSymbol))
        },

        Story(
            name: "titlebar-band-session",
            summary: "An ad hoc session\u{2019}s band: the project\u{2019}s badge then the session, and its own tabs.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: MainPaneTab.forSession(hasPRs: true), selected: .terminal,
                crumbs: TitlebarCrumbs(
                    parent: Fixtures.project.name, parentKind: .project,
                    title: "\(sessionRowTitle) \u{00B7} tidy the release notes"))
        },

        Story(
            name: "titlebar-band-source-task",
            summary: "A Shortcut task\u{2019}s band: its card\u{2019}s badge \u{2014} the Shortcut logo then the card\u{2019}s "
                + "project, MOB, in its colour \u{2014} in the project crumb\u{2019}s place, then the card with the card mark, "
                + "then the task\u{2019}s dot and title with no second badge. Shortcut itself takes none.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: [.terminal, .changes], selected: .terminal,
                crumbs: TitlebarCrumbs(
                    project: "Shortcut", parent: "Billing export", parentKind: .container, title: "Add the CSV column"),
                identity: TitlebarIdentity(
                    tag: "", state: .working, live: true, title: "Add the CSV column", cardBadge: storyCardBadge,
                    cardIcon: Fixtures.shortcutIcon),
                projectColor: nil)
        },

        Story(
            name: "titlebar-band-source-task-no-project",
            summary: "A Shortcut task whose card has no project: the Shortcut logo alone in the project crumb\u{2019}s "
                + "place, then the card, then the task.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: [.terminal, .changes], selected: .terminal,
                crumbs: TitlebarCrumbs(
                    project: "Shortcut", parent: "Billing export", parentKind: .container, title: "Add the CSV column"),
                identity: TitlebarIdentity(
                    tag: "", state: .working, live: true, title: "Add the CSV column", cardIcon: Fixtures.shortcutIcon),
                projectColor: nil)
        },

        Story(
            name: "titlebar-band-container",
            summary: "A Shortcut card\u{2019}s band: its badge \u{2014} the Shortcut logo then MOB \u{2014} in the project "
                + "crumb\u{2019}s place, a slash, then the card mark and the card\u{2019}s title; no tabs and no trailing items.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: [], selected: nil,
                crumbs: TitlebarCrumbs(project: "Shortcut", title: "Billing export"),
                identity: .container(title: "Billing export", icon: Fixtures.shortcutIcon, badge: storyCardBadge),
                projectColor: nil)
        },

        // MARK: - The Changes section

        Story(
            name: "changes-section-commits",
            summary: "The Changes section\u{2019}s body on a review: the commit switcher in a row above the file "
                + "list it filters, then the files with their viewed boxes and tallies.",
            size: CGSize(width: GnatMetrics.navigatorWidth, height: 260)
        ) {
            await changesSection()
        },

        // MARK: - The status bar

        Story(
            name: "status-bar-agent-readout",
            summary: "The status bar with the selection\u{2019}s agent live: the usage windows at the leading edge, "
                + "the agent\u{2019}s model and effort, a divider, then its context clause at the trailing edge, no breadcrumb.",
            size: CGSize(width: 1320, height: GnatMetrics.statusBarHeight)
        ) {
            StatusBarView(
                appModel: await Fixtures.startedAppModel(
                    client: FixtureNatClient(
                        plan: statusBarPlan, agents: Fixtures.agentStatuses, usage: Fixtures.usageReading))
            ) {
                AgentModelHeading(agent: bandAgent)
            }
        },

        Story(
            name: "status-bar-agent-readout-high-context",
            summary: "The same readout at 91% context: the context clause switches to the warning tint.",
            size: CGSize(width: 1320, height: GnatMetrics.statusBarHeight)
        ) {
            StatusBarView(
                appModel: await Fixtures.startedAppModel(
                    client: FixtureNatClient(
                        plan: statusBarPlan, agents: Fixtures.agentStatuses, usage: Fixtures.usageReading))
            ) {
                AgentModelHeading(agent: AgentStatus(
                    sliceID: Fixtures.diffPaneSliceID, session: "nat-1", activity: .working,
                    model: "Sonnet 5", effort: "high", contextPercent: 91, contextTokens: 182_300))
            }
        },

        Story(
            name: "status-bar-github-healthy",
            summary: "The bar after a healthy GitHub reading: nothing about GitHub at all, "
                + "the usage windows the last clause.",
            size: CGSize(width: 1320, height: GnatMetrics.statusBarHeight)
        ) {
            await githubBudgetStatusBar(GitHubRateLimit(
                limit: 5000, remaining: 4211, resetAt: Fixtures.now.addingTimeInterval(46 * 60),
                projectedRemainingAtReset: 4100, pollAfterSeconds: 30, cost: 1))
        },

        Story(
            name: "status-bar-github-throttled",
            summary: "The bar once nat throttles polling to keep the reserve: \u{201C}GitHub \u{00B7} 412 "
                + "left\u{201D} after the usage windows, the projection and the reset its tooltip.",
            size: CGSize(width: 1320, height: GnatMetrics.statusBarHeight)
        ) {
            await githubBudgetStatusBar(GitHubRateLimit(
                limit: 5000, remaining: 412, resetAt: Fixtures.now.addingTimeInterval(46 * 60),
                projectedRemainingAtReset: 120, throttled: true, pollAfterSeconds: 300, cost: 1))
        },

        Story(
            name: "status-bar-github-paused",
            summary: "The bar once GitHub refused and nat paused polling: \u{201C}GitHub limit \u{00B7} "
                + "resets\u{201D} and the time, in the warning tint.",
            size: CGSize(width: 1320, height: GnatMetrics.statusBarHeight)
        ) {
            await githubBudgetStatusBar(GitHubRateLimit(
                limit: 5000, remaining: 0, resetAt: Fixtures.now.addingTimeInterval(46 * 60),
                projectedRemainingAtReset: 0, pausedUntil: Fixtures.now.addingTimeInterval(46 * 60),
                pollAfterSeconds: 2760))
        },

        Story(
            name: "status-bar-no-agents",
            summary: "The same bar with nothing running: the agent count reads zero, "
                + "the usage windows after it.",
            size: CGSize(width: 1320, height: GnatMetrics.statusBarHeight)
        ) {
            StatusBarView(
                appModel: await Fixtures.startedAppModel(
                    client: FixtureNatClient(plan: statusBarPlan, agents: []))
            )
        },

        Story(
            name: "status-bar-several-agents",
            summary: "The bar with the crowded plan and three agents live: the count "
                + "pluralizes.",
            size: CGSize(width: 1320, height: GnatMetrics.statusBarHeight)
        ) {
            StatusBarView(
                appModel: await Fixtures.startedAppModel(
                    client: FixtureNatClient(plan: crowdedPlan, agents: Fixtures.agentStatusesWithPlanner))
            )
        },

        Story(
            name: "status-bar-usage-at-rest",
            summary: "The Claude usage windows after the agent count, each set off by a faint divider, both "
                + "well under the warning threshold: each window's "
                + "percent and reset in the bar's own tertiary tint.",
            size: CGSize(width: 1320, height: GnatMetrics.statusBarHeight)
        ) {
            StatusBarView(
                appModel: await Fixtures.startedAppModel(
                    client: FixtureNatClient(
                        plan: statusBarPlan, agents: Fixtures.agentStatuses, usage: Fixtures.usageReading))
            )
        },

        Story(
            name: "status-bar-usage-one-warning",
            summary: "One window past the warning threshold: its whole clause — "
                + "percent and reset together — switches to the warning tint (system "
                + "orange), the other window stays tertiary.",
            size: CGSize(width: 1320, height: GnatMetrics.statusBarHeight)
        ) {
            StatusBarView(
                appModel: await Fixtures.startedAppModel(
                    client: FixtureNatClient(
                        plan: statusBarPlan, agents: Fixtures.agentStatuses,
                        usage: Fixtures.usageReadingOneWarning))
            )
        },

        Story(
            name: "status-bar-usage-both-warning",
            summary: "Both windows past the warning threshold: both clauses draw in "
                + "the warning tint.",
            size: CGSize(width: 1320, height: GnatMetrics.statusBarHeight)
        ) {
            StatusBarView(
                appModel: await Fixtures.startedAppModel(
                    client: FixtureNatClient(
                        plan: statusBarPlan, agents: Fixtures.agentStatuses,
                        usage: Fixtures.usageReadingBothWarning))
            )
        },

        Story(
            name: "status-bar-usage-unavailable",
            summary: "No usage reading available at all — the readout draws nothing, "
                + "leaving the agent count alone at the bar's leading edge.",
            size: CGSize(width: 1320, height: GnatMetrics.statusBarHeight)
        ) {
            StatusBarView(
                appModel: await Fixtures.startedAppModel(
                    client: FixtureNatClient(
                        plan: statusBarPlan, agents: Fixtures.agentStatuses, usage: .empty))
            )
        },

        Story(
            name: "status-bar-claude-update",
            summary: "A newer Claude Code released: one accent chip after the usage windows, "
                + "\u{201C}Claude Code 2.1.295 available\u{201D} \u{2014} a click runs the update.",
            size: CGSize(width: 1320, height: GnatMetrics.statusBarHeight)
        ) {
            StatusBarView(
                appModel: await Fixtures.startedAppModel(
                    client: FixtureNatClient(
                        plan: statusBarPlan, agents: Fixtures.agentStatuses, usage: Fixtures.usageReading,
                        claudeVersion: Fixtures.claudeVersionBehind))
            )
        },

        Story(
            name: "claude-update-sheet-running",
            summary: "The update sheet while claude update runs: a spinner, Done disabled.",
            size: CGSize(width: 420, height: 160)
        ) {
            ClaudeUpdateSheet(state: .running) {}
        },

        Story(
            name: "claude-update-sheet-finished",
            summary: "The update sheet once it has run: claude update\u{2019}s own output, then that agents "
                + "already running keep their version and ones launched from now on get the new one.",
            size: CGSize(width: 420, height: 230)
        ) {
            ClaudeUpdateSheet(state: .finished(output: Fixtures.claudeUpdateOutput)) {}
        },

        Story(
            name: "claude-update-sheet-failed",
            summary: "The update sheet when claude update failed: nat\u{2019}s refusal, carrying claude\u{2019}s "
                + "own words, and no note about agents \u{2014} nothing changed.",
            size: CGSize(width: 420, height: 160)
        ) {
            ClaudeUpdateSheet(state: .failed(
                message: "nat: claude update: exit status 1: Error: could not write to the install directory")) {}
        },

        // MARK: - The header

        // MARK: - Pieces

        Story(
            name: "notion-page-picker",
            summary: "The sheet \"Choose page\u{2026}\" opens (\"Notion page picker\" in the design): the "
                + "Notion mark and title, the search field, the workspace's pages and databases with a "
                + "database chip and the first one chosen, Cancel beside Create page.",
            size: CGSize(width: 440, height: 380)
        ) {
            let model = NotionPickerModel(client: FixtureNatClient())
            await model.search()
            model.selectedID = Fixtures.notionPlaces.first?.id
            return NotionPickerSheetView(
                model: model, onCancel: {}, onCreate: { _ in nil }, onCreated: {})
        },

        Story(
            name: "notion-page-picker-light",
            summary: "The same sheet in the light theme: the Notion mark a light face under dark lines, as in the dark.",
            size: CGSize(width: 440, height: 380),
            colorScheme: .light
        ) {
            let model = NotionPickerModel(client: FixtureNatClient())
            await model.search()
            model.selectedID = Fixtures.notionPlaces.first?.id
            return NotionPickerSheetView(
                model: model, onCancel: {}, onCreate: { _ in nil }, onCreated: {})
        },

        Story(
            name: "launch-options-model-picker",
            summary: "The Brief tab's launch popover form: the model field is a menu "
                + "picker now, offering Default and AgentOptions' own aliases.",
            size: CGSize(width: 320, height: 220)
        ) {
            LaunchOptionsForm(model: .constant(""), effort: .constant(""), agentOptions: .fallback)
                .padding(14)
                .surface(.window)
        },

        Story(
            name: "launch-options-model-picker-custom",
            summary: "The same form with a full model ID configured: it selects Custom "
                + "and shows the ID in the field, round-tripping rather than landing "
                + "on a blank selection.",
            size: CGSize(width: 320, height: 220)
        ) {
            LaunchOptionsForm(
                model: .constant("claude-sonnet-5"), effort: .constant("high"), agentOptions: .fallback
            )
            .padding(14)
            .surface(.window)
        },

        Story(
            name: "agent-skeleton",
            summary: "The Agent stage the pane advances to the moment Launch agent is pressed, before the session appears.",
            size: pane
        ) {
            AgentSkeletonView()
        },

        Story(
            name: "agent-terminal-selection",
            summary: "The agent terminal with an active click-drag selection spanning two rows.",
            size: pane
        ) {
            TerminalSelectionStubView()
        },

        Story(
            name: "agent-terminal-transcript",
            summary: "The agent terminal as a real SwiftTerm view fed a turn of a Claude Code "
                + "session — the banner, a read, an edit's hunk, a test run, the hand-back and "
                + "the prompt — the one story where the terminal's own type is what is drawn.",
            size: pane
        ) {
            TerminalTranscriptStoryView()
                .background(DesignTokens.fill(.terminal))
        },

        Story(
            name: "pr-composer-typed",
            summary: "The comment box with an emoji comment typed into it: the editor grown to "
                + "its two lines, under its 60pt ceiling, rather than sitting at either bound.",
            size: CGSize(width: 560, height: 140)
        ) {
            PRComposerTypedStory()
        },

        Story(
            name: "pr-conversation-reply",
            summary: "The PR view with Reply pressed on the first comment and a reply typed: its "
                + "composer inside the comment's box under the body, placeholder Reply to <author>, "
                + "Cancel beside send; the pull request's own composer still at the foot.",
            size: pane
        ) {
            await prConversation(replyTo: 0, replyText: "Agreed — the worst verdict wins.\nI'll add a test for the tie.")
        },

        Story(
            name: "pr-conversation-closed",
            summary: "A closed pull request under the pointer: every entry's Reply and the "
                + "description's Edit showing, and the comment box at the foot — GitHub takes "
                + "comments on a closed or merged pull request.",
            size: pane
        ) {
            await prConversation(Fixtures.prGreenClosed).environment(\.hoverForced, true)
        },

        Story(
            name: "pr-description-editing",
            summary: "The description in edit mode: the markdown swapped for the composer's editor "
                + "prefilled with the body, Cancel and Save under it.",
            size: pane
        ) {
            await prConversation(editing: Fixtures.prGreen.body)
        },

        Story(
            name: "markdown-table",
            summary: "A comment carrying a markdown table, at the navigator's width: the long "
                + "Command and Note columns each cut to its share with the expand mark in its "
                + "heading, the table scrolling sideways.",
            size: CGSize(width: 330, height: 220)
        ) {
            MarkdownTableStory(expanded: [])
        },

        Story(
            name: "markdown-table-expanded",
            summary: "The same table with its Note column expanded to its full width: the "
                + "abbreviate mark in its heading, the rest scrolled off to the right.",
            size: CGSize(width: 330, height: 220)
        ) {
            MarkdownTableStory(expanded: [3])
        },

        Story(
            name: "markdown-details",
            summary: "A comment carrying two GitHub <details> folds: one shut on its summary, one "
                + "written open showing the markdown it folds.",
            size: CGSize(width: 420, height: 220)
        ) {
            MarkdownView(text: """
                Ran the suite twice.

                <details>
                <summary>First run's log</summary>

                lots of output
                </details>

                <details open>
                <summary>Second run</summary>

                All **green** — `PRStoreTests` passed on retry.
                </details>
                """, size: Typo.scaled(13.5))
            .padding(12)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
            .surface(.window)
        },

        // MARK: - Follow-ups

        // MARK: - Run commands

        Story(
            name: "titlebar-run",
            summary: "The sidebar\u{2019}s titlebar segment while a project has runs: the play button beside "
                + "Settings and +.",
            size: CGSize(width: 260, height: GnatMetrics.titlebarHeight)
        ) {
            await titlebarRun(treeOpen: false)
        },

        Story(
            name: "titlebar-run-menu",
            summary: "The play button\u{2019}s run tree open, the breadcrumb tree picker\u{2019}s shape: projects "
                + "with runs in one column, the open project\u{2019}s runs and their commands in the next, the "
                + "first marked default.",
            size: CGSize(width: 500, height: 300)
        ) {
            await titlebarRun(treeOpen: true)
        },

        Story(
            name: "titlebar-run-menu-running",
            summary: "The run tree with the project\u{2019}s Board run live: its row greyed and not to be picked, "
                + "Play beside it still offered; the play button spinning.",
            size: CGSize(width: 500, height: 300)
        ) {
            await titlebarRun(treeOpen: true, running: true)
        },

        Story(
            name: "run-menu-running",
            summary: "A slice\u{2019}s run menu with its default, Play, live: Play greyed and not to be picked, "
                + "Board still offered.",
            size: CGSize(width: 320, height: 130)
        ) {
            RunMenuList(runs: Fixtures.runs.sliceRuns, isRunning: { $0 == "Play" }) { _ in }
                .surface(.header)
        },

        Story(
            name: "window-run-heading",
            summary: "A handed-back slice of a project with runs: the run split button at the titlebar "
                + "band\u{2019}s trailing edge, full height, the tabs filling leftwards from it; the play button "
                + "in the sidebar\u{2019}s titlebar segment.",
            size: window
        ) {
            await slicePane(Fixtures.mergeBoxSliceID, config: Fixtures.runsConfig)
        },

        Story(
            name: "window-run-heading-merged",
            summary: "The same slice once merged: the titlebar\u{2019}s run button greyed and disabled, "
                + "the worktree being gone; the action bar reads Task completed.",
            size: window
        ) {
            await slicePane(
                Fixtures.mergeBoxSliceID, plan: Fixtures.mergedReviewProjectInfo, config: Fixtures.runsConfig)
        },

        Story(
            name: "window-run-heading-running",
            summary: "The handed-back slice with its default run, Play, live: the run button\u{2019}s main part "
                + "spinning and greyed, so Play is not started twice; its chevron still live for Board.",
            size: window
        ) {
            await slicePane(Fixtures.mergeBoxSliceID, config: Fixtures.runsConfig) { appModel in
                appModel.runSessionExists = { _ in true }
                await appModel.startRun(projectID: Fixtures.projectID, sliceID: Fixtures.mergeBoxSliceID)
            }
        },

        // MARK: - Settings

        Story(
            name: "settings",
            summary: "The settings window on General over the fixture config: the sidebar's Settings "
                + "heading clear of the (here undrawn) traffic lights, its shaded section tiles, General "
                + "selected in the accent, and its two groups under bold headings.",
            size: CGSize(width: 760, height: 560),
            colorScheme: .light
        ) {
            SettingsView(appModel: await Fixtures.startedAppModel(), client: FixtureNatClient())
        },

        Story(
            name: "settings-about",
            summary: "The settings window on About: the gnat icon, its version and build (dev in a "
                + "bare executable), the embedded nat's version, Check for Updates (disabled with no "
                + "updater, as a dev build's is) and the repository link.",
            size: CGSize(width: 760, height: 560),
            colorScheme: .light
        ) {
            SettingsView(appModel: await Fixtures.startedAppModel(), client: FixtureNatClient(), initialTab: .about)
        },

        Story(
            name: "settings-about-diagnostics",
            summary: "About with Diagnostics unfolded: a throttled GitHub reading's used and total and its "
                + "reset, gnat's own points, readings and actions this session, and how long it has been open.",
            size: CGSize(width: 760, height: 560),
            colorScheme: .light
        ) {
            await aboutDiagnostics()
        },

        Story(
            name: "settings-agents",
            summary: "The settings window's Agents section: the model field is a menu picker "
                + "now, over AgentOptions' own alias set, matching the effort picker's "
                + "own shape.",
            size: CGSize(width: 760, height: 560),
            colorScheme: .light
        ) {
            SettingsView(appModel: await Fixtures.startedAppModel(), client: FixtureNatClient(), initialTab: .agents)
        },

        Story(
            name: "settings-agents-custom-model",
            summary: "The same section with a full model ID already configured: the picker "
                + "selects Custom on its own and shows the ID in the field beneath it.",
            size: CGSize(width: 760, height: 560),
            colorScheme: .light
        ) {
            let client = FixtureNatClient(config: Fixtures.configDocWithCustomModel)
            return SettingsView(appModel: await Fixtures.startedAppModel(client: client), client: client, initialTab: .agents)
        },

        Story(
            name: "project-settings",
            summary: "A project's settings sheet (the project menu's Project settings\u{2026}): one grouped, "
                + "scrolling form headed by the project's Name field, then its working directory \u{2014} the "
                + "field and Choose\u{2026} beside it \u{2014}, its Colour, a swatch per colour with the "
                + "project's own ringed and its badge (NOT, in its teal) after them, its Plan (Notion, with "
                + "Open in Notion) and Run commands (none yet, Add Run), Cancel and Save pinned at the foot.",
            size: CGSize(width: 560, height: 640),
            colorScheme: .light
        ) {
            // The fixture project's ID is no Notion page ID, which would drop
            // Open in Notion; the row is drawn for a real page's.
            let appModel = await Fixtures.startedAppModel()
            let model = ProjectSettingsModel(
                projectID: Fixtures.projectID,
                fields: ProjectSettingsFields(projectID: Fixtures.projectID, config: appModel.config),
                plan: .notion(page: NotionPageURL.forPage("3b738308-f654-811c-948d-e1fb36f71df3")),
                write: { _ in }, reload: {})
            return ProjectSettingsView(projectName: "notion-agent-tracker", projectTag: "NOT", model: model)
        },

        Story(
            name: "project-settings-refused",
            summary: "The project settings sheet after a Save nat refused: the emptied name and the edited "
                + "path kept in their fields with nat's message under each, nothing written.",
            size: CGSize(width: 560, height: 640),
            colorScheme: .light
        ) {
            let appModel = await Fixtures.startedAppModel()
            let nameKey = SettingsModel.nameKey(projectID: Fixtures.projectID)
            let model = ProjectSettingsModel(
                projectID: Fixtures.projectID,
                fields: ProjectSettingsFields(projectID: Fixtures.projectID, config: appModel.config),
                write: { change in
                    throw NatError.commandFailed(change.key == nameKey
                        ? "config-set: \(nameKey) wants a name, given none"
                        : "working_dir: /Users/craig/nowhere is not a directory")
                },
                reload: {})
            model.edited.name = ""
            model.edited.workingDir = "/Users/craig/nowhere"
            _ = await model.save()
            return ProjectSettingsView(projectName: "notion-agent-tracker", projectTag: "NOT", model: model)
        },

        Story(
            name: "project-settings-runs",
            summary: "The settings sheet of a local project with several run commands \u{2014} global, slice "
                + "and scopeless, each a label, a command in the mono face, a scope menu and remove, a grip to "
                + "drag \u{2014} after a Save nat refused for a label offered twice: the rows kept as typed and "
                + "nat's message under the section. Its Plan is Local, the plan file's path under Reveal in Finder.",
            size: CGSize(width: 560, height: 640),
            colorScheme: .light
        ) {
            let runs = [
                RunCommand(label: "Run", command: "swift run --package-path macos gnat", scope: .global),
                RunCommand(label: "Run", command: "NAT_BIN=$PWD/nat swift run --package-path macos gnat", scope: .slice),
                RunCommand(label: "Gallery", command: "swift run --package-path macos gnat --all --out /tmp/gallery"),
                RunCommand(label: "Test", command: "go test ./..."),
            ]
            let model = ProjectSettingsModel(
                projectID: Fixtures.projectID,
                fields: ProjectSettingsFields(
                    name: "notion-agent-tracker", workingDir: "/Users/craig/Projects/notion-agent-tracker",
                    color: .teal, runs: runs),
                plan: .local(file: Fixtures.planFile(projectID: Fixtures.projectID)),
                write: { _ in
                    throw NatError.commandFailed(
                        "config-set: run 4: the label \"Gallery\" is offered twice in the titlebar")
                },
                reload: {})
            model.edited.runs[3].label = "Gallery"
            _ = await model.save()
            // Scrolled to the foot, where the refusal is.
            return ProjectSettingsView(projectName: "notion-agent-tracker", projectTag: "NOT", model: model)
                .defaultScrollAnchor(.bottom)
        },

        Story(
            name: "project-settings-source",
            summary: "A source project's settings sheet: its plugin's title as its Name, read-only, with a "
                + "note that the plugin names it; the working directory; Plan, Source via the plugin; no Colour "
                + "and no Run commands.",
            size: CGSize(width: 560, height: 640),
            colorScheme: .light
        ) {
            let model = ProjectSettingsModel(
                projectID: Fixtures.projectID,
                fields: ProjectSettingsFields(workingDir: ""),
                takesColor: false, isSource: true, plan: .source(plugin: "shortcut"),
                write: { _ in }, reload: {})
            return ProjectSettingsView(projectName: "Shortcut", projectTag: "", model: model)
        },

        Story(
            name: "project-settings-colour-chosen",
            summary: "The project settings sheet, dark, with another colour picked than the one its entry "
                + "holds: purple ringed in the accent and the badge beside the swatches in it, written by Save.",
            size: CGSize(width: 520, height: 260)
        ) {
            let appModel = await Fixtures.startedAppModel()
            let model = ProjectSettingsModel(
                projectID: Fixtures.projectID, config: appModel.config, client: FixtureNatClient(), reload: {})
            model.edited.color = .purple
            // A real sheet stands on the window's own ground; a render has
            // none, so the story gives it the system's, dark.
            return ProjectSettingsView(projectName: "notion-agent-tracker", projectTag: "NOT", model: model)
                .background(.background)
        },

        Story(
            name: "project-settings-agents",
            summary: "The project settings sheet scrolled to Agents: the slice agent's model set for this "
                + "project alone (opus) over the global pair, its effort left to the global one — "
                + "\"Default (high)\" — and the planning agent's effort set, its model \"Default (sonnet)\".",
            size: CGSize(width: 560, height: 640),
            colorScheme: .light
        ) {
            let model = ProjectSettingsModel(
                projectID: Fixtures.projectID,
                fields: ProjectSettingsFields(
                    name: "notion-agent-tracker", workingDir: "/Users/craig/Projects/notion-agent-tracker",
                    color: .teal, sliceModel: "opus", workshopEffort: "low"),
                globalSliceAgent: AgentModel(model: "sonnet", effort: "high"),
                globalWorkshopAgent: AgentModel(model: "sonnet"),
                write: { _ in }, reload: {})
            return ProjectSettingsView(projectName: "notion-agent-tracker", projectTag: "NOT", model: model)
                .defaultScrollAnchor(.center)
        },

        Story(
            name: "project-settings-merge",
            summary: "The project settings sheet scrolled to Merging: squash and merge, the branch deleted "
                + "after merging, develop as the base branch; and beside the colour a tag of its own, NT, the "
                + "badge preview wearing it.",
            size: CGSize(width: 560, height: 640),
            colorScheme: .light
        ) {
            let model = ProjectSettingsModel(
                projectID: Fixtures.projectID,
                fields: ProjectSettingsFields(
                    name: "notion-agent-tracker", workingDir: "/Users/craig/Projects/notion-agent-tracker",
                    color: .teal),
                write: { _ in }, reload: {}, readDefaultBase: { "main" })
            model.edited.mergeMethod = "squash"
            model.edited.deleteBranch = true
            model.edited.baseBranch = "develop"
            model.edited.tag = "nt"
            return ProjectSettingsView(projectName: "notion-agent-tracker", projectTag: "NOT", model: model)
                .defaultScrollAnchor(.center)
        },

        Story(
            name: "settings-sources",
            summary: "The settings window's Sources section: installed plugins (one with an update, "
                + "one manual, one on PATH), what the sources offer, and the sources themselves — "
                + "nat's own marked Default, an extra that could not be read with nat's reason.",
            size: CGSize(width: 760, height: 560),
            colorScheme: .light
        ) {
            SettingsView(appModel: await Fixtures.startedAppModel(), client: FixtureNatClient(), initialTab: .sources)
        },

        Story(
            name: "settings-sources-loading",
            summary: "The Sources section while plugin-list is still out.",
            size: CGSize(width: 760, height: 560),
            colorScheme: .light
        ) {
            SettingsView(
                appModel: await Fixtures.startedAppModel(), client: FixtureNatClient(behaviour: .hanging),
                initialTab: .sources)
        },

        Story(
            name: "settings-sources-error",
            summary: "The Sources section when plugin-list itself failed: nat's reason in place of the groups.",
            size: CGSize(width: 760, height: 560),
            colorScheme: .light
        ) {
            SettingsView(
                appModel: await Fixtures.startedAppModel(),
                client: FixtureNatClient(behaviour: .refusing(
                    "look for task source plugins: open /Users/craig/.config/notion-agent-tracker/plugins: permission denied")),
                initialTab: .sources)
        },

        Story(
            name: "settings-sources-empty",
            summary: "The Sources section on a machine with nothing installed and nothing on offer yet.",
            size: CGSize(width: 760, height: 560),
            colorScheme: .light
        ) {
            SettingsView(
                appModel: await Fixtures.startedAppModel(),
                client: FixtureNatClient(plugins: Fixtures.pluginListingEmpty),
                initialTab: .sources)
        },

        Story(
            name: "settings-sources-setup",
            summary: "The Sources section with Shortcut installed and no token: \u{201C}API token not set\u{201D} "
                + "over an empty secure field, Save disabled, the hint under it — beside a plugin whose "
                + "describe failed, its reason as a warning line.",
            size: CGSize(width: 760, height: 560),
            colorScheme: .light
        ) {
            SettingsView(
                appModel: await Fixtures.startedAppModel(),
                client: FixtureNatClient(plugins: Fixtures.pluginListingShortcut),
                initialTab: .sources)
        },

        Story(
            name: "settings-sources-setup-saved",
            summary: "The same section after a token was saved and plugin-list re-read: \u{201C}API token set\u{201D}, "
                + "the field cleared with a \u{201C}Replace …\u{201D} placeholder, and the plugin's "
                + "\u{201C}Logged in to …\u{201D} under it with a green check.",
            size: CGSize(width: 760, height: 560),
            colorScheme: .light
        ) {
            let client = FixtureNatClient(plugins: Fixtures.pluginListingShortcut)
            let model = PluginsModel(client: client)
            await model.load()
            let key = PluginsModel.SetupKey(plugin: "shortcut", field: "token")
            model.setupValues[key] = "a-token"
            await model.saveSetup(plugin: "shortcut", field: "token")
            return SettingsView(
                appModel: await Fixtures.startedAppModel(), client: client, initialTab: .sources, plugins: model)
        },
    ])
}

/// `PRComposerView` holding a typed comment, which its binding needs a home
/// of its own for.
private struct MarkdownTableStory: View {
    let expanded: Set<Int>

    var body: some View {
        MarkdownTableStoryBody(text: """
            Results of the run:

            | Check | Result | Command | Note |
            |-------|:------:|---------|------|
            | lint | ✓ | `golangci-lint run ./...` | clean on every package, including the generated fixtures |
            | test | ✗ | `swift test --filter PRStoreTests` | `PRStoreTests` timed out waiting on the poll interval |
            """, expanded: expanded)
        .padding(12)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .surface(.window)
    }
}

private struct MarkdownTableStoryBody: View {
    let text: String
    let expanded: Set<Int>

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            ForEach(Array(markdownBlocks(text).enumerated()), id: \.offset) { _, block in
                switch block {
                case .text(let prose):
                    Text(prose.trimmingCharacters(in: .newlines)).font(.system(size: Typo.scaled(13.5))).ink(.primary)
                case .table(let table): MarkdownTableView(table: table, size: 13.5, initiallyExpanded: expanded)
                case .details(let details): MarkdownDetailsView(details: details, size: 13.5, ink: .primary)
                }
            }
        }
    }
}

/// One file of the fixture diff with a line marked, so its comment button
/// shows without a pointer over it.
private struct DiffCommentButtonStory: View {
    var body: some View {
        let file = Fixtures.diffModel.files[0]
        let marked = file.rows.first { $0.kind == .added } ?? file.rows[0]
        var state = DiffCanvasState()
        state.selection = DiffCanvasSelection(path: file.path, rowIDs: [marked.id])
        state.canComment = true
        return DiffCanvasRepresentable(
            files: [file], state: state, attachments: [:], actions: DiffCanvasActions(),
            review: nil, store: nil, authorName: "craig johnston", authorInitials: "CJ")
        .surface(.window)
    }
}

/// The image list on its own, over the handed-back slice's images with the
/// pending comments seeded: the first image zoomed as given, and with
/// `draft`, the comment box open at that point on the image at offset `on`.
/// A zoomed image's sideways scroll starts at `anchor` where given, else its
/// middle.
@MainActor
private enum VisualsPaneStory {
    static func make(zoomFirst: CGFloat, draft: CGPoint?, on: Int = 0, anchor: UnitPoint? = nil) async -> some View {
        let appModel = await Fixtures.startedAppModel(
            client: FixtureNatClient(details: Fixtures.visualsSliceDetails), config: Fixtures.twoProjectConfig)
        let review = VisualReview()
        let store = review.store(appModel)
        store.loader = Fixtures.visualImageLoader
        await store.load(sliceID: Fixtures.mergeBoxSliceID, visuals: Fixtures.visualChanges)
        Fixtures.seedPendingVisualComments(into: store)
        store.setZoom(zoomFirst, sliceID: Fixtures.mergeBoxSliceID, index: 1)
        if let draft, let size = Fixtures.visualPixelSizes[Fixtures.visualChanges[on].uri] {
            review.openDraft(Fixtures.visualChanges[on], point: draft, imageSize: size)
        }
        return VisualsPane(
            appModel: appModel, review: review, slice: Fixtures.slice(Fixtures.mergeBoxSliceID),
            handIn: Fixtures.visualChanges, authorName: "Craig Johnston",
            horizontalAnchor: anchor ?? (zoomFirst > 1 ? .center : .leading))
        .surface(.window)
    }

    /// The image list alone over one pair, a comment pinned on its after,
    /// shown as `show` asks (the divider at the middle where nil) and its
    /// differences highlighted where `highlight` asks.
    static func pair(_ visual: VisualChange, show: VisualCompareSide? = nil, highlight: Bool = false) async -> some View {
        let sliceID = Fixtures.mergeBoxSliceID
        let appModel = await Fixtures.startedAppModel(
            client: FixtureNatClient(details: Fixtures.sliceDetails.merging(
                [sliceID: Fixtures.detail(visuals: [visual])]) { _, new in new }),
            config: Fixtures.twoProjectConfig)
        let review = VisualReview()
        let store = review.store(appModel)
        store.loader = Fixtures.visualImageLoader
        await store.load(sliceID: sliceID, visuals: [visual])
        store.setComment(
            sliceID: sliceID, visual: visual, point: CGPoint(x: 1000, y: 260),
            imageSize: Fixtures.visualPixelSizes[visual.uri] ?? .zero, text: "The checks line now sits flush.")
        if let show { store.show(show, sliceID: sliceID, index: visual.index) }
        if highlight { await store.toggleHighlight(sliceID: sliceID, visual: visual) }
        return VisualsPane(
            appModel: appModel, review: review, slice: Fixtures.slice(sliceID),
            handIn: [visual], authorName: "Craig Johnston")
        .surface(.window)
    }
}

/// The stress diff, jumped to a file halfway down it — the jump is the
/// navigator's own (`DiffReview.requestScroll`), so where it lands is where
/// a click on that file's row would land.
private struct DiffStressStory: View {
    let wrap: Bool
    @State private var review = DiffReview()
    private static let model = Fixtures.stressDiffModel

    var body: some View {
        var state = DiffCanvasState()
        state.wrap = wrap
        if review.scrollRequest == nil { review.requestScroll(to: Self.model.files[150].path) }
        return DiffCanvasRepresentable(
            files: Self.model.files, state: state, attachments: [:], actions: DiffCanvasActions(),
            review: review, store: nil, authorName: "craig johnston", authorInitials: "CJ")
        .surface(.window)
    }
}

private struct DiffFoldsStory: View {
    @State private var review = DiffReview()
    private static let model = Fixtures.diffModel

    var body: some View {
        var state = DiffCanvasState()
        let paths = Self.model.files.map(\.path)
        state.collapsed = [paths[0], paths[1], paths[3]]
        return DiffCanvasRepresentable(
            files: Self.model.files, state: state, attachments: [:], actions: DiffCanvasActions(),
            review: review, store: nil, authorName: "craig johnston", authorInitials: "CJ")
        .surface(.window)
    }
}

private struct PRComposerTypedStory: View {
    @State private var text = "Ship it 🎉 — the worst verdict reads right 👍\nOne nit: the heading wraps 🙈"

    var body: some View {
        PRComposerView(
            placeholder: "Leave a comment on the pull request…",
            text: $text,
            isSending: false,
            error: nil,
            onSend: {}
        )
        .padding(22)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .surface(.window)
    }
}


/// Every state a sidebar dot takes, as rows on the sidebar's own ground.
private struct StateDotsStory: View {
    private let rows: [(String, SliceDisplayState, Bool)] = [
        ("todo", .todo, false),
        ("blocked", .blocked, false),
        ("working — agent live", .working, true),
        ("working — no agent", .working, false),
        ("on standby", .waiting, true),
        ("review", .review, false),
        ("pr open", .pr, false),
        ("done", .done, false),
    ]

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            ForEach(Array(rows.enumerated()), id: \.offset) { _, row in
                HStack(spacing: 7) {
                    StateDot(state: row.1, live: row.2).frame(width: 16)
                    Text(row.0)
                        .font(.system(size: Typo.body))
                        .strikethrough(row.1 == .done)
                        .ink(row.1 == .blocked ? .quaternary : (row.1 == .done ? .tertiary : .primary))
                    Spacer(minLength: 0)
                }
                .padding(.horizontal, 16)
                .frame(height: GnatMetrics.sidebarRowHeight)
            }
            HStack(spacing: 7) {
                Text("folded project, needs you").font(.system(size: Typo.body)).ink(.primary)
                Spacer(minLength: 0)
                Circle().fill(DesignTokens.hot).frame(width: 6, height: 6)
            }
            .padding(.horizontal, 16)
            .frame(height: GnatMetrics.sidebarRowHeight)
        }
        .padding(.vertical, 8)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .surface(.header)
        .environment(\.pulsesPaused, true)
    }
}


