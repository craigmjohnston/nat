import AppKit
import SwiftUI
import NatKit
import NatFixtures

/// The rail's file-tree geometry, shared by every row so the marks line up on
/// one vertical axis: heading icons, session dots and every chevron sit in
/// the same fixed-width `slot` at the same `leading` offset, which puts every
/// title and heading label on one shared left edge; each tree level indents
/// by `indent`. Rows span the rail's full width for their tap
/// targets, and hover paints across that whole width — square and edge to
/// edge, so the highlight fills its container rather than floating inside it
/// — with content kept inside `leading`/`trailing`. Selection still paints as
/// a rounded chip inset from the edges, which is what tells the row the user
/// picked from the row the pointer happens to be over.
/// Internal rather than private, so `RailSkeletonView` builds its
/// placeholder rows on exactly these numbers instead of a copy of them —
/// the whole point of the skeleton being that the plan lands on a layout
/// that is already right.
enum RailSlot {
    static let slot: CGFloat = 13
    static let spacing: CGFloat = 8
    static let leading: CGFloat = 17
    static let trailing: CGFloat = 20
    static let indent: CGFloat = 20
    /// Every tree row's height — folders, slices and the DONE heading.
    static let rowHeight: CGFloat = 28
}

/// Every rail row's hover: the shared `.hoverWash()` (see ViewHelpers.swift)
/// with no radius at all, so the wash meets the rail's own edges instead of
/// floating as a chip inside them. The rail's own inset version is gone
/// rather than zeroed — with the radius and the inset both at nothing there
/// was nothing left in it the shared helper does not already do.
extension View {
    fileprivate func railHoverWash() -> some View {
        hoverWash(cornerRadius: 0)
    }

    /// How the rail measures the pieces it shares its height between — a
    /// section's chrome, a section's list. One helper rather than the same
    /// `onGeometryChange` written out six times.
    fileprivate func measuringHeight(_ action: @escaping (CGFloat) -> Void) -> some View {
        onGeometryChange(for: CGFloat.self) { proxy in
            proxy.size.height
        } action: { height in
            action(height)
        }
    }
}

struct RailView: View {
    /// The two tree glyphs' own sizes: the folder shape, which is drawn
    /// rather than set in type, and the slice icon's point size. Named
    /// rather than inline so `RailSkeletonView` stands in for them at what
    /// they actually come to.
    static let folderGlyphHeight: CGFloat = 10.5
    static let sliceGlyphSize: CGFloat = 12

    @Bindable var appModel: AppModel
    /// What the rail is drawn on, for the few colours it has to compute as a
    /// value rather than apply as a modifier — a selection fill inside a
    /// ternary has nowhere to read the environment from on its own.
    @Environment(\.ground) private var ground
    @State private var expandedMilestones: Set<String> = []
    @State private var expandedSeeded = false
    /// Milestone IDs whose DONE-section folder is expanded. Its own set
    /// rather than `expandedMilestones`, since a part-done milestone is a
    /// folder in both sections and the two fold independently.
    @State private var expandedDoneFolders: Set<String> = []
    /// The sections folded away to their heading alone. The view's own
    /// state, like `expandedMilestones`: which of the three the user has put
    /// away is about this rail on this screen and nothing the plan records.
    /// DONE starts folded, which is the fold the rail always had.
    @State private var collapsed: Set<RailSection>
    /// The slice the delete menu item was picked on, held while its confirm
    /// dialog is up — deleting is the one rail action that cannot be undone
    /// from here (the page goes to Notion's trash), so it asks first, the way
    /// the board's own d does.
    @State private var sliceForDeletion: MilestoneSliceRow?
    /// The ad hoc session Discard was picked on, held while its confirm
    /// dialog is up — discarding removes the worktree, the one rail action
    /// on a session that cannot be undone, mirroring `sliceForDeletion`.
    @State private var sessionForDiscard: ActiveEntry?
    /// The three numbers `RailSectionLayout` shares the rail out on: how
    /// tall the rail's container is, how much of it each section's own
    /// chrome — its rule and its pinned heading — has already taken, and how
    /// tall each section's list comes to. All three are measured rather than
    /// assumed, since all three move: the rail is a resizable column, a
    /// section's heading comes and goes with the section, and the lists grow
    /// an entry at a time as agents start and slices land. The chrome and
    /// the lists are read off the views themselves, which is what they come
    /// to; `railHeight` is read off the container alone — see `body`.
    @State private var railHeight: CGFloat = 0
    @State private var chromeHeights: [RailSection: CGFloat] = [:]
    @State private var contentHeights: [RailSection: CGFloat] = [:]
    /// What the last move or delete refused with — a slice in progress, gh
    /// down, whatever nat said — shown in an alert and cleared by dismissing
    /// it. The rail has no status bar to toast on.
    @State private var actionError: String?
    /// The milestone a folder's "New Slice…" was picked on, held while its
    /// sheet is up so the picker opens on the folder the menu was opened on.
    @State private var milestoneForNewSlice: String?
    /// TODO's own "New Slice…" button, beside its fold chevron — the same
    /// sheet a folder's menu opens, but with no milestone preselected.
    @State private var showTodoNewSliceSheet = false
    /// The milestone a folder's "Rename…" was picked on, and the name being
    /// typed for it. The text is seeded with the name it has, since a rename
    /// is nearly always an edit of what is there rather than a fresh string.
    @State private var milestoneForRename: String?
    @State private var renameText: String = ""
    /// The folder "Delete" was picked on, held while its confirm dialog is
    /// up. Only an empty milestone reaches it — `milestone-remove` refuses
    /// one still holding slices and the menu offers it only when it is empty
    /// — so what the dialog warns about is the plan losing a heading rather
    /// than any work.
    @State private var milestoneForDeletion: MilestoneFolder?
    /// The slice "Edit Description…" was picked on, held while the brief
    /// sheet is up.
    @State private var sliceForEdit: MilestoneSliceRow?
    /// The ACTIVE entry the mouse is over, for the workshop row's ✕ — kept
    /// here rather than in `HoverWash`, whose own hover state a row's other
    /// modifiers cannot read. Any entry may be hovered, but only the
    /// workshop row ever draws a ✕ off it.
    @State private var hoveredActiveEntryID: String?
    /// True while the workshop close's confirm is up — asked only when a
    /// planning agent is actually live, mirroring the delete alert's own
    /// shape (`sliceForDeletion`) rather than sharing it, since what is being
    /// confirmed about is a live session and not a page.
    @State private var confirmingWorkshopClose = false

    /// `collapsedSections` is the gallery's seam and nothing else's: a fold
    /// is the user's own state, so the app takes the default — DONE away,
    /// the rest open — and a story seeds the fold it is a story about.
    init(appModel: AppModel, collapsedSections: Set<RailSection> = [.done]) {
        self.appModel = appModel
        _collapsed = State(initialValue: collapsedSections)
    }

    var railModel: RailModel {
        if let projectInfo = appModel.projectStore?.state.projectInfo {
            // Map ActivityStore agents to the rail model's format
            let liveAgents = (appModel.activityStore?.agents ?? [:])
                .mapValues { AgentActivity($0.activity) }
            return buildRailModel(
                from: projectInfo, liveAgents: liveAgents,
                reviewStats: appModel.reviewStatsStore?.stats ?? [:],
                reviewFileCounts: appModel.reviewStatsStore?.fileCounts ?? [:],
                prReadiness: appModel.reviewStatsStore?.prReadiness ?? [:],
                agentStarts: appModel.activityStore?.firstSeen ?? [:],
                workshop: workshopEntry,
                sessions: appModel.sessionStore?.sessions ?? [],
                fixLaunched: appModel.fixLaunchedSliceIDs
            )
        }
        // With no plan read, the workshop is still the one thing that can be
        // running: a project opened on an empty board and workshopped.
        return RailModel(active: workshopEntry.map { [$0] } ?? [], todoFolders: [])
    }

