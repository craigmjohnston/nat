import AppKit
import SwiftUI
import NatKit
import NatFixtures

/// The window, as the gnat design lays it out: the sidebar, the navigator
/// and the main pane side by side under one 32pt titlebar band, over the
/// status bar.
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
    @State private var openOverride: Set<NavigatorSection>?
    @State private var mainOverride: MainPaneMode?

    /// The gallery's seam: a story seeds the sidebar folds it is a story
    /// about.
    var sidebarFolds: [String: Bool] = [:]

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
    }

    private var board: some View {
        VStack(spacing: 0) {
            titlebar
            HStack(spacing: 0) {
                SidebarView(appModel: appModel, onNewProject: { appModel.openUntitledTab() }, folded: sidebarFolds)
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

            StatusBarView(appModel: appModel)
        }
        .onChange(of: selectionKey) { _, _ in resetToDefaults() }
        .onChange(of: slicePhase) { _, _ in resetToDefaults() }
    }

    // MARK: - The titlebar

    /// The one titlebar across the window: past the traffic lights, where
    /// the selection sits — its project, its milestone, then its dot and
    /// name, slash-separated.
    private var titlebar: some View {
        GnatTitlebar(leading: GnatMetrics.lightsInset) {
            breadcrumb
        }
    }

    /// Where the selection sits, read left to right: a slice's project, its
    /// milestone (or what stands for one), each followed by a quiet slash,
    /// then the selection itself.
    private var breadcrumb: some View {
        let crumbs = crumbs
        return HStack(spacing: 10) {
            if let project = crumbs.project {
                Text(project).ink(.primary)
                Text("/").ink(.tertiary)
            }
            if let parent = crumbs.parent {
                if crumbs.parentIsMilestone {
                    // The sidebar's own milestone mark, open.
                    FolderGlyph(open: true, color: DesignTokens.ink(.tertiary, on: .header))
                        .padding(.trailing, -3)
                }
                Text(parent).ink(.primary)
                Text("/").ink(.tertiary)
            }
            if let dot = crumbs.dot {
                StateDot(state: dot.state, live: dot.live)
                    .padding(.trailing, -3)
            }
            Text(crumbs.title).ink(.primary)
        }
        .font(.system(size: GnatMetrics.titlebarText))
        .lineLimit(1)
        .truncationMode(.tail)
    }

    private var crumbs: (
        project: String?, parent: String?, parentIsMilestone: Bool,
        dot: (state: SliceDisplayState, live: Bool)?, title: String
    ) {
        if appModel.activeTabIsUntitled && !appModel.untitledWorkshopVisible {
            return (nil, nil, false, nil, projectName)
        }
        if appModel.workshopSelected || appModel.untitledWorkshopVisible {
            return (nil, projectName, false, nil, workshopRowTitle)
        }
        if let session = selectedSession {
            return (nil, projectName, false, nil, "\(sessionRowTitle) · \(session.label)")
        }
        if let slice = selectedSlice, let navigatorModel {
            let milestone = appModel.projectStore?.state.projectInfo?.milestones
                .first { $0.id == slice.milestoneID }?.name ?? slice.milestoneID
            return (projectName, milestone, true,
                    (navigatorModel.state, appModel.activityStore?.agents[slice.id] != nil), slice.name)
        }
        return (nil, nil, false, nil, projectName)
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
                fixLaunched: appModel.fixLaunched[slice.id] != nil)
        }
    }

    /// What identifies the selection, for resetting to its defaults.
    private var selectionKey: String {
        [appModel.activeProjectID ?? "", appModel.selectedSliceID ?? "", appModel.selectedSessionID ?? "",
         appModel.workshopSelected ? "workshop" : ""].joined(separator: "|")
    }

    private var slicePhase: NavigatorSection? { navigatorModel?.phase }

    private func resetToDefaults() {
        openOverride = nil
        mainOverride = nil
        review.reset()
    }

    /// A session's defaults: its agent's Thread and terminal while one is
    /// live; once it has exited, its pull requests and its diff.
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
        if selectedSession != nil { return sessionLive ? .terminal : .diff }
        return .empty
    }

    private var open: Binding<Set<NavigatorSection>> {
        Binding(
            get: { openOverride ?? defaultOpen },
            set: { openOverride = $0 }
        )
    }

    /// The main pane's mode. Picking one opens its section too, as the
    /// design's `pickMain` does: the diff opens Changes, the terminal Thread
    /// (the Thread header's Terminal button, a Changes file row).
    private var main: Binding<MainPaneMode> {
        Binding(
            get: { mainOverride ?? defaultMain },
            set: { mode in
                mainOverride = mode
                var sections = open.wrappedValue
                if mode == .diff { sections.insert(.changes) }
                if mode == .terminal { sections.insert(.thread) }
                if sections != open.wrappedValue { openOverride = sections }
            }
        )
    }

    private var projectName: String {
        appModel.projectTabs.first { $0.id == appModel.activeProjectID }?.name ?? ""
    }

    @ViewBuilder
    private var selectionColumns: some View {
        if appModel.activeTabIsUntitled && !appModel.untitledWorkshopVisible {
            StarterView(appModel: appModel, onFromNotion: { showNewProjectSheet = true })
                .frame(maxWidth: .infinity, maxHeight: .infinity)
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
                SliceNavigatorView(appModel: appModel, slice: slice, open: open, main: main, review: review)
            } main: {
                SliceMainPane(appModel: appModel, slice: slice, mode: main, review: review)
            }
            .task(id: slice.id) {
                await appModel.sliceDetailStore(projectID: appModel.projectStore?.projectID ?? "")
                    .fetch(sliceRef: slice.id)
            }
            .task(id: "\(slice.id)|\(slice.branch ?? "")|\(slice.handedBack)") {
                guard navigatorModel?.hasBranch == true else { return }
                await review.fetch(appModel: appModel, slice: slice)
            }
        } else {
            columns {
                NavigatorColumn(anyOpen: true) {
                    NavProse {
                        if appModel.activePlanIsEmpty {
                            Text(EmptyProjectNote.title).ink(.primary)
                            Text(EmptyProjectNote.subtitle(needsWorkingDir: appModel.activeProjectNeedsWorkingDir))
                                .ink(.secondary)
                        } else {
                            Text("Select a slice in the sidebar.").ink(.secondary)
                        }
                    }
                    .frame(maxHeight: .infinity, alignment: .top)
                    .surface(.window)
                }
            } main: {
                VStack(spacing: 0) {
                    if let accepted = appModel.acceptedPlanShown {
                        MainPaneNote(text: ProposalText.acceptedTitle + "\n"
                            + ProposalText.acceptedSubtitle(milestones: accepted.milestones, slices: accepted.slices))
                    } else {
                        MainPaneNote(text: "The terminal opens here on launch.")
                    }
                }
                .surface(.window)
            }
        }
    }

    private func columns<Navigator: View, Main: View>(
        @ViewBuilder navigator: () -> Navigator, @ViewBuilder main: () -> Main
    ) -> some View {
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

#Preview {
    @Previewable @State var appModel = Fixtures.appModel()
    WindowShellView(appModel: appModel)
        .frame(width: 1320, height: 820)
        .task { await Fixtures.start(appModel) }
}
