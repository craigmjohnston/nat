import AppKit
import SwiftUI
import NatKit
import NatFixtures

/// The window, as the gnat design lays it out: the sidebar, the navigator
/// and the main pane side by side over the status bar — the sidebar under
/// its own segment of the 32pt titlebar band, the navigator and main pane
/// under one band between them (`TitlebarBand`).
///
/// What the navigator has open and what the main pane shows belong to the
/// selection, so they are held here, between the two: each starts at the
/// selection's own default (`NavigatorModel.defaultOpen`/`defaultMain`) and
/// follows the user's clicks from there, and both go back to the default
/// when the selection changes or the slice moves to another phase.
struct WindowShellView: View {
    @Bindable var appModel: AppModel
    @State private var showNewProjectSheet = false

    @AppStorage("sidebarWidth") private var sidebarWidth = GnatMetrics.sidebarWidth
    @AppStorage("navigatorWidth") private var navigatorWidth = GnatMetrics.navigatorWidth
    @State private var liveSidebarWidth: Double?
    @State private var liveNavigatorWidth: Double?

    @State private var review = DiffReview()
    @State private var visualReview = VisualReview()
    @State private var launchModel = ""
    @State private var launchEffort = ""
    @State private var openOverride: Set<NavigatorSection>?
    @State private var mainOverride: MainPaneMode?
    /// A selected source container's open sections and main pane, once the
    /// user has moved them off its defaults.
    @State private var containerFocusOverride: ContainerFocus?
    /// The container the navigator's New task asked for a task on.
    @State private var newTaskContainer: String?
    /// Which crumb's tree picker is open: the project's, the milestone's or
    /// the selection's own.
    @State private var crumbPicker: CrumbPickerOrigin?

    /// The gallery's seam: a story seeds the sidebar folds it is a story
    /// about.
    var sidebarFolds: [String: Bool] = [:]
    /// The gallery's seam: a story opens the sections and puts up the view
    /// it is a story about, where the selection's defaults would not.
    var focus: NavigatorFocus?
    /// The same seam for a selected source container.
    var containerFocus: ContainerFocus?

    var body: some View {
        ZStack {
            DesignTokens.fill(.window)
                .ignoresSafeArea()

            if appModel.needsOnboarding {
                OnboardingView(appModel: appModel, onNewProject: { showNewProjectSheet = true })
            } else {
                board
            }
        }
        // With the system title bar hidden, SwiftUI still reserves its height
        // as a top safe-area inset by default; the titlebars are drawn there.
        .ignoresSafeArea(.container, edges: .top)
        .background(DefaultCursorView().ignoresSafeArea())
        // The traffic lights, recentred in the 32pt titlebar band.
        .background(TrafficLightAlignerView(headerHeight: GnatMetrics.titlebarHeight))
        .sheet(isPresented: $showNewProjectSheet) {
            NewProjectSheetView(
                onClose: { showNewProjectSheet = false },
                onAdded: { id, name in
                    showNewProjectSheet = false
                    let untitled = appModel.activeTabIsUntitled ? appModel.activeProjectID : nil
                    Task { await appModel.addProject(id: id, name: name, replacing: untitled) }
                }
            )
        }
        .task {
            await appModel.start()
        }
        .focusedSceneValue(\.shellMenu, ShellMenuActions(
            newProject: { appModel.openUntitledTab() },
            refresh: { Task { await appModel.refresh() } }))
    }