    /// The workshop's own ACTIVE entry — nil while no planning agent is
    /// live, none is launching and the composer is not open, which is when
    /// the section simply lists everything else.
    var workshopEntry: ActiveEntry? {
        let activity: AgentActivity? = appModel.planningAgent.map { AgentActivity($0.activity) }
        return buildWorkshopEntry(
            activity: activity,
            isLaunching: appModel.workshopLaunching,
            isSelected: appModel.workshopSelected,
            firstSeen: appModel.planningAgentKey.flatMap { appModel.activityStore?.firstSeen[$0] }
        )
    }

    var body: some View {
        // What the open sections divide between them, measured rather than
        // assumed: the rail is a resizable column in a resizable window, so
        // how much there is to share changes under the user's hands.
        //
        // The reading is taken off the container — a `GeometryReader` takes
        // the size it is proposed whatever its content comes to — and never
        // off `railColumn`, whose height is the sum of the very shares this
        // number produces. See `RailSectionLayout.available` for the circle
        // that was, and `WindowShellView` for the offer: the shell gives the
        // rail a width and the band's own height, so what is read here is
        // the window's height and the column is drawn inside it.
        GeometryReader { proxy in
            railColumn
                .frame(
                    width: proxy.size.width,
                    height: proxy.size.height,
                    alignment: .topLeading
                )
                .onChange(of: proxy.size.height, initial: true) { _, height in
                    railHeight = height
                }
        }
        .surface(.window)
        .rule(.separator, edges: [.trailing], width: 0.5)
        .alert(
            "Delete \u{201C}\(sliceForDeletion?.name ?? "")\u{201D}?",
            isPresented: Binding(
                get: { sliceForDeletion != nil },
                set: { if !$0 { sliceForDeletion = nil } }
            ),
            presenting: sliceForDeletion
        ) { slice in
            Button("Delete", role: .destructive) {
                Task { await deleteSlice(slice) }
            }
            Button("Cancel", role: .cancel) {}
        } message: { slice in
            // Mirrors the board's own confirm: a Done slice is finished work,
            // so dropping the record of it is warned about rather than refused.
            Text(slice.glyph == .done
                ? "This slice is Done — deleting it drops the record of finished work. The page goes to Notion's trash."
                : "The page goes to Notion's trash.")
        }
        .alert(
            "Rename \u{201C}\(milestoneForRename ?? "")\u{201D}",
            isPresented: presenting($milestoneForRename),
            presenting: milestoneForRename
        ) { name in
            TextField("Milestone name", text: $renameText)
                .font(Typo.mono(size: Typo.code))
            Button("Rename") {
                Task { await renameMilestone(name, to: renameText) }
            }
            Button("Cancel", role: .cancel) {}
        } message: { _ in
            // The one thing a rename costs, said before it is made: a
            // milestone is nothing but the name its slices carry, so
            // everything filed under it is refiled as part of the rename and
            // its place in the plan is kept.
            Text("The slices filed under it are refiled onto the new name, and it keeps its place in the plan.")
        }
        .alert(
            "Delete \u{201C}\(milestoneForDeletion?.title ?? "")\u{201D}?",
            isPresented: presenting($milestoneForDeletion),
            presenting: milestoneForDeletion
        ) { folder in
            Button("Delete", role: .destructive) {
                Task { await removeMilestone(folder) }
            }
            Button("Cancel", role: .cancel) {}
        } message: { _ in
            Text("The milestone is dropped from the plan. It holds no slices, so no work goes with it.")
        }
        .sheet(isPresented: presenting($milestoneForNewSlice)) {
            NewSliceSheetView(
                projectID: appModel.activeProjectID ?? "",
                milestones: planMilestones,
                initialMilestone: milestoneForNewSlice ?? "",
                onClose: { milestoneForNewSlice = nil },
                onCreated: {
                    milestoneForNewSlice = nil
                    Task { await appModel.refresh() }
                }
            )
        }
        .sheet(isPresented: $showTodoNewSliceSheet) {
            NewSliceSheetView(
                projectID: appModel.activeProjectID ?? "",
                milestones: planMilestones,
                onClose: { showTodoNewSliceSheet = false },
                onCreated: {
                    showTodoNewSliceSheet = false
                    Task { await appModel.refresh() }
                }
            )
        }
        .sheet(isPresented: presenting($sliceForEdit)) {
            EditBriefSheetView(
                projectID: appModel.activeProjectID ?? "",
                sliceID: sliceForEdit?.sliceID ?? "",
                sliceName: sliceForEdit?.name ?? "",
                onClose: { sliceForEdit = nil },
                onSaved: {
                    sliceForEdit = nil
                    Task { await appModel.refresh() }
                }
            )
        }
        .alert(
            "End the workshop session?",
            isPresented: $confirmingWorkshopClose
        ) {
            Button("End Session", role: .destructive) {
                Task { await closeWorkshop() }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("The planning agent is still running. Closing the tab ends its session; the draft goes with it.")
        }
        .alert(
            "Discard this session?",
            isPresented: presenting($sessionForDiscard),
            presenting: sessionForDiscard
        ) { entry in
            Button("Discard", role: .destructive) {
                Task { await discardSession(entry) }
            }
            Button("Cancel", role: .cancel) {}
        } message: { _ in
            Text("Its worktree is removed. This cannot be undone.")
        }
        .alert(
            "That didn't work",
            isPresented: Binding(
                get: { actionError != nil },
                set: { if !$0 { actionError = nil } }
            ),
            presenting: actionError
        ) { _ in
            Button("OK") {}
        } message: { message in
            Text(message)
        }
        // The first plan to land opens the milestones already moving — the
        // current one and any with work done or in flight — and leaves the
        // untouched ones closed, the way the mock draws the rail; everything
        // after that is the user's folding.
        .onChange(of: railModel.todoFolders.isEmpty, initial: true) { _, isEmpty in
            guard !expandedSeeded, !isEmpty else { return }
            expandedSeeded = true
            for folder in railModel.todoFolders
            where folder.isCurrent || folder.done > 0 || folder.inFlightCount > 0 {
                expandedMilestones.insert(folder.milestoneID)
            }
            if expandedMilestones.isEmpty, let first = railModel.todoFolders.first {
                expandedMilestones.insert(first.milestoneID)
            }
        }
    }

    // MARK: - The sections

    /// The rail as three sections stacked in one column, each a pinned
    /// heading over a scroll of its own: what is running, what is queued,
    /// what is finished. No heading ever moves — only the list under it does
    /// — so what is running is never something to go and scroll for, and
    /// reading the far end of TODO does not take DONE off the rail.
    ///
    /// What is left of the rail once the headings and the rules between them
    /// have taken their lines is shared out by `RailSectionLayout`: a
    /// collapsed section is not in that share at all, and an over-tall one
    /// scrolls within the share it is given rather than squeezing the others
    /// out.
    private var railColumn: some View {
        let heights = sectionHeights
        return VStack(alignment: .leading, spacing: 0) {
            activeSection(height: heights[.active])
            todoSection(height: heights[.todo])
            if railModel.doneSummary != nil || !railModel.doneSessions.isEmpty {
                doneSection(railModel.doneSummary, height: heights[.done])
            }
            // The air under the last section, and what takes up the rail's
            // slack when the three of them want less than there is.
            Spacer(minLength: CGFloat(RailSectionLayout.footRoom))
        }
    }

    /// ACTIVE — always drawn, holding entries or holding its own note: what
    /// is running is the rail's standing question, and a heading that only
    /// appeared once something was read would be chrome arriving from
    /// nowhere. It is the one flight section there is: the planning agent
    /// and the branches waiting on a review are entries of it rather than
    /// headings of their own, in that order.
    private func activeSection(height: CGFloat?) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            sectionHeading(.active, showNewSessionAction: !appModel.activeTabIsUntitled)
                .padding(.top, 12)
                .measuringHeight { chromeHeights[.active] = $0 }

            if let height {
                ScrollView {
                    VStack(alignment: .leading, spacing: 0) {
                        if railModel.active.isEmpty {
                            activeEmptyNote
                        } else {
                            ForEach(railModel.active) { entry in
                                activeRow(for: entry, isHovered: hoveredActiveEntryID == entry.id)
                                    .contentShape(Rectangle())
                                    .onTapGesture { select(entry) }
                                    .onHover { inside in
                                        if inside {
                                            hoveredActiveEntryID = entry.id
                                        } else if hoveredActiveEntryID == entry.id {
                                            hoveredActiveEntryID = nil
                                        }
                                    }
                                    .contextMenu { sessionMenuItems(for: entry) }
                            }
                        }
                    }
                    .inelastic()
                    .measuringHeight { contentHeights[.active] = $0 }
                }
                .scrollDisabled(!scrolls(.active, within: height))
                .frame(height: height)
            }
        }
    }

    /// TODO — the load's own states, then the milestones still holding work,
    /// folders in a file tree with their remaining slices as files. The load
    /// states belong here rather than to the rail at large because what they
    /// stand in for is the plan: the skeleton is the plan's own shape and the
    /// retry is the plan's read to make again.
    private func todoSection(height: CGFloat?) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            VStack(alignment: .leading, spacing: 0) {
                sectionRule
                sectionHeading(.todo, showTodoActions: !appModel.activeTabIsUntitled)
            }
            .measuringHeight { chromeHeights[.todo] = $0 }

            if let height {
                ScrollView {
                    VStack(alignment: .leading, spacing: 0) {
                        planLoadStates

                        ForEach(railModel.todoFolders, id: \.milestoneID) { folder in
                            folderRows(
                                folder,
                                inDone: false,
                                expanded: expandedMilestones.contains(folder.milestoneID),
                                onToggle: { toggle(folder.milestoneID, in: &expandedMilestones) }
                            )
                        }
                    }
                    .inelastic()
                    .measuringHeight { contentHeights[.todo] = $0 }
                }
                .scrollDisabled(!scrolls(.todo, within: height))
                .frame(height: height)
            }
        }
    }

    /// DONE — the finished slices' home at the foot of the rail: every
    /// milestone with work done lists under it as a folder of its own, one
    /// level deeper than the tree.
    private func doneSection(_ summary: DoneSummary?, height: CGFloat?) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            VStack(alignment: .leading, spacing: 0) {
                sectionRule
                sectionHeading(.done, trailing: summary.map { "\($0.doneCount)/\($0.totalCount)" })
            }
            .measuringHeight { chromeHeights[.done] = $0 }

            if let height {
                ScrollView {
                    VStack(alignment: .leading, spacing: 0) {
                        // Ended ad hoc sessions first, newest-started: they
                        // belong to no milestone, so there is no folder to
                        // file them under.
                        ForEach(railModel.doneSessions) { entry in
                            activeRow(for: entry, isHovered: hoveredActiveEntryID == entry.id)
                                .contentShape(Rectangle())
                                .onTapGesture { select(entry) }
                                .onHover { inside in
                                    if inside {
                                        hoveredActiveEntryID = entry.id
                                    } else if hoveredActiveEntryID == entry.id {
                                        hoveredActiveEntryID = nil
                                    }
                                }
                                .contextMenu { sessionMenuItems(for: entry) }
                        }

                        ForEach(railModel.doneFolders, id: \.milestoneID) { folder in
                            folderRows(
                                folder,
                                inDone: true,
                                expanded: expandedDoneFolders.contains(folder.milestoneID),
                                onToggle: { toggle(folder.milestoneID, in: &expandedDoneFolders) }
                            )
                        }
                    }
                    .inelastic()
                    .measuringHeight { contentHeights[.done] = $0 }
                }
                .scrollDisabled(!scrolls(.done, within: height))
                .frame(height: height)
            }
        }
    }

    /// The load's own states, at the head of the plan: a board that
    /// swallowed its failure would read as an empty tracker, which is worse
    /// than any error. A first load draws the plan's own skeleton, a failed
    /// first load says what nat said and offers the retry, and a failed
    /// refresh keeps the stale plan under one quiet warning line (the TUI
    /// convention: a failure leaves the board as it was).
    @ViewBuilder
    private var planLoadStates: some View {
        // An Untitled tab has no plan to be loading, empty or failed: it says
        // what will be here, wrapped and set under the entries' text column
        // (the heading's slot and its gap in from the rail's edge).
        if appModel.activeTabIsUntitled {
            Text(StarterCard.railExplainer)
                .font(.system(size: Typo.subhead, weight: .regular))
                .ink(.tertiary)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.leading, RailSlot.leading + RailSlot.slot + RailSlot.spacing)
                .padding(.trailing, RailSlot.trailing)
        }

        if let state = appModel.projectStore?.state {
            if state.isLoading && state.projectInfo == nil {
                // A cold load draws the plan's own shape rather than a
                // spinner in an empty column, so what arrives replaces it
                // without moving anything.
                RailSkeletonView()
            } else if let message = state.errorMessage, state.projectInfo == nil {
                VStack(alignment: .leading, spacing: 10) {
                    Label("The plan could not be loaded", systemImage: "exclamationmark.triangle")
                        .font(.system(size: Typo.body, weight: .semibold))
                        .ink(.warning)
                    Text(message)
                        .font(.system(size: Typo.caption))
                        .ink(.secondary)
                    Button("Try Again") {
                        Task { await appModel.refresh() }
                    }
                }
                .padding(12)
                .frame(maxWidth: .infinity, alignment: .leading)
                .surface(.card)
                .cornerRadius(8)
                .padding(.horizontal, 12)
            } else if let message = state.errorMessage {
                HStack(spacing: 6) {
                    Image(systemName: "exclamationmark.triangle")
                        .font(.system(size: Typo.caption))
                    Text("Refresh failed — showing the last plan")
                        .font(.system(size: Typo.caption))
                }
                .ink(.warning)
                .padding(.horizontal, RailSlot.leading)
                .padding(.bottom, 6)
                .help(message)
            }
        }

        // A plan that landed holding nothing: a project opened or created
        // from the "+" tab, whose rail would otherwise be a blank column
        // saying neither that it loaded nor what to do.
        if appModel.activePlanIsEmpty {
            VStack(alignment: .leading, spacing: 6) {
                Text(EmptyProjectNote.title)
                    .font(.system(size: Typo.body, weight: .semibold))
                    .ink(.secondary)
                Text(EmptyProjectNote.subtitle(needsWorkingDir: appModel.activeProjectNeedsWorkingDir))
                    .font(.system(size: Typo.subhead))
                    .ink(.tertiary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .padding(12)
            .frame(maxWidth: .infinity, alignment: .leading)
            .surface(.card)
            .cornerRadius(8)
            .padding(.bottom, 10)
            .padding(.horizontal, 12)
        }
    }

    // MARK: - Sharing the rail out

    /// The sections this rail draws at all: DONE only once there is finished
    /// work to list, the other two always.
    private var drawnSections: [RailSection] {
        RailSection.allCases.filter {
            $0 != .done || railModel.doneSummary != nil || !railModel.doneSessions.isEmpty
        }
    }

    /// The open ones, in the order they are stacked — what the rail's height
    /// is shared between. A collapsed section is its heading alone and takes
    /// no part in the share.
    private var openSections: [RailSection] {
        drawnSections.filter { !collapsed.contains($0) }
    }

    /// Each open section's scroll height, off the shared rule. What there
    /// is to share is `RailSectionLayout.available` on the rail as its
    /// container offers it, less every drawn section's own chrome.
    private var sectionHeights: [RailSection: CGFloat] {
        let open = openSections
        let chrome = drawnSections.reduce(CGFloat.zero) { $0 + (chromeHeights[$1] ?? 0) }
        let shares = RailSectionLayout.heights(
            open: open.map { Double(contentHeights[$0] ?? 0) },
            available: RailSectionLayout.available(
                rail: Double(railHeight), chrome: Double(chrome)
            )
        )
        return Dictionary(uniqueKeysWithValues: zip(open, shares.map { CGFloat($0) }))
    }

    /// Whether a section has more to show than the share it was given — the
    /// same rule its height came from, so a section and its scrolling cannot
    /// disagree.
    private func scrolls(_ section: RailSection, within height: CGFloat) -> Bool {
        RailSectionLayout.scrolls(
            content: Double(contentHeights[section] ?? 0),
            height: Double(height)
        )
    }

    /// Folding a section away, or bringing it back.
    private func toggle(_ section: RailSection) {
        withAnimation(Motion.stateChange) {
            if collapsed.contains(section) {
                collapsed.remove(section)
            } else {
                collapsed.insert(section)
            }
        }
    }

    // MARK: - Section chrome

    /// The line between two sections. It belongs to the section under it —
    /// measured with that section's heading as chrome — and never scrolls:
    /// a separator that moved with what it separates is not one.
    private var sectionRule: some View {
        Rule()
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
    }

    /// Every section's heading, and the one control all three of them are:
    /// the section's own icon in the shared slot, so the labels keep the
    /// rail's one left edge; its title; whatever it counts; and the fold
    /// chevron at the trailing edge, pointing down while the section is open
    /// and right while it is away. Clicking anywhere along it folds the
    /// section to this row alone.
    ///
    /// TODO alone also carries the plan's own launch controls — New Slice and
    /// Workshop the Plan, the same two buttons the window's masthead drew
    /// before they moved here — seated beside its fold chevron rather than
    /// floating over the whole window, since both are actions on the plan
    /// this section is the queue of.
    private func sectionHeading(_ section: RailSection, trailing: String? = nil, showTodoActions: Bool = false, showNewSessionAction: Bool = false) -> some View {
        let open = !collapsed.contains(section)
        return HStack(spacing: RailSlot.spacing) {
            Image(systemName: section.icon)
                .font(.system(size: Typo.caption, weight: .semibold))
                .frame(width: RailSlot.slot)
                .ink(.tertiary)

            Text(section.title)
                .font(.system(size: Typo.caption, weight: .semibold))
                .ink(.tertiary)

            Spacer()

            if let trailing {
                Text(trailing)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .monospacedDigit()
                    .ink(.tertiary)
            }

            if showTodoActions {
                todoHeaderActions
            }

            if showNewSessionAction {
                newSessionButton
            }

            Image(systemName: open ? "chevron.down" : "chevron.right")
                .font(.system(size: 11, weight: .bold))
                .frame(width: RailSlot.slot, alignment: .trailing)
                .ink(.tertiary)
        }
        .padding(.leading, RailSlot.leading)
        .padding(.trailing, RailSlot.trailing)
        // A row's height rather than the bare text line, so the hover wash
        // has the same inset every other row's has instead of hugging the
        // heading's own letters.
        .frame(height: RailSlot.rowHeight)
        .railHoverWash()
        .contentShape(Rectangle())
        .onTapGesture { toggle(section) }
        .accessibilityAddTraits(.isButton)
        .accessibilityLabel("\(section.title), \(open ? "expanded" : "collapsed")")
    }

    /// TODO's own two buttons, moved here from the window's masthead: New
    /// Slice opens the same sheet a folder's own context menu does, unpinned
    /// to any one milestone, and Workshop the Plan opens the planning agent —
    /// unchanged actions, only where they are drawn.
    private var todoHeaderActions: some View {
        let canNewSlice = appModel.activeProjectID != nil
        let canWorkshop = appModel.projectStore != nil
        let enabled = canNewSlice || canWorkshop
        return Menu {
            Button("New Slice\u{2026}") { showTodoNewSliceSheet = true }
                .disabled(!canNewSlice)
            Button("Workshop the Plan") { appModel.openWorkshop() }
                .disabled(!canWorkshop)
        } label: {
            Image(systemName: "plus")
                .font(.system(size: 12, weight: .medium))
                .ink(.tertiary)
                .frame(width: 22, height: 22)
                .contentShape(Rectangle())
        }
        .menuStyle(.borderlessButton)
        .menuIndicator(.hidden)
        .fixedSize()
        .disabled(!enabled)
        .opacity(enabled ? 1 : 0.5)
        .hoverWash(cornerRadius: 5, enabled: enabled)
        .help("Add to the plan")
        .accessibilityLabel("Add to the plan")
    }

    /// ACTIVE's own control, beside its fold chevron like TODO's own two:
    /// starts a bare Claude Code with `nat session-launch`. On a project tab
    /// it goes straight away, the project's own working directory being where
    /// it runs; on the scratch tab, which has no such directory, it first asks
    /// for a folder. Busy while the launch is in flight; a failure surfaces
    /// through the rail's own toast.
    private var newSessionButton: some View {
        Button(action: { Task { await startNewSession() } }) {
            Group {
                if appModel.newSessionLaunching {
                    ProgressView()
                        .controlSize(.mini)
                        .frame(width: 12, height: 12)
                } else {
                    Image(systemName: "plus")
                        .font(.system(size: 12, weight: .medium))
                }
            }
            .ink(.tertiary)
            .frame(width: 22, height: 22)
        }
        .buttonStyle(.plain)
        .disabled(appModel.newSessionLaunching || appModel.activeProjectID == nil)
        .hoverWash(cornerRadius: 5, enabled: !appModel.newSessionLaunching)
        .help(appModel.newSessionNeedsFolder
            ? "Start an ad hoc session in a folder you choose"
            : "Start an ad hoc session in this project's working directory")
        .accessibilityLabel("New session")
    }

    private func startNewSession() async {
        guard appModel.newSessionNeedsFolder else {
            await appModel.launchSession()
            if let error = appModel.newSessionError {
                actionError = error
            }
            return
        }
        // Nothing chosen is nothing launched: cancelling the panel is not an error.
        guard let folder = chooseSessionFolder() else { return }
        await appModel.launchSession(dir: folder)
        if let error = appModel.newSessionError {
            actionError = error
        }
    }

    /// The standard open panel for the scratch tab's New Session, opened on the
    /// folder the last such session used. Nil when the user cancels.
    private func chooseSessionFolder() -> String? {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        panel.canCreateDirectories = true
        panel.prompt = "Start Session"
        panel.message = "Choose the folder the session runs in"
        if let last = appModel.lastSessionFolder {
            panel.directoryURL = URL(fileURLWithPath: last)
        }
        return panel.runModal() == .OK ? panel.url?.path : nil
    }

    /// What the ad hoc session's row is about, for the context menu's own
    /// actions — nil for a session `sessionStore` has not (or no longer)
    /// got a reading for.
    private func session(for entry: ActiveEntry) -> Session? {
        appModel.sessionStore?.sessions.first { $0.id == entry.sliceID }
    }

    /// The session row's own context menu: End session kills its agent
    /// outright, and Discard (confirmed first, since it removes the
    /// worktree) ends it regardless of what is still open. Every other
    /// ACTIVE/DONE row offers none — an empty menu is what a right-click on
    /// one of those already draws.
    @ViewBuilder
    private func sessionMenuItems(for entry: ActiveEntry) -> some View {
        if entry.kind == .session, let session = session(for: entry) {
            Button("End Session") {
                Task { await appModel.endSession(tag: session.tag) }
            }
            Button("Discard\u{2026}", role: .destructive) {
                sessionForDiscard = entry
            }
        }
    }

    /// What the ACTIVE section draws with nothing to list: a recessed well,
    /// full-bleed across the rail and exactly one active entry tall, so the
    /// section, the rule under it and the whole plan below hold still as the
    /// first agent starts and the last one finishes.
    ///
    /// It reads as an empty socket rather than as a line of prose where a
    /// row should be: the field ground, which is the darkest surface the
    /// palette has and so sits below the rail's own; an inner shadow along
    /// the top edge, the light the recess is cut out of; and a hairline on
    /// the top and bottom edges alone — no sides and no corners, since the
    /// band spans the rail and a rounded card floating in it would read as
    /// something to click. Centred in it, the glyph and the note.
    ///
    /// The height is the height of an entry rather than a number typed out
    /// beside one: the two lines `sessionRow` builds, spaced and padded as
    /// it spaces and pads them, drawn as nothing at all. The edges are
    /// overlaid rather than stacked, so the hairlines cost the band no
    /// height of their own.
    private var activeEmptyNote: some View {
        ZStack {
            VStack(alignment: .leading, spacing: 1) {
                Text(" ")
                    .font(.system(size: Typo.body, weight: .regular))
                    .hidden()

                Text(" ")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .hidden()
            }
            .padding(.vertical, 8)

            HStack(spacing: 7) {
                Image(systemName: "moon.zzz")
                    .font(.system(size: 13, weight: .regular))

                Text(EmptyActiveNote.text)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .lineLimit(1)
            }
            .ink(.tertiary)
        }
        .frame(maxWidth: .infinity)
        .background(emptyWell)
        .environment(\.ground, .field)
    }

    /// The socket itself: the field ground with the light caught along its
    /// top edge, and the hairline the band meets the rail on at either end.
    private var emptyWell: some View {
        Rectangle()
            .fill(
                DesignTokens.fill(.field)
                    .shadow(.inner(color: .black.opacity(0.3), radius: 1.5, x: 0, y: 0.5))
            )
            .overlay(alignment: .top) { Rule(.hairline) }
            .overlay(alignment: .bottom) { Rule(.hairline) }
    }

    // MARK: - Session rows

    /// A session row's two lines beside its dot: the title with its
    /// right-aligned meta, and a tertiary detail line underneath.
    private func sessionRow(
        selected: Bool,
        dotColor: InkRole,
        pulsing: Bool,
        name: String,
        meta: String?,
        metaColor: InkRole,
        detail: [(String, InkRole)],
        @ViewBuilder trailing: () -> some View = { EmptyView() }
    ) -> some View {
        HStack(alignment: .top, spacing: RailSlot.spacing) {
            dotView(color: dotColor, pulsing: pulsing && !selected)
                .frame(width: RailSlot.slot, height: 19)

            VStack(alignment: .leading, spacing: 1) {
                Text(name)
                    .font(.system(size: Typo.body, weight: .regular))
                    .ink(.primary)
                    .lineLimit(1)

                HStack(spacing: 5) {
                    ForEach(Array(detail.enumerated()), id: \.offset) { index, piece in
                        if index > 0 {
                            Text("·")
                                .ink(.tertiary)
                        }
                        Text(piece.0)
                            .ink(piece.1)
                            .lineLimit(1)
                    }
                }
                .font(.system(size: Typo.subhead, weight: .regular))
            }

            Spacer(minLength: 0)

            // The meta and the trailing slot centre on the row as a whole —
            // on a two-line row a first-baseline meta and a top-slot ✕ both
            // ride high. Expanding to the HStack's height (set by the text
            // column) and centring inside it leaves dot, title and detail
            // exactly where they were.
            HStack(spacing: RailSlot.spacing) {
                if let meta {
                    Text(meta)
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .monospacedDigit()
                        .ink(metaColor)
                }

                trailing()
            }
            .frame(maxHeight: .infinity)
        }
        .padding(.vertical, 8)
        .padding(.leading, RailSlot.leading)
        .padding(.trailing, RailSlot.trailing)
        .frame(maxWidth: .infinity, alignment: .leading)
        // A wash rather than a solid accent slab: the old fill recolored
        // every piece of the row to accentText, which flattened the dot and
        // state tints right when they were most worth showing. The wash
        // leaves them all their own colour and shows only the title in
        // full-strength label, so a selected row still reads its own state
        // at a glance. Full-bleed and square, exactly the rectangle the
        // hover wash lights: selection and hover are the same row being
        // pointed at, and two shapes for it read as two different rows.
        .background(
            selected ? DesignTokens.wash(.selection, tone: .accent, on: ground) : Color.clear
        )
        .railHoverWash()
    }

    /// Every entry of the one flight section, whatever it stands for: the
    /// model has already resolved the status word, the tint and the rest of
    /// the second line, so a workshop entry, a branch awaiting review and a
    /// slice with an agent on it are all drawn by this.
    private func activeRow(for entry: ActiveEntry, isHovered: Bool) -> some View {
        let tint = tintColor(for: entry.tintRole)
        // Only a working row pulses — the rule itself is the model's, so the
        // rail and the project tab cannot drift apart on it.
        let isLive = entry.tintRole.pulses

        let detail: [(String, InkRole)] = [(entry.displayState, tint)]
            + entry.detail.map { ($0, InkRole.tertiary) }

        return sessionRow(
            selected: isSelected(entry),
            dotColor: tint,
            pulsing: isLive,
            name: entry.name,
            meta: entry.meta,
            metaColor: metaColor(for: entry.metaRole),
            detail: detail
        ) {
            if entry.kind == .workshop {
                workshopCloseButton(isSelected: isSelected(entry), isHovered: isHovered)
            }
        }
    }

    /// The workshop row's own ✕ — no other ACTIVE entry ever carries one.
    /// Styled after the project tab's (`ProjectTabsView`), visible under the
    /// same rule (`ProjectTabRules.closeIsVisible`): selected or hovered, and
    /// hidden-but-present otherwise so the row's layout never shifts as the
    /// mouse crosses it.
    private func workshopCloseButton(isSelected: Bool, isHovered: Bool) -> some View {
        let visible = ProjectTabRules.closeIsVisible(isActive: isSelected, isHovered: isHovered)
        return Button(action: requestCloseWorkshop) {
            Image(systemName: "xmark")
                .font(.system(size: 9, weight: .semibold))
                .ink(.tertiary)
                .frame(width: 16, height: 16)
        }
        .buttonStyle(.plain)
        .hoverWash(cornerRadius: 4)
        .opacity(visible ? 1 : 0)
        .allowsHitTesting(visible)
        .help("Close Workshop")
    }

    /// What the ✕ does: with no planning agent live, close outright; with
    /// one, ask first — `confirmingWorkshopClose`'s alert is what actually
    /// calls `closeWorkshop()`.
    private func requestCloseWorkshop() {
        if appModel.planningAgent != nil {
            confirmingWorkshopClose = true
        } else {
            Task { await closeWorkshop() }
        }
    }

    /// Ends the workshop tab — `AppModel.closeWorkshopTab()` — surfacing a
    /// refusal the same way every other rail action does.
    private func closeWorkshop() async {
        if let refusal = await appModel.closeWorkshopTab() {
            actionError = refusal
        }
    }

    /// What an entry selects when it is tapped: the workshop pane for the
    /// planning agent's entry, the slice for a slice row, and the session
    /// for an ad hoc session row — `sliceID` doubles as the session's own ID
    /// there, same as everywhere else `ActiveEntry` reads it.
    private func select(_ entry: ActiveEntry) {
        switch entry.kind {
        case .workshop: appModel.workshopSelected = true
        case .slice: appModel.selectedSliceID = entry.sliceID
        case .session: appModel.selectedSessionID = entry.sliceID
        }
    }

    private func isSelected(_ entry: ActiveEntry) -> Bool {
        switch entry.kind {
        case .workshop: return appModel.workshopSelected
        case .slice: return appModel.selectedSliceID == entry.sliceID
        case .session: return appModel.selectedSessionID == entry.sliceID
        }
    }

    /// An elapsed time recedes; a diff tally is drawn in the review's own
    /// green, the colour the rail has always drawn a tally in.
    private func metaColor(for role: ActiveMetaRole) -> InkRole {
        switch role {
        case .elapsed: return .tertiary
        case .stat: return .success
        }
    }

    @ViewBuilder
    private func dotView(color: InkRole, pulsing: Bool) -> some View {
        if pulsing {
            Circle()
                .fill(DesignTokens.ink(color, on: ground))
                .frame(width: 9, height: 9)
                .modifier(PulseModifier())
        } else {
            Circle()
                .fill(DesignTokens.ink(color, on: ground))
                .frame(width: 9, height: 9)
        }
    }

    private func tintColor(for role: ActiveTintRole) -> InkRole {
        switch role {
        // The accent rather than an outcome colour: working is not an
        // outcome, and at dot scale the orange it used to take was too near
        // the waiting yellow to tell from it. The project tab's dot reads
        // the same way — one vocabulary.
        case .working: return .accent
        case .waiting: return .warning
        case .blocked: return .tertiary
        // The same green the review affordance already uses, for the two
        // states that are about work that is out.
        case .readyToPush: return .success
        case .needsReview: return .success
        case .launching, .new, .done: return .tertiary
        }
    }

    // MARK: - Tree rows

    /// A milestone folder and, when expanded, its slice files — the DONE
    /// section's sit one level deeper throughout.
    @ViewBuilder
    private func folderRows(
        _ folder: MilestoneFolder,
        inDone: Bool,
        expanded: Bool,
        onToggle: @escaping () -> Void
    ) -> some View {
        folderRow(folder, inDone: inDone, expanded: expanded, onToggle: onToggle)
            .contextMenu {
                folderMenu(for: folder)
            }

        if expanded {
            VStack(spacing: 0) {
                ForEach(folder.slices, id: \.sliceID) { slice in
                    sliceRow(slice, depth: inDone ? 2 : 1)
                        .contentShape(Rectangle())
                        .onTapGesture {
                            appModel.selectedSliceID = slice.sliceID
                        }
                        .contextMenu {
                            sliceMenu(for: slice, under: folder.milestoneID)
                        }
                }
            }
            // The gutter guide: a hairline dropping from under the folder's
            // icon to the foot of its slices, on the icon slot's own centre
            // line. It backs the whole group, so a selected row's fill —
            // drawn by the row itself — paints over its stretch of it.
            .background(alignment: .leading) {
                Rectangle()
                    .fill(DesignTokens.rule(.border, on: .window))
                    .frame(width: 1)
                    .padding(.leading, guideInset(inDone: inDone))
            }
        }
    }

    /// Where a folder's guide line sits: the horizontal centre of its icon
    /// slot, at the folder's own indent.
    private func guideInset(inDone: Bool) -> CGFloat {
        RailSlot.leading + (inDone ? RailSlot.indent : 0) + (RailSlot.slot - 1) / 2
    }

    /// `FolderGlyphShape` drawn open (filled) or closed (outline) — a `Shape`
    /// hands back a different concrete view for `.fill` and `.stroke`, so
    /// this is the one place that picks between them rather than every call
    /// site repeating the branch.
    @ViewBuilder
    private func folderGlyph(open: Bool, color: Color) -> some View {
        if open {
            FolderGlyphShape(open: true).fill(color)
        } else {
            FolderGlyphShape(open: false).stroke(color, lineWidth: FolderGlyphShape.strokeWidth)
        }
    }

    private func folderRow(
        _ folder: MilestoneFolder,
        inDone: Bool,
        expanded: Bool,
        onToggle: @escaping () -> Void
    ) -> some View {
        HStack(spacing: RailSlot.spacing) {
            // The folder is the fold's own indicator — open with its flap
            // swung out while expanded, closed otherwise — sitting in the
            // slot the chevron held, so the tree keeps its one icon axis.
            // Drawn rather than an SF Symbol, since the system set has no
            // open-folder glyph. Closed draws as an outline and open as the
            // filled silhouette: the fill is what says a folder's contents
            // are out on the tree already, and an outline is the neutral
            // treatment for the ones still holding theirs back.
            folderGlyph(
                open: expanded,
                color: DesignTokens.ink(
                    inDone ? .tertiary : folder.isCurrent ? .accent : .secondary,
                    on: ground
                )
            )
            .frame(width: RailSlot.slot, height: Self.folderGlyphHeight)

            Text(folder.title)
                .font(.system(size: Typo.body, weight: folder.isCurrent ? .semibold : .regular))
                .ink(inDone ? .secondary : .primary)
                .lineLimit(1)

            Spacer()

            if inDone && folder.isComplete {
                Image(systemName: "checkmark")
                    .font(.system(size: 9, weight: .bold))
                    .ink(.success)
            }

            Text("\(folder.done)/\(folder.total)")
                .font(.system(size: Typo.subhead, weight: .regular))
                .monospacedDigit()
                .ink(.tertiary)
        }
        .frame(height: RailSlot.rowHeight)
        .padding(.leading, RailSlot.leading + (inDone ? RailSlot.indent : 0))
        .padding(.trailing, RailSlot.trailing)
        .railHoverWash()
        .contentShape(Rectangle())
        .onTapGesture(perform: onToggle)
    }

    private func sliceRow(_ slice: MilestoneSliceRow, depth: Int) -> some View {
        let selected = appModel.selectedSliceID == slice.sliceID
        // Selection no longer overrides the title to accentText — a selected
        // row keeps full-strength label the way an unblocked one always did,
        // rather than the solid fill's own recolor.
        let contentColor: InkRole = selected
            ? .primary
            : (slice.isBlocked ? .tertiary : .primary)

        return HStack(spacing: RailSlot.spacing) {
            Image(systemName: slice.glyph.rawValue)
                .font(.system(size: Self.sliceGlyphSize, weight: .medium))
                .frame(width: RailSlot.slot)
                .ink(glyphColor(for: slice.glyph))

            Text(slice.name)
                .font(.system(size: Typo.body, weight: .regular))
                .lineLimit(1)
                .ink(contentColor)

            Spacer()
        }
        .frame(height: RailSlot.rowHeight)
        .padding(.leading, RailSlot.leading + CGFloat(depth) * RailSlot.indent)
        .padding(.trailing, RailSlot.trailing)
        // The same full-bleed wash as sessionRow, and for the same reasons:
        // the glyph keeps its own status tint under selection instead of
        // being flattened to accentText by a solid fill, and the shape is
        // the hover wash's own rectangle.
        .background(
            selected ? DesignTokens.wash(.selection, tone: .accent, on: ground) : Color.clear
        )
        .railHoverWash()
    }

    /// The mock's status tints for a slice glyph — in progress orange, done
    /// green, and the rest (todo, blocked) muted; these now show through a
    /// selected row rather than being recolored by it.
    private func glyphColor(for glyph: SliceGlyph) -> InkRole {
        switch glyph {
        case .todo, .blocked: return .tertiary
        case .inProgress: return .warning
        case .done: return .success
        }
    }

    // MARK: - Slice actions

    /// The right-click menu on a tree slice row: launch an agent on it, edit
    /// its brief, open its page in Notion, refile it under another
    /// milestone, or delete it behind a confirm. Only tree rows carry it —
    /// a slice drawn in a session section is work in flight, which `nat`
    /// refuses to move or delete anyway.
    ///
    /// Every item is enabled under exactly the condition the control it
    /// mirrors is: the launch under `LaunchPlan`, which is the Brief tab's
    /// own answer, and the edit under `slice-edit`'s own rule that a slice
    /// being worked is not edited under its agent. The menu is a second door
    /// to behaviour that is already there, never a rule of its own.
    @ViewBuilder
    private func sliceMenu(for slice: MilestoneSliceRow, under milestoneID: String) -> some View {
        let targets = (appModel.projectStore?.state.projectInfo?.milestones ?? [])
            .sorted { $0.order < $1.order }
            .filter { $0.id != milestoneID }
        let page = planSlice(slice.sliceID)

        Button("Launch Agent") {
            Task { await launchAgent(on: slice) }
        }
        .disabled(!canLaunch(slice))

        Button("Edit Description\u{2026}") {
            sliceForEdit = slice
        }
        .disabled(page?.status != "Todo")

        if let url = page.flatMap({ URL(string: $0.url) }) ?? NotionPageURL.forPage(slice.sliceID) {
            Button("Open in Notion") {
                NSWorkspace.shared.open(url)
            }
        }

        Divider()

        if !targets.isEmpty {
            Menu("Move to") {
                ForEach(targets) { milestone in
                    Button(milestone.name) {
                        Task { await moveSlice(slice, to: milestone.name) }
                    }
                }
            }
        }
        Button("Delete\u{2026}", role: .destructive) {
            sliceForDeletion = slice
        }
    }

    /// The right-click menu on a milestone folder header: file a slice under
    /// it, rename it, move it in the plan, or drop it.
    ///
    /// The move and the delete are `MilestoneMenuRules`' answers — the CLI's
    /// own rules, asked here so a greyed item is a refusal the user never has
    /// to read: the first milestone has nothing to move up past, the last
    /// nothing to move down past, and only an empty one can be deleted, since
    /// `milestone-remove` refuses one still holding slices.
    @ViewBuilder
    private func folderMenu(for folder: MilestoneFolder) -> some View {
        let actions = MilestoneMenuRules.actions(
            for: folder.milestoneID, in: planMilestones, sliceCount: filedSliceCount(folder.milestoneID))

        Button("New Slice\u{2026}") {
            milestoneForNewSlice = folder.milestoneID
        }
        .disabled(appModel.activeProjectID == nil)

        Button("Rename\u{2026}") {
            renameText = folder.milestoneID
            milestoneForRename = folder.milestoneID
        }

        Divider()

        Button("Move Up") {
            Task { await moveMilestone(folder, before: actions.moveBefore, after: nil) }
        }
        .disabled(actions.moveBefore == nil)

        Button("Move Down") {
            Task { await moveMilestone(folder, before: nil, after: actions.moveAfter) }
        }
        .disabled(actions.moveAfter == nil)

        Divider()

        Button("Delete", role: .destructive) {
            milestoneForDeletion = folder
        }
        .disabled(!actions.canDelete)
    }

    /// The plan's milestones in the order it holds them — what both menus
    /// read, since a folder row carries its own name and nothing about its
    /// neighbours.
    private var planMilestones: [Milestone] {
        appModel.projectStore?.state.projectInfo?.milestones ?? []
    }

    /// How many slices the plan files under a milestone — counted here
    /// rather than read off the folder's own `total`, which is drawn as
    /// `max(1, …)` so an empty milestone reads "0/1" rather than "0/0" and
    /// would have an empty milestone reporting a slice it does not hold.
    private func filedSliceCount(_ milestoneID: String) -> Int {
        (appModel.projectStore?.state.projectInfo?.slices ?? [])
            .filter { $0.milestoneID == milestoneID }
            .count
    }

    /// The slice as the plan holds it, for the facts a rail row does not
    /// carry: its status and the URL of its page. Nil while no plan is read,
    /// which is when every item that depends on one is drawn disabled.
    private func planSlice(_ sliceID: String) -> Slice? {
        appModel.projectStore?.state.projectInfo?.slices.first { $0.id == sliceID }
    }

    /// Whether "Launch Agent" is offered — the Brief tab's own answer, taken
    /// from the same `LaunchPlan` over the same two readings, so the menu and
    /// the pane's launch control can never disagree about one slice.
    private func canLaunch(_ slice: MilestoneSliceRow) -> Bool {
        guard let page = planSlice(slice.sliceID) else { return false }
        let hasLiveAgent = appModel.activityStore?.agents[slice.sliceID] != nil
        return LaunchPlan(for: page, hasLiveAgent: hasLiveAgent).canLaunch
    }

    /// A binding that is true while an optional holds something and nils it
    /// on dismissal — how every sheet and alert here is presented, since the
    /// thing being presented about is what says whether to present at all.
    private func presenting<Value>(_ value: Binding<Value?>) -> Binding<Bool> {
        Binding(
            get: { value.wrappedValue != nil },
            set: { if !$0 { value.wrappedValue = nil } }
        )
    }

    private func launchAgent(on slice: MilestoneSliceRow) async {
        guard let projectID = appModel.activeProjectID else { return }
        // The config's `slice_agent` pair as it stands, which is what the
        // pane's launch popover prefills itself with: a launch that asks
        // nothing takes the default rather than inventing one.
        let agent = appModel.config?.sliceAgent
        do {
            _ = try await NatClient().sliceLaunch(
                projectID: projectID,
                sliceRef: slice.sliceID,
                model: agent?.model,
                effort: agent?.effort
            )
            appModel.selectedSliceID = slice.sliceID
            await appModel.refresh()
        } catch {
            actionError = commandMessage(of: error)
        }
    }

    private func renameMilestone(_ name: String, to newName: String) async {
        guard let projectID = appModel.activeProjectID else { return }
        let trimmed = newName.trimmingCharacters(in: .whitespacesAndNewlines)
        // A name unchanged, or emptied, is the dialog being dismissed rather
        // than an edit to make — `milestone-rename` refuses both, and there
        // is nothing to report about a refusal the user did not ask for.
        guard !trimmed.isEmpty, trimmed != name else { return }
        do {
            try await NatClient().milestoneRename(projectID: projectID, from: name, to: trimmed)
            // Folds are keyed by the milestone's name, which is the one thing
            // a rename changes: carried over, so a folder open before the
            // rename is open after it.
            carryFold(from: name, to: trimmed)
            await appModel.refresh()
        } catch {
            actionError = commandMessage(of: error)
        }
    }

    private func moveMilestone(_ folder: MilestoneFolder, before: String?, after: String?) async {
        guard let projectID = appModel.activeProjectID else { return }
        do {
            try await NatClient().milestoneMove(
                projectID: projectID, name: folder.milestoneID, before: before, after: after)
            await appModel.refresh()
        } catch {
            actionError = commandMessage(of: error)
        }
    }

    private func removeMilestone(_ folder: MilestoneFolder) async {
        guard let projectID = appModel.activeProjectID else { return }
        do {
            try await NatClient().milestoneRemove(projectID: projectID, name: folder.milestoneID)
            expandedMilestones.remove(folder.milestoneID)
            expandedDoneFolders.remove(folder.milestoneID)
            await appModel.refresh()
        } catch {
            actionError = commandMessage(of: error)
        }
    }

    /// Move a milestone's folds onto its new name, for both sections — a
    /// part-done milestone is a folder in each and they fold independently.
    private func carryFold(from old: String, to new: String) {
        if expandedMilestones.remove(old) != nil {
            expandedMilestones.insert(new)
        }
        if expandedDoneFolders.remove(old) != nil {
            expandedDoneFolders.insert(new)
        }
    }

    private func moveSlice(_ slice: MilestoneSliceRow, to milestone: String) async {
        guard let projectID = appModel.activeProjectID else { return }
        do {
            try await NatClient().sliceMove(
                projectID: projectID, sliceRef: slice.sliceID, milestone: milestone)
            await appModel.refresh()
        } catch {
            actionError = commandMessage(of: error)
        }
    }

    private func deleteSlice(_ slice: MilestoneSliceRow) async {
        guard let projectID = appModel.activeProjectID else { return }
        do {
            try await NatClient().sliceDelete(projectID: projectID, sliceRef: slice.sliceID)
            // The page is gone; a selection pointing at it would leave the
            // pane trying to show a slice no read can serve.
            if appModel.selectedSliceID == slice.sliceID {
                appModel.selectedSliceID = nil
            }
            await appModel.refresh()
        } catch {
            actionError = commandMessage(of: error)
        }
    }

    private func discardSession(_ entry: ActiveEntry) async {
        if let refusal = await appModel.discardSession(id: entry.sliceID) {
            actionError = refusal
        }
    }

    /// nat's own first stderr line where there is one — "X is in progress:
    /// …" — and the generic description otherwise, the same unwrapping the
    /// new-slice sheet does.
    private func commandMessage(of error: Error) -> String {
        if case NatError.commandFailed(let message) = error {
            return message
        }
        return error.localizedDescription
    }

    private func toggle(_ id: String, in set: inout Set<String>) {
        withAnimation(Motion.stateChange) {
            if set.contains(id) {
                set.remove(id)
            } else {
                set.insert(id)
            }
        }
    }
}

