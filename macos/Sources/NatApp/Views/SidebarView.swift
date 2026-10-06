import AppKit
import SwiftUI
import NatKit

/// The gnat design's sidebar: the Active fold — every project's work in
/// flight, needs-you first, tagged `Project / title` — and the Projects
/// fold, each project a disclosure over its milestones and their slices —
/// then the Scratch fold, the scratch project's milestones straight under it.
///
/// Everything the old project tabs and rail did that the design does not
/// draw lives on here as the row it belongs to: a project's menu (its `+`'s
/// items, then Open in Notion, Reveal, Project settings…, Close), a
/// milestone's (New Slice, Rename, Move, Delete), a slice's (Launch, Edit,
/// Open, Move, Delete) — each also a three-dot button on its row under the
/// pointer — each project's own `+` (Workshop…, New Milestone, New Slice, New
/// Ad Hoc Session), the titlebar's `+` (a new project, then any of those in a
/// project it asks for) beside its Settings cog, and an ended session under its
/// project.
struct SidebarView: View {
    @Bindable var appModel: AppModel
    var onNewProject: () -> Void = {}
    /// Whether the sidebar draws its own segment of the window titlebar —
    /// the shell's way; a story of the sidebar alone has no window around it.
    var showsTitlebar = false
    /// Where the Projects tree's scroll starts — nil, its top, everywhere
    /// but a story that shows a project's header pinned over its rows.
    var treeAnchor: UnitPoint?
    /// A slice row of the tree to draw under the pointer — a story's, since
    /// a render has no pointer; nil, `.onHover` alone decides.
    var hoveredSlice: String?
    /// An Active row to draw under the pointer, by its slice's, session's or
    /// (a workshop's) project's ID — a story's, the same way.
    var hoveredActiveRow: String?
    /// A milestone row to draw under the pointer, by its fold key
    /// (`m:<project>/<milestone>`) — a story's, the same way.
    var hoveredMilestone: String?
    /// View ▸ Show/Hide Done Items.
    @Environment(\.showsDoneItems) private var showsDoneItems

    /// The folds the user has made, by key: `active`, `work`, `scratch`,
    /// `p:<project>` and `m:<project>/<milestone>`. A project with no entry is
    /// open exactly when it holds the selection — the design's own default —
    /// a milestone with no entry is open exactly when it is partly done or
    /// holds the selection (`SidebarMilestone.opensByDefault`), and Scratch
    /// starts folded.
    @State private var fold: [String: Bool]

    @State private var sliceForDeletion: (row: SidebarSliceRow, done: Bool)?
    @State private var sessionForDiscard: String?
    @State private var newSliceTarget: NewSliceTarget?
    @State private var milestoneForRename: MilestoneRef?
    @State private var renameText = ""
    @State private var milestoneForDeletion: MilestoneRef?
    @State private var sliceForEdit: SidebarSliceRow?
    @State private var projectPendingClose: String?
    @State private var workshopPendingClose = false
    /// What the workshop and project close alerts say — `WorkshopEndRules`'
    /// message, taken as the close was asked for.
    @State private var workshopCloseMessage = ""
    @State private var actionError: String?
    @State private var newMilestoneProject: String?
    @State private var newMilestoneText = ""
    /// The project whose settings sheet is up (the project menu's Project
    /// settings…).
    @State private var projectForSettings: SidebarProject?
    /// The Projects and Scratch folds' trees at their natural heights: what
    /// each takes, at most — open sections share the room only where they
    /// want more than there is.
    @State private var projectsContentHeight: CGFloat = 0
    @State private var scratchContentHeight: CGFloat = 0
    /// The project row under the pointer, whose folder turns into its fold
    /// chevron.
    @State private var hoveredProject: String?
    /// The projects whose header is pinned with their milestones scrolled
    /// under it — SwiftUI has no such state, so each section's body reports
    /// whether its top has gone up under its header (`pinnedMarker`).
    @State private var pinnedProjects: Set<String> = []
    /// A source fold's row under the pointer — a group's or a container's,
    /// by `sourceRowKey` — which shows its hover-only meta, `+` and menu.
    @State private var hoveredSourceRow: String?
    @State private var newTaskContainer: NewTaskContainer?
    /// A source action waiting on its line of text, or on its confirmation.
    @State private var sourceActionNeedingText: PendingSourceAction?
    @State private var sourceActionToConfirm: PendingSourceAction?
    /// The filter editor open — the source header's (no group) or a
    /// segment's — anchored to the row its menu came from.
    @State private var sourceFilterOpen: PendingSourceAction?
    /// Each open source fold's tree at its natural height, by project: what
    /// it takes, at most, where another open section has the room.
    @State private var sourceContentHeights: [String: CGFloat] = [:]

    /// - Parameters:
    ///   - hoveredContainer: a container row to draw under the pointer — a
    ///     story's, since a render has no pointer.
    ///   - hoveredGroup: a source group's row to draw under the pointer, the
    ///     same way (a container, where both are given, wins).
    ///   - hoveredSlice: a slice row to draw under the pointer, by slice ID.
    ///   - hoveredActiveRow: an Active row to draw under the pointer, by its
    ///     target's ID.
    ///   - hoveredMilestone: a milestone row to draw under the pointer, by its
    ///     fold key.
    ///   - hoveredProject: a project row to draw under the pointer, by ID.
    init(
        appModel: AppModel, onNewProject: @escaping () -> Void = {}, showsTitlebar: Bool = false,
        folded: [String: Bool] = [:], treeAnchor: UnitPoint? = nil,
        hoveredContainer: (projectID: String, containerID: String)? = nil,
        hoveredGroup: (projectID: String, groupID: String)? = nil,
        hoveredSlice: String? = nil,
        hoveredActiveRow: String? = nil,
        hoveredMilestone: String? = nil,
        hoveredProject: String? = nil
    ) {
        self.appModel = appModel
        self.onNewProject = onNewProject
        self.showsTitlebar = showsTitlebar
        self.treeAnchor = treeAnchor
        self.hoveredSlice = hoveredSlice
        self.hoveredActiveRow = hoveredActiveRow
        self.hoveredMilestone = hoveredMilestone
        _fold = State(initialValue: folded)
        _hoveredProject = State(initialValue: hoveredProject)
        _hoveredSourceRow = State(initialValue: hoveredContainer.map {
            Self.sourceRowKey($0.projectID, container: $0.containerID)
        } ?? hoveredGroup.map { Self.sourceRowKey($0.projectID, group: $0.groupID) })
    }

    private struct NewSliceTarget: Identifiable {
        let projectID: String
        let milestone: String
        var id: String { "\(projectID)/\(milestone)" }
    }

    fileprivate struct MilestoneRef: Identifiable, Equatable {
        let projectID: String
        let name: String
        var id: String { "\(projectID)/\(name)" }
    }

    /// The container a `+` asked for a new task on.
    fileprivate struct NewTaskContainer: Identifiable {
        let projectID: String
        let container: SidebarContainer
        let noun: String
        var id: String { "\(projectID)/\(container.id)" }
    }

    /// One of a source's actions and the row it came from.
    fileprivate struct PendingSourceAction: Identifiable {
        let projectID: String
        let action: SourceAction
        var group: String?
        var container: String?
        var id: String { "\(projectID)/\(group ?? "")/\(container ?? "")/\(action.id)" }
    }

    /// The sidebar's model, its done work dropped while View ▸ Hide Done
    /// Items is on.
    private var model: SidebarModel {
        let model = appModel.sidebarModel
        guard !showsDoneItems else { return model }
        return SidebarModel(
            active: model.active, projects: model.projects.map { $0.hidingDone() },
            sources: model.sources.map { $0.hidingDone() }, scratch: model.scratch?.hidingDone())
    }

    var body: some View {
        let model = model
        let folds = foldSlots(model)
        VStack(spacing: 0) {
            if showsTitlebar {
                titlebar(model)
            }
            head("active", label: "Active", count: model.needsYouCount) { EmptyView() }
            if isOpen("active") {
                if model.active.isEmpty {
                    GnatNote(text: EmptyActiveNote.text.lowercased(), height: GnatMetrics.sidebarRowHeight)
                } else {
                    ForEach(model.active) { activeRow($0) }
                }
            }

            // The open sections first, each at most its own height and
            // sharing the room between them when they want more than there
            // is; then the folded ones, pinned to the sidebar's foot under
            // whatever room is left — each in its usual order: Projects, the
            // source folds, Scratch.
            ForEach(folds.filter(\.open)) { fold in
                foldView(fold, model)
            }
            Spacer(minLength: 0)
            ForEach(folds.filter { !$0.open }) { fold in
                foldView(fold, model)
            }

            if appModel.mirrorNudgeShown {
                MirrorNudgeCardView(
                    onChoose: { appModel.mirrorPickerPresented = true },
                    onDismiss: { appModel.dismissMirrorNudge() }
                )
                .padding(12)
            }
        }
        .surface(.header)
        .rule(.separator, edges: [.trailing], width: 1)
        // Every menu here — each row's context menu, the `+` menus and their
        // submenus — shows its items' icons: on macOS 15 SwiftUI builds a menu
        // item with no image unless the label style asks for one, and this
        // reaches every menu below it.
        .labelStyle(.titleAndIcon)
        .modifier(SidebarDialogs(view: self))
        .focusedSceneValue(\.sidebarMenu, menuActions)
    }

