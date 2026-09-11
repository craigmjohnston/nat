import SwiftUI
import NatKit

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
                    .fill(hovering ? DesignTokens.hoverWash : Color.clear)
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
                agentStarts: appModel.activityStore?.firstSeen ?? [:]
            )
        }
        return RailModel(needsReview: [], active: [], todoFolders: [])
    }

    /// The WORKSHOP section's one row — nil while no planning agent is live,
    /// none is launching and the composer is not open, which is when the
    /// section is not drawn at all.
    var workshopEntry: WorkshopEntry? {
        let activity: AgentActivity? = appModel.planningAgent.map {
            $0.activity == .waiting ? .waiting : .working
        }
        return buildWorkshopEntry(
            activity: activity,
            isLaunching: appModel.workshopLaunching,
            isSelected: appModel.workshopSelected,
            firstSeen: appModel.activityStore?.firstSeen[AppModel.planSentinel]
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
                                .foregroundStyle(DesignTokens.systemYellow)
                            Text(message)
                                .font(.system(size: Typo.caption))
                                .foregroundStyle(DesignTokens.labelSecondary)
                            Button("Try Again") {
                                Task { await appModel.refresh() }
                            }
                        }
                        .padding(12)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(DesignTokens.controlBg)
                        .cornerRadius(8)
                        .padding(.horizontal, 12)
                    } else if let message = state.errorMessage {
                        HStack(spacing: 6) {
                            Image(systemName: "exclamationmark.triangle")
                                .font(.system(size: Typo.caption))
                            Text("Refresh failed — showing the last plan")
                                .font(.system(size: Typo.caption))
                        }
                        .foregroundStyle(DesignTokens.systemYellow)
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
                            .foregroundStyle(DesignTokens.labelSecondary)
                        Text(EmptyProjectNote.subtitle(needsWorkingDir: appModel.activeProjectNeedsWorkingDir))
                            .font(.system(size: Typo.subhead))
                            .foregroundStyle(DesignTokens.labelTertiary)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    .padding(12)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(DesignTokens.controlBg)
                    .cornerRadius(8)
                    .padding(.horizontal, 12)
                    .padding(.bottom, 10)
                }

                // WORKSHOP section — the planning agent, live or launching,
                // above the slice sessions: it is about the plan the whole
                // rail draws rather than any one slice of it.
                if let workshop = workshopEntry {
                    sectionHeading("WORKSHOP", icon: "wand.and.stars")

                    workshopRow(workshop)
                        .contentShape(Rectangle())
                        .onTapGesture {
                            appModel.workshopSelected = true
                        }
                }

                // NEEDS REVIEW section
                if !railModel.needsReview.isEmpty {
                    sectionHeading("NEEDS REVIEW", icon: "checkmark.seal")
                        .padding(.top, workshopEntry == nil ? 0 : 16)

                    ForEach(railModel.needsReview, id: \.sliceID) { entry in
                        reviewRow(for: entry)
                            .contentShape(Rectangle())
                            .onTapGesture {
                                appModel.selectedSliceID = entry.sliceID
                            }
                    }
                }

                // ACTIVE section
                if !railModel.active.isEmpty {
                    sectionHeading("ACTIVE", icon: "bolt")
                        .padding(.top, railModel.needsReview.isEmpty && workshopEntry == nil ? 0 : 16)

                    ForEach(railModel.active, id: \.sliceID) { entry in
                        activeRow(for: entry)
                            .contentShape(Rectangle())
                            .onTapGesture {
                                appModel.selectedSliceID = entry.sliceID
                            }
                    }
                }

                // The rule under the flight sections exists only where they
                // do — an empty board opening with a bare line would read as
                // chrome missing its content.
                if workshopEntry != nil || !railModel.needsReview.isEmpty || !railModel.active.isEmpty {
                    Divider()
                        .frame(height: 0.5)
                        .foregroundStyle(DesignTokens.separator)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 10)
                }

                // TODO — the milestones still holding work, folders in a
                // file tree with their remaining slices as files.
                if !railModel.todoFolders.isEmpty {
                    sectionHeading("TODO", icon: "list.bullet")
                        // Same "is this the first thing on the rail" zero as
                        // the sections above — TODO only needs extra air when
                        // it lands under the flight-sections divider.
                        .padding(.top, workshopEntry == nil && railModel.needsReview.isEmpty
                            && railModel.active.isEmpty ? 0 : 6)
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
                    Divider()
                        .frame(height: 0.5)
                        .foregroundStyle(DesignTokens.separator)
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
        .background(DesignTokens.windowBg)
        .rectBorderTrailing(width: 0.5, color: DesignTokens.separator)
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
                .foregroundStyle(DesignTokens.labelTertiary)

            Text(title)
                .font(.system(size: Typo.caption, weight: .semibold))
                .foregroundStyle(DesignTokens.labelTertiary)

            Spacer()
        }
        .padding(.leading, RailSlot.leading)
        .padding(.trailing, RailSlot.trailing)
        .padding(.bottom, 5)
    }

    private func doneHeadingRow(_ summary: DoneSummary) -> some View {
        HStack(spacing: RailSlot.spacing) {
            Image(systemName: expandedDoneSummary ? "chevron.down" : "chevron.right")
                .font(.system(size: 11, weight: .bold))
                .frame(width: RailSlot.slot)
                .foregroundStyle(DesignTokens.labelTertiary)

            Text("DONE")
                .font(.system(size: Typo.caption, weight: .semibold))
                .foregroundStyle(DesignTokens.labelTertiary)

            Spacer()

            Text("\(summary.doneCount)/\(summary.totalCount)")
                .font(.system(size: Typo.subhead, weight: .regular))
                .monospacedDigit()
                .foregroundStyle(DesignTokens.labelTertiary)
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
        dotColor: Color,
        pulsing: Bool,
        name: String,
        meta: String?,
        metaColor: Color,
        detail: [(String, Color)]
    ) -> some View {
        HStack(alignment: .top, spacing: RailSlot.spacing) {
            dotView(color: dotColor, pulsing: pulsing && !selected)
                .frame(width: RailSlot.slot, height: 19)

            VStack(alignment: .leading, spacing: 1) {
                HStack(alignment: .firstTextBaseline, spacing: RailSlot.spacing) {
                    Text(name)
                        .font(.system(size: Typo.body, weight: .regular))
                        .foregroundStyle(DesignTokens.label)
                        .lineLimit(1)

                    Spacer(minLength: 0)

                    if let meta {
                        Text(meta)
                            .font(.system(size: Typo.subhead, weight: .regular))
                            .monospacedDigit()
                            .foregroundStyle(metaColor)
                    }
                }

                HStack(spacing: 5) {
                    ForEach(Array(detail.enumerated()), id: \.offset) { index, piece in
                        if index > 0 {
                            Text("·")
                                .foregroundStyle(DesignTokens.labelTertiary)
                        }
                        Text(piece.0)
                            .foregroundStyle(piece.1)
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
                .fill(selected ? DesignTokens.selectionWash : Color.clear)
                .padding(.horizontal, 6)
        }
        .insetHoverWash()
    }

    private func reviewRow(for entry: ReviewEntry) -> some View {
        var detail: [(String, Color)] = []
        if !entry.milestone.isEmpty {
            detail.append((entry.milestone, DesignTokens.labelTertiary))
        }
        if let fileCount = entry.fileCount {
            detail.append(("\(fileCount) file\(fileCount == 1 ? "" : "s")", DesignTokens.labelTertiary))
        }
        return sessionRow(
            selected: appModel.selectedSliceID == entry.sliceID,
            dotColor: DesignTokens.systemGreen,
            pulsing: false,
            name: entry.name,
            meta: entry.stat,
            metaColor: DesignTokens.systemGreen,
            detail: detail
        )
    }

    private func workshopRow(_ entry: WorkshopEntry) -> some View {
        let tint = workshopTint(for: entry.tintRole)
        // A launching row sits still the way a blocked ACTIVE row does; only
        // a live agent pulses.
        let isLive = entry.tintRole == .working || entry.tintRole == .waiting

        return sessionRow(
            selected: appModel.workshopSelected,
            dotColor: tint,
            pulsing: isLive,
            name: "Planning agent",
            meta: entry.elapsed,
            metaColor: DesignTokens.labelTertiary,
            detail: [(entry.displayState, tint)]
        )
    }

    private func workshopTint(for role: WorkshopTintRole) -> Color {
        switch role {
        case .working: return DesignTokens.systemOrange
        case .waiting: return DesignTokens.systemYellow
        case .launching, .new: return DesignTokens.labelTertiary
        }
    }

    private func activeRow(for entry: ActiveEntry) -> some View {
        let tint = tintColor(for: entry.tintRole)
        // Only a live agent is worth pulling the eye to; a row with nothing
        // running on it (blocked, or simply ready to push) sits still.
        let isLive = entry.tintRole == .working || entry.tintRole == .waiting

        var detail: [(String, Color)] = [(entry.displayState, tint)]
        if !entry.milestone.isEmpty {
            detail.append((entry.milestone, DesignTokens.labelTertiary))
        }
        return sessionRow(
            selected: appModel.selectedSliceID == entry.sliceID,
            dotColor: tint,
            pulsing: isLive,
            name: entry.name,
            meta: entry.elapsed,
            metaColor: DesignTokens.labelTertiary,
            detail: detail
        )
    }

    @ViewBuilder
    private func dotView(color: Color, pulsing: Bool) -> some View {
        if pulsing {
            Circle()
                .fill(color)
                .frame(width: 9, height: 9)
                .modifier(PulseModifier())
        } else {
            Circle()
                .fill(color)
                .frame(width: 9, height: 9)
        }
    }

    private func tintColor(for role: ActiveTintRole) -> Color {
        switch role {
        case .working: return DesignTokens.systemOrange
        case .waiting: return DesignTokens.systemYellow
        case .blocked: return DesignTokens.labelTertiary
        case .readyToPush: return DesignTokens.systemGreen
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
                    .fill(DesignTokens.labelQuaternary)
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
                .fill(
                    inDone
                        ? DesignTokens.labelTertiary
                        : folder.isCurrent ? DesignTokens.accent : DesignTokens.labelSecondary
                )
                .frame(width: RailSlot.slot, height: 10.5)

            Text(folder.title)
                .font(.system(size: Typo.body, weight: folder.isCurrent ? .semibold : .regular))
                .foregroundStyle(inDone ? DesignTokens.labelSecondary : DesignTokens.label)
                .lineLimit(1)

            Spacer()

            if inDone && folder.isComplete {
                Image(systemName: "checkmark")
                    .font(.system(size: 9, weight: .bold))
                    .foregroundStyle(DesignTokens.systemGreen)
            }

            Text("\(folder.done)/\(folder.total)")
                .font(.system(size: Typo.subhead, weight: .regular))
                .monospacedDigit()
                .foregroundStyle(DesignTokens.labelTertiary)
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
        let contentColor = selected
            ? DesignTokens.label
            : (slice.isBlocked ? DesignTokens.labelTertiary : DesignTokens.label)

        return HStack(spacing: RailSlot.spacing) {
            Image(systemName: slice.glyph.rawValue)
                .font(.system(size: 12, weight: .medium))
                .frame(width: RailSlot.slot)
                .foregroundStyle(glyphColor(for: slice.glyph))

            Text(slice.name)
                .font(.system(size: Typo.body, weight: .regular))
                .lineLimit(1)
                .foregroundStyle(contentColor)

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
                .fill(selected ? DesignTokens.selectionWash : Color.clear)
                .padding(.horizontal, 6)
        }
        .insetHoverWash()
    }

    /// The mock's status tints for a slice glyph — in progress orange, done
    /// green, and the rest (todo, blocked) muted; these now show through a
    /// selected row rather than being recolored by it.
    private func glyphColor(for glyph: SliceGlyph) -> Color {
        switch glyph {
        case .todo, .blocked: return DesignTokens.labelTertiary
        case .inProgress: return DesignTokens.systemOrange
        case .done: return DesignTokens.systemGreen
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

#Preview {
    let appModel = AppModel()
    WindowShellView(appModel: appModel)
        .frame(width: 1360, height: 840)
}