    private var board: some View {
        VStack(spacing: 0) {
            HStack(spacing: 0) {
                SidebarView(
                    appModel: appModel, onNewProject: { appModel.openUntitledTab() }, showsTitlebar: true,
                    folded: sidebarFolds)
                    .frame(width: liveSidebarWidth ?? sidebarWidth)
                    .zIndex(1)

                selectionColumns
                    .overlay(alignment: .leading) {
                        PaneResizeHandle(
                            width: sidebarWidth, liveWidth: $liveSidebarWidth, onCommit: { sidebarWidth = $0 },
                            minWidth: 200, maxWidth: 420, edge: .trailing)
                            .offset(x: -4.5)
                    }
            }
            .frame(maxHeight: .infinity)

            StatusBarView(appModel: appModel) { AgentModelHeading(agent: selectionAgent) }
        }
        .onAppear {
            if let focus {
                openOverride = focus.open
                mainOverride = focus.main
            }
            if let containerFocus { containerFocusOverride = containerFocus }
        }
        .sheet(item: Binding(
            get: { newTaskContainer.map { NewTaskTarget(containerID: $0) } },
            set: { newTaskContainer = $0?.containerID }
        )) { target in
            NewSliceSheetView(
                projectID: appModel.activeProjectID ?? "",
                milestones: [],
                container: NewSliceSheetView.Container(
                    id: target.containerID,
                    title: appModel.containerTitle(target.containerID, inProject: appModel.activeProjectID ?? ""),
                    noun: appModel.source(ofProject: appModel.activeProjectID ?? "")?.containerNoun ?? "container"),
                onClose: { newTaskContainer = nil },
                onCreated: {
                    newTaskContainer = nil
                    Task { await appModel.refresh() }
                })
        }
        .onChange(of: selectionKey) { _, _ in resetToDefaults() }
        .onChange(of: slicePhase) { _, _ in resetToDefaults() }
        .alert(
            "The run could not start",
            isPresented: Binding(get: { appModel.runError != nil }, set: { if !$0 { appModel.dismissRunError() } })
        ) {
            Button("OK", role: .cancel) { appModel.dismissRunError() }
        } message: {
            Text(appModel.runError ?? "")
        }
    }

    // MARK: - The titlebar

    /// The band over the navigator and the main pane: where the selection
    /// sits (`TitlebarBreadcrumb`) — its last crumb named as its Active row
    /// names it, state dot and title — each crumb opening the tree picker on
    /// itself, then the main pane's tabs.
    private var titlebar: some View {
        TitlebarBand(
            navigatorWidth: liveNavigatorWidth ?? navigatorWidth, tabs: titlebarTabs, selected: selectedTabID,
            onTab: showTitlebarTab
        ) {
            TitlebarBreadcrumb(crumbs: crumbs, identity: titlebarIdentity, openPicker: $crumbPicker) { origin in
                crumbTreePicker(openingOn: origin)
            }
        }
    }

    /// The agent the selection has, for the status bar's readout: a slice's,
    /// an ad hoc session's, the planning agent on the workshop; none for a
    /// container or with nothing selected.
    private var selectionAgent: AgentStatus? {
        if appModel.activeTabIsUntitled && !appModel.untitledWorkshopVisible { return nil }
        if appModel.workshopSelected || appModel.untitledWorkshopVisible { return appModel.planningAgent }
        if let session = selectedSession { return appModel.activityStore?.agents[session.tag] }
        if let slice = selectedSlice { return appModel.activityStore?.agents[slice.id] }
        return nil
    }

    /// The breadcrumb's last crumb's tag, dot and title for a selected slice,
    /// workshop or session; nil with nothing selected and on the Untitled
    /// starter, whose segment stays as it is.
    private var titlebarIdentity: TitlebarIdentity? {
        if appModel.activeTabIsUntitled && !appModel.untitledWorkshopVisible { return nil }
        if appModel.workshopSelected || appModel.untitledWorkshopVisible {
            return appModel.titlebarIdentity(for: .workshop)
        }
        if let session = selectedSession {
            return appModel.titlebarIdentity(for: .session(id: session.id, title: crumbs.title))
        }
        if let slice = selectedSlice, let navigatorModel {
            return appModel.titlebarIdentity(for: .slice(id: slice.id, name: slice.name, state: navigatorModel.state))
        }
        if let containerID = appModel.selectedContainerID {
            let source = appModel.source(ofProject: appModel.activeProjectID ?? "")
            return appModel.titlebarIdentity(for: .container(
                id: containerID, title: crumbs.title, tag: source?.tag ?? "",
                icon: source?.icon ?? SourceIcon(symbol: "")))
        }
        return nil
    }

    /// The main pane's tabs, at the band's trailing edge — one per view the
    /// navigator's sections can put up, for a slice or a session; none
    /// otherwise.
    private var tabs: [MainPaneTab] {
        if let navigatorModel { return navigatorModel.tabs }
        if let session = selectedSession { return MainPaneTab.forSession(hasPRs: !session.prs.isEmpty) }
        return []
    }