    /// The active project's row actions, for the File menu — nil, and so
    /// disabled, wherever its own `+` or menu would not offer them.
    private var menuActions: SidebarMenuActions {
        guard let projectID = appModel.activeProjectID, !appModel.activeTabIsUntitled else {
            return SidebarMenuActions()
        }
        let isProject = !appModel.activeTabIsScratch
        return SidebarMenuActions(
            newSlice: { newSliceTarget = NewSliceTarget(projectID: projectID, milestone: "") },
            newMilestone: {
                newMilestoneText = ""
                newMilestoneProject = projectID
            },
            newSession: appModel.newSessionLaunching ? nil : { Task { await startNewSession(inProject: projectID) } },
            workshop: { Task { await appModel.selectWorkshop(inProject: projectID) } },
            revealWorkingDirectory: workingDirectory(of: projectID).map { directory in
                { NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: directory)]) }
            },
            openProjectInNotion: isProject ? NotionPageURL.forPage(projectID).map { url in
                { NSWorkspace.shared.open(url) }
            } : nil)
    }

    // MARK: - Fold

    private func isOpen(_ key: String, byDefault: Bool = true) -> Bool {
        fold[key].map { !$0 } ?? byDefault
    }

    private func isProjectOpen(_ project: SidebarProject) -> Bool {
        if let folded = fold["p:\(project.id)"] { return !folded }
        guard appModel.activeProjectID == project.id else { return false }
        if let slice = appModel.selectedSliceID { return project.contains(sliceID: slice) }
        return true
    }

    /// Snapped, not animated: a fold opening or closing puts its rows in
    /// place (or takes them away) at once, with no slide and no fade.
    private func toggle(_ key: String, open: Bool) {
        var snap = Transaction()
        snap.disablesAnimations = true
        withTransaction(snap) { fold[key] = open }
    }

    // MARK: - Sections

    /// One of the sections under Active — Projects, a source fold, Scratch —
    /// as the sidebar lays it out: open or folded, whether the line over it
    /// is hidden (the row just above it is the selection, which reads as one
    /// block rather than a wash cut by a rule), and whether that line sits
    /// under rows (4pt of air) or a folded heading (none, which would set the
    /// heading off-centre).
    private struct FoldSlot: Identifiable {
        enum Kind: Equatable {
            case projects
            case source(SidebarProject)
            case scratch(SidebarProject)
        }
        let kind: Kind
        let open: Bool
        var ruleHidden = false
        var afterRows = false

        var id: String {
            switch kind {
            case .projects: "work"
            case .source(let project): "s:\(project.id)"
            case .scratch: "scratch"
            }
        }
    }

    /// The sections under Active in the order they draw: every open one in
    /// its usual order (Projects, the source folds, Scratch), then every
    /// folded one in the same order, pinned to the foot.
    private func foldSlots(_ model: SidebarModel) -> [FoldSlot] {
        var all = [FoldSlot(kind: .projects, open: isOpen("work"))]
        all += model.sources.map { FoldSlot(kind: .source($0), open: isOpen(sourceKey($0))) }
        if let scratch = model.scratch {
            all.append(FoldSlot(kind: .scratch(scratch), open: isOpen("scratch", byDefault: false)))
        }
        var slots = all.filter(\.open) + all.filter { !$0.open }
        var endsInSelection = activeEndsInSelection(model)
        var afterRows = isOpen("active")
        for i in slots.indices {
            slots[i].ruleHidden = endsInSelection
            slots[i].afterRows = afterRows
            endsInSelection = slots[i].open && foldEndsInSelection(slots[i].kind, model)
            afterRows = slots[i].open
        }
        return slots
    }

    /// Whether an open section's last row is the selected one.
    private func foldEndsInSelection(_ kind: FoldSlot.Kind, _ model: SidebarModel) -> Bool {
        switch kind {
        case .projects: projectsEndInSelection(model)
        case .source(let project): sourceEndsInSelection(project)
        case .scratch(let scratch): treeEndsInSelection(scratch)
        }
    }

    @ViewBuilder
    private func foldView(_ fold: FoldSlot, _ model: SidebarModel) -> some View {
        Rule(.separator)
            .opacity(fold.ruleHidden ? 0 : 1)
            .padding(.top, fold.afterRows ? 4 : 0)
        switch fold.kind {
        case .projects:
            projectsFold(model, fold: fold)
        case .source(let project):
            sourceFold(project, fold: fold)
        case .scratch(let scratch):
            scratchFold(scratch, fold: fold)
        }
    }

    /// The Projects fold: its heading, then — open — the projects' tree in a
    /// scroll of its own, one section per project, its own row the header,
    /// pinned at the top while its milestones scroll under it.
    @ViewBuilder
    private func projectsFold(_ model: SidebarModel, fold: FoldSlot) -> some View {
        head("work", label: "Projects", count: 0) {
            Button(action: onNewProject) {
                Image(systemName: "folder.badge.plus")
                    .font(.system(size: 13))
                    .ink(.tertiary)
                    .frame(width: GnatMetrics.trailingControl, height: GnatMetrics.trailingControl)
                    .contentShape(Rectangle())
            }
            .buttonStyle(GnatIconButtonStyle())
            .help("New project\u{2026}")
        }
        if fold.open {
            ScrollView {
                LazyVStack(spacing: 0, pinnedViews: .sectionHeaders) {
                    ForEach(model.projects) { projectSection($0) }
                }
                .onGeometryChange(for: CGFloat.self) { $0.size.height } action: { projectsContentHeight = $0 }
            }
            .coordinateSpace(.named(Self.projectsScroll))
            .defaultScrollAnchor(treeAnchor)
            .thinScrollers()
            .frame(maxHeight: projectsContentHeight)
        }
    }

    // MARK: - Headings

    private func head<Trailing: View>(
        _ key: String, label: String, count: Int, openByDefault: Bool = true, @ViewBuilder trailing: () -> Trailing
    ) -> some View {
        let open = isOpen(key, byDefault: openByDefault)
        return HStack(spacing: 6) {
            DisclosureChevron(open: open)
            Text(label.uppercased())
                .font(.system(size: Typo.subhead))
                .tracking(0.7)
                .ink(.secondary)
            if count > 0 {
                Text("\(count)").monoXS().ink(.hot)
            }
            Spacer(minLength: 0)
            trailing()
        }
        .padding(.horizontal, 10)
        // A navigator section header's height, so Active lines up with the
        // Thread beside it; the air over the row height is shared above and
        // below, so a folded heading sits centred between its rule and the
        // next line.
        .frame(height: GnatMetrics.sectionHeadHeight)
        .contentShape(Rectangle())
        .onTapGesture { toggle(key, open: open) }
    }

    /// A symbol rather than the "+" character, whose glyph sits on the
    /// text baseline and so reads low beside a heading's label.
    private var plusGlyph: some View {
        Image(systemName: "plus")
            .font(.system(size: 13, weight: .light))
            .ink(.tertiary)
            .frame(width: GnatMetrics.trailingControl, height: GnatMetrics.trailingControl)
            .contentShape(Rectangle())
    }

    /// A project row's own `+`, and the Scratch heading's: add to that
    /// project's plan — a milestone or a slice outright, or by workshopping
    /// it with the planning agent.
    private func addMenu(_ project: SidebarProject) -> some View {
        Menu {
            addItems(project)
        } label: {
            plusGlyph
        }
        .menuStyle(.button)
        .buttonStyle(GnatIconButtonStyle())
        .menuIndicator(.hidden)
        .fixedSize()
        .help("Add to \(project.name)")
    }

    @ViewBuilder
    private func addItems(_ project: SidebarProject) -> some View {
        Button("Workshop\u{2026}", systemImage: workshopSymbol) { Task { await appModel.selectWorkshop(inProject: project.id) } }
        Button("New milestone\u{2026}", systemImage: "folder.badge.plus") {
            newMilestoneText = ""
            newMilestoneProject = project.id
        }
        Button("New task\u{2026}", systemImage: "plus") { newSliceTarget = NewSliceTarget(projectID: project.id, milestone: "") }
        Divider()
        Button(
            project.kind == .scratch ? "New ad hoc session\u{2026}" : "New ad hoc session",
            systemImage: "terminal"
        ) { Task { await startNewSession(inProject: project.id) } }
            .disabled(appModel.newSessionLaunching)
    }

    // MARK: - Titlebar

    /// The sidebar's segment of the window titlebar: past the traffic
    /// lights, at its trailing edge over the project rows' own: the run
    /// button where any project has runs, Settings and the `+`.
    private func titlebar(_ model: SidebarModel) -> some View {
        GnatTitlebar(leading: GnatMetrics.lightsInset) {
            Spacer(minLength: 0)
            TitlebarRunButton(appModel: appModel)
            SettingsLink {
                Image(systemName: "gearshape")
                    .font(.system(size: 13))
                    .ink(.tertiary)
                    .frame(width: 18, height: 18)
                    .contentShape(Rectangle())
            }
            .buttonStyle(GnatIconButtonStyle())
            .help("Settings")
            addAnythingMenu(model)
        }
    }

    /// The titlebar's `+`: anything the sidebar can make. A project first;
    /// then what goes in one, each asking which project from a submenu —
    /// every project, then Scratch.
    private func addAnythingMenu(_ model: SidebarModel) -> some View {
        let targets = model.projects.filter { $0.kind == .project } + (model.scratch.map { [$0] } ?? [])
        return Menu {
            Button("New project\u{2026}", systemImage: "folder.badge.plus", action: onNewProject)
            Divider()
            projectSubmenu("Workshop", systemImage: workshopSymbol, targets) { project in
                Task { await appModel.selectWorkshop(inProject: project.id) }
            }
            projectSubmenu("New milestone", systemImage: "folder.badge.plus", targets) { project in
                newMilestoneText = ""
                newMilestoneProject = project.id
            }
            projectSubmenu("New task", systemImage: "plus", targets) { project in
                newSliceTarget = NewSliceTarget(projectID: project.id, milestone: "")
            }
            Divider()
            projectSubmenu("New ad hoc session", systemImage: "terminal", targets) { project in
                Task { await startNewSession(inProject: project.id) }
            }
            .disabled(appModel.newSessionLaunching)
        } label: {
            if appModel.newSessionLaunching {
                ProgressView().controlSize(.mini).frame(width: 18, height: 18)
            } else {
                plusGlyph
            }
        }
        .menuStyle(.button)
        .buttonStyle(GnatIconButtonStyle())
        .menuIndicator(.hidden)
        .fixedSize()
        .help("New\u{2026}")
    }

    /// One of the `+` menu's items, as a submenu naming the project it goes
    /// in.
    private func projectSubmenu(
        _ title: String, systemImage: String, _ targets: [SidebarProject],
        action: @escaping (SidebarProject) -> Void
    ) -> some View {
        Menu(title, systemImage: systemImage) {
            ForEach(targets) { project in
                Button(project.kind == .scratch ? "Scratch" : project.name) { action(project) }
            }
        }
        .disabled(targets.isEmpty)
    }

    // MARK: - Sources

    /// The fold key of a source project's section.
    private func sourceKey(_ project: SidebarProject) -> String { "s:\(project.id)" }

    /// A source project's fold: a heading of its own — the plugin's icon,
    /// its title, its header menu — then, open, the plugin's groups, their
    /// containers and each container's tasks in a scroll of its own, as
    /// Projects and Scratch have theirs, at most its natural height.
    @ViewBuilder
    private func sourceFold(_ project: SidebarProject, fold: FoldSlot) -> some View {
        sourceHead(project, open: fold.open)
        if fold.open {
            ScrollView {
                VStack(spacing: 0) {
                    sourceBody(project)
                }
                .padding(.bottom, 4)
                .onGeometryChange(for: CGFloat.self) { $0.size.height } action: {
                    sourceContentHeights[project.id] = $0
                }
            }
            .thinScrollers()
            .frame(maxHeight: sourceContentHeights[project.id] ?? 0)
        }
    }

    /// A source fold's heading: the plugin's title, always — the section is
    /// the plugin's, whatever the project behind it is called, and nothing
    /// renames it — then the filter button (where the menu has a `filter`
    /// action), its editor anchored to it, then the header menu.
    private func sourceHead(_ project: SidebarProject, open: Bool) -> some View {
        let key = sourceKey(project)
        let source = project.source
        return HStack(spacing: 6) {
            DisclosureChevron(open: open)
            SourceIconView(icon: source?.icon ?? SourceIcon(symbol: ""), size: 13)
                .ink(.secondary)
            Text((source?.title ?? project.name).uppercased())
                .font(.system(size: Typo.subhead))
                .tracking(0.7)
                .ink(.secondary)
                .lineLimit(1)
            if !open && project.needsYou > 0 {
                Text("\(project.needsYou)").monoXS().ink(.hot)
            }
            Spacer(minLength: 0)
            if let filter = source?.menu.filterAction {
                filterButton(filter, projectID: project.id, group: nil, size: 12)
            }
            if let source, !source.menu.menuItems.isEmpty {
                Menu {
                    actionItems(source.menu, projectID: project.id)
                } label: {
                    Image(systemName: "ellipsis")
                        .font(.system(size: 12, weight: .medium))
                        .ink(.tertiary)
                        .frame(width: GnatMetrics.trailingControl, height: GnatMetrics.trailingControl)
                        .contentShape(Rectangle())
                }
                .menuStyle(.button)
                .buttonStyle(GnatIconButtonStyle())
                .menuIndicator(.hidden)
                .fixedSize()
                .help(source.title)
            }
        }
        .padding(.horizontal, 10)
        .frame(height: GnatMetrics.sectionHeadHeight)
        .contentShape(Rectangle())
        .onTapGesture { toggle(key, open: open) }
        .contextMenu { sourceProjectMenu(project) }
    }

    @ViewBuilder
    private func sourceBody(_ project: SidebarProject) -> some View {
        switch project.status {
        case .loading:
            GnatNote(text: "loading\u{2026}", leading: 26, height: GnatMetrics.sidebarRowHeight)
        case .failed(let message):
            failedRow(project, message: message)
        case .stale(let message):
            GnatNote(
                text: "refresh failed, showing the last plan", role: .warning, leading: 26,
                height: GnatMetrics.sidebarRowHeight)
                .help(message)
        case .loaded, .empty, .none:
            EmptyView()
        }
        if let source = project.source {
            if let error = source.error {
                // The plugin's own failed read: the fold's note, its first
                // words drawn and the rest on hover.
                Text(error)
                    .font(.system(size: GnatMetrics.xs))
                    .ink(.warning)
                    .lineLimit(2)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.leading, 26)
                    .padding(.trailing, 10)
                    .padding(.vertical, 4)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .help(error)
            }
            ForEach(source.groups) { group in
                sourceGroup(group, project: project, source: source, indent: 20)
            }
        }
    }

    /// One group of a source fold: its small-caps header with the plugin's
    /// count, then — open — its child groups, a step in, or its containers.
    /// A lazy group's fold is the app model's, since opening it is a read.
    private func sourceGroup(
        _ group: SidebarSourceGroup, project: SidebarProject, source: SidebarSource, indent: CGFloat
    ) -> AnyView {
        let key = "g:\(project.id)/\(group.id)"
        let open = group.lazy
            ? appModel.isSourceGroupExpanded(group.id, inProject: project.id)
            : isOpen(key)
        let rowKey = sourceRowKey(project.id, group: group.id)
        let hovered = hoveredSourceRow == rowKey
        return AnyView(VStack(spacing: 0) {
            HStack(spacing: 6) {
                DisclosureChevron(open: open)
                Text(group.label.uppercased())
                    .font(.system(size: Typo.caption, weight: .medium))
                    .tracking(0.7)
                    .ink(.tertiary)
                    .lineLimit(1)
                if let count = group.count {
                    Text("\(count)").font(Typo.mono(size: Typo.caption)).ink(.tertiary)
                }
                Spacer(minLength: 0)
                // A segment's filter button under the pointer, beside its
                // menu — always while its editor is open, which is anchored
                // to it.
                if let filter = group.menu.filterAction {
                    let showing = hovered || filterPresented(project.id, group: group.id).wrappedValue
                    filterButton(filter, projectID: project.id, group: group.id, size: 11)
                        .opacity(showing ? 1 : 0)
                        .allowsHitTesting(showing)
                }
                if !group.menu.menuItems.isEmpty {
                    Menu {
                        actionItems(group.menu, projectID: project.id, group: group.id)
                    } label: {
                        Image(systemName: "ellipsis")
                            .font(.system(size: 11, weight: .medium))
                            .ink(.tertiary)
                            .frame(width: GnatMetrics.trailingControl, height: GnatMetrics.trailingControl)
                            .contentShape(Rectangle())
                    }
                    .menuStyle(.button)
                    .buttonStyle(GnatIconButtonStyle())
                    .menuIndicator(.hidden)
                    .fixedSize()
                    .opacity(hovered ? 1 : 0)
                    .allowsHitTesting(hovered)
                }
            }
            .padding(.leading, indent)
            .padding(.trailing, 10)
            .frame(height: 24)
            .contentShape(Rectangle())
            .onHover { inside in hover(rowKey, inside) }
            .onTapGesture {
                if group.lazy {
                    Task { await appModel.setSourceGroup(group.id, expanded: !open, inProject: project.id) }
                } else {
                    toggle(key, open: open)
                }
            }
            .contextMenu {
                if !group.menu.menuItems.isEmpty { actionItems(group.menu, projectID: project.id, group: group.id) }
            }

            if open {
                ForEach(group.children) { child in
                    sourceGroup(child, project: project, source: source, indent: indent + 10)
                }
                ForEach(group.containers) { container in
                    containerRows(container, project: project, source: source, indent: indent)
                }
                if group.children.isEmpty && group.containers.isEmpty {
                    GnatNote(
                        text: group.lazy && (group.count ?? 0) > 0 ? "loading\u{2026}" : "no \(source.containerNoun)s",
                        leading: indent + 22, height: GnatMetrics.rowHeight)
                }
            }
        })
    }

    /// A container row — its card glyph, title, hover meta, badges and a
    /// hover `+` for a new task — and its tasks nested a step in beneath.
    @ViewBuilder
    private func containerRows(
        _ container: SidebarContainer, project: SidebarProject, source: SidebarSource, indent: CGFloat
    ) -> some View {
        let selected = appModel.activeProjectID == project.id && appModel.selectedContainerID == container.id
        let rowKey = sourceRowKey(project.id, container: container.id)
        let hovered = hoveredSourceRow == rowKey
        HStack(spacing: 7) {
            // A card with tasks under it is the stacked card; one with none
            // the single card.
            Image(systemName: container.tasks.isEmpty ? SourceGlyph.emptyContainer : SourceGlyph.container)
                .font(.system(size: 11))
                .ink(.tertiary)
                .frame(width: GnatMetrics.treeFolderColumn)
            Text(container.title)
                .font(.system(size: GnatMetrics.body))
                .ink(.secondary)
                .lineLimit(1)
            Spacer(minLength: 0)
            // The meta only under the pointer, taking no room otherwise: a
            // sidebar is narrow, and the title wants it.
            if hovered, let meta = container.meta {
                Text(meta).monoXS().ink(.tertiary)
            }
            // Under the pointer the `+` takes the badges' place — the same
            // trailing slot, at least as wide — rather than pushing them
            // left, so nothing on the row moves as it comes and goes.
            ZStack(alignment: .trailing) {
                HStack(spacing: 7) {
                    ForEach(Array(container.badges.enumerated()), id: \.offset) { _, badge in
                        SourceBadgeView(badge: badge)
                    }
                }
                .opacity(hovered ? 0 : 1)
                .accessibilityHidden(hovered)
                if hovered {
                    Button {
                        newTaskContainer = NewTaskContainer(projectID: project.id, container: container, noun: source.containerNoun)
                    } label: {
                        Image(systemName: "plus")
                            .font(.system(size: 12, weight: .light))
                            .ink(.tertiary)
                            .frame(width: 16, height: 16)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(GnatIconButtonStyle())
                    // Centred in a badge's own fixed slot, so it sits where
                    // the (last) badge did.
                    .frame(width: SourceBadgeView.width)
                    .help("New \(source.taskNoun) on this \(source.containerNoun)")
                }
            }
        }
        .padding(.leading, indent + 6)
        .padding(.trailing, 10)
        .frame(height: GnatMetrics.sidebarRowHeight)
        .gnatRow(selected: selected)
        .contentShape(Rectangle())
        .onHover { inside in hover(rowKey, inside) }
        .onTapGesture { Task { await appModel.selectContainer(container.id, inProject: project.id) } }
        .contextMenu { containerMenu(container, project: project, source: source) }

        ForEach(container.tasks) { sliceRow($0, indent: indent + 14) }
    }

    @ViewBuilder
    private func containerMenu(_ container: SidebarContainer, project: SidebarProject, source: SidebarSource) -> some View {
        Button("New \(source.taskNoun)\u{2026}", systemImage: "plus") {
            newTaskContainer = NewTaskContainer(projectID: project.id, container: container, noun: source.containerNoun)
        }
        if let url = container.externalURL.flatMap(URL.init(string:)) {
            Button("Open in \(source.title)", systemImage: SourceGlyph.externalLink) { NSWorkspace.shared.open(url) }
        }
        if !container.menu.isEmpty {
            Divider()
            actionItems(container.menu, projectID: project.id, container: container.id)
        }
    }

    /// A source project's own menu: its header's actions, then the project's
    /// own Reveal and Close.
    @ViewBuilder
    private func sourceProjectMenu(_ project: SidebarProject) -> some View {
        if let source = project.source, !source.menu.menuItems.isEmpty {
            actionItems(source.menu, projectID: project.id)
            Divider()
        }
        if let directory = workingDirectory(of: project.id) {
            Button("Reveal working directory in Finder", systemImage: "folder") {
                NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: directory)])
            }
        }
        if ProjectTabRules.showsClose(tabCount: appModel.closableTabCount, isScratch: false) {
            Divider()
            Button("Close project", systemImage: "xmark.circle") { requestClose(project.id) }
        }
    }

    /// A menu's items: every action but a `filter` one, which is the
    /// filter button's (`filterButton`).
    private func actionItems(
        _ actions: [SourceAction], projectID: String, group: String? = nil, container: String? = nil
    ) -> some View {
        SourceActionItems(
            actions: actions.menuItems,
            onRun: { action, input in
                runSourceAction(PendingSourceAction(projectID: projectID, action: action, group: group, container: container), input: input)
            },
            onText: { sourceActionNeedingText = PendingSourceAction(projectID: projectID, action: $0, group: group, container: container) },
            onConfirm: { sourceActionToConfirm = PendingSourceAction(projectID: projectID, action: $0, group: group, container: container) })
    }

    /// The button a `filter` action is, beside the menu it would otherwise
    /// be an item of: the funnel, filled in the accent while the filter
    /// narrows anything (`SourceAction.isNarrowing`), opening the editor
    /// anchored to itself — the header's (`group` nil) or a segment's.
    private func filterButton(
        _ action: SourceAction, projectID: String, group: String?, size: CGFloat
    ) -> some View {
        let narrowing = action.isNarrowing
        return Button {
            sourceFilterOpen = PendingSourceAction(projectID: projectID, action: action, group: group, container: nil)
        } label: {
            Image(systemName: narrowing ? "line.3.horizontal.decrease.circle.fill" : "line.3.horizontal.decrease.circle")
                .font(.system(size: size, weight: .medium))
                .ink(narrowing ? .accent : .tertiary)
                .frame(width: GnatMetrics.trailingControl, height: GnatMetrics.trailingControl)
                .contentShape(Rectangle())
        }
        .buttonStyle(GnatIconButtonStyle())
        .help(action.label.trimmingCharacters(in: CharacterSet(charactersIn: "\u{2026}.")))
        .popover(isPresented: filterPresented(projectID, group: group), arrowEdge: .trailing) {
            filterPopover(projectID, group: group)
        }
    }

    /// Whether the filter editor is open on the source header (`group` nil)
    /// or on one group's row.
    private func filterPresented(_ projectID: String, group: String?) -> Binding<Bool> {
        Binding(
            get: { sourceFilterOpen.map { $0.projectID == projectID && $0.group == group } ?? false },
            set: { if !$0 { sourceFilterOpen = nil } })
    }

    /// The filter editor over the action as the tree has it now — read again
    /// once while a field is loading, so it fills in under the open editor.
    @ViewBuilder
    private func filterPopover(_ projectID: String, group: String?) -> some View {
        if let pending = sourceFilterOpen {
            SourceFilterPopover(
                action: appModel.source(ofProject: projectID)?.filterAction(group: group) ?? pending.action,
                onCancel: { sourceFilterOpen = nil },
                onApply: { input in
                    sourceFilterOpen = nil
                    runSourceAction(pending, input: input)
                },
                onReread: { await appModel.rereadSource(projectID: projectID) })
        }
    }

    fileprivate func runSourceAction(_ pending: PendingSourceAction, input: String?) {
        Task {
            if let refusal = await appModel.runSourceAction(
                projectID: pending.projectID, action: pending.action, group: pending.group,
                container: pending.container, input: input) {
                actionError = refusal
            }
        }
    }

    private func sourceRowKey(_ projectID: String, group: String? = nil, container: String? = nil) -> String {
        Self.sourceRowKey(projectID, group: group, container: container)
    }

    private static func sourceRowKey(_ projectID: String, group: String? = nil, container: String? = nil) -> String {
        "\(projectID)/\(group.map { "g:\($0)" } ?? "")\(container.map { "c:\($0)" } ?? "")"
    }

    private func hover(_ key: String, _ inside: Bool) {
        if inside { hoveredSourceRow = key } else if hoveredSourceRow == key { hoveredSourceRow = nil }
    }

    // MARK: - Active

    /// Whether the last Active row is the selected one — what takes the line
    /// under Active away.
    private func activeEndsInSelection(_ model: SidebarModel) -> Bool {
        guard isOpen("active"), let last = model.active.last else { return false }
        return isSelected(last)
    }

    private func activeRow(_ row: SidebarActiveRow) -> some View {
        HStack(spacing: 6) {
            ActiveIdentityLabel(
                tag: row.projectTag, state: row.state, live: row.live, title: row.title, symbol: row.symbol)
            Spacer(minLength: 0)
            // The pull request was last read failing its checks, or
            // conflicting: its marks, each named under the pointer.
            PRMarksView(marks: row.marks)
            // Restored from the last run, its agent not yet read again.
            if row.reconnecting {
                Text(reconnectingLabel).font(.system(size: 11)).ink(.tertiary).lineLimit(1).fixedSize()
            }
            // A badge, not a button: the row's click opens the workshop,
            // which lands on its Plan. Never squeezed — the title gives first.
            if row.planReady {
                Chip(planReadyLabel, tone: .success, size: .small, systemImage: "checkmark").fixedSize()
            }
            if row.kind == .workshop {
                Button { closeWorkshopRow(row) } label: {
                    Image(systemName: "xmark")
                        .font(.system(size: 10, weight: .medium))
                        .ink(.tertiary)
                        .frame(width: GnatMetrics.trailingControl, height: GnatMetrics.trailingControl)
                        .contentShape(Rectangle())
                }
                .buttonStyle(GnatIconButtonStyle())
                .help(row.live ? "End the workshop session\u{2026}" : "Close the workshop")
            }
        }
        .padding(.leading, 18)
        .padding(.trailing, 10)
        .frame(height: GnatMetrics.sidebarRowHeight)
        .gnatRow(selected: isSelected(row))
        .transformEnvironment(\.hoverForced) { if row.targetID == hoveredActiveRow { $0 = true } }
        .contentShape(Rectangle())
        .onTapGesture { select(row) }
        .contextMenu { activeMenu(row) }
    }

    private func isSelected(_ row: SidebarActiveRow) -> Bool {
        guard appModel.activeProjectID == row.projectID else { return false }
        switch row.kind {
        case .slice: return appModel.selectedSliceID == row.targetID
        case .session: return appModel.selectedSessionID == row.targetID
        case .workshop: return appModel.workshopSelected
        }
    }

    /// A workshop row's ✕: with nothing running, the row and its draft go at
    /// once; with a planning agent live, its session is ended — asked first,
    /// on the workshop itself.
    private func closeWorkshopRow(_ row: SidebarActiveRow) {
        guard row.live else {
            appModel.dismissWorkshop(inProject: row.projectID)
            return
        }
        Task { await endWorkshop(inProject: row.projectID) }
    }

    /// Ending a project's live workshop from the sidebar: the workshop is
    /// selected, then asked about only where `WorkshopEndRules` says
    /// something would be lost, else ended at once.
    private func endWorkshop(inProject projectID: String) async {
        await appModel.selectWorkshop(inProject: projectID)
        if let message = appModel.workshopEndConfirmation(forTab: projectID) {
            workshopCloseMessage = message
            workshopPendingClose = true
        } else if let refusal = await appModel.closeWorkshopTab() {
            actionError = refusal
        }
    }

    private func select(_ row: SidebarActiveRow) {
        Task {
            switch row.kind {
            case .slice: await appModel.selectSlice(row.targetID, inProject: row.projectID)
            case .session: await appModel.selectSession(row.targetID, inProject: row.projectID)
            case .workshop: await appModel.selectWorkshop(inProject: row.projectID)
            }
        }
    }

    @ViewBuilder
    private func activeMenu(_ row: SidebarActiveRow) -> some View {
        switch row.kind {
        case .session:
            if let session = appModel.sessionStore?.sessions.first(where: { $0.id == row.targetID }) {
                Button("End session", systemImage: "stop.circle") { Task { await appModel.endSession(tag: session.tag) } }
                Button("Discard\u{2026}", systemImage: "trash", role: .destructive) { sessionForDiscard = session.id }
            }
        case .workshop:
            Button("End workshop session\u{2026}", systemImage: "stop.circle") {
                Task { await endWorkshop(inProject: row.projectID) }
            }
        case .slice:
            if let plan = appModel.plan(projectID: row.projectID),
               let slice = plan.slices.first(where: { $0.id == row.targetID }) {
                sliceMenu(SidebarSliceRow(
                    sliceID: slice.id, projectID: row.projectID, title: slice.name, state: row.state, live: row.live),
                          milestone: slice.milestoneID)
            }
        }
    }

    // MARK: - Projects

    private func projectSection(_ project: SidebarProject) -> some View {
        let open = isProjectOpen(project)
        return Section {
            if open {
                pinnedMarker(project.id)
                projectBody(project, isActive: appModel.activeProjectID == project.id)
            }
        } header: {
            projectHead(project, open: open, pinned: open && pinnedProjects.contains(project.id))
        }
    }

    /// The Projects scroll's coordinate space, its origin the viewport's top.
    private static let projectsScroll = "sidebar-projects"

    /// A zero-height mark at the top of a project's body: once it has gone
    /// up past the header's height in the scroll's viewport, the milestones
    /// are scrolling under a header pinned at the top. Reported only as it
    /// flips, so scrolling redraws nothing more. A mark the lazy stack has
    /// let go of keeps its last word — it went up out of sight, still under.
    private func pinnedMarker(_ id: String) -> some View {
        Color.clear
            .frame(height: 0)
            .onGeometryChange(for: Bool.self) {
                $0.frame(in: .named(Self.projectsScroll)).minY < GnatMetrics.sidebarRowHeight - 0.5
            } action: { under in
                if under { pinnedProjects.insert(id) } else { pinnedProjects.remove(id) }
            }
    }

    /// A project's own row: the fold toggle, and its section's header —
    /// on the sidebar's ground, so its milestones scrolling under it while
    /// it is pinned do not show through. Pinned so, it reads as pinned: its
    /// name in the status bar's quiet ink, the hover wash under it.
    private func projectHead(_ project: SidebarProject, open: Bool, pinned: Bool = false) -> some View {
        let isActive = appModel.activeProjectID == project.id
        return HStack(spacing: 7) {
            // The project's own fold mark: a folder of folders, outlined
            // while folded and open with its flap swung out once its
            // milestones are on the tree.
            // Under the pointer, it gives way to the chevron the click works.
            Group {
                if hoveredProject == project.id {
                    DisclosureChevron(open: open)
                } else {
                    StackedFolderGlyph(
                        open: open,
                        color: DesignTokens.ink(.tertiary, on: .header),
                        backColor: DesignTokens.ink(.tertiary, on: .header))
                }
            }
            .frame(width: GnatMetrics.treeFolderColumn)
            Group {
                switch project.kind {
                case .untitled:
                    Text(project.name).italic()
                case .scratch:
                    Label(project.name, systemImage: DesignTokens.scratchSymbol).labelStyle(.titleAndIcon)
                case .project:
                    Text(project.name)
                }
            }
            .font(.system(size: GnatMetrics.body))
            .ink(pinned ? .tertiary : .secondary)
            .lineLimit(1)
            Spacer(minLength: 0)
            if !open && project.needsYou > 0 {
                Circle().fill(DesignTokens.hot).frame(width: 6, height: 6)
            }
            if projectMenuHasItems(project) {
                // The right-click menu as a button, only under the pointer
                // (unlike the `+`), its slot always kept.
                let showsMenu = hoveredProject == project.id
                RowMenuButton(glyph: 12) { projectMenu(project) }
                    .opacity(showsMenu ? 1 : 0)
                    .allowsHitTesting(showsMenu)
                    .accessibilityHidden(!showsMenu)
            }
            if project.kind != .untitled {
                // Only on a project open on the tree or under the pointer —
                // a column of `+`s down every row read as noise. Hidden
                // rather than left out, so the title never shifts as it
                // comes and goes.
                let showsAdd = open || hoveredProject == project.id
                addMenu(project)
                    .opacity(showsAdd ? 1 : 0)
                    .allowsHitTesting(showsAdd)
                    .accessibilityHidden(!showsAdd)
            }
        }
        .padding(.leading, 18)
        .padding(.trailing, 10)
        .frame(height: GnatMetrics.sidebarRowHeight)
        .gnatRow(selected: project.kind == .untitled && isActive, washed: pinned)
        .contentShape(Rectangle())
        .onHover { inside in
            if inside { hoveredProject = project.id } else if hoveredProject == project.id { hoveredProject = nil }
        }
        .onTapGesture {
            toggle("p:\(project.id)", open: open)
            if project.kind == .untitled { Task { await appModel.activateProject(project.id) } }
        }
        .contextMenu { projectMenu(project) }
        .surface(.header)
    }

    /// A project's tree under its row. `outdent` pulls the whole tree left
    /// by one level — the Scratch fold's, whose milestones sit where a
    /// project row would.
    @ViewBuilder
    private func projectBody(_ project: SidebarProject, isActive: Bool, outdent: CGFloat = 0) -> some View {
        switch project.status {
        case .loading:
            GnatNote(text: "loading\u{2026}", leading: 26 - outdent, height: GnatMetrics.sidebarRowHeight)
        case .failed(let message):
            failedRow(project, message: message, leading: 26 - outdent)
        case .empty where project.kind == .scratch:
            scratchEmptyNote(project, leading: 26 - outdent)
        case .empty:
            GnatNote(text: "no tasks", leading: 26 - outdent, height: GnatMetrics.sidebarRowHeight)
        case .stale(let message):
            GnatNote(
                text: "refresh failed, showing the last plan", role: .warning, leading: 26 - outdent,
                height: GnatMetrics.sidebarRowHeight)
                .help(message)
        case .loaded, .none:
            EmptyView()
        }
        // The scratch project's unfiled slices: loose at the head of the
        // tree, where a milestone would sit, under no folder of their own.
        // The dot's 12pt column centred on a folder's 16pt one.
        ForEach(project.loose) { sliceRow($0, indent: 28 - outdent) }
        let selected = appModel.activeProjectID == project.id ? appModel.selectedSliceID : nil
        ForEach(project.milestones) { milestone in
            let key = "m:\(project.id)/\(milestone.name)"
            let opensItself = milestone.opensByDefault(selecting: selected)
            milestoneHead(
                name: milestone.name.isEmpty ? "No milestone" : milestone.name,
                count: "\(milestone.done)/\(milestone.total)", key: key, indent: 26 - outdent,
                openByDefault: opensItself, menu: { AnyView(milestoneMenu(project.id, milestone.name)) })
                .contextMenu { milestoneMenu(project.id, milestone.name) }
            if isOpen(key, byDefault: opensItself) {
                ForEach(milestone.slices) { sliceRow($0, indent: 34 - outdent) }
            }
        }
        // An ended session is drawn as done, so it goes with the rest of
        // the finished work under Hide done items.
        if isActive && showsDoneItems {
            endedSessions(project, outdent: outdent)
        }
        doneFolder(project, outdent: outdent)
    }

    /// The Scratch fold: the reserved scratch project's tree straight under
    /// its own heading, with no project row — it is a project to nat, but
    /// never drawn as one. Like every open section it takes at most the
    /// height it needs, sharing the room where there is not enough.
    @ViewBuilder
    private func scratchFold(_ scratch: SidebarProject, fold: FoldSlot) -> some View {
        head("scratch", label: "Scratch", count: 0, openByDefault: false) { addMenu(scratch) }
            .contextMenu { addItems(scratch) }
        if fold.open {
            ScrollView {
                VStack(spacing: 0) {
                    projectBody(scratch, isActive: appModel.activeProjectID == scratch.id, outdent: 8)
                }
                .padding(.bottom, 4)
                .onGeometryChange(for: CGFloat.self) { $0.size.height } action: { scratchContentHeight = $0 }
            }
            .thinScrollers()
            .frame(maxHeight: scratchContentHeight)
        }
    }

    /// Whether the Projects tree's last row is the selected one — what takes
    /// the line over Scratch away, as a selected last Active row takes
    /// Active's. Walks the tree from its foot in the order `projectBody` draws
    /// it, stopping at the first row drawn: only a slice, an ended session or
    /// an Untitled project row can be selected, so any heading or note there
    /// first means the tree does not end in the selection.
    private func projectsEndInSelection(_ model: SidebarModel) -> Bool {
        guard isOpen("work"), let project = model.projects.last else { return false }
        let isActive = appModel.activeProjectID == project.id
        guard isProjectOpen(project) else { return project.kind == .untitled && isActive }
        return treeEndsInSelection(project)
    }

    /// Whether a project's tree, drawn open, ends in the selection — the
    /// Projects tree's last project's, or Scratch's.
    private func treeEndsInSelection(_ project: SidebarProject) -> Bool {
        let isActive = appModel.activeProjectID == project.id
        let selectedSlice = isActive ? appModel.selectedSliceID : nil
        func endsIn(_ milestone: SidebarMilestone, key: String, byDefault: Bool = true) -> Bool {
            guard isOpen(key, byDefault: byDefault), let last = milestone.slices.last else { return false }
            return last.sliceID == selectedSlice
        }
        if let last = project.doneMilestones.last {
            let holdsSelection = selectedSlice.map(project.doneContains) ?? false
            guard isOpen("d:\(project.id)", byDefault: holdsSelection) else { return false }
            let opensItself = selectedSlice.map { id in last.slices.contains { $0.sliceID == id } } ?? false
            return endsIn(last, key: "m:\(project.id)/\(last.name)", byDefault: opensItself)
        }
        if isActive && showsDoneItems {
            let live = (appModel.activityStore?.agents ?? [:]).mapValues { AgentActivity($0.activity) }
            let ended = (appModel.sessionStore?.sessions ?? [])
                .filter { sessionIsDone($0, liveAgents: live) }
                .sorted { $0.startedAt > $1.startedAt }
            if let last = ended.last {
                return isOpen("m:\(project.id)/~sessions") && appModel.selectedSessionID == last.id
            }
        }
        if let last = project.milestones.last {
            return endsIn(
                last, key: "m:\(project.id)/\(last.name)",
                byDefault: last.opensByDefault(selecting: selectedSlice))
        }
        return project.loose.last.map { $0.sliceID == selectedSlice } ?? false
    }

    /// Whether a source fold, drawn open, ends in the selection: its last
    /// group open, and that group's last row — a container's last task, or
    /// the container itself — the selected one.
    private func sourceEndsInSelection(_ project: SidebarProject) -> Bool {
        guard appModel.activeProjectID == project.id, var group = project.source?.groups.last else { return false }
        func opens(_ g: SidebarSourceGroup) -> Bool {
            g.lazy ? appModel.isSourceGroupExpanded(g.id, inProject: project.id) : isOpen("g:\(project.id)/\(g.id)")
        }
        guard opens(group) else { return false }
        if let child = group.children.last {
            guard opens(child) else { return false }
            group = child
        }
        guard let container = group.containers.last else { return false }
        if let task = container.tasks.last { return task.sliceID == appModel.selectedSliceID }
        return container.id == appModel.selectedContainerID
    }

    /// An empty Scratch: what to do about it, each way in a link.
    private func scratchEmptyNote(_ project: SidebarProject, leading: CGFloat) -> some View {
        Text(ScratchEmptyNote.markdown)
            .font(.system(size: GnatMetrics.body))
            .ink(.tertiary)
            .tint(DesignTokens.accent)
            .lineSpacing(2)
            .fixedSize(horizontal: false, vertical: true)
            .padding(.leading, leading)
            .padding(.trailing, 10)
            .padding(.vertical, 6)
            .frame(maxWidth: .infinity, alignment: .leading)
            .environment(\.openURL, OpenURLAction { url in
                switch ScratchEmptyNote.Link(url) {
                case .workshop: Task { await appModel.selectWorkshop(inProject: project.id) }
                case .addSlice: newSliceTarget = NewSliceTarget(projectID: project.id, milestone: "")
                case nil: return .systemAction
                }
                return .handled
            })
    }

    private func failedRow(_ project: SidebarProject, message: String, leading: CGFloat = 26) -> some View {
        HStack(spacing: 6) {
            Text("the plan could not be loaded")
                .font(.system(size: GnatMetrics.body))
                .ink(.warning)
                .lineLimit(1)
                .help(message)
            Spacer(minLength: 0)
            Button("Retry") {
                Task {
                    await appModel.activateProject(project.id)
                    await appModel.refresh()
                }
            }
            .buttonStyle(GnatButtonStyle())
        }
        .padding(.leading, leading)
        .padding(.trailing, 10)
        .frame(height: GnatMetrics.sidebarRowHeight + 4)
    }

    private func milestoneHead(
        name: String, count: String, key: String, indent: CGFloat = 26, openByDefault: Bool = true,
        isDone: Bool = false, menu: (() -> AnyView)? = nil
    ) -> some View {
        let open = isOpen(key, byDefault: openByDefault)
        return TreeMilestoneLine(
            name: name, count: count, open: open, indent: indent, isDone: isDone, folds: true, menu: menu)
            .transformEnvironment(\.hoverForced) { if key == hoveredMilestone { $0 = true } }
            .contentShape(Rectangle())
            .onTapGesture { toggle(key, open: open) }
    }

    private func sliceRow(_ row: SidebarSliceRow, indent: CGFloat = 34) -> some View {
        let selected = appModel.activeProjectID == row.projectID && appModel.selectedSliceID == row.sliceID
        return sliceLine(
            title: row.title, state: row.state, live: row.live, selected: selected, indent: indent, marks: row.marks,
            menu: { AnyView(sliceRowMenu(row)) })
            .transformEnvironment(\.hoverForced) { if row.sliceID == hoveredSlice { $0 = true } }
            .onTapGesture { Task { await appModel.selectSlice(row.sliceID, inProject: row.projectID) } }
            .contextMenu { sliceRowMenu(row) }
    }

    private func sliceLine(
        title: String, state: SliceDisplayState, live: Bool, selected: Bool, indent: CGFloat = 34,
        marks: PRMarks = .none, menu: (() -> AnyView)? = nil
    ) -> some View {
        TreeSliceLine(
            title: title, state: state, live: live, selected: selected, indent: indent, marks: marks, menu: menu)
    }

    /// The project's finished milestones, once it has one: a Done folder at
    /// the foot of its tree, folded unless it holds the selected slice, each
    /// milestone in it a folder of its own, folded the same way.
    @ViewBuilder
    private func doneFolder(_ project: SidebarProject, outdent: CGFloat = 0) -> some View {
        if !project.doneMilestones.isEmpty {
            let selected = appModel.activeProjectID == project.id ? appModel.selectedSliceID : nil
            let key = "d:\(project.id)"
            let holdsSelection = selected.map(project.doneContains) ?? false
            let sliceCount = project.doneMilestones.reduce(0) { $0 + $1.total }
            milestoneHead(
                name: "Done", count: "\(sliceCount)", key: key, indent: 26 - outdent,
                openByDefault: holdsSelection, isDone: true)
            if isOpen(key, byDefault: holdsSelection) {
                ForEach(project.doneMilestones) { milestone in
                    let milestoneKey = "m:\(project.id)/\(milestone.name)"
                    let opensItself = selected.map { id in milestone.slices.contains { $0.sliceID == id } } ?? false
                    milestoneHead(
                        name: milestone.name, count: "\(milestone.done)/\(milestone.total)", key: milestoneKey,
                        indent: 36 - outdent, openByDefault: opensItself,
                        menu: { AnyView(milestoneMenu(project.id, milestone.name)) })
                        .contextMenu { milestoneMenu(project.id, milestone.name) }
                    if isOpen(milestoneKey, byDefault: opensItself) {
                        ForEach(milestone.slices) { sliceRow($0, indent: 44 - outdent) }
                    }
                }
            }
        }
    }

    /// The active project's ended ad hoc sessions, under a heading of their
    /// own at the foot of its tree — they belong to no milestone.
    @ViewBuilder
    private func endedSessions(_ project: SidebarProject, outdent: CGFloat = 0) -> some View {
        let live = (appModel.activityStore?.agents ?? [:]).mapValues { AgentActivity($0.activity) }
        let ended = (appModel.sessionStore?.sessions ?? [])
            .filter { sessionIsDone($0, liveAgents: live) }
            .sorted { $0.startedAt > $1.startedAt }
        if !ended.isEmpty {
            let key = "m:\(project.id)/~sessions"
            milestoneHead(name: "Ad hoc sessions", count: "\(ended.count)", key: key, indent: 26 - outdent)
            if isOpen(key) {
                ForEach(ended) { session in
                    sliceLine(
                        title: session.label, state: .done, live: false,
                        selected: appModel.selectedSessionID == session.id, indent: 34 - outdent)
                        .onTapGesture { appModel.selectedSessionID = session.id }
                        .contextMenu {
                            Button("Discard\u{2026}", systemImage: "trash", role: .destructive) { sessionForDiscard = session.id }
                        }
                }
            }
        }
    }

    // MARK: - Menus

    @ViewBuilder
    private func projectMenu(_ project: SidebarProject) -> some View {
        if project.kind != .untitled {
            addItems(project)
            Divider()
        }
        if project.kind == .project, let url = NotionPageURL.forPage(project.id) {
            Button("Open in Notion", systemImage: "arrow.up.right.square") { NSWorkspace.shared.open(url) }
        }
        if let directory = workingDirectory(of: project.id) {
            Button("Reveal working directory in Finder", systemImage: "folder") {
                NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: directory)])
            }
        }
        if project.kind == .project {
            Button("Project settings\u{2026}", systemImage: "gearshape") { projectForSettings = project }
        }
        if ProjectTabRules.showsClose(tabCount: appModel.closableTabCount, isScratch: project.kind == .scratch) {
            Divider()
            Button("Close project", systemImage: "xmark.circle") { requestClose(project.id) }
        }
    }

    /// Whether `projectMenu` has anything to show — what earns a row its
    /// three-dot button. Every project and Scratch has its add items; an
    /// Untitled tab only Reveal and Close.
    private func projectMenuHasItems(_ project: SidebarProject) -> Bool {
        project.kind != .untitled
            || workingDirectory(of: project.id) != nil
            || ProjectTabRules.showsClose(tabCount: appModel.closableTabCount, isScratch: false)
    }

    @ViewBuilder
    private func milestoneMenu(_ projectID: String, _ name: String) -> some View {
        let milestones = appModel.plan(projectID: projectID)?.milestones ?? []
        let filed = appModel.plan(projectID: projectID)?.slices.filter { $0.milestoneID == name }.count ?? 0
        let actions = MilestoneMenuRules.actions(for: name, in: milestones, sliceCount: filed)

        Button("New task\u{2026}", systemImage: "plus") { newSliceTarget = NewSliceTarget(projectID: projectID, milestone: name) }
        Button("Rename\u{2026}", systemImage: "pencil") {
            renameText = name
            milestoneForRename = MilestoneRef(projectID: projectID, name: name)
        }
        Divider()
        Button("Move up", systemImage: "arrow.up") {
            run { try await NatClient().milestoneMove(projectID: projectID, name: name, before: actions.moveBefore, after: nil) }
        }
        .disabled(actions.moveBefore == nil)
        Button("Move down", systemImage: "arrow.down") {
            run { try await NatClient().milestoneMove(projectID: projectID, name: name, before: nil, after: actions.moveAfter) }
        }
        .disabled(actions.moveAfter == nil)
        Divider()
        Button("Delete", systemImage: "trash", role: .destructive) { milestoneForDeletion = MilestoneRef(projectID: projectID, name: name) }
            .disabled(!actions.canDelete)
    }

    /// A slice row's menu, its right-click's and its three-dot's alike.
    @ViewBuilder
    private func sliceRowMenu(_ row: SidebarSliceRow) -> some View {
        sliceMenu(row, milestone: appModel.plan(projectID: row.projectID)?
            .slices.first { $0.id == row.sliceID }?.milestoneID ?? "")
    }

    /// A slice's menu — the rail's own, each item enabled exactly when the
    /// control it mirrors is.
    @ViewBuilder
    private func sliceMenu(_ row: SidebarSliceRow, milestone: String) -> some View {
        let plan = appModel.plan(projectID: row.projectID)
        let page = plan?.slices.first { $0.id == row.sliceID }
        let targets = (plan?.milestones ?? []).sorted { $0.order < $1.order }.filter { $0.id != milestone }
        let hasLiveAgent = appModel.activityStore?.agents[row.sliceID] != nil

        Button("Launch agent", systemImage: "play.circle") { launch(row) }
            .disabled(page.map { !LaunchPlan(for: $0, hasLiveAgent: hasLiveAgent).canLaunch } ?? true)
        Button("Edit description\u{2026}", systemImage: "pencil") { sliceForEdit = row }
            .disabled(page?.status != "Todo")
        if let url = page.flatMap({ URL(string: $0.url) }) ?? NotionPageURL.forPage(row.sliceID) {
            Button("Open in Notion", systemImage: "arrow.up.right.square") { NSWorkspace.shared.open(url) }
        }
        Divider()
        if !targets.isEmpty {
            Menu("Move to", systemImage: "folder") {
                ForEach(targets) { target in
                    Button(target.name) {
                        run { try await NatClient().sliceMove(projectID: row.projectID, sliceRef: row.sliceID, milestone: target.name) }
                    }
                }
            }
        }
        Button("Delete\u{2026}", systemImage: "trash", role: .destructive) { sliceForDeletion = (row, page?.status == "Done") }
    }

    // MARK: - Actions

    private func workingDirectory(of projectID: String) -> String? {
        let directory = appModel.config?.projects[projectID]?.workingDir
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return directory.isEmpty ? nil : directory
    }

    private func requestClose(_ projectID: String) {
        // Only a tab whose close ends a planning session (an Untitled
        // one's) asks, and only where that session has something to lose.
        if appModel.tabHasLiveWorkshop(projectID), let message = appModel.workshopEndConfirmation(forTab: projectID) {
            workshopCloseMessage = message
            projectPendingClose = projectID
        } else {
            Task { await appModel.closeProject(projectID) }
        }
    }

    /// Runs a nat write and refreshes, surfacing nat's own refusal. The write
    /// went through the replica, so the replica is what is read back — a
    /// staleness pull would only wait on news we made. Kept although the
    /// write also nudges: the watcher only notices on its next one-second
    /// tick, and only once a project has started it, where this read lands
    /// the write's result straight away for the cost of a file read.
    private func run(_ work: @escaping @MainActor () async throws -> Void) {
        Task {
            do {
                try await work()
                await appModel.refresh(.replica)
            } catch {
                actionError = commandMessage(of: error)
            }
        }
    }

    private func launch(_ row: SidebarSliceRow) {
        let agent = appModel.config?.sliceAgent
        run {
            _ = try await NatClient().sliceLaunch(
                projectID: row.projectID, sliceRef: row.sliceID, model: agent?.model, effort: agent?.effort)
            await appModel.selectSlice(row.sliceID, inProject: row.projectID)
        }
    }

    fileprivate func renameMilestone(_ ref: MilestoneRef) {
        let trimmed = renameText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty, trimmed != ref.name else { return }
        let from = ref.name
        run { try await NatClient().milestoneRename(projectID: ref.projectID, from: from, to: trimmed) }
        if let folded = fold.removeValue(forKey: "m:\(ref.projectID)/\(from)") {
            fold["m:\(ref.projectID)/\(trimmed)"] = folded
        }
    }

    fileprivate func addMilestone(_ projectID: String) {
        let name = newMilestoneText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !name.isEmpty else { return }
        run { try await NatClient().milestoneAdd(projectID: projectID, name: name) }
    }

    fileprivate func deleteSlice(_ row: SidebarSliceRow) {
        run {
            try await NatClient().sliceDelete(projectID: row.projectID, sliceRef: row.sliceID)
            if appModel.selectedSliceID == row.sliceID { appModel.selectedSliceID = nil }
        }
    }

    private func startNewSession(inProject projectID: String) async {
        var folder: String?
        if appModel.sessionNeedsFolder(inProject: projectID) {
            let panel = NSOpenPanel()
            panel.canChooseDirectories = true
            panel.canChooseFiles = false
            panel.allowsMultipleSelection = false
            panel.canCreateDirectories = true
            panel.prompt = "Start session"
            panel.message = "Choose the folder the session runs in"
            if let last = appModel.lastSessionFolder { panel.directoryURL = URL(fileURLWithPath: last) }
            guard panel.runModal() == .OK, let path = panel.url?.path else { return }
            folder = path
        }
        await appModel.launchSession(inProject: projectID, dir: folder)
        if let error = appModel.newSessionError { actionError = error }
    }

    private func commandMessage(of error: Error) -> String {
        if case NatError.commandFailed(let message) = error { return message }
        return error.localizedDescription
    }

    // MARK: - Dialogs

    /// Every sheet and alert the sidebar's menus open, kept off the body so
    /// the layout above reads as the design does.
    private struct SidebarDialogs: ViewModifier {
        let view: SidebarView

        private func presenting<Value>(_ value: Binding<Value?>) -> Binding<Bool> {
            Binding(get: { value.wrappedValue != nil }, set: { if !$0 { value.wrappedValue = nil } })
        }

        func body(content: Content) -> some View {
            let appModel = view.appModel
            return content
                .alert(
                    "Delete \u{201C}\(view.sliceForDeletion?.row.title ?? "")\u{201D}?",
                    isPresented: Binding(
                        get: { view.sliceForDeletion != nil },
                        set: { if !$0 { view.sliceForDeletion = nil } })
                ) {
                    Button("Delete", role: .destructive) {
                        if let row = view.sliceForDeletion?.row { view.deleteSlice(row) }
                    }
                    Button("Cancel", role: .cancel) {}
                } message: {
                    Text(view.sliceForDeletion?.done == true
                        ? "This task is Done, so deleting it removes the record of finished work. The page goes to Notion's trash."
                        : "The page goes to Notion's trash.")
                }
                .alert(
                    "Rename \u{201C}\(view.milestoneForRename?.name ?? "")\u{201D}",
                    isPresented: presenting(view.$milestoneForRename),
                    presenting: view.milestoneForRename
                ) { ref in
                    TextField("Milestone name", text: view.$renameText)
                        .font(Typo.mono(size: Typo.input))
                    Button("Rename") { view.renameMilestone(ref) }
                    Button("Cancel", role: .cancel) {}
                } message: { _ in
                    Text("The tasks filed under it are refiled onto the new name, and it keeps its place in the plan.")
                }
                .alert(
                    "New milestone",
                    isPresented: presenting(view.$newMilestoneProject),
                    presenting: view.newMilestoneProject
                ) { projectID in
                    TextField("Milestone name", text: view.$newMilestoneText)
                        .font(Typo.mono(size: Typo.input))
                    Button("Add") { view.addMilestone(projectID) }
                    Button("Cancel", role: .cancel) {}
                } message: { _ in
                    Text("It goes at the end of the plan, empty, ready for tasks.")
                }
                .alert(
                    "Delete \u{201C}\(view.milestoneForDeletion?.name ?? "")\u{201D}?",
                    isPresented: presenting(view.$milestoneForDeletion),
                    presenting: view.milestoneForDeletion
                ) { ref in
                    Button("Delete", role: .destructive) {
                        view.run { try await NatClient().milestoneRemove(projectID: ref.projectID, name: ref.name) }
                    }
                    Button("Cancel", role: .cancel) {}
                } message: { _ in
                    Text("The milestone is dropped from the plan. It holds no tasks, so no work goes with it.")
                }
                .sheet(item: view.$newSliceTarget) { target in
                    NewSliceSheetView(
                        projectID: target.projectID,
                        milestones: appModel.plan(projectID: target.projectID)?.milestones ?? [],
                        initialMilestone: target.milestone,
                        milestoneOptional: target.projectID == appModel.scratchProjectID,
                        onClose: { view.newSliceTarget = nil },
                        onCreated: {
                            view.newSliceTarget = nil
                            Task { await appModel.refresh(.replica) }
                        }
                    )
                }
                .sheet(item: view.$newTaskContainer) { target in
                    NewSliceSheetView(
                        projectID: target.projectID,
                        milestones: [],
                        container: NewSliceSheetView.Container(
                            id: target.container.id, title: target.container.title, noun: target.noun),
                        onClose: { view.newTaskContainer = nil },
                        onCreated: {
                            view.newTaskContainer = nil
                            Task { await appModel.refresh(.replica) }
                        }
                    )
                }
                .sheet(item: view.$sourceActionNeedingText) { pending in
                    SourceActionTextSheet(
                        action: pending.action,
                        onCancel: { view.sourceActionNeedingText = nil },
                        onRun: { text in
                            view.sourceActionNeedingText = nil
                            view.runSourceAction(pending, input: text)
                        })
                }
                .alert(
                    "\(view.sourceActionToConfirm?.action.label ?? "")?",
                    isPresented: presenting(view.$sourceActionToConfirm),
                    presenting: view.sourceActionToConfirm
                ) { pending in
                    Button(pending.action.label, role: .destructive) { view.runSourceAction(pending, input: nil) }
                    Button("Cancel", role: .cancel) {}
                } message: { _ in
                    Text("The source plugin does this in its own records.")
                }
                .sheet(item: view.$sliceForEdit) { row in
                    EditBriefSheetView(
                        projectID: row.projectID,
                        sliceID: row.sliceID,
                        sliceName: row.title,
                        onClose: { view.sliceForEdit = nil },
                        onSaved: {
                            view.sliceForEdit = nil
                            Task { await appModel.refresh(.replica) }
                        }
                    )
                }
                .sheet(item: view.$projectForSettings) { project in
                    ProjectSettingsView(appModel: appModel, projectID: project.id, projectName: project.name)
                }
                .sheet(isPresented: Bindable(appModel).mirrorPickerPresented) {
                    NotionPickerSheetView(
                        model: appModel.makeNotionPicker(),
                        onCancel: { appModel.mirrorPickerPresented = false },
                        onCreate: { place in await appModel.mirrorActiveProject(into: place) },
                        onCreated: { appModel.mirrorPickerPresented = false }
                    )
                }
                .alert("End the workshop session?", isPresented: view.$workshopPendingClose) {
                    Button("End session", role: .destructive) {
                        Task {
                            if let refusal = await appModel.closeWorkshopTab() { view.actionError = refusal }
                        }
                    }
                    Button("Cancel", role: .cancel) {}
                } message: {
                    Text(view.workshopCloseMessage)
                }
                .alert(
                    "End the workshop session?",
                    isPresented: presenting(view.$projectPendingClose),
                    presenting: view.projectPendingClose
                ) { projectID in
                    Button("End session", role: .destructive) {
                        Task {
                            if let refusal = await appModel.closeProject(projectID) { view.actionError = refusal }
                        }
                    }
                    Button("Cancel", role: .cancel) {}
                } message: { _ in
                    Text(view.workshopCloseMessage)
                }
                .alert(
                    "Discard this session?",
                    isPresented: presenting(view.$sessionForDiscard),
                    presenting: view.sessionForDiscard
                ) { id in
                    Button("Discard", role: .destructive) {
                        Task {
                            if let refusal = await appModel.discardSession(id: id) { view.actionError = refusal }
                        }
                    }
                    Button("Cancel", role: .cancel) {}
                } message: { _ in
                    Text("Its worktree is removed. This cannot be undone.")
                }
                .alert(
                    "That didn't work",
                    isPresented: presenting(view.$actionError),
                    presenting: view.actionError
                ) { _ in
                    Button("OK") {}
                } message: { message in
                    Text(message)
                }
        }
    }
}

private struct ShowsDoneItemsKey: EnvironmentKey {
    static let defaultValue = true
}

extension EnvironmentValues {
    /// View ▸ Show/Hide Done Items, as the app's menu holds it — an
    /// environment value rather than the sidebar's own storage read so a
    /// gallery story can say which it is about.
    var showsDoneItems: Bool {
        get { self[ShowsDoneItemsKey.self] }
        set { self[ShowsDoneItemsKey.self] = newValue }
    }
}
