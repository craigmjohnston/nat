import AppKit
import SwiftUI
import NatKit

/// The gnat design's sidebar: the Active fold — every project's work in
/// flight, needs-you first, tagged `Project / title` — and the Projects
/// fold, each project a disclosure over its milestones and their slices —
/// then the Scratch fold, the scratch project's milestones straight under it.
///
/// Everything the old project tabs and rail did that the design does not
/// draw lives on here as the row it belongs to: a project's menu (New Slice,
/// Workshop…, Open in Notion, Reveal, Close), a milestone's (New
/// Slice, Rename, Move, Delete), a slice's (Launch, Edit, Open, Move, Delete),
/// each project's own `+` (New Milestone, New Slice, Workshop…, New Ad Hoc
/// Session), the titlebar's `+` (any of those in a project it asks for, or a
/// new project) beside its Settings cog, and an ended session under its
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
    @State private var actionError: String?
    @State private var newMilestoneProject: String?
    @State private var newMilestoneText = ""
    /// The Scratch fold's tree at its natural height: what it takes, at most,
    /// beside an open Projects tree.
    @State private var scratchContentHeight: CGFloat = 0
    /// The project row under the pointer, whose folder turns into its fold
    /// chevron.
    @State private var hoveredProject: String?
    /// A source fold's row under the pointer — a group's or a container's,
    /// by `sourceRowKey` — which shows its hover-only meta, `+` and menu.
    @State private var hoveredSourceRow: String?
    @State private var newTaskContainer: NewTaskContainer?
    /// A source action waiting on its line of text, or on its confirmation.
    @State private var sourceActionNeedingText: PendingSourceAction?
    @State private var sourceActionToConfirm: PendingSourceAction?
    @State private var newSourceProjectPlugin: SourcePlugin?

    init(
        appModel: AppModel, onNewProject: @escaping () -> Void = {}, showsTitlebar: Bool = false,
        folded: [String: Bool] = [:], treeAnchor: UnitPoint? = nil
    ) {
        self.appModel = appModel
        self.onNewProject = onNewProject
        self.showsTitlebar = showsTitlebar
        self.treeAnchor = treeAnchor
        _fold = State(initialValue: folded)
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
        VStack(spacing: 0) {
            if showsTitlebar {
                titlebar(model)
            }
            head("active", label: "Active", count: model.needsYouCount) { EmptyView() }
            // Folded, Projects (and Scratch under it) pins to the sidebar's
            // foot rather than leaving an empty well under its heading.
            let pinsProjects = !isOpen("work") && model.sources.isEmpty
                && !(model.scratch != nil && isOpen("scratch", byDefault: false))
            // The tree's scroll holds the source folds too, under Projects,
            // so it stays up with Projects folded while there are any.
            let treeShown = isOpen("work") || !model.sources.isEmpty
            if isOpen("active") {
                if model.active.isEmpty {
                    GnatNote(text: EmptyActiveNote.text.lowercased(), height: GnatMetrics.sidebarRowHeight)
                } else {
                    ForEach(model.active) { activeRow($0) }
                }
            }

            if pinsProjects {
                Spacer(minLength: 0)
            }

            // 4pt of air above the line under an open Active; under a folded
            // heading there is none, which would set the heading above it
            // off-centre. The line is not drawn while the row over it is
            // selected, so the selection reads as one block rather than a
            // wash cut by a rule — its place is kept, so nothing moves.
            Rule(.separator)
                .opacity(activeEndsInSelection(model) ? 0 : 1)
                .padding(.top, isOpen("active") ? 4 : 0)

            head("work", label: "Projects", count: 0) {
                Button(action: onNewProject) {
                    Image(systemName: "folder.badge.plus")
                        .font(.system(size: 13))
                        .ink(.tertiary)
                        .frame(width: 20, height: 18)
                        .contentShape(Rectangle())
                }
                .buttonStyle(GnatIconButtonStyle())
                .help("New project\u{2026}")
            }
            if treeShown {
                ScrollView {
                    // One section per project and per source fold, its own
                    // row the header, pinned at the top while its milestones
                    // scroll under it.
                    LazyVStack(spacing: 0, pinnedViews: .sectionHeaders) {
                        if isOpen("work") {
                            ForEach(model.projects) { projectSection($0) }
                        }
                        ForEach(model.sources) { sourceSection($0) }
                    }
                }
                .defaultScrollAnchor(treeAnchor)
                .thinScrollers()
                .frame(maxHeight: .infinity)
            }

            if let scratch = model.scratch {
                scratchFold(
                    scratch, projectsOpen: treeShown,
                    hidesRule: model.sources.isEmpty && projectsEndInSelection(model))
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

    // MARK: - Headings

    private func head<Trailing: View>(
        _ key: String, label: String, count: Int, openByDefault: Bool = true, @ViewBuilder trailing: () -> Trailing
    ) -> some View {
        let open = isOpen(key, byDefault: openByDefault)
        return HStack(spacing: 6) {
            DisclosureChevron(open: open)
            Text(label.uppercased())
                .font(.system(size: 12))
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
            .frame(width: 18, height: 18)
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
        Button("New milestone\u{2026}", systemImage: "folder.badge.plus") {
            newMilestoneText = ""
            newMilestoneProject = project.id
        }
        Button("New task\u{2026}", systemImage: "plus") { newSliceTarget = NewSliceTarget(projectID: project.id, milestone: "") }
        Button("Workshop\u{2026}", systemImage: "sparkles") { Task { await appModel.selectWorkshop(inProject: project.id) } }
        Divider()
        Button(
            project.kind == .scratch ? "New ad hoc session\u{2026}" : "New ad hoc session",
            systemImage: "terminal"
        ) { Task { await startNewSession(inProject: project.id) } }
            .disabled(appModel.newSessionLaunching)
    }

    // MARK: - Titlebar

    /// The sidebar's segment of the window titlebar: past the traffic
    /// lights, Settings and the `+`, at its trailing edge over the project
    /// rows' own.
    private func titlebar(_ model: SidebarModel) -> some View {
        GnatTitlebar(leading: GnatMetrics.lightsInset) {
            Spacer(minLength: 0)
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
            // One per plugin that described itself: one that would not has
            // no project to make.
            ForEach(appModel.sourcePlugins.filter { $0.describe != nil }) { plugin in
                Button("New \(plugin.displayTitle) project\u{2026}", systemImage: plugin.iconSymbol) {
                    newSourceProjectPlugin = plugin
                }
            }
            Divider()
            projectSubmenu("New milestone", systemImage: "folder.badge.plus", targets) { project in
                newMilestoneText = ""
                newMilestoneProject = project.id
            }
            projectSubmenu("New task", systemImage: "plus", targets) { project in
                newSliceTarget = NewSliceTarget(projectID: project.id, milestone: "")
            }
            projectSubmenu("Workshop", systemImage: "sparkles", targets) { project in
                Task { await appModel.selectWorkshop(inProject: project.id) }
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

    /// A source project's fold: a heading of its own — the plugin's icon,
    /// the project's name, its header menu — over the plugin's groups, their
    /// containers, and each container's tasks.
    private func sourceSection(_ project: SidebarProject) -> some View {
        let open = isOpen("s:\(project.id)")
        return Section {
            if open {
                sourceBody(project)
            }
        } header: {
            sourceHead(project, open: open)
        }
    }

    /// A source fold's heading and the line over it, drawn on the
    /// sidebar's own ground so the rows scrolling under it while it is
    /// pinned do not show through.
    private func sourceHead(_ project: SidebarProject, open: Bool) -> some View {
        let key = "s:\(project.id)"
        let source = project.source
        return VStack(spacing: 0) {
            Rule(.separator).padding(.top, 4)
            HStack(spacing: 6) {
                DisclosureChevron(open: open)
                SourceIconView(icon: source?.icon ?? SourceIcon(symbol: ""), size: 13)
                    .ink(.secondary)
                Text(project.name.uppercased())
                    .font(.system(size: 12))
                    .tracking(0.7)
                    .ink(.secondary)
                    .lineLimit(1)
                if !open && project.needsYou > 0 {
                    Text("\(project.needsYou)").monoXS().ink(.hot)
                }
                Spacer(minLength: 0)
                if let source, !source.menu.isEmpty {
                    Menu {
                        actionItems(source.menu, projectID: project.id)
                    } label: {
                        Image(systemName: "ellipsis")
                            .font(.system(size: 12, weight: .medium))
                            .ink(.tertiary)
                            .frame(width: 18, height: 18)
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
        .surface(.header)
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
                    .font(.system(size: 11, weight: .medium))
                    .tracking(0.7)
                    .ink(.tertiary)
                    .lineLimit(1)
                if let count = group.count {
                    Text("\(count)").font(Typo.mono(size: 11)).ink(.tertiary)
                }
                Spacer(minLength: 0)
                if !group.menu.isEmpty {
                    Menu {
                        actionItems(group.menu, projectID: project.id, group: group.id)
                    } label: {
                        Image(systemName: "ellipsis")
                            .font(.system(size: 11, weight: .medium))
                            .ink(.tertiary)
                            .frame(width: 16, height: 16)
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
                if !group.menu.isEmpty { actionItems(group.menu, projectID: project.id, group: group.id) }
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
            Image(systemName: SourceGlyph.container)
                .font(.system(size: 11))
                .ink(.tertiary)
                .frame(width: 16)
            Text(container.title)
                .font(.system(size: GnatMetrics.body))
                .ink(.secondary)
                .lineLimit(1)
            Spacer(minLength: 0)
            // The meta and the `+` only under the pointer, taking no room
            // otherwise: a sidebar is narrow, and the title wants it.
            if hovered, let meta = container.meta {
                Text(meta).monoXS().ink(.tertiary)
            }
            ForEach(Array(container.badges.enumerated()), id: \.offset) { _, badge in
                SourceBadgeView(badge: badge)
            }
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
                .help("New \(source.taskNoun) on this \(source.containerNoun)")
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
        if let source = project.source, !source.menu.isEmpty {
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

    private func actionItems(
        _ actions: [SourceAction], projectID: String, group: String? = nil, container: String? = nil
    ) -> some View {
        SourceActionItems(
            actions: actions,
            onRun: { action, input in
                runSourceAction(PendingSourceAction(projectID: projectID, action: action, group: group, container: container), input: input)
            },
            onText: { sourceActionNeedingText = PendingSourceAction(projectID: projectID, action: $0, group: group, container: container) },
            onConfirm: { sourceActionToConfirm = PendingSourceAction(projectID: projectID, action: $0, group: group, container: container) })
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
            ActiveIdentityLabel(tag: row.projectTag, state: row.state, live: row.live, title: row.title)
            Spacer(minLength: 0)
            if row.kind == .workshop {
                Button { closeWorkshopRow(row) } label: {
                    Image(systemName: "xmark")
                        .font(.system(size: 10, weight: .medium))
                        .ink(.tertiary)
                        .frame(width: 16, height: 16)
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
        Task {
            await appModel.selectWorkshop(inProject: row.projectID)
            workshopPendingClose = true
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
                Task {
                    await appModel.selectWorkshop(inProject: row.projectID)
                    workshopPendingClose = true
                }
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
                projectBody(project, isActive: appModel.activeProjectID == project.id)
            }
        } header: {
            projectHead(project, open: open)
        }
    }

    /// A project's own row: the fold toggle, and its section's header —
    /// on the sidebar's ground, so its milestones scrolling under it while
    /// it is pinned do not show through.
    private func projectHead(_ project: SidebarProject, open: Bool) -> some View {
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
            .frame(width: 16)
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
            .ink(.secondary)
            .lineLimit(1)
            Spacer(minLength: 0)
            if !open && project.needsYou > 0 {
                Circle().fill(DesignTokens.hot).frame(width: 6, height: 6)
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
        .gnatRow(selected: project.kind == .untitled && isActive)
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
                openByDefault: opensItself)
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
    /// never drawn as one. Beside an open Projects tree it takes only the
    /// height it needs; with Projects folded, everything that is left.
    @ViewBuilder
    private func scratchFold(_ scratch: SidebarProject, projectsOpen: Bool, hidesRule: Bool) -> some View {
        Rule(.separator).opacity(hidesRule ? 0 : 1).padding(.top, projectsOpen ? 4 : 0)
        head("scratch", label: "Scratch", count: 0, openByDefault: false) { addMenu(scratch) }
            .contextMenu { addItems(scratch) }
        if isOpen("scratch", byDefault: false) {
            ScrollView {
                VStack(spacing: 0) {
                    projectBody(scratch, isActive: appModel.activeProjectID == scratch.id, outdent: 8)
                }
                .padding(.bottom, 4)
                .onGeometryChange(for: CGFloat.self) { $0.size.height } action: { scratchContentHeight = $0 }
            }
            .thinScrollers()
            .frame(maxHeight: projectsOpen ? scratchContentHeight : .infinity)
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
        isDone: Bool = false
    ) -> some View {
        let open = isOpen(key, byDefault: openByDefault)
        return TreeMilestoneLine(name: name, count: count, open: open, indent: indent, isDone: isDone)
            .contentShape(Rectangle())
            .onTapGesture { toggle(key, open: open) }
    }

    private func sliceRow(_ row: SidebarSliceRow, indent: CGFloat = 34) -> some View {
        let selected = appModel.activeProjectID == row.projectID && appModel.selectedSliceID == row.sliceID
        return sliceLine(title: row.title, state: row.state, live: row.live, selected: selected, indent: indent)
            .onTapGesture { Task { await appModel.selectSlice(row.sliceID, inProject: row.projectID) } }
            .contextMenu {
                sliceMenu(row, milestone: appModel.plan(projectID: row.projectID)?
                    .slices.first { $0.id == row.sliceID }?.milestoneID ?? "")
            }
    }

    private func sliceLine(
        title: String, state: SliceDisplayState, live: Bool, selected: Bool, indent: CGFloat = 34
    ) -> some View {
        TreeSliceLine(title: title, state: state, live: live, selected: selected, indent: indent)
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
                        indent: 36 - outdent, openByDefault: opensItself)
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
        if ProjectTabRules.showsClose(tabCount: appModel.closableTabCount, isScratch: project.kind == .scratch) {
            Divider()
            Button("Close project", systemImage: "xmark.circle") { requestClose(project.id) }
        }
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
        if appModel.tabHasLiveWorkshop(projectID) {
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
                        .font(Typo.mono(size: Typo.code))
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
                        .font(Typo.mono(size: Typo.code))
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
                .sheet(item: view.$newSourceProjectPlugin) { plugin in
                    NewSourceProjectSheetView(
                        plugin: plugin,
                        onClose: { view.newSourceProjectPlugin = nil },
                        onAdded: { id, name in
                            view.newSourceProjectPlugin = nil
                            Task { await appModel.addProject(id: id, name: name) }
                        })
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
                    Text("The planning agent is still running. Ending it ends its session; the draft goes with it.")
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
                    Text("The planning agent is still running. Closing the project ends its session; the draft goes with it.")
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