    /// The band's tabs: the workshop's own, else a slice's or session's.
    private var titlebarTabs: [TitlebarTab] {
        if workshopShown { return appModel.workshopTabs.map(\.titlebarTab) }
        return tabs.map(\.titlebarTab)
    }

    private var selectedTabID: String? {
        if workshopShown { return appModel.workshopTab?.titlebarTab.id }
        return tabs.first { $0.mode == main.wrappedValue }?.titlebarTab.id
    }

    private func showTitlebarTab(_ tab: TitlebarTab) {
        if workshopShown {
            if let picked = appModel.workshopTabs.first(where: { $0.titlebarTab == tab }) {
                appModel.showWorkshopTab(picked)
            }
        } else if let picked = tabs.first(where: { $0.titlebarTab == tab }) {
            showTab(picked)
        }
    }

    /// Whether the workshop is what the columns show — a project's, or an
    /// Untitled tab's.
    private var workshopShown: Bool { appModel.workshopSelected || appModel.untitledWorkshopVisible }

    private func showTab(_ tab: MainPaneTab) {
        let focus = NavigatorFocus(open: open.wrappedValue, main: main.wrappedValue).showing(tab.section, shows: tab.mode)
        open.wrappedValue = focus.open
        if focus.main != main.wrappedValue { main.wrappedValue = focus.main }
    }

    // MARK: - The breadcrumb

    private func crumbTreePicker(openingOn origin: CrumbPickerOrigin) -> some View {
        let projectID = appModel.activeProjectID ?? ""
        let milestone = origin == .project ? nil : selectedSlice.flatMap(milestoneName(of:))
        let container = origin == .project ? nil : (sliceContainerID ?? appModel.selectedContainerID)
        return CrumbTreePicker(tree: CrumbTree(
            model: appModel.sidebarModel, projectID: projectID, milestone: milestone, container: container)
        ) { row in
            crumbPicker = nil
            Task { await appModel.selectSlice(row.sliceID, inProject: row.projectID) }
        }
    }

    /// The container the selected slice is filed under, where the active
    /// project is a source project.
    private var sliceContainerID: String? {
        guard let slice = selectedSlice, appModel.source(ofProject: appModel.activeProjectID ?? "") != nil else {
            return nil
        }
        return slice.milestoneID
    }

    /// A slice's milestone by name, or nil for one under the scratch
    /// project's unfiled milestone, which is no milestone to show.
    private func milestoneName(of slice: Slice) -> String? {
        guard let milestone = appModel.projectStore?.state.projectInfo?.milestones
            .first(where: { $0.id == slice.milestoneID }) else { return slice.milestoneID }
        return milestone.unfiled ? nil : milestone.name
    }

    /// Where the selection sits, read left to right: a slice's project and
    /// milestone, a source task's container (and no project — a source's
    /// trail is card, then task), a workshop's or session's project — then
    /// the selection itself.
    private var crumbs: TitlebarCrumbs {
        if appModel.activeTabIsUntitled && !appModel.untitledWorkshopVisible {
            return TitlebarCrumbs(title: projectName)
        }
        if appModel.workshopSelected || appModel.untitledWorkshopVisible {
            return TitlebarCrumbs(parent: projectName, parentKind: .project, title: workshopRowTitle)
        }
        if let session = selectedSession {
            return TitlebarCrumbs(
                parent: projectName, parentKind: .project, title: "\(sessionRowTitle) · \(session.label)")
        }
        if let slice = selectedSlice {
            // A source task reads `<container> / <task>`: its container
            // stands where a project and milestone would.
            if let containerID = sliceContainerID {
                let title = appModel.containerTitle(containerID, inProject: appModel.activeProjectID ?? "")
                return TitlebarCrumbs(parent: title, parentKind: .container, title: slice.name)
            }
            return TitlebarCrumbs(project: projectName, parent: milestoneName(of: slice), title: slice.name)
        }
        if let containerID = appModel.selectedContainerID {
            // A container alone: no project crumb before it, as a source
            // task's trail starts at its container.
            return TitlebarCrumbs(title: appModel.containerTitle(containerID, inProject: appModel.activeProjectID ?? ""))
        }
        // Nothing selected: no breadcrumb at all.
        return .none
    }

