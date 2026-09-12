import SwiftUI
import NatKit
import NatFixtures

/// The rail's file-tree geometry, shared by every row so the marks line up on
/// one vertical axis: heading icons, session dots and every chevron sit in
/// the same fixed-width `slot` at the same `leading` offset, which puts every
/// title and heading label on one shared left edge; each tree level indents
/// by `indent`. Rows still span the rail's full width for their tap targets,
/// but hover and selection paint as rounded chips inset from the edges — see
/// `InsetHoverWash` — with content kept inside `leading`/`trailing`.
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

/// The shared `.hoverWash()` paints edge-to-edge (see ViewHelpers.swift), but
/// a modern sidebar wants its hover and its selection reading as the same
/// soft rounded chip, inset from the rail's edges rather than a full-bleed
/// bar. Rebuilt here rather than widened in the shared helper, since the
/// inset has to be baked into the very shape hover fills — wrapping a
/// full-bleed `.hoverWash()` in padding afterwards only pads its reported
/// size, not what it actually paints.
private struct InsetHoverWash: ViewModifier {
    var cornerRadius: CGFloat = 6
    @State private var hovering = false

    func body(content: Content) -> some View {
        content
            .background(
                RoundedRectangle(cornerRadius: cornerRadius)
                    .fill(hovering ? DesignTokens.fill(.hover) : Color.clear)
                    .padding(.horizontal, 6)
            )
            .onHover { hovering = $0 }
    }
}