/// The tree's folder pictograms, drawn because SF Symbols has no open-folder
/// glyph to pair with `folder`. Both states are one folder: the same tabbed
/// body, the same corner radius, the same bounds. Closed is that body as an
/// outline (inset by half the stroke so it lands on the same bounds the
/// fill does); open is it filled, with the body's lower part swapped for a
/// front flap swung out to the right and a hairline gap between flap and
/// back panel — the closed folder with its flap opened and the body filled.
struct FolderGlyphShape: Shape {
    let open: Bool

    /// The closed outline's line width, which the shape insets by half of.
    static let strokeWidth: CGFloat = 1.1

    /// Where the flap's top edge sits, as a fraction of the height, and the
    /// gap the back panel stops short of it by.
    private static let flapTop: CGFloat = 0.50
    private static let gap: CGFloat = 0.06

    func path(in rect: CGRect) -> Path {
        open ? openPath(in: rect) : closedPath(in: rect)
    }

    private func closedPath(in rect: CGRect) -> Path {
        let inset = Self.strokeWidth / 2
        return body(in: rect.insetBy(dx: inset, dy: inset), bottom: nil)
    }

    private func openPath(in rect: CGRect) -> Path {
        let w = rect.width
        let h = rect.height
        let r = 0.14 * h
        var p = body(in: rect, bottom: rect.minY + (Self.flapTop - Self.gap) * h)
        // The flap: a parallelogram leaning left at the foot, its top edge
        // running out to the body's right bound.
        let lean = 0.14 * w
        let top = rect.minY + Self.flapTop * h
        let pts = [
            CGPoint(x: rect.minX + lean, y: top),
            CGPoint(x: rect.maxX, y: top),
            CGPoint(x: rect.maxX - lean, y: rect.maxY),
            CGPoint(x: rect.minX, y: rect.maxY),
        ]
        p.addPath(rounded(pts, radius: r))
        return p
    }

