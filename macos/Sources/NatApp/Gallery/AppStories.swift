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
/// The catalog is the app's own shape: the window first, then the rail in
/// each of the states a load leaves it in, then one pane per tab of the
/// workflow, then the two screens that are neither — the workshop's composer
/// and the settings window. `--list` reads down it in that order, which is
/// what makes it an index rather than a heap.
///
/// Two regions are drawn rather than run — the agent terminal and the
/// onboarding checklist; see `StorySeams` for why and for how a story says
/// so. Adding a story is an entry in this array and nothing else.
/// `@MainActor` because a story's content is: every one of these builds a
/// view over a fixture app model, and both are the main actor's.
@MainActor
enum AppStories {
    /// The mock's own canvas size — every metric in the shell was chosen at
    /// 1360×840, so this is the one size a whole window is worth drawing at.
    private static let window = CGSize(width: 1360, height: 840)

    /// A pane on its own: wide enough for the diff's sidebar and its widest
    /// fixture line, tall enough for more than one file box.
    private static let pane = CGSize(width: 1040, height: 680)

    /// The rail at the width the window gives it, over the window's full
    /// height — a rail drawn shorter says nothing true about how much of the
    /// plan is in view.
    private static let rail = CGSize(width: 372, height: 840)

    /// The fixture plan with a dozen more slices in flight — what a rail
    /// with more running than fits looks like. The fixture plan's own six
    /// sit inside any share the rail hands out; twice that is what the
    /// sharing is for.
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