extension View {
    fileprivate func insetHoverWash(cornerRadius: CGFloat = 6) -> some View {
        modifier(InsetHoverWash(cornerRadius: cornerRadius))
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
    /// Whether the DONE section is expanded to list its folders. The user's
    /// own fold, like `expandedMilestones`.
    @State private var expandedDoneSummary = false
    /// The slice the delete menu item was picked on, held while its confirm
    /// dialog is up — deleting is the one rail action that cannot be undone
    /// from here (the page goes to Notion's trash), so it asks first, the way
    /// the board's own d does.
    @State private var sliceForDeletion: MilestoneSliceRow?
    /// What the last move or delete refused with — a slice in progress, gh
    /// down, whatever nat said — shown in an alert and cleared by dismissing
    /// it. The rail has no status bar to toast on.
    @State private var actionError: String?

    var railModel: RailModel {
        if let projectInfo = appModel.projectStore?.state.projectInfo {
            // Map ActivityStore agents to the rail model's format
            var liveAgents: [String: AgentActivity] = [:]
            for (sliceID, status) in appModel.activityStore?.agents ?? [:] {
                switch status.activity {
                case .working:
                    liveAgents[sliceID] = .working
                case .waiting:
                    liveAgents[sliceID] = .waiting
                case .unknown:
                    // Treat unknown as working (TUI convention)
                    liveAgents[sliceID] = .working
                }
            }
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
        let activity: AgentActivity? = appModel.planningAgent.map {
            $0.activity == .waiting ? .waiting : .working
        }
        return buildWorkshopEntry(
            activity: activity,
            isLaunching: appModel.workshopLaunching,
            isSelected: appModel.workshopSelected,
            firstSeen: appModel.planningAgentKey.flatMap { appModel.activityStore?.firstSeen[$0] }
        )
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                // The load's own states come first: a board that swallowed
                // its failure would read as an empty tracker, which is worse
                // than any error. A first load draws the plan's own skeleton,
                // a failed first load says what nat said and offers the retry,
                // and a failed refresh keeps the stale plan under one quiet
                // warning line (the TUI convention: a failure leaves the
                // board as it was).
                if let state = appModel.projectStore?.state {
                    if state.isLoading && state.projectInfo == nil {
                        // A cold load draws the plan's own shape rather than
                        // a spinner in an empty column, so what arrives
                        // replaces it without moving anything.
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

                // A plan that landed holding nothing: a project opened or
                // created from the "+" tab, whose rail would otherwise be a
                // blank column saying neither that it loaded nor what to do.
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
                    .padding(.horizontal, 12)
                    .padding(.bottom, 10)
                }

                // ACTIVE section — always drawn, holding entries or holding
                // its own note: what is running is the rail's standing
                // question, and a heading that only appeared once something
                // was read as chrome arriving from nowhere. It is the one
                // flight section there is: the planning agent and the
                // branches waiting on a review are entries of it rather than
                // headings of their own, in that order.
                sectionHeading("ACTIVE", icon: "bolt")

                if railModel.active.isEmpty {
                    activeEmptyNote
                } else {
                    ForEach(railModel.active) { entry in
                        activeRow(for: entry)
                            .contentShape(Rectangle())
                            .onTapGesture { select(entry) }
                    }
                }

                // The rule under the flight sections, drawn whatever they
                // hold: ACTIVE is above it on every rail there is, so there
                // is always a section for it to close.
                Rule()
                    .padding(.horizontal, 12)
                    .padding(.vertical, 10)

                // TODO — the milestones still holding work, folders in a
                // file tree with their remaining slices as files.
                if !railModel.todoFolders.isEmpty {
                    sectionHeading("TODO", icon: "list.bullet")
                        // TODO always lands under the flight-sections
                        // divider now that ACTIVE is always above it, so the
                        // air it needs there is no longer conditional.
                        .padding(.top, 6)
                }

                ForEach(railModel.todoFolders, id: \.milestoneID) { folder in
                    folderRows(
                        folder,
                        inDone: false,
                        expanded: expandedMilestones.contains(folder.milestoneID),
                        onToggle: { toggle(folder.milestoneID, in: &expandedMilestones) }
                    )
                }

                // DONE — the finished slices' home, a heading at the foot of
                // the plan: every milestone with work done expands under it
                // as a folder of its own, one level deeper than the tree.
                if let summary = railModel.doneSummary {
                    Rule()
                        .padding(.horizontal, 12)
                        .padding(.top, 9)
                        .padding(.bottom, 10)

                    doneHeadingRow(summary)

                    if expandedDoneSummary {
                        ForEach(railModel.doneFolders, id: \.milestoneID) { folder in
                            folderRows(
                                folder,
                                inDone: true,
                                expanded: expandedDoneFolders.contains(folder.milestoneID),
                                onToggle: { toggle(folder.milestoneID, in: &expandedDoneFolders) }
                            )
                        }
                    }
                }
            }
            .padding(.top, 12)
            .padding(.bottom, 16)
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

    // MARK: - Section chrome

    /// An all-caps heading with its small icon in the shared slot — or, for
    /// DONE, a chevron in that slot instead, via `doneHeadingRow`.
    private func sectionHeading(_ title: String, icon: String) -> some View {
        HStack(spacing: RailSlot.spacing) {
            Image(systemName: icon)
                .font(.system(size: Typo.caption, weight: .semibold))
                .frame(width: RailSlot.slot)
                .ink(.tertiary)

            Text(title)
                .font(.system(size: Typo.caption, weight: .semibold))
                .ink(.tertiary)

            Spacer()
        }
        .padding(.leading, RailSlot.leading)
        .padding(.trailing, RailSlot.trailing)
        .padding(.bottom, 5)
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

    private func doneHeadingRow(_ summary: DoneSummary) -> some View {
        HStack(spacing: RailSlot.spacing) {
            Image(systemName: expandedDoneSummary ? "chevron.down" : "chevron.right")
                .font(.system(size: 11, weight: .bold))
                .frame(width: RailSlot.slot)
                .ink(.tertiary)

            Text("DONE")
                .font(.system(size: Typo.caption, weight: .semibold))
                .ink(.tertiary)

            Spacer()

            Text("\(summary.doneCount)/\(summary.totalCount)")
                .font(.system(size: Typo.subhead, weight: .regular))
                .monospacedDigit()
                .ink(.tertiary)
        }
        .padding(.leading, RailSlot.leading)
        .padding(.trailing, RailSlot.trailing)
        // A row's height rather than the bare text line, so the hover wash
        // has the same inset every other row's has instead of hugging the
        // heading's own letters.
        .frame(height: RailSlot.rowHeight)
        .insetHoverWash()
        .contentShape(Rectangle())
        .onTapGesture {
            withAnimation(Motion.stateChange) {
                expandedDoneSummary.toggle()
            }
        }
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
        // A soft inset chip rather than a solid accent slab: the old fill
        // recolored every piece of the row to accentText, which flattened
        // the dot and state tints right when they were most worth showing.
        // The wash leaves them all their own colour and shows only the
        // title in full-strength label, so a selected row still reads its
        // own state at a glance.
        .background {
            RoundedRectangle(cornerRadius: 6)
                .fill(selected ? DesignTokens.wash(.selection, tone: .accent, on: ground) : Color.clear)
                .padding(.horizontal, 6)
        }
        .insetHoverWash()
    }

    /// Every entry of the one flight section, whatever it stands for: the
    /// model has already resolved the status word, the tint and the rest of
    /// the second line, so a workshop entry, a branch awaiting review and a
    /// slice with an agent on it are all drawn by this.
    private func activeRow(for entry: ActiveEntry) -> some View {
        let tint = tintColor(for: entry.tintRole)
        // Only a live agent is worth pulling the eye to; a row with nothing
        // running on it (blocked, awaiting a review, launching, or simply
        // ready to push) sits still.
        let isLive = entry.tintRole == .working || entry.tintRole == .waiting

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
        case .working: return .warning
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
        .insetHoverWash()
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
        // Same soft inset chip as sessionRow, and for the same reason: the
        // glyph keeps its own status tint under selection now instead of
        // being flattened to accentText by a solid fill.
        .background {
            RoundedRectangle(cornerRadius: 6)
                .fill(selected ? DesignTokens.wash(.selection, tone: .accent, on: ground) : Color.clear)
                .padding(.horizontal, 6)
        }
        .insetHoverWash()
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

    /// The right-click menu on a tree slice row: refile it under another
    /// milestone, or delete it behind a confirm. Only tree rows carry it —
    /// a slice drawn in a session section is work in flight, which `nat`
    /// refuses to move or delete anyway.
    @ViewBuilder
    private func sliceMenu(for slice: MilestoneSliceRow, under milestoneID: String) -> some View {
        let targets = (appModel.projectStore?.state.projectInfo?.milestones ?? [])
            .sorted { $0.order < $1.order }
            .filter { $0.id != milestoneID }

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
