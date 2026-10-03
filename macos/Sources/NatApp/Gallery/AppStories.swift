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
/// pr, blocked, done, plus fixing), then the screens that are not a slice
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
        containerFocus: ContainerFocus? = nil
    ) -> some View {
        WindowShellView(appModel: appModel, sidebarFolds: folds, focus: focus, containerFocus: containerFocus)
            .environment(\.terminalStubbed, true)
            .environment(\.pulsesPaused, true)
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
    private static func sourceSidebar(
        client: FixtureNatClient = FixtureNatClient(), folded: [String: Bool] = ["work": true],
        hoveredContainer: String? = nil
    ) async -> some View {
        let appModel = await Fixtures.startedAppModel(client: client, config: Fixtures.sourceConfig)
        await appModel.selectContainer(Fixtures.sourceCardID, inProject: Fixtures.sourceProjectID)
        return SidebarView(
            appModel: appModel, folded: folded,
            hoveredContainer: hoveredContainer.map { (Fixtures.sourceProjectID, $0) })
            .environment(\.pulsesPaused, true)
    }

    /// The filter editor as it opens from a menu, alone — a real popover is a
    /// window of its own, which no render of the main one can show.
    private static func filterPopover(_ action: SourceAction?) -> some View {
        SourceFilterPopover(action: action ?? Fixtures.sourceFilterAction([:]), onCancel: {}, onApply: { _ in })
            .surface(.window)
    }

    /// The window on one slice, the fixture plan beside the second project's,
    /// with the live readings the fixtures carry, waiting for those readings
    /// to land so the slice is drawn in the state it is a story about.
    private static func slicePane(
        _ sliceID: String, agents: [AgentStatus] = Fixtures.agentStatuses, fixing: Bool = false,
        details: [String: SliceDetail] = Fixtures.sliceDetails, focus: NavigatorFocus? = nil,
        configure: @MainActor (AppModel) async -> Void = { _ in }
    ) async -> some View {
        let appModel = await Fixtures.startedAppModel(
            client: FixtureNatClient(agents: agents, details: details), config: Fixtures.twoProjectConfig)
        if fixing { appModel.markFixLaunched(sliceID: sliceID) }
        appModel.selectedSliceID = sliceID
        for _ in 0..<50 where !agents.isEmpty && appModel.activityStore?.agents.isEmpty != false {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        await appModel.sliceDetailStore(projectID: Fixtures.projectID).fetch(sliceRef: sliceID)
        let store = appModel.diffStore(projectID: Fixtures.projectID)
        let slice = Fixtures.slice(sliceID)
        if slice.handedBack || !(slice.branch ?? "").isEmpty {
            await store.fetch(projectID: Fixtures.projectID, sliceRef: sliceID)
        }
        if !slice.pr.isEmpty {
            await appModel.prStore(projectID: Fixtures.projectID).fetch(projectID: Fixtures.projectID, sliceRef: sliceID)
        }
        await configure(appModel)
        return shell(appModel, focus: focus)
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
        _ proposal: PlanProposal = Fixtures.proposal, accepting: Bool, scrollTo: String? = nil
    ) async -> some View {
        let client = FixtureNatClient(agents: Fixtures.agentStatusesWithPlanner)
        client.setProposal(proposal, forProject: Fixtures.projectID)
        let appModel = await Fixtures.startedAppModel(client: client)
        await settleOnPlanner(appModel)
        appModel.workshopSelected = true
        await appModel.refreshProposals()
        if let scrollTo { appModel.showProposedSlice(scrollTo) }
        if accepting {
            client.holdAccepts()
            await startHeld { await appModel.acceptProposal() }
        }
        return shell(appModel)
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
                store.toggleViewed(sliceID: sliceID, index: 1)
            }
        }
    }

    /// The fixture plan with a dozen more slices in flight — what a sidebar
    /// with more running than fits looks like.
    /// A titlebar band story's width: a 330pt navigator beside a 730pt
    /// main pane, the window less its sidebar.
    private static let bandWidth = GnatMetrics.navigatorWidth + 730

    private static let longBandTitle =
        "Rework the navigator and main pane titlebars into one band, tabs right-aligned, the title ellipsizing into the gap"

    private static let bandAgent = AgentStatus(
        sliceID: Fixtures.diffPaneSliceID, session: "nat-1", activity: .working,
        model: "Sonnet 5", effort: "high", contextPercent: 42, contextTokens: 84_120)

    /// The titlebar band as the shell lays it out over a 330pt navigator:
    /// the breadcrumb, its last crumb a live `GNA` selection (or `identity`),
    /// then the tabs at the trailing edge.
    private static func band(
        tabs: [MainPaneTab], selected: MainPaneMode?, crumbs: TitlebarCrumbs, state: SliceDisplayState = .working,
        identity: TitlebarIdentity? = nil
    ) -> some View {
        TitlebarBand(
            navigatorWidth: GnatMetrics.navigatorWidth, tabs: tabs.map(\.titlebarTab),
            selected: tabs.first { $0.mode == selected }?.titlebarTab.id
        ) {
            TitlebarBreadcrumb(
                crumbs: crumbs,
                identity: identity ?? TitlebarIdentity(tag: "GNA", state: state, live: true, title: crumbs.title),
                openPicker: .constant(nil)
            ) { _ in EmptyView() }
        }
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

    /// A scratch project with nothing in it.
    private static let emptyScratchPlan = ProjectInfo(
        project: Project(id: Fixtures.scratchProjectID, name: "Scratch", conventions: ""),
        milestones: [], slices: [])

    nonisolated private static func scratchClient(_ scratchPlan: ProjectInfo) -> FixtureNatClient {
        FixtureNatClient(otherPlans: [
            Fixtures.secondProjectID: Fixtures.secondProjectInfo, Fixtures.scratchProjectID: scratchPlan,
        ])
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

    /// An app model on the activity slice with its three follow-ups pending
    /// and its agent waiting, the choices given already made.
    private static func followUpsModel(choices: [Int: FollowUpChoice]) async -> AppModel {
        let sliceID = Fixtures.activitySliceID
        let appModel = await Fixtures.startedAppModel(
            client: FixtureNatClient(agents: Fixtures.agentStatuses, details: Fixtures.followUpsSliceDetails),
            config: Fixtures.twoProjectConfig)
        appModel.selectedSliceID = sliceID
        await appModel.sliceDetailStore(projectID: Fixtures.projectID).fetch(sliceRef: sliceID)
        for _ in 0..<50 where appModel.activityStore?.agents[sliceID] == nil {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        for (index, choice) in choices {
            appModel.followUpStore.setChoice(choice, sliceID: sliceID, index: index)
        }
        return appModel
    }

    static let catalog = StoryCatalog([

        // MARK: - The window, one slice in each phase

        Story(
            name: "window-review",
            summary: "A handed-back slice: Changes open with Send and Approve, the continuous diff in the main pane.",
            size: window
        ) {
            await slicePane(Fixtures.mergeBoxSliceID)
        },

        Story(
            name: "window-review-comments",
            summary: "The same review with comments pending: the count on Send, the dot on the file row, the inline cards in the diff.",
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
            summary: "A blocked slice: its dot hollow and dim, Launch disabled, and the Thread's launch card greyed and hatched over what it waits on.",
            size: window
        ) {
            await slicePane(Fixtures.cacheSliceID)
        },

        Story(
            name: "window-blocked-several",
            summary: "A slice blocked on two slices with a third already done: the brief's depends list one row apiece, each with its dot, and the launch card naming the two still open.",
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
            summary: "A slice in progress whose agent is gone: the Thread's log, then the launch card offering Relaunch.",
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
            summary: "An approved slice: PR open on its checks and review, Merge in the header, its description and conversation in the main pane.",
            size: window
        ) {
            await slicePane(Fixtures.approveSliceID)
        },

        Story(
            name: "window-fixing",
            summary: "An approved slice with a fix session on it: Thread and the terminal, as working.",
            size: window
        ) {
            await slicePane(Fixtures.approveSliceID, agents: Fixtures.agentStatuses + [
                AgentStatus(sliceID: Fixtures.approveSliceID,
                            session: TmuxSession.name(forSlicePageID: Fixtures.approveSliceID), activity: .working)
            ], fixing: true)
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
            summary: "A waiting agent that proposed three follow-ups: their cards in the Thread, Apply in its header.",
            size: window
        ) {
            shell(await followUpsModel(choices: [1: .queue, 2: .fold, 3: .drop]))
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
            name: "workshop-proposal-revision",
            summary: "A project's proposal filing slices into milestones it already has: the new milestone first, then each existing one holding only its proposed slices; the count names what Accept creates; the Plan tab up, only the new milestone marked NEW.",
            size: window
        ) {
            await projectProposalShell(Fixtures.revisionProposal, accepting: false)
        },

        Story(
            name: "workshop-accepting",
            summary: "A project's proposal mid-Accept: Accept busy, Keep workshopping disabled.",
            size: window
        ) {
            await projectProposalShell(accepting: true)
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
            summary: "A merged task's log: handed back three times, sent back twice, two follow-ups triaged, then approved and merged.",
            size: window
        ) {
            await slicePane(
                Fixtures.shellSliceID, agents: [], details: Fixtures.taskLogSliceDetails,
                focus: NavigatorFocus(open: [.thread], main: .diff))
        },

        Story(
            name: "window-task-log-notes",
            summary: "An in-progress task's log with two notes on its brief, each headed \"Another agent left a note\": one from a task on the plan, its task fact the depends-on row (dot, name, hover, click to go), and one from a person, its source fact plain text. Each card is stamped at its header's end — the time for today's, the day for this year's, the year too for last year's.",
            size: window
        ) {
            await slicePane(
                Fixtures.activitySliceID, agents: [], details: Fixtures.notedSliceDetails,
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
            summary: "An Untitled project after Workshop the plan: the planning agent's terminal and Brief alone, no Plan section until a proposal.",
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
            summary: "The sidebar over two projects: Active needs-you first, the active project's tree open, the other folded with its hot dot.",
            size: sidebar
        ) {
            let appModel = await Fixtures.startedAppModel(config: Fixtures.twoProjectConfig)
            appModel.selectedSliceID = Fixtures.mergeBoxSliceID
            return SidebarView(appModel: appModel).environment(\.pulsesPaused, true)
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
                + "fixing, waiting, review, pr open and done, with a folded project's needs-you dot.",
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
                + "its icon and header menu, Doing with its cards and their tasks, the Mine and Board segments "
                + "as top-level groups (one card in both), the lazy Done folded with its count; the first card "
                + "selected, and its badges where the + would be under the pointer. Projects, folded, pins to the foot.",
            size: sidebar
        ) {
            await sourceSidebar()
        },

        Story(
            name: "sidebar-source-hover",
            summary: "A card row under the pointer: its estimate shows, and the + takes the badge's place "
                + "rather than pushing it left — the title does not move.",
            size: sidebar
        ) {
            await sourceSidebar(hoveredContainer: Fixtures.sourceSecondCardID)
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
            summary: "A card selected: the source's icon and tag in the titlebar, Story open with its facts "
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
            summary: "A Todo task under a card: the brief card's facts lead with the card, opening in its "
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
            summary: "A segment's Filter…: Team, Project, Epic and Labels over the workspace's choices, "
                + "opened on the segment's own (team Board); where the section sets a field, Any names "
                + "what it falls through to (\u{201C}Any (section\u{2019}s: Mobile App)\u{201D}).",
            size: CGSize(width: 360, height: 240)
        ) {
            filterPopover(Fixtures.sourceGroups()[1].menu.first { $0.input == .filter })
        },

        Story(
            name: "source-filter-popover-section",
            summary: "The section header's Filter…: the same four fields, narrowing every list the fold "
                + "draws (here, the Mobile App project), with nothing wider to fall through to.",
            size: CGSize(width: 360, height: 240)
        ) {
            filterPopover(Fixtures.sourceInfo().menu.first { $0.input == .filter })
        },

        Story(
            name: "source-filter-popover-loading",
            summary: "The editor opened before the plugin's background fetch of the epic list has landed: "
                + "Epic says it is loading, and the other three fields work regardless.",
            size: CGSize(width: 360, height: 240)
        ) {
            filterPopover(Fixtures.sourceGroups(epicsLoading: true)[1].menu.first { $0.input == .filter })
        },

        // MARK: - The titlebar band

        Story(
            name: "titlebar-band-slice",
            summary: "The titlebar band over a slice: the breadcrumb at the navigator\u{2019}s inset \u{2014} project, "
                + "milestone, then the slice\u{2019}s dot and title with no project tag, the project crumb naming it "
                + "already \u{2014} no rule at the split, the tabs at the trailing edge and nothing beside them.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(tabs: [.terminal, .changes, .pr], selected: .terminal, crumbs: sliceCrumbs("Draw the box"))
        },

        Story(
            name: "titlebar-band-long-title",
            summary: "A long task name runs on past the navigator\u{2019}s width into the gap, the tabs still at "
                + "the right and never left of the split.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(tabs: [.terminal, .changes, .pr], selected: .diff, crumbs: sliceCrumbs(longBandTitle))
        },

        Story(
            name: "titlebar-band-long-title-narrow",
            summary: "The same band in a narrower window: the last crumb\u{2019}s title gives way first, ending in "
                + "an ellipsis with the chevron beside it.",
            size: CGSize(width: 760, height: GnatMetrics.titlebarHeight)
        ) {
            band(tabs: [.terminal, .changes, .pr], selected: .diff, crumbs: sliceCrumbs(longBandTitle))
        },

        Story(
            name: "titlebar-band-workshop",
            summary: "The workshop\u{2019}s band: the project\u{2019}s name then Workshop, no tag, and no tabs.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: [], selected: nil,
                crumbs: TitlebarCrumbs(parent: Fixtures.project.name, parentKind: .project, title: workshopRowTitle))
        },

        Story(
            name: "titlebar-band-session",
            summary: "An ad hoc session\u{2019}s band: the project\u{2019}s name then the session, and its own tabs.",
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
            summary: "A source task\u{2019}s band: its container crumb with the card mark, then the task \u{2014} "
                + "tag kept, no project crumb before it.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: [.terminal, .changes], selected: .terminal,
                crumbs: TitlebarCrumbs(parent: "Billing export", parentKind: .container, title: "Add the CSV column"),
                identity: TitlebarIdentity(tag: "SC", state: .working, live: true, title: "Add the CSV column"))
        },

        Story(
            name: "titlebar-band-container",
            summary: "A container\u{2019}s band: the project crumb, then the source\u{2019}s icon and the "
                + "container\u{2019}s title, no tag; no tabs and no trailing items.",
            size: CGSize(width: bandWidth, height: GnatMetrics.titlebarHeight)
        ) {
            band(
                tabs: [], selected: nil,
                crumbs: TitlebarCrumbs(project: Fixtures.project.name, title: "Billing export"),
                identity: .container(title: "Billing export", tag: "SC", icon: SourceIcon(symbol: "rectangle.stack")))
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
                + "the agent\u{2019}s model, effort and context in small mono at the trailing edge, no breadcrumb.",
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
            summary: "The same readout at 91% context: the percent switches to the warning tint.",
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
            name: "pr-composer-typed",
            summary: "The comment box with an emoji comment typed into it: the editor grown to "
                + "its two lines, under its 60pt ceiling, rather than sitting at either bound.",
            size: CGSize(width: 560, height: 140)
        ) {
            PRComposerTypedStory()
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
                """, size: 13.5)
            .padding(12)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
            .surface(.window)
        },

        // MARK: - Follow-ups

        // MARK: - Settings

        Story(
            name: "settings",
            summary: "The settings window's General tab over the fixture config.",
            size: CGSize(width: 520, height: 560),
            colorScheme: .light
        ) {
            SettingsView(appModel: await Fixtures.startedAppModel(), client: FixtureNatClient())
        },

        Story(
            name: "settings-agents",
            summary: "The settings window's Agents tab: the model field is a menu picker "
                + "now, over AgentOptions' own alias set, matching the effort picker's "
                + "own shape.",
            size: CGSize(width: 520, height: 360),
            colorScheme: .light
        ) {
            SettingsView(appModel: await Fixtures.startedAppModel(), client: FixtureNatClient(), initialTab: .agents)
        },

        Story(
            name: "settings-agents-custom-model",
            summary: "The same tab with a full model ID already configured: the picker "
                + "selects Custom on its own and shows the ID in the field beneath it.",
            size: CGSize(width: 520, height: 360),
            colorScheme: .light
        ) {
            let client = FixtureNatClient(config: Fixtures.configDocWithCustomModel)
            return SettingsView(appModel: await Fixtures.startedAppModel(client: client), client: client, initialTab: .agents)
        },

        Story(
            name: "settings-sources",
            summary: "The settings window's Sources tab: installed plugins (one with an update, "
                + "one manual, one on PATH), what the sources offer, and the sources themselves — "
                + "nat's own marked Default, an extra that could not be read with nat's reason.",
            size: CGSize(width: 520, height: 820),
            colorScheme: .light
        ) {
            SettingsView(appModel: await Fixtures.startedAppModel(), client: FixtureNatClient(), initialTab: .sources)
        },

        Story(
            name: "settings-sources-loading",
            summary: "The Sources tab while plugin-list is still out.",
            size: CGSize(width: 520, height: 200),
            colorScheme: .light
        ) {
            SettingsView(
                appModel: await Fixtures.startedAppModel(), client: FixtureNatClient(behaviour: .hanging),
                initialTab: .sources)
        },

        Story(
            name: "settings-sources-error",
            summary: "The Sources tab when plugin-list itself failed: nat's reason in place of the groups.",
            size: CGSize(width: 520, height: 200),
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
            summary: "The Sources tab on a machine with nothing installed and nothing on offer yet.",
            size: CGSize(width: 520, height: 480),
            colorScheme: .light
        ) {
            SettingsView(
                appModel: await Fixtures.startedAppModel(),
                client: FixtureNatClient(plugins: Fixtures.pluginListingEmpty),
                initialTab: .sources)
        },

        Story(
            name: "settings-sources-setup",
            summary: "The Sources tab with Shortcut installed and no token: \u{201C}API token not set\u{201D} "
                + "over an empty secure field, Save disabled, the hint under it — beside a plugin whose "
                + "describe failed, its reason as a warning line.",
            size: CGSize(width: 520, height: 620),
            colorScheme: .light
        ) {
            SettingsView(
                appModel: await Fixtures.startedAppModel(),
                client: FixtureNatClient(plugins: Fixtures.pluginListingShortcut),
                initialTab: .sources)
        },

        Story(
            name: "settings-sources-setup-saved",
            summary: "The same tab after a token was saved and plugin-list re-read: \u{201C}API token set\u{201D}, "
                + "the field cleared with a \u{201C}Replace …\u{201D} placeholder, and the plugin's "
                + "\u{201C}Logged in to …\u{201D} under it with a green check.",
            size: CGSize(width: 520, height: 620),
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
                    Text(prose.trimmingCharacters(in: .newlines)).font(.system(size: 13.5)).ink(.primary)
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
/// `draft`, the comment box open at that point on it.
@MainActor
private enum VisualsPaneStory {
    static func make(zoomFirst: CGFloat, draft: CGPoint?) async -> some View {
        let appModel = await Fixtures.startedAppModel(
            client: FixtureNatClient(details: Fixtures.visualsSliceDetails), config: Fixtures.twoProjectConfig)
        let review = VisualReview()
        let store = review.store(appModel)
        store.loader = Fixtures.visualImageLoader
        await store.load(sliceID: Fixtures.mergeBoxSliceID, visuals: Fixtures.visualChanges)
        Fixtures.seedPendingVisualComments(into: store)
        store.setZoom(zoomFirst, sliceID: Fixtures.mergeBoxSliceID, index: 1)
        if let draft, let size = Fixtures.visualPixelSizes[Fixtures.visualChanges[0].uri] {
            review.openDraft(Fixtures.visualChanges[0], point: draft, imageSize: size)
        }
        return VisualsPane(
            appModel: appModel, review: review, slice: Fixtures.slice(Fixtures.mergeBoxSliceID),
            visuals: Fixtures.visualChanges, authorName: "Craig Johnston",
            horizontalAnchor: zoomFirst > 1 ? .center : .leading)
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
        ("fixing", .fixing, true),
        ("waiting for you", .waiting, true),
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
                        .font(.system(size: 14))
                        .strikethrough(row.1 == .done)
                        .ink(row.1 == .blocked ? .quaternary : (row.1 == .done ? .tertiary : .primary))
                    Spacer(minLength: 0)
                }
                .padding(.horizontal, 16)
                .frame(height: GnatMetrics.sidebarRowHeight)
            }
            HStack(spacing: 7) {
                Text("folded project, needs you").font(.system(size: 14)).ink(.primary)
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