    static let catalog = StoryCatalog([

        // MARK: - The window

        Story(
            name: "window-shell",
            summary: "The whole window on a loaded project, a handed-back slice selected.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel()
            appModel.selectedSliceID = Fixtures.mergeBoxSliceID
            return WindowShellView(appModel: appModel)
        },

        Story(
            name: "window-rail-crowded",
            summary: "The whole window on a plan taller than it: the rail stays the "
                + "window\u{2019}s height and scrolls within it rather than stretching "
                + "the shell to fit the plan.",
            size: window
        ) {
            // The rail stories draw the rail as the root of their own window,
            // where it is laid out at exactly the size it is given whatever
            // its column comes to — so the one thing they cannot show is a
            // rail pushing the shell out of shape. This is that: the rail
            // inside the shell\u{2019}s own HStack, on a plan with more in
            // flight than the window has room for.
            WindowShellView(appModel: await Fixtures.startedAppModel(
                client: FixtureNatClient(plan: crowdedPlan, agents: Fixtures.agentStatuses)))
        },

        Story(
            name: "window-no-selection",
            summary: "The same window with nothing selected — the pane's own empty state.",
            size: window
        ) {
            WindowShellView(appModel: await Fixtures.startedAppModel())
        },

        Story(
            name: "window-workshop",
            summary: "The window with the workshop entry selected and a planning agent live.",
            size: window
        ) {
            let appModel = await Fixtures.startedAppModel(
                client: FixtureNatClient(agents: Fixtures.agentStatusesWithPlanner))
            appModel.workshopSelected = true
            return WindowShellView(appModel: appModel)
                .environment(\.terminalStubbed, true)
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

        // MARK: - The header

        Story(
            name: "project-tabs",
            summary: "The project tab strip: the attention dot and the count of what wants the user.",
            size: CGSize(width: 640, height: 40)
        ) {
            ProjectTabsView(
                appModel: await Fixtures.startedAppModel(
                    client: FixtureNatClient(agents: Fixtures.agentStatusesWithPlanner)),
                onNewProject: {}
            )
        },

        Story(
            name: "project-tabs-multiple",
            summary: "The tab strip with a second project open: the close button on the "
                + "active tab, with the count pill seated against it.",
            size: CGSize(width: 640, height: 40)
        ) {
            ProjectTabsView(
                appModel: await Fixtures.startedAppModel(
                    client: FixtureNatClient(agents: Fixtures.agentStatusesWithPlanner),
                    config: Fixtures.twoProjectConfig
                ),
                onNewProject: {}
            )
        },

        // MARK: - The rail

        Story(
            name: "rail-skeleton",
            summary: "The rail on a first load that has not landed — the plan's own placeholder.",
            size: rail
        ) {
            // The one state that needs a client which never answers: a
            // skeleton is what a load looks like while it is still in
            // flight, and a fixture that answers instantly has no such
            // moment to catch.
            RailView(appModel: Fixtures.loadingAppModel())
        },

        Story(
            name: "rail-loaded",
            summary: "The rail on the fixture plan: the ACTIVE section, milestones, done.",
            size: rail
        ) {
            RailView(appModel: await Fixtures.startedAppModel())
        },

        Story(
            name: "rail-workshop",
            summary: "The one ACTIVE section with a planning agent live: the workshop entry, "
                + "the branches awaiting review, then the slices being worked.",
            size: rail
        ) {
            let appModel = await Fixtures.startedAppModel(
                client: FixtureNatClient(agents: Fixtures.agentStatusesWithPlanner))
            appModel.workshopSelected = true
            return RailView(appModel: appModel)
        },

        Story(
            name: "rail-active-crowded",
            summary: "A dozen slices in flight: ACTIVE scrolls within its share of the "
                + "rail rather than pushing TODO and DONE off it.",
            size: rail
        ) {
            RailView(appModel: await Fixtures.startedAppModel(
                client: FixtureNatClient(plan: crowdedPlan, agents: Fixtures.agentStatuses)))
        },

        Story(
            name: "rail-folded",
            summary: "ACTIVE and TODO folded away to their headings: the three titles "
                + "hold their places and DONE takes the space the other two gave back.",
            size: rail
        ) {
            RailView(
                appModel: await Fixtures.startedAppModel(),
                collapsedSections: [.active, .todo]
            )
        },

        Story(
            name: "rail-sections-open",
            summary: "All three sections open on a crowded plan: each heading pinned "
                + "over a scroll of its own, the rail shared between them.",
            size: rail
        ) {
            RailView(
                appModel: await Fixtures.startedAppModel(
                    client: FixtureNatClient(plan: crowdedPlan, agents: Fixtures.agentStatuses)),
                collapsedSections: []
            )
        },

        Story(
            name: "rail-empty",
            summary: "The rail of a project with nothing queued into it yet.",
            size: rail
        ) {
            RailView(appModel: await Fixtures.startedAppModel(
                client: FixtureNatClient(plan: Fixtures.emptyProjectInfo, agents: [])))
        },

        Story(
            name: "rail-error",
            summary: "The rail when the first read of the plan failed, with the retry.",
            size: rail
        ) {
            RailView(appModel: await Fixtures.startedAppModel(
                client: FixtureNatClient(behaviour: .refusing(Fixtures.loadErrorMessage))))
        },

        // MARK: - The workflow's tabs

        Story(
            name: "brief-handed-back",
            summary: "The Brief tab of a slice whose branch is waiting to be reviewed.",
            size: pane
        ) {
            let appModel = await Fixtures.startedAppModel()
            return BriefTabView(appModel: appModel, slice: Fixtures.slice(Fixtures.mergeBoxSliceID))
                .surface(.window)
        },

        Story(
            name: "brief-blocked",
            summary: "The Brief tab of a slice still waiting on what it depends on.",
            size: pane
        ) {
            let appModel = await Fixtures.startedAppModel()
            return BriefTabView(appModel: appModel, slice: Fixtures.slice(Fixtures.cacheSliceID))
                .surface(.window)
        },

        Story(
            name: "agent-terminal",
            summary: "The Agent tab with a session attached — the terminal region is drawn, not run.",
            size: pane
        ) {
            let appModel = await Fixtures.startedAppModel()
            appModel.selectedSliceID = Fixtures.diffPaneSliceID
            return AgentTabView(appModel: appModel, slice: Fixtures.slice(Fixtures.diffPaneSliceID))
                .environment(\.terminalStubbed, true)
        },

        Story(
            name: "diff-handed-back",
            summary: "The Diff tab: the handed-back branch, one box per file, beside its file list.",
            size: pane
        ) {
            let appModel = await Fixtures.startedAppModel()
            return DiffTabView(appModel: appModel, slice: Fixtures.slice(Fixtures.mergeBoxSliceID))
                .surface(.window)
        },

        Story(
            name: "diff-pending-comments",
            summary: "The same diff with a review left on it and not yet sent.",
            size: pane
        ) {
            let appModel = await Fixtures.startedAppModel()
            let slice = Fixtures.slice(Fixtures.mergeBoxSliceID)
            // The comments are the store's own state rather than anything a
            // reading carries, so the branch is read here first and the
            // review left on the rows it came back with — exactly the order
            // a user reaches this state in.
            let store = appModel.diffStore(projectID: Fixtures.projectID)
            await store.fetch(projectID: Fixtures.projectID, sliceRef: slice.id)
            Fixtures.seedPendingComments(into: store)
            return DiffTabView(appModel: appModel, slice: slice)
                .surface(.window)
        },

        Story(
            name: "pr-ready-to-merge",
            summary: "The PR tab on a green pull request — the merge box says yes.",
            size: pane
        ) {
            let appModel = await Fixtures.startedAppModel()
            return PRTabView(appModel: appModel, slice: Fixtures.slice(Fixtures.approveSliceID))
                .surface(.window)
        },

        Story(
            name: "pr-failing-checks",
            summary: "The PR tab with checks red: the rollup and the verdict that refuses the merge.",
            size: pane
        ) {
            let appModel = await Fixtures.startedAppModel(
                client: FixtureNatClient(pr: Fixtures.prFailingChecks))
            return PRTabView(appModel: appModel, slice: Fixtures.slice(Fixtures.approveSliceID))
                .surface(.window)
        },

        Story(
            name: "pr-conflicting",
            summary: "The PR tab on a branch that conflicts with its base.",
            size: pane
        ) {
            let appModel = await Fixtures.startedAppModel(
                client: FixtureNatClient(pr: Fixtures.prConflicting))
            return PRTabView(appModel: appModel, slice: Fixtures.slice(Fixtures.approveSliceID))
                .surface(.window)
        },

        // MARK: - The screens that are neither

        Story(
            name: "workshop-composer",
            summary: "The workshop pane with no session running: the request to start one on.",
            size: pane
        ) {
            WorkshopPaneView(appModel: await Fixtures.startedAppModel())
        },

        // Drawn in the light palette because the settings window is: it
        // follows the Mac rather than the app's own theme (see `NatApp`), and
        // the machine a reference is rendered on has no theme to follow.
        // The tab strip above the form is the settings *scene's* toolbar,
        // which a window made for a capture has none of — the form under it
        // is the whole of what this story is for.
        Story(
            name: "settings",
            summary: "The settings window's General tab over the fixture config.",
            size: CGSize(width: 520, height: 560),
            colorScheme: .light
        ) {
            SettingsView(appModel: await Fixtures.startedAppModel(), client: FixtureNatClient())
        },
    ])
}
