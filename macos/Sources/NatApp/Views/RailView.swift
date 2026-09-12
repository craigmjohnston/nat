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
    /// The three numbers `RailSectionLayout` shares the rail out on: how
    /// tall the rail is, how much of it each section's own chrome — its rule
    /// and its pinned heading — has already taken, and how tall each
    /// section's list comes to. Measured with `onGeometryChange` rather than
    /// assumed, since all three move: the rail is a resizable column, a
    /// section's heading comes and goes with the section, and the lists grow
    /// an entry at a time as agents start and slices land.
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
                workshop: workshopEntry
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
        railColumn
        // What the open sections divide between them. Measured rather than
        // assumed: the rail is a resizable column in a resizable window, so
        // how much there is to share changes under the user's hands.
        .onGeometryChange(for: CGFloat.self) { proxy in
            proxy.size.height
        } action: { height in
            railHeight = height
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
        // untouched ones closed, the way the mock draws Wishlist; everything
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
            if let summary = railModel.doneSummary {
                doneSection(summary, height: heights[.done])
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
            sectionHeading(.active)
                .padding(.top, 12)
                .measuringHeight { chromeHeights[.active] = $0 }

            if let height {
                ScrollView {
                    VStack(alignment: .leading, spacing: 0) {
                        if railModel.active.isEmpty {
                            activeEmptyNote
                        } else {
                            ForEach(railModel.active) { entry in
                                activeRow(for: entry)
                                    .contentShape(Rectangle())
                                    .onTapGesture { select(entry) }
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
                sectionHeading(.todo)
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
    private func doneSection(_ summary: DoneSummary, height: CGFloat?) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            VStack(alignment: .leading, spacing: 0) {
                sectionRule
                sectionHeading(.done, trailing: "\(summary.doneCount)/\(summary.totalCount)")
            }
            .measuringHeight { chromeHeights[.done] = $0 }

            if let height {
                ScrollView {
                    VStack(alignment: .leading, spacing: 0) {
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
        RailSection.allCases.filter { $0 != .done || railModel.doneSummary != nil }
    }

    /// The open ones, in the order they are stacked — what the rail's height
    /// is shared between. A collapsed section is its heading alone and takes
    /// no part in the share.
    private var openSections: [RailSection] {
        drawnSections.filter { !collapsed.contains($0) }
    }

    /// Each open section's scroll height, off the shared rule. What is left
    /// to share is the rail less every drawn section's own chrome — the rule
    /// and the pinned heading, which never scroll and never yield — and less
    /// the air under the last of them.
    private var sectionHeights: [RailSection: CGFloat] {
        let open = openSections
        let chrome = drawnSections.reduce(CGFloat.zero) { $0 + (chromeHeights[$1] ?? 0) }
        let shares = RailSectionLayout.heights(
            open: open.map { Double(contentHeights[$0] ?? 0) },
            available: Double(railHeight - chrome) - RailSectionLayout.footRoom
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
    private func sectionHeading(_ section: RailSection, trailing: String? = nil) -> some View {
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

    /// What the ACTIVE section draws with nothing to list: the note indented
    /// to the column an entry's own name starts in, so the section reads as
    /// one whose rows are missing rather than one drawn to another rule.
    ///
    /// It reserves the height of a two-line entry rather than the single
    /// line the mock draws — the departure the design README records — so
    /// the section, the divider under it and the whole plan below hold still
    /// as the first agent starts and the last one finishes. Both lines are
    /// reserved the way `sessionRow` builds them: a title line at the body
    /// size the note itself is not set in, and a detail line under it.
    private var activeEmptyNote: some View {
        VStack(alignment: .leading, spacing: 1) {
            ZStack(alignment: .leading) {
                Text(" ")
                    .font(.system(size: Typo.body, weight: .regular))
                    .hidden()

                Text(EmptyActiveNote.text)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.tertiary)
                    .lineLimit(1)
            }

            Text(" ")
                .font(.system(size: Typo.subhead, weight: .regular))
                .hidden()
        }
        .padding(.vertical, 8)
        // The entry text column: the row's own leading inset, plus the slot
        // every dot and icon sits in and the gap after it.
        .padding(.leading, RailSlot.leading + RailSlot.slot + RailSlot.spacing)
        .padding(.trailing, RailSlot.trailing)
        .frame(maxWidth: .infinity, alignment: .leading)
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
        detail: [(String, InkRole)]
    ) -> some View {
        HStack(alignment: .top, spacing: RailSlot.spacing) {
            dotView(color: dotColor, pulsing: pulsing && !selected)
                .frame(width: RailSlot.slot, height: 19)

            VStack(alignment: .leading, spacing: 1) {
                HStack(alignment: .firstTextBaseline, spacing: RailSlot.spacing) {
                    Text(name)
                        .font(.system(size: Typo.body, weight: .regular))
                        .ink(.primary)
                        .lineLimit(1)

                    Spacer(minLength: 0)

                    if let meta {
                        Text(meta)
                            .font(.system(size: Typo.subhead, weight: .regular))
                            .monospacedDigit()
                            .ink(metaColor)
                    }
                }

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
    private func activeRow(for entry: ActiveEntry) -> some View {
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
        )
    }

    /// What an entry selects when it is tapped: the workshop pane for the
    /// planning agent's entry, and the slice for every other.
    private func select(_ entry: ActiveEntry) {
        switch entry.kind {
        case .workshop: appModel.workshopSelected = true
        case .slice: appModel.selectedSliceID = entry.sliceID
        }
    }

    private func isSelected(_ entry: ActiveEntry) -> Bool {
        switch entry.kind {
        case .workshop: return appModel.workshopSelected
        case .slice: return appModel.selectedSliceID == entry.sliceID
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
        case .launching, .new: return .tertiary
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
            // open-folder glyph.
            FolderGlyphShape(open: expanded)
                .fill(DesignTokens.ink(
                    inDone ? .tertiary : folder.isCurrent ? .accent : .secondary,
                    on: ground
                ))
                .frame(width: RailSlot.slot, height: 10.5)

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
                .font(.system(size: 12, weight: .medium))
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
/// glyph to pair with `folder`. Closed is the familiar tabbed body; open is
/// the same body with its front flap swung out — the flap's top edge
/// overhanging the body's right side and its foot leaning left, which is
/// what reads as "open" at 13 points. Filled silhouettes, so they take a
/// row's colour the way the SF folder did.
struct FolderGlyphShape: Shape {
    let open: Bool

    func path(in rect: CGRect) -> Path {
        open ? openPath(in: rect) : closedPath(in: rect)
    }

    private func closedPath(in rect: CGRect) -> Path {
        body(in: rect, rightEdge: rect.width)
    }

    private func openPath(in rect: CGRect) -> Path {
        let w = rect.width
        let h = rect.height
        // The body stops short of the rect so the flap's wing can overhang.
        var p = body(in: rect, rightEdge: 0.78 * w)
        p.move(to: CGPoint(x: rect.minX + 0.20 * w, y: rect.minY + 0.44 * h))
        p.addLine(to: CGPoint(x: rect.minX + w, y: rect.minY + 0.44 * h))
        p.addLine(to: CGPoint(x: rect.minX + 0.80 * w, y: rect.minY + h))
        p.addLine(to: CGPoint(x: rect.minX + 0.06 * w, y: rect.minY + h))
        p.closeSubpath()
        return p
    }

    /// The tabbed folder body: rounded corners, the tab across the top left,
    /// a short slant joining tab to top edge.
    private func body(in rect: CGRect, rightEdge: CGFloat) -> Path {
        let h = rect.height
        let r = 0.14 * h
        let tabW = 0.36 * rect.width
        let slant = 0.10 * rect.width
        let tabH = 0.22 * h
        let x0 = rect.minX
        let y0 = rect.minY
        let right = x0 + rightEdge

        var p = Path()
        p.move(to: CGPoint(x: x0, y: y0 + r))
        p.addArc(center: CGPoint(x: x0 + r, y: y0 + r), radius: r,
                 startAngle: .degrees(180), endAngle: .degrees(270), clockwise: false)
        p.addLine(to: CGPoint(x: x0 + tabW, y: y0))
        p.addLine(to: CGPoint(x: x0 + tabW + slant, y: y0 + tabH))
        p.addLine(to: CGPoint(x: right - r, y: y0 + tabH))
        p.addArc(center: CGPoint(x: right - r, y: y0 + tabH + r), radius: r,
                 startAngle: .degrees(270), endAngle: .degrees(0), clockwise: false)
        p.addLine(to: CGPoint(x: right, y: y0 + h - r))
        p.addArc(center: CGPoint(x: right - r, y: y0 + h - r), radius: r,
                 startAngle: .degrees(0), endAngle: .degrees(90), clockwise: false)
        p.addLine(to: CGPoint(x: x0 + r, y: y0 + h))
        p.addArc(center: CGPoint(x: x0 + r, y: y0 + h - r), radius: r,
                 startAngle: .degrees(90), endAngle: .degrees(180), clockwise: false)
        p.closeSubpath()
        return p
    }
}

#Preview("Loaded plan") {
    @Previewable @State var appModel = Fixtures.appModel()
    WindowShellView(appModel: appModel)
        .frame(width: 1360, height: 840)
        .task { await Fixtures.start(appModel) }
}

#Preview("Empty plan") {
    @Previewable @State var appModel = Fixtures.appModel(
        client: FixtureNatClient(plan: Fixtures.emptyProjectInfo, agents: [])
    )
    WindowShellView(appModel: appModel)
        .frame(width: 1360, height: 840)
        .task { await Fixtures.start(appModel) }
}

#Preview("Failed read") {
    @Previewable @State var appModel = Fixtures.appModel(
        client: FixtureNatClient(behaviour: .refusing(Fixtures.loadErrorMessage))
    )
    WindowShellView(appModel: appModel)
        .frame(width: 1360, height: 840)
        .task { await Fixtures.start(appModel) }
}