    /// The tabbed folder body: rounded corners, the tab across the top left,
    /// a short slant joining tab to top edge. `bottom` lifts the lower edge
    /// (still rounded) for the open folder's back panel.
    private func body(in rect: CGRect, bottom: CGFloat?) -> Path {
        let h = rect.height
        let r = 0.14 * h
        let tabW = 0.36 * rect.width
        let slant = 0.10 * rect.width
        let tabH = 0.22 * h
        let x0 = rect.minX
        let y0 = rect.minY
        let right = rect.maxX
        let foot = bottom ?? rect.maxY
        // The open folder's back panel is a short strip, so its cut edge
        // takes a tighter corner than the full body's.
        let rb = bottom == nil ? r : 0.07 * h

        var p = Path()
        p.move(to: CGPoint(x: x0, y: y0 + r))
        p.addArc(center: CGPoint(x: x0 + r, y: y0 + r), radius: r,
                 startAngle: .degrees(180), endAngle: .degrees(270), clockwise: false)
        p.addLine(to: CGPoint(x: x0 + tabW, y: y0))
        p.addLine(to: CGPoint(x: x0 + tabW + slant, y: y0 + tabH))
        p.addLine(to: CGPoint(x: right - r, y: y0 + tabH))
        p.addArc(center: CGPoint(x: right - r, y: y0 + tabH + r), radius: r,
                 startAngle: .degrees(270), endAngle: .degrees(0), clockwise: false)
        p.addLine(to: CGPoint(x: right, y: foot - rb))
        p.addArc(center: CGPoint(x: right - rb, y: foot - rb), radius: rb,
                 startAngle: .degrees(0), endAngle: .degrees(90), clockwise: false)
        p.addLine(to: CGPoint(x: x0 + rb, y: foot))
        p.addArc(center: CGPoint(x: x0 + rb, y: foot - rb), radius: rb,
                 startAngle: .degrees(90), endAngle: .degrees(180), clockwise: false)
        p.closeSubpath()
        return p
    }

    /// A closed polygon with every corner rounded to `radius`.
    private func rounded(_ pts: [CGPoint], radius: CGFloat) -> Path {
        var p = Path()
        let n = pts.count
        p.move(to: CGPoint(x: (pts[0].x + pts[n - 1].x) / 2, y: (pts[0].y + pts[n - 1].y) / 2))
        for i in 0..<n {
            p.addArc(tangent1End: pts[i], tangent2End: pts[(i + 1) % n], radius: radius)
        }
        p.closeSubpath()
        return p
    }
}