    // MARK: - The selection

    private var selectedSlice: Slice? {
        guard let id = appModel.selectedSliceID else { return nil }
        return appModel.projectStore?.state.projectInfo?.slices.first { $0.id == id }
    }

    private var selectedSession: Session? {
        guard let id = appModel.selectedSessionID else { return nil }
        return appModel.sessionStore?.sessions.first { $0.id == id }
    }

    private var navigatorModel: NavigatorModel? {
        selectedSlice.map { slice in
            NavigatorModel(
                slice: slice, agent: appModel.activityStore?.agents[slice.id].map { AgentActivity($0.activity) },
                hasVisuals: !(appModel.sliceDetailStore(projectID: appModel.projectStore?.projectID ?? "")
                    .state(for: slice.id).detail?.visuals.isEmpty ?? true))
        }
    }

    /// What identifies the selection, for resetting to its defaults.
    private var selectionKey: String {
        [appModel.activeProjectID ?? "", appModel.selectedSliceID ?? "", appModel.selectedSessionID ?? "",
         appModel.workshopSelected ? "workshop" : "", appModel.selectedContainerID ?? ""].joined(separator: "|")
    }

    private var slicePhase: NavigatorSection? { navigatorModel?.phase }

    private func resetToDefaults() {
        openOverride = nil
        mainOverride = nil
        containerFocusOverride = nil
        review.reset()
        visualReview.reset()
    }

    /// A session's defaults: its agent's Thread and terminal while one is
    /// live; once it has exited, its pull request's section and conversation,
    /// or with none its diff.
    private var sessionLive: Bool {
        selectedSession.map { appModel.activityStore?.agents[$0.tag] != nil } ?? false
    }

    private var defaultOpen: Set<NavigatorSection> {
        if let navigatorModel { return navigatorModel.defaultOpen }
        if let session = selectedSession, !sessionLive {
            return session.prs.isEmpty ? [.changes] : [.pr]
        }
        return [.thread]
    }

    private var defaultMain: MainPaneMode {
        if let navigatorModel { return navigatorModel.defaultMain }
        if let session = selectedSession {
            if sessionLive { return .terminal }
            return session.prs.isEmpty ? .diff : .pr
        }
        return .empty
    }

    private var open: Binding<Set<NavigatorSection>> {
        Binding(
            get: { openOverride ?? defaultOpen },
            set: { openOverride = $0 }
        )
    }

    /// The main pane's mode. Setting it opens no section: a header click
    /// sets both together (`NavigatorFocus`), and a fold must be able to
    /// leave its section's view up without the view reopening it.
    private var main: Binding<MainPaneMode> {
        Binding(
            get: { mainOverride ?? defaultMain },
            set: { mainOverride = $0 }
        )
    }

    /// A selected container's focus: the user's, else its reading's defaults
    /// (its first section open, its story up).
    private func containerFocusBinding(_ containerID: String) -> Binding<ContainerFocus> {
        Binding(
            get: {
                if let containerFocusOverride { return containerFocusOverride }
                let show = appModel.containerStore(projectID: appModel.activeProjectID ?? "")
                    .state(for: containerID).show
                return show.map { ContainerNavigatorModel(show: $0).defaultFocus }
                    ?? ContainerFocus(open: [ContainerNavigatorModel.storyID], main: .story)
            },
            set: { containerFocusOverride = $0 }
        )
    }

    private var projectName: String {
        appModel.projectTabs.first { $0.id == appModel.activeProjectID }?.name ?? ""
    }

    @ViewBuilder
    private var selectionColumns: some View {
        if appModel.activeTabIsUntitled && !appModel.untitledWorkshopVisible {
            VStack(spacing: 0) {
                GnatTitlebar { Spacer(minLength: 0) }
                StarterView(appModel: appModel, onFromNotion: { showNewProjectSheet = true })
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        } else if appModel.workshopSelected || appModel.untitledWorkshopVisible {
            columns {
                WorkshopNavigatorView(appModel: appModel, projectName: projectName)
            } main: {
                WorkshopMainPane(appModel: appModel)
            }
        } else if let session = selectedSession {
            columns {
                SessionNavigatorView(appModel: appModel, session: session, open: open, main: main, review: review)
            } main: {
                SessionMainPane(appModel: appModel, session: session, mode: main, review: review)
            }
        } else if let slice = selectedSlice {
            columns {
                SliceNavigatorView(
                    appModel: appModel, slice: slice, open: open, main: main, review: review,
                    visualReview: visualReview, model: $launchModel, effort: $launchEffort)
            } main: {
                SliceMainPane(
                    appModel: appModel, slice: slice, mode: main, review: review, visualReview: visualReview)
            }
            .task(id: slice.id) {
                await appModel.sliceDetailStore(projectID: appModel.projectStore?.projectID ?? "")
                    .fetch(sliceRef: slice.id)
            }
            .task(id: "\(slice.id)|\(slice.branch ?? "")|\(slice.handedBack)") {
                guard navigatorModel?.hasBranch == true else { return }
                await review.fetch(appModel: appModel, slice: slice)
            }
        } else if let containerID = appModel.selectedContainerID {
            columns {
                ContainerNavigatorView(
                    appModel: appModel, containerID: containerID, focus: containerFocusBinding(containerID),
                    onNewTask: { newTaskContainer = containerID })
            } main: {
                ContainerPane(
                    appModel: appModel, containerID: containerID,
                    mode: containerFocusBinding(containerID).wrappedValue.main)
            }
            .task(id: containerID) {
                await appModel.containerStore(projectID: appModel.activeProjectID ?? "").fetch(containerID: containerID)
            }
        } else {
            columns {
                NavigatorColumn(anyOpen: true) {
                    if appModel.activePlanIsEmpty {
                        NavProse {
                            Text(EmptyProjectNote.title).ink(.primary)
                            Text(EmptyProjectNote.subtitle(needsWorkingDir: appModel.activeProjectNeedsWorkingDir))
                                .ink(.secondary)
                        }
                        .frame(maxHeight: .infinity, alignment: .top)
                        .surface(.window)
                    } else {
                        Text("Select a task in the sidebar.")
                            .font(.system(size: GnatMetrics.body))
                            .ink(.tertiary)
                            .multilineTextAlignment(.center)
                            .padding(24)
                            .frame(maxWidth: .infinity, maxHeight: .infinity)
                            .surface(.window)
                    }
                }
            } main: {
                VStack(spacing: 0) {
                    if let accepted = appModel.acceptedPlanShown {
                        MainPaneNote(text: ProposalText.acceptedTitle + "\n"
                            + ProposalText.acceptedSubtitle(milestones: accepted.milestones, slices: accepted.slices))
                    } else {
                        MainPaneEmptyState()
                    }
                }
                .surface(.window)
            }
        }
    }

    /// The navigator and the main pane side by side under their one
    /// titlebar band, the drag handle between them below it.
    private func columns<Navigator: View, Main: View>(
        @ViewBuilder navigator: () -> Navigator, @ViewBuilder main: () -> Main
    ) -> some View {
        VStack(spacing: 0) {
            titlebar
            HStack(spacing: 0) {
                navigator()
                    .frame(width: liveNavigatorWidth ?? navigatorWidth)
                    .zIndex(1)
                main()
                    .frame(maxWidth: .infinity)
                    .overlay(alignment: .leading) {
                        PaneResizeHandle(
                            width: navigatorWidth, liveWidth: $liveNavigatorWidth, onCommit: { navigatorWidth = $0 },
                            minWidth: 260, maxWidth: 520, edge: .trailing)
                            .offset(x: -4.5)
                    }
            }
        }
    }
}

/// The container a New task sheet is open on, as `sheet(item:)` takes it.
private struct NewTaskTarget: Identifiable {
    let containerID: String
    var id: String { containerID }
}

#Preview {
    @Previewable @State var appModel = Fixtures.appModel()
    WindowShellView(appModel: appModel)
        .frame(width: 1320, height: 820)
        .task { await Fixtures.start(appModel) }
}
